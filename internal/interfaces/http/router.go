package http

import (
	"github.com/gin-gonic/gin"

	"co-browsing-session-server/internal/interfaces/http/middleware"
)

// Handler는 라우터에 자기 엔드포인트를 등록할 수 있는 어댑터다.
type Handler interface {
	Register(r *gin.Engine)
}

// NewRouter는 gin 엔진을 만들고 기본 미들웨어를 단 뒤 전달받은 핸들러들을 등록한다.
func NewRouter(handlers ...Handler) *gin.Engine {
	r := gin.New()
	r.Use(middleware.Default()...)
	for _, h := range handlers {
		h.Register(r)
	}
	return r
}
