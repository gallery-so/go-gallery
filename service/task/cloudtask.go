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

type FeedbotSlackPostMessage struct {
	PostID persist.DBID `json:"post_id" binding:"required"`
}

type TokenProcessingUserMessage struct {
	UserID   persist.DBID   `json:"user_id" binding:"required"`
	TokenIDs []persist.DBID `json:"token_ids" binding:"required"`
}

type TokenProcessingContractTokensMessage struct {
	ContractID   persist.DBID `json:"contract_id" binding:"required"`
	ForceRefresh bool         `json:"force_refresh"`
}

type TokenProcessingTokenMessage struct {
	Token    persist.TokenIdentifiers `json:"token" binding:"required"`
	Attempts int                      `json:"attempts" binding:"required"`
}

type AddEmailToMailingListMessage struct {
	UserID persist.DBID `json:"user_id" binding:"required"`
}

type PostPreflightMessage struct {
	Token  persist.TokenIdentifiers `json:"token" binding:"required"`
	UserID persist.DBID             `json:"user_id"`
}

type AutosocialProcessUsersMessage struct {
	Users map[persist.DBID]map[persist.SocialProvider][]persist.ChainAddress `json:"users" binding:"required"`
}

type AutosocialPollFarcasterMessage struct {
	SignerUUID string       `form:"signer_uuid" binding:"required"`
	UserID     persist.DBID `form:"user_id" binding:"required"`
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

func CreateTaskForFeed(ctx context.Context, message FeedMessage, client *gcptasks.Client) error {
	span, ctx := tracing.StartSpan(ctx, "cloudtask.create", "createTaskForFeed")
	defer tracing.FinishSpan(span)
	tracing.AddEventDataToSpan(span, map[string]any{"Event ID": message.ID})
	queue := env.GetString("GCLOUD_FEED_QUEUE")
	url := fmt.Sprintf("%s/tasks/feed-event", env.GetString("FEED_URL"))
	secret := env.GetString("FEED_SECRET")
	delay := time.Duration(env.GetInt("GCLOUD_FEED_BUFFER_SECS")) * time.Second
	return submitTask(ctx, client, queue, url, withDelay(delay), withJSON(message), withTrace(span), withBasicAuth(secret))
}

func CreateTaskForFeedbot(ctx context.Context, message FeedbotMessage, client *gcptasks.Client) error {
	span, ctx := tracing.StartSpan(ctx, "cloudtask.create", "createTaskForFeedbot")
	defer tracing.FinishSpan(span)
	tracing.AddEventDataToSpan(span, map[string]any{"Event ID": message.FeedEventID})
	queue := env.GetString("GCLOUD_FEEDBOT_TASK_QUEUE")
	url := fmt.Sprintf("%s/tasks/feed-event", env.GetString("FEEDBOT_URL"))
	secret := env.GetString("FEEDBOT_SECRET")
	return submitTask(ctx, client, queue, url, withJSON(message), withTrace(span), withBasicAuth(secret))
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

func CreateTaskForTokenTokenProcessing(ctx context.Context, message TokenProcessingTokenMessage, client *gcptasks.Client, delay time.Duration) error {
	span, ctx := tracing.StartSpan(ctx, "cloudtask.create", "createTaskForTokenTokenProcessing")
	defer tracing.FinishSpan(span)
	queue := env.GetString("TOKEN_PROCESSING_QUEUE")
	url := fmt.Sprintf("%s/media/tokenmanage/process/token", env.GetString("TOKEN_PROCESSING_URL"))
	return submitTask(ctx, client, queue, url, withJSON(message), withTrace(span), withDelay(delay))
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

func CreateTaskForPostPreflight(ctx context.Context, message PostPreflightMessage, client *gcptasks.Client) error {
	span, ctx := tracing.StartSpan(ctx, "cloudtask.create", "createTaskForPostPreflight")
	defer tracing.FinishSpan(span)
	queue := env.GetString("TOKEN_PROCESSING_QUEUE")
	url := fmt.Sprintf("%s/media/process/post-preflight", env.GetString("TOKEN_PROCESSING_URL"))
	return submitTask(ctx, client, queue, url, withJSON(message), withTrace(span))
}

func CreateTaskForAutosocialProcessUsers(ctx context.Context, message AutosocialProcessUsersMessage, client *gcptasks.Client) error {
	span, ctx := tracing.StartSpan(ctx, "cloudtask.create", "createTaskForAutosocialProcessUsers")
	defer tracing.FinishSpan(span)
	queue := env.GetString("AUTOSOCIAL_QUEUE")
	url := fmt.Sprintf("%s/process/users", env.GetString("AUTOSOCIAL_URL"))
	return submitTask(ctx, client, queue, url, withJSON(message), withTrace(span))
}

func CreateTaskForAutosocialPollFarcaster(ctx context.Context, message AutosocialPollFarcasterMessage, client *gcptasks.Client) error {
	span, ctx := tracing.StartSpan(ctx, "cloudtask.create", "createTaskForAutosocialPollFarcaster")
	defer tracing.FinishSpan(span)
	queue := env.GetString("AUTOSOCIAL_POLL_QUEUE")
	url := fmt.Sprintf("%s/checkFarcasterApproval/signer_uuid=%s&user_id=%s", env.GetString("AUTOSOCIAL_URL"), message.SignerUUID, message.UserID)
	return submitTask(ctx, client, queue, url, withTrace(span))
}

func CreateTaskForSlackPostFeedBot(ctx context.Context, message FeedbotSlackPostMessage, client *gcptasks.Client) error {
	span, ctx := tracing.StartSpan(ctx, "cloudtask.create", "createTaskForSlackPostFeedBot")
	defer tracing.FinishSpan(span)
	queue := env.GetString("GCLOUD_FEEDBOT_TASK_QUEUE")
	url := fmt.Sprintf("%s/tasks/slack-post", env.GetString("FEEDBOT_URL"))
	secret := env.GetString("FEEDBOT_SECRET")
	return submitTask(ctx, client, queue, url, withJSON(message), withTrace(span), withBasicAuth(secret), withDelay(2*time.Minute))
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

func withDelay(delay time.Duration) func(*taskspb.Task) error {
	return func(t *taskspb.Task) error {
		scheduleOn := time.Now().Add(delay)
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
		addHeader(t.GetHttpRequest(), "Authorization", basicauth.MakeHeader(nil, secret))
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
