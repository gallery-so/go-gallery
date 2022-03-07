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

var errInvalidUserEvent = errors.New("unknown user event type")

func handleUserEvents(ctx context.Context, userRepo persist.UserRepository, userEventRepo persist.UserEventRepository, message event.EventMessage) error {
	switch persist.NameFromEventCode(message.EventCode) {
	case persist.UserCreatedEvent:
		return handleUserCreated(ctx, userRepo, userEventRepo, message)
	default:
		return errInvalidUserEvent
	}
}

func handleUserCreated(ctx context.Context, userRepo persist.UserRepository, userEventRepo persist.UserEventRepository, message event.EventMessage) error {
	event, err := userEventRepo.Get(ctx, message.ID)
	if err != nil {
		return err
	}
	user, err := userRepo.GetByID(ctx, event.UserID)
	if err != nil {
		return err
	}
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
