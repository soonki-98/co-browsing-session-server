package room

import (
	"time"

	httpbase "co-browsing-session-server/internal/interfaces/http"
)

type CreateInput struct{}

type CreateData struct {
	SerialNumber string    `json:"serial_number" doc:"방 초대 시리얼 코드"`
	ExpiresAt    time.Time `json:"expires_at" doc:"초대 코드 만료 시각"`
}

type CreateResponse = httpbase.SuccessResponse[CreateData]
