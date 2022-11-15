package emails

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/jackc/pgx/v4/pgxpool"
	migrate "github.com/mikeydub/go-gallery/db"
	"github.com/mikeydub/go-gallery/db/gen/coredb"
	"github.com/mikeydub/go-gallery/docker"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/service/persist/postgres"
	"github.com/ory/dockertest"
	"github.com/stretchr/testify/assert"
)

var testUser = coredb.User{
	Username:           sql.NullString{String: "test1", Valid: true},
	UsernameIdempotent: sql.NullString{String: "test1", Valid: true},
	Email:              persist.NullString("bc@gallery.so"),
}

var testUser2 = coredb.User{
	Username:           sql.NullString{String: "test2", Valid: true},
	UsernameIdempotent: sql.NullString{String: "test2", Valid: true},
	Email:              persist.NullString("bcc@gallery.so"),
}

var admireNotif coredb.Notification

var followNotif coredb.Notification

var commentNotif coredb.Notification

var viewNotif coredb.Notification

var testGallery coredb.Gallery

var feedEvent coredb.FeedEvent

var comment coredb.Comment

func setupTest(t *testing.T) (*assert.Assertions, *sql.DB, *pgxpool.Pool) {
	setDefaults()
	pg, pgUnpatch := docker.InitPostgres()

	db := postgres.NewClient()
	pgx := postgres.NewPgxClient()
	err := migrate.RunMigration(db, "./db/migrations/core")
	if err != nil {
		t.Fatalf("failed to seed db: %s", err)
	}

	t.Cleanup(func() {
		defer db.Close()
		defer pgUnpatch()
		defer pgx.Close()
		for _, r := range []*dockertest.Resource{pg} {
			if err := r.Close(); err != nil {
				t.Fatalf("could not purge resource: %s", err)
			}
		}
	})

	return assert.New(t), db, pgx
}

func seedNotifications(ctx context.Context, t *testing.T, q *coredb.Queries, repos *postgres.Repositories) {

	email := testUser.Email.String()
	userID, err := repos.UserRepository.Create(ctx, persist.CreateUserInput{Username: testUser.Username.String, Email: &email})
	if err != nil {
		t.Fatalf("failed to create user: %s", err)
	}

	email2 := testUser2.Email.String()
	userID2, err := repos.UserRepository.Create(ctx, persist.CreateUserInput{Username: testUser2.Username.String, Email: &email2})
	if err != nil {
		t.Fatalf("failed to create user: %s", err)
	}

	testUser.ID = userID

	testUser2.ID = userID2

	galleryInsert := persist.GalleryDB{OwnerUserID: userID, Collections: []persist.DBID{}}

	galleryID, err := repos.GalleryRepository.Create(ctx, galleryInsert)
	if err != nil {
		t.Fatalf("failed to create gallery: %s", err)
	}

	galleryInsert.OwnerUserID = userID2

	_, err = repos.GalleryRepository.Create(ctx, galleryInsert)
	if err != nil {
		t.Fatalf("failed to create gallery: %s", err)
	}

	testGallery, err = q.GetGalleryById(ctx, galleryID)
	if err != nil {
		t.Fatalf("failed to get gallery: %s", err)
	}

	event, err := q.CreateCollectionEvent(ctx, coredb.CreateCollectionEventParams{
		ActorID:        persist.DBIDToNullStr(userID),
		Action:         persist.ActionCollectionCreated,
		ResourceTypeID: persist.ResourceTypeCollection,
		CollectionID:   testGallery.Collections[0],
	})

	if err != nil {
		t.Fatalf("failed to create admire event: %s", err)
	}

	feedEvent, err = q.CreateFeedEvent(ctx, coredb.CreateFeedEventParams{
		OwnerID: userID,
		Action:  persist.ActionCollectionCreated,
		Data: persist.FeedEventData{
			CollectionID: testGallery.Collections[0],
		},
		EventTime: time.Now(),
		EventIds:  []persist.DBID{event.ID},
	})

	if err != nil {
		t.Fatalf("failed to create admire event: %s", err)
	}

	seedAdmireNotif(ctx, t, q, userID, userID2)
	seedCommentNotif(ctx, t, q, repos, userID, userID2)
	seedViewNotif(ctx, t, q, repos, userID, userID2)

}

func seedAdmireNotif(ctx context.Context, t *testing.T, q *coredb.Queries, userID persist.DBID, userID2 persist.DBID) {

	admire, err := q.CreateAdmire(ctx, coredb.CreateAdmireParams{
		ActorID:     userID2,
		FeedEventID: feedEvent.ID,
	})

	if err != nil {
		t.Fatalf("failed to create admire event: %s", err)
	}

	admireEvent, err := q.CreateAdmireEvent(ctx, coredb.CreateAdmireEventParams{
		ActorID:        persist.DBIDToNullStr(userID2),
		Action:         persist.ActionAdmiredFeedEvent,
		ResourceTypeID: persist.ResourceTypeAdmire,
		AdmireID:       admire,
		FeedEventID:    feedEvent.ID,
	})

	if err != nil {
		t.Fatalf("failed to create admire event: %s", err)
	}

	admireNotif, err = q.CreateAdmireNotification(ctx, coredb.CreateAdmireNotificationParams{
		OwnerID:     userID,
		Action:      persist.ActionAdmiredFeedEvent,
		EventIds:    []persist.DBID{admireEvent.ID},
		FeedEventID: feedEvent.ID,
		Data: persist.NotificationData{
			AdmirerIDs: []persist.DBID{userID2},
		},
	})

	if err != nil {
		t.Fatalf("failed to create admire event: %s", err)
	}

}

func seedCommentNotif(ctx context.Context, t *testing.T, q *coredb.Queries, repos *postgres.Repositories, userID persist.DBID, userID2 persist.DBID) {

	commentID, err := repos.CommentRepository.CreateComment(ctx, feedEvent.ID, userID2, nil, "test comment")

	if err != nil {
		t.Fatalf("failed to create admire event: %s", err)
	}

	comment, err = q.GetCommentByCommentID(ctx, commentID)

	if err != nil {
		t.Fatalf("failed to create admire event: %s", err)
	}

	commentEvent, err := q.CreateCommentEvent(ctx, coredb.CreateCommentEventParams{
		ActorID:        persist.DBIDToNullStr(userID2),
		Action:         persist.ActionCommentedOnFeedEvent,
		ResourceTypeID: persist.ResourceTypeAdmire,
		CommentID:      commentID,
		FeedEventID:    feedEvent.ID,
	})

	if err != nil {
		t.Fatalf("failed to create admire event: %s", err)
	}

	commentNotif, err = q.CreateCommentNotification(ctx, coredb.CreateCommentNotificationParams{
		OwnerID:     userID,
		Action:      persist.ActionCommentedOnFeedEvent,
		EventIds:    []persist.DBID{commentEvent.ID},
		FeedEventID: feedEvent.ID,
		CommentID:   commentID,
	})

	if err != nil {
		t.Fatalf("failed to create admire event: %s", err)
	}

}

func seedViewNotif(ctx context.Context, t *testing.T, q *coredb.Queries, repos *postgres.Repositories, userID persist.DBID, userID2 persist.DBID) {

	viewEvent, err := q.CreateGalleryEvent(ctx, coredb.CreateGalleryEventParams{
		ActorID:        persist.DBIDToNullStr(userID2),
		Action:         persist.ActionViewedGallery,
		ResourceTypeID: persist.ResourceTypeAdmire,
		GalleryID:      testGallery.ID,
	})

	if err != nil {
		t.Fatalf("failed to create admire event: %s", err)
	}

	viewNotif, err = q.CreateViewGalleryNotification(ctx, coredb.CreateViewGalleryNotificationParams{
		OwnerID:  userID,
		Action:   persist.ActionViewedGallery,
		EventIds: []persist.DBID{viewEvent.ID},
		Data: persist.NotificationData{
			AuthedViewerIDs: []persist.DBID{userID2},
		},
		GalleryID: testGallery.ID,
	})

	if err != nil {
		t.Fatalf("failed to create admire event: %s", err)
	}

}

func seedFollowNotif(ctx context.Context, t *testing.T, q *coredb.Queries, repos *postgres.Repositories, userID persist.DBID, userID2 persist.DBID) {

	viewEvent, err := q.CreateUserEvent(ctx, coredb.CreateUserEventParams{
		ActorID:        persist.DBIDToNullStr(userID2),
		Action:         persist.ActionAdmiredFeedEvent,
		ResourceTypeID: persist.ResourceTypeAdmire,
		UserID:         userID,
	})

	if err != nil {
		t.Fatalf("failed to create admire event: %s", err)
	}

	followNotif, err = q.CreateFollowNotification(ctx, coredb.CreateFollowNotificationParams{
		OwnerID:  userID,
		Action:   persist.ActionAdmiredFeedEvent,
		EventIds: []persist.DBID{viewEvent.ID},
		Data: persist.NotificationData{
			FollowerIDs: []persist.DBID{userID2},
		},
	})

	if err != nil {
		t.Fatalf("failed to create admire event: %s", err)
	}

}
