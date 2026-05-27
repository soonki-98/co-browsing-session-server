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

func (h *RoomHandler) Register(r *gin.Engine) {
	r.POST("/rooms", h.postRoom)
}

func (h *RoomHandler) postRoom(c *gin.Context) {
	_, inv, err := h.service.Create(c.Request.Context())
	if err != nil {
		c.JSON(nethttp.StatusInternalServerError, ErrorResponse{Error: err.Error()})
		return
	}
	c.JSON(nethttp.StatusOK, PostRoomResponse{
		SerialNumber: inv.Serial.String(),
		ExpiresAt:    inv.ExpiresAt,
	})
}
