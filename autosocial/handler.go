package autosocial

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/mikeydub/go-gallery/db/gen/coredb"
	"github.com/mikeydub/go-gallery/service/farcaster"
	"github.com/mikeydub/go-gallery/service/lens"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/service/task"
	"github.com/mikeydub/go-gallery/util"
	"github.com/sirupsen/logrus"
	"github.com/sourcegraph/conc/pool"
)

func processUsers(q *coredb.Queries, n *farcaster.NeynarAPI, l *lens.LensAPI) gin.HandlerFunc {
	return func(c *gin.Context) {
		var in task.AutosocialProcessUsersMessage
		if err := c.ShouldBindJSON(&in); err != nil {
			util.ErrResponse(c, http.StatusBadRequest, err)
			return
		}

		lp := pool.New().WithMaxGoroutines(3).WithErrors().WithContext(c)
		fp := pool.New().WithMaxGoroutines(20).WithErrors().WithContext(c)

		for u, s := range in.Users {
			userID := u
			socials := s

			if address, ok := socials[persist.SocialProviderLens]; ok && address.Address() != "" {

				lp.Go(func(ctx context.Context) error {
					u, err := l.DefaultProfileByAddress(ctx, address.Address())
					if err != nil {
						if strings.Contains(err.Error(), "too many requests") {
							time.Sleep(4 * time.Minute)
							u, err = l.DefaultProfileByAddress(ctx, address.Address())
							if err != nil {
								logrus.Error(err)
								return nil
							}
						} else {
							return nil
						}
					}
					logrus.Infof("got lens user %s %s %s %s", u.Name, u.Handle, u.Picture.Optimized.URL, u.Bio)
					return q.AddSocialToUser(ctx, coredb.AddSocialToUserParams{
						UserID: userID,
						Socials: persist.Socials{
							persist.SocialProviderLens: persist.SocialUserIdentifiers{
								Provider: persist.SocialProviderLens,
								ID:       u.ID,
								Display:  true,
								Metadata: map[string]interface{}{
									"username":          u.Handle,
									"name":              util.FirstNonEmptyString(u.Name, u.Handle),
									"profile_image_url": util.FirstNonEmptyString(u.Picture.Optimized.URL, u.Picture.URI),
									"bio":               u.Bio,
								},
							},
						},
					})

				})
			}

			if address, ok := socials[persist.SocialProviderFarcaster]; ok && address.Address() != "" {
				fp.Go(func(ctx context.Context) error {
					u, err := n.UserByAddress(ctx, address.Address())
					if err != nil {
						return nil
					}
					logrus.Infof("got farcaster user %s %s %s %s", u.Username, u.DisplayName, u.Pfp.URL, u.Profile.Bio.Text)

					return q.AddSocialToUser(ctx, coredb.AddSocialToUserParams{
						UserID: userID,
						Socials: persist.Socials{
							persist.SocialProviderFarcaster: persist.SocialUserIdentifiers{
								Provider: persist.SocialProviderFarcaster,
								ID:       u.Fid,
								Display:  true,
								Metadata: map[string]interface{}{
									"username":          u.Username,
									"name":              u.DisplayName,
									"profile_image_url": u.Pfp.URL,
									"bio":               u.Profile.Bio.Text,
								},
							},
						},
					})

				})
			}
		}

		err := fp.Wait()
		if err != nil {
			util.ErrResponse(c, http.StatusInternalServerError, err)
			return
		}

		err = lp.Wait()
		if err != nil {
			util.ErrResponse(c, http.StatusInternalServerError, err)
			return
		}

		c.JSON(http.StatusOK, util.SuccessResponse{Success: true})

	}
}
