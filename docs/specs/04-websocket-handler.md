# 04. WebSocket Handler Spec

## Overview

`GET /ws` HTTP 엔드포인트. 시리얼을 받아 `Invitation`으로 RoomID를 풀고, WebSocket으로 업그레이드한 뒤 Hub에 클라이언트를 등록하고 read/write 루프를 실행한다. 양쪽이 모이면 `RoomSession`을 `active`로 전이시키고, disconnect 시 `ended`로 전이 + Invitation을 명시 삭제한다.

전체 8단계 구현 중 **4번째** — WebSocket Hub(#3) 구현 후 진행. 시그널링(#5)과 제어 이벤트(#6)가 이 핸들러 위에서 동작한다.

---

## Implementation Order

```
[1] Domain Stores ✓
[2] Room Handler (POST /rooms) ✓
[3] WebSocket Hub ✓
[4] WebSocket Handler  ← 지금 여기
[5] Signaling Protocol
[6] Control Event Relay
[7] TURN Credentials
[8] CORS Middleware
```

- **선행 의존성:** `#1 Domain Stores`, `#3 WebSocket Hub`
- **후행 의존성:** `#5 Signaling Protocol`, `#6 Control Event Relay`

---

## Dependencies

```go
// 핸들러는 package http에 속하므로 net/http는 alias로 import
import (
    "encoding/json"
    "errors"
    "log"
    nethttp "net/http"

    "github.com/gin-gonic/gin"
    "github.com/gorilla/websocket"

    "co-browsing-session-server/internal/domain/invitation"
    "co-browsing-session-server/internal/domain/roomsession"
    "co-browsing-session-server/internal/domain/serialnumber"
    "co-browsing-session-server/internal/services/hub"
    rssvc "co-browsing-session-server/internal/services/roomsession"
)
```

WS 핸들러는 `interfaces/http` 레이어에 있으므로 도메인 타입(`invitation.Repository`, `roomsession.RoomID`, `serialnumber.SerialNumber`)과 서비스(`hub.Hub`, `roomsession.Service`)에 의존한다. 인프라(memory 구현)는 모른다.

핸들러가 보유하는 협력자:
- `invRepo invitation.Repository` — 시리얼 → RoomID 해석 + disconnect 시 명시 삭제
- `rsService *rssvc.Service` — RoomSession 상태 전이 (Activate / End)
- `hub *hub.Hub` — 휘발 LiveRoom 관리

---

## Data Structures

### WebSocket 메시지 DTO

WS 프로토콜 페이로드는 interface(adapter) 레이어의 DTO로 두고, 도메인 타입과 섞지 않는다. `interfaces/http/signaling.go`에 별도 파일로 분리:

```go
// internal/interfaces/http/signaling.go
package http

// 모든 WebSocket 메시지의 공통 래퍼
type Message struct {
    Type    string          `json:"type"`
    Payload json.RawMessage `json:"payload,omitempty"`
}

// --- 클라이언트 → 서버 페이로드 ---

type SDPPayload struct {
    SDP string `json:"sdp"`
}

type ICECandidatePayload struct {
    Candidate string `json:"candidate"`
}

// control-event payload는 spec #6(relay)에서 자체 정의. WS 핸들러는 raw json.RawMessage로 위임.

// --- 서버 → 클라이언트 페이로드 ---

type ErrorPayload struct {
    Code    string `json:"code"`
    Message string `json:"message"`
}
```

### Client 구조 확장

`services/hub.Client` 구조에 `Serial` 필드를 둔다 — disconnect 시 `Invitation` 삭제에 사용:

```go
// services/hub.go (spec #3 보강)
type Client struct {
    Conn   *websocket.Conn
    Role   Role
    RoomID string                  // = invitation.RoomID 값
    Serial serialnumber.SerialNumber  // disconnect 시 invRepo.Delete에 사용
    Send   chan []byte
}
```

이 필드는 핸들러가 `Invitation.ResolveBySerial` 결과를 들고 와서 Client 생성 시 함께 설정한다.

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
실패 400 Bad Request:
  { "error": "missing serial or role parameter" }
실패 404 Not Found:
  { "error": "invitation not found or expired" }
실패 410 Gone:
  { "error": "session already ended" }
```

쿼리 파라미터 키는 `serial` — UX 용어. 내부 처리는 즉시 `invitation.Serial`로 변환.

### 핸들러 생성자

```go
// internal/interfaces/http/websocket.go
type WebSocketHandler struct {
    hub       *hub.Hub
    invRepo   invitation.Repository
    rsService *rssvc.Service
}

func NewWebSocketHandler(h *hub.Hub, invRepo invitation.Repository, rsService *rssvc.Service) *WebSocketHandler

func (h *WebSocketHandler) Register(r *gin.Engine) {
    r.GET("/ws", h.handleUpgrade)
}
```

---

## Behavior

### 연결 수립 흐름

```
1. query param 검증: serial, role 필수
   - 누락 → 400 반환

2. inv, err := h.invRepo.ResolveBySerial(serialnumber.SerialNumber(serial))
   - errors.Is(err, invitation.ErrNotFound) || errors.Is(err, invitation.ErrExpired)
     → 404 반환
   - 그 외 에러 → 500 반환

3. (선택) rs, err := h.rsService.Get(inv.RoomID)
   - status == StatusEnded → 410 반환
   - 만료된 경우 (roomsession.ErrExpired) → 404
   - (`Activate`/`End`이 도메인 메서드를 통해 검증하므로 이 단계는 생략 가능 — 다만 unnecessary upgrade를 막는 가벼운 단계)

4. conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
   - 실패 시 gorilla가 응답 자동 처리

5. Client 생성:
   client := &hub.Client{
       Conn:   conn,
       Role:   hub.Role(role),
       RoomID: string(inv.RoomID),
       Serial: inv.Serial,
       Send:   make(chan []byte, 256),
   }

6. peer := h.hub.JoinRoom(client.RoomID, client)
   peer != nil이면 (= 양쪽 모두 모인 첫 순간):
   - h.rsService.Activate(inv.RoomID)
     · ErrInvalidTransition은 이미 active/ended이므로 무시
     · 그 외 에러는 로깅 후 연결 끊기
   - peer.Send에 peer-joined 이벤트 전송:
       peer.Send <- marshalMessage("peer-joined", nil)

7. 두 goroutine 실행:
   go h.writePump(client)
   go h.readPump(client)
```

### readPump (WebSocket 읽기 루프)

```go
func (h *WebSocketHandler) readPump(client *hub.Client) {
    defer h.cleanup(client)

    for {
        _, message, err := client.Conn.ReadMessage()
        if err != nil {
            // close / timeout / 네트워크 오류
            break
        }

        var msg Message
        if err := json.Unmarshal(message, &msg); err != nil {
            sendError(client, "MALFORMED", "invalid message format")
            continue
        }

        peer := h.hub.GetPeer(client)

        switch msg.Type {
        case signaling.MsgTypeOffer, signaling.MsgTypeAnswer, signaling.MsgTypeICECandidate:
            signaling.HandleSignalingMessage(client, peer, msg.Type, message)  // 원본 바이트
        case "control-event":
            relay.HandleControlEvent(client, peer, msg.Payload)
        case "leave":
            return  // defer cleanup으로 정리
        default:
            sendError(client, "UNKNOWN_TYPE", "unknown message type: "+msg.Type)
        }
    }
}
```

### cleanup (disconnect 처리)

```go
func (h *WebSocketHandler) cleanup(client *hub.Client) {
    peer := h.hub.LeaveRoom(client)
    client.Conn.Close()

    if peer != nil {
        // 첫 disconnect → 세션 종료 + Invitation 명시 삭제
        if err := h.rsService.End(roomsession.RoomID(client.RoomID), client.Serial); err != nil {
            log.Printf("end session: %v", err)
        }
        peer.Send <- marshalMessage("peer-left", nil)
    }
    // peer == nil이면 이미 종료 처리됐거나 단독 접속 후 나간 경우 — 아무것도 안 함
}
```

`rsService.End`는 RoomSession을 ended로 전이 + Invitation을 삭제하는 두 작업을 함께 수행:

```go
// services/roomsession/service.go (spec #2에 정의됨, End 메서드 추가)
func (s *Service) End(roomID roomsession.RoomID, serial serialnumber.SerialNumber) error {
    rs, err := s.rsRepo.Get(roomID)
    if err != nil {
        if errors.Is(err, roomsession.ErrNotFound) || errors.Is(err, roomsession.ErrExpired) {
            // 이미 정리됨 — Invitation만 best-effort 삭제
            _ = s.invRepo.Delete(serial)
            return nil
        }
        return err
    }
    if err := rs.Transition(roomsession.StatusEnded); err != nil {
        if !errors.Is(err, roomsession.ErrInvalidTransition) {
            return err
        }
        // 이미 ended — Invitation만 삭제하고 OK
    } else {
        if _, err := s.rsRepo.Update(rs); err != nil {
            return err
        }
    }
    _ = s.invRepo.Delete(serial)  // best-effort (defense in depth)
    return nil
}
```

### writePump (WebSocket 쓰기 루프)

```go
func (h *WebSocketHandler) writePump(client *hub.Client) {
    defer client.Conn.Close()
    for {
        message, ok := <-client.Send
        if !ok {
            client.Conn.WriteMessage(websocket.CloseMessage, []byte{})
            return
        }
        if err := client.Conn.WriteMessage(websocket.TextMessage, message); err != nil {
            return
        }
    }
}
```

### 헬퍼 함수

```go
// 임의 타입/페이로드를 {"type":"...","payload":...} JSON 바이트로 직렬화
func marshalMessage(msgType string, payload any) []byte

// 발신 클라이언트에게 에러 메시지 push
// 내부적으로 marshalMessage("error", ErrorPayload{Code, Message}) 사용
func sendError(client *hub.Client, code, message string)
```

---

## File Locations

| 작업 | 파일 |
|------|------|
| 신규 생성 | `internal/interfaces/http/websocket.go` (Handler 구현 + WS 루프) |
| 신규 생성 | `internal/interfaces/http/signaling.go` (WS 메시지 공통 DTO) |
| 수정 | `internal/services/hub/hub.go` (Client struct에 `Serial` 필드 추가 — spec #3 보강) |
| 수정 | `internal/services/roomsession/service.go` (`Activate`, `End` 메서드 추가 — spec #2 base) |
| 수정 | `internal/app/app.go` (Hub 생성 + WS 핸들러에 Hub + invRepo + rsService 주입, `NewRouter`에 추가) |

---

## Acceptance Criteria

- [ ] `GET /ws?serial=존재하지않는값&role=customer` → 404 (invitation not found)
- [ ] `GET /ws?serial=유효&role=customer` → WebSocket 연결 성공, RoomSession은 여전히 `waiting`
- [ ] 고객 접속 후 상담사 접속 → 고객 측에서 `{"type":"peer-joined"}` 수신, RoomSession status가 `active`로 전이
- [ ] 상담사가 `{"type":"leave"}` 전송 → 고객 측에서 `{"type":"peer-left"}` 수신, RoomSession status `ended`, Invitation 삭제됨 (`invRepo.ResolveBySerial(s)` → `ErrNotFound`)
- [ ] 연결 끊김(네트워크 오류) 시에도 동일한 종료 처리 (peer-left 알림 + 세션 종료 + invitation 삭제)
- [ ] readPump와 writePump가 독립 goroutine으로 실행됨 (블로킹 없음)
- [ ] 잘못된 메시지 타입 수신 시 연결 유지, `{"type":"error","payload":{"code":"UNKNOWN_TYPE"}}` 응답
- [ ] 세션 종료 후 동일 시리얼로 재접속 시도 → 404 (invitation 삭제됨)
- [ ] TTL 10분 경과 후 사용되지 않은 invitation으로 접속 시도 → 404 (read-on-check 만료)
