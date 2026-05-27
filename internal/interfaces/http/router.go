package http

import (
	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humagin"
	"github.com/gin-gonic/gin"

	"co-browsing-session-server/internal/interfaces/http/middleware"
)

// Handler는 huma API에 자기 엔드포인트를 등록할 수 있는 어댑터다.
type Handler interface {
	Register(api huma.API)
}

// NewRouter는 gin 엔진 위에 humagin 어댑터로 huma API를 마운트하고 핸들러들을 등록한다.
// huma가 /openapi.json, /openapi.yaml, /docs (Stoplight Elements UI) 경로를 자동으로 노출한다.
func NewRouter(handlers ...Handler) *gin.Engine {
	engine := gin.New()
	engine.Use(middleware.Default()...)

	api := humagin.New(engine, huma.DefaultConfig("Co-Browsing Session Server", "1.0.0"))
	for _, handler := range handlers {
		handler.Register(api)
	}
	return engine
}
