package relay

import (
	"encoding/json"
	"testing"
	"time"

	"co-browsing-session-server/internal/services/hub"
)

// newCustomerPeer는 같은 룸에 접속해 있는 고객 peer 하나를 만든다.
// HandleControlEvent의 전달 대상(상대=고객)으로 쓰인다.
func newCustomerPeer() *hub.Client {
	return &hub.Client{Role: hub.RoleCustomer, RoomID: "room-1"}
}

// outboundPayload는 Outbound 바이트({"type":"control-event","payload":{...}})에서
// payload 객체만 디코드해 돌려준다. 관찰 가능한 wire 포맷만 검증하기 위함이다.
func outboundPayload(t *testing.T, outbound []byte) map[string]any {
	t.Helper()

	var envelope struct {
		Type    string          `json:"type"`
		Payload json.RawMessage `json:"payload"`
	}
	if err := json.Unmarshal(outbound, &envelope); err != nil {
		t.Fatalf("Outbound 봉투 디코드 실패: %v (raw=%s)", err, outbound)
	}
	if envelope.Type != "control-event" {
		t.Fatalf("Outbound 봉투 type은 control-event여야 한다, got %q", envelope.Type)
	}

	var payload map[string]any
	if err := json.Unmarshal(envelope.Payload, &payload); err != nil {
		t.Fatalf("Outbound payload 디코드 실패: %v", err)
	}
	return payload
}

func mustRaw(t *testing.T, v any) json.RawMessage {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("payload marshal 실패: %v", err)
	}
	return raw
}

// AC-1, AC-7: 상담원이 위치 포함 click 전송 → Rejected=false, Outbound payload에 동일 x/y 포함.
func TestHandleControlEvent_AgentClickRelaysPosition(t *testing.T) {
	t.Parallel()

	raw := mustRaw(t, map[string]any{
		"type":      "click",
		"x":         320,
		"y":         240,
		"timestamp": 1716000000000,
	})

	result := HandleControlEvent(hub.RoleAgent, newCustomerPeer(), raw)

	if result.Rejected {
		t.Fatalf("정상 click은 거부되면 안 된다, got Code=%q Message=%q", result.Code, result.Message)
	}

	payload := outboundPayload(t, result.Outbound)
	if got := payload["type"]; got != "click" {
		t.Fatalf("payload type은 click이어야 한다, got %v", got)
	}
	if got := payload["x"]; got != float64(320) {
		t.Fatalf("payload x는 320이어야 한다, got %v", got)
	}
	if got := payload["y"]; got != float64(240) {
		t.Fatalf("payload y는 240이어야 한다, got %v", got)
	}
}

// AC-2: timestamp 0(미설정)으로 전송 → Outbound timestamp가 현재 시각(Unix ms)으로 보완.
func TestHandleControlEvent_FillsMissingTimestamp(t *testing.T) {
	t.Parallel()

	raw := mustRaw(t, map[string]any{
		"type":      "click",
		"x":         10,
		"y":         20,
		"timestamp": 0,
	})

	before := time.Now().UnixMilli()
	result := HandleControlEvent(hub.RoleAgent, newCustomerPeer(), raw)
	after := time.Now().UnixMilli()

	if result.Rejected {
		t.Fatalf("정상 이벤트는 거부되면 안 된다, got Code=%q", result.Code)
	}

	payload := outboundPayload(t, result.Outbound)
	tsRaw, ok := payload["timestamp"]
	if !ok {
		t.Fatalf("보완된 timestamp가 payload에 있어야 한다, payload=%v", payload)
	}
	ts := int64(tsRaw.(float64))
	if ts == 0 {
		t.Fatalf("timestamp 0은 서버 시각으로 보완되어야 한다, got 0")
	}
	if ts < before || ts > after {
		t.Fatalf("보완된 timestamp는 호출 전후 범위 안이어야 한다 [%d, %d], got %d", before, after, ts)
	}
}

// AC-3: timestamp 지정해 전송 → Outbound timestamp가 입력값과 동일(보존, 덮어쓰지 않음).
func TestHandleControlEvent_PreservesProvidedTimestamp(t *testing.T) {
	t.Parallel()

	const provided int64 = 1716000000123

	raw := mustRaw(t, map[string]any{
		"type":      "click",
		"x":         1,
		"y":         2,
		"timestamp": provided,
	})

	result := HandleControlEvent(hub.RoleAgent, newCustomerPeer(), raw)
	if result.Rejected {
		t.Fatalf("정상 이벤트는 거부되면 안 된다, got Code=%q", result.Code)
	}

	payload := outboundPayload(t, result.Outbound)
	ts := int64(payload["timestamp"].(float64))
	if ts != provided {
		t.Fatalf("지정한 timestamp는 보존되어야 한다, want %d got %d", provided, ts)
	}
}

// AC-4, AC-8: 발신자=고객 → Rejected=true, Code=FORBIDDEN (중계 안 함).
func TestHandleControlEvent_CustomerSenderForbidden(t *testing.T) {
	t.Parallel()

	raw := mustRaw(t, map[string]any{
		"type":      "click",
		"x":         5,
		"y":         5,
		"timestamp": 1716000000000,
	})

	// peer로 상담원을 넘겨도, 발신자가 고객이면 권한 거부가 먼저 적용되어야 한다.
	agentPeer := &hub.Client{Role: hub.RoleAgent, RoomID: "room-1"}
	result := HandleControlEvent(hub.RoleCustomer, agentPeer, raw)

	if !result.Rejected {
		t.Fatalf("고객 발신은 거부되어야 한다")
	}
	if result.Code != RejectForbidden {
		t.Fatalf("고객 발신 거부 코드는 FORBIDDEN이어야 한다, got %q", result.Code)
	}
	if len(result.Outbound) != 0 {
		t.Fatalf("거부 시 Outbound는 비어 있어야 한다, got %s", result.Outbound)
	}
}

// AC-5, AC-8: type="hover"(미허용) → Rejected=true, Code=INVALID_EVENT_TYPE.
func TestHandleControlEvent_DisallowedTypeInvalid(t *testing.T) {
	t.Parallel()

	raw := mustRaw(t, map[string]any{
		"type":      "hover",
		"x":         5,
		"y":         5,
		"timestamp": 1716000000000,
	})

	result := HandleControlEvent(hub.RoleAgent, newCustomerPeer(), raw)

	if !result.Rejected {
		t.Fatalf("미허용 종류는 거부되어야 한다")
	}
	if result.Code != RejectInvalidType {
		t.Fatalf("미허용 종류 거부 코드는 INVALID_EVENT_TYPE이어야 한다, got %q", result.Code)
	}
	if len(result.Outbound) != 0 {
		t.Fatalf("거부 시 Outbound는 비어 있어야 한다, got %s", result.Outbound)
	}
}

// AC-6, AC-8: peer==nil 또는 peer가 고객이 아님 → Rejected=true, Code=PEER_NOT_CONNECTED.
func TestHandleControlEvent_PeerNotConnected(t *testing.T) {
	t.Parallel()

	raw := mustRaw(t, map[string]any{
		"type":      "click",
		"x":         5,
		"y":         5,
		"timestamp": 1716000000000,
	})

	tests := map[string]*hub.Client{
		"peer가 nil":    nil,
		"peer가 고객이 아님": {Role: hub.RoleAgent, RoomID: "room-1"},
	}

	for name, peer := range tests {
		peer := peer
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			result := HandleControlEvent(hub.RoleAgent, peer, raw)

			if !result.Rejected {
				t.Fatalf("고객 미접속은 거부되어야 한다")
			}
			if result.Code != RejectPeerNotConnected {
				t.Fatalf("거부 코드는 PEER_NOT_CONNECTED여야 한다, got %q", result.Code)
			}
			if len(result.Outbound) != 0 {
				t.Fatalf("거부 시 Outbound는 비어 있어야 한다, got %s", result.Outbound)
			}
		})
	}
}

// AC-7: scroll(deltaY)·keydown(key) 각각 통과 + 종류에 맞는 필드만 포함, 무관 필드는 omitempty로 제외.
func TestHandleControlEvent_TypeSpecificFields(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		input       map[string]any
		wantPresent []string
		wantAbsent  []string
	}{
		"click은 x/y만": {
			input: map[string]any{
				"type": "click", "x": 100, "y": 200, "timestamp": 1716000000000,
			},
			wantPresent: []string{"type", "x", "y", "timestamp"},
			wantAbsent:  []string{"key", "deltaY"},
		},
		"scroll은 deltaY만": {
			input: map[string]any{
				"type": "scroll", "deltaY": 120, "timestamp": 1716000000000,
			},
			wantPresent: []string{"type", "deltaY", "timestamp"},
			wantAbsent:  []string{"key"},
		},
		"keydown은 key만": {
			input: map[string]any{
				"type": "keydown", "key": "Enter", "timestamp": 1716000000000,
			},
			wantPresent: []string{"type", "key", "timestamp"},
			wantAbsent:  []string{"x", "y", "deltaY"},
		},
	}

	for name, tc := range tests {
		tc := tc
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			result := HandleControlEvent(hub.RoleAgent, newCustomerPeer(), mustRaw(t, tc.input))
			if result.Rejected {
				t.Fatalf("허용 종류는 거부되면 안 된다, got Code=%q", result.Code)
			}

			payload := outboundPayload(t, result.Outbound)
			for _, key := range tc.wantPresent {
				if _, ok := payload[key]; !ok {
					t.Fatalf("payload에 %q가 포함되어야 한다, payload=%v", key, payload)
				}
			}
			for _, key := range tc.wantAbsent {
				if _, ok := payload[key]; ok {
					t.Fatalf("payload에 %q는 omitempty로 제외되어야 한다, payload=%v", key, payload)
				}
			}
		})
	}
}

// 설계 Test Plan: 잘못된 JSON payload → INVALID_EVENT_TYPE로 거부, 패닉 없음.
func TestHandleControlEvent_MalformedJSONRejected(t *testing.T) {
	t.Parallel()

	raw := json.RawMessage(`{"type": "click", "x":`) // 잘린 JSON

	result := HandleControlEvent(hub.RoleAgent, newCustomerPeer(), raw)

	if !result.Rejected {
		t.Fatalf("잘못된 JSON은 거부되어야 한다")
	}
	if result.Code != RejectInvalidType {
		t.Fatalf("잘못된 JSON 거부 코드는 INVALID_EVENT_TYPE이어야 한다, got %q", result.Code)
	}
	if len(result.Outbound) != 0 {
		t.Fatalf("거부 시 Outbound는 비어 있어야 한다, got %s", result.Outbound)
	}
}
