package cloudtask

import (
	"context"
	"time"

	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/sirupsen/logrus"
)

type NftFeedEvent struct {
	NftEventRepo persist.NftEventRepository
}

func (t NftFeedEvent) Handle(event persist.NftEventRecord) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	eventID, err := t.NftEventRepo.Add(ctx, event)
	if err != nil {
		logrus.Errorf("failed to add event to token event repo: %s", err)
		return
	}

	err = createTaskForFeedbot(ctx, time.Time(event.CreationTime), eventID, event.Code)
	if err != nil {
		logrus.Errorf("failed to create task: %s", err)
		return
	}
}
