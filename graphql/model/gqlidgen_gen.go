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
	//-----------------------------------------------------------------------------------------------
	//-----------------------------------------------------------------------------------------------
	// Some fields specified by @goGqlId require manual binding because one of the following is true:
	// (a) the field does not exist on the Community type, or
	// (b) the field exists but is not a string type
	//-----------------------------------------------------------------------------------------------
	// Please create binding methods on the Community type with the following signatures:
	// func (r *Community) GetGqlIDField_ContractAddress() string
	// func (r *Community) GetGqlIDField_Chain() string
	//-----------------------------------------------------------------------------------------------
	return GqlID(fmt.Sprintf("Community:%s:%s", r.GetGqlIDField_ContractAddress(), r.GetGqlIDField_Chain()))
}

func (r *Contract) ID() GqlID {
	return GqlID(fmt.Sprintf("Contract:%s", r.Dbid))
}

func (r *FeedEvent) ID() GqlID {
	return GqlID(fmt.Sprintf("FeedEvent:%s", r.Dbid))
}

func (r *Gallery) ID() GqlID {
	return GqlID(fmt.Sprintf("Gallery:%s", r.Dbid))
}

func (r *GalleryUser) ID() GqlID {
	return GqlID(fmt.Sprintf("GalleryUser:%s", r.Dbid))
}

func (r *MembershipTier) ID() GqlID {
	return GqlID(fmt.Sprintf("MembershipTier:%s", r.Dbid))
}

func (r *SomeoneAdmiredYourFeedEventNotification) ID() GqlID {
	return GqlID(fmt.Sprintf("SomeoneAdmiredYourFeedEventNotification:%s", r.Dbid))
}

func (r *SomeoneCommentedOnYourFeedEventNotification) ID() GqlID {
	return GqlID(fmt.Sprintf("SomeoneCommentedOnYourFeedEventNotification:%s", r.Dbid))
}

func (r *SomeoneFollowedYouBackNotification) ID() GqlID {
	return GqlID(fmt.Sprintf("SomeoneFollowedYouBackNotification:%s", r.Dbid))
}

func (r *SomeoneFollowedYouNotification) ID() GqlID {
	return GqlID(fmt.Sprintf("SomeoneFollowedYouNotification:%s", r.Dbid))
}

func (r *SomeoneViewedYourGalleryNotification) ID() GqlID {
	return GqlID(fmt.Sprintf("SomeoneViewedYourGalleryNotification:%s", r.Dbid))
}

func (r *Token) ID() GqlID {
	return GqlID(fmt.Sprintf("Token:%s", r.Dbid))
}

func (r *Wallet) ID() GqlID {
	return GqlID(fmt.Sprintf("Wallet:%s", r.Dbid))
}

type NodeFetcher struct {
	OnAdmire                                      func(ctx context.Context, dbid persist.DBID) (*Admire, error)
	OnCollection                                  func(ctx context.Context, dbid persist.DBID) (*Collection, error)
	OnCollectionToken                             func(ctx context.Context, tokenId string, collectionId string) (*CollectionToken, error)
	OnComment                                     func(ctx context.Context, dbid persist.DBID) (*Comment, error)
	OnCommunity                                   func(ctx context.Context, contractAddress string, chain string) (*Community, error)
	OnContract                                    func(ctx context.Context, dbid persist.DBID) (*Contract, error)
	OnFeedEvent                                   func(ctx context.Context, dbid persist.DBID) (*FeedEvent, error)
	OnGallery                                     func(ctx context.Context, dbid persist.DBID) (*Gallery, error)
	OnGalleryUser                                 func(ctx context.Context, dbid persist.DBID) (*GalleryUser, error)
	OnMembershipTier                              func(ctx context.Context, dbid persist.DBID) (*MembershipTier, error)
	OnSomeoneAdmiredYourFeedEventNotification     func(ctx context.Context, dbid persist.DBID) (*SomeoneAdmiredYourFeedEventNotification, error)
	OnSomeoneCommentedOnYourFeedEventNotification func(ctx context.Context, dbid persist.DBID) (*SomeoneCommentedOnYourFeedEventNotification, error)
	OnSomeoneFollowedYouBackNotification          func(ctx context.Context, dbid persist.DBID) (*SomeoneFollowedYouBackNotification, error)
	OnSomeoneFollowedYouNotification              func(ctx context.Context, dbid persist.DBID) (*SomeoneFollowedYouNotification, error)
	OnSomeoneViewedYourGalleryNotification        func(ctx context.Context, dbid persist.DBID) (*SomeoneViewedYourGalleryNotification, error)
	OnToken                                       func(ctx context.Context, dbid persist.DBID) (*Token, error)
	OnWallet                                      func(ctx context.Context, dbid persist.DBID) (*Wallet, error)
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
		if len(ids) != 2 {
			return nil, ErrInvalidIDFormat{message: fmt.Sprintf("'Community' type requires 2 ID component(s) (%d component(s) supplied)", len(ids))}
		}
		return n.OnCommunity(ctx, string(ids[0]), string(ids[1]))
	case "Contract":
		if len(ids) != 1 {
			return nil, ErrInvalidIDFormat{message: fmt.Sprintf("'Contract' type requires 1 ID component(s) (%d component(s) supplied)", len(ids))}
		}
		return n.OnContract(ctx, persist.DBID(ids[0]))
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
	case "SomeoneAdmiredYourFeedEventNotification":
		if len(ids) != 1 {
			return nil, ErrInvalidIDFormat{message: fmt.Sprintf("'SomeoneAdmiredYourFeedEventNotification' type requires 1 ID component(s) (%d component(s) supplied)", len(ids))}
		}
		return n.OnSomeoneAdmiredYourFeedEventNotification(ctx, persist.DBID(ids[0]))
	case "SomeoneCommentedOnYourFeedEventNotification":
		if len(ids) != 1 {
			return nil, ErrInvalidIDFormat{message: fmt.Sprintf("'SomeoneCommentedOnYourFeedEventNotification' type requires 1 ID component(s) (%d component(s) supplied)", len(ids))}
		}
		return n.OnSomeoneCommentedOnYourFeedEventNotification(ctx, persist.DBID(ids[0]))
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
	case "SomeoneViewedYourGalleryNotification":
		if len(ids) != 1 {
			return nil, ErrInvalidIDFormat{message: fmt.Sprintf("'SomeoneViewedYourGalleryNotification' type requires 1 ID component(s) (%d component(s) supplied)", len(ids))}
		}
		return n.OnSomeoneViewedYourGalleryNotification(ctx, persist.DBID(ids[0]))
	case "Token":
		if len(ids) != 1 {
			return nil, ErrInvalidIDFormat{message: fmt.Sprintf("'Token' type requires 1 ID component(s) (%d component(s) supplied)", len(ids))}
		}
		return n.OnToken(ctx, persist.DBID(ids[0]))
	case "Wallet":
		if len(ids) != 1 {
			return nil, ErrInvalidIDFormat{message: fmt.Sprintf("'Wallet' type requires 1 ID component(s) (%d component(s) supplied)", len(ids))}
		}
		return n.OnWallet(ctx, persist.DBID(ids[0]))
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
	case n.OnFeedEvent == nil:
		panic("NodeFetcher handler validation failed: no handler set for NodeFetcher.OnFeedEvent")
	case n.OnGallery == nil:
		panic("NodeFetcher handler validation failed: no handler set for NodeFetcher.OnGallery")
	case n.OnGalleryUser == nil:
		panic("NodeFetcher handler validation failed: no handler set for NodeFetcher.OnGalleryUser")
	case n.OnMembershipTier == nil:
		panic("NodeFetcher handler validation failed: no handler set for NodeFetcher.OnMembershipTier")
	case n.OnSomeoneAdmiredYourFeedEventNotification == nil:
		panic("NodeFetcher handler validation failed: no handler set for NodeFetcher.OnSomeoneAdmiredYourFeedEventNotification")
	case n.OnSomeoneCommentedOnYourFeedEventNotification == nil:
		panic("NodeFetcher handler validation failed: no handler set for NodeFetcher.OnSomeoneCommentedOnYourFeedEventNotification")
	case n.OnSomeoneFollowedYouBackNotification == nil:
		panic("NodeFetcher handler validation failed: no handler set for NodeFetcher.OnSomeoneFollowedYouBackNotification")
	case n.OnSomeoneFollowedYouNotification == nil:
		panic("NodeFetcher handler validation failed: no handler set for NodeFetcher.OnSomeoneFollowedYouNotification")
	case n.OnSomeoneViewedYourGalleryNotification == nil:
		panic("NodeFetcher handler validation failed: no handler set for NodeFetcher.OnSomeoneViewedYourGalleryNotification")
	case n.OnToken == nil:
		panic("NodeFetcher handler validation failed: no handler set for NodeFetcher.OnToken")
	case n.OnWallet == nil:
		panic("NodeFetcher handler validation failed: no handler set for NodeFetcher.OnWallet")
	}
}
