package handler

import (
	"co-browsing-session-server/internal/middleware"
	"co-browsing-session-server/internal/service"
	"net/http"

	"github.com/gin-gonic/gin"
)

func postSerialNumber(c *gin.Context) {
	sessionStore := middleware.GetSessionStore(c)

	session, err := service.CreateSession(sessionStore)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"serial_number": session.Serial})
}

func RegisterSerialNumberRoutes(router *gin.Engine) {
	router.POST("/serial_number", postSerialNumber)
}
