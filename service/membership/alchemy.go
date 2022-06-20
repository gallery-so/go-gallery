package membership

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"cloud.google.com/go/storage"
	"github.com/everFinance/goar"
	shell "github.com/ipfs/go-ipfs-api"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/sirupsen/logrus"
)

type alchemyGetOwnersForTokensResponse struct {
	Owners []persist.Address `json:"owners"`
}

type alchemyGetNFTMetadataResponse struct {
	Metadata alchemyNFTMetadata `json:"metadata"`
}

type alchemyNFTMetadata struct {
	Name  string `json:"name"`
	Image string `json:"image"`
}

type indexerTokenResponse struct {
	NFTs []persist.Token `json:"nfts"`
}

// func getOwnersForToken(ctx context.Context, tid persist.TokenID, contractAddress persist.Address) ([]persist.Address, error) {
// 	alchemyURL := viper.GetString("CONTRACT_INTERACTION_URL") + "/getOwnersForToken"

// 	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("%s?contractAddress=%s&tokenId=%s", alchemyURL, contractAddress, fmt.Sprintf("0x0%s", tid)), nil)
// 	if err != nil {
// 		return nil, err
// 	}
// 	resp, err := http.DefaultClient.Do(req)
// 	if err != nil {
// 		return nil, err
// 	}
// 	defer resp.Body.Close()

// 	var response alchemyGetOwnersForTokensResponse
// 	err = json.NewDecoder(resp.Body).Decode(&response)
// 	if err != nil {
// 		return nil, err
// 	}

// 	return response.Owners, nil
// }

// func getTokenMetadata(ctx context.Context, tid persist.TokenID, contractAddress persist.Address, ipfsClient *shell.Shell, arweaveClient *goar.Client, stg *storage.Client) (alchemyNFTMetadata, error) {
// 	alchemyURL := viper.GetString("CONTRACT_INTERACTION_URL") + "/getNFTMetadata"

// 	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("%s?contractAddress=%s&tokenId=%s", alchemyURL, contractAddress, tid), nil)
// 	if err != nil {
// 		return alchemyNFTMetadata{}, err
// 	}
// 	resp, err := http.DefaultClient.Do(req)
// 	if err != nil {
// 		return alchemyNFTMetadata{}, err
// 	}
// 	defer resp.Body.Close()

// 	var response alchemyGetNFTMetadataResponse
// 	err = json.NewDecoder(resp.Body).Decode(&response)
// 	if err != nil {
// 		return alchemyNFTMetadata{}, err
// 	}

// 	asURI := persist.TokenURI(response.Metadata.Image)
// 	asURI = asURI.ReplaceID(tid)

// 	response.Metadata.Image = asURI.String()

// 	switch asURI.Type() {
// 	case persist.URITypeArweave, persist.URITypeIPFS:
// 		md, err := rpc.GetMetadataFromURI(ctx, asURI, ipfsClient, arweaveClient)
// 		if err != nil {
// 			logger.For(ctx).WithError(err).Error("Failed to get metadata from URI")
// 			return response.Metadata, nil
// 		}
// 		med, err := media.MakePreviewsForMetadata(ctx, md, contractAddress, tid, asURI, ipfsClient, arweaveClient, stg)
// 		if err != nil {
// 			logger.For(ctx).WithError(err).Error("Failed to make previews")
// 			return response.Metadata, nil
// 		}
// 		response.Metadata.Image = med.MediaURL.String()
// 	}

// 	return response.Metadata, nil
// }

func getOwnersForToken(ctx context.Context, tid persist.TokenID, contractAddress persist.EthereumAddress) ([]persist.EthereumAddress, error) {
	url := fmt.Sprintf("https://indexer-dot-gallery-prod-325303.wl.r.appspot.com/nfts/get?token_id=%s&contract_address=%s", tid, contractAddress)

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

func getTokenMetadata(ctx context.Context, tid persist.TokenID, contractAddress persist.EthereumAddress, ipfsClient *shell.Shell, arweaveClient *goar.Client, stg *storage.Client) (alchemyNFTMetadata, error) {
	url := fmt.Sprintf("https://indexer-dot-gallery-prod-325303.wl.r.appspot.com/nfts/get?token_id=%s&contract_address=%s&limit=1", tid, contractAddress)

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
	return alchemyNFTMetadata{
		Name:  name,
		Image: token.Media.ThumbnailURL.String(),
	}, nil

}
