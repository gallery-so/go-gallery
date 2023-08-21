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
	defaultCursorBeforePosition = -1
	// Some position that comes before any other position
	defaultCursorAfterPosition = math.MaxInt32
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

func pageFrom[T any](allEdges []T, countF func() (int, error), cF cursorable, before, after *string, first, last *int) ([]T, PageInfo, error) {
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

func packNode(cF cursorable, node any) (string, error) {
	cursor, err := cF(node)
	if err != nil {
		return "", err
	}
	return cursor.Pack()
}

func pageInfoFrom[T any](cursorEdges, edgesPaged []T, countF func() (int, error), cF cursorable, before, after *string, first, last *int) (pageInfo PageInfo, err error) {
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

func pageEdgesFrom[T any](edges []T, before, after *string, first, last *int) ([]T, error) {
	if first != nil && len(edges) > *first {
		return edges[:*first], nil
	}

	if last != nil && len(edges) > *last {
		return edges[len(edges)-*last:], nil
	}

	return edges, nil
}

func applyCursors[T any](allEdges []T, cursorable func(any) (cursorer, error), before, after *string) ([]T, error) {
	edges := append([]T{}, allEdges...)

	if after != nil {
		for i, edge := range edges {
			cur, err := packNode(cursorable, edge)
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
			cur, err := packNode(cursorable, edge)
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
type keysetPaginator struct {
	// QueryFunc returns paginated results for the given paging parameters
	QueryFunc func(limit int32, pagingForward bool) (nodes []interface{}, err error)

	// Cursorable produces a cursorer for encoding nodes to cursor strings
	Cursorable cursorable

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

func (p *timeIDPaginator) paginate(before *string, after *string, first *int, last *int) ([]any, PageInfo, error) {
	queryFunc := func(limit int32, pagingForward bool) ([]any, error) {
		beforeCur := timeIDCursor{
			Time: defaultCursorBeforeTime,
			ID:   defaultCursorBeforeID,
		}
		afterCur := timeIDCursor{
			Time: defaultCursorAfterTime,
			ID:   defaultCursorAfterID,
		}

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

	paginator := keysetPaginator{
		QueryFunc:  queryFunc,
		Cursorable: cursors.NewTimeIDCursorer(p.CursorFunc),
		CountFunc:  p.CountFunc,
	}

	return paginator.paginate(before, after, first, last)
}

type sharedFollowersPaginator struct{ timeIDPaginator }

func (p *sharedFollowersPaginator) paginate(before *string, after *string, first *int, last *int) ([]interface{}, PageInfo, error) {
	queryFunc := func(limit int32, pagingForward bool) ([]interface{}, error) {
		// The shared followers query orders results in descending order when
		// paging forward (vs. ascending order which is more typical).
		beforeCur := timeIDCursor{
			Time: time.Date(1970, 1, 1, 1, 1, 1, 1, time.UTC),
			ID:   defaultCursorBeforeID,
		}
		afterCur := timeIDCursor{
			Time: time.Date(3000, 1, 1, 1, 1, 1, 1, time.UTC),
			ID:   defaultCursorAfterID,
		}

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

	paginator := keysetPaginator{
		QueryFunc:  queryFunc,
		Cursorable: cursors.NewTimeIDCursorer(p.CursorFunc),
		CountFunc:  p.CountFunc,
	}

	return paginator.paginate(before, after, first, last)
}

type sharedContractsPaginatorParams struct {
	Limit                        int32
	CursorBeforeDisplayedByUserA bool
	CursorBeforeDisplayedByUserB bool
	CursorBeforeOwnedCount       int
	CursorBeforeContractID       persist.DBID
	CursorAfterDisplayedByUserA  bool
	CursorAfterDisplayedByUserB  bool
	CursorAfterOwnedCount        int
	CursorAfterContractID        persist.DBID
	PagingForward                bool
}

type sharedContractsPaginator struct {
	// QueryFunc returns paginated results for the given paging parameters
	QueryFunc func(params sharedContractsPaginatorParams) ([]interface{}, error)

	// CursorFunc returns:
	//  * A bool indicating that userA displays the contract on their gallery
	//  * A bool indicating that userB displays the contract on their gallery
	//  * An int indicating how many tokens userA owns for a contract
	//  * A DBID indicating the ID of the contract
	CursorFunc func(node interface{}) (bool, bool, int64, persist.DBID, error)

	// CountFunc returns the total number of items that can be paginated. May be nil, in which
	// case the resulting PageInfo will omit the total field.
	CountFunc func() (count int, err error)
}

func (p *sharedContractsPaginator) paginate(before *string, after *string, first *int, last *int) ([]interface{}, PageInfo, error) {
	queryFunc := func(limit int32, pagingForward bool) ([]interface{}, error) {
		beforeCur := boolBootIntIDCursor{
			Bool1: false,
			Bool2: false,
			Int:   -1,
			ID:    defaultCursorBeforeID,
		}
		afterCur := boolBootIntIDCursor{
			Bool1: true,
			Bool2: true,
			Int:   math.MaxInt32,
			ID:    defaultCursorAfterID,
		}

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

		queryParams := sharedContractsPaginatorParams{
			Limit:                        limit,
			CursorBeforeDisplayedByUserA: beforeCur.Bool1,
			CursorBeforeDisplayedByUserB: beforeCur.Bool2,
			CursorBeforeOwnedCount:       int(beforeCur.Int),
			CursorBeforeContractID:       beforeCur.ID,
			CursorAfterDisplayedByUserA:  afterCur.Bool1,
			CursorAfterDisplayedByUserB:  afterCur.Bool2,
			CursorAfterOwnedCount:        int(afterCur.Int),
			CursorAfterContractID:        afterCur.ID,
			PagingForward:                pagingForward,
		}

		return p.QueryFunc(queryParams)
	}

	paginator := keysetPaginator{
		QueryFunc:  queryFunc,
		Cursorable: cursors.NewBoolBoolIntIDCursorer(p.CursorFunc),
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
}

func (p *boolTimeIDPaginator) paginate(before *string, after *string, first *int, last *int) ([]interface{}, PageInfo, error) {
	queryFunc := func(limit int32, pagingForward bool) ([]interface{}, error) {
		beforeCur := boolTimeIDCursor{
			Bool: true,
			Time: defaultCursorBeforeTime,
			ID:   defaultCursorBeforeID,
		}
		afterCur := boolTimeIDCursor{
			Bool: false,
			Time: defaultCursorAfterTime,
			ID:   defaultCursorAfterID,
		}

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

	paginator := keysetPaginator{
		QueryFunc:  queryFunc,
		Cursorable: cursors.NewBoolTimeIDCursorer(p.CursorFunc),
		CountFunc:  p.CountFunc,
	}

	return paginator.paginate(before, after, first, last)
}

type lexicalPaginator struct {
	// QueryFunc returns paginated results for the given paging parameters
	QueryFunc func(params lexicalPagingParams) ([]interface{}, error)

	// CursorFunc returns a time and DBID that will be encoded into a cursor string
	CursorFunc func(node interface{}) (string, persist.DBID, error)

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

func (p *lexicalPaginator) paginate(before *string, after *string, first *int, last *int) ([]interface{}, PageInfo, error) {
	queryFunc := func(limit int32, pagingForward bool) ([]interface{}, error) {
		beforeCur := stringIDCursor{
			String: defaultCursorBeforeKey,
			ID:     defaultCursorBeforeID,
		}
		afterCur := stringIDCursor{
			String: defaultCursorAfterKey,
			ID:     defaultCursorAfterID,
		}

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

	paginator := keysetPaginator{
		QueryFunc:  queryFunc,
		Cursorable: cursors.NewStringIDCursorer(p.CursorFunc),
		CountFunc:  p.CountFunc,
	}

	return paginator.paginate(before, after, first, last)
}

// positionPaginator paginates results based on a position of an element in a fixed list
type positionPaginator struct {
	// QueryFunc returns paginated results for the given paging parameters
	QueryFunc func(params positionPagingParams) ([]any, error)

	// CursorFunc returns the current position and a fixed slice of DBIDs that will be encoded into a cursor string
	CursorFunc func(node interface{}) (int64, []persist.DBID, error)

	// CountFunc returns the total number of items that can be paginated. May be nil, in which
	// case the resulting PageInfo will omit the total field.
	CountFunc func() (count int, err error)
}

type positionPaginatorArgs struct {
	CurBeforePos int
	CurAfterPos  int
}

type positionPaginatorOpts struct{}

var positionOpts positionPaginatorOpts

// WithDefaultCursors configures the starting cursors to use when none are provided
func (positionPaginatorOpts) WithStartingCursors(before, after int) func(a *positionPaginatorArgs) {
	return func(a *positionPaginatorArgs) {
		a.CurBeforePos = before
		a.CurAfterPos = after
	}
}

type positionPagingParams struct {
	Limit           int32
	CursorBeforePos int32
	CursorAfterPos  int32
	PagingForward   bool
	IDs             []persist.DBID
}

func (p *positionPaginator) paginate(before *string, after *string, first *int, last *int, opts ...func(*positionPaginatorArgs)) ([]interface{}, PageInfo, error) {
	queryFunc := func(limit int32, pagingForward bool) ([]interface{}, error) {
		var ids []persist.DBID

		args := positionPaginatorArgs{
			CurBeforePos: defaultCursorBeforePosition,
			CurAfterPos:  defaultCursorAfterPosition,
		}

		var beforeCur positionCursor
		var afterCur positionCursor

		for _, opt := range opts {
			opt(&args)
		}

		if before != nil {
			if err := beforeCur.Unpack(*before); err != nil {
				return nil, err
			}
			args.CurBeforePos = int(beforeCur.CurrentPosition)
			ids = beforeCur.IDs
		}

		if after != nil {
			if err := afterCur.Unpack(*after); err != nil {
				return nil, err
			}
			args.CurAfterPos = int(afterCur.CurrentPosition)
			ids = afterCur.IDs
		}

		queryParams := positionPagingParams{
			Limit:           limit,
			CursorBeforePos: int32(args.CurBeforePos),
			CursorAfterPos:  int32(args.CurAfterPos),
			PagingForward:   pagingForward,
			IDs:             ids,
		}

		return p.QueryFunc(queryParams)
	}

	paginator := keysetPaginator{
		QueryFunc:  queryFunc,
		Cursorable: cursors.NewPositionCursorer(p.CursorFunc),
		CountFunc:  p.CountFunc,
	}

	return paginator.paginate(before, after, first, last)
}

type intTimeIDPaginator struct {
	QueryFunc func(params intTimeIDPagingParams) ([]interface{}, error)

	CursorFunc func(node interface{}) (int64, time.Time, persist.DBID, error)

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

func (p *intTimeIDPaginator) paginate(before *string, after *string, first *int, last *int) ([]interface{}, PageInfo, error) {
	queryFunc := func(limit int32, pagingForward bool) ([]interface{}, error) {
		beforeCur := intTimeIDCursor{
			Int:  math.MaxInt32,
			Time: defaultCursorBeforeTime,
			ID:   defaultCursorBeforeID,
		}
		afterCur := intTimeIDCursor{
			Int:  0,
			Time: defaultCursorAfterTime,
			ID:   defaultCursorAfterID,
		}

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

	paginator := keysetPaginator{
		QueryFunc:  queryFunc,
		Cursorable: cursors.NewIntTimeIDCursorer(p.CursorFunc),
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

func (e *cursorEncoder) appendFeedEntityType(i persist.FeedEntityType) {
	e.appendInt64(int64(i))
}

type cursorDecoder struct {
	reader *bytes.Reader
}

func newCursorDecoder(base64Cursor string) (cursorDecoder, error) {
	decoded, err := base64.RawStdEncoding.DecodeString(base64Cursor)
	if err != nil {
		return cursorDecoder{}, err
	}

	return cursorDecoder{reader: bytes.NewReader(decoded)}, nil
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

// readFeedEntityType reads FeedEntityType from the underlying reader and advances the stream
func (d *cursorDecoder) readFeedEntityType() (persist.FeedEntityType, error) {
	i, err := binary.ReadVarint(d.reader)
	if err != nil {
		return 0, err
	}
	return persist.FeedEntityType(i), nil
}

//------------------------------------------------------------------------------

type cursorer interface {
	Pack() (string, error)
	Unpack(string) error
}
type cursorable func(any) (cursorer, error)
type curs struct{}

var cursors curs

func (curs) NewTimeIDCursorer(f func(any) (time.Time, persist.DBID, error)) cursorable {
	return func(node any) (c cursorer, err error) {
		var cur timeIDCursor
		cur.Time, cur.ID, err = f(node)
		return &cur, err
	}
}

func (curs) NewBoolBoolIntIDCursorer(f func(any) (bool, bool, int64, persist.DBID, error)) cursorable {
	return func(node any) (c cursorer, err error) {
		var cur boolBootIntIDCursor
		cur.Bool1, cur.Bool2, cur.Int, cur.ID, err = f(node)
		return &cur, err
	}
}

func (curs) NewBoolTimeIDCursorer(f func(any) (bool, time.Time, persist.DBID, error)) cursorable {
	return func(node any) (c cursorer, err error) {
		var cur boolTimeIDCursor
		cur.Bool, cur.Time, cur.ID, err = f(node)
		return &cur, err
	}
}

func (curs) NewStringIDCursorer(f func(any) (string, persist.DBID, error)) cursorable {
	return func(node any) (c cursorer, err error) {
		var cur stringIDCursor
		cur.String, cur.ID, err = f(node)
		return &cur, err
	}
}

func (curs) NewIntTimeIDCursorer(f func(any) (int64, time.Time, persist.DBID, error)) cursorable {
	return func(node any) (c cursorer, err error) {
		var cur intTimeIDCursor
		cur.Int, cur.Time, cur.ID, err = f(node)
		return &cur, err
	}
}

func (curs) NewPositionCursorer(f func(any) (int64, []persist.DBID, error)) cursorable {
	return func(node any) (c cursorer, err error) {
		var cur positionCursor
		cur.CurrentPosition, cur.IDs, err = f(node)
		return &cur, err
	}
}

func (curs) NewFeedPositionCursorer(f func(any) (int64, []persist.FeedEntityType, []persist.DBID, error)) cursorable {
	return func(node any) (c cursorer, err error) {
		var cur feedPositionCursor
		cur.CurrentPosition, cur.EntityTypes, cur.EntityIDs, err = f(node)
		return &cur, err
	}
}

//------------------------------------------------------------------------------

type timeIDCursor struct {
	Time time.Time
	ID   persist.DBID
}

func (c timeIDCursor) Pack() (string, error) { return pack(c.Time, c.ID) }
func (c *timeIDCursor) Unpack(s string) error {
	d, err := newCursorDecoder(s)
	if err != nil {
		return err
	}
	c.Time, err = d.readTime()
	if err != nil {
		return err
	}
	c.ID, err = d.readDBID()
	return err
}

//------------------------------------------------------------------------------

type boolBootIntIDCursor struct {
	Bool1 bool
	Bool2 bool
	Int   int64
	ID    persist.DBID
}

func (c boolBootIntIDCursor) Pack() (string, error) { return pack(c.Bool1, c.Bool2, c.Int, c.ID) }
func (c *boolBootIntIDCursor) Unpack(s string) error {
	d, err := newCursorDecoder(s)
	if err != nil {
		return err
	}
	c.Bool1, err = d.readBool()
	if err != nil {
		return err
	}
	c.Bool2, err = d.readBool()
	if err != nil {
		return err
	}
	c.Int, err = d.readInt64()
	if err != nil {
		return err
	}
	c.ID, err = d.readDBID()
	return err
}

//------------------------------------------------------------------------------

type boolTimeIDCursor struct {
	Bool bool
	Time time.Time
	ID   persist.DBID
}

func (c boolTimeIDCursor) Pack() (string, error) { return pack(c.Bool, c.Time, c.ID) }
func (c *boolTimeIDCursor) Unpack(s string) error {
	d, err := newCursorDecoder(s)
	if err != nil {
		return err
	}
	c.Bool, err = d.readBool()
	if err != nil {
		return err
	}
	c.Time, err = d.readTime()
	if err != nil {
		return err
	}
	c.ID, err = d.readDBID()
	return err
}

//------------------------------------------------------------------------------

type stringIDCursor struct {
	String string
	ID     persist.DBID
}

func (c stringIDCursor) Pack() (string, error) { return pack(c.String, c.ID) }
func (c *stringIDCursor) Unpack(s string) error {
	d, err := newCursorDecoder(s)
	if err != nil {
		return err
	}
	c.String, err = d.readString()
	if err != nil {
		return err
	}
	c.ID, err = d.readDBID()
	return err
}

//------------------------------------------------------------------------------

type intTimeIDCursor struct {
	Int  int64
	Time time.Time
	ID   persist.DBID
}

func (c intTimeIDCursor) Pack() (string, error) { return pack(c.Int, c.Time, c.ID) }
func (c *intTimeIDCursor) Unpack(s string) error {
	d, err := newCursorDecoder(s)
	if err != nil {
		return err
	}
	c.Int, err = d.readInt64()
	if err != nil {
		return err
	}
	c.Time, err = d.readTime()
	if err != nil {
		return err
	}
	c.ID, err = d.readDBID()
	return err
}

//------------------------------------------------------------------------------

type feedPositionCursor struct {
	CurrentPosition int64
	EntityTypes     []persist.FeedEntityType
	EntityIDs       []persist.DBID
}

func (c feedPositionCursor) Pack() (string, error) {
	return pack(c.CurrentPosition, c.EntityTypes, c.EntityIDs)
}

func (c *feedPositionCursor) Unpack(s string) (err error) {
	d, err := newCursorDecoder(s)
	if err != nil {
		return err
	}

	c.CurrentPosition, err = d.readInt64()
	if err != nil {
		return err
	}

	c.EntityTypes, err = unpackSlice[persist.FeedEntityType](&d, func(d *cursorDecoder) (persist.FeedEntityType, error) {
		return d.readFeedEntityType()
	})
	if err != nil {
		return err
	}

	c.EntityIDs, err = unpackSlice[persist.DBID](&d, func(d *cursorDecoder) (persist.DBID, error) {
		return d.readDBID()
	})

	return err
}

//------------------------------------------------------------------------------

type positionCursor struct {
	CurrentPosition int64
	IDs             []persist.DBID
}

func (c positionCursor) Pack() (string, error) { return pack(c.CurrentPosition, c.IDs) }
func (c *positionCursor) Unpack(s string) (err error) {
	d, err := newCursorDecoder(s)
	if err != nil {
		return err
	}

	c.CurrentPosition, err = d.readInt64()
	if err != nil {
		return err
	}

	c.IDs, err = unpackSlice[persist.DBID](&d, func(d *cursorDecoder) (persist.DBID, error) {
		return d.readDBID()
	})

	return err
}

//------------------------------------------------------------------------------

func pack(vals ...any) (string, error) {
	e := newCursorEncoder()

	if err := packVals(&e, vals...); err != nil {
		return "", err
	}

	return e.AsBase64(), nil
}

func packVal(e *cursorEncoder, val any) error {
	switch v := val.(type) {
	case bool:
		e.appendBool(v)
	case string:
		e.appendString(v)
	case persist.DBID:
		e.appendDBID(v)
	case uint64:
		e.appendUInt64(v)
	case int64:
		e.appendInt64(v)
	case int:
		e.appendInt64(int64(v))
	case time.Time:
		if err := e.appendTime(v); err != nil {
			return err
		}
	case persist.FeedEntityType:
		e.appendFeedEntityType(v)
	case []persist.DBID:
		if err := packSlice(e, v); err != nil {
			return err
		}
	case []persist.FeedEntityType:
		if err := packSlice(e, v); err != nil {
			return err
		}
	default:
		panic(fmt.Sprintf("unknown cursor type: %T", v))
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
func packSlice[T any](e *cursorEncoder, s []T) error {
	e.appendInt64(int64(len(s)))
	return packVals(e, s...)
}

func unpackSlice[T any](d *cursorDecoder, f func(d *cursorDecoder) (T, error)) ([]T, error) {
	l, err := d.readInt64()
	if err != nil {
		return nil, err
	}

	items := make([]T, l)

	for i := int64(0); i < l; i++ {
		id, err := f(d)
		if err != nil {
			return nil, err
		}
		items[i] = id
	}

	return items, nil
}
