package app

import (
	"github.com/gin-gonic/gin"

	"co-browsing-session-server/internal/domain/serialnumber"
	"co-browsing-session-server/internal/infrastructure/memory"
	httpiface "co-browsing-session-server/internal/interfaces/http"
	rssvc "co-browsing-session-server/internal/services/roomsession"
)

type App struct {
	router *gin.Engine
}

// New는 composition root다. 구체 타입을 만들어 생성자에 주입하고 라우터를 구성한다.
// 의존성 변경(예: in-memory → Redis)은 이 함수만 수정하면 된다.
func New() *App {
	// domain
	serialNumberGenerator := serialnumber.NewRandomGenerator()

	// infrastructure
	roomSessionRepository := memory.NewRoomSessionRepository()
	invitationRepository := memory.NewInvitationRepository()

	// application (use cases)
	roomSessionService := rssvc.NewService(roomSessionRepository, invitationRepository, serialNumberGenerator)

	// interfaces (HTTP)
	router := httpiface.NewRouter(
		httpiface.NewRoomHandler(roomSessionService),
		httpiface.NewPingHandler(),
	)

	return &App{router: router}
}

func (app *App) Run(addr string) error {
	return app.router.Run(addr)
}
