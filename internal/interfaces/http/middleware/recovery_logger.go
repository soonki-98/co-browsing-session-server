package middleware

import "github.com/gin-gonic/gin"

// Default는 모든 라우터에 항상 붙는 기술적 미들웨어(logger + recovery)를 반환한다.
func Default() []gin.HandlerFunc {
	return []gin.HandlerFunc{gin.Logger(), gin.Recovery()}
}
