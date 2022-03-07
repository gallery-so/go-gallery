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

var errInvalidUserEvent = errors.New("unknown user event type")

func handleUserEvents(ctx context.Context, userEventRepo persist.UserEventRepository, message event.EventMessage) error {
	switch event.GetSubtypeFromEventTypeID(message.EventTypeID) {
	case event.UserCreatedEvent:
		return handleUserCreated(ctx, userEventRepo, message)
	default:
		return errInvalidUserEvent
	}
	return nil
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

	messagePost := map[string]interface{}{
		"content": fmt.Sprintf("**%s** joined Gallery: %s/%s",
			event.Event.Username, viper.GetString("GALLERY_HOST"), event.Event.Username,
		),
		"tts": false,
	}
	payload, err := json.Marshal(messagePost)
	if err != nil {
		return err
	}

	return sendMessage(ctx, payload)
}
