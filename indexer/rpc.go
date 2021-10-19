package indexer

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
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
	Category    string            `json:"category"`
	BlockNumber *big.Int          `json:"blockNum"`
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

type errHTTP struct {
	status string
}

// getTokenContractMetadata returns the metadata for a given contract (without URI)
func getTokenContractMetadata(address address, ethClient *ethclient.Client) (*tokenContractMetadata, error) {
	contract := common.HexToAddress(string(address))
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

// getERC721TokenURI returns metadata URI for a given token address
func getERC721TokenURI(address address, tokenID tokenID, ethClient *ethclient.Client) (uri, error) {

	contract := common.HexToAddress(string(address))
	instance, err := contracts.NewIERC721MetadataCaller(contract, ethClient)
	if err != nil {
		return "", err
	}

	i, err := util.HexToBigInt(string(tokenID))
	if err != nil {
		return "", err
	}
	logrus.Debugf("Token ID: %d\tToken Address: %s", i.Uint64(), contract.Hex())

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*3)
	defer cancel()
	turi, err := instance.TokenURI(&bind.CallOpts{
		Context: ctx,
	}, i)
	if err != nil {
		return "", err
	}

	return uri(strings.ReplaceAll(turi, "\x00", "")), nil

}

// getMetadataFromURI parses and returns the NFT metadata for a given token URI
func getMetadataFromURI(turi uri, ipfsClient *shell.Shell) (metadata, error) {

	client := &http.Client{
		Timeout: time.Second * 15,
	}

	asString := string(turi)

	if strings.Contains(string(turi), "data:application/json;base64,") {
		// decode the base64 encoded json
		b64data := asString[strings.IndexByte(asString, ',')+1:]
		decoded, err := base64.StdEncoding.DecodeString(string(b64data))
		if err != nil {
			return nil, err
		}

		metadata := metadata{}
		err = json.Unmarshal(decoded, &metadata)
		if err != nil {
			return nil, err
		}

		return metadata, nil
	} else if strings.HasPrefix(asString, "ipfs://") {

		path := strings.TrimPrefix(asString, "ipfs://")
		pathMinusExtra := strings.TrimPrefix(path, "ipfs/")

		it, err := ipfsClient.Cat(pathMinusExtra)
		if err != nil {
			return nil, err
		}
		defer it.Close()

		buf := &bytes.Buffer{}
		_, err = io.Copy(buf, it)
		if err != nil {
			return nil, err
		}
		metadata := metadata{}
		err = json.Unmarshal(buf.Bytes(), &metadata)
		if err != nil {
			return nil, err
		}

		return metadata, nil
	} else if strings.HasPrefix(asString, "https://") || strings.HasPrefix(asString, "http://") {
		resp, err := client.Get(asString)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
		if resp.StatusCode > 299 {
			time.Sleep(time.Second * 10)
			resp, err = client.Get(asString)
			if err != nil {
				return nil, err
			}
			defer resp.Body.Close()
			if resp.StatusCode > 299 {
				return nil, errHTTP{status: resp.Status}
			}
		}
		buf := &bytes.Buffer{}
		_, err = io.Copy(buf, resp.Body)
		if err != nil {
			return nil, err
		}

		// parse the json
		metadata := metadata{}
		err = json.Unmarshal(buf.Bytes(), &metadata)
		if err != nil {
			return nil, err
		}

		return metadata, nil
	} else {
		return nil, nil
	}

}

// if logging all events is too large and takes too much time, start from the front and go backwards until one is found
// given that the most recent URI event should be the current URI
func getERC1155TokenURI(pContractAddress address, pTokenID tokenID, ethClient *ethclient.Client) (uri, error) {
	contract := common.HexToAddress(string(pContractAddress))
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
	if err != nil {
		return "", err
	}
	cancel()
	if turi != "" {
		return uri(strings.ReplaceAll(turi, "\x00", "")), nil
	}

	topics := [][]common.Hash{{common.HexToHash("0x6bb7ff708619ba0610cba295a58592e0451dee2622938c8755667688daf3529b")}, {common.HexToHash("0x" + padHex(string(pTokenID), 64))}}
	ctx, cancel = context.WithTimeout(context.Background(), time.Second*15)
	defer cancel()

	def := new(big.Int).SetUint64(defaultStartingBlock)

	logs, err := ethClient.FilterLogs(ctx, ethereum.FilterQuery{
		FromBlock: def,
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
	uri := uri(logs[0].Data[offset.Uint64()+32 : offset.Uint64()+32+length.Uint64()])
	return uri, nil

}

func getBalanceOfERC1155Token(pOwnerAddress, pContractAddress address, pTokenID tokenID, ethClient *ethclient.Client) (*big.Int, error) {
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
	return fmt.Sprintf("HTTP Error: %s", h.status)
}
