# 05. Signaling Protocol Spec

## Overview

WebRTC P2P 연결 수립에 필요한 SDP offer/answer와 ICE candidate를 고객과 상담사 사이에서 중계하는 시그널링 로직.  
전체 8단계 구현 중 **5번째** — WebSocket Handler(#4)의 readPump 메시지 디스패치에서 호출된다.

---

## Implementation Order

```
[1] Session Store ✓
[2] Serial Number Update ✓
[3] WebSocket Hub ✓
[4] WebSocket Handler ✓
[5] Signaling Protocol  ← 지금 여기
[6] Control Event Relay
[7] TURN Credentials
[8] CORS Middleware
```

- **선행 의존성:** `#4 WebSocket Handler` (readPump 내 switch 블록에 삽입)
- **후행 의존성:** 없음

---

## Dependencies

```go
import (
    "encoding/json"

    "co-browsing-session-server/internal/services/hub"
)
```

의존성 방향 주의: services 레이어는 interfaces 레이어를 import하지 않는다. WS 핸들러(`interfaces/http`)가 JSON을 파싱한 뒤 signaling 함수에 메시지 타입과 원본 바이트를 값으로 전달한다.

신규 패키지 없음.

---

## Data Structures

WS 메시지 DTO(`SDPPayload`, `ICECandidatePayload` 등)는 `internal/interfaces/http/signaling.go`에 정의된다 (spec #4 참고). signaling 서비스 자체는 페이로드를 파싱하지 않고 원본 바이트만 중계하므로 DTO 타입을 참조할 필요가 없다.

```go
// internal/services/signaling/signaling.go: 메시지 타입 상수 (signaling 패키지에서 정의·export)
// 인터페이스 레이어가 import 방향상 services를 import할 수 있으므로
// readPump의 switch에서 signaling.MsgTypeOffer 등으로 참조 가능.
const (
    MsgTypeOffer        = "offer"
    MsgTypeAnswer       = "answer"
    MsgTypeICECandidate = "ice-candidate"
    MsgTypePeerJoined   = "peer-joined"
    MsgTypePeerLeft     = "peer-left"
    MsgTypeError        = "error"
)
```

---

## Interfaces / Contracts

```go
// internal/services/signaling/signaling.go

package signaling

// HandleSignalingMessage: readPump에서 호출되는 시그널링 메시지 처리 함수
// client:   메시지를 보낸 주체
// peer:     상대방 클라이언트 (nil이면 상대방 미접속)
// msgType:  파싱된 메시지 타입 ("offer" | "answer" | "ice-candidate")
// rawBytes: 원본 WS 메시지 (SDP 보존을 위해 재직렬화하지 않음)
func HandleSignalingMessage(client *hub.Client, peer *hub.Client, msgType string, rawBytes []byte) error
```

---

## Behavior

### 메시지 라우팅 규칙

| 수신 타입 | 발신자 | 수신자 | 검증 조건 |
|-----------|--------|--------|-----------|
| `offer` | customer | agent | 발신자 role == customer |
| `answer` | agent | customer | 발신자 role == agent |
| `ice-candidate` | 양쪽 | 상대방 | 없음 |

### 처리 흐름

```
func HandleSignalingMessage(client, peer, msgType, rawBytes):
    if peer == nil:
        → client.Send <- marshalError("PEER_NOT_CONNECTED", "peer is not connected yet")
        → return

    switch msgType:
    case "offer":
        if client.Role != hub.RoleCustomer:
            → client.Send <- marshalError("INVALID_SENDER", "offer must be sent by customer")
            → return
        peer.Send <- rawBytes  // 원본 JSON 그대로 전달

    case "answer":
        if client.Role != hub.RoleAgent:
            → client.Send <- marshalError("INVALID_SENDER", "answer must be sent by agent")
            → return
        peer.Send <- rawBytes  // 원본 JSON 그대로 전달

    case "ice-candidate":
        peer.Send <- rawBytes  // 발신자 무관, 상대방에게 그대로 전달
```

`marshalError`는 signaling 패키지 내부 헬퍼 — `{"type":"error","payload":{"code":...,"message":...}}` 형태로 직렬화. interfaces/http의 헬퍼와는 별개 (의존 방향 보존).

### 메시지 투명 중계

SDP payload는 서버가 파싱하지 않고 JSON 원문 그대로 전달한다.  
이유: SDP는 클라이언트(브라우저 WebRTC API)가 생성하고 소비하는 불투명 데이터이며, 서버가 수정하면 안 된다.

```go
// 올바른 구현: 원본 바이트 그대로 전달
peer.Send <- originalRawBytes

// 잘못된 구현: 재직렬화하면 SDP 포맷이 변형될 수 있음
// json.Marshal(msg) → X
```

### WebSocket Handler와의 통합 (readPump switch 블록)

```go
// internal/interfaces/http/websocket.go readPump 내부
// message는 ReadMessage가 돌려준 원본 바이트 (spec #4 참고)
peer := h.GetPeer(client)

switch msg.Type {
case "offer", "answer", "ice-candidate":
    signaling.HandleSignalingMessage(client, peer, msg.Type, message)
case "control-event":
    // #6에서 처리
case "leave":
    break
default:
    sendError(client, "UNKNOWN_TYPE", "unknown message type: "+msg.Type)
}
```

원본 `message` 바이트를 그대로 전달하는 이유: SDP 원문 보존을 위해 재직렬화를 피한다.

### 에러 응답 형식

```go
// 에러 발생 시 발신자(client)에게만 전송
{
  "type": "error",
  "payload": {
    "code": "PEER_NOT_CONNECTED",
    "message": "peer is not connected yet"
  }
}
```

---

## File Locations

| 작업 | 파일 |
|------|------|
| 신규 생성 | `internal/services/signaling/signaling.go` |
| 수정 | `internal/interfaces/http/websocket.go` (readPump switch에 호출 추가) |

---

## Acceptance Criteria

- [ ] 고객이 `{"type":"offer","payload":{"sdp":"v=0..."}}` 전송 → 상담사가 동일 메시지 수신
- [ ] 상담사가 `{"type":"answer","payload":{"sdp":"v=0..."}}` 전송 → 고객이 동일 메시지 수신
- [ ] 양쪽 모두 `ice-candidate` 교환 가능
- [ ] 상담사가 미접속 상태에서 고객이 offer 전송 → `{"type":"error","payload":{"code":"PEER_NOT_CONNECTED"}}` 수신
- [ ] 상담사가 offer 전송 시 → `{"type":"error","payload":{"code":"INVALID_SENDER"}}` 수신
- [ ] SDP 값이 중계 과정에서 변경되지 않음 (원문 보존)
