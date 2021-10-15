package util

import "github.com/gin-gonic/gin"

// ErrorResponse represents a json response for an error during endpoint execution
type ErrorResponse struct {
	Error string `json:"error"`
}

// SuccessResponse represents a true or false success response for an endpoint
type SuccessResponse struct {
	Success bool `json:"success"`
}

// ErrResponse sends a json response for an error during endpoint execution
func ErrResponse(c *gin.Context, code int, err error) {
	c.Error(err)
	c.JSON(code, ErrorResponse{Error: err.Error()})
}
