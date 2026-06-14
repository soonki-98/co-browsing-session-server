package signaling

import (
	"encoding/json"
	"testing"
	"time"

	"co-browsing-session-server/internal/services/hub"
)

// newClient는 WS 커넥션 없이 Send 채널만 채운 경량 hub.Client 더블을 만든다.
// 시그널링 서비스는 채널 주입까지만 책임지므로(설계: services 책임), 채널 내용만 관찰하면 된다.
func newClient(role hub.Role) *hub.Client {
	return &hub.Client{
		Role: role,
		Send: make(chan []byte, 8), // 버퍼드: 송신이 블록되지 않게 한다.
	}
}

// recv는 ch에서 한 메시지를 짧은 타임아웃으로 읽는다. 비면 nil, false.
func recv(t *testing.T, ch chan []byte) ([]byte, bool) {
	t.Helper()
	select {
	case msg := <-ch:
		return msg, true
	case <-time.After(100 * time.Millisecond):
		return nil, false
	}
}

// drained는 ch에 아무 메시지도 들어오지 않았음을 확인한다(상대방 무수신 검증용).
func drained(t *testing.T, ch chan []byte) bool {
	t.Helper()
	select {
	case msg := <-ch:
		t.Logf("예상치 못한 수신: %q", msg)
		return false
	case <-time.After(50 * time.Millisecond):
		return true
	}
}

// errCode는 error WS 봉투 {"type":"error","payload":{"code":...}}에서 code를 뽑는다.
func errCode(t *testing.T, raw []byte) string {
	t.Helper()
	var envelope struct {
		Type    string `json:"type"`
		Payload struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"payload"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		t.Fatalf("error 메시지 언마샬 실패: %v (raw=%q)", err, raw)
	}
	if envelope.Type != "error" {
		t.Fatalf("error 메시지의 type은 %q여야 한다, got %q", "error", envelope.Type)
	}
	return envelope.Payload.Code
}

// TestHandleSignalingMessage_RelaysToPeer는 정상 경로에서 원본 바이트가
// 상대방의 Send 채널로 byte-for-byte 그대로 전달되는지 검증한다.
// AC-1(고객 offer→상담원), AC-2(상담원 answer→고객), AC-3(ice 양방향), AC-7(무변형).
func TestHandleSignalingMessage_RelaysToPeer(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name       string
		senderRole hub.Role
		peerRole   hub.Role
		msgType    string
		raw        []byte
		ac         string
	}{
		{
			name:       "offer는 고객이 보내면 상담원이 받는다",
			senderRole: hub.RoleCustomer,
			peerRole:   hub.RoleAgent,
			msgType:    MsgTypeOffer,
			raw:        []byte(`{"type":"offer","payload":{"sdp":"v=0\r\no=- 1 1 IN IP4 0.0.0.0\r\n"}}`),
			ac:         "AC-1",
		},
		{
			name:       "answer는 상담원이 보내면 고객이 받는다",
			senderRole: hub.RoleAgent,
			peerRole:   hub.RoleCustomer,
			msgType:    MsgTypeAnswer,
			raw:        []byte(`{"type":"answer","payload":{"sdp":"v=0\r\na=recvonly\r\n"}}`),
			ac:         "AC-2",
		},
		{
			name:       "ice-candidate는 고객이 보내면 상담원이 받는다",
			senderRole: hub.RoleCustomer,
			peerRole:   hub.RoleAgent,
			msgType:    MsgTypeICECandidate,
			raw:        []byte(`{"type":"ice-candidate","payload":{"candidate":"candidate:1 1 UDP 1 1.2.3.4 5 typ host"}}`),
			ac:         "AC-3",
		},
		{
			name:       "ice-candidate는 상담원이 보내면 고객이 받는다",
			senderRole: hub.RoleAgent,
			peerRole:   hub.RoleCustomer,
			msgType:    MsgTypeICECandidate,
			raw:        []byte(`{"type":"ice-candidate","payload":{"candidate":"candidate:2 1 UDP 2 5.6.7.8 9 typ srflx"}}`),
			ac:         "AC-3",
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			client := newClient(tc.senderRole)
			peer := newClient(tc.peerRole)

			if err := HandleSignalingMessage(client, peer, tc.msgType, tc.raw); err != nil {
				t.Fatalf("[%s] HandleSignalingMessage가 에러를 반환했다: %v", tc.ac, err)
			}

			got, ok := recv(t, peer.Send)
			if !ok {
				t.Fatalf("[%s] 상대방이 중계 메시지를 받지 못했다", tc.ac)
			}
			if string(got) != string(tc.raw) {
				t.Fatalf("[%s] 상대방이 받은 바이트가 원본과 다르다\n원본: %q\n수신: %q", tc.ac, tc.raw, got)
			}

			// 발신자 자신에게는 아무것도 돌아가지 않아야 한다(정상 중계 시 에러 없음).
			if !drained(t, client.Send) {
				t.Fatalf("[%s] 정상 중계 시 발신자에게 메시지가 돌아가서는 안 된다", tc.ac)
			}
		})
	}
}

// TestHandleSignalingMessage_PreservesRawBytes는 비표준 공백·키 순서가 섞인
// SDP raw가 한 바이트도 변하지 않고 전달되는지 검증한다. AC-7(무변형).
func TestHandleSignalingMessage_PreservesRawBytes(t *testing.T) {
	t.Parallel()

	// 일부러 비표준: 키 순서(payload 먼저), 들여쓰기 공백, 줄바꿈, 후행 공백.
	raw := []byte("{\n  \"payload\" : {\n\t\"sdp\":\"v=0\\r\\n   a=group:BUNDLE 0  \\r\\n\"}  ,\n \"type\":\"offer\"\n}   ")

	client := newClient(hub.RoleCustomer)
	peer := newClient(hub.RoleAgent)

	if err := HandleSignalingMessage(client, peer, MsgTypeOffer, raw); err != nil {
		t.Fatalf("[AC-7] HandleSignalingMessage가 에러를 반환했다: %v", err)
	}

	got, ok := recv(t, peer.Send)
	if !ok {
		t.Fatalf("[AC-7] 상대방이 중계 메시지를 받지 못했다")
	}
	if len(got) != len(raw) {
		t.Fatalf("[AC-7] 길이가 달라졌다: 원본 %d바이트, 수신 %d바이트", len(raw), len(got))
	}
	for i := range raw {
		if got[i] != raw[i] {
			t.Fatalf("[AC-7] %d번째 바이트가 변형됐다: 원본 %#x, 수신 %#x", i, raw[i], got[i])
		}
	}
}

// TestHandleSignalingMessage_PeerNotConnected는 peer가 nil일 때
// 중계하지 않고 발신자에게만 PEER_NOT_CONNECTED를 push하는지 검증한다.
// AC-4(상대 미접속 offer), AC-8(상대 무수신).
func TestHandleSignalingMessage_PeerNotConnected(t *testing.T) {
	t.Parallel()

	msgTypes := []struct {
		name    string
		role    hub.Role
		msgType string
	}{
		{"offer", hub.RoleCustomer, MsgTypeOffer},
		{"answer", hub.RoleAgent, MsgTypeAnswer},
		{"ice-candidate", hub.RoleCustomer, MsgTypeICECandidate},
	}

	for _, tc := range msgTypes {
		tc := tc
		t.Run(tc.name+" with nil peer", func(t *testing.T) {
			t.Parallel()

			client := newClient(tc.role)
			raw := []byte(`{"type":"` + tc.msgType + `","payload":{}}`)

			if err := HandleSignalingMessage(client, nil, tc.msgType, raw); err != nil {
				t.Fatalf("[AC-4] HandleSignalingMessage가 에러를 반환했다: %v", err)
			}

			got, ok := recv(t, client.Send)
			if !ok {
				t.Fatalf("[AC-4] 발신자가 PEER_NOT_CONNECTED 안내를 받지 못했다")
			}
			if code := errCode(t, got); code != "PEER_NOT_CONNECTED" {
				t.Fatalf("[AC-4] 에러 code는 %q여야 한다, got %q", "PEER_NOT_CONNECTED", code)
			}
		})
	}
}

// TestHandleSignalingMessage_InvalidSender는 역할에 맞지 않는 발신을
// 중계하지 않고 발신자에게만 INVALID_SENDER를 push하며,
// 상대방은 아무것도 받지 않는지 검증한다. AC-5, AC-6, AC-8.
func TestHandleSignalingMessage_InvalidSender(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name       string
		senderRole hub.Role
		peerRole   hub.Role
		msgType    string
		ac         string
	}{
		{
			name:       "상담원이 offer를 보내면 INVALID_SENDER",
			senderRole: hub.RoleAgent,
			peerRole:   hub.RoleCustomer,
			msgType:    MsgTypeOffer,
			ac:         "AC-5",
		},
		{
			name:       "고객이 answer를 보내면 INVALID_SENDER",
			senderRole: hub.RoleCustomer,
			peerRole:   hub.RoleAgent,
			msgType:    MsgTypeAnswer,
			ac:         "AC-6",
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			client := newClient(tc.senderRole)
			peer := newClient(tc.peerRole)
			raw := []byte(`{"type":"` + tc.msgType + `","payload":{"sdp":"v=0"}}`)

			if err := HandleSignalingMessage(client, peer, tc.msgType, raw); err != nil {
				t.Fatalf("[%s] HandleSignalingMessage가 에러를 반환했다: %v", tc.ac, err)
			}

			// 발신자에게만 INVALID_SENDER 안내.
			got, ok := recv(t, client.Send)
			if !ok {
				t.Fatalf("[%s] 발신자가 INVALID_SENDER 안내를 받지 못했다", tc.ac)
			}
			if code := errCode(t, got); code != "INVALID_SENDER" {
				t.Fatalf("[%s] 에러 code는 %q여야 한다, got %q", tc.ac, "INVALID_SENDER", code)
			}

			// 상대방은 아무것도 받지 않아야 한다(AC-8).
			if !drained(t, peer.Send) {
				t.Fatalf("[%s/AC-8] 거부 시 상대방은 어떤 메시지도 받아서는 안 된다", tc.ac)
			}
		})
	}
}
