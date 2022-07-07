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
	var evt FeedEventQuery

	if err := r.gql.Query(ctx, &evt, map[string]interface{}{
		"id": message.FeedEventID,
	}); err != nil {
		return "", err
	}

	if evt.FeedEvent.Event.EventData.UserCreated.Owner.Username == "" {
		return "", nil
	}

	return fmt.Sprintf("**%s** joined Gallery: %s",
		evt.FeedEvent.Event.EventData.UserCreated.Owner.Username, userURL(evt.FeedEvent.Event.EventData.UserCreated.Owner.Username),
	), nil
}

func (r *PostRenderer) createUserFollowedUsersPost(ctx context.Context, message task.FeedbotMessage) (string, error) {
	var evt FeedEventQuery

	if err := r.gql.Query(ctx, &evt, map[string]interface{}{
		"id": message.FeedEventID,
	}); err != nil {
		return "", err
	}

	if evt.FeedEvent.Event.EventData.UserFollowedUsers.Owner.Username == "" {
		return "", nil
	}

	if len(evt.FeedEvent.Event.EventData.UserFollowedUsers.Followed) == 1 {
		return fmt.Sprintf("**%s** followed **%s**: %s",
			evt.FeedEvent.Event.EventData.UserFollowedUsers.Owner.Username,
			evt.FeedEvent.Event.EventData.UserFollowedUsers.Followed[0].User.Username,
			userURL(evt.FeedEvent.Event.EventData.UserFollowedUsers.Followed[0].User.Username),
		), nil
	} else {
		return fmt.Sprintf("**%s** followed **%s** and %d other(s): %s",
			evt.FeedEvent.Event.EventData.UserFollowedUsers.Owner.Username,
			evt.FeedEvent.Event.EventData.UserFollowedUsers.Followed[0].User.Username,
			len(evt.FeedEvent.Event.EventData.UserFollowedUsers.Followed)-1,
			userURL(evt.FeedEvent.Event.EventData.UserFollowedUsers.Followed[0].User.Username),
		), nil
	}
}

func (r *PostRenderer) createCollectorsNoteAddedToTokenPost(ctx context.Context, message task.FeedbotMessage) (string, error) {
	var evt FeedEventQuery

	if err := r.gql.Query(ctx, &evt, map[string]interface{}{
		"id": message.FeedEventID,
	}); err != nil {
		return "", err
	}

	if evt.FeedEvent.Event.EventData.CollectorsNoteAddedToToken.Owner.Username == "" {
		return "", nil
	}

	if evt.FeedEvent.Event.EventData.CollectorsNoteAddedToToken.Token.Token.Name != "" {
		return fmt.Sprintf("**%s** added a collector's note to *%s*: %s",
			evt.FeedEvent.Event.EventData.CollectorsNoteAddedToToken.Owner.Username,
			evt.FeedEvent.Event.EventData.CollectorsNoteAddedToToken.Token.Token.Name,
			tokenURL(
				evt.FeedEvent.Event.EventData.CollectorsNoteAddedToToken.Owner.Username,
				evt.FeedEvent.Event.EventData.CollectorsNoteAddedToToken.Token.Collection.Dbid,
				evt.FeedEvent.Event.EventData.CollectorsNoteAddedToToken.Token.Token.Dbid,
			),
		), nil
	} else {
		return fmt.Sprintf("**%s** added a collector's note to their piece: %s",
			evt.FeedEvent.Event.EventData.CollectorsNoteAddedToToken.Owner.Username,
			tokenURL(
				evt.FeedEvent.Event.EventData.CollectorsNoteAddedToToken.Owner.Username,
				evt.FeedEvent.Event.EventData.CollectorsNoteAddedToToken.Token.Collection.Dbid,
				evt.FeedEvent.Event.EventData.CollectorsNoteAddedToToken.Token.Token.Dbid,
			),
		), nil
	}
}

func (r *PostRenderer) createCollectionCreatedPost(ctx context.Context, message task.FeedbotMessage) (string, error) {
	var evt FeedEventQuery

	if err := r.gql.Query(ctx, &evt, map[string]interface{}{
		"id": message.FeedEventID,
	}); err != nil {
		return "", err
	}

	if evt.FeedEvent.Event.EventData.CollectionCreated.Owner.Username == "" {
		return "", nil
	}

	if evt.FeedEvent.Event.EventData.CollectionCreated.Collection.Name != "" {
		return fmt.Sprintf("**%s** created a collection titled '*%s'*: %s",
			evt.FeedEvent.Event.EventData.CollectionCreated.Owner.Username,
			evt.FeedEvent.Event.EventData.CollectionCreated.Collection.Name,
			collectionURL(
				evt.FeedEvent.Event.EventData.CollectionCreated.Owner.Username,
				evt.FeedEvent.Event.EventData.CollectionCreated.Collection.Dbid,
			),
		), nil
	} else {
		return fmt.Sprintf("**%s** created a collection: %s",
			evt.FeedEvent.Event.EventData.CollectionCreated.Owner.Username,
			collectionURL(
				evt.FeedEvent.Event.EventData.CollectionCreated.Owner.Username,
				evt.FeedEvent.Event.EventData.CollectionCreated.Collection.Dbid,
			),
		), nil
	}
}

func (r *PostRenderer) createCollectorsNoteAddedToCollectionPost(ctx context.Context, message task.FeedbotMessage) (string, error) {
	var evt FeedEventQuery

	if err := r.gql.Query(ctx, &evt, map[string]interface{}{
		"id": message.FeedEventID,
	}); err != nil {
		return "", err
	}

	if evt.FeedEvent.Event.EventData.CollectorsNoteAddedToCollection.Owner.Username == "" {
		return "", nil
	}

	if evt.FeedEvent.Event.EventData.CollectorsNoteAddedToCollection.Collection.Name != "" {
		return fmt.Sprintf("**%s** added a collector's note to their collection, *%s*: %s",
			evt.FeedEvent.Event.EventData.CollectorsNoteAddedToCollection.Owner.Username,
			evt.FeedEvent.Event.EventData.CollectorsNoteAddedToCollection.Collection.Name,
			collectionURL(
				evt.FeedEvent.Event.EventData.CollectorsNoteAddedToCollection.Owner.Username,
				evt.FeedEvent.Event.EventData.CollectorsNoteAddedToCollection.Collection.Dbid,
			),
		), nil
	} else {
		return fmt.Sprintf("**%s** added a collector's note to their collection: %s",
			evt.FeedEvent.Event.EventData.CollectorsNoteAddedToCollection.Owner.Username,
			collectionURL(
				evt.FeedEvent.Event.EventData.CollectorsNoteAddedToCollection.Owner.Username,
				evt.FeedEvent.Event.EventData.CollectorsNoteAddedToCollection.Collection.Dbid,
			),
		), nil
	}
}

func (r *PostRenderer) createTokensAddedToCollectionPost(ctx context.Context, message task.FeedbotMessage) (string, error) {
	var evt FeedEventQuery

	if err := r.gql.Query(ctx, &evt, map[string]interface{}{
		"id": message.FeedEventID,
	}); err != nil {
		return "", err
	}

	if evt.FeedEvent.Event.EventData.TokensAddedToCollection.Owner.Username == "" {
		return "", nil
	}

	tokensAdded := len(evt.FeedEvent.Event.EventData.TokensAddedToCollection.NewTokens)

	var tokenName string
	for _, token := range evt.FeedEvent.Event.EventData.TokensAddedToCollection.NewTokens {
		if token.Token.Name != "" {
			tokenName = token.Token.Name
			break
		}
	}

	if evt.FeedEvent.Event.EventData.TokensAddedToCollection.IsPreFeed {
		if evt.FeedEvent.Event.EventData.TokensAddedToCollection.Collection.Name != "" {
			msg := fmt.Sprintf("**%s** added pieces to their collection, *%s*: %s",
				evt.FeedEvent.Event.EventData.TokensAddedToCollection.Owner.Username,
				evt.FeedEvent.Event.EventData.TokensAddedToCollection.Collection.Name,
				collectionURL(
					evt.FeedEvent.Event.EventData.CollectorsNoteAddedToToken.Owner.Username,
					evt.FeedEvent.Event.EventData.TokensAddedToCollection.Collection.Dbid,
				),
			)
			return msg, nil
		}
		return fmt.Sprintf("**%s** added pieces to their collection: %s",
			evt.FeedEvent.Event.EventData.TokensAddedToCollection.Owner.Username,
			collectionURL(
				evt.FeedEvent.Event.EventData.CollectorsNoteAddedToToken.Owner.Username,
				evt.FeedEvent.Event.EventData.TokensAddedToCollection.Collection.Dbid,
			),
		), nil
	}

	if evt.FeedEvent.Event.EventData.TokensAddedToCollection.Collection.Name != "" && tokenName != "" {
		msg := fmt.Sprintf("**%s** added *%s* ", evt.FeedEvent.Event.EventData.TokensAddedToCollection.Owner.Username, tokenName)
		if tokensAdded == 1 {
			msg += fmt.Sprintf("to their collection, *%s*: %s",
				evt.FeedEvent.Event.EventData.TokensAddedToCollection.Collection.Name,
				collectionURL(
					evt.FeedEvent.Event.EventData.CollectorsNoteAddedToToken.Owner.Username,
					evt.FeedEvent.Event.EventData.TokensAddedToCollection.Collection.Dbid,
				),
			)
			return msg, nil
		} else {
			msg += fmt.Sprintf("and %v other NFT(s) to their collection, *%s*: %s",
				tokensAdded-1,
				evt.FeedEvent.Event.EventData.TokensAddedToCollection.Collection.Name,
				collectionURL(
					evt.FeedEvent.Event.EventData.CollectorsNoteAddedToToken.Owner.Username,
					evt.FeedEvent.Event.EventData.TokensAddedToCollection.Collection.Dbid,
				),
			)
			return msg, nil
		}
	} else if evt.FeedEvent.Event.EventData.TokensAddedToCollection.Collection.Name == "" && tokenName != "" {
		msg := fmt.Sprintf("**%s** added *%s* ", evt.FeedEvent.Event.EventData.TokensAddedToCollection.Owner.Username, tokenName)
		if tokensAdded == 1 {
			msg += fmt.Sprintf("to their collection: %s", collectionURL(
				evt.FeedEvent.Event.EventData.CollectorsNoteAddedToToken.Owner.Username,
				evt.FeedEvent.Event.EventData.TokensAddedToCollection.Collection.Dbid,
			))
			return msg, nil
		} else {
			msg += fmt.Sprintf("and %v other NFT(s) to their collection: %s",
				tokensAdded-1,
				collectionURL(
					evt.FeedEvent.Event.EventData.CollectorsNoteAddedToToken.Owner.Username,
					evt.FeedEvent.Event.EventData.TokensAddedToCollection.Collection.Dbid,
				),
			)
			return msg, nil
		}
	} else if evt.FeedEvent.Event.EventData.TokensAddedToCollection.Collection.Name != "" && tokenName == "" {
		return fmt.Sprintf("**%s** added %v NFT(s) to their collection, *%s*: %s",
			evt.FeedEvent.Event.EventData.CollectorsNoteAddedToToken.Owner.Username,
			tokensAdded,
			evt.FeedEvent.Event.EventData.TokensAddedToCollection.Collection.Name,
			collectionURL(
				evt.FeedEvent.Event.EventData.CollectorsNoteAddedToToken.Owner.Username,
				evt.FeedEvent.Event.EventData.TokensAddedToCollection.Collection.Dbid,
			),
		), nil
	} else {
		return fmt.Sprintf("**%s** added %v NFT(s) to their collection: %s",
			evt.FeedEvent.Event.EventData.CollectorsNoteAddedToToken.Owner.Username,
			tokensAdded,
			collectionURL(
				evt.FeedEvent.Event.EventData.CollectorsNoteAddedToToken.Owner.Username,
				evt.FeedEvent.Event.EventData.TokensAddedToCollection.Collection.Dbid,
			),
		), nil
	}
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

type FeedEventQuery struct {
	FeedEvent struct {
		Event struct {
			EventData struct {
				UserCreated struct {
					Owner UserFragment
				} `graphql:"...on UserCreatedFeedEventData"`
				UserFollowedUsers struct {
					Owner    UserFragment
					Followed []struct {
						User UserFragment
					}
				} `graphql:"...on UserFollowedUsersFeedEventData"`
				CollectorsNoteAddedToToken struct {
					Owner UserFragment
					Token struct {
						Token      TokenFragment
						Collection CollectionFragment
					}
				} `graphql:"...on CollectorsNoteAddedToTokenFeedEventData"`
				CollectionCreated struct {
					Owner      UserFragment
					Collection CollectionFragment
				} `graphql:"...on CollectionCreatedFeedEventData"`
				CollectorsNoteAddedToCollection struct {
					Owner      UserFragment
					Collection CollectionFragment
				} `graphql:"...on CollectorsNoteAddedToCollectionFeedEventData"`
				TokensAddedToCollection struct {
					Owner      UserFragment
					Collection CollectionFragment
					NewTokens  []struct {
						Token TokenFragment
					}
					IsPreFeed bool
				} `graphql:"...on TokensAddedToCollectionFeedEventData"`
			}
		} `graphql:"...on FeedEvent"`
	} `graphql:"feedEventById(id: $id)"`
}
