package eth

import (
	"context"
	"errors"
	"regexp"
	"strings"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
	ens "github.com/wealdtech/go-ens/v3"

	"github.com/mikeydub/go-gallery/service/persist"
)

var ErrNoResolution = errors.New("no resolution")
var ErrUnknownENSAvatarURI = errors.New("unknown ENS avatar uri")

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

// ReverseResolves returns true if the given address resolves to the given domain
func ReverseResolves(ctx context.Context, ethClient *ethclient.Client, domain string, address persist.EthereumAddress) (bool, error) {
	revDomain, err := ReverseResolve(ctx, ethClient, address)
	if errors.Is(err, ErrNoResolution) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return strings.EqualFold(domain, revDomain), nil
}

func EnsAvatarRecordFor(ctx context.Context, ethClient *ethclient.Client, a persist.EthereumAddress) (avatar EnsAvatar, err error) {
	domain, err := ReverseResolve(ctx, ethClient, a)
	if errors.Is(err, ErrNoResolution) {
		return avatar, nil
	}

	resolver, err := ens.NewResolver(ethClient, domain)
	if err != nil {
		return avatar, err
	}

	record, err := resolver.Text("avatar")
	if err != nil {
		return avatar, err
	}

	if record == "" {
		return avatar, nil
	}

	uri, err := ENSRecordToURI(record)
	if err != nil {
		return EnsAvatar{}, err
	}

	return EnsAvatar{URI: uri}, nil
}

// Regex for CAIP-19 asset type with required asset ID
// https://github.com/ChainAgnostic/CAIPs/blob/master/CAIPs/caip-19.md
var caip19AssetTypeWithAssetID = regexp.MustCompile(`^(?P<chain_id>[-a-z0-9]{3,8}:[-_a-zA-Z0-9]{1,32})/(?P<asset_namespace>[-a-z0-9]{3,8}):(?P<asset_reference>[-.%a-zA-Z0-9]{1,78})/(?P<token_id>[-.%a-zA-Z0-9]{1,78})$`)

func ENSRecordToURI(r string) (avatarURI, error) {
	switch {
	case strings.HasPrefix(r, "https://"):
		return EnsHttpUri{URL: r}, nil
	case strings.HasPrefix(r, "ipfs://"):
		return EnsIpfsUri{URL: r}, nil
	case strings.HasPrefix(r, "data:"):
		return EnsDataUri{URL: r}, nil
	case caip19AssetTypeWithAssetID.MatchString(r):
		grps := caip19AssetTypeWithAssetID.FindStringSubmatch(r)
		return EnsNftUri{
			ChainID:        grps[1],
			AssetNamespace: grps[2],
			AssetReference: grps[3],
			AssetID:        grps[4],
		}, nil
	default:
		return nil, ErrUnknownENSAvatarURI
	}
}

// EnsAvatar is a representation of the ENS avatar text record
type EnsAvatar struct {
	URI avatarURI
}

type avatarURI interface {
	IsAvatarURI()
}

type EnsHttpUri struct {
	URL string
}

func (EnsHttpUri) IsAvatarURI() {}

type EnsDataUri struct {
	URL string
}

func (EnsDataUri) IsAvatarURI() {}

type EnsIpfsUri struct {
	URL string
}

func (EnsIpfsUri) IsAvatarURI() {}

type EnsNftUri struct {
	ChainID        string
	AssetNamespace string
	AssetReference string
	AssetID        string
}

func (EnsNftUri) IsAvatarURI() {}

func (e EnsNftUri) Chain() persist.Chain {
	panic("not implemented")
}

func (e EnsNftUri) TokenID() persist.TokenID {
	panic("not implemented")
}

func (e EnsNftUri) Address() persist.Address {
	panic("not implemented")
}
