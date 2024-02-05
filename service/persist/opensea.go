package persist

import (
	"encoding/json"
	"fmt"
	"math/big"
	"strings"
)

type OpenseaNFTID struct {
	Chain           Chain
	ContractAddress Address
	TokenID         TokenID
}

func (o OpenseaNFTID) String() string {
	cstring := "ethereum"
	switch o.Chain {
	case ChainPolygon:
		cstring = "matic"
	case ChainArbitrum:
		cstring = "arbitrum"
	case ChainOptimism:
		cstring = "optimism"
	case ChainZora:
		cstring = "zora"
	case ChainBase:
		cstring = "base"

	}
	return fmt.Sprintf("%s/%s/%s", cstring, o.ContractAddress, o.TokenID.Base10String())
}

func (o *OpenseaNFTID) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}

	split := strings.Split(s, "/")
	if len(split) != 3 {
		return fmt.Errorf("invalid opensea nft id: %s", s)
	}

	var chain Chain
	switch split[0] {
	case "ethereum":
		chain = ChainETH
	case "matic":
		chain = ChainPolygon
	case "arbitrum":
		chain = ChainArbitrum
	case "optimism":
		chain = ChainOptimism
	case "zora":
		chain = ChainZora
	case "base":
		chain = ChainBase

	default:
		return fmt.Errorf("invalid opensea chain: %s", split[0])
	}

	o.Chain = chain
	o.ContractAddress = Address(strings.ToLower(split[1]))
	asBig, ok := big.NewInt(0).SetString(split[2], 10)
	if !ok {
		asBig, ok = big.NewInt(0).SetString(split[2], 16)
		if !ok {
			return fmt.Errorf("invalid opensea token id: %s", split[2])
		}
	}
	o.TokenID = TokenID(asBig.Text(16))

	return nil
}

func (o OpenseaNFTID) MarshalJSON() ([]byte, error) {
	return json.Marshal(o.String())
}

type OpenSeaWebhookInput struct {
	EventType string `json:"event_type"`
	Payload   struct {
		Chain      string `json:"chain"`
		Collection struct {
			Slug string `json:"slug"`
		} `json:"collection"`
		EventTimestamp string `json:"event_timestamp"`
		FromAccount    struct {
			Address EthereumAddress `json:"address"`
		} `json:"from_account"`
		Item struct {
			Chain struct {
				Name string `json:"name"`
			} `json:"chain"`
			Metadata  TokenMetadata `json:"metadata"`
			NFTID     OpenseaNFTID  `json:"nft_id"`
			Permalink string        `json:"permalink"`
		} `json:"item"`
		Quantity  int `json:"quantity"`
		ToAccount struct {
			Address EthereumAddress `json:"address"`
		} `json:"to_account"`
	} `json:"payload"`
}
