package mongodb

import (
	"context"
	"testing"
	"time"

	"github.com/mikeydub/go-gallery/service/memstore/redis"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/mongo/writeconcern"
)

func TestGalleryAccountsForCollections_Success(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
	defer cancel()
	mOpts := options.Client().ApplyURI(string("mongodb://localhost:27017/"))
	mOpts.SetRegistry(CustomRegistry)
	mOpts.SetRetryWrites(true)
	mOpts.SetWriteConcern(writeconcern.New(writeconcern.WMajority()))

	client := NewMongoClient(ctx, mOpts)
	defer client.Disconnect(ctx)
	defer client.Database("gallery").Drop(context.Background())

	viper.SetDefault("REDIS_URL", "localhost:6379")

	collCache := redis.NewCache(0)
	gallCache := redis.NewCache(1)
	nftCache := redis.NewCache(2)
	openseaCache := redis.NewCache(3)

	galleryRepo := NewGalleryRepository(client, gallCache)

	nftRepo := NewNFTRepository(client, nftCache, openseaCache, galleryRepo)

	collRepo := NewCollectionRepository(client, collCache, galleryRepo, nftRepo)

	userRepo := NewUserRepository(client)

	user := persist.User{
		Username:           "test",
		UsernameIdempotent: "test",
		Addresses:          []persist.Address{"0x8914496dC01Efcc49a2FA340331Fb90969B6F1d2"},
		Bio:                "test",
	}

	userID, err := userRepo.Create(ctx, user)
	if err != nil {
		t.Errorf("Error creating user: %v", err)
	}

	collections := []persist.CollectionDB{
		{
			OwnerUserID: userID,
			Name:        "test",
		},
		{
			OwnerUserID: userID,
			Name:        "test2",
		},
	}

	for _, collection := range collections {
		logrus.Infof("Creating collection %+v", collection)
		_, err := collRepo.Create(ctx, collection)
		if err != nil {
			t.Errorf("Error creating collection: %v", err)
		}
	}

	ga := persist.GalleryDB{
		OwnerUserID: userID,
	}

	id, err := galleryRepo.Create(ctx, ga)
	if err != nil {
		t.Errorf("Error creating gallery: %v", err)
	}

	newGa, err := galleryRepo.GetByID(ctx, id)
	if err != nil {
		t.Errorf("Error getting gallery: %v", err)
	}

	if len(newGa.Collections) != 2 {
		t.Errorf("Expected 2 collections, got %d", len(newGa.Collections))
	}

}
