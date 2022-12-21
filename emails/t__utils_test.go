package emails

import (
	"context"
	"database/sql"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v4/pgxpool"
	migrate "github.com/mikeydub/go-gallery/db"
	"github.com/mikeydub/go-gallery/db/gen/coredb"
	"github.com/mikeydub/go-gallery/docker"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/service/persist/postgres"
	"github.com/stretchr/testify/assert"
)

var testUser = coredb.UsersWithPii{
	Username:           sql.NullString{String: "test1", Valid: true},
	UsernameIdempotent: sql.NullString{String: "test1", Valid: true},
	PiiEmailAddress:    persist.Email("bc@gallery.so"),
}

var testUser2 = coredb.UsersWithPii{
	Username:           sql.NullString{String: "test2", Valid: true},
	UsernameIdempotent: sql.NullString{String: "test2", Valid: true},
	PiiEmailAddress:    persist.Email("bcc@gallery.so"),
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
	r, err := docker.StartPostgres()
	if err != nil {
		t.Fatal(err)
	}

	hostAndPort := strings.Split(r.GetHostPort("5432/tcp"), ":")
	t.Setenv("POSTGRES_HOST", hostAndPort[0])
	t.Setenv("POSTGRES_PORT", hostAndPort[1])
	t.Cleanup(func() { r.Close() })

	db := postgres.NewClient()
	pgx := postgres.NewPgxClient()
	err = migrate.RunMigration(db, "./db/migrations/core")
	if err != nil {
		t.Fatal(err)
	}

	seedNotifications(context.Background(), t, coredb.New(pgx), newRepos(db, pgx))

	return assert.New(t), db, pgx
}

func newRepos(pq *sql.DB, pgx *pgxpool.Pool) *postgres.Repositories {
	queries := coredb.New(pgx)

	return &postgres.Repositories{
		UserRepository:        postgres.NewUserRepository(pq, queries),
		NonceRepository:       postgres.NewNonceRepository(pq, queries),
		TokenRepository:       postgres.NewTokenGalleryRepository(pq, queries),
		CollectionRepository:  postgres.NewCollectionTokenRepository(pq, queries),
		GalleryRepository:     postgres.NewGalleryRepository(queries),
		ContractRepository:    postgres.NewContractGalleryRepository(pq, queries),
		MembershipRepository:  postgres.NewMembershipRepository(pq, queries),
		EarlyAccessRepository: postgres.NewEarlyAccessRepository(pq, queries),
		WalletRepository:      postgres.NewWalletRepository(pq, queries),
		AdmireRepository:      postgres.NewAdmireRepository(queries),
		CommentRepository:     postgres.NewCommentRepository(pq, queries),
	}
}

func seedNotifications(ctx context.Context, t *testing.T, q *coredb.Queries, repos *postgres.Repositories) {

	email := testUser.PiiEmailAddress
	userID, err := repos.UserRepository.Create(ctx, persist.CreateUserInput{Username: testUser.Username.String, Email: &email, ChainAddress: persist.NewChainAddress("0x8914496dc01efcc49a2fa340331fb90969b6f1d2", persist.ChainETH)})
	if err != nil {
		t.Fatalf("failed to create user: %s", err)
	}

	email2 := testUser2.PiiEmailAddress
	userID2, err := repos.UserRepository.Create(ctx, persist.CreateUserInput{Username: testUser2.Username.String, Email: &email2, ChainAddress: persist.NewChainAddress("0x9a3f9764b21adaf3c6fdf6f947e6d3340a3f8ac5", persist.ChainETH)})
	if err != nil {
		t.Fatalf("failed to create user: %s", err)
	}

	testUser.ID = userID

	testUser2.ID = userID2

	collID, err := repos.CollectionRepository.Create(ctx, persist.CollectionDB{
		Name:        "test coll",
		OwnerUserID: userID,
	})

	if err != nil {
		t.Fatalf("failed to create collection: %s", err)
	}

	galleryInsert := persist.GalleryDB{OwnerUserID: userID, Collections: []persist.DBID{collID}}

	galleryID, err := repos.GalleryRepository.Create(ctx, galleryInsert)
	if err != nil {
		t.Fatalf("failed to create gallery: %s", err)
	}

	galleryInsert2 := persist.GalleryDB{OwnerUserID: userID2}

	_, err = repos.GalleryRepository.Create(ctx, galleryInsert2)
	if err != nil {
		t.Fatalf("failed to create gallery: %s", err)
	}

	testGallery, err = q.GetGalleryById(ctx, galleryID)
	if err != nil {
		t.Fatalf("failed to get gallery: %s", err)
	}

	event, err := q.CreateCollectionEvent(ctx, coredb.CreateCollectionEventParams{
		ID:             persist.GenerateID(),
		ActorID:        persist.DBIDToNullStr(userID),
		Action:         persist.ActionCollectionCreated,
		ResourceTypeID: persist.ResourceTypeCollection,
		CollectionID:   testGallery.Collections[0],
	})

	if err != nil {
		t.Fatalf("failed to create admire event: %s", err)
	}

	feedEvent, err = q.CreateFeedEvent(ctx, coredb.CreateFeedEventParams{
		ID:      persist.GenerateID(),
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
	seedFollowNotif(ctx, t, q, repos, userID, userID2)

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
		ID:             persist.GenerateID(),
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
		ID:          persist.GenerateID(),
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
		ID:             persist.GenerateID(),
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
		ID:          persist.GenerateID(),
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
		ID:             persist.GenerateID(),
		ActorID:        persist.DBIDToNullStr(userID2),
		Action:         persist.ActionViewedGallery,
		ResourceTypeID: persist.ResourceTypeAdmire,
		GalleryID:      testGallery.ID,
	})

	if err != nil {
		t.Fatalf("failed to create admire event: %s", err)
	}

	viewNotif, err = q.CreateViewGalleryNotification(ctx, coredb.CreateViewGalleryNotificationParams{
		ID:       persist.GenerateID(),
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
		ID:             persist.GenerateID(),
		ActorID:        persist.DBIDToNullStr(userID2),
		Action:         persist.ActionUserFollowedUsers,
		ResourceTypeID: persist.ResourceTypeAdmire,
		UserID:         userID,
	})

	if err != nil {
		t.Fatalf("failed to create admire event: %s", err)
	}

	followNotif, err = q.CreateFollowNotification(ctx, coredb.CreateFollowNotificationParams{
		ID:       persist.GenerateID(),
		OwnerID:  userID,
		Action:   persist.ActionUserFollowedUsers,
		EventIds: []persist.DBID{viewEvent.ID},
		Data: persist.NotificationData{
			FollowerIDs: []persist.DBID{userID2},
		},
	})

	if err != nil {
		t.Fatalf("failed to create admire event: %s", err)
	}

}
