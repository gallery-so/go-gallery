package orchestrator

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v4/pgxpool"
	"github.com/mikeydub/go-gallery/service/logger"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/service/task"
	"github.com/mikeydub/go-gallery/util"
)

type idAddressTuple struct {
	ID      persist.DBID
	Address persist.ChainAddress
	Social  persist.SocialProvider
}

func processAllUsers(pg *pgxpool.Pool, tc *task.Client) gin.HandlerFunc {
	return func(c *gin.Context) {

		noSocials, err := pg.Query(c, `select u.id, w.address, u.pii_socials->>'Lens' is null, u.pii_socials->>'Farcaster' is null from pii.user_view u join wallets w on w.id = any(u.wallets) where u.deleted = false and w.l1_chain = 0 and w.deleted = false and u.universal = false and (u.pii_socials->>'Lens' is null or u.pii_socials->>'Farcaster' is null) order by u.created_at desc;`)
		if err != nil {
			util.ErrResponse(c, http.StatusInternalServerError, err)
			return
		}

		send := make(map[persist.DBID]map[persist.SocialProvider][]persist.ChainAddress)
		var allErrs []error

		for i := 0; noSocials.Next(); i++ {
			var userID persist.DBID
			var walletAddress persist.Address
			var noLens bool
			var noFarcaster bool

			err := noSocials.Scan(&userID, &walletAddress, &noLens, &noFarcaster)
			if err != nil {
				logger.For(c).Error(err)
				allErrs = append(allErrs, err)
				continue
			}

			if walletAddress == "" {
				continue
			}

			if noLens {
				it, ok := send[userID]
				if !ok {
					it = make(map[persist.SocialProvider][]persist.ChainAddress)
				}
				it[persist.SocialProviderLens] = append(it[persist.SocialProviderLens], persist.NewChainAddress(walletAddress, persist.ChainETH))
				send[userID] = it
			}

			if noFarcaster {
				it, ok := send[userID]
				if !ok {
					it = make(map[persist.SocialProvider][]persist.ChainAddress)
				}
				it[persist.SocialProviderFarcaster] = append(it[persist.SocialProviderFarcaster], persist.NewChainAddress(walletAddress, persist.ChainETH))
				send[userID] = it
			}

			if len(send) >= 200 {
				err = tc.CreateTaskForAutosocialProcessUsers(c, task.AutosocialProcessUsersMessage{
					Users: send,
				})
				if err != nil {
					allErrs = append(allErrs, err)
				}

				send = make(map[persist.DBID]map[persist.SocialProvider][]persist.ChainAddress)
			}
		}
		if len(send) > 0 {
			err = tc.CreateTaskForAutosocialProcessUsers(c, task.AutosocialProcessUsersMessage{
				Users: send,
			})
			if err != nil {
				allErrs = append(allErrs, err)
			}
		}

		if len(allErrs) > 0 {
			// statusOK because we don't want to retry
			util.ErrResponse(c, http.StatusOK, util.MultiErr(allErrs))
			return
		}
		c.JSON(http.StatusOK, util.SuccessResponse{Success: true})
		return

	}
}
