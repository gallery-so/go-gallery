package farcaster

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/mikeydub/go-gallery/env"
	"github.com/mikeydub/go-gallery/service/persist"
)

const neynarBaseURL = "https://api.neynar.com/v1/farcaster"

func init() {
	env.RegisterValidation("NEYNAR_API_KEY", "required")
}

type NeynarAPI struct {
	httpClient *http.Client
	apiKey     string
}

func NewNeynarAPI(httpClient *http.Client) *NeynarAPI {
	return &NeynarAPI{
		httpClient: httpClient,
		apiKey:     env.GetString("NEYNAR_API_KEY"),
	}
}

/*
UserByVerification
{
	result: {
		user: {
			user: {
				fid: "194",
				username: "rish",
				displayName: "rish",
					pfp: {
						url: "https://res.cloudinary.com/merkle-manufactory/image/fetch/c_fill,f_png,w_256/https://lh3.googleusercontent.com/MEaRCAMdER6MKcvmlfN1-0fVxOGz6w98R8CrP_Rpzse9KZudgn95frTd0L0ZViWVklBj9fuAcJuM6tt7P-BRN0ouAR87NpzZeh2DGw"
					},
				profile: {
					bio: {
						text: "@neynar, ethOS | ex Coinbase, FB | nf.td/rish"
					}
				}
			}
		}
	}
}
*/

type NeynarUser struct {
	Fid         string `json:"fid"`
	Username    string `json:"username"`
	DisplayName string `json:"displayName"`
	Pfp         struct {
		URL string `json:"url"`
	} `json:"pfp"`
	Profile struct {
		Bio struct {
			Text string `json:"text"`
		} `json:"bio"`
	} `json:"profile"`
}

type NeynarUserByVerificationResponse struct {
	Result *struct {
		User NeynarUser `json:"user"`
	} `json:"result"`
}

func (n *NeynarAPI) UserByAddress(ctx context.Context, address persist.Address) (NeynarUser, error) {
	u := fmt.Sprintf("%s/user-by-verification/?api_key=%s&address=%s", neynarBaseURL, n.apiKey, address)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return NeynarUser{}, err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", n.apiKey)

	resp, err := n.httpClient.Do(req)
	if err != nil {
		return NeynarUser{}, err
	}

	defer resp.Body.Close()

	var neynarResp NeynarUserByVerificationResponse
	if err := json.NewDecoder(resp.Body).Decode(&neynarResp); err != nil {
		return NeynarUser{}, err
	}
	if neynarResp.Result == nil {
		return NeynarUser{}, fmt.Errorf("no result for %s", address)
	}

	return neynarResp.Result.User, nil
}
