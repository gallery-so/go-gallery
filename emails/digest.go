package emails

import (
	"encoding/json"
	"fmt"
	"math"
	"net/http"

	"cloud.google.com/go/storage"
	"github.com/gin-gonic/gin"
	"github.com/mikeydub/go-gallery/db/gen/coredb"
	"github.com/mikeydub/go-gallery/env"
	"github.com/mikeydub/go-gallery/publicapi"
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
	TopPosts       []Selected `json:"posts"`
	TopCollections []Selected `json:"collections"`
	TopGalleries   []Selected `json:"galleries"`
	TopFirstPosts  []Selected `json:"first_posts"`
}

type SelectedID struct {
	ID       persist.DBID `json:"id"`
	Position int          `json:"position"`
}

type DigestValueOverrides struct {
	TopPosts       []SelectedID `json:"posts"`
	TopCollections []SelectedID `json:"collections"`
	TopGalleries   []SelectedID `json:"galleries"`
	TopFirstPosts  []SelectedID `json:"first_posts"`
}

const overrideFile = "email_digest_overrides.json"

func getDigestValues(q *coredb.Queries, stg *storage.Client, f *publicapi.FeedAPI) gin.HandlerFunc {
	return func(c *gin.Context) {

		// mimic backend auth with no signed in user
		c.Set("auth.auth_error", nil)
		c.Set("auth.user_id", persist.DBID(""))

		_, err := stg.Bucket(env.GetString("CONFIGURATION_BUCKET")).Object(overrideFile).Attrs(c)
		if err != nil && err != storage.ErrObjectNotExist {
			util.ErrResponse(c, http.StatusInternalServerError, fmt.Errorf("error getting overrides attrs: %v", err))
			return
		}

		if err == storage.ErrObjectNotExist {
			w := stg.Bucket(env.GetString("CONFIGURATION_BUCKET")).Object(overrideFile).NewWriter(c)
			err = json.NewEncoder(w).Encode(DigestValueOverrides{})
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

		reader, err := stg.Bucket(env.GetString("CONFIGURATION_BUCKET")).Object(overrideFile).NewReader(c)
		if err != nil {
			util.ErrResponse(c, http.StatusInternalServerError, fmt.Errorf("error getting overrides: %v", err))
			return
		}

		var overrides DigestValueOverrides
		err = json.NewDecoder(reader).Decode(&overrides)
		if err != nil {
			util.ErrResponse(c, http.StatusInternalServerError, fmt.Errorf("error decoding overrides: %v", err))
			return
		}

		trendingFeed, _, err := f.TrendingFeed(c, nil, nil, util.ToPointer(10), nil)
		if err != nil {
			util.ErrResponse(c, http.StatusInternalServerError, fmt.Errorf("error getting trending feed: %v", err))
			return
		}

		topPosts := util.Filter(util.MapWithoutError(trendingFeed, func(a any) any {
			if post, ok := a.(coredb.Post); ok {
				return post
			}
			return coredb.Post{}
		}), func(p any) bool {
			return p.(coredb.Post).ID != ""
		}, false)

		selectedPosts := selectResults(topPosts, overrides.TopPosts, func(s SelectedID) Selected {
			p, err := q.GetPostByID(c, s.ID)
			if err != nil {
				return Selected{}
			}
			return Selected{
				Entity:   p,
				Position: &s.Position,
			}
		})

		topCollections, err := q.GetTopCommunitiesByPosts(c, 10)
		if err != nil {
			util.ErrResponse(c, http.StatusInternalServerError, err)
			return
		}

		selectedCollections := selectResults(util.MapWithoutError(topCollections, func(c coredb.GetTopCommunitiesByPostsRow) any { return c }), overrides.TopCollections, func(s SelectedID) Selected {
			c, err := q.GetContractByID(c, s.ID)
			if err != nil {
				return Selected{}
			}
			return Selected{
				Entity:   c,
				Position: &s.Position,
			}
		})

		c.JSON(http.StatusOK, DigestValues{
			TopPosts:       selectedPosts,
			TopCollections: selectedCollections,
			// TODO top galleries and top first posts
		})
	}
}

func selectResults(initial []any, overrides []SelectedID, overrideFetcher func(s SelectedID) Selected) []Selected {
	selectedResults := make([]Selected, int(math.Max(float64(len(initial)), float64(len(overrides)))))
	for _, post := range overrides {
		selectedResults[post.Position] = overrideFetcher(post)
	}
outer:
	for i, it := range initial {
		ic := i
		if selectedResults[i].Position != nil && i < 5 {
			// add to the next available position while keeping the order, if we exceed 5, append to the end still without a position
			// also ensure that i is updated so that we don't overwrite the same position in the next loop
			for j := i; j < 5; j++ {
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
		if i < 5 {
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
