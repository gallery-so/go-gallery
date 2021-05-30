package glry_lib

import (
	"fmt"
	"math/rand"
	"time"
	"context"
	"go.mongodb.org/mongo-driver/bson/primitive"
	// log "github.com/sirupsen/logrus"
	gf_core "github.com/gloflow/gloflow/go/gf_core"
	"github.com/mikeydub/go-gallery/glry_core"
	"github.com/mikeydub/go-gallery/glry_db"
	// "github.com/davecgh/go-spew/spew"
)

//-------------------------------------------------------------
// NONCE
//-------------------------------------------------------------
// NONCE_CREATE__PIPELINE
func AuthNonceCreatePipeline(pUserIDstr glry_db.GLRYuserID,
	pUserAddressStr glry_db.GLRYuserAddress,
	pCtx            context.Context,
	pRuntime        *glry_core.Runtime) (*glry_db.GLRYuserNonce, *gf_core.Gf_error) {
	
	// NONCE
	nonceStr := AuthGenerateRandom()

	creationTimeUNIXf := float64(time.Now().UnixNano())/1000000000.0
	nonce := &glry_db.GLRYuserNonce{
		VersionInt:    0,
		ID:            primitive.NewObjectID(),
		CreationTimeF: creationTimeUNIXf,
		DeletedBool:   false,

		ValueStr:   nonceStr,
		UserIDstr:  pUserIDstr,
		AddressStr: pUserAddressStr,
	}

	// DB_CREATE
	gErr := glry_db.AuthNonceCreate(nonce, pCtx, pRuntime)
	if gErr != nil {
		return nil, gErr
	}

	return nonce, nil
}

//-------------------------------------------------------------
// NONCE_GENERATE
func AuthGenerateRandom() string {
	seededRand := rand.New(rand.NewSource(time.Now().UnixNano()))
	nonceInt   := seededRand.Int()
	nonceStr   := fmt.Sprintf("%d", nonceInt)
	return nonceStr	  
}

