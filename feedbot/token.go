package feedbot

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/mikeydub/go-gallery/service/event"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/spf13/viper"
)

var errInvalidTokenEvent = errors.New("unknown user event type")

func handleTokenEvents(ctx context.Context, tokenEventRepo persist.TokenEventRepository, message event.EventMessage) error {
	switch event.GetSubtypeFromEventTypeID(message.EventTypeID) {
	case event.TokenCollectorsNoteAddedEvent:
		return handleTokenCollectorsNoteAdded(ctx, tokenEventRepo, message)
	default:
		return errInvalidTokenEvent
	}
	return nil
}

func handleTokenCollectorsNoteAdded(ctx context.Context, tokenEventRepo persist.TokenEventRepository, message event.EventMessage) error {
	event, err := tokenEventRepo.Get(ctx, message.ID)
	if err != nil {
		return err
	}

	eventsSince, err := tokenEventRepo.GetEventsSince(ctx, event, time.Now())
	if err != nil {
		return err
	}
	if len(eventsSince) > 0 {
		return nil
	}

	messagePost := map[string]interface{}{
		"content": fmt.Sprintf("**%s** added a collector's note to their NFT: %s/%s/%s/%s",
			event.Event.Username, viper.GetString("GALLERY_HOST"), event.Event.Username, event.Event.CollectionID, event.TokenID,
		),
		"tts": false,
	}
	payload, err := json.Marshal(messagePost)
	if err != nil {
		return err
	}

	return sendMessage(ctx, payload)
}
