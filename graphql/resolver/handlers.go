package graphql

import (
	"context"
	"errors"
	"fmt"
	"github.com/mikeydub/go-gallery/publicapi"

	gqlgen "github.com/99designs/gqlgen/graphql"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/mikeydub/go-gallery/graphql/model"
	"github.com/mikeydub/go-gallery/service/auth"
	"github.com/mikeydub/go-gallery/service/eth"
	"github.com/mikeydub/go-gallery/util"
	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	"github.com/vektah/gqlparser/v2/ast"
	"github.com/vektah/gqlparser/v2/validator"
	"sort"
	"strings"
)

func AddErrorsToGin(ctx context.Context, next gqlgen.ResponseHandler) *gqlgen.Response {
	response := next(ctx)
	gc := util.GinContextFromContext(ctx)
	for _, err := range response.Errors {
		gc.Error(err)
	}
	return response
}

func RemapErrors(ctx context.Context, next gqlgen.Resolver) (res interface{}, err error) {
	res, err = next(ctx)

	if err == nil {
		return res, err
	}

	fc := gqlgen.GetFieldContext(ctx)
	typeName := fc.Field.Field.Definition.Type.NamedType

	// If a resolver returns an error that can be mapped to that resolver's expected GQL type,
	// remap it and return the appropriate GQL model instead of an error. This is common for
	// union types where the result could be an object or a set of errors.
	if fc.IsResolver {
		if remapped, ok := errorToGraphqlType(ctx, err, typeName); ok {
			return remapped, nil
		}
	}

	return res, err
}

func AuthRequiredDirectiveHandler(ethClient *ethclient.Client) func(ctx context.Context, obj interface{}, next gqlgen.Resolver) (res interface{}, err error) {
	return func(ctx context.Context, obj interface{}, next gqlgen.Resolver) (res interface{}, err error) {
		gc := util.GinContextFromContext(ctx)

		makeErrNotAuthorized := func(e string, c model.AuthorizationError) model.ErrNotAuthorized {
			return model.ErrNotAuthorized{
				Message: fmt.Sprintf("authorization failed: %s", e),
				Cause:   c,
			}
		}

		if authError := auth.GetAuthErrorFromCtx(gc); authError != nil {
			if authError != auth.ErrNoCookie {
				// Clear the user's cookie on any auth error (except for ErrNoCookie, since there is no cookie set)
				auth.Logout(ctx)
			}

			var gqlModel model.AuthorizationError
			errorMsg := authError.Error()

			switch authError {
			case auth.ErrNoCookie:
				gqlModel = model.ErrNoCookie{Message: errorMsg}
				addError(ctx, authError, gqlModel)
			case auth.ErrInvalidJWT:
				gqlModel = model.ErrInvalidToken{Message: errorMsg}
				addError(ctx, authError, gqlModel)
			default:
				return nil, authError
			}

			return makeErrNotAuthorized(errorMsg, gqlModel), nil
		}

		userID := auth.GetUserIDFromCtx(gc)
		if userID == "" {
			panic(fmt.Errorf("userID is empty, but no auth error occurred"))
		}

		if viper.GetBool("REQUIRE_NFTS") {
			user, err := publicapi.For(ctx).User.GetUserById(ctx, userID)

			if err != nil {
				return nil, err
			}

			has := false
			for _, addr := range user.Addresses {
				allowlist := auth.GetAllowlistContracts()
				for k, v := range allowlist {
					if found, _ := eth.HasNFTs(gc, k, v, addr, ethClient); found {
						has = true
						break
					}
				}
			}
			if !has {
				errorMsg := auth.ErrDoesNotOwnRequiredNFT{}.Error()
				nftErr := model.ErrDoesNotOwnRequiredNft{Message: errorMsg}

				return makeErrNotAuthorized(errorMsg, nftErr), nil
			}
		}

		return next(ctx)
	}
}

func RestrictEnvironmentDirectiveHandler() func(ctx context.Context, obj interface{}, next gqlgen.Resolver, allowed []string) (res interface{}, err error) {
	env := viper.GetString("ENV")
	restrictionErr := errors.New("schema restriction: functionality not allowed in the current environment")

	return func(ctx context.Context, obj interface{}, next gqlgen.Resolver, allowed []string) (res interface{}, err error) {
		for _, allowedEnv := range allowed {
			if strings.EqualFold(env, allowedEnv) {
				return next(ctx)
			}
		}

		return nil, restrictionErr
	}
}

func ScrubbedRequestLogger(schema *ast.Schema) func(ctx context.Context, next gqlgen.OperationHandler) gqlgen.ResponseHandler {
	logger := logrus.New()

	// To make queries show up in a readable format in a local console, we want a text formatter that
	// doesn't escape newlines. To make queries readable in GCP logs, we actually want a JSON formatter;
	// otherwise, each individual line of the query will be treated as a separate log entry.
	if viper.GetString("ENV") == "local" {
		logger.SetFormatter(&logrus.TextFormatter{DisableQuote: true})
	} else {
		logger.SetFormatter(&logrus.JSONFormatter{})
	}

	return func(ctx context.Context, next gqlgen.OperationHandler) gqlgen.ResponseHandler {
		gc := util.GinContextFromContext(ctx)
		userId := auth.GetUserIDFromCtx(gc)
		oc := gqlgen.GetOperationContext(ctx)
		scrubbedQuery := getScrubbedQuery(schema, oc.Doc, oc.RawQuery)
		logger.WithFields(logrus.Fields{
			"authenticated": userId != "",
			"userId":        userId,
			"scrubbedQuery": scrubbedQuery,
		}).Info("Received GraphQL query")

		return next(ctx)
	}
}

func scrubChildren(value *ast.Value, positions map[int]*ast.Position) {
	if value.Children == nil {
		scrubPosition := value.Position
		positions[scrubPosition.Start] = scrubPosition
		return
	}

	for _, child := range value.Children {
		scrubChildren(child.Value, positions)
	}
}

func scrubValue(value *ast.Value, positions map[int]*ast.Position) {
	if value.Definition == nil || value.Definition.Fields == nil {
		return
	}

	// Look through all the fields defined for this value
	for _, field := range value.Definition.Fields {
		if field.Directives == nil {
			continue
		}

		// Find field definitions with directives on them
		for _, directive := range field.Directives {
			// Look for a directive named "scrub"
			if directive.Name != "scrub" {
				continue
			}

			// Get the value associated with the scrubbed field
			childValue := value.Children.ForName(field.Name)

			// If the value has children, it's not a scalar. Don't try to scrub a non-scalar value directly;
			// recursively scrub its children instead
			if childValue.Children != nil {
				scrubChildren(childValue, positions)
			} else {
				// It's a scalar -- scrub it!
				scrubPosition := childValue.Position
				positions[scrubPosition.Start] = scrubPosition
			}
		}
	}
}

func getScrubbedQuery(schema *ast.Schema, queryDoc *ast.QueryDocument, rawQuery string) string {
	scrubPositions := make(map[int]*ast.Position)

	observers := validator.Events{}
	observers.OnValue(func(walker *validator.Walker, value *ast.Value) {
		scrubValue(value, scrubPositions)
	})

	validator.Walk(schema, queryDoc, &observers)

	sortedKeys := make([]int, 0, len(scrubPositions))
	for key := range scrubPositions {
		sortedKeys = append(sortedKeys, key)
	}

	sort.Ints(sortedKeys)

	var builder strings.Builder
	builder.Grow(len(rawQuery))

	strIndex := 0

	runes := []rune(rawQuery)
	for _, key := range sortedKeys {
		position := scrubPositions[key]
		writeRunes(&builder, runes[strIndex:position.Start])
		builder.WriteString("<scrubbed>")
		strIndex = position.End
	}

	writeRunes(&builder, runes[strIndex:])

	return builder.String()
}

func writeRunes(builder *strings.Builder, runes []rune) {
	for _, r := range runes {
		builder.WriteRune(r)
	}
}
