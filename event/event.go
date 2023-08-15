package event

import (
	"context"
	"fmt"
	"time"

	"github.com/go-playground/validator/v10"
	"github.com/mikeydub/go-gallery/env"
	sentryutil "github.com/mikeydub/go-gallery/service/sentry"
	"github.com/mikeydub/go-gallery/service/task"

	cloudtasks "cloud.google.com/go/cloudtasks/apiv2"
	"github.com/gin-gonic/gin"
	db "github.com/mikeydub/go-gallery/db/gen/coredb"
	"github.com/mikeydub/go-gallery/feed"
	"github.com/mikeydub/go-gallery/graphql/dataloader"
	"github.com/mikeydub/go-gallery/service/logger"
	"github.com/mikeydub/go-gallery/service/notifications"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/service/persist/postgres"
	"github.com/mikeydub/go-gallery/util"
	"golang.org/x/sync/errgroup"
)

type sendType int

const (
	eventSenderContextKey          = "event.eventSender"
	delayedKey            sendType = iota
	immediateKey
	groupKey
)

// Register specific event handlers
func AddTo(ctx *gin.Context, disableDataloaderCaching bool, notif *notifications.NotificationHandlers, queries *db.Queries, taskClient *cloudtasks.Client) {
	sender := newEventSender(queries)

	feed := newEventDispatcher()
	feedHandler := newFeedHandler(queries, taskClient)
	sender.addDelayedHandler(feed, persist.ActionUserCreated, feedHandler)
	sender.addDelayedHandler(feed, persist.ActionUserFollowedUsers, feedHandler)
	sender.addDelayedHandler(feed, persist.ActionCollectorsNoteAddedToToken, feedHandler)
	sender.addDelayedHandler(feed, persist.ActionCollectionCreated, feedHandler)
	sender.addDelayedHandler(feed, persist.ActionCollectorsNoteAddedToCollection, feedHandler)
	sender.addDelayedHandler(feed, persist.ActionTokensAddedToCollection, feedHandler)
	sender.addDelayedHandler(feed, persist.ActionGalleryUpdated, feedHandler)
	sender.addDelayedHandler(feed, persist.ActionGalleryInfoUpdated, feedHandler)
	sender.addImmediateHandler(feed, persist.ActionCollectionCreated, feedHandler)
	sender.addImmediateHandler(feed, persist.ActionTokensAddedToCollection, feedHandler)
	sender.addImmediateHandler(feed, persist.ActionCollectorsNoteAddedToCollection, feedHandler)
	sender.addImmediateHandler(feed, persist.ActionGalleryInfoUpdated, feedHandler)
	sender.addImmediateHandler(feed, persist.ActionCollectorsNoteAddedToToken, feedHandler)
	sender.addGroupHandler(feed, persist.ActionGalleryUpdated, feedHandler)

	notifications := newEventDispatcher()
	notificationHandler := newNotificationHandler(notif, disableDataloaderCaching, queries)
	sender.addDelayedHandler(notifications, persist.ActionUserFollowedUsers, notificationHandler)
	sender.addDelayedHandler(notifications, persist.ActionAdmiredFeedEvent, notificationHandler)
	sender.addDelayedHandler(notifications, persist.ActionAdmiredPost, notificationHandler)
	sender.addDelayedHandler(notifications, persist.ActionViewedGallery, notificationHandler)
	sender.addDelayedHandler(notifications, persist.ActionCommentedOnFeedEvent, notificationHandler)
	sender.addDelayedHandler(notifications, persist.ActionCommentedOnPost, notificationHandler)
	sender.addDelayedHandler(notifications, persist.ActionReplyToComment, notificationHandler)
	sender.addDelayedHandler(notifications, persist.ActionMentionUser, notificationHandler)
	sender.addDelayedHandler(notifications, persist.ActionMentionCommunity, notificationHandler)
	sender.addDelayedHandler(notifications, persist.ActionNewTokensReceived, notificationHandler)

	sender.feed = feed
	sender.notifications = notifications
	ctx.Set(eventSenderContextKey, &sender)
}

func DispatchEvent(ctx context.Context, evt db.Event, v *validator.Validate, caption *string) (*db.FeedEvent, error) {
	ctx = sentryutil.NewSentryHubGinContext(ctx)
	if err := v.Struct(evt); err != nil {
		return nil, err
	}

	if caption != nil {
		evt.Caption = persist.StrPtrToNullStr(caption)
		return DispatchImmediate(ctx, []db.Event{evt})
	}

	go PushEvent(ctx, evt)
	return nil, nil
}

func DispatchEvents(ctx context.Context, evts []db.Event, v *validator.Validate, editID *string, caption *string) (*db.FeedEvent, error) {

	if len(evts) == 0 {
		return nil, nil
	}

	ctx = sentryutil.NewSentryHubGinContext(ctx)
	for i, evt := range evts {
		evt.GroupID = persist.StrPtrToNullStr(editID)
		if err := v.Struct(evt); err != nil {
			return nil, err
		}
		evts[i] = evt
	}

	if caption != nil {
		for i, evt := range evts {
			evt.Caption = persist.StrPtrToNullStr(caption)
			evts[i] = evt
		}
		return DispatchImmediate(ctx, evts)
	}

	for _, evt := range evts {
		go PushEvent(ctx, evt)
	}
	return nil, nil
}

func PushEvent(ctx context.Context, evt db.Event) {
	if hub := sentryutil.SentryHubFromContext(ctx); hub != nil {
		sentryutil.SetEventContext(hub.Scope(), persist.NullStrToDBID(evt.ActorID), evt.SubjectID, evt.Action)
	}
	if err := DispatchDelayed(ctx, evt); err != nil {
		logger.For(ctx).Error(err)
		sentryutil.ReportError(ctx, err)
	}
}

// DispatchDelayed sends the event to all of its registered handlers.
func DispatchDelayed(ctx context.Context, event db.Event) error {
	gc := util.MustGetGinContext(ctx)
	sender := For(gc)

	if _, handable := sender.registry[delayedKey][event.Action]; !handable {
		logger.For(ctx).WithField("action", event.Action).Warn("no delayed handler configured for action")
		return nil
	}

	persistedEvent, err := sender.eventRepo.Add(ctx, event)
	if err != nil {
		return err
	}

	eg, ctx := errgroup.WithContext(ctx)
	eg.Go(func() error { return sender.feed.dispatchDelayed(ctx, *persistedEvent) })
	eg.Go(func() error { return sender.notifications.dispatchDelayed(ctx, *persistedEvent) })
	return eg.Wait()
}

// DispatchImmediate flushes the event immediately to its registered handlers.
func DispatchImmediate(ctx context.Context, events []db.Event) (*db.FeedEvent, error) {
	gc := util.MustGetGinContext(ctx)
	sender := For(gc)

	for _, e := range events {
		if _, handable := sender.registry[immediateKey][e.Action]; !handable {
			logger.For(ctx).WithField("action", e.Action).Warn("no immediate handler configured for action")
			return nil, nil
		}
	}

	persistedEvents := make([]db.Event, 0, len(events))
	for _, e := range events {
		persistedEvent, err := sender.eventRepo.Add(ctx, e)
		if err != nil {
			return nil, err
		}

		persistedEvents = append(persistedEvents, *persistedEvent)
	}

	go func() {

		ctx := sentryutil.NewSentryHubGinContext(ctx)
		if _, err := sender.notifications.dispatchImmediate(ctx, persistedEvents); err != nil {
			logger.For(ctx).Error(err)
			sentryutil.ReportError(ctx, err)
		}

	}()

	feedEvent, err := sender.feed.dispatchImmediate(ctx, persistedEvents)
	if err != nil {
		return nil, err
	}

	return feedEvent.(*db.FeedEvent), nil
}

// DispatchGroup flushes the event group immediately to its registered handlers.
func DispatchGroup(ctx context.Context, groupID string, action persist.Action, caption *string) (*db.FeedEvent, error) {
	gc := util.MustGetGinContext(ctx)
	sender := For(gc)

	if _, handable := sender.registry[groupKey][action]; !handable {
		logger.For(ctx).WithField("action", action).Warn("no group handler configured for action")
		return nil, nil
	}

	if caption != nil {
		err := sender.eventRepo.Queries.UpdateEventCaptionByGroup(ctx, db.UpdateEventCaptionByGroupParams{
			Caption: persist.StrPtrToNullStr(caption),
			GroupID: persist.StrPtrToNullStr(&groupID),
		})
		if err != nil {
			return nil, err
		}
	}

	go func() {

		ctx := sentryutil.NewSentryHubGinContext(ctx)
		if _, err := sender.notifications.dispatchGroup(ctx, groupID, action); err != nil {
			logger.For(ctx).Error(err)
			sentryutil.ReportError(ctx, err)
		}

	}()

	feedEvent, err := sender.feed.dispatchGroup(ctx, groupID, action)
	if err != nil {
		return nil, err
	}

	return feedEvent.(*db.FeedEvent), nil
}

func For(ctx context.Context) *eventSender {
	gc := util.MustGetGinContext(ctx)
	return gc.Value(eventSenderContextKey).(*eventSender)
}

type registedActions map[persist.Action]struct{}

type eventSender struct {
	feed          *eventDispatcher
	notifications *eventDispatcher
	registry      map[sendType]registedActions
	queries       *db.Queries
	eventRepo     postgres.EventRepository
}

func newEventSender(queries *db.Queries) eventSender {
	return eventSender{
		registry: map[sendType]registedActions{
			delayedKey:   {},
			immediateKey: {},
			groupKey:     {},
		},
		queries:   queries,
		eventRepo: postgres.EventRepository{Queries: queries},
	}
}

func (e *eventSender) addDelayedHandler(dispatcher *eventDispatcher, action persist.Action, handler delayedHandler) {
	dispatcher.addDelayed(action, handler)
	e.registry[delayedKey][action] = struct{}{}
}

func (e *eventSender) addImmediateHandler(dispatcher *eventDispatcher, action persist.Action, handler immediateHandler) {
	dispatcher.addImmediate(action, handler)
	e.registry[immediateKey][action] = struct{}{}
}

func (e *eventSender) addGroupHandler(dispatcher *eventDispatcher, action persist.Action, handler groupHandler) {
	dispatcher.addGroup(action, handler)
	e.registry[groupKey][action] = struct{}{}
}

type eventDispatcher struct {
	delayedHandlers   map[persist.Action]delayedHandler
	immediateHandlers map[persist.Action]immediateHandler
	groupHandlers     map[persist.Action]groupHandler
}

func newEventDispatcher() *eventDispatcher {
	return &eventDispatcher{
		delayedHandlers:   map[persist.Action]delayedHandler{},
		immediateHandlers: map[persist.Action]immediateHandler{},
		groupHandlers:     map[persist.Action]groupHandler{},
	}
}

func (d *eventDispatcher) addDelayed(action persist.Action, handler delayedHandler) {
	d.delayedHandlers[action] = handler
}

func (d *eventDispatcher) addImmediate(action persist.Action, handler immediateHandler) {
	d.immediateHandlers[action] = handler
}

func (d *eventDispatcher) addGroup(action persist.Action, handler groupHandler) {
	d.groupHandlers[action] = handler
}

func (d *eventDispatcher) dispatchDelayed(ctx context.Context, event db.Event) error {
	if handler, ok := d.delayedHandlers[event.Action]; ok {
		return handler.handleDelayed(ctx, event)
	}
	return nil
}

// this will run the handler for each event and return the final non-nil result returned by the handler.
// in the case of the feed, immediate events should be grouped such that only one feed event is created
// and one event is returned
func (d *eventDispatcher) dispatchImmediate(ctx context.Context, event []db.Event) (interface{}, error) {

	resultChan := make(chan interface{})
	errChan := make(chan error)
	var handleables int
	for _, e := range event {
		if handler, ok := d.immediateHandlers[e.Action]; ok {
			handleables++
			go func(event db.Event) {
				result, err := handler.handleImmediate(ctx, event)
				if err != nil {
					errChan <- err
					return
				}
				resultChan <- result
			}(e)
		}
	}

	var result interface{}
	for i := 0; i < handleables; i++ {
		select {
		case r := <-resultChan:
			if r != nil {
				result = r
			}
		case err := <-errChan:
			return nil, err
		}
	}

	return result, nil
}

func (d *eventDispatcher) dispatchGroup(ctx context.Context, groupID string, action persist.Action) (interface{}, error) {
	if handler, ok := d.groupHandlers[action]; ok {
		return handler.handleGroup(ctx, groupID, action)
	}
	return nil, nil
}

type delayedHandler interface {
	handleDelayed(context.Context, db.Event) error
}

type immediateHandler interface {
	handleImmediate(context.Context, db.Event) (*db.FeedEvent, error)
}

type groupHandler interface {
	handleGroup(context.Context, string, persist.Action) (*db.FeedEvent, error)
}

// feedHandler handles events for consumption as feed events.
type feedHandler struct {
	queries      *db.Queries
	eventBuilder *feed.EventBuilder
	tc           *cloudtasks.Client
}

func newFeedHandler(queries *db.Queries, taskClient *cloudtasks.Client) feedHandler {
	return feedHandler{
		queries:      queries,
		eventBuilder: feed.NewEventBuilder(queries),
		tc:           taskClient,
	}
}

var actionsToBeHandledByFeedService = map[persist.Action]bool{
	persist.ActionUserFollowedUsers: true,
}

// handleDelayed creates a delayed task for the Feed service to handle later.
func (h feedHandler) handleDelayed(ctx context.Context, persistedEvent db.Event) error {
	if !actionsToBeHandledByFeedService[persistedEvent.Action] {
		return nil
	}

	scheduleOn := persistedEvent.CreatedAt.Add(time.Duration(env.GetInt("GCLOUD_FEED_BUFFER_SECS")) * time.Second)
	return task.CreateTaskForFeed(ctx, scheduleOn, task.FeedMessage{ID: persistedEvent.ID}, h.tc)
}

// handledImmediate sidesteps the Feed service so that an event is immediately available as a feed event.
func (h feedHandler) handleImmediate(ctx context.Context, persistedEvent db.Event) (*db.FeedEvent, error) {
	return h.eventBuilder.NewFeedEventFromEvent(ctx, persistedEvent)
}

// handleGrouped processes a group of events into a single feed event.
func (h feedHandler) handleGroup(ctx context.Context, groupID string, action persist.Action) (*db.FeedEvent, error) {

	existsForGroup, err := h.queries.IsFeedEventExistsForGroup(ctx, persist.StrPtrToNullStr(&groupID))
	if err != nil {
		return nil, err
	}
	if existsForGroup {
		feedEvent, err := h.queries.UpdateFeedEventCaptionByGroup(ctx, persist.StrPtrToNullStr(&groupID))
		if err != nil {
			return nil, err
		}
		return &feedEvent, nil
	}

	feedEvent, err := h.eventBuilder.NewFeedEventFromGroup(ctx, groupID, action)
	if err != nil {
		return nil, err
	}

	if feedEvent != nil {
		// Send event to feedbot
		err = task.CreateTaskForFeedbot(ctx,
			time.Now(), task.FeedbotMessage{FeedEventID: feedEvent.ID, Action: feedEvent.Action}, h.tc,
		)
		if err != nil {
			logger.For(ctx).Errorf("failed to create task for feedbot: %s", err.Error())
		}
	}
	return feedEvent, nil
}

// notificationHandlers handles events for consumption as notifications.
type notificationHandler struct {
	dataloaders          *dataloader.Loaders
	notificationHandlers *notifications.NotificationHandlers
}

func newNotificationHandler(notifiers *notifications.NotificationHandlers, disableDataloaderCaching bool, queries *db.Queries) *notificationHandler {
	return &notificationHandler{
		notificationHandlers: notifiers,
		dataloaders:          dataloader.NewLoaders(context.Background(), queries, disableDataloaderCaching),
	}
}

func (h notificationHandler) handleDelayed(ctx context.Context, persistedEvent db.Event) error {
	owner, err := h.findOwnerForNotificationFromEvent(persistedEvent)
	if err != nil {
		return err
	}

	// Don't notify the user on self events
	if persist.DBID(persist.NullStrToStr(persistedEvent.ActorID)) == owner && persistedEvent.Action != persist.ActionNewTokensReceived {

		return nil
	}

	// Don't notify the user on un-authed views
	if persistedEvent.Action == persist.ActionViewedGallery && persistedEvent.ActorID.String == "" {
		return nil
	}

	return h.notificationHandlers.Notifications.Dispatch(ctx, db.Notification{
		OwnerID:     owner,
		Action:      persistedEvent.Action,
		Data:        h.createNotificationDataForEvent(persistedEvent),
		EventIds:    persist.DBIDList{persistedEvent.ID},
		GalleryID:   persistedEvent.GalleryID,
		FeedEventID: persistedEvent.FeedEventID,
		PostID:      persistedEvent.PostID,
		CommentID:   persistedEvent.CommentID,
		TokenID:     persistedEvent.TokenID,
		ContractID:  persistedEvent.ContractID,
	})
}

func (h notificationHandler) createNotificationDataForEvent(event db.Event) (data persist.NotificationData) {
	switch event.Action {
	case persist.ActionViewedGallery:
		if event.ActorID.String != "" {
			data.AuthedViewerIDs = []persist.DBID{persist.NullStrToDBID(event.ActorID)}
		}
		if event.ExternalID.String != "" {
			data.UnauthedViewerIDs = []string{persist.NullStrToStr(event.ExternalID)}
		}
	case persist.ActionAdmiredFeedEvent, persist.ActionAdmiredPost:
		if event.ActorID.String != "" {
			data.AdmirerIDs = []persist.DBID{persist.NullStrToDBID(event.ActorID)}
		}
	case persist.ActionUserFollowedUsers:
		if event.ActorID.String != "" {
			data.FollowerIDs = []persist.DBID{persist.NullStrToDBID(event.ActorID)}
		}
		data.FollowedBack = persist.NullBool(event.Data.UserFollowedBack)
		data.Refollowed = persist.NullBool(event.Data.UserRefollowed)
	case persist.ActionNewTokensReceived:
		data.NewTokenID = event.Data.NewTokenID
		data.NewTokenQuantity = event.Data.NewTokenQuantity
	case persist.ActionReplyToComment:
		data.OriginalCommentID = event.SubjectID
	default:
		logger.For(nil).Debugf("no notification data for event: %s", event.Action)
	}
	return
}

func (h notificationHandler) findOwnerForNotificationFromEvent(event db.Event) (persist.DBID, error) {
	switch event.ResourceTypeID {
	case persist.ResourceTypeGallery:
		gallery, err := h.dataloaders.GalleryByGalleryID.Load(event.GalleryID)
		if err != nil {
			return "", err
		}
		return gallery.OwnerUserID, nil
	case persist.ResourceTypeComment:
		if event.Action == persist.ActionReplyToComment {
			comment, err := h.dataloaders.CommentByCommentID.Load(event.SubjectID)
			if err != nil {
				return "", err
			}
			return comment.ActorID, nil
		}
		if event.FeedEventID != "" {
			feedEvent, err := h.dataloaders.FeedEventByFeedEventID.Load(event.FeedEventID)
			if err != nil {
				return "", err
			}
			return feedEvent.OwnerID, nil
		} else if event.PostID != "" {
			post, err := h.dataloaders.PostByPostID.Load(event.PostID)
			if err != nil {
				return "", err
			}
			return post.ActorID, nil
		}

	case persist.ResourceTypeAdmire:
		if event.FeedEventID != "" {
			feedEvent, err := h.dataloaders.FeedEventByFeedEventID.Load(event.FeedEventID)
			if err != nil {
				return "", err
			}
			return feedEvent.OwnerID, nil
		} else if event.PostID != "" {
			post, err := h.dataloaders.PostByPostID.Load(event.PostID)
			if err != nil {
				return "", err
			}
			return post.ActorID, nil
		}
	case persist.ResourceTypeUser:
		return event.SubjectID, nil
	case persist.ResourceTypeToken:
		return persist.DBID(event.ActorID.String), nil
	case persist.ResourceTypeContract:
		c, err := h.dataloaders.ContractByContractID.Load(event.ContractID)
		if err != nil {
			return "", err
		}
		u, err := h.dataloaders.UserByAddress.Load(db.GetUserByAddressBatchParams{
			Address: c.Address,
			Chain:   int32(c.Chain),
		})
		if err != nil {
			return "", err
		}
		return u.ID, nil
	}

	return "", fmt.Errorf("no owner found for event: %s", event.Action)
}
