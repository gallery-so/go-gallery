package cloudtask

import (
	"context"
	"time"

	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/sirupsen/logrus"
)

type UserFeedTask struct {
	UserEventRepo persist.UserEventRepository
}

func (u UserFeedTask) Handle(event persist.UserEventRecord) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	eventID, err := u.UserEventRepo.Add(ctx, event)
	if err != nil {
		logrus.Errorf("failed to add event to user event repo: %s", err)
		return
	}

	err = createTaskForFeedbot(ctx, time.Time(event.CreationTime), eventID, event.Code)
	if err != nil {
		logrus.Errorf("failed to create task: %s", err)
		return
	}
}
