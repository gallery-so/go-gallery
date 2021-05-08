package glry_lib

//-------------------------------------------------------------
import (
	"fmt"
	"math/rand"
	"time"
	"context"
	gfcore "github.com/gloflow/gloflow/go/gf_core"
	"github.com/mikeydub/go-gallery/glry_db"
)

//-------------------------------------------------------------
func AuthUserPipelineGet(pAddressStr string,
	pCtx        context.Context,
	pRuntimeSys *gfcore.Runtime_sys) (int, *gfcore.Gf_error) {




	// DB_GET_USER_BY_ADDRESS
	user, gErr := glry_db.AuthUserGetByAddress(pAddressStr, pCtx, pRuntimeSys)
	if gErr != nil {
		return 0, gErr
	}

	fmt.Println(user)


	// NONCE
	nonceInt := AuthNonceGenerate()






	return nonceInt, nil
}

//-------------------------------------------------------------
func AuthNonceGenerate() int {
	seededRand := rand.New(rand.NewSource(time.Now().UnixNano()))
	nonceInt   := seededRand.Int()
	return nonceInt	  
}