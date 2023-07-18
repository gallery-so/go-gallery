package publicapi

import (
	heappkg "container/heap"
	"context"
	"database/sql"
	"fmt"
	"math"
	"time"

	"github.com/mikeydub/go-gallery/service/persist/postgres"
	"github.com/mikeydub/go-gallery/service/redis"
	"github.com/mikeydub/go-gallery/validate"

	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/go-playground/validator/v10"
	db "github.com/mikeydub/go-gallery/db/gen/coredb"
	"github.com/mikeydub/go-gallery/graphql/dataloader"
	"github.com/mikeydub/go-gallery/graphql/model"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/service/recommend"
	"github.com/mikeydub/go-gallery/util"
)

type FeedAPI struct {
	repos     *postgres.Repositories
	queries   *db.Queries
	loaders   *dataloader.Loaders
	validator *validator.Validate
	ethClient *ethclient.Client
	cache     *redis.Cache
}

func (api FeedAPI) BlockUser(ctx context.Context, userId persist.DBID, action persist.Action) error {
	// Validate
	err := validate.ValidateFields(api.validator, validate.ValidationMap{
		"userId": validate.WithTag(userId, "required"),
		"action": validate.WithTag(action, "required"),
	})

	if err != nil {
		return err
	}

	return api.queries.BlockUserFromFeed(ctx, db.BlockUserFromFeedParams{
		ID:     persist.GenerateID(),
		UserID: userId,
		Action: action,
	})

}

func (api FeedAPI) UnBlockUser(ctx context.Context, userId persist.DBID) error {
	// Validate
	err := validate.ValidateFields(api.validator, validate.ValidationMap{
		"userId": validate.WithTag(userId, "required"),
	})

	if err != nil {
		return err
	}

	return api.queries.UnblockUserFromFeed(ctx, userId)

}

func (api FeedAPI) GetFeedEventById(ctx context.Context, feedEventID persist.DBID) (*db.FeedEvent, error) {
	// Validate
	if err := validate.ValidateFields(api.validator, validate.ValidationMap{
		"feedEventID": validate.WithTag(feedEventID, "required"),
	}); err != nil {
		return nil, err
	}

	event, err := api.loaders.FeedEventByFeedEventID.Load(feedEventID)
	if err != nil {
		return nil, err
	}

	return &event, nil
}

func (api FeedAPI) GetPostById(ctx context.Context, postID persist.DBID) (*db.Post, error) {
	// Validate
	if err := validate.ValidateFields(api.validator, validate.ValidationMap{
		"feedEventID": validate.WithTag(postID, "required"),
	}); err != nil {
		return nil, err
	}

	post, err := api.loaders.PostByPostID.Load(postID)
	if err != nil {
		return nil, err
	}

	return &post, nil
}

func (api FeedAPI) PostTokens(ctx context.Context, tokenIDs []persist.DBID, caption *string) (persist.DBID, error) {
	// Validate
	if err := validate.ValidateFields(api.validator, validate.ValidationMap{
		"tokenIDs": validate.WithTag(tokenIDs, "required"),
		// caption can be null but less than 600 chars
		"caption": validate.WithTag(caption, "max=600"),
	}); err != nil {
		return "", err
	}
	actorID, err := getAuthenticatedUserID(ctx)
	if err != nil {
		return "", err
	}

	var cap sql.NullString
	if caption != nil {
		cap = sql.NullString{
			String: *caption,
			Valid:  true,
		}
	}

	contracts, err := api.queries.GetContractsByTokenIDs(ctx, tokenIDs)
	if err != nil {
		return "", err
	}

	contractIDs, _ := util.Map(contracts, func(c db.Contract) (persist.DBID, error) {
		return c.ID, nil
	})

	id, err := api.queries.InsertPost(ctx, db.InsertPostParams{
		ID:          persist.GenerateID(),
		TokenIds:    tokenIDs,
		ContractIds: contractIDs,
		ActorID:     actorID,
		Caption:     cap,
	})
	if err != nil {
		return "", err
	}
	return id, nil
}

func (api FeedAPI) DeletePostById(ctx context.Context, postID persist.DBID) error {
	// Validate
	if err := validate.ValidateFields(api.validator, validate.ValidationMap{
		"postID": validate.WithTag(postID, "required"),
	}); err != nil {
		return err
	}

	err := api.queries.DeletePostByID(ctx, postID)
	if err != nil {
		return err
	}

	return nil
}

func (api FeedAPI) GetRawEventById(ctx context.Context, eventID persist.DBID) (*db.Event, error) {
	// Validate
	if err := validate.ValidateFields(api.validator, validate.ValidationMap{
		"eventID": validate.WithTag(eventID, "required"),
	}); err != nil {
		return nil, err
	}

	event, err := api.queries.GetEvent(ctx, eventID)
	if err != nil {
		return nil, err
	}

	return &event, nil
}

func (api FeedAPI) PersonalFeed(ctx context.Context, before *string, after *string, first *int, last *int) ([]any, PageInfo, error) {
	userID, err := getAuthenticatedUserID(ctx)
	if err != nil {
		return nil, PageInfo{}, err
	}

	// Validate
	if err := validate.ValidateFields(api.validator, validate.ValidationMap{
		"userID": validate.WithTag(userID, "required"),
	}); err != nil {
		return nil, PageInfo{}, err
	}

	if err := validatePaginationParams(api.validator, first, last); err != nil {
		return nil, PageInfo{}, err
	}

	queryFunc := func(params timeIDPagingParams) ([]interface{}, error) {
		keys, err := api.queries.PaginatePersonalFeedByUserID(ctx, db.PaginatePersonalFeedByUserIDParams{
			Follower:      userID,
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

		return feedEntityToTypedType(ctx, api.loaders, keys)
	}

	paginator := timeIDPaginator{
		QueryFunc:  queryFunc,
		CursorFunc: feedCursor,
	}

	return paginator.paginate(before, after, first, last)
}

func (api FeedAPI) UserFeed(ctx context.Context, userID persist.DBID, before *string, after *string,
	first *int, last *int) ([]any, PageInfo, error) {
	// Validate
	if err := validate.ValidateFields(api.validator, validate.ValidationMap{
		"userID": validate.WithTag(userID, "required"),
	}); err != nil {
		return nil, PageInfo{}, err
	}

	if err := validatePaginationParams(api.validator, first, last); err != nil {
		return nil, PageInfo{}, err
	}

	queryFunc := func(params timeIDPagingParams) ([]interface{}, error) {
		keys, err := api.queries.PaginateUserFeedByUserID(ctx, db.PaginateUserFeedByUserIDParams{
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

		return feedEntityToTypedType(ctx, api.loaders, keys)
	}

	paginator := timeIDPaginator{
		QueryFunc:  queryFunc,
		CursorFunc: feedCursor,
	}

	return paginator.paginate(before, after, first, last)
}

func (api FeedAPI) GlobalFeed(ctx context.Context, before *string, after *string, first *int, last *int) ([]any, PageInfo, error) {
	// Validate
	if err := validatePaginationParams(api.validator, first, last); err != nil {
		return nil, PageInfo{}, err
	}

	queryFunc := func(params timeIDPagingParams) ([]interface{}, error) {
		keys, err := api.queries.PaginateGlobalFeed(ctx, db.PaginateGlobalFeedParams{
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

		return feedEntityToTypedType(ctx, api.loaders, keys)
	}

	paginator := timeIDPaginator{
		QueryFunc:  queryFunc,
		CursorFunc: feedCursor,
	}

	return paginator.paginate(before, after, first, last)
}

func (api FeedAPI) TrendingFeed(ctx context.Context, before *string, after *string, first *int, last *int) ([]any, PageInfo, error) {
	// Validate
	if err := validatePaginationParams(api.validator, first, last); err != nil {
		return nil, PageInfo{}, err
	}

	var err error
	var trendingIDs []persist.DBID
	var paginator = positionPaginator{}
	var idToCursorPos = make(map[persist.DBID]int)

	// If a cursor is provided, we can skip querying the cache
	if before != nil {
		if _, trendingIDs, err = paginator.decodeCursor(*before); err != nil {
			return nil, PageInfo{}, err
		}
	} else if after != nil {
		if _, trendingIDs, err = paginator.decodeCursor(*after); err != nil {
			return nil, PageInfo{}, err
		}
	} else {
		calcFunc := func(ctx context.Context) ([]persist.DBID, error) {
			entities, err := api.queries.GetLatestFeedEntities(ctx, db.GetLatestFeedEntitiesParams{
				WindowEnd:           time.Now().Add(-time.Duration(72 * time.Hour)),
				FeedEntityType:      persist.FeedEventTypeTag,
				PostEntityType:      persist.PostTypeTag,
				ExcludedFeedActions: []string{string(persist.ActionUserCreated), string(persist.ActionUserFollowedUsers)},
			})
			if err != nil {
				return nil, err
			}

			h := heap{}
			now := time.Now()

			for _, e := range entities {
				score := timeFactor(now, e.CreatedAt) * float64(e.Interactions)
				node := heapItem{id: e.ID, score: score}

				// Add first 100 numbers in the heap
				if len(h) < 100 {
					heappkg.Push(&h, node)
					continue
				}

				// If the score is greater than the smallest score in the heap, replace it
				if score > h[0].(heapItem).score {
					heappkg.Pop(&h)
					heappkg.Push(&h, node)
				}
			}

			trendingIDs := make([]persist.DBID, 100)

			for len(h) > 0 {
				node := heappkg.Pop(&h).(heapItem)
				trendingIDs = append(trendingIDs, node.id)
			}

			return trendingIDs, nil
		}

		l := newDBIDCache(api.cache, "trending:feedEvents", time.Minute*10, calcFunc)

		trendingIDs, err = l.Load(ctx)
		if err != nil {
			return nil, PageInfo{}, err
		}
	}

	idsAsStr := make([]string, len(trendingIDs))
	for i, id := range trendingIDs {
		// Postgres uses 1-based indexing
		idToCursorPos[id] = i + 1
		idsAsStr[i] = id.String()
	}

	queryFunc := func(params positionPagingParams) ([]any, error) {
		keys, err := api.queries.PaginateFeedEntities(ctx, db.PaginateFeedEntitiesParams{
			FeedEntityIds: idsAsStr,
			CurBeforePos:  params.CursorBeforePos,
			CurAfterPos:   params.CursorAfterPos,
			PagingForward: params.PagingForward,
			Limit:         params.Limit,
		})
		if err != nil {
			return nil, err
		}
		return feedEntityToTypedType(ctx, api.loaders, keys)
	}

	cursorFunc := func(node any) (int, []persist.DBID, error) {
		_, id, err := feedCursor(node)
		return idToCursorPos[id], trendingIDs, err
	}

	paginator.QueryFunc = queryFunc
	paginator.CursorFunc = cursorFunc
	return paginator.paginate(before, after, first, last)
}

func (api FeedAPI) CuratedFeed(ctx context.Context, userID persist.DBID, before, after *string, first, last *int) ([]any, PageInfo, error) {
	// Validate
	if err := validate.ValidateFields(api.validator, validate.ValidationMap{
		"userID": validate.WithTag(userID, "required"),
	}); err != nil {
		return nil, PageInfo{}, err
	}

	if err := validatePaginationParams(api.validator, first, last); err != nil {
		return nil, PageInfo{}, err
	}

	// TODO: Support feed events too
	posts, err := api.queries.GetLatestPosts(ctx, db.GetLatestPostsParams{
		WindowEnd: time.Now().Add(-time.Duration(72 * time.Hour)),
	})
	if err != nil {
		return nil, PageInfo{}, err
	}

	h := heap{}
	now := time.Now()

	var paginator = positionPaginator{}
	var topPosts []persist.DBID
	var idToCursorPos = make(map[persist.DBID]int)

	for _, p := range posts {
		timeF := timeFactor(now, p.Post.CreatedAt)
		engagementF := float64(p.Interactions)
		personalizationF := personalizationFactor(ctx, nil, userID, p.Post)
		score := timeF * engagementF * personalizationF
		node := heapItem{id: p.Post.ID, score: score}

		if len(h) < 100 {
			heappkg.Push(&h, heapItem{id: p.Post.ID, score: score})
			continue
		}

		// If the score is greater than the smallest score in the heap, replace it
		if score > h[0].(heapItem).score {
			heappkg.Pop(&h)
			heappkg.Push(&h, node)
		}

		topPosts = make([]persist.DBID, len(h))

		for len(h) > 0 {
			node := heappkg.Pop(&h).(heapItem)
			topPosts = append(topPosts, node.id)
		}
	}

	idsAsStr := make([]string, len(topPosts))
	for i, id := range topPosts {
		// Postgres uses 1-based indexing
		idToCursorPos[id] = i + 1
		idsAsStr[i] = id.String()
	}

	queryFunc := func(params positionPagingParams) ([]any, error) {
		keys, err := api.queries.PaginateFeedEntities(ctx, db.PaginateFeedEntitiesParams{
			FeedEntityIds: idsAsStr,
			CurBeforePos:  params.CursorBeforePos,
			CurAfterPos:   params.CursorAfterPos,
			PagingForward: params.PagingForward,
			Limit:         params.Limit,
		})
		if err != nil {
			return nil, err
		}
		return feedEntityToTypedType(ctx, api.loaders, keys)
	}

	cursorFunc := func(node any) (int, []persist.DBID, error) {
		_, id, err := feedCursor(node)
		return idToCursorPos[id], topPosts, err
	}

	paginator.QueryFunc = queryFunc
	paginator.CursorFunc = cursorFunc
	return paginator.paginate(before, after, first, last)
}

func (api FeedAPI) TrendingUsers(ctx context.Context, report model.Window) ([]db.User, error) {
	ttl := time.Hour

	// Reports that span a week or greater are calculated once every 24 hours rather than once an hour.
	if report.Duration > 7*24*time.Hour {
		ttl *= 24
	}

	calcFunc := func(ctx context.Context) ([]persist.DBID, error) {
		return api.queries.GetAllTimeTrendingUserIDs(ctx, 24)
	}

	if report.Name != "ALL_TIME" {
		calcFunc = func(ctx context.Context) ([]persist.DBID, error) {
			return api.queries.GetWindowedTrendingUserIDs(ctx, db.GetWindowedTrendingUserIDsParams{
				WindowEnd: time.Now().Add(-time.Duration(report.Duration)),
				Limit:     24,
			})
		}
	}

	l := newDBIDCache(api.cache, "trending:users:"+report.Name, ttl, calcFunc)

	ids, err := l.Load(ctx)
	if err != nil {
		return nil, err
	}

	asStr, _ := util.Map(ids, func(id persist.DBID) (string, error) {
		return id.String(), nil
	})

	return api.queries.GetTrendingUsersByIDs(ctx, asStr)
}

func feedCursor(i interface{}) (time.Time, persist.DBID, error) {
	switch row := i.(type) {
	case db.FeedEvent:
		return row.EventTime, row.ID, nil
	case db.Post:
		return row.CreatedAt, row.ID, nil
	}
	return time.Time{}, "", fmt.Errorf("interface{} is not a feed entity: %T", i)
}

func feedEntityToTypedType(ctx context.Context, d *dataloader.Loaders, ids []db.FeedEntity) ([]any, error) {
	entities := make([]any, len(ids))
	feedEventIDs := make([]persist.DBID, 0, len(ids))
	postIDs := make([]persist.DBID, 0, len(ids))
	for _, id := range ids {
		switch id.FeedEntityType {
		case persist.FeedEventTypeTag:
			feedEventIDs = append(feedEventIDs, id.ID)
		case persist.PostTypeTag:
			postIDs = append(postIDs, id.ID)
		default:
			return nil, fmt.Errorf("unknown feed entity type %d", id.FeedEntityType)
		}
	}

	incomingFeedEvents := make(chan []db.FeedEvent)
	incomingFeedPosts := make(chan []db.Post)
	incomingErrors := make(chan error)

	go func() {
		feedEvents, errs := d.FeedEventByFeedEventID.LoadAll(feedEventIDs)
		for _, err := range errs {
			if err != nil {
				incomingErrors <- err
				return
			}
		}
		incomingFeedEvents <- feedEvents
	}()

	go func() {
		feedPosts, errs := d.PostByPostID.LoadAll(postIDs)
		for _, err := range errs {
			if err != nil {
				incomingErrors <- err
				return
			}
		}
		incomingFeedPosts <- feedPosts
	}()

	for i := 0; i < 2; i++ {
		select {
		case feedEvents := <-incomingFeedEvents:
			idsToFeedEvents := make(map[persist.DBID]db.FeedEvent, len(feedEvents))
			for _, evt := range feedEvents {
				idsToFeedEvents[evt.ID] = evt
			}

			for j, id := range ids {
				if it, ok := idsToFeedEvents[id.ID]; ok {
					entities[j] = it
				}
			}
		case feedPosts := <-incomingFeedPosts:
			idsToFeedPosts := make(map[persist.DBID]db.Post, len(feedPosts))
			for _, evt := range feedPosts {
				idsToFeedPosts[evt.ID] = evt
			}

			for j, id := range ids {
				if it, ok := idsToFeedPosts[id.ID]; ok {
					entities[j] = it
				}
			}
		case err := <-incomingErrors:
			return nil, err
		}
	}

	return entities, nil
}

func timeFactor(t0, t1 time.Time) float64 {
	lambda := 1.0 / 100_000
	age := t0.Sub(t1).Seconds()
	return 1 * math.Pow(math.E, (-lambda*age))
}

func personalizationFactor(ctx context.Context, p *recommend.Personalization, userID persist.DBID, post db.Post) (s float64) {
	if len(post.ContractIds) == 0 {
		return 1
	}
	for _, contractID := range post.ContractIds {
		r := p.RelevanceTo(ctx, userID, post.ActorID, contractID)
		if r > s {
			s = r
		}
	}
	return s
}

type heapItem struct {
	id    persist.DBID
	score float64
}

type heap []any

func (h heap) Len() int      { return len(h) }
func (h heap) Swap(i, j int) { h[i], h[j] = h[j], h[i] }
func (h *heap) Push(s any)   { *h = append(*h, s) }

func (h heap) Less(i, j int) bool {
	// We want Pop to give us the highest, not lowest, score so we use greater than here.
	return h[i].(heapItem).score > h[j].(heapItem).score
}

func (h *heap) Pop() any {
	old := *h
	n := len(old)
	item := old[n-1]
	old[n-1] = nil
	*h = old[:n-1]
	return item
}
