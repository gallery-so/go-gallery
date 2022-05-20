package cloudtask

import (
	"context"
	"encoding/json"
	"time"

	gcptasks "cloud.google.com/go/cloudtasks/apiv2"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/spf13/viper"
	"google.golang.org/api/option"
	taskspb "google.golang.org/genproto/googleapis/cloud/tasks/v2"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type EventMessage struct {
	ID        persist.DBID      `json:"id" binding:"required"`
	EventCode persist.EventCode `json:"event_code" binding:"required"`
}

func newClient(ctx context.Context) (*gcptasks.Client, error) {
	if viper.GetString("ENV") != "local" {
		return gcptasks.NewClient(ctx)
	} else {
		conn, err := grpc.Dial(viper.GetString("TASK_QUEUE_HOST"), grpc.WithInsecure())
		if err != nil {
			return nil, err
		}
		clientOpt := option.WithGRPCConn(conn)
		return gcptasks.NewClient(ctx, clientOpt)
	}
}

func createTaskForService(ctx context.Context, queuePath string, scheduleOn time.Time, service string, uri string, headers map[string]string, message EventMessage) error {
	client, err := newClient(ctx)
	if err != nil {
		return err
	}
	defer client.Close()

	req := &taskspb.CreateTaskRequest{
		Parent: queuePath,
		Task: &taskspb.Task{
			Name:         queuePath + "/tasks/" + message.ID.String(),
			ScheduleTime: timestamppb.New(scheduleOn),
			MessageType: &taskspb.Task_AppEngineHttpRequest{
				AppEngineHttpRequest: &taskspb.AppEngineHttpRequest{
					HttpMethod: taskspb.HttpMethod_POST,
					// XXX: AppEngineRouting: &taskspb.AppEngineRouting{Service: service},
					RelativeUri: uri,
					Headers:     headers,
				},
			},
		},
	}

	body, err := json.Marshal(message)
	if err != nil {
		return err
	}

	req.Task.GetAppEngineHttpRequest().Body = body
	_, err = client.CreateTask(ctx, req)
	return err
}

func createTaskForFeedbot(ctx context.Context, createdOn time.Time, eventID persist.DBID, eventCode persist.EventCode) error {
	queuePath := viper.GetString("GCLOUD_FEED_TASK_QUEUE")
	buffer := viper.GetInt("GCLOUD_FEED_TASK_BUFFER_SECS")
	scheduleOn := createdOn.Add(time.Duration(buffer) * time.Second)

	headers := map[string]string{
		"Content-type":  "application/json",
		"Authorization": "Basic " + viper.GetString("FEEDBOT_SECRET"),
	}

	message := EventMessage{ID: eventID, EventCode: eventCode}

	return createTaskForService(ctx, queuePath, scheduleOn, "feedbot", "/tasks/feed-event", headers, message)
}
