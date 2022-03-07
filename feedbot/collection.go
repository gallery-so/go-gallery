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

var errInvalidCollectionEvent = errors.New("unknown user event type")

func handleCollectionEvents(ctx context.Context, collectionEventRepo persist.CollectionEventRepository, message event.EventMessage) error {
	switch event.GetSubtypeFromEventTypeID(message.EventTypeID) {
	case event.CollectionCreatedEvent:
		return handleCollectionCreatedEvent(ctx, collectionEventRepo, message)
	case event.CollectionCollectorsNoteAdded:
		return handleCollectionCollectorsNoteAdded(ctx, collectionEventRepo, message)
	case event.CollectionEventType:
		return handleCollectionTokensAdded(ctx, collectionEventRepo, message)
	default:
		return errInvalidCollectionEvent
	}
	return nil
}

func handleCollectionCreatedEvent(ctx context.Context, collectionEventRepo persist.CollectionEventRepository, message event.EventMessage) error {
	event, err := collectionEventRepo.Get(ctx, message.ID)
	if err != nil {
		return err
	}
	if len(event.Event.NFTs) < 1 {
		return nil
	}

	messagePost := map[string]interface{}{
		"content": fmt.Sprintf("**%s** created a collection: %s/%s/%s",
			event.Event.Username, viper.GetString("GALLERY_HOST"), event.Event.Username, event.CollectionID,
		),
		"tts": false,
	}
	payload, err := json.Marshal(messagePost)
	if err != nil {
		return err
	}

	return sendMessage(ctx, payload)
}

func handleCollectionCollectorsNoteAdded(ctx context.Context, collectionEventRepo persist.CollectionEventRepository, message event.EventMessage) error {
	event, err := collectionEventRepo.Get(ctx, message.ID)
	if err != nil {
		return err
	}

	eventsSince, err := collectionEventRepo.GetEventsSince(ctx, event, time.Now())
	if err != nil {
		return err
	}
	if len(eventsSince) > 0 {
		return nil
	}

	messagePost := map[string]interface{}{
		"content": fmt.Sprintf("**%s** added a collector's note to their collection: %s/%s/%s",
			event.Event.Username, viper.GetString("GALLERY_HOST"), event.Event.Username, event.CollectionID,
		),
		"tts": false,
	}
	payload, err := json.Marshal(messagePost)
	if err != nil {
		return err
	}

	return sendMessage(ctx, payload)
}

func handleCollectionTokensAdded(ctx context.Context, collectionEventRepo persist.CollectionEventRepository, message event.EventMessage) error {
	event, err := collectionEventRepo.Get(ctx, message.ID)
	if err != nil {
		return err
	}

	eventsSince, err := collectionEventRepo.GetEventsSince(ctx, event, time.Now())
	if err != nil {
		return err
	}
	if len(eventsSince) > 0 {
		return nil
	}

	messagePost := map[string]interface{}{
		"content": fmt.Sprintf("**%s** added a collector's note to their collection: %s/%s/%s",
			event.Event.Username, viper.GetString("GALLERY_HOST"), event.Event.Username, event.CollectionID,
		),
		"tts": false,
	}
	payload, err := json.Marshal(messagePost)
	if err != nil {
		return err
	}

	return sendMessage(ctx, payload)
}
