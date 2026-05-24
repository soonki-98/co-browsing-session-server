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
// 핸들러는 package http에 속하므로 net/http는 alias로 import한다.
import (
    "encoding/json"
    "log"
    nethttp "net/http"

    "github.com/gin-gonic/gin"
    "github.com/gorilla/websocket"

    "co-browsing-session-server/internal/domain/serialnumber"
    "co-browsing-session-server/internal/domain/session"
    "co-browsing-session-server/internal/services/hub"
)
```

WS 핸들러는 `interfaces/http` 레이어에 있으므로 도메인 타입(`session.Repository`, `serialnumber.SerialNumber`)과 서비스(`hub.Hub`)에만 의존한다. 인프라(memory 구현)는 모른다.

---

## Data Structures

### WebSocket 메시지 DTO

WS 프로토콜 페이로드는 interface(adapter) 레이어의 DTO로 두고, 도메인 타입과 섞지 않는다. `interfaces/http` 패키지 내 별도 파일로 분리:

```go
// internal/interfaces/http/signaling.go

package http

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

// control-event payload는 spec #6 (relay 서비스)에서 자체 정의한다.
// WS 핸들러는 control-event를 raw json.RawMessage로 받아 relay에 위임만 한다.

// --- 서버 → 클라이언트 페이로드 ---

type ErrorPayload struct {
    Code    string `json:"code"`
    Message string `json:"message"`
}
```

### Upgrader 설정

```go
var upgrader = websocket.Upgrader{
    CheckOrigin: func(r *nethttp.Request) bool {
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
// internal/interfaces/http/websocket.go
func NewWebSocketHandler(h *hub.Hub, repo session.Repository) *WebSocketHandler

// 기존 SessionHandler와 동일하게 Handler 인터페이스 구현 — router.go 변경 없음
func (h *WebSocketHandler) Register(r *gin.Engine) {
    r.GET("/ws", h.handleUpgrade)
}
```

---

## Behavior

### 연결 수립 흐름

```
1. query param 검증: serial, role 필수
   - 누락 시 400 반환

2. repo.Get(serialnumber.SerialNumber(serial)) 검증
   - session.ErrNotFound / session.ErrExpired → 404 반환
   - status == session.StatusEnded → 404 반환

3. upgrader.Upgrade(c.Writer, c.Request) → *websocket.Conn 획득
   - 실패 시 500 (gorilla가 자동 처리)

4. Client 생성:
   client = &hub.Client{
       Conn:   conn,
       Role:   hub.Role(role),
       Serial: serial,
       Send:   make(chan []byte, 256),
   }

5. peer := h.JoinRoom(serial, client) (h: 핸들러에 주입된 *hub.Hub 인스턴스)
   peer가 nil이 아니면 (= 두 번째 접속자가 들어와 양쪽이 모인 경우):
   - peer.Send에 peer-joined 이벤트 전송
   - 세션 상태 전이 waiting → active:
     if s, err := repo.Get(serialnumber.SerialNumber(serial)); err == nil {
         if err := s.Transition(session.StatusActive); err == nil {  // 도메인 행동: 전이 + ExpiresAt 무기한
             repo.Update(s)
         }
         // ErrInvalidTransition은 이미 active/ended이므로 무시한다.
     }

6. 두 goroutine 실행:
   go writePump(client)               // Send 채널 → WebSocket 쓰기
   go readPump(client, h, repo)       // WebSocket 읽기 → 메시지 디스패치
```

### readPump (WebSocket 읽기 루프)

```
for {
    _, message, err = conn.ReadMessage()
    if err (close / timeout) → break

    var msg Message  // package http의 DTO
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
peer := h.LeaveRoom(client)
conn.Close()
if peer != nil {
    peer.Send <- marshalMessage("peer-left", nil)
}
// 세션 종료 전이 (이미 ended면 ErrInvalidTransition은 무시)
if s, err := repo.Get(serialnumber.SerialNumber(serial)); err == nil {
    if err := s.Transition(session.StatusEnded); err == nil {
        repo.Update(s)
    }
}
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
// internal/interfaces/http/websocket.go 내부 헬퍼

// 임의 타입과 페이로드로 {"type":"...","payload":...} JSON 바이트를 생성
func marshalMessage(msgType string, payload any) []byte

// 발신 클라이언트에게 에러 메시지 전송 (Send 채널 push)
// 내부적으로 marshalMessage("error", ErrorPayload{Code: code, Message: message})를 사용
func sendError(client *hub.Client, code, message string)
```

---

## File Locations

| 작업 | 파일 |
|------|------|
| 신규 생성 | `internal/interfaces/http/websocket.go` (Handler 구현 + WS 루프) |
| 신규 생성 | `internal/interfaces/http/signaling.go` (WS 메시지 DTO) |
| 수정 | `internal/app/app.go` (Hub 생성 후 WS 핸들러에 Hub + Repository 주입, `NewRouter`에 추가) |

---

## Acceptance Criteria

- [ ] `GET /ws?serial=존재하지않는값&role=customer` → 연결 거부 (404)
- [ ] `GET /ws?serial=유효&role=customer` → WebSocket 연결 성공
- [ ] 고객 접속 후 상담사 접속 → 고객 측에서 `{"type":"peer-joined"}` 수신
- [ ] 상담사가 `{"type":"leave"}` 전송 → 고객 측에서 `{"type":"peer-left"}` 수신
- [ ] 연결 끊김(네트워크 오류) 시 상대방에게 `peer-left` 전송
- [ ] readPump와 writePump가 독립 goroutine으로 실행됨 (블로킹 없음)
- [ ] 잘못된 메시지 타입 수신 시 연결 유지, `{"type":"error", "payload":{"code":"UNKNOWN_TYPE"}}` 응답
