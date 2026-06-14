// Package relay는 상담원이 만든 제어 동작(click/scroll/keydown)을 같은 룸 세션의
// 고객에게 단방향으로 중계하는 유즈케이스를 담는다.
//
// 검증(발신자·종류·고객 연결)과 타임스탬프 보완, 전달 메시지 직렬화를 수행하고,
// "무엇을 보낼지"(또는 거부 사유)만 결정해 반환한다. 채널 송신·소켓 쓰기는
// interfaces 어댑터의 책임이므로 이 패키지는 같은 services 레이어의 hub에만 의존한다.
package relay

import (
	"encoding/json"
	"time"

	"co-browsing-session-server/internal/services/hub"
)

// ControlEventPayload는 control-event 메시지의 payload다.
// 종류(Type)에 따라 의미 있는 부가 필드만 채워지며, 나머지는 omitempty로 직렬화에서 제외된다.
type ControlEventPayload struct {
	Type      string  `json:"type"` // "click" | "scroll" | "keydown"
	X         *int    `json:"x,omitempty"`
	Y         *int    `json:"y,omitempty"`
	Key       *string `json:"key,omitempty"`
	DeltaY    *int    `json:"deltaY,omitempty"`
	Timestamp int64   `json:"timestamp"` // Unix milliseconds, 0이면 서버가 보완
}

// allowedControlEventTypes는 중계를 허용하는 제어 이벤트 종류 집합이다.
var allowedControlEventTypes = map[string]bool{
	"click":   true,
	"scroll":  true,
	"keydown": true,
}

// RejectionCode는 제어 이벤트가 거부된 사유다. error 메시지의 code로 발신자에게 전달된다.
type RejectionCode string

const (
	RejectForbidden        RejectionCode = "FORBIDDEN"          // 발신자가 상담원이 아님
	RejectInvalidType      RejectionCode = "INVALID_EVENT_TYPE" // 허용되지 않은 종류 / 파싱 실패
	RejectPeerNotConnected RejectionCode = "PEER_NOT_CONNECTED" // 고객 미접속
)

// Result는 제어 이벤트 처리 결과다.
// Rejected가 true면 Code/Message로 발신자에게 거부를 회신하고 전달하지 않는다.
// false면 Outbound를 고객에게 전달한다.
type Result struct {
	Rejected bool
	Code     RejectionCode
	Message  string
	Outbound []byte // 고객에게 보낼 {"type":"control-event","payload":{...}} 직렬화 바이트
}

// HandleControlEvent는 control-event 한 건을 처리한다.
//
//	senderRole: 메시지를 보낸 클라이언트의 역할 (반드시 RoleAgent여야 통과)
//	peer:       상대방 클라이언트 (nil이거나 고객이 아니면 PEER_NOT_CONNECTED)
//	rawPayload: control-event의 payload JSON
func HandleControlEvent(senderRole hub.Role, peer *hub.Client, rawPayload json.RawMessage) Result {
	// 1. 발신자 검증 — 상담원만 제어 동작을 보낼 수 있다 (단방향 상담원 → 고객).
	if senderRole != hub.RoleAgent {
		return Result{
			Rejected: true,
			Code:     RejectForbidden,
			Message:  "only agent can send control events",
		}
	}

	// 2. 이벤트 종류 검증을 위해 payload를 먼저 파싱한다.
	var evt ControlEventPayload
	if err := json.Unmarshal(rawPayload, &evt); err != nil {
		return Result{
			Rejected: true,
			Code:     RejectInvalidType,
			Message:  "malformed control event payload",
		}
	}

	// 3. 이벤트 종류 검증 — click/scroll/keydown만 허용. 그 외는 거부.
	if !allowedControlEventTypes[evt.Type] {
		return Result{
			Rejected: true,
			Code:     RejectInvalidType,
			Message:  "unknown control event type: " + evt.Type,
		}
	}

	// 4. 고객 연결 확인 — 상대가 없거나 고객이 아니면 전달 대상이 없다.
	if peer == nil || peer.Role != hub.RoleCustomer {
		return Result{
			Rejected: true,
			Code:     RejectPeerNotConnected,
			Message:  "customer is not connected",
		}
	}

	// 5. 타임스탬프 보완 — 0(미설정)일 때만 서버 처리 시점으로 채운다. 값이 있으면 보존.
	if evt.Timestamp == 0 {
		evt.Timestamp = time.Now().UnixMilli()
	}

	// 6. 고객에게 보낼 메시지 직렬화 — omitempty로 종류에 맞는 필드만 포함.
	return Result{Outbound: marshalControlEvent(evt)}
}

// marshalControlEvent는 control-event 한 건을
// {"type":"control-event","payload":{...}} 봉투로 직렬화한다.
func marshalControlEvent(evt ControlEventPayload) []byte {
	envelope := struct {
		Type    string              `json:"type"`
		Payload ControlEventPayload `json:"payload"`
	}{
		Type:    "control-event",
		Payload: evt,
	}

	// envelope는 항상 직렬화 가능한 형태이므로 에러를 무시해도 안전하다.
	out, _ := json.Marshal(envelope)
	return out
}
