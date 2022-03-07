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

var errInvalidCollectionEvent = errors.New("unknown user event type")

func handleCollectionEvents(ctx context.Context, userRepo persist.UserRepository, collectionEventRepo persist.CollectionEventRepository, message event.EventMessage) error {
	switch persist.CategoryFromEventID(message.EventID) {
	case persist.CollectionCreatedEvent:
		return handleCollectionCreatedEvent(ctx, userRepo, collectionEventRepo, message)
	case persist.CollectionCollectorsNoteAdded:
		return handleCollectionCollectorsNoteAdded(ctx, userRepo, collectionEventRepo, message)
	case persist.CollectionEventType:
		return handleCollectionTokensAdded(ctx, userRepo, collectionEventRepo, message)
	default:
		return errInvalidCollectionEvent
	}
}

func handleCollectionCreatedEvent(ctx context.Context, userRepo persist.UserRepository, collectionEventRepo persist.CollectionEventRepository, message event.EventMessage) error {
	event, err := collectionEventRepo.Get(ctx, message.ID)
	if err != nil {
		return err
	}
	if len(event.Data.NFTs) < 1 {
		return nil
	}

	user, err := userRepo.GetByID(ctx, event.UserID)
	if err != nil {
		return err
	}
	payload, err := createMessage(
		fmt.Sprintf("**%s** created a collection: %s/%s/%s",
			user.Username, viper.GetString("GALLERY_HOST"), user.Username, event.CollectionID,
		),
	)
	if err != nil {
		return err
	}

	return sendMessage(ctx, payload)
}

func handleCollectionCollectorsNoteAdded(ctx context.Context, userRepo persist.UserRepository, collectionEventRepo persist.CollectionEventRepository, message event.EventMessage) error {
	event, err := collectionEventRepo.Get(ctx, message.ID)
	if err != nil {
		return err
	}
	if event.Data.CollectorsNote == "" {
		return nil
	}

	eventsSince, err := collectionEventRepo.GetEventsSince(ctx, event, time.Now())
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
		fmt.Sprintf("**%s** added a collector's note to their collection: %s/%s/%s",
			user.Username, viper.GetString("GALLERY_HOST"), user.Username, event.CollectionID,
		),
	)
	if err != nil {
		return err
	}

	return sendMessage(ctx, payload)
}

func handleCollectionTokensAdded(ctx context.Context, userRepo persist.UserRepository, collectionEventRepo persist.CollectionEventRepository, message event.EventMessage) error {
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

	user, err := userRepo.GetByID(ctx, event.UserID)
	if err != nil {
		return err
	}
	payload, err := createMessage(
		fmt.Sprintf("**%s** added a collector's note to their collection: %s/%s/%s",
			user.Username, viper.GetString("GALLERY_HOST"), user.Username, event.CollectionID,
		),
	)
	if err != nil {
		return err
	}

	return sendMessage(ctx, payload)
}
