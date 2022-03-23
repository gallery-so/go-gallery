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

var errInvalidUserEvent = errors.New("unknown user event type")
var errMissingUserEvent = errors.New("user event does not exist")

func handleUserEvents(ctx context.Context, userRepo persist.UserRepository, userEventRepo persist.UserEventRepository, message cloudtask.EventMessage) error {
	var err error
	switch message.EventCode {
	case persist.UserCreatedEvent:
		err = handleUserCreated(ctx, userRepo, userEventRepo, message)
	default:
		err = errInvalidUserEvent
	}
	if err == sql.ErrNoRows {
		return errMissingUserEvent
	}
	return err
}

func handleUserCreated(ctx context.Context, userRepo persist.UserRepository, userEventRepo persist.UserEventRepository, message cloudtask.EventMessage) error {
	event, err := userEventRepo.Get(ctx, message.ID)
	if err != nil {
		return err
	}
	user, err := userRepo.GetByID(ctx, event.UserID)
	if err != nil {
		return err
	}

	// Make sure the username is set.
	if user.Username == "" {
		return nil
	}

	eventsSince, err := userEventRepo.GetEventsSince(ctx, event, time.Now())
	if err != nil {
		return err
	}
	if len(eventsSince) > 0 {
		return nil
	}

	eventBefore, err := userEventRepo.GetEventBefore(ctx, event)
	if err != nil {
		return err
	}

	// Don't send if the username is the same as before.
	if eventBefore != nil && (event.Data.Username == eventBefore.Data.Username) {
		return nil
	}

	payload, err := createMessage(
		fmt.Sprintf("**%s** joined Gallery: %s/%s",
			user.Username, viper.GetString("GALLERY_HOST"), user.Username,
		),
	)
	if err != nil {
		return err
	}

	return sendMessage(ctx, payload)
}
