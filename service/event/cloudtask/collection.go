package cloudtask

import (
	"context"
	"time"

	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/sirupsen/logrus"
)

type CollectionFeedTask struct {
	CollectionEventRepo persist.CollectionEventRepository
}

func (c CollectionFeedTask) Handle(record persist.CollectionEventRecord) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	saved, err := c.CollectionEventRepo.Add(ctx, record)
	if err != nil {
		logrus.Errorf("failed to add event to collection event repo: %s", err)
		return
	}

	err = createTaskForFeedbot(ctx, time.Time(saved.CreationTime), saved.ID, saved.Code)
	if err != nil {
		logrus.Errorf("failed to create task: %s", err)
		return
	}
}
