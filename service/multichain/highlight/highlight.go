package highlight

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/shurcooL/graphql"

	"github.com/mikeydub/go-gallery/env"
	"github.com/mikeydub/go-gallery/service/logger"
	"github.com/mikeydub/go-gallery/service/persist"
	sentryutil "github.com/mikeydub/go-gallery/service/sentry"
)

const (
	baseURL                  = "https://api.highlight.xyz:8080"
	txCodeInitiated          = "INITIATED"
	txCodeSent               = "TX_SENT"
	txCodeReverted           = "TX_REVERTED"
	txCodeCancelled          = "TX_CANCELLED"
	txCodeComplete           = "TX_COMPLETE"
	claimCodeMintUnavailable = "MINT_UNAVAILABLE"
	// Lifecycle of a highlight mint
	ClaimStatusTxPending       ClaimStatus = "TX_PENDING"
	ClaimStatusTxFailed        ClaimStatus = "TX_FAILED"
	ClaimStatusTxSucceeded     ClaimStatus = "TX_SUCCEEDED"
	ClaimStatusMediaProcessing ClaimStatus = "MEDIA_PROCESSING"
	ClaimStatusMediaProcessed  ClaimStatus = "MEDIA_PROCESSED"
	ClaimStatusMediaFailed     ClaimStatus = "MEDIA_FAILED"
	ClaimStatusFailedInternal  ClaimStatus = "FAILED_INTERNAL"
)

var ErrHighlightChainNotSupported = errors.New("chain is not supported by highlight")

// ClaimStatus represents the progress of a Highlight mint
type ClaimStatus string

type ErrHighlightTxnFailed struct{ Msg string }
type ErrHighlightCollectionMintUnavailable struct{ CollectionID string }
type ErrHighlightInternalError struct{ Err error }

func (e ErrHighlightTxnFailed) Error() string { return e.Msg }

func (e ErrHighlightCollectionMintUnavailable) Error() string {
	return fmt.Sprintf("collectionID=%s is unavailable for minting", e.CollectionID)
}

func (e ErrHighlightInternalError) Unwrap() error { return e.Err }
func (e ErrHighlightInternalError) Error() string {
	return fmt.Sprintf("unexpected reply from highlight: %s", e.Err)
}

// authT adds authentication headers to each request
type authT struct {
	transport        http.RoundTripper
	appID, appSecret string
}

func (t *authT) RoundTrip(r *http.Request) (*http.Response, error) {
	r.Header.Add("x-app-id", t.appID)
	r.Header.Add("x-app-secret", t.appSecret)
	if t.transport == nil {
		return http.DefaultTransport.RoundTrip(r)
	}
	return t.transport.RoundTrip(r)
}

var chainIDtoChain = map[int]persist.Chain{
	84532: persist.ChainBaseSepolia,
	8453:  persist.ChainBase,
}

func mustChainIDToChain(chainID int) persist.Chain {
	chain, ok := chainIDtoChain[chainID]
	if !ok {
		panic(fmt.Sprintf("unknown chainID=%d", chainID))
	}
	return chain
}

type Provider struct{ gql *graphql.Client }

func NewProvider(c *http.Client) *Provider {
	appID := env.GetString("HIGHLIGHT_APP_ID")
	if appID == "" {
		panic("HIGHLIGHT_APP_ID is not set")
	}
	appSecret := env.GetString("HIGHLIGHT_APP_SECRET")
	if appSecret == "" {
		panic("HIGHLIGHT_APP_SECRET is not set")
	}
	authedC := *c
	authedC.Transport = &authT{
		transport: c.Transport,
		appID:     appID,
		appSecret: appSecret,
	}
	return &Provider{gql: graphql.NewClient(baseURL, &authedC)}
}

type mintMutation struct {
	ClaimByCollectionApp struct {
		ClaimId     string
		ClaimStatus string
		Address     string
		ChainID     float64
	} `graphql:"claimByCollectionApp(collectionId: $collectionId, data: {numTokensToMint: $qty mintRecipient: $recipient})"`
}

func (api *Provider) ClaimMint(ctx context.Context, collectionID string, qty int, recipient persist.ChainAddress) (claimID string, status ClaimStatus, chainAddress persist.ChainAddress, err error) {
	if recipient.Chain().L1Chain() != persist.L1Chain(persist.ChainETH) {
		return "", "", persist.ChainAddress{}, ErrHighlightChainNotSupported
	}

	var m mintMutation

	err = api.gql.Mutate(ctx, &m, map[string]any{
		"collectionId": graphql.String(collectionID),
		"qty":          graphql.Float(qty),
		"recipient":    graphql.String(recipient.Address().String()),
	})
	// Minting is closed
	if err != nil && err.Error() == claimCodeMintUnavailable {
		err = ErrHighlightCollectionMintUnavailable{CollectionID: collectionID}
		logger.For(ctx).Error(err)
		sentryutil.ReportError(ctx, err)
		return "", "", persist.ChainAddress{}, err
	}
	// Unknown error
	if err != nil {
		err = ErrHighlightInternalError{err}
		logger.For(ctx).Error(err)
		sentryutil.ReportError(ctx, err)
		return "", "", persist.ChainAddress{}, err
	}

	chainAddress = persist.NewChainAddress(persist.Address(m.ClaimByCollectionApp.Address), mustChainIDToChain(int(m.ClaimByCollectionApp.ChainID)))

	// Txn failed
	if status := m.ClaimByCollectionApp.ClaimStatus; status == txCodeReverted || status == txCodeCancelled {
		err := ErrHighlightTxnFailed{fmt.Sprintf("failed to execute mint transaction for collectionID=%s; chain=%d; address=%s: %s", collectionID, persist.ChainBaseSepolia, recipient.Address(), status)}
		logger.For(ctx).Error(err)
		sentryutil.ReportError(ctx, err)
		return "", ClaimStatusTxFailed, chainAddress, err
	}

	return m.ClaimByCollectionApp.ClaimId, ClaimStatusTxPending, chainAddress, nil
}

type tokenMetadata struct {
	Name         string
	Description  string
	ImageUrl     string
	AnimationUrl string
}

type token struct {
	TokenId  float64
	ChainId  float64
	Address  string
	Metadata tokenMetadata
}

type checkStatusQuery struct {
	ClaimStatusApp struct {
		Status     string
		RevertCode string
		Nfts       []token
	} `graphql:"claimStatusApp(claimId: $claimId)"`
}

func (api *Provider) GetClaimStatus(ctx context.Context, claimID string) (ClaimStatus, persist.DecimalTokenID, persist.TokenMetadata, error) {
	var q checkStatusQuery

	err := api.gql.Query(ctx, &q, map[string]any{"claimId": graphql.String(claimID)})
	if err != nil {
		return ClaimStatusFailedInternal, "", persist.TokenMetadata{}, err
	}

	status := q.ClaimStatusApp.Status
	revertCode := q.ClaimStatusApp.RevertCode

	if status == txCodeInitiated || status == txCodeSent {
		return ClaimStatusTxPending, "", persist.TokenMetadata{}, nil
	}

	if status == txCodeReverted || status == txCodeCancelled {
		return ClaimStatusTxFailed, "", persist.TokenMetadata{}, fmt.Errorf("claimID=%s transaction failed: %s", claimID, string(revertCode))
	}

	if status != txCodeComplete {
		err = fmt.Errorf("claimID=%s has an unknown status: '%s'; revertCode: %s", claimID, status, revertCode)
		return ClaimStatusFailedInternal, "", persist.TokenMetadata{}, err
	}

	if len(q.ClaimStatusApp.Nfts) <= 0 {
		return ClaimStatusFailedInternal, "", persist.TokenMetadata{}, fmt.Errorf("no tokens were minted from claimID=%s, but transaction succeeded", claimID)
	}

	// It's possible to mint more than one NFT in a transaction, but we only support claiming a single NFT for now.
	token := q.ClaimStatusApp.Nfts[0]
	tokenID := persist.DecimalTokenID(fmt.Sprint(int(token.TokenId)))
	metadata := persist.TokenMetadata{
		"name":          token.Metadata.Name,
		"description":   token.Metadata.Description,
		"image_url":     token.Metadata.ImageUrl,
		"animation_url": token.Metadata.AnimationUrl,
	}
	return ClaimStatusTxSucceeded, tokenID, metadata, nil
}
