package orchestrator

import (
	"net/http"

	cloudtasks "cloud.google.com/go/cloudtasks/apiv2"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v4/pgxpool"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/service/task"
	"github.com/mikeydub/go-gallery/util"
	"github.com/sourcegraph/conc/pool"
)

type idAddressTuple struct {
	ID      persist.DBID
	Address persist.ChainAddress
	Social  persist.SocialProvider
}

func processAllUsers(pg *pgxpool.Pool, ctc *cloudtasks.Client) gin.HandlerFunc {
	return func(c *gin.Context) {

		noSocials, err := pg.Query(c, `select u.id, w.address, u.pii_socials->>'Lens' is null, u.pii_socials->>'Farcaster' is null from users_view u join wallets w on w.id = any(u.wallets) where u.deleted = false and w.chain = 0 and w.deleted = false and u.universal = false and (u.pii_socials->>'Lens' is null or u.pii_socials->>'Farcaster' is null) order by u.created_at desc;`)
		if err != nil {
			util.ErrResponse(c, http.StatusInternalServerError, err)
			return
		}

		rec := make(chan []idAddressTuple)
		errs := make(chan error)

		go func() {
			defer close(rec)
			var tuples []idAddressTuple
			for i := 0; noSocials.Next(); i++ {
				var userID persist.DBID
				var walletAddress persist.Address
				var noLens bool
				var noFarcaster bool

				err := noSocials.Scan(&userID, &walletAddress, &noLens, &noFarcaster)
				if err != nil {
					errs <- err
					return
				}

				if noLens {
					tuples = append(tuples, idAddressTuple{ID: userID, Address: persist.NewChainAddress(walletAddress, persist.ChainETH), Social: persist.SocialProviderLens})
				}

				if noFarcaster {
					tuples = append(tuples, idAddressTuple{ID: userID, Address: persist.NewChainAddress(walletAddress, persist.ChainETH), Social: persist.SocialProviderFarcaster})
				}

				if i%100 == 0 {
					rec <- tuples
					tuples = nil
				}
			}
		}()

		activeRequests := pool.New().WithMaxGoroutines(100).WithErrors().WithContext(c)

		var allErrs []error
		send := make(map[persist.DBID]map[persist.SocialProvider]persist.ChainAddress)
		for {
			select {
			case tuples, ok := <-rec:
				if ok {
					for _, t := range tuples {
						if _, ok := send[t.ID]; !ok {
							send[t.ID] = make(map[persist.SocialProvider]persist.ChainAddress)
						}
						send[t.ID][t.Social] = t.Address
					}
					if len(send) >= 200 {
						copy := send

						err = task.CreateTaskForAutosocialProcessUsers(c, task.AutosocialProcessUsersMessage{
							Users: copy,
						}, ctc)
						if err != nil {
							allErrs = append(allErrs, err)
						}

						send = make(map[persist.DBID]map[persist.SocialProvider]persist.ChainAddress)
					}
				} else {
					if len(send) > 0 {
						copy := send

						err = task.CreateTaskForAutosocialProcessUsers(c, task.AutosocialProcessUsersMessage{
							Users: copy,
						}, ctc)
						if err != nil {
							allErrs = append(allErrs, err)
						}

					}

					finalErr := activeRequests.Wait()
					if finalErr != nil {
						allErrs = append(allErrs, finalErr)
					}

					if len(allErrs) > 0 {
						// statusOK because we don't want to retry
						util.ErrResponse(c, http.StatusOK, util.MultiErr(allErrs))
						return
					}
					c.JSON(http.StatusOK, util.SuccessResponse{Success: true})
					return
				}
			case err := <-errs:
				allErrs = append(allErrs, err)
			}
		}
	}
}
