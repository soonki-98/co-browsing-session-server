package middleware

import (
	"net/http"
	"os"
	"strings"

	"github.com/gin-gonic/gin"
)

// defaultAllowedOrigin은 CORS_ALLOWED_ORIGINS 미설정 시 쓰는 로컬 개발용 출처다.
const defaultAllowedOrigin = "http://localhost:3000"

// LoadAllowedOrigins는 환경변수 CORS_ALLOWED_ORIGINS(쉼표 구분)에서 허용 출처를 읽는다.
// 각 출처는 앞뒤 공백을 제거한다. 미설정이거나 빈 문자열이면 로컬 개발용 기본값을 반환한다.
// router.go에서 호출하므로 export한다.
func LoadAllowedOrigins() []string {
	raw := os.Getenv("CORS_ALLOWED_ORIGINS")
	if raw == "" {
		return []string{defaultAllowedOrigin}
	}

	parts := strings.Split(raw, ",")
	origins := make([]string, 0, len(parts))
	for _, part := range parts {
		origins = append(origins, strings.TrimSpace(part))
	}
	return origins
}

// CORSMiddleware는 allowedOrigins를 기준으로 교차 출처 요청을 처리하는 Gin 미들웨어를 반환한다.
//
// 요청 Origin 헤더가 allowedOrigins와 정확히 일치하면 그 출처를 그대로
// Access-Control-Allow-Origin에 반사하고(와일드카드 미사용) 자격 정보 동반 요청을 위한
// 헤더를 함께 내려준다. OPTIONS preflight는 204로 즉시 응답하고 본 핸들러를 진행하지 않는다.
// Origin 헤더가 없거나 허용 목록에 없으면 CORS 헤더 없이 다음 핸들러로 통과시킨다.
func CORSMiddleware(allowedOrigins []string) gin.HandlerFunc {
	allowed := make(map[string]struct{}, len(allowedOrigins))
	for _, origin := range allowedOrigins {
		allowed[origin] = struct{}{}
	}

	return func(c *gin.Context) {
		origin := c.Request.Header.Get("Origin")
		if _, ok := allowed[origin]; origin != "" && ok {
			header := c.Writer.Header()
			header.Set("Access-Control-Allow-Origin", origin)
			header.Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
			header.Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
			header.Set("Access-Control-Allow-Credentials", "true")
		}

		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}

		c.Next()
	}
}
