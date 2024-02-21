package util

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"
)

// ErrorResponse represents a json response for an error during endpoint execution
type ErrorResponse struct {
	Error string `json:"error"`
}

// SuccessResponse represents a true or false success response for an endpoint
type SuccessResponse struct {
	Success bool `json:"success"`
}

// ErrInvalidInput is an error response for an invalid input
type ErrInvalidInput struct {
	Reason string `json:"reason"`
}

func (e ErrInvalidInput) Error() string {
	return fmt.Sprintf("invalid input: %s", e.Reason)
}

// ErrHTTP represents an error returned from an HTTP request
type ErrHTTP struct {
	URL    string
	Status int
	Err    error
}

func (h ErrHTTP) Error() string {
	return fmt.Sprintf("HTTP Error Status - %d | URL - %s | Error: %s", h.Status, h.URL, h.Err)
}

func (h ErrHTTP) Unwrap() error {
	return h.Err
}

// ErrResponse sends a json response for an error during endpoint execution
func ErrResponse(c *gin.Context, code int, err error) {
	c.Error(err)
	c.JSON(code, ErrorResponse{Error: err.Error()})
}

func GetErrFromResp(res *http.Response) error {
	errResp := map[string]interface{}{}
	json.NewDecoder(res.Body).Decode(&errResp)
	return ErrHTTP{URL: res.Request.URL.String(), Status: res.StatusCode, Err: fmt.Errorf("%+v", errResp)}
}

type ErrReadBody struct {
	Err error
}

func (e ErrReadBody) Error() string {
	return fmt.Sprintf("error parsing body: %s", e.Err)
}

func (e ErrReadBody) Unwrap() error {
	return e.Err
}

// BodyAsError returns the HTTP body as an error
// Returns ErrReadBody if the body cannot be read
func BodyAsError(res *http.Response) error {
	body, err := io.ReadAll(res.Body)
	if err != nil {
		return ErrReadBody{Err: err}
	}

	// Check if the body is an error response
	var errResp ErrorResponse
	if err := json.Unmarshal(body, &errResp); err == nil {
		return fmt.Errorf(errResp.Error)
	}

	if len(body) == 0 {
		return fmt.Errorf("empty body")
	}

	// Otherwise, return the entire body as an error
	return errors.New(string(body))
}

func HealthCheckHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.JSON(http.StatusOK, SuccessResponse{Success: true})
	}
}
