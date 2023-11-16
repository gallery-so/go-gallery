package activitystats

import (
	"encoding/json"
	"net/http"

	"cloud.google.com/go/storage"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v4/pgxpool"
	"github.com/mikeydub/go-gallery/db/gen/coredb"
	"github.com/mikeydub/go-gallery/env"
	"github.com/mikeydub/go-gallery/event"
	"github.com/mikeydub/go-gallery/service/logger"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/util"
)

const topConfFileName = "top_activity_badges.json"

type TopActivityConfiguration struct {
	AdmiresGivenWeight     int32 `json:"admires_given_weight"`
	AdmiresReceivedWeight  int32 `json:"admires_received_weight"`
	CommentsMadeWeight     int32 `json:"comments_made_weight"`
	CommentsReceivedWeight int32 `json:"comments_received_weight"`
	Total                  int32 `json:"total"`
}

func calculateTopActivityBadges(q *coredb.Queries, stg *storage.Client, pgx *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		confR, err := stg.Bucket(env.GetString("CONFIGURATION_BUCKET")).Object(topConfFileName).NewReader(c)
		if err != nil {
			util.ErrResponse(c, http.StatusInternalServerError, err)
			return
		}

		var conf TopActivityConfiguration
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

		top, err := qtx.GetMostActiveUsers(c, coredb.GetMostActiveUsersParams{
			Limit:                  conf.Total,
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
		top = util.Filter(top, func(r coredb.GetMostActiveUsersRow) bool {
			return r.ActorID != "" && r.Score > 0
		}, false)

		logger.For(c).Debugf("top: %d %+v", len(top), top)

		userIDs := util.MapWithoutError(top, func(r coredb.GetMostActiveUsersRow) persist.DBID {
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

		for _, r := range top {
			err := event.Dispatch(c, coredb.Event{
				ID:             persist.GenerateID(),
				ActorID:        util.ToNullString(r.ActorID.String(), true),
				ResourceTypeID: persist.ResourceTypeUser,
				UserID:         r.ActorID,
				SubjectID:      r.ActorID,
				Action:         persist.ActionTopActivityBadgeReceived,
				Data: persist.EventData{
					ActivityBadgeThreshold: int(conf.Total),
				},
			})
			if err != nil {
				logger.For(c).Errorf("error dispatching event: %s", err)
			}
		}

		c.JSON(http.StatusOK, top)
	}
}

func updateTopActivityConfiguration(stg *storage.Client) gin.HandlerFunc {
	return func(c *gin.Context) {
		var conf TopActivityConfiguration
		if err := c.ShouldBindJSON(&conf); err != nil {
			util.ErrResponse(c, http.StatusBadRequest, err)
			return
		}

		confW := stg.Bucket(env.GetString("CONFIGURATION_BUCKET")).Object(topConfFileName).NewWriter(c)
		if err := json.NewEncoder(confW).Encode(&conf); err != nil {
			util.ErrResponse(c, http.StatusInternalServerError, err)
			return
		}

		if err := confW.Close(); err != nil {
			util.ErrResponse(c, http.StatusInternalServerError, err)
			return
		}

		c.JSON(http.StatusOK, conf)
	}
}

func getTopActivityConfiguration(stg *storage.Client) gin.HandlerFunc {
	return func(c *gin.Context) {
		confR, err := stg.Bucket(env.GetString("CONFIGURATION_BUCKET")).Object(topConfFileName).NewReader(c)
		if err != nil {
			util.ErrResponse(c, http.StatusInternalServerError, err)
			return
		}

		var conf TopActivityConfiguration
		if err := json.NewDecoder(confR).Decode(&conf); err != nil {
			util.ErrResponse(c, http.StatusInternalServerError, err)
			return
		}

		c.JSON(http.StatusOK, conf)
	}
}
