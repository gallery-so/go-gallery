package rpc

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image/jpeg"
	"io"
	"math/big"
	"math/rand"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"golang.org/x/image/bmp"

	"github.com/getsentry/sentry-go"
	"github.com/mikeydub/go-gallery/service/logger"
	"github.com/mikeydub/go-gallery/service/tracing"

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
	"github.com/spf13/viper"
)

const (
	defaultHTTPTimeout             = 30
	defaultHTTPKeepAlive           = 600
	defaultHTTPMaxIdleConns        = 100
	defaultHTTPMaxIdleConnsPerHost = 100
)

var (
	defaultHTTPClient     = newHTTPClientForRPC(true, true)
	defaultMetricsHandler = metricsHandler{}
)

// rateLimited is the content returned from an RPC call when rate limited.
var rateLimited = "429 Too Many Requests"

// DefaultRetry is the default retry applied to RPC calls.
var DefaultRetry = Retry{
	Base:  4,
	Cap:   64,
	Tries: 8,
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
}

// TokenContractMetadata represents a token contract's metadata
type TokenContractMetadata struct {
	Name   string
	Symbol string
}

// ErrHTTP represents an error returned from an HTTP request
type ErrHTTP struct {
	URL    string
	Status int
}

// Retry configures retries for RPC calls.
type Retry struct {
	Base  int // min amount of time to sleep per iteration
	Cap   int // max amount of time to sleep per iteration
	Tries int // number of times to retry
}

// Sleep will sleep based on the current iteration.
func (r Retry) Sleep(i int) {
	sleep := rand.Intn(minInt(r.Cap, r.Base*powerInt(2, i)))
	time.Sleep(time.Duration(sleep) * time.Second)
}

// NewEthClient returns an ethclient.Client
func NewEthClient() *ethclient.Client {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	rpcClient, err := rpc.DialContext(ctx, viper.GetString("RPC_URL"))
	if err != nil {
		panic(err)
	}

	return ethclient.NewClient(rpcClient)

}

// NewEthHTTPClient returns a new http client with request tracing enabled
func NewEthHTTPClient() *ethclient.Client {
	if !strings.HasPrefix(viper.GetString("RPC_URL"), "http") {
		return NewEthClient()
	}

	httpClient := newHTTPClientForRPC(false, true, sentryutil.TransactionNameSafe("gethRPC"))
	rpcClient, err := rpc.DialHTTPWithClient(viper.GetString("RPC_URL"), httpClient)
	if err != nil {
		panic(err)
	}

	return ethclient.NewClient(rpcClient)
}

// NewEthSocketClient returns a new websocket client with request tracing enabled
func NewEthSocketClient() *ethclient.Client {
	if !strings.HasPrefix(viper.GetString("RPC_URL"), "wss") {
		return NewEthClient()
	}

	log.Root().SetHandler(log.FilterHandler(func(r *log.Record) bool {
		if reqID := valFromSlice(r.Ctx, "reqid"); reqID == nil {
			return false
		}
		return true
	}, defaultMetricsHandler))

	return NewEthClient()
}

// metricsHandler traces RPC records that get logged by the RPC client
type metricsHandler struct{}

// Log sends trace information to Sentry.
// Geth logs the duration of each RPC call: https://github.com/ethereum/go-ethereum/blob/master/rpc/handler.go#L242
// We take advantage of this by configuring the root logger with a custom handler that parses records
// that have useful metrics data attached such as the duration and the request id.
func (h metricsHandler) Log(r *log.Record) error {
	reqID := valFromSlice(r.Ctx, "reqid")

	// A useful context isn't passed to the log record, so we use the background context here.
	ctx := context.Background()
	span, _ := tracing.StartSpan(ctx, "geth.wss", "rpcCall", sentryutil.TransactionNameSafe("gethRPC"))
	tracing.AddEventDataToSpan(span, map[string]interface{}{"reqID": reqID})
	defer tracing.FinishSpan(span)

	if d := valFromSlice(r.Ctx, "duration"); d != nil {
		d := d.(time.Duration)
		span.StartTime = r.Time.Add(-d)
		span.EndTime = r.Time
	}

	return nil
}

// NewIPFSShell returns an IPFS shell
func NewIPFSShell() *shell.Shell {
	sh := shell.NewShellWithClient(viper.GetString("IPFS_API_URL"), newClientForIPFS(viper.GetString("IPFS_PROJECT_ID"), viper.GetString("IPFS_PROJECT_SECRET"), false, true))
	sh.SetTimeout(time.Minute * 2)
	return sh
}

// newHTTPClientForIPFS returns an http.Client configured with default settings intended for IPFS calls.
func newClientForIPFS(projectID, projectSecret string, continueOnly, errorsOnly bool) *http.Client {
	return &http.Client{
		Transport: authTransport{
			RoundTripper:  tracing.NewTracingTransport(http.DefaultTransport, continueOnly, errorsOnly),
			ProjectID:     projectID,
			ProjectSecret: projectSecret,
		},
	}
}

// newHTTPClientForRPC returns an http.Client configured with default settings intended for RPC calls.
func newHTTPClientForRPC(continueTrace, errorsOnly bool, spanOptions ...sentry.SpanOption) *http.Client {
	return &http.Client{
		Timeout: time.Second * defaultHTTPTimeout,
		Transport: tracing.NewTracingTransport(&http.Transport{
			Dial:                (&net.Dialer{KeepAlive: defaultHTTPKeepAlive * time.Second}).Dial,
			MaxIdleConns:        defaultHTTPMaxIdleConns,
			MaxIdleConnsPerHost: defaultHTTPMaxIdleConnsPerHost,
		}, continueTrace, errorsOnly, spanOptions...),
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
func RetryGetBlockNumber(ctx context.Context, ethClient *ethclient.Client, retry Retry) (uint64, error) {
	var height uint64
	var err error
	for i := 0; i < retry.Tries; i++ {
		height, err = GetBlockNumber(ctx, ethClient)
		if !isRateLimitedError(err) {
			break
		}
		retry.Sleep(i)
	}
	return height, err
}

// GetLogs returns log events for the given block range and query.
func GetLogs(ctx context.Context, ethClient *ethclient.Client, query ethereum.FilterQuery) ([]types.Log, error) {
	return ethClient.FilterLogs(ctx, query)
}

// RetryGetLogs calls GetLogs with backoff.
func RetryGetLogs(ctx context.Context, ethClient *ethclient.Client, query ethereum.FilterQuery, retry Retry) ([]types.Log, error) {
	logs := make([]types.Log, 0)
	var err error
	for i := 0; i < retry.Tries; i++ {
		logs, err = GetLogs(ctx, ethClient, query)
		if !isRateLimitedError(err) {
			break
		}
		retry.Sleep(i)
	}
	return logs, err
}

// GetTransaction returns the transaction of the given hash.
func GetTransaction(ctx context.Context, ethClient *ethclient.Client, txHash common.Hash) (*types.Transaction, bool, error) {
	return ethClient.TransactionByHash(ctx, txHash)
}

// RetryGetTransaction calls GetTransaction with backoff.
func RetryGetTransaction(ctx context.Context, ethClient *ethclient.Client, txHash common.Hash, retry Retry) (*types.Transaction, bool, error) {
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
func RetryGetTokenContractMetadata(ctx context.Context, contractAddress persist.EthereumAddress, ethClient *ethclient.Client, retry Retry) (*TokenContractMetadata, error) {
	var metadata *TokenContractMetadata
	var err error
	for i := 0; i < retry.Tries; i++ {
		metadata, err = GetTokenContractMetadata(ctx, contractAddress, ethClient)
		if !isRateLimitedError(err) {
			break
		}
		retry.Sleep(i)
	}
	return metadata, err
}

// GetMetadataFromURI parses and returns the NFT metadata for a given token URI
func GetMetadataFromURI(ctx context.Context, turi persist.TokenURI, ipfsClient *shell.Shell, arweaveClient *goar.Client) (persist.TokenMetadata, error) {

	ctx, cancel := context.WithTimeout(ctx, time.Minute*2)
	defer cancel()
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
		path := util.GetIPFSPath(asString, true)

		bs, err := GetIPFSData(ctx, ipfsClient, path)
		if err != nil {
			return nil, err
		}

		return util.RemoveBOM(bs), nil
	case persist.URITypeArweave:
		path := strings.ReplaceAll(asString, "arweave://", "")
		path = strings.ReplaceAll(path, "ar://", "")
		bs, err := GetArweaveData(arweaveClient, path)
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
			return nil, fmt.Errorf("error getting data from http: %s", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode > 399 || resp.StatusCode < 200 {
			return nil, ErrHTTP{Status: resp.StatusCode, URL: asString}
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
	default:
		return []byte(turi), nil
	}

}

// GetDataFromURIAsReader calls URI and returns the data as an unread reader with the headers pre-read
func GetDataFromURIAsReader(ctx context.Context, turi persist.TokenURI, ipfsClient *shell.Shell, arweaveClient *goar.Client) (util.FileHeaderReader, error) {

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
				return util.FileHeaderReader{}, fmt.Errorf("error decoding base64 data: %s \n\n%s", err, b64data)
			}
		}

		buf := bytes.NewBuffer(util.RemoveBOM(decoded))

		return util.NewFileHeaderReader(buf)
	case persist.URITypeIPFS, persist.URITypeIPFSGateway:
		path := util.GetIPFSPath(asString, true)

		resp, err := GetIPFSResponse(ctx, ipfsClient, path)
		if err != nil {
			return util.FileHeaderReader{}, err
		}

		return util.NewFileHeaderReader(resp)
	case persist.URITypeArweave:
		path := strings.ReplaceAll(asString, "arweave://", "")
		path = strings.ReplaceAll(path, "ar://", "")
		bs, err := GetArweaveData(arweaveClient, path)
		if err != nil {
			return util.FileHeaderReader{}, err
		}
		buf := bytes.NewBuffer(util.RemoveBOM(bs))
		return util.NewFileHeaderReader(buf)
	case persist.URITypeHTTP:

		req, err := http.NewRequestWithContext(ctx, "GET", asString, nil)
		if err != nil {
			return util.FileHeaderReader{}, fmt.Errorf("error creating request: %s", err)
		}
		resp, err := defaultHTTPClient.Do(req)
		if err != nil {
			return util.FileHeaderReader{}, fmt.Errorf("error getting data from http: %s", err)
		}
		if resp.StatusCode > 399 || resp.StatusCode < 200 {
			return util.FileHeaderReader{}, ErrHTTP{Status: resp.StatusCode, URL: asString}
		}
		return util.NewFileHeaderReader(resp.Body)
	case persist.URITypeIPFSAPI:
		parsedURL, err := url.Parse(asString)
		if err != nil {
			return util.FileHeaderReader{}, err
		}
		path := parsedURL.Query().Get("arg")
		resp, err := GetIPFSResponse(ctx, ipfsClient, path)
		if err != nil {
			return util.FileHeaderReader{}, err
		}

		return util.NewFileHeaderReader(resp)
	case persist.URITypeJSON, persist.URITypeSVG:
		idx := strings.IndexByte(asString, ',')
		if idx == -1 {
			buf := bytes.NewBuffer(util.RemoveBOM([]byte(asString)))
			return util.NewFileHeaderReader(buf)
		}
		buf := bytes.NewBuffer(util.RemoveBOM([]byte(asString[idx+1:])))
		return util.NewFileHeaderReader(buf)
	case persist.URITypeBase64BMP:
		b64data := asString[strings.IndexByte(asString, ',')+1:]
		decoded, err := base64.RawStdEncoding.DecodeString(string(b64data))
		if err != nil {
			decoded, err = base64.StdEncoding.DecodeString(string(b64data))
			if err != nil {
				return util.FileHeaderReader{}, fmt.Errorf("error decoding base64 bmp data: %s \n\n%s", err, b64data)
			}
		}
		img, err := bmp.Decode(bytes.NewReader(decoded))
		if err != nil {
			return util.FileHeaderReader{}, fmt.Errorf("error decoding bmp data: %s \n\n%s", err, b64data)
		}
		newImage := &bytes.Buffer{}
		err = jpeg.Encode(newImage, img, nil)
		if err != nil {
			return util.FileHeaderReader{}, fmt.Errorf("error encoding jpeg data: %s \n\n%s", err, b64data)
		}
		return util.NewFileHeaderReader(newImage)
	default:
		buf := bytes.NewBuffer([]byte(turi))
		return util.NewFileHeaderReader(buf)
	}

}

// DecodeMetadataFromURI calls URI and decodes the data into a metadata map
func DecodeMetadataFromURI(ctx context.Context, turi persist.TokenURI, into *persist.TokenMetadata, ipfsClient *shell.Shell, arweaveClient *goar.Client) error {

	d, _ := ctx.Deadline()
	logger.For(ctx).Debugf("Getting data from URI: %s -timeout: %s", turi.String(), time.Until(d))
	asString := turi.String()

	logger.For(ctx).Debugf("Getting data from %s with type %s", asString, turi.Type())

	switch turi.Type() {
	case persist.URITypeBase64JSON:
		// decode the base64 encoded json
		b64data := asString[strings.IndexByte(asString, ',')+1:]
		decoded, err := base64.StdEncoding.DecodeString(string(b64data))
		if err != nil {
			return fmt.Errorf("error decoding base64 data: %s \n\n%s", err, b64data)
		}

		return json.Unmarshal(util.RemoveBOM(decoded), into)
	case persist.URITypeBase64SVG:
		b64data := asString[strings.IndexByte(asString, ',')+1:]
		decoded, err := base64.StdEncoding.DecodeString(string(b64data))
		if err != nil {
			return fmt.Errorf("error decoding base64 data: %s \n\n%s", err, b64data)
		}
		into = &persist.TokenMetadata{"image": string(decoded)}
		return nil
	case persist.URITypeIPFS, persist.URITypeIPFSGateway:

		bs, err := GetIPFSData(ctx, ipfsClient, util.GetIPFSPath(asString, false))
		if err != nil {
			return err
		}
		return json.Unmarshal(bs, into)
	case persist.URITypeArweave:
		path := strings.ReplaceAll(asString, "arweave://", "")
		path = strings.ReplaceAll(path, "ar://", "")
		result, err := GetArweaveData(arweaveClient, path)
		if err != nil {
			return err
		}
		return json.Unmarshal(result, into)
	case persist.URITypeHTTP:

		req, err := http.NewRequestWithContext(ctx, "GET", asString, nil)
		if err != nil {
			return fmt.Errorf("error creating request: %s", err)
		}
		resp, err := defaultHTTPClient.Do(req)
		if err != nil {
			return fmt.Errorf("error getting data from http: %s", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode > 399 || resp.StatusCode < 200 {
			return ErrHTTP{Status: resp.StatusCode, URL: asString}
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
		return fmt.Errorf("unknown token URI type: %s", turi.Type())
	}

}

func GetIPFSResponse(pCtx context.Context, ipfsClient *shell.Shell, path string) (io.ReadCloser, error) {
	dataReader, err := ipfsClient.Cat(path)
	if err != nil {
		logger.For(pCtx).WithError(err).Errorf("error getting cat data from ipfs: %s", path)

		url := fmt.Sprintf("%s/ipfs/%s", viper.GetString("IPFS_URL"), path)

		req, err := http.NewRequestWithContext(pCtx, "GET", url, nil)
		if err != nil {
			return nil, fmt.Errorf("error creating request: %s", err)
		}
		resp, err := defaultHTTPClient.Do(req)
		if err != nil {
			return nil, fmt.Errorf("error getting data from http: %s", err)
		}
		if resp.StatusCode > 399 || resp.StatusCode < 200 {
			return nil, ErrHTTP{Status: resp.StatusCode, URL: url}
		}
		return resp.Body, nil
	}
	return dataReader, nil
}

func GetIPFSData(pCtx context.Context, ipfsClient *shell.Shell, path string) ([]byte, error) {
	dataReader, err := ipfsClient.Cat(path)
	if err != nil {
		logger.For(pCtx).WithError(err).Warnf("error getting cat data from ipfs: %s", path)

		url := fmt.Sprintf("%s/ipfs/%s", viper.GetString("IPFS_URL"), path)

		req, err := http.NewRequestWithContext(pCtx, "GET", url, nil)
		if err != nil {
			return nil, fmt.Errorf("error creating request: %s", err)
		}
		resp, err := defaultHTTPClient.Do(req)
		if err != nil {
			return nil, fmt.Errorf("error getting data from http: %s", err)
		}
		if resp.StatusCode > 399 || resp.StatusCode < 200 {
			return nil, ErrHTTP{Status: resp.StatusCode, URL: url}
		}
		defer resp.Body.Close()

		buf := &bytes.Buffer{}
		err = util.CopyMax(buf, resp.Body, 1024*1024*1024)
		if err != nil {
			return nil, err
		}

		return buf.Bytes(), nil
	}
	defer dataReader.Close()
	buf := &bytes.Buffer{}
	err = util.CopyMax(buf, dataReader, 1024*1024*1024)
	if err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// GetIPFSHeaders returns the headers for the given IPFS hash
func GetIPFSHeaders(pCtx context.Context, path string) (http.Header, error) {
	url := fmt.Sprintf("%s/ipfs/%s", viper.GetString("IPFS_URL"), path)

	req, err := http.NewRequestWithContext(pCtx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("error creating request: %s", err)
	}
	resp, err := defaultHTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error getting data from http: %s", err)
	}
	if resp.StatusCode > 399 || resp.StatusCode < 200 {
		return nil, ErrHTTP{Status: resp.StatusCode, URL: url}
	}
	defer resp.Body.Close()

	return resp.Header, nil
}

// GetTokenURI returns metadata URI for a given token address.
func GetTokenURI(ctx context.Context, pTokenType persist.TokenType, pContractAddress persist.EthereumAddress, pTokenID persist.TokenID, ethClient *ethclient.Client) (persist.TokenURI, error) {

	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	contract := common.HexToAddress(string(pContractAddress))
	switch pTokenType {
	case persist.TokenTypeERC721:

		instance, err := contracts.NewIERC721MetadataCaller(contract, ethClient)
		if err != nil {
			return "", err
		}

		logger.For(ctx).Debugf("Token ID: %s\tToken Address: %s", pTokenID.String(), contract.Hex())

		turi, err := instance.TokenURI(&bind.CallOpts{
			Context: ctx,
		}, pTokenID.BigInt())
		if err != nil {
			return "", err
		}

		return persist.TokenURI(strings.ReplaceAll(turi, "\x00", "")), nil
	case persist.TokenTypeERC1155:

		instance, err := contracts.NewIERC1155MetadataURICaller(contract, ethClient)
		if err != nil {
			return "", err
		}

		logger.For(ctx).Debugf("Token ID: %d\tToken Address: %s", pTokenID.BigInt().Uint64(), contract.Hex())

		turi, err := instance.Uri(&bind.CallOpts{
			Context: ctx,
		}, pTokenID.BigInt())
		if err != nil {
			return "", err
		}

		return persist.TokenURI(strings.ReplaceAll(turi, "\x00", "")), nil

	default:
		return "", fmt.Errorf("unknown token type: %s", pTokenType)
	}
}

// RetryGetTokenURI calls GetTokenURI with backoff.
func RetryGetTokenURI(ctx context.Context, tokenType persist.TokenType, contractAddress persist.EthereumAddress, tokenID persist.TokenID, ethClient *ethclient.Client, retry Retry) (persist.TokenURI, error) {
	var u persist.TokenURI
	var err error
	for i := 0; i < retry.Tries; i++ {
		u, err = GetTokenURI(ctx, tokenType, contractAddress, tokenID, ethClient)
		if !isRateLimitedError(err) {
			break
		}
		retry.Sleep(i)
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
func RetryGetBalanceOfERC1155Token(ctx context.Context, pOwnerAddress, pContractAddress persist.EthereumAddress, pTokenID persist.TokenID, ethClient *ethclient.Client, retry Retry) (*big.Int, error) {
	var balance *big.Int
	var err error
	for i := 0; i < retry.Tries; i++ {
		balance, err = GetBalanceOfERC1155Token(ctx, pOwnerAddress, pContractAddress, pTokenID, ethClient)
		if !isRateLimitedError(err) {
			break
		}
		retry.Sleep(i)
	}
	return balance, err
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
			return nil, err
		}
		tx = t
		data, err = client.GetTransactionData(currentID)
		if err != nil {
			return nil, err
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
			return nil, err
		}
		if strings.EqualFold(string(decodedName), "Content-Encoding") {
			decodedValue, err := base64.RawURLEncoding.DecodeString(tag.Value)
			if err != nil {
				return nil, err
			}
			if strings.EqualFold(string(decodedValue), "gzip") {
				zipped, err := gzip.NewReader(bytes.NewReader(data))
				if err != nil {
					return nil, err
				}
				buf := new(bytes.Buffer)
				_, err = io.Copy(buf, zipped)
				if err != nil {
					return nil, err
				}
				data = buf.Bytes()
			}
		}
	}
	return util.RemoveBOM(data), nil
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

func (h ErrHTTP) Error() string {
	return fmt.Sprintf("HTTP Error Status - %d | URL - %s", h.Status, h.URL)
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

// powerInt returns the base-x exponential of y.
func powerInt(x, y int) int {
	ret := 1
	for i := 0; i < y; i++ {
		ret *= x
	}
	return ret
}

// minInt returns the minimum of two ints.
func minInt(x, y int) int {
	if x < y {
		return x
	}
	return y
}

func isRateLimitedError(err error) bool {
	if err != nil && strings.Contains(err.Error(), rateLimited) {
		return true
	}
	return false
}
