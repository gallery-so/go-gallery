package db

import (
	"fmt"
	"context"
	log "github.com/sirupsen/logrus"
	// "github.com/georgysavva/scany/pgxscan"
	// "github.com/jackc/pgx/v4/pgxpool"
	"go.mongodb.org/mongo-driver/mongo"
	gfcore "github.com/gloflow/gloflow/go/gf_core"
)

//-------------------------------------------------------------
/*type Storage interface {
	GetNFTsByUserID(ctx context.Context, userID string) ([]*NFT, error)
	Cleanup()
}*/

type DB struct {
	// pool *pgxpool.Pool
	Mongo *mongo.Database
}

//-------------------------------------------------------------
func Init(pMongoHostStr string,
	pMongoDBNamestr string,
	pRuntimeSys     *gfcore.Runtime_sys) (*DB, *gfcore.Gf_error) {

	mongoURLstr  := fmt.Sprintf("mongodb://%s", pMongoHostStr)
	log.WithFields(log.Fields{
		"host":    pMongoHostStr,
		"db_name": pMongoDBNamestr,
	}).Info("Mongo conn info")

	//-------------------------------------------------------------
	// GF_GET_DB
	GFgetDBfun := func() (*mongo.Database, *gfcore.Gf_error) {

		mongoDB, gErr := gfcore.Mongo__connect_new(mongoURLstr,
			pMongoDBNamestr,
			pRuntimeSys)
		if gErr != nil {
			return nil, gErr
		}
		log.Info("mongodb connected...")

		return mongoDB, nil
	}

	//-------------------------------------------------------------
	mongoDB, gErr := GFgetDBfun()
	if gErr != nil {
		return nil, gErr
	}

	

	db := &DB{
		Mongo: mongoDB,
	}

	return db, nil
}

/*func NewDB(ctx context.Context, uri string) (*DB, error) {
	pool, err := pgxpool.Connect(ctx, uri)
	if err != nil {
		return nil, err
	}

	return &DB{pool: pool}, nil
}*/

//-------------------------------------------------------------
func NFTgetByUserID(pUserIDstr string,
	pCtx context.Context) ([]*NFT, *gfcore.Gf_error) {





	return nil, nil
}

/*func (db *DB) GetNFTsByUserID(ctx context.Context, userID string) ([]*NFT, error) {
	var nfts []*NFT

	query := `
SELECT
	id,
	user_id,
	image_url,
--	description
	name,
	collection_name,
	position,
	external_url,
	created_date,
	creator_address,
	contract_address,
--	token_id,
	hidden,
	image_thumbnail_url,
	image_preview_url
FROM nfts
WHERE user_id='%s'
`
	err := pgxscan.Select(ctx, db.pool, &nfts, fmt.Sprintf(query, userID))
	if err != nil {
		return nil, err
	}

	return nfts, nil
}*/

//-------------------------------------------------------------
/*func (db *DB) Cleanup() {
	db.pool.Close()
}*/
