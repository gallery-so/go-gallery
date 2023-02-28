package publicapi

import (
	"context"
	"fmt"
	"github.com/go-playground/validator/v10"
	db "github.com/mikeydub/go-gallery/db/gen/coredb"
	"github.com/mikeydub/go-gallery/graphql/dataloader"
	"github.com/mikeydub/go-gallery/validate"
)

const maxSearchQueryLength = 256
const maxSearchResults = 100

type SearchAPI struct {
	queries   *db.Queries
	loaders   *dataloader.Loaders
	validator *validator.Validate
}

// SearchUsers searches for users with the given query, limit, and optional weights. Weights may be nil to accept default values.
// Weighting will probably be removed after we settle on defaults that feel correct!
func (api SearchAPI) SearchUsers(ctx context.Context, query string, limit int, usernameWeight float32, bioWeight float32) ([]db.User, error) {
	// Validate
	if err := validate.ValidateFields(api.validator, validate.ValidationMap{
		"query":          {query, fmt.Sprintf("required,min=1,max=%d", maxSearchQueryLength)},
		"limit":          {limit, fmt.Sprintf("min=1,max=%d", maxSearchResults)},
		"usernameWeight": {usernameWeight, "gte=0.0,lte=1.0"},
		"bioWeight":      {bioWeight, "gte=0.0,lte=1.0"},
	}); err != nil {
		return nil, err
	}

	// Sanitize
	query = validate.SanitizationPolicy.Sanitize(query)

	return api.queries.SearchUsers(ctx, db.SearchUsersParams{
		Limit:          int32(limit),
		Query:          query,
		UsernameWeight: usernameWeight,
		BioWeight:      bioWeight,
	})
}

// SearchGalleries searches for galleries with the given query, limit, and optional weights. Weights may be nil to accept default values.
// Weighting will probably be removed after we settle on defaults that feel correct!
func (api SearchAPI) SearchGalleries(ctx context.Context, query string, limit int, nameWeight float32, descriptionWeight float32) ([]db.Gallery, error) {
	// Validate
	if err := validate.ValidateFields(api.validator, validate.ValidationMap{
		"query":             {query, fmt.Sprintf("required,min=1,max=%d", maxSearchQueryLength)},
		"limit":             {limit, fmt.Sprintf("min=1,max=%d", maxSearchResults)},
		"nameWeight":        {nameWeight, "gte=0.0,lte=1.0"},
		"descriptionWeight": {descriptionWeight, "gte=0.0,lte=1.0"},
	}); err != nil {
		return nil, err
	}

	// Sanitize
	query = validate.SanitizationPolicy.Sanitize(query)

	return api.queries.SearchGalleries(ctx, db.SearchGalleriesParams{
		Limit:             int32(limit),
		Query:             query,
		NameWeight:        nameWeight,
		DescriptionWeight: descriptionWeight,
	})
}

// SearchContracts searches for contracts with the given query, limit, and optional weights. Weights may be nil to accept default values.
// Weighting will probably be removed after we settle on defaults that feel correct!
func (api SearchAPI) SearchContracts(ctx context.Context, query string, limit int, nameWeight float32, descriptionWeight float32, poapAddressWeight float32) ([]db.Contract, error) {
	// Validate
	if err := validate.ValidateFields(api.validator, validate.ValidationMap{
		"query":             {query, fmt.Sprintf("required,min=1,max=%d", maxSearchQueryLength)},
		"limit":             {limit, fmt.Sprintf("min=1,max=%d", maxSearchResults)},
		"nameWeight":        {nameWeight, "gte=0.0,lte=1.0"},
		"descriptionWeight": {descriptionWeight, "gte=0.0,lte=1.0"},
		"poapAddressWeight": {poapAddressWeight, "gte=0.0,lte=1.0"},
	}); err != nil {
		return nil, err
	}

	// Sanitize
	query = validate.SanitizationPolicy.Sanitize(query)

	return api.queries.SearchContracts(ctx, db.SearchContractsParams{
		Limit:             int32(limit),
		Query:             query,
		NameWeight:        nameWeight,
		DescriptionWeight: descriptionWeight,
		PoapAddressWeight: poapAddressWeight,
	})
}
