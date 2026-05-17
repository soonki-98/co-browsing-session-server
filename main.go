package main

import (
	handler "co-browsing-session-server/internal/handler"

	"github.com/gin-gonic/gin"
)

func main() {
  var router = gin.New()
  router.Use(gin.Logger(), gin.Recovery())

  handler.RegisterSerialNumberRoutes(router)
  handler.RegisterPingRoutes(router)

  router.Run(":8080")
}