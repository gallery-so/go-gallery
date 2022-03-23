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

var errInvalidNftEvent = errors.New("unknown nft event type")
var errMissingNftEvent = errors.New("nft event does not exist")

func handleNftEvents(ctx context.Context, userRepo persist.UserRepository, nftEventRepo persist.NftEventRepository, message cloudtask.EventMessage) error {
	var err error
	switch message.EventCode {
	case persist.NftCollectorsNoteAddedEvent:
		err = handleNftCollectorsNoteAdded(ctx, userRepo, nftEventRepo, message)
	default:
		err = errInvalidNftEvent
	}
	if err == sql.ErrNoRows {
		return errMissingNftEvent
	}
	return err
}

func handleNftCollectorsNoteAdded(ctx context.Context, userRepo persist.UserRepository, nftEventRepo persist.NftEventRepository, message cloudtask.EventMessage) error {
	event, err := nftEventRepo.Get(ctx, message.ID)
	if err != nil {
		return err
	}

	// Don't send with empty notes.
	if event.Data.CollectorsNote == "" {
		return nil
	}

	eventsSince, err := nftEventRepo.GetEventsSince(ctx, event, time.Now())
	if err != nil {
		return err
	}
	if len(eventsSince) > 0 {
		return nil
	}

	eventBefore, err := nftEventRepo.GetEventBefore(ctx, event)
	if err != nil {
		return err
	}

	// Don't send if the note is the same as before.
	if eventBefore != nil && (event.Data.CollectorsNote == eventBefore.Data.CollectorsNote) {
		return nil
	}

	user, err := userRepo.GetByID(ctx, event.UserID)
	if err != nil {
		return err
	}
	payload, err := createMessage(
		fmt.Sprintf("**%s** added a collector's note to their NFT: %s/%s/%s/%s",
			user.Username, viper.GetString("GALLERY_HOST"), user.Username, event.Data.CollectionID, event.NftID,
		),
	)
	if err != nil {
		return err
	}

	return sendMessage(ctx, payload)
}
