package lens

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/mikeydub/go-gallery/service/persist"
)

const baseURL = "https://api.lens.dev"

type LensAPI struct {
	httpClient *http.Client
}

func NewAPI(httpClient *http.Client) *LensAPI {
	return &LensAPI{
		httpClient: httpClient,
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
	ID        string `json:"id"`
	Name      string `json:"name"`
	Bio       string `json:"bio"`
	IsDefault bool   `json:"isDefault"`
	Picture   struct {
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
