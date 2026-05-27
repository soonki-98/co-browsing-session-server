package ping

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"
)

type Handler struct{}

func NewHandler() *Handler {
	return &Handler{}
}

func (handler *Handler) Register(api huma.API) {
	huma.Register(api, huma.Operation{
		OperationID: "ping",
		Method:      http.MethodGet,
		Path:        "/ping",
		Summary:     "헬스 체크",
		Description: "서버가 살아있는지 확인합니다.",
		Tags:        []string{"system"},
	}, handler.Ping)
}

func (handler *Handler) Ping(ctx context.Context, _ *Input) (*Response, error) {
	response := &Response{}
	response.Body.Data.Message = "pong"
	return response, nil
}
