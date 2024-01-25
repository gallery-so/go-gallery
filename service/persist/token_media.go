package persist

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"github.com/lib/pq"
)

// Media represents a token's media content with processed images from metadata
type Media struct {
	ThumbnailURL    NullString `json:"thumbnail_url,omitempty"`
	LivePreviewURL  NullString `json:"live_preview_url,omitempty"`
	ProfileImageURL NullString `json:"profile_image_url,omitempty"`
	MediaURL        NullString `json:"media_url,omitempty"`
	MediaType       MediaType  `json:"media_type"`
	Dimensions      Dimensions `json:"dimensions"`
}

// IsServable returns true if the token's Media has enough information to serve it's assets.
func (m Media) IsServable() bool {
	return m.MediaURL != "" && m.MediaType.IsValid()
}

// Value implements the driver.Valuer interface for media
func (m Media) Value() (driver.Value, error) {
	return json.Marshal(m)
}

// Scan implements the sql.Scanner interface for media
func (m *Media) Scan(src interface{}) error {
	if src == nil {
		*m = Media{}
		return nil
	}
	return json.Unmarshal(src.([]byte), &m)
}

// MediaList is a slice of Media, used to implement scanner/valuer interfaces
type MediaList []Media

func (l MediaList) Value() (driver.Value, error) {
	return pq.Array(l).Value()
}

// Scan implements the Scanner interface for the MediaList type
func (l *MediaList) Scan(value interface{}) error {
	return pq.Array(l).Scan(value)
}

var errMediaNotFound ErrMediaNotFound

type ErrMediaNotFound struct{}

func (e ErrMediaNotFound) Unwrap() error { return notFoundError }
func (e ErrMediaNotFound) Error() string { return "media not found" }

type ErrMediaNotFoundByID struct{ ID DBID }

func (e ErrMediaNotFoundByID) Unwrap() error { return errMediaNotFound }
func (e ErrMediaNotFoundByID) Error() string { return fmt.Sprintf("no media found for ID %s", e.ID) }
