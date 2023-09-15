package multichain

import (
	"context"
	"encoding/json"
	"fmt"
	db "github.com/mikeydub/go-gallery/db/gen/coredb"
	"github.com/mikeydub/go-gallery/service/logger"
	"github.com/mikeydub/go-gallery/service/persist"
	sentryutil "github.com/mikeydub/go-gallery/service/sentry"
	"github.com/mikeydub/go-gallery/util"
	"github.com/mikeydub/go-gallery/util/retry"
	"math/big"
	"net/http"
)

func (d *Provider) getProhibitionCommunity(ctx context.Context, httpClient *http.Client, projectID string) (communityInfo, error) {
	apiURL := "https://prohibition.art/api/project/chaindata/0x47a91457a3a1f700097199fd63c039c4784384ab"
	req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("%s/%s/project-onchain", apiURL, projectID), nil)
	if err != nil {
		return communityInfo{}, err
	}
	resp, err := retry.RetryRequest(httpClient, req)
	if err != nil {
		return communityInfo{}, err
	}

	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return communityInfo{}, util.GetErrFromResp(resp)
	}

	var info prohibitionCommunityInfo
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return communityInfo{}, err
	}

	ci := communityInfo{
		Name:        info.ProjectName,
		Description: info.Description,
		Type:        persist.CommunityTypeProhibition,
		Subtype:     "",
		Key:         projectID,
	}

	return ci, nil
}

type prohibitionCommunityInfo struct {
	ProjectName string `json:"projectName"`
	Artist      string `json:"artist"`
	Description string `json:"description"`
	Website     string `json:"website"`
}

func (p *Provider) getProhibitionCommunities(ctx context.Context, projectIDs []string) map[string]communityInfo {
	httpClient := http.DefaultClient
	communities := make(map[string]communityInfo)
	for _, projectID := range projectIDs {
		c, err := p.getProhibitionCommunity(ctx, httpClient, projectID)
		if err != nil {
			// TODO: Return failures so we can add them to a status table
			logger.For(ctx).Infof("failed to get community for project %s: %s", projectID, err)
			continue
		}

		communities[projectID] = c
	}

	return communities
}

func (p *Provider) processProhibitionTokenCommunities(ctx context.Context, contracts []persist.ContractGallery, tokens []persist.TokenGallery) error {
	// TODO: We could shortcut this on prod by hardcoding the Prohibition contract ID
	const prohibitionContractAddress = "0x47a91457a3a1f700097199fd63c039c4784384ab"
	var prohibitionContractID persist.DBID
	found := false
	for _, contract := range contracts {
		if contract.Chain == persist.ChainArbitrum && contract.Address == prohibitionContractAddress {
			prohibitionContractID = contract.ID
			found = true
			break
		}
	}

	if !found {
		err := fmt.Errorf("could not find Prohibition contract")
		sentryutil.ReportError(ctx, err)
		return err
	}

	// Prohibition project IDs are the token ID divided by 1,000,000
	oneMillion := big.NewInt(1000000)
	tokensByProjectID := make(map[string][]persist.TokenGallery)

	for _, token := range tokens {
		if token.Contract != prohibitionContractID {
			continue
		}

		var projectIDInt big.Int
		projectIDInt.Div(token.TokenID.BigInt(), oneMillion)
		projectID := projectIDInt.String()
		tokensByProjectID[projectID] = append(tokensByProjectID[projectID], token)
	}

	communityKeys := util.MapKeys(tokensByProjectID)
	communityTypes := util.FillSliceWithValue(make([]int32, len(communityKeys)), int32(persist.CommunityTypeProhibition))
	// Prohibition communities don't have subtypes
	communitySubtypes := util.FillSliceWithValue(make([]string, len(communityKeys)), "")

	// TODO: Abstract this pattern into a helper function. This will be a common pattern among community providers:
	// look up communities in the database, query for and upsert any communities we don't have in the database,
	// and then merge the results
	existingCommunities, err := p.Queries.GetCommunitiesByKeys(ctx, db.GetCommunitiesByKeysParams{
		Types:    communityTypes,
		Subtypes: communitySubtypes,
		Keys:     communityKeys,
	})

	if err != nil {
		return err
	}

	communitiesByProjectID := make(map[string]db.Community)
	for _, c := range existingCommunities {
		communitiesByProjectID[c.CommunityKey] = c
	}

	if len(communityKeys) != len(existingCommunities) {
		communitiesToFetch := make([]string, 0, len(communityKeys)-len(existingCommunities))
		for _, key := range communityKeys {
			if _, exists := communitiesByProjectID[key]; !exists {
				communitiesToFetch = append(communitiesToFetch, key)
			}
		}

		communities := p.getProhibitionCommunities(ctx, communitiesToFetch)
		if len(communities) > 0 {
			upsertParams := db.UpsertCommunitiesParams{}
			for _, c := range communities {
				upsertParams.Ids = append(upsertParams.Ids, persist.GenerateID().String())
				upsertParams.Version = append(upsertParams.Version, 0)
				upsertParams.Name = append(upsertParams.Name, c.Name)
				upsertParams.Description = append(upsertParams.Description, c.Description)
				upsertParams.CommunityType = append(upsertParams.CommunityType, int32(c.Type))
				upsertParams.CommunitySubtype = append(upsertParams.CommunitySubtype, c.Subtype)
				upsertParams.CommunityKey = append(upsertParams.CommunityKey, c.Key)
			}
			if persistedCommunities, err := p.Queries.UpsertCommunities(ctx, upsertParams); err != nil {
				logger.For(ctx).Errorf("failed to upsert new Prohibition communities: %s", err)
			} else {
				for _, c := range persistedCommunities {
					communitiesByProjectID[c.CommunityKey] = c
				}
			}
		}
	}

	upsertParams := db.UpsertTokenCommunityMembershipsParams{}
	for projectID, projectTokens := range tokensByProjectID {
		community, ok := communitiesByProjectID[projectID]
		if !ok {
			logger.For(ctx).Warnf("could not find community for Prohibition projectID=%s", projectID)
			continue
		}

		for _, token := range projectTokens {
			upsertParams.Ids = append(upsertParams.Ids, persist.GenerateID().String())
			upsertParams.TokenID = append(upsertParams.TokenID, token.ID.String())
			upsertParams.CommunityID = append(upsertParams.CommunityID, community.ID.String())
		}
	}

	if _, err = p.Queries.UpsertTokenCommunityMemberships(ctx, upsertParams); err != nil {
		logger.For(ctx).Errorf("failed to upsert Prohibition token community memberships: %s", err)
	}

	return nil
}
