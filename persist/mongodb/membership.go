package mongodb

import (
	"context"
	"time"

	"github.com/mikeydub/go-gallery/persist"
	"github.com/sirupsen/logrus"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const membershipColName = "membership"

// MembershipRepository is a repository for storing membership information in the database
type MembershipRepository struct {
	membershipsStorage *storage
}

// NewMembershipMongoRepository returns a new instance of a membership repository
func NewMembershipMongoRepository(mgoClient *mongo.Client) *MembershipRepository {
	return &MembershipRepository{
		membershipsStorage: newStorage(mgoClient, 0, galleryDBName, membershipColName),
	}
}

// UpsertByTokenID upserts an membership tier by a given token ID
func (c *MembershipRepository) UpsertByTokenID(pCtx context.Context, pTokenID persist.TokenID, pUpsert *persist.MembershipTier) error {

	_, err := c.membershipsStorage.upsert(pCtx, bson.M{
		"token_id": pTokenID,
	}, pUpsert)
	if err != nil {
		return err
	}

	return nil
}

// GetByTokenID returns a membership tier by token ID
func (c *MembershipRepository) GetByTokenID(pCtx context.Context, pTokenID persist.TokenID) (*persist.MembershipTier, error) {

	opts := options.Find()
	if deadline, ok := pCtx.Deadline(); ok {
		dur := time.Until(deadline)
		opts.SetMaxTime(dur)
	}

	result := []*persist.MembershipTier{}
	err := c.membershipsStorage.find(pCtx, bson.M{"token_id": pTokenID}, &result, opts)

	if err != nil {
		return nil, err
	}

	if len(result) < 1 {
		return nil, persist.ErrMembershipNotFoundByTokenID{TokenID: pTokenID}
	}

	if len(result) > 1 {
		logrus.Errorf("found more than one membership tier for token ID: %s", pTokenID)
	}

	return result[0], nil
}

// GetAll returns all membership tiers
func (c *MembershipRepository) GetAll(pCtx context.Context) ([]*persist.MembershipTier, error) {

	opts := options.Find()
	if deadline, ok := pCtx.Deadline(); ok {
		dur := time.Until(deadline)
		opts.SetMaxTime(dur)
	}

	result := []*persist.MembershipTier{}
	err := c.membershipsStorage.find(pCtx, bson.M{}, &result, opts)

	if err != nil {
		return nil, err
	}

	return result, nil
}
