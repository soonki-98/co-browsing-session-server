# 04. WebSocket Handler Spec

## Overview

`GET /ws` HTTP 엔드포인트. HTTP 연결을 WebSocket으로 업그레이드하고, Hub에 클라이언트를 등록한 뒤 메시지 read/write 루프를 실행한다. 메시지 타입별 디스패치 로직도 이 핸들러에서 시작된다.  
전체 8단계 구현 중 **4번째** — WebSocket Hub(#3) 구현 후 진행. 시그널링(#5)과 제어 이벤트(#6)가 이 핸들러 위에서 동작한다.

---

## Implementation Order

```
[1] Session Store ✓
[2] Serial Number Update ✓
[3] WebSocket Hub ✓
[4] WebSocket Handler  ← 지금 여기
[5] Signaling Protocol
[6] Control Event Relay
[7] TURN Credentials
[8] CORS Middleware
```

- **선행 의존성:** `#3 WebSocket Hub`
- **후행 의존성:** `#5 Signaling Protocol`, `#6 Control Event Relay`

---

## Dependencies

```go
import (
    "co-browsing-session-server/internal/hub"
    "co-browsing-session-server/internal/repository"
    "co-browsing-session-server/internal/model"
    "encoding/json"
    "github.com/gin-gonic/gin"
    "github.com/gorilla/websocket"
    "log"
    "net/http"
)
```

---

## Data Structures

### WebSocket 메시지 모델 (model 패키지 업데이트)

`internal/model/signaling.go`를 요구사항 프로토콜에 맞게 교체:

```go
// internal/model/signaling.go (전면 교체)

package model

// 모든 WebSocket 메시지의 공통 래퍼
type Message struct {
    Type    string          `json:"type"`
    Payload json.RawMessage `json:"payload,omitempty"`
}

// --- 클라이언트 → 서버 페이로드 ---

type JoinPayload struct {
    Serial string `json:"serial"`
    Role   string `json:"role"` // "customer" | "agent"
}

type SDPPayload struct {
    SDP string `json:"sdp"`
}

type ICECandidatePayload struct {
    Candidate string `json:"candidate"`
}

type ControlEventPayload struct {
    Type      string  `json:"type"`            // "click" | "scroll" | "keydown"
    X         *int    `json:"x,omitempty"`
    Y         *int    `json:"y,omitempty"`
    Key       *string `json:"key,omitempty"`
    DeltaY    *int    `json:"deltaY,omitempty"`
    Timestamp int64   `json:"timestamp"`
}

// --- 서버 → 클라이언트 페이로드 ---

type ErrorPayload struct {
    Code    string `json:"code"`
    Message string `json:"message"`
}
```

### Upgrader 설정

```go
var upgrader = websocket.Upgrader{
    CheckOrigin: func(r *http.Request) bool {
        return true // CORS는 #8에서 Gin 미들웨어로 처리
    },
}
```

---

## Interfaces / Contracts

### HTTP Endpoint

```
GET /ws?serial=AB3K7M&role=customer
  또는
GET /ws?serial=AB3K7M&role=agent

Upgrade: websocket
Connection: Upgrade

연결 성공 시: WebSocket 프로토콜로 전환
실패 시 (400 Bad Request):
  { "error": "missing serial or role parameter" }
실패 시 (404 Not Found):
  { "error": "session not found or expired" }
```

### 핸들러 생성자

```go
func NewWebSocketHandler(h *hub.Hub, s *repository.SessionStore) gin.HandlerFunc

func RegisterWebSocketRoutes(router *gin.Engine, h *hub.Hub, s *repository.SessionStore)
```

---

## Behavior

### 연결 수립 흐름

```
1. query param 검증: serial, role 필수
   - 누락 시 400 반환

2. store.Get(serial) 검증
   - ErrSessionNotFound / ErrSessionExpired → 404 반환
   - status == "ended" → 404 반환

3. websocket.Upgrader.Upgrade(w, r) → *websocket.Conn 획득
   - 실패 시 500 (gorilla가 자동 처리)

4. Client 생성:
   client = &hub.Client{
       Conn:   conn,
       Role:   hub.Role(role),
       Serial: serial,
       Send:   make(chan []byte, 256),
   }

5. hub.JoinRoom(serial, client) 호출
   - peer != nil이면 → peer-joined 이벤트를 고객(customer)에게 전송
   - peer 접속으로 인해 session status가 "waiting" → "active" 전환:
     store.UpdateStatus(serial, StatusActive)

6. 두 goroutine 실행:
   go writePump(client)   // Send 채널 → WebSocket 쓰기
   go readPump(client, hub, store)  // WebSocket 읽기 → 메시지 디스패치
```

### readPump (WebSocket 읽기 루프)

```
for {
    _, message, err = conn.ReadMessage()
    if err (close / timeout) → break

    var msg model.Message
    json.Unmarshal(message, &msg)

    switch msg.Type {
    case "offer":         → #5 시그널링 처리
    case "answer":        → #5 시그널링 처리
    case "ice-candidate": → #5 시그널링 처리
    case "control-event": → #6 제어 이벤트 처리
    case "leave":         → break loop
    default:              → 에러 메시지 전송, 무시
    }
}

// 루프 종료 시 정리:
hub.LeaveRoom(client)
conn.Close()
if peer := hub.GetPeer(client); peer != nil {
    peer.Send <- marshalMessage("peer-left", nil)
}
store.UpdateStatus(serial, StatusEnded)  // 이미 ended이면 무시
```

### writePump (WebSocket 쓰기 루프)

```
for {
    select {
    case message, ok := <-client.Send:
        if !ok → conn.WriteMessage(CloseMessage) → return
        conn.WriteMessage(TextMessage, message)
    }
}
```

### 헬퍼 함수

```go
// JSON 메시지 직렬화 헬퍼
func marshalMessage(msgType string, payload any) []byte
```

---

## File Locations

| 작업 | 파일 |
|------|------|
| 신규 생성 | `internal/handler/websocket.go` |
| 수정 (전면 교체) | `internal/model/signaling.go` |
| 수정 | `main.go` (Hub, Store 주입) |

---

## Acceptance Criteria

- [ ] `GET /ws?serial=존재하지않는값&role=customer` → 연결 거부 (404)
- [ ] `GET /ws?serial=유효&role=customer` → WebSocket 연결 성공
- [ ] 고객 접속 후 상담사 접속 → 고객 측에서 `{"type":"peer-joined"}` 수신
- [ ] 상담사가 `{"type":"leave"}` 전송 → 고객 측에서 `{"type":"peer-left"}` 수신
- [ ] 연결 끊김(네트워크 오류) 시 상대방에게 `peer-left` 전송
- [ ] readPump와 writePump가 독립 goroutine으로 실행됨 (블로킹 없음)
- [ ] 잘못된 메시지 타입 수신 시 연결 유지, `{"type":"error", "payload":{"code":"UNKNOWN_TYPE"}}` 응답
