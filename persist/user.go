package persist

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/mikeydub/go-gallery/runtime"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const usersCollName = "users"

// User represents a user in the datase and throughout the application
type User struct {
	Version      int64   `bson:"version"` // schema version for this model
	ID           DBID    `bson:"_id"           json:"id" binding:"required"`
	CreationTime float64 `bson:"creation_time" json:"creation_time"`
	Deleted      bool    `bson:"deleted"`

	UserName           string   `bson:"username"         json:"username"` // mutable
	UserNameIdempotent string   `bson:"username_idempotent" json:"username_idempotent"`
	Addresses          []string `bson:"addresses"     json:"addresses"` // IMPORTANT!! - users can have multiple addresses associated with their account
	Bio                string   `bson:"bio"  json:"bio"`
}

// UserUpdateInput represents the data to be updated when updating a user
type UserUpdateInput struct {
	UserName           string   `bson:"username"`
	UserNameIdempotent string   `bson:"username_idempotent"`
	Addresses          []string `bson:"addresses"`
	Bio                string   `bson:"bio"`
}

// UserUpdateByID updates a user by ID
// pUpdate represents a struct with bson tags to specify which fields to update
func UserUpdateByID(pCtx context.Context, pID DBID, pUpdate interface{},
	pRuntime *runtime.Runtime) error {

	mp := NewMongoStorage(0, usersCollName, pRuntime)

	err := mp.Update(pCtx, bson.M{
		"_id": pID,
	}, pUpdate)
	if err != nil {
		if mongo.IsDuplicateKeyError(err) {
			return fmt.Errorf("attempt to update username to a taken username")
		}
		return err
	}

	return nil
}

// UserExistsByAddress returns true if a user exists with the given address
func UserExistsByAddress(pCtx context.Context, pAddress string,
	pRuntime *runtime.Runtime) (bool, error) {

	mp := NewMongoStorage(0, usersCollName, pRuntime)

	countInt, err := mp.Count(pCtx, bson.M{"addresses": bson.M{"$in": []string{pAddress}}})

	if err != nil {
		return false, err
	}

	return countInt > 0, nil
}

// UserCreate inserts a user into the database
func UserCreate(pCtx context.Context, pUser *User,
	pRuntime *runtime.Runtime) (DBID, error) {

	mp := NewMongoStorage(0, usersCollName, pRuntime)

	return mp.Insert(pCtx, pUser)

}

// UserDelete marks a user as deleted in the database
func UserDelete(pCtx context.Context, pUserID DBID,
	pRuntime *runtime.Runtime) error {

	mp := NewMongoStorage(0, usersCollName, pRuntime)

	return mp.Update(pCtx, bson.M{"_id": pUserID}, bson.M{"$set": bson.M{"deleted": true}})

}

// UserGetByID returns a user by a given ID
func UserGetByID(pCtx context.Context, userID DBID,
	pRuntime *runtime.Runtime) (*User, error) {

	opts := options.Find()
	if deadline, ok := pCtx.Deadline(); ok {
		dur := time.Until(deadline)
		opts.SetMaxTime(dur)
	}

	mp := NewMongoStorage(0, usersCollName, pRuntime)

	result := []*User{}
	err := mp.Find(pCtx, bson.M{"_id": userID}, &result, opts)

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

// UserGetByAddress returns a user by a given wallet address
func UserGetByAddress(pCtx context.Context, pAddress string,
	pRuntime *runtime.Runtime) (*User, error) {

	opts := options.Find()
	if deadline, ok := pCtx.Deadline(); ok {
		dur := time.Until(deadline)
		opts.SetMaxTime(dur)
	}

	mp := NewMongoStorage(0, usersCollName, pRuntime)

	result := []*User{}
	err := mp.Find(pCtx, bson.M{"addresses": bson.M{"$in": []string{pAddress}}}, &result, opts)

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

// UserGetByUsername returns a user by a given username (case insensitive)
func UserGetByUsername(pCtx context.Context, pUsername string,
	pRuntime *runtime.Runtime) (*User, error) {

	opts := options.Find()
	if deadline, ok := pCtx.Deadline(); ok {
		dur := time.Until(deadline)
		opts.SetMaxTime(dur)
	}

	mp := NewMongoStorage(0, usersCollName, pRuntime)

	result := []*User{}
	err := mp.Find(pCtx, bson.M{"username_idempotent": strings.ToLower(pUsername)}, &result, opts)

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
