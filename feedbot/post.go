package feedbot

import (
	"context"
	"encoding/json"
	"fmt"
	"html"

	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/service/task"
	"github.com/shurcooL/graphql"
	"github.com/spf13/viper"
)

type PostRenderSender struct {
	PostRenderer
	PostSender
}

func (r *PostRenderSender) RenderAndSend(ctx context.Context, message task.FeedbotMessage) error {
	msg, err := r.Render(ctx, message)

	if err != nil {
		return err
	}

	if msg == "" {
		return nil
	}

	return r.Send(ctx, msg)
}

type PostRenderer struct {
	gql *graphql.Client
}

func (r *PostRenderer) Render(ctx context.Context, message task.FeedbotMessage) (string, error) {
	switch message.Action {
	case persist.ActionUserCreated:
		return r.createUserCreatedPost(ctx, message)
	case persist.ActionUserFollowedUsers:
		return r.createUserFollowedUsersPost(ctx, message)
	case persist.ActionCollectorsNoteAddedToToken:
		return r.createCollectorsNoteAddedToTokenPost(ctx, message)
	case persist.ActionCollectionCreated:
		return r.createCollectionCreatedPost(ctx, message)
	case persist.ActionCollectorsNoteAddedToCollection:
		return r.createCollectorsNoteAddedToCollectionPost(ctx, message)
	case persist.ActionTokensAddedToCollection:
		return r.createTokensAddedToCollectionPost(ctx, message)
	default:
		return "", fmt.Errorf("unknown action=%s; id=%s", message.Action, message.FeedEventID)
	}
}

func (r *PostRenderer) createUserCreatedPost(ctx context.Context, message task.FeedbotMessage) (string, error) {
	var evt EventQuery

	if err := r.gql.Query(ctx, &evt, map[string]interface{}{
		"id": fmt.Sprintf("%sEvent", message.Action),
	}); err != nil {
		return "", err
	}

	if evt.FeedEvent.UserCreated.Owner.Username == "" {
		return "", nil
	}

	return fmt.Sprintf("**%s** joined Gallery: %s",
		evt.FeedEvent.UserCreated.Owner.Username, userURL(evt.FeedEvent.UserCreated.Owner.Username),
	), nil
}

func (r *PostRenderer) createUserFollowedUsersPost(ctx context.Context, message task.FeedbotMessage) (string, error) {
	var evt EventQuery

	if err := r.gql.Query(ctx, &evt, map[string]interface{}{
		"id": fmt.Sprintf("%sEvent", message.Action),
	}); err != nil {
		return "", err
	}

	if evt.FeedEvent.UserFollowedUsers.Owner.Username == "" {
		return "", nil
	}

	if len(evt.FeedEvent.UserFollowedUsers.Followed) == 1 {
		return fmt.Sprintf("**%s** followed **%s**: %s",
			evt.FeedEvent.UserFollowedUsers.Owner.Username,
			evt.FeedEvent.UserFollowedUsers.Followed[0].User.Username,
			userURL(evt.FeedEvent.UserFollowedUsers.Followed[0].User.Username),
		), nil
	} else {
		return fmt.Sprintf("**%s** followed **%s** and %d other(s): %s",
			evt.FeedEvent.UserFollowedUsers.Owner.Username,
			evt.FeedEvent.UserFollowedUsers.Followed[0].User.Username,
			len(evt.FeedEvent.UserFollowedUsers.Followed)-1,
			userURL(evt.FeedEvent.UserFollowedUsers.Followed[0].User.Username),
		), nil
	}
}

func (r *PostRenderer) createCollectorsNoteAddedToTokenPost(ctx context.Context, message task.FeedbotMessage) (string, error) {
	var evt EventQuery

	if err := r.gql.Query(ctx, &evt, map[string]interface{}{
		"id": fmt.Sprintf("%sEvent", message.Action),
	}); err != nil {
		return "", err
	}

	if evt.FeedEvent.CollectorsNoteAddedToToken.Owner.Username == "" {
		return "", nil
	}

	if evt.FeedEvent.CollectorsNoteAddedToToken.CollectionToken.Token.Name != "" {
		return fmt.Sprintf("**%s** added a collector's note to *%s*: %s",
			evt.FeedEvent.CollectorsNoteAddedToToken.Owner.Username,
			evt.FeedEvent.CollectorsNoteAddedToToken.CollectionToken.Token.Name,
			tokenURL(
				evt.FeedEvent.CollectorsNoteAddedToToken.Owner.Username,
				evt.FeedEvent.CollectorsNoteAddedToToken.CollectionToken.Collection.Dbid,
				evt.FeedEvent.CollectorsNoteAddedToToken.CollectionToken.Token.Dbid,
			),
		), nil
	} else {
		return fmt.Sprintf("**%s** added a collector's note to their piece: %s",
			evt.FeedEvent.CollectorsNoteAddedToToken.Owner.Username,
			tokenURL(
				evt.FeedEvent.CollectorsNoteAddedToToken.Owner.Username,
				evt.FeedEvent.CollectorsNoteAddedToToken.CollectionToken.Collection.Dbid,
				evt.FeedEvent.CollectorsNoteAddedToToken.CollectionToken.Token.Dbid,
			),
		), nil
	}
}

func (r *PostRenderer) createCollectionCreatedPost(ctx context.Context, message task.FeedbotMessage) (string, error) {
	var evt EventQuery

	if err := r.gql.Query(ctx, &evt, map[string]interface{}{
		"id": fmt.Sprintf("%sEvent", message.Action),
	}); err != nil {
		return "", err
	}

	if evt.FeedEvent.CollectionCreated.Owner.Username == "" {
		return "", nil
	}

	if evt.FeedEvent.CollectionCreated.Collection.Name != "" {
		return fmt.Sprintf("**%s** created a collection titled '*%s'*: %s",
			evt.FeedEvent.CollectionCreated.Owner.Username,
			evt.FeedEvent.CollectionCreated.Collection.Name,
			collectionURL(
				evt.FeedEvent.CollectionCreated.Owner.Username,
				evt.FeedEvent.CollectionCreated.Collection.Dbid,
			),
		), nil
	} else {
		return fmt.Sprintf("**%s** created a collection: %s",
			evt.FeedEvent.CollectionCreated.Owner.Username,
			collectionURL(
				evt.FeedEvent.CollectionCreated.Owner.Username,
				evt.FeedEvent.CollectionCreated.Collection.Dbid,
			),
		), nil
	}
}

func (r *PostRenderer) createCollectorsNoteAddedToCollectionPost(ctx context.Context, message task.FeedbotMessage) (string, error) {
	var evt EventQuery

	if err := r.gql.Query(ctx, &evt, map[string]interface{}{
		"id": fmt.Sprintf("%sEvent", message.Action),
	}); err != nil {
		return "", err
	}

	if evt.FeedEvent.CollectorsNoteAddedToCollection.Owner.Username == "" {
		return "", nil
	}

	if evt.FeedEvent.CollectorsNoteAddedToCollection.Collection.Name != "" {
		return fmt.Sprintf("**%s** added a collector's note to their collection, *%s*: %s",
			evt.FeedEvent.CollectorsNoteAddedToCollection.Owner.Username,
			evt.FeedEvent.CollectorsNoteAddedToCollection.Collection.Name,
			collectionURL(
				evt.FeedEvent.CollectorsNoteAddedToCollection.Owner.Username,
				evt.FeedEvent.CollectorsNoteAddedToCollection.Collection.Dbid,
			),
		), nil
	} else {
		return fmt.Sprintf("**%s** added a collector's note to their collection: %s",
			evt.FeedEvent.CollectorsNoteAddedToCollection.Owner.Username,
			collectionURL(
				evt.FeedEvent.CollectorsNoteAddedToCollection.Owner.Username,
				evt.FeedEvent.CollectorsNoteAddedToCollection.Collection.Dbid,
			),
		), nil
	}
}

func (r *PostRenderer) createTokensAddedToCollectionPost(ctx context.Context, message task.FeedbotMessage) (string, error) {
	var evt EventQuery

	if err := r.gql.Query(ctx, &evt, map[string]interface{}{
		"id": fmt.Sprintf("%sEvent", message.Action),
	}); err != nil {
		return "", err
	}

	if evt.FeedEvent.TokensAddedToCollection.Owner.Username == "" {
		return "", nil
	}

	tokensAdded := len(evt.FeedEvent.TokensAddedToCollection.NewTokens)
	username := evt.FeedEvent.TokensAddedToCollection.Owner.Username
	collectionName := evt.FeedEvent.TokensAddedToCollection.Collection.Name
	url := collectionURL(username, evt.FeedEvent.TokensAddedToCollection.Collection.Dbid)

	var tokenName string
	for _, token := range evt.FeedEvent.TokensAddedToCollection.NewTokens {
		if token.Token.Name != "" {
			tokenName = token.Token.Name
			break
		}
	}

	var msg string

	if collectionName != "" && tokenName != "" {
		msg = fmt.Sprintf("**%s** added *%s* ", username, tokenName)
		if tokensAdded == 1 {
			msg += fmt.Sprintf("to their collection, *%s*: %s", collectionName, url)
		} else {
			msg += fmt.Sprintf("and %v other NFT(s) to their collection, *%s*: %s",
				tokensAdded-1, collectionName, url,
			)
		}
		return msg, nil
	} else if collectionName == "" && tokenName != "" {
		msg = fmt.Sprintf("**%s** added *%s* ", username, tokenName)
		if tokensAdded == 1 {
			msg += fmt.Sprintf("to their collection: %s", url)
		} else {
			msg += fmt.Sprintf("and %v other NFT(s) to their collection: %s", tokensAdded-1, url)
		}
	} else if collectionName != "" && tokenName == "" {
		return fmt.Sprintf("**%s** added %v NFT(s) to their collection, *%s*: %s",
			username, tokensAdded, collectionName, url,
		), nil
	} else {
		return fmt.Sprintf("**%s** added %v NFT(s) to their collection: %s",
			username, tokensAdded, url,
		), nil
	}

	return msg, nil
}

type PostSender struct{}

func (s *PostSender) Send(ctx context.Context, post string) error {
	content := html.UnescapeString(post)

	message, err := json.Marshal(map[string]interface{}{
		"content": content,
		"tts":     false,
	})

	if err != nil {
		return err
	}

	return sendMessage(ctx, message)
}

func userURL(username string) string {
	return fmt.Sprintf("%s/%s", viper.GetString("GALLERY_HOST"), username)
}

func collectionURL(username, collectionID string) string {
	return fmt.Sprintf("%s/%s", userURL(username), collectionID)
}

func tokenURL(username, collectionID, tokenID string) string {
	return fmt.Sprintf("%s/%s", collectionURL(username, collectionID), tokenID)
}

type UserFragment struct {
	Username string
}

type TokenFragment struct {
	Dbid string
	Name string
}

type CollectionFragment struct {
	Dbid string
	Name string
}

type EventQuery struct {
	FeedEvent struct {
		UserCreated struct {
			Owner UserFragment
		} `graphql:"...on UserCreatedEvent"`
		UserFollowedUsers struct {
			Owner    UserFragment
			Followed []struct {
				User UserFragment
			}
		} `graphql:"...on UserFollowedUsersEvent"`
		CollectorsNoteAddedToToken struct {
			Owner           UserFragment
			CollectionToken struct {
				Token      TokenFragment
				Collection CollectionFragment
			}
		} `graphql:"...on CollectorsNoteAddedToTokenEvent"`
		CollectionCreated struct {
			Owner      UserFragment
			Collection CollectionFragment
		} `graphql:"...on CollectionCreatedEvent"`
		CollectorsNoteAddedToCollection struct {
			Owner      UserFragment
			Collection CollectionFragment
		} `graphql:"...on CollectorsNoteAddedToCollectionEvent"`
		TokensAddedToCollection struct {
			Owner      UserFragment
			Collection CollectionFragment
			NewTokens  []struct {
				Token TokenFragment
			}
		} `graphql:"...on TokensAddedToCollectionEvent"`
	} `graphql:"node(id: $id)"`
}
