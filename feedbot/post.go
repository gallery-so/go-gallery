package feedbot

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/mikeydub/go-gallery/service/logger"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/spf13/viper"
)

type Query struct {
	EventID           persist.DBID
	EventCode         persist.EventCode
	EventsSince       int
	UserID            persist.DBID
	Username          string
	NftID             persist.DBID
	NftName           string
	NftCollectorsNote string
	CollectionID      persist.DBID
	CollectionName    string
	CollectionNfts    []struct {
		Nft struct {
			Dbid persist.DBID
			Name string
		}
	}
	CollectionCollectorsNote string
	FollowedUserID           persist.DBID
	FollowedUsername         string
	LastUserEvent            *persist.UserEventRecord
	LastNftEvent             *persist.NftEventRecord
	LastCollectionEvent      *persist.CollectionEventRecord
}

func (q Query) String() string {
	b, _ := json.MarshalIndent(q, "", "\t")
	return string(b)
}

type Post struct {
	name   string
	rule   []Criterion
	poster Poster
}

func (f *Post) Handle(ctx context.Context, q Query) (bool, error) {
	if err := f.poster.withQuery(ctx, q); err != nil {
		return false, err
	}
	return true, nil
}

type Poster interface {
	withQuery(context.Context, Query) error
}

type Feed struct {
	User       []*Post
	Nft        []*Post
	Collection []*Post
	cache      map[string]bool
	mu         sync.Mutex
}

func newFeed() *Feed {
	criteria := newCriteria()
	base := []Criterion{
		criteria.EventIsValid,
		criteria.EventNoMoreRecentEvents,
		criteria.UserHasUsername,
	}
	return &Feed{
		User: []*Post{
			newUserCreatedPost(criteria, base),
			newUserFollowedPost(criteria, base),
		},
		Nft: []*Post{
			newNftCollectorsNoteAddedPost(criteria, base),
		},
		Collection: []*Post{
			newCollectionCreatedPost(criteria, base),
			newCollectionCollectorsNoteAddedPost(criteria, base),
			newCollectionTokensAddedPost(criteria, base),
		},
		cache: map[string]bool{},
	}
}

func (f *Feed) postMatches(post *Post, q Query) bool {
	for _, c := range post.rule {
		key := fmt.Sprintf("%s:%s", q.EventID, c.name)

		if _, ok := f.cache[key]; !ok {
			f.mu.Lock()
			f.cache[key] = c.eval(q)
			f.mu.Unlock()
		}

		logger.For(nil).Debugf("%s:%s evaluated query as: %v", post.name, c.name, f.cache[key])

		if !f.cache[key] {
			return false
		}
	}
	return true
}

func (f *Feed) SearchFor(ctx context.Context, q Query) (bool, error) {
	logger.For(ctx).Debugf("handling query: %s", q)

	var posts []*Post

	switch persist.CategoryFromEventCode(q.EventCode) {
	case persist.UserEventCode:
		posts = f.User
	case persist.NftEventCode:
		posts = f.Nft
	case persist.CollectionEventCode:
		posts = f.Collection
	}

	for _, p := range posts {
		if f.postMatches(p, q) {
			handled, err := p.Handle(ctx, q)

			if err != nil {
				return false, err
			}

			return handled, nil
		}
	}

	return false, nil
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

func newUserCreatedPost(critiera Criteria, baseRule []Criterion) *Post {
	return &Post{
		name: "UserCreated",
		rule: append(
			baseRule,
			critiera.IsUserCreatedEvent,
			critiera.UserNoEventsBefore,
		),
		poster: &DiscordPoster{
			renderQuery: func(q Query) string {
				return fmt.Sprintf("**%s** joined Gallery: %s", q.Username, userURL(q.Username))
			},
		},
	}
}

func newUserFollowedPost(criteria Criteria, baseRule []Criterion) *Post {
	return &Post{
		name: "UserFollowed",
		rule: append(
			baseRule,
			criteria.IsUserFollowedEvent,
			criteria.FollowedUserHasUsername,
		),
		poster: &DiscordPoster{
			renderQuery: func(q Query) string {
				return fmt.Sprintf("**%s** followed **%s**: %s", q.Username, q.FollowedUsername, userURL(q.FollowedUsername))
			},
		},
	}
}

func newNftCollectorsNoteAddedPost(criteria Criteria, baseRule []Criterion) *Post {
	return &Post{
		name: "NftCollectorsNoteAdded",
		rule: append(
			baseRule,
			criteria.IsNftCollectorsNoteAddedEvent,
			criteria.NftHasCollectorsNote,
			criteria.NftBelongsToCollection,
		),
		poster: &DiscordPoster{
			renderQuery: func(q Query) string {
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

func newCollectionCreatedPost(criteria Criteria, baseRule []Criterion) *Post {
	return &Post{
		name: "CollectionCreated",
		rule: append(
			baseRule,
			criteria.IsCollectionCreatedEvent,
			criteria.CollectionHasNfts,
		),
		poster: &DiscordPoster{
			renderQuery: func(q Query) string {
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

func newCollectionCollectorsNoteAddedPost(criteria Criteria, baseRule []Criterion) *Post {
	return &Post{
		name: "CollectionCollectorsNoteAdded",
		rule: append(
			baseRule,
			criteria.IsCollectionCollectorsNoteAddedEvent,
			criteria.CollectionHasCollectorsNote,
		),
		poster: &DiscordPoster{
			renderQuery: func(q Query) string {
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

func newCollectionTokensAddedPost(criteria Criteria, baseRule []Criterion) *Post {
	return &Post{
		name: "CollectionTokensAdded",
		rule: append(
			baseRule,
			criteria.IsCollectionTokensAddedEvent,
			criteria.CollectionHasNfts,
			criteria.CollectionHasNewTokensAdded,
		),
		poster: &DiscordPoster{
			renderQuery: func(q Query) string {
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
