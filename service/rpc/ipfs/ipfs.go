package ipfs

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/ipfs/go-ipfs-api"
	"github.com/mikeydub/go-gallery/env"
	"github.com/mikeydub/go-gallery/service/tracing"
	"github.com/mikeydub/go-gallery/util"
)

func init() {
	env.RegisterValidation("IPFS_URL", "required")
	env.RegisterValidation("FALLBACK_IPFS_URL", "required")
}

type ErrInfuraQuotaExceeded struct {
	Err error
}

func (r ErrInfuraQuotaExceeded) Unwrap() error { return r.Err }
func (r ErrInfuraQuotaExceeded) Error() string {
	return fmt.Sprintf("quota exceeded: %s", r.Err.Error())
}

// HTTPReader is a reader that uses a HTTP gateway to read from
type HTTPReader struct {
	Host   string
	Client *http.Client
}

func (r HTTPReader) Do(ctx context.Context, path string) (io.ReadCloser, error) {
	path = pathURL(r.Host, path)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, path, nil)
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
		return nil, util.ErrHTTP{Status: resp.StatusCode, URL: path}
	}
	return resp.Body, nil
}

func (r HTTPReader) Head(ctx context.Context, path string) (http.Header, error) {
	path = pathURL(r.Host, path)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}
	resp, err := r.Client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, util.ErrHTTP{Status: resp.StatusCode, URL: path}
	}
	return resp.Header, nil
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
	return reader, err
}

// NewShell returns an IPFS shell with default configuration
func NewShell() *shell.Shell {
	sh := shell.NewShellWithClient(env.GetString("IPFS_API_URL"), defaultHTTPClient())
	sh.SetTimeout(600 * time.Second)
	return sh
}

var (
	nodeGallery = func(h *http.Client, s *shell.Shell) HTTPReader {
		return HTTPReader{Host: env.GetString("FALLBACK_IPFS_URL"), Client: h}
	}
	nodeIPFS = func(h *http.Client, s *shell.Shell) IPFSReader {
		return IPFSReader{Client: s}
	}
	nodeIpfsIO = func(h *http.Client, s *shell.Shell) HTTPReader {
		return HTTPReader{Host: "https://ipfs.io", Client: h}
	}
	nodePinata = func(h *http.Client, s *shell.Shell) HTTPReader {
		return HTTPReader{Host: "https://gateway.pinata.cloud", Client: h}
	}
	nodeNftStorage = func(h *http.Client, s *shell.Shell) HTTPReader {
		return HTTPReader{Host: "https://nftstorage.link", Client: h}
	}
	nodeCloudFlare = func(h *http.Client, s *shell.Shell) HTTPReader {
		return HTTPReader{Host: "https://cloudflare-ipfs.com", Client: h}
	}
	nodeFxHash = func(h *http.Client, s *shell.Shell) HTTPReader {
		return HTTPReader{Host: "https://gateway.fxhash.xyz", Client: h}
	}
)

func GetResponse(ctx context.Context, path string) (io.ReadCloser, error) {
	httpClient := defaultHTTPClient()
	ipfsClient := NewShell()
	return util.FirstNonErrorWithValue(ctx, false, nil,
		func(ctx context.Context) (io.ReadCloser, error) {
			return nodeGallery(httpClient, ipfsClient).Do(ctx, path)
		},
		func(ctx context.Context) (io.ReadCloser, error) {
			return nodeIPFS(httpClient, ipfsClient).Do(ctx, path)
		},
		func(ctx context.Context) (io.ReadCloser, error) {
			return nodeIpfsIO(httpClient, ipfsClient).Do(ctx, path)
		},
		func(ctx context.Context) (io.ReadCloser, error) {
			return nodePinata(httpClient, ipfsClient).Do(ctx, path)
		},
		func(ctx context.Context) (io.ReadCloser, error) {
			return nodeNftStorage(httpClient, ipfsClient).Do(ctx, path)
		},
		func(ctx context.Context) (io.ReadCloser, error) {
			return nodeCloudFlare(httpClient, ipfsClient).Do(ctx, path)
		},
	)
}

func GetHeader(ctx context.Context, path string) (http.Header, error) {
	httpClient := defaultHTTPClient()
	ipfsClient := NewShell()
	return util.FirstNonErrorWithValue(ctx, true, nil,
		func(ctx context.Context) (http.Header, error) {
			return nodeGallery(httpClient, ipfsClient).Head(ctx, path)
		},
		func(ctx context.Context) (http.Header, error) {
			return nodeIpfsIO(httpClient, ipfsClient).Head(ctx, path)
		},
		func(ctx context.Context) (http.Header, error) {
			return nodePinata(httpClient, ipfsClient).Head(ctx, path)
		},
		func(ctx context.Context) (http.Header, error) {
			return nodeNftStorage(httpClient, ipfsClient).Head(ctx, path)
		},
		func(ctx context.Context) (http.Header, error) {
			return nodeCloudFlare(httpClient, ipfsClient).Head(ctx, path)
		},
	)
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

// BestGatewayNodeFrom rewrites an IPFS URL to a gateway URL using the appropriate gateway
func BestGatewayNodeFrom(ipfsURL string, isFxHash bool) string {
	if !IsIpfsURL(ipfsURL) {
		return ipfsURL
	}
	// Use fxhash's specific node
	if isFxHash {
		return PathGatewayFrom(nodeFxHash(nil, nil).Host, ipfsURL)
	}
	// Re-write to a more reliable one while Infura node is down
	if strings.HasPrefix(ipfsURL, "https://gallery.infura-ipfs.io") {
		return DefaultGatewayFrom(ipfsURL)
	}
	// Keep the original gateway otherwise
	if IsIpfsGatewayURL(ipfsURL) {
		return ipfsURL
	}
	// Map ipfs:// to the default gateway
	return DefaultGatewayFrom(ipfsURL)
}

// DefaultGatewayFrom rewrites an IPFS URL to a gateway URL using the default gateway
func DefaultGatewayFrom(ipfsURL string) string {
	return PathGatewayFrom(nodeIpfsIO(nil, nil).Host, ipfsURL)
}

// PathGatewayFrom is a helper function that rewrites an IPFS URI to an IPFS gateway URL
func PathGatewayFrom(gatewayHost, ipfsURL string) string {
	return pathURL(gatewayHost, util.GetURIPath(ipfsURL, false))
}

func IsIpfsURL(ipfsURL string) bool {
	return IsIpfsProtoURL(ipfsURL) || IsIpfsGatewayURL(ipfsURL)
}

func IsIpfsProtoURL(ipfsURL string) bool {
	return strings.HasPrefix(ipfsURL, "ipfs://")
}

func IsIpfsGatewayURL(ipfsURL string) bool {
	u, err := url.Parse(ipfsURL)
	if err != nil {
		return false
	}
	if u.Scheme != "https" {
		return false
	}
	return strings.HasPrefix(u.Path, "/ipfs")
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
func pathURL(host, uri string) string {
	uri = standardizeQueryParams(uri)
	return fmt.Sprintf("%s/ipfs/%s", host, uri)
}

// standardizeQueryParams converts a URI with optional params from the format <cid>?key=val&key=val to the format <cid>/?key=val&key=val
// Most gateways will redirect the former to the latter, but some gateways don't. https://docs.ipfs.tech/concepts/ipfs-gateway/#path
func standardizeQueryParams(uri string) string {
	paramIdx := strings.Index(uri, "?")
	isClean := strings.Contains(uri, "/?")
	if paramIdx != -1 && !isClean {
		uri = uri[:paramIdx] + "/?" + uri[paramIdx+1:]
	}
	return uri
}

func isInfura(gateway string) bool {
	return strings.Contains(gateway, "infura")
}
