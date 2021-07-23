package persist

import (
	"context"
	"fmt"
	"testing"

	"github.com/mikeydub/go-gallery/runtime"
	"github.com/stretchr/testify/assert"
	// gfcore "github.com/gloflow/gloflow/go/gf_core"
)

//---------------------------------------------------
func TestCreateAndGetNFT(pTest *testing.T) {

	fmt.Println("TEST__NFT ==============================================")

	ctx := context.Background()
	if deadline, ok := pTest.Deadline(); ok {
		newCtx, cancel := context.WithDeadline(ctx, deadline)
		defer cancel()
		ctx = newCtx
	}

	//--------------------
	// RUNTIME_SYS

	runtime, gErr := runtime.RuntimeGet(&runtime.Config{MongoURLstr: "mongodb://127.0.0.1:27017", MongoDBnameStr: "gallery", Port: 4000, BaseURL: "http://localhost:4000", EnvStr: "glry_test"})
	if gErr != nil {
		pTest.Fail()
	}

	id, err := NftCreate(&Nft{OwnerUserIdStr: "poop", DescriptionStr: "cool nft", NameStr: "Big Bobby's Balooga"}, ctx, runtime)
	assert.Nil(pTest, err)

	err = NftUpdateById(id, "poop", &Nft{OwnerUserIdStr: "poop", DescriptionStr: "extremely cool nft", NameStr: "Big Bobby's Balooga"}, ctx, runtime)
	assert.Nil(pTest, err)

	nfts, err := NftGetById(id, ctx, runtime)
	assert.Nil(pTest, err)

	assert.Equal(pTest, "extremely cool nft", nfts[0].DescriptionStr)

	//--------------------
}
