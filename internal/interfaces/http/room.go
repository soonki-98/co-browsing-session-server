package http

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	rssvc "co-browsing-session-server/internal/services/roomsession"
)

type RoomHandler struct {
	service *rssvc.Service
}

func NewRoomHandler(service *rssvc.Service) *RoomHandler {
	return &RoomHandler{service: service}
}

func (roomHandler *RoomHandler) Register(api huma.API) {
	huma.Register(api, huma.Operation{
		OperationID: "createRoom",
		Method:      http.MethodPost,
		Path:        "/rooms",
		Summary:     "새 방 생성",
		Description: "새 RoomSession과 초대 코드를 발급합니다.",
		Tags:        []string{"rooms"},
	}, roomHandler.PostRoom)
}

func (roomHandler *RoomHandler) PostRoom(ctx context.Context, _ *EmptyInput) (*PostRoomOutput, error) {
	_, createdInvitation, err := roomHandler.service.Create(ctx)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to create room", err)
	}
	output := &PostRoomOutput{}
	output.Body.SerialNumber = createdInvitation.Serial.String()
	output.Body.ExpiresAt = createdInvitation.ExpiresAt
	return output, nil
}
