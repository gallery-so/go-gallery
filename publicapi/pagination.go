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
	defaultCursorBeforePositon = -1
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

func (p *timeIDPaginator) encodeCursor(t time.Time, id persist.DBID) (string, error) {
	encoder := newCursorEncoder()
	if err := encoder.appendTime(t); err != nil {
		return "", err
	}
	encoder.appendDBID(id)
	return encoder.AsBase64(), nil
}

func (p *timeIDPaginator) decodeCursor(cursor string) (time.Time, persist.DBID, error) {
	decoder, err := newCursorDecoder(cursor)
	if err != nil {
		return time.Time{}, "", err
	}

	t, err := decoder.readTime()
	if err != nil {
		return time.Time{}, "", err
	}

	id, err := decoder.readDBID()
	if err != nil {
		return time.Time{}, "", err
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
			curBeforeTime, curBeforeID, err = p.decodeCursor(*before)
			if err != nil {
				return nil, err
			}
		}

		if after != nil {
			curAfterTime, curAfterID, err = p.decodeCursor(*after)
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

		return p.encodeCursor(nodeTime, nodeID)
	}

	paginator := keysetPaginator{
		QueryFunc:  queryFunc,
		CursorFunc: cursorFunc,
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
	CursorFunc func(node interface{}) (bool, bool, int, persist.DBID, error)

	// CountFunc returns the total number of items that can be paginated. May be nil, in which
	// case the resulting PageInfo will omit the total field.
	CountFunc func() (count int, err error)
}

func (p *sharedContractsPaginator) encodeCursor(displayedA, displayedB bool, i int, contractID persist.DBID) (string, error) {
	encoder := newCursorEncoder()
	encoder.appendBool(displayedA)
	encoder.appendBool(displayedB)
	encoder.appendInt64(int64(i))
	encoder.appendDBID(contractID)
	return encoder.AsBase64(), nil
}

func (p *sharedContractsPaginator) decodeCursor(cursor string) (bool, bool, int, persist.DBID, error) {
	decoder, err := newCursorDecoder(cursor)
	if err != nil {
		return false, false, 0, "", nil
	}

	displayedA, err := decoder.readBool()
	if err != nil {
		return false, false, 0, "", nil
	}

	displayedB, err := decoder.readBool()
	if err != nil {
		return false, false, 0, "", nil
	}

	ownedCount, err := decoder.readInt64()
	if err != nil {
		return false, false, 0, "", nil
	}

	contractID, err := decoder.readDBID()
	if err != nil {
		return false, false, 0, "", nil
	}

	return displayedA, displayedB, int(ownedCount), contractID, nil
}

func (p *sharedContractsPaginator) paginate(before *string, after *string, first *int, last *int) ([]interface{}, PageInfo, error) {
	queryFunc := func(limit int32, pagingForward bool) ([]interface{}, error) {
		cursorBeforeDisplayedByUserA := true
		cursorBeforeDisplayedByUserB := true
		cursorBeforeOwnedCount := math.MaxInt32
		cursorBeforeContractID := defaultCursorBeforeID
		cursorAfterDisplayedByUserA := false
		cursorAfterDisplayedByUserB := false
		cursorAfterOwnedCount := -1
		cursorAfterContractID := defaultCursorAfterID

		var err error
		if before != nil {
			cursorBeforeDisplayedByUserA, cursorBeforeDisplayedByUserB, cursorBeforeOwnedCount, cursorBeforeContractID, err = p.decodeCursor(*before)
			if err != nil {
				panic(err)
				return nil, err
			}
		}

		if after != nil {
			cursorAfterDisplayedByUserA, cursorAfterDisplayedByUserB, cursorAfterOwnedCount, cursorAfterContractID, err = p.decodeCursor(*after)
			if err != nil {
				panic(err)
				return nil, err
			}
		}

		queryParams := sharedContractsPaginatorParams{
			Limit:                        limit,
			CursorBeforeDisplayedByUserA: cursorBeforeDisplayedByUserA,
			CursorBeforeDisplayedByUserB: cursorBeforeDisplayedByUserB,
			CursorBeforeOwnedCount:       cursorBeforeOwnedCount,
			CursorBeforeContractID:       cursorBeforeContractID,
			CursorAfterDisplayedByUserA:  cursorAfterDisplayedByUserA,
			CursorAfterDisplayedByUserB:  cursorAfterDisplayedByUserB,
			CursorAfterOwnedCount:        cursorAfterOwnedCount,
			CursorAfterContractID:        cursorAfterContractID,
			PagingForward:                pagingForward,
		}

		return p.QueryFunc(queryParams)
	}

	cursorFunc := func(node interface{}) (string, error) {
		displayedUserA, displayedUserB, ownedCount, contractID, err := p.CursorFunc(node)
		if err != nil {
			return "", err
		}

		return p.encodeCursor(displayedUserA, displayedUserB, ownedCount, contractID)
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
}

func (p *boolTimeIDPaginator) encodeCursor(b bool, t time.Time, id persist.DBID) (string, error) {
	encoder := newCursorEncoder()
	encoder.appendBool(b)
	if err := encoder.appendTime(t); err != nil {
		return "", err
	}
	encoder.appendDBID(id)
	return encoder.AsBase64(), nil
}

func (p *boolTimeIDPaginator) decodeCursor(cursor string) (bool, time.Time, persist.DBID, error) {
	decoder, err := newCursorDecoder(cursor)
	if err != nil {
		return false, time.Time{}, "", err
	}

	b, err := decoder.readBool()
	if err != nil {
		return false, time.Time{}, "", err
	}

	t, err := decoder.readTime()
	if err != nil {
		return false, time.Time{}, "", err
	}

	id, err := decoder.readDBID()
	if err != nil {
		return false, time.Time{}, "", err
	}

	return b, t, id, nil
}

func (p *boolTimeIDPaginator) paginate(before *string, after *string, first *int, last *int) ([]interface{}, PageInfo, error) {
	queryFunc := func(limit int32, pagingForward bool) ([]interface{}, error) {
		curBeforeTime := defaultCursorBeforeTime
		curBeforeID := defaultCursorBeforeID
		curAfterTime := defaultCursorAfterTime
		curAfterID := defaultCursorAfterID
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

func (p *lexicalPaginator) encodeCursor(sortKey string, id persist.DBID) (string, error) {
	encoder := newCursorEncoder()
	encoder.appendString(sortKey)
	encoder.appendDBID(id)
	return encoder.AsBase64(), nil
}

func (p *lexicalPaginator) decodeCursor(cursor string) (string, persist.DBID, error) {
	decoder, err := newCursorDecoder(cursor)
	if err != nil {
		return "", "", err
	}

	sortKey, err := decoder.readString()
	if err != nil {
		return "", "", err
	}

	id, err := decoder.readDBID()
	if err != nil {
		return "", "", err
	}

	return sortKey, id, nil
}

func (p *lexicalPaginator) paginate(before *string, after *string, first *int, last *int) ([]interface{}, PageInfo, error) {
	queryFunc := func(limit int32, pagingForward bool) ([]interface{}, error) {
		curBeforeKey := defaultCursorBeforeKey
		curBeforeID := defaultCursorBeforeID
		curAfterKey := defaultCursorAfterKey
		curAfterID := defaultCursorAfterID

		var err error
		if before != nil {
			curBeforeKey, curBeforeID, err = p.decodeCursor(*before)
			if err != nil {
				return nil, err
			}
		}

		if after != nil {
			curAfterKey, curAfterID, err = p.decodeCursor(*after)
			if err != nil {
				return nil, err
			}
		}

		queryParams := lexicalPagingParams{
			Limit:           limit,
			CursorBeforeKey: curBeforeKey,
			CursorBeforeID:  curBeforeID,
			CursorAfterKey:  curAfterKey,
			CursorAfterID:   curAfterID,
			PagingForward:   pagingForward,
		}

		return p.QueryFunc(queryParams)
	}

	cursorFunc := func(node interface{}) (string, error) {
		nodeKey, nodeID, err := p.CursorFunc(node)
		if err != nil {
			return "", err
		}

		return p.encodeCursor(nodeKey, nodeID)
	}

	paginator := keysetPaginator{
		QueryFunc:  queryFunc,
		CursorFunc: cursorFunc,
		CountFunc:  p.CountFunc,
	}

	return paginator.paginate(before, after, first, last)
}

// positionPaginator paginates results based on a position of an element in a fixed list
type positionPaginator struct {
	// QueryFunc returns paginated results for the given paging parameters
	QueryFunc func(params positionPagingParams) ([]any, error)

	// CursorFunc returns the current position and a fixed slice of DBIDs that will be encoded into a cursor string
	CursorFunc func(node interface{}) (int, []persist.DBID, error)

	// CountFunc returns the total number of items that can be paginated. May be nil, in which
	// case the resulting PageInfo will omit the total field.
	CountFunc func() (count int, err error)
}

type positionPagingParams struct {
	Limit           int32
	CursorBeforePos int32
	CursorAfterPos  int32
	PagingForward   bool
	IDs             []persist.DBID
}

func (p *positionPaginator) encodeCursor(position int, ids []persist.DBID) (string, error) {
	encoder := newCursorEncoder()
	encoder.appendInt64(int64(position))
	encoder.appendInt64(int64(len(ids)))
	for _, id := range ids {
		encoder.appendDBID(id)
	}
	return encoder.AsBase64(), nil
}

func (p *positionPaginator) decodeCursor(cursor string) (int, []persist.DBID, error) {
	decoder, err := newCursorDecoder(cursor)
	if err != nil {
		return 0, nil, err
	}

	position, err := decoder.readInt64()
	if err != nil {
		return 0, nil, err
	}

	totalItems, err := decoder.readInt64()
	if err != nil {
		return 0, nil, err
	}

	ids := make([]persist.DBID, totalItems)
	for i := int64(0); i < totalItems; i++ {
		id, err := decoder.readDBID()
		if err != nil {
			return 0, nil, err
		}
		ids[i] = id
	}

	return int(position), ids, nil
}

func (p *positionPaginator) paginate(before *string, after *string, first *int, last *int) ([]interface{}, PageInfo, error) {
	queryFunc := func(limit int32, pagingForward bool) ([]interface{}, error) {
		curBeforePos := defaultCursorBeforePositon
		curAfterPos := defaultCursorAfterPosition

		var err error
		var ids []persist.DBID

		if before != nil {
			curBeforePos, ids, err = p.decodeCursor(*before)
			if err != nil {
				return nil, err
			}
		}

		if after != nil {
			curAfterPos, ids, err = p.decodeCursor(*after)
			if err != nil {
				return nil, err
			}
		}

		queryParams := positionPagingParams{
			Limit:           limit,
			CursorBeforePos: int32(curBeforePos),
			CursorAfterPos:  int32(curAfterPos),
			PagingForward:   pagingForward,
			IDs:             ids,
		}

		return p.QueryFunc(queryParams)
	}

	cursorFunc := func(node any) (string, error) {
		pos, nodeID, err := p.CursorFunc(node)
		if err != nil {
			return "", err
		}
		return p.encodeCursor(pos, nodeID)
	}

	paginator := keysetPaginator{
		QueryFunc:  queryFunc,
		CursorFunc: cursorFunc,
		CountFunc:  p.CountFunc,
	}

	return paginator.paginate(before, after, first, last)
}

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
		return t, fmt.Errorf("error reading time: expected %d bytes, but only read %d bytes\n", timeLen, numRead)
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
		return "", fmt.Errorf("error reading string: expected %d bytes, but only read %d bytes\n", strLen, numRead)
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
