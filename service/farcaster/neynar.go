package farcaster

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/accounts"
	"github.com/ethereum/go-ethereum/common/math"
	"github.com/ethereum/go-ethereum/signer/core/apitypes"
	hdwallet "github.com/miguelmota/go-ethereum-hdwallet"
	"github.com/mikeydub/go-gallery/env"
	"github.com/mikeydub/go-gallery/service/logger"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/util"
)

const neynarV1BaseURL = "https://api.neynar.com/v1/farcaster"
const neynarV2BaseURL = "https://api.neynar.com/v2/farcaster"

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

type NeynarID string

func (n NeynarID) String() string {
	return string(n)
}

// could be a string or a number
func (n *NeynarID) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		var i int
		if err := json.Unmarshal(b, &i); err != nil {
			return err
		}
		*n = NeynarID(fmt.Sprintf("%d", i))
	} else {
		*n = NeynarID(s)
	}
	return nil
}

func (n NeynarID) MarshalJSON() ([]byte, error) {
	return json.Marshal(n.String())
}

type NeynarUser struct {
	Fid         NeynarID `json:"fid"`
	Username    string   `json:"username"`
	DisplayName string   `json:"displayName"`
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
	u := fmt.Sprintf("%s/user-by-verification/?api_key=%s&address=%s", neynarV1BaseURL, n.apiKey, address)
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

	if resp.StatusCode != http.StatusOK {
		bs, err := io.ReadAll(resp.Body)
		if err != nil {
			return NeynarUser{}, err
		}
		return NeynarUser{}, fmt.Errorf("neynar returned status %d (%s)", resp.StatusCode, bs)
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

type NeynarFollowingByUserIDResponse struct {
	Result *struct {
		Users []NeynarUser `json:"users"`
	} `json:"result"`
}

func (n *NeynarAPI) FollowingByUserID(ctx context.Context, fid string) ([]NeynarUser, error) {
	// e.g. https://api.neynar.com/v1/farcaster/following/?api_key={$api-key}&fid=3
	u := fmt.Sprintf("%s/following/?api_key=%s&fid=%s", neynarV1BaseURL, n.apiKey, fid)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", n.apiKey)

	resp, err := n.httpClient.Do(req)
	if err != nil {
		return nil, err
	}

	defer resp.Body.Close()

	var neynarResp NeynarFollowingByUserIDResponse
	if err := json.NewDecoder(resp.Body).Decode(&neynarResp); err != nil {
		return nil, err
	}

	if neynarResp.Result == nil {
		return nil, fmt.Errorf("no following result for %s", fid)
	}

	return neynarResp.Result.Users, nil
}

type NeynarSigner struct {
	SignerUUID        string `json:"signer_uuid"`
	PublicKey         string `json:"public_key"`
	Status            string `json:"status"`
	SignerApprovalURL string `json:"signer_approval_url"`
	SignerApprovalFID any    `json:"fid"`
}
type EIP712Domain struct {
	Name              string   `json:"name"`
	Version           string   `json:"version"`
	ChainID           *big.Int `json:"chainId"`
	VerifyingContract string   `json:"verifyingContract"`
}

type SignedKeyRequest struct {
	RequestFid *big.Int `json:"requestFid"`
	Key        []byte   `json:"key"`
	Deadline   *big.Int `json:"deadline"`
}

func (n *NeynarAPI) CreateSignerForUser(ctx context.Context, fid NeynarID) (NeynarSigner, error) {
	su := fmt.Sprintf("%s/signer/?api_key=%s", neynarV2BaseURL, n.apiKey)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, su, nil)
	if err != nil {
		return NeynarSigner{}, err
	}

	sResp, err := n.httpClient.Do(req)
	if err != nil {
		return NeynarSigner{}, err
	}

	if sResp.StatusCode != http.StatusOK {
		return NeynarSigner{}, fmt.Errorf("neynar returned status %d (%s)", sResp.StatusCode, util.GetErrFromResp(sResp))
	}
	defer sResp.Body.Close()

	var curSigner NeynarSigner
	if err := json.NewDecoder(sResp.Body).Decode(&curSigner); err != nil {
		return NeynarSigner{}, err
	}

	appFidStr := env.GetString("FARCASTER_APP_ID")
	appFid := new(big.Int)
	appFid.SetString(appFidStr, 10)

	deadline := big.NewInt(time.Now().Unix() + 86400)

	// Make sure this matches the network you're using
	signature, err := generateSignatureForSigner(ctx, curSigner, appFid, deadline)
	if err != nil {
		return NeynarSigner{}, err
	}

	rsu := fmt.Sprintf("%s/signer/signed_key/?api_key=%s", neynarV2BaseURL, n.apiKey)
	in := map[string]any{
		"signer_uuid": curSigner.SignerUUID,
		"signature":   fmt.Sprintf("0x%s", hex.EncodeToString(signature)),
		"app_fid":     appFid,
		"deadline":    deadline,
	}
	asJSON, err := json.Marshal(in)
	if err != nil {
		return NeynarSigner{}, err
	}
	buf := bytes.NewBuffer(asJSON)
	req, err = http.NewRequestWithContext(ctx, http.MethodPost, rsu, buf)
	if err != nil {
		return NeynarSigner{}, err
	}

	rsResp, err := n.httpClient.Do(req)
	if err != nil {
		return NeynarSigner{}, err
	}

	if rsResp.StatusCode != http.StatusOK {
		return NeynarSigner{}, fmt.Errorf("neynar returned status %d (%s)", rsResp.StatusCode, util.GetErrFromResp(rsResp))
	}
	defer rsResp.Body.Close()

	err = json.NewDecoder(rsResp.Body).Decode(&curSigner)
	if err != nil {
		return NeynarSigner{}, err
	}

	return curSigner, nil
}

func generateSignatureForSigner(ctx context.Context, curSigner NeynarSigner, appFid, deadline *big.Int) ([]byte, error) {

	mnemonic := env.GetString("FARCASTER_MNEMONIC")
	wallet, err := hdwallet.NewFromMnemonic(mnemonic)
	if err != nil {
		return nil, err
	}

	account, err := wallet.Derive(accounts.DefaultBaseDerivationPath, true)
	if err != nil {
		return nil, err
	}

	logger.For(ctx).Warnf("account %s", account.Address.Hex())

	pubBytes, err := hex.DecodeString(strings.TrimPrefix(curSigner.PublicKey, "0x"))
	if err != nil {
		return nil, err
	}

	domain := apitypes.TypedDataDomain{
		Name:              "Farcaster SignedKeyRequestValidator",
		Version:           "1",
		ChainId:           math.NewHexOrDecimal256(10),
		VerifyingContract: "0x00000000fc700472606ed4fa22623acf62c60553",
	}

	typedData := apitypes.TypedData{
		Types: apitypes.Types{
			"EIP712Domain": []apitypes.Type{
				{Name: "name", Type: "string"},
				{Name: "version", Type: "string"},
				{Name: "chainId", Type: "uint256"},
				{Name: "verifyingContract", Type: "address"},
			},
			"SignedKeyRequest": []apitypes.Type{
				{Name: "requestFid", Type: "uint256"},
				{Name: "key", Type: "bytes"},
				{Name: "deadline", Type: "uint256"},
			},
		},
		PrimaryType: "SignedKeyRequest",
		Domain:      domain,
		Message: map[string]interface{}{
			"requestFid": appFid.String(),
			"key":        pubBytes,
			"deadline":   deadline.String(),
		},
	}

	signature, err := signEIP712TypedData(wallet, account, typedData)
	if err != nil {
		return nil, err
	}
	return signature, nil
}

func signEIP712TypedData(wallet *hdwallet.Wallet, account accounts.Account, typedData apitypes.TypedData) ([]byte, error) {

	hash, _, err := apitypes.TypedDataAndHash(typedData)
	if err != nil {
		return nil, err
	}

	// Sign the final hash
	signature, err := wallet.SignHash(account, hash)
	if err != nil {
		return nil, err
	}

	signature[64] += 27

	return signature, nil
}

func (n *NeynarAPI) GetSignerByUUID(ctx context.Context, uuid string) (NeynarSigner, error) {
	su := fmt.Sprintf("%s/signer/?api_key=%s&signer_uuid=%s", neynarV2BaseURL, n.apiKey, uuid)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, su, nil)
	if err != nil {
		return NeynarSigner{}, err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", n.apiKey)

	sResp, err := n.httpClient.Do(req)
	if err != nil {
		return NeynarSigner{}, err
	}

	if sResp.StatusCode != http.StatusOK {
		bs, err := io.ReadAll(sResp.Body)
		if err != nil {
			return NeynarSigner{}, err
		}
		return NeynarSigner{}, fmt.Errorf("neynar returned status %d (%s)", sResp.StatusCode, bs)
	}
	defer sResp.Body.Close()

	var curSigner NeynarSigner
	if err := json.NewDecoder(sResp.Body).Decode(&curSigner); err != nil {
		return NeynarSigner{}, err
	}

	return curSigner, nil
}
