package twitter

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/mikeydub/go-gallery/db/gen/coredb"
	"github.com/mikeydub/go-gallery/service/redis"
	"github.com/mikeydub/go-gallery/service/tracing"
	"github.com/mikeydub/go-gallery/util"
	"github.com/spf13/viper"
)

const maxFollowingReturn = 1000

// 1 week for following to last in cache
var followingTTL = time.Hour * 24 * 7

type AccessTokenResponse struct {
	TokenType    string `json:"token_type"`
	AccessToken  string `json:"access_token"`
	Scope        string `json:"scope"`
	RefreshToken string `json:"refresh_token"`
}

type GetUserMeResponse struct {
	Data TwitterIdentifiers `json:"data"`
}

type GetUserFollowingResponse struct {
	Data []TwitterIdentifiers `json:"data"`
	Meta struct {
		ResultCount int    `json:"result_count"`
		NextToken   string `json:"next_token"`
	}
}

type API struct {
	httpClient *http.Client
	queries    *coredb.Queries
	redis      *redis.Cache

	isAuthed    bool
	accessCode  string
	refreshCode string
	TIDs        TwitterIdentifiers
}

type TwitterIdentifiers struct {
	ID              string `json:"id"`
	Username        string `json:"username"`
	Name            string `json:"name"`
	ProfileImageURL string `json:"profile_image_url"`
}

var errUnauthed = errors.New("unauthorized to use twitter API")
var errAPINotAuthed = errors.New("not authenticated with twitter, use (*API).WithAuth to authenticate")

func NewAPI(queries *coredb.Queries, redis *redis.Cache) *API {
	httpClient := &http.Client{}
	httpClient.Transport = tracing.NewTracingTransport(http.DefaultTransport, false)

	return &API{
		httpClient: httpClient,
		queries:    queries,
		redis:      redis,
	}
}

// GetAuthedUserFromCode creates a new twitter API client with an auth code
func (a *API) GetAuthedUserFromCode(ctx context.Context, authCode string) (TwitterIdentifiers, AccessTokenResponse, error) {

	accessToken, err := a.generateAuthTokenFromCode(ctx, authCode)
	if err != nil {
		return TwitterIdentifiers{}, AccessTokenResponse{}, err
	}

	tids, err := a.getAuthedUser(ctx, accessToken.AccessToken)
	if err != nil {
		return TwitterIdentifiers{}, AccessTokenResponse{}, err
	}
	return tids, accessToken, nil
}

func (a *API) generateAuthTokenFromCode(ctx context.Context, authCode string) (AccessTokenResponse, error) {

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

func (a *API) generateAuthTokenFromRefresh(ctx context.Context, refreshToken string) (AccessTokenResponse, error) {

	q := url.Values{}
	q.Set("refresh_token", refreshToken)
	q.Set("grant_type", "refresh_token")
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

	if accessResp.StatusCode == http.StatusUnauthorized {
		return AccessTokenResponse{}, errUnauthed
	}
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

	if resp.StatusCode == http.StatusUnauthorized {
		return TwitterIdentifiers{}, errUnauthed
	}
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

// WithAuth establishes the API with an access token and refresh token, if the passed in access token is invalid, it will attempt to refresh it
// and optionally return the new access token and refresh token
func (a *API) WithAuth(ctx context.Context, accessToken string, refreshToken string) (*API, *AccessTokenResponse, error) {

	user, err := a.getAuthedUser(ctx, accessToken)
	if err != nil {
		if err == errUnauthed {
			newAtr, err := a.generateAuthTokenFromRefresh(ctx, refreshToken)
			if err != nil {
				return nil, nil, err
			}

			a.accessCode = newAtr.AccessToken
			a.refreshCode = newAtr.RefreshToken
			user, err = a.getAuthedUser(ctx, newAtr.AccessToken)
			if err != nil {
				return nil, nil, err
			}
			a.TIDs = user
			a.isAuthed = true
			return a, &newAtr, nil
		}
		return nil, nil, err
	}

	a.TIDs = user
	a.accessCode = accessToken
	a.refreshCode = refreshToken
	a.isAuthed = true

	return a, nil, nil
}

func (a *API) GetFollowing(ctx context.Context) ([]TwitterIdentifiers, error) {
	if !a.isAuthed {
		return nil, errAPINotAuthed
	}

	redisPath := fmt.Sprintf("twitter.%s.following", a.TIDs.ID)

	var following []TwitterIdentifiers

	bs, err := a.redis.Get(ctx, redisPath)
	if err == nil && len(bs) > 0 {
		if err := json.Unmarshal(bs, &following); err != nil {
			return nil, err
		}
		return following, nil
	}

	nextToken := ""

	// get maxFollowingReturn * 10 users (10k)
	for i := 0; i < 10; i++ {
		url := "https://api.twitter.com/2/users/" + a.TIDs.ID + "/following?user.fields=profile_image_url&max_results=" + strconv.Itoa(maxFollowingReturn)
		if nextToken != "" {
			url += "&pagination_token=" + nextToken
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return nil, err
		}

		req.Header.Add("Authorization", "Bearer "+a.accessCode)

		resp, err := a.httpClient.Do(req)
		if err != nil {
			return nil, fmt.Errorf("failed to get following: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			err = util.GetErrFromResp(resp)
			return nil, fmt.Errorf("failed to get following: %s", err)
		}

		var fresp GetUserFollowingResponse
		if err := json.NewDecoder(resp.Body).Decode(&fresp); err != nil {
			return nil, fmt.Errorf("failed to decode following: %w", err)
		}

		following = append(following, fresp.Data...)

		if fresp.Meta.NextToken == "" || fresp.Meta.ResultCount < maxFollowingReturn {
			break
		}

		nextToken = fresp.Meta.NextToken
	}

	bs, err = json.Marshal(following)
	if err != nil {
		return nil, err
	}

	if err := a.redis.Set(ctx, redisPath, bs, followingTTL); err != nil {
		return nil, err
	}

	return following, nil

}
