package twitter

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"

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
	Data struct {
		ID              string `json:"id"`
		Name            string `json:"name"`
		Username        string `json:"username"`
		ProfileImageURL string `json:"profile_image_url"`
	} `json:"data"`
}

type API struct {
	httpClient *http.Client
	queries    *coredb.Queries
}

type TwitterIdentifiers struct {
	ID              string
	Username        string
	Name            string
	ProfileImageURL string
}

func NewAPI(queries *coredb.Queries) *API {
	httpClient := &http.Client{}
	httpClient.Transport = tracing.NewTracingTransport(http.DefaultTransport, false)

	return &API{
		httpClient: httpClient,
		queries:    queries,
	}
}

// GetAuthedUserFromCode creates a new twitter API client with an auth code
func (a *API) GetAuthedUserFromCode(ctx context.Context, userID persist.DBID, authCode string) (TwitterIdentifiers, AccessTokenResponse, error) {

	accessToken, err := a.generateAuthTokenFromCode(ctx, userID, authCode)
	if err != nil {
		return TwitterIdentifiers{}, AccessTokenResponse{}, err
	}

	tids, err := a.getAuthedUser(ctx, accessToken.AccessToken)
	if err != nil {
		return TwitterIdentifiers{}, AccessTokenResponse{}, err
	}
	return tids, accessToken, nil
}

func (a *API) generateAuthTokenFromCode(ctx context.Context, userID persist.DBID, authCode string) (AccessTokenResponse, error) {

	q := url.Values{}
	q.Set("code", authCode)
	q.Set("grant_type", "authorization_code")
	q.Set("redirect_uri", viper.GetString("TWITTER_AUTH_REDIRECT_URI"))
	q.Set("code_verifier", "challenge")
	encoded := q.Encode()

	accessReq, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.twitter.com/2/oauth2/token", strings.NewReader(encoded))
	if err != nil {
		return AccessTokenResponse{}, fmt.Errorf("failed to create access token request: %w", err)
	}

	accessReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	accessReq.Header.Set("Content-Length", fmt.Sprintf("%d", len(encoded)))
	accessReq.Header.Set("Accept", "*/*")

	accessReq.SetBasicAuth(viper.GetString("TWITTER_CLIENT_ID"), viper.GetString("TWITTER_CLIENT_SECRET"))

	accessResp, err := http.DefaultClient.Do(accessReq)
	if err != nil {
		err = util.GetErrFromResp(accessResp)
		return AccessTokenResponse{}, fmt.Errorf("failed to get access token: %s", err)

	}

	defer accessResp.Body.Close()

	if accessResp.StatusCode != http.StatusOK {
		err = util.GetErrFromResp(accessResp)
		return AccessTokenResponse{}, fmt.Errorf("failed to get access token, returned status: %s", err)
	}

	var accessToken AccessTokenResponse
	if err := json.NewDecoder(accessResp.Body).Decode(&accessToken); err != nil {
		return AccessTokenResponse{}, err
	}

	return accessToken, nil
}

func (a *API) getAuthedUser(ctx context.Context, accessToken string) (TwitterIdentifiers, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.twitter.com/2/users/me?user.fields=profile_image_url", nil)
	if err != nil {
		return TwitterIdentifiers{}, err
	}

	req.Header.Add("Authorization", "Bearer "+accessToken)

	resp, err := a.httpClient.Do(req)
	if err != nil {
		return TwitterIdentifiers{}, fmt.Errorf("failed to get user me: %w", err)
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		err = util.GetErrFromResp(resp)
		return TwitterIdentifiers{}, fmt.Errorf("failed to get user me: %s", err)
	}

	var userMe GetUserMeResponse
	if err := json.NewDecoder(resp.Body).Decode(&userMe); err != nil {
		return TwitterIdentifiers{}, err
	}

	return TwitterIdentifiers{
		ID:              userMe.Data.ID,
		Username:        userMe.Data.Username,
		ProfileImageURL: userMe.Data.ProfileImageURL,
		Name:            userMe.Data.Name,
	}, nil
}
