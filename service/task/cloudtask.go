package task

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	gcptasks "cloud.google.com/go/cloudtasks/apiv2"
	taskspb "cloud.google.com/go/cloudtasks/apiv2/cloudtaskspb"
	"github.com/getsentry/sentry-go"
	"github.com/mikeydub/go-gallery/service/auth/basicauth"
	"google.golang.org/api/option"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/mikeydub/go-gallery/env"
	"github.com/mikeydub/go-gallery/service/logger"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/service/tracing"
	"github.com/mikeydub/go-gallery/util"
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

type TokenProcessingTokenInstanceMessage struct {
	TokenDBID persist.DBID `json:"token_dbid" binding:"required"`
	Attempts  int          `json:"attempts" binding:"required"`
}

type TokenProcessingContractTokensMessage struct {
	ContractID   persist.DBID `json:"contract_id" binding:"required"`
	ForceRefresh bool         `json:"force_refresh"`
}

type AddEmailToMailingListMessage struct {
	UserID persist.DBID `json:"user_id" binding:"required"`
}

type TokenIdentifiersQuantities map[persist.TokenUniqueIdentifiers]persist.HexString

func (t TokenIdentifiersQuantities) MarshalJSON() ([]byte, error) {
	m := make(map[string]string)
	for k, v := range t {
		m[k.String()] = v.String()
	}
	return json.Marshal(m)
}

func (t *TokenIdentifiersQuantities) UnmarshalJSON(b []byte) error {
	m := make(map[string]string)
	if err := json.Unmarshal(b, &m); err != nil {
		return err
	}
	result := make(TokenIdentifiersQuantities)
	for k, v := range m {
		identifiers, err := persist.TokenUniqueIdentifiersFromString(k)
		if err != nil {
			return err
		}
		result[identifiers] = persist.HexString(v)
	}
	*t = result
	return nil
}

type TokenProcessingUserTokensMessage struct {
	UserID           persist.DBID               `json:"user_id" binding:"required"`
	TokenIdentifiers TokenIdentifiersQuantities `json:"token_identifiers" binding:"required"`
}

type TokenProcessingWalletRemovalMessage struct {
	UserID    persist.DBID   `json:"user_id" binding:"required"`
	WalletIDs []persist.DBID `json:"wallet_ids" binding:"required"`
}

type ValidateNFTsMessage struct {
	OwnerAddress persist.EthereumAddress `json:"wallet"`
}

type PushNotificationMessage struct {
	PushTokenID persist.DBID   `json:"pushTokenID"`
	Title       string         `json:"title"`
	Subtitle    string         `json:"subtitle"`
	Body        string         `json:"body"`
	Data        map[string]any `json:"data"`
	Sound       bool           `json:"sound"`
	Badge       int            `json:"badge"`
}

func CreateTaskForPushNotification(ctx context.Context, message PushNotificationMessage, client *gcptasks.Client) error {
	span, ctx := tracing.StartSpan(ctx, "cloudtask.create", "createTaskForPushNotification")
	defer tracing.FinishSpan(span)
	tracing.AddEventDataToSpan(span, map[string]any{"PushTokenID": message.PushTokenID})
	queue := env.GetString("GCLOUD_PUSH_NOTIFICATIONS_QUEUE")
	url := fmt.Sprintf("%s/tasks/send-push-notification", env.GetString("PUSH_NOTIFICATIONS_URL"))
	secret := env.GetString("PUSH_NOTIFICATIONS_SECRET")
	return submitTask(ctx, client, queue, url, withJSON(message), withTrace(span), withBasicAuth(secret))
}

func CreateTaskForFeed(ctx context.Context, scheduleOn time.Time, message FeedMessage, client *gcptasks.Client) error {
	span, ctx := tracing.StartSpan(ctx, "cloudtask.create", "createTaskForFeed")
	defer tracing.FinishSpan(span)
	tracing.AddEventDataToSpan(span, map[string]any{"Event ID": message.ID})
	queue := env.GetString("GCLOUD_FEED_QUEUE")
	url := fmt.Sprintf("%s/tasks/feed-event", env.GetString("FEED_URL"))
	secret := env.GetString("FEED_SECRET")
	return submitTask(ctx, client, queue, url, withScheduleOn(scheduleOn), withJSON(message), withTrace(span), withBasicAuth(secret))
}

func CreateTaskForFeedbot(ctx context.Context, scheduleOn time.Time, message FeedbotMessage, client *gcptasks.Client) error {
	span, ctx := tracing.StartSpan(ctx, "cloudtask.create", "createTaskForFeedbot")
	defer tracing.FinishSpan(span)
	tracing.AddEventDataToSpan(span, map[string]any{"Event ID": message.FeedEventID})
	queue := env.GetString("GCLOUD_FEEDBOT_TASK_QUEUE")
	url := fmt.Sprintf("%s/tasks/feed-event", env.GetString("FEEDBOT_URL"))
	secret := env.GetString("FEEDBOT_SECRET")
	return submitTask(ctx, client, queue, url, withScheduleOn(scheduleOn), withJSON(message), withTrace(span), withBasicAuth(secret))
}

func CreateTaskForTokenProcessing(ctx context.Context, message TokenProcessingUserMessage, client *gcptasks.Client) error {
	span, ctx := tracing.StartSpan(ctx, "cloudtask.create", "createTaskForTokenProcessing")
	defer tracing.FinishSpan(span)
	tracing.AddEventDataToSpan(span, map[string]any{"User ID": message.UserID})
	queue := env.GetString("TOKEN_PROCESSING_QUEUE")
	url := fmt.Sprintf("%s/media/process", env.GetString("TOKEN_PROCESSING_URL"))
	return submitTask(ctx, client, queue, url, withDeadline(time.Minute*30), withJSON(message), withTrace(span))
}

func CreateTaskForContractOwnerProcessing(ctx context.Context, message TokenProcessingContractTokensMessage, client *gcptasks.Client) error {
	span, ctx := tracing.StartSpan(ctx, "cloudtask.create", "createTaskForContractOwnerProcessing")
	defer tracing.FinishSpan(span)
	tracing.AddEventDataToSpan(span, map[string]any{"Contract ID": message.ContractID})
	queue := env.GetString("TOKEN_PROCESSING_QUEUE")
	url := fmt.Sprintf("%s/owners/process/contract", env.GetString("TOKEN_PROCESSING_URL"))
	return submitTask(ctx, client, queue, url, withJSON(message), withTrace(span))
}

func CreateTaskForUserTokenProcessing(ctx context.Context, message TokenProcessingUserTokensMessage, client *gcptasks.Client) error {
	span, ctx := tracing.StartSpan(ctx, "cloudtask.create", "createTaskForUserTokenProcessing")
	defer tracing.FinishSpan(span)
	tracing.AddEventDataToSpan(span, map[string]any{"User ID": message.UserID})
	queue := env.GetString("TOKEN_PROCESSING_QUEUE")
	url := fmt.Sprintf("%s/owners/process/user", env.GetString("TOKEN_PROCESSING_URL"))
	return submitTask(ctx, client, queue, url, withJSON(message), withTrace(span))
}

func CreateTaskForTokenInstanceTokenProcessing(ctx context.Context, message TokenProcessingTokenInstanceMessage, client *gcptasks.Client, delay time.Duration) error {
	span, ctx := tracing.StartSpan(ctx, "cloudtask.create", "createTaskForTokenInstanceTokenProcessing")
	defer tracing.FinishSpan(span)
	queue := env.GetString("TOKEN_PROCESSING_QUEUE")
	url := fmt.Sprintf("%s/media/process/token-id", env.GetString("TOKEN_PROCESSING_URL"))
	return submitTask(ctx, client, queue, url, withJSON(message), withTrace(span), withScheduleOn(time.Now().Add(delay)))
}

func CreateTaskForWalletRemoval(ctx context.Context, message TokenProcessingWalletRemovalMessage, client *gcptasks.Client) error {
	span, ctx := tracing.StartSpan(ctx, "cloudtask.create", "createTaskForWalletRemoval")
	defer tracing.FinishSpan(span)
	tracing.AddEventDataToSpan(span, map[string]any{"User ID": message.UserID, "Wallet IDs": message.WalletIDs})
	queue := env.GetString("TOKEN_PROCESSING_QUEUE")
	url := fmt.Sprintf("%s/owners/process/wallet-removal", env.GetString("TOKEN_PROCESSING_URL"))
	return submitTask(ctx, client, queue, url, withJSON(message), withTrace(span))
}

func CreateTaskForAddingEmailToMailingList(ctx context.Context, message AddEmailToMailingListMessage, client *gcptasks.Client) error {
	span, ctx := tracing.StartSpan(ctx, "cloudtask.create", "createTaskForAddingEmailToMailingList")
	defer tracing.FinishSpan(span)
	tracing.AddEventDataToSpan(span, map[string]any{"User ID": message.UserID})
	queue := env.GetString("EMAILS_QUEUE")
	url := fmt.Sprintf("%s/send/process/add-to-mailing-list", env.GetString("EMAILS_HOST"))
	secret := env.GetString("EMAILS_TASK_SECRET")
	return submitTask(ctx, client, queue, url, withJSON(message), withTrace(span), withBasicAuth(secret))
}

// NewClient returns a new task client with tracing enabled.
func NewClient(ctx context.Context) *gcptasks.Client {
	trace := tracing.NewTracingInterceptor(true)

	copts := []option.ClientOption{
		option.WithGRPCDialOption(grpc.WithUnaryInterceptor(trace.UnaryInterceptor)),
		option.WithGRPCDialOption(grpc.WithTimeout(time.Duration(2) * time.Second)),
	}

	// Configure the client depending on whether or not the cloud task emulator is used.
	if env.GetString("ENV") == "local" {
		if host := env.GetString("TASK_QUEUE_HOST"); host != "" {
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

func withScheduleOn(scheduleOn time.Time) func(*taskspb.Task) error {
	return func(t *taskspb.Task) error {
		t.ScheduleTime = timestamppb.New(scheduleOn)
		return nil
	}
}

func withDeadline(d time.Duration) func(*taskspb.Task) error {
	return func(t *taskspb.Task) error {
		t.DispatchDeadline = durationpb.New(d)
		return nil
	}
}

func withBasicAuth(secret string) func(*taskspb.Task) error {
	return func(t *taskspb.Task) error {
		addHeader(t.GetHttpRequest(), "Authorization", basicauth.MakeHeader(nil, env.GetString("PUSH_NOTIFICATIONS_SECRET")))
		return nil
	}
}

func withJSON(data any) func(*taskspb.Task) error {
	return func(t *taskspb.Task) error {
		body, err := json.Marshal(data)
		if err != nil {
			return err
		}
		t.GetHttpRequest().Body = body
		addHeader(t.GetHttpRequest(), "Content-type", "application/json")
		return nil
	}
}

func withTrace(span *sentry.Span) func(*taskspb.Task) error {
	return func(t *taskspb.Task) error {
		addHeader(t.GetHttpRequest(), "sentry-trace", span.TraceID.String())
		return nil
	}
}

func addHeader(r *taskspb.HttpRequest, key, value string) {
	if r.Headers == nil {
		r.Headers = map[string]string{}
	}
	r.Headers[key] = value
}

func submitTask(ctx context.Context, c *gcptasks.Client, queue, url string, opts ...func(*taskspb.Task) error) error {
	task := &taskspb.Task{
		MessageType: &taskspb.Task_HttpRequest{
			HttpRequest: &taskspb.HttpRequest{
				HttpMethod: taskspb.HttpMethod_POST,
				Url:        url,
			},
		},
	}
	for _, opt := range opts {
		if err := opt(task); err != nil {
			return err
		}
	}
	_, err := c.CreateTask(ctx, &taskspb.CreateTaskRequest{Parent: queue, Task: task})
	return err
}
