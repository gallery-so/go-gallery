package main

import (
	"context"
	"fmt"
	"math/rand"
	"os"
	"time"

	"github.com/mikeydub/go-gallery/db/gen/coredb"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/service/persist/postgres"
	"github.com/spf13/viper"
)

// run with `go run cmd/notification_prepop/main.go ${some user ID to use as the viewer}`

func main() {

	setDefaults()

	start := time.Now()
	defer func() {
		elapsed := time.Since(start)
		fmt.Printf("Took %s", elapsed)
	}()

	ownerID := persist.DBID(os.Args[1])

	pg := postgres.NewPgxClient()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	var ownerGalleryID persist.DBID
	err := pg.QueryRow(ctx, "SELECT id FROM galleries WHERE owner_user_id = $1", ownerID).Scan(&ownerGalleryID)
	if err != nil {
		panic(err)
	}

	aFewUsers, err := pg.Query(ctx, "SELECT ID FROM USERS LIMIT 20")
	if err != nil {
		panic(err)
	}

	userIDs := make([]persist.DBID, 0)
	for aFewUsers.Next() {
		var id persist.DBID
		err := aFewUsers.Scan(&id)
		if err != nil {
			panic(err)
		}
		userIDs = append(userIDs, id)
	}

	aFewFeedEvents, err := pg.Query(ctx, "SELECT ID FROM feed_events LIMIT 20")
	if err != nil {
		panic(err)
	}

	feedEventIDs := make([]persist.DBID, 0)
	for aFewFeedEvents.Next() {
		var id persist.DBID
		err := aFewFeedEvents.Scan(&id)
		if err != nil {
			panic(err)
		}
		feedEventIDs = append(feedEventIDs, id)
	}

	notifs := make([]coredb.Notification, 0, len(userIDs))
	comments := make([]coredb.Comment, 0, len(userIDs))
	admires := make([]coredb.Admire, 0, len(userIDs))
	events := make([]coredb.Event, 0, len(userIDs))
	for _, id := range userIDs {
		action := actionForNum(rand.Intn(5))

		var resource persist.ResourceType
		var subject persist.DBID
		var extraneousID persist.DBID
		switch action {
		case persist.ActionViewedGallery:
			resource = persist.ResourceTypeGallery
			subject = ownerGalleryID
		case persist.ActionUserFollowedUsers:
			resource = persist.ResourceTypeUser
			subject = ownerID
		case persist.ActionCommentedOnFeedEvent:
			comment := coredb.Comment{
				ID:          persist.GenerateID(),
				FeedEventID: feedEventIDs[rand.Intn(len(feedEventIDs))],
				ActorID:     id,
				Comment:     "This is a comment",
			}
			resource = persist.ResourceTypeComment
			subject = comment.ID
			extraneousID = comment.FeedEventID
			comments = append(comments, comment)
		case persist.ActionAdmiredFeedEvent:
			admire := coredb.Admire{
				ID:          persist.GenerateID(),
				FeedEventID: feedEventIDs[rand.Intn(len(feedEventIDs))],
				ActorID:     id,
			}
			resource = persist.ResourceTypeAdmire
			subject = admire.ID
			extraneousID = admire.FeedEventID
			admires = append(admires, admire)
		}
		event := coredb.Event{
			ID:             persist.GenerateID(),
			ActorID:        id,
			ResourceTypeID: resource,
			SubjectID:      subject,
			Action:         action,
		}

		if action == persist.ActionViewedGallery {
			event.GalleryID = subject
		} else if action == persist.ActionUserFollowedUsers {
			event.UserID = subject
		} else if action == persist.ActionCommentedOnFeedEvent {
			event.FeedEventID = extraneousID
			event.CommentID = subject
		} else if action == persist.ActionAdmiredFeedEvent {
			event.FeedEventID = extraneousID
			event.AdmireID = subject
		}

		events = append(events, event)

		notif := coredb.Notification{
			ID:       persist.GenerateID(),
			OwnerID:  ownerID,
			Action:   action,
			EventIds: []persist.DBID{event.ID},
		}
		if action == persist.ActionViewedGallery {
			notif.GalleryID = ownerGalleryID
			notif.Data.AuthedViewerIDs = []persist.DBID{id}
		} else if action == persist.ActionUserFollowedUsers {
			notif.Data.FollowerIDs = []persist.DBID{id}
			randBool := rand.Intn(2) == 1
			notif.Data.FollowedBack = persist.NullBool(randBool)
		} else if action == persist.ActionCommentedOnFeedEvent {
			notif.CommentID = subject
			notif.FeedEventID = extraneousID
		} else if action == persist.ActionAdmiredFeedEvent {
			notif.Data.AdmirerIDs = []persist.DBID{id}
			notif.FeedEventID = extraneousID
		}
		notifs = append(notifs, notif)
	}

	for _, comment := range comments {
		_, err := pg.Exec(ctx, "INSERT INTO comments (id, feed_event_id, actor_id, comment) VALUES ($1, $2, $3, $4)", comment.ID, comment.FeedEventID, comment.ActorID, comment.Comment)
		if err != nil {
			panic(err)
		}
	}

	for _, admire := range admires {
		_, err := pg.Exec(ctx, "INSERT INTO admires (id, feed_event_id, actor_id) VALUES ($1, $2, $3) ON CONFLICT (ACTOR_ID, FEED_EVENT_ID) WHERE DELETED = false DO UPDATE SET ID = $1;", admire.ID, admire.FeedEventID, admire.ActorID)
		if err != nil {
			panic(err)
		}
	}

	for _, event := range events {
		if event.Action == persist.ActionViewedGallery {
			fmt.Printf("GalleryID %s\n", event.GalleryID)
			_, err := pg.Exec(ctx, "INSERT INTO EVENTS (ID, ACTOR_ID, RESOURCE_TYPE_ID, SUBJECT_ID, GALLERY_ID, ACTION) VALUES ($1, $2, $3, $4, $5, $6)", event.ID, event.ActorID, event.ResourceTypeID, event.SubjectID, event.GalleryID, event.Action)
			if err != nil {
				panic(err)
			}
		} else if event.Action == persist.ActionUserFollowedUsers {
			fmt.Printf("UserID %s\n", event.UserID)
			_, err := pg.Exec(ctx, "INSERT INTO EVENTS (ID, ACTOR_ID, RESOURCE_TYPE_ID, SUBJECT_ID, USER_ID, ACTION) VALUES ($1, $2, $3, $4, $5, $6)", event.ID, event.ActorID, event.ResourceTypeID, event.SubjectID, event.UserID, event.Action)
			if err != nil {
				panic(err)
			}
		} else if event.Action == persist.ActionCommentedOnFeedEvent {
			fmt.Printf("CommentID %s\n", event.CommentID)
			_, err := pg.Exec(ctx, "INSERT INTO EVENTS (ID, ACTOR_ID, RESOURCE_TYPE_ID, SUBJECT_ID, COMMENT_ID, FEED_EVENT_ID, ACTION) VALUES ($1, $2, $3, $4, $5, $6, $7)", event.ID, event.ActorID, event.ResourceTypeID, event.SubjectID, event.CommentID, event.FeedEventID, event.Action)
			if err != nil {
				panic(err)
			}
		} else if event.Action == persist.ActionAdmiredFeedEvent {
			fmt.Printf("AdmireID %s\n", event.AdmireID)
			_, err := pg.Exec(ctx, "INSERT INTO EVENTS (ID, ACTOR_ID, RESOURCE_TYPE_ID, SUBJECT_ID, ADMIRE_ID, FEED_EVENT_ID, ACTION) VALUES ($1, $2, $3, $4, $5, $6, $7)", event.ID, event.ActorID, event.ResourceTypeID, event.SubjectID, event.AdmireID, event.FeedEventID, event.Action)
			if err != nil {
				panic(err)
			}
		} else {
			_, err := pg.Exec(ctx, "INSERT INTO EVENTS (ID, ACTOR_ID, RESOURCE_TYPE_ID, SUBJECT_ID, ACTION) VALUES ($1, $2, $3, $4, $5)", event.ID, event.ActorID, event.ResourceTypeID, event.SubjectID, event.Action)
			if err != nil {
				panic(err)
			}
		}
	}

	for _, notif := range notifs {
		if notif.Action == persist.ActionViewedGallery {
			fmt.Printf("GalleryID %s\n", notif.GalleryID)
			_, err := pg.Exec(ctx, "INSERT INTO NOTIFICATIONS (ID, OWNER_ID, ACTION, GALLERY_ID, DATA, EVENT_IDS) VALUES ($1, $2, $3, $4, $5, $6)", notif.ID, notif.OwnerID, notif.Action, notif.GalleryID, notif.Data, notif.EventIds)
			if err != nil {
				panic(err)
			}
		} else if notif.Action == persist.ActionUserFollowedUsers {
			fmt.Printf("UserID %s\n", notif.Data.FollowerIDs)
			_, err := pg.Exec(ctx, "INSERT INTO NOTIFICATIONS (ID, OWNER_ID, ACTION, DATA, EVENT_IDS) VALUES ($1, $2, $3, $4, $5)", notif.ID, notif.OwnerID, notif.Action, notif.Data, notif.EventIds)
			if err != nil {
				panic(err)
			}
		} else if notif.Action == persist.ActionCommentedOnFeedEvent {
			fmt.Printf("CommentID %s\n", notif.CommentID)
			_, err := pg.Exec(ctx, "INSERT INTO NOTIFICATIONS (ID, OWNER_ID, ACTION, COMMENT_ID, FEED_EVENT_ID, DATA, EVENT_IDS) VALUES ($1, $2, $3, $4, $5, $6, $7)", notif.ID, notif.OwnerID, notif.Action, notif.CommentID, notif.FeedEventID, notif.Data, notif.EventIds)
			if err != nil {
				panic(err)
			}
		} else if notif.Action == persist.ActionAdmiredFeedEvent {
			fmt.Printf("AdmireID %s\n", notif.Data.AdmirerIDs)
			_, err := pg.Exec(ctx, "INSERT INTO NOTIFICATIONS (ID, OWNER_ID, ACTION, FEED_EVENT_ID, DATA, EVENT_IDS) VALUES ($1, $2, $3, $4, $5, $6)", notif.ID, notif.OwnerID, notif.Action, notif.FeedEventID, notif.Data, notif.EventIds)
			if err != nil {
				panic(err)
			}
		} else {
			_, err := pg.Exec(ctx, "INSERT INTO NOTIFICATIONS (ID, OWNER_ID, ACTION, DATA, EVENT_IDS) VALUES ($1, $2, $3, $4, $5)", notif.ID, notif.OwnerID, notif.Action, notif.Data, notif.EventIds)
			if err != nil {
				panic(err)
			}
		}
	}

}

func actionForNum(num int) persist.Action {
	switch num {
	case 0:
		return persist.ActionViewedGallery
	case 1:
		return persist.ActionUserFollowedUsers
	case 2:
		return persist.ActionCommentedOnFeedEvent
	case 3:
		return persist.ActionAdmiredFeedEvent
	default:
		return persist.ActionViewedGallery
	}
}

func setDefaults() {
	viper.SetDefault("POSTGRES_HOST", "0.0.0.0")
	viper.SetDefault("POSTGRES_PORT", 5432)
	viper.SetDefault("POSTGRES_USER", "postgres")
	viper.SetDefault("POSTGRES_PASSWORD", "")
	viper.SetDefault("POSTGRES_DB", "postgres")

	viper.AutomaticEnv()
}
