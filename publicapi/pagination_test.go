package publicapi

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMain(t *testing.T) {
	t.Run("test cursor pagination", func(t *testing.T) {
		t.Run("cursor pagination returns expected edges", func(t *testing.T) {
			t.Run("should return no edges if no edges", func(t *testing.T) {
				edges := []string{}
				first := 10
				var last int

				actual, _, err := pageFrom(edges, nil, identityCursor, nil, nil, &first, &last)
				assert.NoError(t, err)
				assert.Equal(t, 0, len(actual))
			})

			t.Run("should return all edges", func(t *testing.T) {
				edges := []string{"a", "b", "c", "d", "e"}
				expected := []string{"a", "b", "c", "d", "e"}

				actual, _, err := pageFrom(edges, nil, identityCursor, nil, nil, nil, nil)
				assert.NoError(t, err)
				assert.Equal(t, expected, actual)
			})

			t.Run("should return no edges if first is zero", func(t *testing.T) {
				edges := []string{"a", "b", "c", "d", "e"}
				first := 0

				actual, _, err := pageFrom(edges, nil, identityCursor, nil, nil, &first, nil)
				assert.NoError(t, err)
				assert.Equal(t, 0, len(actual))
			})

			t.Run("should return no edges if last is zero", func(t *testing.T) {
				edges := []string{"a", "b", "c", "d", "e"}
				last := 0

				actual, _, err := pageFrom(edges, nil, identityCursor, nil, nil, nil, &last)
				assert.NoError(t, err)
				assert.Equal(t, 0, len(actual))
			})

			t.Run("should return expected edges when paging forward", func(t *testing.T) {
				edges := []string{"a", "b", "c", "d", "e"}
				first := 2
				expected := []string{"a", "b"}

				actual, _, err := pageFrom(edges, nil, identityCursor, nil, nil, &first, nil)
				assert.NoError(t, err)
				assert.Equal(t, expected, actual)
			})

			t.Run("should return expected edges when paging backwards", func(t *testing.T) {
				edges := []string{"a", "b", "c", "d", "e"}
				last := 2
				expected := []string{"d", "e"}

				actual, _, err := pageFrom(edges, nil, identityCursor, nil, nil, nil, &last)
				assert.NoError(t, err)
				assert.Equal(t, expected, actual)
			})

			t.Run("should return expected edges after cursor", func(t *testing.T) {
				edges := []string{"a", "b", "c", "d", "e"}
				first := 2
				after := "b"
				expected := []string{"c", "d"}

				actual, _, err := pageFrom(edges, nil, identityCursor, nil, &after, &first, nil)
				assert.NoError(t, err)
				assert.Equal(t, expected, actual)
			})

			t.Run("should return expected edges before cursor", func(t *testing.T) {
				edges := []string{"a", "b", "c", "d", "e"}
				last := 2
				before := "d"
				expected := []string{"b", "c"}

				actual, _, err := pageFrom(edges, nil, identityCursor, &before, nil, nil, &last)
				assert.NoError(t, err)
				assert.Equal(t, expected, actual)
			})

			t.Run("should return expected edges before and after cursors", func(t *testing.T) {
				edges := []string{"a", "b", "c", "d", "e"}
				last := 2
				before := "e"
				after := "a"
				expected := []string{"c", "d"}

				actual, _, err := pageFrom(edges, nil, identityCursor, &before, &after, nil, &last)
				assert.NoError(t, err)
				assert.Equal(t, expected, actual)
			})
		})

		t.Run("test cursor pagination returns expected page info", func(t *testing.T) {
			t.Run("should return expected info when no edges", func(t *testing.T) {
				edges := []string{}
				first := 10

				_, pageInfo, err := pageFrom(edges, nil, identityCursor, nil, nil, &first, nil)
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

				_, pageInfo, err := pageFrom(edges, nil, identityCursor, nil, nil, &first, nil)
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

				_, pageInfo, err := pageFrom(edges, nil, identityCursor, nil, nil, &first, nil)
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

				_, pageInfo, err := pageFrom(edges, nil, identityCursor, nil, nil, nil, &last)
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
			p := newStubKeysetPaginator([]any{"a", "b", "c", "d", "e", "extra"})
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
			p := newStubKeysetPaginator([]any{"a", "b", "c", "d", "e", "extra"})
			first := 5

			_, pageInfo, err := p.paginate(nil, nil, &first, nil)

			assert.NoError(t, err)
			assert.Equal(t, 5, pageInfo.Size)
			assert.Equal(t, true, pageInfo.HasNextPage)
			assert.Equal(t, "a", pageInfo.StartCursor)
			assert.Equal(t, "e", pageInfo.EndCursor)
		})

		t.Run("should return expected edge order when paging backwards", func(t *testing.T) {
			p := newStubKeysetPaginator([]any{"e", "d", "c", "b", "a", "extra"})
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
			p := newStubKeysetPaginator([]any{"e", "d", "c", "b", "a", "extra"})
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

func identityCursor(s string) (string, error) {
	return s, nil
}

func newStubKeysetPaginator(ret []any) keysetPaginator {
	var p keysetPaginator

	p.QueryFunc = func(int32, bool) ([]any, error) {
		return ret, nil
	}

	p.CursorFunc = func(a any) (string, error) {
		return a.(string), nil
	}

	return p
}
