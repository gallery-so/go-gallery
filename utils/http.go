package utils

import (
	"encoding/json"
	"net/http"
	"time"
)

// CreateHTTPClient creates an HTTP Client with a timeout of 30 seconds
func CreateHTTPClient() http.Client {
	return http.Client{
		Timeout: time.Second * 30,
	}
}

// RespondWithBody sends a standard success response
func RespondWithBody(w http.ResponseWriter, body interface{}) {
	if marshalled := marshallResponseBody(w, body); marshalled != nil {
		w.Header().Set("Content-Type", "application/json")
		w.Write(*marshalled)
	}
}

// marshalls a JSON response to byte array
func marshallResponseBody(w http.ResponseWriter, body interface{}) *[]byte {
	marshalled, err := json.Marshal(body)
	if err != nil {
		http.Error(w, "Error Marshalling Response Body", 400)
		return nil
	}
	return &marshalled
}