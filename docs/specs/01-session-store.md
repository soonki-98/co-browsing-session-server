# 01. Domain Stores Spec (RoomSession + Invitation)

## Overview

도메인 entity와 그 영속 계약. 두 도메인이 한 spec에 묶이는 이유는 구현 순서상 함께 가장 먼저 만들어야 다른 컴포넌트가 이들을 주입받을 수 있기 때문 — `RoomSession`(co-browsing 세션의 영속 상태 머신)과 `Invitation`(시리얼 → RoomID 매핑 entity)을 정의하고 각자의 in-memory repository를 둔다.

전체 8단계 구현 중 **1번째**.

---

## Implementation Order

```
[1] Domain Stores (RoomSession + Invitation)  ← 지금 여기
 └─> [2] Room Handler (POST /rooms)
 └─> [3] WebSocket Hub
       └─> [4] WebSocket Handler
             └─> [5] Signaling Protocol
             └─> [6] Control Event Relay
[7] TURN Credentials  (독립)
[8] CORS Middleware   (독립)
```

- **선행 의존성:** 없음 (`domain/serialnumber`는 이미 존재하는 값 객체 패키지)
- **후행 의존성:** #2(service)와 #4(WS handler)가 두 Repository port를 주입받음. #3 Hub는 self-contained이며 RoomID(UUID 문자열)를 opaque key로만 다룸.

---

## Dependencies

```go
// internal/domain/roomsession/roomsession.go
import (
    "errors"
    "time"

    "github.com/google/uuid"
)

// internal/domain/invitation/invitation.go
import (
    "errors"
    "time"

    "co-browsing-session-server/internal/domain/roomsession"
    "co-browsing-session-server/internal/domain/serialnumber"
)

// internal/infrastructure/memory/room_session_repository.go
// internal/infrastructure/memory/invitation_repository.go
import (
    "sync"
    "time"

    "co-browsing-session-server/internal/domain/invitation"
    "co-browsing-session-server/internal/domain/roomsession"
    "co-browsing-session-server/internal/domain/serialnumber"
)
```

신규 외부 패키지: `github.com/google/uuid v1.x` (`go get github.com/google/uuid`).

`domain/serialnumber`는 본 spec에서 **변경 없음** — `SerialNumber` 값 타입과 `Generator` 인터페이스만 그대로 사용.

---

## Data Structures

### RoomSession (영속 상태 머신)

```go
// internal/domain/roomsession/roomsession.go
package roomsession

type RoomID string

func NewID() RoomID {
    return RoomID(uuid.NewString())
}

type RoomSession struct {
    ID         RoomID
    Status     Status
    StartedAt  time.Time
    ExpiresAt  time.Time   // active 진입 시 zero (무기한)
}

func New(id RoomID) *RoomSession {
    now := time.Now()
    return &RoomSession{
        ID:        id,
        Status:    StatusWaiting,
        StartedAt: now,
        ExpiresAt: now.Add(SessionTTL),
    }
}
```

```go
// internal/domain/roomsession/status.go (기존 status.go 그대로)
package roomsession

type Status string

const (
    StatusWaiting Status = "waiting"  // 발급됨, 양쪽 미접속
    StatusActive  Status = "active"   // 양쪽 접속, 진행 중
    StatusEnded   Status = "ended"
)

func (s Status) IsValid() bool { ... }
func (s Status) CanTransitionTo(to Status) bool { ... }
```

### Invitation (시리얼 → RoomID 브릿지)

```go
// internal/domain/invitation/invitation.go
package invitation

type Invitation struct {
    Serial    serialnumber.SerialNumber
    RoomID    roomsession.RoomID
    IssuedAt  time.Time
    ExpiresAt time.Time
}

func New(serial serialnumber.SerialNumber, roomID roomsession.RoomID) *Invitation {
    now := time.Now()
    return &Invitation{
        Serial:    serial,
        RoomID:    roomID,
        IssuedAt:  now,
        ExpiresAt: now.Add(InvitationTTL),
    }
}

func (i *Invitation) IsExpired(now time.Time) bool {
    return now.After(i.ExpiresAt)
}
```

### In-memory repositories

```go
// internal/infrastructure/memory/room_session_repository.go
type RoomSessionRepository struct {
    mu       sync.Mutex   // read-on-check 시 삭제하므로 Lock
    sessions map[roomsession.RoomID]*roomsession.RoomSession
}

// internal/infrastructure/memory/invitation_repository.go
type InvitationRepository struct {
    mu          sync.Mutex
    invitations map[serialnumber.SerialNumber]*invitation.Invitation
}
```

---

## Interfaces / Contracts

### RoomSession Repository (도메인 계약)

```go
// internal/domain/roomsession/repository.go
package roomsession

type Repository interface {
    Create(s *RoomSession) (*RoomSession, error)
    Get(id RoomID) (*RoomSession, error)
    Update(s *RoomSession) (*RoomSession, error)
    Delete(id RoomID) error
}
```

### Invitation Repository (도메인 계약)

```go
// internal/domain/invitation/repository.go
package invitation

type Repository interface {
    Create(i *Invitation) (*Invitation, error)
    ResolveBySerial(serialnumber.SerialNumber) (*Invitation, error)
    Delete(serialnumber.SerialNumber) error
}
```

### 상태 전이 (RoomSession 도메인 행동)

```go
// internal/domain/roomsession/roomsession.go
func (s *RoomSession) Transition(to Status) error
func (s *RoomSession) IsExpired(now time.Time) bool
```

`Transition` 내부 규칙:
- `to == StatusActive`이면 `ExpiresAt = time.Time{}` (무기한)
- 전이 규칙 위반 시 `ErrInvalidTransition`

### 상수

```go
// internal/domain/roomsession/roomsession.go
const SessionTTL = 10 * time.Minute

// internal/domain/invitation/invitation.go
const InvitationTTL = 10 * time.Minute
```

`serialLength = 6`은 application service(`internal/services/roomsession/service.go`)의 상수로 관리.

---

## Behavior

### RoomSession 상태 머신

```
[POST /rooms 시 생성]  waiting
    │
    │ (양쪽 WS 모두 접속 → Activate 호출 → Transition(active))
    ▼
  active
    │
    │ (어느 한쪽 disconnect → End 호출 → Transition(ended))
    ▼
  ended
```

유효한 전이:
- `waiting` → `active` (허용)
- `active`  → `ended`  (허용)
- `waiting` → `ended`  (허용, TTL 직전 종료 또는 비정상 케이스)
- 그 외 → `ErrInvalidTransition`

### TTL 동작 — RoomSession

- `roomsession.New(id)` 호출 시 `ExpiresAt = time.Now().Add(SessionTTL)`
- `Transition(StatusActive)` 내부에서 `ExpiresAt = time.Time{}`로 무기한 처리
- `Repository.Get(id)`은 **read-on-check**:
  - 만료된 세션 발견 시 삭제 + `roomsession.ErrExpired` 반환
  - status가 active(ExpiresAt zero)이면 만료 검사 스킵
- 별도 백그라운드 goroutine 없음 (단순화)

### TTL 동작 — Invitation

- `invitation.New(...)` 호출 시 `ExpiresAt = time.Now().Add(InvitationTTL)`
- `Repository.ResolveBySerial(s)`은 **read-on-check**:
  - 만료된 invitation 발견 시 삭제 + `invitation.ErrExpired` 반환
- Invitation은 status 개념 없음 — 존재하면 valid, 만료/삭제되면 invalid.
- 명시적 삭제는 `RoomSessionService.End()`에서 발생 (spec #4 참고).

### 에러 타입

`internal/domain/roomsession/roomsession.go`:
```go
var (
    ErrNotFound          = errors.New("room session not found")
    ErrExpired           = errors.New("room session expired")
    ErrAlreadyExists     = errors.New("room session already exists")
    ErrInvalidTransition = errors.New("invalid status transition")
)
```

`internal/domain/invitation/invitation.go`:
```go
var (
    ErrNotFound      = errors.New("invitation not found")
    ErrExpired       = errors.New("invitation expired")
    ErrAlreadyExists = errors.New("invitation already exists")
)
```

### 동시성

- 두 in-memory repo 모두 `sync.Mutex`로 보호 (read-on-check가 write를 동반하므로 RWMutex 부적합)
- Repository는 호출자의 트랜잭션 경계를 모름 — `RoomSessionService.Create`가 두 repo의 Create를 호출하며 한 쪽 실패 시 다른 쪽 롤백 (보상 트랜잭션) 책임짐 (spec #2 참고)

---

## File Locations

| 작업 | 파일 |
|------|------|
| 신규 생성 | `internal/domain/roomsession/roomsession.go` (RoomID, RoomSession, 에러, SessionTTL) |
| 신규 생성 | `internal/domain/roomsession/status.go` (Status 값 객체 + 전이 규칙) — 기존 `internal/domain/session/status.go`에서 이전 |
| 신규 생성 | `internal/domain/roomsession/repository.go` (Repository port) |
| 신규 생성 | `internal/domain/invitation/invitation.go` (Invitation 엔티티, 에러, InvitationTTL) |
| 신규 생성 | `internal/domain/invitation/repository.go` (Repository port) |
| 신규 생성 | `internal/infrastructure/memory/room_session_repository.go` (인메모리 어댑터) |
| 신규 생성 | `internal/infrastructure/memory/invitation_repository.go` (인메모리 어댑터) |
| 삭제 | `internal/domain/session/*` (도메인 이전 후) |
| 삭제 | `internal/infrastructure/memory/session_repository.go` (대체됨) |

---

## Acceptance Criteria

### RoomSession Repository
- [ ] `rsRepo.Create(roomsession.New(roomsession.NewID()))` 호출 → status `waiting`, ExpiresAt 10분 후 설정된 RoomSession 반환
- [ ] 동일 RoomID로 `Create` 재호출 → `roomsession.ErrAlreadyExists`
- [ ] `rsRepo.Get(id)` 호출 시 만료된 세션 → 세션 삭제 + `roomsession.ErrExpired` 반환
- [ ] `s.Transition(StatusActive)` 후 `rsRepo.Update(s)` → ExpiresAt zero (무기한)
- [ ] `waiting` → `ended` 전이 허용
- [ ] `ended` → `active` 전이 → `roomsession.ErrInvalidTransition`
- [ ] 동시 goroutine 100개에서 `Create`/`Get` 혼합 호출 시 data race 없음 (`go test -race`)

### Invitation Repository
- [ ] `invRepo.Create(invitation.New(serial, roomID))` → ExpiresAt 10분 후 설정된 Invitation 반환
- [ ] 동일 시리얼로 `Create` 재호출 → `invitation.ErrAlreadyExists`
- [ ] `invRepo.ResolveBySerial(s)` 호출 시 만료된 invitation → 삭제 + `invitation.ErrExpired`
- [ ] `invRepo.ResolveBySerial("UNKNOWN")` → `invitation.ErrNotFound`
- [ ] `invRepo.Delete(s)` 호출 후 `ResolveBySerial(s)` → `ErrNotFound`
- [ ] 동시 goroutine 100개에서 `Create`/`ResolveBySerial`/`Delete` 혼합 호출 시 data race 없음
