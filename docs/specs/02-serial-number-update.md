# 02. Serial Number Handler Update Spec

## Overview

기존 `POST /serial_number` 핸들러를 업데이트하여 시리얼 번호 발급과 동시에 세션 Repository에 세션을 등록한다.  
전체 8단계 구현 중 **2번째** — Session Store(#1) 구현 후 진행.

---

## Implementation Order

```
[1] Session Store ✓
[2] Serial Number Update  ← 지금 여기
[3] WebSocket Hub
[4] WebSocket Handler
[5] Signaling Protocol
[6] Control Event Relay
[7] TURN Credentials
[8] CORS Middleware
```

- **선행 의존성:** `#1 Session Store`
- **후행 의존성:** 없음 (독립 엔드포인트)

---

## Dependencies

```go
// 신규 외부 패키지 없음
// 핸들러는 package http에 속하므로 net/http는 alias로 import한다.
import (
    nethttp "net/http"

    "github.com/gin-gonic/gin"

    sessionsvc "co-browsing-session-server/internal/services/session"
)
```

핸들러는 도메인/저장소를 직접 참조하지 않고 application service(`services/session.Service`)에만 의존한다. 시리얼 생성과 충돌 재시도는 service 내부에서 캡슐화한다.

---

## Data Structures

```go
// POST /serial_number 응답 구조체
// internal/interfaces/http/dto.go
type CreateSessionResponse struct {
    SerialNumber string `json:"serial_number"`
}

type ErrorResponse struct {
    Error string `json:"error"`
}
```

---

## Interfaces / Contracts

### HTTP Endpoint

```
POST /serial_number
Content-Type: application/json (body 없음)

Response 200 OK:
{
  "serial_number": "AB3K7M"
}

Response 500 Internal Server Error:
{
  "error": "failed to create session"
}
```

### 핸들러 함수 시그니처

```go
// internal/interfaces/http/session.go
func NewSessionHandler(service *sessionsvc.Service) *SessionHandler

// internal/interfaces/http/router.go에 정의된 Handler 인터페이스를 구현
// type Handler interface { Register(r *gin.Engine) }
func (h *SessionHandler) Register(r *gin.Engine) {
    r.POST("/serial_number", h.postSerialNumber)
}

// internal/interfaces/http/router.go
// 라우터는 variadic Handler를 받아 자동으로 Register를 호출한다
func NewRouter(handlers ...Handler) *gin.Engine
```

---

## Behavior

### 처리 흐름

```
핸들러 (internal/interfaces/http/session.go):
  type SessionHandler struct { service *sessionsvc.Service }

  func (h *SessionHandler) postSerialNumber(c *gin.Context):
    1. s, err := h.service.Create(c.Request.Context())
    2. err != nil → c.JSON(nethttp.StatusInternalServerError, ErrorResponse{Error: err.Error()})
    3. else      → c.JSON(nethttp.StatusOK, CreateSessionResponse{SerialNumber: s.Serial.String()})

Service (internal/services/session/service.go):
  for range createMaxRetries (5):
    serial     := gen.Generate(serialLength)       // serialLength = 6
    newSession := session.New(serial)
    if _, err := repo.Create(newSession); err == nil           → return newSession
    else if !errors.Is(err, session.ErrAlreadyExists)          → 즉시 에러 반환
  // 5회 모두 충돌 → 에러
```

### 충돌 처리

시리얼 번호 공간은 34^6 ≈ 1.5억개로 충분히 크지만, 만약 동일 시리얼이 이미 활성 세션에 존재하면 재생성한다.  
재시도 로직은 **service 레이어**에 위치한다 (handler는 retry를 모름).

### Composition (internal/app/app.go)

```go
// internal/app/app.go — 의존성 조립 (main.go 대신 composition root)
import (
    "co-browsing-session-server/internal/domain/serialnumber"
    "co-browsing-session-server/internal/infrastructure/memory"
    httpiface "co-browsing-session-server/internal/interfaces/http"  // package http → alias
    sessionsvc "co-browsing-session-server/internal/services/session" // domain/session과 충돌 방지 alias
)

func New() *App {
    serialGen      := serialnumber.NewRandomGenerator()
    sessionRepo    := memory.NewSessionRepository()
    sessionService := sessionsvc.NewService(sessionRepo, serialGen)

    router := httpiface.NewRouter(
        httpiface.NewSessionHandler(sessionService),
        httpiface.NewPingHandler(),
    )
    return &App{router: router}
}
```

---

## File Locations

| 작업 | 파일 |
|------|------|
| 수정/신규 | `internal/interfaces/http/session.go` (핸들러) |
| 수정/신규 | `internal/services/session/service.go` (재시도 포함 use case) |
| 수정 | `internal/app/app.go` (composition root에서 주입) |
| 의존 | `internal/domain/session/*` (spec #1) |
| 의존 | `internal/infrastructure/memory/session_repository.go` (spec #1) |

`internal/domain/serialnumber/`(Generator port + 랜덤 구현)는 별도 도메인으로 이미 존재하며 본 spec에서 변경 없음.

---

## Acceptance Criteria

- [ ] `POST /serial_number` 호출 → 6자리 시리얼 번호 반환
- [ ] 동일 요청 두 번 호출 시 서로 다른 시리얼 번호 반환 (매우 높은 확률로)
- [ ] `repo.Get(serialnumber.SerialNumber(반환된_시리얼))` 호출 시 `Status == session.StatusWaiting`인 세션 존재 확인
- [ ] 서버 재시작 시 세션 초기화 (인메모리 저장이므로 정상 동작)
- [ ] 핸들러가 repository나 도메인을 직접 import하지 않고 service만 주입받는 구조
