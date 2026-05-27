package ping

import httpbase "co-browsing-session-server/internal/interfaces/http"

type Input struct{}

type PingData struct {
	Message string `json:"message" doc:"항상 pong" example:"pong"`
}

type Response = httpbase.SuccessResponse[PingData]
