package persist

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
)

// Media represents a token's media content with processed images from metadata
type Media struct {
	ThumbnailURL   NullString `json:"thumbnail_url,omitempty"`
	LivePreviewURL NullString `json:"live_preview_url,omitempty"`
	MediaURL       NullString `json:"media_url,omitempty"`
	MediaType      MediaType  `json:"media_type"`
	Dimensions     Dimensions `json:"dimensions"`
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

type ErrMediaNotFound struct {
	TokenID DBID
}

func (e ErrMediaNotFound) Error() string {
	return fmt.Sprintf("no media found for tokenID %s", e.TokenID)
}
