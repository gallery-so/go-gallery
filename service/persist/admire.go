package persist

import (
	"errors"
	"fmt"
)

var (
	ErrAdmireAlreadyExists = errors.New("admire already exists")
	errAdmireNotFound      ErrAdmireNotFound
)

type ErrAdmireNotFound struct{}

func (e ErrAdmireNotFound) Unwrap() error { return notFoundError }
func (e ErrAdmireNotFound) Error() string { return "admire not found" }

type ErrAdmireNotFoundByFeedEventID struct {
	ActorID     DBID
	FeedEventID DBID
}

func (e ErrAdmireNotFoundByFeedEventID) Unwrap() error { return errAdmireNotFound }
func (e ErrAdmireNotFoundByFeedEventID) Error() string {
	return fmt.Sprintf("admire not found by actorID=%s; feedEventID=%s", e.ActorID, e.FeedEventID)
}

type ErrAdmireNotFoundByPostID struct {
	ActorID DBID
	PostID  DBID
}

func (e ErrAdmireNotFoundByPostID) Unwrap() error { return errAdmireNotFound }
func (e ErrAdmireNotFoundByPostID) Error() string {
	return fmt.Sprintf("admire not found by actorID=%s; postID=%s", e.ActorID, e.PostID)
}

type ErrAdmireNotFoundByTokenID struct {
	ActorID DBID
	TokenID DBID
}

func (e ErrAdmireNotFoundByTokenID) Unwrap() error { return errAdmireNotFound }
func (e ErrAdmireNotFoundByTokenID) Error() string {
	return fmt.Sprintf("admire not found by actorID=%s; tokenID=%s", e.ActorID, e.TokenID)
}
