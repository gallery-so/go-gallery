package eth

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"regexp"
	"strconv"
	"strings"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
	ens "github.com/wealdtech/go-ens/v3"

	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/service/rpc"
)

var ErrNoResolution = errors.New("no resolution")
var ErrUnknownENSAvatarURI = errors.New("unknown ENS avatar uri")
var ErrChainNotSupported = errors.New("chain not supported")
var ErrUnknownTokenType = errors.New("unknown token type")

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
	domain, err := ens.ReverseResolve(ethClient, common.HexToAddress(address.String()))
	if err != nil && strings.Contains(err.Error(), "not a resolver") {
		return "", ErrNoResolution
	}
	if err != nil && strings.Contains(err.Error(), "no resolution") {
		return "", ErrNoResolution
	}
	if err != nil {
		return "", err
	}
	if domain == "" {
		return "", ErrNoResolution
	}
	return domain, nil
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
// Copied from main branch of go-ens, which isn't available yet on the latest release at the time of writing (v3.5.5)
func DeriveTokenID(domain string) (persist.TokenID, error) {
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
	return persist.TokenID(fmt.Sprintf("%x", labelHash)), nil
}

// NormalizeDomain converts a domain to its canonical form
func NormalizeDomain(domain string) (string, error) {
	if domain == "" {
		return "", errors.New("empty domain")
	}
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

	resolver, err := ens.NewResolver(ethClient, domain)
	if err != nil {
		return avatar, domain, err
	}

	record, err := resolver.Text("avatar")
	if err != nil {
		return avatar, domain, err
	}

	if record == "" {
		return avatar, domain, nil
	}

	uri, err := toRecord(record)
	if err != nil {
		return nil, domain, err
	}

	return uri, domain, nil
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
func TokenInfoFor(r EnsTokenRecord) (persist.Chain, persist.Address, persist.TokenType, persist.TokenID, error) {
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
		return nil, ErrUnknownENSAvatarURI
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

func (e EnsTokenRecord) TokenID() (persist.TokenID, error) {
	if e.ChainID != ethMainnetChainID {
		return "", ErrChainNotSupported
	}
	return asTokenID(e.AssetID)
}

func (e EnsTokenRecord) Address() (persist.Address, error) {
	if e.ChainID != ethMainnetChainID {
		return "", ErrChainNotSupported
	}
	return persist.Address(e.AssetReference), nil
}

// asTokenID converts a decimal string to a token ID
func asTokenID(s string) (persist.TokenID, error) {
	i, err := strconv.ParseInt(s, 10, 0)
	if err != nil {
		return "", err
	}
	return persist.TokenID(fmt.Sprintf("%x", i)), nil
}
