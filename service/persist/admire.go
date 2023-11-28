package persist

import (
	"fmt"
)

var errAdmireNotFound ErrAdmireNotFound

type ErrAdmireNotFound struct{}

func (e ErrAdmireNotFound) Unwrap() error { return notFoundError }
func (e ErrAdmireNotFound) Error() string { return "admire not found" }

type ErrAdmireNotFoundByID struct{ ID DBID }

func (e ErrAdmireNotFoundByID) Unwrap() error { return errAdmireNotFound }
func (e ErrAdmireNotFoundByID) Error() string { return fmt.Sprintf("admire not found by id=%s", e.ID) }

type ErrAdmireNotFoundByActorIDFeedEventID struct {
	ActorID     DBID
	FeedEventID DBID
}

func (e ErrAdmireNotFoundByActorIDFeedEventID) Unwrap() error { return errAdmireNotFound }
func (e ErrAdmireNotFoundByActorIDFeedEventID) Error() string {
	return fmt.Sprintf("admire not found by actorID=%s; feedEventID=%s", e.ActorID, e.FeedEventID)
}

type ErrAdmireNotFoundByActorIDPostID struct {
	ActorID DBID
	PostID  DBID
}

func (e ErrAdmireNotFoundByActorIDPostID) Unwrap() error { return errAdmireNotFound }
func (e ErrAdmireNotFoundByActorIDPostID) Error() string {
	return fmt.Sprintf("admire not found by actorID=%s; postID=%s", e.ActorID, e.PostID)
}

type ErrAdmireNotFoundByActorIDTokenID struct {
	ActorID DBID
	TokenID DBID
}

func (e ErrAdmireNotFoundByActorIDTokenID) Unwrap() error { return errAdmireNotFound }
func (e ErrAdmireNotFoundByActorIDTokenID) Error() string {
	return fmt.Sprintf("admire not found by actorID=%s; tokenID=%s", e.ActorID, e.TokenID)
}
