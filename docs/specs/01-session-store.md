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
- **후행 의존성:** #2, #3 (모든 컴포넌트가 SessionStore를 주입받음)

---

## Dependencies

```go
// 표준 라이브러리만 사용
import (
    "sync"
    "time"
)
```

신규 외부 패키지 없음.

---

## Data Structures

```go
// internal/store/session.go

package store

import (
    "sync"
    "time"
)

type SessionStatus string

const (
    StatusWaiting SessionStatus = "waiting"  // 고객만 접속, 상담사 대기 중
    StatusActive  SessionStatus = "active"   // 양쪽 모두 접속, 세션 진행 중
    StatusEnded   SessionStatus = "ended"    // 세션 종료
)

type Session struct {
    Serial      string        // 6자리 시리얼 번호 (Primary Key)
    Status      SessionStatus
    CustomerID  string        // WebSocket 연결 시 할당 (현재는 미사용, 확장용)
    AgentID     string        // WebSocket 연결 시 할당 (현재는 미사용, 확장용)
    CreatedAt   time.Time
    ExpiresAt   time.Time     // TTL 만료 시각
}

type SessionStore struct {
    mu       sync.RWMutex
    sessions map[string]*Session // key: serial number
}

func NewSessionStore() *SessionStore {
    return &SessionStore{
        sessions: make(map[string]*Session),
    }
}
```

---

## Interfaces / Contracts

```go
// SessionStore 메서드 시그니처

// Create: 새 세션 등록. 동일 시리얼이 이미 존재하면 에러 반환.
func (s *SessionStore) Create(serial string) (*Session, error)

// Get: 시리얼로 세션 조회. 없거나 만료됐으면 nil, error 반환.
func (s *SessionStore) Get(serial string) (*Session, error)

// UpdateStatus: 세션 상태 전이. 유효하지 않은 전이면 에러 반환.
func (s *SessionStore) UpdateStatus(serial string, status SessionStatus) error

// Delete: 세션 삭제.
func (s *SessionStore) Delete(serial string)
```

### 상수

```go
const (
    SessionTTL        = 10 * time.Minute  // 고객 접속 후 상담사 미접속 시 만료
    SerialNumberLength = 6
)
```

---

## Behavior

### 상태 머신

```
[생성 시]  waiting
    │
    │ (상담사 WebSocket 접속 → UpdateStatus)
    ▼
  active
    │
    │ (어느 쪽이든 leave 메시지 / 연결 끊김)
    ▼
  ended
```

유효한 상태 전이:
- `waiting` → `active` (허용)
- `active` → `ended` (허용)
- `waiting` → `ended` (허용, 고객이 직접 나간 경우)
- 그 외 전이는 에러 반환

### TTL 동작

- `Create` 호출 시 `ExpiresAt = time.Now().Add(SessionTTL)` 설정
- `Get` 호출 시 `time.Now().After(ExpiresAt)` 체크 → 만료됐으면 세션 삭제 후 `ErrSessionExpired` 반환
- 별도 백그라운드 goroutine 없이 **read-on-check** 방식으로 만료 처리 (MVP 단순화)
- 상태가 `active`로 전환되면 TTL을 무제한으로 연장 (`ExpiresAt = time.Time{}` 또는 매우 큰 값)

### 에러 타입

```go
var (
    ErrSessionNotFound = errors.New("session not found")
    ErrSessionExpired  = errors.New("session expired")
    ErrSessionExists   = errors.New("session already exists")
    ErrInvalidTransition = errors.New("invalid status transition")
)
```

### 동시성

- 모든 map 접근은 `sync.RWMutex`로 보호
- Read 연산(`Get`)은 `RLock`, Write 연산(`Create`, `UpdateStatus`, `Delete`)은 `Lock`

---

## File Locations

| 작업 | 파일 |
|------|------|
| 신규 생성 | `internal/store/session.go` |

`internal/repository/` 디렉토리는 비어 있으므로 새 `internal/store/` 패키지 사용.

---

## Acceptance Criteria

- [ ] `Create("ABC123")` 호출 → status `waiting`, ExpiresAt 10분 후로 설정된 세션 반환
- [ ] 동일 시리얼로 `Create` 재호출 → `ErrSessionExists` 반환
- [ ] `Get` 호출 시 만료된 세션 → 세션 삭제 + `ErrSessionExpired` 반환
- [ ] `UpdateStatus("ABC123", StatusActive)` → ExpiresAt 무제한 연장
- [ ] `waiting` → `ended` 전이 허용
- [ ] `ended` → `active` 전이 → `ErrInvalidTransition` 반환
- [ ] 동시 goroutine 100개에서 `Create`/`Get` 혼합 호출 시 data race 없음 (`go test -race`)
