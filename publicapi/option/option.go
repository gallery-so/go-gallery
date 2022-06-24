package option

import (
	"github.com/mikeydub/go-gallery/graphql/model"
	"github.com/mikeydub/go-gallery/service/persist"
)

var defaultLastFeedToken persist.DBID
var defaultFeedEventLimit = 24

type FeedSearchSettings struct {
	Viewer persist.DBID
	Token  persist.DBID
	Limit  int
}

type FeedOption interface {
	Apply(*FeedSearchSettings)
}

// WithPage fetches a subset of records.
func WithFeedPage(page *model.Pagination) FeedOption {
	return withFeedPage{page}
}

type withFeedPage struct {
	page *model.Pagination
}

func (w withFeedPage) Apply(s *FeedSearchSettings) {
	if w.page.Token != nil {
		s.Token = *w.page.Token
	}
	if w.page.Limit != nil {
		s.Limit = *w.page.Limit
	}
}

func DefaultFeedOptions() []FeedOption {
	return []FeedOption{
		withFeedPage{
			page: &model.Pagination{
				Token: &defaultLastFeedToken,
				Limit: &defaultFeedEventLimit,
			}},
	}
}
