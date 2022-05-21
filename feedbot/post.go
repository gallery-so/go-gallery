package feedbot

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/mikeydub/go-gallery/service/logger"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/util"
	"github.com/sirupsen/logrus"
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
		Id   string
		Name string
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

type FeedPoster interface {
	handleQuery(context.Context, Query) error
}

type FeedPost struct {
	name   string
	rule   []func(Query) bool
	poster FeedPoster
}

func (f *FeedPost) Matches(q Query) bool {
	for _, criterion := range f.rule {
		eval := criterion(q)

		if log := logger.For(nil); log.Level <= logrus.DebugLevel {
			parts := strings.Split(util.FuncName(criterion), ".")
			log.Debugf("%s:%s evaluated to: %v", f.name, parts[len(parts)-1], eval)
		}

		if !eval {
			return false
		}
	}
	return true
}

func (f *FeedPost) Handle(ctx context.Context, q Query) (bool, error) {
	if !f.Matches(q) {
		return false, nil
	}

	err := f.poster.handleQuery(ctx, q)
	if err != nil {
		return false, err
	}

	return true, nil
}

type FeedRules struct {
	posts []*FeedPost
}

func newFeedRules() *FeedRules {
	criteria := FeedCriteria{}
	base := []func(Query) bool{
		criteria.EventIsValid,
		criteria.EventNoMoreRecentEvents,
		criteria.UserHasUsername,
	}
	return &FeedRules{
		posts: []*FeedPost{
			newUserCreatedPost(criteria, base),
			newUserFollowedPost(criteria, base),
			newNftCollectorsNoteAddedPost(criteria, base),
			newCollectionCreatedPost(criteria, base),
			newCollectionCollectorsNoteAddedPost(criteria, base),
			newCollectionTokensAddedPost(criteria, base),
		},
	}
}

func (p *FeedRules) Handle(ctx context.Context, q Query) (bool, error) {
	logger.For(ctx).Debugf("handling query %s", q)
	for _, post := range p.posts {
		handled, err := post.Handle(ctx, q)
		if err != nil {
			return false, err
		}
		if handled {
			return true, nil
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

func newUserCreatedPost(critiera FeedCriteria, baseRule []func(Query) bool) *FeedPost {
	return &FeedPost{
		name: "UserCreated",
		rule: append(
			baseRule,
			critiera.IsUserCreatedEvent,
			critiera.UserNoEventsBefore,
		),
		poster: &DiscordPoster{
			renderFromQuery: func(q Query) string {
				return fmt.Sprintf("**%s** joined Gallery: %s", q.Username, userURL(q.Username))
			},
		},
	}
}

func newUserFollowedPost(criteria FeedCriteria, baseRule []func(Query) bool) *FeedPost {
	return &FeedPost{
		name: "UserFollowed",
		rule: append(
			baseRule,
			criteria.IsUserFollowedEvent,
			criteria.FollowedUserHasUsername,
		),
		poster: &DiscordPoster{
			renderFromQuery: func(q Query) string {
				return fmt.Sprintf("**%s** followed **%s**: %s", q.Username, q.FollowedUsername, userURL(q.FollowedUsername))
			},
		},
	}
}

func newNftCollectorsNoteAddedPost(criteria FeedCriteria, baseRule []func(Query) bool) *FeedPost {
	return &FeedPost{
		name: "NftCollectorsNoteAdded",
		rule: append(
			baseRule,
			criteria.IsNftCollectorsNoteAddedEvent,
			criteria.NftHasCollectorsNote,
			criteria.NftBelongsToCollection,
		),
		poster: &DiscordPoster{
			renderFromQuery: func(q Query) string {
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

func newCollectionCreatedPost(criteria FeedCriteria, baseRule []func(Query) bool) *FeedPost {
	return &FeedPost{
		name: "CollectionCreated",
		rule: append(
			baseRule,
			criteria.IsCollectionCreatedEvent,
			criteria.CollectionHasNfts,
		),
		poster: &DiscordPoster{
			renderFromQuery: func(q Query) string {
				var message string
				if q.CollectionName != "" {
					message = fmt.Sprintf("**%s** created a collection, *%s*: %s", q.Username, q.CollectionName, collectionURL(q.Username, q.CollectionID.String()))
				} else {
					message = fmt.Sprintf("**%s** created a collection: %s", q.Username, collectionURL(q.Username, q.CollectionID.String()))
				}
				return message
			},
		},
	}
}

func newCollectionCollectorsNoteAddedPost(criteria FeedCriteria, baseRule []func(Query) bool) *FeedPost {
	return &FeedPost{
		name: "CollectionCollectorsNoteAdded",
		rule: append(
			baseRule,
			criteria.IsCollectionCollectorsNoteAddedEvent,
			criteria.CollectionHasCollectorsNote,
		),
		poster: &DiscordPoster{
			renderFromQuery: func(q Query) string {
				var message string
				if q.CollectionName != "" {
					message = fmt.Sprintf("**%s** added a collector's note to *%s*: %s", q.Username, q.CollectionName, collectionURL(q.Username, q.CollectionID.String()))
				} else {
					message = fmt.Sprintf("**%s** added a collector's note to their collection: %s", q.Username, collectionURL(q.Username, q.CollectionID.String()))
				}
				return message
			},
		},
	}
}

func newCollectionTokensAddedPost(criteria FeedCriteria, baseRule []func(Query) bool) *FeedPost {
	return &FeedPost{
		name: "CollectionTokensAdded",
		rule: append(
			baseRule,
			criteria.IsCollectionTokensAddedEvent,
			criteria.CollectionHasNfts,
			criteria.CollectionHasNewTokensAdded,
		),
		poster: &DiscordPoster{
			renderFromQuery: func(q Query) string {
				var tokensAdded int
				var tokenName string

				for _, nft := range q.CollectionNfts {
					contains := false
					for _, otherId := range q.LastCollectionEvent.Data.NFTs {
						if nft.Id == otherId.String() {
							contains = true
							break
						}
					}
					if !contains {
						tokensAdded++
						if tokenName == "" && nft.Name != "" {
							tokenName = nft.Name
						}
					}
				}

				url := collectionURL(q.Username, q.CollectionID.String())
				var message string
				if q.CollectionName != "" && tokenName != "" {
					message := fmt.Sprintf("**%s** added *%s* ", q.Username, tokenName)
					if tokensAdded == 1 {
						message += fmt.Sprintf("to *%s*: %s", q.CollectionName, url)
					} else {
						message += fmt.Sprintf("and %v other NFT(s) to their collection: %s", tokensAdded-1, url)
					}
				} else if q.CollectionName == "" && tokenName != "" {
					message := fmt.Sprintf("**%s** added *%s* ", q.Username, tokenName)
					if tokensAdded == 1 {
						message += fmt.Sprintf("to their collection: %s", url)
					} else {
						message += fmt.Sprintf("and %v other NFT(s) to their collection: %s", tokensAdded-1, url)
					}
				} else if q.CollectionName != "" && tokenName == "" {
					message = fmt.Sprintf("**%s** added %v NFT(s) to *%s*: %s", q.Username, tokensAdded, q.CollectionName, url)
				} else {
					message = fmt.Sprintf("**%s** added %v NFT(s) to their collection: %s", q.Username, tokensAdded, url)
				}

				return message
			},
		},
	}
}
