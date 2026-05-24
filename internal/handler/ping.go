package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

func pingHandler(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"message": "pong",
	})
}

func RegisterPingRoutes(router *gin.Engine) {
	router.GET("/ping", pingHandler)
}
