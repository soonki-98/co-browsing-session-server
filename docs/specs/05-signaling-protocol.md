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
    "co-browsing-session-server/internal/hub"
    "co-browsing-session-server/internal/model"
    "encoding/json"
)
```

신규 패키지 없음.

---

## Data Structures

기존 `model.SDPPayload`, `model.ICECandidatePayload` 사용 (spec #4에서 정의됨).

```go
// 시그널링 관련 메시지 타입 상수 (핸들러 또는 별도 상수 파일에 정의)
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
// internal/signaling/signaling.go

package signaling

// HandleSignalingMessage: readPump에서 호출되는 시그널링 메시지 처리 함수
// client: 메시지를 보낸 주체
// peer: 상대방 클라이언트 (nil이면 상대방 미접속)
// msg: 수신된 메시지
func HandleSignalingMessage(client *hub.Client, peer *hub.Client, msg *model.Message) error
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
func HandleSignalingMessage(client, peer, msg):
    if peer == nil:
        → 에러: peer가 아직 미접속
        → client.Send <- error("PEER_NOT_CONNECTED", "peer is not connected yet")
        → return

    switch msg.Type:
    case "offer":
        if client.Role != RoleCustomer → error("INVALID_SENDER", "offer must be sent by customer")
        peer.Send <- msg (원본 JSON 그대로 전달)

    case "answer":
        if client.Role != RoleAgent → error("INVALID_SENDER", "answer must be sent by agent")
        peer.Send <- msg (원본 JSON 그대로 전달)

    case "ice-candidate":
        peer.Send <- msg (발신자 무관, 상대방에게 그대로 전달)
```

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
// internal/handler/websocket.go readPump 내부
peer := h.GetPeer(client)

switch msg.Type {
case "offer", "answer", "ice-candidate":
    signaling.HandleSignalingMessage(client, peer, &msg, rawBytes)
case "control-event":
    // #6에서 처리
case "leave":
    break
default:
    sendError(client, "UNKNOWN_TYPE", "unknown message type: "+msg.Type)
}
```

rawBytes를 전달하는 이유: SDP 원문 보존을 위해.

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
| 신규 생성 | `internal/signaling/signaling.go` |
| 수정 | `internal/handler/websocket.go` (readPump switch에 호출 추가) |

---

## Acceptance Criteria

- [ ] 고객이 `{"type":"offer","payload":{"sdp":"v=0..."}}` 전송 → 상담사가 동일 메시지 수신
- [ ] 상담사가 `{"type":"answer","payload":{"sdp":"v=0..."}}` 전송 → 고객이 동일 메시지 수신
- [ ] 양쪽 모두 `ice-candidate` 교환 가능
- [ ] 상담사가 미접속 상태에서 고객이 offer 전송 → `{"type":"error","payload":{"code":"PEER_NOT_CONNECTED"}}` 수신
- [ ] 상담사가 offer 전송 시 → `{"type":"error","payload":{"code":"INVALID_SENDER"}}` 수신
- [ ] SDP 값이 중계 과정에서 변경되지 않음 (원문 보존)
