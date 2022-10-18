package publicapi

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/mikeydub/go-gallery/validate"

	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/go-playground/validator/v10"
	db "github.com/mikeydub/go-gallery/db/gen/coredb"
	"github.com/mikeydub/go-gallery/graphql/dataloader"
	"github.com/mikeydub/go-gallery/service/persist"
)

// Some date that comes before any other valid timestamps in our database
var defaultCursorAfterTime = time.Date(1970, 1, 1, 1, 1, 1, 1, time.UTC)

// Some date that comes after any other valid timestamps in our database
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

func (api InteractionAPI) makeTagMap(typeFilter []persist.InteractionType) map[persist.InteractionType]int32 {
	tags := make(map[persist.InteractionType]int32)

	if len(typeFilter) > 0 {
		for _, t := range typeFilter {
			tags[t] = int32(t)
		}
	} else {
		for i := int32(persist.MinInteractionTypeValue); i <= int32(persist.MaxInteractionTypeValue); i++ {
			tags[persist.InteractionType(i)] = i
		}
	}

	return tags
}

func (api InteractionAPI) PaginateInteractionsByFeedEventID(ctx context.Context, feedEventID persist.DBID, before *string, after *string,
	first *int, last *int, typeFilter []persist.InteractionType) ([]interface{}, PageInfo, error) {
	// Validate
	if err := validateFields(api.validator, validationMap{
		"feedEventID": {feedEventID, "required"},
		"typeFilter":  {typeFilter, fmt.Sprintf("omitempty,min=1,unique,dive,gte=%d,lte=%d", persist.MinInteractionTypeValue, persist.MaxInteractionTypeValue)},
	}); err != nil {
		return nil, PageInfo{}, err
	}

	if err := validatePaginationParams(api.validator, first, last); err != nil {
		return nil, PageInfo{}, err
	}

	tags := api.makeTagMap(typeFilter)

	queryFunc := func(params timeIDPagingParams) ([]interface{}, error) {
		keys, err := api.loaders.InteractionsByFeedEventID.Load(db.PaginateInteractionsByFeedEventIDBatchParams{
			FeedEventID:   feedEventID,
			Limit:         params.Limit,
			CurBeforeTime: params.CursorBeforeTime,
			CurBeforeID:   params.CursorBeforeID,
			CurAfterTime:  params.CursorAfterTime,
			CurAfterID:    params.CursorAfterID,
			PagingForward: params.PagingForward,
			AdmireTag:     tags[persist.InteractionTypeAdmire],
			CommentTag:    tags[persist.InteractionTypeComment],
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
		counts, err := api.loaders.InteractionCountByFeedEventID.Load(db.CountInteractionsByFeedEventIDBatchParams{
			FeedEventID: feedEventID,
			AdmireTag:   tags[persist.InteractionTypeAdmire],
			CommentTag:  tags[persist.InteractionTypeComment],
		})

		total := 0

		for _, count := range counts {
			total += int(count.Count)
		}

		return total, err
	}

	cursorFunc := func(i interface{}) (time.Time, persist.DBID, error) {
		if row, ok := i.(db.PaginateInteractionsByFeedEventIDBatchRow); ok {
			return row.CreatedAt, row.ID, nil
		}
		return time.Time{}, "", fmt.Errorf("interface{} is not the correct type")
	}

	paginator := timeIDPaginator{
		QueryFunc:  queryFunc,
		CursorFunc: cursorFunc,
		CountFunc:  countFunc,
	}

	results, pageInfo, err := paginator.paginate(before, after, first, last)

	if err != nil {
		return nil, PageInfo{}, err
	}

	orderedKeys := make([]db.PaginateInteractionsByFeedEventIDBatchRow, len(results))
	typeToIDs := make(map[int32][]persist.DBID)

	for i, result := range results {
		row := result.(db.PaginateInteractionsByFeedEventIDBatchRow)
		orderedKeys[i] = row
		typeToIDs[row.Tag] = append(typeToIDs[row.Tag], row.ID)
	}

	var interactions []interface{}
	interactionsByID := make(map[persist.DBID]interface{})
	var interactionsByIDMutex sync.Mutex
	var wg sync.WaitGroup

	admireIDs := typeToIDs[tags[persist.InteractionTypeAdmire]]
	commentIDs := typeToIDs[tags[persist.InteractionTypeComment]]

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

	if err := validatePaginationParams(api.validator, first, last); err != nil {
		return nil, PageInfo{}, err
	}

	queryFunc := func(params timeIDPagingParams) ([]interface{}, error) {
		admires, err := api.loaders.AdmiresByFeedEventID.Load(db.PaginateAdmiresByFeedEventIDBatchParams{
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
		total, err := api.loaders.AdmireCountByFeedEventID.Load(feedEventID)
		return total, err
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

func (api InteractionAPI) PaginateCommentsByFeedEventID(ctx context.Context, feedEventID persist.DBID, before *string, after *string,
	first *int, last *int) ([]db.Comment, PageInfo, error) {
	// Validate
	if err := validateFields(api.validator, validationMap{
		"feedEventID": {feedEventID, "required"},
	}); err != nil {
		return nil, PageInfo{}, err
	}

	if err := validatePaginationParams(api.validator, first, last); err != nil {
		return nil, PageInfo{}, err
	}

	queryFunc := func(params timeIDPagingParams) ([]interface{}, error) {
		comments, err := api.loaders.CommentsByFeedEventID.Load(db.PaginateCommentsByFeedEventIDBatchParams{
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
		total, err := api.loaders.CommentCountByFeedEventID.Load(feedEventID)
		return total, err
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
	if err := validateFields(api.validator, validationMap{
		"actorID":     {actorID, "required"},
		"feedEventID": {feedEventID, "required"},
	}); err != nil {
		return nil, err
	}

	admire, err := api.loaders.AdmireByActorIDAndFeedEventID.Load(db.GetAdmireByActorIDAndFeedEventIDParams{
		ActorID:     actorID,
		FeedEventID: feedEventID,
	})

	if err != nil {
		return nil, err
	}

	return &admire, nil
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

	userID, err := getAuthenticatedUser(ctx)
	if err != nil {
		return "", err
	}

	admire, err := api.GetAdmireByActorIDAndFeedEventID(ctx, userID, feedEventID)
	if err == nil {
		return "", persist.ErrAdmireAlreadyExists{AdmireID: admire.ID, ActorID: userID, FeedEventID: feedEventID}
	}

	notFoundErr := persist.ErrAdmireNotFound{}
	if !errors.As(err, &notFoundErr) {
		return "", err
	}

	admireID, err := api.repos.AdmireRepository.CreateAdmire(ctx, feedEventID, userID)

	go dispatchEvent(ctx, db.Event{
		ActorID:        userID,
		ResourceTypeID: persist.ResourceTypeFeedEvent,
		SubjectID:      feedEventID,
		FeedEventID:    feedEventID,
		AdmireID:       admireID,
	})

	return admireID, err
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

func (api InteractionAPI) HasUserAdmiredFeedEvent(ctx context.Context, userID persist.DBID, feedEventID persist.DBID) (*bool, error) {
	// Validate
	if err := validateFields(api.validator, validationMap{
		"userID":      {userID, "required"},
		"feedEventID": {feedEventID, "required"},
	}); err != nil {
		return nil, err
	}

	_, err := api.GetAdmireByActorIDAndFeedEventID(ctx, userID, feedEventID)
	if err == nil {
		hasAdmired := true
		return &hasAdmired, nil
	}

	notFoundErr := persist.ErrAdmireNotFound{}
	if errors.As(err, &notFoundErr) {
		hasAdmired := false
		return &hasAdmired, nil
	}

	return nil, err
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
	// Trim whitespace first, so comments consisting only of whitespace will fail
	// the "required" validation below
	comment = strings.TrimSpace(comment)

	// Validate
	if err := validateFields(api.validator, validationMap{
		"feedEventID": {feedEventID, "required"},
		"comment":     {comment, "required"},
	}); err != nil {
		return "", err
	}

	actor, err := getAuthenticatedUser(ctx)
	if err != nil {
		return "", err
	}

	// Sanitize
	comment = validate.SanitizationPolicy.Sanitize(comment)

	commentID, err := api.repos.CommentRepository.CreateComment(ctx, feedEventID, actor, replyToID, comment)
	if err != nil {
		return "", err
	}

	go dispatchEvent(ctx, db.Event{
		ActorID:        actor,
		ResourceTypeID: persist.ResourceTypeFeedEvent,
		SubjectID:      feedEventID,
		FeedEventID:    feedEventID,
		CommentID:      commentID,
	})

	return commentID, nil
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
