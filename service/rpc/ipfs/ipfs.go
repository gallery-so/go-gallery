package ipfs

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/ipfs/go-ipfs-api"
	"github.com/mikeydub/go-gallery/env"
	"github.com/mikeydub/go-gallery/service/logger"
	"github.com/mikeydub/go-gallery/service/tracing"
	"github.com/mikeydub/go-gallery/util"
	"github.com/mikeydub/go-gallery/util/retry"
)

func init() {
	env.RegisterValidation("IPFS_URL", "required")
	env.RegisterValidation("FALLBACK_IPFS_URL", "required")
}

type ErrInfuraQuotaExceeded struct {
	Err error
}

func (r ErrInfuraQuotaExceeded) Error() string {
	return fmt.Sprintf("quota exceeded: %s", r.Err.Error())
}

func (r ErrInfuraQuotaExceeded) Unwrap() error {
	return r.Err
}

type Reader interface {
	Do(ctx context.Context, path string) (io.ReadCloser, error)
}

// HTTPReader is a reader that uses a HTTP gateway to read from
type HTTPReader struct {
	Host   string
	Client http.Client
}

func (r HTTPReader) Do(ctx context.Context, path string) (io.ReadCloser, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, pathURL(r.Host, path), nil)
	if err != nil {
		return nil, err
	}

	resp, err := r.Client.Do(req)
	if err != nil {
		return nil, err
	}

	if isInfura(path) && resp.StatusCode == http.StatusTooManyRequests {
		return nil, ErrInfuraQuotaExceeded{Err: err}
	}

	if resp.StatusCode != http.StatusOK {
		return nil, util.ErrHTTP{
			Status: resp.StatusCode,
			URL:    path,
		}
	}

	return resp.Body, nil
}

// IPFSReader is a reader that uses an IPFS shell to read from IPFS
type IPFSReader struct {
	Client *shell.Shell
}

func (r IPFSReader) Do(ctx context.Context, path string) (io.ReadCloser, error) {
	reader, err := r.Client.Cat(path)

	if err != nil && isInfura(path) && strings.Contains(err.Error(), "transfer quota reached") {
		return nil, ErrInfuraQuotaExceeded{Err: err}
	}

	if err != nil {
		return nil, err
	}

	return reader, nil
}

// NewShell returns an IPFS shell with default configuration
func NewShell() *shell.Shell {
	sh := shell.NewShellWithClient(env.GetString("IPFS_API_URL"), defaultHTTPClient())
	sh.SetTimeout(600 * time.Second)
	return sh
}

func GetIPFSResponse(ctx context.Context, httpClient http.Client, ipfsClient *shell.Shell, path string) (io.ReadCloser, error) {
	infuraReader := HTTPReader{
		Host:   env.GetString("IPFS_URL"),
		Client: httpClient,
	}
	galleryReader := HTTPReader{
		Host:   env.GetString("FALLBACK_IPFS_URL"),
		Client: httpClient,
	}
	ipfsReader := IPFSReader{
		Client: ipfsClient,
	}

	logStatus := func(readerName, path string, err error) {
		if err == nil {
			logger.For(ctx).Infof("read CID: %s using [%s]", path, readerName)
		} else {
			logger.For(ctx).Warnf("failed to read CID: %s with [%s]: %s", path, readerName, err)
		}
	}

	r, err := util.FirstNonErrorWithValue(ctx, false, retry.HTTPErrNotFound,
		func(ctx context.Context) (io.ReadCloser, error) {
			r, err := infuraReader.Do(ctx, path)
			logStatus("infuraNode", path, err)
			return r, err
		},
		func(ctx context.Context) (io.ReadCloser, error) {
			r, err := galleryReader.Do(ctx, path)
			logStatus("galleryNode", path, err)
			return r, err
		},
		func(ctx context.Context) (io.ReadCloser, error) {
			r, err := ipfsReader.Do(ctx, path)
			logStatus("ipfsNode", path, err)
			return r, err
		},
	)

	if err == nil {
		return r, nil
	}

	logger.For(ctx).Warnf("failed to read CID: %s from any node, using fallback: %s", path, err)
	return HTTPReader{Host: "https://ipfs.io", Client: httpClient}.Do(ctx, path)
}

// defaultHTTPClient returns an http.Client configured with default settings intended for IPFS calls.
func defaultHTTPClient() *http.Client {
	return &http.Client{
		Timeout: 600 * time.Second,
		Transport: authTransport{
			RoundTripper:  tracing.NewTracingTransport(http.DefaultTransport, false),
			ProjectID:     env.GetString("IPFS_PROJECT_ID"),
			ProjectSecret: env.GetString("IPFS_PROJECT_SECRET"),
		},
	}
}

// authTransport decorates each request with a basic auth header.
type authTransport struct {
	http.RoundTripper
	ProjectID     string
	ProjectSecret string
}

func (t authTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	r.SetBasicAuth(t.ProjectID, t.ProjectSecret)
	return t.RoundTripper.RoundTrip(r)
}

// pathURL returns the gateway URL in path resolution sytle
func pathURL(host, path string) string {
	return fmt.Sprintf("%s/ipfs/%s", host, path)
}

func isInfura(gateway string) bool {
	return strings.Contains(gateway, "infura")
}
