package mongodb

import (
	"context"

	"github.com/mikeydub/go-gallery/persist"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	// "github.com/davecgh/go-spew/spew"
)

const (
	loginAttemptCollName = "user_login_attempts"
	noncesCollName       = "nonces"
)

// LoginMongoRepository is a repository for storing login attempts in a MongoDB database
type LoginMongoRepository struct {
	mp *storage
}

// NonceMongoRepository is a repository for storing authentication nonces in a MongoDB database
type NonceMongoRepository struct {
	mp *storage
}

// NewLoginMongoRepository returns a new instance of a login attempt repository
func NewLoginMongoRepository(mgoClient *mongo.Client) *LoginMongoRepository {
	return &LoginMongoRepository{
		mp: newStorage(mgoClient, 0, galleryDBName, loginAttemptCollName),
	}
}

// NewNonceMongoRepository returns a new instance of a nonce repository
func NewNonceMongoRepository(mgoClient *mongo.Client) *NonceMongoRepository {
	return &NonceMongoRepository{
		mp: newStorage(mgoClient, 0, galleryDBName, noncesCollName),
	}
}

// Create inserts a single login attempt into the database and will return the ID of the inserted attempt
func (l *LoginMongoRepository) Create(pCtx context.Context, pLoginAttempt *persist.UserLoginAttempt,
) (persist.DBID, error) {
	return l.mp.insert(pCtx, pLoginAttempt)
}

// Get returns the most recent nonce for a given address
func (n *NonceMongoRepository) Get(pCtx context.Context, pAddress persist.Address) (*persist.UserNonce, error) {

	opts := options.Find()
	opts.SetSort(bson.M{"created_at": -1})
	opts.SetLimit(1)

	result := []*persist.UserNonce{}
	err := n.mp.find(pCtx, bson.M{"address": pAddress}, &result, opts)

	if err != nil {
		return nil, err
	}

	if len(result) != 1 {
		return nil, persist.ErrNonceNotFoundForAddress{Address: pAddress}
	}

	return result[0], nil
}

// Create inserts a new nonce into the database and will return the ID of the inserted nonce
func (n *NonceMongoRepository) Create(pCtx context.Context, pNonce *persist.UserNonce) error {
	_, err := n.mp.insert(pCtx, pNonce)
	return err
}
