package membership

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"cloud.google.com/go/storage"
	"github.com/everFinance/goar"
	shell "github.com/ipfs/go-ipfs-api"
	"github.com/mikeydub/go-gallery/service/media"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/service/rpc"
	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
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

func getOwnersForToken(ctx context.Context, tid persist.TokenID, contractAddress persist.Address) ([]persist.Address, error) {
	alchemyURL := viper.GetString("CONTRACT_INTERACTION_URL") + "/getOwnersForToken"

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("%s?contractAddress=%s&tokenId=%s", alchemyURL, contractAddress, tid), nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var response alchemyGetOwnersForTokensResponse
	err = json.NewDecoder(resp.Body).Decode(&response)
	if err != nil {
		return nil, err
	}

	return response.Owners, nil
}

func getTokenMetadata(ctx context.Context, tid persist.TokenID, contractAddress persist.Address, ipfsClient *shell.Shell, arweaveClient *goar.Client, stg *storage.Client) (alchemyNFTMetadata, error) {
	alchemyURL := viper.GetString("CONTRACT_INTERACTION_URL") + "/getNFTMetadata"

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("%s?contractAddress=%s&tokenId=%s", alchemyURL, contractAddress, tid), nil)
	if err != nil {
		return alchemyNFTMetadata{}, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return alchemyNFTMetadata{}, err
	}
	defer resp.Body.Close()

	var response alchemyGetNFTMetadataResponse
	err = json.NewDecoder(resp.Body).Decode(&response)
	if err != nil {
		return alchemyNFTMetadata{}, err
	}

	asURI := persist.TokenURI(response.Metadata.Image)
	asURI = asURI.ReplaceID(tid)

	response.Metadata.Image = asURI.String()

	switch asURI.Type() {
	case persist.URITypeArweave, persist.URITypeIPFS:
		md, err := rpc.GetMetadataFromURI(ctx, asURI, ipfsClient, arweaveClient)
		if err != nil {
			logrus.WithError(err).Error("Failed to get metadata from URI")
			return response.Metadata, nil
		}
		med, err := media.MakePreviewsForMetadata(ctx, md, contractAddress, tid, asURI, ipfsClient, arweaveClient, stg)
		if err != nil {
			logrus.WithError(err).Error("Failed to make previews")
			return response.Metadata, nil
		}
		response.Metadata.Image = med.MediaURL.String()
	}

	return response.Metadata, nil
}
