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
	first *int, last *int) ([]interface{}, PageInfo, error) {

	err := api.validateInteractionParams(feedEventID, first, last, "feedEventID")
	if err != nil {
		return nil, PageInfo{}, err
	}

	tags := map[interactionType]int32{
		interactionTypeComment: 1,
		interactionTypeAdmire:  2,
	}

	queryFunc := func(params intTimeIDPagingParams) ([]interface{}, error) {
		keys, err := api.loaders.PaginateInteractionsByFeedEventIDBatch.Load(db.PaginateInteractionsByFeedEventIDBatchParams{
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

		if err != nil {
			return nil, err
		}

		results := make([]interface{}, len(keys))
		for i, key := range keys {
			results[i] = key
		}

		return results, nil
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

	cursorFunc := func(i interface{}) (int64, time.Time, persist.DBID, error) {
		if row, ok := i.(db.PaginateInteractionsByFeedEventIDBatchRow); ok {
			return int64(row.Tag), row.CreatedAt, row.ID, nil
		}
		return 0, time.Time{}, "", fmt.Errorf("interface{} is not the correct type")
	}

	paginator := intTimeIDPaginator{
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
		row := result.(db.PaginateInteractionsByFeedEventIDBatchRow)
		orderedKeys[i] = InteractionKey{
			ID:  row.ID,
			Tag: row.Tag,
		}
		typeToIDs[row.Tag] = append(typeToIDs[row.Tag], row.ID)
	}
	interactions, err := api.loadInteractions(orderedKeys, typeToIDs, tags)
	if err != nil {
		return nil, PageInfo{}, err
	}

	return interactions, pageInfo, nil
}

func (api InteractionAPI) PaginateInteractionsByPostID(ctx context.Context, postID persist.DBID, before *string, after *string, first *int, last *int) ([]interface{}, PageInfo, error) {

	err := api.validateInteractionParams(postID, first, last, "postID")
	if err != nil {
		return nil, PageInfo{}, err
	}

	tags := map[interactionType]int32{
		interactionTypeComment: 1,
		interactionTypeAdmire:  2,
	}

	queryFunc := func(params intTimeIDPagingParams) ([]interface{}, error) {
		keys, err := api.loaders.PaginateInteractionsByPostIDBatch.Load(db.PaginateInteractionsByPostIDBatchParams{
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

		if err != nil {
			return nil, err
		}

		results := make([]interface{}, len(keys))
		for i, key := range keys {
			results[i] = key
		}

		return results, nil
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

	cursorFunc := func(i interface{}) (int64, time.Time, persist.DBID, error) {
		if row, ok := i.(db.PaginateInteractionsByPostIDBatchRow); ok {
			return int64(row.Tag), row.CreatedAt, row.ID, nil
		}
		return 0, time.Time{}, "", fmt.Errorf("interface{} is not the correct type")
	}

	paginator := intTimeIDPaginator{
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
		row := result.(db.PaginateInteractionsByPostIDBatchRow)
		orderedKeys[i] = InteractionKey{
			ID:  row.ID,
			Tag: row.Tag,
		}
		typeToIDs[row.Tag] = append(typeToIDs[row.Tag], row.ID)
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

	queryFunc := func(params timeIDPagingParams) ([]interface{}, error) {
		admires, err := api.loaders.PaginateAdmiresByFeedEventIDBatch.Load(db.PaginateAdmiresByFeedEventIDBatchParams{
			FeedEventID:   feedEventID,
			Limit:         params.Limit,
			CurBeforeTime: params.CursorBeforeTime,
			CurBeforeID:   params.CursorBeforeID,
			CurAfterTime:  params.CursorAfterTime,
			CurAfterID:    params.CursorAfterID,
			PagingForward: params.PagingForward,
		})

		if err != nil {
			return nil, err
		}

		results := make([]interface{}, len(admires))
		for i, admire := range admires {
			results[i] = admire
		}

		return results, nil
	}

	countFunc := func() (int, error) {
		total, err := api.loaders.CountAdmiresByFeedEventIDBatch.Load(feedEventID)
		return int(total), err
	}

	cursorFunc := func(i interface{}) (time.Time, persist.DBID, error) {
		if admire, ok := i.(db.Admire); ok {
			return admire.CreatedAt, admire.ID, nil
		}
		return time.Time{}, "", fmt.Errorf("interface{} is not an admire")
	}

	paginator := timeIDPaginator{
		QueryFunc:  queryFunc,
		CursorFunc: cursorFunc,
		CountFunc:  countFunc,
	}

	results, pageInfo, err := paginator.paginate(before, after, first, last)

	admires := make([]db.Admire, len(results))
	for i, result := range results {
		admires[i] = result.(db.Admire)
	}

	return admires, pageInfo, err
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

	queryFunc := func(params timeIDPagingParams) ([]interface{}, error) {
		comments, err := api.loaders.PaginateCommentsByFeedEventIDBatch.Load(db.PaginateCommentsByFeedEventIDBatchParams{
			FeedEventID:   feedEventID,
			Limit:         params.Limit,
			CurBeforeTime: params.CursorBeforeTime,
			CurBeforeID:   params.CursorBeforeID,
			CurAfterTime:  params.CursorAfterTime,
			CurAfterID:    params.CursorAfterID,
			PagingForward: params.PagingForward,
		})

		if err != nil {
			return nil, err
		}

		results := make([]interface{}, len(comments))
		for i, comment := range comments {
			results[i] = comment
		}

		return results, nil
	}

	countFunc := func() (int, error) {
		total, err := api.loaders.CountCommentsByFeedEventIDBatch.Load(feedEventID)
		return int(total), err
	}

	cursorFunc := func(i interface{}) (time.Time, persist.DBID, error) {
		if comment, ok := i.(db.Comment); ok {
			return comment.CreatedAt, comment.ID, nil
		}
		return time.Time{}, "", fmt.Errorf("interface{} is not an comment")
	}

	paginator := timeIDPaginator{
		QueryFunc:  queryFunc,
		CursorFunc: cursorFunc,
		CountFunc:  countFunc,
	}

	results, pageInfo, err := paginator.paginate(before, after, first, last)

	comments := make([]db.Comment, len(results))
	for i, result := range results {
		comments[i] = result.(db.Comment)
	}

	return comments, pageInfo, err
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

	queryFunc := func(params timeIDPagingParams) ([]interface{}, error) {

		comments, err := api.loaders.PaginateRepliesByCommentIDBatch.Load(db.PaginateRepliesByCommentIDBatchParams{
			CommentID:     commentID,
			Limit:         params.Limit,
			CurBeforeTime: params.CursorBeforeTime,
			CurBeforeID:   params.CursorBeforeID,
			CurAfterTime:  params.CursorAfterTime,
			CurAfterID:    params.CursorAfterID,
			PagingForward: params.PagingForward,
		})

		if err != nil {
			return nil, err
		}

		results := make([]interface{}, len(comments))
		for i, comment := range comments {
			results[i] = comment
		}

		return results, nil
	}

	countFunc := func() (int, error) {
		total, err := api.loaders.CountRepliesByCommentIDBatch.Load(commentID)
		return int(total), err
	}

	cursorFunc := func(i interface{}) (time.Time, persist.DBID, error) {
		if comment, ok := i.(db.Comment); ok {
			return comment.CreatedAt, comment.ID, nil
		}
		return time.Time{}, "", fmt.Errorf("interface{} is not an comment")
	}

	paginator := timeIDPaginator{
		QueryFunc:  queryFunc,
		CursorFunc: cursorFunc,
		CountFunc:  countFunc,
	}

	results, pageInfo, err := paginator.paginate(before, after, first, last)

	comments := make([]db.Comment, len(results))
	for i, result := range results {
		comments[i] = result.(db.Comment)
	}

	return comments, pageInfo, err
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

	queryFunc := func(params timeIDPagingParams) ([]interface{}, error) {
		admires, err := api.loaders.PaginateAdmiresByPostIDBatch.Load(db.PaginateAdmiresByPostIDBatchParams{
			PostID:        postID,
			Limit:         params.Limit,
			CurBeforeTime: params.CursorBeforeTime,
			CurBeforeID:   params.CursorBeforeID,
			CurAfterTime:  params.CursorAfterTime,
			CurAfterID:    params.CursorAfterID,
			PagingForward: params.PagingForward,
		})

		if err != nil {
			return nil, err
		}

		results := make([]interface{}, len(admires))
		for i, admire := range admires {
			results[i] = admire
		}

		return results, nil
	}

	countFunc := func() (int, error) {
		total, err := api.loaders.CountAdmiresByPostIDBatch.Load(postID)
		return int(total), err
	}

	cursorFunc := func(i interface{}) (time.Time, persist.DBID, error) {
		if admire, ok := i.(db.Admire); ok {
			return admire.CreatedAt, admire.ID, nil
		}
		return time.Time{}, "", fmt.Errorf("interface{} is not an admire")
	}

	paginator := timeIDPaginator{
		QueryFunc:  queryFunc,
		CursorFunc: cursorFunc,
		CountFunc:  countFunc,
	}

	results, pageInfo, err := paginator.paginate(before, after, first, last)

	admires := make([]db.Admire, len(results))
	for i, result := range results {
		admires[i] = result.(db.Admire)
	}

	return admires, pageInfo, err
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

	queryFunc := func(params timeIDPagingParams) ([]interface{}, error) {
		admires, err := api.loaders.PaginateAdmiresByTokenIDBatch.Load(db.PaginateAdmiresByTokenIDBatchParams{
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

		if err != nil {
			return nil, err
		}

		results := make([]interface{}, len(admires))
		for i, admire := range admires {
			results[i] = admire
		}

		return results, nil
	}

	countFunc := func() (int, error) {
		total, err := api.loaders.CountAdmiresByTokenIDBatch.Load(tokenID)
		return int(total), err
	}

	cursorFunc := func(i interface{}) (time.Time, persist.DBID, error) {
		if admire, ok := i.(db.Admire); ok {
			return admire.CreatedAt, admire.ID, nil
		}
		return time.Time{}, "", fmt.Errorf("interface{} is not an admire")
	}

	paginator := timeIDPaginator{
		QueryFunc:  queryFunc,
		CursorFunc: cursorFunc,
		CountFunc:  countFunc,
	}

	results, pageInfo, err := paginator.paginate(before, after, first, last)

	admires := make([]db.Admire, len(results))
	for i, result := range results {
		admires[i] = result.(db.Admire)
	}

	return admires, pageInfo, err
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

	queryFunc := func(params timeIDPagingParams) ([]interface{}, error) {
		comments, err := api.loaders.PaginateCommentsByPostIDBatch.Load(db.PaginateCommentsByPostIDBatchParams{
			PostID:        postID,
			Limit:         params.Limit,
			CurBeforeTime: params.CursorBeforeTime,
			CurBeforeID:   params.CursorBeforeID,
			CurAfterTime:  params.CursorAfterTime,
			CurAfterID:    params.CursorAfterID,
			PagingForward: params.PagingForward,
		})

		if err != nil {
			return nil, err
		}

		results := make([]interface{}, len(comments))
		for i, comment := range comments {
			results[i] = comment
		}

		return results, nil
	}

	countFunc := func() (int, error) {
		total, err := api.loaders.CountCommentsByPostIDBatch.Load(postID)
		return int(total), err
	}

	cursorFunc := func(i interface{}) (time.Time, persist.DBID, error) {
		if comment, ok := i.(db.Comment); ok {
			return comment.CreatedAt, comment.ID, nil
		}
		return time.Time{}, "", fmt.Errorf("interface{} is not an comment")
	}

	paginator := timeIDPaginator{
		QueryFunc:  queryFunc,
		CursorFunc: cursorFunc,
		CountFunc:  countFunc,
	}

	results, pageInfo, err := paginator.paginate(before, after, first, last)

	comments := make([]db.Comment, len(results))
	for i, result := range results {
		comments[i] = result.(db.Comment)
	}

	return comments, pageInfo, err
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
	if err := validate.ValidateFields(api.validator, validate.ValidationMap{
		"feedEventID": validate.WithTag(feedEventID, "required"),
	}); err != nil {
		return "", err
	}

	userID, err := getAuthenticatedUserID(ctx)
	if err != nil {
		return "", err
	}

	admire, err := api.GetAdmireByActorIDAndFeedEventID(ctx, userID, feedEventID)
	if err == nil {
		return "", persist.ErrAdmireAlreadyExists{AdmireID: admire.ID, ActorID: userID, FeedEventID: feedEventID}
	}

	notFoundErr := persist.ErrAdmireFeedEventNotFound{}
	if !errors.As(err, &notFoundErr) {
		return "", err
	}

	admireID, err := api.repos.AdmireRepository.CreateAdmire(ctx, feedEventID, "", userID)
	if err != nil {
		return "", err
	}

	err = event.Dispatch(ctx, db.Event{
		ActorID:        persist.DBIDToNullStr(userID),
		ResourceTypeID: persist.ResourceTypeAdmire,
		SubjectID:      feedEventID,
		FeedEventID:    feedEventID,
		AdmireID:       admireID,
		Action:         persist.ActionAdmiredFeedEvent,
	})

	return admireID, err
}

func (api InteractionAPI) AdmireToken(ctx context.Context, tokenID persist.DBID) (persist.DBID, error) {
	// Validate
	if err := validate.ValidateFields(api.validator, validate.ValidationMap{
		"tokenID": validate.WithTag(tokenID, "required"),
	}); err != nil {
		return "", err
	}

	_, err := api.loaders.GetTokenByIdBatch.Load(tokenID)
	if err != nil {
		return "", err
	}

	userID, err := getAuthenticatedUserID(ctx)
	if err != nil {
		return "", err
	}

	// if admire already exists, the existing admireID is returned
	admireID, err := api.repos.AdmireRepository.CreateTokenAdmire(ctx, tokenID, userID)
	if err != nil {
		return "", err
	}

	err = event.Dispatch(ctx, db.Event{
		ActorID:        persist.DBIDToNullStr(userID),
		ResourceTypeID: persist.ResourceTypeAdmire,
		SubjectID:      tokenID,
		TokenID:        tokenID,
		AdmireID:       admireID,
		Action:         persist.ActionAdmiredToken,
	})

	return admireID, err
}

func (api InteractionAPI) AdmirePost(ctx context.Context, postID persist.DBID) (persist.DBID, error) {
	// Validate
	if err := validate.ValidateFields(api.validator, validate.ValidationMap{
		"postID": validate.WithTag(postID, "required"),
	}); err != nil {
		return "", err
	}

	userID, err := getAuthenticatedUserID(ctx)
	if err != nil {
		return "", err
	}

	admire, err := api.GetAdmireByActorIDAndPostID(ctx, userID, postID)
	if err == nil {
		return "", persist.ErrAdmireAlreadyExists{AdmireID: admire.ID, ActorID: userID, PostID: postID}
	}

	notFoundErr := persist.ErrAdmirePostNotFound{}
	if !errors.As(err, &notFoundErr) {
		return "", err
	}

	admireID, err := api.repos.AdmireRepository.CreateAdmire(ctx, "", postID, userID)
	if err != nil {
		return "", err
	}

	err = event.Dispatch(ctx, db.Event{
		ActorID:        persist.DBIDToNullStr(userID),
		ResourceTypeID: persist.ResourceTypeAdmire,
		SubjectID:      postID,
		PostID:         postID,
		AdmireID:       admireID,
		Action:         persist.ActionAdmiredPost,
	})

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

	return admire.FeedEventID, admire.PostID, api.repos.AdmireRepository.RemoveAdmire(ctx, admireID)
}

func (api InteractionAPI) HasUserAdmiredFeedEvent(ctx context.Context, userID persist.DBID, feedEventID persist.DBID) (*bool, error) {
	// Validate
	if err := validate.ValidateFields(api.validator, validate.ValidationMap{
		"userID":      validate.WithTag(userID, "required"),
		"feedEventID": validate.WithTag(feedEventID, "required"),
	}); err != nil {
		return nil, err
	}

	_, err := api.GetAdmireByActorIDAndFeedEventID(ctx, userID, feedEventID)
	if err == nil {
		hasAdmired := true
		return &hasAdmired, nil
	}

	notFoundErr := persist.ErrAdmireFeedEventNotFound{}
	if errors.As(err, &notFoundErr) {
		hasAdmired := false
		return &hasAdmired, nil
	}

	return nil, err
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
	if feedEventID != "" {
		action = persist.ActionCommentedOnFeedEvent
	} else if postID != "" {
		action = persist.ActionCommentedOnPost
	} else {
		panic("commenting on neither feed event nor post")
	}

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
			case mention.ContractID != "":
				err = event.Dispatch(ctx, db.Event{
					ActorID:        persist.DBIDToNullStr(actor),
					ResourceTypeID: persist.ResourceTypeContract,
					SubjectID:      mention.ContractID,
					PostID:         postID,
					FeedEventID:    feedEventID,
					ContractID:     mention.ContractID,
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
			if c, err := queries.GetContractByID(ctx, *m.CommunityID); c.ID == "" || err != nil {
				return nil, fmt.Errorf("could retrieve community: %s (%s)", *m.CommunityID, err)
			}
			mention.ContractID = *m.CommunityID
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
