package dummy

import (
	"context"
	"github.com/mikeydub/go-gallery/service/multichain"
	"github.com/mikeydub/go-gallery/service/persist"
)

type Provider struct {
	tokensByWalletAddress      map[persist.Address][]multichain.ChainAgnosticToken
	contractsByContractAddress map[persist.Address]multichain.ChainAgnosticContract
}

func (p *Provider) AddTokens(walletAddress persist.Address, tokens ...multichain.ChainAgnosticToken) {
	p.tokensByWalletAddress[walletAddress] = append(p.tokensByWalletAddress[walletAddress], tokens...)
}

func (p *Provider) ClearTokens(walletAddress persist.Address) {
	p.tokensByWalletAddress[walletAddress] = []multichain.ChainAgnosticToken{}
}

func (p *Provider) GetBlockchainInfo(context.Context) (multichain.BlockchainInfo, error) {
	return multichain.BlockchainInfo{
		Chain:   persist.ChainETH,
		ChainID: 0,
	}, nil
}

func (p *Provider) GetTokensByWalletAddress(ctx context.Context, address persist.Address) ([]multichain.ChainAgnosticToken, []multichain.ChainAgnosticContract, error) {
	//assets, err := FetchAssetsForWallet(ctx, persist.EthereumAddress(address.String()), "", 0, nil)
	//if err != nil {
	//	return nil, nil, err
	//}
	//return assetsToTokens(ctx, address, assets, p.ethClient)

	return nil, nil, nil
}

func (p *Provider) GetTokensByContractAddress(ctx context.Context, address persist.Address) ([]multichain.ChainAgnosticToken, multichain.ChainAgnosticContract, error) {
	// Currently not called by anything
	return nil, multichain.ChainAgnosticContract{}, nil
}

func (p *Provider) GetTokensByTokenIdentifiers(ctx context.Context, ti persist.TokenIdentifiers) ([]multichain.ChainAgnosticToken, []multichain.ChainAgnosticContract, error) {
	// Currently not called by anything
	return nil, nil, nil
}

func (p *Provider) GetContractByAddress(ctx context.Context, contract persist.Address) (multichain.ChainAgnosticContract, error) {
	// Currently not called by anything
	return multichain.ChainAgnosticContract{}, nil
}

func (p *Provider) UpdateMediaForWallet(context.Context, persist.Address, bool) error {
	// Currently not called by anything
	return nil
}

func (p *Provider) ValidateTokensForWallet(context.Context, persist.Address, bool) error {
	// Currently not called by anything
	return nil
}

// VerifySignature will verify a signature using all available methods (eth_sign and personal_sign)
func (d *Provider) VerifySignature(pCtx context.Context, pAddressStr persist.Address, pWalletType persist.WalletType, pNonce string, pSignatureStr string) (bool, error) {
	return true, nil
}
