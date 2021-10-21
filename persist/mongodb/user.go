package mongodb

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/mikeydub/go-gallery/persist"
	"github.com/sirupsen/logrus"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const usersCollName = "users"

// UserMongoRepository is a repository that stores collections in a MongoDB database
type UserMongoRepository struct {
	mp *storage
}

// NewUserMongoRepository creates a new instance of the collection mongo repository
func NewUserMongoRepository(mgoClient *mongo.Client) *UserMongoRepository {
	b := true
	mgoClient.Database(galleryDBName).Collection(usersCollName).Indexes().CreateOne(context.Background(), mongo.IndexModel{
		Keys: bson.M{"username_idempotent": 1},
		Options: &options.IndexOptions{
			Unique: &b,
			Sparse: &b,
		},
	})
	return &UserMongoRepository{
		mp: newStorage(mgoClient, 0, galleryDBName, usersCollName),
	}
}

// UpdateByID updates a user by ID
// pUpdate represents a struct with bson tags to specify which fields to update
func (u *UserMongoRepository) UpdateByID(pCtx context.Context, pID persist.DBID, pUpdate interface{},
) error {

	err := u.mp.update(pCtx, bson.M{"_id": pID}, pUpdate)
	if err != nil {
		if mongo.IsDuplicateKeyError(err) {
			return fmt.Errorf("attempt to update username to a taken username")
		}
		return err
	}

	return nil
}

// ExistsByAddress returns true if a user exists with the given address
func (u *UserMongoRepository) ExistsByAddress(pCtx context.Context, pAddress persist.Address) (bool, error) {

	countInt, err := u.mp.count(pCtx, bson.M{"addresses": bson.M{"$in": []persist.Address{pAddress}}})

	if err != nil {
		return false, err
	}

	return countInt > 0, nil
}

// Create inserts a user into the database
func (u *UserMongoRepository) Create(pCtx context.Context, pUser *persist.User) (persist.DBID, error) {
	return u.mp.insert(pCtx, pUser)
}

// Delete marks a user as deleted in the database
func (u *UserMongoRepository) Delete(pCtx context.Context, pUserID persist.DBID,
) error {
	return u.mp.update(pCtx, bson.M{"_id": pUserID}, bson.M{"$set": bson.M{"deleted": true}})
}

// GetByID returns a user by a given ID
func (u *UserMongoRepository) GetByID(pCtx context.Context, userID persist.DBID) (*persist.User, error) {

	opts := options.Find()
	if deadline, ok := pCtx.Deadline(); ok {
		dur := time.Until(deadline)
		opts.SetMaxTime(dur)
	}

	result := []*persist.User{}
	err := u.mp.find(pCtx, bson.M{"_id": userID}, &result, opts)

	if err != nil {
		return nil, err
	}

	if len(result) != 1 {
		return nil, persist.ErrUserNotFoundByID{ID: userID}
	}

	return result[0], nil
}

// GetByAddress returns a user by a given wallet address
func (u *UserMongoRepository) GetByAddress(pCtx context.Context, pAddress persist.Address) (*persist.User, error) {

	opts := options.Find()
	if deadline, ok := pCtx.Deadline(); ok {
		dur := time.Until(deadline)
		opts.SetMaxTime(dur)
	}

	result := []*persist.User{}
	err := u.mp.find(pCtx, bson.M{"addresses": bson.M{"$in": []persist.Address{pAddress.Lower()}}}, &result, opts)

	if err != nil {
		return nil, err
	}

	if len(result) != 1 {
		return nil, persist.ErrUserNotFoundByAddress{Address: pAddress}
	}

	if len(result) > 1 {
		logrus.Errorf("found more than one user for address: %s", pAddress)
	}

	return result[0], nil
}

// GetByUsername returns a user by a given username (case insensitive)
func (u *UserMongoRepository) GetByUsername(pCtx context.Context, pUsername string) (*persist.User, error) {

	opts := options.Find()
	if deadline, ok := pCtx.Deadline(); ok {
		dur := time.Until(deadline)
		opts.SetMaxTime(dur)
	}

	result := []*persist.User{}
	err := u.mp.find(pCtx, bson.M{"username_idempotent": strings.ToLower(pUsername)}, &result, opts)

	if err != nil {
		return nil, err
	}

	if len(result) < 1 {
		return nil, persist.ErrUserNotFoundByUsername{Username: pUsername}
	}

	if len(result) > 1 {
		logrus.Errorf("found more than one user for username: %s", pUsername)
	}

	return result[0], nil
}

// AddAddresses pushes addresses into a user's address list
func (u *UserMongoRepository) AddAddresses(pCtx context.Context, pUserID persist.DBID, pAddresses []persist.Address) error {

	for i, address := range pAddresses {
		pAddresses[i] = address.Lower()
	}
	return u.mp.push(pCtx, bson.M{"_id": pUserID}, "addresses", pAddresses)
}

// RemoveAddresses removes addresses from a user's address list
func (u *UserMongoRepository) RemoveAddresses(pCtx context.Context, pUserID persist.DBID, pAddresses []persist.Address) error {
	for i, address := range pAddresses {
		pAddresses[i] = address.Lower()
	}
	return u.mp.pullAll(pCtx, bson.M{"_id": pUserID}, "addresses", pAddresses)
}
