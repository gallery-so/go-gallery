package feedbot

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/mikeydub/go-gallery/service/event/cloudtask"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/spf13/viper"
)

var errInvalidCollectionEvent = errors.New("unknown collection event type")
var errMissingCollectionEvent = errors.New("collection event does not exist")

func handleCollectionEvents(ctx context.Context, userRepo persist.UserRepository, collectionEventRepo persist.CollectionEventRepository, message cloudtask.EventMessage) error {
	var err error
	switch message.EventCode {
	case persist.CollectionCreatedEvent:
		err = handleCollectionCreatedEvent(ctx, userRepo, collectionEventRepo, message)
	case persist.CollectionCollectorsNoteAdded:
		err = handleCollectionCollectorsNoteAdded(ctx, userRepo, collectionEventRepo, message)
	case persist.CollectionTokensAdded:
		err = handleCollectionTokensAdded(ctx, userRepo, collectionEventRepo, message)
	default:
		err = errInvalidCollectionEvent
	}
	if err == sql.ErrNoRows {
		return errMissingCollectionEvent
	}
	return err
}

func handleCollectionCreatedEvent(ctx context.Context, userRepo persist.UserRepository, collectionEventRepo persist.CollectionEventRepository, message cloudtask.EventMessage) error {
	event, err := collectionEventRepo.Get(ctx, message.ID)
	if err != nil {
		return err
	}

	// Don't send empty collections.
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

func handleCollectionCollectorsNoteAdded(ctx context.Context, userRepo persist.UserRepository, collectionEventRepo persist.CollectionEventRepository, message cloudtask.EventMessage) error {
	event, err := collectionEventRepo.Get(ctx, message.ID)
	if err != nil {
		return err
	}

	// Don't send with empty notes.
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
		fmt.Sprintf("**%s** updated their collection's details: %s/%s/%s",
			user.Username, viper.GetString("GALLERY_HOST"), user.Username, event.CollectionID,
		),
	)
	if err != nil {
		return err
	}

	return sendMessage(ctx, payload)
}

func handleCollectionTokensAdded(ctx context.Context, userRepo persist.UserRepository, collectionEventRepo persist.CollectionEventRepository, message cloudtask.EventMessage) error {
	event, err := collectionEventRepo.Get(ctx, message.ID)
	if err != nil {
		return err
	}

	// Don't send empty collections.
	if len(event.Data.NFTs) < 1 {
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
		fmt.Sprintf("**%s** curated the NFT(s) in their collection: %s/%s/%s",
			user.Username, viper.GetString("GALLERY_HOST"), user.Username, event.CollectionID,
		),
	)
	if err != nil {
		return err
	}

	return sendMessage(ctx, payload)
}
