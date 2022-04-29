package graphql

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	gqlgen "github.com/99designs/gqlgen/graphql"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/getsentry/sentry-go"
	"github.com/mikeydub/go-gallery/graphql/model"
	"github.com/mikeydub/go-gallery/publicapi"
	"github.com/mikeydub/go-gallery/service/auth"
	"github.com/mikeydub/go-gallery/service/eth"
	"github.com/mikeydub/go-gallery/service/logger"
	"github.com/mikeydub/go-gallery/util"
	"github.com/segmentio/ksuid"
	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	"github.com/vektah/gqlparser/v2/ast"
	"github.com/vektah/gqlparser/v2/gqlerror"
	"github.com/vektah/gqlparser/v2/validator"
	"sort"
	"strings"
)

const scrubText = "<scrubbed>"
const scrubDirectiveName = "scrub"

const gqlRequestIdContextKey = "graphql.gqlRequestId"

// Google Cloud Logging limits our log entries to 256kb. This is too small for some large GraphQL responses,
// so we truncate those fields. If we don't truncate them manually, Cloud Logging will do it for us, but in
// the process it may wipe out other logging fields that we care about (like traceId). Note that our limit
// is set to 240kb (not 256kb) to allow space for additional logging fields.
const cloudLoggingMaxBytes = 240 * 1024

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

	// Unwrap any gqlerror.Error wrappers to get the underlying error type
	var gqlErr *gqlerror.Error
	for errors.As(err, &gqlErr) {
		err = gqlErr.Unwrap()
	}

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

func RequestTracer() func(ctx context.Context, next gqlgen.OperationHandler) gqlgen.ResponseHandler {
	return func(ctx context.Context, next gqlgen.OperationHandler) gqlgen.ResponseHandler {
		oc := gqlgen.GetOperationContext(ctx)

		span := sentry.StartSpan(ctx, oc.Operation.Name)
		result := next(logger.NewContextWithSpan(span.Context(), span))
		span.Finish()

		return result
	}
}

func ResponseTracer() func(ctx context.Context, next gqlgen.ResponseHandler) *gqlgen.Response {
	return func(ctx context.Context, next gqlgen.ResponseHandler) *gqlgen.Response {
		oc := gqlgen.GetOperationContext(ctx)

		span := sentry.StartSpan(ctx, oc.Operation.Name)
		result := next(logger.NewContextWithSpan(span.Context(), span))
		span.Finish()

		return result
	}
}

func FieldTracer() func(ctx context.Context, next gqlgen.Resolver) (res interface{}, err error) {
	return func(ctx context.Context, next gqlgen.Resolver) (res interface{}, err error) {
		fc := gqlgen.GetFieldContext(ctx)

		// Only trace resolvers
		if !fc.IsResolver {
			return next(ctx)
		}

		span := sentry.StartSpan(ctx, fc.Field.Name)
		res, err = next(logger.NewContextWithSpan(span.Context(), span))
		span.Finish()

		return res, err
	}
}

// truncateField returns a string no larger than maxBytes length. It slices based on
// bytes, and as a result, the final rune in the output may be invalid. This is okay
// for our purposes here (it should just display as an invalid character in log entries),
// and allowing this means we don't need to spend cycles converting a gigantic string to
// runes.
func truncateField(text string, maxBytes int) string {
	if len(text) < maxBytes {
		return text
	}

	return text[:maxBytes]
}

func ResponseLogger() func(ctx context.Context, next gqlgen.ResponseHandler) *gqlgen.Response {
	return func(ctx context.Context, next gqlgen.ResponseHandler) *gqlgen.Response {
		response := next(ctx)

		var message string
		messageBytes, err := json.Marshal(&response.Data)

		if err != nil {
			message = "failed to marshal json.RawMessage; unable to log GraphQL response"
		} else {
			message = string(messageBytes)
		}

		gc := util.GinContextFromContext(ctx)
		userId := auth.GetUserIDFromCtx(gc)

		// Retrieve the requestID generated by the request logger (if available). This allows logged requests to be
		// matched with their logged responses. This is more specific than a trace-id, which might group multiple
		// requests and responses into the same transaction.
		requestID := ctx.Value(gqlRequestIdContextKey)

		logger.For(ctx).WithFields(logrus.Fields{
			"authenticated": userId != "",
			"userId":        userId,
			"gqlRequestId":  requestID,
			"response":      truncateField(message, cloudLoggingMaxBytes),
		}).Info("Sending GraphQL response")

		return response
	}
}

func ScrubbedRequestLogger(schema *ast.Schema) func(ctx context.Context, next gqlgen.OperationHandler) gqlgen.ResponseHandler {
	return func(ctx context.Context, next gqlgen.OperationHandler) gqlgen.ResponseHandler {
		requestID := ksuid.New().String()

		gc := util.GinContextFromContext(ctx)
		userId := auth.GetUserIDFromCtx(gc)
		oc := gqlgen.GetOperationContext(ctx)
		scrubbedQuery, scrubbedVariables := getScrubbedQuery(schema, oc.Doc, oc.RawQuery, oc.Variables)
		logger.For(ctx).WithFields(logrus.Fields{
			"authenticated":     userId != "",
			"userId":            userId,
			"gqlRequestId":      requestID,
			"scrubbedQuery":     truncateField(scrubbedQuery, cloudLoggingMaxBytes),
			"scrubbedVariables": scrubbedVariables,
		}).Info("Received GraphQL query")

		// Add the requestID to the context so the ResponseLogger can find it
		requestCtx := context.WithValue(ctx, gqlRequestIdContextKey, requestID)
		return next(requestCtx)
	}
}

func scrubVariable(variableDefinition *ast.VariableDefinition, schema *ast.Schema, allQueryVariables map[string]interface{}, scrubbedOutput map[string]interface{}) {
	definition := schema.Types[variableDefinition.Type.NamedType]
	scrubField := false

	for _, directive := range definition.Directives {
		if directive.Name == scrubDirectiveName {
			scrubField = true
			break
		}
	}

	if scrubField {
		scrubbedOutput[variableDefinition.Variable] = scrubText
	}

	if len(definition.Fields) == 0 {
		if !scrubField {
			scrubbedOutput[variableDefinition.Variable] = allQueryVariables[variableDefinition.Variable]
		}
		return
	}

	outputForDefinition := make(map[string]interface{})
	if !scrubField {
		scrubbedOutput[variableDefinition.Variable] = outputForDefinition
	}

	varsForDefinition, ok := allQueryVariables[variableDefinition.Variable].(map[string]interface{})
	if !ok {
		logrus.Warnf("scrubVariable: failed to convert variables '%v' to map[string]interface{}", varsForDefinition)
		return
	}

	for _, field := range definition.Fields {
		scrubVariableField(schema, field, varsForDefinition, outputForDefinition)
	}
}

func scrubVariableField(schema *ast.Schema, field *ast.FieldDefinition, variables map[string]interface{}, scrubbedOutput map[string]interface{}) {
	scrubField := false
	fieldValue, hasField := variables[field.Name]

	for _, directive := range field.Directives {
		if directive.Name == scrubDirectiveName {
			scrubField = true
			break
		}
	}

	if hasField && scrubField {
		scrubbedOutput[field.Name] = scrubText
	}

	definition := schema.Types[field.Type.NamedType]

	if len(definition.Fields) == 0 {
		if hasField && !scrubField {
			scrubbedOutput[field.Name] = fieldValue
		}
		return
	}

	outputForDefinition := make(map[string]interface{})

	if hasField && !scrubField {
		scrubbedOutput[field.Name] = outputForDefinition
	}

	varsForDefinition, ok := variables[field.Name].(map[string]interface{})
	if !ok {
		logrus.Warnf("scrubVariable: failed to convert variables '%v' to map[string]interface{}", varsForDefinition)
		return
	}

	for _, childField := range definition.Fields {
		scrubVariableField(schema, childField, varsForDefinition, outputForDefinition)
	}
}

func scrubChildren(value *ast.Value, schema *ast.Schema, positions map[int]*ast.Position) {
	if value.Children == nil {
		scrubPosition := value.Position
		positions[scrubPosition.Start] = scrubPosition
		return
	}

	for _, child := range value.Children {
		scrubChildren(child.Value, schema, positions)
	}
}

func scrubValue(value *ast.Value, schema *ast.Schema, positions map[int]*ast.Position) {
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
			if directive.Name != scrubDirectiveName {
				continue
			}

			if value.Children == nil {
				continue
			}

			// Get the value associated with the scrubbed field
			childValue := value.Children.ForName(field.Name)

			// If the value has children, it's not a scalar. Don't try to scrub a non-scalar value directly;
			// recursively scrub its children instead
			if childValue.Children != nil {
				scrubChildren(childValue, schema, positions)
			} else {
				// It's a scalar -- scrub it!
				scrubPosition := childValue.Position
				positions[scrubPosition.Start] = scrubPosition
			}
		}
	}
}

func getScrubbedQuery(schema *ast.Schema, queryDoc *ast.QueryDocument, rawQuery string, allQueryVariables map[string]interface{}) (string, map[string]interface{}) {
	scrubPositions := make(map[int]*ast.Position)
	scrubbedVariables := make(map[string]interface{})

	observers := validator.Events{}
	observers.OnValue(func(walker *validator.Walker, value *ast.Value) {
		scrubValue(value, schema, scrubPositions)
	})

	observers.OnVariable(func(walker *validator.Walker, variableDefinition *ast.VariableDefinition) {
		scrubVariable(variableDefinition, schema, allQueryVariables, scrubbedVariables)
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
		builder.WriteString(scrubText)
		strIndex = position.End
	}

	writeRunes(&builder, runes[strIndex:])

	return builder.String(), scrubbedVariables
}

func writeRunes(builder *strings.Builder, runes []rune) {
	for _, r := range runes {
		builder.WriteRune(r)
	}
}
