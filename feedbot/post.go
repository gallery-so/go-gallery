package feedbot

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/mikeydub/go-gallery/service/logger"
	"github.com/spf13/viper"
)

type Post struct {
	name     string
	criteria []func(Query) bool
	poster   Poster
}

type Poster interface {
	Send(context.Context, Query) error
}

type DiscordPoster struct {
	render func(Query) string
}

func (d *DiscordPoster) Send(ctx context.Context, q Query) error {
	content := d.render(q)

	message, err := json.Marshal(map[string]interface{}{
		"content": content,
		"tts":     false,
	})

	if err != nil {
		return err
	}

	return sendMessage(ctx, message)
}

func (p *Post) Handle(ctx context.Context, q Query) (bool, error) {
	if err := p.poster.Send(ctx, q); err != nil {
		return false, err
	}
	return true, nil
}

func (p *Post) Matches(q Query) bool {
	for i, criteria := range p.criteria {
		match := criteria(q)
		logger.For(nil).Debugf("%s.%d is %v", p.name, i, match)
		if !match {
			return false
		}
	}
	return true
}

type Feed []*Post

var feedPosts = Feed{
	userCreatedPost(),
	userFollowedPost(),
	nftCollectorsNoteAddedPost(),
	collectionCreatedPost(),
	collectionCollectorsNoteAddedPost(),
	collectionTokensAddedPost(),
}

func (f Feed) SearchFor(ctx context.Context, q Query) (bool, error) {
	logger.For(ctx).Debugf("handling event=%s; query=%v", q.EventID, q)

	var handled bool
	var err error

	for _, p := range f {
		if p.Matches(q) {
			logger.For(ctx).Debugf("event=%s matches post=%s", q.EventID, p.name)
			handled, err = p.Handle(ctx, q)
			break
		}
	}

	logger.For(ctx).Debugf("event=%s handled=%v err=%v", q.EventID, handled, err)
	return handled, err
}

func userURL(username string) string {
	return fmt.Sprintf("%s/%s", viper.GetString("GALLERY_HOST"), username)
}

func collectionURL(username, collectionID string) string {
	return fmt.Sprintf("%s/%s", userURL(username), collectionID)
}

func nftURL(username, collectionID, nftID string) string {
	return fmt.Sprintf("%s/%s", collectionURL(username, collectionID), nftID)
}

func userCreatedPost() *Post {
	return &Post{
		name: "UserCreated",
		criteria: append(
			baseCriteria,
			isUserFollowedEvent,
			userNoEventsBefore,
		),
		poster: &DiscordPoster{
			render: func(q Query) string {
				return fmt.Sprintf("**%s** joined Gallery: %s", q.Username, userURL(q.Username))
			},
		},
	}
}

func userFollowedPost() *Post {
	return &Post{
		name: "UserFollowed",
		criteria: append(
			baseCriteria,
			isUserFollowedEvent,
			followedUserHasUsername,
		),
		poster: &DiscordPoster{
			render: func(q Query) string {
				return fmt.Sprintf("**%s** followed **%s**: %s", q.Username, q.FollowedUsername, userURL(q.FollowedUsername))
			},
		},
	}
}

func nftCollectorsNoteAddedPost() *Post {
	return &Post{
		name: "NftCollectorsNoteAdded",
		criteria: append(
			baseCriteria,
			isNftCollectorsNoteAddedEvent,
			nftHasCollectorsNote,
			nftBelongsToCollection,
		),
		poster: &DiscordPoster{
			render: func(q Query) string {
				var message string
				if q.NftName != "" {
					message = fmt.Sprintf("**%s** added a collector's note to *%s*: %s", q.Username, q.NftName, nftURL(q.Username, q.CollectionID.String(), q.NftID.String()))
				} else {
					message = fmt.Sprintf("**%s** added a collector's note to their NFT: %s", q.Username, nftURL(q.Username, q.CollectionID.String(), q.NftID.String()))
				}
				return message
			},
		},
	}
}

func collectionCreatedPost() *Post {
	return &Post{
		name: "CollectionCreated",
		criteria: append(
			baseCriteria,
			isCollectionCreatedEvent,
			collectionHasNfts,
		),
		poster: &DiscordPoster{
			render: func(q Query) string {
				var message string
				if q.CollectionName != "" {
					message = fmt.Sprintf("**%s** created a collection titled '*%s'*: %s", q.Username, q.CollectionName, collectionURL(q.Username, q.CollectionID.String()))
				} else {
					message = fmt.Sprintf("**%s** created a collection: %s", q.Username, collectionURL(q.Username, q.CollectionID.String()))
				}
				return message
			},
		},
	}
}

func collectionCollectorsNoteAddedPost() *Post {
	return &Post{
		name: "CollectionCollectorsNoteAdded",
		criteria: append(
			baseCriteria,
			isCollectionCollectorsNoteAddedEvent,
			collectionHasCollectorsNote,
		),
		poster: &DiscordPoster{
			render: func(q Query) string {
				var message string
				if q.CollectionName != "" {
					message = fmt.Sprintf("**%s** added a collector's note to their collection, *%s*: %s", q.Username, q.CollectionName, collectionURL(q.Username, q.CollectionID.String()))
				} else {
					message = fmt.Sprintf("**%s** added a collector's note to their collection: %s", q.Username, collectionURL(q.Username, q.CollectionID.String()))
				}
				return message
			},
		},
	}
}

func collectionTokensAddedPost() *Post {
	return &Post{
		name: "CollectionTokensAdded",
		criteria: append(
			baseCriteria,
			isCollectionTokensAddedEvent,
			collectionHasNfts,
			collectionHasNewTokensAdded,
		),
		poster: &DiscordPoster{
			render: func(q Query) string {
				var tokensAdded int
				var tokenName string

				if q.LastCollectionEvent != nil {
					for _, nft := range q.CollectionNfts {
						contains := false
						for _, otherId := range q.LastCollectionEvent.Data.NFTs {
							if nft.Nft.Dbid == otherId {
								contains = true
								break
							}
						}
						if !contains {
							tokensAdded++
							if tokenName == "" && nft.Nft.Name != "" {
								tokenName = nft.Nft.Name
							}
						}
					}
				} else {
					tokensAdded = len(q.CollectionNfts)

					for _, nft := range q.CollectionNfts {
						if nft.Nft.Name != "" {
							tokenName = nft.Nft.Name
							break
						}
					}
				}

				url := collectionURL(q.Username, q.CollectionID.String())
				var message string

				if q.CollectionName != "" && tokenName != "" {
					message = fmt.Sprintf("**%s** added *%s* ", q.Username, tokenName)
					if tokensAdded == 1 {
						message += fmt.Sprintf("to their collection, *%s*: %s", q.CollectionName, url)
					} else {
						message += fmt.Sprintf("and %v other NFT(s) to their collection, *%s*: %s", tokensAdded-1, q.CollectionName, url)
					}
				} else if q.CollectionName == "" && tokenName != "" {
					message = fmt.Sprintf("**%s** added *%s* ", q.Username, tokenName)
					if tokensAdded == 1 {
						message += fmt.Sprintf("to their collection: %s", url)
					} else {
						message += fmt.Sprintf("and %v other NFT(s) to their collection: %s", tokensAdded-1, url)
					}
				} else if q.CollectionName != "" && tokenName == "" {
					message = fmt.Sprintf("**%s** added %v NFT(s) to their collection, *%s*: %s", q.Username, tokensAdded, q.CollectionName, url)
				} else {
					message = fmt.Sprintf("**%s** added %v NFT(s) to their collection: %s", q.Username, tokensAdded, url)
				}

				return message
			},
		},
	}
}
