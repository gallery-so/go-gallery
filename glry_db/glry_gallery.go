package glry_db

import (
	"context"
	"time"

	"github.com/mikeydub/go-gallery/glry_core"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const galleryColName = "glry_galleries"

//-------------------------------------------------------------
type GLRYgalleryID string
type GLRYgallery struct {
	VersionInt    int64      `bson:"version"       json:"version"` // schema version for this model
	IDstr         GLRYcollID `bson:"_id"           json:"id"`
	CreationTimeF float64    `bson:"creation_time" json:"creation_time"`
	DeletedBool   bool       `bson:"deleted"`

	OwnerUserIDstr string   `bson:"owner_user_id,omitempty" json:"owner_user_id"`
	CollectionsLst []string `bson:"collections,omitempty"          json:"collections"`
}

//-------------------------------------------------------------
func GalleryCreate(pGallery *GLRYgallery,
	pCtx context.Context,
	pRuntime *glry_core.Runtime) error {

	mp := glry_core.NewMongoPersister(0, collectionColName, pRuntime)

	return mp.Insert(pCtx, pGallery)
}

//-------------------------------------------------------------
func GalleryGetByUserID(pUserIDstr GLRYuserID,
	pCtx context.Context,
	pRuntime *glry_core.Runtime) ([]*GLRYgallery, error) {

	opts := &options.FindOptions{}
	if deadline, ok := pCtx.Deadline(); ok {
		dur := time.Until(deadline)
		opts.MaxTime = &dur
	}

	mp := glry_core.NewMongoPersister(0, collectionColName, pRuntime)

	result := []*GLRYgallery{}

	if err := mp.Find(pCtx, bson.M{"user_id": pUserIDstr}, result, opts); err != nil {
		return nil, err
	}

	return result, nil
}

//-------------------------------------------------------------
func GalleryGetByID(pIDstr string,
	pCtx context.Context,
	pRuntime *glry_core.Runtime) ([]*GLRYgallery, error) {
	opts := &options.FindOptions{}
	if deadline, ok := pCtx.Deadline(); ok {
		dur := time.Until(deadline)
		opts.MaxTime = &dur
	}

	mp := glry_core.NewMongoPersister(0, collectionColName, pRuntime)

	result := []*GLRYgallery{}

	if err := mp.Find(pCtx, bson.M{"_id": pIDstr}, result, opts); err != nil {
		return nil, err
	}

	return result, nil
}
