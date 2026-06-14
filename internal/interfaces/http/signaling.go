package http

import "encoding/json"

// Message는 모든 WebSocket 메시지의 공통 래퍼다.
// 프로토콜 페이로드는 adapter(interfaces) 레이어의 DTO로만 다루고 도메인 타입과 섞지 않는다.
type Message struct {
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload,omitempty"`
}

// ErrorPayload는 서버 → 클라이언트 error 메시지의 페이로드다.
type ErrorPayload struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// WebSocket 메시지 타입 상수.
// 시그널링(offer/answer/ice-candidate)과 control-event는 Step 5/6에서 전용 로직으로 다뤄지며,
// 현재는 핸들러가 raw 바이트를 상대방에게 그대로 전달(passthrough)한다.
const (
	msgTypeOffer        = "offer"
	msgTypeAnswer       = "answer"
	msgTypeICECandidate = "ice-candidate"
	msgTypeControlEvent = "control-event"
	msgTypeLeave        = "leave"
	msgTypePeerJoined   = "peer-joined"
	msgTypePeerLeft     = "peer-left"
	msgTypeError        = "error"
)
