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
	defaultEventGroups = map[persist.Action]actionGroup{
		persist.ActionCollectionCreated:               collectionGroup,
		persist.ActionCollectorsNoteAddedToCollection: collectionGroup,
		persist.ActionTokensAddedToCollection:         collectionGroup,
	}

	defaultEventCriterias = map[persist.Action]criteria{
		persist.ActionUserCreated:                     noCriteria,
		persist.ActionUserFollowedUsers:               actorActionCriteria,
		persist.ActionCollectorsNoteAddedToToken:      actorSubjectActionCriteria,
		persist.ActionCollectionCreated:               actorSubjectActionCriteria,
		persist.ActionCollectorsNoteAddedToCollection: actorSubjectActionCriteria,
		persist.ActionTokensAddedToCollection:         actorSubjectActionCriteria,
	}
)

var collectionGroup = actionGroup{
	persist.ActionCollectionCreated,
	persist.ActionTokensAddedToCollection,
	persist.ActionCollectorsNoteAddedToCollection,
}

const (
	noCriteria criteria = iota
	actorActionCriteria
	actorSubjectCriteria
	actorSubjectActionCriteria
)

var errUnhandledSingleEvent = errors.New("unhandled single event")
var errUnhandledGroupedEvent = errors.New("unhandled group event")

type actionGroup []persist.Action

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
	blocked, err := b.feedBlocklistRepo.IsBlocked(ctx, event.ActorID, event.Action)
	if err != nil || blocked {
		return nil, err
	}
	if _, groupable := defaultEventGroups[event.Action]; groupable {
		return b.createGroupedFeedEvent(ctx, event)
	}
	return b.createSingleFeedEvent(ctx, event)
}

func (b *EventBuilder) createGroupedFeedEvent(ctx context.Context, event db.Event) (*db.FeedEvent, error) {
	switch {
	case reflect.DeepEqual(defaultEventGroups[event.Action], collectionGroup):
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
	group, ok := defaultEventGroups[event.Action]
	if !ok {
		group = actionGroup{event.Action}
	}

	var criteria criteria
	if reflect.DeepEqual(group, collectionGroup) {
		criteria = actorSubjectCriteria
	} else if eventCriteria, ok := defaultEventCriterias[event.Action]; ok {
		criteria = eventCriteria
	} else {
		criteria = noCriteria
	}

	switch criteria {
	case actorActionCriteria:
		return b.eventRepo.IsActorActionActive(ctx, event, actions)
	case actorSubjectCriteria:
		return b.eventRepo.IsActorSubjectActive(ctx, event)
	case actorSubjectActionCriteria:
		return b.eventRepo.IsActorSubjectActionActive(ctx, event, actions)
	case noCriteria:
		return false, nil
	}

	return false, nil
}

func (b *EventBuilder) createUserCreatedFeedEvent(ctx context.Context, event db.Event) (*db.FeedEvent, error) {
	useEvent, err := b.useEvent(ctx, event, event.Action)
	if err != nil || !useEvent {
		return nil, err
	}

	priorEvent, err := b.feedRepo.LastPublishedUserFeedEvent(ctx, event)
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
		Caption:   event.Caption,
	})
}

func (b *EventBuilder) createUserFollowedUsersFeedEvent(ctx context.Context, event db.Event) (*db.FeedEvent, error) {
	useEvent, err := b.useEvent(ctx, event, event.Action)
	if err != nil || !useEvent {
		return nil, err
	}

	priorEvent, err := b.feedRepo.LastPublishedUserFeedEvent(ctx, event)
	if err != nil {
		return nil, err
	}

	events := []db.Event{event}

	if priorEvent != nil {
		events, err = b.eventRepo.EventsInWindow(ctx, event.ID, viper.GetInt("FEED_WINDOW_SIZE"))
		if err != nil {
			return nil, err
		}
	}

	merged := mergedFollowEvent{}.merge(events...).asFeedEvent()
	if len(merged.Data.UserFollowedIDs) == 0 {
		return nil, nil
	}

	return b.feedRepo.Add(ctx, merged)
}

func (b *EventBuilder) createCollectorsNoteAddedToTokenFeedEvent(ctx context.Context, event db.Event) (*db.FeedEvent, error) {
	useEvent, err := b.useEvent(ctx, event, event.Action)
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

	priorEvent, err := b.feedRepo.LastPublishedTokenFeedEvent(ctx, event)
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
	useEvent, err := b.useEvent(ctx, event, event.Action)
	if err != nil || !useEvent {
		return nil, err
	}

	// don't present empty notes
	if event.Data.CollectionCollectorsNote == "" {
		return nil, nil
	}

	priorEvent, err := b.feedRepo.LastPublishedCollectionFeedEvent(ctx, event.ActorID, event.SubjectID, event.CreatedAt, event.Action)
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
	useEvent, err := b.useEvent(ctx, event, event.Action)
	if err != nil || !useEvent {
		return nil, err
	}

	// Don't show empty collections
	if len(event.Data.CollectionTokenIDs) == 0 {
		return nil, nil
	}

	priorEvent, err := b.feedRepo.LastPublishedCollectionFeedEvent(ctx, event.ActorID, event.CollectionID, event.CreatedAt, collectionGroup)
	if err != nil {
		return nil, err
	}

	addedTokens := make([]persist.DBID, 0)
	var isPreFeed bool

	if priorEvent == nil {
		isPreFeed = true
	} else {
		addedTokens = newTokens(event.Data.CollectionTokenIDs, priorEvent.Data.CollectionTokenIDs)
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
	return nil, nil
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
