package handler

import (
	"co-browsing-session-server/internal/middleware"
	"co-browsing-session-server/internal/service"
	"net/http"

	"github.com/gin-gonic/gin"
)

func createSerialNumber(c *gin.Context) {
	const SERIAL_NUMBER_LENGTH = 6

	sessionStore := middleware.GetSessionStore(c)

	serialNumber := service.GenerateRandomSerialNumber(SERIAL_NUMBER_LENGTH)

	_, err := sessionStore.Create(serialNumber)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"serial_number": serialNumber,
	})
}

func RegisterSerialNumberRoutes(router *gin.Engine) {
	router.POST("/serial_number", createSerialNumber)
}
