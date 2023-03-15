package task

import (
	"context"
	"fmt"

	"encoding/json"
	"time"

	gcptasks "cloud.google.com/go/cloudtasks/apiv2"
	"github.com/mikeydub/go-gallery/service/logger"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/service/tracing"
	"github.com/mikeydub/go-gallery/util"
	"github.com/spf13/viper"
	"google.golang.org/api/option"
	taskspb "google.golang.org/genproto/googleapis/cloud/tasks/v2"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// FeedMessage is the input message to the feed service
type FeedMessage struct {
	ID persist.DBID `json:"id" binding:"required"`
}

// FeedbotMessage is the input message to the feedbot service
type FeedbotMessage struct {
	FeedEventID persist.DBID   `json:"id" binding:"required"`
	Action      persist.Action `json:"action" binding:"required"`
}

type TokenProcessingUserMessage struct {
	UserID   persist.DBID   `json:"user_id" binding:"required"`
	TokenIDs []persist.DBID `json:"token_ids" binding:"required"`
}

type TokenProcessingContractTokensMessage struct {
	ContractID   persist.DBID `json:"contract_id" binding:"required"`
	ForceRefresh bool         `json:"force_refresh"`
}

// DeepRefreshMessage is the input message to the indexer-api for deep refreshes
type DeepRefreshMessage struct {
	OwnerAddress    persist.EthereumAddress `json:"owner_address"`
	TokenID         persist.TokenID         `json:"token_id"`
	ContractAddress persist.EthereumAddress `json:"contract_address"`
	RefreshRange    persist.BlockRange      `json:"refresh_range"`
}

type ValidateNFTsMessage struct {
	OwnerAddress persist.EthereumAddress `json:"owner_address"`
}

func CreateTaskForFeed(ctx context.Context, scheduleOn time.Time, message FeedMessage, client *gcptasks.Client) error {
	span, ctx := tracing.StartSpan(ctx, "cloudtask.create", "createTaskForFeed")
	defer tracing.FinishSpan(span)

	tracing.AddEventDataToSpan(span, map[string]interface{}{
		"Event ID": message.ID,
	})

	url := fmt.Sprintf("%s/tasks/feed-event", viper.GetString("FEED_URL"))
	logger.For(ctx).Infof("creating task for feed event %s, scheduling on %s, sending to %s", message.ID, scheduleOn, url)

	queue := viper.GetString("GCLOUD_FEED_QUEUE")
	task := &taskspb.Task{
		ScheduleTime: timestamppb.New(scheduleOn),
		MessageType: &taskspb.Task_HttpRequest{
			HttpRequest: &taskspb.HttpRequest{
				HttpMethod: taskspb.HttpMethod_POST,
				Url:        url,
				Headers: map[string]string{
					"Content-type":  "application/json",
					"sentry-trace":  span.TraceID.String(),
					"Authorization": "Basic " + viper.GetString("FEED_SECRET"),
				},
			},
		},
	}

	body, err := json.Marshal(message)
	if err != nil {
		return err
	}

	return submitHttpTask(ctx, client, queue, task, body)
}

func CreateTaskForFeedbot(ctx context.Context, scheduleOn time.Time, message FeedbotMessage, client *gcptasks.Client) error {
	span, ctx := tracing.StartSpan(ctx, "cloudtask.create", "createTaskForFeedbot")
	defer tracing.FinishSpan(span)

	tracing.AddEventDataToSpan(span, map[string]interface{}{
		"Event ID": message.FeedEventID,
	})

	queue := viper.GetString("GCLOUD_FEEDBOT_TASK_QUEUE")
	task := &taskspb.Task{
		Name:         fmt.Sprintf("%s/tasks/%s", queue, message.FeedEventID.String()),
		ScheduleTime: timestamppb.New(scheduleOn),
		MessageType: &taskspb.Task_HttpRequest{
			HttpRequest: &taskspb.HttpRequest{
				HttpMethod: taskspb.HttpMethod_POST,
				Url:        fmt.Sprintf("%s/tasks/feed-event", viper.GetString("FEEDBOT_URL")),
				Headers: map[string]string{
					"Content-type":  "application/json",
					"Authorization": "Basic " + viper.GetString("FEEDBOT_SECRET"),
					"sentry-trace":  span.TraceID.String(),
				},
			},
		},
	}

	body, err := json.Marshal(message)
	if err != nil {
		return err
	}

	return submitHttpTask(ctx, client, queue, task, body)
}

func CreateTaskForTokenProcessing(ctx context.Context, client *gcptasks.Client, message TokenProcessingUserMessage) error {
	span, ctx := tracing.StartSpan(ctx, "cloudtask.create", "createTaskForTokenProcessing")
	defer tracing.FinishSpan(span)

	tracing.AddEventDataToSpan(span, map[string]interface{}{"User ID": message.UserID})

	queue := viper.GetString("TOKEN_PROCESSING_QUEUE")
	task := &taskspb.Task{
		MessageType: &taskspb.Task_HttpRequest{
			HttpRequest: &taskspb.HttpRequest{
				HttpMethod: taskspb.HttpMethod_POST,
				Url:        fmt.Sprintf("%s/media/process", viper.GetString("TOKEN_PROCESSING_URL")),
				Headers: map[string]string{
					"Content-type": "application/json",
					"sentry-trace": span.TraceID.String(),
				},
			},
		},
	}

	body, err := json.Marshal(message)
	if err != nil {
		return err
	}

	return submitHttpTask(ctx, client, queue, task, body)
}

func CreateTaskForContractOwnerProcessing(ctx context.Context, message TokenProcessingContractTokensMessage, client *gcptasks.Client) error {
	span, ctx := tracing.StartSpan(ctx, "cloudtask.create", "createTaskForContractOwnerProcessing")
	defer tracing.FinishSpan(span)

	tracing.AddEventDataToSpan(span, map[string]interface{}{
		"Contract ID": message.ContractID,
	})

	queue := viper.GetString("TOKEN_PROCESSING_QUEUE")
	task := &taskspb.Task{
		MessageType: &taskspb.Task_HttpRequest{
			HttpRequest: &taskspb.HttpRequest{
				HttpMethod: taskspb.HttpMethod_POST,
				Url:        fmt.Sprintf("%s/owners/process/contract", viper.GetString("TOKEN_PROCESSING_URL")),
				Headers: map[string]string{
					"Content-type": "application/json",
					"sentry-trace": span.TraceID.String(),
				},
			},
		},
	}

	body, err := json.Marshal(message)
	if err != nil {
		return err
	}

	return submitHttpTask(ctx, client, queue, task, body)
}

func CreateTaskForDeepRefresh(ctx context.Context, message DeepRefreshMessage, client *gcptasks.Client) error {
	span, ctx := tracing.StartSpan(ctx, "cloudtask.create", "createTaskForDeepRefresh")
	defer tracing.FinishSpan(span)

	queue := viper.GetString("GCLOUD_REFRESH_TASK_QUEUE")
	task := &taskspb.Task{
		MessageType: &taskspb.Task_HttpRequest{
			HttpRequest: &taskspb.HttpRequest{
				HttpMethod: taskspb.HttpMethod_POST,
				Url:        fmt.Sprintf("%s/tasks/refresh", viper.GetString("INDEXER_HOST")),
				Headers: map[string]string{
					"Content-type": "application/json",
					"sentry-trace": span.TraceID.String(),
				},
			},
		},
	}

	body, err := json.Marshal(message)
	if err != nil {
		return err
	}

	return submitHttpTask(ctx, client, queue, task, body)
}

func CreateTaskForWalletValidation(ctx context.Context, message ValidateNFTsMessage, client *gcptasks.Client) error {
	span, ctx := tracing.StartSpan(ctx, "cloudtask.create", "createTaskForWalletValidate")
	defer tracing.FinishSpan(span)

	queue := viper.GetString("GCLOUD_WALLET_VALIDATE_QUEUE")
	task := &taskspb.Task{
		MessageType: &taskspb.Task_HttpRequest{
			HttpRequest: &taskspb.HttpRequest{
				HttpMethod: taskspb.HttpMethod_POST,
				Url:        fmt.Sprintf("%s/nfts/validate", viper.GetString("INDEXER_HOST")),
				Headers: map[string]string{
					"Content-type": "application/json",
					"sentry-trace": span.TraceID.String(),
				},
			},
		},
	}

	body, err := json.Marshal(message)
	if err != nil {
		return err
	}

	return submitHttpTask(ctx, client, queue, task, body)
}

// NewClient returns a new task client with tracing enabled.
func NewClient(ctx context.Context) *gcptasks.Client {
	trace := tracing.NewTracingInterceptor(true)

	copts := []option.ClientOption{
		option.WithGRPCDialOption(grpc.WithUnaryInterceptor(trace.UnaryInterceptor)),
		option.WithGRPCDialOption(grpc.WithTimeout(time.Duration(2) * time.Second)),
	}

	// Configure the client depending on whether or not
	// the cloud task emulator is used.
	if viper.GetString("ENV") == "local" {
		if host := viper.GetString("TASK_QUEUE_HOST"); host != "" {
			copts = append(
				copts,
				option.WithEndpoint(host),
				option.WithGRPCDialOption(grpc.WithTransportCredentials(insecure.NewCredentials())),
				option.WithoutAuthentication(),
			)
		} else {
			fi, err := util.LoadEncryptedServiceKeyOrError("./secrets/dev/service-key-dev.json")
			if err != nil {
				logger.For(ctx).WithError(err).Error("failed to find service key, running without task client")
				return nil
			}
			copts = append(
				copts,
				option.WithCredentialsJSON(fi),
			)
		}
	}

	client, err := gcptasks.NewClient(ctx, copts...)
	if err != nil {
		panic(err)
	}

	return client
}

func submitHttpTask(ctx context.Context, client *gcptasks.Client, queue string, task *taskspb.Task, messageBody []byte) error {
	req := &taskspb.CreateTaskRequest{Parent: queue, Task: task}
	req.Task.GetHttpRequest().Body = messageBody
	_, err := client.CreateTask(ctx, req)
	return err
}
