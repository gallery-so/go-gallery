package server

type errorResponse struct {
	Error string `json:"error"`
}

type successOutput struct {
	Success bool `json:"success"`
}
