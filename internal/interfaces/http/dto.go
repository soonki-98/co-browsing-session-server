package http

type ErrorResponse struct {
	Error string `json:"error"`
}

type CreateSessionResponse struct {
	SerialNumber string `json:"serial_number"`
}
