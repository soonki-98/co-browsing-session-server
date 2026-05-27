# 06. Control Event Relay Spec

## Overview

상담사(agent)가 전송하는 마우스/키보드 제어 이벤트를 고객(customer)에게 중계한다. 타임스탬프를 서버에서 보완하고 이벤트 타입을 검증한다.  
전체 8단계 구현 중 **6번째** — WebSocket Handler(#4)의 readPump에서 `control-event` 타입 수신 시 호출된다.

---

## Implementation Order

```
[1] Domain Stores ✓
[2] Room Handler (POST /rooms) ✓
[3] WebSocket Hub ✓
[4] WebSocket Handler ✓
[5] Signaling Protocol ✓
[6] Control Event Relay  ← 지금 여기
[7] TURN Credentials
[8] CORS Middleware
```

- **선행 의존성:** `#4 WebSocket Handler`
- **후행 의존성:** 없음

---

## Dependencies

```go
import (
    "encoding/json"
    "time"

    "co-browsing-session-server/internal/services/hub"
)
```

의존성 방향: services 레이어는 interfaces를 import하지 않는다. ControlEventPayload는 **relay 패키지에 자체 정의**한다 (spec #4의 `interfaces/http/signaling.go` DTO와는 동일한 JSON 형태지만 별개의 Go 타입 — 핸들러가 raw bytes로 위임하므로 변환 비용은 없음).

신규 패키지 없음.

---

## Data Structures

```go
// internal/services/relay/control.go
type ControlEventPayload struct {
    Type      string  `json:"type"`            // "click" | "scroll" | "keydown"
    X         *int    `json:"x,omitempty"`
    Y         *int    `json:"y,omitempty"`
    Key       *string `json:"key,omitempty"`
    DeltaY    *int    `json:"deltaY,omitempty"`
    Timestamp int64   `json:"timestamp"`        // Unix milliseconds
}
```

허용되는 이벤트 타입:

```go
var allowedControlEventTypes = map[string]bool{
    "click":   true,
    "scroll":  true,
    "keydown": true,
}
```

---

## Interfaces / Contracts

```go
// internal/services/relay/control.go

package relay

// HandleControlEvent: readPump에서 control-event 수신 시 호출
// client: 메시지를 보낸 주체 (반드시 agent여야 함)
// peer: 고객 클라이언트 (nil이면 고객 미접속)
// rawPayload: control-event의 payload JSON (json.RawMessage)
func HandleControlEvent(client *hub.Client, peer *hub.Client, rawPayload json.RawMessage) error
```

---

## Behavior

### 처리 흐름

```
func HandleControlEvent(client, peer, rawPayload):

1. 발신자 검증:
   if client.Role != hub.RoleAgent:
       → client.Send <- marshalError("FORBIDDEN", "only agent can send control events")
       → return

2. 고객 연결 확인:
   if peer == nil || peer.Role != hub.RoleCustomer:
       → client.Send <- marshalError("PEER_NOT_CONNECTED", "customer is not connected")
       → return

3. payload 파싱:
   var evt ControlEventPayload  // relay 패키지에서 정의 (위 Data Structures 참고)
   json.Unmarshal(rawPayload, &evt)

4. 이벤트 타입 검증:
   if !allowedControlEventTypes[evt.Type]:
       → client.Send <- marshalError("INVALID_EVENT_TYPE", "unknown control event type: "+evt.Type)
       → return

5. 타임스탬프 보완:
   if evt.Timestamp == 0:
       evt.Timestamp = time.Now().UnixMilli()

6. 고객에게 전달:
   outMsg = marshalControlEvent(evt)  // relay 패키지의 helper — `{"type":"control-event","payload":{...}}` 형태로 직렬화
   peer.Send <- outMsg
```

`marshalError` / `marshalControlEvent`는 relay 패키지 내부 헬퍼 — interfaces/http의 `marshalMessage`와는 별개로 정의한다 (의존 방향 보존).

### 이벤트 타입별 필수 필드

| 이벤트 타입 | 필수 필드 | 선택 필드 |
|-------------|-----------|-----------|
| `click` | `x`, `y` | - |
| `scroll` | `deltaY` | `x`, `y` |
| `keydown` | `key` | - |

MVP에서는 필드 누락 검증을 하지 않는다 (클라이언트 책임). 타입 검증만 수행.

### 메시지 변환

- 서버는 payload를 재직렬화하여 전달 (SDP와 달리 타임스탬프 보완 필요)
- 재직렬화 시 `omitempty` 태그 적용 → 없는 필드는 JSON에서 제외

### 고객이 수신하는 메시지 형식

```json
{
  "type": "control-event",
  "payload": {
    "type": "click",
    "x": 320,
    "y": 240,
    "timestamp": 1716000000000
  }
}
```

---

## File Locations

| 작업 | 파일 |
|------|------|
| 신규 생성 | `internal/services/relay/control.go` |
| 수정 | `internal/interfaces/http/websocket.go` (readPump의 `case "control-event":` 블록에 호출 추가) |

---

## Acceptance Criteria

- [ ] 상담사가 `{"type":"control-event","payload":{"type":"click","x":100,"y":200}}` 전송 → 고객이 동일 payload + 타임스탬프 수신
- [ ] 타임스탬프 없이 전송 시 서버가 현재 시각(Unix ms)으로 보완
- [ ] 고객이 `control-event` 전송 시 → `{"type":"error","payload":{"code":"FORBIDDEN"}}` 응답
- [ ] 알 수 없는 이벤트 타입(`"type":"hover"`) 전송 시 → `{"type":"error","payload":{"code":"INVALID_EVENT_TYPE"}}` 응답
- [ ] 고객 미접속 상태에서 상담사가 이벤트 전송 → `{"type":"error","payload":{"code":"PEER_NOT_CONNECTED"}}` 응답
