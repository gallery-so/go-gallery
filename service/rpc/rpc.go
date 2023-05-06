package rpc

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image/jpeg"
	"image/png"
	"io"
	"io/fs"
	"io/ioutil"
	"math/big"
	"net"
	"net/http"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"cloud.google.com/go/storage"
	"golang.org/x/image/bmp"
	"google.golang.org/api/option"

	"github.com/getsentry/sentry-go"
	"github.com/googleapis/gax-go/v2"
	"github.com/mikeydub/go-gallery/env"
	"github.com/mikeydub/go-gallery/service/logger"
	"github.com/mikeydub/go-gallery/service/tracing"
	"github.com/mikeydub/go-gallery/util/retry"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/everFinance/goar"
	goartypes "github.com/everFinance/goar/types"
	shell "github.com/ipfs/go-ipfs-api"
	"github.com/mikeydub/go-gallery/contracts"
	"github.com/mikeydub/go-gallery/service/persist"
	sentryutil "github.com/mikeydub/go-gallery/service/sentry"
	"github.com/mikeydub/go-gallery/util"
	htransport "google.golang.org/api/transport/http"
)

func init() {
	env.RegisterValidation("IPFS_URL", "required")
	env.RegisterValidation("FALLBACK_IPFS_URL", "required")
}

const (
	defaultHTTPTimeout             = 600
	defaultHTTPKeepAlive           = 600
	defaultHTTPMaxIdleConns        = 100
	defaultHTTPMaxIdleConnsPerHost = 100
	GethSocketOpName               = "geth.wss"
)

var (
	defaultHTTPClient     = newHTTPClientForRPC(true)
	defaultMetricsHandler = metricsHandler{}
)

// rateLimited is the content returned from an RPC call when rate limited.
var rateLimited = "429 Too Many Requests"

type ErrEthClient struct {
	Err error
}

type ErrTokenURINotFound struct {
	Err error
}

func (e ErrEthClient) Error() string {
	return e.Err.Error()
}

func (e ErrTokenURINotFound) Error() string {
	return e.Err.Error()
}

// Transfer represents a Transfer from the RPC response
type Transfer struct {
	BlockNumber     persist.BlockNumber
	From            persist.EthereumAddress
	To              persist.EthereumAddress
	TokenID         persist.TokenID
	TokenType       persist.TokenType
	Amount          uint64
	ContractAddress persist.EthereumAddress
	// These are geth types which are useful for getting more details about a transaction.
	TxHash    common.Hash
	BlockHash common.Hash
	TxIndex   uint
}

// TokenContractMetadata represents a token contract's metadata
type TokenContractMetadata struct {
	Name   string
	Symbol string
}

// NewEthClient returns an ethclient.Client
func NewEthClient() *ethclient.Client {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	rpcClient, err := rpc.DialContext(ctx, env.GetString("RPC_URL"))
	if err != nil {
		panic(err)
	}

	return ethclient.NewClient(rpcClient)

}

// NewEthHTTPClient returns a new http client with request tracing enabled
func NewEthHTTPClient() *ethclient.Client {
	if !strings.HasPrefix(env.GetString("RPC_URL"), "http") {
		return NewEthClient()
	}

	httpClient := newHTTPClientForRPC(false, sentryutil.TransactionNameSafe("gethRPC"))
	rpcClient, err := rpc.DialHTTPWithClient(env.GetString("RPC_URL"), httpClient)
	if err != nil {
		panic(err)
	}

	return ethclient.NewClient(rpcClient)
}

// NewEthSocketClient returns a new websocket client with request tracing enabled
func NewEthSocketClient() *ethclient.Client {
	if !strings.HasPrefix(env.GetString("RPC_URL"), "wss") {
		return NewEthClient()
	}

	log.Root().SetHandler(log.FilterHandler(func(r *log.Record) bool {
		if reqID := valFromSlice(r.Ctx, "reqid"); reqID == nil || r.Msg != "Handled RPC response" {
			return false
		}
		return true
	}, defaultMetricsHandler))

	return NewEthClient()
}

func NewStorageClient(ctx context.Context) *storage.Client {
	opts := append([]option.ClientOption{}, option.WithScopes([]string{storage.ScopeFullControl}...))

	if env.GetString("ENV") == "local" {
		fi, err := util.LoadEncryptedServiceKeyOrError("./secrets/dev/service-key-dev.json")
		if err != nil {
			logger.For(ctx).WithError(err).Error("failed to find service key file (local), running without storage client")
			return nil
		}
		opts = append(opts, option.WithCredentialsJSON(fi))
	}

	transport, err := htransport.NewTransport(ctx, tracing.NewTracingTransport(http.DefaultTransport, false), opts...)
	if err != nil {
		panic(err)
	}

	client, _, err := htransport.NewClient(ctx)
	if err != nil {
		panic(err)
	}
	client.Transport = transport

	storageClient, err := storage.NewClient(ctx, option.WithHTTPClient(client))
	if err != nil {
		panic(err)
	}

	storageClient.SetRetry(storage.WithPolicy(storage.RetryAlways), storage.WithBackoff(gax.Backoff{Initial: 100 * time.Millisecond, Max: 2 * time.Minute, Multiplier: 1.3}), storage.WithErrorFunc(storage.ShouldRetry))

	return storageClient
}

// metricsHandler traces RPC records that get logged by the RPC client
type metricsHandler struct{}

// Log sends trace information to Sentry.
// Geth logs each response it receives in the handleImmediate method of handler: https://github.com/ethereum/go-ethereum/blob/master/rpc/handler.go
// We take advantage of this by configuring the client's root logger with a custom handler that sends a span to Sentry each time we get the log message.
func (h metricsHandler) Log(r *log.Record) error {
	reqID := valFromSlice(r.Ctx, "reqid")

	// A useful context isn't passed to the log record, so we use the background context here.
	ctx := context.Background()
	span, _ := tracing.StartSpan(ctx, GethSocketOpName, "rpcCall", sentryutil.TransactionNameSafe("gethRPC"))
	tracing.AddEventDataToSpan(span, map[string]interface{}{"reqID": reqID})
	defer tracing.FinishSpan(span)

	// Fix the duration to 100ms because there isn't a useful duration to use
	span.EndTime = r.Time
	span.StartTime = r.Time.Add(time.Millisecond * -100)

	return nil
}

// NewIPFSShell returns an IPFS shell
func NewIPFSShell() *shell.Shell {
	sh := shell.NewShellWithClient(env.GetString("IPFS_API_URL"), newClientForIPFS(env.GetString("IPFS_PROJECT_ID"), env.GetString("IPFS_PROJECT_SECRET"), false))
	sh.SetTimeout(defaultHTTPTimeout * time.Second)
	return sh
}

// newHTTPClientForIPFS returns an http.Client configured with default settings intended for IPFS calls.
func newClientForIPFS(projectID, projectSecret string, continueOnly bool) *http.Client {
	return &http.Client{

		Timeout: defaultHTTPTimeout * time.Second,
		Transport: authTransport{
			RoundTripper:  tracing.NewTracingTransport(http.DefaultTransport, continueOnly),
			ProjectID:     projectID,
			ProjectSecret: projectSecret,
		},
	}
}

// newHTTPClientForRPC returns an http.Client configured with default settings intended for RPC calls.
func newHTTPClientForRPC(continueTrace bool, spanOptions ...sentry.SpanOption) *http.Client {
	// get x509 cert pool
	pool, err := x509.SystemCertPool()
	if err != nil {
		panic(err)
	}

	// walk every file in the tls directory and add them to the cert pool
	filepath.WalkDir("root-certs", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		bs, err := ioutil.ReadFile(path)
		if err != nil {
			return err
		}

		// append cert to pool
		ok := pool.AppendCertsFromPEM(bs)
		if !ok {
			return fmt.Errorf("failed to append cert to pool")
		}
		return nil
	})

	return &http.Client{
		Timeout: 0,
		Transport: tracing.NewTracingTransport(&http.Transport{
			TLSClientConfig: &tls.Config{
				RootCAs: pool,
			},
			Dial:                (&net.Dialer{KeepAlive: defaultHTTPKeepAlive * time.Second}).Dial,
			MaxIdleConns:        defaultHTTPMaxIdleConns,
			MaxIdleConnsPerHost: defaultHTTPMaxIdleConnsPerHost,
		}, continueTrace, spanOptions...),
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

// NewArweaveClient returns an Arweave client
func NewArweaveClient() *goar.Client {
	return goar.NewClient("https://arweave.net")
}

// GetBlockNumber returns the current block height.
func GetBlockNumber(ctx context.Context, ethClient *ethclient.Client) (uint64, error) {
	return ethClient.BlockNumber(ctx)
}

// RetryGetBlockNumber calls GetBlockNumber with backoff.
func RetryGetBlockNumber(ctx context.Context, ethClient *ethclient.Client) (uint64, error) {
	var height uint64
	var err error
	for i := 0; i < retry.DefaultRetry.Tries; i++ {
		height, err = GetBlockNumber(ctx, ethClient)
		if !isRateLimitedError(err) {
			break
		}
		retry.DefaultRetry.Sleep(i)
	}
	return height, err
}

// GetLogs returns log events for the given block range and query.
func GetLogs(ctx context.Context, ethClient *ethclient.Client, query ethereum.FilterQuery) ([]types.Log, error) {
	return ethClient.FilterLogs(ctx, query)
}

// RetryGetLogs calls GetLogs with backoff.
func RetryGetLogs(ctx context.Context, ethClient *ethclient.Client, query ethereum.FilterQuery) ([]types.Log, error) {
	logs := make([]types.Log, 0)
	var err error
	for i := 0; i < retry.DefaultRetry.Tries; i++ {
		logs, err = GetLogs(ctx, ethClient, query)
		if !isRateLimitedError(err) {
			break
		}
		retry.DefaultRetry.Sleep(i)
	}
	return logs, err
}

// GetTransaction returns the transaction of the given hash.
func GetTransaction(ctx context.Context, ethClient *ethclient.Client, txHash common.Hash) (*types.Transaction, bool, error) {
	return ethClient.TransactionByHash(ctx, txHash)
}

// RetryGetTransaction calls GetTransaction with backoff.
func RetryGetTransaction(ctx context.Context, ethClient *ethclient.Client, txHash common.Hash, retry retry.Retry) (*types.Transaction, bool, error) {
	var tx *types.Transaction
	var pending bool
	var err error
	for i := 0; i < retry.Tries; i++ {
		tx, pending, err = GetTransaction(ctx, ethClient, txHash)
		if !isRateLimitedError(err) {
			break
		}
		retry.Sleep(i)
	}
	return tx, pending, err
}

// GetTokenContractMetadata returns the metadata for a given contract (without URI)
func GetTokenContractMetadata(ctx context.Context, address persist.EthereumAddress, ethClient *ethclient.Client) (*TokenContractMetadata, error) {
	contract := address.Address()
	instance, err := contracts.NewIERC721MetadataCaller(contract, ethClient)
	if err != nil {
		return nil, err
	}

	name, err := instance.Name(&bind.CallOpts{
		Context: ctx,
	})
	if err != nil {
		return nil, err
	}
	symbol, err := instance.Symbol(&bind.CallOpts{
		Context: ctx,
	})
	if err != nil {
		return nil, err
	}

	return &TokenContractMetadata{Name: name, Symbol: symbol}, nil
}

// RetryGetTokenContractMetaData calls GetTokenContractMetadata with backoff.
func RetryGetTokenContractMetadata(ctx context.Context, contractAddress persist.EthereumAddress, ethClient *ethclient.Client) (*TokenContractMetadata, error) {
	var metadata *TokenContractMetadata
	var err error
	for i := 0; i < retry.DefaultRetry.Tries; i++ {
		metadata, err = GetTokenContractMetadata(ctx, contractAddress, ethClient)
		if !isRateLimitedError(err) {
			break
		}
		retry.DefaultRetry.Sleep(i)
	}
	return metadata, err
}

// GetMetadataFromURI parses and returns the NFT metadata for a given token URI
func GetMetadataFromURI(ctx context.Context, turi persist.TokenURI, ipfsClient *shell.Shell, arweaveClient *goar.Client) (persist.TokenMetadata, error) {

	var meta persist.TokenMetadata
	err := DecodeMetadataFromURI(ctx, turi, &meta, ipfsClient, arweaveClient)
	if err != nil {
		return nil, err
	}

	return meta, nil

}

// GetDataFromURI calls URI and returns the data
func GetDataFromURI(ctx context.Context, turi persist.TokenURI, ipfsClient *shell.Shell, arweaveClient *goar.Client) ([]byte, error) {

	d, _ := ctx.Deadline()
	logger.For(ctx).Infof("Getting data from URI: %s -timeout: %s -type: %s", turi.String(), time.Until(d), turi.Type())
	asString := turi.String()

	switch turi.Type() {
	case persist.URITypeBase64JSON, persist.URITypeBase64SVG:
		// decode the base64 encoded json
		b64data := asString[strings.IndexByte(asString, ',')+1:]
		decoded, err := base64.RawStdEncoding.DecodeString(string(b64data))
		if err != nil {
			decoded, err = base64.StdEncoding.DecodeString(string(b64data))
			if err != nil {
				return nil, fmt.Errorf("error decoding base64 data: %s \n\n%s", err, b64data)
			}
		}

		return util.RemoveBOM(decoded), nil
	case persist.URITypeIPFS, persist.URITypeIPFSGateway:
		path := util.GetURIPath(asString, true)

		bs, err := GetIPFSData(ctx, ipfsClient, path)
		if err != nil {
			return nil, err
		}

		return util.RemoveBOM(bs), nil
	case persist.URITypeArweave, persist.URITypeArweaveGateway:
		path := util.GetURIPath(asString, true)

		bs, err := util.FirstNonErrorWithValue(ctx, func() ([]byte, error) { return GetArweaveData(arweaveClient, path) }, func() ([]byte, error) { return GetArweaveDataHTTP(ctx, path) })
		if err != nil {
			return nil, err
		}
		return util.RemoveBOM(bs), nil
	case persist.URITypeHTTP:

		req, err := http.NewRequestWithContext(ctx, "GET", asString, nil)
		if err != nil {
			return nil, fmt.Errorf("error creating request: %s", err)
		}
		resp, err := defaultHTTPClient.Do(req)
		if err != nil {
			if dnsErr, ok := err.(*net.DNSError); ok {
				return nil, dnsErr
			}
			if urlErr, ok := err.(*url.Error); ok {
				return nil, urlErr
			}
			if resp != nil {
				return nil, util.ErrHTTP{Err: err, Status: resp.StatusCode, URL: asString}
			}
			return nil, util.ErrHTTP{Err: err, URL: asString}
		}
		defer resp.Body.Close()
		if resp.StatusCode > 399 || resp.StatusCode < 200 {
			return nil, util.ErrHTTP{Status: resp.StatusCode, URL: asString}
		}
		buf := &bytes.Buffer{}
		err = util.CopyMax(buf, resp.Body, 1024*1024*1024)
		if err != nil {
			return nil, fmt.Errorf("error getting data from http: %s - %s", err, asString)
		}

		return util.RemoveBOM(buf.Bytes()), nil
	case persist.URITypeIPFSAPI:
		parsedURL, err := url.Parse(asString)
		if err != nil {
			return nil, err
		}
		path := parsedURL.Query().Get("arg")
		bs, err := GetIPFSData(ctx, ipfsClient, path)
		if err != nil {
			return nil, err
		}

		return util.RemoveBOM(bs), nil
	case persist.URITypeJSON, persist.URITypeSVG:
		idx := strings.IndexByte(asString, ',')
		if idx == -1 {
			return util.RemoveBOM([]byte(asString)), nil
		}
		return util.RemoveBOM([]byte(asString[idx+1:])), nil
	case persist.URITypeBase64BMP:
		b64data := asString[strings.IndexByte(asString, ',')+1:]
		decoded, err := base64.RawStdEncoding.DecodeString(string(b64data))
		if err != nil {
			decoded, err = base64.StdEncoding.DecodeString(string(b64data))
			if err != nil {
				return nil, fmt.Errorf("error decoding base64 bmp data: %s \n\n%s", err, b64data)
			}
		}
		img, err := bmp.Decode(bytes.NewReader(decoded))
		if err != nil {
			return nil, fmt.Errorf("error decoding bmp data: %s \n\n%s", err, b64data)
		}
		newImage := &bytes.Buffer{}
		err = jpeg.Encode(newImage, img, nil)
		if err != nil {
			return nil, fmt.Errorf("error encoding jpeg data: %s \n\n%s", err, b64data)
		}
		return util.RemoveBOM(newImage.Bytes()), nil
	case persist.URITypeBase64PNG:
		b64data := asString[strings.IndexByte(asString, ',')+1:]
		decoded, err := base64.RawStdEncoding.DecodeString(string(b64data))
		if err != nil {
			decoded, err = base64.StdEncoding.DecodeString(string(b64data))
			if err != nil {
				return nil, fmt.Errorf("error decoding base64 png data: %s \n\n%s", err, b64data)
			}
		}
		img, err := png.Decode(bytes.NewReader(decoded))
		if err != nil {
			return nil, fmt.Errorf("error decoding png data: %s \n\n%s", err, b64data)
		}
		newImage := &bytes.Buffer{}
		err = png.Encode(newImage, img)
		if err != nil {
			return nil, fmt.Errorf("error encoding jpeg data: %s \n\n%s", err, b64data)
		}
		return util.RemoveBOM(newImage.Bytes()), nil
	default:
		return []byte(turi), nil
	}

}

// GetDataFromURIAsReader calls URI and returns the data as an unread reader with the headers pre-read
func GetDataFromURIAsReader(ctx context.Context, turi persist.TokenURI, ipfsClient *shell.Shell, arweaveClient *goar.Client) (*util.FileHeaderReader, error) {

	d, _ := ctx.Deadline()
	logger.For(ctx).Infof("Getting data from URI: %s -timeout: %s -type: %s", turi.String(), time.Until(d), turi.Type())
	asString := turi.String()

	switch turi.Type() {
	case persist.URITypeBase64JSON, persist.URITypeBase64SVG:
		// decode the base64 encoded json
		b64data := asString[strings.IndexByte(asString, ',')+1:]
		decoded, err := base64.RawStdEncoding.DecodeString(string(b64data))
		if err != nil {
			decoded, err = base64.StdEncoding.DecodeString(string(b64data))
			if err != nil {
				return nil, fmt.Errorf("error decoding base64 data: %s \n\n%s", err, b64data)
			}
		}

		buf := bytes.NewBuffer(util.RemoveBOM(decoded))

		return util.NewFileHeaderReader(buf), nil
	case persist.URITypeIPFS:
		path := util.GetURIPath(asString, true)
		resp, err := GetIPFSResponse(ctx, ipfsClient, path)
		if err != nil {
			return nil, err
		}

		return util.NewFileHeaderReader(resp), nil
	case persist.URITypeArweave, persist.URITypeArweaveGateway:
		path := util.GetURIPath(asString, true)

		bs, err := GetArweaveData(arweaveClient, path)
		if err != nil {
			resp, err := GetArweaveDataHTTPReader(ctx, path)
			if err != nil {
				return nil, err
			}
			return util.NewFileHeaderReader(resp), nil
		}
		buf := bytes.NewBuffer(util.RemoveBOM(bs))
		return util.NewFileHeaderReader(buf), nil
	case persist.URITypeIPFSGateway:
		path := util.GetURIPath(asString, false)
		resp, err := GetIPFSResponse(ctx, ipfsClient, path)
		if err != nil {
			logger.For(ctx).Errorf("Error getting data from IPFS: %s", err)
		} else {
			return util.NewFileHeaderReader(resp), nil
		}
		fallthrough
	case persist.URITypeHTTP:
		req, err := http.NewRequestWithContext(ctx, "GET", asString, nil)
		if err != nil {
			return nil, fmt.Errorf("error creating request: %s", err)
		}
		resp, err := defaultHTTPClient.Do(req)
		if err != nil {
			if dnsErr, ok := err.(*net.DNSError); ok {
				return nil, dnsErr
			}
			if urlErr, ok := err.(*url.Error); ok {
				return nil, urlErr
			}
			return nil, fmt.Errorf("error getting data from http: %s <%T>", err, err)
		}
		if resp.StatusCode > 399 || resp.StatusCode < 200 {
			return nil, util.ErrHTTP{Status: resp.StatusCode, URL: asString}
		}
		return util.NewFileHeaderReader(resp.Body), nil
	case persist.URITypeIPFSAPI:
		parsedURL, err := url.Parse(asString)
		if err != nil {
			return nil, err
		}
		path := parsedURL.Query().Get("arg")
		resp, err := GetIPFSResponse(ctx, ipfsClient, path)
		if err != nil {
			return nil, err
		}

		return util.NewFileHeaderReader(resp), nil
	case persist.URITypeJSON, persist.URITypeSVG:
		// query unescape asString first
		asString, err := url.QueryUnescape(asString)
		if err != nil {
			logger.For(ctx).Errorf("error unescaping uri: %s", err)
			asString = turi.String()
		}
		idx := strings.IndexByte(asString, ',')
		if idx == -1 {
			buf := bytes.NewBuffer(util.RemoveBOM([]byte(asString)))
			return util.NewFileHeaderReader(buf), nil
		}
		buf := bytes.NewBuffer(util.RemoveBOM([]byte(asString[idx+1:])))
		return util.NewFileHeaderReader(buf), nil
	case persist.URITypeBase64BMP:
		b64data := asString[strings.IndexByte(asString, ',')+1:]
		decoded, err := base64.RawStdEncoding.DecodeString(string(b64data))
		if err != nil {
			decoded, err = base64.StdEncoding.DecodeString(string(b64data))
			if err != nil {
				return nil, fmt.Errorf("error decoding base64 bmp data: %s \n\n%s", err, b64data)
			}
		}
		img, err := bmp.Decode(bytes.NewReader(decoded))
		if err != nil {
			return nil, fmt.Errorf("error decoding bmp data: %s \n\n%s", err, b64data)
		}
		newImage := &bytes.Buffer{}
		err = jpeg.Encode(newImage, img, nil)
		if err != nil {
			return nil, fmt.Errorf("error encoding jpeg data: %s \n\n%s", err, b64data)
		}
		return util.NewFileHeaderReader(newImage), nil
	case persist.URITypeBase64PNG:
		b64data := asString[strings.IndexByte(asString, ',')+1:]
		decoded, err := base64.RawStdEncoding.DecodeString(string(b64data))
		if err != nil {
			decoded, err = base64.StdEncoding.DecodeString(string(b64data))
			if err != nil {
				return nil, fmt.Errorf("error decoding base64 png data: %s \n\n%s", err, b64data)
			}
		}
		img, err := png.Decode(bytes.NewReader(decoded))
		if err != nil {
			return nil, fmt.Errorf("error decoding png data: %s \n\n%s", err, b64data)
		}
		newImage := &bytes.Buffer{}
		err = png.Encode(newImage, img)
		if err != nil {
			return nil, fmt.Errorf("error encoding jpeg data: %s \n\n%s", err, b64data)
		}
		return util.NewFileHeaderReader(newImage), nil
	default:
		buf := bytes.NewBuffer([]byte(turi))
		return util.NewFileHeaderReader(buf), nil
	}

}

// DecodeMetadataFromURI calls URI and decodes the data into a metadata map
func DecodeMetadataFromURI(ctx context.Context, turi persist.TokenURI, into *persist.TokenMetadata, ipfsClient *shell.Shell, arweaveClient *goar.Client) error {

	d, _ := ctx.Deadline()
	logger.For(ctx).Debugf("Getting metadata from URI: %s -timeout: %s", turi.String(), time.Until(d))
	asString := turi.String()

	logger.For(ctx).Debugf("Getting metadata from %s with type %s", asString, turi.Type())

	switch turi.Type() {
	case persist.URITypeBase64JSON:
		// decode the base64 encoded json
		b64data := asString[strings.IndexByte(asString, ',')+1:]
		decoded, err := base64.RawStdEncoding.DecodeString(string(b64data))
		if err != nil {
			decoded, err = base64.StdEncoding.DecodeString(string(b64data))
			if err != nil {
				return fmt.Errorf("error decoding base64 data: %s \n\n%s", err, b64data)
			}
		}

		return json.Unmarshal(util.RemoveBOM(decoded), into)
	case persist.URITypeBase64SVG:
		b64data := asString[strings.IndexByte(asString, ',')+1:]
		decoded, err := base64.RawStdEncoding.DecodeString(string(b64data))
		if err != nil {
			decoded, err = base64.StdEncoding.DecodeString(string(b64data))
			if err != nil {
				return fmt.Errorf("error decoding base64 data: %s \n\n%s", err, b64data)
			}
		}
		*into = persist.TokenMetadata{"image": string(decoded)}
		return nil
	case persist.URITypeIPFS, persist.URITypeIPFSGateway:

		bs, err := GetIPFSData(ctx, ipfsClient, util.GetURIPath(asString, false))
		if err != nil {
			return err
		}
		return json.Unmarshal(util.RemoveBOM(bs), into)
	case persist.URITypeArweave, persist.URITypeArweaveGateway:
		path := strings.ReplaceAll(asString, "arweave://", "")
		path = strings.ReplaceAll(path, "ar://", "")
		result, err := GetArweaveData(arweaveClient, path)
		if err != nil {
			result, err = GetArweaveDataHTTP(ctx, path)
			if err != nil {
				return err
			}
		}
		return json.Unmarshal(result, into)
	case persist.URITypeHTTP:

		req, err := http.NewRequestWithContext(ctx, "GET", asString, nil)
		if err != nil {
			return fmt.Errorf("error creating request: %s", err)
		}
		resp, err := defaultHTTPClient.Do(req)
		if err != nil {
			if dnsErr, ok := err.(*net.DNSError); ok {
				return dnsErr
			}
			if urlErr, ok := err.(*url.Error); ok {
				return urlErr
			}
			return fmt.Errorf("error decoding metadatadata from http: %s <%T>", err, err)
		}
		defer resp.Body.Close()
		if resp.StatusCode > 399 || resp.StatusCode < 200 {
			return util.ErrHTTP{Status: resp.StatusCode, URL: asString}
		}
		return json.NewDecoder(resp.Body).Decode(into)
	case persist.URITypeIPFSAPI:
		parsedURL, err := url.Parse(asString)
		if err != nil {
			return err
		}
		query := parsedURL.Query().Get("arg")
		it, err := ipfsClient.Cat(query)
		if err != nil {
			return err
		}
		defer it.Close()
		return json.NewDecoder(it).Decode(into)
	case persist.URITypeJSON, persist.URITypeSVG:
		idx := strings.IndexByte(asString, '{')
		if idx == -1 {
			return json.Unmarshal(util.RemoveBOM([]byte(asString)), into)
		}
		return json.Unmarshal(util.RemoveBOM([]byte(asString[idx:])), into)

	default:
		return fmt.Errorf("unknown token URI type for metadata: %s", turi.Type())
	}

}

func GetIPFSData(pCtx context.Context, ipfsClient *shell.Shell, path string) ([]byte, error) {
	response, err := GetIPFSResponse(pCtx, ipfsClient, path)
	if err != nil {
		return nil, err
	}
	defer response.Close()
	buf := &bytes.Buffer{}
	if err := util.CopyMax(buf, response, 1024*1024*1024); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func getHeaders(ctx context.Context, method, url string) (http.Header, error) {
	req, err := http.NewRequestWithContext(ctx, method, url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := defaultHTTPClient.Do(req)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode > 399 || resp.StatusCode < 200 {
		return nil, util.ErrHTTP{Status: resp.StatusCode, URL: url}
	}

	defer resp.Body.Close()
	return resp.Header, nil
}

func parseContentLength(contentLength string) (int64, error) {
	if contentLength != "" {
		contentLengthInt, err := strconv.Atoi(contentLength)
		if err != nil {
			return 0, err
		}
		return int64(contentLengthInt), nil
	}
	return 0, nil
}

func parseContentType(contentType string) string {
	contentType = strings.TrimSpace(contentType)
	whereCharset := strings.IndexByte(contentType, ';')
	if whereCharset != -1 {
		contentType = contentType[:whereCharset]
	}
	return contentType
}

type fetchResulter interface {
	Error() error
	ErrIsForceClose(error) bool
}

type headerResult struct {
	contentType   string
	contentLength int64
	err           error
}

func (r headerResult) Error() error {
	return r.err
}

func (r headerResult) ErrIsForceClose(err error) bool {
	if err != nil {
		if it, ok := err.(util.ErrHTTP); ok && it.Status == http.StatusNotFound {
			return true
		}
	}
	return false
}

type ipfsResult struct {
	resp io.ReadCloser
	err  error
}

func (r ipfsResult) Error() error {
	return r.err
}

func (r ipfsResult) ErrIsForceClose(err error) bool {
	if err != nil {
		if it, ok := err.(util.ErrHTTP); ok && it.Status == http.StatusNotFound {
			return true
		}
	}
	return false
}

func firstNonError(ctx context.Context, fetches ...func(context.Context) fetchResulter) fetchResulter {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	c := make(chan fetchResulter)
	done := make(chan bool)

	for i := range fetches {
		go func(i int) {
			select {
			case <-ctx.Done():
				return
			case <-done:
				return
			case c <- fetches[i](ctx):
			}
		}(i)
	}

	var lastError fetchResulter

	for i := 0; i < len(fetches); i++ {
		r := <-c
		if r.Error() == nil {
			close(done)
			return r
		}
		lastError = r
		if r.ErrIsForceClose(r.Error()) {
			close(done)
			return r
		}
		i++
	}

	return lastError
}

func getContentHeaders(ctx context.Context, url string) (contentType string, contentLength int64, err error) {
	contentHeader := func(method, url string) func(ctx context.Context) fetchResulter {
		return func(ctx context.Context) fetchResulter {
			headers, err := getHeaders(ctx, method, url)
			if err != nil {
				return headerResult{err: err}
			}
			contentType := parseContentType(headers.Get("Content-Type"))
			contentLength, err := parseContentLength(headers.Get("Content-Length"))
			return headerResult{contentType, contentLength, err}
		}
	}
	fromHEAD := contentHeader(http.MethodHead, url)
	fromGET := contentHeader(http.MethodGet, url)
	result := firstNonError(ctx, fromHEAD, fromGET)
	headers := result.(headerResult)
	return headers.contentType, headers.contentLength, headers.Error()
}

func GetIPFSResponse(ctx context.Context, ipfsClient *shell.Shell, path string) (io.ReadCloser, error) {
	fromHTTP := func(ctx context.Context) fetchResulter {
		url := fmt.Sprintf("%s/ipfs/%s", env.GetString("IPFS_URL"), path)
		req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
		if err != nil {
			return ipfsResult{err: err}
		}

		resp, err := defaultHTTPClient.Do(req)
		if err != nil {
			return ipfsResult{err: err}
		}

		if resp.StatusCode > 399 || resp.StatusCode < 200 {
			if resp.StatusCode == http.StatusNotFound {
				return ipfsResult{err: util.ErrHTTP{Status: resp.StatusCode, URL: url}}
			}
			url := fmt.Sprintf("%s/ipfs/%s", env.GetString("FALLBACK_IPFS_URL"), path)
			req, err = http.NewRequestWithContext(ctx, "GET", url, nil)
			if err != nil {
				return ipfsResult{err: err}
			}

			resp, err = defaultHTTPClient.Do(req)
			if err != nil {
				return ipfsResult{err: err}
			}
			if resp.StatusCode > 399 || resp.StatusCode < 200 {
				return ipfsResult{err: util.ErrHTTP{Status: resp.StatusCode, URL: url}}
			}
			logger.For(ctx).Infof("IPFS HTTP fallback fallback successful %s", path)
		}

		logger.For(ctx).Infof("IPFS HTTP fallback successful %s", path)

		return ipfsResult{resp: resp.Body}
	}

	fromIPFS := func(ctx context.Context) fetchResulter {
		_, _, err := ipfsClient.BlockStat(path)
		if err != nil {
			return ipfsResult{err: err}
		}
		reader, err := ipfsClient.Cat(path)
		logger.For(ctx).Infof("IPFS cat fallback successful %s", path)
		return ipfsResult{reader, err}
	}

	fromIPFSAPI := func(ctx context.Context) fetchResulter {
		url := fmt.Sprintf("https://ipfs.io/ipfs/%s", path)
		req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
		if err != nil {
			return ipfsResult{err: err}
		}

		resp, err := defaultHTTPClient.Do(req)
		if err != nil {
			return ipfsResult{err: err}
		}

		if resp.StatusCode > 399 || resp.StatusCode < 200 {
			return ipfsResult{err: util.ErrHTTP{Status: resp.StatusCode, URL: url}}
		}

		logger.For(ctx).Infof("IPFS API fallback successful %s", path)

		return ipfsResult{resp: resp.Body}
	}

	result := firstNonError(ctx, fromHTTP, fromIPFS, fromIPFSAPI)
	response := result.(ipfsResult)
	return response.resp, response.Error()
}

// GetIPFSHeaders returns the headers for the given IPFS hash
func GetIPFSHeaders(ctx context.Context, path string) (contentType string, contentLength int64, err error) {
	url := fmt.Sprintf("%s/ipfs/%s", env.GetString("IPFS_URL"), path)
	return getContentHeaders(ctx, url)
}

// GetHTTPHeaders returns the headers for the given URL
func GetHTTPHeaders(ctx context.Context, url string) (contentType string, contentLength int64, err error) {
	return getContentHeaders(ctx, url)
}

// GetTokenURI returns metadata URI for a given token address.
func GetTokenURI(ctx context.Context, pTokenType persist.TokenType, pContractAddress persist.EthereumAddress, pTokenID persist.TokenID, ethClient *ethclient.Client) (persist.TokenURI, error) {

	contract := pContractAddress.Address()
	switch pTokenType {
	case persist.TokenTypeERC721:

		instance, err := contracts.NewIERC721MetadataCaller(contract, ethClient)
		if err != nil {
			return "", ErrEthClient{err}
		}

		logger.For(ctx).Debugf("Token ID: %s\tToken Address: %s", pTokenID.String(), contract.Hex())

		turi, err := instance.TokenURI(&bind.CallOpts{
			Context: ctx,
		}, pTokenID.BigInt())
		if err != nil {
			logger.For(ctx).Errorf("Error getting token URI: %s (%T)", err, err)
			return "", ErrTokenURINotFound{err}
		}

		return persist.TokenURI(strings.ReplaceAll(turi, "\x00", "")), nil
	case persist.TokenTypeERC1155:

		instance, err := contracts.NewIERC1155MetadataURICaller(contract, ethClient)
		if err != nil {
			return "", ErrEthClient{err}
		}

		logger.For(ctx).Debugf("Token ID: %d\tToken Address: %s", pTokenID.BigInt().Uint64(), contract.Hex())

		turi, err := instance.Uri(&bind.CallOpts{
			Context: ctx,
		}, pTokenID.BigInt())
		if err != nil {
			logger.For(ctx).Errorf("Error getting token URI: %s (%T)", err, err)
			return "", ErrTokenURINotFound{err}
		}

		return persist.TokenURI(strings.ReplaceAll(turi, "\x00", "")), nil

	default:
		tokenURI, err := GetTokenURI(ctx, persist.TokenTypeERC721, pContractAddress, pTokenID, ethClient)
		if err == nil {
			return tokenURI, nil
		}

		tokenURI, err = GetTokenURI(ctx, persist.TokenTypeERC1155, pContractAddress, pTokenID, ethClient)
		if err == nil {
			return tokenURI, nil
		}

		logger.For(ctx).Errorf("Error getting token URI: %s (%T) (token type: %s)", err, err, pTokenType)

		return "", err
	}
}

// RetryGetTokenURI calls GetTokenURI with backoff.
func RetryGetTokenURI(ctx context.Context, tokenType persist.TokenType, contractAddress persist.EthereumAddress, tokenID persist.TokenID, ethClient *ethclient.Client) (persist.TokenURI, error) {
	var u persist.TokenURI
	var err error
	for i := 0; i < retry.DefaultRetry.Tries; i++ {
		u, err = GetTokenURI(ctx, tokenType, contractAddress, tokenID, ethClient)
		if !isRateLimitedError(err) {
			break
		}
		retry.DefaultRetry.Sleep(i)
	}
	return u, err
}

// GetBalanceOfERC1155Token returns the balance of an ERC1155 token
func GetBalanceOfERC1155Token(ctx context.Context, pOwnerAddress, pContractAddress persist.EthereumAddress, pTokenID persist.TokenID, ethClient *ethclient.Client) (*big.Int, error) {
	contract := common.HexToAddress(string(pContractAddress))
	owner := common.HexToAddress(string(pOwnerAddress))
	instance, err := contracts.NewIERC1155(contract, ethClient)
	if err != nil {
		return nil, err
	}

	bal, err := instance.BalanceOf(&bind.CallOpts{
		Context: ctx,
	}, owner, pTokenID.BigInt())
	if err != nil {
		return nil, err
	}

	return bal, nil
}

// RetryGetBalanceOfERC1155Token calls GetBalanceOfERC1155Token with backoff.
func RetryGetBalanceOfERC1155Token(ctx context.Context, pOwnerAddress, pContractAddress persist.EthereumAddress, pTokenID persist.TokenID, ethClient *ethclient.Client) (*big.Int, error) {
	var balance *big.Int
	var err error
	for i := 0; i < retry.DefaultRetry.Tries; i++ {
		balance, err = GetBalanceOfERC1155Token(ctx, pOwnerAddress, pContractAddress, pTokenID, ethClient)
		if !isRateLimitedError(err) {
			break
		}
		retry.DefaultRetry.Sleep(i)
	}
	return balance, err
}

// GetOwnerOfERC721Token returns the Owner of an ERC721 token
func GetOwnerOfERC721Token(ctx context.Context, pContractAddress persist.EthereumAddress, pTokenID persist.TokenID, ethClient *ethclient.Client) (persist.EthereumAddress, error) {
	contract := common.HexToAddress(string(pContractAddress))

	instance, err := contracts.NewIERC721Caller(contract, ethClient)
	if err != nil {
		return "", err
	}

	owner, err := instance.OwnerOf(&bind.CallOpts{
		Context: ctx,
	}, pTokenID.BigInt())
	if err != nil {
		return "", err
	}

	return persist.EthereumAddress(strings.ToLower(owner.String())), nil
}

// RetryGetOwnerOfERC721Token calls GetOwnerOfERC721Token with backoff.
func RetryGetOwnerOfERC721Token(ctx context.Context, pContractAddress persist.EthereumAddress, pTokenID persist.TokenID, ethClient *ethclient.Client) (persist.EthereumAddress, error) {
	var owner persist.EthereumAddress
	var err error
	for i := 0; i < retry.DefaultRetry.Tries; i++ {
		owner, err = GetOwnerOfERC721Token(ctx, pContractAddress, pTokenID, ethClient)
		if !isRateLimitedError(err) {
			break
		}
		retry.DefaultRetry.Sleep(i)
	}
	return owner, err
}

// GetContractCreator returns the address of the contract creator
func GetContractCreator(ctx context.Context, contractAddress persist.EthereumAddress, ethClient *ethclient.Client) (persist.EthereumAddress, error) {
	highestBlock, err := ethClient.BlockNumber(ctx)
	if err != nil {
		return "", fmt.Errorf("error getting highest block: %s", err.Error())
	}
	_, err = ethClient.CodeAt(ctx, contractAddress.Address(), big.NewInt(int64(highestBlock)))
	if err != nil {
		return "", fmt.Errorf("error getting code at: %s", err.Error())
	}
	lowestBlock := uint64(0)

	for lowestBlock <= highestBlock {
		midBlock := uint64((highestBlock + lowestBlock) / 2)
		codeAt, err := ethClient.CodeAt(ctx, contractAddress.Address(), big.NewInt(int64(midBlock)))
		if err != nil {
			return "", fmt.Errorf("error getting code at: %s", err.Error())
		}
		if len(codeAt) > 0 {
			highestBlock = midBlock
		} else {
			lowestBlock = midBlock
		}

		if lowestBlock+1 == highestBlock {
			break
		}
	}
	block, err := ethClient.BlockByNumber(ctx, big.NewInt(int64(highestBlock)))
	if err != nil {
		return "", fmt.Errorf("error getting block: %s", err.Error())
	}
	txs := block.Transactions()
	for _, tx := range txs {
		receipt, err := ethClient.TransactionReceipt(ctx, tx.Hash())
		if err != nil {
			return "", fmt.Errorf("error getting transaction receipt: %s", err.Error())
		}
		if receipt.ContractAddress == contractAddress.Address() {
			msg, err := tx.AsMessage(types.HomesteadSigner{}, nil)
			if err != nil {
				return "", fmt.Errorf("error getting message: %s", err.Error())
			}
			return persist.EthereumAddress(fmt.Sprintf("0x%s", strings.ToLower(msg.From().String()))), nil
		}
	}
	return "", fmt.Errorf("could not find contract creator")
}

// GetContractOwner returns the address of the contract owner
func GetContractOwner(ctx context.Context, contractAddress persist.EthereumAddress, ethClient *ethclient.Client) (persist.EthereumAddress, error) {
	instance, err := contracts.NewOwnableCaller(contractAddress.Address(), ethClient)
	if err != nil {
		return "", err
	}

	owner, err := instance.Owner(&bind.CallOpts{
		Context: ctx,
	})
	if err != nil {
		return "", err
	}

	return persist.EthereumAddress(strings.ToLower(owner.String())), nil
}

/*
	{
	  "manifest": "arweave/paths",
	  "version": "0.1.0",
	  "index": { "path": "0" },
	  "paths": {
	    "0": { "id": "4vdubhlnXQp7jGjEjXwWjOa-6Pm44zOF7o6lAHEAYB4" },
	    "1": { "id": "O6ZosH1YVePA7n31UVKJLY9OORIs2ukxwarxE7JYJdY" },
	    "2": { "id": "1ROXHTSaTTKSCpPVlDhRpxEJ6JE3WQ5ZAgfglo_z4W8" },
	    "3": { "id": "LF7g-RV4dob0yNAjIaPEjxs8UgXShJI4GFxx6CjVavM" },
	    "4": { "id": "fudz-Ig2CtM4ZhZcwEn9jnWFWH9S4loZ2taoJoQP1b8" },
	    "5": { "id": "qYaBEv7QaBKeXPZP9LohHHzr1rwYWMY3bJrDaRoRQ2Q" },
	    "6": { "id": "jI-4Q2_Z9ZpefzBVBeowpDizAmFtXFSe7w5eOP_CCvA" },
	    "7": { "id": "2B_s60w4ZS0_QdO6dd0qi0GKqAkYeTJ_bL05kr_tkgk" }
	  }
	}
*/
type arweaveManifest struct {
	Manifest string `json:"manifest"`
	Version  string `json:"version"`
	Index    struct {
		Path string `json:"path"`
	} `json:"index"`
	Paths map[string]struct {
		ID string `json:"id"`
	} `json:"paths"`
}

// GetArweaveData returns the data from an Arweave transaction
func GetArweaveData(client *goar.Client, id string) ([]byte, error) {
	splitPath := strings.Split(id, "/")
	var data []byte
	var tx *goartypes.Transaction
	currentID := splitPath[0]
	for i := range splitPath {
		t, err := client.GetTransactionByID(currentID)
		if err != nil {
			return nil, fmt.Errorf("error getting transaction: %s", err.Error())
		}
		tx = t
		data, err = client.GetTransactionData(currentID)
		if err != nil {
			return nil, fmt.Errorf("error getting transaction data: %s", err.Error())
		}
		if i < len(splitPath)-1 {
			decoded, err := base64.RawStdEncoding.DecodeString(string(data))
			var manifest arweaveManifest
			err = json.Unmarshal(decoded, &manifest)
			if err != nil {
				return nil, fmt.Errorf("error unmarshalling manifest: %s - %s", err.Error(), string(decoded))
			}
			currentID = manifest.Paths[splitPath[i+1]].ID
		}
	}

	decoded, err := base64.RawURLEncoding.DecodeString(string(data))
	if err == nil {
		data = decoded
	}

	for _, tag := range tx.Tags {
		decodedName, err := base64.RawURLEncoding.DecodeString(tag.Name)
		if err != nil {
			return nil, fmt.Errorf("error decoding tag name: %s", err.Error())
		}
		if strings.EqualFold(string(decodedName), "Content-Encoding") {
			decodedValue, err := base64.RawURLEncoding.DecodeString(tag.Value)
			if err != nil {
				return nil, fmt.Errorf("error decoding tag value: %s", err.Error())
			}
			if strings.EqualFold(string(decodedValue), "gzip") {
				zipped, err := gzip.NewReader(bytes.NewReader(data))
				if err != nil {
					return nil, fmt.Errorf("error unzipping data: %s", err.Error())
				}
				buf := new(bytes.Buffer)
				_, err = io.Copy(buf, zipped)
				if err != nil {
					return nil, fmt.Errorf("error copying data: %s", err.Error())
				}
				data = buf.Bytes()
			}
		}
	}
	return util.RemoveBOM(data), nil
}

func GetArweaveDataHTTPReader(ctx context.Context, id string) (io.ReadCloser, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("https://arweave.net/%s", id), nil)
	if err != nil {
		return nil, fmt.Errorf("error getting data: %s", err.Error())
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, util.ErrHTTP{Err: err, URL: req.URL.String(), Status: resp.StatusCode}
	}
	return resp.Body, nil
}

func GetArweaveDataHTTP(ctx context.Context, id string) ([]byte, error) {
	resp, err := GetArweaveDataHTTPReader(ctx, id)
	if err != nil {
		return nil, err
	}
	defer resp.Close()
	data, err := ioutil.ReadAll(resp)
	if err != nil {
		return nil, fmt.Errorf("error reading data: %s", err.Error())
	}
	return data, nil
}

// GetArweaveContentType returns the content-type from an Arweave transaction
func GetArweaveContentType(client *goar.Client, id string) (string, error) {
	data, err := client.GetTransactionTags(id)
	if err != nil {
		return "", err
	}

	for _, tag := range data {
		decodedName, err := base64.RawURLEncoding.DecodeString(tag.Name)
		if err != nil {
			return "", err
		}
		if strings.EqualFold(string(decodedName), "Content-Encoding") || strings.EqualFold(string(decodedName), "Content-Type") {
			decodedValue, err := base64.RawURLEncoding.DecodeString(tag.Value)
			if err != nil {
				return "", err
			}
			return string(decodedValue), nil
		}
	}
	return "", nil
}

func padHex(pHex string, pLength int) string {
	for len(pHex) < pLength {
		pHex = "0" + pHex
	}
	return pHex
}

// valFromSlice returns the value from a slice formatted as [key val key val ...]
func valFromSlice(s []interface{}, keyName string) interface{} {
	for i, key := range s {
		if key == keyName {
			return s[i+1]
		}
	}
	return nil
}

func isRateLimitedError(err error) bool {
	if err != nil && strings.Contains(err.Error(), rateLimited) {
		return true
	}
	return false
}
