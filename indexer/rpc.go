package indexer

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"math/big"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
	shell "github.com/ipfs/go-ipfs-api"
	"github.com/mikeydub/go-gallery/contracts"
	"github.com/mikeydub/go-gallery/persist"
	"github.com/mikeydub/go-gallery/util"
	"github.com/sirupsen/logrus"
)

// transfer represents a transfer from the RPC response
type transfer struct {
	blockNumber     persist.BlockNumber
	from            persist.Address
	to              persist.Address
	tokenID         persist.TokenID
	tokenType       persist.TokenType
	amount          uint64
	contractAddress persist.Address
}

// tokenContractMetadata represents a token contract's metadata
type tokenContractMetadata struct {
	Name   string
	Symbol string
}

type errHTTP struct {
	url    string
	status string
}

// getTokenContractMetadata returns the metadata for a given contract (without URI)
func getTokenContractMetadata(address persist.Address, ethClient *ethclient.Client) (*tokenContractMetadata, error) {
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

	return &tokenContractMetadata{Name: name, Symbol: symbol}, nil
}

// GetMetadataFromURI parses and returns the NFT metadata for a given token URI
func GetMetadataFromURI(turi persist.TokenURI, ipfsClient *shell.Shell) (persist.TokenMetadata, error) {

	bs, err := GetDataFromURI(turi, ipfsClient)
	if err != nil {
		return persist.TokenMetadata{}, err
	}
	// parse the json
	metadata := persist.TokenMetadata{}
	err = json.Unmarshal(bs, &metadata)
	if err != nil {
		return nil, err
	}

	return metadata, nil

}

// GetDataFromURI calls URI and returns the data
func GetDataFromURI(turi persist.TokenURI, ipfsClient *shell.Shell) ([]byte, error) {

	client := &http.Client{
		Timeout: time.Second * 10,
	}

	asString := turi.String()

	switch turi.Type() {
	case persist.URITypeBase64JSON:
		// decode the base64 encoded json
		b64data := asString[strings.IndexByte(asString, ',')+1:]
		decoded, err := base64.StdEncoding.DecodeString(string(b64data))
		if err != nil {
			return nil, err
		}

		return decoded, nil
	case persist.URITypeIPFS:
		path := strings.TrimPrefix(asString, "ipfs://")
		pathMinusExtra := strings.TrimPrefix(path, "ipfs/")

		it, err := ipfsClient.Cat(pathMinusExtra)
		if err != nil {
			return nil, err
		}
		defer it.Close()

		bytes, err := ioutil.ReadAll(it)
		if err != nil {
			return nil, err
		}
		return bytes, nil
	case persist.URITypeHTTP:
		var body io.Reader
		if strings.Contains(asString, "ipfs/") {
			toCat := asString[strings.Index(asString, "ipfs/")+5:]
			it, err := ipfsClient.Cat(toCat)
			if err != nil {
				return nil, err
			}
			defer it.Close()
			body = it
		} else {
			resp, err := client.Get(asString)
			if err != nil {
				return nil, err
			}
			defer resp.Body.Close()
			if resp.StatusCode > 299 || resp.StatusCode < 200 {
				return nil, errHTTP{status: resp.Status, url: asString}
			}
			body = resp.Body
		}
		bytes, err := ioutil.ReadAll(body)
		if err != nil {
			return nil, err
		}

		return bytes, nil
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
		bytes, err := ioutil.ReadAll(it)
		if err != nil {
			return nil, err
		}

		return bytes, nil
	default:
		return nil, fmt.Errorf("unknown token URI type: %s", turi.Type())
	}

}

// GetTokenURI returns metadata URI for a given token address.
func GetTokenURI(pTokenType persist.TokenType, pContractAddress persist.Address, pTokenID persist.TokenID, ethClient *ethclient.Client) (persist.TokenURI, error) {
	contract := common.HexToAddress(string(pContractAddress))
	switch pTokenType {
	case persist.TokenTypeERC721:

		instance, err := contracts.NewIERC721MetadataCaller(contract, ethClient)
		if err != nil {
			return "", err
		}

		logrus.Debugf("Token ID: %s\tToken Address: %s", pTokenID.String(), contract.Hex())

		ctx, cancel := context.WithTimeout(context.Background(), time.Second*3)
		defer cancel()
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

		i, err := util.HexToBigInt(string(pTokenID))
		if err != nil {
			return "", err
		}
		logrus.Debugf("Token ID: %d\tToken Address: %s", i.Uint64(), contract.Hex())

		ctx, cancel := context.WithTimeout(context.Background(), time.Second*3)
		turi, err := instance.Uri(&bind.CallOpts{
			Context: ctx,
		}, i)
		cancel()
		if err != nil {
			return "", err
		}
		if turi != "" {
			return persist.TokenURI(strings.ReplaceAll(turi, "\x00", "")), nil
		}

		topics := [][]common.Hash{{common.HexToHash("0x6bb7ff708619ba0610cba295a58592e0451dee2622938c8755667688daf3529b")}, {common.HexToHash("0x" + padHex(string(pTokenID), 64))}}
		ctx, cancel = context.WithTimeout(context.Background(), time.Second*15)
		defer cancel()

		logs, err := ethClient.FilterLogs(ctx, ethereum.FilterQuery{
			FromBlock: defaultStartingBlock.BigInt(),
			Addresses: []common.Address{contract},
			Topics:    topics,
		})
		if err != nil {
			return "", err
		}
		if len(logs) == 0 {
			return "", errors.New("no logs found")
		}

		sort.Slice(logs, func(i, j int) bool {
			return logs[i].BlockNumber > logs[j].BlockNumber
		})
		if len(logs[0].Data) < 128 {
			return "", errors.New("invalid data")
		}

		offset := new(big.Int).SetBytes(logs[0].Data[:32])
		length := new(big.Int).SetBytes(logs[0].Data[32:64])
		uri := persist.TokenURI(logs[0].Data[offset.Uint64()+32 : offset.Uint64()+32+length.Uint64()])
		return uri, nil
	default:
		return "", fmt.Errorf("unknown token type: %s", pTokenType)
	}
}

func getBalanceOfERC1155Token(pOwnerAddress, pContractAddress persist.Address, pTokenID persist.TokenID, ethClient *ethclient.Client) (*big.Int, error) {
	contract := common.HexToAddress(string(pContractAddress))
	owner := common.HexToAddress(string(pOwnerAddress))
	instance, err := contracts.NewIERC1155(contract, ethClient)
	if err != nil {
		return nil, err
	}

	i, err := util.HexToBigInt(string(pTokenID))
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
	defer cancel()
	bal, err := instance.BalanceOf(&bind.CallOpts{
		Context: ctx,
	}, owner, i)
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

func (h errHTTP) Error() string {
	return fmt.Sprintf("HTTP Error Status - %s | URL - %s", h.status, h.url)
}
