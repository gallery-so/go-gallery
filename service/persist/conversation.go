package persist

import (
	"database/sql/driver"
	"encoding/json"

	"github.com/jackc/pgtype"
	"github.com/sashabaranov/go-openai"
)

type ConversationMessages []openai.ChatCompletionMessage

type GivenIDs map[int]DBID

func (c ConversationMessages) Value() (driver.Value, error) {
	asJSON, err := json.Marshal(c)
	if err != nil {
		return nil, err
	}
	return pgtype.JSONB{Bytes: asJSON, Status: pgtype.Present}.Value()
}

// Scan implements the Scanner interface for the DBIDList type
func (c *ConversationMessages) Scan(value interface{}) error {
	it := &pgtype.JSONB{Status: pgtype.Present}
	if err := it.Scan(value); err != nil {
		return err
	}
	return json.Unmarshal(it.Bytes, c)
}

func (g GivenIDs) Value() (driver.Value, error) {
	asJSON, err := json.Marshal(g)
	if err != nil {
		return nil, err
	}
	return pgtype.JSONB{Bytes: asJSON, Status: pgtype.Present}.Value()
}

// Scan implements the Scanner interface for the DBIDList type
func (g *GivenIDs) Scan(value interface{}) error {
	it := &pgtype.JSONB{Status: pgtype.Present}
	if err := it.Scan(value); err != nil {
		return err
	}
	return json.Unmarshal(it.Bytes, g)
}
