package persist

import (
	"context"
	"fmt"
	"testing"

	"github.com/mikeydub/go-gallery/runtime"
	"github.com/stretchr/testify/assert"
)

func TestUnassignedWithAggregation(t *testing.T) {

	runtime, gErr := runtime.RuntimeGet(&runtime.Config{MongoURLstr: "mongodb://127.0.0.1:27017", MongoDBnameStr: "gallery", Port: 4000, BaseURL: "http://localhost:4000", EnvStr: "glry_test"})
	if gErr != nil {
		t.Fail()
	}

	user := &User{UserNameStr: "Bob", AddressesLst: []string{"0x456d569592f15Af845D0dbe984C12BAB8F430e31"}}

	userId, err := UserCreate(user, context.Background(), runtime)
	assert.Nil(t, err)

	nfts := []*Nft{}

	for i := 0; i < 30; i++ {
		nfts = append(nfts, &Nft{NameStr: fmt.Sprint(i), OwnerUserIdStr: userId})
	}
	nftIds, err := NftCreateBulk(nfts, context.Background(), runtime)
	assert.Nil(t, err)
	assert.Len(t, nftIds, 30)

	nftsInColOne := []DbId{}
	nftsInColTwo := []DbId{}

	for i, id := range nftIds {

		if i%3 == 0 {
			nftsInColOne = append(nftsInColOne, id)
		} else if i%7 == 0 {
			nftsInColTwo = append(nftsInColTwo, id)
		}
	}
	assert.Len(t, nftsInColOne, 10)
	assert.Len(t, nftsInColTwo, 3)

	_, err = CollCreate(&CollectionDb{NameStr: "Poop", NftsLst: nftsInColOne, OwnerUserIDstr: userId}, context.Background(), runtime)
	assert.Nil(t, err)
	_, err = CollCreate(&CollectionDb{NameStr: "Baby", NftsLst: nftsInColTwo, OwnerUserIDstr: userId}, context.Background(), runtime)
	assert.Nil(t, err)

	unassignedCollection, err := CollGetUnassigned(user.IDstr, context.Background(), runtime)
	assert.Nil(t, err)

	unassignedIds := []DbId{}

	for _, k := range unassignedCollection.NftsLst {
		unassignedIds = append(unassignedIds, k.IDstr)
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
