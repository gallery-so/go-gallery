package persist

import (
	"fmt"
)

var errTokenDefinitionNotFound ErrTokenDefinitionNotFound

type ErrTokenDefinitionNotFound struct{}

func (e ErrTokenDefinitionNotFound) Unwrap() error { return notFoundError }
func (e ErrTokenDefinitionNotFound) Error() string { return "tokenDefinition not found" }

type ErrTokenDefinitionNotFoundByID struct{ ID DBID }

func (e ErrTokenDefinitionNotFoundByID) Unwrap() error { return errTokenDefinitionNotFound }
func (e ErrTokenDefinitionNotFoundByID) Error() string {
	return fmt.Sprintf("tokenDefinition not found by ID=%s", e.ID)
}
