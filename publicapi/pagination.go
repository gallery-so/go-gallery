package publicapi

import (
	"encoding/base64"
	"time"
	"unicode/utf8"

	"github.com/go-playground/validator/v10"
	"github.com/mikeydub/go-gallery/service/logger"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/validate"
)

type PageInfo struct {
	Total           *int
	Size            int
	HasPreviousPage bool
	HasNextPage     bool
	StartCursor     string
	EndCursor       string
}

func validatePaginationParams(validator *validator.Validate, first *int, last *int) error {
	if err := validateFields(validator, validationMap{
		"first": {first, "omitempty,gte=0"},
		"last":  {last, "omitempty,gte=0"},
	}); err != nil {
		return err
	}

	if err := validator.Struct(validate.ConnectionPaginationParams{
		First: first,
		Last:  last,
	}); err != nil {
		return err
	}

	return nil
}

// keysetPaginator is the base keyset pagination struct. You probably don't want to use this directly;
// use a cursor-specific helper like timeIDPaginator.
// For reasons to favor keyset pagination, see: https://www.citusdata.com/blog/2016/03/30/five-ways-to-paginate/
type keysetPaginator struct {
	// QueryFunc returns paginated results for the given paging parameters
	QueryFunc func(limit int32, pagingForward bool) (nodes []interface{}, err error)

	// CursorFunc returns a cursor string for the given node value
	CursorFunc func(node interface{}) (cursor string, err error)

	// CountFunc returns the total number of items that can be paginated. May be nil, in which
	// case the resulting PageInfo will omit the total field.
	CountFunc func() (count int, err error)
}

func (p *keysetPaginator) paginate(before *string, after *string, first *int, last *int) ([]interface{}, PageInfo, error) {
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
	// use opposite ORDER BY clauses for forward and backward paging, but the Relay pagination spec
	// requires that returned elements should always be in the same order, regardless of whether we're
	// paging forward or backward.
	if !pagingForward {
		for i, j := 0, len(results)-1; i < j; i, j = i+1, j-1 {
			results[i], results[j] = results[j], results[i]
		}
	}

	// If a count function is supplied, fill in pageInfo.Total
	if p.CountFunc != nil {
		total, err := p.CountFunc()
		if err != nil {
			return nil, PageInfo{}, err
		}
		pageInfo.Total = &total
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

// timeIDPaginator paginates results using a cursor with a time.Time and a persist.DBID.
// By using the combination of a timestamp and a unique DBID for our ORDER BY clause,
// we can achieve fast keyset pagination results while avoiding edge cases when multiple
// rows have the same timestamp.
type timeIDPaginator struct {
	// QueryFunc returns paginated results for the given paging parameters
	QueryFunc func(params timeIDPagingParams) ([]interface{}, error)

	// CursorFunc returns a time and DBID that will be encoded into a cursor string
	CursorFunc func(node interface{}) (time.Time, persist.DBID, error)

	// CountFunc returns the total number of items that can be paginated. May be nil, in which
	// case the resulting PageInfo will omit the total field.
	CountFunc func() (count int, err error)
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
	// The first byte declares how many bytes the time.Time is
	bytes, err := marshallTimeBytes(t, id)
	if err != nil {
		return "", err
	}
	idBytes := []byte(id)
	bytes = append(bytes, idBytes...)

	return base64.RawStdEncoding.EncodeToString(bytes), nil
}

func (p *timeIDPaginator) decodeTimeIDCursor(cursor string) (time.Time, persist.DBID, error) {
	bytes, err := base64.RawStdEncoding.DecodeString(cursor)
	if err != nil || len(bytes) == 0 {
		return time.Time{}, "", errBadCursorFormat
	}

	// The first byte declares how many bytes the time.Time is
	t, timeLen, err := parseTime(bytes, 0, err)
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

	paginator := keysetPaginator{
		QueryFunc:  queryFunc,
		CursorFunc: cursorFunc,
		CountFunc:  p.CountFunc,
	}

	return paginator.paginate(before, after, first, last)
}

type boolTimeIDPagingParams struct {
	Limit            int32
	CursorBeforeBool bool
	CursorBeforeTime time.Time
	CursorBeforeID   persist.DBID
	CursorAfterBool  bool
	CursorAfterTime  time.Time
	CursorAfterID    persist.DBID
	PagingForward    bool
}

type boolTimeIDPaginator struct {
	// QueryFunc returns paginated results for the given paging parameters
	QueryFunc func(params boolTimeIDPagingParams) ([]interface{}, error)

	// CursorFunc returns a time and DBID that will be encoded into a cursor string
	CursorFunc func(node interface{}) (bool, time.Time, persist.DBID, error)

	// CountFunc returns the total number of items that can be paginated. May be nil, in which
	// case the resulting PageInfo will omit the total field.
	CountFunc func() (count int, err error)

	// SortPagesAscending determines whether the sort order within a page should be ascending
	// or descending. Defaults to false (descending), since our clients are typically paginating
	// backward.
	SortPagesAscending bool
}

func (p *boolTimeIDPaginator) encodeCursor(b bool, t time.Time, id persist.DBID) (string, error) {
	firstByte := byte(0)
	if b {
		firstByte = 1
	}
	bytes := []byte{firstByte}
	// The first byte declares how many timeBytes the time.Time is
	timeBytes, err := marshallTimeBytes(t, id)
	if err != nil {
		return "", err
	}
	bytes = append(bytes, timeBytes...)
	if id != "" {
		bytes = append(bytes, []byte(id)...)
	}

	valid := utf8.Valid(bytes)
	valid2 := utf8.ValidString(base64.RawStdEncoding.EncodeToString(bytes))

	logger.For(nil).Debugf("encodeCursor: %v %v %v %v %v %v %v", b, t, id, base64.RawStdEncoding.EncodeToString(bytes), bytes, valid, valid2)

	return base64.RawStdEncoding.EncodeToString(bytes), nil
}

func (p *boolTimeIDPaginator) decodeCursor(cursor string) (bool, time.Time, persist.DBID, error) {
	bytes, err := base64.RawStdEncoding.DecodeString(cursor)
	if err != nil || len(bytes) < 2 {
		return false, time.Time{}, "", errBadCursorFormat
	}

	// the first byte is a bool
	b := false
	if bytes[0] > 0 {
		b = true
	}
	// The second byte declares how many bytes the time.Time is
	t, timeLen, err := parseTime(bytes, 1, err)
	if err != nil {
		return false, time.Time{}, "", err
	}

	id := persist.DBID("")

	// It's okay for the ID to be empty
	if len(bytes) > (timeLen + 1) {
		id = persist.DBID(bytes[timeLen+1:])
	}

	logger.For(nil).Debugf("decodeCursor for %s: %v %v %v %v %v %v %v", cursor, b, t, id, base64.RawStdEncoding.EncodeToString(bytes), bytes, len(bytes), timeLen)

	return b, t, id, nil
}

func (p *boolTimeIDPaginator) paginate(before *string, after *string, first *int, last *int) ([]interface{}, PageInfo, error) {
	queryFunc := func(limit int32, pagingForward bool) ([]interface{}, error) {
		curBeforeTime := defaultCursorBeforeTime
		curBeforeID := persist.DBID("")
		curAfterTime := defaultCursorAfterTime
		curAfterID := persist.DBID("")
		curBeforeBool := true
		curAfterBool := false

		var err error
		if before != nil {
			curBeforeBool, curBeforeTime, curBeforeID, err = p.decodeCursor(*before)
			if err != nil {
				return nil, err
			}
		}

		if after != nil {
			curAfterBool, curAfterTime, curAfterID, err = p.decodeCursor(*after)
			if err != nil {
				return nil, err
			}
		}

		queryParams := boolTimeIDPagingParams{
			Limit:            limit,
			CursorBeforeBool: curBeforeBool,
			CursorBeforeTime: curBeforeTime,
			CursorBeforeID:   curBeforeID,
			CursorAfterBool:  curAfterBool,
			CursorAfterTime:  curAfterTime,
			CursorAfterID:    curAfterID,
			PagingForward:    pagingForward,
		}

		return p.QueryFunc(queryParams)
	}

	cursorFunc := func(node interface{}) (string, error) {
		nodeBool, nodeTime, nodeID, err := p.CursorFunc(node)
		if err != nil {
			return "", err
		}

		return p.encodeCursor(nodeBool, nodeTime, nodeID)
	}

	paginator := keysetPaginator{
		QueryFunc:  queryFunc,
		CursorFunc: cursorFunc,
		CountFunc:  p.CountFunc,
	}

	return paginator.paginate(before, after, first, last)
}

func parseTime(bytes []byte, idx int, err error) (time.Time, int, error) {
	timeSlice := bytes[idx:]
	timeLen := int(timeSlice[0])
	if timeLen > len(timeSlice)-1 {
		return time.Time{}, 0, errBadCursorFormat
	}

	t := time.Time{}
	err = t.UnmarshalBinary(timeSlice[1 : timeLen+1])
	if err != nil {
		return time.Time{}, 0, err
	}
	return t, len(timeSlice[1:timeLen+1]) + 1, nil
}

func marshallTimeBytes(t time.Time, id persist.DBID) ([]byte, error) {
	timeBytes, err := t.MarshalBinary()
	if err != nil {
		return nil, err
	}

	idBytes := []byte(id)

	bytes := make([]byte, 0, 1+len(timeBytes)+len(idBytes))

	bytes = append(bytes, byte(len(timeBytes)))

	bytes = append(bytes, timeBytes...)
	return bytes, nil
}
