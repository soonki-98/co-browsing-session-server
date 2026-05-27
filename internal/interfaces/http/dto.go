package http

import "time"

type EmptyInput struct{}

type PingOutput struct {
	Body struct {
		Message string `json:"message" doc:"항상 pong" example:"pong"`
	}
}

type PostRoomOutput struct {
	Body struct {
		SerialNumber string    `json:"serial_number" doc:"방 초대 시리얼 코드"`
		ExpiresAt    time.Time `json:"expires_at" doc:"초대 코드 만료 시각"`
	}
}
