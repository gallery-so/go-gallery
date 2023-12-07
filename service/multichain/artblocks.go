package multichain

import (
	"context"
	"encoding/json"
	"fmt"
	db "github.com/mikeydub/go-gallery/db/gen/coredb"
	"github.com/mikeydub/go-gallery/service/logger"
	op "github.com/mikeydub/go-gallery/service/multichain/operation"
	"github.com/mikeydub/go-gallery/service/persist"
	sentryutil "github.com/mikeydub/go-gallery/service/sentry"
	"github.com/mikeydub/go-gallery/util"
	"github.com/mikeydub/go-gallery/util/retry"
	"math/big"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

func (d *Provider) getArtBlocksCommunity(ctx context.Context, httpClient *http.Client, key artBlocksCommunityKey) (communityInfo, error) {
	// TODO: Use env variables for these
	apiURL := "https://token.artblocks.io"
	if key.ContractChain == persist.ChainArbitrum {
		apiURL = "https://token.arbitrum.artblocks.io"
	}

	// The API works with token IDs, not project IDs, so get the 0th token for the given project ID
	base10TokenID := projectIDToBase10ArtBlocksTokenID(key.ProjectID)

	logger.For(ctx).Infof("Requesting Art Blocks community info for contract %s, token ID %s", key.ContractAddress, base10TokenID)

	req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("%s/%s/%s", apiURL, key.ContractAddress, base10TokenID), nil)
	if err != nil {
		return nil, err
	}
	resp, err := retry.RetryRequest(httpClient, req)
	if err != nil {
		return nil, err
	}

	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, util.GetErrFromResp(resp)
	}

	info := artBlocksCommunityInfo{
		Chain:           key.ContractChain,
		ContractAddress: key.ContractAddress,
	}

	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return nil, err
	}

	return info, nil
}

type artBlocksCommunityInfo struct {
	Platform        string `json:"platform"`
	TokenID         string `json:"tokenID"`
	Artist          string `json:"artist"`
	Description     string `json:"description"`
	ProjectID       string `json:"project_id"`
	PreviewAssetURL string `json:"preview_asset_url"`

	CollectionName string `json:"collection_name"`

	RoyaltyInfo struct {
		ArtistAddress   persist.Address `json:"artistAddress"`
		AdditionalPayee persist.Address `json:"additionalPayee"`
	} `json:"royaltyInfo"`

	// Manual fields not set by the API
	Chain           persist.Chain   `json:"-"`
	ContractAddress persist.Address `json:"-"`
}

func (i artBlocksCommunityInfo) GetKey() persist.CommunityKey {
	return persist.CommunityKey{
		Type: persist.CommunityTypeArtBlocks,
		Key1: fmt.Sprintf("%d", i.Chain),
		Key2: i.ContractAddress.String(),
		Key3: i.ProjectID,
	}
}

func (i artBlocksCommunityInfo) GetName() string {
	// The ArtBlocks API always appends the artist's name to the collection name, but we'd like to
	// separate them for display purposes.
	return strings.TrimSuffix(i.CollectionName, fmt.Sprintf(" by %s", i.Artist))
}

func (i artBlocksCommunityInfo) GetDescription() string {
	return i.Description
}

func (i artBlocksCommunityInfo) GetProfileImageURL() string {
	return i.PreviewAssetURL
}

func (i artBlocksCommunityInfo) GetCreatorAddresses() []persist.ChainAddress {
	return []persist.ChainAddress{persist.NewChainAddress(i.RoyaltyInfo.ArtistAddress, i.Chain)}
}

// Art Blocks project IDs are the token ID divided by 1,000,000
var oneMillion = big.NewInt(1000000)

func tokenIDToArtBlocksProjectID(tokenID persist.TokenID) string {
	var projectIDInt big.Int
	projectIDInt.Div(tokenID.BigInt(), oneMillion)
	return projectIDInt.String()
}

// projectIDToTokenID returns the first token ID that would belong to a given project ID.
// Note: the Art Blocks API expects base 10 token IDs, so for efficiency, this helper method
// works directly with base 10 numbers and doesn't use the hex-based persist.TokenID type.
func projectIDToBase10ArtBlocksTokenID(projectID string) string {
	var projectIDInt big.Int
	projectIDInt.SetString(projectID, 10)
	var tokenIDInt big.Int
	tokenIDInt.Mul(&projectIDInt, oneMillion)
	return tokenIDInt.String()
}

func (p *Provider) getArtBlocksCommunities(ctx context.Context, keys []artBlocksCommunityKey) (found map[artBlocksCommunityKey]communityInfo, notFound []artBlocksCommunityKey) {
	httpClient := http.DefaultClient
	found = make(map[artBlocksCommunityKey]communityInfo)
	for _, key := range keys {
		c, err := p.getArtBlocksCommunity(ctx, httpClient, key)
		if err != nil {
			if httpErr, ok := util.ErrorAs[util.ErrHTTP](err); ok && httpErr.Status == http.StatusNotFound {
				notFound = append(notFound, key)
				continue
			}

			logger.For(ctx).Infof("failed to get community with key %s: %s", key, err)
			continue
		}

		found[key] = c
	}

	return found, notFound
}

// hasArtBlocksData checks various columns and metadata fields to determine whether a token likely belongs to an Art Blocks contract
func hasArtBlocksData(token op.TokenFullDetails) bool {
	isArtBlocksURL := func(str string) bool {
		parsed, err := url.Parse(str)
		if err != nil {
			return false
		}

		return strings.HasSuffix(strings.ToLower(parsed.Hostname()), ".artblocks.io")
	}

	if token.Definition.ExternalUrl.Valid && isArtBlocksURL(token.Definition.ExternalUrl.String) {
		return true
	}

	if isArtBlocksURL(token.Definition.FallbackMedia.ImageURL.String()) {
		return true
	}

	if token.Definition.Metadata == nil {
		return false
	}

	metadataIsArtBlocksURL := func(key string) bool {
		if str, ok := token.Definition.Metadata[key].(string); ok {
			return isArtBlocksURL(str)
		}
		return false
	}

	if metadataIsArtBlocksURL("image") {
		return true
	}

	if metadataIsArtBlocksURL("image_url") {
		return true
	}

	if metadataIsArtBlocksURL("animation_url") {
		return true
	}

	if metadataIsArtBlocksURL("generator_url") {
		return true
	}

	return false
}

// Group tokens by Art Blocks projectID + chain + contract address, since those uniquely identify an Art Blocks community.
type artBlocksCommunityKey struct {
	ProjectID       string
	ContractAddress persist.Address
	ContractChain   persist.Chain
}

func newArtBlocksCommunityKey(contractChain persist.Chain, contractAddress persist.Address, projectID string) artBlocksCommunityKey {
	return artBlocksCommunityKey{
		ContractChain:   contractChain,
		ContractAddress: contractAddress,
		ProjectID:       projectID,
	}
}

func (a artBlocksCommunityKey) String() string {
	return fmt.Sprintf("ProjectID=%s:Chain=%d:ContractAddress=%s", a.ProjectID, a.ContractChain, a.ContractAddress)
}

func (p *Provider) processArtBlocksTokenCommunities(ctx context.Context, knownTypes []db.ContractCommunityType, tokens []op.TokenFullDetails) error {
	artBlocksContracts := make(map[persist.DBID]bool)
	for _, t := range knownTypes {
		if t.CommunityType == persist.CommunityTypeArtBlocks {
			artBlocksContracts[t.ContractID] = t.IsValidType
		}
	}

	tokensByCommunityKey := make(map[artBlocksCommunityKey][]op.TokenFullDetails)
	contractByCommunityKey := make(map[artBlocksCommunityKey]db.Contract)

	for _, token := range tokens {
		if isValid, exists := artBlocksContracts[token.Contract.ID]; exists {
			if !isValid {
				// We may have encountered this contract previously and assumed it might be an Art Blocks contract based on a token's
				// metadata. If we checked with the Art Blocks API and discovered that the contract isn't an Art Blocks contract,
				// we can skip it here.
				continue
			}
		} else if !hasArtBlocksData(token) {
			continue
		}

		key := newArtBlocksCommunityKey(token.Contract.Chain, token.Contract.Address, tokenIDToArtBlocksProjectID(token.Definition.TokenID))
		tokensByCommunityKey[key] = append(tokensByCommunityKey[key], token)
		contractByCommunityKey[key] = token.Contract
	}

	communityTypes := make([]int32, 0, len(tokensByCommunityKey))
	communityKeys1 := make([]string, 0, len(tokensByCommunityKey))
	communityKeys2 := make([]string, 0, len(tokensByCommunityKey))
	communityKeys3 := make([]string, 0, len(tokensByCommunityKey))
	communityKeys4 := make([]string, 0, len(tokensByCommunityKey))

	for key := range tokensByCommunityKey {
		communityTypes = append(communityTypes, int32(persist.CommunityTypeArtBlocks))
		communityKeys1 = append(communityKeys1, fmt.Sprintf("%d", key.ContractChain))
		communityKeys2 = append(communityKeys2, key.ContractAddress.String())
		communityKeys3 = append(communityKeys3, key.ProjectID)
		communityKeys4 = append(communityKeys4, "")
	}

	existingCommunities, err := p.Queries.GetCommunitiesByKeys(ctx, db.GetCommunitiesByKeysParams{
		Types: communityTypes,
		Key1:  communityKeys1,
		Key2:  communityKeys2,
		Key3:  communityKeys3,
		Key4:  communityKeys4,
	})

	if err != nil {
		return err
	}

	communitiesByKey := make(map[artBlocksCommunityKey]db.Community)
	for _, c := range existingCommunities {
		chainInt, err := strconv.Atoi(c.Community.Key1)
		if err != nil {
			err = fmt.Errorf("failed to parse chain for community with ID %s: %w", c.Community.ID, err)
			logger.For(ctx).WithError(err).Error(err)
			sentryutil.ReportError(ctx, err)
		}
		key := newArtBlocksCommunityKey(persist.Chain(chainInt), persist.Address(c.Community.Key2), c.Community.Key3)
		communitiesByKey[key] = c.Community
	}

	if len(tokensByCommunityKey) != len(existingCommunities) {
		// communitiesToFetch is the set of community keys that either:
		// - DO belong to an Art Blocks contract, but we don't have a community for this project ID yet, or
		// - MAY belong to an Art Blocks contract, but we aren't sure because we haven't confirmed it with the Art Blocks API yet
		communitiesToFetch := make([]artBlocksCommunityKey, 0, len(tokensByCommunityKey)-len(existingCommunities))
		for key := range tokensByCommunityKey {
			if _, exists := communitiesByKey[key]; !exists {
				communitiesToFetch = append(communitiesToFetch, key)
			}
		}

		foundCommunities, notFoundCommunities := p.getArtBlocksCommunities(ctx, communitiesToFetch)
		communityContractTypes := make(map[persist.DBID]bool)

		if len(foundCommunities) > 0 {
			communityInfoByKey := make(map[artBlocksCommunityKey]communityInfo)
			upsertParams := db.UpsertCommunitiesParams{}
			for k, c := range foundCommunities {
				contract := contractByCommunityKey[k]
				communityContractTypes[contract.ID] = true
				communityInfoByKey[k] = c

				communityKey := c.GetKey()
				upsertParams.Ids = append(upsertParams.Ids, persist.GenerateID().String())
				upsertParams.Version = append(upsertParams.Version, 0)
				upsertParams.Name = append(upsertParams.Name, c.GetName())
				upsertParams.Description = append(upsertParams.Description, c.GetDescription())
				upsertParams.CommunityType = append(upsertParams.CommunityType, int32(communityKey.Type))
				upsertParams.Key1 = append(upsertParams.Key1, communityKey.Key1)
				upsertParams.Key2 = append(upsertParams.Key2, communityKey.Key2)
				upsertParams.Key3 = append(upsertParams.Key3, communityKey.Key3)
				upsertParams.Key4 = append(upsertParams.Key4, communityKey.Key4)
				upsertParams.ProfileImageUrl = append(upsertParams.ProfileImageUrl, c.GetProfileImageURL())
				upsertParams.ContractID = append(upsertParams.ContractID, contract.ID.String())
			}

			if persistedCommunities, err := p.Queries.UpsertCommunities(ctx, upsertParams); err != nil {
				logger.For(ctx).Errorf("failed to upsert new Art Blocks communities: %s", err)
			} else {
				creatorUpsertParams := db.UpsertCommunityCreatorsParams{}
				for _, community := range persistedCommunities {
					chainInt, err := strconv.Atoi(community.Key1)
					if err != nil {
						err = fmt.Errorf("failed to parse chain for community with ID %s: %w", community.ID, err)
						logger.For(ctx).WithError(err).Error(err)
						sentryutil.ReportError(ctx, err)
						continue
					}
					key := newArtBlocksCommunityKey(persist.Chain(chainInt), persist.Address(community.Key2), community.Key3)
					communitiesByKey[key] = community

					if info, ok := communityInfoByKey[key]; ok {
						for _, creatorAddress := range info.GetCreatorAddresses() {
							creatorUpsertParams.Ids = append(creatorUpsertParams.Ids, persist.GenerateID().String())
							creatorUpsertParams.CreatorType = append(creatorUpsertParams.CreatorType, int32(persist.CommunityCreatorTypeProvider))
							creatorUpsertParams.CommunityID = append(creatorUpsertParams.CommunityID, community.ID.String())
							creatorUpsertParams.CreatorAddress = append(creatorUpsertParams.CreatorAddress, creatorAddress.Address().String())
							creatorUpsertParams.CreatorAddressChain = append(creatorUpsertParams.CreatorAddressChain, int32(creatorAddress.Chain()))
							creatorUpsertParams.CreatorAddressL1Chain = append(creatorUpsertParams.CreatorAddressL1Chain, int32(creatorAddress.Chain().L1Chain()))
						}
					}
				}

				if len(creatorUpsertParams.Ids) > 0 {
					_, err = p.Queries.UpsertCommunityCreators(ctx, creatorUpsertParams)
					if err != nil {
						err := fmt.Errorf("failed to upsert Art Blocks community creators: %w", err)
						logger.For(ctx).WithError(err).Error(err)
						sentryutil.ReportError(ctx, err)
					}
				}
			}
		}

		// Any contracts that the Art Blocks API said it doesn't own should be flagged as invalid, so we don't check
		// them again in the future
		for _, k := range notFoundCommunities {
			communityContractTypes[contractByCommunityKey[k].ID] = false
		}

		typeUpsertParams := db.UpsertContractCommunityTypesParams{}
		for contractID, isValid := range communityContractTypes {
			typeUpsertParams.Ids = append(typeUpsertParams.Ids, persist.GenerateID().String())
			typeUpsertParams.ContractID = append(typeUpsertParams.ContractID, contractID.String())
			typeUpsertParams.CommunityType = append(typeUpsertParams.CommunityType, int32(persist.CommunityTypeArtBlocks))
			typeUpsertParams.IsValidType = append(typeUpsertParams.IsValidType, isValid)
		}

		if err := p.Queries.UpsertContractCommunityTypes(ctx, typeUpsertParams); err != nil {
			logger.For(ctx).Errorf("failed to upsert Art Blocks contract community types: %s", err)
		}
	}

	upsertParams := db.UpsertTokenCommunityMembershipsParams{}
	for communityKey, communityTokens := range tokensByCommunityKey {
		community, ok := communitiesByKey[communityKey]
		if !ok {
			// This is expected; we may have thought this contract was an Art Blocks contract, but their API said it wasn't
			logger.For(ctx).Infof("skipping UpsertTokenCommunityMemberships for Art Blocks community with key: %s", communityKey)
			continue
		}

		for _, token := range communityTokens {
			upsertParams.Ids = append(upsertParams.Ids, persist.GenerateID().String())
			upsertParams.TokenDefinitionID = append(upsertParams.TokenDefinitionID, token.Definition.ID.String())
			upsertParams.CommunityID = append(upsertParams.CommunityID, community.ID.String())
		}
	}

	if _, err = p.Queries.UpsertTokenCommunityMemberships(ctx, upsertParams); err != nil {
		logger.For(ctx).Errorf("failed to upsert Art Blocks token community memberships: %s", err)
	}

	return nil
}
