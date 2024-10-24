// Code generated by github.com/99designs/gqlgen, DO NOT EDIT.

package model

import (
	"context"
	"fmt"
	"strings"

	"github.com/mikeydub/go-gallery/service/persist"
)

func (r *Admire) ID() GqlID {
	return GqlID(fmt.Sprintf("Admire:%s", r.Dbid))
}

func (r *Collection) ID() GqlID {
	return GqlID(fmt.Sprintf("Collection:%s", r.Dbid))
}

func (r *CollectionToken) ID() GqlID {
	//-----------------------------------------------------------------------------------------------
	//-----------------------------------------------------------------------------------------------
	// Some fields specified by @goGqlId require manual binding because one of the following is true:
	// (a) the field does not exist on the CollectionToken type, or
	// (b) the field exists but is not a string type
	//-----------------------------------------------------------------------------------------------
	// Please create binding methods on the CollectionToken type with the following signatures:
	// func (r *CollectionToken) GetGqlIDField_TokenID() string
	// func (r *CollectionToken) GetGqlIDField_CollectionID() string
	//-----------------------------------------------------------------------------------------------
	return GqlID(fmt.Sprintf("CollectionToken:%s:%s", r.GetGqlIDField_TokenID(), r.GetGqlIDField_CollectionID()))
}

func (r *Comment) ID() GqlID {
	return GqlID(fmt.Sprintf("Comment:%s", r.Dbid))
}

func (r *Community) ID() GqlID {
	return GqlID(fmt.Sprintf("Community:%s", r.Dbid))
}

func (r *Contract) ID() GqlID {
	return GqlID(fmt.Sprintf("Contract:%s", r.Dbid))
}

func (r *DeletedNode) ID() GqlID {
	return GqlID(fmt.Sprintf("DeletedNode:%s", r.Dbid))
}

func (r *FeedEvent) ID() GqlID {
	return GqlID(fmt.Sprintf("FeedEvent:%s", r.Dbid))
}

func (r *Gallery) ID() GqlID {
	return GqlID(fmt.Sprintf("Gallery:%s", r.Dbid))
}

func (r *GalleryAnnouncementNotification) ID() GqlID {
	return GqlID(fmt.Sprintf("GalleryAnnouncementNotification:%s", r.Dbid))
}

func (r *GalleryUser) ID() GqlID {
	return GqlID(fmt.Sprintf("GalleryUser:%s", r.Dbid))
}

func (r *MembershipTier) ID() GqlID {
	return GqlID(fmt.Sprintf("MembershipTier:%s", r.Dbid))
}

func (r *MerchToken) ID() GqlID {
	return GqlID(fmt.Sprintf("MerchToken:%s", r.TokenID))
}

func (r *NewTokensNotification) ID() GqlID {
	return GqlID(fmt.Sprintf("NewTokensNotification:%s", r.Dbid))
}

func (r *Post) ID() GqlID {
	return GqlID(fmt.Sprintf("Post:%s", r.Dbid))
}

func (r *SocialConnection) ID() GqlID {
	return GqlID(fmt.Sprintf("SocialConnection:%s:%s", r.SocialID, r.SocialType))
}

func (r *SomeoneAdmiredYourCommentNotification) ID() GqlID {
	return GqlID(fmt.Sprintf("SomeoneAdmiredYourCommentNotification:%s", r.Dbid))
}

func (r *SomeoneAdmiredYourFeedEventNotification) ID() GqlID {
	return GqlID(fmt.Sprintf("SomeoneAdmiredYourFeedEventNotification:%s", r.Dbid))
}

func (r *SomeoneAdmiredYourPostNotification) ID() GqlID {
	return GqlID(fmt.Sprintf("SomeoneAdmiredYourPostNotification:%s", r.Dbid))
}

func (r *SomeoneAdmiredYourTokenNotification) ID() GqlID {
	return GqlID(fmt.Sprintf("SomeoneAdmiredYourTokenNotification:%s", r.Dbid))
}

func (r *SomeoneCommentedOnYourFeedEventNotification) ID() GqlID {
	return GqlID(fmt.Sprintf("SomeoneCommentedOnYourFeedEventNotification:%s", r.Dbid))
}

func (r *SomeoneCommentedOnYourPostNotification) ID() GqlID {
	return GqlID(fmt.Sprintf("SomeoneCommentedOnYourPostNotification:%s", r.Dbid))
}

func (r *SomeoneFollowedYouBackNotification) ID() GqlID {
	return GqlID(fmt.Sprintf("SomeoneFollowedYouBackNotification:%s", r.Dbid))
}

func (r *SomeoneFollowedYouNotification) ID() GqlID {
	return GqlID(fmt.Sprintf("SomeoneFollowedYouNotification:%s", r.Dbid))
}

func (r *SomeoneMentionedYouNotification) ID() GqlID {
	return GqlID(fmt.Sprintf("SomeoneMentionedYouNotification:%s", r.Dbid))
}

func (r *SomeoneMentionedYourCommunityNotification) ID() GqlID {
	return GqlID(fmt.Sprintf("SomeoneMentionedYourCommunityNotification:%s", r.Dbid))
}

func (r *SomeonePostedYourWorkNotification) ID() GqlID {
	return GqlID(fmt.Sprintf("SomeonePostedYourWorkNotification:%s", r.Dbid))
}

func (r *SomeoneRepliedToYourCommentNotification) ID() GqlID {
	return GqlID(fmt.Sprintf("SomeoneRepliedToYourCommentNotification:%s", r.Dbid))
}

func (r *SomeoneViewedYourGalleryNotification) ID() GqlID {
	return GqlID(fmt.Sprintf("SomeoneViewedYourGalleryNotification:%s", r.Dbid))
}

func (r *SomeoneYouFollowOnFarcasterJoinedNotification) ID() GqlID {
	return GqlID(fmt.Sprintf("SomeoneYouFollowOnFarcasterJoinedNotification:%s", r.Dbid))
}

func (r *SomeoneYouFollowPostedTheirFirstPostNotification) ID() GqlID {
	return GqlID(fmt.Sprintf("SomeoneYouFollowPostedTheirFirstPostNotification:%s", r.Dbid))
}

func (r *Token) ID() GqlID {
	return GqlID(fmt.Sprintf("Token:%s", r.Dbid))
}

func (r *TokenDefinition) ID() GqlID {
	return GqlID(fmt.Sprintf("TokenDefinition:%s", r.Dbid))
}

func (r *Viewer) ID() GqlID {
	//-----------------------------------------------------------------------------------------------
	//-----------------------------------------------------------------------------------------------
	// Some fields specified by @goGqlId require manual binding because one of the following is true:
	// (a) the field does not exist on the Viewer type, or
	// (b) the field exists but is not a string type
	//-----------------------------------------------------------------------------------------------
	// Please create binding methods on the Viewer type with the following signatures:
	// func (r *Viewer) GetGqlIDField_UserID() string
	//-----------------------------------------------------------------------------------------------
	return GqlID(fmt.Sprintf("Viewer:%s", r.GetGqlIDField_UserID()))
}

func (r *Wallet) ID() GqlID {
	return GqlID(fmt.Sprintf("Wallet:%s", r.Dbid))
}

func (r *YouReceivedTopActivityBadgeNotification) ID() GqlID {
	return GqlID(fmt.Sprintf("YouReceivedTopActivityBadgeNotification:%s", r.Dbid))
}

type NodeFetcher struct {
	OnAdmire                                           func(ctx context.Context, dbid persist.DBID) (*Admire, error)
	OnCollection                                       func(ctx context.Context, dbid persist.DBID) (*Collection, error)
	OnCollectionToken                                  func(ctx context.Context, tokenId string, collectionId string) (*CollectionToken, error)
	OnComment                                          func(ctx context.Context, dbid persist.DBID) (*Comment, error)
	OnCommunity                                        func(ctx context.Context, dbid persist.DBID) (*Community, error)
	OnContract                                         func(ctx context.Context, dbid persist.DBID) (*Contract, error)
	OnDeletedNode                                      func(ctx context.Context, dbid persist.DBID) (*DeletedNode, error)
	OnFeedEvent                                        func(ctx context.Context, dbid persist.DBID) (*FeedEvent, error)
	OnGallery                                          func(ctx context.Context, dbid persist.DBID) (*Gallery, error)
	OnGalleryAnnouncementNotification                  func(ctx context.Context, dbid persist.DBID) (*GalleryAnnouncementNotification, error)
	OnGalleryUser                                      func(ctx context.Context, dbid persist.DBID) (*GalleryUser, error)
	OnMembershipTier                                   func(ctx context.Context, dbid persist.DBID) (*MembershipTier, error)
	OnMerchToken                                       func(ctx context.Context, tokenId string) (*MerchToken, error)
	OnNewTokensNotification                            func(ctx context.Context, dbid persist.DBID) (*NewTokensNotification, error)
	OnPost                                             func(ctx context.Context, dbid persist.DBID) (*Post, error)
	OnSocialConnection                                 func(ctx context.Context, socialId string, socialType persist.SocialProvider) (*SocialConnection, error)
	OnSomeoneAdmiredYourCommentNotification            func(ctx context.Context, dbid persist.DBID) (*SomeoneAdmiredYourCommentNotification, error)
	OnSomeoneAdmiredYourFeedEventNotification          func(ctx context.Context, dbid persist.DBID) (*SomeoneAdmiredYourFeedEventNotification, error)
	OnSomeoneAdmiredYourPostNotification               func(ctx context.Context, dbid persist.DBID) (*SomeoneAdmiredYourPostNotification, error)
	OnSomeoneAdmiredYourTokenNotification              func(ctx context.Context, dbid persist.DBID) (*SomeoneAdmiredYourTokenNotification, error)
	OnSomeoneCommentedOnYourFeedEventNotification      func(ctx context.Context, dbid persist.DBID) (*SomeoneCommentedOnYourFeedEventNotification, error)
	OnSomeoneCommentedOnYourPostNotification           func(ctx context.Context, dbid persist.DBID) (*SomeoneCommentedOnYourPostNotification, error)
	OnSomeoneFollowedYouBackNotification               func(ctx context.Context, dbid persist.DBID) (*SomeoneFollowedYouBackNotification, error)
	OnSomeoneFollowedYouNotification                   func(ctx context.Context, dbid persist.DBID) (*SomeoneFollowedYouNotification, error)
	OnSomeoneMentionedYouNotification                  func(ctx context.Context, dbid persist.DBID) (*SomeoneMentionedYouNotification, error)
	OnSomeoneMentionedYourCommunityNotification        func(ctx context.Context, dbid persist.DBID) (*SomeoneMentionedYourCommunityNotification, error)
	OnSomeonePostedYourWorkNotification                func(ctx context.Context, dbid persist.DBID) (*SomeonePostedYourWorkNotification, error)
	OnSomeoneRepliedToYourCommentNotification          func(ctx context.Context, dbid persist.DBID) (*SomeoneRepliedToYourCommentNotification, error)
	OnSomeoneViewedYourGalleryNotification             func(ctx context.Context, dbid persist.DBID) (*SomeoneViewedYourGalleryNotification, error)
	OnSomeoneYouFollowOnFarcasterJoinedNotification    func(ctx context.Context, dbid persist.DBID) (*SomeoneYouFollowOnFarcasterJoinedNotification, error)
	OnSomeoneYouFollowPostedTheirFirstPostNotification func(ctx context.Context, dbid persist.DBID) (*SomeoneYouFollowPostedTheirFirstPostNotification, error)
	OnToken                                            func(ctx context.Context, dbid persist.DBID) (*Token, error)
	OnTokenDefinition                                  func(ctx context.Context, dbid persist.DBID) (*TokenDefinition, error)
	OnViewer                                           func(ctx context.Context, userId string) (*Viewer, error)
	OnWallet                                           func(ctx context.Context, dbid persist.DBID) (*Wallet, error)
	OnYouReceivedTopActivityBadgeNotification          func(ctx context.Context, dbid persist.DBID) (*YouReceivedTopActivityBadgeNotification, error)
}

func (n *NodeFetcher) GetNodeByGqlID(ctx context.Context, id GqlID) (Node, error) {
	parts := strings.Split(string(id), ":")
	if len(parts) == 1 {
		return nil, ErrInvalidIDFormat{message: "no ID components specified after type name"}
	}

	typeName := parts[0]
	ids := parts[1:]

	switch typeName {
	case "Admire":
		if len(ids) != 1 {
			return nil, ErrInvalidIDFormat{message: fmt.Sprintf("'Admire' type requires 1 ID component(s) (%d component(s) supplied)", len(ids))}
		}
		return n.OnAdmire(ctx, persist.DBID(ids[0]))
	case "Collection":
		if len(ids) != 1 {
			return nil, ErrInvalidIDFormat{message: fmt.Sprintf("'Collection' type requires 1 ID component(s) (%d component(s) supplied)", len(ids))}
		}
		return n.OnCollection(ctx, persist.DBID(ids[0]))
	case "CollectionToken":
		if len(ids) != 2 {
			return nil, ErrInvalidIDFormat{message: fmt.Sprintf("'CollectionToken' type requires 2 ID component(s) (%d component(s) supplied)", len(ids))}
		}
		return n.OnCollectionToken(ctx, string(ids[0]), string(ids[1]))
	case "Comment":
		if len(ids) != 1 {
			return nil, ErrInvalidIDFormat{message: fmt.Sprintf("'Comment' type requires 1 ID component(s) (%d component(s) supplied)", len(ids))}
		}
		return n.OnComment(ctx, persist.DBID(ids[0]))
	case "Community":
		if len(ids) != 1 {
			return nil, ErrInvalidIDFormat{message: fmt.Sprintf("'Community' type requires 1 ID component(s) (%d component(s) supplied)", len(ids))}
		}
		return n.OnCommunity(ctx, persist.DBID(ids[0]))
	case "Contract":
		if len(ids) != 1 {
			return nil, ErrInvalidIDFormat{message: fmt.Sprintf("'Contract' type requires 1 ID component(s) (%d component(s) supplied)", len(ids))}
		}
		return n.OnContract(ctx, persist.DBID(ids[0]))
	case "DeletedNode":
		if len(ids) != 1 {
			return nil, ErrInvalidIDFormat{message: fmt.Sprintf("'DeletedNode' type requires 1 ID component(s) (%d component(s) supplied)", len(ids))}
		}
		return n.OnDeletedNode(ctx, persist.DBID(ids[0]))
	case "FeedEvent":
		if len(ids) != 1 {
			return nil, ErrInvalidIDFormat{message: fmt.Sprintf("'FeedEvent' type requires 1 ID component(s) (%d component(s) supplied)", len(ids))}
		}
		return n.OnFeedEvent(ctx, persist.DBID(ids[0]))
	case "Gallery":
		if len(ids) != 1 {
			return nil, ErrInvalidIDFormat{message: fmt.Sprintf("'Gallery' type requires 1 ID component(s) (%d component(s) supplied)", len(ids))}
		}
		return n.OnGallery(ctx, persist.DBID(ids[0]))
	case "GalleryAnnouncementNotification":
		if len(ids) != 1 {
			return nil, ErrInvalidIDFormat{message: fmt.Sprintf("'GalleryAnnouncementNotification' type requires 1 ID component(s) (%d component(s) supplied)", len(ids))}
		}
		return n.OnGalleryAnnouncementNotification(ctx, persist.DBID(ids[0]))
	case "GalleryUser":
		if len(ids) != 1 {
			return nil, ErrInvalidIDFormat{message: fmt.Sprintf("'GalleryUser' type requires 1 ID component(s) (%d component(s) supplied)", len(ids))}
		}
		return n.OnGalleryUser(ctx, persist.DBID(ids[0]))
	case "MembershipTier":
		if len(ids) != 1 {
			return nil, ErrInvalidIDFormat{message: fmt.Sprintf("'MembershipTier' type requires 1 ID component(s) (%d component(s) supplied)", len(ids))}
		}
		return n.OnMembershipTier(ctx, persist.DBID(ids[0]))
	case "MerchToken":
		if len(ids) != 1 {
			return nil, ErrInvalidIDFormat{message: fmt.Sprintf("'MerchToken' type requires 1 ID component(s) (%d component(s) supplied)", len(ids))}
		}
		return n.OnMerchToken(ctx, string(ids[0]))
	case "NewTokensNotification":
		if len(ids) != 1 {
			return nil, ErrInvalidIDFormat{message: fmt.Sprintf("'NewTokensNotification' type requires 1 ID component(s) (%d component(s) supplied)", len(ids))}
		}
		return n.OnNewTokensNotification(ctx, persist.DBID(ids[0]))
	case "Post":
		if len(ids) != 1 {
			return nil, ErrInvalidIDFormat{message: fmt.Sprintf("'Post' type requires 1 ID component(s) (%d component(s) supplied)", len(ids))}
		}
		return n.OnPost(ctx, persist.DBID(ids[0]))
	case "SocialConnection":
		if len(ids) != 2 {
			return nil, ErrInvalidIDFormat{message: fmt.Sprintf("'SocialConnection' type requires 2 ID component(s) (%d component(s) supplied)", len(ids))}
		}
		return n.OnSocialConnection(ctx, string(ids[0]), persist.SocialProvider(ids[1]))
	case "SomeoneAdmiredYourCommentNotification":
		if len(ids) != 1 {
			return nil, ErrInvalidIDFormat{message: fmt.Sprintf("'SomeoneAdmiredYourCommentNotification' type requires 1 ID component(s) (%d component(s) supplied)", len(ids))}
		}
		return n.OnSomeoneAdmiredYourCommentNotification(ctx, persist.DBID(ids[0]))
	case "SomeoneAdmiredYourFeedEventNotification":
		if len(ids) != 1 {
			return nil, ErrInvalidIDFormat{message: fmt.Sprintf("'SomeoneAdmiredYourFeedEventNotification' type requires 1 ID component(s) (%d component(s) supplied)", len(ids))}
		}
		return n.OnSomeoneAdmiredYourFeedEventNotification(ctx, persist.DBID(ids[0]))
	case "SomeoneAdmiredYourPostNotification":
		if len(ids) != 1 {
			return nil, ErrInvalidIDFormat{message: fmt.Sprintf("'SomeoneAdmiredYourPostNotification' type requires 1 ID component(s) (%d component(s) supplied)", len(ids))}
		}
		return n.OnSomeoneAdmiredYourPostNotification(ctx, persist.DBID(ids[0]))
	case "SomeoneAdmiredYourTokenNotification":
		if len(ids) != 1 {
			return nil, ErrInvalidIDFormat{message: fmt.Sprintf("'SomeoneAdmiredYourTokenNotification' type requires 1 ID component(s) (%d component(s) supplied)", len(ids))}
		}
		return n.OnSomeoneAdmiredYourTokenNotification(ctx, persist.DBID(ids[0]))
	case "SomeoneCommentedOnYourFeedEventNotification":
		if len(ids) != 1 {
			return nil, ErrInvalidIDFormat{message: fmt.Sprintf("'SomeoneCommentedOnYourFeedEventNotification' type requires 1 ID component(s) (%d component(s) supplied)", len(ids))}
		}
		return n.OnSomeoneCommentedOnYourFeedEventNotification(ctx, persist.DBID(ids[0]))
	case "SomeoneCommentedOnYourPostNotification":
		if len(ids) != 1 {
			return nil, ErrInvalidIDFormat{message: fmt.Sprintf("'SomeoneCommentedOnYourPostNotification' type requires 1 ID component(s) (%d component(s) supplied)", len(ids))}
		}
		return n.OnSomeoneCommentedOnYourPostNotification(ctx, persist.DBID(ids[0]))
	case "SomeoneFollowedYouBackNotification":
		if len(ids) != 1 {
			return nil, ErrInvalidIDFormat{message: fmt.Sprintf("'SomeoneFollowedYouBackNotification' type requires 1 ID component(s) (%d component(s) supplied)", len(ids))}
		}
		return n.OnSomeoneFollowedYouBackNotification(ctx, persist.DBID(ids[0]))
	case "SomeoneFollowedYouNotification":
		if len(ids) != 1 {
			return nil, ErrInvalidIDFormat{message: fmt.Sprintf("'SomeoneFollowedYouNotification' type requires 1 ID component(s) (%d component(s) supplied)", len(ids))}
		}
		return n.OnSomeoneFollowedYouNotification(ctx, persist.DBID(ids[0]))
	case "SomeoneMentionedYouNotification":
		if len(ids) != 1 {
			return nil, ErrInvalidIDFormat{message: fmt.Sprintf("'SomeoneMentionedYouNotification' type requires 1 ID component(s) (%d component(s) supplied)", len(ids))}
		}
		return n.OnSomeoneMentionedYouNotification(ctx, persist.DBID(ids[0]))
	case "SomeoneMentionedYourCommunityNotification":
		if len(ids) != 1 {
			return nil, ErrInvalidIDFormat{message: fmt.Sprintf("'SomeoneMentionedYourCommunityNotification' type requires 1 ID component(s) (%d component(s) supplied)", len(ids))}
		}
		return n.OnSomeoneMentionedYourCommunityNotification(ctx, persist.DBID(ids[0]))
	case "SomeonePostedYourWorkNotification":
		if len(ids) != 1 {
			return nil, ErrInvalidIDFormat{message: fmt.Sprintf("'SomeonePostedYourWorkNotification' type requires 1 ID component(s) (%d component(s) supplied)", len(ids))}
		}
		return n.OnSomeonePostedYourWorkNotification(ctx, persist.DBID(ids[0]))
	case "SomeoneRepliedToYourCommentNotification":
		if len(ids) != 1 {
			return nil, ErrInvalidIDFormat{message: fmt.Sprintf("'SomeoneRepliedToYourCommentNotification' type requires 1 ID component(s) (%d component(s) supplied)", len(ids))}
		}
		return n.OnSomeoneRepliedToYourCommentNotification(ctx, persist.DBID(ids[0]))
	case "SomeoneViewedYourGalleryNotification":
		if len(ids) != 1 {
			return nil, ErrInvalidIDFormat{message: fmt.Sprintf("'SomeoneViewedYourGalleryNotification' type requires 1 ID component(s) (%d component(s) supplied)", len(ids))}
		}
		return n.OnSomeoneViewedYourGalleryNotification(ctx, persist.DBID(ids[0]))
	case "SomeoneYouFollowOnFarcasterJoinedNotification":
		if len(ids) != 1 {
			return nil, ErrInvalidIDFormat{message: fmt.Sprintf("'SomeoneYouFollowOnFarcasterJoinedNotification' type requires 1 ID component(s) (%d component(s) supplied)", len(ids))}
		}
		return n.OnSomeoneYouFollowOnFarcasterJoinedNotification(ctx, persist.DBID(ids[0]))
	case "SomeoneYouFollowPostedTheirFirstPostNotification":
		if len(ids) != 1 {
			return nil, ErrInvalidIDFormat{message: fmt.Sprintf("'SomeoneYouFollowPostedTheirFirstPostNotification' type requires 1 ID component(s) (%d component(s) supplied)", len(ids))}
		}
		return n.OnSomeoneYouFollowPostedTheirFirstPostNotification(ctx, persist.DBID(ids[0]))
	case "Token":
		if len(ids) != 1 {
			return nil, ErrInvalidIDFormat{message: fmt.Sprintf("'Token' type requires 1 ID component(s) (%d component(s) supplied)", len(ids))}
		}
		return n.OnToken(ctx, persist.DBID(ids[0]))
	case "TokenDefinition":
		if len(ids) != 1 {
			return nil, ErrInvalidIDFormat{message: fmt.Sprintf("'TokenDefinition' type requires 1 ID component(s) (%d component(s) supplied)", len(ids))}
		}
		return n.OnTokenDefinition(ctx, persist.DBID(ids[0]))
	case "Viewer":
		if len(ids) != 1 {
			return nil, ErrInvalidIDFormat{message: fmt.Sprintf("'Viewer' type requires 1 ID component(s) (%d component(s) supplied)", len(ids))}
		}
		return n.OnViewer(ctx, string(ids[0]))
	case "Wallet":
		if len(ids) != 1 {
			return nil, ErrInvalidIDFormat{message: fmt.Sprintf("'Wallet' type requires 1 ID component(s) (%d component(s) supplied)", len(ids))}
		}
		return n.OnWallet(ctx, persist.DBID(ids[0]))
	case "YouReceivedTopActivityBadgeNotification":
		if len(ids) != 1 {
			return nil, ErrInvalidIDFormat{message: fmt.Sprintf("'YouReceivedTopActivityBadgeNotification' type requires 1 ID component(s) (%d component(s) supplied)", len(ids))}
		}
		return n.OnYouReceivedTopActivityBadgeNotification(ctx, persist.DBID(ids[0]))
	}

	return nil, ErrInvalidIDFormat{typeName}
}

func (n *NodeFetcher) ValidateHandlers() {
	switch {
	case n.OnAdmire == nil:
		panic("NodeFetcher handler validation failed: no handler set for NodeFetcher.OnAdmire")
	case n.OnCollection == nil:
		panic("NodeFetcher handler validation failed: no handler set for NodeFetcher.OnCollection")
	case n.OnCollectionToken == nil:
		panic("NodeFetcher handler validation failed: no handler set for NodeFetcher.OnCollectionToken")
	case n.OnComment == nil:
		panic("NodeFetcher handler validation failed: no handler set for NodeFetcher.OnComment")
	case n.OnCommunity == nil:
		panic("NodeFetcher handler validation failed: no handler set for NodeFetcher.OnCommunity")
	case n.OnContract == nil:
		panic("NodeFetcher handler validation failed: no handler set for NodeFetcher.OnContract")
	case n.OnDeletedNode == nil:
		panic("NodeFetcher handler validation failed: no handler set for NodeFetcher.OnDeletedNode")
	case n.OnFeedEvent == nil:
		panic("NodeFetcher handler validation failed: no handler set for NodeFetcher.OnFeedEvent")
	case n.OnGallery == nil:
		panic("NodeFetcher handler validation failed: no handler set for NodeFetcher.OnGallery")
	case n.OnGalleryAnnouncementNotification == nil:
		panic("NodeFetcher handler validation failed: no handler set for NodeFetcher.OnGalleryAnnouncementNotification")
	case n.OnGalleryUser == nil:
		panic("NodeFetcher handler validation failed: no handler set for NodeFetcher.OnGalleryUser")
	case n.OnMembershipTier == nil:
		panic("NodeFetcher handler validation failed: no handler set for NodeFetcher.OnMembershipTier")
	case n.OnMerchToken == nil:
		panic("NodeFetcher handler validation failed: no handler set for NodeFetcher.OnMerchToken")
	case n.OnNewTokensNotification == nil:
		panic("NodeFetcher handler validation failed: no handler set for NodeFetcher.OnNewTokensNotification")
	case n.OnPost == nil:
		panic("NodeFetcher handler validation failed: no handler set for NodeFetcher.OnPost")
	case n.OnSocialConnection == nil:
		panic("NodeFetcher handler validation failed: no handler set for NodeFetcher.OnSocialConnection")
	case n.OnSomeoneAdmiredYourCommentNotification == nil:
		panic("NodeFetcher handler validation failed: no handler set for NodeFetcher.OnSomeoneAdmiredYourCommentNotification")
	case n.OnSomeoneAdmiredYourFeedEventNotification == nil:
		panic("NodeFetcher handler validation failed: no handler set for NodeFetcher.OnSomeoneAdmiredYourFeedEventNotification")
	case n.OnSomeoneAdmiredYourPostNotification == nil:
		panic("NodeFetcher handler validation failed: no handler set for NodeFetcher.OnSomeoneAdmiredYourPostNotification")
	case n.OnSomeoneAdmiredYourTokenNotification == nil:
		panic("NodeFetcher handler validation failed: no handler set for NodeFetcher.OnSomeoneAdmiredYourTokenNotification")
	case n.OnSomeoneCommentedOnYourFeedEventNotification == nil:
		panic("NodeFetcher handler validation failed: no handler set for NodeFetcher.OnSomeoneCommentedOnYourFeedEventNotification")
	case n.OnSomeoneCommentedOnYourPostNotification == nil:
		panic("NodeFetcher handler validation failed: no handler set for NodeFetcher.OnSomeoneCommentedOnYourPostNotification")
	case n.OnSomeoneFollowedYouBackNotification == nil:
		panic("NodeFetcher handler validation failed: no handler set for NodeFetcher.OnSomeoneFollowedYouBackNotification")
	case n.OnSomeoneFollowedYouNotification == nil:
		panic("NodeFetcher handler validation failed: no handler set for NodeFetcher.OnSomeoneFollowedYouNotification")
	case n.OnSomeoneMentionedYouNotification == nil:
		panic("NodeFetcher handler validation failed: no handler set for NodeFetcher.OnSomeoneMentionedYouNotification")
	case n.OnSomeoneMentionedYourCommunityNotification == nil:
		panic("NodeFetcher handler validation failed: no handler set for NodeFetcher.OnSomeoneMentionedYourCommunityNotification")
	case n.OnSomeonePostedYourWorkNotification == nil:
		panic("NodeFetcher handler validation failed: no handler set for NodeFetcher.OnSomeonePostedYourWorkNotification")
	case n.OnSomeoneRepliedToYourCommentNotification == nil:
		panic("NodeFetcher handler validation failed: no handler set for NodeFetcher.OnSomeoneRepliedToYourCommentNotification")
	case n.OnSomeoneViewedYourGalleryNotification == nil:
		panic("NodeFetcher handler validation failed: no handler set for NodeFetcher.OnSomeoneViewedYourGalleryNotification")
	case n.OnSomeoneYouFollowOnFarcasterJoinedNotification == nil:
		panic("NodeFetcher handler validation failed: no handler set for NodeFetcher.OnSomeoneYouFollowOnFarcasterJoinedNotification")
	case n.OnSomeoneYouFollowPostedTheirFirstPostNotification == nil:
		panic("NodeFetcher handler validation failed: no handler set for NodeFetcher.OnSomeoneYouFollowPostedTheirFirstPostNotification")
	case n.OnToken == nil:
		panic("NodeFetcher handler validation failed: no handler set for NodeFetcher.OnToken")
	case n.OnTokenDefinition == nil:
		panic("NodeFetcher handler validation failed: no handler set for NodeFetcher.OnTokenDefinition")
	case n.OnViewer == nil:
		panic("NodeFetcher handler validation failed: no handler set for NodeFetcher.OnViewer")
	case n.OnWallet == nil:
		panic("NodeFetcher handler validation failed: no handler set for NodeFetcher.OnWallet")
	case n.OnYouReceivedTopActivityBadgeNotification == nil:
		panic("NodeFetcher handler validation failed: no handler set for NodeFetcher.OnYouReceivedTopActivityBadgeNotification")
	}
}
