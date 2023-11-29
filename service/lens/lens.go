package lens

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/ethereum/go-ethereum/signer/core/apitypes"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/service/redis"
)

const baseURL = "https://api.lens.dev"
const DispatcherAddress = "0xdfd7D26fd33473F475b57556118F8251464a24eb"

type LensAPI struct {
	httpClient *http.Client
	redis      *redis.Cache
}

type AuthenticatedLensAPI struct {
	api          *LensAPI
	accessToken  string
	refreshToken string
	profileID    string
}

func NewAPI(httpClient *http.Client, reddis *redis.Cache) *LensAPI {
	return &LensAPI{
		httpClient: httpClient,
		redis:      reddis,
	}
}

/*
DefaultProfileByAddress
{
  "data": {
    "defaultProfile": {
      "id": "0x0f",
      "name": null,
      "bio": null,
      "isDefault": true,
      "attributes": [],
      "followNftAddress": null,
      "metadata": null,
      "handle": "yoooo1",
      "picture": {
        "original": {
          "url": "https://ipfs.infura.io/ipfs/Qma8mXoeorvPqodDazf7xqARoFD394s1njkze7q1X4CK8U",
          "mimeType": null
        }
      },
      "coverPicture": null,
      "ownedBy": "0x3A5bd1E37b099aE3386D13947b6a90d97675e5e3",
      "dispatcher": null,
      "stats": {
        "totalFollowers": 111,
        "totalFollowing": 15,
        "totalPosts": 89,
        "totalComments": 64,
        "totalMirrors": 15,
        "totalPublications": 168,
        "totalCollects": 215
      },
      "followModule": null
    }
  }
}
*/

type User struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Bio        string `json:"bio"`
	IsDefault  bool   `json:"isDefault"`
	Dispatcher struct {
		CanUseRelay bool   `json:"canUseRelay"`
		Address     string `json:"address"`
		Sponsor     string `json:"sponsor"`
	} `json:"dispatcher"`
	Picture struct {
		Optimized struct {
			URL      string `json:"url"`
			MimeType string `json:"mimeType"`
		} `json:"optimized"`
		URI string `json:"uri"`
	} `json:"picture"`
	Handle  string `json:"handle"`
	OwnedBy string `json:"ownedBy"`
}

type DefaultProfileByAddressResponse struct {
	Data *struct {
		DefaultProfile *User `json:"defaultProfile"`
	} `json:"data"`
	Error *string `json:"error"`
}

func (n *LensAPI) DefaultProfileByAddress(ctx context.Context, address persist.Address) (User, error) {
	gqlQuery := fmt.Sprintf(`query {
		defaultProfile(request: { ethereumAddress: "%s"}) {
			id
			name
			bio
			dispatcher {
      			canUseRelay
      			address
      			sponsor
    		}
			picture {
				... on MediaSet {
					optimized {
						url
						mimeType
					}
				}
				... on NftImage {
					uri
				}
			}
			handle
			ownedBy
		}
	}`, address)

	body, err := json.Marshal(map[string]string{
		"query": gqlQuery,
	})
	if err != nil {
		return User{}, err
	}
	buf := bytes.NewBuffer(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL, buf)
	if err != nil {
		return User{}, err
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := n.httpClient.Do(req)
	if err != nil {
		return User{}, err
	}

	defer resp.Body.Close()

	var lensResp DefaultProfileByAddressResponse
	if err := json.NewDecoder(resp.Body).Decode(&lensResp); err != nil {
		return User{}, err
	}
	if lensResp.Data == nil || lensResp.Data.DefaultProfile == nil {
		var errStr string
		if lensResp.Error != nil {
			errStr = *lensResp.Error
		}
		return User{}, fmt.Errorf("no result for %s (err %s)", address, errStr)
	}

	return *lensResp.Data.DefaultProfile, nil
}

type AuthResponse struct {
	Data *struct {
		Authenticate *struct {
			AccessToken  string `json:"accessToken"`
			RefreshToken string `json:"refreshToken"`
		} `json:"authenticate"`
	} `json:"data"`
	Error *string `json:"error"`
}

func (n *LensAPI) AuthenticateWithSignature(ctx context.Context, address persist.Address, sig string) (string, string, error) {
	gqlQuery := fmt.Sprintf(`mutation {
		authenticate(request: {
    address: "%s",
    signature: "%s"
  }) {
    accessToken
    refreshToken
  }
	}`, address, sig)

	body, err := json.Marshal(map[string]string{
		"query": gqlQuery,
	})
	if err != nil {
		return "", "", err
	}
	buf := bytes.NewBuffer(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL, buf)
	if err != nil {
		return "", "", err
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := n.httpClient.Do(req)
	if err != nil {
		return "", "", err
	}

	defer resp.Body.Close()

	var lensResp AuthResponse
	if err := json.NewDecoder(resp.Body).Decode(&lensResp); err != nil {
		return "", "", err
	}
	if lensResp.Data == nil || lensResp.Data.Authenticate == nil {
		var errStr string
		if lensResp.Error != nil {
			errStr = *lensResp.Error
		}
		return "", "", fmt.Errorf("no result for %s (err %s)", address, errStr)
	}

	return lensResp.Data.Authenticate.AccessToken, lensResp.Data.Authenticate.RefreshToken, nil
}

type RefreshResponse struct {
	Data *struct {
		Refresh *struct {
			AccessToken  string `json:"accessToken"`
			RefreshToken string `json:"refreshToken"`
		} `json:"Refresh"`
	} `json:"data"`
	Error *string `json:"error"`
}

func (n *LensAPI) RefreshAccessToken(ctx context.Context, refreshToken string) (string, string, error) {
	gqlQuery := fmt.Sprintf(`mutation {
		refresh(request: {
    refreshToken: "%s"
  }) {
    accessToken
    refreshToken
  }
	}`, refreshToken)

	body, err := json.Marshal(map[string]string{
		"query": gqlQuery,
	})
	if err != nil {
		return "", "", err
	}
	buf := bytes.NewBuffer(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL, buf)
	if err != nil {
		return "", "", err
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := n.httpClient.Do(req)
	if err != nil {
		return "", "", err
	}

	defer resp.Body.Close()

	var lensResp RefreshResponse
	if err := json.NewDecoder(resp.Body).Decode(&lensResp); err != nil {
		return "", "", err
	}
	if lensResp.Data == nil || lensResp.Data.Refresh == nil {
		var errStr string
		if lensResp.Error != nil {
			errStr = *lensResp.Error
		}
		return "", "", fmt.Errorf("no result (err %s)", errStr)
	}

	return lensResp.Data.Refresh.AccessToken, lensResp.Data.Refresh.RefreshToken, nil
}

type ChallengeResponse struct {
	Data *struct {
		Challenge *struct {
			Text string `json:"text"`
		} `json:"challenge"`
	} `json:"data"`
	Error *string `json:"error"`
}

func (n *LensAPI) GetChallenge(ctx context.Context, address persist.Address) (string, error) {
	gqlQuery := fmt.Sprintf(`mutation {
		challenge(request: {
    address: "%s",
  }) {
    text
  }
	}`, address)

	body, err := json.Marshal(map[string]string{
		"query": gqlQuery,
	})
	if err != nil {
		return "", err
	}
	buf := bytes.NewBuffer(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL, buf)
	if err != nil {
		return "", err
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := n.httpClient.Do(req)
	if err != nil {
		return "", err
	}

	defer resp.Body.Close()

	var lensResp ChallengeResponse
	if err := json.NewDecoder(resp.Body).Decode(&lensResp); err != nil {
		return "", err
	}
	if lensResp.Data == nil || lensResp.Data.Challenge == nil {
		var errStr string
		if lensResp.Error != nil {
			errStr = *lensResp.Error
		}
		return "", fmt.Errorf("no result for %s (err %s)", address, errStr)
	}

	return lensResp.Data.Challenge.Text, nil
}

func (l *LensAPI) WithAuth(ctx context.Context, profileID, accessToken, refreshToken string) *AuthenticatedLensAPI {
	return &AuthenticatedLensAPI{
		api:          l,
		accessToken:  accessToken,
		refreshToken: refreshToken,
		profileID:    profileID,
	}
}

type BroadcastResponse struct {
	Data *struct {
		Broadcast *struct {
			TxHash string `json:"txHash"`
			TxID   string `json:"txId"`
			Reason string `json:"reason"`
		} `json:"broadcast"`
	} `json:"data"`
	Error *string `json:"error"`
}

func (n *AuthenticatedLensAPI) BroadcastDispatcherChange(ctx context.Context, sig string) error {
	redisKey := fmt.Sprintf("lens:typeddata:%s", n.profileID)

	fromCache, err := n.api.redis.Get(ctx, redisKey)
	if err != nil {
		return err
	}

	var cached map[string]string
	if err := json.Unmarshal(fromCache, &cached); err != nil {
		return err
	}

	sigID, ok := cached["typedDataID"]
	if !ok {
		return fmt.Errorf("no typed data id found for profile %s, recreate dispatcher typed data", n.profileID)
	}

	gqlQuery := fmt.Sprintf(`mutation {
		broadcast(request: { id: %s, signature: %s }) {
			... on RelayerResult {
			txHash
			txId
			}

			... on RelayError {
			reason
			}
		}
	}`, sigID, sig)

	body, err := json.Marshal(map[string]string{
		"query": gqlQuery,
	})
	if err != nil {
		return err
	}
	buf := bytes.NewBuffer(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL, buf)
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-access-token", n.accessToken)

	resp, err := n.api.httpClient.Do(req)
	if err != nil {
		return err
	}

	defer resp.Body.Close()

	var lensResp BroadcastResponse
	if err := json.NewDecoder(resp.Body).Decode(&lensResp); err != nil {
		return err
	}
	if lensResp.Data == nil || lensResp.Data.Broadcast == nil || lensResp.Data.Broadcast.Reason != "" {
		var errStr string
		if lensResp.Error != nil {
			errStr = *lensResp.Error
		}
		if lensResp.Data != nil && lensResp.Data.Broadcast != nil && lensResp.Data.Broadcast.Reason != "" {
			errStr += lensResp.Data.Broadcast.Reason
		}
		return fmt.Errorf("no result for profile %s (err %s)", n.profileID, errStr)
	}

	return nil
}

type CreateDispatcherTypedDataResponse struct {
	Data *struct {
		CreateSetDispatcherTypedData *struct {
			ID        string `json:"id"`
			ExpiresAt string `json:"expiresAt"`
			TypedData struct {
				Types struct {
					SetDispatcherWithSig []struct {
						Name string `json:"name"`
						Type string `json:"type"`
					} `json:"SetDispatcherWithSig"`
				}
				Domain apitypes.TypedDataDomain  `json:"domain"`
				Value  apitypes.TypedDataMessage `json:"value"`
			}
		} `json:"createSetDispatcherTypedData"`
	} `json:"data"`
	Error *string `json:"error"`
}

func (n *AuthenticatedLensAPI) GetDispatcherTypedData(ctx context.Context, bustCache bool) (string, string, error) {

	redisKey := fmt.Sprintf("lens:typeddata:%s", n.profileID)

	fromCache, err := n.api.redis.Get(ctx, redisKey)
	if err == nil && len(fromCache) > 0 {
		var cached map[string]string
		if err := json.Unmarshal(fromCache, &cached); err != nil {
			return "", "", err
		}

		return cached["typedDataID"], cached["typedData"], nil
	}

	gqlQuery := fmt.Sprintf(`mutation {
		createSetDispatcherTypedData(request:{
			profileId: "%s",
			dispatcher: "%s"
		}) {
			id
			expiresAt
			typedData {
				types {
					SetDispatcherWithSig {
					name
					type
					}
				}
				domain {
					name
					chainId
					version
					verifyingContract
				}
				value {
					nonce
					deadline
					profileId
					dispatcher
				}
			}
		}
	}`, n.profileID, DispatcherAddress)

	body, err := json.Marshal(map[string]string{
		"query": gqlQuery,
	})
	if err != nil {
		return "", "", err
	}
	buf := bytes.NewBuffer(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL, buf)
	if err != nil {
		return "", "", err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-access-token", n.accessToken)

	resp, err := n.api.httpClient.Do(req)
	if err != nil {
		return "", "", err
	}

	defer resp.Body.Close()

	var lensResp CreateDispatcherTypedDataResponse
	if err := json.NewDecoder(resp.Body).Decode(&lensResp); err != nil {
		return "", "", err
	}
	if lensResp.Data == nil || lensResp.Data.CreateSetDispatcherTypedData == nil {
		var errStr string
		if lensResp.Error != nil {
			errStr = *lensResp.Error
		}

		return "", "", fmt.Errorf("no result for profile %s (err %s)", n.profileID, errStr)
	}

	// store typed data in redis for profile

	types := apitypes.Types{}
	for _, t := range lensResp.Data.CreateSetDispatcherTypedData.TypedData.Types.SetDispatcherWithSig {
		types["SetDispatcherWithSig"] = append(types["SetDispatcherWithSig"], apitypes.Type{
			Name: t.Name,
			Type: t.Type,
		})
	}

	result := apitypes.TypedData{
		Types:       types,
		PrimaryType: "SetDispatcherWithSig",
		Domain:      lensResp.Data.CreateSetDispatcherTypedData.TypedData.Domain,
		Message:     lensResp.Data.CreateSetDispatcherTypedData.TypedData.Value,
	}

	asJSON, err := json.Marshal(result)
	if err != nil {
		return "", "", err
	}

	toCache := map[string]string{
		"typedDataID": lensResp.Data.CreateSetDispatcherTypedData.ID,
		"typedData":   string(asJSON),
	}

	cacheJSON, err := json.Marshal(toCache)
	if err != nil {
		return "", "", err
	}

	expiryTime, err := time.Parse(time.RFC3339, lensResp.Data.CreateSetDispatcherTypedData.ExpiresAt)
	if err != nil {
		return "", "", err
	}

	if err := n.api.redis.Set(ctx, redisKey, cacheJSON, time.Until(expiryTime)); err != nil {
		return "", "", err
	}

	return lensResp.Data.CreateSetDispatcherTypedData.ID, string(asJSON), nil
}
