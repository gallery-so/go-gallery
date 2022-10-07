package publicapi

import (
	"context"
	"errors"
	"github.com/mikeydub/go-gallery/validate"
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

func (api InteractionAPI) PaginateInteractionsByFeedEventID(ctx context.Context, feedEventID persist.DBID, before *string, after *string,
	first *int, last *int) ([]interface{}, PageInfo, error) {
	// Validate
	if err := validateFields(api.validator, validationMap{
		"feedEventID": {feedEventID, "required"},
		"first":       {first, "omitempty,gte=0"},
		"last":        {last, "omitempty,gte=0"},
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
		curBeforeTime, curBeforeID, err = decodeTimestampDBIDCursor(*before)
		if err != nil {
			return nil, PageInfo{}, err
		}
	}

	if after != nil {
		curAfterTime, curAfterID, err = decodeTimestampDBIDCursor(*after)
		if err != nil {
			return nil, PageInfo{}, err
		}
	}

	type interactionKey struct {
		ID        persist.DBID
		CreatedAt time.Time
		Tag       int
	}
	var orderedKeys []interactionKey
	typeToIDs := make(map[int][]persist.DBID)

	if first != nil {
		rows, err := api.queries.PaginateInteractionsByFeedEventIDForward(ctx, db.PaginateInteractionsByFeedEventIDForwardParams{
			FeedEventID:   feedEventID,
			Limit:         int32(limit),
			CurBeforeTime: curBeforeTime,
			CurBeforeID:   curBeforeID,
			CurAfterTime:  curAfterTime,
			CurAfterID:    curAfterID,
			AdmireTag:     1,
			CommentTag:    2,
		})

		if err != nil {
			return nil, PageInfo{}, err
		}

		for _, row := range rows {
			orderedKeys = append(orderedKeys, interactionKey{ID: row.ID, CreatedAt: row.CreatedAt, Tag: int(row.Tag)})
			typeToIDs[int(row.Tag)] = append(typeToIDs[int(row.Tag)], row.ID)
		}
	} else {
		rows, err := api.queries.PaginateInteractionsByFeedEventIDBackward(ctx, db.PaginateInteractionsByFeedEventIDBackwardParams{
			FeedEventID:   feedEventID,
			Limit:         int32(limit),
			CurBeforeTime: curBeforeTime,
			CurBeforeID:   curBeforeID,
			CurAfterTime:  curAfterTime,
			CurAfterID:    curAfterID,
			AdmireTag:     1,
			CommentTag:    2,
		})

		if err != nil {
			return nil, PageInfo{}, err
		}

		for _, row := range rows {
			orderedKeys = append(orderedKeys, interactionKey{ID: row.ID, CreatedAt: row.CreatedAt, Tag: int(row.Tag)})
			typeToIDs[int(row.Tag)] = append(typeToIDs[int(row.Tag)], row.ID)
		}
	}

	if err != nil {
		return nil, PageInfo{}, err
	}

	pageInfo := PageInfo{}

	// Since limit is actually 1 more than requested, if len(comments) == limit, there must be additional pages
	if len(orderedKeys) == limit {
		orderedKeys = orderedKeys[:len(orderedKeys)-1]
		if first != nil {
			pageInfo.HasNextPage = true
		} else {
			pageInfo.HasPreviousPage = true
		}
	}

	if last != nil {
		// Reverse the slice if we're paginating backwards, since forward and backward
		// pagination are supposed to have elements in the same order.
		for i, j := 0, len(orderedKeys)-1; i < j; i, j = i+1, j-1 {
			orderedKeys[i], orderedKeys[j] = orderedKeys[j], orderedKeys[i]
		}
	}

	// If this is the first query (i.e. no cursors have been supplied), return the total count too
	if before == nil && after == nil {
		total, err := api.queries.CountInteractionsByFeedEventID(ctx, db.CountInteractionsByFeedEventIDParams{
			FeedEventID: feedEventID,
			AdmireTag:   1,
			CommentTag:  2,
		})

		if err != nil {
			return nil, PageInfo{}, err
		}
		totalInt := int(total)
		pageInfo.Total = &totalInt
	}

	pageInfo.Size = len(orderedKeys)

	if len(orderedKeys) > 0 {
		firstNode := orderedKeys[0]
		lastNode := orderedKeys[len(orderedKeys)-1]

		pageInfo.StartCursor, err = encodeTimestampDBIDCursor(firstNode.CreatedAt, firstNode.ID)
		if err != nil {
			return nil, PageInfo{}, err
		}

		pageInfo.EndCursor, err = encodeTimestampDBIDCursor(lastNode.CreatedAt, lastNode.ID)
		if err != nil {
			return nil, PageInfo{}, err
		}
	}

	var results []interface{}
	resultsByID := make(map[persist.DBID]interface{})

	// TODO: Execute these queries in parallel
	admireIDs := typeToIDs[1]
	if len(admireIDs) > 0 {
		admires, err := api.queries.GetAdmiresByAdmireIDs(ctx, admireIDs)
		if err != nil {
			return nil, PageInfo{}, err
		}

		for _, admire := range admires {
			resultsByID[admire.ID] = admire
		}
	}

	commentIDs := typeToIDs[2]
	if len(commentIDs) > 0 {
		comments, err := api.queries.GetCommentsByCommentIDs(ctx, commentIDs)
		if err != nil {
			return nil, PageInfo{}, err
		}

		for _, comment := range comments {
			resultsByID[comment.ID] = comment
		}
	}

	for _, key := range orderedKeys {
		if result, ok := resultsByID[key.ID]; ok {
			results = append(results, result)
		}
	}

	return results, pageInfo, err
}

func (api InteractionAPI) PaginateCommentsByFeedEventID(ctx context.Context, feedEventID persist.DBID, before *string, after *string,
	first *int, last *int) ([]db.Comment, PageInfo, error) {
	// Validate
	if err := validateFields(api.validator, validationMap{
		"feedEventID": {feedEventID, "required"},
		"first":       {first, "omitempty,gte=0"},
		"last":        {last, "omitempty,gte=0"},
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
		curBeforeTime, curBeforeID, err = decodeTimestampDBIDCursor(*before)
		if err != nil {
			return nil, PageInfo{}, err
		}
	}

	if after != nil {
		curAfterTime, curAfterID, err = decodeTimestampDBIDCursor(*after)
		if err != nil {
			return nil, PageInfo{}, err
		}
	}

	var comments []db.Comment

	if first != nil {
		comments, err = api.queries.PaginateCommentsByFeedEventIDForward(ctx, db.PaginateCommentsByFeedEventIDForwardParams{
			FeedEventID:   feedEventID,
			Limit:         int32(limit),
			CurBeforeTime: curBeforeTime,
			CurBeforeID:   curBeforeID,
			CurAfterTime:  curAfterTime,
			CurAfterID:    curAfterID,
		})
	} else {
		comments, err = api.queries.PaginateCommentsByFeedEventIDBackward(ctx, db.PaginateCommentsByFeedEventIDBackwardParams{
			FeedEventID:   feedEventID,
			Limit:         int32(limit),
			CurBeforeTime: curBeforeTime,
			CurBeforeID:   curBeforeID,
			CurAfterTime:  curAfterTime,
			CurAfterID:    curAfterID,
		})
	}

	if err != nil {
		return nil, PageInfo{}, err
	}

	pageInfo := PageInfo{}

	// Since limit is actually 1 more than requested, if len(comments) == limit, there must be additional pages
	if len(comments) == limit {
		comments = comments[:len(comments)-1]
		if first != nil {
			pageInfo.HasNextPage = true
		} else {
			pageInfo.HasPreviousPage = true
		}
	}

	if last != nil {
		// Reverse the slice if we're paginating backwards, since forward and backward
		// pagination are supposed to have elements in the same order.
		for i, j := 0, len(comments)-1; i < j; i, j = i+1, j-1 {
			comments[i], comments[j] = comments[j], comments[i]
		}
	}

	// If this is the first query (i.e. no cursors have been supplied), return the total count too
	if before == nil && after == nil {
		total, err := api.queries.CountAdmiresByFeedEventID(ctx, feedEventID)
		if err != nil {
			return nil, PageInfo{}, err
		}
		totalInt := int(total)
		pageInfo.Total = &totalInt
	}

	pageInfo.Size = len(comments)

	if len(comments) > 0 {
		firstNode := comments[0]
		lastNode := comments[len(comments)-1]

		pageInfo.StartCursor, err = encodeTimestampDBIDCursor(firstNode.CreatedAt, firstNode.ID)
		if err != nil {
			return nil, PageInfo{}, err
		}

		pageInfo.EndCursor, err = encodeTimestampDBIDCursor(lastNode.CreatedAt, lastNode.ID)
		if err != nil {
			return nil, PageInfo{}, err
		}
	}

	return comments, pageInfo, err
}

func (api InteractionAPI) PaginateAdmiresByFeedEventID(ctx context.Context, feedEventID persist.DBID, before *string, after *string,
	first *int, last *int) ([]db.Admire, PageInfo, error) {
	// Validate
	if err := validateFields(api.validator, validationMap{
		"feedEventID": {feedEventID, "required"},
		"first":       {first, "omitempty,gte=0"},
		"last":        {last, "omitempty,gte=0"},
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
		curBeforeTime, curBeforeID, err = decodeTimestampDBIDCursor(*before)
		if err != nil {
			return nil, PageInfo{}, err
		}
	}

	if after != nil {
		curAfterTime, curAfterID, err = decodeTimestampDBIDCursor(*after)
		if err != nil {
			return nil, PageInfo{}, err
		}
	}

	var admires []db.Admire

	if first != nil {
		admires, err = api.queries.PaginateAdmiresByFeedEventIDForward(ctx, db.PaginateAdmiresByFeedEventIDForwardParams{
			FeedEventID:   feedEventID,
			Limit:         int32(limit),
			CurBeforeTime: curBeforeTime,
			CurBeforeID:   curBeforeID,
			CurAfterTime:  curAfterTime,
			CurAfterID:    curAfterID,
		})
	} else {
		admires, err = api.queries.PaginateAdmiresByFeedEventIDBackward(ctx, db.PaginateAdmiresByFeedEventIDBackwardParams{
			FeedEventID:   feedEventID,
			Limit:         int32(limit),
			CurBeforeTime: curBeforeTime,
			CurBeforeID:   curBeforeID,
			CurAfterTime:  curAfterTime,
			CurAfterID:    curAfterID,
		})
	}

	if err != nil {
		return nil, PageInfo{}, err
	}

	pageInfo := PageInfo{}

	// Since limit is actually 1 more than requested, if len(admires) == limit, there must be additional pages
	if len(admires) == limit {
		admires = admires[:len(admires)-1]
		if first != nil {
			pageInfo.HasNextPage = true
		} else {
			pageInfo.HasPreviousPage = true
		}
	}

	if last != nil {
		// Reverse the slice if we're paginating backwards, since forward and backward
		// pagination are supposed to have elements in the same order.
		for i, j := 0, len(admires)-1; i < j; i, j = i+1, j-1 {
			admires[i], admires[j] = admires[j], admires[i]
		}
	}

	// If this is the first query (i.e. no cursors have been supplied), return the total count too
	if before == nil && after == nil {
		total, err := api.queries.CountAdmiresByFeedEventID(ctx, feedEventID)
		if err != nil {
			return nil, PageInfo{}, err
		}
		totalInt := int(total)
		pageInfo.Total = &totalInt
	}

	pageInfo.Size = len(admires)

	if len(admires) > 0 {
		firstNode := admires[0]
		lastNode := admires[len(admires)-1]

		pageInfo.StartCursor, err = encodeTimestampDBIDCursor(firstNode.CreatedAt, firstNode.ID)
		if err != nil {
			return nil, PageInfo{}, err
		}

		pageInfo.EndCursor, err = encodeTimestampDBIDCursor(lastNode.CreatedAt, lastNode.ID)
		if err != nil {
			return nil, PageInfo{}, err
		}
	}

	return admires, pageInfo, err
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

func (api InteractionAPI) GetAdmiresByFeedEventIDOld(ctx context.Context, feedEventID persist.DBID) ([]db.Admire, error) {
	// Validate
	if err := validateFields(api.validator, validationMap{
		"feedEventID": {feedEventID, "required"},
	}); err != nil {
		return nil, err
	}

	admires, err := api.loaders.AdmiresByFeedEventID.Load(feedEventID)
	if err != nil {
		return nil, err
	}

	return admires, nil
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
