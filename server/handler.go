package server

import (
	"context"
	"fmt"

	"cloud.google.com/go/storage"
	gqlgen "github.com/99designs/gqlgen/graphql"
	"github.com/99designs/gqlgen/graphql/handler"
	"github.com/99designs/gqlgen/graphql/playground"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/everFinance/goar"
	"github.com/gin-gonic/gin"
	shell "github.com/ipfs/go-ipfs-api"
	"github.com/mikeydub/go-gallery/db/sqlc"
	"github.com/mikeydub/go-gallery/graphql/generated"
	graphql "github.com/mikeydub/go-gallery/graphql/resolver"
	"github.com/mikeydub/go-gallery/middleware"
	"github.com/mikeydub/go-gallery/publicapi"
	"github.com/mikeydub/go-gallery/service/event"
	"github.com/mikeydub/go-gallery/service/multichain"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/service/sentry"
	"github.com/spf13/viper"
)

func handlersInit(router *gin.Engine, repos *persist.Repositories, queries *sqlc.Queries, ethClient *ethclient.Client, ipfsClient *shell.Shell, arweaveClient *goar.Client, stg *storage.Client, mcProvider *multichain.Provider) *gin.Engine {

	graphqlGroup := router.Group("/glry/graphql")

	graphqlHandlersInit(graphqlGroup, repos, queries, ethClient, ipfsClient, arweaveClient, stg, mcProvider)

	return router
}

func graphqlHandlersInit(parent *gin.RouterGroup, repos *persist.Repositories, queries *sqlc.Queries, ethClient *ethclient.Client, ipfsClient *shell.Shell, arweaveClient *goar.Client, storageClient *storage.Client, mcProvider *multichain.Provider) {
	parent.POST("/query", middleware.AddAuthToContext(), graphqlHandler(repos, queries, ethClient, ipfsClient, arweaveClient, storageClient, mcProvider))

	if viper.GetString("ENV") != "production" {
		// TODO: Consider completely disabling introspection in production
		parent.GET("/playground", graphqlPlaygroundHandler())
	}
}

func graphqlHandler(repos *persist.Repositories, queries *sqlc.Queries, ethClient *ethclient.Client, ipfsClient *shell.Shell, arweaveClient *goar.Client, storageClient *storage.Client, mp *multichain.Provider) gin.HandlerFunc {
	config := generated.Config{Resolvers: &graphql.Resolver{}}
	config.Directives.AuthRequired = graphql.AuthRequiredDirectiveHandler(ethClient)
	config.Directives.RestrictEnvironment = graphql.RestrictEnvironmentDirectiveHandler()

	schema := generated.NewExecutableSchema(config)
	h := handler.NewDefaultServer(schema)
	h.AroundOperations(graphql.ScrubbedRequestLogger(schema.Schema()))
	h.AroundFields(graphql.RemapErrors)
	h.AroundResponses(graphql.AddErrorsToGin)

	h.SetRecoverFunc(func(ctx context.Context, err interface{}) error {
		if hub := sentry.SentryHubFromContext(ctx); hub != nil {
			hub.Recover(err)
		}

		return gqlgen.DefaultRecover(ctx, err)
	})

	return func(c *gin.Context) {
		c.Set(graphql.GraphQLErrorsKey, &graphql.GraphQLErrorContext{})

		hub := sentry.SentryHubFromContext(c)
		if hub != nil {
			sentry.SetSentryAuthContext(c, hub)
		}

		defer func() {
			if hub != nil {
				for _, err := range c.Errors {
					hub.Scope().SetContext(sentry.ErrorSentryContextName, sentry.SentryErrorContext{})
					hub.CaptureException(err)
				}

				if gqlErrCtx := graphql.GqlErrorContextFromContext(c); gqlErrCtx != nil {
					for _, mappedErr := range gqlErrCtx.Errors() {
						errCtx := sentry.SentryErrorContext{}

						if mappedErr.Model != nil {
							errCtx.Mapped = true
							errCtx.MappedTo = fmt.Sprintf("%T", mappedErr.Model)
						}

						hub.Scope().SetContext(sentry.ErrorSentryContextName, errCtx)
						hub.CaptureException(mappedErr.Error)
					}
				}
			}
		}()

		event.AddTo(c, repos)
		publicapi.AddTo(c, repos, queries, ethClient, ipfsClient, arweaveClient, storageClient, mp)
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
