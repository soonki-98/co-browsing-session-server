package room

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	rssvc "co-browsing-session-server/internal/services/roomsession"
)

type Handler struct {
	service *rssvc.Service
}

func NewHandler(service *rssvc.Service) *Handler {
	return &Handler{service: service}
}

func (handler *Handler) Register(api huma.API) {
	huma.Register(api, huma.Operation{
		OperationID: "createRoom",
		Method:      http.MethodPost,
		Path:        "/rooms",
		Summary:     "새 방 생성",
		Description: "새 RoomSession과 초대 코드를 발급합니다.",
		Tags:        []string{"rooms"},
	}, handler.Create)
}

func (handler *Handler) Create(ctx context.Context, _ *CreateInput) (*CreateResponse, error) {
	_, createdInvitation, err := handler.service.Create(ctx)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to create room", err)
	}
	response := &CreateResponse{}
	response.Body.Data.SerialNumber = createdInvitation.Serial.String()
	response.Body.Data.ExpiresAt = createdInvitation.ExpiresAt
	return response, nil
}
