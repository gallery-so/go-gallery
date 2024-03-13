package event

import (
	"context"
	"errors"
	"fmt"

	"github.com/getsentry/sentry-go"
	"github.com/gin-gonic/gin"
	"github.com/go-playground/validator/v10"
	"golang.org/x/sync/errgroup"

	db "github.com/mikeydub/go-gallery/db/gen/coredb"
	"github.com/mikeydub/go-gallery/feed"
	"github.com/mikeydub/go-gallery/graphql/dataloader"
	"github.com/mikeydub/go-gallery/service/farcaster"
	"github.com/mikeydub/go-gallery/service/logger"
	"github.com/mikeydub/go-gallery/service/notifications"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/service/persist/postgres"
	sentryutil "github.com/mikeydub/go-gallery/service/sentry"
	"github.com/mikeydub/go-gallery/service/task"
	"github.com/mikeydub/go-gallery/service/tracing"
	"github.com/mikeydub/go-gallery/util"
	"github.com/mikeydub/go-gallery/validate"
)

type sendType int

const (
	eventSenderContextKey           = "event.eventSender"
	sentryEventContextName          = "event context"
	delayedKey             sendType = iota
	immediateKey
	groupKey
)

// Register specific event handlers
func AddTo(ctx *gin.Context, disableDataloaderCaching bool, notif *notifications.NotificationHandlers, queries *db.Queries, taskClient *task.Client, neynarAPI *farcaster.NeynarAPI) {
	dataloaders := dataloader.NewLoaders(context.Background(), queries, disableDataloaderCaching, tracing.DataloaderPreFetchHook, tracing.DataloaderPostFetchHook)
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
	sender.addDelayedHandler(feed, persist.ActionUserPosted, slackHandler{taskClient})
	sender.addImmediateHandler(feed, persist.ActionCollectionCreated, feedHandler)
	sender.addImmediateHandler(feed, persist.ActionTokensAddedToCollection, feedHandler)
	sender.addImmediateHandler(feed, persist.ActionCollectorsNoteAddedToCollection, feedHandler)
	sender.addImmediateHandler(feed, persist.ActionGalleryInfoUpdated, feedHandler)
	sender.addImmediateHandler(feed, persist.ActionCollectorsNoteAddedToToken, feedHandler)
	sender.addGroupHandler(feed, persist.ActionGalleryUpdated, feedHandler)

	notifications := newEventDispatcher()
	notificationHandler := notificationHandler{dataloaders: dataloaders, notificationHandlers: notif}
	sender.addDelayedHandler(notifications, persist.ActionUserFollowedUsers, notificationHandler)
	sender.addDelayedHandler(notifications, persist.ActionAdmiredFeedEvent, notificationHandler)
	sender.addDelayedHandler(notifications, persist.ActionAdmiredToken, notificationHandler)
	sender.addDelayedHandler(notifications, persist.ActionAdmiredPost, notificationHandler)
	sender.addDelayedHandler(notifications, persist.ActionAdmiredComment, notificationHandler)
	sender.addDelayedHandler(notifications, persist.ActionViewedGallery, notificationHandler)
	sender.addDelayedHandler(notifications, persist.ActionCommentedOnFeedEvent, notificationHandler)
	sender.addDelayedHandler(notifications, persist.ActionCommentedOnPost, notificationHandler)
	sender.addDelayedHandler(notifications, persist.ActionReplyToComment, notificationHandler)
	sender.addDelayedHandler(notifications, persist.ActionMentionUser, notificationHandler)
	sender.addDelayedHandler(notifications, persist.ActionMentionCommunity, notificationHandler)
	sender.addDelayedHandler(notifications, persist.ActionNewTokensReceived, notificationHandler)
	sender.addDelayedHandler(notifications, persist.ActionUserPostedYourWork, notificationHandler)
	sender.addDelayedHandler(notifications, persist.ActionUserPosted, &userPostedNotificationHandler{notif, queries, dataloaders, neynarAPI})
	sender.addDelayedHandler(notifications, persist.ActionTopActivityBadgeReceived, notificationHandler)
	sender.addDelayedHandler(notifications, persist.ActionAnnouncement, &announcementNotificationHandler{notif})
	sender.addDelayedHandler(notifications, persist.ActionUserCreated, userCreatedNotificationHandler{notif, queries, dataloaders, neynarAPI})

	sender.feed = feed
	sender.notifications = notifications
	ctx.Set(eventSenderContextKey, &sender)
}

func Dispatch(ctx context.Context, evt db.Event) error {
	ctx = sentryutil.NewSentryHubGinContext(ctx)
	go PushEvent(ctx, evt)
	return nil
}

func DispatchCaptioned(ctx context.Context, evt db.Event, caption *string) (*db.FeedEvent, error) {
	ctx = sentryutil.NewSentryHubGinContext(ctx)

	if caption != nil {
		evt.Caption = persist.StrPtrToNullStr(caption)
		return dispatchImmediate(ctx, []db.Event{evt})
	}

	go PushEvent(ctx, evt)
	return nil, nil
}

func DispatchMany(ctx context.Context, evts []db.Event, editID *string) error {
	if len(evts) == 0 {
		return nil
	}

	for i := range evts {
		evts[i].GroupID = persist.StrPtrToNullStr(editID)
	}

	ctx = sentryutil.NewSentryHubGinContext(ctx)

	for _, evt := range evts {
		go PushEvent(ctx, evt)
	}

	return nil
}

func PushEvent(ctx context.Context, evt db.Event) {
	err := dispatchDelayed(ctx, evt)
	if err != nil {
		sentryutil.ReportError(ctx, err, func(scope *sentry.Scope) {
			logger.For(ctx).Error(err)
			setEventContext(scope, persist.NullStrToDBID(evt.ActorID), evt.SubjectID, evt.Action)
		})
	}
}

func setEventContext(scope *sentry.Scope, actorID, subjectID persist.DBID, action persist.Action) {
	scope.SetContext(sentryEventContextName, sentry.Context{
		"ActorID":   actorID,
		"SubjectID": subjectID,
		"Action":    action,
	})
}

// dispatchDelayed sends the event to all of its registered handlers.
func dispatchDelayed(ctx context.Context, event db.Event) error {
	gc := util.MustGetGinContext(ctx)
	sender := For(gc)

	// Vaidate event
	err := sender.validate.Struct(event)
	if err != nil {
		return err
	}

	if _, handable := sender.registry[delayedKey][event.Action]; !handable {
		logger.For(ctx).WithField("action", event.Action).Warn("no delayed handler configured for action")
		return nil
	}

	e, err := sender.eventRepo.Add(ctx, event)
	if err != nil {
		return err
	}

	eg, ctx := errgroup.WithContext(ctx)
	eg.Go(func() error { return sender.feed.dispatchDelayed(ctx, *e) })
	eg.Go(func() error { return sender.notifications.dispatchDelayed(ctx, *e) })
	return eg.Wait()
}

// dispatchImmediate flushes the event immediately to its registered handlers.
func dispatchImmediate(ctx context.Context, events []db.Event) (*db.FeedEvent, error) {
	gc := util.MustGetGinContext(ctx)
	sender := For(gc)

	for _, e := range events {
		// Vaidate event
		if err := sender.validate.Struct(e); err != nil {
			return nil, err
		}
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
	validate      *validator.Validate
}

func newEventSender(queries *db.Queries) eventSender {
	v := validator.New()
	v.RegisterStructValidation(validate.EventValidator, db.Event{})
	return eventSender{
		registry:  map[sendType]registedActions{delayedKey: {}, immediateKey: {}, groupKey: {}},
		queries:   queries,
		eventRepo: postgres.EventRepository{Queries: queries},
		validate:  v,
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
	tc           *task.Client
}

func newFeedHandler(queries *db.Queries, taskClient *task.Client) feedHandler {
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
func (h feedHandler) handleDelayed(ctx context.Context, e db.Event) error {
	if !actionsToBeHandledByFeedService[e.Action] {
		return nil
	}
	return h.tc.CreateTaskForFeed(ctx, task.FeedMessage{ID: e.ID})
}

// handledImmediate sidesteps the Feed service so that an event is immediately available as a feed event.
func (h feedHandler) handleImmediate(ctx context.Context, e db.Event) (*db.FeedEvent, error) {
	return h.eventBuilder.NewFeedEventFromEvent(ctx, e)
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
		err = h.tc.CreateTaskForFeedbot(ctx, task.FeedbotMessage{FeedEventID: feedEvent.ID, Action: feedEvent.Action})
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

func (h notificationHandler) handleDelayed(ctx context.Context, e db.Event) error {
	owner, err := h.findOwnerForNotificationFromEvent(ctx, e)
	if err != nil {
		return err
	}

	// if no gallery user found to notify, don't notify (e.g. for mentions to community creators that are not gallery users)
	if owner == "" {
		return nil
	}

	// Don't notify the user on self events
	if persist.DBID(persist.NullStrToStr(e.ActorID)) == owner && (e.Action != persist.ActionNewTokensReceived && e.Action != persist.ActionTopActivityBadgeReceived) {
		return nil
	}

	// Don't notify the user on un-authed views
	if e.Action == persist.ActionViewedGallery && e.ActorID.String == "" {
		return nil
	}

	return h.notificationHandlers.Notifications.Dispatch(ctx, db.Notification{
		OwnerID:     owner,
		Action:      e.Action,
		Data:        h.createNotificationDataForEvent(e),
		EventIds:    persist.DBIDList{e.ID},
		GalleryID:   e.GalleryID,
		FeedEventID: e.FeedEventID,
		PostID:      e.PostID,
		CommentID:   e.CommentID,
		TokenID:     e.TokenID,
		CommunityID: e.CommunityID,
		MentionID:   e.MentionID,
	})
}

func (h notificationHandler) findOwnerForNotificationFromEvent(ctx context.Context, event db.Event) (persist.DBID, error) {
	switch event.ResourceTypeID {
	case persist.ResourceTypeGallery:
		gallery, err := h.dataloaders.GetGalleryByIdBatch.Load(event.GalleryID)
		if err != nil {
			return "", err
		}
		return gallery.OwnerUserID, nil
	case persist.ResourceTypeComment:
		if event.Action == persist.ActionReplyToComment {
			comment, err := h.dataloaders.GetCommentByCommentIDBatch.Load(event.SubjectID)
			if err != nil {
				return "", err
			}
			return comment.ActorID, nil
		}
		if event.FeedEventID != "" {
			feedEvent, err := h.dataloaders.GetEventByIdBatch.Load(event.FeedEventID)
			if err != nil {
				return "", err
			}
			return feedEvent.OwnerID, nil
		} else if event.PostID != "" {
			post, err := h.dataloaders.GetPostByIdBatch.Load(event.PostID)
			if err != nil {
				return "", err
			}
			return post.ActorID, nil
		}
	case persist.ResourceTypeAdmire:
		if event.Action == persist.ActionAdmiredToken {
			token, err := h.dataloaders.GetTokenByIdBatch.Load(event.SubjectID)
			if err != nil {
				return "", err
			}
			return token.Token.OwnerUserID, nil
		} else if event.FeedEventID != "" {
			feedEvent, err := h.dataloaders.GetEventByIdBatch.Load(event.FeedEventID)
			if err != nil {
				return "", err
			}
			return feedEvent.OwnerID, nil
		} else if event.PostID != "" {
			post, err := h.dataloaders.GetPostByIdBatch.Load(event.PostID)
			if err != nil {
				return "", err
			}
			return post.ActorID, nil
		} else if event.CommentID != "" {
			comment, err := h.dataloaders.GetCommentByCommentIDBatch.Load(event.CommentID)
			if err != nil {
				return "", err
			}
			return comment.ActorID, nil
		}
	case persist.ResourceTypeUser:
		return event.SubjectID, nil
	case persist.ResourceTypeToken:
		return persist.DBID(event.ActorID.String), nil
	case persist.ResourceTypeCommunity:
		// TODO: a community can technically have multiple creators. For now, just return the first one.
		creators, err := h.dataloaders.GetCreatorsByCommunityID.Load(event.CommunityID)
		if err != nil {
			err = fmt.Errorf("error getting creators for community ID %s: %w", event.CommunityID, err)
			logger.For(ctx).WithError(err).Warn(err)
			sentryutil.ReportError(ctx, err)
			return "", nil
		}
		if len(creators) > 0 && creators[0].CreatorUserID != "" {
			return creators[0].CreatorUserID, nil
		}
		return "", nil
	}

	return "", fmt.Errorf("no owner found for event: %s", event.Action)
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
	case persist.ActionAdmiredFeedEvent, persist.ActionAdmiredPost, persist.ActionAdmiredToken, persist.ActionAdmiredComment:
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
	case persist.ActionTopActivityBadgeReceived:
		data.ActivityBadgeThreshold = event.Data.ActivityBadgeThreshold
		data.NewTopActiveUser = event.Data.NewTopActiveUser
	default:
		logger.For(nil).Debugf("no notification data for event: %s", event.Action)
	}
	return
}

// global handles events for consumption as global notifications.
type announcementNotificationHandler struct {
	notificationHandlers *notifications.NotificationHandlers
}

func (h announcementNotificationHandler) handleDelayed(ctx context.Context, e db.Event) error {
	return h.notificationHandlers.Notifications.Dispatch(ctx, db.Notification{
		// no owner or data for follower notifications
		Action:   e.Action,
		EventIds: persist.DBIDList{e.ID},
		Data:     persist.NotificationData{AnnouncementDetails: e.Data.AnnouncementDetails},
	})
}

// slackHandler posts events to Slack
type slackHandler struct{ tc *task.Client }

func (s slackHandler) handleDelayed(ctx context.Context, e db.Event) error {
	return s.tc.CreateTaskForSlackPostFeedBot(ctx, task.FeedbotSlackPostMessage{PostID: e.PostID})
}

type userCreatedNotificationHandler struct {
	notificationHandlers *notifications.NotificationHandlers
	q                    *db.Queries
	dataloaders          *dataloader.Loaders
	fc                   *farcaster.NeynarAPI
}

func (u userCreatedNotificationHandler) handleDelayed(ctx context.Context, e db.Event) error {
	fID, err := u.fc.FarcasterIDByGalleryID(ctx, e.UserID)
	if err != nil && errors.Is(err, farcaster.ErrUserNotOnFarcaster) {
		return nil
	}
	if err != nil {
		return err
	}

	fcUsers, err := u.fc.FollowersByUserID(ctx, fID)
	if err != nil {
		return err
	}

	followerFIDs := util.MapWithoutError(fcUsers, func(u farcaster.NeynarUser) string { return u.Fid.String() })
	fcUsersOnGallery, err := u.q.GetUsersByFarcasterIDs(ctx, followerFIDs)
	if err != nil {
		return err
	}

	followers, err := u.dataloaders.GetFollowersByUserIdBatch.Load(e.UserID)
	if err != nil {
		return err
	}

	isFollowing := make(map[persist.DBID]bool, len(followers))
	for _, f := range followers {
		isFollowing[f.ID] = true
	}

	for _, f := range fcUsersOnGallery {
		if isFollowing[f.ID] {
			continue
		}
		err = u.notificationHandlers.Notifications.Dispatch(ctx, db.Notification{
			OwnerID:  f.ID,
			Action:   persist.ActionUserFromFarcasterJoined,
			Data:     persist.NotificationData{UserFromFarcasterJoinedDetails: &persist.UserFromFarcasterJoinedDetails{UserID: e.UserID}},
			EventIds: []persist.DBID{e.ID},
		})
		if err != nil {
			logger.For(ctx).Errorf("failed to send farcaster user joined notification: %s", err)
		}
	}

	return nil
}

type userPostedNotificationHandler struct {
	notificationHandlers *notifications.NotificationHandlers
	q                    *db.Queries
	dataloaders          *dataloader.Loaders
	fc                   *farcaster.NeynarAPI
}

func (u userPostedNotificationHandler) handleDelayed(ctx context.Context, e db.Event) error {
	postedYourWorkReceipients, err := u.receipientsOfUserPostedYourWorkNotification(ctx, e)
	if err != nil {
		logger.For(ctx).Errorf("failed to get receipients of posted your work notification: %s", err)
	}

	postedFistPostReceipients, err := u.receipientsOfUserPostedFirstPostNotification(ctx, e)
	if err != nil {
		logger.For(ctx).Errorf("failed to get receipients of first post notification: %s", err)
	}

	// Merge recipients with the more specific notification merged last
	recipients := postedFistPostReceipients
	for u, n := range postedYourWorkReceipients {
		recipients[u] = n
	}

	for _, n := range recipients {
		err = u.notificationHandlers.Notifications.Dispatch(ctx, n)
		if err != nil {
			logger.For(ctx).Errorf("failed to send %s notification to %s", n.Action, n.OwnerID)
		}
	}

	return nil
}

func (u userPostedNotificationHandler) receipientsOfUserPostedYourWorkNotification(ctx context.Context, e db.Event) (recipients map[persist.DBID]db.Notification, err error) {
	recipients = make(map[persist.DBID]db.Notification)

	post, err := u.dataloaders.GetPostByIdBatch.Load(e.PostID)
	if err != nil {
		return recipients, err
	}

	// Load token definitions from the post
	tokenDefinitions, errors := u.dataloaders.GetTokenDefinitionByTokenDbidBatch.LoadAll(post.TokenIds)
	j := 0
	for i := range tokenDefinitions {
		if errors[i] == nil {
			tokenDefinitions[j] = tokenDefinitions[i]
			j++
		}
	}
	tokenDefinitions = tokenDefinitions[:j]

	// Load communities
	tokenDefinitionIDs := util.MapWithoutError(tokenDefinitions, func(t db.TokenDefinition) persist.DBID { return t.ID })
	communities, errors := u.dataloaders.GetCommunitiesByTokenDefinitionID.LoadAll(tokenDefinitionIDs)
	communitiesFlat := make([]db.Community, 0)
	for i := range communities {
		if errors[i] == nil {
			communitiesFlat = append(communitiesFlat, communities[i]...)
		}
	}

	// Load creators
	communityIDs := util.MapWithoutError(communitiesFlat, func(c db.Community) persist.DBID { return c.ID })
	creators, errors := u.dataloaders.GetCreatorsByCommunityID.LoadAll(communityIDs)
	creatorsFlat := make([]db.GetCreatorsByCommunityIDRow, 0)
	for i := range creators {
		if errors[i] == nil {
			creatorsFlat = append(creatorsFlat, creators[i]...)
		}
	}

	// Only notifiy creators once per posts, even if the post includes tokens from multiple
	// communities owned by the same creator.
	for _, c := range creatorsFlat {
		if c.CreatorUserID != "" && c.CreatorUserID != post.ActorID {
			recipients[c.CreatorUserID] = db.Notification{
				OwnerID:     c.CreatorUserID,
				Action:      persist.ActionUserPostedYourWork,
				PostID:      e.PostID,
				EventIds:    []persist.DBID{e.ID},
				CommunityID: c.CommunityID,
			}
		}
	}

	return recipients, nil
}

func (u userPostedNotificationHandler) receipientsOfUserPostedFirstPostNotification(ctx context.Context, e db.Event) (recipients map[persist.DBID]db.Notification, err error) {
	actorID := persist.DBID(e.ActorID.String)
	recipients = make(map[persist.DBID]db.Notification)

	postCount, err := u.q.CountPostsByUserID(ctx, actorID)
	if err != nil {
		return recipients, err
	}

	// Only send notification if this is the first post
	if postCount != 1 {
		return recipients, nil
	}

	followers, err := u.dataloaders.GetFollowersByUserIdBatch.Load(actorID)
	if err != nil {
		return recipients, err
	}

	fID, err := u.fc.FarcasterIDByGalleryID(ctx, actorID)
	if err != nil && !errors.Is(err, farcaster.ErrUserNotOnFarcaster) {
		return recipients, err
	}
	if errors.Is(err, farcaster.ErrUserNotOnFarcaster) {
		return recipients, nil
	}

	fcUsers, err := u.fc.FollowersByUserID(ctx, fID)
	if err != nil {
		return recipients, err
	}

	followerFIDs := util.MapWithoutError(fcUsers, func(u farcaster.NeynarUser) string { return u.Fid.String() })
	fcUsersOnGallery, err := u.q.GetUsersByFarcasterIDs(ctx, followerFIDs)
	if err != nil {
		return recipients, err
	}

	isFollowingOnGallery := make(map[persist.DBID]bool, len(followers))
	for _, f := range followers {
		isFollowingOnGallery[f.ID] = true
	}

	// Also notify farcaster followers that aren't gallery followers
	for _, u := range fcUsersOnGallery {
		if !isFollowingOnGallery[u.ID] {
			followers = append(followers, u)
		}
	}

	for _, f := range followers {
		recipients[f.ID] = db.Notification{
			OwnerID:  f.ID,
			Action:   persist.ActionUserPostedFirstPost,
			PostID:   e.PostID,
			EventIds: []persist.DBID{e.ID},
		}
	}

	return recipients, nil
}
