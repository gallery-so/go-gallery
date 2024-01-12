package tezos

import (
	"context"
	"encoding/hex"
	"fmt"
	"net/http"
	"strings"

	tezospkg "blockwatch.cc/tzgo/tezos"
	mgql "github.com/machinebox/graphql"
	"golang.org/x/crypto/blake2b"

	"github.com/mikeydub/go-gallery/service/auth"
	"github.com/mikeydub/go-gallery/service/multichain"
	"github.com/mikeydub/go-gallery/service/persist"
)

type TokenStandard string

const (
	TokenStandardFa12 TokenStandard = "fa1.2"
	TokenStandardFa2  TokenStandard = "fa2"
	tezDomainsApiURL                = "https://api.tezos.domains/graphql"
	tezosNoncePrepend               = "Tezos Signed Message: "
)

func ToAddress(address persist.Address) (persist.Address, error) {
	if strings.HasPrefix(address.String(), "tz") {
		return address, nil
	}
	key, err := tezospkg.ParseKey(address.String())
	if err != nil {
		return "", err
	}
	return persist.Address(key.Address().String()), nil
}

type Provider struct {
	tzDomainsGQL *mgql.Client
}

func NewProvider(httpClient *http.Client) *Provider {
	return &Provider{
		tzDomainsGQL: mgql.NewClient(tezDomainsApiURL, mgql.WithHTTPClient(httpClient)),
	}
}

func (p *Provider) ProviderInfo() multichain.ProviderInfo {
	return multichain.ProviderInfo{
		Chain:      persist.ChainTezos,
		ChainID:    0,
		ProviderID: "tezos",
	}
}

// VerifySignature will verify a signature using the ed25519 algorithm
// the address provided must be the tezos public key, not the hashed address
func (p *Provider) VerifySignature(pCtx context.Context, pPubKey persist.PubKey, pWalletType persist.WalletType, pNonce string, pSignatureStr string) (bool, error) {
	key, err := tezospkg.ParseKey(pPubKey.String())
	if err != nil {
		return false, err
	}
	sig, err := tezospkg.ParseSignature(pSignatureStr)
	if err != nil {
		return false, err
	}
	nonce := tezosNoncePrepend + auth.NewNoncePrepend + pNonce
	asBytes := []byte(nonce)
	asHex := hex.EncodeToString(asBytes)
	lenHexBytes := []byte(fmt.Sprintf("%d", len(asHex)))

	asBytes = append(lenHexBytes, asBytes...)
	// these three bytes will always be at the front of a hashed signed message according to the tezos standard
	// https://tezostaquito.io/docs/signing/
	asBytes = append([]byte{0x05, 0x01, 0x00}, asBytes...)

	hash, err := blake2b.New256(nil)
	if err != nil {
		return false, err
	}
	_, err = hash.Write(asBytes)
	if err != nil {
		return false, err
	}

	return key.Verify(hash.Sum(nil), sig) == nil, nil
}
