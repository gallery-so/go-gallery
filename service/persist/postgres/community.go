package postgres

import (
	"context"
	db "github.com/mikeydub/go-gallery/db/gen/coredb"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/util"
)

// CommunityRepository represents a community repository in the postgres database
type CommunityRepository struct {
	queries *db.Queries
}

// NewCommunityRepository creates a new postgres repository for interacting with communities
func NewCommunityRepository(queries *db.Queries) *CommunityRepository {
	return &CommunityRepository{queries: queries}
}

// UpsertCommunities upserts a list of communities
func (c *CommunityRepository) UpsertCommunities(ctx context.Context, communities []db.Community) ([]db.Community, error) {
	if len(communities) == 0 {
		return []db.Community{}, nil
	}

	communities = util.DedupeWithTranslate(communities, false, func(c db.Community) persist.CommunityKey {
		return persist.CommunityKey{
			Type: c.CommunityType,
			Key1: c.Key1,
			Key2: c.Key2,
			Key3: c.Key3,
			Key4: c.Key4,
		}
	})

	params := db.UpsertCommunitiesParams{}
	for i := range communities {
		c := &communities[i]
		params.Ids = append(params.Ids, persist.GenerateID().String())
		params.Version = append(params.Version, c.Version)
		params.CommunityType = append(params.CommunityType, int32(c.CommunityType))
		params.Key1 = append(params.Key1, c.Key1)
		params.Key2 = append(params.Key2, c.Key2)
		params.Key3 = append(params.Key3, c.Key3)
		params.Key4 = append(params.Key4, c.Key4)
		params.Name = append(params.Name, c.Name)
		params.Description = append(params.Description, c.Description)
		params.ProfileImageUrl = append(params.ProfileImageUrl, c.ProfileImageUrl.String)
		params.ContractID = append(params.ContractID, c.ContractID.String())
	}

	return c.queries.UpsertCommunities(ctx, params)
}
