package emails

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"

	"cloud.google.com/go/storage"
	"github.com/gin-gonic/gin"
	"github.com/mikeydub/go-gallery/db/gen/coredb"
	"github.com/mikeydub/go-gallery/env"
	"github.com/mikeydub/go-gallery/graphql/dataloader"
	"github.com/mikeydub/go-gallery/publicapi"
	"github.com/mikeydub/go-gallery/service/logger"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/util"
)

type Selected struct {
	Position *int `json:"position"`
	Entity   any  `json:"entity"`
}

type GalleryWithUser struct {
	Gallery coredb.Gallery `json:"gallery"`
	User    coredb.User    `json:"user"`
}

type DigestValues struct {
	TopPosts       IncludedSelected `json:"posts"`
	TopCommunities IncludedSelected `json:"communities"`
	TopGalleries   IncludedSelected `json:"galleries"`
	TopFirstPosts  IncludedSelected `json:"first_posts"`
}

type IncludedSelected struct {
	Selected []Selected `json:"selected"`
	Include  bool       `json:"include"`
}

type SelectedID struct {
	ID       persist.DBID `json:"id"`
	Position int          `json:"position"`
}

type DigestValueOverrides struct {
	TopPosts              []SelectedID `json:"posts"`
	TopCommunities        []SelectedID `json:"communities"`
	TopGalleries          []SelectedID `json:"galleries"`
	TopFirstPosts         []SelectedID `json:"first_posts"`
	PostCount             int          `json:"post_count"`
	CommunityCount        int          `json:"community_count"`
	GalleryCount          int          `json:"gallery_count"`
	FirstPostCount        int          `json:"first_post_count"`
	IncludeTopPosts       *bool        `json:"include_top_posts,omitempty"`
	IncludeTopCommunities *bool        `json:"include_top_communities,omitempty"`
	IncludeTopGalleries   *bool        `json:"include_top_galleries,omitempty"`
	IncludeTopFirstPosts  *bool        `json:"include_top_first,omitempty"`
}

type UserFacingToken struct {
	TokenID         persist.DBID `json:"token_id"`
	Name            string       `json:"name"`
	Description     string       `json:"description"`
	PreviewImageURL string       `json:"preview_image_url"`
	Override        bool         `json:"override"`
}
type UserFacingPost struct {
	PostID          persist.DBID      `json:"post_id"`
	Caption         string            `json:"caption"`
	Author          string            `json:"author"`
	Tokens          []UserFacingToken `json:"tokens"`
	PreviewImageURL string            `json:"preview_image_url"`
	Override        bool              `json:"override"`
}

type UserFacingContract struct {
	ContractID      persist.DBID    `json:"contract_id"`
	ContractAddress persist.Address `json:"contract_address"`
	Chain           persist.Chain   `json:"chain"`
	Name            string          `json:"name"`
	Description     string          `json:"description"`
	PreviewImageURL string          `json:"preview_image_url"`
	Override        bool            `json:"override"`
}

const (
	defaultPostCount             = 5
	defaultCommunityCount        = 5
	defaultGalleryCount          = 5
	defaultFirstPostCount        = 5
	defaultIncludeTopPosts       = true
	defaultIncludeTopGaleries    = false
	defaultIncludeTopCommunities = true
	defaultIncludeTopFirstPosts  = false
)

const overrideFile = "email_digest_overrides.json"

func getDigestValues(q *coredb.Queries, loaders *dataloader.Loaders, stg *storage.Client, f *publicapi.FeedAPI) gin.HandlerFunc {
	return func(c *gin.Context) {

		// mimic backend auth
		c.Set("auth.user_id", persist.DBID(""))
		c.Set("auth.auth_error", nil)

		result, err := getDigest(c, stg, f, q, loaders)
		if err != nil {
			util.ErrResponse(c, http.StatusInternalServerError, err)
			return
		}
		c.JSON(http.StatusOK, result)
	}
}

func getDigest(c context.Context, stg *storage.Client, f *publicapi.FeedAPI, q *coredb.Queries, loaders *dataloader.Loaders) (DigestValues, error) {
	// TODO top galleries and top first posts
	overrides, err := getOverrides(c, stg)
	if err != nil {
		return DigestValues{}, fmt.Errorf("error getting overrides: %v", err)
	}

	postCount := defaultPostCount
	collectionCount := defaultCommunityCount

	if overrides.PostCount != 0 {
		postCount = overrides.PostCount
	}

	if overrides.CommunityCount != 0 {
		collectionCount = overrides.CommunityCount
	}

	trendingFeed, _, err := f.TrendingFeed(c, nil, nil, util.ToPointer(10), nil)
	if err != nil {
		return DigestValues{}, fmt.Errorf("error getting trending feed: %v", err)
	}

	topPosts := util.Filter(util.MapWithoutError(trendingFeed, func(a any) any {
		if post, ok := a.(coredb.Post); ok {
			up, err := postToUserFacing(c, q, post, loaders, false)
			if err != nil {
				logger.For(c).Errorf("error converting post to user facing: %s", err)
				return nil
			}
			return up
		}
		return nil
	}), func(p any) bool {
		return p.(UserFacingPost).PostID != ""
	}, false)

	selectedPosts := selectResults(topPosts, overrides.TopPosts, func(s SelectedID) Selected {
		p, err := q.GetPostByID(c, s.ID)
		if err != nil {
			logger.For(c).Errorf("error getting post by id: %s", err)
			return Selected{}
		}
		up, err := postToUserFacing(c, q, p, loaders, true)
		if err != nil {
			logger.For(c).Errorf("error converting post to user facing: %s", err)
			return Selected{}
		}
		return Selected{
			Entity:   up,
			Position: &s.Position,
		}
	}, postCount)

	topCollectionsDB, err := q.GetTopCommunitiesByPosts(c, 10)
	if err != nil {
		return DigestValues{}, err
	}

	topCollectionsUserFacing := util.MapWithoutError(topCollectionsDB, func(co coredb.GetTopCommunitiesByPostsRow) any {
		return contractToUserFacing(c, q, loaders, co.Contract, false)
	})

	selectedCollections := selectResults(topCollectionsUserFacing, overrides.TopCommunities, func(s SelectedID) Selected {
		co, err := q.GetContractByID(c, s.ID)
		if err != nil {
			return Selected{}
		}
		return Selected{
			Entity:   contractToUserFacing(c, q, loaders, co, true),
			Position: &s.Position,
		}
	}, collectionCount)

	includePosts := defaultIncludeTopPosts
	includeCommunities := defaultIncludeTopCommunities
	if overrides.IncludeTopPosts != nil {
		includePosts = *overrides.IncludeTopPosts
	}
	if overrides.IncludeTopCommunities != nil {
		includeCommunities = *overrides.IncludeTopCommunities
	}

	result := DigestValues{
		TopPosts: IncludedSelected{
			Selected: selectedPosts,
			Include:  includePosts,
		},
		TopCommunities: IncludedSelected{
			Selected: selectedCollections,
			Include:  includeCommunities,
		},
	}
	return result, nil
}

func getOverrides(c context.Context, stg *storage.Client) (DigestValueOverrides, error) {
	_, err := stg.Bucket(env.GetString("CONFIGURATION_BUCKET")).Object(overrideFile).Attrs(c)
	if err != nil && err != storage.ErrObjectNotExist {
		return DigestValueOverrides{}, fmt.Errorf("error getting overrides attrs: %v", err)
	}

	if err == storage.ErrObjectNotExist {
		w := stg.Bucket(env.GetString("CONFIGURATION_BUCKET")).Object(overrideFile).NewWriter(c)
		err = json.NewEncoder(w).Encode(DigestValueOverrides{})
		if err != nil {
			return DigestValueOverrides{}, fmt.Errorf("error encoding overrides: %v", err)
		}
		err = w.Close()
		if err != nil {
			return DigestValueOverrides{}, fmt.Errorf("error closing writer: %v", err)
		}
	}

	reader, err := stg.Bucket(env.GetString("CONFIGURATION_BUCKET")).Object(overrideFile).NewReader(c)
	if err != nil {
		return DigestValueOverrides{}, fmt.Errorf("error getting overrides: %v", err)
	}

	var overrides DigestValueOverrides
	err = json.NewDecoder(reader).Decode(&overrides)
	if err != nil {
		return DigestValueOverrides{}, fmt.Errorf("error decoding overrides: %v", err)
	}
	return overrides, nil
}

func contractToUserFacing(ctx context.Context, q *coredb.Queries, l *dataloader.Loaders, collection coredb.Contract, override bool) UserFacingContract {
	if collection.ProfileImageUrl.String == "" {
		tokens, err := q.GetTokensByContractIdPaginate(ctx, coredb.GetTokensByContractIdPaginateParams{
			ID:               collection.ID,
			Limit:            1,
			GalleryUsersOnly: true,
		})
		if err == nil && len(tokens) > 0 {
			media, err := l.GetMediaByMediaIdIgnoringStatusBatch.Load(tokens[0].TokenDefinition.TokenMediaID)
			if err == nil {
				collection.ProfileImageUrl.String = util.FirstNonEmptyString(media.Media.ThumbnailURL.String(), media.Media.MediaURL.String())
			}
		}

	}
	return UserFacingContract{
		ContractID:      collection.ID,
		Name:            collection.Name.String,
		Description:     collection.Description.String,
		PreviewImageURL: collection.ProfileImageUrl.String,
		ContractAddress: collection.Address,
		Chain:           collection.Chain,
		Override:        override,
	}
}

func tokenToUserFacing(c context.Context, tokenID persist.DBID, q *coredb.Queries, loaders *dataloader.Loaders, override bool) (UserFacingToken, error) {
	token, err := q.GetTokenById(c, tokenID)
	if err != nil {
		return UserFacingToken{}, fmt.Errorf("error getting token by id: %s", err)
	}
	media, err := loaders.GetMediaByMediaIdIgnoringStatusBatch.Load(token.TokenDefinition.TokenMediaID)
	if err != nil {
		return UserFacingToken{}, fmt.Errorf("error getting media by id: %s", err)
	}
	return UserFacingToken{
		TokenID:         tokenID,
		Name:            token.TokenDefinition.Name.String,
		Description:     token.TokenDefinition.Description.String,
		PreviewImageURL: util.FirstNonEmptyString(media.Media.ThumbnailURL.String(), media.Media.MediaURL.String()),
		Override:        override,
	}, nil
}

func postToUserFacing(c context.Context, q *coredb.Queries, post coredb.Post, loaders *dataloader.Loaders, override bool) (UserFacingPost, error) {
	user, err := q.GetUserById(c, post.ActorID)
	if err != nil {
		return UserFacingPost{}, fmt.Errorf("error getting user by id: %s", err)
	}
	var tokens []UserFacingToken
	for _, t := range post.TokenIds {
		ut, err := tokenToUserFacing(c, t, q, loaders, override)
		if err != nil {
			return UserFacingPost{}, fmt.Errorf("error getting token by id: %s", err)
		}

		tokens = append(tokens, ut)
	}
	var previewURL string
	if len(tokens) > 0 {
		previewURL = tokens[0].PreviewImageURL
	}

	return UserFacingPost{
		PostID:          post.ID,
		Caption:         post.Caption.String,
		Author:          user.Username.String,
		Tokens:          tokens,
		PreviewImageURL: previewURL,
		Override:        override,
	}, nil
}

func selectResults(initial []any, overrides []SelectedID, overrideFetcher func(s SelectedID) Selected, selectedCount int) []Selected {
	selectedResults := make([]Selected, int(math.Max(float64(len(initial)), float64(len(overrides)))))
	for _, post := range overrides {
		selectedResults[post.Position] = overrideFetcher(post)
	}
outer:
	for i, it := range initial {
		ic := i
		if selectedResults[i].Position != nil && i < selectedCount {
			// add to the next available position while keeping the order, if we exceed 5, append to the end still without a position
			// also ensure that i is updated so that we don't overwrite the same position in the next loop
			for j := i; j < selectedCount; j++ {
				j := j
				if selectedResults[j].Position == nil {
					selectedResults[j] = Selected{
						Entity:   it,
						Position: &j,
					}
					i = j
					continue outer
				}
			}
		}
		if i < selectedCount {
			selectedResults[i] = Selected{
				Entity:   it,
				Position: &ic,
			}
		} else {
			selectedResults[i] = Selected{
				Entity:   it,
				Position: nil,
			}
		}
	}
	return selectedResults
}

func updateDigestValues(stg *storage.Client) gin.HandlerFunc {
	return func(c *gin.Context) {

		input := DigestValueOverrides{}
		if err := c.ShouldBindJSON(&input); err != nil {
			util.ErrResponse(c, http.StatusBadRequest, fmt.Errorf("error binding json: %v", err))
			return
		}

		currentOverrides, err := getOverrides(c, stg)
		if err != nil {
			util.ErrResponse(c, http.StatusInternalServerError, fmt.Errorf("error getting overrides: %v", err))
			return
		}

		merged := mergeOverrides(currentOverrides, input)

		w := stg.Bucket(env.GetString("CONFIGURATION_BUCKET")).Object(overrideFile).NewWriter(c)
		err = json.NewEncoder(w).Encode(&merged)
		if err != nil {
			util.ErrResponse(c, http.StatusInternalServerError, fmt.Errorf("error encoding overrides: %v", err))
			return
		}
		err = w.Close()
		if err != nil {
			util.ErrResponse(c, http.StatusInternalServerError, fmt.Errorf("error closing writer: %v", err))
			return
		}
	}
}

// mergeOverrides takes two overrides and merges them with the second taking precendence. For each of the arrays, if a position overlaps, the second one is taken
func mergeOverrides(first, second DigestValueOverrides) DigestValueOverrides {
	seenPostPositions := make(map[int]bool)
	seenCommunityPositions := make(map[int]bool)
	for _, p := range second.TopPosts {
		seenPostPositions[p.Position] = true
	}
	for _, p := range second.TopCommunities {
		seenCommunityPositions[p.Position] = true
	}

	for _, p := range first.TopPosts {
		if _, ok := seenPostPositions[p.Position]; !ok {
			second.TopPosts = append(second.TopPosts, p)
		}
	}

	for _, p := range first.TopCommunities {
		if _, ok := seenCommunityPositions[p.Position]; !ok {
			second.TopCommunities = append(second.TopCommunities, p)
		}
	}

	if second.PostCount == 0 {
		second.PostCount = first.PostCount
	}

	if second.CommunityCount == 0 {
		second.CommunityCount = first.CommunityCount
	}

	return second
}
