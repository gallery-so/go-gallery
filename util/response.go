package util

// ErrorResponse represents a json response for an error during endpoint execution
type ErrorResponse struct {
	Error string `json:"error"`
}

// SuccessResponse represents a true or false success response for an endpoint
type SuccessResponse struct {
	Success bool `json:"success"`
}
