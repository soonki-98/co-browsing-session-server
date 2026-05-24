# 08. CORS Middleware Spec

## Overview

웹 콘솔(상담사 프론트엔드)의 도메인에서 발생하는 Cross-Origin 요청을 허용하는 Gin 미들웨어.  
전체 8단계 구현 중 **8번째** — 다른 컴포넌트와 독립적이며, `internal/interfaces/http/router.go`의 라우터 구성에 미들웨어를 추가하는 것으로 완성된다.

---

## Implementation Order

```
[1] Session Store ✓
[2] Serial Number Update ✓
[3] WebSocket Hub ✓
[4] WebSocket Handler ✓
[5] Signaling Protocol ✓
[6] Control Event Relay ✓
[7] TURN Credentials ✓
[8] CORS Middleware  ← 지금 여기
```

- **선행 의존성:** 없음 (독립 구현 가능)
- **후행 의존성:** 없음

---

## Dependencies

```go
// 직접 구현 (외부 패키지 불필요)
import (
    "net/http"
    "os"
    "strings"
    "github.com/gin-gonic/gin"
)
```

외부 CORS 패키지(`github.com/gin-contrib/cors` 등) 사용하지 않음 — 설정이 단순하여 직접 구현이 더 명확하다.

---

## Data Structures

추가 데이터 구조 없음.

---

## Interfaces / Contracts

```go
// internal/interfaces/http/middleware/cors.go

package middleware

// CORSMiddleware: Gin 미들웨어 반환
// allowedOrigins: 허용할 오리진 목록 (환경변수에서 로드)
func CORSMiddleware(allowedOrigins []string) gin.HandlerFunc
```

---

## Behavior

### 허용 오리진 로드

```go
// 환경변수 CORS_ALLOWED_ORIGINS (쉼표 구분)
// 예: "https://console.example.com,http://localhost:3000"
// 기본값: "http://localhost:3000" (로컬 개발용)
// router.go에서 호출하므로 export(대문자 시작)한다.
func LoadAllowedOrigins() []string {
    raw := os.Getenv("CORS_ALLOWED_ORIGINS")
    if raw == "" {
        return []string{"http://localhost:3000"}
    }
    return strings.Split(raw, ",")
}
```

### 미들웨어 동작

```
모든 요청에 대해:

1. 요청 Origin 헤더 추출
2. allowedOrigins 목록에 포함되는지 확인
   - 포함: Access-Control-Allow-Origin = 요청 Origin 값 (와일드카드 미사용)
   - 미포함: Origin 헤더 없이 통과 (브라우저가 차단)

3. 항상 추가하는 헤더:
   Access-Control-Allow-Methods: GET, POST, OPTIONS
   Access-Control-Allow-Headers: Content-Type, Authorization
   Access-Control-Allow-Credentials: true

4. Preflight (OPTIONS) 요청:
   → 204 No Content 반환, 이후 핸들러 실행 중단
```

### WebSocket Upgrade와 CORS

`GET /ws`는 WebSocket 업그레이드 요청이므로 브라우저가 `Origin` 헤더를 전송한다.  
gorilla/websocket Upgrader의 `CheckOrigin`은 항상 `true`를 반환하되, 실제 CORS 검증은 이 미들웨어에서 처리된다.

단, WebSocket은 브라우저의 표준 CORS 차단 대상이 아니므로 미들웨어가 응답 헤더를 설정해도 업그레이드 자체에는 영향 없음. 미들웨어는 주로 `/serial_number`, `/turn-credentials` 같은 HTTP 엔드포인트를 위한 것이다.

### Composition (internal/interfaces/http/router.go)

```go
// internal/interfaces/http/router.go
func NewRouter(handlers ...Handler) *gin.Engine {
    r := gin.New()
    r.Use(middleware.Default()...)                                // 기존 logger/recovery
    r.Use(middleware.CORSMiddleware(middleware.LoadAllowedOrigins()))  // 신규 CORS
    for _, h := range handlers {
        h.Register(r)
    }
    return r
}
```

미들웨어는 모든 라우트 등록보다 먼저 `Use`에 등록해야 한다. 기존 `internal/interfaces/http/middleware/recovery_logger.go`와 동일한 위치/패키지(`middleware`)를 사용한다.

---

## File Locations

| 작업 | 파일 |
|------|------|
| 신규 생성 | `internal/interfaces/http/middleware/cors.go` |
| 수정 | `internal/interfaces/http/router.go` (미들웨어 `Use` 추가) |

---

## Acceptance Criteria

- [ ] 허용된 오리진(`http://localhost:3000`)에서 요청 시 `Access-Control-Allow-Origin: http://localhost:3000` 응답 헤더 포함
- [ ] 허용되지 않은 오리진에서 요청 시 `Access-Control-Allow-Origin` 헤더 없음
- [ ] `OPTIONS /serial_number` preflight 요청 → 204 응답
- [ ] `CORS_ALLOWED_ORIGINS=https://a.com,https://b.com` 설정 시 두 오리진 모두 허용
- [ ] 와일드카드(`*`) 미사용 (`Access-Control-Allow-Credentials: true`와 동시 사용 불가)
