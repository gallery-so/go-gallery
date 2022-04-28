package cloudtask

import (
	"context"
	"time"

	"github.com/mikeydub/go-gallery/service/persist"
	sentryutil "github.com/mikeydub/go-gallery/service/sentry"
	"github.com/sirupsen/logrus"
)

type CollectionFeedTask struct {
	CollectionEventRepo persist.CollectionEventRepository
}

func (c CollectionFeedTask) Handle(ctx context.Context, record persist.CollectionEventRecord) {
	hub := sentryutil.SentryHubFromContext(ctx)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	saved, err := c.CollectionEventRepo.Add(ctx, record)
	if err != nil {
		logrus.Errorf("failed to add event to collection event repo: %s", err)

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
