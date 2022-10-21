package feed

import (
	"context"
	"reflect"

	db "github.com/mikeydub/go-gallery/db/gen/coredb"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/service/persist/postgres"
	"github.com/mikeydub/go-gallery/service/task"
	"github.com/mikeydub/go-gallery/service/tracing"
	"github.com/spf13/viper"
)

var (
	defaultEventGroups = map[persist.Action]mergeGroup{
		persist.ActionCollectionCreated:               collectionGroup,
		persist.ActionCollectorsNoteAddedToCollection: collectionGroup,
		persist.ActionTokensAddedToCollection:         collectionGroup,
	}

	defaultGroupCriterias = map[string]activeCriteria{
		collectionGroup.String(): actorAndSubject,
	}

	defaultEventCriterias = map[persist.Action]activeCriteria{
		persist.ActionUserCreated:                     noCriteria,
		persist.ActionUserFollowedUsers:               actorAndAction,
		persist.ActionCollectorsNoteAddedToToken:      actorSubjectAndAction,
		persist.ActionCollectionCreated:               actorSubjectAndAction,
		persist.ActionCollectorsNoteAddedToCollection: actorSubjectAndAction,
		persist.ActionTokensAddedToCollection:         actorSubjectAndAction,
	}
)

var collectionGroup = mergeGroup{
	persist.ActionCollectionCreated,
	persist.ActionTokensAddedToCollection,
	persist.ActionCollectorsNoteAddedToCollection,
}

var (
	noCriteria            = activeCriteria{}
	actorAndAction        = activeCriteria{Actor: true, Action: true}
	actorAndSubject       = activeCriteria{Actor: true, Subject: true}
	actorSubjectAndAction = activeCriteria{Actor: true, Subject: true, Action: true}
)

type mergeGroup []persist.Action

func (m mergeGroup) String() string {
	result := ""
	for _, action := range m {
		result = result + "|" + string(action)
	}
	return result
}

type activeCriteria struct {
	Actor,
	Subject,
	Action bool
}

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

func (b *EventBuilder) NewEventFromTask(ctx context.Context, message task.FeedMessage) (*db.FeedEvent, error) {
	span, ctx := tracing.StartSpan(ctx, "eventBuilder.NewEvent", "newEvent")
	defer tracing.FinishSpan(span)

	event, err := b.eventRepo.Get(ctx, message.ID)
	if err != nil {
		return nil, err
	}

	return b.NewEvent(ctx, event)
}

func (b *EventBuilder) NewEvent(ctx context.Context, event db.Event) (*db.FeedEvent, error) {
	blocked, err := b.feedBlocklistRepo.IsBlocked(ctx, event.ActorID, event.Action)
	if err != nil {
		return nil, err
	}
	if blocked {
		return nil, nil
	}

	if reflect.DeepEqual(defaultEventGroups[event.Action], collectionGroup) {
		return createGroup
	}

	switch event.Action {
	case persist.ActionUserCreated:
		return b.createUserCreatedEvent(ctx, event)
	case persist.ActionUserFollowedUsers:
		return b.createUserFollowedUsersEvent(ctx, event)
	case persist.ActionCollectorsNoteAddedToToken:
		return b.createCollectorsNoteAddedToTokenEvent(ctx, event)
	case persist.ActionCollectionCreated:
		return b.createCollectionCreatedEvent(ctx, event)
	case persist.ActionCollectorsNoteAddedToCollection:
		return b.createCollectorsNoteAddedToCollectionEvent(ctx, event)
	case persist.ActionTokensAddedToCollection:
		return b.createTokensAddedToCollectionEvent(ctx, event)
	default:
		return nil, persist.ErrUnknownAction{Action: event.Action}
	}
}

func (b *EventBuilder) useEvent(ctx context.Context, event db.Event) (bool, error) {
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
	group, ok := defaultEventGroups[event.Action]
	if !ok {
		group = mergeGroup{event.Action}
	}

	key := group.String()
	groupCriteria, gOK := defaultGroupCriterias[key]
	eventCritiera, eOK := defaultEventCriterias[persist.Action(key)]

	var criteria activeCriteria
	switch {
	case gOK:
		criteria = groupCriteria
	case eOK:
		criteria = eventCritiera
	default:
		criteria = noCriteria
	}

	switch criteria {
	case actorAndAction:
		panic("implement")
	case actorAndSubject:
		panic("implement")
	case actorSubjectAndAction:
		panic("implement")
	}

	return false, nil
}

func (b *EventBuilder) createUserCreatedEvent(ctx context.Context, event db.Event) (*db.FeedEvent, error) {
	useEvent, err := b.useEvent(ctx, event)
	if err != nil || !useEvent {
		return nil, err
	}

	feedEvent, err := b.feedRepo.LastEventFrom(ctx, event)
	if err != nil {
		return nil, err
	}

	// only want to store this event type once
	if feedEvent != nil {
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

func (b *EventBuilder) createUserFollowedUsersEvent(ctx context.Context, event db.Event) (*db.FeedEvent, error) {
	useEvent, err := b.useEvent(ctx, event)
	if err != nil || !useEvent {
		return nil, err
	}

	feedEvent, err := b.feedRepo.LastEventFrom(ctx, event)
	if err != nil {
		return nil, err
	}

	events := []db.Event{event}

	if feedEvent != nil {
		events, err = b.eventRepo.EventsInWindow(ctx, event.ID, viper.GetInt("FEED_WINDOW_SIZE"))
		if err != nil {
			return nil, err
		}
	}

	merger := mergedFollowEvent{}
	mergedEvent := merger.Merge(events...)

	if !merger.hasNewFollows() {
		return nil, nil
	}

	return b.feedRepo.Add(ctx, mergedEvent)
}

func (b *EventBuilder) createCollectorsNoteAddedToTokenEvent(ctx context.Context, event db.Event) (*db.FeedEvent, error) {
	useEvent, err := b.useEvent(ctx, event)
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

	feedEvent, err := b.feedRepo.LastTokenEventFromEvent(ctx, event)
	if err != nil {
		return nil, err
	}

	// only show if note has changed
	if feedEvent != nil && feedEvent.Data.TokenNewCollectorsNote == event.Data.TokenCollectorsNote {
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

func (b *EventBuilder) createCollectionCreatedEvent(ctx context.Context, event db.Event) (*db.FeedEvent, error) {
	useEvent, err := b.useEvent(ctx, event)
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

func (b *EventBuilder) createCollectorsNoteAddedToCollectionEvent(ctx context.Context, event db.Event) (*db.FeedEvent, error) {
	useEvent, err := b.useEvent(ctx, event)
	if err != nil || !useEvent {
		return nil, err
	}

	// don't present empty notes
	if event.Data.CollectionCollectorsNote == "" {
		return nil, nil
	}

	feedEvent, err := b.feedRepo.LastCollectionEventFromEvent(ctx, event)
	if err != nil {
		return nil, err
	}

	// only show if note has changed
	if feedEvent != nil && feedEvent.Data.CollectionNewCollectorsNote == event.Data.CollectionCollectorsNote {
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

func (b *EventBuilder) createTokensAddedToCollectionEvent(ctx context.Context, event db.Event) (*db.FeedEvent, error) {
	useEvent, err := b.useEvent(ctx, event)
	if err != nil || !useEvent {
		return nil, err
	}

	// don't show empty collections
	if len(event.Data.CollectionTokenIDs) < 1 {
		return nil, nil
	}

	feedEvent, err := b.feedRepo.LastCollectionEventFromEvent(ctx, event)
	if err != nil {
		return nil, err
	}

	createEvent, err := b.feedRepo.LastCollectionEvent(ctx,
		event.ActorID, persist.ActionCollectionCreated, event.SubjectID, event.CreatedAt,
	)
	if err != nil {
		return nil, err
	}

	added := make([]persist.DBID, 0)
	var isPreFeed bool

	if feedEvent != nil {
		// compare against last token added event
		added = newTokens(event.Data.CollectionTokenIDs, feedEvent.Data.CollectionTokenIDs)
	} else if createEvent != nil {
		// compare against the create collection event
		added = newTokens(event.Data.CollectionTokenIDs, createEvent.Data.CollectionTokenIDs)
	} else {
		// don't have the create event, likely because the collection was created
		// before the feed
		isPreFeed = true
	}

	// only send if tokens added
	if !isPreFeed && len(added) == 0 {
		return nil, nil
	}

	return b.feedRepo.Add(ctx, db.FeedEvent{
		ID:      persist.GenerateID(),
		OwnerID: event.ActorID,
		Action:  event.Action,
		Data: persist.FeedEventData{
			CollectionID:          event.SubjectID,
			CollectionTokenIDs:    event.Data.CollectionTokenIDs,
			CollectionNewTokenIDs: added,
			CollectionIsPreFeed:   isPreFeed,
		},
		EventTime: event.CreatedAt,
		EventIds:  persist.DBIDList{event.ID},
		Caption:   event.Caption,
	})
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
