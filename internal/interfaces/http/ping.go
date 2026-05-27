package http

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"
)

type PingHandler struct{}

func NewPingHandler() *PingHandler {
	return &PingHandler{}
}

func (pingHandler *PingHandler) Register(api huma.API) {
	huma.Register(api, huma.Operation{
		OperationID: "ping",
		Method:      http.MethodGet,
		Path:        "/ping",
		Summary:     "헬스 체크",
		Description: "서버가 살아있는지 확인합니다.",
		Tags:        []string{"system"},
	}, pingHandler.Ping)
}

func (pingHandler *PingHandler) Ping(ctx context.Context, _ *EmptyInput) (*PingOutput, error) {
	output := &PingOutput{}
	output.Body.Message = "pong"
	return output, nil
}
