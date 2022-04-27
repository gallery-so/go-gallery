package eth

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/mikeydub/go-gallery/contracts"
	"github.com/mikeydub/go-gallery/indexer"
	"github.com/mikeydub/go-gallery/service/auth"
	"github.com/mikeydub/go-gallery/service/multichain"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/util"
	"github.com/sirupsen/logrus"
)

var eip1271MagicValue = [4]byte{0x16, 0x26, 0xBA, 0x7E}

// Provider is an the struct for retrieving data from the Ethereum blockchain
type Provider struct {
	indexerBaseURL string
	httpClient     *http.Client
	ethClient      *ethclient.Client
}

// NewDataRetriever creates a new DataRetriever
func NewDataRetriever(indexerBaseURL string, httpClient *http.Client, ec *ethclient.Client) *Provider {
	return &Provider{
		indexerBaseURL: indexerBaseURL,
		httpClient:     httpClient,
		ethClient:      ec,
	}
}

// GetBlockchainInfo retrieves blockchain info for ETH
func (d *Provider) GetBlockchainInfo(ctx context.Context) (multichain.BlockchainInfo, error) {
	return multichain.BlockchainInfo{
		ChainName: persist.ChainETH,
		ChainID:   0,
	}, nil
}

// GetTokensByWalletAddress retrieves tokens for a wallet address on the Ethereum Blockchain
func (d *Provider) GetTokensByWalletAddress(ctx context.Context, addr persist.EthereumAddress) ([]persist.TokenGallery, error) {
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
func (d *Provider) GetTokensByContractAddress(ctx context.Context, contractAddress persist.EthereumAddress) ([]persist.TokenGallery, error) {
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
func (d *Provider) GetTokensByTokenIdentifiers(ctx context.Context, tokenIdentifiers persist.TokenIdentifiers) ([]persist.TokenGallery, error) {
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
func (d *Provider) GetContractByAddress(ctx context.Context, addr persist.EthereumAddress) (persist.ContractGallery, error) {
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
func (d *Provider) UpdateMediaForWallet(ctx context.Context, wallet persist.EthereumAddress, all bool) error {

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
func (d *Provider) ValidateTokensForWallet(ctx context.Context, wallet persist.EthereumAddress, all bool) error {
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

// VerifySignature will verify a signature using all available methods (eth_sign and personal_sign)
func (d *Provider) VerifySignature(pCtx context.Context,
	pAddressStr string, pWalletType persist.WalletType, pNonce string, pSignatureStr string) (bool, error) {

	nonce := auth.NewNoncePrepend + pNonce
	// personal_sign
	validBool, err := verifySignature(pSignatureStr,
		nonce,
		pAddressStr, pWalletType,
		true, d.ethClient)

	if !validBool || err != nil {
		// eth_sign
		validBool, err = verifySignature(pSignatureStr,
			nonce,
			pAddressStr, pWalletType,
			false, d.ethClient)
		if err != nil || !validBool {
			nonce = auth.NoncePrepend + pNonce
			validBool, err = verifySignature(pSignatureStr,
				nonce,
				pAddressStr, pWalletType,
				true, d.ethClient)
			if err != nil || !validBool {
				validBool, err = verifySignature(pSignatureStr,
					nonce,
					pAddressStr, pWalletType,
					false, d.ethClient)
			}
		}
	}

	if err != nil {
		return false, err
	}

	return validBool, nil
}

func verifySignature(pSignatureStr string,
	pData string,
	pAddress string, pWalletType persist.WalletType,
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
		log.Println("pubkeyAddressHexStr:", pubkeyAddressHexStr)
		log.Println("pAddress:", pAddress)
		if !strings.EqualFold(pubkeyAddressHexStr, pAddress) {
			return false, auth.ErrAddressSignatureMismatch
		}

		publicKeyBytes := crypto.CompressPubkey(sigPublicKeyECDSA)

		signatureNoRecoverID := sig[:len(sig)-1]

		return crypto.VerifySignature(publicKeyBytes, dataHash.Bytes(), signatureNoRecoverID), nil
	case persist.WalletTypeGnosis:
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		sigValidator, err := contracts.NewISignatureValidator(common.HexToAddress(pAddress), ec)
		if err != nil {
			return false, err
		}

		hashedData := crypto.Keccak256([]byte(data))
		var input [32]byte
		copy(input[:], hashedData)

		result, err := sigValidator.IsValidSignature(&bind.CallOpts{Context: ctx}, input, []byte{})
		if err != nil {
			logrus.WithError(err).Error("IsValidSignature")
			return false, nil
		}

		return result == eip1271MagicValue, nil
	default:
		return false, errors.New("wallet type not supported")
	}

}

func tokensToTokensGallery(tokens []persist.Token) []persist.TokenGallery {
	res := make([]persist.TokenGallery, len(tokens))
	for i, token := range tokens {
		res[i] = persist.TokenGallery{
			TokenID:          token.TokenID,
			ContractAddress:  persist.Address{Address: persist.AddressValue(token.ContractAddress.String()), Chain: persist.ChainETH},
			OwnerAddress:     persist.Address{Address: persist.AddressValue(token.OwnerAddress.String()), Chain: persist.ChainETH},
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
		Address:        persist.Address{Address: persist.AddressValue(contract.Address.String()), Chain: persist.ChainETH},
		Name:           contract.Name,
		Symbol:         contract.Symbol,
		CreatorAddress: contract.CreatorAddress,
	}
}

func ethereumAddressAtBlockToGallery(addrs []persist.EthereumAddressAtBlock) []persist.AddressAtBlock {
	res := make([]persist.AddressAtBlock, len(addrs))
	for i, addr := range addrs {
		res[i] = persist.AddressAtBlock{
			Address: persist.Address{Address: persist.AddressValue(addr.Address.String()), Chain: persist.ChainETH},
			Block:   addr.Block,
		}
	}
	return res
}
