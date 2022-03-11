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

func createTask(ctx context.Context, createdOn time.Time, eventID persist.DBID, eventCode persist.EventCode) error {
	client, err := newClient(ctx)
	if err != nil {
		return err
	}
	defer client.Close()

	queuePath := viper.GetString("GCLOUD_FEED_TASK_QUEUE")
	scheduleOn := createdOn.Add(time.Duration(viper.GetInt("GCLOUD_FEED_TASK_BUFFER_SECS")) * time.Second)

	req := &taskspb.CreateTaskRequest{
		Parent: queuePath,
		Task: &taskspb.Task{
			Name:         queuePath + "/tasks/" + eventID.String(),
			ScheduleTime: timestamppb.New(scheduleOn),
			MessageType: &taskspb.Task_AppEngineHttpRequest{
				AppEngineHttpRequest: &taskspb.AppEngineHttpRequest{
					HttpMethod:  taskspb.HttpMethod_POST,
					RelativeUri: "/tasks/feed-event",
					Headers: map[string]string{
						"Content-type":  "application/json",
						"Authorization": "Basic " + viper.GetString("FEEDBOT_SECRET"),
					},
				},
			},
		},
	}

	body, err := json.Marshal(EventMessage{eventID, eventCode})
	if err != nil {
		return err
	}

	req.Task.GetAppEngineHttpRequest().Body = body
	_, err = client.CreateTask(ctx, req)
	return err
}
