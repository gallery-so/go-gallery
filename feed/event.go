package feed

import (
	"context"
	"errors"
	"reflect"
	"time"

	db "github.com/mikeydub/go-gallery/db/gen/coredb"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/service/persist/postgres"
	"github.com/mikeydub/go-gallery/service/task"
	"github.com/mikeydub/go-gallery/service/tracing"
	"github.com/spf13/viper"
)

var (
	defaultEventGroups = map[persist.Action]actions{
		persist.ActionCollectionCreated:               editCollectionActions,
		persist.ActionCollectorsNoteAddedToCollection: editCollectionActions,
		persist.ActionTokensAddedToCollection:         editCollectionActions,
	}

	defaultSegments = map[persist.Action]segment{
		persist.ActionUserCreated:                     noSegment,
		persist.ActionUserFollowedUsers:               actorActionSegment,
		persist.ActionCollectorsNoteAddedToToken:      actorSubjectActionSegment,
		persist.ActionCollectionCreated:               actorSubjectActionSegment,
		persist.ActionCollectorsNoteAddedToCollection: actorSubjectActionSegment,
		persist.ActionTokensAddedToCollection:         actorSubjectActionSegment,
	}

	// Events in this group can be grouped together as a single collection update
	editCollectionActions = actions{
		persist.ActionCollectionCreated,
		persist.ActionTokensAddedToCollection,
		persist.ActionCollectorsNoteAddedToCollection,
	}

	// Feed events in this group can contain a collection collector's note
	collectionCollectorsNoteActions = actions{
		persist.ActionCollectionUpdated,
		persist.ActionCollectorsNoteAddedToCollection,
		persist.ActionCollectionCreated,
	}

	// Feed events in this group can contain added tokens
	collectionTokensAddedActions = actions{
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
)

var errUnhandledSingleEvent = errors.New("unhandled single event")
var errUnhandledGroupedEvent = errors.New("unhandled group event")

type segment int
type actions []persist.Action

type EventBuilder struct {
	eventRepo         *postgres.EventRepository
	feedRepo          *postgres.FeedRepository
	feedBlocklistRepo *postgres.FeedBlocklistRepository
	// skipCooldown, if enabled, will disregard the requisite "cooldown"
	// period of an incoming event.
	skipCooldown bool
	// windowSize is used to determine if a user is still editing and is ignored if
	// skipCooldown is enabled.
	windowSize time.Duration
}

func NewEventBuilder(queries *db.Queries, skipCooldown bool) *EventBuilder {
	return &EventBuilder{
		eventRepo:         &postgres.EventRepository{Queries: queries},
		feedRepo:          &postgres.FeedRepository{Queries: queries},
		feedBlocklistRepo: &postgres.FeedBlocklistRepository{Queries: queries},
		skipCooldown:      skipCooldown,
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
	if useEvent, err := b.useEvent(ctx, event); err != nil || !useEvent {
		return nil, err
	}

	if _, groupable := defaultEventGroups[event.Action]; groupable {
		return b.createGroupedFeedEvent(ctx, event)
	}

	return b.createFeedEvent(ctx, event)
}

func (b *EventBuilder) createGroupedFeedEvent(ctx context.Context, event db.Event) (*db.FeedEvent, error) {
	if reflect.DeepEqual(defaultEventGroups[event.Action], editCollectionActions) {
		return b.createCollectionUpdatedFeedEvent(ctx, event)
	}
	return nil, errUnhandledGroupedEvent
}

func (b *EventBuilder) createFeedEvent(ctx context.Context, event db.Event) (*db.FeedEvent, error) {
	switch event.Action {
	case persist.ActionUserCreated:
		return b.createUserCreatedFeedEvent(ctx, event)
	case persist.ActionUserFollowedUsers:
		return b.createUserFollowedUsersFeedEvent(ctx, event)
	case persist.ActionCollectorsNoteAddedToToken:
		return b.createCollectorsNoteAddedToTokenFeedEvent(ctx, event)
	case persist.ActionCollectionCreated:
		return b.createCollectionCreatedFeedEvent(ctx, event)
	case persist.ActionCollectorsNoteAddedToCollection:
		return b.createCollectorsNoteAddedToCollectionFeedEvent(ctx, event)
	case persist.ActionTokensAddedToCollection:
		return b.createTokensAddedToCollectionFeedEvent(ctx, event)
	default:
		return nil, errUnhandledSingleEvent
	}
}

func (b *EventBuilder) useEvent(ctx context.Context, event db.Event) (bool, error) {
	blocked, err := b.feedBlocklistRepo.IsBlocked(ctx, event.ActorID, event.Action)
	if err != nil || blocked {
		return false, err
	}

	if b.skipCooldown {
		return true, nil
	}

	active, err := b.isActive(ctx, event)
	if err != nil {
		return false, err
	}

	return !active, nil
}

func (b *EventBuilder) isActive(ctx context.Context, event db.Event) (bool, error) {
	segment, actions := segmentForAction(event.Action)
	switch segment {
	case actorActionSegment:
		return b.eventRepo.IsActorActionActive(ctx, event, actions, b.windowSize)
	case actorSubjectSegment:
		return b.eventRepo.IsActorSubjectActive(ctx, event, b.windowSize)
	case actorSubjectActionSegment:
		return b.eventRepo.IsActorSubjectActionActive(ctx, event, actions, b.windowSize)
	case noSegment:
		return false, nil
	default:
		return false, nil
	}
}

func (b *EventBuilder) createUserCreatedFeedEvent(ctx context.Context, event db.Event) (*db.FeedEvent, error) {
	priorEvent, err := b.feedRepo.LastPublishedUserFeedEvent(ctx, event.ActorID, event.CreatedAt, actions{persist.ActionUserCreated})
	if err != nil {
		return nil, err
	}

	// only want to store this event type once
	if priorEvent != nil {
		return nil, nil
	}

	return b.feedRepo.Add(ctx, db.FeedEvent{
		ID:        persist.GenerateID(),
		OwnerID:   event.ActorID,
		Action:    event.Action,
		EventTime: event.CreatedAt,
		Data:      persist.FeedEventData{UserBio: event.Data.UserBio},
		EventIds:  persist.DBIDList{event.ID},
	})
}

func (b *EventBuilder) createUserFollowedUsersFeedEvent(ctx context.Context, event db.Event) (*db.FeedEvent, error) {
	events, err := b.eventRepo.EventsInWindow(ctx, event.ID, viper.GetInt("FEED_WINDOW_SIZE"), actions{persist.ActionUserFollowedUsers})
	if err != nil {
		return nil, err
	}

	merged := mergeFollowEvents(events)
	if len(merged.followedIDs) < 1 {
		return nil, nil
	}

	return b.feedRepo.Add(ctx, db.FeedEvent{
		ID:        persist.GenerateID(),
		OwnerID:   merged.event.ActorID,
		Action:    merged.event.Action,
		EventTime: merged.event.CreatedAt,
		EventIds:  merged.eventIDs,
		Data: persist.FeedEventData{
			UserFollowedIDs:  merged.followedIDs,
			UserFollowedBack: merged.followedBack,
		},
	})
}

func (b *EventBuilder) createCollectorsNoteAddedToTokenFeedEvent(ctx context.Context, event db.Event) (*db.FeedEvent, error) {
	// don't present empty notes
	if event.Data.TokenCollectorsNote == "" {
		return nil, nil
	}

	// token should be edited in the context of a collection
	if event.Data.TokenCollectionID == "" {
		return nil, nil
	}

	priorEvent, err := b.feedRepo.LastPublishedTokenFeedEvent(ctx, event.ActorID, event.TokenID, event.CreatedAt, collectionCollectorsNoteActions)
	if err != nil {
		return nil, err
	}

	// only show if note has changed
	if priorEvent != nil && priorEvent.Data.TokenNewCollectorsNote == event.Data.TokenCollectorsNote {
		return nil, nil
	}

	return b.feedRepo.Add(ctx, db.FeedEvent{
		ID:      persist.GenerateID(),
		OwnerID: event.ActorID,
		Action:  event.Action,
		Data: persist.FeedEventData{
			TokenID:                event.SubjectID,
			TokenCollectionID:      event.Data.TokenCollectionID,
			TokenNewCollectorsNote: event.Data.TokenCollectorsNote,
		},
		EventTime: event.CreatedAt,
		EventIds:  persist.DBIDList{event.ID},
		Caption:   event.Caption,
	})
}

func (b *EventBuilder) createCollectionCreatedFeedEvent(ctx context.Context, event db.Event) (*db.FeedEvent, error) {
	// don't show empty collections
	if len(event.Data.CollectionTokenIDs) < 1 {
		return nil, nil
	}

	return b.feedRepo.Add(ctx, db.FeedEvent{
		ID:      persist.GenerateID(),
		OwnerID: event.ActorID,
		Action:  event.Action,
		Data: persist.FeedEventData{
			CollectionID:                event.SubjectID,
			CollectionTokenIDs:          event.Data.CollectionTokenIDs,
			CollectionNewTokenIDs:       event.Data.CollectionTokenIDs,
			CollectionNewCollectorsNote: event.Data.CollectionCollectorsNote,
		},
		EventTime: event.CreatedAt,
		EventIds:  persist.DBIDList{event.ID},
		Caption:   event.Caption,
	})
}

func (b *EventBuilder) createCollectorsNoteAddedToCollectionFeedEvent(ctx context.Context, event db.Event) (*db.FeedEvent, error) {
	// don't present empty notes
	if event.Data.CollectionCollectorsNote == "" {
		return nil, nil
	}

	if changed, err := isCollectionCollectorsNoteChanged(ctx, b.feedRepo, event); err != nil || !changed {
		return nil, err
	}

	return b.feedRepo.Add(ctx, db.FeedEvent{
		ID:      persist.GenerateID(),
		OwnerID: event.ActorID,
		Action:  event.Action,
		Data: persist.FeedEventData{
			CollectionID:                event.SubjectID,
			CollectionNewCollectorsNote: event.Data.CollectionCollectorsNote,
		},
		EventTime: event.CreatedAt,
		EventIds:  persist.DBIDList{event.ID},
		Caption:   event.Caption,
	})
}

func (b *EventBuilder) createTokensAddedToCollectionFeedEvent(ctx context.Context, event db.Event) (*db.FeedEvent, error) {
	// Don't show empty collections
	if len(event.Data.CollectionTokenIDs) == 0 {
		return nil, nil
	}

	addedTokens, hasPrior, err := getAddedTokens(ctx, b.feedRepo, event)
	if err != nil || (hasPrior && len(addedTokens) < 1) {
		return nil, err
	}

	return b.feedRepo.Add(ctx, db.FeedEvent{
		ID:      persist.GenerateID(),
		OwnerID: event.ActorID,
		Action:  event.Action,
		Data: persist.FeedEventData{
			CollectionID:          event.SubjectID,
			CollectionTokenIDs:    event.Data.CollectionTokenIDs,
			CollectionNewTokenIDs: addedTokens,
			CollectionIsPreFeed:   !hasPrior,
		},
		EventTime: event.CreatedAt,
		EventIds:  persist.DBIDList{event.ID},
		Caption:   event.Caption,
	})
}

func (b *EventBuilder) createCollectionUpdatedFeedEvent(ctx context.Context, event db.Event) (*db.FeedEvent, error) {
	events, err := b.eventRepo.EventsInWindow(ctx, event.ID, viper.GetInt("FEED_WINDOW_SIZE"), editCollectionActions)
	if err != nil {
		return nil, err
	}

	merged := mergeCollectionEvents(events)

	// Just treat the event as a typical event
	if merged.event.Action != persist.ActionCollectionUpdated {
		return b.createFeedEvent(ctx, merged.event)
	}

	addedTokens, _, err := getAddedTokens(ctx, b.feedRepo, merged.event)
	if err != nil {
		return nil, err
	}

	noteChanged, err := isCollectionCollectorsNoteChanged(ctx, b.feedRepo, event)
	if err != nil {
		return nil, err
	}

	// Skip storing the event if nothing interesting changed
	if len(addedTokens) < 1 && (merged.event.Data.CollectionCollectorsNote == "" || !noteChanged) {
		return nil, nil
	}

	return b.feedRepo.Add(ctx, db.FeedEvent{
		ID:        persist.GenerateID(),
		OwnerID:   merged.event.ActorID,
		Action:    merged.event.Action,
		EventTime: merged.event.CreatedAt,
		EventIds:  merged.eventIDs,
		Data: persist.FeedEventData{
			CollectionID:                merged.event.SubjectID,
			CollectionTokenIDs:          merged.event.Data.CollectionTokenIDs,
			CollectionNewCollectorsNote: merged.event.Data.CollectionCollectorsNote,
			CollectionIsNew:             merged.isNewCollection,
			CollectionNewTokenIDs:       addedTokens,
		},
	})
}

// getAddedTokens returns the new tokens that were added since the last published feed event.
func getAddedTokens(ctx context.Context, feedRepo *postgres.FeedRepository, event db.Event) (added []persist.DBID, hasPrior bool, err error) {
	priorEvent, err := feedRepo.LastPublishedCollectionFeedEvent(ctx, event.ActorID, event.CollectionID, event.CreatedAt, collectionTokensAddedActions)
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
	priorEvent, err := feedRepo.LastPublishedCollectionFeedEvent(ctx, event.ActorID, event.CollectionID, event.CreatedAt, collectionCollectorsNoteActions)
	if err != nil {
		return false, err
	}

	if priorEvent != nil && priorEvent.Data.CollectionNewCollectorsNote == event.Data.CollectionCollectorsNote {
		return false, nil
	}

	return true, nil
}

func segmentForAction(action persist.Action) (segment, actions) {
	if reflect.DeepEqual(defaultEventGroups[action], editCollectionActions) {
		return actorSubjectSegment, editCollectionActions
	} else if eventSegment, ok := defaultSegments[action]; ok {
		return eventSegment, actions{action}
	} else {
		return noSegment, actions{}
	}
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
