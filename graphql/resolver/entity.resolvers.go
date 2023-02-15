package graphql

// This file will be automatically regenerated based on the schema, any resolver implementations
// will be copied through when generating and any unknown code will be moved to the end.
// Code generated by github.com/99designs/gqlgen version v0.17.24

import (
	"context"
	"github.com/mikeydub/go-gallery/graphql/generated"
	"github.com/mikeydub/go-gallery/graphql/model"
)

// FindManyFeedEventByDbids is the resolver for the findManyFeedEventByDbids field.
func (r *entityResolver) FindManyFeedEventByDbids(ctx context.Context, reps []*model.FeedEventByDbidsInput) ([]*model.FeedEvent, error) {
	var events []*model.FeedEvent
	for index, _ := range reps {
		event, err := resolveFeedEventByEventID(ctx, reps[index].Dbid)

		if err != nil {
			return nil, err
		}

		events = append(events, event)
	}

	return events, nil
}

// Entity returns generated.EntityResolver implementation.
func (r *Resolver) Entity() generated.EntityResolver { return &entityResolver{r} }

type entityResolver struct{ *Resolver }