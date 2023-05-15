package util

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
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

// BodyAsError returns the HTTP body as an error
func BodyAsError(res *http.Response) error {
	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return err
	}
	return ErrHTTP{URL: res.Request.URL.String(), Status: res.StatusCode, Err: fmt.Errorf("%s", body)}
}

func HealthCheckHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.JSON(http.StatusOK, SuccessResponse{Success: true})
	}
}
