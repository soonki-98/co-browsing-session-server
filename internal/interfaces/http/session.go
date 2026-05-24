package http

import (
	nethttp "net/http"

	"github.com/gin-gonic/gin"

	sessionsvc "co-browsing-session-server/internal/services/session"
)

type SessionHandler struct {
	service *sessionsvc.Service
}

func NewSessionHandler(service *sessionsvc.Service) *SessionHandler {
	return &SessionHandler{service: service}
}

func (h *SessionHandler) Register(r *gin.Engine) {
	r.POST("/serial_number", h.postSerialNumber)
}

func (h *SessionHandler) postSerialNumber(c *gin.Context) {
	s, err := h.service.Create(c.Request.Context())
	if err != nil {
		c.JSON(nethttp.StatusInternalServerError, ErrorResponse{Error: err.Error()})
		return
	}
	c.JSON(nethttp.StatusOK, CreateSessionResponse{SerialNumber: s.Serial.String()})
}
