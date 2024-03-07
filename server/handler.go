package server

import (
	"context"
	"net/http"
	"time"

	"github.com/mikeydub/go-gallery/service/farcaster"
	"github.com/mikeydub/go-gallery/service/task"

	"cloud.google.com/go/pubsub"
	secretmanager "cloud.google.com/go/secretmanager/apiv1"
	"cloud.google.com/go/storage"
	gqlgen "github.com/99designs/gqlgen/graphql"
	"github.com/99designs/gqlgen/graphql/handler"
	"github.com/99designs/gqlgen/graphql/handler/extension"
	"github.com/99designs/gqlgen/graphql/handler/lru"
	"github.com/99designs/gqlgen/graphql/handler/transport"
	"github.com/99designs/gqlgen/graphql/playground"
	"github.com/bsm/redislock"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/everFinance/goar"
	sentry "github.com/getsentry/sentry-go"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	shell "github.com/ipfs/go-ipfs-api"
	magicclient "github.com/magiclabs/magic-admin-go/client"

	db "github.com/mikeydub/go-gallery/db/gen/coredb"
	"github.com/mikeydub/go-gallery/env"
	"github.com/mikeydub/go-gallery/event"
	"github.com/mikeydub/go-gallery/graphql/apq"
	"github.com/mikeydub/go-gallery/graphql/generated"
	graphql "github.com/mikeydub/go-gallery/graphql/resolver"
	"github.com/mikeydub/go-gallery/middleware"
	"github.com/mikeydub/go-gallery/publicapi"
	"github.com/mikeydub/go-gallery/service/auth"
	"github.com/mikeydub/go-gallery/service/mediamapper"
	"github.com/mikeydub/go-gallery/service/multichain"
	"github.com/mikeydub/go-gallery/service/notifications"
	"github.com/mikeydub/go-gallery/service/persist/postgres"
	"github.com/mikeydub/go-gallery/service/recommend"
	"github.com/mikeydub/go-gallery/service/recommend/userpref"
	"github.com/mikeydub/go-gallery/service/redis"
	sentryutil "github.com/mikeydub/go-gallery/service/sentry"
	"github.com/mikeydub/go-gallery/service/throttle"
	"github.com/mikeydub/go-gallery/util"
)

func handlersInit(router *gin.Engine, repos *postgres.Repositories, queries *db.Queries, httpClient *http.Client, ethClient *ethclient.Client, ipfsClient *shell.Shell, arweaveClient *goar.Client, stg *storage.Client, mcProvider *multichain.Provider, throttler *throttle.Locker, taskClient *task.Client, pub *pubsub.Client, lock *redislock.Client, secrets *secretmanager.Client, graphqlAPQCache, feedCache, socialCache, authRefreshCache, tokenManageCache, oneTimeLoginCache *redis.Cache, magicClient *magicclient.API, recommender *recommend.Recommender, p *userpref.Personalization, neynar *farcaster.NeynarAPI) *gin.Engine {

	graphqlGroup := router.Group("/glry/graphql")
	graphqlHandlersInit(graphqlGroup, repos, queries, httpClient, ethClient, ipfsClient, arweaveClient, stg, mcProvider, throttler, taskClient, pub, lock, secrets, graphqlAPQCache, feedCache, socialCache, authRefreshCache, tokenManageCache, oneTimeLoginCache, magicClient, recommender, p, neynar)

	router.GET("/alive", util.HealthCheckHandler())

	return router
}

func graphqlHandlersInit(parent *gin.RouterGroup, repos *postgres.Repositories, queries *db.Queries, httpClient *http.Client, ethClient *ethclient.Client, ipfsClient *shell.Shell, arweaveClient *goar.Client, storageClient *storage.Client, mcProvider *multichain.Provider, throttler *throttle.Locker, taskClient *task.Client, pub *pubsub.Client, lock *redislock.Client, secrets *secretmanager.Client, graphqlAPQCache, feedCache, socialCache, authRefreshCache, tokenManageCache, oneTimeLoginCache *redis.Cache, magicClient *magicclient.API, recommender *recommend.Recommender, p *userpref.Personalization, neynar *farcaster.NeynarAPI) {
	handler := graphqlHandler(repos, queries, httpClient, ethClient, ipfsClient, arweaveClient, storageClient, mcProvider, throttler, taskClient, pub, lock, secrets, graphqlAPQCache, feedCache, socialCache, authRefreshCache, tokenManageCache, oneTimeLoginCache, magicClient, recommender, p, neynar)
	parent.Any("/query", middleware.ContinueSession(queries, authRefreshCache), handler)
	parent.Any("/query/:operationName", middleware.ContinueSession(queries, authRefreshCache), handler)
	parent.GET("/playground", graphqlPlaygroundHandler())
}

func graphqlHandler(repos *postgres.Repositories, queries *db.Queries, httpClient *http.Client, ethClient *ethclient.Client, ipfsClient *shell.Shell, arweaveClient *goar.Client, storageClient *storage.Client, mp *multichain.Provider, throttler *throttle.Locker, taskClient *task.Client, pub *pubsub.Client, lock *redislock.Client, secrets *secretmanager.Client, graphqlAPQCache, feedCache, socialCache, authRefreshCache, tokenManageCache, oneTimeLoginCache *redis.Cache, magicClient *magicclient.API, recommender *recommend.Recommender, p *userpref.Personalization, neynar *farcaster.NeynarAPI) gin.HandlerFunc {
	config := generated.Config{Resolvers: &graphql.Resolver{}}
	config.Directives.AuthRequired = graphql.AuthRequiredDirectiveHandler()
	config.Directives.RestrictEnvironment = graphql.RestrictEnvironmentDirectiveHandler()
	config.Directives.BasicAuth = graphql.BasicAuthDirectiveHandler()
	config.Directives.FrontendBuildAuth = graphql.FrontendBuildAuthDirectiveHandler()
	config.Directives.Experimental = graphql.ExperimentalDirectiveHandler()

	schema := generated.NewExecutableSchema(config)
	h := handler.New(schema)

	// This code is ripped from ExecutableSchema.NewDefaultServer
	// We're not using NewDefaultServer anymore because we need a custom
	// WebSocket transport so we can modify the CheckOrigin function
	h.AddTransport(transport.Options{})
	h.AddTransport(transport.GET{})
	h.AddTransport(transport.POST{})
	h.AddTransport(transport.MultipartForm{})

	apqCache := &apq.APQCache{Cache: graphqlAPQCache}

	h.SetQueryCache(lru.New(1000))

	h.Use(extension.Introspection{})
	h.Use(extension.AutomaticPersistedQuery{
		Cache: apqCache,
	})

	// End code stolen from handler.NewDefaultServer

	h.AddTransport(&transport.Websocket{
		Upgrader: websocket.Upgrader{
			// This is okay to blindly return true since our
			// HandleCORS middleware function would block us
			// before arriving at this code path.
			CheckOrigin: func(r *http.Request) bool {
				requestOrigin := r.Header.Get("Origin")

				return middleware.IsOriginAllowed(requestOrigin)
			},
			ReadBufferSize:  1024,
			WriteBufferSize: 1024,
		},
		KeepAlivePingInterval: 15 * time.Second,
	})

	// Request/response logging is spammy in a local environment and can typically be better handled via browser debug tools.
	// It might be worth logging top-level queries and mutations in a single log line, though.
	enableLogging := env.GetString("ENV") != "local"

	h.AroundOperations(graphql.RequestReporter(schema.Schema(), enableLogging, true))
	h.AroundResponses(graphql.ResponseReporter(enableLogging, true))
	h.AroundFields(graphql.FieldReporter(true))
	h.SetErrorPresenter(graphql.ErrorLogger)

	// Should happen after FieldReporter, so Sentry trace context is set up prior to error reporting
	h.AroundFields(graphql.RemapAndReportErrors)

	newPublicAPI := func(ctx context.Context, disableDataloaderCaching bool) *publicapi.PublicAPI {
		return publicapi.New(ctx, disableDataloaderCaching, repos, queries, httpClient, ethClient, ipfsClient, arweaveClient, storageClient, mp, taskClient, throttler, secrets, apqCache, feedCache, socialCache, authRefreshCache, tokenManageCache, oneTimeLoginCache, magicClient, neynar)
	}

	notificationsHandler := notifications.New(queries, pub, taskClient, lock, true)

	h.AroundFields(graphql.MutationCachingHandler(newPublicAPI))

	h.SetRecoverFunc(func(ctx context.Context, err interface{}) error {
		if hub := sentryutil.SentryHubFromContext(ctx); hub != nil {
			hub.Recover(err)
		}

		return gqlgen.DefaultRecover(ctx, err)
	})

	return func(c *gin.Context) {
		if hub := sentryutil.SentryHubFromContext(c); hub != nil {
			auth.SetAuthContext(hub.Scope(), c)

			hub.Scope().AddEventProcessor(func(event *sentry.Event, hint *sentry.EventHint) *sentry.Event {
				// Filter the request body because queries may contain sensitive data. Other middleware (e.g. RequestReporter)
				// can update the request body later with an appropriately scrubbed version of the query.
				event.Request.Data = "[filtered]"
				return event
			})

			hub.Scope().AddEventProcessor(sentryutil.SpanFilterEventProcessor(c, 1000, 1*time.Millisecond, 8, true))
		}

		disableDataloaderCaching := false

		mediamapper.AddTo(c)
		event.AddTo(c, disableDataloaderCaching, notificationsHandler, queries, taskClient, neynar)
		notifications.AddTo(c, notificationsHandler)
		recommend.AddTo(c, recommender)
		userpref.AddTo(c, p)

		// Use the request context so dataloaders will add their traces to the request span
		publicapi.AddTo(c, newPublicAPI(c.Request.Context(), disableDataloaderCaching))

		h.ServeHTTP(c.Writer, c.Request)
	}
}

// GraphQL playground GUI for experimenting and debugging
func graphqlPlaygroundHandler() gin.HandlerFunc {
	h := playground.Handler("GraphQL", "/glry/graphql/query")

	return func(c *gin.Context) {
		h.ServeHTTP(c.Writer, c.Request)
	}
}
