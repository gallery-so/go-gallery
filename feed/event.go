package feed

import (
	"context"
	"errors"
	"reflect"

	db "github.com/mikeydub/go-gallery/db/gen/coredb"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/service/persist/postgres"
	"github.com/mikeydub/go-gallery/service/task"
	"github.com/mikeydub/go-gallery/service/tracing"
	"github.com/spf13/viper"
)

var (
	defaultEventGroups = map[persist.Action][]persist.Action{
		persist.ActionCollectionCreated:               updateCollectionActions,
		persist.ActionCollectorsNoteAddedToCollection: updateCollectionActions,
		persist.ActionTokensAddedToCollection:         updateCollectionActions,
	}

	defaultEventCriterias = map[persist.Action]criteria{
		persist.ActionUserCreated:                     noCriteria,
		persist.ActionUserFollowedUsers:               actorActionCriteria,
		persist.ActionCollectorsNoteAddedToToken:      actorSubjectActionCriteria,
		persist.ActionCollectionCreated:               actorSubjectActionCriteria,
		persist.ActionCollectorsNoteAddedToCollection: actorSubjectActionCriteria,
		persist.ActionTokensAddedToCollection:         actorSubjectActionCriteria,
	}

	// Events in this group can be grouped together as a collection update
	updateCollectionActions = []persist.Action{
		persist.ActionCollectionCreated,
		persist.ActionTokensAddedToCollection,
		persist.ActionCollectorsNoteAddedToCollection,
	}

	// Events in this group can contain a collection collector's note
	collectionCollectorsNoteActions = []persist.Action{
		persist.ActionCollectionUpdated,
		persist.ActionCollectorsNoteAddedToCollection,
		persist.ActionCollectionCreated,
	}
)

const (
	noCriteria criteria = iota
	actorActionCriteria
	actorSubjectCriteria
	actorSubjectActionCriteria
)

var errUnhandledSingleEvent = errors.New("unhandled single event")
var errUnhandledGroupedEvent = errors.New("unhandled group event")

type criteria int

type EventBuilder struct {
	eventRepo         *postgres.EventRepository
	feedRepo          *postgres.FeedRepository
	feedBlocklistRepo *postgres.FeedBlocklistRepository
	// skipCooldown, if enabled, will disregard the requisite "cooldown"
	// period of an incoming event.
	skipCooldown bool
}

func NewEventBuilder(queries *db.Queries, skipCooldown bool) *EventBuilder {
	return &EventBuilder{
		eventRepo:         &postgres.EventRepository{Queries: queries},
		feedRepo:          &postgres.FeedRepository{Queries: queries},
		feedBlocklistRepo: &postgres.FeedBlocklistRepository{Queries: queries},
		skipCooldown:      skipCooldown,
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
	if _, groupable := defaultEventGroups[event.Action]; groupable {
		return b.createGroupedFeedEvent(ctx, event)
	}
	return b.createSingleFeedEvent(ctx, event)
}

func (b *EventBuilder) createGroupedFeedEvent(ctx context.Context, event db.Event) (*db.FeedEvent, error) {
	switch {
	case reflect.DeepEqual(defaultEventGroups[event.Action], updateCollectionActions):
		return b.createCollectionUpdatedFeedEvent(ctx, event)
	default:
		return nil, errUnhandledGroupedEvent
	}
}

func (b *EventBuilder) createSingleFeedEvent(ctx context.Context, event db.Event) (*db.FeedEvent, error) {
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

func (b *EventBuilder) useEvent(ctx context.Context, event db.Event, actions ...persist.Action) (bool, error) {
	blocked, err := b.feedBlocklistRepo.IsBlocked(ctx, event.ActorID, event.Action)
	if err != nil || blocked {
		return false, err
	}
	if b.skipCooldown {
		return true, nil
	}
	active, err := b.isActive(ctx, event, actions)
	if err != nil {
		return false, err
	}
	return !active, nil
}

func (b *EventBuilder) isActive(ctx context.Context, event db.Event, actions []persist.Action) (bool, error) {
	switch activeCriteriaForAction(event.Action) {
	case actorActionCriteria:
		return b.eventRepo.IsActorActionActive(ctx, event, actions)
	case actorSubjectCriteria:
		return b.eventRepo.IsActorSubjectActive(ctx, event)
	case actorSubjectActionCriteria:
		return b.eventRepo.IsActorSubjectActionActive(ctx, event, actions)
	case noCriteria:
		return false, nil
	default:
		return false, nil
	}
}

func (b *EventBuilder) createUserCreatedFeedEvent(ctx context.Context, event db.Event) (*db.FeedEvent, error) {
	useEvent, err := b.useEvent(ctx, event, persist.ActionUserCreated)
	if err != nil || !useEvent {
		return nil, err
	}

	priorEvent, err := b.feedRepo.LastPublishedUserFeedEvent(ctx, event.ActorID, event.CreatedAt, persist.ActionUserCreated)
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
		Action:    persist.ActionUserCreated,
		EventTime: event.CreatedAt,
		Data:      persist.FeedEventData{UserBio: event.Data.UserBio},
		EventIds:  persist.DBIDList{event.ID},
	})
}

func (b *EventBuilder) createUserFollowedUsersFeedEvent(ctx context.Context, event db.Event) (*db.FeedEvent, error) {
	useEvent, err := b.useEvent(ctx, event, persist.ActionUserFollowedUsers)
	if err != nil || !useEvent {
		return nil, err
	}

	events, err := b.eventRepo.EventsInWindow(ctx, event.ID, viper.GetInt("FEED_WINDOW_SIZE"), persist.ActionUserFollowedUsers)
	if err != nil {
		return nil, err
	}

	merged := mergeFollowEvents(events)
	if len(merged.followedIDs) < 1 {
		return nil, nil
	}

	return b.feedRepo.Add(ctx, merged.asFeedEvent())
}

func (b *EventBuilder) createCollectorsNoteAddedToTokenFeedEvent(ctx context.Context, event db.Event) (*db.FeedEvent, error) {
	useEvent, err := b.useEvent(ctx, event, persist.ActionCollectorsNoteAddedToToken)
	if err != nil || !useEvent {
		return nil, err
	}

	// don't present empty notes
	if event.Data.TokenCollectorsNote == "" {
		return nil, nil
	}

	// token should be edited in the context of a collection
	if event.Data.TokenCollectionID == "" {
		return nil, nil
	}

	priorEvent, err := b.feedRepo.LastPublishedTokenFeedEvent(ctx, event.ActorID, event.TokenID, event.CreatedAt, persist.ActionCollectorsNoteAddedToToken)
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
		Action:  persist.ActionCollectorsNoteAddedToToken,
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
	useEvent, err := b.useEvent(ctx, event, event.Action)
	if err != nil || !useEvent {
		return nil, err
	}

	// don't show empty collections
	if len(event.Data.CollectionTokenIDs) < 1 {
		return nil, nil
	}

	return b.feedRepo.Add(ctx, db.FeedEvent{
		ID:      persist.GenerateID(),
		OwnerID: event.ActorID,
		Action:  persist.ActionCollectionCreated,
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
	useEvent, err := b.useEvent(ctx, event, persist.ActionCollectorsNoteAddedToCollection)
	if err != nil || !useEvent {
		return nil, err
	}

	// don't present empty notes
	if event.Data.CollectionCollectorsNote == "" {
		return nil, nil
	}

	priorEvent, err := b.feedRepo.LastPublishedCollectionFeedEvent(ctx, event.ActorID, event.SubjectID, event.CreatedAt, collectionCollectorsNoteActions...)
	if err != nil {
		return nil, err
	}

	// only show if note has changed
	if priorEvent != nil && priorEvent.Data.CollectionNewCollectorsNote == event.Data.CollectionCollectorsNote {
		return nil, nil
	}

	return b.feedRepo.Add(ctx, db.FeedEvent{
		ID:      persist.GenerateID(),
		OwnerID: event.ActorID,
		Action:  persist.ActionCollectorsNoteAddedToCollection,
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
	useEvent, err := b.useEvent(ctx, event, event.Action)
	if err != nil || !useEvent {
		return nil, err
	}

	// Don't show empty collections
	if len(event.Data.CollectionTokenIDs) == 0 {
		return nil, nil
	}

	addedTokens, isPreFeed, err := getNewlyAddedTokens(ctx, b.feedRepo, event)
	if err != nil {
		return nil, err
	}

	// Only send if tokens were added
	if !isPreFeed && len(addedTokens) == 0 {
		return nil, nil
	}

	return b.feedRepo.Add(ctx, db.FeedEvent{
		ID:      persist.GenerateID(),
		OwnerID: event.ActorID,
		Action:  event.Action,
		Data: persist.FeedEventData{
			CollectionID:          event.SubjectID,
			CollectionTokenIDs:    event.Data.CollectionTokenIDs,
			CollectionNewTokenIDs: addedTokens,
			CollectionIsPreFeed:   isPreFeed,
		},
		EventTime: event.CreatedAt,
		EventIds:  persist.DBIDList{event.ID},
		Caption:   event.Caption,
	})
}

func (b *EventBuilder) createCollectionUpdatedFeedEvent(ctx context.Context, event db.Event) (*db.FeedEvent, error) {
	useEvent, err := b.useEvent(ctx, event, updateCollectionActions...)
	if err != nil || !useEvent {
		return nil, err
	}

	events, err := b.eventRepo.EventsInWindow(ctx, event.ID, viper.GetInt("FEED_WINDOW_SIZE"), updateCollectionActions...)
	if err != nil {
		return nil, err
	}

	merged := mergeCollectionEvents(events)
	// If the merged event is made up of only one type of event,
	// then treat the merged event as a normal event.
	if merged.evt.Action != persist.ActionCollectionUpdated {
		return b.createSingleFeedEvent(ctx, merged.evt)
	}

	addedTokens, _, err := getNewlyAddedTokens(ctx, b.feedRepo, event)
	if err != nil {
		return nil, err
	}

	// It's not a very interesting event to show if no tokens were added or
	if len(addedTokens) < 1 && merged.evt.Data.CollectionCollectorsNote == "" {
		return nil, nil
	}

	return b.feedRepo.Add(ctx, merged.asFeedEvent(addedTokens))
}

func getNewlyAddedTokens(ctx context.Context, feedRepo *postgres.FeedRepository, event db.Event) (addedTokens []persist.DBID, isPreFeed bool, err error) {
	priorEvent, err := feedRepo.LastPublishedCollectionFeedEvent(ctx, event.ActorID, event.CollectionID, event.CreatedAt, persist.ActionCollectionCreated, persist.ActionTokensAddedToCollection)
	if err != nil {
		return nil, true, err
	}

	// If a create event doesn't exist, then the collection was made before the feed.
	if priorEvent == nil {
		return nil, true, nil
	}

	addedTokens = newTokens(event.Data.CollectionTokenIDs, priorEvent.Data.CollectionTokenIDs)
	return addedTokens, false, nil
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

func activeCriteriaForAction(action persist.Action) criteria {
	if reflect.DeepEqual(defaultEventGroups[action], updateCollectionActions) {
		return actorSubjectCriteria
	} else if eventCriteria, ok := defaultEventCriterias[action]; ok {
		return eventCriteria
	} else {
		return noCriteria
	}
}
