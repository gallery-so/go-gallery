package sqlc

// TokenLayout defines the layout of a collection of tokens
type TokenLayout struct {
	Columns    int   `json:"columns"`
	Whitespace []int `json:"whitespace"`
	// Padding         int   `bson:"padding" json:"padding"`
}
