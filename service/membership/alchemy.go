package membership

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"cloud.google.com/go/storage"
	"github.com/everFinance/goar"
	shell "github.com/ipfs/go-ipfs-api"
	"github.com/mikeydub/go-gallery/env"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/util"
	"github.com/sirupsen/logrus"
)

type alchemyNFTMetadata struct {
	Name  string `json:"name"`
	Image string `json:"image"`
}

type indexerTokenResponse struct {
	NFTs []persist.Token `json:"nfts"`
}

func getOwnersForToken(ctx context.Context, tid persist.HexTokenID, contractAddress persist.EthereumAddress) ([]persist.EthereumAddress, error) {
	url := fmt.Sprintf("%s/nfts/get?token_id=%s&contract_address=%s", env.GetString("INDEXER_HOST"), tid, contractAddress)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var response indexerTokenResponse
	err = json.NewDecoder(resp.Body).Decode(&response)
	if err != nil {
		return nil, err
	}

	owners := make([]persist.EthereumAddress, 0, len(response.NFTs))
	for _, nft := range response.NFTs {
		if nft.OwnerAddress == "" {
			continue
		}
		owners = append(owners, nft.OwnerAddress)
	}
	return owners, nil
}

func getTokenMetadata(ctx context.Context, tid persist.HexTokenID, contractAddress persist.EthereumAddress, ipfsClient *shell.Shell, arweaveClient *goar.Client, stg *storage.Client) (alchemyNFTMetadata, error) {
	url := fmt.Sprintf("%s/nfts/get?token_id=%s&contract_address=%s&limit=1", env.GetString("INDEXER_HOST"), tid, contractAddress)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return alchemyNFTMetadata{}, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return alchemyNFTMetadata{}, err
	}
	defer resp.Body.Close()

	var response indexerTokenResponse
	err = json.NewDecoder(resp.Body).Decode(&response)
	if err != nil {
		return alchemyNFTMetadata{}, err
	}

	if len(response.NFTs) == 0 {
		return alchemyNFTMetadata{}, fmt.Errorf("no nft found for token %s", tid)
	}

	token := response.NFTs[0]

	val, _ := token.TokenMetadata.Value()
	logrus.Infof("%s", val)
	name, ok := token.TokenMetadata["name"].(string)
	if !ok {
		name = ""
	}

	it, _ := util.FindFirstFieldFromMap(token.TokenMetadata, "image", "image_url", "thumbnail", "thumbnail_url").(string)
	return alchemyNFTMetadata{
		Name:  name,
		Image: it,
	}, nil

}
