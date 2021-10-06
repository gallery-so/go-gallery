package mongodb

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/mikeydub/go-gallery/persist"
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
func NewUserMongoRepository() *UserMongoRepository {
	return &UserMongoRepository{
		mp: newStorage(0, galleryDBName, usersCollName),
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
func (u *UserMongoRepository) ExistsByAddress(pCtx context.Context, pAddress string) (bool, error) {

	countInt, err := u.mp.count(pCtx, bson.M{"addresses": bson.M{"$in": []string{strings.ToLower(pAddress)}}})

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
func (u *UserMongoRepository) GetByID(pCtx context.Context, userID persist.DBID,
) (*persist.User, error) {

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

	if len(result) == 0 {
		return nil, fmt.Errorf("no users found")
	}
	if len(result) > 1 {
		return nil, fmt.Errorf("more than one user found when expecting a single result")
	}

	return result[0], nil
}

// GetByAddress returns a user by a given wallet address
func (u *UserMongoRepository) GetByAddress(pCtx context.Context, pAddress string,
) (*persist.User, error) {

	opts := options.Find()
	if deadline, ok := pCtx.Deadline(); ok {
		dur := time.Until(deadline)
		opts.SetMaxTime(dur)
	}

	result := []*persist.User{}
	err := u.mp.find(pCtx, bson.M{"addresses": bson.M{"$in": []string{strings.ToLower(pAddress)}}}, &result, opts)

	if err != nil {
		return nil, err
	}

	if len(result) == 0 {
		return nil, fmt.Errorf("no users found")
	}
	if len(result) > 1 {
		return nil, fmt.Errorf("more than one user found when expecting a single result")
	}

	return result[0], nil
}

// GetByUsername returns a user by a given username (case insensitive)
func (u *UserMongoRepository) GetByUsername(pCtx context.Context, pUsername string,
) (*persist.User, error) {

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

	if len(result) == 0 {
		return nil, fmt.Errorf("no users found")
	}
	if len(result) > 1 {
		return nil, fmt.Errorf("more than one user found when expecting a single result")
	}

	return result[0], nil
}

// AddAddresses pushes addresses into a user's address list
func (u *UserMongoRepository) AddAddresses(pCtx context.Context, pUserID persist.DBID, pAddresses []string) error {

	for i, addr := range pAddresses {
		pAddresses[i] = strings.ToLower(addr)
	}

	return u.mp.push(pCtx, bson.M{"_id": pUserID}, "addresses", pAddresses)
}

// RemoveAddresses removes addresses from a user's address list
func (u *UserMongoRepository) RemoveAddresses(pCtx context.Context, pUserID persist.DBID, pAddresses []string) error {
	for i, addr := range pAddresses {
		pAddresses[i] = strings.ToLower(addr)
	}

	return u.mp.pullAll(pCtx, bson.M{"_id": pUserID}, "addresses", pAddresses)
}
