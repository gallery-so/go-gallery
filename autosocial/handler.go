package autosocial

import (
	"context"
	"fmt"
	"github.com/mikeydub/go-gallery/service/auth"
	"github.com/mikeydub/go-gallery/service/persist/postgres"
	"github.com/mikeydub/go-gallery/service/user"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/mikeydub/go-gallery/db/gen/coredb"
	"github.com/mikeydub/go-gallery/service/farcaster"
	"github.com/mikeydub/go-gallery/service/lens"
	"github.com/mikeydub/go-gallery/service/logger"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/service/task"
	"github.com/mikeydub/go-gallery/util"
	"github.com/sirupsen/logrus"
	"github.com/sourcegraph/conc/pool"
)

func processUsers(q *coredb.Queries, n *farcaster.NeynarAPI, l *lens.LensAPI, repos *postgres.Repositories) gin.HandlerFunc {
	return func(c *gin.Context) {
		var in task.AutosocialProcessUsersMessage
		if err := c.ShouldBindJSON(&in); err != nil {
			util.ErrResponse(c, http.StatusBadRequest, err)
			return
		}

		logger.For(c).Infof("processing socials for %d users", len(in.Users))

		lp := pool.New().WithMaxGoroutines(3).WithErrors().WithContext(c)
		fp := pool.New().WithMaxGoroutines(3).WithErrors().WithContext(c)

		fcImportWalletsLookup := make(map[persist.DBID]bool)
		for u, s := range in.ImportSocialWallets {
			if _, ok := s[persist.SocialProviderFarcaster]; ok {
				fcImportWalletsLookup[u] = s[persist.SocialProviderFarcaster]
			}
		}

		// chunk users in groups of 350 for farcaster
		userLookup := make(map[persist.Address]persist.DBID, 350)
		chunkedAddresses := make([]persist.Address, 0, 350)
		cur := 0
		for u, s := range in.Users {
			userID := u
			socials := s
			if userAddresses, ok := socials[persist.SocialProviderFarcaster]; ok {
				for _, a := range userAddresses {
					if cur == 350 {
						copylookup := userLookup
						copychunked := chunkedAddresses
						fp.Go(func(ctx context.Context) error {
							return addFarcasterProfilesToUsers(c, n, copychunked, q, copylookup, fcImportWalletsLookup, repos.UserRepository)
						})
						userLookup = make(map[persist.Address]persist.DBID, 350)
						chunkedAddresses = make([]persist.Address, 0, 350)
						cur = 0
					}
					userLookup[a.Address()] = userID
					chunkedAddresses = append(chunkedAddresses, a.Address())
					cur++
				}
			}
		}

		if cur > 0 {
			fp.Go(func(ctx context.Context) error {
				return addFarcasterProfilesToUsers(c, n, chunkedAddresses, q, userLookup, fcImportWalletsLookup, repos.UserRepository)
			})
		}

		// process lens profiles one user at a time
		for u, s := range in.Users {
			userID := u
			socials := s

			if addresses, ok := socials[persist.SocialProviderLens]; ok {
				lp.Go(func(ctx context.Context) error {
					return addLensProfileToUser(ctx, l, addresses, q, userID)
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

func checkFarcasterApproval(q *coredb.Queries, n *farcaster.NeynarAPI) gin.HandlerFunc {
	return func(c *gin.Context) {
		var in task.AutosocialPollFarcasterMessage
		if err := c.ShouldBindQuery(&in); err != nil {
			util.ErrResponse(c, http.StatusBadRequest, err)
			return
		}

		s, err := n.GetSignerByUUID(c, in.SignerUUID)
		if err != nil {
			util.ErrResponse(c, http.StatusInternalServerError, err)
			return
		}

		logger.For(c).Infof("farcaster signer status for %s: %s", in.SignerUUID, s.Status)

		if s.Status != "approved" {
			util.ErrResponse(c, http.StatusInternalServerError, fmt.Errorf("signer status is %s", s.Status))
			return
		}

		user, err := q.GetSocialsByUserID(c, in.UserID)
		if err != nil {
			util.ErrResponse(c, http.StatusInternalServerError, err)
			return
		}
		far, ok := user[persist.SocialProviderFarcaster]
		if !ok {
			util.ErrResponse(c, http.StatusInternalServerError, fmt.Errorf("user does not have farcaster social"))
			return
		}

		far.Metadata["signer_status"] = s.Status

		err = q.AddSocialToUser(c, coredb.AddSocialToUserParams{
			UserID: in.UserID,
			Socials: persist.Socials{
				persist.SocialProviderFarcaster: far,
			},
		})
		if err != nil {
			util.ErrResponse(c, http.StatusInternalServerError, err)
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

func addFarcasterProfilesToUsers(ctx context.Context, n *farcaster.NeynarAPI, addresses []persist.Address, q *coredb.Queries, userLookup map[persist.Address]persist.DBID,
	importWalletsLookup map[persist.DBID]bool, userRepo *postgres.UserRepository) error {
	users, err := n.UsersByAddresses(ctx, addresses, false)
	if err != nil {
		return err
	}
	for address, fusers := range users {
		if len(fusers) == 0 {
			continue
		}
		// we only store one farcaster profile per user
		u := fusers[0]

		for _, fuser := range fusers {
			if fuser.Fid.String() != "" {
				u = fuser
				break
			}
		}

		if u.Fid.String() == "" {
			continue
		}

		guser, ok := userLookup[address]
		if !ok {
			guser, ok = userLookup[persist.Address(strings.ToLower(address.String()))]
			if !ok {
				continue
			}
		}

		logrus.Infof("got farcaster user %s %s %s %s", u.Username, u.DisplayName, u.Pfp.URL, u.Profile.Bio.Text)
		err := q.AddSocialToUser(ctx, coredb.AddSocialToUserParams{
			UserID: guser,
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
		if err != nil {
			return err
		}

		if importWalletsLookup[guser] {
			// Neynar says the addresses are verified, so we'll use a trivial wrapper authenticator that
			// accepts what Neynar says and doesn't require further verification of addresses
			authenticator := addressAuthenticator{u.VerifiedAddresses.EthAddresses}
			for _, addr := range u.VerifiedAddresses.EthAddresses {
				err = user.AddWalletToUser(ctx, guser, persist.NewChainAddress(addr, persist.ChainETH), authenticator, userRepo, nil)
				if err != nil {
					err = fmt.Errorf("failed to add Neynar verified wallet (address: %s) to user (id: %s): %w", addr, guser, err)
					logger.For(ctx).Error(err)
				}
			}
		}
	}
	return nil
}

type addressAuthenticator struct {
	addresses []persist.Address
}

func (a addressAuthenticator) GetDescription() string { return "Autosocial Address Authenticator" }

func (a addressAuthenticator) Authenticate(ctx context.Context) (*auth.AuthResult, error) {
	result := &auth.AuthResult{}
	for _, address := range a.addresses {
		result.Addresses = append(result.Addresses, auth.AuthenticatedAddress{
			ChainAddress: persist.NewChainAddress(address, persist.ChainETH),
			WalletType:   persist.WalletTypeEOA,
		})
	}

	return result, nil
}
