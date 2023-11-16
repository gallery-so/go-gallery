package eth

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/mikeydub/go-gallery/service/task"
	"net/http"
	"strings"
	"time"

	ens "github.com/benny-conn/go-ens"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/mikeydub/go-gallery/contracts"
	"github.com/mikeydub/go-gallery/env"
	"github.com/mikeydub/go-gallery/indexer"
	"github.com/mikeydub/go-gallery/service/auth"
	"github.com/mikeydub/go-gallery/service/logger"
	"github.com/mikeydub/go-gallery/service/multichain"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/util"
)

var eip1271MagicValue = [4]byte{0x16, 0x26, 0xBA, 0x7E}

// Provider is an the struct for retrieving data from the Ethereum blockchain
type Provider struct {
	indexerBaseURL string
	httpClient     *http.Client
	ethClient      *ethclient.Client
	taskClient     *task.Client
}

// NewProvider creates a new ethereum Provider
func NewProvider(httpClient *http.Client, ec *ethclient.Client, tc *task.Client) *Provider {
	return &Provider{
		indexerBaseURL: env.GetString("INDEXER_HOST"),
		httpClient:     httpClient,
		ethClient:      ec,
		taskClient:     tc,
	}
}

// GetBlockchainInfo retrieves blockchain info for ETH
func (d *Provider) GetBlockchainInfo() multichain.BlockchainInfo {
	return multichain.BlockchainInfo{
		Chain:      persist.ChainETH,
		ChainID:    0,
		ProviderID: "eth",
	}
}

// GetTokenMetadataByTokenIdentifiers retrieves a token's metadata for a given contract address and token ID
func (d *Provider) GetTokenMetadataByTokenIdentifiers(ctx context.Context, ti multichain.ChainAgnosticIdentifiers) (persist.TokenMetadata, error) {
	url := fmt.Sprintf("%s/nfts/get/metadata?contract_address=%s&token_id=%s", d.indexerBaseURL, ti.ContractAddress, ti.TokenID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	res, err := d.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	if res.StatusCode != 200 {
		return nil, util.GetErrFromResp(res)
	}

	var tokens indexer.GetTokenMetadataOutput
	err = json.NewDecoder(res.Body).Decode(&tokens)
	if err != nil {
		return nil, err
	}

	return tokens.Metadata, nil
}

func (d *Provider) GetTokenDescriptorsByTokenIdentifiers(ctx context.Context, ti multichain.ChainAgnosticIdentifiers) (multichain.ChainAgnosticTokenDescriptors, multichain.ChainAgnosticContractDescriptors, error) {
	metadata, err := d.GetTokenMetadataByTokenIdentifiers(ctx, ti)
	if err != nil {
		return multichain.ChainAgnosticTokenDescriptors{}, multichain.ChainAgnosticContractDescriptors{}, err
	}
	name, _ := metadata["name"].(string)
	description, _ := metadata["description"].(string)
	contractName, _ := metadata["contract_name"].(string)
	contractDescription, _ := metadata["contract_description"].(string)
	contractSymbol, _ := metadata["contract_symbol"].(string)
	return multichain.ChainAgnosticTokenDescriptors{
			Name:        name,
			Description: description,
		}, multichain.ChainAgnosticContractDescriptors{
			Name:        contractName,
			Symbol:      contractSymbol,
			Description: contractDescription,
		}, nil
}

// GetContractByAddress retrieves an ethereum contract by address
func (d *Provider) GetContractByAddress(ctx context.Context, addr persist.Address) (multichain.ChainAgnosticContract, error) {
	logger.For(ctx).Warn("ETH")
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
		return multichain.ChainAgnosticContract{}, util.GetErrFromResp(res)
	}
	var contract indexer.GetContractOutput
	err = json.NewDecoder(res.Body).Decode(&contract)
	if err != nil {
		return multichain.ChainAgnosticContract{}, err
	}

	return contractToChainAgnostic(contract.Contract), nil

}

// GetContractsByOwnerAddress retrieves ethereum contracts by their owner address
func (d *Provider) GetContractsByOwnerAddress(ctx context.Context, addr persist.Address) ([]multichain.ChainAgnosticContract, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("%s/contracts/get?owner=%s", d.indexerBaseURL, addr), nil)
	if err != nil {
		return nil, err
	}
	res, err := d.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	if res.StatusCode != 200 {
		return nil, util.GetErrFromResp(res)
	}
	var contract indexer.GetContractsOutput
	err = json.NewDecoder(res.Body).Decode(&contract)
	if err != nil {
		return nil, err
	}

	out := make([]multichain.ChainAgnosticContract, len(contract.Contracts))
	for i, c := range contract.Contracts {
		out[i] = contractToChainAgnostic(c)
	}

	return out, nil
}

func (d *Provider) GetDisplayNameByAddress(ctx context.Context, addr persist.Address) string {

	resultChan := make(chan string)
	errChan := make(chan error)
	go func() {
		// no context? who do these guys think they are!? I had to add a goroutine to make sure this doesn't block forever
		domain, err := ens.ReverseResolve(d.ethClient, persist.EthereumAddress(addr).Address())
		if err != nil {
			errChan <- err
			return
		}
		resultChan <- domain
	}()
	select {
	case result := <-resultChan:
		return result
	case err := <-errChan:
		logger.For(ctx).Errorf("error resolving ens domain: %s", err.Error())
		return addr.String()
	case <-ctx.Done():
		logger.For(ctx).Errorf("error resolving ens domain: %s", ctx.Err().Error())
		return addr.String()
	}
}

// RefreshContract refreshes the metadata for a contract
func (d *Provider) RefreshContract(ctx context.Context, addr persist.Address) error {
	input := indexer.UpdateContractMetadataInput{
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
		return util.GetErrFromResp(res)
	}

	return nil
}

// VerifySignature will verify a signature using all available methods (eth_sign and personal_sign)
func (d *Provider) VerifySignature(pCtx context.Context,
	pAddressStr persist.PubKey, pWalletType persist.WalletType, pNonce string, pSignatureStr string) (bool, error) {

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
			logger.For(nil).WithError(err).Error("IsValidSignature")
			return false, nil
		}

		return result == eip1271MagicValue, nil
	default:
		return false, errors.New("wallet type not supported")
	}

}

func contractToChainAgnostic(contract persist.Contract) multichain.ChainAgnosticContract {
	return multichain.ChainAgnosticContract{
		Address: persist.Address(contract.Address.String()),
		Descriptors: multichain.ChainAgnosticContractDescriptors{
			Name:         contract.Name.String(),
			Symbol:       contract.Symbol.String(),
			OwnerAddress: persist.Address(util.FirstNonEmptyString(contract.OwnerAddress.String(), contract.CreatorAddress.String())),
		},
	}
}
