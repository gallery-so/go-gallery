package feedbot

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/mikeydub/go-gallery/service/event"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/spf13/viper"
)

var errInvalidUserEvent = errors.New("unknown user event type")

func handleUserEvents(ctx context.Context, userEventRepo persist.UserEventRepository, message event.EventMessage) error {
	switch event.GetSubtypeFromEventTypeID(message.EventTypeID) {
	case event.UserCreatedEvent:
		return handleUserCreated(ctx, userEventRepo, message)
	default:
		return errInvalidUserEvent
	}
}

func handleUserCreated(ctx context.Context, userEventRepo persist.UserEventRepository, message event.EventMessage) error {
	event, err := userEventRepo.Get(ctx, message.ID)
	if err != nil {
		return err
	}
	if event.Event.Username == "" {
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
			event.Event.Username, viper.GetString("GALLERY_HOST"), event.Event.Username,
		),
	)
	if err != nil {
		return err
	}

	return sendMessage(ctx, payload)
}
