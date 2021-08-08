package persist

import (
	"context"
	"fmt"
	"time"

	"github.com/mikeydub/go-gallery/runtime"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const usersCollName = "users"

type User struct {
	VersionInt    int64   `bson:"version"` // schema version for this model
	IDstr         DbId    `bson:"_id,omitempty"           json:"id" binding:"required"`
	CreationTimeF float64 `bson:"creation_time" json:"creation_time"`
	DeletedBool   bool    `bson:"deleted"`

	UserNameStr  string   `bson:"name"         json:"name"`       // mutable
	AddressesLst []string `bson:"addresses"     json:"addresses"` // IMPORTANT!! - users can have multiple addresses associated with their account
	BioStr       string   `bson:"bio"  json:"bio"`
}

type UserUpdateInput struct {
	UserNameStr  string   `bson:"name,omitempty"`
	AddressesLst []string `bson:"addresses,omitempty"`
	BioStr       string   `bson:"bio,omitempty"`
}

//-------------------------------------------------------------
// USER
//-------------------------------------------------------------
// UPDATE
func UserUpdateById(pIDstr DbId, pUser interface{},
	pCtx context.Context,
	pRuntime *runtime.Runtime) error {

	mp := NewMongoStorage(0, usersCollName, pRuntime)

	opts := options.Update()
	opts.SetCollation(&options.Collation{Locale: "en", Strength: 2})

	err := mp.Update(pCtx, bson.M{
		"_id": pIDstr,
	}, pUser, opts)
	if err != nil {
		if mongo.IsDuplicateKeyError(err) {
			return fmt.Errorf("attempt to update username to a taken username")
		}
		return err
	}

	return nil
}

//-------------------------------------------------------------
// EXISTS BY ADDRESS
func UserExistsByAddress(pAddress string,
	pCtx context.Context,
	pRuntime *runtime.Runtime) (bool, error) {

	mp := NewMongoStorage(0, usersCollName, pRuntime)

	countInt, err := mp.Count(pCtx, bson.M{"addresses": bson.M{"$in": []string{pAddress}}})

	if err != nil {
		return false, err
	}

	return countInt > 0, nil
}

//-------------------------------------------------------------
// CREATE
func UserCreate(pUser *User,
	pCtx context.Context,
	pRuntime *runtime.Runtime) (DbId, error) {

	mp := NewMongoStorage(0, usersCollName, pRuntime)

	return mp.Insert(pCtx, pUser)

}

//-------------------------------------------------------------
// DELETE
func UserDelete(pUserID DbId,
	pCtx context.Context,
	pRuntime *runtime.Runtime) error {

	mp := NewMongoStorage(0, usersCollName, pRuntime)

	return mp.Update(pCtx, bson.M{"_id": pUserID}, bson.M{"$set": bson.M{"deleted": true}})

}

//-------------------------------------------------------------
// GET BY ID
func UserGetById(userId DbId,
	pCtx context.Context,
	pRuntime *runtime.Runtime) (*User, error) {

	opts := options.Find()
	if deadline, ok := pCtx.Deadline(); ok {
		dur := time.Until(deadline)
		opts.SetMaxTime(dur)
	}

	mp := NewMongoStorage(0, usersCollName, pRuntime)

	result := []*User{}
	err := mp.Find(pCtx, bson.M{"_id": userId}, &result, opts)

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

//-------------------------------------------------------------
// GET BY ADDRESS
func UserGetByAddress(pAddress string,
	pCtx context.Context,
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

//-------------------------------------------------------------
// GET BY USERNAME
func UserGetByUsername(pUsername string,
	pCtx context.Context,
	pRuntime *runtime.Runtime) (*User, error) {

	opts := options.Find()
	if deadline, ok := pCtx.Deadline(); ok {
		dur := time.Until(deadline)
		opts.SetMaxTime(dur)
	}

	mp := NewMongoStorage(0, usersCollName, pRuntime)

	result := []*User{}
	err := mp.Find(pCtx, bson.M{"username": pUsername}, &result, opts)

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
