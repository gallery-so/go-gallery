package publicapi

import (
	"context"
	"fmt"

	"github.com/go-playground/validator/v10"
	db "github.com/mikeydub/go-gallery/db/gen/coredb"
	"github.com/mikeydub/go-gallery/validate"

	"github.com/mikeydub/go-gallery/service/ai"
	"github.com/mikeydub/go-gallery/service/persist"
)

type ConversationAPI struct {
	queries            *db.Queries
	validator          *validator.Validate
	conversationClient *ai.ConversationClient
}

func (api ConversationAPI) GalleryConverse(ctx context.Context, message string, conversationID *persist.DBID) (string, persist.DBID, error) {
	// Validate
	if err := validate.ValidateFields(api.validator, validate.ValidationMap{
		"message": {message, "required"},
	}); err != nil {
		return "", "", err
	}

	userID, err := getAuthenticatedUserID(ctx)
	if err != nil {
		return "", "", err
	}
	results, _, resultID, err := api.conversationClient.GalleryConverse(ctx, message, userID, conversationID)
	if err != nil {
		return "", "", err
	}

	result := fmt.Sprintf("ConversationAPI.GalleryConverse: results: %+v", results)

	return result, resultID, nil
}
