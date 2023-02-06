package feed

import (
	"context"
	"errors"
	"time"

	db "github.com/mikeydub/go-gallery/db/gen/coredb"
	"github.com/mikeydub/go-gallery/service/logger"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/service/persist/postgres"
	"github.com/mikeydub/go-gallery/service/task"
	"github.com/mikeydub/go-gallery/service/tracing"
	"github.com/spf13/viper"
)

var (
	groupingConfig = map[persist.Action]persist.ActionList{
		persist.ActionGalleryUpdated: {
			persist.ActionCollectionUpdated,
			persist.ActionCollectionCreated,
			persist.ActionTokensAddedToCollection,
			persist.ActionCollectorsNoteAddedToToken,
			persist.ActionCollectorsNoteAddedToCollection,
			persist.ActionGalleryInfoUpdated,
		},
	}
	eventGroups = createEventGroups(groupingConfig)
)

var (
	eventSegments = map[persist.Action]segment{
		persist.ActionUserFollowedUsers: actorActionSegment,
	}

	// Feed events in this group can contain a collection collector's note
	collectionCollectorsNoteActions = persist.ActionList{
		persist.ActionCollectionUpdated,
		persist.ActionCollectorsNoteAddedToCollection,
		persist.ActionCollectionCreated,
	}

	// Feed events in this group can contain added tokens
	collectionTokensAddedActions = persist.ActionList{
		persist.ActionCollectionUpdated,
		persist.ActionTokensAddedToCollection,
		persist.ActionCollectionCreated,
	}
)

const (
	noSegment segment = iota
	actorActionSegment
	actorSubjectSegment
	actorSubjectActionSegment
	actorGallerySegment
)

var errUnhandledSingleEvent = errors.New("unhandable single event")
var errUnhandledGroupedEvent = errors.New("unhandable group event")

func createEventGroups(groupingConfig map[persist.Action]persist.ActionList) map[persist.Action]persist.Action {
	eventGroups := map[persist.Action]persist.Action{}
	for parent, actions := range groupingConfig {
		for _, action := range actions {
			eventGroups[action] = parent
		}
	}
	return eventGroups
}

type segment int

type EventBuilder struct {
	queries           *db.Queries
	eventRepo         *postgres.EventRepository
	feedRepo          *postgres.FeedRepository
	feedBlocklistRepo *postgres.FeedBlocklistRepository
	// windowSize is used to determine if a user is still editing and is ignored if
	// skipCooldown is enabled.
	windowSize time.Duration
}

func NewEventBuilder(queries *db.Queries) *EventBuilder {
	return &EventBuilder{
		queries:           queries,
		eventRepo:         &postgres.EventRepository{Queries: queries},
		feedRepo:          &postgres.FeedRepository{Queries: queries},
		feedBlocklistRepo: &postgres.FeedBlocklistRepository{Queries: queries},
		windowSize:        viper.GetDuration("FEED_WINDOW_SIZE") * time.Second,
	}
}

func (b *EventBuilder) NewFeedEventFromTask(ctx context.Context, message task.FeedMessage) (*db.FeedEvent, error) {
	span, ctx := tracing.StartSpan(ctx, "eventBuilder.NewEvent", "newEvent")
	defer tracing.FinishSpan(span)

	event, err := b.eventRepo.Get(ctx, message.ID)
	if err != nil {
		return nil, err
	}

	return b.NewFeedEventFromEvent(ctx, event)
}

func (b *EventBuilder) NewFeedEventFromEvent(ctx context.Context, event db.Event) (*db.FeedEvent, error) {

	if event.GroupID.String != "" {
		// if the event is being dispatched immediately, ensure that it is not supposed to be group with other events that are being dispatched immediately
		wait, err := b.queries.HasLaterGroupedEvent(ctx, db.HasLaterGroupedEventParams{
			GroupID: event.GroupID,
			EventID: event.ID,
		})
		if err != nil {
			return nil, err
		}
		if wait {
			return nil, nil
		}
	}

	if can, err := b.canEvent(ctx, event); err != nil || !can {
		return nil, err
	}
	_, groupable := eventGroups[event.Action]
	// Events with a caption are treated as a singular event.
	if event.Caption.String != "" || !groupable {
		return b.createFeedEvent(ctx, event)
	}
	return b.createGroupedFeedEventFromEvent(ctx, event)
}

func (b *EventBuilder) NewFeedEventFromGroup(ctx context.Context, groupID string, action persist.Action) (*db.FeedEvent, error) {
	events, err := b.eventRepo.Queries.GetEventsInGroup(ctx, persist.StrToNullStr(&groupID))
	if err != nil {
		return nil, err
	}

	return b.createGroupedFeedEventFromEvents(ctx, events, action)
}

func (b *EventBuilder) createGroupedFeedEventFromEvents(ctx context.Context, events []db.Event, action persist.Action) (*db.FeedEvent, error) {

	if eventGroups[action] == persist.ActionGalleryUpdated {
		return b.createGalleryUpdatedFeedEventFromEvents(ctx, events)
	}
	return nil, errUnhandledGroupedEvent
}

func (b *EventBuilder) createGroupedFeedEventFromEvent(ctx context.Context, event db.Event) (*db.FeedEvent, error) {
	if eventGroups[event.Action] == persist.ActionGalleryUpdated {
		return b.createGalleryUpdatedFeedEvent(ctx, event)
	}
	return nil, errUnhandledGroupedEvent
}

func (b *EventBuilder) createFeedEvent(ctx context.Context, event db.Event) (*db.FeedEvent, error) {
	switch event.Action {
	case persist.ActionUserFollowedUsers:
		return b.createUserFollowedUsersFeedEvent(ctx, event)
	case persist.ActionCollectorsNoteAddedToToken, persist.ActionGalleryInfoUpdated, persist.ActionTokensAddedToCollection, persist.ActionCollectorsNoteAddedToCollection, persist.ActionCollectionCreated:
		return b.createGalleryUpdatedFeedEvent(ctx, event)
	default:
		return nil, errUnhandledSingleEvent
	}
}

func (b *EventBuilder) canEvent(ctx context.Context, event db.Event) (bool, error) {
	blocked, err := b.feedBlocklistRepo.IsBlocked(ctx, persist.NullStrToDBID(event.ActorID), event.Action)
	if err != nil || blocked {
		return false, err
	}

	stillEditing, err := b.isStillEditing(ctx, event)
	if err != nil {
		return false, err
	}

	logger.For(ctx).Infof("event %s still editing: %t", event.ID, stillEditing)

	return !stillEditing, nil
}
func (b *EventBuilder) isStillEditing(ctx context.Context, event db.Event) (bool, error) {
	switch getSegment(event.Action) {
	case actorActionSegment:
		return b.eventRepo.IsActorActionActive(ctx, event, getActions(event.Action), b.windowSize)
	case actorSubjectSegment:
		return b.eventRepo.IsActorSubjectActive(ctx, event, b.windowSize)
	case actorSubjectActionSegment:
		return b.eventRepo.IsActorSubjectActionActive(ctx, event, getActions(event.Action), b.windowSize)
	case actorGallerySegment:
		return b.eventRepo.IsActorGalleryActive(ctx, event, b.windowSize)
	case noSegment:
		return false, nil
	default:
		return false, nil
	}
}

func (b *EventBuilder) createUserFollowedUsersFeedEvent(ctx context.Context, event db.Event) (*db.FeedEvent, error) {
	var events []db.Event
	var err error
	if event.GroupID.String != "" {
		events, err = b.eventRepo.Queries.GetEventsInGroup(ctx, event.GroupID)
	} else {
		events, err = b.eventRepo.EventsInWindow(ctx, event.ID, viper.GetInt("FEED_WINDOW_SIZE"), persist.ActionList{event.Action}, false)
	}
	if err != nil {
		return nil, err
	}

	merged := mergeFollowEvents(events)
	if len(merged.followedIDs) < 1 {
		return nil, nil
	}

	return b.feedRepo.Add(ctx, db.FeedEvent{
		ID:        persist.GenerateID(),
		OwnerID:   persist.NullStrToDBID(merged.event.ActorID),
		Action:    merged.event.Action,
		EventTime: merged.event.CreatedAt,
		EventIds:  merged.eventIDs,
		Data: persist.FeedEventData{
			UserFollowedIDs:  merged.followedIDs,
			UserFollowedBack: merged.followedBack,
		},
	})
}

func (b *EventBuilder) createGalleryUpdatedFeedEvent(ctx context.Context, event db.Event) (*db.FeedEvent, error) {

	var events []db.Event
	var err error
	if event.GroupID.String != "" {
		events, err = b.queries.GetEventsInGroup(ctx, event.GroupID)
	} else {
		events, err = b.eventRepo.EventsInWindowForGallery(ctx, event.ID, event.GalleryID, viper.GetInt("FEED_WINDOW_SIZE"), groupingConfig[persist.ActionGalleryUpdated], false)
	}
	if err != nil {
		return nil, err
	}

	return b.createGalleryUpdatedFeedEventFromEvents(ctx, events)

}

func (b *EventBuilder) createGalleryUpdatedFeedEventFromEvents(ctx context.Context, events []db.Event) (*db.FeedEvent, error) {
	if len(events) == 0 {
		return nil, nil
	}

	merged := mergeGalleryEvents(events)
	if len(merged.eventIDs) == 0 {
		return nil, nil
	}

	return b.feedRepo.Add(ctx, db.FeedEvent{
		ID:        persist.GenerateID(),
		OwnerID:   merged.actorID,
		Action:    persist.ActionGalleryUpdated,
		EventTime: merged.eventTime,
		EventIds:  merged.eventIDs,
		Caption:   persist.StrToNullStr(merged.caption),
		Data: persist.FeedEventData{
			GalleryID:                           merged.galleryID,
			GalleryName:                         merged.galleryName,
			GalleryDescription:                  merged.galleryDescription,
			GalleryNewCollections:               merged.newCollections,
			GalleryNewCollectionTokenIDs:        merged.tokensAdded,
			GalleryNewCollectionCollectorsNotes: merged.collectionCollectorsNotes,
			GalleryNewTokenCollectorsNotes:      merged.tokenCollectorsNotes,
		},
	})
}

// getAddedTokens returns the new tokens that were added since the last published feed event.
func getAddedTokens(ctx context.Context, feedRepo *postgres.FeedRepository, event db.Event) (added []persist.DBID, hasPrior bool, err error) {
	priorEvent, err := feedRepo.LastPublishedCollectionFeedEvent(ctx, persist.NullStrToDBID(event.ActorID), event.CollectionID, event.CreatedAt, collectionTokensAddedActions)
	if err != nil {
		return added, true, err
	}

	// If a create event doesn't exist, then the collection was made before the feed
	// or the event itself is the create event.
	if priorEvent == nil {
		return event.Data.CollectionTokenIDs, false, nil
	}

	added = newTokens(event.Data.CollectionTokenIDs, priorEvent.Data.CollectionTokenIDs)
	return added, true, nil
}

// isCollectionCollectorsNoteChanged returns true if the collector's note differs from the last published feed event.
func isCollectionCollectorsNoteChanged(ctx context.Context, feedRepo *postgres.FeedRepository, event db.Event) (bool, error) {
	priorEvent, err := feedRepo.LastPublishedCollectionFeedEvent(ctx, persist.NullStrToDBID(event.ActorID), event.CollectionID, event.CreatedAt, collectionCollectorsNoteActions)
	if err != nil {
		return false, err
	}

	if priorEvent != nil && priorEvent.Data.CollectionNewCollectorsNote == event.Data.CollectionCollectorsNote {
		return false, nil
	}

	return true, nil
}

func getActions(action persist.Action) persist.ActionList {
	// Check if action belongs to a group
	if actions, ok := groupingConfig[action]; ok {
		return actions
	}
	return persist.ActionList{action}
}

func getSegment(action persist.Action) segment {
	// Check if action has a parent action
	if parent, ok := eventGroups[action]; ok {
		action = parent
	}

	eventSegment, ok := eventSegments[action]
	if !ok {
		return noSegment
	}
	return eventSegment
}

func newTokens(tokens []persist.DBID, otherTokens []persist.DBID) []persist.DBID {
	newTokens := make([]persist.DBID, 0)

	for _, token := range tokens {
		var exists bool

		for _, otherToken := range otherTokens {
			if token == otherToken {
				exists = true
				break
			}
		}

		if !exists {
			newTokens = append(newTokens, token)
		}
	}

	return newTokens
}
