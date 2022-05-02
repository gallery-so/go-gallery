package cloudtask

import (
	"context"
	"time"

	"github.com/mikeydub/go-gallery/service/persist"
	sentryutil "github.com/mikeydub/go-gallery/service/sentry"
	"github.com/sirupsen/logrus"
)

type NftFeedEvent struct {
	NftEventRepo persist.NftEventRepository
}

func (t NftFeedEvent) Handle(ctx context.Context, event persist.NftEventRecord) {
	hub := sentryutil.SentryHubFromContext(ctx)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	saved, err := t.NftEventRepo.Add(ctx, event)
	if err != nil {
		logrus.Errorf("failed to add event to token event repo: %s", err)

		if hub != nil {
			hub.CaptureException(err)
		}

		return
	}

	err = createTaskForFeedbot(ctx, time.Time(saved.CreationTime), saved.ID, saved.Code)
	if err != nil {
		logrus.Errorf("failed to create task: %s", err)

		if hub != nil {
			hub.CaptureException(err)
		}

		return
	}
}
