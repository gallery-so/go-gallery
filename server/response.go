package server

type ErrorResponse struct {
	Error string `json:"error"`
}

type successOutput struct {
	Success bool `json:"success"`
}
