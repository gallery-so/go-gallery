package socialauth

import (
	"context"
	"fmt"
	"net/http"

	cloudtasks "cloud.google.com/go/cloudtasks/apiv2"
	"github.com/mikeydub/go-gallery/db/gen/coredb"
	"github.com/mikeydub/go-gallery/service/farcaster"
	"github.com/mikeydub/go-gallery/service/lens"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/service/redis"
	"github.com/mikeydub/go-gallery/service/task"
	"github.com/mikeydub/go-gallery/service/twitter"
	"github.com/mikeydub/go-gallery/util"
)

type SocialAuthResult struct {
	Provider persist.SocialProvider `json:"provider" binding:"required"`
	ID       string                 `json:"id" binding:"required"`
	Metadata map[string]interface{} `json:"metadata"`
}

type Authenticator interface {
	Authenticate(context.Context) (*SocialAuthResult, error)
}

type TwitterAuthenticator struct {
	Queries *coredb.Queries
	Redis   *redis.Cache

	UserID   persist.DBID
	AuthCode string
}

func (a TwitterAuthenticator) Authenticate(ctx context.Context) (*SocialAuthResult, error) {
	tAPI := twitter.NewAPI(a.Queries, a.Redis)

	ids, access, err := tAPI.GetAuthedUserFromCode(ctx, a.AuthCode)
	if err != nil {
		return nil, err
	}

	err = a.Queries.UpsertSocialOAuth(ctx, coredb.UpsertSocialOAuthParams{
		ID:           persist.GenerateID(),
		UserID:       a.UserID,
		Provider:     persist.SocialProviderTwitter,
		AccessToken:  util.ToNullString(access.AccessToken, false),
		RefreshToken: util.ToNullString(access.RefreshToken, false),
	})
	if err != nil {
		return nil, err
	}

	return &SocialAuthResult{
		Provider: persist.SocialProviderTwitter,
		ID:       ids.ID,
		Metadata: map[string]interface{}{
			"username":          ids.Username,
			"name":              ids.Name,
			"profile_image_url": ids.ProfileImageURL,
		},
	}, nil
}

type FarcasterAuthenticator struct {
	Queries    *coredb.Queries
	HTTPClient *http.Client
	TaskClient *cloudtasks.Client

	UserID     persist.DBID
	Address    persist.Address
	WithSigner bool
}

func (a FarcasterAuthenticator) Authenticate(ctx context.Context) (*SocialAuthResult, error) {
	api := farcaster.NewNeynarAPI(a.HTTPClient)
	user, err := a.Queries.GetUserByAddressAndL1(ctx, coredb.GetUserByAddressAndL1Params{
		Address: persist.Address(persist.ChainETH.NormalizeAddress(a.Address)),
		L1Chain: persist.L1Chain(persist.ChainETH),
	})
	if err != nil {
		return nil, fmt.Errorf("get user by address and l1: %w", err)
	}

	if user.ID != a.UserID {
		return nil, persist.ErrAddressNotOwnedByUser{
			ChainAddress: persist.NewChainAddress(a.Address, persist.ChainETH),
			UserID:       a.UserID,
		}
	}

	fu, err := api.UserByAddress(ctx, a.Address)
	if err != nil {
		return nil, fmt.Errorf("get user by address: %w", err)
	}

	res := &SocialAuthResult{
		Provider: persist.SocialProviderFarcaster,
		ID:       fu.Fid.String(),
		Metadata: map[string]interface{}{
			"username":          fu.Username,
			"name":              fu.DisplayName,
			"profile_image_url": fu.Pfp.URL,
			"bio":               fu.Profile.Bio.Text,
		},
	}

	if a.WithSigner {

		var signer farcaster.NeynarSigner

		curSigner, err := a.Queries.GetSocialAuthByUserID(ctx, coredb.GetSocialAuthByUserIDParams{
			UserID:   a.UserID,
			Provider: persist.SocialProviderFarcaster,
		})
		if curSigner.AccessToken.String != "" {
			maybeApprovedSigner, err := api.GetSignerByUUID(ctx, curSigner.AccessToken.String)
			if err != nil {
				return nil, fmt.Errorf("get signer by uuid: %w", err)
			}
			if signer.Status == "approved" {
				signer = maybeApprovedSigner
			}
		}

		if signer.Status != "approved" {
			signer, err = api.CreateSignerForUser(ctx, fu.Fid)
			if err != nil {
				return nil, fmt.Errorf("get signer by address: %w", err)
			}
		}

		err = a.Queries.UpsertSocialOAuth(ctx, coredb.UpsertSocialOAuthParams{
			ID:          persist.GenerateID(),
			UserID:      a.UserID,
			Provider:    persist.SocialProviderFarcaster,
			AccessToken: util.ToNullString(signer.SignerUUID, false),
		})
		if err != nil {
			return nil, err
		}

		res.Metadata["signer_status"] = signer.Status
		res.Metadata["approval_url"] = signer.SignerApprovalURL
		err = task.CreateTaskForAutosocialPollFarcaster(ctx, task.AutosocialPollFarcasterMessage{
			SignerUUID: signer.SignerUUID,
			UserID:     a.UserID,
		}, a.TaskClient)
		if err != nil {
			return nil, fmt.Errorf("create task for autosocial poll farcaster: %w", err)
		}
	}

	return res, nil

}

type LensAuthenticator struct {
	Queries    *coredb.Queries
	HTTPClient *http.Client

	UserID    persist.DBID
	Address   persist.Address
	Signature string
}

func (a LensAuthenticator) Authenticate(ctx context.Context) (*SocialAuthResult, error) {
	api := lens.NewAPI(a.HTTPClient)
	user, err := a.Queries.GetUserByAddressAndL1(ctx, coredb.GetUserByAddressAndL1Params{
		Address: a.Address,
		L1Chain: persist.L1Chain(persist.ChainETH),
	})
	if err != nil {
		return nil, err
	}

	if user.ID != a.UserID {
		return nil, persist.ErrAddressNotOwnedByUser{
			ChainAddress: persist.NewChainAddress(a.Address, persist.ChainETH),
			UserID:       a.UserID,
		}
	}

	lu, err := api.DefaultProfileByAddress(ctx, a.Address)
	if err != nil {
		return nil, err
	}

	if a.Signature != "" {
		access, refresh, err := api.AuthenticateWithSignature(ctx, a.Address, a.Signature)
		if err != nil {
			return nil, err
		}
		err = a.Queries.UpsertSocialOAuth(ctx, coredb.UpsertSocialOAuthParams{
			ID:           persist.GenerateID(),
			UserID:       a.UserID,
			Provider:     persist.SocialProviderLens,
			AccessToken:  util.ToNullString(access, false),
			RefreshToken: util.ToNullString(refresh, false),
		})
		if err != nil {
			return nil, err
		}
	}

	return &SocialAuthResult{
		Provider: persist.SocialProviderFarcaster,
		ID:       lu.ID,
		Metadata: map[string]interface{}{
			"username":          lu.Handle,
			"name":              lu.Name,
			"profile_image_url": lu.Picture.Optimized.URL,
			"bio":               lu.Bio,
		},
	}, nil

}
