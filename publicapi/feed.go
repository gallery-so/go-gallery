package publicapi

import (
	heappkg "container/heap"
	"context"
	"database/sql"
	"errors"
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
	"github.com/mikeydub/go-gallery/service/recommend/koala"
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

	var (
		err           error
		paginator     feedPaginator
		entityTypes   []persist.FeedEntityType
		entityIDs     []persist.DBID
		entityIDToPos = make(map[persist.DBID]int)
	)

	hasCursors := before != nil || after != nil

	if !hasCursors {
		calcFunc := func(ctx context.Context) ([]persist.FeedEntityType, []persist.DBID, error) {
			trendData, err := api.queries.FeedEntityScoring(ctx, db.FeedEntityScoringParams{
				WindowEnd:           time.Now().Add(-time.Duration(72 * time.Hour)),
				FeedEventEntityType: int32(persist.FeedEventTypeTag),
				PostEntityType:      int32(persist.PostTypeTag),
				ExcludedFeedActions: []string{string(persist.ActionUserCreated), string(persist.ActionUserFollowedUsers)},
				ExcludeViewer:       false, // Viewer's post can be included in the trending feed
			})
			if err != nil {
				return nil, nil, err
			}

			h := &heap{}
			now := time.Now()

			for _, event := range trendData {
				score := timeFactor(event.CreatedAt, now) * engagementFactor(int(event.Interactions))
				node := feedNode{
					id:    event.ID,
					score: score,
					typ:   persist.FeedEntityType(event.FeedEntityType),
				}

				// Add first 100 numbers in the heap
				if h.Len() < 100 {
					heappkg.Push(h, node)
					continue
				}

				// If the score is greater than the smallest score in the heap, replace it
				if score > (*h)[0].(feedNode).score {
					heappkg.Pop(h)
					heappkg.Push(h, node)
				}
			}

			entityTypes = make([]persist.FeedEntityType, h.Len())
			entityIDs = make([]persist.DBID, h.Len())

			i := h.Len() - 1
			for h.Len() > 0 {
				node := heappkg.Pop(h).(feedNode)
				// Postgres uses 1-based indexing
				entityIDToPos[node.id] = i + 1
				entityTypes[i] = node.typ
				entityIDs[i] = node.id
				i--
			}

			return entityTypes, entityIDs, nil
		}

		l := newFeedCache(api.cache, calcFunc)

		entityTypes, entityIDs, err = l.Load(ctx)
		if err != nil {
			return nil, PageInfo{}, err
		}
	}

	queryFunc := func(params feedPagingParams) ([]any, error) {
		if !hasCursors {
			params.EntityTypes = entityTypes
			params.EntityIDs = entityIDs
		}

		// Restrict cursors to actual size of the slice
		curBeforePos := max(params.CurBeforePos, 0)
		curAfterPos := min(params.CurAfterPos-1, len(params.EntityIDs))

		if params.PagingForward {
			// If paging forward, we extend from the after cursor up to the limit or to the before cursor, whatever is larger
			limitPos := max(curAfterPos-int(params.Limit), curBeforePos)
			params.EntityTypes = params.EntityTypes[limitPos:curAfterPos]
			params.EntityIDs = params.EntityIDs[limitPos:curAfterPos]
		} else {
			// If paging backwards, we extend from the before cursor up to the limit or to the after cursor, whatever is smaller
			limitPos := min(curBeforePos+int(params.Limit), curAfterPos)
			params.EntityTypes = params.EntityTypes[curBeforePos:limitPos]
			params.EntityIDs = params.EntityIDs[curBeforePos:limitPos]
		}

		return loadFeedEntities(ctx, api.loaders, params.EntityTypes, params.EntityIDs)
	}

	cursorFunc := func(node any) (int, []persist.FeedEntityType, []persist.DBID, error) {
		_, id, err := feedCursor(node)
		return entityIDToPos[id], entityTypes, entityIDs, err
	}

	paginator.QueryFunc = queryFunc
	paginator.CursorFunc = cursorFunc
	return paginator.paginate(before, after, first, last)
}

func (api FeedAPI) CuratedFeed(ctx context.Context, before, after *string, first, last *int) ([]any, PageInfo, error) {
	// Validate
	userID, err := getAuthenticatedUserID(ctx)
	if err != nil {
		return nil, PageInfo{}, err
	}

	if err := validatePaginationParams(api.validator, first, last); err != nil {
		return nil, PageInfo{}, err
	}

	var (
		paginator     feedPaginator
		entityTypes   []persist.FeedEntityType
		entityIDs     []persist.DBID
		entityIDToPos = make(map[persist.DBID]int)
	)

	hasCursors := before != nil || after != nil

	if !hasCursors {
		trendData, err := api.queries.FeedEntityScoring(ctx, db.FeedEntityScoringParams{
			WindowEnd:           time.Now().Add(-time.Duration(72 * time.Hour)),
			PostEntityType:      int32(persist.PostTypeTag),
			FeedEventEntityType: int32(persist.FeedEventTypeTag),
			ExcludedFeedActions: []string{string(persist.ActionUserCreated), string(persist.ActionUserFollowedUsers)},
			ExcludeViewer:       true,
			ViewerID:            userID.String(),
		})
		if err != nil {
			return nil, PageInfo{}, err
		}

		h := &heap{}

		eventIDs := make([]persist.DBID, 0)
		tokenIDs := make([]persist.DBID, 0)

		for _, event := range trendData {
			if event.FeedEntityType == int32(persist.FeedEventTypeTag) {
				eventIDs = append(eventIDs, event.ID)
			}
		}

		events, errs := api.loaders.FeedEventByFeedEventID.LoadAll(eventIDs)
		for _, err := range errs {
			if err != nil {
				panic(err)
			}
		}

		eventIDToEvent := make(map[persist.DBID]db.FeedEvent)
		for _, event := range events {
			eventIDToEvent[event.ID] = event
			if event.Data.TokenID != "" {
				tokenIDs = append(tokenIDs, event.Data.TokenID)
			}
			if event.Data.TokenCollectionID != "" {
				tokenIDs = append(tokenIDs, event.Data.TokenCollectionID)
			}
			for _, tokenID := range event.Data.CollectionTokenIDs {
				tokenIDs = append(tokenIDs, tokenID)
			}
			for _, tIDs := range event.Data.GalleryNewCollectionTokenIDs {
				for _, tokenID := range tIDs {
					tokenIDs = append(tokenIDs, tokenID)
				}
			}
		}

		tokens, errs := api.loaders.TokenByTokenID.LoadAll(tokenIDs)
		for _, err := range errs {
			if err != nil && !util.ErrorAs[persist.ErrTokenNotFoundByID](err) {
				panic(err)
			}
		}

		tokenToContractID := make(map[persist.DBID]persist.DBID)
		for _, token := range tokens {
			tokenToContractID[token.ID] = token.Contract
		}

		now := time.Now()
		for _, event := range trendData {
			e := eventIDToEvent[event.ID]
			if event.FeedEntityType == int32(persist.FeedEventTypeTag) {
				if contractID := tokenToContractID[e.Data.TokenID]; contractID != "" {
					event.ContractIds = append(event.ContractIds, contractID)
				}
				if contractID := tokenToContractID[e.Data.TokenCollectionID]; contractID != "" {
					event.ContractIds = append(event.ContractIds, contractID)
				}
				for _, tokenID := range e.Data.CollectionTokenIDs {
					if contractID := tokenToContractID[tokenID]; contractID != "" {
						event.ContractIds = append(event.ContractIds, contractID)
					}
				}
				for _, tIDs := range e.Data.GalleryNewCollectionTokenIDs {
					for _, tokenID := range tIDs {
						if contractID := tokenToContractID[tokenID]; contractID != "" {
							event.ContractIds = append(event.ContractIds, contractID)
						}
					}
				}
			}

			score := scoreFeedEntity(ctx, userID, event, now, int(event.Interactions))

			node := feedNode{
				id:    event.ID,
				score: score,
				typ:   persist.FeedEntityType(event.FeedEntityType),
			}

			// Add first 100 numbers in the heap
			if h.Len() < 100 {
				heappkg.Push(h, node)
				continue
			}

			if score > (*h)[0].(feedNode).score {
				heappkg.Pop(h)
				heappkg.Push(h, node)
			}
		}
		fmt.Printf("took %s to process %d events\n", time.Since(now), len(trendData))

		entityTypes = make([]persist.FeedEntityType, h.Len())
		entityIDs = make([]persist.DBID, h.Len())

		i := h.Len() - 1
		for h.Len() > 0 {
			node := heappkg.Pop(h).(feedNode)
			// Postgres uses 1-based indexing
			entityIDToPos[node.id] = i + 1
			entityTypes[i] = node.typ
			entityIDs[i] = node.id
			i--
		}
	}

	queryFunc := func(params feedPagingParams) ([]any, error) {
		if !hasCursors {
			params.EntityTypes = entityTypes
			params.EntityIDs = entityIDs
		}

		// Restrict cursors to actual size of the slice
		curBeforePos := max(params.CurBeforePos, 0)
		curAfterPos := min(params.CurAfterPos-1, len(params.EntityIDs))

		if params.PagingForward {
			// If paging forward, we extend from the after cursor up to the limit or to the before cursor, whatever is larger
			limitPos := max(curAfterPos-int(params.Limit), curBeforePos)
			params.EntityTypes = params.EntityTypes[limitPos:curAfterPos]
			params.EntityIDs = params.EntityIDs[limitPos:curAfterPos]
		} else {
			// If paging backwards, we extend from the before cursor up to the limit or to the after cursor, whatever is smaller
			limitPos := min(curBeforePos+int(params.Limit), curAfterPos)
			params.EntityTypes = params.EntityTypes[curBeforePos:limitPos]
			params.EntityIDs = params.EntityIDs[curBeforePos:limitPos]
		}

		return loadFeedEntities(ctx, api.loaders, params.EntityTypes, params.EntityIDs)
	}

	cursorFunc := func(node any) (int, []persist.FeedEntityType, []persist.DBID, error) {
		_, id, err := feedCursor(node)
		return entityIDToPos[id], entityTypes, entityIDs, err
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

func feedEntityToTypedType(ctx context.Context, d *dataloader.Loaders, entities []db.FeedEntity) ([]any, error) {
	idTypes := make([]persist.FeedEntityType, len(entities))
	entityIDs := make([]persist.DBID, len(entities))
	for i := 0; i < len(entities); i++ {
		idTypes[i] = persist.FeedEntityType(entities[i].FeedEntityType)
		entityIDs[i] = entities[i].ID
	}
	return loadFeedEntities(ctx, d, idTypes, entityIDs)
}

func loadFeedEntities(ctx context.Context, d *dataloader.Loaders, typs []persist.FeedEntityType, ids []persist.DBID) ([]any, error) {
	if len(typs) != len(ids) {
		panic("length of types and ids must be equal")
	}
	entities := make([]any, len(ids))
	feedEventIDs := make([]persist.DBID, 0, len(ids))
	postIDs := make([]persist.DBID, 0, len(ids))
	for i := 0; i < len(typs); i++ {
		switch persist.FeedEntityType(typs[i]) {
		case persist.FeedEventTypeTag:
			feedEventIDs = append(feedEventIDs, ids[i])
		case persist.PostTypeTag:
			postIDs = append(postIDs, ids[i])
		default:
			return nil, fmt.Errorf("unknown feed entity type %d", typs[i])
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
				if it, ok := idsToFeedEvents[id]; ok {
					entities[j] = it
				}
			}
		case feedPosts := <-incomingFeedPosts:
			idsToFeedPosts := make(map[persist.DBID]db.Post, len(feedPosts))
			for _, evt := range feedPosts {
				idsToFeedPosts[evt.ID] = evt
			}

			for j, id := range ids {
				if it, ok := idsToFeedPosts[id]; ok {
					entities[j] = it
				}
			}
		case err := <-incomingErrors:
			return nil, err
		}
	}

	return entities, nil
}

func scoreFeedEntity(ctx context.Context, viewerID persist.DBID, e db.FeedEntityScoringRow, t time.Time, interactions int) float64 {
	// timeF := timeFactor(e.CreatedAt, t)
	timeF := 1.0
	// engagementF := engagementFactor(interactions)
	engagementF := 1.0
	personalizationF, err := koala.For(ctx).RelevanceTo(viewerID, e)
	if errors.Is(err, koala.ErrNoInputData) {
		// Use a default value of 0.1 so that the post isn't completely penalized because of missing data.
		personalizationF = 0.1
	}
	return timeF * engagementF * personalizationF
}

func ScoreFeedEntity(k *koala.Koala, viewerID persist.DBID, e db.FeedEntityScoringRow, t time.Time, interactions int) float64 {
	// timeF := timeFactor(e.CreatedAt, t)
	timeF := 1.0
	// engagementF := engagementFactor(interactions)
	engagementF := 1.0
	personalizationF, err := k.RelevanceTo(viewerID, e)
	if errors.Is(err, koala.ErrNoInputData) {
		// Use a default value of 0.1 so that the post isn't completely penalized because of missing data.
		personalizationF = 0.1
	}
	return timeF * engagementF * personalizationF
}

func timeFactor(t0, t1 time.Time) float64 {
	lambda := 1.0 / 100_000
	age := t1.Sub(t0).Seconds()
	return 1 * math.Pow(math.E, (-lambda*age))
}

func engagementFactor(interactions int) float64 {
	return 1.0 + float64(interactions)
}

type priorityNode interface {
	Score() float64
}

type feedNode struct {
	id    persist.DBID
	typ   persist.FeedEntityType
	score float64
}

func (f feedNode) Score() float64 {
	return f.score
}

type heap []any

func (h heap) Len() int      { return len(h) }
func (h heap) Swap(i, j int) { h[i], h[j] = h[j], h[i] }
func (h *heap) Push(s any)   { *h = append(*h, s) }

func (h heap) Less(i, j int) bool {
	return h[i].(priorityNode).Score() < h[j].(priorityNode).Score()
}

func (h *heap) Pop() any {
	old := *h
	n := len(old)
	item := old[n-1]
	old[n-1] = nil
	*h = old[:n-1]
	return item
}

type feedPagingParams struct {
	CurBeforePos  int
	CurAfterPos   int
	PagingForward bool
	Limit         int
	EntityTypes   []persist.FeedEntityType
	EntityIDs     []persist.DBID
}

type feedPaginator struct {
	QueryFunc  func(params feedPagingParams) ([]any, error)
	CursorFunc func(node any) (pos int, feedEntityType []persist.FeedEntityType, ids []persist.DBID, err error)
	CountFunc  func() (count int, err error)
}

func (p *feedPaginator) encodeCursor(pos int, typ []persist.FeedEntityType, ids []persist.DBID) (string, error) {
	if len(typ) != len(ids) {
		panic("type and ids must be the same length")
	}
	encoder := newCursorEncoder()
	encoder.appendInt64(int64(pos))
	encoder.appendInt64(int64(len(ids)))
	for i := range typ {
		encoder.appendInt64(int64(typ[i]))
		encoder.appendDBID(ids[i])
	}
	return encoder.AsBase64(), nil
}

func (p *feedPaginator) decodeCursor(cursor string) (pos int, typs []persist.FeedEntityType, ids []persist.DBID, err error) {
	decoder, err := newCursorDecoder(cursor)
	if err != nil {
		return 0, nil, nil, err
	}

	curPos, err := decoder.readInt64()
	if err != nil {
		return 0, nil, nil, err
	}

	totalItems, err := decoder.readInt64()
	if err != nil {
		return 0, nil, nil, err
	}

	typs = make([]persist.FeedEntityType, totalItems)
	ids = make([]persist.DBID, totalItems)

	for i := 0; i < int(totalItems); i++ {
		typ, err := decoder.readInt64()
		if err != nil {
			return 0, nil, nil, err
		}

		id, err := decoder.readDBID()
		if err != nil {
			return 0, nil, nil, err
		}

		typs[i] = persist.FeedEntityType(typ)
		ids[i] = id
	}

	return int(curPos), typs, ids, nil
}

func (p *feedPaginator) paginate(before, after *string, first, last *int) ([]any, PageInfo, error) {
	queryFunc := func(limit int32, pagingForward bool) ([]any, error) {
		args := feedPagingParams{
			Limit:         int(limit),
			CurBeforePos:  defaultCursorBeforePosition,
			CurAfterPos:   defaultCursorAfterPosition,
			PagingForward: pagingForward,
		}

		if before != nil {
			curBeforePos, typs, ids, err := p.decodeCursor(*before)
			if err != nil {
				return nil, err
			}
			args.CurBeforePos = curBeforePos
			args.EntityTypes = typs
			args.EntityIDs = ids
		}

		if after != nil {
			curAfterPos, typs, ids, err := p.decodeCursor(*after)
			if err != nil {
				return nil, err
			}
			args.CurAfterPos = curAfterPos
			args.EntityTypes = typs
			args.EntityIDs = ids
		}

		return p.QueryFunc(args)
	}

	cursorFunc := func(node any) (string, error) {
		pos, typs, ids, err := p.CursorFunc(node)
		if err != nil {
			return "", err
		}
		return p.encodeCursor(pos, typs, ids)
	}

	paginator := keysetPaginator{
		QueryFunc:  queryFunc,
		CursorFunc: cursorFunc,
		CountFunc:  p.CountFunc,
	}

	return paginator.paginate(before, after, first, last)
}

type feedCache struct {
	*redis.LazyCache
	CalcFunc func(context.Context) ([]persist.FeedEntityType, []persist.DBID, error)
}

func newFeedCache(cache *redis.Cache, f func(context.Context) ([]persist.FeedEntityType, []persist.DBID, error)) *feedCache {
	return &feedCache{
		LazyCache: &redis.LazyCache{
			Cache: cache,
			Key:   "trending:feedEvents",
			TTL:   time.Minute * 10,
			CalcFunc: func(ctx context.Context) ([]byte, error) {
				types, ids, err := f(ctx)
				if err != nil {
					return nil, err
				}
				var p feedPaginator
				b, err := p.encodeCursor(0, types, ids)
				return []byte(b), err
			},
		},
	}
}

func (f feedCache) Load(ctx context.Context) ([]persist.FeedEntityType, []persist.DBID, error) {
	b, err := f.LazyCache.Load(ctx)
	if err != nil {
		return nil, nil, err
	}
	var p feedPaginator
	_, types, ids, err := p.decodeCursor(string(b))
	return types, ids, err
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

var (
	cpuprofile string = "koala.prof"
	memprofile string = "koala.mprof"
)
