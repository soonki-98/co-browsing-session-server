# 03. WebSocket Hub Spec

## Overview

WebSocket 연결들을 룸 단위로 관리하는 중앙 허브(Hub). 고객(customer)과 상담사(agent)를 같은 룸에 묶고, 메시지 라우팅의 주소록 역할을 한다.  
전체 8단계 구현 중 **3번째** — Session Store(#1) 구현 후 진행. WebSocket Handler(#4)가 이 Hub를 사용한다.

---

## Implementation Order

```
[1] Session Store ✓
[2] Serial Number Update ✓
[3] WebSocket Hub  ← 지금 여기
[4] WebSocket Handler
[5] Signaling Protocol
[6] Control Event Relay
[7] TURN Credentials
[8] CORS Middleware
```

- **선행 의존성:** `#1 Session Store`
- **후행 의존성:** `#4 WebSocket Handler`, `#5 Signaling Protocol`, `#6 Control Event Relay`

---

## Dependencies

```go
// 신규 외부 패키지 추가 필요
// go get github.com/gorilla/websocket
import (
    "sync"

    "github.com/gorilla/websocket"
)
```

Hub는 self-contained — 도메인이나 인프라를 참조하지 않는다. serial 유효성 검증은 호출자(WS 핸들러)가 `repo.Get`으로 먼저 검사한 뒤 JoinRoom에 넘긴다. Hub는 serial을 opaque key로만 다룬다.

`go.mod`에 `github.com/gorilla/websocket v1.5.3` 추가.

---

## Data Structures

```go
// internal/services/hub/hub.go

package hub

import (
    "sync"

    "github.com/gorilla/websocket"
)

type Role string

const (
    RoleCustomer Role = "customer"
    RoleAgent    Role = "agent"
)

// Client: WebSocket 연결 하나를 나타냄
type Client struct {
    Conn   *websocket.Conn
    Role   Role
    Serial string  // 속한 룸의 시리얼 번호
    Send   chan []byte  // 이 클라이언트에게 보낼 메시지 채널
}

// Room: 하나의 co-browsing 세션 룸
type Room struct {
    Serial   string
    Customer *Client  // nil이면 아직 미접속
    Agent    *Client  // nil이면 아직 미접속
}

// Hub: 모든 룸을 관리
type Hub struct {
    mu    sync.RWMutex
    rooms map[string]*Room  // key: serial number
}

func NewHub() *Hub {
    return &Hub{
        rooms: make(map[string]*Room),
    }
}
```

---

## Interfaces / Contracts

```go
// Hub 메서드 시그니처

// JoinRoom: 클라이언트를 룸에 추가. 룸이 없으면 생성.
// 동일 role이 이미 접속 중이면 기존 연결을 Close()한 뒤 교체.
// 반환: 상대 role 클라이언트 (nil이면 아직 미접속)
// Hub는 self-contained이므로 별도 error는 반환하지 않는다(호출자가 사전 검증).
func (h *Hub) JoinRoom(serial string, client *Client) (peer *Client)

// LeaveRoom: 클라이언트를 룸에서 제거.
// 룸에 아무도 없으면 룸 삭제.
// 반환: 상대방 클라이언트 (peer-left 알림 전송용, nil 가능)
func (h *Hub) LeaveRoom(client *Client) (peer *Client)

// GetRoom: 룸 조회. 없으면 nil 반환.
func (h *Hub) GetRoom(serial string) *Room

// GetPeer: 특정 클라이언트의 상대방 반환. 없으면 nil.
func (h *Hub) GetPeer(client *Client) *Client
```

---

## Behavior

### JoinRoom 흐름

```
1. hub.mu.Lock()
2. rooms[serial] 조회
   - 없으면 새 Room 생성 후 rooms[serial] = room
3. client.Role에 따라 room.Customer 또는 room.Agent에 할당
   - 이미 할당돼 있으면 기존 conn.Close() 후 교체
4. peer = 상대방 Client (nil 가능)
5. hub.mu.Unlock()
6. peer 반환
```

### LeaveRoom 흐름

```
1. hub.mu.Lock()
2. rooms[serial] 조회
3. 해당 role 슬롯을 nil로 설정
4. Customer == nil && Agent == nil → rooms[serial] 삭제
5. peer 반환 (nil 가능)
6. hub.mu.Unlock()
```

### Client.Send 채널

- 크기: `make(chan []byte, 256)` (버퍼드)
- WebSocket Handler의 쓰기 goroutine이 이 채널을 읽어서 전송
- 채널 가득 찼을 때(슬로우 클라이언트): 연결 종료

### 에러 타입

Hub는 self-contained이며 외부에 노출하는 에러 sentinel을 두지 않는다. 룸/peer 조회 실패는 nil 반환으로 신호한다. serial 유효성 검증은 호출자(WS 핸들러)가 `repo.Get`으로 사전 수행한다.

---

## File Locations

| 작업 | 파일 |
|------|------|
| 신규 생성 | `internal/services/hub/hub.go` |
| 수정 | `go.mod` (gorilla/websocket 추가) |
| 수정 | `internal/app/app.go` (composition root에서 Hub 생성 및 WS 핸들러에 주입) |

---

## Acceptance Criteria

- [ ] 고객 접속 → `JoinRoom` 반환값 peer = nil
- [ ] 상담사 접속 → `JoinRoom` 반환값 peer = 고객 Client
- [ ] 고객 재접속 시 기존 고객 연결 종료 후 교체
- [ ] 양쪽 모두 나간 후 `GetRoom` → nil 반환 (룸 삭제 확인)
- [ ] 동시 goroutine 50개에서 JoinRoom/LeaveRoom 혼합 호출 시 data race 없음 (`go test -race`)
- [ ] `Client.Send` 채널 가득 찼을 때 해당 클라이언트 연결 종료 (드롭하지 않음)
