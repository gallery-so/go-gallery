package activitystats

import (
	"encoding/json"
	"net/http"

	"cloud.google.com/go/storage"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v4/pgxpool"
	"github.com/mikeydub/go-gallery/db/gen/coredb"
	"github.com/mikeydub/go-gallery/env"
	"github.com/mikeydub/go-gallery/service/logger"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/util"
)

const top100ConfFile = "100_activity_badges.json"

type Top100ActivityConfiguration struct {
	AdmiresGivenWeight     int32 `json:"admires_given_weight"`
	AdmiresReceivedWeight  int32 `json:"admires_received_weight"`
	CommentsMadeWeight     int32 `json:"comments_made_weight"`
	CommentsReceivedWeight int32 `json:"comments_received_weight"`
}

func calculateTop100ActivityBadges(q *coredb.Queries, stg *storage.Client, pgx *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		confR, err := stg.Bucket(env.GetString("CONFIGURATION_BUCKET")).Object(top100ConfFile).NewReader(c)
		if err != nil {
			util.ErrResponse(c, http.StatusInternalServerError, err)
			return
		}

		var conf Top100ActivityConfiguration
		if err := json.NewDecoder(confR).Decode(&conf); err != nil {
			util.ErrResponse(c, http.StatusInternalServerError, err)
			return
		}

		tx, err := pgx.Begin(c)
		if err != nil {
			util.ErrResponse(c, http.StatusInternalServerError, err)
			return
		}
		defer tx.Rollback(c)

		qtx := q.WithTx(tx)

		top100, err := qtx.GetMostActiveUsers(c, coredb.GetMostActiveUsersParams{
			Limit:                  100,
			AdmireReceivedWeight:   conf.AdmiresReceivedWeight,
			AdmireGivenWeight:      conf.AdmiresGivenWeight,
			CommentsMadeWeight:     conf.CommentsMadeWeight,
			CommentsReceivedWeight: conf.CommentsReceivedWeight,
		})
		if err != nil {
			util.ErrResponse(c, http.StatusInternalServerError, err)
			return
		}

		// filter out empties
		top100 = util.Filter(top100, func(r coredb.GetMostActiveUsersRow) bool {
			return r.ActorID != "" && r.Score > 0
		}, false)

		logger.For(c).Debugf("top100: %d %+v", len(top100), top100)

		userIDs := util.MapWithoutError(top100, func(r coredb.GetMostActiveUsersRow) persist.DBID {
			return r.ActorID
		})

		err = qtx.UpdateTop100Users(c, userIDs)
		if err != nil {
			util.ErrResponse(c, http.StatusInternalServerError, err)
			return
		}

		err = tx.Commit(c)
		if err != nil {
			util.ErrResponse(c, http.StatusInternalServerError, err)
			return
		}

		c.JSON(http.StatusOK, top100)
	}
}

func updateTop100ActivityConfiguration(stg *storage.Client) gin.HandlerFunc {
	return func(c *gin.Context) {
		conf := Top100ActivityConfiguration{}
		if err := c.ShouldBindJSON(&conf); err != nil {
			util.ErrResponse(c, http.StatusBadRequest, err)
			return
		}

		confW := stg.Bucket(env.GetString("CONFIGURATION_BUCKET")).Object(top100ConfFile).NewWriter(c)
		if err := json.NewEncoder(confW).Encode(&conf); err != nil {
			util.ErrResponse(c, http.StatusInternalServerError, err)
			return
		}

		if err := confW.Close(); err != nil {
			util.ErrResponse(c, http.StatusInternalServerError, err)
			return
		}

	}
}

func getTop100ActivityConfiguration(stg *storage.Client) gin.HandlerFunc {
	return func(c *gin.Context) {
		confR, err := stg.Bucket(env.GetString("CONFIGURATION_BUCKET")).Object(top100ConfFile).NewReader(c)
		if err != nil {
			util.ErrResponse(c, http.StatusInternalServerError, err)
			return
		}

		var conf Top100ActivityConfiguration
		if err := json.NewDecoder(confR).Decode(&conf); err != nil {
			util.ErrResponse(c, http.StatusInternalServerError, err)
			return
		}

		c.JSON(http.StatusOK, conf)
	}
}
