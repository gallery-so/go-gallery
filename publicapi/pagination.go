package publicapi

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/go-playground/validator/v10"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/validate"
)

var (
	defaultCursorBeforeID = persist.DBID("")
	defaultCursorAfterID  = persist.DBID("")

	// Some date that comes after any other valid timestamps in our database
	defaultCursorBeforeTime = time.Date(3000, 1, 1, 1, 1, 1, 1, time.UTC)
	// Some date that comes before any other valid timestamps in our database
	defaultCursorAfterTime = time.Date(1970, 1, 1, 1, 1, 1, 1, time.UTC)

	// Some value that comes after any other sequence of characters
	defaultCursorBeforeKey = strings.Repeat("Z", 255)
	// Some value that comes before any other sequence of characters
	defaultCursorAfterKey = ""

	// Some position that comes after any other position
	defaultCursorBeforePosition = math.MaxInt32 - 1
	// Some position that comes before any other position
	defaultCursorAfterPosition = -1

	defaultCursorBeforeFloat = math.MaxFloat64
	defaultCursorAfterFloat  = -math.MaxFloat64
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
	if err := validate.ValidateFields(validator, validate.ValidationMap{
		"first": validate.WithTag(first, "omitempty,gte=0"),
		"last":  validate.WithTag(last, "omitempty,gte=0"),
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

func pageFrom[Node any, K cursor](allEdges []Node, countF func() (int, error), cF cursorable[Node, K], before, after *string, first, last *int) ([]Node, PageInfo, error) {
	cursorEdges, err := applyCursors(allEdges, cF, before, after)
	if err != nil {
		return nil, PageInfo{}, err
	}

	edgesPaged, err := pageEdgesFrom(cursorEdges, before, after, first, last)
	if err != nil {
		return nil, PageInfo{}, err
	}

	pageInfo, err := pageInfoFrom(cursorEdges, edgesPaged, countF, cF, before, after, first, last)
	return edgesPaged, pageInfo, err
}

func packNode[Node any, K cursor](cF cursorable[Node, K], node Node) (string, error) {
	cursor, err := cF(node)
	if err != nil {
		return "", err
	}
	return cursor.Pack()
}

func pageInfoFrom[Node any, K cursor](cursorEdges, edgesPaged []Node, countF func() (int, error), cF cursorable[Node, K], before, after *string, first, last *int) (pageInfo PageInfo, err error) {
	if len(edgesPaged) > 0 {
		firstNode := edgesPaged[0]
		lastNode := edgesPaged[len(edgesPaged)-1]

		pageInfo.StartCursor, err = packNode(cF, firstNode)
		if err != nil {
			return PageInfo{}, err
		}

		pageInfo.EndCursor, err = packNode(cF, lastNode)
		if err != nil {
			return PageInfo{}, err
		}
	}

	if last != nil {
		pageInfo.HasPreviousPage = len(cursorEdges) > *last
	}

	if first != nil {
		pageInfo.HasNextPage = len(cursorEdges) > *first
	}

	if countF != nil {
		total, err := countF()
		if err != nil {
			return PageInfo{}, err
		}
		pageInfo.Total = &total
	}

	pageInfo.Size = len(edgesPaged)

	return pageInfo, nil
}

func pageEdgesFrom[Node any](edges []Node, before, after *string, first, last *int) ([]Node, error) {
	if first != nil && len(edges) > *first {
		return edges[:*first], nil
	}

	if last != nil && len(edges) > *last {
		return edges[len(edges)-*last:], nil
	}

	return edges, nil
}

func applyCursors[Node any, K cursor](allEdges []Node, cF cursorable[Node, K], before, after *string) ([]Node, error) {
	edges := append([]Node{}, allEdges...)

	if after != nil {
		for i, edge := range edges {
			cur, err := packNode(cF, edge)
			if err != nil {
				return nil, err
			}
			if cur == *after {
				edges = edges[i+1:]
				break
			}
		}
	}

	if before != nil {
		for i, edge := range edges {
			cur, err := packNode(cF, edge)
			if err != nil {
				return nil, err
			}
			if cur == *before {
				edges = edges[:i]
				break
			}
		}
	}

	return edges, nil
}

// keysetPaginator is the base keyset pagination struct. You probably don't want to use this directly;
// use a cursor-specific helper like timeIDPaginator.
// For reasons to favor keyset pagination, see: https://www.citusdata.com/blog/2016/03/30/five-ways-to-paginate/
type keysetPaginator[Node any, Cur cursor] struct {
	// QueryFunc returns paginated results for the given paging parameters
	QueryFunc func(limit int32, pagingForward bool) (nodes []Node, err error)

	// Cursorable produces a cursor for encoding nodes to cursor strings
	Cursorable cursorable[Node, Cur]

	// CountFunc returns the total number of items that can be paginated. May be nil, in which
	// case the resulting PageInfo will omit the total field.
	CountFunc func() (count int, err error)
}

func (p *keysetPaginator[Node, Cur]) paginate(before *string, after *string, first *int, last *int) ([]Node, PageInfo, error) {
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

	// Reverse the slice if we're paginating backward. Keyset pagination requires our SQL queries to
	// use opposite ORDER BY clauses for forward and backward paging, but the Relay pagination spec
	// requires that returned elements should always be in the same order, regardless of whether we're
	// paging forward or backward.
	if !pagingForward {
		for i, j := 0, len(results)-1; i < j; i, j = i+1, j-1 {
			results[i], results[j] = results[j], results[i]
		}
	}

	return pageFrom(results, p.CountFunc, p.Cursorable, before, after, first, last)
}

// timeIDPaginator paginates results using a cursor with a time.Time and a persist.DBID.
// By using the combination of a timestamp and a unique DBID for our ORDER BY clause,
// we can achieve fast keyset pagination results while avoiding edge cases when multiple
// rows have the same timestamp.
type timeIDPaginator[Node any] struct {
	// QueryFunc returns paginated results for the given paging parameters
	QueryFunc func(params timeIDPagingParams) ([]Node, error)

	// CursorFunc returns a time and DBID that will be encoded into a cursor string
	CursorFunc func(node Node) (time.Time, persist.DBID, error)

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

func (p *timeIDPaginator[Node]) paginate(before *string, after *string, first *int, last *int) ([]Node, PageInfo, error) {
	queryFunc := func(limit int32, pagingForward bool) ([]Node, error) {
		beforeCur := cursors.NewTimeIDCursor()
		beforeCur.Time = defaultCursorBeforeTime
		beforeCur.ID = defaultCursorBeforeID
		afterCur := cursors.NewTimeIDCursor()
		afterCur.Time = defaultCursorAfterTime
		afterCur.ID = defaultCursorAfterID

		if before != nil {
			if err := beforeCur.Unpack(*before); err != nil {
				return nil, err
			}
		}

		if after != nil {
			if err := afterCur.Unpack(*after); err != nil {
				return nil, err
			}
		}

		queryParams := timeIDPagingParams{
			Limit:            limit,
			CursorBeforeTime: beforeCur.Time,
			CursorBeforeID:   beforeCur.ID,
			CursorAfterTime:  afterCur.Time,
			CursorAfterID:    afterCur.ID,
			PagingForward:    pagingForward,
		}

		return p.QueryFunc(queryParams)
	}

	paginator := keysetPaginator[Node, *timeIDCursor]{
		QueryFunc:  queryFunc,
		Cursorable: newTimeIDCursor(p.CursorFunc),
		CountFunc:  p.CountFunc,
	}

	return paginator.paginate(before, after, first, last)
}

// floatIDPaginator paginates results using a cursor with a float and a persist.DBID.
// By using the combination of a float and a unique DBID for our ORDER BY clause,
// we can achieve fast keyset pagination results while avoiding edge cases when multiple
// rows have the same float value.
type floatIDPaginator[Node any] struct {
	// QueryFunc returns paginated results for the given paging parameters
	QueryFunc func(params floatIDPagingParams) ([]Node, error)

	// CursorFunc returns a float and DBID that will be encoded into a cursor string
	CursorFunc func(node Node) (float64, persist.DBID, error)

	// CountFunc returns the total number of items that can be paginated. May be nil, in which
	// case the resulting PageInfo will omit the total field.
	CountFunc func() (count int, err error)
}

// floatIDPagingParams are the parameters used to paginate with a float+DBID cursor
type floatIDPagingParams struct {
	Limit             int32
	CursorBeforeFloat float64
	CursorBeforeID    persist.DBID
	CursorAfterFloat  float64
	CursorAfterID     persist.DBID
	PagingForward     bool
}

func (p *floatIDPaginator[Node]) paginate(before *string, after *string, first *int, last *int) ([]Node, PageInfo, error) {
	queryFunc := func(limit int32, pagingForward bool) ([]Node, error) {
		beforeCur := cursors.NewFloatIDCursor()
		beforeCur.Float = defaultCursorBeforeFloat
		beforeCur.ID = defaultCursorBeforeID
		afterCur := cursors.NewFloatIDCursor()
		afterCur.Float = defaultCursorAfterFloat
		afterCur.ID = defaultCursorAfterID

		if before != nil {
			if err := beforeCur.Unpack(*before); err != nil {
				return nil, err
			}
		}

		if after != nil {
			if err := afterCur.Unpack(*after); err != nil {
				return nil, err
			}
		}

		queryParams := floatIDPagingParams{
			Limit:             limit,
			CursorBeforeFloat: beforeCur.Float,
			CursorBeforeID:    beforeCur.ID,
			CursorAfterFloat:  afterCur.Float,
			CursorAfterID:     afterCur.ID,
			PagingForward:     pagingForward,
		}

		return p.QueryFunc(queryParams)
	}

	paginator := keysetPaginator[Node, *floatIDCursor]{
		QueryFunc:  queryFunc,
		Cursorable: newFloatIDCursor(p.CursorFunc),
		CountFunc:  p.CountFunc,
	}

	return paginator.paginate(before, after, first, last)
}

type sharedFollowersPaginator[Node any] struct{ timeIDPaginator[Node] }

func (p *sharedFollowersPaginator[Node]) paginate(before *string, after *string, first *int, last *int) ([]Node, PageInfo, error) {
	queryFunc := func(limit int32, pagingForward bool) ([]Node, error) {
		// The shared followers query orders results in descending order when
		// paging forward (vs. ascending order which is more typical).
		beforeCur := cursors.NewTimeIDCursor()
		beforeCur.Time = time.Date(1970, 1, 1, 1, 1, 1, 1, time.UTC)
		beforeCur.ID = defaultCursorBeforeID
		afterCur := cursors.NewTimeIDCursor()
		afterCur.Time = time.Date(3000, 1, 1, 1, 1, 1, 1, time.UTC)
		afterCur.ID = defaultCursorAfterID

		if before != nil {
			if err := beforeCur.Unpack(*before); err != nil {
				return nil, err
			}
		}

		if after != nil {
			if err := afterCur.Unpack(*after); err != nil {
				return nil, err
			}
		}

		queryParams := timeIDPagingParams{
			Limit:            limit,
			CursorBeforeTime: beforeCur.Time,
			CursorBeforeID:   beforeCur.ID,
			CursorAfterTime:  afterCur.Time,
			CursorAfterID:    afterCur.ID,
			PagingForward:    pagingForward,
		}

		return p.QueryFunc(queryParams)
	}

	paginator := keysetPaginator[Node, *timeIDCursor]{
		QueryFunc:  queryFunc,
		Cursorable: newTimeIDCursor(p.CursorFunc),
		CountFunc:  p.CountFunc,
	}

	return paginator.paginate(before, after, first, last)
}

type sharedCommunitiesPaginatorParams struct {
	Limit                        int32
	CursorBeforeDisplayedByUserA bool
	CursorBeforeDisplayedByUserB bool
	CursorBeforeOwnedCount       int
	CursorBeforeCommunityID      persist.DBID
	CursorAfterDisplayedByUserA  bool
	CursorAfterDisplayedByUserB  bool
	CursorAfterOwnedCount        int
	CursorAfterCommunityID       persist.DBID
	PagingForward                bool
}

type sharedCommunitiesPaginator[Node any] struct {
	// QueryFunc returns paginated results for the given paging parameters
	QueryFunc func(params sharedCommunitiesPaginatorParams) ([]Node, error)

	// CursorFunc returns:
	//  * A bool indicating that userA displays the community on their gallery
	//  * A bool indicating that userB displays the community on their gallery
	//  * An int indicating how many tokens userA owns for a community
	//  * A DBID indicating the ID of the community
	CursorFunc func(node Node) (bool, bool, int64, persist.DBID, error)

	// CountFunc returns the total number of items that can be paginated. May be nil, in which
	// case the resulting PageInfo will omit the total field.
	CountFunc func() (count int, err error)
}

func (p *sharedCommunitiesPaginator[Node]) paginate(before *string, after *string, first *int, last *int) ([]Node, PageInfo, error) {
	queryFunc := func(limit int32, pagingForward bool) ([]Node, error) {
		beforeCur := cursors.NewBoolBoolIntIDCursor()
		beforeCur.Bool1 = false
		beforeCur.Bool2 = false
		beforeCur.Int = -1
		beforeCur.ID = defaultCursorBeforeID
		afterCur := cursors.NewBoolBoolIntIDCursor()
		afterCur.Bool1 = true
		afterCur.Bool2 = true
		afterCur.Int = math.MaxInt32
		afterCur.ID = defaultCursorAfterID

		if before != nil {
			if err := beforeCur.Unpack(*before); err != nil {
				return nil, err
			}
		}

		if after != nil {
			if err := afterCur.Unpack(*after); err != nil {
				return nil, err
			}
		}

		queryParams := sharedCommunitiesPaginatorParams{
			Limit:                        limit,
			CursorBeforeDisplayedByUserA: beforeCur.Bool1,
			CursorBeforeDisplayedByUserB: beforeCur.Bool2,
			CursorBeforeOwnedCount:       int(beforeCur.Int),
			CursorBeforeCommunityID:      beforeCur.ID,
			CursorAfterDisplayedByUserA:  afterCur.Bool1,
			CursorAfterDisplayedByUserB:  afterCur.Bool2,
			CursorAfterOwnedCount:        int(afterCur.Int),
			CursorAfterCommunityID:       afterCur.ID,
			PagingForward:                pagingForward,
		}

		return p.QueryFunc(queryParams)
	}

	paginator := keysetPaginator[Node, *boolBoolIntIDCursor]{
		QueryFunc:  queryFunc,
		Cursorable: newBoolBoolIntIDCursor(p.CursorFunc),
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

type boolTimeIDPaginator[Node any] struct {
	// QueryFunc returns paginated results for the given paging parameters
	QueryFunc func(params boolTimeIDPagingParams) ([]Node, error)

	// CursorFunc returns a time and DBID that will be encoded into a cursor string
	CursorFunc func(node Node) (bool, time.Time, persist.DBID, error)

	// CountFunc returns the total number of items that can be paginated. May be nil, in which
	// case the resulting PageInfo will omit the total field.
	CountFunc func() (count int, err error)
}

func (p *boolTimeIDPaginator[Node]) paginate(before *string, after *string, first *int, last *int) ([]Node, PageInfo, error) {
	queryFunc := func(limit int32, pagingForward bool) ([]Node, error) {
		beforeCur := cursors.NewBoolTimeIDCursor()
		beforeCur.Bool = true
		beforeCur.Time = defaultCursorBeforeTime
		beforeCur.ID = defaultCursorBeforeID
		afterCur := cursors.NewBoolTimeIDCursor()
		afterCur.Bool = false
		afterCur.Time = defaultCursorAfterTime
		afterCur.ID = defaultCursorAfterID

		if before != nil {
			if err := beforeCur.Unpack(*before); err != nil {
				return nil, err
			}
		}

		if after != nil {
			if err := afterCur.Unpack(*after); err != nil {
				return nil, err
			}
		}

		queryParams := boolTimeIDPagingParams{
			Limit:            limit,
			CursorBeforeBool: beforeCur.Bool,
			CursorBeforeTime: beforeCur.Time,
			CursorBeforeID:   beforeCur.ID,
			CursorAfterBool:  afterCur.Bool,
			CursorAfterTime:  afterCur.Time,
			CursorAfterID:    afterCur.ID,
			PagingForward:    pagingForward,
		}

		return p.QueryFunc(queryParams)
	}

	paginator := keysetPaginator[Node, *boolTimeIDCursor]{
		QueryFunc:  queryFunc,
		Cursorable: newBoolTimeIDCursor(p.CursorFunc),
		CountFunc:  p.CountFunc,
	}

	return paginator.paginate(before, after, first, last)
}

type lexicalPaginator[Node any] struct {
	// QueryFunc returns paginated results for the given paging parameters
	QueryFunc func(params lexicalPagingParams) ([]Node, error)

	// CursorFunc returns a time and DBID that will be encoded into a cursor string
	CursorFunc func(node Node) (string, persist.DBID, error)

	// CountFunc returns the total number of items that can be paginated. May be nil, in which
	// case the resulting PageInfo will omit the total field.
	CountFunc func() (count int, err error)
}

type lexicalPagingParams struct {
	Limit           int32
	CursorBeforeKey string
	CursorBeforeID  persist.DBID
	CursorAfterKey  string
	CursorAfterID   persist.DBID
	PagingForward   bool
}

func (p *lexicalPaginator[Node]) paginate(before *string, after *string, first *int, last *int) ([]Node, PageInfo, error) {
	queryFunc := func(limit int32, pagingForward bool) ([]Node, error) {
		beforeCur := cursors.NewStringIDCursor()
		beforeCur.String = defaultCursorBeforeKey
		beforeCur.ID = defaultCursorBeforeID
		afterCur := cursors.NewStringIDCursor()
		afterCur.String = defaultCursorAfterKey
		afterCur.ID = defaultCursorAfterID

		if before != nil {
			if err := beforeCur.Unpack(*before); err != nil {
				return nil, err
			}
		}

		if after != nil {
			if err := afterCur.Unpack(*after); err != nil {
				return nil, err
			}
		}

		queryParams := lexicalPagingParams{
			Limit:           limit,
			CursorBeforeKey: beforeCur.String,
			CursorBeforeID:  beforeCur.ID,
			CursorAfterKey:  afterCur.String,
			CursorAfterID:   afterCur.ID,
			PagingForward:   pagingForward,
		}

		return p.QueryFunc(queryParams)
	}

	paginator := keysetPaginator[Node, *stringIDCursor]{
		QueryFunc:  queryFunc,
		Cursorable: newStringIDCursor(p.CursorFunc),
		CountFunc:  p.CountFunc,
	}

	return paginator.paginate(before, after, first, last)
}

// positionPaginator paginates results based on a position of an element in a fixed list
type positionPaginator[Node any] struct {
	// QueryFunc returns paginated results for the given paging parameters
	QueryFunc func(params positionPagingParams) ([]Node, error)

	// CursorFunc returns the current position and a fixed slice of DBIDs that will be encoded into a cursor string
	CursorFunc func(node Node) (curPos int64, ids []persist.DBID, err error)

	// CountFunc returns the total number of items that can be paginated. May be nil, in which
	// case the resulting PageInfo will omit the total field.
	CountFunc func() (count int, err error)
}

type positionPagingParams struct {
	CursorBeforePos int32
	CursorAfterPos  int32
}

func (p *positionPaginator[Node]) paginate(before *string, after *string, first *int, last *int) ([]Node, PageInfo, error) {
	args := positionPagingParams{
		CursorBeforePos: int32(defaultCursorBeforePosition),
		CursorAfterPos:  int32(defaultCursorAfterPosition),
	}

	beforeCur := cursors.NewPositionCursor()
	afterCur := cursors.NewPositionCursor()

	if before != nil {
		if err := beforeCur.Unpack(*before); err != nil {
			return nil, PageInfo{}, err
		}
		args.CursorBeforePos = int32(beforeCur.CurrentPosition)
	}

	if after != nil {
		if err := afterCur.Unpack(*after); err != nil {
			return nil, PageInfo{}, err
		}
		args.CursorAfterPos = int32(afterCur.CurrentPosition)
	}

	results, err := p.QueryFunc(args)
	if err != nil {
		return nil, PageInfo{}, err
	}

	return pageFrom(results, nil, newPositionCursor(p.CursorFunc), before, after, first, last)
}

type intTimeIDPaginator[Node any] struct {
	QueryFunc func(params intTimeIDPagingParams) ([]Node, error)

	CursorFunc func(node Node) (int64, time.Time, persist.DBID, error)

	// CountFunc returns the total number of items that can be paginated. May be nil, in which
	// case the resulting PageInfo will omit the total field.
	CountFunc func() (count int, err error)
}

// intTimeIDPaginator are the parameters used to paginate with an int+time+DBID cursor
type intTimeIDPagingParams struct {
	Limit            int32
	CursorBeforeInt  int32
	CursorBeforeTime time.Time
	CursorBeforeID   persist.DBID
	CursorAfterInt   int32
	CursorAfterTime  time.Time
	CursorAfterID    persist.DBID
	PagingForward    bool
}

func (p *intTimeIDPaginator[Node]) paginate(before *string, after *string, first *int, last *int) ([]Node, PageInfo, error) {
	queryFunc := func(limit int32, pagingForward bool) ([]Node, error) {
		beforeCur := cursors.NewIntTimeIDCursor()
		beforeCur.Int = math.MaxInt32
		beforeCur.Time = defaultCursorBeforeTime
		beforeCur.ID = defaultCursorBeforeID
		afterCur := cursors.NewIntTimeIDCursor()
		afterCur.Int = 0
		afterCur.Time = defaultCursorAfterTime
		afterCur.ID = defaultCursorAfterID

		if before != nil {
			if err := beforeCur.Unpack(*before); err != nil {
				return nil, err
			}
		}

		if after != nil {
			if err := afterCur.Unpack(*after); err != nil {
				return nil, err
			}
		}

		queryParams := intTimeIDPagingParams{
			Limit:            limit,
			CursorBeforeInt:  int32(beforeCur.Int),
			CursorBeforeTime: beforeCur.Time,
			CursorBeforeID:   beforeCur.ID,
			CursorAfterInt:   int32(afterCur.Int),
			CursorAfterTime:  afterCur.Time,
			CursorAfterID:    afterCur.ID,
			PagingForward:    pagingForward,
		}

		return p.QueryFunc(queryParams)
	}

	paginator := keysetPaginator[Node, *intTimeIDCursor]{
		QueryFunc:  queryFunc,
		Cursorable: newIntTimeIDCursor(p.CursorFunc),
		CountFunc:  p.CountFunc,
	}

	return paginator.paginate(before, after, first, last)
}

//------------------------------------------------------------------------------

type cursorEncoder struct {
	buffer []byte
}

func newCursorEncoder() cursorEncoder {
	return cursorEncoder{}
}

// AsBase64 returns the underlying byte buffer as a Base64 string
func (e *cursorEncoder) AsBase64() string {
	return base64.RawStdEncoding.EncodeToString(e.buffer)
}

func (e *cursorEncoder) appendBool(b bool) {
	val := 0
	if b {
		val = 1
	}

	// appendUInt64 uses a variable-length encoding, so this will only use one byte
	e.appendUInt64(uint64(val))
}

func (e *cursorEncoder) appendTime(t time.Time) error {
	timeBytes, err := t.MarshalBinary()
	if err != nil {
		return err
	}

	// Write the time's length first
	e.appendUInt64(uint64(len(timeBytes)))

	// Then write the time's bytes
	e.buffer = append(e.buffer, timeBytes...)

	return nil
}

func (e *cursorEncoder) appendString(str string) {
	strLen := len(str)

	// Write the string's length first
	e.appendUInt64(uint64(strLen))

	// Then write the string's bytes
	if strLen != 0 {
		e.buffer = append(e.buffer, []byte(str)...)
	}
}

func (e *cursorEncoder) appendDBID(dbid persist.DBID) {
	e.appendString(dbid.String())
}

// appendUInt64 appends a uint64 to the underlying buffer, using a variable-length
// encoding (smaller numbers require fewer bytes)
func (e *cursorEncoder) appendUInt64(i uint64) {
	buf := make([]byte, binary.MaxVarintLen64)
	bytesWritten := binary.PutUvarint(buf, i)
	e.buffer = append(e.buffer, buf[:bytesWritten]...)
}

// appendInt64 appends an int64 to the underlying buffer, using a variable-length
// encoding (smaller numbers require fewer bytes)
func (e *cursorEncoder) appendInt64(i int64) {
	buf := make([]byte, binary.MaxVarintLen64)
	bytesWritten := binary.PutVarint(buf, i)
	e.buffer = append(e.buffer, buf[:bytesWritten]...)
}

// appendFloat64 appends a float64 to the underlying buffer
func (e *cursorEncoder) appendFloat64(f float64) {
	bits := math.Float64bits(f)
	e.appendUInt64(bits)
}

func (e *cursorEncoder) appendFeedEntityType(i persist.FeedEntityType) {
	e.appendInt64(int64(i))
}

type cursorDecoder struct {
	reader *bytes.Reader
}

func newCursorDecoder(base64Cursor string) (cursorDecoder, error) {
	c := cursorDecoder{}
	err := c.setReader(base64Cursor)
	return c, err
}

func (c *cursorDecoder) setReader(base64Cursor string) error {
	decoded, err := base64.RawStdEncoding.DecodeString(base64Cursor)
	if err != nil {
		return err
	}
	c.reader = bytes.NewReader(decoded)
	return nil
}

// readBool reads a bool from the underlying reader and advances the stream
func (d *cursorDecoder) readBool() (bool, error) {
	b, err := d.readUInt64()

	if err != nil {
		return false, err
	}

	return b > 0, nil
}

// readTime reads a time from the underlying reader and advances the stream
func (d *cursorDecoder) readTime() (time.Time, error) {
	t := time.Time{}

	// Times are prefixed with their length
	timeLen, err := d.readUInt64()
	if err != nil {
		return t, err
	}

	timeBytes := make([]byte, timeLen)
	numRead, err := d.reader.Read(timeBytes)
	if err != nil {
		return t, err
	}

	if uint64(numRead) != timeLen {
		return t, fmt.Errorf("error reading time: expected %d bytes, but only read %d bytes", timeLen, numRead)
	}

	err = t.UnmarshalBinary(timeBytes)
	if err != nil {
		return t, err
	}

	return t, nil
}

// readString reads a string from the underlying reader and advances the stream
func (d *cursorDecoder) readString() (string, error) {
	// Strings are prefixed with their length
	strLen, err := d.readUInt64()
	if err != nil {
		return "", err
	}

	strBytes := make([]byte, strLen)
	numRead, err := d.reader.Read(strBytes)
	if err != nil {
		return "", err
	}

	if uint64(numRead) != strLen {
		return "", fmt.Errorf("error reading string: expected %d bytes, but only read %d bytes", strLen, numRead)
	}

	return string(strBytes), nil
}

// readDBID reads a DBID from the underlying reader and advances the stream
func (d *cursorDecoder) readDBID() (persist.DBID, error) {
	str, err := d.readString()
	if err != nil {
		return "", err
	}

	return persist.DBID(str), nil
}

// readUInt64 reads a uint64 from the underlying reader and advances the stream,
// using a variable-length encoding (smaller numbers require fewer bytes)
func (d *cursorDecoder) readUInt64() (uint64, error) {
	return binary.ReadUvarint(d.reader)
}

// readInt64 reads an int64 from the underlying reader and advances the stream,
// using a variable-length encoding (smaller numbers require fewer bytes)
func (d *cursorDecoder) readInt64() (int64, error) {
	return binary.ReadVarint(d.reader)
}

// readFloat64 reads a float64 from the underlying reader and advances the stream
func (d *cursorDecoder) readFloat64() (float64, error) {
	bits, err := d.readUInt64()
	if err != nil {
		return 0, err
	}

	return math.Float64frombits(bits), nil
}

// readFeedEntityType reads FeedEntityType from the underlying reader and advances the stream
func (d *cursorDecoder) readFeedEntityType() (persist.FeedEntityType, error) {
	i, err := binary.ReadVarint(d.reader)
	if err != nil {
		return 0, err
	}
	return persist.FeedEntityType(i), nil
}

//------------------------------------------------------------------------------

type cursor interface {
	Pack() (string, error)
	Unpack(string) error
}

type baseCursor struct {
	packable
	unpackable
}

// cursorable is a function that creates a cursor N from a node of type T
type cursorable[Node any, K cursor] func(Node) (K, error)

type cursorN struct{} // namespace for available cursors

var cursors cursorN

func newTimeIDCursor[Node any](f func(Node) (time.Time, persist.DBID, error)) cursorable[Node, *timeIDCursor] {
	return func(node Node) (c *timeIDCursor, err error) {
		c = cursors.NewTimeIDCursor()
		c.Time, c.ID, err = f(node)
		return c, err
	}
}

func newBoolBoolIntIDCursor[Node any](f func(Node) (bool, bool, int64, persist.DBID, error)) cursorable[Node, *boolBoolIntIDCursor] {
	return func(node Node) (c *boolBoolIntIDCursor, err error) {
		c = cursors.NewBoolBoolIntIDCursor()
		c.Bool1, c.Bool2, c.Int, c.ID, err = f(node)
		return c, err
	}
}

func newBoolTimeIDCursor[Node any](f func(Node) (bool, time.Time, persist.DBID, error)) cursorable[Node, *boolTimeIDCursor] {
	return func(node Node) (c *boolTimeIDCursor, err error) {
		c = cursors.NewBoolTimeIDCursor()
		c.Bool, c.Time, c.ID, err = f(node)
		return c, err
	}
}

func newStringIDCursor[Node any](f func(Node) (string, persist.DBID, error)) cursorable[Node, *stringIDCursor] {
	return func(node Node) (c *stringIDCursor, err error) {
		c = cursors.NewStringIDCursor()
		c.String, c.ID, err = f(node)
		return c, err
	}
}

func newIntTimeIDCursor[Node any](f func(Node) (int64, time.Time, persist.DBID, error)) cursorable[Node, *intTimeIDCursor] {
	return func(node Node) (c *intTimeIDCursor, err error) {
		c = cursors.NewIntTimeIDCursor()
		c.Int, c.Time, c.ID, err = f(node)
		return c, err
	}
}

// sliceToMapIndex returns a lookup of each element's index in the slice
func sliceToMapIndex[V comparable](s []V) map[V]int64 {
	result := make(map[V]int64, len(s))
	for i, v := range s {
		result[v] = int64(i)
	}
	return result
}

func newPositionCursor[Node any](f func(Node) (int64, []persist.DBID, error)) cursorable[Node, *positionCursor] {
	return func(node Node) (c *positionCursor, err error) {
		c = cursors.NewPositionCursor()
		c.CurrentPosition, c.IDs, err = f(node)
		c.Positions = sliceToMapIndex(c.IDs)
		return c, err
	}
}

func newFeedPositionCursor[Node any](f func(Node) (int64, []persist.FeedEntityType, []persist.DBID, error)) cursorable[Node, *feedPositionCursor] {
	return func(node Node) (c *feedPositionCursor, err error) {
		c = cursors.NewFeedPositionCursor()
		c.CurrentPosition, c.EntityTypes, c.EntityIDs, err = f(node)
		c.Positions = sliceToMapIndex(c.EntityIDs)
		return c, err
	}
}

func newFloatIDCursor[Node any](f func(Node) (float64, persist.DBID, error)) cursorable[Node, *floatIDCursor] {
	return func(node Node) (c *floatIDCursor, err error) {
		c = cursors.NewFloatIDCursor()
		c.Float, c.ID, err = f(node)
		return c, err
	}
}

//------------------------------------------------------------------------------

type timeIDCursor struct {
	*baseCursor
	Time time.Time
	ID   persist.DBID
}

func (cursorN) NewTimeIDCursor() *timeIDCursor {
	c := timeIDCursor{baseCursor: &baseCursor{}}
	initCursor(c.baseCursor, &c.Time, &c.ID)
	return &c
}

//------------------------------------------------------------------------------

type boolBoolIntIDCursor struct {
	*baseCursor
	Bool1 bool
	Bool2 bool
	Int   int64
	ID    persist.DBID
}

func (cursorN) NewBoolBoolIntIDCursor() *boolBoolIntIDCursor {
	c := boolBoolIntIDCursor{baseCursor: &baseCursor{}}
	initCursor(c.baseCursor, &c.Bool1, &c.Bool2, &c.Int, &c.ID)
	return &c
}

//------------------------------------------------------------------------------

type boolTimeIDCursor struct {
	*baseCursor
	Bool bool
	Time time.Time
	ID   persist.DBID
}

func (cursorN) NewBoolTimeIDCursor() *boolTimeIDCursor {
	c := boolTimeIDCursor{baseCursor: &baseCursor{}}
	initCursor(c.baseCursor, &c.Bool, &c.Time, &c.ID)
	return &c
}

//------------------------------------------------------------------------------

type stringIDCursor struct {
	*baseCursor
	String string
	ID     persist.DBID
}

func (cursorN) NewStringIDCursor() *stringIDCursor {
	c := stringIDCursor{baseCursor: &baseCursor{}}
	initCursor(c.baseCursor, &c.String, &c.ID)
	return &c
}

//------------------------------------------------------------------------------

type intTimeIDCursor struct {
	*baseCursor
	Int  int64
	Time time.Time
	ID   persist.DBID
}

func (cursorN) NewIntTimeIDCursor() *intTimeIDCursor {
	c := intTimeIDCursor{baseCursor: &baseCursor{}}
	initCursor(c.baseCursor, &c.Int, &c.Time, &c.ID)
	return &c
}

//------------------------------------------------------------------------------

type feedPositionCursor struct {
	*baseCursor
	CurrentPosition int64
	EntityTypes     []persist.FeedEntityType
	EntityIDs       []persist.DBID
	Positions       map[persist.DBID]int64
}

func (f *feedPositionCursor) Unpack(s string) error {
	err := f.baseCursor.Unpack(s)
	if err != nil {
		return err
	}
	f.Positions = sliceToMapIndex(f.EntityIDs)
	return nil
}

func (cursorN) NewFeedPositionCursor() *feedPositionCursor {
	c := feedPositionCursor{baseCursor: &baseCursor{}, Positions: make(map[persist.DBID]int64)}
	initCursor(c.baseCursor, &c.CurrentPosition, &c.EntityTypes, &c.EntityIDs)
	return &c
}

//------------------------------------------------------------------------------

type positionCursor struct {
	*baseCursor
	CurrentPosition int64
	IDs             []persist.DBID
	Positions       map[persist.DBID]int64
}

func (f *positionCursor) Unpack(s string) error {
	err := f.baseCursor.Unpack(s)
	if err != nil {
		return err
	}
	f.Positions = sliceToMapIndex(f.IDs)
	return nil
}

func (cursorN) NewPositionCursor() *positionCursor {
	c := positionCursor{baseCursor: &baseCursor{}, Positions: make(map[persist.DBID]int64)}
	initCursor(c.baseCursor, &c.CurrentPosition, &c.IDs)
	return &c
}

//------------------------------------------------------------------------------

type floatIDCursor struct {
	*baseCursor
	Float float64
	ID    persist.DBID
}

func (cursorN) NewFloatIDCursor() *floatIDCursor {
	c := floatIDCursor{baseCursor: &baseCursor{}}
	initCursor(c.baseCursor, &c.Float, &c.ID)
	return &c
}

//------------------------------------------------------------------------------

func initCursor(cur *baseCursor, vals ...any) {
	cur.packVals = vals
	d, _ := newCursorDecoder("")
	cur.d = &d
	cur.unpackFs = unpackVals(&d, vals...)
}

type packable struct {
	packVals []any
}

func (p *packable) Pack() (string, error) {
	e := newCursorEncoder()
	if err := packVals(&e, p.packVals...); err != nil {
		return "", err
	}
	return e.AsBase64(), nil
}

func packVal(e *cursorEncoder, val any) error {
	switch v := val.(type) {
	case *bool:
		e.appendBool(*v)
	case *string:
		e.appendString(*v)
	case *persist.DBID:
		e.appendDBID(*v)
	case persist.DBID:
		e.appendDBID(v)
	case *uint64:
		e.appendUInt64(*v)
	case *int64:
		e.appendInt64(*v)
	case *int:
		e.appendInt64(int64(*v))
	case *time.Time:
		if err := e.appendTime(*v); err != nil {
			return err
		}
	case *persist.FeedEntityType:
		e.appendFeedEntityType(*v)
	case persist.FeedEntityType:
		e.appendFeedEntityType(v)
	case *[]persist.DBID:
		if err := packSlice(e, *v...); err != nil {
			return err
		}
	case *[]persist.FeedEntityType:
		if err := packSlice(e, *v...); err != nil {
			return err
		}
	default:
		panic(fmt.Sprintf("don't know how to encode type: %T", v))
	}
	return nil
}

func packVals[T any](e *cursorEncoder, vals ...T) error {
	for _, val := range vals {
		if err := packVal(e, val); err != nil {
			return err
		}
	}
	return nil
}

// Encode the length of the slice as an int64, then encode each val
func packSlice[T any](e *cursorEncoder, s ...T) error {
	e.appendInt64(int64(len(s)))
	return packVals(e, s...)
}

type unpackF func() error

type unpackable struct {
	d        *cursorDecoder
	unpackFs []unpackF
}

func (u *unpackable) Unpack(s string) (err error) {
	if s == "" {
		return nil
	}
	if err = u.d.setReader(s); err != nil {
		return err
	}
	for _, f := range u.unpackFs {
		if err = f(); err != nil {
			return err
		}
	}
	return nil
}

func unpackVal(d *cursorDecoder, val any) unpackF {
	switch v := val.(type) {
	case *string:
		return unpackTo(v, d.readString)
	case *time.Time:
		return unpackTo(v, d.readTime)
	case *persist.DBID:
		return unpackTo(v, d.readDBID)
	case *bool:
		return unpackTo(v, d.readBool)
	case *int64:
		return unpackTo(v, d.readInt64)
	case *[]persist.FeedEntityType:
		return unpackSliceTo(v, d, d.readFeedEntityType)
	case *[]persist.DBID:
		return unpackSliceTo(v, d, d.readDBID)
	default:
		panic(fmt.Sprintf("don't know how to unpack type: %T", v))
	}
}

func unpackVals[T any](d *cursorDecoder, vals ...T) []unpackF {
	unpackFs := make([]unpackF, len(vals))
	for i, val := range vals {
		unpackFs[i] = unpackVal(d, val)
	}
	return unpackFs
}

func unpackTo[T any](into *T, f func() (T, error)) unpackF {
	return func() (err error) {
		(*into), err = f()
		return err
	}
}

func unpackSliceTo[T any](into *[]T, d *cursorDecoder, f func() (T, error)) func() error {
	return func() error {
		l, err := d.readInt64()
		if err != nil {
			return err
		}

		items := make([]T, l)

		for i := int64(0); i < l; i++ {
			id, err := f()
			if err != nil {
				return err
			}
			items[i] = id
		}

		(*into) = items

		return nil
	}
}
