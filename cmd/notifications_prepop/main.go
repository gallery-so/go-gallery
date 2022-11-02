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

	aFewUsers, err := pg.Query(ctx, "SELECT ID FROM USERS LIMIT 10")
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

	aFewGalleries, err := pg.Query(ctx, "SELECT ID FROM GALLERIES LIMIT 10")
	if err != nil {
		panic(err)
	}

	galleryIDs := make([]persist.DBID, 0)
	for aFewGalleries.Next() {
		var id persist.DBID
		err := aFewGalleries.Scan(&id)
		if err != nil {
			panic(err)
		}
		fmt.Println(id)
		galleryIDs = append(galleryIDs, id)
	}

	notifs := make([]coredb.Notification, 0, len(userIDs))
	events := make([]coredb.Event, 0, len(userIDs))
	for _, id := range userIDs {
		action, actionID := actionForNum(rand.Intn(4), userIDs, galleryIDs)

		var resource persist.ResourceType
		switch action {
		case persist.ActionViewedGallery:
			resource = persist.ResourceTypeGallery
		case persist.ActionUserFollowedUsers:
			resource = persist.ResourceTypeUser
		}
		event := coredb.Event{
			ID:             persist.GenerateID(),
			ActorID:        id,
			ResourceTypeID: resource,
			SubjectID:      actionID,
			Action:         action,
		}

		if action == persist.ActionViewedGallery {
			event.GalleryID = actionID
		} else if action == persist.ActionUserFollowedUsers {
			event.UserID = ownerID
		}

		events = append(events, event)

		notif := coredb.Notification{
			ID:       persist.GenerateID(),
			OwnerID:  ownerID,
			Action:   action,
			EventIds: []persist.DBID{event.ID},
		}
		if action == persist.ActionViewedGallery {
			notif.GalleryID = actionID
			notif.Data.AuthedViewerIDs = []persist.DBID{id}
		} else if action == persist.ActionUserFollowedUsers {
			notif.Data.FollowerIDs = []persist.DBID{id}
		}
		notifs = append(notifs, notif)
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
		}
	}

}

func actionForNum(num int, userIDs, galleryIDs []persist.DBID) (persist.Action, persist.DBID) {
	switch num {
	case 0:
		return persist.ActionViewedGallery, galleryIDs[rand.Intn(len(galleryIDs))]
	case 1:
		return persist.ActionUserFollowedUsers, userIDs[rand.Intn(len(userIDs))]
	default:
		return persist.ActionViewedGallery, galleryIDs[rand.Intn(len(galleryIDs))]
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
