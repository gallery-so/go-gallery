package publicapi

import (
	"context"
	"errors"
	"fmt"
	"github.com/mikeydub/go-gallery/validate"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/go-playground/validator/v10"
	db "github.com/mikeydub/go-gallery/db/gen/coredb"
	"github.com/mikeydub/go-gallery/graphql/dataloader"
	"github.com/mikeydub/go-gallery/service/persist"
)

// Some date that comes before any created/updated timestamps in our database
var defaultCursorAfterTime = time.Date(1970, 1, 1, 1, 1, 1, 1, time.UTC)

// Some date that comes after any created/updated timestamps in our database
var defaultCursorBeforeTime = time.Date(3000, 1, 1, 1, 1, 1, 1, time.UTC)

var ErrOnlyRemoveOwnAdmire = errors.New("only the actor who created the admire can remove it")
var ErrOnlyRemoveOwnComment = errors.New("only the actor who created the comment can remove it")

type InteractionAPI struct {
	repos     *persist.Repositories
	queries   *db.Queries
	loaders   *dataloader.Loaders
	validator *validator.Validate
	ethClient *ethclient.Client
}

// timeIDPagingParams are the parameters used to paginate with a time+DBID cursor
type timeIDPagingParams struct {
	Limit         int32
	CurBeforeTime time.Time
	CurBeforeID   persist.DBID
	CurAfterTime  time.Time
	CurAfterID    persist.DBID
	PagingForward bool
}

// timeIDPagedQuery returns paginated results for the given paging parameters
type timeIDPagedQuery func(params timeIDPagingParams) ([]interface{}, error)

// timeIDTotalCount returns the total number of items that can be paginated
type timeIDTotalCount func() (int, error)

// timeIDCursorComponents returns a time and DBID that will be used to encode an opaque cursor string
type timeIDCursorComponents func(interface{}) (time.Time, persist.DBID, error)

func (api InteractionAPI) PaginateInteractionsByFeedEventID(ctx context.Context, feedEventID persist.DBID, before *string, after *string,
	first *int, last *int) ([]interface{}, PageInfo, error) {
	// Validate
	if err := validateFields(api.validator, validationMap{
		"feedEventID": {feedEventID, "required"},
	}); err != nil {
		return nil, PageInfo{}, err
	}

	queryFunc := func(params timeIDPagingParams) ([]interface{}, error) {
		keys, err := api.queries.PaginateInteractionsByFeedEventID(ctx, db.PaginateInteractionsByFeedEventIDParams{
			FeedEventID:   feedEventID,
			Limit:         params.Limit,
			CurBeforeTime: params.CurBeforeTime,
			CurBeforeID:   params.CurBeforeID,
			CurAfterTime:  params.CurAfterTime,
			CurAfterID:    params.CurAfterID,
			PagingForward: params.PagingForward,
			AdmireTag:     1,
			CommentTag:    2,
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
		total, err := api.queries.CountInteractionsByFeedEventID(ctx, db.CountInteractionsByFeedEventIDParams{
			FeedEventID: feedEventID,
			AdmireTag:   1,
			CommentTag:  2,
		})
		return int(total), err
	}

	cursorFunc := func(i interface{}) (time.Time, persist.DBID, error) {
		if row, ok := i.(db.PaginateInteractionsByFeedEventIDRow); ok {
			return row.CreatedAt, row.ID, nil
		}
		return time.Time{}, "", fmt.Errorf("interface{} is not the correct type")
	}

	results, pageInfo, err := api.paginateWithTimeIDCursor(ctx, before, after, first, last, queryFunc, countFunc, cursorFunc)

	if err != nil {
		return nil, PageInfo{}, err
	}

	orderedKeys := make([]db.PaginateInteractionsByFeedEventIDRow, len(results))
	typeToIDs := make(map[int][]persist.DBID)

	for i, result := range results {
		row := result.(db.PaginateInteractionsByFeedEventIDRow)
		orderedKeys[i] = row
		typeToIDs[int(row.Tag)] = append(typeToIDs[int(row.Tag)], row.ID)
	}

	var interactions []interface{}
	interactionsByID := make(map[persist.DBID]interface{})
	var interactionsByIDMutex sync.Mutex
	var wg sync.WaitGroup

	admireIDs := typeToIDs[1]
	commentIDs := typeToIDs[2]

	if len(admireIDs) > 0 {
		wg.Add(1)
		go func() {
			admires, errs := api.loaders.AdmireByAdmireID.LoadAll(admireIDs)

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
			comments, errs := api.loaders.CommentByCommentID.LoadAll(commentIDs)

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

	return interactions, pageInfo, err
}

func (api InteractionAPI) PaginateAdmiresByFeedEventID(ctx context.Context, feedEventID persist.DBID, before *string, after *string,
	first *int, last *int) ([]db.Admire, PageInfo, error) {
	// Validate
	if err := validateFields(api.validator, validationMap{
		"feedEventID": {feedEventID, "required"},
	}); err != nil {
		return nil, PageInfo{}, err
	}

	queryFunc := func(params timeIDPagingParams) ([]interface{}, error) {
		admires, err := api.loaders.FeedEventAdmires.Load(db.PaginateAdmiresByFeedEventIDBatchParams{
			FeedEventID:   feedEventID,
			Limit:         params.Limit,
			CurBeforeTime: params.CurBeforeTime,
			CurBeforeID:   params.CurBeforeID,
			CurAfterTime:  params.CurAfterTime,
			CurAfterID:    params.CurAfterID,
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
		total, err := api.queries.CountAdmiresByFeedEventID(ctx, feedEventID)
		return int(total), err
	}

	cursorFunc := func(i interface{}) (time.Time, persist.DBID, error) {
		if admire, ok := i.(db.Admire); ok {
			return admire.CreatedAt, admire.ID, nil
		}
		return time.Time{}, "", fmt.Errorf("interface{} is not an admire")
	}

	results, pageInfo, err := api.paginateWithTimeIDCursor(ctx, before, after, first, last, queryFunc, countFunc, cursorFunc)

	admires := make([]db.Admire, len(results))
	for i, result := range results {
		admires[i] = result.(db.Admire)
	}

	return admires, pageInfo, err
}

func (api InteractionAPI) PaginateCommentsByFeedEventID(ctx context.Context, feedEventID persist.DBID, before *string, after *string,
	first *int, last *int) ([]db.Comment, PageInfo, error) {
	// Validate
	if err := validateFields(api.validator, validationMap{
		"feedEventID": {feedEventID, "required"},
	}); err != nil {
		return nil, PageInfo{}, err
	}

	queryFunc := func(params timeIDPagingParams) ([]interface{}, error) {
		comments, err := api.loaders.FeedEventComments.Load(db.PaginateCommentsByFeedEventIDBatchParams{
			FeedEventID:   feedEventID,
			Limit:         params.Limit,
			CurBeforeTime: params.CurBeforeTime,
			CurBeforeID:   params.CurBeforeID,
			CurAfterTime:  params.CurAfterTime,
			CurAfterID:    params.CurAfterID,
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
		total, err := api.queries.CountCommentsByFeedEventID(ctx, feedEventID)
		return int(total), err
	}

	cursorFunc := func(i interface{}) (time.Time, persist.DBID, error) {
		if comment, ok := i.(db.Comment); ok {
			return comment.CreatedAt, comment.ID, nil
		}
		return time.Time{}, "", fmt.Errorf("interface{} is not an comment")
	}

	results, pageInfo, err := api.paginateWithTimeIDCursor(ctx, before, after, first, last, queryFunc, countFunc, cursorFunc)

	comments := make([]db.Comment, len(results))
	for i, result := range results {
		comments[i] = result.(db.Comment)
	}

	return comments, pageInfo, err
}

func (api InteractionAPI) paginateWithTimeIDCursor(ctx context.Context, before *string, after *string,
	first *int, last *int, queryFunc timeIDPagedQuery, countFunc timeIDTotalCount, cursorFunc timeIDCursorComponents) ([]interface{}, PageInfo, error) {
	// Validate
	if err := validateFields(api.validator, validationMap{
		"first": {first, "omitempty,gte=0"},
		"last":  {last, "omitempty,gte=0"},
	}); err != nil {
		return nil, PageInfo{}, err
	}

	if err := api.validator.Struct(validate.ConnectionPaginationParams{
		Before: before,
		After:  after,
		First:  first,
		Last:   last,
	}); err != nil {
		return nil, PageInfo{}, err
	}

	curBeforeTime := defaultCursorBeforeTime
	curBeforeID := persist.DBID("")
	curAfterTime := defaultCursorAfterTime
	curAfterID := persist.DBID("")

	// Limit is intentionally 1 more than requested, so we can see if there are additional pages
	limit := 1
	if first != nil {
		limit += *first
	} else {
		limit += *last
	}

	var err error
	if before != nil {
		curBeforeTime, curBeforeID, err = decodeTimeIDCursor(*before)
		if err != nil {
			return nil, PageInfo{}, err
		}
	}

	if after != nil {
		curAfterTime, curAfterID, err = decodeTimeIDCursor(*after)
		if err != nil {
			return nil, PageInfo{}, err
		}
	}

	queryParams := timeIDPagingParams{
		Limit:         int32(limit),
		CurBeforeTime: curBeforeTime,
		CurBeforeID:   curBeforeID,
		CurAfterTime:  curAfterTime,
		CurAfterID:    curAfterID,
		PagingForward: first != nil,
	}

	results, err := queryFunc(queryParams)

	if err != nil {
		return nil, PageInfo{}, err
	}

	pageInfo := PageInfo{}

	// Since limit is actually 1 more than requested, if len(results) == limit, there must be additional pages
	if len(results) == limit {
		results = results[:len(results)-1]
		if first != nil {
			pageInfo.HasNextPage = true
		} else {
			pageInfo.HasPreviousPage = true
		}
	}

	if last != nil {
		// Reverse the slice if we're paginating backwards, since forward and backward
		// pagination are supposed to have elements in the same order.
		for i, j := 0, len(results)-1; i < j; i, j = i+1, j-1 {
			results[i], results[j] = results[j], results[i]
		}
	}

	// If this is the first query (i.e. no cursors have been supplied), return the total count too
	if before == nil && after == nil {
		total, err := countFunc()
		if err != nil {
			return nil, PageInfo{}, err
		}
		totalInt := int(total)
		pageInfo.Total = &totalInt
	}

	pageInfo.Size = len(results)

	if len(results) > 0 {
		firstNode := results[0]
		lastNode := results[len(results)-1]

		firstTime, firstID, err := cursorFunc(firstNode)
		if err != nil {
			return nil, PageInfo{}, err
		}

		lastTime, lastID, err := cursorFunc(lastNode)
		if err != nil {
			return nil, PageInfo{}, err
		}

		pageInfo.StartCursor, err = encodeTimeIDCursor(firstTime, firstID)
		if err != nil {
			return nil, PageInfo{}, err
		}

		pageInfo.EndCursor, err = encodeTimeIDCursor(lastTime, lastID)
		if err != nil {
			return nil, PageInfo{}, err
		}
	}

	return results, pageInfo, err
}

func (api InteractionAPI) GetAdmireByID(ctx context.Context, admireID persist.DBID) (*db.Admire, error) {
	// Validate
	if err := validateFields(api.validator, validationMap{
		"admireID": {admireID, "required"},
	}); err != nil {
		return nil, err
	}

	admire, err := api.loaders.AdmireByAdmireID.Load(admireID)
	if err != nil {
		return nil, err
	}

	return &admire, nil
}

func (api InteractionAPI) AdmireFeedEvent(ctx context.Context, feedEventID persist.DBID) (persist.DBID, error) {
	// Validate
	if err := validateFields(api.validator, validationMap{
		"feedEventID": {feedEventID, "required"},
	}); err != nil {
		return "", err
	}

	return api.repos.AdmireRepository.CreateAdmire(ctx, feedEventID, For(ctx).User.GetLoggedInUserId(ctx))
}

func (api InteractionAPI) RemoveAdmire(ctx context.Context, admireID persist.DBID) (persist.DBID, error) {
	// Validate
	if err := validateFields(api.validator, validationMap{
		"admireID": {admireID, "required"},
	}); err != nil {
		return "", err
	}

	// will also fail if admire does not exist
	admire, err := api.GetAdmireByID(ctx, admireID)
	if err != nil {
		return "", err
	}
	if admire.ActorID != For(ctx).User.GetLoggedInUserId(ctx) {
		return "", ErrOnlyRemoveOwnAdmire
	}

	return admire.FeedEventID, api.repos.AdmireRepository.RemoveAdmire(ctx, admireID)
}

func (api InteractionAPI) GetCommentByID(ctx context.Context, commentID persist.DBID) (*db.Comment, error) {
	// Validate
	if err := validateFields(api.validator, validationMap{
		"commentID": {commentID, "required"},
	}); err != nil {
		return nil, err
	}

	comment, err := api.loaders.CommentByCommentID.Load(commentID)
	if err != nil {
		return nil, err
	}

	return &comment, nil
}

func (api InteractionAPI) CommentOnFeedEvent(ctx context.Context, feedEventID persist.DBID, replyToID *persist.DBID, comment string) (persist.DBID, error) {
	// Validate
	if err := validateFields(api.validator, validationMap{
		"feedEventID": {feedEventID, "required"},
	}); err != nil {
		return "", err
	}

	return api.repos.CommentRepository.CreateComment(ctx, feedEventID, For(ctx).User.GetLoggedInUserId(ctx), replyToID, comment)
}

func (api InteractionAPI) RemoveComment(ctx context.Context, commentID persist.DBID) (persist.DBID, error) {
	// Validate
	if err := validateFields(api.validator, validationMap{
		"commentID": {commentID, "required"},
	}); err != nil {
		return "", err
	}
	comment, err := api.GetCommentByID(ctx, commentID)
	if err != nil {
		return "", err
	}
	if comment.ActorID != For(ctx).User.GetLoggedInUserId(ctx) {
		return "", ErrOnlyRemoveOwnComment
	}

	return comment.FeedEventID, api.repos.CommentRepository.RemoveComment(ctx, commentID)
}
