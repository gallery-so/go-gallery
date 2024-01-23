package main

import (
	"bytes"
	"context"
	"encoding/csv"
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"

	"github.com/gin-gonic/gin"

	db "github.com/mikeydub/go-gallery/db/gen/coredb"
	"github.com/mikeydub/go-gallery/env"
	"github.com/mikeydub/go-gallery/graphql/dataloader"
	"github.com/mikeydub/go-gallery/publicapi"
	"github.com/mikeydub/go-gallery/server"
	"github.com/mikeydub/go-gallery/service/multichain/wlta"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/util"
)

func main() {
	server.SetDefaults()
	ctx := context.Background()
	clients := server.ClientInit(ctx)
	loaders := dataloader.NewLoaders(ctx, clients.Queries, true, nil, nil)
	ctx = createCtx(ctx, clients.Queries)
	api := newAPI(ctx, clients)

	userID, err := publicapi.GetAuthenticatedUserID(ctx)
	if err != nil {
		panic(err)
	}

	galleries, err := api.Gallery.GetGalleriesByUserId(ctx, userID)
	if err != nil {
		panic(err)
	}

	// delete old galleries
	for _, g := range galleries {
		err := api.Gallery.DeleteGallery(ctx, g.ID)
		if err != nil {
			panic(err)
		}
	}

	categorySubmissions := readSubmissionMappings(fmt.Sprintf("./service/multichain/wlta/id_to_category_%s.csv", env.GetString("ENV")))

	// create gallery for each category
	var pos int
	for cat, name := range map[wlta.Category]string{
		wlta.GenreOneOfOnes: "1 of 1's",
		wlta.GenreAi:        "AI Art",
		wlta.GenreGenArt:    "Generative Art",
		wlta.GenreMusic:     "Music",
	} {
		fmt.Printf("[%s] creating gallery: %d tokens\n", name, len(categorySubmissions[cat]))
		gallery, err := api.Gallery.CreateGallery(ctx, &name, util.ToPointer(""), strconv.Itoa(pos))
		if err != nil {
			panic(err)
		}

		// set first gallery as featured
		if pos == 0 {
			err = api.User.UpdateFeaturedGallery(ctx, gallery.ID)
			if err != nil {
				panic(err)
			}
		}

		layout := persist.TokenLayout{
			Sections:      []int{0}, // one section
			SectionLayout: []persist.CollectionSectionLayout{{Columns: 4}},
		}

		params := make([]db.GetTokenByUserTokenIdentifiersBatchParams, len(categorySubmissions[cat]))
		for i, m := range categorySubmissions[cat] {
			params[i] = db.GetTokenByUserTokenIdentifiersBatchParams{
				OwnerID:         userID,
				TokenID:         m.TokenID,
				Chain:           m.Chain,
				ContractAddress: m.ContractAddress,
			}
		}

		tokens, _ := loaders.GetTokenByUserTokenIdentifiersBatch.LoadAll(params)
		tokens = util.Filter(tokens, func(t db.GetTokenByUserTokenIdentifiersBatchRow) bool { return t.Token.ID != "" }, true)
		tokenIDs := util.MapWithoutError(tokens, func(t db.GetTokenByUserTokenIdentifiersBatchRow) persist.DBID { return t.Token.ID })

		collections := [][]persist.DBID{[]persist.DBID{}}

		for _, t := range tokenIDs {
			currentCollection := &collections[len(collections)-1]
			if len(*currentCollection) >= 32 {
				collections = append(collections, make([]persist.DBID, 0))
				currentCollection = &collections[len(collections)-1]
			}
			*currentCollection = append(*currentCollection, t)
		}

		deduped := make([][]persist.DBID, len(collections))

		for i, c := range collections {
			deduped[i] = util.Dedupe(c, false)
		}

		sort.Slice(deduped, func(i, j int) bool { return len(deduped[i]) < len(deduped[j]) })

		for _, c := range deduped {
			_, _, err = api.Collection.CreateCollection(ctx, gallery.ID, "", "", c, layout, nil, nil)
			if err != nil {
				panic(err)
			}
		}

		pos++
	}
}

func createCtx(ctx context.Context, q *db.Queries) context.Context {
	user, err := q.GetUserByUsername(ctx, "welovetheart")
	if err != nil {
		panic(err)
	}
	var gCtx gin.Context
	gCtx.Set("auth.auth_error", nil)
	gCtx.Set("auth.user_id", user.ID)
	ctx = context.WithValue(ctx, util.GinContextKey, &gCtx)
	return ctx
}

func newAPI(ctx context.Context, clients *server.Clients) *publicapi.PublicAPI {
	return publicapi.New(
		ctx,
		false,
		clients.Repos,
		clients.Queries,
		clients.HTTPClient,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
	)
}

type submissionMapping struct {
	Chain           persist.Chain
	ContractAddress persist.Address
	TokenID         persist.TokenID
	SubmissionID    int
	Category        wlta.Category
}

func readSubmissionMappings(f string) map[wlta.Category][]submissionMapping {
	fmt.Printf("reading submission mappings from: %s\n", f)
	byt, err := os.ReadFile(f)
	if err != nil {
		panic(err)
	}

	r := bytes.NewReader(byt)
	c := csv.NewReader(r)

	categorySubmissions := make(map[wlta.Category][]submissionMapping)
	categorySubmissions[wlta.GenreOneOfOnes] = make([]submissionMapping, 0)
	categorySubmissions[wlta.GenreAi] = make([]submissionMapping, 0)
	categorySubmissions[wlta.GenreGenArt] = make([]submissionMapping, 0)
	categorySubmissions[wlta.GenreMusic] = make([]submissionMapping, 0)

	for {
		record, err := c.Read()
		if err == io.EOF {
			break
		}

		if err != nil {
			panic(err)
		}

		m := rowToSubmissionMapping(record)
		categorySubmissions[m.Category] = append(categorySubmissions[m.Category], m)
	}

	return categorySubmissions
}

func rowToSubmissionMapping(r []string) submissionMapping {
	c, err := strconv.Atoi(r[0])
	if err != nil {
		panic(err)
	}
	s, err := strconv.Atoi(r[3])
	if err != nil {
		panic(err)
	}
	cat, err := strconv.Atoi(r[4])
	if err != nil {
		panic(err)
	}
	return submissionMapping{
		Chain:           persist.Chain(c),
		ContractAddress: persist.Address(r[1]),
		TokenID:         persist.TokenID(r[2]),
		SubmissionID:    s,
		Category:        wlta.Category(cat),
	}
}
