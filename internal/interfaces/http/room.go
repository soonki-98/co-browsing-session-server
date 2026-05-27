package http

import (
	nethttp "net/http"

	"github.com/gin-gonic/gin"

	rssvc "co-browsing-session-server/internal/services/roomsession"
)

type RoomHandler struct {
	service *rssvc.Service
}

func NewRoomHandler(service *rssvc.Service) *RoomHandler {
	return &RoomHandler{service: service}
}

func (roomHandler *RoomHandler) Register(engine *gin.Engine) {
	engine.POST("/rooms", roomHandler.postRoom)
}

func (roomHandler *RoomHandler) postRoom(ginContext *gin.Context) {
	_, createdInvitation, err := roomHandler.service.Create(ginContext.Request.Context())
	if err != nil {
		ginContext.JSON(nethttp.StatusInternalServerError, ErrorResponse{Error: err.Error()})
		return
	}
	ginContext.JSON(nethttp.StatusOK, PostRoomResponse{
		SerialNumber: createdInvitation.Serial.String(),
		ExpiresAt:    createdInvitation.ExpiresAt,
	})
}
