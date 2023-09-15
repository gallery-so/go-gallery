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

	type communityKey struct {
		Type    persist.CommunityType
		Subtype string
		Key     string
	}

	communities = util.DedupeByKey(communities, false, func(c db.Community) communityKey {
		return communityKey{
			Type:    c.CommunityType,
			Subtype: c.CommunitySubtype,
			Key:     c.CommunityKey,
		}
	})

	params := db.UpsertCommunitiesParams{}
	for i := range communities {
		c := &communities[i]
		params.Ids = append(params.Ids, persist.GenerateID().String())
		params.Version = append(params.Version, c.Version)
		params.CommunityType = append(params.CommunityType, int32(c.CommunityType))
		params.CommunitySubtype = append(params.CommunitySubtype, c.CommunitySubtype)
		params.CommunityKey = append(params.CommunityKey, c.CommunityKey)
		params.Name = append(params.Name, c.Name)
		params.Description = append(params.Description, c.Description)
	}

	return c.queries.UpsertCommunities(ctx, params)
}
