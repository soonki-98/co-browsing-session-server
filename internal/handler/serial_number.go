package handler

import (
	"co-browsing-session-server/internal/service"
	"net/http"

	"github.com/gin-gonic/gin"
)

 
func createSerialNumberHandler(c *gin.Context) {
	const SERIAL_NUMBER_LENGTH = 6

	serialNumber := service.GenerateRandomSerialNumber(SERIAL_NUMBER_LENGTH)
	
	c.JSON(http.StatusOK, gin.H{
		"serial_number": serialNumber,
	})
}

func RegisterSerialNumberRoutes(router *gin.Engine) {
	router.POST("/serial_number", createSerialNumberHandler)
}