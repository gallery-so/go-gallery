package eth

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/mikeydub/go-gallery/indexer"
	"github.com/mikeydub/go-gallery/service/multichain"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/util"
)

// DataRetriever is an the struct for retrieving data from the Ethereum blockchain
type DataRetriever struct {
	indexerBaseURL string
	httpClient     *http.Client
}

// NewDataRetriever creates a new DataRetriever
func NewDataRetriever(indexerBaseURL string, httpClient *http.Client) *DataRetriever {
	return &DataRetriever{
		indexerBaseURL: indexerBaseURL,
		httpClient:     httpClient,
	}
}

// GetBlockchainInfo retrieves blockchain info for ETH
func (d *DataRetriever) GetBlockchainInfo(ctx context.Context) (multichain.BlockchainInfo, error) {
	return multichain.BlockchainInfo{
		ChainName: persist.ChainETH,
		ChainID:   0,
	}, nil
}

// GetTokensByWalletAddress retrieves tokens for a wallet address on the Ethereum Blockchain
func (d *DataRetriever) GetTokensByWalletAddress(ctx context.Context, addr persist.EthereumAddress) ([]persist.TokenGallery, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("%s/tokens?address=%s&limit=-1", d.indexerBaseURL, addr), nil)
	if err != nil {
		return nil, err
	}
	res, err := d.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	if res.StatusCode >= 299 || res.StatusCode < 200 {
		errResp := util.ErrorResponse{}
		err = json.NewDecoder(res.Body).Decode(&errResp)
		if err != nil {
			return nil, err
		}
		return nil, fmt.Errorf("unexpected status: %s | err: %s ", res.Status, errResp.Error)
	}

	var tokens indexer.GetTokensOutput
	err = json.NewDecoder(res.Body).Decode(&tokens)
	if err != nil {
		return nil, err
	}

	return tokensToTokensGallery(tokens.NFTs), nil

}

// GetTokensByContractAddress retrieves tokens for a contract address on the Ethereum Blockchain
func (d *DataRetriever) GetTokensByContractAddress(ctx context.Context, contractAddress persist.EthereumAddress) ([]persist.TokenGallery, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("%s/nfts/get?contract_address=%s&limit=-1", d.indexerBaseURL, contractAddress), nil)
	if err != nil {
		return nil, err
	}
	res, err := d.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	if res.StatusCode >= 299 || res.StatusCode < 200 {
		errResp := util.ErrorResponse{}
		err = json.NewDecoder(res.Body).Decode(&errResp)
		if err != nil {
			return nil, err
		}
		return nil, fmt.Errorf("unexpected status: %s | err: %s ", res.Status, errResp.Error)
	}

	var tokens indexer.GetTokensOutput
	err = json.NewDecoder(res.Body).Decode(&tokens)
	if err != nil {
		return nil, err
	}

	return tokensToTokensGallery(tokens.NFTs), nil
}

// GetTokensByTokenIdentifiers retrieves tokens for a token identifiers on the Ethereum Blockchain
func (d *DataRetriever) GetTokensByTokenIdentifiers(ctx context.Context, tokenIdentifiers persist.TokenIdentifiers) ([]persist.TokenGallery, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("%s/nfts/get?contract_address=%s&token_id=%s&limit=-1", d.indexerBaseURL, tokenIdentifiers.ContractAddress, tokenIdentifiers.TokenID), nil)
	if err != nil {
		return nil, err
	}
	res, err := d.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	if res.StatusCode >= 299 || res.StatusCode < 200 {
		errResp := util.ErrorResponse{}
		err = json.NewDecoder(res.Body).Decode(&errResp)
		if err != nil {
			return nil, err
		}
		return nil, fmt.Errorf("unexpected status: %s | err: %s ", res.Status, errResp.Error)
	}

	var tokens indexer.GetTokensOutput
	err = json.NewDecoder(res.Body).Decode(&tokens)
	if err != nil {
		return nil, err
	}

	return tokensToTokensGallery(tokens.NFTs), nil
}

// GetContractByAddress retrieves an ethereum contract by address
func (d *DataRetriever) GetContractByAddress(ctx context.Context, addr persist.EthereumAddress) (persist.ContractGallery, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("%s/contracts/get?address=%s", d.indexerBaseURL, addr), nil)
	if err != nil {
		return persist.ContractGallery{}, err
	}
	res, err := d.httpClient.Do(req)
	if err != nil {
		return persist.ContractGallery{}, err
	}
	defer res.Body.Close()

	if res.StatusCode >= 299 || res.StatusCode < 200 {
		errResp := util.ErrorResponse{}
		err = json.NewDecoder(res.Body).Decode(&errResp)
		if err != nil {
			return persist.ContractGallery{}, err
		}
		return persist.ContractGallery{}, fmt.Errorf("unexpected status: %s | err: %s ", res.Status, errResp.Error)
	}
	var contract indexer.GetContractOutput
	err = json.NewDecoder(res.Body).Decode(&contract)
	if err != nil {
		return persist.ContractGallery{}, err
	}

	return contractToContractGallery(contract.Contract), nil

}

// UpdateMediaForWallet updates media for the tokens owned by a wallet on the Ethereum Blockchain
func (d *DataRetriever) UpdateMediaForWallet(ctx context.Context, wallet persist.EthereumAddress, all bool) error {

	input := indexer.UpdateMediaInput{
		OwnerAddress: wallet,
		UpdateAll:    all,
	}

	asJSON, err := json.Marshal(input)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, fmt.Sprintf("%s/media/update", d.indexerBaseURL), bytes.NewReader(asJSON))
	if err != nil {
		return err
	}

	res, err := d.httpClient.Do(req)
	if err != nil {
		return err
	}

	defer res.Body.Close()

	if res.StatusCode >= 299 || res.StatusCode < 200 {
		errResp := util.ErrorResponse{}
		err = json.NewDecoder(res.Body).Decode(&errResp)
		if err != nil {
			return err
		}
		return fmt.Errorf("unexpected status: %s | err: %s ", res.Status, errResp.Error)
	}

	return nil
}

// ValidateTokensForWallet validates tokens for a wallet address on the Ethereum Blockchain
func (d *DataRetriever) ValidateTokensForWallet(ctx context.Context, wallet persist.EthereumAddress, all bool) error {
	input := indexer.ValidateUsersNFTsInput{Wallet: wallet, All: all}

	asJSON, err := json.Marshal(input)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, fmt.Sprintf("%s/nfts/validate", d.indexerBaseURL), bytes.NewReader(asJSON))
	if err != nil {
		return err
	}

	res, err := d.httpClient.Do(req)
	if err != nil {
		return err
	}

	defer res.Body.Close()

	if res.StatusCode >= 299 || res.StatusCode < 200 {
		errResp := util.ErrorResponse{}
		err = json.NewDecoder(res.Body).Decode(&errResp)
		if err != nil {
			return err
		}
		return fmt.Errorf("unexpected status: %s | err: %s ", res.Status, errResp.Error)
	}

	return nil

}

func tokensToTokensGallery(tokens []persist.Token) []persist.TokenGallery {
	res := make([]persist.TokenGallery, len(tokens))
	for i, token := range tokens {
		res[i] = persist.TokenGallery{
			TokenID:          token.TokenID,
			ContractAddress:  persist.Address(token.ContractAddress.String()),
			OwnerAddress:     persist.Address(token.OwnerAddress.String()),
			TokenURI:         token.TokenURI,
			Media:            token.Media,
			TokenType:        token.TokenType,
			Chain:            token.Chain,
			Name:             token.Name,
			Description:      token.Description,
			Quantity:         token.Quantity,
			OwnershipHistory: ethereumAddressAtBlockToGallery(token.OwnershipHistory),
			TokenMetadata:    token.TokenMetadata,
			ExternalURL:      token.ExternalURL,
			BlockNumber:      token.BlockNumber,
		}
	}
	return res
}

func contractToContractGallery(contract persist.Contract) persist.ContractGallery {
	return persist.ContractGallery{
		Address:        persist.Address(contract.Address.String()),
		Name:           contract.Name,
		Symbol:         contract.Symbol,
		CreatorAddress: contract.CreatorAddress,
	}
}

func ethereumAddressAtBlockToGallery(addrs []persist.EthereumAddressAtBlock) []persist.AddressAtBlock {
	res := make([]persist.AddressAtBlock, len(addrs))
	for i, addr := range addrs {
		res[i] = persist.AddressAtBlock{
			Address: persist.Address(addr.Address.String()),
			Block:   addr.Block,
		}
	}
	return res
}
