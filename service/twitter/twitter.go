package twitter

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/mikeydub/go-gallery/db/gen/coredb"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/service/tracing"
	"github.com/mikeydub/go-gallery/util"
	"github.com/spf13/viper"
)

type AccessTokenResponse struct {
	TokenType    string `json:"token_type"`
	AccessToken  string `json:"access_token"`
	Scope        string `json:"scope"`
	RefreshToken string `json:"refresh_token"`
}

type GetUserMeResponse struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Username string `json:"username"`
}

type API struct {
	httpClient *http.Client
	queries    *coredb.Queries

	userID persist.DBID
}

func NewAPI(userID persist.DBID, queries *coredb.Queries) *API {
	httpClient := &http.Client{}
	httpClient.Transport = tracing.NewTracingTransport(http.DefaultTransport, false)

	return &API{
		httpClient: httpClient,
		queries:    queries,
		userID:     userID,
	}
}

// GetAuthedUserFromCode creates a new twitter API client with an auth code
func (a *API) GetAuthedUserFromCode(ctx context.Context, authCode string) (persist.SocialUserIdentifers, error) {

	accessToken, err := a.generateAuthTokenFromCode(ctx, authCode)
	if err != nil {
		return persist.SocialUserIdentifers{}, err
	}

	return a.getAuthedUser(ctx, accessToken.AccessToken)
}

func (a *API) generateAuthTokenFromCode(ctx context.Context, authCode string) (AccessTokenResponse, error) {
	accessReq, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.twitter.com/oauth2/token", nil)
	if err != nil {
		return AccessTokenResponse{}, err
	}

	accessReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	q := accessReq.URL.Query()
	q.Set("grant_type", "authorization_code")
	q.Set("code", authCode)
	q.Set("redirect_uri", viper.GetString("TWITTER_AUTH_REDIRECT_URI"))
	q.Set("code_verifier", "challenge")
	accessReq.URL.RawQuery = q.Encode()

	accessReq.SetBasicAuth(viper.GetString("TWITTER_CLIENT_ID"), viper.GetString("TWITTER_CLIENT_SECRET"))

	accessResp, err := a.httpClient.Do(accessReq)
	if err != nil {
		return AccessTokenResponse{}, err
	}

	defer accessResp.Body.Close()

	if accessResp.StatusCode != http.StatusOK {
		return AccessTokenResponse{}, err
	}

	var accessToken AccessTokenResponse
	if err := json.NewDecoder(accessResp.Body).Decode(&accessToken); err != nil {
		return AccessTokenResponse{}, err
	}

	err = a.queries.UpsertSocialMediaOAuth(ctx, coredb.UpsertSocialMediaOAuthParams{
		ID:           persist.GenerateID(),
		UserID:       a.userID,
		Provider:     persist.SocialProviderTwitter,
		AccessToken:  util.ToNullString(accessToken.AccessToken),
		RefreshToken: util.ToNullString(accessToken.RefreshToken),
	})
	if err != nil {
		return AccessTokenResponse{}, err
	}
	return accessToken, nil
}

func (a *API) getAuthedUser(ctx context.Context, accessToken string) (persist.SocialUserIdentifers, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.twitter.com/2/users/me", nil)
	if err != nil {
		return persist.SocialUserIdentifers{}, err
	}

	req.Header.Add("Authorization", "Bearer "+accessToken)

	resp, err := a.httpClient.Do(req)
	if err != nil {
		return persist.SocialUserIdentifers{}, err
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return persist.SocialUserIdentifers{}, err
	}

	var userMe GetUserMeResponse
	if err := json.NewDecoder(resp.Body).Decode(&userMe); err != nil {
		return persist.SocialUserIdentifers{}, err
	}

	return persist.SocialUserIdentifers{
		Provider: persist.SocialProviderTwitter,
		ID:       userMe.ID,
		Metadata: map[string]interface{}{
			"username": userMe.Username,
			"name":     userMe.Name,
		},
	}, nil
}
