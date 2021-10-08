package mongodb

import "github.com/mikeydub/go-gallery/copy"

// DocumentNotFoundError is a custom error type for if a Mongo document could not be found
type DocumentNotFoundError struct {
}

func (e *DocumentNotFoundError) Error() string {
	return copy.CouldNotFindDocument
}
