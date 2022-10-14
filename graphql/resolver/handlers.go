package graphql

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/mikeydub/go-gallery/publicapi"

	gqlgen "github.com/99designs/gqlgen/graphql"
	"github.com/getsentry/sentry-go"
	"github.com/mikeydub/go-gallery/graphql/model"
	"github.com/mikeydub/go-gallery/service/auth"
	"github.com/mikeydub/go-gallery/service/fingerprints"
	"github.com/mikeydub/go-gallery/service/logger"
	sentryutil "github.com/mikeydub/go-gallery/service/sentry"
	"github.com/mikeydub/go-gallery/service/tracing"

	"sort"
	"strings"

	"github.com/mikeydub/go-gallery/util"
	"github.com/segmentio/ksuid"
	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	"github.com/vektah/gqlparser/v2/ast"
	"github.com/vektah/gqlparser/v2/gqlerror"
	"github.com/vektah/gqlparser/v2/validator"
)

const scrubText = "<scrubbed>"
const scrubDirectiveName = "scrub"

const gqlRequestIdContextKey = "graphql.gqlRequestId"
const noCachePublicAPIContextKey = "graphql.noCachePublicAPI"
const maxSentryDataLength = 8 * 1024

func getFieldName(fc *gqlgen.FieldContext) string {
	if fc == nil {
		return "UnknownField"
	}

	return fc.Field.Name
}

func getOperationName(oc *gqlgen.OperationContext) string {
	if oc == nil || oc.Operation == nil {
		return "UnknownOperation"
	}

	return oc.Operation.Name
}

func MutationCachingHandler(newPublicAPI func(context.Context, bool) *publicapi.PublicAPI) func(ctx context.Context, next gqlgen.Resolver) (res interface{}, err error) {
	return func(ctx context.Context, next gqlgen.Resolver) (res interface{}, err error) {
		fc := gqlgen.GetFieldContext(ctx)

		// If the current field isn't a top-level mutation, no handling is necessary
		if fc == nil || fc.Field.ObjectDefinition == nil || fc.Field.ObjectDefinition.Name != "Mutation" {
			return next(ctx)
		}

		// Get the request context so dataloaders will add their traces to the request span
		gc := util.GinContextFromContext(ctx)
		requestContext := gc.Request.Context()

		// Get or create a new public API with caching disabled, and push it to our context
		newAPI := new(publicapi.PublicAPI)
		if existingAPI, ok := gc.Value(noCachePublicAPIContextKey).(*publicapi.PublicAPI); ok {
			// Multiple mutations can share an instance of the PublicAPI with caching disabled, so see
			// if we've already created one for this request
			*newAPI = *existingAPI
		} else {
			noCacheAPI := newPublicAPI(requestContext, true)
			gc.Set(noCachePublicAPIContextKey, noCacheAPI)
			*newAPI = *noCacheAPI
		}

		ctx = publicapi.PushTo(ctx, newAPI)

		// Invoke next() with the new context so our mutation will run with caching disabled
		res, err = next(ctx)

		// Now that the mutation has run, we want to replace its no-caching PublicAPI with a new
		// version that has caching enabled again, such that any child fields returned by the
		// mutation will benefit from caching. Our options here are limited by gqlgen; child
		// fields are going to receive the context we passed to the next() function above, so
		// the best way to make sure those fields benefit from caching is to replace the PublicAPI
		// their context points to. This is safe to do because child fields won't be resolved
		// until this middleware returns, so it's safe to modify the PublicAPI on the context
		// here without a lock.
		*newAPI = *newPublicAPI(requestContext, false)

		return res, err
	}
}

func RemapAndReportErrors(ctx context.Context, next gqlgen.Resolver) (res interface{}, err error) {
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
			sentryutil.ReportRemappedError(ctx, err, remapped)
			return remapped, nil
		}
		logger.For(ctx).Debugf("unmapped error: %s", err)
	}

	sentryutil.ReportError(ctx, err)
	return res, err
}

func AuthRequiredDirectiveHandler() func(ctx context.Context, obj interface{}, next gqlgen.Resolver) (res interface{}, err error) {

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
				// Don't report this error -- it just means the user isn't logged in
				gqlModel = model.ErrNoCookie{Message: errorMsg}
			case auth.ErrInvalidJWT:
				// Report this error for now, since there may be value in knowing whose token expired when
				gqlModel = model.ErrInvalidToken{Message: errorMsg}
				sentryutil.ReportRemappedError(ctx, authError, gqlModel)
			default:
				return nil, authError
			}

			return makeErrNotAuthorized(errorMsg, gqlModel), nil
		}

		userID := auth.GetUserIDFromCtx(gc)
		if userID == "" {
			panic(fmt.Errorf("userID is empty, but no auth error occurred"))
		}

		return next(ctx)
	}
}

func FingerprintRequiredDirectiveHandler() func(ctx context.Context, obj interface{}, next gqlgen.Resolver) (res interface{}, err error) {

	return func(ctx context.Context, obj interface{}, next gqlgen.Resolver) (res interface{}, err error) {
		gc := util.GinContextFromContext(ctx)
		fingerprint, err := fingerprints.GetFingerprintFromCtx(gc)
		if err != nil || fingerprint == "" {
			switch {
			case err == fingerprints.ErrNoFingerprint:
				return model.ErrNotFingerprinted{Message: "fingerprinting is required"}, nil
			case fingerprint == "" && err == nil:
				return model.ErrNotFingerprinted{Message: "fingerprinting is required"}, nil
			default:
				return nil, err
			}
		}
		return next(ctx)
	}
}

func FingerprintOrAuthRequiredDirectiveHandler() func(ctx context.Context, obj interface{}, next gqlgen.Resolver) (res interface{}, err error) {

	return func(ctx context.Context, obj interface{}, next gqlgen.Resolver) (res interface{}, err error) {
		gc := util.GinContextFromContext(ctx)
		fingerprint, err := fingerprints.GetFingerprintFromCtx(gc)
		if err != nil || fingerprint == "" {
			return AuthRequiredDirectiveHandler()(ctx, obj, next)
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

func FieldReporter(trace bool) func(ctx context.Context, next gqlgen.Resolver) (res interface{}, err error) {
	return func(ctx context.Context, next gqlgen.Resolver) (res interface{}, err error) {
		var span *sentry.Span

		if trace {
			fc := gqlgen.GetFieldContext(ctx)

			// Only trace resolvers
			if fc.IsResolver {
				// Sentry docs say we need to clone a new hub per thread/goroutine to avoid concurrency issues,
				// and gqlgen will run resolvers concurrently, so we need to clone a hub per resolver.
				// Fortunately, cloning a hub is not an expensive operation.
				if parentHub := sentry.GetHubFromContext(ctx); parentHub != nil {
					ctx = sentry.SetHubOnContext(ctx, parentHub.Clone())
				}

				span, ctx = tracing.StartSpan(ctx, "gql.resolve", getFieldName(fc))
			}
		}

		res, err = next(ctx)

		if span != nil && err == nil {
			// If our result type implements the GraphQL Node pattern, add its ID to our event data
			if node, ok := res.(interface{ ID() model.GqlID }); ok {
				tracing.AddEventDataToSpan(span, map[string]interface{}{
					"resolvedNodeId": node.ID(),
				})
			}

			tracing.FinishSpan(span)
		}

		return res, err
	}
}

func ResponseReporter(log bool, trace bool) func(ctx context.Context, next gqlgen.ResponseHandler) *gqlgen.Response {
	return func(ctx context.Context, next gqlgen.ResponseHandler) *gqlgen.Response {
		var span *sentry.Span
		var responseLocatorId string

		if trace {
			oc := gqlgen.GetOperationContext(ctx)
			span, ctx = tracing.StartSpan(ctx, "gql.response", getOperationName(oc))
		}

		response := next(ctx)

		var responseData *json.RawMessage
		if response != nil && response.Data != nil {
			responseData = &response.Data
		} else {
			var placeholder json.RawMessage = []byte("<nil response>")
			responseData = &placeholder
		}

		if log {
			// Retrieve the gqlRequestId generated by the request logger (if available). This allows logged requests to be
			// matched with their logged responses. This is more specific than a trace-id, which might group multiple
			// requests and responses under the same ID.
			gqlRequestId := ctx.Value(gqlRequestIdContextKey)

			// Unique ID to make finding this particular log entry easy
			responseLocatorId = ksuid.New().String()

			gc := util.GinContextFromContext(ctx)
			userId := auth.GetUserIDFromCtx(gc)

			// Fields are logged in alphabetical order, so scrubbedQuery is prefixed with a zzz_ to make sure
			// it's last. In cases where a log entry is too large and gets truncated (e.g. Google Cloud Logging
			// limit is 256kb per entry), we want to make sure all of our fields are visible.
			logger.For(ctx).WithFields(logrus.Fields{
				"authenticated":     userId != "",
				"userId":            userId,
				"gqlRequestId":      gqlRequestId,
				"responseLocatorId": responseLocatorId,
				"zzz_response":      responseData,
			}).Info("Sending GraphQL response")
		}

		if span != nil {
			responseSizeLimited := limitEventDataSize(len(response.Data), maxSentryDataLength, "response", responseLocatorId, responseData)

			tracing.AddEventDataToSpan(span, map[string]interface{}{
				"response": responseSizeLimited,
			})

			tracing.FinishSpan(span)
		}

		return response
	}
}

func RequestReporter(schema *ast.Schema, log bool, trace bool) func(ctx context.Context, next gqlgen.OperationHandler) gqlgen.ResponseHandler {
	return func(ctx context.Context, next gqlgen.OperationHandler) gqlgen.ResponseHandler {
		var span *sentry.Span
		var requestLocatorId string

		gc := util.GinContextFromContext(ctx)
		oc := gqlgen.GetOperationContext(ctx)

		if trace {
			operationName := getOperationName(oc)
			transactionName := fmt.Sprintf("%s %s (%s)", gc.Request.Method, gc.Request.URL.Path, operationName)
			span, ctx = tracing.StartSpan(ctx, "gql.request", operationName, sentry.TransactionName(transactionName))
		}

		userId := auth.GetUserIDFromCtx(gc)
		scrubbedQuery, scrubbedVariables := getScrubbedQuery(ctx, schema, oc.Doc, oc.RawQuery, oc.Variables)

		if log {
			// Unique ID to connect this request with its associated response
			gqlRequestId := ksuid.New().String()
			ctx = context.WithValue(ctx, gqlRequestIdContextKey, gqlRequestId)

			// Unique ID to make finding this particular log entry easy
			requestLocatorId = ksuid.New().String()

			// Fields are logged in alphabetical order, so scrubbedQuery is prefixed with a zzz_ to make sure
			// it's last. In cases where a log entry is too large and gets truncated (e.g. Google Cloud Logging
			// limit is 256kb per entry), we want to make sure all of our fields are visible.
			logger.For(ctx).WithFields(logrus.Fields{
				"authenticated":     userId != "",
				"userId":            userId,
				"gqlRequestId":      gqlRequestId,
				"requestLocatorId":  requestLocatorId,
				"scrubbedVariables": scrubbedVariables,
				"zzz_scrubbedQuery": scrubbedQuery,
			}).Info("Received GraphQL query")
		}

		scrubbedQuerySizeLimited := limitEventDataSize(len(scrubbedQuery), maxSentryDataLength, "scrubbedQuery", requestLocatorId, scrubbedQuery)

		if hub := sentry.GetHubFromContext(ctx); hub != nil {
			scrubbedVariablesJson := "error converting variables to JSON string"
			if jsonBytes, err := json.Marshal(scrubbedVariables); err == nil {
				scrubbedVariablesJson = string(jsonBytes)
			}

			hub.Scope().AddEventProcessor(func(event *sentry.Event, hint *sentry.EventHint) *sentry.Event {
				// Replace the request body data with scrubbed data
				event.Request.Data = fmt.Sprintf("scrubbedVariables: %s\n\nscrubbedQuery: %s", scrubbedVariablesJson, scrubbedQuerySizeLimited)
				return event
			})
		}

		result := next(ctx)

		if span != nil {
			tracing.AddEventDataToSpan(span, map[string]interface{}{
				"scrubbedVariables": scrubbedVariables,
				"scrubbedQuery":     scrubbedQuerySizeLimited,
			})

			tracing.FinishSpan(span)
		}

		return result
	}
}

// Sentry will drop events if they contain too much data. It's convenient to attach our GraphQL
// requests and responses to Sentry events, but we don't want to risk dropping events, so we limit
// the size to something small (like 8kB). Larger payloads should still be logged and available
// via Google Cloud Logging (and easy to find with the included locatorId). Given that some event
// data might be unparsed JSON bytes, truncation is a bad idea: we don't want to truncate bytes at
// maxLength and try to parse invalid/incomplete data. Instead, we just replace the data with a
// helpful placeholder message.
func limitEventDataSize(length int, maxLength int, name string, locatorId string, data interface{}) interface{} {
	if length <= maxLength {
		return data
	}

	placeholder := fmt.Sprintf("%s omitted because it is too large (%d bytes)", name, length)
	if locatorId != "" {
		placeholder += fmt.Sprintf(", but it should be accessible by searching logs for this unique ID: %s", locatorId)
	}

	return placeholder
}

func scrubVariable(variableDefinition *ast.VariableDefinition, schema *ast.Schema, allQueryVariables map[string]interface{}, scrubbedOutput map[string]interface{}) {
	namedType := variableDefinition.Type.NamedType
	if namedType == "" && variableDefinition.Type.Elem != nil {
		namedType = variableDefinition.Type.Elem.NamedType
	}

	definition := schema.Types[namedType]
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

	if definition == nil || len(definition.Fields) == 0 {
		if !scrubField {
			scrubbedOutput[variableDefinition.Variable] = allQueryVariables[variableDefinition.Variable]
		}
		return
	}

	outputForDefinition := make(map[string]interface{})
	if !scrubField {
		scrubbedOutput[variableDefinition.Variable] = outputForDefinition
	}

	varsInterface := allQueryVariables[variableDefinition.Variable]
	varsForDefinition, ok := varsInterface.(map[string]interface{})
	if !ok {
		if varsInterface != nil {
			logger.For(nil).Warnf("scrubVariable: failed to convert variables '%v' to map[string]interface{}", varsForDefinition)
		}
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

	namedType := field.Type.NamedType
	if namedType == "" && field.Type.Elem != nil {
		namedType = field.Type.Elem.NamedType
	}

	definition := schema.Types[namedType]

	if definition == nil || len(definition.Fields) == 0 {
		if hasField && !scrubField {
			scrubbedOutput[field.Name] = fieldValue
		}
		return
	}

	outputForDefinition := make(map[string]interface{})

	if hasField && !scrubField {
		scrubbedOutput[field.Name] = outputForDefinition
	}

	varsInterface := variables[field.Name]
	varsForDefinition, ok := varsInterface.(map[string]interface{})
	if !ok {
		if varsInterface != nil {
			logger.For(nil).Warnf("scrubVariable: failed to convert variables '%v' to map[string]interface{}", varsForDefinition)
		}
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

func getScrubbedQuery(ctx context.Context, schema *ast.Schema, queryDoc *ast.QueryDocument, rawQuery string, allQueryVariables map[string]interface{}) (scrubbedQuery string, scrubbedVariables map[string]interface{}) {
	defer func() {
		// If scrubbing fails for some reason, return placeholder values
		if r := recover(); r != nil {
			scrubbedQuery = fmt.Sprintf("<error occurred while scrubbing query: %v>", r)
			scrubbedVariables = make(map[string]interface{})
			sentryutil.ReportError(ctx, fmt.Errorf("getScrubbedQuery failed: %v", r))
		}
	}()

	scrubPositions := make(map[int]*ast.Position)
	scrubbedVariables = make(map[string]interface{})

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
