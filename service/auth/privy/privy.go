package privy

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/golang-jwt/jwt/v5"
	"github.com/jackc/pgx/v4"
	"github.com/mikeydub/go-gallery/db/gen/coredb"
	"github.com/mikeydub/go-gallery/env"
	"github.com/mikeydub/go-gallery/service/auth"
	"github.com/mikeydub/go-gallery/service/logger"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/service/persist/postgres"
	"github.com/mikeydub/go-gallery/service/sentry"
	"github.com/mikeydub/go-gallery/util"
	"net/http"
)

const usersURL = "https://auth.privy.io/api/v1/users"

type PrivyAuthenticator struct {
	userRepo       *postgres.UserRepository
	queries        *coredb.Queries
	privyClient    *Client
	privyAuthToken string
}

func (a PrivyAuthenticator) GetDescription() string {
	return fmt.Sprintf("PrivyAuthenticator")
}

func NewAuthenticator(userRepo *postgres.UserRepository, queries *coredb.Queries, privyClient *Client, privyAuthToken string) PrivyAuthenticator {
	return PrivyAuthenticator{
		userRepo:       userRepo,
		queries:        queries,
		privyClient:    privyClient,
		privyAuthToken: privyAuthToken,
	}
}

func (a PrivyAuthenticator) Authenticate(ctx context.Context) (*auth.AuthResult, error) {
	privyDID, err := a.privyClient.parseAuthToken(ctx, a.privyAuthToken)
	if err != nil {
		return nil, err
	}

	details, err := a.privyClient.getUserDetails(ctx, privyDID)
	if err != nil {
		return nil, err
	}

	var user *coredb.User

	// Look for an existing user, but don't error if there isn't one
	u, err := a.queries.GetUserByPrivyDID(ctx, privyDID)
	if err != nil {
		if !errors.Is(err, pgx.ErrNoRows) {
			return nil, err
		}
	} else {
		user = &u
	}

	// If we didn't find a user by Privy DID, see if we can find one by verified email address
	if user == nil && details.EmailAddress != nil {
		u, err = a.queries.GetUserByVerifiedEmailAddress(ctx, string(*details.EmailAddress))
		if err != nil {
			if !errors.Is(err, pgx.ErrNoRows) {
				return nil, err
			}
		} else {
			user = &u
		}
	}

	authResult := auth.AuthResult{
		User:     user,
		Email:    details.EmailAddress,
		PrivyDID: util.ToPointer(privyDID),
	}

	for _, wallet := range details.Wallets {
		authResult.Addresses = append(authResult.Addresses, auth.AuthenticatedAddress{
			ChainAddress: wallet,
			WalletType:   persist.WalletTypeEOA,
		})
	}

	return &authResult, nil
}

func NewPrivyClient(httpClient *http.Client) *Client {
	appID := env.GetString("PRIVY_APP_ID")
	appSecret := env.GetString("PRIVY_APP_SECRET")
	jwtPublicKey := env.GetString("PRIVY_APP_VERIFICATION_PUBLIC_KEY")

	return &Client{
		httpClient:   httpClient,
		appID:        appID,
		appSecret:    appSecret,
		jwtPublicKey: jwtPublicKey,
	}
}

type Client struct {
	httpClient   *http.Client
	appID        string
	appSecret    string
	jwtPublicKey string
}

type userResponse struct {
	ID             string          `json:"id"`
	CreatedAt      uint64          `json:"created_at"`
	LinkedAccounts []linkedAccount `json:"linked_accounts"`
}

type linkedAccount struct {
	Type             string `json:"type"`
	Address          string `json:"address"`
	ChainType        string `json:"chain_type"`
	ChainID          string `json:"chain_id"`
	WalletClient     string `json:"wallet_client"`
	WalletClientType string `json:"wallet_client_type"`
	ConnectorType    string `json:"connector_type"`
	VerifiedAt       uint64 `json:"verified_at"`
}

type privyClaims struct {
	jwt.RegisteredClaims
}

// parseAuthToken parses a Privy JWT token and returns the privy DID.
// See: https://docs.privy.io/guide/server/authorization/verification
func (c *Client) parseAuthToken(ctx context.Context, token string) (string, error) {
	privyKeyFunc := func(token *jwt.Token) (interface{}, error) {
		if token.Method.Alg() != "ES256" {
			return nil, fmt.Errorf("unexpected JWT signing method=%v", token.Header["alg"])
		}

		return jwt.ParseECPublicKeyFromPEM([]byte(c.jwtPublicKey))
	}

	claims := privyClaims{}
	parsedToken, err := jwt.ParseWithClaims(token, &claims, privyKeyFunc)

	if err != nil || !parsedToken.Valid {
		return "", auth.ErrInvalidJWT
	}

	if len(claims.Audience) != 1 || claims.Audience[0] != c.appID {
		return "", errors.New("audience claim must be your Privy App ID")
	}

	if claims.Issuer != "privy.io" {
		return "", errors.New("issuer claim must be 'privy.io'")
	}

	return claims.Subject, nil
}

type userDetails struct {
	EmailAddress *persist.Email
	Wallets      []persist.ChainAddress
}

func (c *Client) getUserDetails(ctx context.Context, privyDID string) (userDetails, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("%s/%s", usersURL, privyDID), http.NoBody)
	if err != nil {
		return userDetails{}, err
	}

	req.SetBasicAuth(c.appID, c.appSecret)
	req.Header.Add("privy-app-id", c.appID)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return userDetails{}, err
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return userDetails{}, util.GetErrFromResp(resp)
	}

	var output userResponse
	err = json.NewDecoder(resp.Body).Decode(&output)
	if err != nil {
		return userDetails{}, err
	}

	details := userDetails{}

	for _, account := range output.LinkedAccounts {
		if account.Type == "email" {
			details.EmailAddress = util.ToPointer(persist.Email(account.Address))
		} else if account.Type == "wallet" {
			chain, err := privyChainToPersistChain(account.ChainType)
			if err != nil {
				err := fmt.Errorf("error parsing Privy chain type: %w", err)
				logger.For(ctx).Error(err)
				sentryutil.ReportError(ctx, err)
				continue
			}
			details.Wallets = append(details.Wallets, persist.NewChainAddress(persist.Address(account.Address), chain))
		}
	}

	return details, nil
}

func privyChainToPersistChain(privyChain string) (persist.Chain, error) {
	// According to Privy docs, "ethereum" is currently the only valid chain_type
	switch privyChain {
	case "ethereum":
		return persist.ChainETH, nil
	default:
		return -1, fmt.Errorf("unrecognized chain type: %s", privyChain)
	}
}
