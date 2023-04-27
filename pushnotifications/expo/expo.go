package expo

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/getsentry/sentry-go"
	db "github.com/mikeydub/go-gallery/db/gen/coredb"
	"github.com/mikeydub/go-gallery/service/persist"
	sentryutil "github.com/mikeydub/go-gallery/service/sentry"
	"github.com/mikeydub/go-gallery/util"
	"net/http"
	"time"
)

const (
	StatusOK    = "ok"
	StatusError = "error"

	PriorityDefault = "default"
	PriorityNormal  = "normal"
	PriorityHigh    = "high"

	maxReceiptsPerRequest         = 1000
	maxCheckReceiptsBackoff       = 2 * time.Hour
	maxCheckPushTicketsRecursions = 100
)

var (
	ErrDeviceNotRegistered = newPushError("DeviceNotRegistered")
	ErrMessageTooBig       = newPushError("MessageTooBig")
	ErrMessageRateExceeded = newPushError("MessageRateExceeded")
	ErrMismatchSenderId    = newPushError("MismatchSenderId")
	ErrInvalidCredentials  = newPushError("InvalidCredentials")

	ErrTooManyRequests          = newPushError("TOO_MANY_REQUESTS")
	ErrPushTooManyExperienceIDs = newPushError("PUSH_TOO_MANY_EXPERIENCE_IDS")
	ErrPushTooManyNotifications = newPushError("PUSH_TOO_MANY_NOTIFICATIONS")
	ErrPushTooManyReceipts      = newPushError("PUSH_TOO_MANY_RECEIPTS")

	ErrPushTokenNotFound = newPushError("Push token not found for DBID")
)

type Client struct {
	apiURL      string
	httpClient  *http.Client
	accessToken string
}

type GetReceiptsRequest struct {
	TicketIDs []string `json:"ids"`
}

type GetReceiptsResponse struct {
	Data   map[string]PushReceipt `json:"data"`
	Errors []map[string]string    `json:"errors"`
}

type SendMessagesResponse struct {
	Data   []PushTicket        `json:"data"`
	Errors []map[string]string `json:"errors"`
}

type PushReceipt struct {
	Status  string            `json:"status"`
	Message string            `json:"message"`
	Details map[string]string `json:"details"`
}

type PushTicket struct {
	TicketID string            `json:"id"`
	Status   string            `json:"status"`
	Message  string            `json:"message"`
	Details  map[string]string `json:"details"`
}

type PushError struct {
	Code string
}

func (e *PushError) Error() string {
	return e.Code
}

func newPushError(code string) *PushError {
	return &PushError{Code: code}
}

// GetError gets the error for the ticket, if any. Returns nil if no error.
// See: https://docs.expo.dev/push-notifications/sending-notifications/#push-ticket-errors
func (t PushTicket) GetError() error {
	if t.Status != StatusError {
		return nil
	}

	code := t.Details["error"]

	err := findErrorByCode(code, []*PushError{
		ErrDeviceNotRegistered,
	})

	if err != nil {
		return err
	}

	return errors.New("unknown error: " + code)
}

// GetError gets the error for the receipt, if any. Returns nil if no error.
// See: https://docs.expo.dev/push-notifications/sending-notifications#push-receipt-errors
func (t PushReceipt) GetError() error {
	if t.Status != StatusError {
		return nil
	}

	code := t.Details["error"]

	err := findErrorByCode(code, []*PushError{
		ErrDeviceNotRegistered,
		ErrMessageTooBig,
		ErrMessageRateExceeded,
		ErrMismatchSenderId,
		ErrInvalidCredentials,
	})

	if err != nil {
		return err
	}

	return errors.New("unknown error: " + code)
}

func (r *SendMessagesResponse) GetError() error {
	return getResponseError(r.Errors)
}

func (r *GetReceiptsResponse) GetError() error {
	return getResponseError(r.Errors)
}

func findErrorByCode(errorString string, errs []*PushError) error {
	for _, err := range errs {
		if err.Code == errorString {
			return err
		}
	}

	return nil
}

func getResponseError(errs []map[string]string) error {
	if errs == nil || len(errs) == 0 {
		return nil
	}

	// Find a known error type if we can
	for _, e := range errs {
		code := e["code"]

		err := findErrorByCode(code, []*PushError{
			ErrTooManyRequests,
			ErrPushTooManyExperienceIDs,
			ErrPushTooManyNotifications,
			ErrPushTooManyReceipts,
		})

		if err != nil {
			return err
		}
	}

	// Otherwise just return the first error
	return errors.New("unknown error: " + errs[0]["code"])
}

// PushMessage is an Expo-formatted push message.
// See: https://docs.expo.dev/push-notifications/sending-notifications/#message-request-format
type PushMessage struct {
	// Not part of the Expo message; used to keep track of the DBID associated with the "To" field's push token.
	pushTokenID persist.DBID

	// An Expo push token or an array of Expo push tokens specifying the recipient(s) of this message.
	To string `json:"to"`

	// A JSON object delivered to your app. It may be up to about 4KiB; the total notification payload
	//sent to Apple and Google must be at most 4KiB or else you will get a "Message Too Big" error.
	Data map[string]any `json:"data,omitempty"`

	// The title to display in the notification. Often displayed above the notification body
	Title string `json:"title,omitempty"`

	// The message to display in the notification.
	Body string `json:"body"`

	// Time to Live: the number of seconds for which the message may be kept around for redelivery if it
	// hasn't been delivered yet. Defaults to undefined in order to use the respective defaults of each
	// provider (0 for iOS/APNs and 2419200 (4 weeks) for Android/FCM).
	TTL int `json:"ttl,omitempty"`

	// Timestamp since the Unix epoch specifying when the message expires.
	// Same effect as ttl (ttl takes precedence over expiration).
	Expiration int64 `json:"expiration,omitempty"`

	// The delivery priority of the message. Specify "default" or omit this field to use the default
	// priority on each platform ("normal" on Android and "high" on iOS).
	Priority string `json:"priority,omitempty"`

	// iOS only. The subtitle to display in the notification below the title.
	Subtitle string `json:"subtitle,omitempty"`

	// iOS only. Play a sound when the recipient receives this notification. Specify "default" to play
	//the device's default notification sound, or omit this field to play no sound. Custom sounds are not supported.
	Sound string `json:"sound,omitempty"`

	// iOS only. Number to display in the badge on the app icon. Specify zero to clear the badge.
	Badge int `json:"badge,omitempty"`

	// Android only. ID of the Notification Channel through which to display this notification. If an ID is
	// specified but the corresponding channel does not exist on the device (i.e. has not yet been created
	// by your app), the notification will not be displayed to the user.
	ChannelID string `json:"channelId,omitempty"`

	// ID of the notification category that this notification is associated with. Must be on at least SDK 41 or bare workflow.
	// Find out more about notification categories here:
	// https://docs.expo.dev/versions/latest/sdk/notifications#managing-notification-categories-interactive-notifications
	CategoryID string `json:"categoryId,omitempty"`

	// iOS only. Specifies whether this notification can be intercepted by the client app. In Expo Go, this
	// defaults to true, and if you change that to false, you may experience issues. In standalone and bare apps,
	// this defaults to false.
	// See: https://developer.apple.com/documentation/usernotifications/modifying_content_in_newly_delivered_notifications
	MutableContent bool `json:"mutableContent,omitempty"`
}

// NewClient creates a new Expo push client with the specified API URL and access token.
// accessToken may be an empty string if the organization's push settings don't require an access token.
func NewClient(apiURL string, accessToken string) *Client {
	return &Client{
		apiURL:      apiURL,
		httpClient:  http.DefaultClient,
		accessToken: accessToken,
	}
}

func (c *Client) SendMessages(ctx context.Context, messages []PushMessage) ([]PushTicket, error) {
	body, err := json.Marshal(messages)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, fmt.Sprintf("%s/send", c.apiURL), bytes.NewReader(body))
	if err != nil {
		return nil, err
	}

	req.Header.Add("Content-Type", "application/json")

	// Expo allows the use of an optional access token. When enabled, push requests will
	// fail if the correct token isn't provided.
	if c.accessToken != "" {
		req.Header.Add("Authorization", "Bearer "+c.accessToken)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, util.GetErrFromResp(resp)
	}

	var output SendMessagesResponse
	err = json.NewDecoder(resp.Body).Decode(&output)
	if err != nil {
		return nil, err
	}

	if err := output.GetError(); err != nil {
		return nil, err
	}

	return output.Data, nil
}

func (c *Client) GetReceipts(ctx context.Context, pushTicketIDs []string) (map[string]PushReceipt, error) {
	input := GetReceiptsRequest{
		TicketIDs: pushTicketIDs,
	}

	body, err := json.Marshal(input)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, fmt.Sprintf("%s/getReceipts", c.apiURL), bytes.NewBuffer(body))
	if err != nil {
		return nil, err
	}

	req.Header.Add("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, util.GetErrFromResp(resp)
	}

	var output GetReceiptsResponse
	err = json.NewDecoder(resp.Body).Decode(&output)
	if err != nil {
		return nil, err
	}

	if err := output.GetError(); err != nil {
		return nil, err
	}

	return output.Data, nil
}

func reportError(ctx context.Context, err error) {
	sentryutil.ReportError(ctx, err)
}

func reportTicketError(ctx context.Context, err error, ticket db.PushNotificationTicket) {
	sentryutil.ReportError(ctx, err, func(scope *sentry.Scope) {
		setPushTicketTags(scope, ticket)
	})
}

func setPushTicketTags(scope *sentry.Scope, ticket db.PushNotificationTicket) {
	scope.SetTag("ticket.ID", ticket.ID.String())
	scope.SetTag("ticket.TicketID", ticket.TicketID)
	scope.SetTag("ticket.PushTokenID", ticket.PushTokenID.String())
	scope.SetTag("ticket.NumCheckAttempts", fmt.Sprintf("%d", ticket.NumCheckAttempts))
	scope.SetTag("ticket.CheckAfter", ticket.CheckAfter.String())
}
