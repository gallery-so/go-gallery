package publicapi

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/mikeydub/go-gallery/service/persist"
)

func TestMain(t *testing.T) {
	t.Run("test cursor pagination", func(t *testing.T) {
		t.Run("cursor decodes expected types", func(t *testing.T) {
			t.Run("can decode timeID", func(t *testing.T) {
				curA := cursors.NewTimeIDCursor()
				curA.Time = time.Now()
				curA.ID = persist.GenerateID()
				packed, err := curA.Pack()
				assert.NoError(t, err)

				curB := cursors.NewTimeIDCursor()
				err = curB.Unpack(packed)

				assert.NoError(t, err)
				assert.NoError(t, curB.Unpack(packed))
				assert.True(t, curA.Time.Equal(curB.Time))
				assert.Equal(t, curA.ID, curB.ID)
			})

			t.Run("can decode boolBoolIntID", func(t *testing.T) {
				curA := cursors.NewBoolBoolIntIDCursor()
				curA.Bool1 = true
				curA.Bool2 = true
				curA.Int = 1337
				curA.ID = persist.GenerateID()
				packed, err := curA.Pack()
				assert.NoError(t, err)

				curB := cursors.NewBoolBoolIntIDCursor()

				assert.NoError(t, curB.Unpack(packed))
				assert.Equal(t, curA.Int, curB.Int)
			})

			t.Run("can decode boolTimeID", func(t *testing.T) {
				curA := cursors.NewBoolTimeIDCursor()
				curA.Bool = true
				curA.Time = time.Now()
				curA.ID = persist.GenerateID()
				packed, err := curA.Pack()
				assert.NoError(t, err)

				curB := cursors.NewBoolTimeIDCursor()

				assert.NoError(t, curB.Unpack(packed))
				assert.Equal(t, curA.Bool, curB.Bool)
				assert.True(t, curA.Time.Equal(curB.Time))
				assert.Equal(t, curA.ID, curB.ID)
			})

			t.Run("can decode stringID", func(t *testing.T) {
				curA := cursors.NewStringIDCursor()
				curA.String = "string"
				curA.ID = persist.GenerateID()
				packed, err := curA.Pack()
				assert.NoError(t, err)

				curB := cursors.NewStringIDCursor()

				assert.NoError(t, curB.Unpack(packed))
				assert.Equal(t, curA.String, curB.String)
				assert.Equal(t, curA.ID, curB.ID)
			})

			t.Run("can decode intTimeID", func(t *testing.T) {
				curA := cursors.NewIntTimeIDCursor()
				curA.Int = 1337
				curA.Time = time.Now()
				curA.ID = persist.GenerateID()
				packed, err := curA.Pack()
				assert.NoError(t, err)

				curB := cursors.NewIntTimeIDCursor()

				assert.NoError(t, curB.Unpack(packed))
				assert.Equal(t, curA.Int, curB.Int)
				assert.True(t, curA.Time.Equal(curB.Time))
				assert.Equal(t, curA.ID, curB.ID)
			})

			t.Run("can decode feedPosition", func(t *testing.T) {
				curA := cursors.NewFeedPositionCursor()
				curA.CurrentPosition = 2
				curA.EntityTypes = []persist.FeedEntityType{0, 1}
				curA.EntityIDs = []persist.DBID{"a", "b", "c", "d", "e"}
				packed, err := curA.Pack()
				assert.NoError(t, err)

				curB := cursors.NewFeedPositionCursor()
				err = curB.Unpack(packed)

				assert.NoError(t, err)
				assert.NoError(t, curB.Unpack(packed))
				assert.Equal(t, curA.CurrentPosition, curB.CurrentPosition)
				assert.Equal(t, curA.EntityTypes, curB.EntityTypes)
				assert.Equal(t, curA.EntityIDs, curB.EntityIDs)
			})

			t.Run("can decode position", func(t *testing.T) {
				curA := cursors.NewPositionCursor()
				curA.CurrentPosition = 2
				curA.IDs = []persist.DBID{"a", "b", "c", "d", "e"}
				packed, err := curA.Pack()
				assert.NoError(t, err)

				curB := cursors.NewPositionCursor()
				err = curB.Unpack(packed)

				assert.NoError(t, err)
				assert.NoError(t, curB.Unpack(packed))
				assert.Equal(t, curA.CurrentPosition, curB.CurrentPosition)
				assert.Equal(t, curA.IDs, curB.IDs)
			})
		})

		t.Run("cursor pagination returns expected edges", func(t *testing.T) {
			t.Run("should return no edges if no edges", func(t *testing.T) {
				edges := []string{}
				first := 10
				var last int

				actual, _, err := pageFrom(edges, nil, stubbedCursor, nil, nil, &first, &last)
				assert.NoError(t, err)
				assert.Equal(t, 0, len(actual))
			})

			t.Run("should return all edges", func(t *testing.T) {
				edges := []string{"a", "b", "c", "d", "e"}
				expected := []string{"a", "b", "c", "d", "e"}

				actual, _, err := pageFrom(edges, nil, stubbedCursor, nil, nil, nil, nil)
				assert.NoError(t, err)
				assert.Equal(t, expected, actual)
			})

			t.Run("should return no edges if first is zero", func(t *testing.T) {
				edges := []string{"a", "b", "c", "d", "e"}
				first := 0

				actual, _, err := pageFrom(edges, nil, stubbedCursor, nil, nil, &first, nil)
				assert.NoError(t, err)
				assert.Equal(t, 0, len(actual))
			})

			t.Run("should return no edges if last is zero", func(t *testing.T) {
				edges := []string{"a", "b", "c", "d", "e"}
				last := 0

				actual, _, err := pageFrom(edges, nil, stubbedCursor, nil, nil, nil, &last)
				assert.NoError(t, err)
				assert.Equal(t, 0, len(actual))
			})

			t.Run("should return expected edges when paging forward", func(t *testing.T) {
				edges := []string{"a", "b", "c", "d", "e"}
				first := 2
				expected := []string{"a", "b"}

				actual, _, err := pageFrom(edges, nil, stubbedCursor, nil, nil, &first, nil)
				assert.NoError(t, err)
				assert.Equal(t, expected, actual)
			})

			t.Run("should return expected edges when paging backwards", func(t *testing.T) {
				edges := []string{"a", "b", "c", "d", "e"}
				last := 2
				expected := []string{"d", "e"}

				actual, _, err := pageFrom(edges, nil, stubbedCursor, nil, nil, nil, &last)
				assert.NoError(t, err)
				assert.Equal(t, expected, actual)
			})

			t.Run("should return expected edges after cursor", func(t *testing.T) {
				edges := []string{"a", "b", "c", "d", "e"}
				first := 2
				after := "b"
				expected := []string{"c", "d"}

				actual, _, err := pageFrom(edges, nil, stubbedCursor, nil, &after, &first, nil)
				assert.NoError(t, err)
				assert.Equal(t, expected, actual)
			})

			t.Run("should return expected edges before cursor", func(t *testing.T) {
				edges := []string{"a", "b", "c", "d", "e"}
				last := 2
				before := "d"
				expected := []string{"b", "c"}

				actual, _, err := pageFrom(edges, nil, stubbedCursor, &before, nil, nil, &last)
				assert.NoError(t, err)
				assert.Equal(t, expected, actual)
			})

			t.Run("should return expected edges before and after cursors", func(t *testing.T) {
				edges := []string{"a", "b", "c", "d", "e"}
				last := 2
				before := "e"
				after := "a"
				expected := []string{"c", "d"}

				actual, _, err := pageFrom(edges, nil, stubbedCursor, &before, &after, nil, &last)
				assert.NoError(t, err)
				assert.Equal(t, expected, actual)
			})
		})

		t.Run("test cursor pagination returns expected page info", func(t *testing.T) {
			t.Run("should return expected info when no edges", func(t *testing.T) {
				edges := []string{}
				first := 10

				_, pageInfo, err := pageFrom(edges, nil, stubbedCursor, nil, nil, &first, nil)
				assert.NoError(t, err)
				assert.Equal(t, 0, pageInfo.Size)
				assert.Equal(t, false, pageInfo.HasPreviousPage)
				assert.Equal(t, false, pageInfo.HasNextPage)
				assert.Equal(t, "", pageInfo.StartCursor)
				assert.Equal(t, "", pageInfo.EndCursor)
			})

			t.Run("should return expected info when all edges", func(t *testing.T) {
				edges := []string{"a", "b", "c", "d", "e"}
				first := 10

				_, pageInfo, err := pageFrom(edges, nil, stubbedCursor, nil, nil, &first, nil)
				assert.NoError(t, err)
				assert.Equal(t, 5, pageInfo.Size)
				assert.Equal(t, false, pageInfo.HasPreviousPage)
				assert.Equal(t, false, pageInfo.HasNextPage)
				assert.Equal(t, "a", pageInfo.StartCursor)
				assert.Equal(t, "e", pageInfo.EndCursor)
			})

			t.Run("should return expected info when paging forward", func(t *testing.T) {
				edges := []string{"a", "b", "c", "d", "e"}
				first := 2

				_, pageInfo, err := pageFrom(edges, nil, stubbedCursor, nil, nil, &first, nil)
				assert.NoError(t, err)
				assert.Equal(t, 2, pageInfo.Size)
				assert.Equal(t, false, pageInfo.HasPreviousPage)
				assert.Equal(t, true, pageInfo.HasNextPage)
				assert.Equal(t, "a", pageInfo.StartCursor)
				assert.Equal(t, "b", pageInfo.EndCursor)
			})

			t.Run("should return expected info when paging backwards", func(t *testing.T) {
				edges := []string{"a", "b", "c", "d", "e"}
				last := 2

				_, pageInfo, err := pageFrom(edges, nil, stubbedCursor, nil, nil, nil, &last)
				assert.NoError(t, err)
				assert.Equal(t, 2, pageInfo.Size)
				assert.Equal(t, true, pageInfo.HasPreviousPage)
				assert.Equal(t, false, pageInfo.HasNextPage)
				assert.Equal(t, "d", pageInfo.StartCursor)
				assert.Equal(t, "e", pageInfo.EndCursor)
			})
		})
	})

	t.Run("test keyset pagination", func(t *testing.T) {
		t.Run("should exclude extra edges", func(t *testing.T) {
			p := newStubPaginator([]any{"a", "b", "c", "d", "e", "extra"})
			expected := []string{"a", "b", "c", "d", "e"}
			first := 5

			ret, _, err := p.paginate(nil, nil, &first, nil)

			actual := make([]string, len(ret))
			for i, v := range ret {
				actual[i] = v.(string)
			}

			assert.NoError(t, err)
			assert.Equal(t, expected, actual)
		})

		t.Run("should return expected page info when paging forward", func(t *testing.T) {
			p := newStubPaginator([]any{"a", "b", "c", "d", "e", "extra"})
			first := 5

			_, pageInfo, err := p.paginate(nil, nil, &first, nil)

			assert.NoError(t, err)
			assert.Equal(t, 5, pageInfo.Size)
			assert.Equal(t, true, pageInfo.HasNextPage)
			assert.Equal(t, "a", pageInfo.StartCursor)
			assert.Equal(t, "e", pageInfo.EndCursor)
		})

		t.Run("should return expected edge order when paging backwards", func(t *testing.T) {
			p := newStubPaginator([]any{"e", "d", "c", "b", "a", "extra"})
			expected := []string{"a", "b", "c", "d", "e"}
			last := 5

			ret, _, err := p.paginate(nil, nil, nil, &last)

			actual := make([]string, len(ret))
			for i, v := range ret {
				actual[i] = v.(string)
			}

			assert.NoError(t, err)
			assert.Equal(t, expected, actual)
		})

		t.Run("should return expected page info when paging backwards", func(t *testing.T) {
			p := newStubPaginator([]any{"e", "d", "c", "b", "a", "extra"})
			last := 5

			_, pageInfo, err := p.paginate(nil, nil, nil, &last)

			assert.NoError(t, err)
			assert.Equal(t, 5, pageInfo.Size)
			assert.Equal(t, true, pageInfo.HasPreviousPage)
			assert.Equal(t, "a", pageInfo.StartCursor)
			assert.Equal(t, "e", pageInfo.EndCursor)
		})
	})
}

type stubCursor struct{ ID string }

func (p stubCursor) Pack() (string, error) { return p.ID, nil }
func (p stubCursor) Unpack(s string) error { panic("not implemented") }

var stubbedCursor = func(node any) (c cursorer, err error) { return stubCursor{ID: node.(string)}, nil }

func newStubPaginator(ret []any) keysetPaginator {
	var p keysetPaginator

	p.QueryFunc = func(int32, bool) ([]any, error) {
		return ret, nil
	}

	p.Cursorable = stubbedCursor

	return p
}
