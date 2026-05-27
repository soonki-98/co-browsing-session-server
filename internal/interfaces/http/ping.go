package http

import (
	nethttp "net/http"

	"github.com/gin-gonic/gin"
)

type PingHandler struct{}

func NewPingHandler() *PingHandler {
	return &PingHandler{}
}

func (pingHandler *PingHandler) Register(engine *gin.Engine) {
	engine.GET("/ping", pingHandler.ping)
}

func (pingHandler *PingHandler) ping(ginContext *gin.Context) {
	ginContext.JSON(nethttp.StatusOK, gin.H{"message": "pong"})
}
