package publicapi

import (
	"context"

	"github.com/gin-gonic/gin"
	"github.com/mikeydub/go-gallery/service/auth"
	"github.com/mikeydub/go-gallery/service/event"
	"github.com/mikeydub/go-gallery/service/persist"
)

type UserWithDispatch struct {
	PublicUserAPI
	gc *gin.Context
}

// Forwarding methods to wrapped API
func (u UserWithDispatch) UpdateUserInfo(ctx context.Context, username, bio string) error {
	err := u.PublicUserAPI.UpdateUserInfo(ctx, username, bio)
	if err != nil {
		return err
	}

	evt := persist.UserEventRecord{
		UserID: auth.GetUserIDFromCtx(u.gc),
		Code:   persist.UserCreatedEvent,
		Data:   persist.UserEvent{Username: username, Bio: persist.NullString(bio)},
	}

	userHandlers := event.For(u.gc).User
	userHandlers.Dispatch(evt)

	return err
}
