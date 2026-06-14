// Package signaling은 WebRTC 시그널링(offer/answer/ice-candidate)을
// 같은 LiveRoom에 합류한 고객과 상담원 사이에서 변형 없이 중계하는 유즈케이스다.
// 서버는 통로(transparent relay) 역할만 하며 페이로드를 재파싱·재직렬화하지 않는다.
package signaling

import (
	"encoding/json"

	"co-browsing-session-server/internal/services/hub"
)

// 시그널링 메시지 타입. 이 패키지가 단일 진실 출처로 export하며,
// interfaces(ws)의 readPump 디스패치가 매직 스트링 대신 이 상수를 참조한다.
const (
	MsgTypeOffer        = "offer"
	MsgTypeAnswer       = "answer"
	MsgTypeICECandidate = "ice-candidate"
)

// HandleSignalingMessage는 readPump에서 호출되는 시그널링 메시지 처리 함수다.
//
//	client:   메시지를 보낸 주체
//	peer:     상대방 클라이언트 (nil이면 상대방 미접속)
//	msgType:  파싱된 메시지 타입 (offer | answer | ice-candidate)
//	rawBytes: 원본 WS 메시지 바이트 (SDP/ICE 원문 보존을 위해 재직렬화하지 않는다)
//
// 역할 검증·peer 미접속 등 거부 안내는 발신자(client.Send)에게만 push하고
// 상대방의 채널에는 아무것도 넣지 않는다. 정상이면 rawBytes를 byte-for-byte
// 그대로 peer.Send에 넣는다.
func HandleSignalingMessage(client *hub.Client, peer *hub.Client, msgType string, rawBytes []byte) error {
	if peer == nil {
		trySend(client, marshalError("PEER_NOT_CONNECTED", "peer is not connected yet"))
		return nil
	}

	switch msgType {
	case MsgTypeOffer:
		if client.Role != hub.RoleCustomer {
			trySend(client, marshalError("INVALID_SENDER", "offer must be sent by customer"))
			return nil
		}
		trySend(peer, rawBytes)

	case MsgTypeAnswer:
		if client.Role != hub.RoleAgent {
			trySend(client, marshalError("INVALID_SENDER", "answer must be sent by agent"))
			return nil
		}
		trySend(peer, rawBytes)

	case MsgTypeICECandidate:
		// 발신자 역할 무관, 상대방에게 원문 그대로 전달.
		trySend(peer, rawBytes)
	}
	return nil
}

// trySend는 클라이언트의 Send 채널에 비차단으로 메시지를 넣는다.
// 채널이 가득 차면 드롭한다 — 연결 종료(Conn.Close) 같은 트랜스포트 동작은
// interfaces(writePump)의 책임이며, services는 채널만 다룬다.
func trySend(client *hub.Client, message []byte) {
	select {
	case client.Send <- message:
	default:
	}
}

// marshalError는 error WS 메시지를 바이트로 직렬화한다.
// {"type":"error","payload":{"code":...,"message":...}} 형태이며,
// interfaces/ws의 marshalMessage와는 의존 방향 보존을 위해 별개로 구현한다.
func marshalError(code, message string) []byte {
	type errorPayload struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	}
	type errorMessage struct {
		Type    string       `json:"type"`
		Payload errorPayload `json:"payload"`
	}
	encoded, _ := json.Marshal(errorMessage{
		Type:    "error",
		Payload: errorPayload{Code: code, Message: message},
	})
	return encoded
}
