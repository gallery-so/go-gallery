// TODO: Remove when the feedbot uses the feed API instead of creating its own posts.
// Everything below can be removed.
package task

import (
	"context"
	"time"

	"github.com/mikeydub/go-gallery/service/logger"

	"github.com/mikeydub/go-gallery/service/persist"
	sentryutil "github.com/mikeydub/go-gallery/service/sentry"
)

type UserFeedTask struct {
	UserEventRepo persist.UserEventRepository
}

func (u UserFeedTask) Handle(ctx context.Context, event persist.UserEventRecord) {
	hub := sentryutil.SentryHubFromContext(ctx)

	saved, err := u.UserEventRepo.Add(ctx, event)
	if err != nil {
		logger.For(ctx).Errorf("failed to add event to user event repo: %s", err)

		if hub != nil {
			hub.CaptureException(err)
		}

		return
	}

	err = createTaskForFeedbot(ctx, time.Time(saved.CreationTime), saved.ID, saved.Code)
	if err != nil {
		logger.For(ctx).Errorf("failed to create task: %s", err)

		if hub != nil {
			hub.CaptureException(err)
		}

		return
	}
}
