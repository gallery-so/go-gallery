package publicapi

import (
	"context"
	"errors"

	admin "github.com/mikeydub/go-gallery/adminapi"
	"github.com/mikeydub/go-gallery/graphql/apq"

	"github.com/mikeydub/go-gallery/service/persist/postgres"

	"github.com/gin-gonic/gin"
	db "github.com/mikeydub/go-gallery/db/gen/coredb"
	"github.com/mikeydub/go-gallery/event"

	gcptasks "cloud.google.com/go/cloudtasks/apiv2"
	secretmanager "cloud.google.com/go/secretmanager/apiv1"
	"cloud.google.com/go/storage"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/everFinance/goar"
	"github.com/go-playground/validator/v10"
	shell "github.com/ipfs/go-ipfs-api"
	"github.com/mikeydub/go-gallery/graphql/dataloader"
	"github.com/mikeydub/go-gallery/service/auth"
	"github.com/mikeydub/go-gallery/service/logger"
	"github.com/mikeydub/go-gallery/service/multichain"
	"github.com/mikeydub/go-gallery/service/persist"
	sentryutil "github.com/mikeydub/go-gallery/service/sentry"
	"github.com/mikeydub/go-gallery/service/throttle"
	"github.com/mikeydub/go-gallery/util"
	"github.com/mikeydub/go-gallery/validate"
)

var errBadCursorFormat = errors.New("bad cursor format")

const apiContextKey = "publicapi.api"

type PublicAPI struct {
	repos     *postgres.Repositories
	queries   *db.Queries
	loaders   *dataloader.Loaders
	validator *validator.Validate
	APQ       *apq.APQCache

	Auth          *AuthAPI
	Collection    *CollectionAPI
	Gallery       *GalleryAPI
	User          *UserAPI
	Token         *TokenAPI
	Contract      *ContractAPI
	Wallet        *WalletAPI
	Misc          *MiscAPI
	Feed          *FeedAPI
	Notifications *NotificationsAPI
	Interaction   *InteractionAPI
	Admin         *admin.AdminAPI
	Merch         *MerchAPI
}

func New(ctx context.Context, disableDataloaderCaching bool, repos *postgres.Repositories, queries *db.Queries, ethClient *ethclient.Client, ipfsClient *shell.Shell,
	arweaveClient *goar.Client, storageClient *storage.Client, multichainProvider *multichain.Provider, taskClient *gcptasks.Client, throttler *throttle.Locker, secrets *secretmanager.Client, apq *apq.APQCache) *PublicAPI {

	loaders := dataloader.NewLoaders(ctx, queries, disableDataloaderCaching)
	validator := validate.WithCustomValidators()

	return &PublicAPI{
		repos:     repos,
		queries:   queries,
		loaders:   loaders,
		validator: validator,
		APQ:       apq,

		Auth:          &AuthAPI{repos: repos, queries: queries, loaders: loaders, validator: validator, ethClient: ethClient, multiChainProvider: multichainProvider},
		Collection:    &CollectionAPI{repos: repos, queries: queries, loaders: loaders, validator: validator, ethClient: ethClient},
		Gallery:       &GalleryAPI{repos: repos, queries: queries, loaders: loaders, validator: validator, ethClient: ethClient},
		User:          &UserAPI{repos: repos, queries: queries, loaders: loaders, validator: validator, ethClient: ethClient, ipfsClient: ipfsClient, arweaveClient: arweaveClient, storageClient: storageClient},
		Contract:      &ContractAPI{repos: repos, queries: queries, loaders: loaders, validator: validator, ethClient: ethClient, multichainProvider: multichainProvider, taskClient: taskClient},
		Token:         &TokenAPI{repos: repos, queries: queries, loaders: loaders, validator: validator, ethClient: ethClient, multichainProvider: multichainProvider, throttler: throttler},
		Wallet:        &WalletAPI{repos: repos, queries: queries, loaders: loaders, validator: validator, ethClient: ethClient, multichainProvider: multichainProvider},
		Misc:          &MiscAPI{repos: repos, queries: queries, loaders: loaders, validator: validator, ethClient: ethClient, storageClient: storageClient},
		Feed:          &FeedAPI{repos: repos, queries: queries, loaders: loaders, validator: validator, ethClient: ethClient},
		Interaction:   &InteractionAPI{repos: repos, queries: queries, loaders: loaders, validator: validator, ethClient: ethClient},
		Notifications: &NotificationsAPI{queries: queries, loaders: loaders, validator: validator},
		Admin:         admin.NewAPI(repos, queries, validator),
		Merch:         &MerchAPI{repos: repos, queries: queries, loaders: loaders, validator: validator, ethClient: ethClient, multichainProvider: multichainProvider, secrets: secrets},
	}
}

// AddTo adds the specified PublicAPI to a gin context
func AddTo(ctx *gin.Context, api *PublicAPI) {
	ctx.Set(apiContextKey, api)
}

// PushTo pushes the specified PublicAPI onto the context stack and returns the new context
func PushTo(ctx context.Context, api *PublicAPI) context.Context {
	return context.WithValue(ctx, apiContextKey, api)
}

func For(ctx context.Context) *PublicAPI {
	// See if a newer PublicAPI instance has been pushed to the context stack
	if api, ok := ctx.Value(apiContextKey).(*PublicAPI); ok {
		return api
	}

	// If not, fall back to the one added to the gin context
	gc := util.GinContextFromContext(ctx)
	return gc.Value(apiContextKey).(*PublicAPI)
}

func getAuthenticatedUserID(ctx context.Context) (persist.DBID, error) {
	gc := util.GinContextFromContext(ctx)
	authError := auth.GetAuthErrorFromCtx(gc)

	if authError != nil {
		return "", authError
	}

	userID := auth.GetUserIDFromCtx(gc)
	return userID, nil
}

func dispatchEvent(ctx context.Context, evt db.Event, v *validator.Validate, caption *string) (*db.FeedEvent, error) {
	ctx = sentryutil.NewSentryHubGinContext(ctx)
	if err := v.Struct(evt); err != nil {
		return nil, err
	}

	if caption != nil {
		evt.Caption = persist.StrToNullStr(caption)
		return event.DispatchImmediate(ctx, []db.Event{evt})
	}

	go pushEvent(ctx, evt)
	return nil, nil
}

func dispatchEvents(ctx context.Context, evts []db.Event, v *validator.Validate, caption *string) (*db.FeedEvent, error) {
	ctx = sentryutil.NewSentryHubGinContext(ctx)
	groupID := persist.GenerateID()
	for i, evt := range evts {
		if err := v.Struct(evt); err != nil {
			return nil, err
		}
		evt.GroupID = persist.DBIDToNullStr(groupID)
		evts[i] = evt
	}

	if caption != nil {
		for i, evt := range evts {
			evt.Caption = persist.StrToNullStr(caption)
			evts[i] = evt
		}
		return event.DispatchImmediate(ctx, evts)
	}

	for _, evt := range evts {
		go pushEvent(ctx, evt)
	}
	return nil, nil
}

func pushEvent(ctx context.Context, evt db.Event) {
	if hub := sentryutil.SentryHubFromContext(ctx); hub != nil {
		sentryutil.SetEventContext(hub.Scope(), persist.NullStrToDBID(evt.ActorID), evt.SubjectID, evt.Action)
	}
	if err := event.DispatchDelayed(ctx, evt); err != nil {
		logger.For(ctx).Error(err)
		sentryutil.ReportError(ctx, err)
	}
}
