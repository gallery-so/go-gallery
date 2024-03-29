package graphql_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mikeydub/go-gallery/server"
	"github.com/mikeydub/go-gallery/service/multichain"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/service/recommend"
	"github.com/mikeydub/go-gallery/service/recommend/userpref"
	"github.com/mikeydub/go-gallery/service/task"
	"github.com/mikeydub/go-gallery/tokenprocessing"
	"github.com/mikeydub/go-gallery/util"
	"github.com/stretchr/testify/mock"
)

// stubProvider returns a canned set of tokens and contracts
type stubProvider struct {
	Contracts     []multichain.ChainAgnosticContract
	Tokens        []multichain.ChainAgnosticToken
	FetchMetadata func() (persist.TokenMetadata, error)
	RetErr        error
}

func (p stubProvider) GetTokensByWalletAddress(ctx context.Context, address persist.Address) ([]multichain.ChainAgnosticToken, []multichain.ChainAgnosticContract, error) {
	return p.Tokens, p.Contracts, p.RetErr
}

func (p stubProvider) GetTokensIncrementallyByWalletAddress(ctx context.Context, address persist.Address) (<-chan multichain.ChainAgnosticTokensAndContracts, <-chan error) {
	recCh := make(chan multichain.ChainAgnosticTokensAndContracts)
	errCh := make(chan error)
	go func() {
		defer close(recCh)
		defer close(errCh)
		if p.RetErr != nil {
			errCh <- p.RetErr
			return
		}
		recCh <- multichain.ChainAgnosticTokensAndContracts{
			Tokens:    p.Tokens,
			Contracts: p.Contracts,
		}
	}()
	return recCh, errCh
}

func (p stubProvider) GetTokenMetadataByTokenIdentifiers(ctx context.Context, ti multichain.ChainAgnosticIdentifiers) (persist.TokenMetadata, error) {
	return p.FetchMetadata()
}

func (p stubProvider) GetTokensByContractAddress(ctx context.Context, contract persist.Address, limit int, offset int) ([]multichain.ChainAgnosticToken, multichain.ChainAgnosticContract, error) {
	panic("not implemented")
}

func (p stubProvider) GetTokensByContractAddressAndOwner(ctx context.Context, owner persist.Address, contract persist.Address, limit, offset int) ([]multichain.ChainAgnosticToken, multichain.ChainAgnosticContract, error) {
	panic("not implemented")
}

func (p stubProvider) GetTokenByTokenIdentifiersAndOwner(context.Context, multichain.ChainAgnosticIdentifiers, persist.Address) (multichain.ChainAgnosticToken, multichain.ChainAgnosticContract, error) {
	panic("not implemented")
}

type providerOpt func(*stubProvider)

func newStubProvider(opts ...providerOpt) stubProvider {
	provider := stubProvider{}
	for _, opt := range opts {
		opt(&provider)
	}
	return provider
}

// withContracts configures the stubProvider to return a canned set of contracts
func withContracts(contracts []multichain.ChainAgnosticContract) providerOpt {
	return func(p *stubProvider) {
		p.Contracts = contracts
	}
}

// withTokens configures the stubProvider to return a canned set of tokens
func withTokens(tokens []multichain.ChainAgnosticToken) providerOpt {
	return func(p *stubProvider) {
		p.Tokens = tokens
	}
}

// withReturnError configures the stubProvider to return an error
func withReturnError(err error) providerOpt {
	return func(p *stubProvider) {
		p.RetErr = err
	}
}

// withDummyTokenN will generate n dummy tokens from the provided contract
func withDummyTokenN(contract multichain.ChainAgnosticContract, ownerAddress persist.Address, n int) providerOpt {
	return func(p *stubProvider) {
		tokens := []multichain.ChainAgnosticToken{}
		for i := 0; i < n; i++ {
			tokenID := persist.HexTokenID(fmt.Sprintf("%X", i))
			token := dummyTokenIDContract(ownerAddress, contract.Address, tokenID)
			tokens = append(tokens, token)
		}
		withContracts([]multichain.ChainAgnosticContract{contract})(p)
		withTokens(tokens)(p)
	}
}

// withDummyTokenID will generate a token with the provided token ID
func withDummyTokenID(ownerAddress persist.Address, tokenID persist.HexTokenID) providerOpt {
	c := multichain.ChainAgnosticContract{Address: "0x123"}
	return func(p *stubProvider) {
		withContracts([]multichain.ChainAgnosticContract{c})(p)
		withTokens([]multichain.ChainAgnosticToken{dummyTokenIDContract(ownerAddress, c.Address, tokenID)})(p)
	}
}

// withFetchMetadata configures how the provider will retrieve metadata
func withFetchMetadata(f func() (persist.TokenMetadata, error)) providerOpt {
	return func(p *stubProvider) {
		p.FetchMetadata = f
	}
}

// defaultStubProvider returns a stubProvider that returns dummy tokens
func defaultStubProvider(ownerAddress persist.Address) stubProvider {
	contract := multichain.ChainAgnosticContract{Address: "0x123", Descriptors: multichain.ChainAgnosticContractDescriptors{Name: "testContract"}}
	return newStubProvider(withDummyTokenN(contract, ownerAddress, 10))
}

// newStubRecommender returns a recommender that returns a canned set of recommendations
func newStubRecommender(t *testing.T, userIDs []persist.DBID) *recommend.Recommender {
	return &recommend.Recommender{
		LoadFunc:      func(context.Context) {},
		BootstrapFunc: func(context.Context) ([]persist.DBID, error) { return userIDs, nil },
	}
}

// newStubPersonalization returns a stub of the personalization service
func newStubPersonaliztion(t *testing.T) *userpref.Personalization {
	return &userpref.Personalization{}
}

// sendTokensRecorder records tokenprocessing messages
type sendTokensRecorder struct {
	mock.Mock
	SubmitTokens multichain.SubmitTokensF
	Tasks        []task.TokenProcessingBatchMessage
}

func (r *sendTokensRecorder) Send(ctx context.Context, tDefIDs []persist.DBID) error {
	r.Called(ctx, tDefIDs)
	r.Tasks = append(r.Tasks, task.TokenProcessingBatchMessage{BatchID: persist.GenerateID(), TokenDefinitionIDs: tDefIDs})
	return nil
}

// submitUserTokensNoop is useful when the code under test doesn't require tokenprocessing
func submitUserTokensNoop(ctx context.Context, tDefIDs []persist.DBID) error {
	return nil
}

// sendTokensToHTTPHandler makes an HTTP request to the passed handler
func sendTokensToHTTPHandler(handler http.Handler, method, endpoint string) multichain.SubmitTokensF {
	return func(ctx context.Context, tDefIDs []persist.DBID) error {
		m := task.TokenProcessingBatchMessage{BatchID: persist.GenerateID(), TokenDefinitionIDs: tDefIDs}
		byt, _ := json.Marshal(m)
		r := bytes.NewReader(byt)
		req := httptest.NewRequest(method, endpoint, r)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		res := w.Result()
		if res.StatusCode != http.StatusOK {
			return util.BodyAsError(res)
		}
		return nil
	}
}

// sendTokensToTokenProcessing processes a batch of tokens synchronously through tokenprocessing
func sendTokensToTokenProcessing(ctx context.Context, c *server.Clients, provider *multichain.Provider) multichain.SubmitTokensF {
	return func(ctx context.Context, tDefIDs []persist.DBID) error {
		h := tokenprocessing.CoreInitServer(ctx, c, provider)
		return sendTokensToHTTPHandler(h, http.MethodPost, "/media/process")(ctx, tDefIDs)
	}
}

// fetchFromDummyEndpoint fetches metadata from the given endpoint
func fetchFromDummyEndpoint(url, endpoint string) func() (persist.TokenMetadata, error) {
	return func() (persist.TokenMetadata, error) {
		r := httptest.NewRequest(http.MethodGet, url+endpoint, nil)
		r.RequestURI = ""

		res, err := http.DefaultClient.Do(r)
		if err != nil {
			return nil, err
		}
		defer res.Body.Close()

		body, err := io.ReadAll(res.Body)
		if err != nil {
			return nil, err
		}

		// Don't try to JSON parse base64 encoded data
		if strings.HasPrefix(string(body), "data:application/json;base64,") {
			body = []byte(strings.Split(string(body), ",")[1])
			body, err = util.Base64Decode(string(body), base64.StdEncoding, base64.RawStdEncoding)
			if err != nil {
				return nil, err
			}
		}

		var meta persist.TokenMetadata
		err = json.Unmarshal(body, &meta)

		return meta, err
	}
}
