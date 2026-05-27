package http

import "time"

type ErrorResponse struct {
	Error string `json:"error"`
}

type PostRoomResponse struct {
	SerialNumber string    `json:"serial_number"`
	ExpiresAt    time.Time `json:"expires_at"`
}
