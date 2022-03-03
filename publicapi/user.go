package publicapi

import (
	"context"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/mikeydub/go-gallery/graphql/dataloader"
	"github.com/mikeydub/go-gallery/service/auth"
	"github.com/mikeydub/go-gallery/service/membership"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/service/user"
)

type UserAPI struct {
	repos     *persist.Repositories
	loaders   *dataloader.Loaders
	ethClient *ethclient.Client
}

func (api UserAPI) AddUserAddress(ctx context.Context, address persist.Address, authenticator auth.Authenticator) error {
	//userID, err := getAuthenticatedUser(ctx)
	//if err != nil {
	//	return err
	//}

	//user.AddAddressToUser()
	panic("need to rework with authenticators")
}

func (api UserAPI) RemoveUserAddresses(ctx context.Context, addresses []persist.Address) error {
	userID, err := getAuthenticatedUser(ctx)
	if err != nil {
		return err
	}

	// TODO: Validation error?
	addresses = persist.RemoveDuplicateAddresses(addresses)

	return user.RemoveAddressesFromUser(ctx, userID, addresses, api.repos.UserRepository)
}

func (api UserAPI) UpdateUserInfo(ctx context.Context, username string, bio string) error {
	userID, err := getAuthenticatedUser(ctx)
	if err != nil {
		return err
	}

	// TODO: Validation of username and bio, sanitize one or both also

	return user.UpdateUser(ctx, userID, username, bio, api.repos.UserRepository, api.ethClient)
}

func (api UserAPI) GetMembershipTiers(ctx context.Context, forceRefresh bool) ([]persist.MembershipTier, error) {
	return membership.GetMembershipTiers(ctx, forceRefresh, api.repos.MembershipRepository, api.repos.UserRepository, api.repos.GalleryRepository, api.ethClient)
}
