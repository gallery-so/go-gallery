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
	"github.com/sirupsen/logrus"
)

var eip1271MagicValue = [4]byte{0x16, 0x26, 0xBA, 0x7E}

// Provider is an the struct for retrieving data from the Ethereum blockchain
type Provider struct {
	indexerBaseURL string
	httpClient     *http.Client
	ethClient      *ethclient.Client
}

// NewProvider creates a new ethereum Provider
func NewProvider(indexerBaseURL string, httpClient *http.Client, ec *ethclient.Client) *Provider {
	return &Provider{
		indexerBaseURL: indexerBaseURL,
		httpClient:     httpClient,
		ethClient:      ec,
	}
}

// GetBlockchainInfo retrieves blockchain info for ETH
func (d *Provider) GetBlockchainInfo(ctx context.Context) (multichain.BlockchainInfo, error) {
	return multichain.BlockchainInfo{
		Chain:   persist.ChainETH,
		ChainID: 0,
	}, nil
}

// GetTokensByWalletAddress retrieves tokens for a wallet address on the Ethereum Blockchain
func (d *Provider) GetTokensByWalletAddress(ctx context.Context, addr persist.Address) ([]multichain.ChainAgnosticToken, []multichain.ChainAgnosticContract, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("%s/nfts/get?address=%s", d.indexerBaseURL, addr), nil)
	if err != nil {
		return nil, nil, err
	}
	res, err := d.httpClient.Do(req)
	if err != nil {
		return nil, nil, err
	}
	defer res.Body.Close()

	if res.StatusCode != 200 {
		return nil, nil, getErrFromResp(res)
	}

	var tokens indexer.GetTokensOutput
	err = json.NewDecoder(res.Body).Decode(&tokens)
	if err != nil {
		return nil, nil, err
	}

	return tokensToChainAgnostic(tokens.NFTs), contractsToChainAgnostic(tokens.Contracts), nil

}

// GetTokensByContractAddress retrieves tokens for a contract address on the Ethereum Blockchain
func (d *Provider) GetTokensByContractAddress(ctx context.Context, contractAddress persist.Address) ([]multichain.ChainAgnosticToken, multichain.ChainAgnosticContract, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("%s/nfts/get?contract_address=%s&limit=-1", d.indexerBaseURL, contractAddress), nil)
	if err != nil {
		return nil, multichain.ChainAgnosticContract{}, err
	}
	res, err := d.httpClient.Do(req)
	if err != nil {
		return nil, multichain.ChainAgnosticContract{}, err
	}
	defer res.Body.Close()

	if res.StatusCode != 200 {
		return nil, multichain.ChainAgnosticContract{}, getErrFromResp(res)
	}

	var tokens indexer.GetTokensOutput
	err = json.NewDecoder(res.Body).Decode(&tokens)
	if err != nil {
		return nil, multichain.ChainAgnosticContract{}, err
	}

	contract := multichain.ChainAgnosticContract{}
	if len(tokens.Contracts) > 0 {
		contract = contractToChainAgnostic(tokens.Contracts[0])
	}
	return tokensToChainAgnostic(tokens.NFTs), contract, nil
}

// GetTokensByTokenIdentifiers retrieves tokens for a token identifiers on the Ethereum Blockchain
func (d *Provider) GetTokensByTokenIdentifiers(ctx context.Context, tokenIdentifiers multichain.ChainAgnosticIdentifiers) ([]multichain.ChainAgnosticToken, []multichain.ChainAgnosticContract, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("%s/nfts/get?contract_address=%s&token_id=%s&limit=-1", d.indexerBaseURL, tokenIdentifiers.ContractAddress, tokenIdentifiers.TokenID), nil)
	if err != nil {
		return nil, nil, err
	}
	res, err := d.httpClient.Do(req)
	if err != nil {
		return nil, nil, err
	}
	defer res.Body.Close()

	if res.StatusCode != 200 {

		return nil, nil, getErrFromResp(res)
	}

	var tokens indexer.GetTokensOutput
	err = json.NewDecoder(res.Body).Decode(&tokens)
	if err != nil {
		return nil, nil, err
	}

	return tokensToChainAgnostic(tokens.NFTs), contractsToChainAgnostic(tokens.Contracts), nil
}

// GetContractByAddress retrieves an ethereum contract by address
func (d *Provider) GetContractByAddress(ctx context.Context, addr persist.Address) (multichain.ChainAgnosticContract, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("%s/contracts/get?address=%s", d.indexerBaseURL, addr), nil)
	if err != nil {
		return multichain.ChainAgnosticContract{}, err
	}
	res, err := d.httpClient.Do(req)
	if err != nil {
		return multichain.ChainAgnosticContract{}, err
	}
	defer res.Body.Close()

	if res.StatusCode != 200 {
		return multichain.ChainAgnosticContract{}, getErrFromResp(res)
	}
	var contract indexer.GetContractOutput
	err = json.NewDecoder(res.Body).Decode(&contract)
	if err != nil {
		return multichain.ChainAgnosticContract{}, err
	}

	return contractToChainAgnostic(contract.Contract), nil

}

// RefreshToken refreshes the metadata for a given token.
func (d *Provider) RefreshToken(ctx context.Context, ti multichain.ChainAgnosticIdentifiers) error {

	input := indexer.UpdateTokenMediaInput{
		TokenID:         ti.TokenID,
		ContractAddress: persist.EthereumAddress(persist.ChainETH.NormalizeAddress(ti.ContractAddress)),
		UpdateAll:       true,
	}

	m, err := json.Marshal(input)

	buf := bytes.NewBuffer(m)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, fmt.Sprintf("%s/nfts/refresh", d.indexerBaseURL), buf)
	if err != nil {
		return err
	}
	res, err := d.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()

	if res.StatusCode != 200 {

		return getErrFromResp(res)
	}

	return nil
}

// UpdateMediaForWallet updates media for the tokens owned by a wallet on the Ethereum Blockchain
func (d *Provider) UpdateMediaForWallet(ctx context.Context, wallet persist.Address, all bool) error {

	input := indexer.UpdateTokenMediaInput{
		OwnerAddress: persist.EthereumAddress(persist.ChainETH.NormalizeAddress(wallet)),
		UpdateAll:    all,
	}

	asJSON, err := json.Marshal(input)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, fmt.Sprintf("%s/nfts/refresh", d.indexerBaseURL), bytes.NewReader(asJSON))
	if err != nil {
		return err
	}

	res, err := d.httpClient.Do(req)
	if err != nil {
		return err
	}

	defer res.Body.Close()

	if res.StatusCode != 200 {
		return getErrFromResp(res)
	}

	return nil
}

// RefreshContract refreshes the metadata for a contract
func (d *Provider) RefreshContract(ctx context.Context, addr persist.Address) error {
	input := indexer.UpdateContractMediaInput{
		Address: persist.EthereumAddress(persist.ChainETH.NormalizeAddress(addr)),
	}

	asJSON, err := json.Marshal(input)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, fmt.Sprintf("%s/contracts/refresh", d.indexerBaseURL), bytes.NewReader(asJSON))
	if err != nil {
		return err
	}

	res, err := d.httpClient.Do(req)
	if err != nil {
		return err
	}

	defer res.Body.Close()

	if res.StatusCode != 200 {
		return getErrFromResp(res)
	}

	return nil
}

// ValidateTokensForWallet validates tokens for a wallet address on the Ethereum Blockchain
func (d *Provider) ValidateTokensForWallet(ctx context.Context, wallet persist.Address, all bool) error {
	input := indexer.ValidateWalletNFTsInput{Wallet: persist.EthereumAddress(wallet.String()), All: all}

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

	if res.StatusCode != 200 {
		return getErrFromResp(res)
	}

	return nil

}

// VerifySignature will verify a signature using all available methods (eth_sign and personal_sign)
func (d *Provider) VerifySignature(pCtx context.Context,
	pAddressStr persist.Address, pWalletType persist.WalletType, pNonce string, pSignatureStr string) (bool, error) {

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
	pAddress persist.Address, pWalletType persist.WalletType,
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
		if !strings.EqualFold(pubkeyAddressHexStr, pAddress.String()) {
			return false, auth.ErrAddressSignatureMismatch
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
			logrus.WithError(err).Error("IsValidSignature")
			return false, nil
		}

		return result == eip1271MagicValue, nil
	default:
		return false, errors.New("wallet type not supported")
	}

}

func tokensToChainAgnostic(tokens []persist.Token) []multichain.ChainAgnosticToken {
	res := make([]multichain.ChainAgnosticToken, len(tokens))
	for i, token := range tokens {
		res[i] = multichain.ChainAgnosticToken{
			TokenID:          token.TokenID,
			ContractAddress:  persist.Address(token.ContractAddress.String()),
			OwnerAddress:     persist.Address(token.OwnerAddress.String()),
			TokenURI:         token.TokenURI,
			Media:            token.Media,
			TokenType:        token.TokenType,
			Name:             token.Name.String(),
			Description:      token.Description.String(),
			Quantity:         token.Quantity,
			OwnershipHistory: ethereumAddressAtBlockToChainAgnostic(token.OwnershipHistory),
			TokenMetadata:    token.TokenMetadata,
			ExternalURL:      token.ExternalURL.String(),
			BlockNumber:      token.BlockNumber,
		}
	}
	return res
}

func contractsToChainAgnostic(contracts []persist.Contract) []multichain.ChainAgnosticContract {
	result := make([]multichain.ChainAgnosticContract, len(contracts))
	for _, contract := range contracts {
		result = append(result, contractToChainAgnostic(contract))
	}
	return result
}

func contractToChainAgnostic(contract persist.Contract) multichain.ChainAgnosticContract {
	return multichain.ChainAgnosticContract{
		Address:        persist.Address(contract.Address.String()),
		Name:           contract.Name.String(),
		Symbol:         contract.Symbol.String(),
		CreatorAddress: persist.Address(contract.CreatorAddress.String()),
	}
}

func ethereumAddressAtBlockToChainAgnostic(addrs []persist.EthereumAddressAtBlock) []multichain.ChainAgnosticAddressAtBlock {
	res := make([]multichain.ChainAgnosticAddressAtBlock, len(addrs))
	for i, addr := range addrs {
		res[i] = multichain.ChainAgnosticAddressAtBlock{
			Address: persist.Address(addr.Address.String()),
			Block:   addr.Block,
		}
	}
	return res
}

func getErrFromResp(res *http.Response) error {
	errResp := map[string]interface{}{}
	json.NewDecoder(res.Body).Decode(&errResp)
	return fmt.Errorf("unexpected status: %s | err: %v ", res.Status, errResp)
}
