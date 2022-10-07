package publicapi

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/go-playground/validator/v10"
	db "github.com/mikeydub/go-gallery/db/gen/coredb"
	"github.com/mikeydub/go-gallery/graphql/dataloader"
	"github.com/mikeydub/go-gallery/graphql/model"
	"github.com/mikeydub/go-gallery/service/persist"
)

type FeedAPI struct {
	repos     *persist.Repositories
	queries   *db.Queries
	loaders   *dataloader.Loaders
	validator *validator.Validate
	ethClient *ethclient.Client
}

func (api FeedAPI) GetEventById(ctx context.Context, eventID persist.DBID) (*db.FeedEvent, error) {
	// Validate
	if err := validateFields(api.validator, validationMap{
		"eventID": {eventID, "required"},
	}); err != nil {
		return nil, err
	}

	event, err := api.loaders.EventByEventID.Load(eventID)
	if err != nil {
		return nil, err
	}

	return &event, nil
}

func (api FeedAPI) GetViewerFeed(ctx context.Context, before *persist.DBID, after *persist.DBID, first *int, last *int) (persist.DBID, []db.FeedEvent, error) {
	userID, err := getAuthenticatedUser(ctx)
	if err != nil {
		return "", nil, err
	}

	// Validate
	if err := validateFields(api.validator, validationMap{
		"userID": {userID, "required"},
	}); err != nil {
		return "", nil, err
	}

	if err := validatePaginationParams(api.validator, first, last); err != nil {
		return "", nil, err
	}

	params := db.GetPersonalFeedViewBatchParams{Follower: userID}

	if first != nil {
		params.FromFirst = true
		params.Limit = int32(*first)
	}

	if last != nil {
		params.FromFirst = false
		params.Limit = int32(*last)
	}

	if before != nil {
		params.CurBefore = string(*before)
	}

	if after != nil {
		params.CurAfter = string(*after)
	}

	events, err := api.loaders.PersonalFeedByUserID.Load(params)

	return userID, events, err
}

func (api FeedAPI) PaginateUserFeedByEventID(ctx context.Context, userID persist.DBID, before *string, after *string,
	first *int, last *int) ([]db.FeedEvent, PageInfo, error) {
	// Validate
	if err := validateFields(api.validator, validationMap{
		"userID": {userID, "required"},
	}); err != nil {
		return nil, PageInfo{}, err
	}

	if err := validatePaginationParams(api.validator, before, after, first, last); err != nil {
		return nil, PageInfo{}, err
	}

	queryFunc := func(params timeIDPagingParams) ([]interface{}, error) {
		keys, err := api.queries.PaginateUserFeedByFeedEventID(ctx, db.PaginateUserFeedByFeedEventIDParams{
			OwnerID:       userID,
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

	paginator := timeIDPaginator{
		QueryFunc:  queryFunc,
		CountFunc:  countFunc,
		CursorFunc: cursorFunc,
	}

	results, pageInfo, err := paginator.paginate(before, after, first, last)

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

func (api FeedAPI) GetGlobalFeed(ctx context.Context, before *persist.DBID, after *persist.DBID, first *int, last *int) ([]db.FeedEvent, error) {
	// Validate
	if err := validatePaginationParams(api.validator, first, last); err != nil {
		return nil, err
	}

	params := db.GetGlobalFeedViewBatchParams{}

	if first != nil {
		params.FromFirst = true
		params.Limit = int32(*first)
	}

	if last != nil {
		params.FromFirst = false
		params.Limit = int32(*last)
	}

	if before != nil {
		params.CurBefore = string(*before)
	}

	if after != nil {
		params.CurAfter = string(*after)
	}

	return api.loaders.GlobalFeed.Load(params)
}

func (api FeedAPI) HasPage(ctx context.Context, cursor string, userId persist.DBID, byFirst bool) (bool, error) {
	eventID, err := model.Cursor.DecodeToDBID(&cursor)
	if err != nil {
		return false, err
	}

	if userId != "" {
		return api.queries.PersonalFeedHasMoreEvents(ctx, db.PersonalFeedHasMoreEventsParams{
			Follower:  userId,
			ID:        *eventID,
			FromFirst: byFirst,
		})
	} else {
		return api.queries.GlobalFeedHasMoreEvents(ctx, db.GlobalFeedHasMoreEventsParams{
			ID:        *eventID,
			FromFirst: byFirst,
		})
	}
}
