package publicapi

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/go-playground/validator/v10"
	"github.com/jackc/pgx/v4"

	db "github.com/mikeydub/go-gallery/db/gen/coredb"
	"github.com/mikeydub/go-gallery/event"
	"github.com/mikeydub/go-gallery/graphql/dataloader"
	"github.com/mikeydub/go-gallery/graphql/model"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/service/persist/postgres"
	"github.com/mikeydub/go-gallery/util"
	"github.com/mikeydub/go-gallery/validate"
)

var ErrOnlyRemoveOwnAdmire = errors.New("only the actor who created the admire can remove it")
var ErrOnlyRemoveOwnComment = errors.New("only the actor who created the comment can remove it")

type interactionType int

const (
	interactionTypeAdmire interactionType = iota + 1
	interactionTypeComment
)

type InteractionAPI struct {
	repos     *postgres.Repositories
	queries   *db.Queries
	loaders   *dataloader.Loaders
	validator *validator.Validate
	ethClient *ethclient.Client
}

type InteractionKey struct {
	Tag int32
	ID  persist.DBID
}

func (api InteractionAPI) validateInteractionParams(id persist.DBID, first *int, last *int, idField string) error {
	// Validate
	if err := validate.ValidateFields(api.validator, validate.ValidationMap{
		idField: validate.WithTag(id, "required"),
	}); err != nil {
		return err
	}

	if err := validatePaginationParams(api.validator, first, last); err != nil {
		return err
	}

	return nil
}

func (api InteractionAPI) loadInteractions(orderedKeys []InteractionKey,
	typeToIDs map[int32][]persist.DBID, tags map[interactionType]int32) ([]interface{}, error) {
	var interactions []interface{}
	interactionsByID := make(map[persist.DBID]interface{})
	var interactionsByIDMutex sync.Mutex
	var wg sync.WaitGroup

	admireIDs := typeToIDs[tags[interactionTypeAdmire]]
	commentIDs := typeToIDs[tags[interactionTypeComment]]

	if len(admireIDs) > 0 {
		wg.Add(1)
		go func() {
			admires, errs := api.loaders.GetAdmireByAdmireIDBatch.LoadAll(admireIDs)

			interactionsByIDMutex.Lock()
			defer interactionsByIDMutex.Unlock()

			for i, admire := range admires {
				if errs[i] == nil {
					interactionsByID[admire.ID] = admire
				}
			}
			wg.Done()
		}()
	}

	if len(commentIDs) > 0 {
		wg.Add(1)
		go func() {
			comments, errs := api.loaders.GetCommentByCommentIDBatch.LoadAll(commentIDs)

			interactionsByIDMutex.Lock()
			defer interactionsByIDMutex.Unlock()

			for i, comment := range comments {
				if errs[i] == nil {
					interactionsByID[comment.ID] = comment
				}
			}
			wg.Done()
		}()
	}

	wg.Wait()

	for _, key := range orderedKeys {
		if interaction, ok := interactionsByID[key.ID]; ok {
			interactions = append(interactions, interaction)
		}
	}

	return interactions, nil
}
func (api InteractionAPI) PaginateInteractionsByFeedEventID(ctx context.Context, feedEventID persist.DBID, before *string, after *string,
	first *int, last *int) ([]any, PageInfo, error) {

	err := api.validateInteractionParams(feedEventID, first, last, "feedEventID")
	if err != nil {
		return nil, PageInfo{}, err
	}

	tags := map[interactionType]int32{
		interactionTypeComment: 1,
		interactionTypeAdmire:  2,
	}

	queryFunc := func(params intTimeIDPagingParams) ([]db.PaginateInteractionsByFeedEventIDBatchRow, error) {
		return api.loaders.PaginateInteractionsByFeedEventIDBatch.Load(db.PaginateInteractionsByFeedEventIDBatchParams{
			FeedEventID:   feedEventID,
			Limit:         params.Limit,
			CurBeforeTag:  params.CursorBeforeInt,
			CurBeforeTime: params.CursorBeforeTime,
			CurBeforeID:   params.CursorBeforeID,
			CurAfterTag:   params.CursorAfterInt,
			CurAfterTime:  params.CursorAfterTime,
			CurAfterID:    params.CursorAfterID,
			PagingForward: params.PagingForward,
			AdmireTag:     tags[interactionTypeAdmire],
			CommentTag:    tags[interactionTypeComment],
		})
	}

	countFunc := func() (int, error) {
		counts, err := api.loaders.CountInteractionsByFeedEventIDBatch.Load(db.CountInteractionsByFeedEventIDBatchParams{
			FeedEventID: feedEventID,
			AdmireTag:   tags[interactionTypeAdmire],
			CommentTag:  tags[interactionTypeComment],
		})

		total := 0

		for _, count := range counts {
			total += int(count.Count)
		}

		return total, err
	}

	cursorFunc := func(r db.PaginateInteractionsByFeedEventIDBatchRow) (int64, time.Time, persist.DBID, error) {
		return int64(r.Tag), r.CreatedAt, r.ID, nil
	}

	paginator := intTimeIDPaginator[db.PaginateInteractionsByFeedEventIDBatchRow]{
		QueryFunc:  queryFunc,
		CursorFunc: cursorFunc,
		CountFunc:  countFunc,
	}

	results, pageInfo, err := paginator.paginate(before, after, first, last)

	if err != nil {
		return nil, PageInfo{}, err
	}

	orderedKeys := make([]InteractionKey, len(results))
	typeToIDs := make(map[int32][]persist.DBID)

	for i, result := range results {
		orderedKeys[i] = InteractionKey{
			ID:  result.ID,
			Tag: result.Tag,
		}
		typeToIDs[result.Tag] = append(typeToIDs[result.Tag], result.ID)
	}
	interactions, err := api.loadInteractions(orderedKeys, typeToIDs, tags)
	if err != nil {
		return nil, PageInfo{}, err
	}

	return interactions, pageInfo, nil
}

func (api InteractionAPI) PaginateInteractionsByPostID(ctx context.Context, postID persist.DBID, before *string, after *string, first *int, last *int) ([]any, PageInfo, error) {

	err := api.validateInteractionParams(postID, first, last, "postID")
	if err != nil {
		return nil, PageInfo{}, err
	}

	tags := map[interactionType]int32{
		interactionTypeComment: 1,
		interactionTypeAdmire:  2,
	}

	queryFunc := func(params intTimeIDPagingParams) ([]db.PaginateInteractionsByPostIDBatchRow, error) {
		return api.loaders.PaginateInteractionsByPostIDBatch.Load(db.PaginateInteractionsByPostIDBatchParams{
			PostID:        postID,
			Limit:         params.Limit,
			CurBeforeTag:  params.CursorBeforeInt,
			CurBeforeTime: params.CursorBeforeTime,
			CurBeforeID:   params.CursorBeforeID,
			CurAfterTag:   params.CursorAfterInt,
			CurAfterTime:  params.CursorAfterTime,
			CurAfterID:    params.CursorAfterID,
			PagingForward: params.PagingForward,
			AdmireTag:     tags[interactionTypeAdmire],
			CommentTag:    tags[interactionTypeComment],
		})
	}

	countFunc := func() (int, error) {
		counts, err := api.loaders.CountInteractionsByPostIDBatch.Load(db.CountInteractionsByPostIDBatchParams{
			PostID:     postID,
			AdmireTag:  tags[interactionTypeAdmire],
			CommentTag: tags[interactionTypeComment],
		})

		total := 0

		for _, count := range counts {
			total += int(count.Count)
		}

		return total, err
	}

	cursorFunc := func(r db.PaginateInteractionsByPostIDBatchRow) (int64, time.Time, persist.DBID, error) {
		return int64(r.Tag), r.CreatedAt, r.ID, nil
	}

	paginator := intTimeIDPaginator[db.PaginateInteractionsByPostIDBatchRow]{
		QueryFunc:  queryFunc,
		CursorFunc: cursorFunc,
		CountFunc:  countFunc,
	}

	results, pageInfo, err := paginator.paginate(before, after, first, last)

	if err != nil {
		return nil, PageInfo{}, err
	}

	orderedKeys := make([]InteractionKey, len(results))
	typeToIDs := make(map[int32][]persist.DBID)

	for i, result := range results {
		orderedKeys[i] = InteractionKey{
			ID:  result.ID,
			Tag: result.Tag,
		}
		typeToIDs[result.Tag] = append(typeToIDs[result.Tag], result.ID)
	}

	interactions, err := api.loadInteractions(orderedKeys, typeToIDs, tags)
	if err != nil {
		return nil, PageInfo{}, err
	}

	return interactions, pageInfo, nil
}

func (api InteractionAPI) PaginateAdmiresByFeedEventID(ctx context.Context, feedEventID persist.DBID, before *string, after *string,
	first *int, last *int) ([]db.Admire, PageInfo, error) {
	// Validate
	if err := validate.ValidateFields(api.validator, validate.ValidationMap{
		"feedEventID": validate.WithTag(feedEventID, "required"),
	}); err != nil {
		return nil, PageInfo{}, err
	}

	if err := validatePaginationParams(api.validator, first, last); err != nil {
		return nil, PageInfo{}, err
	}

	queryFunc := func(params TimeIDPagingParams) ([]db.Admire, error) {
		return api.loaders.PaginateAdmiresByFeedEventIDBatch.Load(db.PaginateAdmiresByFeedEventIDBatchParams{
			FeedEventID:   feedEventID,
			Limit:         params.Limit,
			CurBeforeTime: params.CursorBeforeTime,
			CurBeforeID:   params.CursorBeforeID,
			CurAfterTime:  params.CursorAfterTime,
			CurAfterID:    params.CursorAfterID,
			PagingForward: params.PagingForward,
		})
	}

	countFunc := func() (int, error) {
		total, err := api.loaders.CountAdmiresByFeedEventIDBatch.Load(feedEventID)
		return int(total), err
	}

	cursorFunc := func(a db.Admire) (time.Time, persist.DBID, error) {
		return a.CreatedAt, a.ID, nil
	}

	paginator := TimeIDPaginator[db.Admire]{
		QueryFunc:  queryFunc,
		CursorFunc: cursorFunc,
		CountFunc:  countFunc,
	}

	return paginator.paginate(before, after, first, last)
}

func (api InteractionAPI) PaginateAdmiresByCommentID(ctx context.Context, commentID persist.DBID, before *string, after *string, first *int, last *int) ([]db.Admire, PageInfo, error) {
	// Validate
	if err := validate.ValidateFields(api.validator, validate.ValidationMap{
		"commentID": validate.WithTag(commentID, "required"),
	}); err != nil {
		return nil, PageInfo{}, err
	}

	if err := validatePaginationParams(api.validator, first, last); err != nil {
		return nil, PageInfo{}, err
	}

	queryFunc := func(params TimeIDPagingParams) ([]db.Admire, error) {
		return api.loaders.PaginateAdmiresByCommentIDBatch.Load(db.PaginateAdmiresByCommentIDBatchParams{
			CommentID:     commentID,
			Limit:         params.Limit,
			CurBeforeTime: params.CursorBeforeTime,
			CurBeforeID:   params.CursorBeforeID,
			CurAfterTime:  params.CursorAfterTime,
			CurAfterID:    params.CursorAfterID,
			PagingForward: params.PagingForward,
		})
	}

	countFunc := func() (int, error) {
		total, err := api.loaders.CountAdmiresByCommentIDBatch.Load(commentID)
		return int(total), err
	}

	cursorFunc := func(a db.Admire) (time.Time, persist.DBID, error) {
		return a.CreatedAt, a.ID, nil
	}

	paginator := TimeIDPaginator[db.Admire]{
		QueryFunc:  queryFunc,
		CursorFunc: cursorFunc,
		CountFunc:  countFunc,
	}

	return paginator.paginate(before, after, first, last)
}

func (api InteractionAPI) PaginateCommentsByFeedEventID(ctx context.Context, feedEventID persist.DBID, before *string, after *string, first *int, last *int) ([]db.Comment, PageInfo, error) {
	// Validate
	if err := validate.ValidateFields(api.validator, validate.ValidationMap{
		"feedEventID": validate.WithTag(feedEventID, "required"),
	}); err != nil {
		return nil, PageInfo{}, err
	}

	if err := validatePaginationParams(api.validator, first, last); err != nil {
		return nil, PageInfo{}, err
	}

	queryFunc := func(params TimeIDPagingParams) ([]db.Comment, error) {
		return api.loaders.PaginateCommentsByFeedEventIDBatch.Load(db.PaginateCommentsByFeedEventIDBatchParams{
			FeedEventID:   feedEventID,
			Limit:         params.Limit,
			CurBeforeTime: params.CursorBeforeTime,
			CurBeforeID:   params.CursorBeforeID,
			CurAfterTime:  params.CursorAfterTime,
			CurAfterID:    params.CursorAfterID,
			PagingForward: params.PagingForward,
		})
	}

	countFunc := func() (int, error) {
		total, err := api.loaders.CountCommentsByFeedEventIDBatch.Load(feedEventID)
		return int(total), err
	}

	cursorFunc := func(c db.Comment) (time.Time, persist.DBID, error) {
		return c.CreatedAt, c.ID, nil
	}

	paginator := TimeIDPaginator[db.Comment]{
		QueryFunc:  queryFunc,
		CursorFunc: cursorFunc,
		CountFunc:  countFunc,
	}

	return paginator.paginate(before, after, first, last)
}

func (api InteractionAPI) PaginateRepliesByCommentID(ctx context.Context, commentID persist.DBID, before *string, after *string, first *int, last *int) ([]db.Comment, PageInfo, error) {
	// Validate
	if err := validate.ValidateFields(api.validator, validate.ValidationMap{
		"commentID": validate.WithTag(commentID, "required"),
	}); err != nil {
		return nil, PageInfo{}, err
	}

	if err := validatePaginationParams(api.validator, first, last); err != nil {
		return nil, PageInfo{}, err
	}

	queryFunc := func(params TimeIDPagingParams) ([]db.Comment, error) {
		return api.loaders.PaginateRepliesByCommentIDBatch.Load(db.PaginateRepliesByCommentIDBatchParams{
			CommentID:     commentID,
			Limit:         params.Limit,
			CurBeforeTime: params.CursorBeforeTime,
			CurBeforeID:   params.CursorBeforeID,
			CurAfterTime:  params.CursorAfterTime,
			CurAfterID:    params.CursorAfterID,
			PagingForward: params.PagingForward,
		})
	}

	countFunc := func() (int, error) {
		total, err := api.loaders.CountRepliesByCommentIDBatch.Load(commentID)
		return int(total), err
	}

	cursorFunc := func(c db.Comment) (time.Time, persist.DBID, error) {
		return c.CreatedAt, c.ID, nil
	}

	paginator := TimeIDPaginator[db.Comment]{
		QueryFunc:  queryFunc,
		CursorFunc: cursorFunc,
		CountFunc:  countFunc,
	}

	return paginator.paginate(before, after, first, last)
}

func (api InteractionAPI) GetTotalCommentsByPostID(ctx context.Context, postID persist.DBID) (*int, error) {
	// Validate
	if err := validate.ValidateFields(api.validator, validate.ValidationMap{
		"postID": validate.WithTag(postID, "required"),
	}); err != nil {
		return nil, err
	}

	count, err := api.queries.CountCommentsAndRepliesByPostID(ctx, postID)
	if err != nil {
		return nil, err
	}

	return util.ToPointer(int(count)), nil
}

func (api InteractionAPI) GetTotalCommentsByFeedEventID(ctx context.Context, feedEventID persist.DBID) (*int, error) {
	// Validate
	if err := validate.ValidateFields(api.validator, validate.ValidationMap{
		"feedEventID": validate.WithTag(feedEventID, "required"),
	}); err != nil {
		return nil, err
	}

	count, err := api.queries.CountCommentsAndRepliesByFeedEventID(ctx, feedEventID)
	if err != nil {
		return nil, err
	}

	return util.ToPointer(int(count)), nil
}

func (api InteractionAPI) PaginateAdmiresByPostID(ctx context.Context, postID persist.DBID, before *string, after *string,
	first *int, last *int) ([]db.Admire, PageInfo, error) {
	// Validate
	if err := validate.ValidateFields(api.validator, validate.ValidationMap{
		"feedEventID": validate.WithTag(postID, "required"),
	}); err != nil {
		return nil, PageInfo{}, err
	}

	if err := validatePaginationParams(api.validator, first, last); err != nil {
		return nil, PageInfo{}, err
	}

	queryFunc := func(params TimeIDPagingParams) ([]db.Admire, error) {
		return api.loaders.PaginateAdmiresByPostIDBatch.Load(db.PaginateAdmiresByPostIDBatchParams{
			PostID:        postID,
			Limit:         params.Limit,
			CurBeforeTime: params.CursorBeforeTime,
			CurBeforeID:   params.CursorBeforeID,
			CurAfterTime:  params.CursorAfterTime,
			CurAfterID:    params.CursorAfterID,
			PagingForward: params.PagingForward,
		})
	}

	countFunc := func() (int, error) {
		total, err := api.loaders.CountAdmiresByPostIDBatch.Load(postID)
		return int(total), err
	}

	cursorFunc := func(a db.Admire) (time.Time, persist.DBID, error) {
		return a.CreatedAt, a.ID, nil
	}

	paginator := TimeIDPaginator[db.Admire]{
		QueryFunc:  queryFunc,
		CursorFunc: cursorFunc,
		CountFunc:  countFunc,
	}

	return paginator.paginate(before, after, first, last)
}

func (api InteractionAPI) PaginateAdmiresByTokenID(ctx context.Context, tokenID persist.DBID, before *string, after *string,
	first *int, last *int, userID *persist.DBID) ([]db.Admire, PageInfo, error) {
	// Validate
	if err := validate.ValidateFields(api.validator, validate.ValidationMap{
		"tokenID": validate.WithTag(tokenID, "required"),
	}); err != nil {
		return nil, PageInfo{}, err
	}

	if err := validatePaginationParams(api.validator, first, last); err != nil {
		return nil, PageInfo{}, err
	}

	var actorID persist.DBID
	if userID != nil {
		actorID = *userID
	} else {
		actorID = ""
	}
	onlyForActor := actorID != ""

	queryFunc := func(params TimeIDPagingParams) ([]db.Admire, error) {
		return api.loaders.PaginateAdmiresByTokenIDBatch.Load(db.PaginateAdmiresByTokenIDBatchParams{
			TokenID:       tokenID,
			Limit:         params.Limit,
			OnlyForActor:  onlyForActor,
			ActorID:       actorID,
			CurBeforeTime: params.CursorBeforeTime,
			CurBeforeID:   params.CursorBeforeID,
			CurAfterTime:  params.CursorAfterTime,
			CurAfterID:    params.CursorAfterID,
			PagingForward: params.PagingForward,
		})
	}

	countFunc := func() (int, error) {
		total, err := api.loaders.CountAdmiresByTokenIDBatch.Load(tokenID)
		return int(total), err
	}

	cursorFunc := func(a db.Admire) (time.Time, persist.DBID, error) {
		return a.CreatedAt, a.ID, nil
	}

	paginator := TimeIDPaginator[db.Admire]{
		QueryFunc:  queryFunc,
		CursorFunc: cursorFunc,
		CountFunc:  countFunc,
	}

	return paginator.paginate(before, after, first, last)
}

func (api InteractionAPI) PaginateCommentsByPostID(ctx context.Context, postID persist.DBID, before *string, after *string, first *int, last *int) ([]db.Comment, PageInfo, error) {
	// Validate
	if err := validate.ValidateFields(api.validator, validate.ValidationMap{
		"feedEventID": validate.WithTag(postID, "required"),
	}); err != nil {
		return nil, PageInfo{}, err
	}

	if err := validatePaginationParams(api.validator, first, last); err != nil {
		return nil, PageInfo{}, err
	}

	queryFunc := func(params TimeIDPagingParams) ([]db.Comment, error) {
		return api.loaders.PaginateCommentsByPostIDBatch.Load(db.PaginateCommentsByPostIDBatchParams{
			PostID:        postID,
			Limit:         params.Limit,
			CurBeforeTime: params.CursorBeforeTime,
			CurBeforeID:   params.CursorBeforeID,
			CurAfterTime:  params.CursorAfterTime,
			CurAfterID:    params.CursorAfterID,
			PagingForward: params.PagingForward,
		})
	}

	countFunc := func() (int, error) {
		total, err := api.loaders.CountCommentsByPostIDBatch.Load(postID)
		return int(total), err
	}

	cursorFunc := func(c db.Comment) (time.Time, persist.DBID, error) {
		return c.CreatedAt, c.ID, nil
	}

	paginator := TimeIDPaginator[db.Comment]{
		QueryFunc:  queryFunc,
		CursorFunc: cursorFunc,
		CountFunc:  countFunc,
	}

	return paginator.paginate(before, after, first, last)
}

func (api InteractionAPI) GetAdmireByActorIDAndFeedEventID(ctx context.Context, actorID persist.DBID, feedEventID persist.DBID) (*db.Admire, error) {
	// Validate
	if err := validate.ValidateFields(api.validator, validate.ValidationMap{
		"actorID":     validate.WithTag(actorID, "required"),
		"feedEventID": validate.WithTag(feedEventID, "required"),
	}); err != nil {
		return nil, err
	}

	admire, err := api.loaders.GetAdmireByActorIDAndFeedEventID.Load(db.GetAdmireByActorIDAndFeedEventIDParams{
		ActorID:     actorID,
		FeedEventID: feedEventID,
	})

	if err != nil {
		return nil, err
	}

	return &admire, nil
}

func (api InteractionAPI) GetAdmireByActorIDAndPostID(ctx context.Context, actorID persist.DBID, postID persist.DBID) (*db.Admire, error) {
	// Validate
	if err := validate.ValidateFields(api.validator, validate.ValidationMap{
		"actorID":     validate.WithTag(actorID, "required"),
		"feedEventID": validate.WithTag(postID, "required"),
	}); err != nil {
		return nil, err
	}

	admire, err := api.loaders.GetAdmireByActorIDAndPostID.Load(db.GetAdmireByActorIDAndPostIDParams{
		ActorID: actorID,
		PostID:  postID,
	})

	if err != nil {
		return nil, err
	}

	return &admire, nil
}

func (api InteractionAPI) GetAdmireByActorIDAndTokenID(ctx context.Context, actorID persist.DBID, tokenID persist.DBID) (*db.Admire, error) {
	// Validate
	if err := validate.ValidateFields(api.validator, validate.ValidationMap{
		"actorID": validate.WithTag(actorID, "required"),
		"tokenID": validate.WithTag(tokenID, "required"),
	}); err != nil {
		return nil, err
	}

	admire, err := api.loaders.GetAdmireByActorIDAndTokenID.Load(db.GetAdmireByActorIDAndTokenIDParams{
		ActorID: actorID,
		TokenID: tokenID,
	})

	if err != nil {
		return nil, err
	}

	return &admire, nil
}

func (api InteractionAPI) GetAdmireByActorIDAndCommentID(ctx context.Context, actorID persist.DBID, commentID persist.DBID) (*db.Admire, error) {
	// Validate
	if err := validate.ValidateFields(api.validator, validate.ValidationMap{
		"actorID":   validate.WithTag(actorID, "required"),
		"commentID": validate.WithTag(commentID, "required"),
	}); err != nil {
		return nil, err
	}

	admire, err := api.loaders.GetAdmireByActorIDAndCommentID.Load(db.GetAdmireByActorIDAndCommentIDParams{
		ActorID:   actorID,
		CommentID: commentID,
	})

	if err != nil {
		return nil, err
	}

	return &admire, nil
}

func (api InteractionAPI) GetAdmireByID(ctx context.Context, admireID persist.DBID) (*db.Admire, error) {
	// Validate
	if err := validate.ValidateFields(api.validator, validate.ValidationMap{
		"admireID": validate.WithTag(admireID, "required"),
	}); err != nil {
		return nil, err
	}

	admire, err := api.loaders.GetAdmireByAdmireIDBatch.Load(admireID)
	if err != nil {
		return nil, err
	}

	return &admire, nil
}

func (api InteractionAPI) AdmireFeedEvent(ctx context.Context, feedEventID persist.DBID) (persist.DBID, error) {
	// Validate
	userID, err := getAuthenticatedUserID(ctx)
	if err != nil {
		return "", err
	}
	_, err = For(ctx).Feed.GetFeedEventById(ctx, feedEventID)
	if err != nil {
		return "", err
	}

	params := db.CreateFeedEventAdmireParams{
		ID:          persist.GenerateID(),
		FeedEventID: feedEventID,
		ActorID:     userID,
	}
	admireID, err := api.queries.CreateFeedEventAdmire(ctx, params)
	if err != nil {
		return "", err
	}

	// Admire did not exist before, so dispatch event
	if admireID == params.ID {
		err = event.Dispatch(ctx, db.Event{
			ActorID:        persist.DBIDToNullStr(userID),
			ResourceTypeID: persist.ResourceTypeAdmire,
			SubjectID:      feedEventID,
			FeedEventID:    feedEventID,
			AdmireID:       admireID,
			Action:         persist.ActionAdmiredFeedEvent,
		})
	}

	return admireID, err
}

func (api InteractionAPI) AdmireToken(ctx context.Context, tokenID persist.DBID) (persist.DBID, error) {
	// Validate
	userID, err := getAuthenticatedUserID(ctx)
	if err != nil {
		return "", err
	}
	_, err = For(ctx).Token.GetTokenById(ctx, tokenID)
	if err != nil {
		return "", err
	}

	params := db.CreateTokenAdmireParams{
		ID:      persist.GenerateID(),
		TokenID: tokenID,
		ActorID: userID,
	}
	admireID, err := api.queries.CreateTokenAdmire(ctx, params)
	if err != nil {
		return "", err
	}

	// Admire did not exist before, so dispatch event
	if admireID == params.ID {
		err = event.Dispatch(ctx, db.Event{
			ActorID:        persist.DBIDToNullStr(userID),
			ResourceTypeID: persist.ResourceTypeAdmire,
			SubjectID:      tokenID,
			TokenID:        tokenID,
			AdmireID:       admireID,
			Action:         persist.ActionAdmiredToken,
		})
	}

	return admireID, err
}

func (api InteractionAPI) AdmirePost(ctx context.Context, postID persist.DBID) (persist.DBID, error) {
	// Validate
	userID, err := getAuthenticatedUserID(ctx)
	if err != nil {
		return "", err
	}
	_, err = For(ctx).Feed.GetPostById(ctx, postID)
	if err != nil {
		return "", err
	}

	params := db.CreatePostAdmireParams{
		ID:      persist.GenerateID(),
		PostID:  postID,
		ActorID: userID,
	}

	admireID, err := api.queries.CreatePostAdmire(ctx, params)
	if err != nil {
		return "", err
	}

	// Admire did not exist before, so dispatch event
	if admireID == params.ID {
		err = event.Dispatch(ctx, db.Event{
			ActorID:        persist.DBIDToNullStr(userID),
			ResourceTypeID: persist.ResourceTypeAdmire,
			SubjectID:      postID,
			PostID:         postID,
			AdmireID:       admireID,
			Action:         persist.ActionAdmiredPost,
		})
	}

	return admireID, err
}

func (api InteractionAPI) AdmireComment(ctx context.Context, commentID persist.DBID) (persist.DBID, error) {
	// Validate
	userID, err := getAuthenticatedUserID(ctx)
	if err != nil {
		return "", err
	}
	_, err = api.GetCommentByID(ctx, commentID)
	if err != nil {
		return "", err
	}

	params := db.CreateCommentAdmireParams{
		ID:        persist.GenerateID(),
		CommentID: commentID,
		ActorID:   userID,
	}
	admireID, err := api.queries.CreateCommentAdmire(ctx, params)
	if err != nil {
		return "", err
	}

	// Admire did not exist before, so dispatch event
	if admireID == params.ID {
		err = event.Dispatch(ctx, db.Event{
			ActorID:        persist.DBIDToNullStr(userID),
			ResourceTypeID: persist.ResourceTypeAdmire,
			SubjectID:      commentID,
			CommentID:      commentID,
			AdmireID:       admireID,
			Action:         persist.ActionAdmiredComment,
		})
	}

	return admireID, err
}

func (api InteractionAPI) RemoveAdmire(ctx context.Context, admireID persist.DBID) (persist.DBID, persist.DBID, error) {
	// Validate
	if err := validate.ValidateFields(api.validator, validate.ValidationMap{
		"admireID": validate.WithTag(admireID, "required"),
	}); err != nil {
		return "", "", err
	}

	// will also fail if admire does not exist
	admire, err := api.GetAdmireByID(ctx, admireID)
	if err != nil {
		return "", "", err
	}
	if admire.ActorID != For(ctx).User.GetLoggedInUserId(ctx) {
		return "", "", ErrOnlyRemoveOwnAdmire
	}

	return admire.FeedEventID, admire.PostID, api.queries.DeleteAdmireByID(ctx, admireID)
}

func (api InteractionAPI) GetCommentByID(ctx context.Context, commentID persist.DBID) (*db.Comment, error) {
	// Validate
	if err := validate.ValidateFields(api.validator, validate.ValidationMap{
		"commentID": validate.WithTag(commentID, "required"),
	}); err != nil {
		return nil, err
	}

	comment, err := api.loaders.GetCommentByCommentIDBatch.Load(commentID)
	if err != nil {
		return nil, err
	}

	return &comment, nil
}

func (api InteractionAPI) CommentOnFeedEvent(ctx context.Context, feedEventID persist.DBID, replyToID *persist.DBID, mentions []*model.MentionInput, comment string) (persist.DBID, error) {
	// Trim whitespace first, so comments consisting only of whitespace will fail
	// the "required" validation below
	comment = strings.TrimSpace(comment)

	// Validate
	if err := validate.ValidateFields(api.validator, validate.ValidationMap{
		"feedEventID": validate.WithTag(feedEventID, "required"),
		"comment":     validate.WithTag(comment, "required"),
	}); err != nil {
		return "", err
	}

	return api.comment(ctx, comment, feedEventID, "", replyToID, mentions)
}

func (api InteractionAPI) CommentOnPost(ctx context.Context, postID persist.DBID, replyToID *persist.DBID, mentions []*model.MentionInput, comment string) (persist.DBID, error) {
	// Trim whitespace first, so comments consisting only of whitespace will fail
	// the "required" validation below
	comment = strings.TrimSpace(comment)

	// Validate
	if err := validate.ValidateFields(api.validator, validate.ValidationMap{
		"postID":  validate.WithTag(postID, "required"),
		"comment": validate.WithTag(comment, "required"),
	}); err != nil {
		return "", err
	}

	return api.comment(ctx, comment, "", postID, replyToID, mentions)
}

func (api InteractionAPI) comment(ctx context.Context, comment string, feedEventID, postID persist.DBID, replyToID *persist.DBID, mentions []*model.MentionInput) (persist.DBID, error) {
	actor, err := getAuthenticatedUserID(ctx)
	if err != nil {
		return "", err
	}

	comment = validate.SanitizationPolicy.Sanitize(comment)

	dbMentions, err := mentionInputsToMentions(ctx, mentions, api.queries)
	if err != nil {
		return "", err
	}

	commentID, resultMentions, err := api.repos.CommentRepository.CreateComment(ctx, feedEventID, postID, actor, replyToID, comment, dbMentions)
	if err != nil {
		return "", err
	}
	var action persist.Action
	var feedEntityOwner persist.DBID
	if feedEventID != "" {
		action = persist.ActionCommentedOnFeedEvent
		f, err := api.queries.GetFeedEventByID(ctx, feedEventID)
		if err != nil {
			return "", err
		}
		feedEntityOwner = f.OwnerID
	} else if postID != "" {
		action = persist.ActionCommentedOnPost
		p, err := api.queries.GetPostByID(ctx, postID)
		if err != nil {
			return "", err
		}
		feedEntityOwner = p.ActorID
	} else {
		panic("commenting on neither feed event nor post")
	}

	var replyToUser *persist.DBID
	if replyToID != nil {
		err = event.Dispatch(ctx, db.Event{
			ActorID:        persist.DBIDToNullStr(actor),
			ResourceTypeID: persist.ResourceTypeComment,
			SubjectID:      *replyToID,
			PostID:         postID,
			FeedEventID:    feedEventID,
			CommentID:      commentID,
			Action:         persist.ActionReplyToComment,
		})
		if err != nil {
			return "", err
		}

		replyToComment, err := api.GetCommentByID(ctx, *replyToID)
		if err != nil {
			return "", err
		}

		replyToUser = &replyToComment.ActorID
	}

	if replyToUser == nil || *replyToUser != feedEntityOwner {
		err = event.Dispatch(ctx, db.Event{
			ActorID:        persist.DBIDToNullStr(actor),
			ResourceTypeID: persist.ResourceTypeComment,
			SubjectID:      persist.DBID(util.FirstNonEmptyString(postID.String(), feedEventID.String())),
			PostID:         postID,
			FeedEventID:    feedEventID,
			CommentID:      commentID,
			Action:         action,
		})
		if err != nil {
			return "", err
		}
	}

	if len(mentions) > 0 {
		for _, mention := range resultMentions {

			if replyToUser != nil && mention.UserID == *replyToUser {
				continue
			}

			switch {
			case mention.UserID != "":
				err = event.Dispatch(ctx, db.Event{
					ActorID:        persist.DBIDToNullStr(actor),
					ResourceTypeID: persist.ResourceTypeUser,
					SubjectID:      mention.UserID,
					PostID:         postID,
					FeedEventID:    feedEventID,
					UserID:         mention.UserID,
					CommentID:      commentID,
					MentionID:      mention.ID,
					Action:         persist.ActionMentionUser,
				})
				if err != nil {
					return "", err
				}
			case mention.CommunityID != "":
				err = event.Dispatch(ctx, db.Event{
					ActorID:        persist.DBIDToNullStr(actor),
					ResourceTypeID: persist.ResourceTypeCommunity,
					SubjectID:      mention.CommunityID,
					PostID:         postID,
					FeedEventID:    feedEventID,
					CommunityID:    mention.CommunityID,
					CommentID:      commentID,
					MentionID:      mention.ID,
					Action:         persist.ActionMentionCommunity,
				})
				if err != nil {
					return "", err
				}
			default:
				return "", fmt.Errorf("invalid mention type: %+v", mention)
			}
		}
	}

	return commentID, nil
}

func (api InteractionAPI) RemoveComment(ctx context.Context, commentID persist.DBID) (persist.DBID, persist.DBID, error) {
	// Validate
	if err := validate.ValidateFields(api.validator, validate.ValidationMap{
		"commentID": validate.WithTag(commentID, "required"),
	}); err != nil {
		return "", "", err
	}
	comment, err := api.GetCommentByID(ctx, commentID)
	if err != nil {
		return "", "", err
	}
	if comment.ActorID != For(ctx).User.GetLoggedInUserId(ctx) {
		return "", "", ErrOnlyRemoveOwnComment
	}

	return comment.FeedEventID, comment.PostID, api.queries.RemoveComment(ctx, commentID)
}

func (api InteractionAPI) GetMentionsByCommentID(ctx context.Context, commentID persist.DBID) ([]db.Mention, error) {
	// Validate
	if err := validate.ValidateFields(api.validator, validate.ValidationMap{
		"commentID": validate.WithTag(commentID, "required"),
	}); err != nil {
		return nil, err
	}

	return api.loaders.GetMentionsByCommentID.Load(commentID)
}

func (api InteractionAPI) GetMentionsByPostID(ctx context.Context, postID persist.DBID) ([]db.Mention, error) {
	// Validate
	if err := validate.ValidateFields(api.validator, validate.ValidationMap{
		"postID": validate.WithTag(postID, "required"),
	}); err != nil {
		return nil, err
	}

	return api.loaders.GetMentionsByPostID.Load(postID)
}

func (api InteractionAPI) ReportPost(ctx context.Context, postID persist.DBID, reason persist.ReportReason) error {
	// Validate
	if err := validate.ValidateFields(api.validator, validate.ValidationMap{
		"postID": validate.WithTag(postID, "required"),
		"reason": validate.WithTag(reason, "required"),
	}); err != nil {
		return err
	}
	userID, _ := getAuthenticatedUserID(ctx)
	_, err := api.queries.ReportPost(ctx, db.ReportPostParams{
		ID:       persist.GenerateID(),
		PostID:   postID,
		Reporter: util.ToNullString(userID.String(), true),
		Reason:   reason,
	})
	if err != nil && errors.Is(err, pgx.ErrNoRows) {
		return persist.ErrPostNotFoundByID{ID: postID}
	}
	return err
}

func mentionInputsToMentions(ctx context.Context, ms []*model.MentionInput, queries *db.Queries) ([]db.Mention, error) {
	res := make([]db.Mention, len(ms))

	for i, m := range ms {
		if m.CommunityID != nil && m.UserID != nil {
			return nil, fmt.Errorf("mention input cannot have both communityID and userID set")
		}
		mention := db.Mention{}
		if m.Interval != nil {
			mention.Length = sql.NullInt32{Int32: int32(m.Interval.Length), Valid: true}
			mention.Start = sql.NullInt32{Int32: int32(m.Interval.Start), Valid: true}

		}
		if m.CommunityID != nil {
			if c, err := queries.GetCommunityByID(ctx, *m.CommunityID); c.ID == "" || err != nil {
				return nil, fmt.Errorf("could retrieve community: %s (%s)", *m.CommunityID, err)
			}
			mention.CommunityID = *m.CommunityID
		}
		if m.UserID != nil {
			if u, err := queries.GetUserById(ctx, *m.UserID); u.ID == "" || err != nil {
				return nil, fmt.Errorf("could retrieve user: %s (%s)", *m.UserID, err)
			}
			mention.UserID = *m.UserID
		}

		res[i] = mention
	}

	return res, nil
}
