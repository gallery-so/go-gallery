package infra

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"math/big"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/mikeydub/go-gallery/contracts"
	"github.com/mikeydub/go-gallery/persist"
	"github.com/mikeydub/go-gallery/runtime"
	"github.com/mikeydub/go-gallery/util"
	"github.com/sirupsen/logrus"
)

const defaultERC721Block = 4900000

// transfers represents the transfers for a given rpc response
type transfers struct {
	PageKey   string      `json:"pageKey"`
	Transfers []*transfer `json:"transfers"`
}

// transfer represents a transfer from the RPC response
type transfer struct {
	Category    string            `json:"category"`
	BlockNumber string            `json:"blockNum"`
	From        string            `json:"from"`
	To          string            `json:"to"`
	Value       float64           `json:"value"`
	TokenID     string            `json:"erc721TokenId"`
	Type        persist.TokenType `json:"type"`
	Amount      uint64            `json:"amount"`
	Asset       string            `json:"asset"`
	Hash        string            `json:"hash"`
	RawContract contract          `json:"rawContract"`
}

// contract represents a contract that is interacted with during a transfer
type contract struct {
	Address string `json:"address"`
	Value   string `json:"value"`
	Decimal string `json:"decimal"`
}

// tokenContractMetadata represents a token contract's metadata
type tokenContractMetadata struct {
	Name   string `json:"name"`
	Symbol string `json:"symbol"`
}

type uriWithMetadata struct {
	uri string
	md  map[string]interface{}
}

type tokenWithBlockNumber struct {
	token       *persist.Token
	blockNumber string
}

// getTokenContractMetadata returns the metadata for a given contract (without URI)
func getTokenContractMetadata(address string, pRuntime *runtime.Runtime) (*tokenContractMetadata, error) {
	contract := common.HexToAddress(address)
	instance, err := contracts.NewIERC721MetadataCaller(contract, pRuntime.InfraClients.ETHClient)
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

// getERC721TokenURI returns metadata URI for a given token address
func getERC721TokenURI(address, tokenID string, pRuntime *runtime.Runtime) (string, error) {

	contract := common.HexToAddress(address)
	instance, err := contracts.NewIERC721MetadataCaller(contract, pRuntime.InfraClients.ETHClient)
	if err != nil {
		return "", err
	}

	i, err := util.HexToBigInt(tokenID)
	if err != nil {
		return "", err
	}
	logrus.Debugf("Token ID: %d\tToken Address: %s", i.Uint64(), contract.Hex())

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*3)
	defer cancel()
	tokenURI, err := instance.TokenURI(&bind.CallOpts{
		Context: ctx,
	}, i)
	if err != nil {
		return "", err
	}

	return strings.ReplaceAll(tokenURI, "\x00", ""), nil

}

// getMetadataFromURI parses and returns the NFT metadata for a given token URI
func getMetadataFromURI(tokenURI string, pRuntime *runtime.Runtime) (map[string]interface{}, persist.MediaType, error) {

	client := &http.Client{
		Timeout: time.Second * 3,
	}

	if strings.Contains(tokenURI, "data:application/json;base64,") {
		// decode the base64 encoded json
		b64data := tokenURI[strings.IndexByte(tokenURI, ',')+1:]
		decoded, err := base64.StdEncoding.DecodeString(b64data)
		if err != nil {
			return nil, "", err
		}

		metadata := map[string]interface{}{}
		err = json.Unmarshal(decoded, &metadata)
		if err != nil {
			return nil, "", err
		}

		return metadata, persist.MediaTypeBase64JSON, nil
	} else if strings.HasPrefix(tokenURI, "ipfs://") {

		path := strings.TrimPrefix(tokenURI, "ipfs://")

		it, err := pRuntime.IPFS.Cat(path)
		if err != nil {
			return nil, "", err
		}
		defer it.Close()

		buf := &bytes.Buffer{}
		_, err = io.Copy(buf, it)
		if err != nil {
			return nil, "", err
		}
		metadata := map[string]interface{}{}
		err = json.Unmarshal(buf.Bytes(), &metadata)
		if err != nil {
			return nil, "", err
		}

		return metadata, persist.SniffMediaType(buf.Bytes()), nil
	} else if strings.HasPrefix(tokenURI, "https://") || strings.HasPrefix(tokenURI, "http://") {
		resp, err := client.Get(tokenURI)
		if err != nil {
			return nil, "", err
		}
		defer resp.Body.Close()
		buf := &bytes.Buffer{}
		_, err = io.Copy(buf, resp.Body)
		if err != nil {
			return nil, "", err
		}

		// parse the json
		metadata := map[string]interface{}{}
		err = json.Unmarshal(buf.Bytes(), &metadata)
		if err != nil {
			return nil, "", err
		}

		return metadata, persist.SniffMediaType(buf.Bytes()), nil
	} else {
		return nil, persist.MediaTypeUnknown, nil
	}

}

// if logging all events is too large and takes too much time, start from the front and go backwards until one is found
// given that the most recent URI event should be the current URI
func getERC1155TokenURI(pContractAddress, pTokenID string, pRuntime *runtime.Runtime) (string, error) {
	topics := [][]common.Hash{{common.HexToHash("0x6bb7ff708619ba0610cba295a58592e0451dee2622938c8755667688daf3529b")}, {common.HexToHash("0x" + padHex(pTokenID, 64))}}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*15)
	defer cancel()

	def := new(big.Int).SetUint64(defaultERC721Block)

	logs, err := pRuntime.InfraClients.ETHClient.FilterLogs(ctx, ethereum.FilterQuery{
		FromBlock: def,
		Addresses: []common.Address{common.HexToAddress(pContractAddress)},
		Topics:    topics,
	})
	if err != nil {
		return "", err
	}
	if len(logs) == 0 {
		return "", errors.New("No logs found")
	}

	sort.Slice(logs, func(i, j int) bool {
		return logs[i].BlockNumber > logs[j].BlockNumber
	})
	if len(logs[0].Data) < 128 {
		return "", errors.New("invalid data")
	}

	offset := new(big.Int).SetBytes(logs[0].Data[:32])
	length := new(big.Int).SetBytes(logs[0].Data[32:64])
	uri := string(logs[0].Data[offset.Uint64()+32 : offset.Uint64()+32+length.Uint64()])
	return uri, nil
}

func getBalanceOfERC1155Token(pOwnerAddress, pContractAddress, pTokenID string, pRuntime *runtime.Runtime) (*big.Int, error) {
	contract := common.HexToAddress(pContractAddress)
	owner := common.HexToAddress(pOwnerAddress)
	instance, err := contracts.NewIERC1155(contract, pRuntime.InfraClients.ETHClient)
	if err != nil {
		return nil, err
	}

	i, err := util.HexToBigInt(pTokenID)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
	defer cancel()
	tokenURI, err := instance.BalanceOf(&bind.CallOpts{
		Context: ctx,
	}, owner, i)
	if err != nil {
		return nil, err
	}

	return tokenURI, nil
}

func padHex(pHex string, pLength int) string {
	for len(pHex) < pLength {
		pHex = "0" + pHex
	}
	return pHex
}
