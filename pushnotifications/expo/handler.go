package expo

import (
	"context"
	"fmt"
	"github.com/jackc/pgx/v4"
	db "github.com/mikeydub/go-gallery/db/gen/coredb"
	"github.com/mikeydub/go-gallery/service/logger"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/util"
	"github.com/sirupsen/logrus"
	"sync"
	"time"
)

func NewPushNotificationHandler(ctx context.Context, queries *db.Queries, apiURL string, accessToken string) *PushNotificationHandler {
	return &PushNotificationHandler{
		client:   NewClient(apiURL, accessToken),
		ctx:      ctx,
		queries:  queries,
		maxBatch: 100, // per Expo docs, max 100 messages per API call
		wait:     time.Second,
	}
}

// Batching pattern adapted from dataloaden (https://github.com/vektah/dataloaden)
type PushNotificationHandler struct {
	mu       sync.Mutex
	client   *Client
	ctx      context.Context
	queries  *db.Queries
	maxBatch int
	wait     time.Duration
	batch    *messageBatch
}

func (h *PushNotificationHandler) SendPushNotification(pushTokenID persist.DBID, title string, subtitle string, body string, data map[string]any, sound bool, badge int) error {

	// Expo only supports playing the "default" notification sound or none at all, so we convert the sound
	// bool to the appropriate string here
	soundStr := ""
	if sound {
		soundStr = "default"
	}

	message := PushMessage{
		pushTokenID: pushTokenID,

		To:       "", // intentionally empty; will be filled in when we look up the pushTokenID
		Data:     data,
		Title:    title,
		Subtitle: subtitle,
		Body:     body,
		Sound:    soundStr,
		Badge:    badge,
	}

	return h.sendThunk(message)()
}

type messageBatch struct {
	messages []PushMessage
	errors   []error
	closing  bool
	done     chan struct{}
}

func (h *PushNotificationHandler) sendThunk(message PushMessage) func() error {
	h.mu.Lock()

	if h.batch == nil {
		h.batch = &messageBatch{done: make(chan struct{})}
	}

	batch := h.batch
	pos := batch.appendMessage(h, message)
	h.mu.Unlock()

	return func() error {
		<-batch.done

		var err error
		// If we just get a single error back, return it to all callers
		if len(batch.errors) == 1 {
			err = batch.errors[0]
		} else if batch.errors != nil {
			err = batch.errors[pos]
		}

		return err
	}
}

func (b *messageBatch) appendMessage(l *PushNotificationHandler, message PushMessage) int {
	pos := len(b.messages)
	b.messages = append(b.messages, message)
	if pos == 0 {
		go b.startTimer(l)
	}

	if l.maxBatch != 0 && pos >= l.maxBatch-1 {
		if !b.closing {
			b.closing = true
			l.batch = nil
			go b.end(l)
		}
	}

	return pos
}

func (b *messageBatch) startTimer(l *PushNotificationHandler) {
	time.Sleep(l.wait)
	l.mu.Lock()

	// we must have hit a batch limit and are already finalizing this batch
	if b.closing {
		l.mu.Unlock()
		return
	}

	l.batch = nil
	l.mu.Unlock()

	b.end(l)
}

func (b *messageBatch) end(h *PushNotificationHandler) {
	h.sendMessageBatch(b.messages)
	close(b.done)
}

func (h *PushNotificationHandler) sendMessageBatch(messages []PushMessage) []error {
	ctx := logger.NewContextWithFields(h.ctx, logrus.Fields{
		"messageBatchID": persist.GenerateID(),
		"batchSize":      len(messages),
	})

	dbidToMessageIndex := make(map[persist.DBID]int)
	for i, message := range messages {
		dbidToMessageIndex[message.pushTokenID] = i
	}

	dbids, _ := util.Map(messages, func(m PushMessage) (string, error) { return m.pushTokenID.String(), nil })
	tokens, err := h.queries.GetPushTokensByIDs(ctx, dbids)

	if (err == nil && len(tokens) == 0) || err == pgx.ErrNoRows {
		logger.For(ctx).Infof("Skipping message batch, no push tokens found for DBIDs: %v", dbids)
		return []error{ErrPushTokenNotFound}
	}

	if err != nil {
		return []error{err}
	}

	errs := make([]error, len(messages))
	sendableMessages := messages[:0]
	tokenIndex := 0

	// GetPushTokensByIDs will omit tokens that aren't found, and return the remaining tokens in the
	// same order they were requested in. We need to filter out messages with invalid (not found) tokens,
	// since trying to send them will result in errors.
	for _, message := range messages {
		if message.pushTokenID == tokens[tokenIndex].ID {
			message.To = tokens[tokenIndex].PushToken
			sendableMessages = append(sendableMessages, message)
			tokenIndex++
		} else {
			errs[dbidToMessageIndex[message.pushTokenID]] = ErrPushTokenNotFound
			logger.For(ctx).Infof("Skipping message, no push token found for DBID: %s", message.pushTokenID)
		}
	}

	logger.For(ctx).Infof("Sending %d push notifications", len(sendableMessages))
	responses, err := h.client.SendMessages(ctx, sendableMessages)

	// If sending failed, return a single error for the whole batch
	if err != nil {
		logger.For(ctx).WithError(err).Warnf("Failed to send push notifications")
		return []error{err}
	}

	ticketDBIDs := make([]string, 0, len(responses))
	ticketExpoIDs := make([]string, 0, len(responses))
	ticketTokenDBIDs := make([]string, 0, len(responses))
	tokensToUnregister := make([]persist.DBID, 0)

	// Otherwise, return individual errors based on each response
	for i, response := range responses {
		message := sendableMessages[i]

		// Get this message's position in the original slice of messages and use that for error indexing
		errIndex := dbidToMessageIndex[message.pushTokenID]
		e := response.GetError()
		errs[errIndex] = e

		if e == ErrDeviceNotRegistered {
			logger.For(ctx).WithError(e).Infof("Unregistering push token with DBID: %s", message.pushTokenID)
			tokensToUnregister = append(tokensToUnregister, message.pushTokenID)
		} else if e == nil {
			ticketDBIDs = append(ticketDBIDs, persist.GenerateID().String())
			ticketExpoIDs = append(ticketExpoIDs, response.TicketID)
			ticketTokenDBIDs = append(ticketTokenDBIDs, message.pushTokenID.String())
		}
	}

	// These database operations (deleting old tokens, creating new tickets) aren't fatal
	// if they fail, and they don't signify failure of the entire send operation. Failure to
	// delete an old token just means we'll have to try deleting it again in the future if
	// it's used again, and failure to create a push ticket means that a message made it to
	// Expo for delivery, but we won't find out whether it was ultimately delivered or not.
	if err = h.queries.DeletePushTokensByIDs(ctx, tokensToUnregister); err != nil {
		logger.For(ctx).Errorf("Error deleting push tokens: %s", err)
	}

	if err = h.queries.CreatePushTickets(ctx, db.CreatePushTicketsParams{
		Ids:          ticketDBIDs,
		PushTokenIds: ticketTokenDBIDs,
		TicketIds:    ticketExpoIDs,
	}); err != nil {
		logger.For(ctx).Errorf("Error creating push tickets: %s", err)
	}

	return errs
}

func (h *PushNotificationHandler) CheckPushTickets() error {
	return h.checkPushTicketsRecursive(h.ctx, 0)
}

func (h *PushNotificationHandler) checkPushTicketsRecursive(ctx context.Context, recursionDepth int) error {
	if recursionDepth >= maxCheckPushTicketsRecursions {
		err := fmt.Errorf("checkPushTicketsRecursive: exceeded recursion depth (%d iterations)", maxCheckPushTicketsRecursions)
		reportError(ctx, err)
	}

	tickets, err := h.queries.GetCheckablePushTickets(ctx, maxReceiptsPerRequest)
	if err != nil {
		return err
	}

	if len(tickets) == 0 {
		return nil
	}

	ticketIDs, _ := util.Map(tickets, func(t db.PushNotificationTicket) (string, error) { return t.TicketID, nil })
	receipts, err := h.client.GetReceipts(ctx, ticketIDs)
	if err != nil {
		return err
	}

	params, tokensToUnregister := processTicketReceipts(ctx, tickets, receipts)

	if err = h.queries.UpdatePushTickets(ctx, params); err != nil {
		logger.For(ctx).Errorf("Error updating push tickets: %s", err)
	}

	if len(tokensToUnregister) > 0 {
		if err = h.queries.DeletePushTokensByIDs(ctx, tokensToUnregister); err != nil {
			logger.For(ctx).Errorf("Error deleting push tickets: %s", err)
		}
	}

	// If we got the maximum number of receipts back, there might be more to check
	if len(receipts) == maxReceiptsPerRequest {
		return h.checkPushTicketsRecursive(ctx, recursionDepth+1)
	}

	return nil
}

func processTicketReceipts(ctx context.Context, tickets []db.PushNotificationTicket, receipts map[string]PushReceipt) (params db.UpdatePushTicketsParams, tokensToUnregister []persist.DBID) {
	params.Ids = make([]string, len(receipts))
	params.CheckAfter = make([]time.Time, len(receipts))
	params.NumCheckAttempts = make([]int32, len(receipts))
	params.Deleted = make([]bool, len(receipts))

	for i, ticket := range tickets {
		// Start with all the ticket's existing values
		params.Ids[i] = ticket.ID.String()
		params.CheckAfter[i] = ticket.CheckAfter
		params.NumCheckAttempts[i] = ticket.NumCheckAttempts
		params.Deleted[i] = ticket.Deleted

		ctx := logger.NewContextWithFields(ctx, logrus.Fields{
			"pushTicketID":     ticket.ID,
			"pushTokenID":      ticket.PushTokenID,
			"numCheckAttempts": ticket.NumCheckAttempts,
			"checkAfter":       ticket.CheckAfter,
		})

		receipt, ok := receipts[ticket.TicketID]
		if !ok {
			// If the receipt wasn't found, delete the associated ticket. Expo deletes receipts after 24 hours.
			params.Deleted[i] = true
			logger.For(ctx).Info("Deleting push ticket (no receipt found)")
			continue
		}

		err := receipt.GetError()
		if err == nil {
			// If we got a receipt and there wasn't an error, the message was delivered, and we don't need to check again.
			params.Deleted[i] = true
			continue
		}

		if err == ErrDeviceNotRegistered {
			// If the device is no longer registered, we need to delete the associated push token
			logger.For(ctx).WithError(err).Info("Deleting push ticket (device not registered)")
			tokensToUnregister = append(tokensToUnregister, ticket.PushTokenID)
			params.Deleted[i] = true
			continue
		}

		if err == ErrMessageTooBig {
			// Delete the push ticket -- no amount of retrying will fix ErrMessageTooBig
			logger.For(ctx).WithError(err).Error("Deleting push ticket (message too big)")
			reportTicketError(ctx, err, ticket)
			params.Deleted[i] = true
			continue
		}

		// In all other cases, we should retry with exponential backoff
		params.NumCheckAttempts[i]++
		params.CheckAfter[i] = getNextCheckAfter(params.NumCheckAttempts[i])

		logger.For(ctx).WithError(err).Infof("Rechecking ticket receipt after %v due to error", params.CheckAfter[i])
	}

	return params, tokensToUnregister
}

func getNextCheckAfter(numAttempts int32) time.Time {
	duration := time.Duration(numAttempts*numAttempts) * time.Minute
	if duration > maxCheckReceiptsBackoff {
		duration = maxCheckReceiptsBackoff
	}
	return time.Now().Add(duration)
}
