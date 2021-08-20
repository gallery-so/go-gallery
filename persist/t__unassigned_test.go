package persist

import (
	"context"
	"fmt"
	"testing"

	"github.com/mikeydub/go-gallery/runtime"
	"github.com/stretchr/testify/assert"
)

func TestUnassignedWithAggregation(t *testing.T) {

	runtime, gErr := runtime.GetRuntime(&runtime.Config{MongoURL: "mongodb://127.0.0.1:27017", MongoDBName: "gallery", Port: 4000, BaseURL: "http://localhost:4000", EnvStr: "glry_test"})
	if gErr != nil {
		t.Fail()
	}

	user := &User{UserName: "Bob", Addresses: []string{"0x456d569592f15Af845D0dbe984C12BAB8F430e31"}}

	userID, err := UserCreate(context.Background(), user, runtime)
	assert.Nil(t, err)

	nfts := []*Nft{}

	for i := 0; i < 30; i++ {
		nfts = append(nfts, &Nft{Name: fmt.Sprint(i), OwnerUserID: userID})
	}
	nftIds, err := NftCreateBulk(context.Background(), nfts, runtime)
	assert.Nil(t, err)
	assert.Len(t, nftIds, 30)

	nftsInColOne := []DBID{}
	nftsInColTwo := []DBID{}

	for i, id := range nftIds {

		if i%3 == 0 {
			nftsInColOne = append(nftsInColOne, id)
		} else if i%7 == 0 {
			nftsInColTwo = append(nftsInColTwo, id)
		}
	}
	assert.Len(t, nftsInColOne, 10)
	assert.Len(t, nftsInColTwo, 3)

	_, err = CollCreate(context.Background(), &CollectionDB{Name: "Poop", Nfts: nftsInColOne, OwnerUserID: userID}, runtime)
	assert.Nil(t, err)
	_, err = CollCreate(context.Background(), &CollectionDB{Name: "Baby", Nfts: nftsInColTwo, OwnerUserID: userID}, runtime)
	assert.Nil(t, err)

	unassignedCollection, err := CollGetUnassigned(context.Background(), user.ID, true, runtime)
	assert.Nil(t, err)

	unassignedIds := []DBID{}

	for _, k := range unassignedCollection.Nfts {
		unassignedIds = append(unassignedIds, k.ID)
	}

	assert.Len(t, unassignedIds, len(nftIds)-(len(nftsInColOne)+len(nftsInColTwo)))
	assert.Contains(t, unassignedIds, nftIds[26])
	assert.NotContains(t, unassignedIds, nftsInColOne[0])
	assert.NotContains(t, unassignedIds, nftsInColTwo[0])

}

// NOTE: only uncomment init when you want to run benchmark so that the tests don't take long

// var r *runtime.Runtime
// var ctx context.Context

// func init() {
// 	r, _ = runtime.RuntimeGet(&runtime.Config{MongoURLstr: "mongodb://127.0.0.1:27017", MongoDBnameStr: "gallery", Port: 4000, BaseURL: "http://localhost:4000", EnvStr: "glry_test"})
// 	ctx = context.Background()
// 	nfts := []*Nft{}

// 	for i := 0; i < 10000; i++ {
// 		nfts = append(nfts, &Nft{NameStr: fmt.Sprint(i), OwnerUserIdStr: "jim"})
// 	}
// 	nftIds, _ := NftCreateBulk(nfts, context.Background(), r)

// 	moreNfts := []*Nft{}
// 	for i := 0; i < 10000; i++ {
// 		moreNfts = append(moreNfts, &Nft{NameStr: fmt.Sprint(i), OwnerUserIdStr: "bob"})
// 	}
// 	NftCreateBulk(moreNfts, context.Background(), r)

// 	nftsInColOne := []DbId{}
// 	nftsInColTwo := []DbId{}

// 	for i, id := range nftIds {

// 		if i%3 == 0 {
// 			nftsInColOne = append(nftsInColOne, id)
// 		} else if i%7 == 0 {
// 			nftsInColTwo = append(nftsInColTwo, id)
// 		}
// 	}

// 	colOne := &CollectionDb{NameStr: "Poop", NFTsLst: nftsInColOne, OwnerUserIDstr: "jim"}
// 	CollCreate(colOne, ctx, r)

// 	colTwo := &CollectionDb{NameStr: "Baby", NFTsLst: nftsInColTwo, OwnerUserIDstr: "bob"}
// 	CollCreate(colTwo, ctx, r)

// }

// func BenchmarkUnassigned(b *testing.B) {

// 	// b.ResetTimer()
// 	for i := 0; i < b.N; i++ {
// 		CollGetUnassigned("jim", ctx, r)
// 	}

// }
