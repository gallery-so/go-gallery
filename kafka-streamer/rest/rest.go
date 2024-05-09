package rest

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/mikeydub/go-gallery/env"
	"github.com/mikeydub/go-gallery/service/logger"
	"github.com/mikeydub/go-gallery/util"
	"github.com/mikeydub/go-gallery/util/retry"
	"net/http"
	"strings"
)

type Contract struct {
	Type                   *string `json:"type"`
	Name                   *string `json:"name"`
	Symbol                 *string `json:"symbol"`
	DeployedBy             *string `json:"deployed_by"`
	DeployedViaContract    *string `json:"deployed_via_contract"`
	OwnedBy                *string `json:"owned_by"`
	HasMultipleCollections *bool   `json:"has_multiple_collections"`
}

type Collection struct {
	CollectionID                 *string             `json:"collection_id"`
	Name                         *string             `json:"name"`
	Description                  *string             `json:"description"`
	ImageUrl                     *string             `json:"image_url"`
	BannerImageUrl               *string             `json:"banner_image_url"`
	Category                     *string             `json:"category"`
	IsNsfw                       *bool               `json:"is_nsfw"`
	ExternalUrl                  *string             `json:"external_url"`
	TwitterUsername              *string             `json:"twitter_username"`
	DiscordUrl                   *string             `json:"discord_url"`
	InstagramUrl                 *string             `json:"instagram_url"`
	MediumUsername               *string             `json:"medium_username"`
	TelegramUrl                  *string             `json:"telegram_url"`
	MarketplacePages             []MarketplacePage   `json:"marketplace_pages"`
	MetaplexMint                 *string             `json:"metaplex_mint"`
	MetaplexCandyMachine         *string             `json:"metaplex_candy_machine"`
	MetaplexFirstVerifiedCreator *string             `json:"metaplex_first_verified_creator"`
	SpamScore                    *int32              `json:"spam_score"`
	Chains                       []string            `json:"chains"`
	TopContracts                 []string            `json:"top_contracts"`
	CollectionRoyalties          []CollectionRoyalty `json:"collection_royalties"`
}

type MarketplacePage struct {
	MarketplaceID           string  `json:"marketplace_id"`
	MarketplaceName         string  `json:"marketplace_name"`
	MarketplaceCollectionID string  `json:"marketplace_collection_id"`
	NFTUrl                  *string `json:"nft_url"`
	CollectionURL           string  `json:"collection_url"`
	Verified                *bool   `json:"verified"`
}

type CollectionRoyalty struct {
	Source                     string `json:"source"`
	TotalCreatorFeeBasisPoints int    `json:"total_creator_fee_basis_points"`
	Recipients                 []struct {
		Address     string  `json:"address"`
		Percentage  float64 `json:"percentage"`
		BasisPoints int
	}
}

type NFT struct {
	Chain           *string     `json:"chain"`
	ContractAddress *string     `json:"contract_address"`
	Contract        *Contract   `json:"contract"`
	Collection      *Collection `json:"collection"`
}

// Normalize makes SimpleHash addresses lowercase
func (n *NFT) Normalize() {
	n.ContractAddress = normalizeCase(n.ContractAddress)

	if n.Contract != nil {
		n.Contract.Normalize()
	}

	if n.Collection != nil {
		n.Collection.Normalize()
	}
}

// Normalize makes SimpleHash addresses lowercase
func (c *Contract) Normalize() {
	c.DeployedBy = normalizeCase(c.DeployedBy)
	c.DeployedViaContract = normalizeCase(c.DeployedViaContract)
	c.OwnedBy = normalizeCase(c.OwnedBy)
}

// Normalize makes SimpleHash addresses lowercase
func (c *Collection) Normalize() {
	for i := range c.TopContracts {
		c.TopContracts[i] = strings.ToLower(c.TopContracts[i])
	}

	for i := range c.CollectionRoyalties {
		for j := range c.CollectionRoyalties[i].Recipients {
			c.CollectionRoyalties[i].Recipients[j].Address = strings.ToLower(c.CollectionRoyalties[i].Recipients[j].Address)
		}
	}
}

func normalizeCase(s *string) *string {
	if s == nil {
		return s
	}

	return util.ToPointer(strings.ToLower(*s))
}

type nftsByTokenListResponse struct {
	NFTs []NFT `json:"nfts"`
}

type collectionsByIDListResponse struct {
	Collections []Collection `json:"collections"`
}

func GetSimpleHashNFTs(ctx context.Context, httpClient *http.Client, tokenIDs []string) ([]NFT, error) {
	if len(tokenIDs) == 0 {
		logger.For(ctx).Warnf("GetSimpleHashNFTs: no token IDs provided to get NFTs")
		return []NFT{}, nil
	}

	apiURL := "https://api.simplehash.com/api/v0/nfts/assets"

	// Add the token IDs to the URL
	url := fmt.Sprintf("%s?nft_ids=%s", apiURL, strings.Join(tokenIDs, ","))

	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("X-API-KEY", env.GetString("SIMPLEHASH_REST_API_KEY"))
	req.Header.Set("Accept", "application/json")

	resp, err := retry.RetryRequest(httpClient, req)
	if err != nil {
		return nil, err
	}

	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, util.GetErrFromResp(resp)
	}

	response := nftsByTokenListResponse{}

	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return nil, err
	}

	return response.NFTs, nil
}

func GetSimpleHashCollections(ctx context.Context, httpClient *http.Client, collectionIDs []string) ([]Collection, error) {
	if len(collectionIDs) == 0 {
		logger.For(ctx).Warnf("GetSimpleHashCollections: no collection IDs provided to get collections")
		return []Collection{}, nil
	}

	apiURL := "https://api.simplehash.com/api/v0/nfts/collections/ids"

	// Add the collection IDs to the URL
	url := fmt.Sprintf("%s?collection_ids=%s", apiURL, strings.Join(collectionIDs, ","))

	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("X-API-KEY", env.GetString("SIMPLEHASH_REST_API_KEY"))
	req.Header.Set("Accept", "application/json")

	resp, err := retry.RetryRequest(httpClient, req)
	if err != nil {
		return nil, err
	}

	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, util.GetErrFromResp(resp)
	}

	response := collectionsByIDListResponse{}

	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return nil, err
	}

	return response.Collections, nil
}
