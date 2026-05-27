# 02. Room Creation Handler Spec (POST /rooms)

## Overview

`POST /rooms` 엔드포인트를 구현한다. 한 번의 호출로 `RoomSession`(영속 상태)과 `Invitation`(시리얼 → RoomID 매핑)을 atomically 생성하고, 클라이언트에 시리얼 번호와 만료 시각을 돌려준다.

전체 8단계 구현 중 **2번째** — Domain Stores(#1) 완료 후 진행.

---

## Implementation Order

```
[1] Domain Stores ✓
[2] Room Handler (POST /rooms)  ← 지금 여기
[3] WebSocket Hub
[4] WebSocket Handler
[5] Signaling Protocol
[6] Control Event Relay
[7] TURN Credentials
[8] CORS Middleware
```

- **선행 의존성:** `#1 Domain Stores`
- **후행 의존성:** 없음 (독립 엔드포인트)

---

## Dependencies

```go
// 핸들러는 package http에 속하므로 net/http는 alias로 import
import (
    nethttp "net/http"

    "github.com/gin-gonic/gin"

    rssvc "co-browsing-session-server/internal/services/roomsession"
)
```

핸들러는 도메인/저장소를 직접 참조하지 않고 application service(`services/roomsession.Service`)에만 의존한다. 시리얼 생성 / 충돌 재시도 / RoomSession+Invitation atomic 생성 / 보상 트랜잭션은 모두 service 레이어에서 캡슐화한다.

신규 외부 패키지 없음 (uuid는 #1에서 추가됨).

---

## Data Structures

### 응답 DTO

```go
// internal/interfaces/http/dto.go
type PostRoomResponse struct {
    SerialNumber string    `json:"serial_number"`  // UX 용어 유지
    ExpiresAt    time.Time `json:"expires_at"`     // ISO8601, UI 카운트다운용
}

type ErrorResponse struct {
    Error string `json:"error"`
}
```

`serial_number`는 응답 페이로드에서만 살아남는 외부 용어 — 내부 모델은 `Invitation.Serial`로 표기.

---

## Interfaces / Contracts

### HTTP Endpoint

```
POST /rooms
Content-Type: application/json (body 없음)

Response 200 OK:
{
  "serial_number": "AB3K7M",
  "expires_at":    "2026-05-27T14:35:00Z"
}

Response 500 Internal Server Error:
{
  "error": "failed to create room"
}
```

`POST /serial_number`는 **폐기** (이전 spec에서 변경).

### 핸들러 시그니처

```go
// internal/interfaces/http/room.go
type RoomHandler struct {
    service *rssvc.Service
}

func NewRoomHandler(service *rssvc.Service) *RoomHandler

func (h *RoomHandler) Register(r *gin.Engine) {
    r.POST("/rooms", h.postRoom)
}
```

`Handler` 인터페이스(`router.go`에 이미 정의)를 구현 — 라우터 코드 변경 없음.

---

## Behavior

### 처리 흐름 (handler)

```
RoomHandler.postRoom(c):
  1. rs, inv, err := h.service.Create(c.Request.Context())
  2. err != nil → c.JSON(500, ErrorResponse{Error: "failed to create room"})
  3. else      → c.JSON(200, PostRoomResponse{
                    SerialNumber: string(inv.Serial),
                    ExpiresAt:    inv.ExpiresAt,
                  })
```

### Service: atomic 생성 + 충돌 재시도

```go
// internal/services/roomsession/service.go
const (
    createMaxRetries = 5
    serialLength     = 6
)

type Service struct {
    rsRepo  roomsession.Repository
    invRepo invitation.Repository
    gen     serialnumber.Generator
}

func NewService(
    rsRepo roomsession.Repository,
    invRepo invitation.Repository,
    gen serialnumber.Generator,
) *Service

func (s *Service) Create(ctx context.Context) (*roomsession.RoomSession, *invitation.Invitation, error) {
    // 1. RoomSession 먼저 생성 (UUID는 충돌 거의 없음)
    roomID := roomsession.NewID()
    rs     := roomsession.New(roomID)
    if _, err := s.rsRepo.Create(rs); err != nil {
        return nil, nil, fmt.Errorf("create room session: %w", err)
    }

    // 2. Invitation 생성 — 시리얼 충돌 시 재시도 (RoomSession은 그대로 두고)
    for range createMaxRetries {
        serial := s.gen.Generate(serialLength)
        inv    := invitation.New(serial, roomID)

        if _, err := s.invRepo.Create(inv); err == nil {
            return rs, inv, nil
        } else if !errors.Is(err, invitation.ErrAlreadyExists) {
            // 비-충돌 에러 → RoomSession 롤백 후 에러 반환
            s.rsRepo.Delete(roomID)
            return nil, nil, fmt.Errorf("create invitation: %w", err)
        }
        // 시리얼 충돌 → 재시도
    }

    // 재시도 모두 실패 → RoomSession 롤백
    s.rsRepo.Delete(roomID)
    return nil, nil, fmt.Errorf("create invitation: exhausted %d retries due to serial collisions", createMaxRetries)
}
```

### 보상 트랜잭션 (rollback) 근거

두 repository에 걸친 atomic 동작이 필요한데 in-memory에는 트랜잭션 개념이 없으므로:
- RoomSession Create는 먼저 수행 (UUID는 충돌 확률 무시 가능)
- Invitation Create 실패 시 RoomSession을 `Delete`로 명시적 롤백
- 미래에 영속 저장소로 옮길 때 이 자리에 실제 DB 트랜잭션이 들어옴 — 인터페이스/흐름은 그대로 유지

### 충돌 처리

시리얼 번호 공간은 34^6 ≈ 1.5억으로 크지만, 만약 동일 시리얼이 활성 Invitation에 존재하면 재생성한다. 재시도 로직은 **service 레이어**에 위치 (handler는 retry를 모름).

### Composition (internal/app/app.go)

```go
// internal/app/app.go — 의존성 조립
import (
    "co-browsing-session-server/internal/domain/serialnumber"
    "co-browsing-session-server/internal/infrastructure/memory"
    httpiface "co-browsing-session-server/internal/interfaces/http"
    rssvc     "co-browsing-session-server/internal/services/roomsession"
)

func New() *App {
    serialGen   := serialnumber.NewRandomGenerator()
    rsRepo      := memory.NewRoomSessionRepository()
    invRepo     := memory.NewInvitationRepository()
    rsService   := rssvc.NewService(rsRepo, invRepo, serialGen)

    router := httpiface.NewRouter(
        httpiface.NewRoomHandler(rsService),
        httpiface.NewPingHandler(),
        // (#4 추가 예정: WebSocketHandler — rsService + invRepo + hub)
    )
    return &App{router: router}
}
```

---

## File Locations

| 작업 | 파일 |
|------|------|
| 신규 생성 | `internal/interfaces/http/room.go` (RoomHandler) |
| 신규 생성 | `internal/services/roomsession/service.go` (atomic 생성 + 재시도 + 롤백) |
| 수정 | `internal/interfaces/http/dto.go` (PostRoomResponse 추가, CreateSessionResponse 삭제) |
| 수정 | `internal/app/app.go` (composition 갱신) |
| 의존 | `internal/domain/roomsession/*`, `internal/domain/invitation/*` (spec #1) |
| 의존 | `internal/infrastructure/memory/*_repository.go` (spec #1) |
| 삭제 | `internal/interfaces/http/session.go` (대체됨) |
| 삭제 | `internal/services/session/*` (roomsession으로 이전) |

`internal/domain/serialnumber/`는 변경 없음.

---

## Acceptance Criteria

- [ ] `POST /rooms` 호출 → `{"serial_number": "AB3K7M", "expires_at": "..."}` 응답, HTTP 200
- [ ] 동일 요청 두 번 호출 시 서로 다른 시리얼 번호 반환 (매우 높은 확률로)
- [ ] 응답의 `expires_at`이 ISO8601 + 호출 시각으로부터 10분 후
- [ ] 호출 후 `rsRepo.Get(roomID)`로 조회 → `Status == StatusWaiting`인 RoomSession 존재 (단, roomID는 외부에 노출 안 됨)
- [ ] 호출 후 `invRepo.ResolveBySerial(반환된_시리얼)` → 매핑된 Invitation 반환, `RoomID`가 위 RoomSession의 ID와 일치
- [ ] 시리얼 충돌 시뮬레이션 (Generator를 mock으로 같은 값 반복) → service가 5회 재시도 후 에러, RoomSession도 cleanup 됨
- [ ] 핸들러가 repository나 도메인을 직접 import하지 않고 service만 주입받는 구조
- [ ] 서버 재시작 시 상태 초기화 (인메모리 저장이므로 정상 동작)
