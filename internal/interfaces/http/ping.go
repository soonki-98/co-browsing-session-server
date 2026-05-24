package http

import (
	nethttp "net/http"

	"github.com/gin-gonic/gin"
)

type PingHandler struct{}

func NewPingHandler() *PingHandler {
	return &PingHandler{}
}

func (h *PingHandler) Register(r *gin.Engine) {
	r.GET("/ping", h.ping)
}

func (h *PingHandler) ping(c *gin.Context) {
	c.JSON(nethttp.StatusOK, gin.H{"message": "pong"})
}
