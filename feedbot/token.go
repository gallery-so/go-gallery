package feedbot

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/mikeydub/go-gallery/event"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/spf13/viper"
)

var errInvalidTokenEvent = errors.New("unknown user event type")

func handleTokenEvents(ctx context.Context, userRepo persist.UserRepository, tokenEventRepo persist.TokenEventRepository, message event.EventMessage) error {
	switch persist.NameFromEventType(message.EventType) {
	case persist.TokenCollectorsNoteAddedEvent:
		return handleTokenCollectorsNoteAdded(ctx, userRepo, tokenEventRepo, message)
	default:
		return errInvalidTokenEvent
	}
}

func handleTokenCollectorsNoteAdded(ctx context.Context, userRepo persist.UserRepository, tokenEventRepo persist.TokenEventRepository, message event.EventMessage) error {
	event, err := tokenEventRepo.Get(ctx, message.ID)
	if err != nil {
		return err
	}
	if event.Data.CollectorsNote == "" {
		return nil
	}

	eventsSince, err := tokenEventRepo.GetEventsSince(ctx, event, time.Now())
	if err != nil {
		return err
	}
	if len(eventsSince) > 0 {
		return nil
	}

	user, err := userRepo.GetByID(ctx, event.UserID)
	if err != nil {
		return err
	}
	payload, err := createMessage(
		fmt.Sprintf("**%s** added a collector's note to their NFT: %s/%s/%s/%s",
			user.Username, viper.GetString("GALLERY_HOST"), user.Username, event.Data.CollectionID, event.TokenID,
		),
	)
	if err != nil {
		return err
	}

	return sendMessage(ctx, payload)
}
