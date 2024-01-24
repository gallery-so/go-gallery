//go:generate go get github.com/Khan/genqlient/generate
//go:generate go run github.com/Khan/genqlient
package emails

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/Khan/genqlient/graphql"
	"github.com/gin-gonic/gin"
	"github.com/sourcegraph/conc"

	db "github.com/mikeydub/go-gallery/db/gen/coredb"
	"github.com/mikeydub/go-gallery/service/logger"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/service/store"
	"github.com/mikeydub/go-gallery/util"
)

const (
	defaultPostCount             = 5
	defaultCommunityCount        = 3
	defaultGalleryCount          = 3
	defaultIncludeTopPosts       = true
	defaultIncludeTopGaleries    = true
	defaultIncludeTopCommunities = true
	overrideFile                 = "email_digest_overrides.json"
)

var lookbackWindow time.Duration = time.Duration(7 * 24 * time.Hour)

type DigestValues struct {
	Date           string           `json:"date"`
	IntroText      *string          `json:"intro_text"`
	Posts          IncludedSelected `json:"posts"`
	Communities    IncludedSelected `json:"communities"`
	Galleries      IncludedSelected `json:"galleries"`
	PostCount      int              `json:"post_count"`
	CommunityCount int              `json:"community_count"`
	GalleryCount   int              `json:"gallery_count"`
}

type IncludedSelected struct {
	Selected []any `json:"selected"`
	Include  bool  `json:"include"`
}

type DigestValueOverrides struct {
	Posts                 []persist.DBID `json:"posts"`
	Communities           []persist.DBID `json:"communities"`
	Galleries             []persist.DBID `json:"galleries"`
	PostCount             int            `json:"post_count"`
	CommunityCount        int            `json:"community_count"`
	GalleryCount          int            `json:"gallery_count"`
	IncludeTopPosts       *bool          `json:"include_top_posts,omitempty"`
	IncludeTopCommunities *bool          `json:"include_top_communities,omitempty"`
	IncludeTopGalleries   *bool          `json:"include_top_galleries,omitempty"`
	IntroText             *string        `json:"intro_text,omitempty"`
}

type TokenDigestEntity struct {
	TokenID         persist.DBID `json:"token_id"`
	Name            string       `json:"name"`
	Description     string       `json:"description"`
	PreviewImageURL string       `json:"preview_image_url"`
	Editorialized   bool         `json:"editorialized"`
}

type PostDigestEntity struct {
	PostID         persist.DBID        `json:"post_id"`
	Caption        string              `json:"caption"`
	AuthorUsername string              `json:"author_username"`
	AuthorPFPURL   string              `json:"author_pfp_url"`
	Tokens         []TokenDigestEntity `json:"tokens"`
	Editorialized  bool                `json:"editorialized"`
}

type CommunityDigestEntity struct {
	CommunityID            persist.DBID        `json:"community_id"`
	ContractAddress        persist.Address     `json:"contract_address"`
	ChainName              string              `json:"chain_name"`
	Name                   string              `json:"name"`
	Description            string              `json:"description"`
	PreviewImageURL        string              `json:"preview_image_url"`
	CreatorName            string              `json:"creator_name"`
	CreatorPreviewImageURL string              `json:"creator_preview_image_url"`
	Tokens                 []TokenDigestEntity `json:"tokens"`
	Editorialized          bool                `json:"editorialized"`
}

type GalleryDigestEntity struct {
	GalleryID       persist.DBID        `json:"gallery_id"`
	CreatorUsername string              `json:"creator_username"`
	Name            string              `json:"name"`
	Description     string              `json:"description"`
	Tokens          []TokenDigestEntity `json:"tokens"`
	Editorialized   bool                `json:"editorialized"`
}

func getDigestValues(q *db.Queries, b *store.BucketStorer, gql *graphql.Client) gin.HandlerFunc {
	return func(ctx *gin.Context) {
		result, err := buildDigestTemplate(ctx, b, q, gql)
		if err != nil {
			util.ErrResponse(ctx, http.StatusInternalServerError, err)
			return
		}
		ctx.JSON(http.StatusOK, result)
	}
}

func updateDigestValues(b *store.BucketStorer) gin.HandlerFunc {
	return func(ctx *gin.Context) {
		var input DigestValueOverrides

		if err := ctx.ShouldBindJSON(&input); err != nil {
			util.ErrResponse(ctx, http.StatusBadRequest, fmt.Errorf("error reading overrides: %v", err))
			return
		}

		byt, err := json.Marshal(input)
		if err != nil {
			util.ErrResponse(ctx, http.StatusInternalServerError, fmt.Errorf("error encoding overrides: %v", err))
			return
		}

		_, err = b.Write(ctx, overrideFile, byt)
		if err != nil {
			util.ErrResponse(ctx, http.StatusInternalServerError, fmt.Errorf("failed to write overrides: %s", err))
			return
		}

		ctx.JSON(http.StatusOK, util.SuccessResponse{Success: true})
	}
}

func buildDigestTemplate(ctx context.Context, b *store.BucketStorer, q *db.Queries, gql *graphql.Client) (DigestValues, error) {
	overrides, err := getOverrides(ctx, b)
	if err != nil {
		return DigestValues{}, fmt.Errorf("error getting overrides: %s", err)
	}

	postCount := defaultPostCount
	if overrides.PostCount != 0 {
		postCount = overrides.PostCount
	}

	communityCount := overrides.CommunityCount
	if communityCount == 0 || communityCount > defaultCommunityCount {
		communityCount = defaultCommunityCount
	}

	galleryCount := overrides.GalleryCount
	if galleryCount == 0 || galleryCount > defaultGalleryCount {
		galleryCount = defaultGalleryCount
	}

	posts := make([]PostDigestEntity, 0, postCount)
	communities := make([]CommunityDigestEntity, 0, communityCount)
	galleries := make([]GalleryDigestEntity, 0, galleryCount)
	var postErr error
	var communityErr error
	var galleryErr error

	var wg conc.WaitGroup
	wg.Go(func() {
		posts, postErr = getSpotlightPosts(ctx, q, gql, postCount, overrides.Posts)
	})
	wg.Go(func() {
		communities, communityErr = getSpotlightCommunities(ctx, q, gql, communityCount, overrides.Communities)
	})
	wg.Go(func() {
		galleries, galleryErr = getSpotlightGalleries(ctx, q, gql, galleryCount, overrides.Galleries)
	})
	wg.Wait()

	if postErr != nil {
		return DigestValues{}, fmt.Errorf("error getting spotlight posts: %s", postErr)
	}
	if communityErr != nil {
		return DigestValues{}, fmt.Errorf("error getting spotlight communities: %s", communityErr)
	}
	if galleryErr != nil {
		return DigestValues{}, fmt.Errorf("error getting spotlight galleries: %s", galleryErr)
	}

	includePosts := util.GetOptionalValue(overrides.IncludeTopPosts, defaultIncludeTopPosts)
	if includePosts && len(posts) == 0 {
		return DigestValues{}, fmt.Errorf("no posts to include")
	}

	includeCommunities := util.GetOptionalValue(overrides.IncludeTopCommunities, defaultIncludeTopCommunities)
	if includeCommunities && len(communities) == 0 {
		return DigestValues{}, fmt.Errorf("no communities to include")
	}

	includeGalleries := util.GetOptionalValue(overrides.IncludeTopGalleries, defaultIncludeTopGaleries)
	if includeGalleries && len(galleries) == 0 {
		return DigestValues{}, fmt.Errorf("no galleries to include")
	}

	return DigestValues{
		Date:           time.Now().Format("2 January 2006"),
		IntroText:      overrides.IntroText,
		PostCount:      postCount,
		CommunityCount: communityCount,
		GalleryCount:   galleryCount,
		Posts: IncludedSelected{
			Selected: util.MapWithoutError(posts, func(p PostDigestEntity) any { return p }),
			Include:  includePosts,
		},
		Communities: IncludedSelected{
			Selected: util.MapWithoutError(communities, func(c CommunityDigestEntity) any { return c }),
			Include:  includeCommunities,
		},
		Galleries: IncludedSelected{
			Selected: util.MapWithoutError(galleries, func(c GalleryDigestEntity) any { return c }),
			Include:  includeGalleries,
		},
	}, nil
}

func getSpotlightGalleries(ctx context.Context, q *db.Queries, gql *graphql.Client, n int, editorialGalleries []persist.DBID) ([]GalleryDigestEntity, error) {
	added := make(map[persist.DBID]bool)
	entities := make([]GalleryDigestEntity, 0, n)

	addGalleries(ctx, gql, &entities, added, n, editorialGalleries, true)
	if len(entities) >= n {
		return entities[:n], nil
	}

	trendingGalleries, err := q.GetTopGalleriesByViews(ctx, time.Now().Add(-lookbackWindow))
	if err != nil {
		return nil, err
	}

	addGalleries(ctx, gql, &entities, added, n, trendingGalleries, false)
	return entities, nil
}

func getSpotlightCommunities(ctx context.Context, q *db.Queries, gql *graphql.Client, n int, editorialCommunities []persist.DBID) ([]CommunityDigestEntity, error) {
	added := make(map[persist.DBID]bool)
	entities := make([]CommunityDigestEntity, 0, n)

	addCommunities(ctx, gql, &entities, added, n, editorialCommunities, true)
	if len(entities) >= n {
		return entities[:n], nil
	}

	trendingCommunities, err := q.GetTopCommunitiesByPosts(ctx, time.Now().Add(-lookbackWindow))
	if err != nil {
		return nil, err
	}

	addCommunities(ctx, gql, &entities, added, n, trendingCommunities, false)
	return entities, nil
}

func getSpotlightPosts(ctx context.Context, q *db.Queries, gql *graphql.Client, n int, editorialPosts []persist.DBID) ([]PostDigestEntity, error) {
	added := make(map[persist.DBID]bool)
	entities := make([]PostDigestEntity, 0, n)

	addPosts(ctx, gql, &entities, added, n, editorialPosts, true)
	if len(entities) >= n {
		return entities[:n], nil
	}

	trendingPosts, err := q.GetFeedEntityScores(ctx, db.GetFeedEntityScoresParams{WindowEnd: time.Now().Add(-lookbackWindow)})
	if err != nil {
		return nil, err
	}

	trendingPosts = util.Filter(trendingPosts, func(p db.GetFeedEntityScoresRow) bool { return !p.IsGalleryPost && !p.Post.Deleted }, true)
	sort.Slice(trendingPosts, func(i, j int) bool {
		return trendingPosts[i].FeedEntityScore.Interactions > trendingPosts[j].FeedEntityScore.Interactions
	})

	trendingIDs := util.MapWithoutError(trendingPosts, func(p db.GetFeedEntityScoresRow) persist.DBID { return p.Post.ID })
	addPosts(ctx, gql, &entities, added, n, trendingIDs, false)
	return entities, nil
}

func addGalleries(
	ctx context.Context,
	gql *graphql.Client,
	spotlightGalleries *[]GalleryDigestEntity,
	added map[persist.DBID]bool,
	n int,
	entityIDs []persist.DBID,
	editorialized bool,
) {
	var mu sync.Mutex
	batch := make([]persist.DBID, 0, 4)
	var i int
	var wg conc.WaitGroup

	for len(*spotlightGalleries) < n && i < len(entityIDs)-1 {
		// fill the batch not exceeding the number of entities needed
		for j := 0; j < 4 && (len(batch)+len(*spotlightGalleries)) < n; j++ {
			batch = append(batch, entityIDs[i])
			i++
		}

		// run the batch
		for i := 0; i < len(batch); i++ {
			i := i
			wg.Go(func() {
				entity, err := galleryToEntity(ctx, *gql, batch[i], editorialized)
				if err != nil {
					logger.For(ctx).Error(err)
					return
				}
				mu.Lock()
				*spotlightGalleries = append(*spotlightGalleries, entity)
				added[batch[i]] = true
				mu.Unlock()
			})
		}

		wg.Wait()

		// clear the batch
		batch = batch[:0]
	}
}

func addCommunities(
	ctx context.Context,
	gql *graphql.Client,
	spotlightCommunities *[]CommunityDigestEntity,
	added map[persist.DBID]bool,
	n int,
	entityIDs []persist.DBID,
	editorialized bool,
) {
	var mu sync.Mutex
	batch := make([]persist.DBID, 0, 4)
	var i int
	var wg conc.WaitGroup

	for len(*spotlightCommunities) < n && i < len(entityIDs)-1 {
		// fill the batch not exceeding the number of entities needed
		for j := 0; j < 4 && (len(batch)+len(*spotlightCommunities)) < n; j++ {
			batch = append(batch, entityIDs[i])
			i++
		}

		// run the batch
		for i := 0; i < len(batch); i++ {
			i := i
			wg.Go(func() {
				entity, err := communityToEntity(ctx, *gql, batch[i], editorialized)
				if err != nil {
					logger.For(ctx).Error(err)
					return
				}
				mu.Lock()
				*spotlightCommunities = append(*spotlightCommunities, entity)
				added[batch[i]] = true
				mu.Unlock()
			})
		}

		wg.Wait()

		// clear the batch
		batch = batch[:0]
	}
}

func addPosts(
	ctx context.Context,
	gql *graphql.Client,
	spotlightPosts *[]PostDigestEntity,
	added map[persist.DBID]bool,
	n int,
	entityIDs []persist.DBID,
	editorialized bool,
) {
	var mu sync.Mutex
	batch := make([]persist.DBID, 0, 4)
	var i int
	var wg conc.WaitGroup

	for len(*spotlightPosts) < n && i < len(entityIDs)-1 {
		// fill the batch not exceeding the number of entities needed
		for j := 0; j < 4 && (len(batch)+len(*spotlightPosts)) < n; j++ {
			batch = append(batch, entityIDs[i])
			i++
		}

		// run the batch
		for i := 0; i < len(batch); i++ {
			i := i
			wg.Go(func() {
				entity, err := postToEntity(ctx, *gql, batch[i], editorialized)
				if err != nil {
					logger.For(ctx).Error(err)
					return
				}
				mu.Lock()
				*spotlightPosts = append(*spotlightPosts, entity)
				added[batch[i]] = true
				mu.Unlock()
			})
		}

		wg.Wait()

		// clear the batch
		batch = batch[:0]
	}
}

func getOverrides(ctx context.Context, b *store.BucketStorer) (DigestValueOverrides, error) {
	exists, err := b.Exists(ctx, overrideFile)
	if err != nil {
		return DigestValueOverrides{}, err
	}

	if !exists {
		return DigestValueOverrides{}, nil
	}

	r, err := b.NewReader(ctx, overrideFile)
	if err != nil {
		return DigestValueOverrides{}, nil
	}

	defer r.Close()

	var o DigestValueOverrides
	err = util.UnmarshallBody(&o, r)

	return o, err
}

func galleryToEntity(ctx context.Context, gql graphql.Client, galleryID persist.DBID, editorialized bool) (GalleryDigestEntity, error) {
	r, err := galleryDigestEntityQuery(ctx, gql, galleryID)
	if err != nil {
		return GalleryDigestEntity{}, err
	}

	switch t := (*r.GalleryById).(type) {
	case *galleryDigestEntityQueryGalleryByIdGallery:
		entity := GalleryDigestEntity{
			GalleryID:     t.Dbid,
			Name:          util.GetOptionalValue(t.Name, ""),
			Description:   util.GetOptionalValue(t.Description, ""),
			Tokens:        make([]TokenDigestEntity, 0, len(t.TokenPreviews)),
			Editorialized: editorialized,
		}

		if t.Owner != nil {
			entity.CreatorUsername = util.GetOptionalValue(t.Owner.Username, "")
		}

		for _, t := range t.TokenPreviews {
			if t.Small != nil {
				entity.Tokens = append(entity.Tokens, TokenDigestEntity{
					Name:            "",    // not needed by template
					Description:     "",    // not needed by template
					Editorialized:   false, // not needed by template
					PreviewImageURL: util.GetOptionalValue(t.Small, ""),
				})
			}
		}

		if len(entity.Tokens) == 0 {
			return GalleryDigestEntity{}, errors.New("no token previews found for gallery")
		}

		// previews look jank if there are more than two previews
		if len(entity.Tokens) > 2 {
			entity.Tokens = entity.Tokens[:2]
		}

		return entity, nil
	case *galleryDigestEntityQueryGalleryByIdErrGalleryNotFound:
		return GalleryDigestEntity{}, errors.New(t.Message)
	default:
		return GalleryDigestEntity{}, fmt.Errorf("unexpected gallery response %T", t)
	}
}

func communityToEntity(ctx context.Context, gql graphql.Client, communityID persist.DBID, editorialized bool) (CommunityDigestEntity, error) {
	r, err := communityDigestEntityQuery(ctx, gql, communityID)
	if err != nil {
		return CommunityDigestEntity{}, err
	}

	switch t := (*r.CommunityById).(type) {
	case *communityDigestEntityQueryCommunityByIdCommunity:
		entity := CommunityDigestEntity{
			CommunityID:     t.Dbid,
			Name:            util.GetOptionalValue(t.Name, ""),
			Description:     util.GetOptionalValue(t.Description, ""),
			PreviewImageURL: util.GetOptionalValue(t.ProfileImageURL, ""),
			Editorialized:   editorialized,
		}
		switch c := (*t.Subtype).(type) {
		case *communityDigestEntityQueryCommunityByIdCommunitySubtypeArtBlocksCommunity:
			if c.Contract.ContractAddress != nil {
				entity.ContractAddress = util.GetOptionalValue(c.Contract.ContractAddress.Address, "")
				entity.ChainName = strings.ToLower(string(util.GetOptionalValue(c.Contract.ContractAddress.Chain, "")))
			}
		case *communityDigestEntityQueryCommunityByIdCommunitySubtypeContractCommunity:
			entity.ContractAddress = util.GetOptionalValue(c.Contract.ContractAddress.Address, "")
			entity.ChainName = strings.ToLower(string(util.GetOptionalValue(c.Contract.ContractAddress.Chain, "")))
		}
		if len(t.Creators) > 0 {
			switch creator := (*t.Creators[0]).(type) {
			case *communityDigestEntityQueryCommunityByIdCommunityCreatorsChainAddress:
				entity.CreatorName = creator.Address.String()
				entity.CreatorPreviewImageURL = ""
			case *communityDigestEntityQueryCommunityByIdCommunityCreatorsGalleryUser:
				entity.CreatorName = util.GetOptionalValue(creator.Username, "")
				if creator.ProfileImage != nil {
					switch pfp := (*creator.ProfileImage).(type) {
					case *communityDigestEntityQueryCommunityByIdCommunityCreatorsGalleryUserProfileImageTokenProfileImage:
						entity.CreatorPreviewImageURL = addImagePreview(*pfp.Token.Definition.Media)
						entity.CreatorPreviewImageURL = ""
					case *communityDigestEntityQueryCommunityByIdCommunityCreatorsGalleryUserProfileImageEnsProfileImage:
						if pfp.PfpToken != nil {
							entity.CreatorPreviewImageURL = addImagePreview(*pfp.PfpToken.Definition.Media)
						} else if pfp.EnsToken != nil && pfp.EnsToken.PreviewURLs != nil {
							entity.CreatorPreviewImageURL = util.GetOptionalValue(pfp.EnsToken.PreviewURLs.Small, "")
						}
					}
				}
			}
		}

		if t.Tokens != nil {
			for _, token := range t.Tokens.Edges {
				if token.Node != nil {
					entity.Tokens = append(entity.Tokens, tokenToEntity(token.Node.tokenFrag))
				}
			}
		}

		// validate
		if len(entity.Tokens) == 0 {
			return entity, errors.New("no tokens found for community")
		}
		if entity.ChainName == "" {
			return entity, errors.New("no chain name found for community")
		}
		if entity.ContractAddress == "" {
			return entity, errors.New("no contract address found for community")
		}

		// previews look jank if there are more than two previews
		if len(entity.Tokens) > 2 {
			entity.Tokens = entity.Tokens[:2]
		}

		return entity, nil
	case *communityDigestEntityQueryCommunityByIdErrCommunityNotFound:
		return CommunityDigestEntity{}, errors.New(t.Message)
	case *communityDigestEntityQueryCommunityByIdErrInvalidInput:
		return CommunityDigestEntity{}, errors.New(t.Message)
	default:
		return CommunityDigestEntity{}, fmt.Errorf("unexpected response %T", t)
	}
}

func postToEntity(ctx context.Context, gql graphql.Client, postID persist.DBID, editorialized bool) (PostDigestEntity, error) {
	r, err := postDigestEntityQuery(ctx, gql, postID)
	if err != nil {
		return PostDigestEntity{}, err
	}

	switch t := (*r.PostById).(type) {
	case *postDigestEntityQueryPostByIdPost:
		entity := PostDigestEntity{
			PostID:        postID,
			Caption:       util.GetOptionalValue(t.Caption, ""),
			Editorialized: editorialized,
		}
		if t.Author != nil {
			entity.AuthorUsername = util.GetOptionalValue(t.Author.Username, "")
			if t.Author.ProfileImage != nil {
				switch pfp := (*t.Author.ProfileImage).(type) {
				case *postDigestEntityQueryPostByIdPostAuthorGalleryUserProfileImageTokenProfileImage:
					entity.AuthorPFPURL = addImagePreview(*pfp.Token.Definition.Media)
				case *postDigestEntityQueryPostByIdPostAuthorGalleryUserProfileImageEnsProfileImage:
					if pfp.PfpToken != nil {
						entity.AuthorPFPURL = addImagePreview(*pfp.PfpToken.Definition.Media)
					} else if pfp.EnsToken != nil && pfp.EnsToken.PreviewURLs != nil {
						entity.AuthorPFPURL = util.GetOptionalValue(pfp.EnsToken.PreviewURLs.Small, "")
					}
				}
			}
		}
		for _, token := range t.Tokens {
			entity.Tokens = append(entity.Tokens, tokenToEntity(token.tokenFrag))
		}

		// validate
		if len(entity.Tokens) == 0 {
			return entity, errors.New("no tokens found for post")
		}

		return entity, nil
	case *postDigestEntityQueryPostByIdErrPostNotFound:
		return PostDigestEntity{}, errors.New(t.Message)
	case *postDigestEntityQueryPostByIdErrInvalidInput:
		return PostDigestEntity{}, errors.New(t.Message)
	default:
		return PostDigestEntity{}, fmt.Errorf("unexpected response %T", t)
	}
}

func tokenToEntity(t tokenFrag) TokenDigestEntity {
	return TokenDigestEntity{
		TokenID:         persist.DBID(t.Dbid),
		Name:            util.GetOptionalValue(t.Definition.Name, ""),
		Description:     util.GetOptionalValue(t.Definition.Description, ""),
		PreviewImageURL: addImagePreview(*t.Definition.Media),
		Editorialized:   false,
	}
}

func addImagePreview(m definitionFragMediaMediaSubtype) string {
	switch media := (m).(type) {
	case *definitionFragMediaAudioMedia:
		return util.GetOptionalValue(media.PreviewURLs.Small, util.GetOptionalValue(media.FallbackMedia.MediaURL, ""))
	case *definitionFragMediaGIFMedia:
		return util.GetOptionalValue(media.PreviewURLs.Small, util.GetOptionalValue(media.FallbackMedia.MediaURL, ""))
	case *definitionFragMediaGltfMedia:
		return util.GetOptionalValue(media.PreviewURLs.Small, util.GetOptionalValue(media.FallbackMedia.MediaURL, ""))
	case *definitionFragMediaHtmlMedia:
		return util.GetOptionalValue(media.PreviewURLs.Small, util.GetOptionalValue(media.FallbackMedia.MediaURL, ""))
	case *definitionFragMediaImageMedia:
		return util.GetOptionalValue(media.PreviewURLs.Small, util.GetOptionalValue(media.FallbackMedia.MediaURL, ""))
	case *definitionFragMediaInvalidMedia:
		return util.GetOptionalValue(media.PreviewURLs.Small, util.GetOptionalValue(media.FallbackMedia.MediaURL, ""))
	case *definitionFragMediaJsonMedia:
		return util.GetOptionalValue(media.PreviewURLs.Small, util.GetOptionalValue(media.FallbackMedia.MediaURL, ""))
	case *definitionFragMediaPdfMedia:
		return util.GetOptionalValue(media.PreviewURLs.Small, util.GetOptionalValue(media.FallbackMedia.MediaURL, ""))
	case *definitionFragMediaSyncingMedia:
		return util.GetOptionalValue(media.PreviewURLs.Small, util.GetOptionalValue(media.FallbackMedia.MediaURL, ""))
	case *definitionFragMediaTextMedia:
		return util.GetOptionalValue(media.PreviewURLs.Small, util.GetOptionalValue(media.FallbackMedia.MediaURL, ""))
	case *definitionFragMediaUnknownMedia:
		return util.GetOptionalValue(media.PreviewURLs.Small, util.GetOptionalValue(media.FallbackMedia.MediaURL, ""))
	case *definitionFragMediaVideoMedia:
		return util.GetOptionalValue(media.PreviewURLs.Small, util.GetOptionalValue(media.FallbackMedia.MediaURL, ""))
	}
	return ""
}

func defaultIntroText(username string) string {
	if username == "" {
		return "This is your weekly digest."
	}
	return fmt.Sprintf("Hey %s, this is your weekly digest.", username)
}
