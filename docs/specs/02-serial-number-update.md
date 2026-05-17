# 02. Serial Number Handler Update Spec

## Overview

기존 `POST /serial_number` 핸들러를 업데이트하여 시리얼 번호 발급과 동시에 SessionStore에 세션을 등록한다.  
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
import (
    "co-browsing-session-server/internal/store"
    "co-browsing-session-server/internal/service"
    "net/http"
    "github.com/gin-gonic/gin"
)
```

---

## Data Structures

```go
// POST /serial_number 응답 구조체
type CreateSerialNumberResponse struct {
    SerialNumber string `json:"serial_number"`
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

### 핸들러 함수 시그니처 변경

```go
// 변경 전 (현재): 핸들러가 store를 모름
func createSerialNumberHandler(c *gin.Context)

// 변경 후: store를 클로저로 주입
func NewSerialNumberHandler(store *store.SessionStore) gin.HandlerFunc

// RegisterSerialNumberRoutes도 store를 받도록 변경
func RegisterSerialNumberRoutes(router *gin.Engine, store *store.SessionStore)
```

---

## Behavior

### 처리 흐름

```
1. service.GenerateRandomSerialNumber(6) 호출 → serial 생성
2. store.Create(serial) 호출
   - 충돌(ErrSessionExists) 발생 시: 재시도 최대 5회
   - 5회 모두 실패 시: 500 에러 반환
3. 성공 시: { "serial_number": serial } 반환
```

### 충돌 처리

시리얼 번호 공간은 34^6 ≈ 1.5억개로 충분히 크지만, 만약 동일 시리얼이 이미 활성 세션에 존재하면 재생성한다.  
반복 재시도 로직은 핸들러 내부에서 처리 (서비스 레이어 변경 불필요).

### main.go 변경

```go
// main.go: SessionStore를 생성하고 핸들러에 주입
sessionStore := store.NewSessionStore()
handler.RegisterSerialNumberRoutes(router, sessionStore)
handler.RegisterPingRoutes(router)
```

---

## File Locations

| 작업 | 파일 |
|------|------|
| 수정 | `internal/handler/serial_number.go` |
| 수정 | `main.go` (store 초기화 및 주입) |
| 신규 생성됨 (spec #1) | `internal/store/session.go` |

`internal/service/serial_number.go`는 변경 없음.

---

## Acceptance Criteria

- [ ] `POST /serial_number` 호출 → 6자리 시리얼 번호 반환
- [ ] 동일 요청 두 번 호출 시 서로 다른 시리얼 번호 반환 (매우 높은 확률로)
- [ ] `store.Get(반환된_시리얼)` 호출 시 status `waiting` 세션 존재 확인
- [ ] 서버 재시작 시 세션 초기화 (인메모리 저장이므로 정상 동작)
- [ ] 핸들러가 store를 직접 import하지 않고 주입받는 구조
