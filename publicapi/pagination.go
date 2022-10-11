package publicapi

import (
	"encoding/base64"
	"github.com/go-playground/validator/v10"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/validate"
	"time"
)

type PageInfo struct {
	Total           *int
	Size            int
	HasPreviousPage bool
	HasNextPage     bool
	StartCursor     string
	EndCursor       string
}

func validatePaginationParams(validator *validator.Validate, before *string, after *string, first *int, last *int) error {
	if err := validateFields(validator, validationMap{
		"first": {first, "omitempty,gte=0"},
		"last":  {last, "omitempty,gte=0"},
	}); err != nil {
		return err
	}

	if err := validator.Struct(validate.ConnectionPaginationParams{
		Before: before,
		After:  after,
		First:  first,
		Last:   last,
	}); err != nil {
		return err
	}

	return nil
}

// basePaginator is the base pagination struct. You probably don't want to use this directly; use
// a cursor-specific helper like timeIDPaginator.
type basePaginator struct {
	QueryFunc  func(int32, bool) ([]interface{}, error)
	CursorFunc func(interface{}) (string, error)
	CountFunc  func() (int, error)
}

func (p *basePaginator) paginate(before *string, after *string, first *int, last *int) ([]interface{}, PageInfo, error) {
	// Limit is intentionally 1 more than requested, so we can see if there are additional pages
	limit := 1
	if first != nil {
		limit += *first
	} else {
		limit += *last
	}

	// Either first or last will be supplied (but not both). If first isn't nil, we're paging forward!
	pagingForward := first != nil
	results, err := p.QueryFunc(int32(limit), pagingForward)

	if err != nil {
		return nil, PageInfo{}, err
	}

	pageInfo := PageInfo{}

	// We requested 1 more result than the caller asked for. If we got that extra element, remove it
	// from our results and set HasNextPage to true.
	if len(results) == limit {
		results = results[:len(results)-1]
		if first != nil {
			pageInfo.HasNextPage = true
		} else {
			pageInfo.HasPreviousPage = true
		}
	}

	// Reverse the slice if we're paginating backward. Keyset pagination requires our SQL queries to
	// ORDER BY ASC for forward paging and ORDER BY DESC for backward paging, but the Relay pagination
	// spec requires that returned elements should always be in the same order, regardless of whether
	// we're paging forward or backward.
	if last != nil {
		for i, j := 0, len(results)-1; i < j; i, j = i+1, j-1 {
			results[i], results[j] = results[j], results[i]
		}
	}

	// If this is the first query (i.e. no cursors have been supplied), return the total count too
	if before == nil && after == nil {
		total, err := p.CountFunc()
		if err != nil {
			return nil, PageInfo{}, err
		}
		totalInt := int(total)
		pageInfo.Total = &totalInt
	}

	pageInfo.Size = len(results)

	// If there are any results, encode start and end cursors
	if len(results) > 0 {
		firstNode := results[0]
		lastNode := results[len(results)-1]

		pageInfo.StartCursor, err = p.CursorFunc(firstNode)
		if err != nil {
			return nil, PageInfo{}, err
		}

		pageInfo.EndCursor, err = p.CursorFunc(lastNode)
		if err != nil {
			return nil, PageInfo{}, err
		}
	}

	return results, pageInfo, err
}

// timeIDPaginator paginates results using a cursor with time.Time and persist.DBID components
type timeIDPaginator struct {
	// QueryFunc returns paginated results for the given paging parameters
	QueryFunc func(params timeIDPagingParams) ([]interface{}, error)

	// CountFunc returns the total number of items that can be paginated
	CountFunc func() (count int, err error)

	// CursorFunc returns a time and DBID that will be encoded into a cursor string
	CursorFunc func(node interface{}) (time.Time, persist.DBID, error)
}

// timeIDPagingParams are the parameters used to paginate with a time+DBID cursor
type timeIDPagingParams struct {
	Limit            int32
	CursorBeforeTime time.Time
	CursorBeforeID   persist.DBID
	CursorAfterTime  time.Time
	CursorAfterID    persist.DBID
	PagingForward    bool
}

func (p *timeIDPaginator) encodeTimeIDCursor(t time.Time, id persist.DBID) (string, error) {
	timeBytes, err := t.MarshalBinary()
	if err != nil {
		return "", err
	}

	idBytes := []byte(id)

	bytes := make([]byte, 0, 1+len(timeBytes)+len(idBytes))

	// The first byte declares how many bytes the time.Time is
	bytes = append(bytes, byte(len(timeBytes)))

	bytes = append(bytes, timeBytes...)
	bytes = append(bytes, idBytes...)

	return base64.StdEncoding.EncodeToString(bytes), nil
}

func (p *timeIDPaginator) decodeTimeIDCursor(cursor string) (time.Time, persist.DBID, error) {
	bytes, err := base64.StdEncoding.DecodeString(cursor)
	if err != nil || len(bytes) == 0 {
		return time.Time{}, "", errBadCursorFormat
	}

	// The first byte declares how many bytes the time.Time is
	timeLen := int(bytes[0])
	if timeLen > len(bytes)-1 {
		return time.Time{}, "", errBadCursorFormat
	}

	t := time.Time{}
	err = t.UnmarshalBinary(bytes[1 : timeLen+1])
	if err != nil {
		return time.Time{}, "", err
	}

	id := persist.DBID("")

	// It's okay for the ID to be empty
	if len(bytes) > (timeLen + 1) {
		id = persist.DBID(bytes[timeLen+1:])
	}

	return t, id, nil
}

func (p *timeIDPaginator) paginate(before *string, after *string, first *int, last *int) ([]interface{}, PageInfo, error) {
	queryFunc := func(limit int32, pagingForward bool) ([]interface{}, error) {
		curBeforeTime := defaultCursorBeforeTime
		curBeforeID := persist.DBID("")
		curAfterTime := defaultCursorAfterTime
		curAfterID := persist.DBID("")

		var err error
		if before != nil {
			curBeforeTime, curBeforeID, err = p.decodeTimeIDCursor(*before)
			if err != nil {
				return nil, err
			}
		}

		if after != nil {
			curAfterTime, curAfterID, err = p.decodeTimeIDCursor(*after)
			if err != nil {
				return nil, err
			}
		}

		queryParams := timeIDPagingParams{
			Limit:            limit,
			CursorBeforeTime: curBeforeTime,
			CursorBeforeID:   curBeforeID,
			CursorAfterTime:  curAfterTime,
			CursorAfterID:    curAfterID,
			PagingForward:    pagingForward,
		}

		return p.QueryFunc(queryParams)
	}

	cursorFunc := func(node interface{}) (string, error) {
		nodeTime, nodeID, err := p.CursorFunc(node)
		if err != nil {
			return "", err
		}

		return p.encodeTimeIDCursor(nodeTime, nodeID)
	}

	paginator := basePaginator{
		QueryFunc:  queryFunc,
		CursorFunc: cursorFunc,
		CountFunc:  p.CountFunc,
	}

	return paginator.paginate(before, after, first, last)
}
