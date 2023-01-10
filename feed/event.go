package feed

import (
	"context"
	"errors"
	"time"

	db "github.com/mikeydub/go-gallery/db/gen/coredb"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/service/persist/postgres"
	"github.com/mikeydub/go-gallery/service/task"
	"github.com/mikeydub/go-gallery/service/tracing"
	"github.com/spf13/viper"
)

var (
	groupingConfig = map[persist.Action]persist.ActionList{
		persist.ActionCollectionUpdated: {
			persist.ActionCollectionCreated,
			persist.ActionTokensAddedToCollection,
			persist.ActionCollectorsNoteAddedToCollection,
		},
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
		persist.ActionUserFollowedUsers:               actorActionSegment,
		persist.ActionCollectorsNoteAddedToToken:      actorGallerySegment,
		persist.ActionCollectionCreated:               actorGallerySegment,
		persist.ActionCollectorsNoteAddedToCollection: actorGallerySegment,
		persist.ActionTokensAddedToCollection:         actorGallerySegment,
		persist.ActionCollectionUpdated:               actorSubjectSegment,
		persist.ActionGalleryUpdated:                  actorSubjectActionSegment,
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
	// skipCooldown, if enabled, will disregard the requisite "cooldown"
	// period of an incoming event.
	skipCooldown bool
	// windowSize is used to determine if a user is still editing and is ignored if
	// skipCooldown is enabled.
	windowSize time.Duration
}

func NewEventBuilder(queries *db.Queries, skipCooldown bool) *EventBuilder {
	return &EventBuilder{
		queries:           queries,
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

	return b.NewFeedEventFromEvent(ctx, event, false)
}

func (b *EventBuilder) NewFeedEventFromEvent(ctx context.Context, event db.Event, isImmediate bool) (*db.FeedEvent, error) {

	if isImmediate {
		switch eventGroups[event.Action] {
		case persist.ActionGalleryUpdated:
			wait, err := b.queries.HasLaterGalleryEvent(ctx, db.HasLaterGalleryEventParams{
				ActorID:   event.ActorID,
				Actions:   groupingConfig[persist.ActionGalleryUpdated],
				GalleryID: event.GalleryID,
				Caption:   event.Caption,
				EventID:   event.ID,
			})
			if err != nil {
				return nil, err
			}
			if wait {
				return nil, nil
			}
		}
	}

	if useEvent, err := b.useEvent(ctx, event); err != nil || !useEvent {
		return nil, err
	}
	_, groupable := eventGroups[event.Action]
	// Events with a caption are treated as a singular event.
	if event.Caption.String != "" || !groupable {
		return b.createFeedEvent(ctx, event)
	}
	return b.createGroupedFeedEvent(ctx, event)
}

func (b *EventBuilder) createGroupedFeedEvent(ctx context.Context, event db.Event) (*db.FeedEvent, error) {
	if eventGroups[event.Action] == persist.ActionCollectionUpdated {
		return b.createCollectionUpdatedFeedEvent(ctx, event)
	} else if eventGroups[event.Action] == persist.ActionGalleryUpdated {
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

func (b *EventBuilder) useEvent(ctx context.Context, event db.Event) (bool, error) {
	blocked, err := b.feedBlocklistRepo.IsBlocked(ctx, persist.NullStrToDBID(event.ActorID), event.Action)
	if err != nil || blocked {
		return false, err
	}

	if b.skipCooldown {
		return true, nil
	}

	stillEditing, err := b.isStillEditing(ctx, event)
	if err != nil {
		return false, err
	}

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
	events, err := b.eventRepo.EventsInWindow(ctx, event.ID, viper.GetInt("FEED_WINDOW_SIZE"), persist.ActionList{event.Action}, false)
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
	// will checking by gallery ID cause some events to slip through the cracks and not be handled?
	events, err := b.eventRepo.EventsInWindowForGallery(ctx, event.ID, event.GalleryID, viper.GetInt("FEED_WINDOW_SIZE"), groupingConfig[persist.ActionGalleryUpdated], false)
	if err != nil {
		return nil, err
	}

	merged := mergeGalleryEvents(events)
	if len(merged.eventIDs) == 0 {
		return nil, nil
	}
	return b.feedRepo.Add(ctx, db.FeedEvent{
		ID:        persist.GenerateID(),
		OwnerID:   merged.actorID,
		Action:    persist.ActionGalleryUpdated,
		EventTime: merged.createdAt.Time(),
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

func (b *EventBuilder) createSingleUpdateGalleryEvent(event db.Event, ctx context.Context) (*db.FeedEvent, error) {
	switch event.Action {

	case persist.ActionCollectorsNoteAddedToToken:
		return b.createCollectorsNoteAddedToTokenFeedEvent(ctx, event)
	case persist.ActionCollectionCreated:
		return b.createCollectionCreatedFeedEvent(ctx, event)
	case persist.ActionCollectorsNoteAddedToCollection:
		return b.createCollectorsNoteAddedToCollectionFeedEvent(ctx, event)
	case persist.ActionTokensAddedToCollection:
		return b.createTokensAddedToCollectionFeedEvent(ctx, event)
	case persist.ActionGalleryInfoUpdated:
		return b.createUpdateGalleryInfoFeedEvent(ctx, event)
	default:
		return nil, errUnhandledSingleEvent
	}

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

	priorEvent, err := b.feedRepo.LastPublishedTokenFeedEvent(ctx, persist.NullStrToDBID(event.ActorID), event.TokenID, event.CreatedAt, collectionCollectorsNoteActions)
	if err != nil {
		return nil, err
	}

	// only show if note has changed
	if priorEvent != nil && priorEvent.Data.TokenNewCollectorsNote == event.Data.TokenCollectorsNote {
		return nil, nil
	}

	return b.feedRepo.Add(ctx, db.FeedEvent{
		ID:      persist.GenerateID(),
		OwnerID: persist.NullStrToDBID(event.ActorID),
		Action:  event.Action,
		Data: persist.FeedEventData{
			TokenGalleryID:         event.GalleryID,
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
		OwnerID: persist.NullStrToDBID(event.ActorID),
		Action:  event.Action,
		Data: persist.FeedEventData{
			CollectionGalleryID:         event.GalleryID,
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

func (b *EventBuilder) createUpdateGalleryInfoFeedEvent(ctx context.Context, event db.Event) (*db.FeedEvent, error) {

	return b.feedRepo.Add(ctx, db.FeedEvent{
		ID:      persist.GenerateID(),
		OwnerID: persist.NullStrToDBID(event.ActorID),
		Action:  event.Action,
		Data: persist.FeedEventData{
			GalleryID:          event.SubjectID,
			GalleryName:        event.Data.GalleryName,
			GalleryDescription: event.Data.GalleryDescription,
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
		OwnerID: persist.NullStrToDBID(event.ActorID),
		Action:  event.Action,
		Data: persist.FeedEventData{
			CollectionGalleryID:         event.GalleryID,
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
		OwnerID: persist.NullStrToDBID(event.ActorID),
		Action:  event.Action,
		Data: persist.FeedEventData{
			CollectionGalleryID:   event.GalleryID,
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
	events, err := b.eventRepo.EventsInWindow(ctx, event.ID, viper.GetInt("FEED_WINDOW_SIZE"), groupingConfig[persist.ActionCollectionUpdated], true)
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
	// Treat the event as a collector's note event if no new tokens were added.
	if len(addedTokens) < 1 && (merged.event.Data.CollectionCollectorsNote != "" && noteChanged) {
		merged.event.Action = persist.ActionCollectorsNoteAddedToCollection
		return b.createFeedEvent(ctx, merged.event)
	}
	// Treat the event as a tokens added event if the note didn't change or is empty.
	if len(addedTokens) > 0 && (merged.event.Data.CollectionCollectorsNote == "" || !noteChanged) {
		merged.event.Action = persist.ActionTokensAddedToCollection
		return b.createFeedEvent(ctx, merged.event)
	}

	return b.feedRepo.Add(ctx, db.FeedEvent{
		ID:        persist.GenerateID(),
		OwnerID:   persist.NullStrToDBID(merged.event.ActorID),
		Action:    merged.event.Action,
		EventTime: merged.event.CreatedAt,
		EventIds:  merged.eventIDs,
		Data: persist.FeedEventData{
			CollectionGalleryID:         merged.event.GalleryID,
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
