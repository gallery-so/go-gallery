package cloudtask

import (
	"context"
	"fmt"

	"encoding/json"
	"time"

	gcptasks "cloud.google.com/go/cloudtasks/apiv2"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/service/tracing"
	"github.com/spf13/viper"
	"google.golang.org/api/option"
	taskspb "google.golang.org/genproto/googleapis/cloud/tasks/v2"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type EventMessage struct {
	ID        persist.DBID      `json:"id" binding:"required"`
	EventCode persist.EventCode `json:"event_code" binding:"required"`
}

func newClient(ctx context.Context) (*gcptasks.Client, error) {
	trace := tracing.NewTracingInterceptor(true)
	dopt := grpc.WithUnaryInterceptor(trace.UnaryInterceptor)
	copts := []option.ClientOption{option.WithGRPCDialOption(dopt)}

	if viper.GetString("ENV") == "local" {
		copts = append(
			copts,
			option.WithEndpoint(viper.GetString("TASK_QUEUE_HOST")),
			option.WithGRPCDialOption(grpc.WithTransportCredentials(insecure.NewCredentials())),
			option.WithoutAuthentication(),
		)
	}

	return gcptasks.NewClient(ctx, copts...)
}

func submitTask(ctx context.Context, client *gcptasks.Client, queue string, task *taskspb.Task, messageBody []byte) error {
	req := &taskspb.CreateTaskRequest{Parent: queue, Task: task}
	req.Task.GetAppEngineHttpRequest().Body = messageBody
	_, err := client.CreateTask(ctx, req)
	return err
}

func createTaskForFeedbot(ctx context.Context, createdOn time.Time, eventID persist.DBID, eventCode persist.EventCode) error {
	span, ctx := tracing.StartSpan(ctx, "cloudtask.create", "createTaskForFeedbot")
	defer tracing.FinishSpan(span)

	tracing.AddEventDataToSpan(span, map[string]interface{}{
		"Event ID":   eventID,
		"Event Code": eventCode,
	})

	client, err := newClient(ctx)
	if err != nil {
		return err
	}
	defer client.Close()

	queue := viper.GetString("GCLOUD_FEED_TASK_QUEUE")
	task := &taskspb.Task{
		Name:         fmt.Sprintf("%s/tasks/%s", queue, eventID.String()),
		ScheduleTime: timestamppb.New(createdOn.Add(time.Duration(viper.GetInt("GCLOUD_FEED_TASK_BUFFER_SECS")) * time.Second)),
		MessageType: &taskspb.Task_AppEngineHttpRequest{
			AppEngineHttpRequest: &taskspb.AppEngineHttpRequest{
				HttpMethod:       taskspb.HttpMethod_POST,
				AppEngineRouting: &taskspb.AppEngineRouting{Service: "feedbot"},
				RelativeUri:      "/tasks/feed-event",
				Headers: map[string]string{
					"Content-type":  "application/json",
					"Authorization": "Basic " + viper.GetString("FEEDBOT_SECRET"),
					"sentry-trace":  span.TraceID.String(),
				},
			},
		},
	}

	body, _ := json.Marshal(EventMessage{ID: eventID, EventCode: eventCode})

	return submitTask(ctx, client, queue, task, body)
}
