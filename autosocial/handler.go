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
		fp := pool.New().WithMaxGoroutines(3).WithErrors().WithContext(c)

		for u, s := range in.Users {
			userID := u
			socials := s

			if addresses, ok := socials[persist.SocialProviderLens]; ok {
				lp.Go(func(ctx context.Context) error {
					return addLensProfileToUser(ctx, l, addresses, q, userID)
				})
			}

			if addresses, ok := socials[persist.SocialProviderFarcaster]; ok {
				fp.Go(func(ctx context.Context) error {
					return addFarcasterProfileToUser(ctx, n, addresses, q, userID)
				})
			}
		}

		errs := make([]error, 0, 2)
		err := fp.Wait()
		if err != nil {
			errs = append(errs, err)
		}

		err = lp.Wait()
		if err != nil {
			errs = append(errs, err)
		}

		if len(errs) > 0 {
			util.ErrResponse(c, http.StatusInternalServerError, util.MultiErr(errs))
			return
		}

		c.JSON(http.StatusOK, util.SuccessResponse{Success: true})
	}
}

func addLensProfileToUser(ctx context.Context, l *lens.LensAPI, address []persist.ChainAddress, q *coredb.Queries, userID persist.DBID) error {
	for _, a := range address {
		if a.Address() == "" {
			continue
		}
		u, err := l.DefaultProfileByAddress(ctx, a.Address())
		if err != nil {
			if strings.Contains(err.Error(), "too many requests") {
				time.Sleep(4 * time.Minute)
				u, err = l.DefaultProfileByAddress(ctx, a.Address())
				if err != nil {
					logrus.Error(err)
					continue
				}
			} else {
				continue
			}
		}
		if u.ID == "" {
			continue
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

	}
	return nil
}

func addFarcasterProfileToUser(ctx context.Context, n *farcaster.NeynarAPI, address []persist.ChainAddress, q *coredb.Queries, userID persist.DBID) error {
	for _, a := range address {
		if a.Address() == "" {
			continue
		}
		u, err := n.UserByAddress(ctx, a.Address())
		if err != nil {
			logrus.Error(err)
			continue
		}
		if u.Fid.String() == "" {
			continue
		}

		logrus.Infof("got farcaster user %s %s %s %s", u.Username, u.DisplayName, u.Pfp.URL, u.Profile.Bio.Text)
		return q.AddSocialToUser(ctx, coredb.AddSocialToUserParams{
			UserID: userID,
			Socials: persist.Socials{
				persist.SocialProviderFarcaster: persist.SocialUserIdentifiers{
					Provider: persist.SocialProviderFarcaster,
					ID:       u.Fid.String(),
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
	}
	return nil
}
