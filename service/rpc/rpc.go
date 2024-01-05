package rpc

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image/gif"
	"image/jpeg"
	"image/png"
	"io"
	"io/fs"
	"math/big"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"cloud.google.com/go/storage"
	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/everFinance/goar"
	"github.com/getsentry/sentry-go"
	"github.com/googleapis/gax-go/v2"
	shell "github.com/ipfs/go-ipfs-api"
	"golang.org/x/image/bmp"
	"google.golang.org/api/option"
	htransport "google.golang.org/api/transport/http"

	"github.com/mikeydub/go-gallery/contracts"
	"github.com/mikeydub/go-gallery/env"
	"github.com/mikeydub/go-gallery/service/logger"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/service/rpc/ipfs"
	sentryutil "github.com/mikeydub/go-gallery/service/sentry"
	"github.com/mikeydub/go-gallery/service/tracing"
	"github.com/mikeydub/go-gallery/util"
	"github.com/mikeydub/go-gallery/util/retry"
)

func init() {
	env.RegisterValidation("IPFS_URL", "required")
	env.RegisterValidation("FALLBACK_IPFS_URL", "required")
}

const (
	defaultHTTPKeepAlive           = 600
	defaultHTTPMaxIdleConns        = 250
	defaultHTTPMaxIdleConnsPerHost = 250
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

	var client *rpc.Client
	var err error

	if endpoint := env.GetString("RPC_URL"); strings.HasPrefix(endpoint, "https://") {
		client, err = rpc.DialHTTPWithClient(endpoint, defaultHTTPClient)
		if err != nil {
			panic(err)
		}
	} else {
		client, err = rpc.DialContext(ctx, endpoint)
		if err != nil {
			panic(err)
		}
	}

	return ethclient.NewClient(client)
}

// NewEthSocketClient returns a websocket client with request tracing enabled
func NewEthSocketClient() *ethclient.Client {
	if strings.HasPrefix(env.GetString("RPC_URL"), "wss") {
		log.Root().SetHandler(log.FilterHandler(func(r *log.Record) bool {
			if reqID := valFromSlice(r.Ctx, "reqid"); reqID == nil || r.Msg != "Handled RPC response" {
				return false
			}
			return true
		}, defaultMetricsHandler))
	}
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
		bs, err := os.ReadFile(path)
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
	r, _, err := GetDataFromURIAsReader(ctx, turi, turi.Type().ToMediaType(), ipfsClient, arweaveClient, 1024, time.Minute, false)
	if err != nil {
		return meta, err
	}
	defer r.Close()

	// decode the json
	err = json.NewDecoder(r).Decode(&meta)
	if err != nil {
		return meta, err
	}

	return meta, nil

}

// GetDataFromURIAsReader calls URI and returns the data as an unread reader with the headers pre-read.
// retrieveTimeout is the timeout for just the retrieval of the reader, not the reading of the reader which will be handled by the context
// recurseRawReturns will cause the function to recursively call itself if the reader returned from any initial call is a "raw" URI (a URI that in itself contains the data to be retrieved, not a URI that points to the data to be retrieved)
func GetDataFromURIAsReader(ctx context.Context, turi persist.TokenURI, mediaType persist.MediaType, ipfsClient *shell.Shell, arweaveClient *goar.Client, bufSize int, retrieveTimeout time.Duration, recurseRawReturns bool) (*util.FileHeaderReader, persist.MediaType, error) {

	errChan := make(chan error)
	readerChan := make(chan *util.FileHeaderReader)

	go func() {
		d, _ := ctx.Deadline()
		logger.For(ctx).Infof("Getting data from URI: %s -timeout: %s -type: %s", turi.String(), time.Until(d), turi.Type())
		asString := turi.String()

		switch turi.Type() {
		case persist.URITypeBase64JSON, persist.URITypeBase64SVG, persist.URITypeBase64HTML, persist.URITypeBase64WAV, persist.URITypeBase64MP3:
			// decode the base64 encoded json
			b64data := asString[strings.IndexByte(asString, ',')+1:]
			decoded, err := util.Base64Decode(b64data, base64.RawStdEncoding, base64.StdEncoding, base64.RawURLEncoding, base64.URLEncoding)
			if err != nil {
				errChan <- fmt.Errorf("error decoding base64 data: %s \n\n%s", err, b64data)
				return
			}

			buf := bytes.NewBuffer(util.RemoveBOM(decoded))

			readerChan <- util.NewFileHeaderReader(buf, bufSize)
		case persist.URITypeBase64BMP:
			b64data := asString[strings.IndexByte(asString, ',')+1:]
			decoded, err := util.Base64Decode(b64data, base64.RawStdEncoding, base64.StdEncoding, base64.RawURLEncoding, base64.URLEncoding)
			if err != nil {
				errChan <- fmt.Errorf("error decoding base64 bmp data: %s \n\n%s", err, b64data)
				return
			}
			img, err := bmp.Decode(bytes.NewReader(decoded))
			if err != nil {
				errChan <- fmt.Errorf("error decoding bmp data: %s \n\n%s", err, b64data)
				return
			}
			newImage := bytes.NewBuffer(nil)
			err = jpeg.Encode(newImage, img, nil)
			if err != nil {
				errChan <- fmt.Errorf("error encoding jpeg data: %s \n\n%s", err, b64data)
				return
			}
			readerChan <- util.NewFileHeaderReader(newImage, bufSize)
		case persist.URITypeBase64PNG:
			b64data := asString[strings.IndexByte(asString, ',')+1:]
			decoded, err := util.Base64Decode(b64data, base64.RawStdEncoding, base64.StdEncoding, base64.RawURLEncoding, base64.URLEncoding)
			if err != nil {
				errChan <- fmt.Errorf("error decoding base64 png data: %s \n\n%s", err, b64data)
				return
			}
			img, err := png.Decode(bytes.NewReader(decoded))
			if err != nil {
				errChan <- fmt.Errorf("error decoding png data: %s \n\n%s", err, b64data)
				return
			}
			newImage := bytes.NewBuffer(nil)
			err = png.Encode(newImage, img)
			if err != nil {
				errChan <- fmt.Errorf("error encoding png data: %s \n\n%s", err, b64data)
				return
			}
			readerChan <- util.NewFileHeaderReader(newImage, bufSize)
		case persist.URITypeBase64JPEG:
			b64data := asString[strings.IndexByte(asString, ',')+1:]
			decoded, err := util.Base64Decode(b64data, base64.RawStdEncoding, base64.StdEncoding, base64.RawURLEncoding, base64.URLEncoding)
			if err != nil {
				errChan <- fmt.Errorf("error decoding base64 jpeg data: %s \n\n%s", err, b64data)
				return
			}
			img, err := jpeg.Decode(bytes.NewReader(decoded))
			if err != nil {
				errChan <- fmt.Errorf("error decoding jpeg data: %s \n\n%s", err, b64data)
				return
			}
			newImage := bytes.NewBuffer(nil)
			err = jpeg.Encode(newImage, img, nil)
			if err != nil {
				errChan <- fmt.Errorf("error encoding jpeg data: %s \n\n%s", err, b64data)
				return
			}
			readerChan <- util.NewFileHeaderReader(newImage, bufSize)
		case persist.URITypeBase64GIF:
			b64data := asString[strings.IndexByte(asString, ',')+1:]
			decoded, err := util.Base64Decode(b64data, base64.RawStdEncoding, base64.StdEncoding, base64.RawURLEncoding, base64.URLEncoding)
			if err != nil {
				errChan <- fmt.Errorf("error decoding base64 gif data: %s \n\n%s", err, b64data)
				return
			}
			img, err := gif.Decode(bytes.NewReader(decoded))
			if err != nil {
				errChan <- fmt.Errorf("error decoding gif data: %s \n\n%s", err, b64data)
				return
			}
			newGif := bytes.NewBuffer(nil)
			err = gif.Encode(newGif, img, nil)
			if err != nil {
				errChan <- fmt.Errorf("error encoding gif data: %s \n\n%s", err, b64data)
				return
			}
			readerChan <- util.NewFileHeaderReader(newGif, bufSize)
		case persist.URITypeArweave, persist.URITypeArweaveGateway:
			path := util.GetURIPath(asString, true)

			resp, err := GetArweaveDataHTTPReader(ctx, path)
			if err != nil {
				errChan <- err
				return
			}

			readerChan <- util.NewFileHeaderReader(resp, bufSize)
		case persist.URITypeIPFS:
			path := util.GetURIPath(asString, true)
			resp, err := ipfs.GetResponse(ctx, path)
			if err != nil {
				errChan <- err
				return
			}

			readerChan <- util.NewFileHeaderReader(resp, bufSize)
		case persist.URITypeIPFSGateway:
			path := util.GetURIPath(asString, false)
			resp, err := ipfs.GetResponse(ctx, path)
			if err != nil {
				logger.For(ctx).Errorf("Error getting data from IPFS: %s", err)
			} else {
				readerChan <- util.NewFileHeaderReader(resp, bufSize)
				return
			}
			fallthrough
		case persist.URITypeHTTP:
			req, err := http.NewRequestWithContext(ctx, "GET", asString, nil)
			if err != nil {
				errChan <- fmt.Errorf("error creating request: %s", err)
				return
			}
			resp, err := defaultHTTPClient.Do(req)
			if err != nil {
				if dnsErr, ok := err.(*net.DNSError); ok {
					errChan <- dnsErr
					return
				}
				if urlErr, ok := err.(*url.Error); ok {
					errChan <- urlErr
					return
				}
				errChan <- fmt.Errorf("error getting data from http: %s <%T>", err, err)
				return
			}
			if resp.StatusCode > 399 || resp.StatusCode < 200 {
				errChan <- util.ErrHTTP{Status: resp.StatusCode, URL: asString}
				return
			}
			readerChan <- util.NewFileHeaderReader(resp.Body, bufSize)
		case persist.URITypeIPFSAPI:
			parsedURL, err := url.Parse(asString)
			if err != nil {
				errChan <- err
				return
			}
			path := parsedURL.Query().Get("arg")
			resp, err := ipfs.GetResponse(ctx, path)
			if err != nil {
				errChan <- err
				return
			}

			readerChan <- util.NewFileHeaderReader(resp, bufSize)
		case persist.URITypeJSON, persist.URITypeSVG:
			// query unescape asString first
			if needsUnescape(asString) {
				escaped, err := url.QueryUnescape(asString)
				if err == nil {
					logger.For(ctx).Warnf("error unescaping uri: %s", err)
				} else {
					asString = escaped
				}
			}
			if strings.HasPrefix(asString, "data:") {
				idx := strings.IndexByte(asString, ',')
				if idx != -1 {
					buf := bytes.NewBuffer(util.RemoveBOM([]byte(asString[idx+1:])))
					readerChan <- util.NewFileHeaderReader(buf, bufSize)
					return
				}
			}
			buf := bytes.NewBuffer(util.RemoveBOM([]byte(asString)))
			readerChan <- util.NewFileHeaderReader(buf, bufSize)
		default:
			buf := bytes.NewBuffer([]byte(turi))
			readerChan <- util.NewFileHeaderReader(buf, bufSize)
		}
	}()

	select {
	case <-ctx.Done():
		return nil, mediaType, ctx.Err()
	case err := <-errChan:
		return nil, mediaType, err
	case reader := <-readerChan:
		h, err := reader.Headers()
		if err != nil {
			return nil, mediaType, err
		}
		uriType := persist.TokenURI(h).Type()
		logger.For(ctx).Debugf("uriType for recurse: %s", uriType)
		if recurseRawReturns && uriType.IsRaw() {
			logger.For(ctx).Infof("recurseRawReturns is true, recursing on raw uri: %s", util.TruncateWithEllipsis(string(h), 50))
			full := &bytes.Buffer{}
			_, err := io.Copy(full, reader)
			if err != nil {
				return nil, mediaType, err
			}

			return GetDataFromURIAsReader(ctx, persist.TokenURI(full.String()), uriType.ToMediaType(), ipfsClient, arweaveClient, full.Len(), retrieveTimeout, false)
		}
		return reader, mediaType, nil
	case <-time.After(retrieveTimeout):
		return nil, mediaType, fmt.Errorf("%s: timeout retrieving data from uri: %s", context.DeadlineExceeded.Error(), turi.String())
	}
}

func needsUnescape(str string) bool {
	// Regex to match percent-encoded characters
	re := regexp.MustCompile(`%[0-9a-fA-F]{2}`)
	return re.MatchString(str)
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
	defer resp.Body.Close()

	if resp.StatusCode > 399 || resp.StatusCode < 200 {
		return nil, util.ErrHTTP{Status: resp.StatusCode, URL: url}
	}

	return resp.Header, nil
}

// GetHTTPHeaders returns the headers for the given URL
func GetHTTPHeaders(ctx context.Context, url string) (http.Header, error) {
	contentHeader := func(method, url string) func(ctx context.Context) (http.Header, error) {
		return func(ctx context.Context) (http.Header, error) { return getHeaders(ctx, method, url) }
	}
	return util.FirstNonErrorWithValue(ctx, true, nil,
		contentHeader(http.MethodHead, url),
		contentHeader(http.MethodGet, url),
	)
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

		return persist.TokenURI(strings.ReplaceAll(turi, "\x00", "")).ReplaceID(pTokenID), nil

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

func GetArweaveDataHTTPReader(ctx context.Context, id string) (io.ReadCloser, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("https://arweave.net/%s", id), nil)
	if err != nil {
		return nil, fmt.Errorf("error getting data: %s", err.Error())
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		url := ""
		if req.URL != nil {
			url = req.URL.String()
		}
		var status int
		if resp != nil {
			status = resp.StatusCode
		}
		return nil, util.ErrHTTP{Err: err, URL: url, Status: status}
	}
	return resp.Body, nil
}

func GetArweaveDataHTTP(ctx context.Context, id string) ([]byte, error) {
	resp, err := GetArweaveDataHTTPReader(ctx, id)
	if err != nil {
		return nil, err
	}
	defer resp.Close()
	data, err := io.ReadAll(resp)
	if err != nil {
		return nil, fmt.Errorf("error reading data: %s", err.Error())
	}
	return data, nil
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
