package ws

import (
	"encoding/json"
	"errors"
	"log"
	nethttp "net/http"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"

	"co-browsing-session-server/internal/services/connection"
	"co-browsing-session-server/internal/services/hub"
	"co-browsing-session-server/internal/services/relay"
	"co-browsing-session-server/internal/services/signaling"
)

// upgrader는 HTTP 연결을 WebSocket으로 승격한다.
// Origin 검증은 Step 8의 CORS 미들웨어가 담당하므로 여기서는 모두 허용한다.
var upgrader = websocket.Upgrader{
	CheckOrigin: func(_ *nethttp.Request) bool { return true },
}

// Handler는 GET /ws 엔드포인트를 담당하는 트랜스포트 어댑터다.
// 업그레이드, 바이트 ↔ DTO 직렬화, read/write 펌프만 책임지고
// 연결 수립/해제의 유즈케이스 흐름은 connection.Coordinator에 위임한다.
type Handler struct {
	coordinator *connection.Coordinator
}

func NewHandler(coordinator *connection.Coordinator) *Handler {
	return &Handler{coordinator: coordinator}
}

// Register는 gin 엔진에 직접 라우트를 등록한다.
// WebSocket 업그레이드는 raw http.ResponseWriter가 필요해 huma 타입드 핸들러를 통과할 수 없다.
func (handler *Handler) Register(engine *gin.Engine) {
	engine.GET("/ws", handler.handleUpgrade)
}

func (handler *Handler) handleUpgrade(ginContext *gin.Context) {
	serial := ginContext.Query("serial")
	role := ginContext.Query("role")
	if serial == "" || role == "" {
		ginContext.JSON(nethttp.StatusBadRequest, gin.H{"error": "missing serial or role parameter"})
		return
	}
	if role != string(hub.RoleCustomer) && role != string(hub.RoleAgent) {
		ginContext.JSON(nethttp.StatusBadRequest, gin.H{"error": "role must be customer or agent"})
		return
	}

	target, err := handler.coordinator.Resolve(serial)
	if err != nil {
		switch {
		case errors.Is(err, connection.ErrInvitationInvalid):
			ginContext.JSON(nethttp.StatusNotFound, gin.H{"error": "invitation not found or expired"})
		case errors.Is(err, connection.ErrSessionEnded):
			ginContext.JSON(nethttp.StatusGone, gin.H{"error": "session already ended"})
		default:
			ginContext.JSON(nethttp.StatusInternalServerError, gin.H{"error": "failed to establish session"})
		}
		return
	}

	conn, err := upgrader.Upgrade(ginContext.Writer, ginContext.Request, nil)
	if err != nil {
		// 업그레이드 실패 시 gorilla가 응답을 이미 처리했다.
		return
	}

	client := &hub.Client{
		Conn:   conn,
		Role:   hub.Role(role),
		RoomID: target.RoomID,
		Serial: target.Serial,
		Send:   make(chan []byte, 256),
	}

	peer, err := handler.coordinator.Join(client)
	if err != nil {
		log.Printf("join room %s: %v", client.RoomID, err)
	}
	if peer != nil {
		// 양쪽이 모두 모인 첫 순간 — 대기 중이던 상대에게 알린다.
		trySend(peer, marshalMessage(msgTypePeerJoined, nil))
	}

	done := make(chan struct{})
	go handler.writePump(client, done)
	go handler.readPump(client, done)
}

// readPump는 WebSocket 읽기 루프다. 종료 시 cleanup으로 정리한다.
func (handler *Handler) readPump(client *hub.Client, done chan struct{}) {
	defer handler.cleanup(client, done)

	for {
		_, rawMessage, err := client.Conn.ReadMessage()
		if err != nil {
			return
		}

		var incoming Message
		if err := json.Unmarshal(rawMessage, &incoming); err != nil {
			sendError(client, "MALFORMED", "invalid message format")
			continue
		}

		switch incoming.Type {
		case msgTypeOffer, msgTypeAnswer, msgTypeICECandidate:
			// 시그널링 중계: 역할 검증·peer 미접속 안내·원문 송신은 모두 서비스가 수행한다.
			// peer는 nil일 수 있고(상대 미접속), 서비스가 PEER_NOT_CONNECTED로 처리한다.
			peer := handler.coordinator.Peer(client)
			_ = signaling.HandleSignalingMessage(client, peer, incoming.Type, rawMessage)
		case msgTypeControlEvent:
			// 제어 이벤트 중계: 검증·타임스탬프 보완·직렬화는 서비스가 결정하고,
			// 채널 송신(거부 회신 / 고객 전달)만 트랜스포트가 수행한다.
			peer := handler.coordinator.Peer(client)
			result := relay.HandleControlEvent(client.Role, peer, incoming.Payload)
			if result.Rejected {
				sendError(client, string(result.Code), result.Message)
				continue
			}
			trySend(peer, result.Outbound)
		case msgTypeLeave:
			return
		default:
			sendError(client, "UNKNOWN_TYPE", "unknown message type: "+incoming.Type)
		}
	}
}

// writePump는 WebSocket 쓰기 루프다. Send 채널을 소비해 전송하며,
// done이 닫히면(상대 정리/연결 종료) 루프를 종료한다.
func (handler *Handler) writePump(client *hub.Client, done chan struct{}) {
	defer func() { _ = client.Conn.Close() }()

	for {
		select {
		case message := <-client.Send:
			if err := client.Conn.WriteMessage(websocket.TextMessage, message); err != nil {
				return
			}
		case <-done:
			return
		}
	}
}

// cleanup은 disconnect 처리다. Hub에서 제거하고 세션을 종료한 뒤(coordinator),
// 첫 disconnect라면 상대에게 peer-left를 알린다.
func (handler *Handler) cleanup(client *hub.Client, done chan struct{}) {
	close(done)
	peer, err := handler.coordinator.Leave(client)
	_ = client.Conn.Close()

	if err != nil {
		log.Printf("leave room %s: %v", client.RoomID, err)
	}
	if peer != nil {
		trySend(peer, marshalMessage(msgTypePeerLeft, nil))
	}
}

// trySend는 클라이언트의 Send 채널에 비차단으로 메시지를 넣는다.
// 채널이 가득 찬 슬로우 클라이언트는 메시지를 드롭하지 않고 연결을 종료한다.
func trySend(client *hub.Client, message []byte) {
	select {
	case client.Send <- message:
	default:
		_ = client.Conn.Close()
	}
}

// sendError는 발신 클라이언트에게 error 메시지를 push한다.
func sendError(client *hub.Client, code, message string) {
	trySend(client, marshalMessage(msgTypeError, ErrorPayload{Code: code, Message: message}))
}
