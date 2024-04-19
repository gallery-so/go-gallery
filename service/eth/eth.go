package eth

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"regexp"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	ens "github.com/wealdtech/go-ens/v3"
	"github.com/wealdtech/go-ens/v3/contracts/resolver"
	"github.com/wealdtech/go-ens/v3/contracts/reverseresolver"

	"github.com/mikeydub/go-gallery/contracts"
	"github.com/mikeydub/go-gallery/service/logger"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/service/rpc"
)

var ErrNoResolution = errors.New("unable to resolve ENS domain from address")
var ErrUnknownEnsAvatarURI = errors.New("unknown ENS avatar uri")
var ErrChainNotSupported = errors.New("chain not supported")
var ErrUnknownTokenType = errors.New("unknown token type")
var ErrNoAvatarRecord = errors.New("no avatar record set")

// ErrAddressSignatureMismatch is returned when the address signature does not match the address cryptographically
var ErrAddressSignatureMismatch = errors.New("address does not match signature")

var eip1271MagicValue = [4]byte{0x16, 0x26, 0xBA, 0x7E}

// Regex for CAIP-19 asset type with required asset ID
// https://github.com/ChainAgnostic/CAIPs/blob/master/CAIPs/caip-19.md
var caip19AssetTypeWithAssetID = regexp.MustCompile(
	"^(?P<chain_id>[-a-z0-9]{3,8}:[-_a-zA-Z0-9]{1,32})/" +
		"(?P<asset_namespace>[-a-z0-9]{3,8}):" +
		"(?P<asset_reference>[-.%a-zA-Z0-9]{1,78})/" +
		"(?P<token_id>[-.%a-zA-Z0-9]{1,78})$",
)

const (
	EnsAddress        = "0x57f1887a8bf19b14fc0df6fd9b2acc9af147ea85"
	ethMainnetChainID = "eip155:1"
)

// ReverseResolve returns the domain name for the given address
func ReverseResolve(ctx context.Context, ethClient *ethclient.Client, address persist.EthereumAddress) (string, error) {
	registry, err := ens.NewRegistry(ethClient)
	if err != nil {
		return "", err
	}

	// Fetch the resolver address
	addr := common.HexToAddress(address.String())
	domain := fmt.Sprintf("%x.addr.reverse", addr)
	resolverAddress, err := registry.ResolverAddress(domain)
	if err != nil {
		return "", err
	}

	// Init the resolver contract
	contract, err := reverseresolver.NewContract(resolverAddress, ethClient)
	if err != nil {
		return "", err
	}

	nameHash, err := ens.NameHash(fmt.Sprintf("%s.addr.reverse", addr.Hex()[2:]))
	if err != nil {
		return "", err
	}

	name, err := contract.Name(nil, nameHash)
	if err != nil && err.Error() == "no contract code at given address" {
		return "", ErrNoResolution
	}

	if name == "" {
		return "", ErrNoResolution
	}

	return name, err
}

// ReverseResolves returns true if the reverse resolves to any domain
func ReverseResolves(ctx context.Context, ethClient *ethclient.Client, address persist.EthereumAddress) (bool, error) {
	_, err := ReverseResolve(ctx, ethClient, address)
	if errors.Is(err, ErrNoResolution) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

// ReverseResolvesTo returns true if the address reverse resolves to the given domain
func ReverseResolvesTo(ctx context.Context, ethClient *ethclient.Client, domain string, address persist.EthereumAddress) (bool, error) {
	revDomain, err := ReverseResolve(ctx, ethClient, address)
	if errors.Is(err, ErrNoResolution) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	// Resolve the returned domain to ensure that it actually resolves to the given address
	addr, err := ens.Resolve(ethClient, revDomain)
	if err != nil {
		return false, err
	}

	return addr == common.HexToAddress(address.String()), nil
}

// DeriveTokenID derives the token ID (in hexadecimal) from a domain
func DeriveTokenID(domain string) (persist.HexTokenID, error) {
	domain, err := NormalizeDomain(domain)
	if err != nil {
		return "", err
	}
	domain, err = ens.DomainPart(domain, 1)
	if err != nil {
		return "", err
	}
	labelHash, err := ens.LabelHash(domain)
	if err != nil {
		return "", err
	}
	return persist.HexTokenID(fmt.Sprintf("%x", labelHash)), nil
}

// NormalizeDomain converts a domain to its canonical form
func NormalizeDomain(domain string) (string, error) {
	if domain == "" {
		return "", errors.New("empty domain")
	}
	// Some ENS tokens are formatted as "ENS: vitalk.eth", so we remove the "ENS: " prefix
	domain = strings.Replace(domain, "ENS: ", "", 1)
	domain, err := ens.NormaliseDomain(domain)
	if err != nil {
		return "", err
	}
	return domain, nil
}

// EnsAvatarRecordFor returns the avatar record for the given address
func EnsAvatarRecordFor(ctx context.Context, ethClient *ethclient.Client, a persist.EthereumAddress) (avatar AvatarRecord, domain string, err error) {
	domain, err = ReverseResolve(ctx, ethClient, a)
	if err != nil {
		return avatar, domain, err
	}

	registry, err := ens.NewRegistry(ethClient)
	if err != nil {
		return nil, "", err
	}

	resolverAddress, err := registry.ResolverAddress(domain)
	if err != nil {
		return nil, "", err
	}

	contract, err := resolver.NewContract(resolverAddress, ethClient)
	if err != nil {
		return nil, "", err
	}

	nameHash, err := ens.NameHash(domain)
	if err != nil {
		return nil, "", err
	}

	record, err := contract.Text(nil, nameHash, "avatar")
	if err != nil {
		return nil, "", err
	}

	if record == "" {
		return avatar, domain, ErrNoAvatarRecord
	}

	uri, err := toRecord(record)
	return uri, domain, err
}

// IsOwner returns true if the address is the current holder of the token
func IsOwner(ctx context.Context, ethClient *ethclient.Client, addr persist.EthereumAddress, uri EnsTokenRecord) (bool, error) {
	chain, contractAddr, tokenType, tokenID, err := TokenInfoFor(uri)
	if err != nil {
		return false, err
	}

	if chain != persist.ChainETH {
		return false, ErrChainNotSupported
	}

	if tokenType == persist.TokenTypeERC721 {
		curOwner, err := rpc.GetOwnerOfERC721Token(ctx, persist.EthereumAddress(contractAddr), tokenID, ethClient)
		if err != nil {
			return false, err
		}
		return strings.EqualFold(curOwner.String(), addr.String()), nil
	}

	if tokenType == persist.TokenTypeERC1155 {
		balance, err := rpc.GetBalanceOfERC1155Token(ctx, addr, persist.EthereumAddress(contractAddr), tokenID, ethClient)
		if err != nil {
			return false, err
		}
		var zero big.Int
		return balance.Cmp(&zero) > 0, nil
	}

	return false, ErrUnknownTokenType
}

// TokenInfoFor is a helper function for parsing info from a token record
func TokenInfoFor(r EnsTokenRecord) (persist.Chain, persist.Address, persist.TokenType, persist.HexTokenID, error) {
	errs := make([]error, 4)
	chain, err := r.Chain()
	errs[0] = err
	address, err := r.Address()
	errs[1] = err
	tokenType, err := r.TokenType()
	errs[2] = err
	tokenID, err := r.TokenID()
	errs[3] = err
	for _, err := range errs {
		if err != nil {
			return 0, "", "", "", err
		}
	}
	return chain, address, tokenType, tokenID, nil
}

func toRecord(r string) (AvatarRecord, error) {
	switch {
	case strings.HasPrefix(r, "https://"), strings.HasPrefix(r, "http://"):
		return EnsHttpRecord{URL: r}, nil
	case strings.HasPrefix(r, "ipfs://"):
		return EnsIpfsRecord{URL: r}, nil
	case caip19AssetTypeWithAssetID.MatchString(r):
		g := caip19AssetTypeWithAssetID.FindStringSubmatch(r)
		return EnsTokenRecord{
			ChainID:        g[1],
			AssetNamespace: g[2],
			AssetReference: g[3],
			AssetID:        g[4],
		}, nil
	default:
		return nil, ErrUnknownEnsAvatarURI
	}
}

type AvatarRecord interface {
	IsAvatarURI()
}

type EnsHttpRecord struct {
	URL string
}

func (EnsHttpRecord) IsAvatarURI() {}

type EnsIpfsRecord struct {
	URL string
}

func (EnsIpfsRecord) IsAvatarURI() {}

type EnsTokenRecord struct {
	ChainID        string
	AssetNamespace string
	AssetReference string
	AssetID        string
}

func (EnsTokenRecord) IsAvatarURI() {}

func (e EnsTokenRecord) Chain() (persist.Chain, error) {
	if e.ChainID != ethMainnetChainID {
		return 0, ErrChainNotSupported
	}
	return persist.ChainETH, nil
}

func (e EnsTokenRecord) TokenType() (persist.TokenType, error) {
	if e.ChainID != ethMainnetChainID {
		return "", ErrChainNotSupported
	}
	if e.AssetNamespace == "erc721" {
		return persist.TokenTypeERC721, nil
	}
	if e.AssetNamespace == "erc1155" {
		return persist.TokenTypeERC1155, nil
	}
	return "", ErrUnknownTokenType
}

func (e EnsTokenRecord) TokenID() (persist.HexTokenID, error) {
	if e.ChainID != ethMainnetChainID {
		return "", ErrChainNotSupported
	}
	return persist.MustTokenID(e.AssetID), nil
}

func (e EnsTokenRecord) Address() (persist.Address, error) {
	if e.ChainID != ethMainnetChainID {
		return "", ErrChainNotSupported
	}
	return persist.Address(e.AssetReference), nil
}

type Verifier struct {
	Client *ethclient.Client
}

// VerifySignature will verify a signature using all available methods (eth_sign and personal_sign)
func (p *Verifier) VerifySignature(pCtx context.Context, pAddressStr persist.PubKey, pWalletType persist.WalletType, pMessage string, pSignatureStr string) (bool, error) {

	// personal_sign
	validBool, err := verifySignature(pSignatureStr, pMessage, pAddressStr, pWalletType, true, p.Client)

	if !validBool || err != nil {
		// eth_sign
		validBool, err = verifySignature(pSignatureStr, pMessage, pAddressStr, pWalletType, false, p.Client)
	}

	if err != nil {
		return false, err
	}

	return validBool, nil
}

func verifySignature(pSignatureStr string,
	pData string,
	pAddress persist.PubKey, pWalletType persist.WalletType,
	pUseDataHeaderBool bool, ec *ethclient.Client) (bool, error) {

	// eth_sign:
	// - https://goethereumbook.org/signature-verify/
	// - http://man.hubwiz.com/docset/Ethereum.docset/Contents/Resources/Documents/eth_sign.html
	// - sign(keccak256("\x19Ethereum Signed Message:\n" + len(message) + message)))

	var data string
	if pUseDataHeaderBool {
		data = fmt.Sprintf("\x19Ethereum Signed Message:\n%d%s", len(pData), pData)
	} else {
		data = pData
	}

	switch pWalletType {
	case persist.WalletTypeEOA:
		dataHash := crypto.Keccak256Hash([]byte(data))

		sig, err := hexutil.Decode(pSignatureStr)
		if err != nil {
			return false, err
		}
		// Ledger-produced signatures have v = 0 or 1
		if sig[64] == 0 || sig[64] == 1 {
			sig[64] += 27
		}
		v := sig[64]
		if v != 27 && v != 28 {
			return false, errors.New("invalid signature (V is not 27 or 28)")
		}
		sig[64] -= 27

		sigPublicKeyECDSA, err := crypto.SigToPub(dataHash.Bytes(), sig)
		if err != nil {
			return false, err
		}

		pubkeyAddressHexStr := crypto.PubkeyToAddress(*sigPublicKeyECDSA).Hex()
		logger.For(nil).Infof("pubkeyAddressHexStr: %s", pubkeyAddressHexStr)
		logger.For(nil).Infof("pAddress: %s", pAddress)
		if !strings.EqualFold(pubkeyAddressHexStr, pAddress.String()) {
			return false, ErrAddressSignatureMismatch
		}

		publicKeyBytes := crypto.CompressPubkey(sigPublicKeyECDSA)

		signatureNoRecoverID := sig[:len(sig)-1]

		return crypto.VerifySignature(publicKeyBytes, dataHash.Bytes(), signatureNoRecoverID), nil
	case persist.WalletTypeGnosis:
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		sigValidator, err := contracts.NewISignatureValidator(common.HexToAddress(pAddress.String()), ec)
		if err != nil {
			return false, err
		}

		hashedData := crypto.Keccak256([]byte(data))
		var input [32]byte
		copy(input[:], hashedData)

		result, err := sigValidator.IsValidSignature(&bind.CallOpts{Context: ctx}, input, []byte{})
		if err != nil {
			logger.For(nil).WithError(err).Error("IsValidSignature")
			return false, nil
		}

		return result == eip1271MagicValue, nil
	default:
		return false, errors.New("wallet type not supported")
	}
}
