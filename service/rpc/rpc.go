package rpc

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
	shell "github.com/ipfs/go-ipfs-api"
	"github.com/mikeydub/go-gallery/contracts"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/sirupsen/logrus"
)

// Transfer represents a Transfer from the RPC response
type Transfer struct {
	BlockNumber     persist.BlockNumber
	From            persist.Address
	To              persist.Address
	TokenID         persist.TokenID
	TokenType       persist.TokenType
	Amount          uint64
	ContractAddress persist.Address
}

// TokenContractMetadata represents a token contract's metadata
type TokenContractMetadata struct {
	Name   string
	Symbol string
}

// ErrHTTP represents an error returned from an HTTP request
type ErrHTTP struct {
	URL    string
	Status int
}

// GetTokenContractMetadata returns the metadata for a given contract (without URI)
func GetTokenContractMetadata(address persist.Address, ethClient *ethclient.Client) (*TokenContractMetadata, error) {
	contract := address.Address()
	instance, err := contracts.NewIERC721MetadataCaller(contract, ethClient)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
	defer cancel()
	name, err := instance.Name(&bind.CallOpts{
		Context: ctx,
	})
	if err != nil {
		return nil, err
	}
	symbol, err := instance.Symbol(&bind.CallOpts{
		Context: ctx,
	})
	if err != nil {
		return nil, err
	}

	return &TokenContractMetadata{Name: name, Symbol: symbol}, nil
}

// GetMetadataFromURI parses and returns the NFT metadata for a given token URI
func GetMetadataFromURI(ctx context.Context, turi persist.TokenURI, ipfsClient *shell.Shell) (persist.TokenMetadata, error) {

	bs, err := GetDataFromURI(ctx, turi, ipfsClient)
	if err != nil {
		return persist.TokenMetadata{}, err
	}

	// remove BOM https://en.wikipedia.org/wiki/Byte_order_mark
	bs = bytes.TrimPrefix(bs, []byte("\xef\xbb\xbf"))

	var metadata persist.TokenMetadata
	switch turi.Type() {

	case persist.URITypeBase64SVG, persist.URITypeSVG:
		metadata = persist.TokenMetadata{"image": string(bs)}
	default:
		err = json.Unmarshal(bs, &metadata)
		if err != nil {
			return persist.TokenMetadata{}, err
		}

	}

	return metadata, nil

}

// GetDataFromURI calls URI and returns the data
func GetDataFromURI(ctx context.Context, turi persist.TokenURI, ipfsClient *shell.Shell) ([]byte, error) {

	client := &http.Client{}
	deadline, ok := ctx.Deadline()
	if !ok {
		deadline = time.Now().Add(time.Second * 10)
	}
	client.Timeout = time.Until(deadline)

	asString := turi.String()

	switch turi.Type() {
	case persist.URITypeBase64JSON, persist.URITypeBase64SVG:
		// decode the base64 encoded json
		b64data := asString[strings.IndexByte(asString, ',')+1:]
		decoded, err := base64.StdEncoding.DecodeString(string(b64data))
		if err != nil {
			return nil, fmt.Errorf("error decoding base64 data: %s \n\n%s", err, b64data)
		}

		return decoded, nil
	case persist.URITypeIPFS:
		path := strings.TrimPrefix(asString, "ipfs://")
		pathMinusExtra := strings.TrimPrefix(path, "ipfs/")

		it, err := ipfsClient.Cat(pathMinusExtra)
		if err != nil {
			return nil, fmt.Errorf("error getting data from ipfs: %s", err)
		}
		defer it.Close()

		bs, err := io.ReadAll(it)
		if err != nil {
			return nil, fmt.Errorf("error reading data from ipfs: %s", err)
		}
		return bs, nil
	case persist.URITypeHTTP:

		resp, err := client.Get(asString)
		if err != nil {
			return nil, fmt.Errorf("error getting data from http: %s", err)
		}
		if resp.StatusCode > 299 || resp.StatusCode < 200 {
			return nil, ErrHTTP{Status: resp.StatusCode, URL: asString}
		}
		defer resp.Body.Close()

		buf := &bytes.Buffer{}
		if _, err := io.Copy(buf, resp.Body); err != nil {
			return nil, err
		}

		return buf.Bytes(), nil
	case persist.URITypeIPFSAPI:
		parsedURL, err := url.Parse(asString)
		if err != nil {
			return nil, err
		}
		query := parsedURL.Query().Get("arg")
		it, err := ipfsClient.Cat(query)
		if err != nil {
			return nil, err
		}
		defer it.Close()
		buf := &bytes.Buffer{}
		if _, err = io.Copy(buf, it); err != nil {
			return nil, err
		}

		return buf.Bytes(), nil
	case persist.URITypeJSON, persist.URITypeSVG:
		asString = strings.Replace(asString, "data:application/json;utf8,", "", 0)
		asString = strings.Replace(asString, "data:text/plain,{,", "{", 0)
		return []byte(asString), nil
	default:
		return nil, fmt.Errorf("unknown token URI type: %s", turi.Type())
	}

}

// GetTokenURI returns metadata URI for a given token address.
func GetTokenURI(ctx context.Context, pTokenType persist.TokenType, pContractAddress persist.Address, pTokenID persist.TokenID, ethClient *ethclient.Client) (persist.TokenURI, error) {

	contract := common.HexToAddress(string(pContractAddress))
	switch pTokenType {
	case persist.TokenTypeERC721:

		instance, err := contracts.NewIERC721MetadataCaller(contract, ethClient)
		if err != nil {
			return "", err
		}

		logrus.Debugf("Token ID: %s\tToken Address: %s", pTokenID.String(), contract.Hex())

		turi, err := instance.TokenURI(&bind.CallOpts{
			Context: ctx,
		}, pTokenID.BigInt())
		if err != nil {
			return "", err
		}

		return persist.TokenURI(strings.ReplaceAll(turi, "\x00", "")), nil
	case persist.TokenTypeERC1155:

		instance, err := contracts.NewIERC1155MetadataURI(contract, ethClient)
		if err != nil {
			return "", err
		}

		logrus.Debugf("Token ID: %d\tToken Address: %s", pTokenID.BigInt().Uint64(), contract.Hex())

		turi, err := instance.Uri(&bind.CallOpts{
			Context: ctx,
		}, pTokenID.BigInt())
		if err != nil {
			return "", err
		}

		return persist.TokenURI(strings.ReplaceAll(turi, "\x00", "")), nil

	default:
		return "", fmt.Errorf("unknown token type: %s", pTokenType)
	}
}

// GetBalanceOfERC1155Token returns the balance of an ERC1155 token
func GetBalanceOfERC1155Token(pOwnerAddress, pContractAddress persist.Address, pTokenID persist.TokenID, ethClient *ethclient.Client) (*big.Int, error) {
	contract := common.HexToAddress(string(pContractAddress))
	owner := common.HexToAddress(string(pOwnerAddress))
	instance, err := contracts.NewIERC1155(contract, ethClient)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
	defer cancel()
	bal, err := instance.BalanceOf(&bind.CallOpts{
		Context: ctx,
	}, owner, pTokenID.BigInt())
	if err != nil {
		return nil, err
	}

	return bal, nil
}

func padHex(pHex string, pLength int) string {
	for len(pHex) < pLength {
		pHex = "0" + pHex
	}
	return pHex
}

func (h ErrHTTP) Error() string {
	return fmt.Sprintf("HTTP Error Status - %d | URL - %s", h.Status, h.URL)
}
