# 01. Session Store Spec

## Overview

인메모리 세션 스토어. 시리얼 번호를 키로 세션 상태를 관리하는 핵심 데이터 레이어.  
전체 8단계 구현 중 **1번째** — 다른 모든 컴포넌트가 이 스토어에 의존하므로 가장 먼저 구현한다.

---

## Implementation Order

```
[1] Session Store  ← 지금 여기
 └─> [2] Serial Number Update
 └─> [3] WebSocket Hub
       └─> [4] WebSocket Handler
             └─> [5] Signaling Protocol
             └─> [6] Control Event Relay
[7] TURN Credentials  (독립)
[8] CORS Middleware   (독립)
```

- **선행 의존성:** 없음
- **후행 의존성:** #2(service)와 #4(WS handler)가 `session.Repository` port를 주입받는다. #3 Hub는 self-contained이며 serial을 opaque key로만 다룬다.

---

## Dependencies

외부 패키지 없음. 표준 라이브러리(`errors`, `sync`, `time`)와 내부 도메인 패키지(`domain/serialnumber`)만 사용한다.

```go
// internal/domain/session/session.go
import (
    "errors"
    "time"
    "co-browsing-session-server/internal/domain/serialnumber"
)

// internal/infrastructure/memory/session_repository.go
import (
    "sync"
    "co-browsing-session-server/internal/domain/serialnumber"
    "co-browsing-session-server/internal/domain/session"
)
```

---

## Data Structures

클린 아키텍처 레이어에 맞춰 도메인 타입 + Repository port는 `internal/domain/session/`에, 인메모리 구현체는 `internal/infrastructure/memory/`에 분리한다.

```go
// internal/domain/session/session.go
package session

import (
    "time"
    "co-browsing-session-server/internal/domain/serialnumber"
)

type Session struct {
    Serial     serialnumber.SerialNumber
    Status     Status
    CustomerID string
    AgentID    string
    CreateAt   time.Time
    ExpiresAt  time.Time
}
```

```go
// internal/domain/session/status.go
package session

type Status string

const (
    StatusWaiting Status = "waiting"  // 고객만 접속, 상담사 대기 중
    StatusActive  Status = "active"   // 양쪽 모두 접속, 세션 진행 중
    StatusEnded   Status = "ended"    // 세션 종료
)
```

```go
// internal/infrastructure/memory/session_repository.go
package memory

import (
    "sync"
    "co-browsing-session-server/internal/domain/serialnumber"
    "co-browsing-session-server/internal/domain/session"
)

type SessionRepository struct {
    mu       sync.RWMutex
    sessions map[serialnumber.SerialNumber]*session.Session
}

func NewSessionRepository() *SessionRepository {
    return &SessionRepository{
        sessions: make(map[serialnumber.SerialNumber]*session.Session),
    }
}
```

---

## Interfaces / Contracts

### Repository port (도메인 계약)

```go
// internal/domain/session/repository.go
package session

import "co-browsing-session-server/internal/domain/serialnumber"

type Repository interface {
    Create(s *Session) (*Session, error)
    Get(serial serialnumber.SerialNumber) (*Session, error)
    Update(s *Session) (*Session, error)
    Delete(serial serialnumber.SerialNumber) error
}
```

### 상태 전이 (도메인 행동)

상태 전이는 Session 엔티티 자체의 메서드로 캡슐화한다. Repository는 저장만 책임지고, 전이 규칙은 도메인에 둔다.

```go
// internal/domain/session/session.go
func (s *Session) Transition(to Status) error  // 전이 규칙 검증 후 적용
func (s *Session) IsExpired(now time.Time) bool

// internal/domain/session/status.go
func (s Status) IsValid() bool
func (s Status) CanTransitionTo(to Status) bool
```

### 상수

```go
// internal/domain/session/session.go
const TTL = 10 * time.Minute  // 고객 접속 후 상담사 미접속 시 만료
```

시리얼 번호 자체(`internal/domain/serialnumber/`)는 별도 도메인이며, 길이(`serialLength = 6`)는 application service(`internal/services/session/service.go`)의 상수로 관리된다.

---

## Behavior

### 상태 머신

```
[생성 시]  waiting
    │
    │ (상담사 WebSocket 접속 → Session.Transition(StatusActive))
    ▼
  active
    │
    │ (어느 쪽이든 leave 메시지 / 연결 끊김 → Session.Transition(StatusEnded))
    ▼
  ended
```

유효한 상태 전이:
- `waiting` → `active` (허용)
- `active` → `ended` (허용)
- `waiting` → `ended` (허용, 고객이 직접 나간 경우)
- 그 외 전이는 에러 반환

### TTL 동작

- `session.New(serial)` 호출 시 `ExpiresAt = time.Now().Add(session.TTL)` 설정
- 만료 판정은 `Session.IsExpired(now)` 도메인 메서드로 노출
- repository `Get` 호출 시 만료 체크 + 만료된 세션 삭제 + `session.ErrExpired` 반환 (read-on-check)
- 별도 백그라운드 goroutine 없이 **read-on-check** 방식으로 만료 처리 (MVP 단순화)
- 상태가 `active`로 전환되면 `Transition` 내부에서 `ExpiresAt = time.Time{}`(zero)로 설정하여 무기한 유지

### 에러 타입

도메인 패키지(`internal/domain/session/session.go`)에 위치:

```go
var (
    ErrNotFound          = errors.New("session not found")
    ErrExpired           = errors.New("session expired")
    ErrAlreadyExists     = errors.New("session already exists")
    ErrInvalidTransition = errors.New("invalid status transition")
)
```

### 동시성

- 모든 map 접근은 `sync.RWMutex`로 보호 (infrastructure 레이어의 책임)
- Read 연산(`Get`)은 `RLock`, Write 연산(`Create`, `Update`, `Delete`)은 `Lock`

---

## File Locations

| 작업 | 파일 |
|------|------|
| 신규 생성 | `internal/domain/session/session.go` (엔티티 + 에러 + TTL 상수) |
| 신규 생성 | `internal/domain/session/status.go` (Status 값 객체 + 전이 규칙) |
| 신규 생성 | `internal/domain/session/repository.go` (Repository port) |
| 신규 생성 | `internal/infrastructure/memory/session_repository.go` (인메모리 어댑터) |

---

## Acceptance Criteria

- [ ] `repo.Create(session.New(serialnumber.SerialNumber("ABC123")))` 호출 → status `waiting`, ExpiresAt 10분 후로 설정된 세션 반환
- [ ] 동일 시리얼로 `Create` 재호출 → `session.ErrAlreadyExists` 반환
- [ ] `repo.Get` 호출 시 만료된 세션 → 세션 삭제 + `session.ErrExpired` 반환
- [ ] `s.Transition(session.StatusActive)` 후 `repo.Update(s)` → ExpiresAt 무제한 연장
- [ ] `waiting` → `ended` 전이 허용
- [ ] `ended` → `active` 전이 → `session.ErrInvalidTransition` 반환
- [ ] 동시 goroutine 100개에서 `Create`/`Get` 혼합 호출 시 data race 없음 (`go test -race`)
