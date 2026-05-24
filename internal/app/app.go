package app

import (
	"co-browsing-session-server/internal/handler"
	"co-browsing-session-server/internal/middleware"
	"co-browsing-session-server/internal/repository/session"

	"github.com/gin-gonic/gin"
)

type App struct {
	router       *gin.Engine
	sessionStore *session.SessionStore
}

func NewApp() *App {
	app := &App{
		router:       gin.New(),
		sessionStore: session.NewSessionStore(),
	}

	app.router.Use(gin.Logger(), gin.Recovery())
	app.router.Use(middleware.InjectSessionStore(app.sessionStore))

	app.registerRoutes()

	return app
}

func (app *App) registerRoutes() {
	handler.RegisterSerialNumberRoutes(app.router)
	handler.RegisterPingRoutes(app.router)
}

func (app *App) Run(addr string) {
	app.router.Run(addr)
}
