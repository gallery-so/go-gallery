// TODO: Remove when the feedbot uses the feed API instead of creating its own posts.
// Everything below can be removed.
package task

import (
	"context"
	"time"

	"github.com/mikeydub/go-gallery/service/logger"
	sentryutil "github.com/mikeydub/go-gallery/service/sentry"

	"github.com/mikeydub/go-gallery/service/persist"
)

type CollectionFeedTask struct {
	CollectionEventRepo persist.CollectionEventRepository
}

func (c CollectionFeedTask) Handle(ctx context.Context, record persist.CollectionEventRecord) {
	hub := sentryutil.SentryHubFromContext(ctx)

	saved, err := c.CollectionEventRepo.Add(ctx, record)
	if err != nil {
		logger.For(ctx).Errorf("failed to add event to collection event repo: %s", err)

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
