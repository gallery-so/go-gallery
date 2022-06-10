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

type NftFeedEvent struct {
	NftEventRepo persist.NftEventRepository
}

func (t NftFeedEvent) Handle(ctx context.Context, event persist.NftEventRecord) {
	hub := sentryutil.SentryHubFromContext(ctx)

	saved, err := t.NftEventRepo.Add(ctx, event)
	if err != nil {
		logger.For(ctx).Errorf("failed to add event to token event repo: %s", err)

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
