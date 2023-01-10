package feed

import (
	db "github.com/mikeydub/go-gallery/db/gen/coredb"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/util"
)

func mergeFollowEvents(eventsAsc []db.Event) *combinedFollowEvent {
	var combined combinedFollowEvent
	return combined.merge(eventsAsc)
}

func mergeCollectionEvents(eventsAsc []db.Event) *combinedCollectionEvent {
	var combined combinedCollectionEvent
	return combined.merge(eventsAsc)
}

func mergeGalleryEvents(eventsAsc []db.Event) *combinedGalleryEvent {
	var combined combinedGalleryEvent
	return combined.merge(eventsAsc)
}

type combinedFollowEvent struct {
	event        db.Event
	eventIDs     []persist.DBID
	followedIDs  []persist.DBID
	followedBack []bool
}

func (c *combinedFollowEvent) merge(eventsAsc []db.Event) *combinedFollowEvent {
	for _, other := range eventsAsc {
		if !other.Data.UserRefollowed {
			c.event = db.Event{
				ID:        other.ID,
				ActorID:   other.ActorID,
				Action:    other.Action,
				CreatedAt: other.CreatedAt,
			}
			c.eventIDs = append(c.eventIDs, other.ID)
			c.followedIDs = append(c.followedIDs, other.SubjectID)
			c.followedBack = append(c.followedBack, other.Data.UserFollowedBack)
		}
	}
	return c
}

type combinedCollectionEvent struct {
	event           db.Event
	eventIDs        []persist.DBID
	isNewCollection bool
}

func (c *combinedCollectionEvent) merge(eventsAsc []db.Event) *combinedCollectionEvent {
	for _, other := range eventsAsc {
		action := c.event.Action
		caption := c.event.Caption

		// If the collection is new, then categorize the event as a new collection event. Otherwise,
		// if there are two or more unique actions, the resulting event is categorized as
		// a generic update.
		if c.event.Action == "" {
			action = other.Action
		} else if action != persist.ActionCollectionCreated && c.event.Action != other.Action {
			action = persist.ActionCollectionUpdated
		}

		if c.event.Caption.String == "" {
			caption = other.Caption
		}

		// Not every event has tokens attached to it, so we check that the event is relevant first.
		collectionTokenIDs := c.event.Data.CollectionTokenIDs
		if other.Action == persist.ActionCollectionCreated || other.Action == persist.ActionTokensAddedToCollection {
			collectionTokenIDs = other.Data.CollectionTokenIDs
		}

		// Not every event has a collector's note attached to it, so we check that the event is relevant first.
		collectorsNote := c.event.Data.CollectionCollectorsNote
		if other.Action == persist.ActionCollectionCreated || other.Action == persist.ActionCollectorsNoteAddedToCollection {
			collectorsNote = other.Data.CollectionCollectorsNote
		}

		c.event = db.Event{
			ID:           other.ID,
			ActorID:      other.ActorID,
			SubjectID:    other.SubjectID,
			GalleryID:    other.GalleryID,
			CollectionID: other.CollectionID,
			Action:       action,
			CreatedAt:    other.CreatedAt,
			Caption:      caption,
			Data: persist.EventData{
				CollectionTokenIDs:       collectionTokenIDs,
				CollectionCollectorsNote: collectorsNote,
			},
		}
		c.eventIDs = append(c.eventIDs, other.ID)
		c.isNewCollection = c.isNewCollection || other.Action == persist.ActionCollectionCreated
	}
	return c
}

type combinedGalleryEvent struct {
	createdAt                 persist.CreationTime
	galleryID                 persist.DBID
	actorID                   persist.DBID
	eventIDs                  []persist.DBID
	newCollections            []persist.DBID
	collectionCollectorsNotes map[persist.DBID]string
	tokenCollectorsNotes      map[persist.DBID]string
	tokensAdded               map[persist.DBID]persist.DBIDList
	galleryName               string
	galleryDescription        string
	caption                   *string
}

func (c *combinedGalleryEvent) merge(eventsAsc []db.Event) *combinedGalleryEvent {

	// first group collection events by coll id
	collectionEvents := make(map[persist.DBID][]db.Event)
	for _, event := range eventsAsc {
		if event.Action == persist.ActionGalleryInfoUpdated {
			c.galleryName = event.Data.GalleryName
			c.galleryDescription = event.Data.GalleryDescription
			continue
		} else if event.Action == persist.ActionCollectorsNoteAddedToToken {
			if c.tokenCollectorsNotes == nil {
				c.tokenCollectorsNotes = make(map[persist.DBID]string)
			}
			c.tokenCollectorsNotes[event.SubjectID] = event.Data.TokenCollectorsNote
			continue
		}

		if event.Caption.String != "" {
			c.caption = &event.Caption.String
		}

		collectionEvents[event.CollectionID] = append(collectionEvents[event.CollectionID], event)
	}

	mergedCollEvents := make([]*combinedCollectionEvent, 0, len(collectionEvents))
	for _, events := range collectionEvents {
		merged := mergeCollectionEvents(events)
		mergedCollEvents = append(mergedCollEvents, merged)
	}

	for _, collEvent := range mergedCollEvents {
		if collEvent.event.Data.CollectionCollectorsNote != "" {
			if c.collectionCollectorsNotes == nil {
				c.collectionCollectorsNotes = make(map[persist.DBID]string)
			}
			c.collectionCollectorsNotes[collEvent.event.CollectionID] = collEvent.event.Data.CollectionCollectorsNote
		}
		if collEvent.event.Data.CollectionTokenIDs != nil {
			if c.tokensAdded == nil {
				c.tokensAdded = make(map[persist.DBID]persist.DBIDList)
			}
			c.tokensAdded[collEvent.event.CollectionID] = collEvent.event.Data.CollectionTokenIDs
		}
		if collEvent.isNewCollection {
			c.newCollections = append(c.newCollections, collEvent.event.CollectionID)
		}

		if collEvent.event.Caption.String != "" {
			c.caption = &collEvent.event.Caption.String
		}
	}

	c.eventIDs, _ = util.Map(eventsAsc, func(event db.Event) (persist.DBID, error) {
		return event.ID, nil
	})

	return c
}
