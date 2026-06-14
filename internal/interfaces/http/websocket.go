package http

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

// upgrader는 HTTP 연결을 WebSocket으로 승격한다.
// Origin 검증은 Step 8의 CORS 미들웨어가 담당하므로 여기서는 모두 허용한다.
var upgrader = websocket.Upgrader{
	CheckOrigin: func(_ *nethttp.Request) bool { return true },
}

// WebSocketHandler는 GET /ws 엔드포인트를 담당한다.
// 시리얼을 Invitation으로 풀어 RoomID를 얻고, WebSocket으로 승격한 뒤
// Hub에 클라이언트를 등록하고 read/write 루프를 실행한다.
type WebSocketHandler struct {
	hub                  *hub.Hub
	invitationRepository invitation.Repository
	roomSessionService   *rssvc.Service
}

func NewWebSocketHandler(
	roomHub *hub.Hub,
	invitationRepository invitation.Repository,
	roomSessionService *rssvc.Service,
) *WebSocketHandler {
	return &WebSocketHandler{
		hub:                  roomHub,
		invitationRepository: invitationRepository,
		roomSessionService:   roomSessionService,
	}
}

// Register는 gin 엔진에 직접 라우트를 등록한다.
// WebSocket 업그레이드는 raw http.ResponseWriter가 필요해 huma 타입드 핸들러를 통과할 수 없다.
func (handler *WebSocketHandler) Register(engine *gin.Engine) {
	engine.GET("/ws", handler.handleUpgrade)
}

func (handler *WebSocketHandler) handleUpgrade(ginContext *gin.Context) {
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

	resolvedInvitation, err := handler.invitationRepository.ResolveBySerial(serialnumber.SerialNumber(serial))
	if err != nil {
		if errors.Is(err, invitation.ErrNotFound) || errors.Is(err, invitation.ErrExpired) {
			ginContext.JSON(nethttp.StatusNotFound, gin.H{"error": "invitation not found or expired"})
			return
		}
		ginContext.JSON(nethttp.StatusInternalServerError, gin.H{"error": "failed to resolve invitation"})
		return
	}

	// 불필요한 업그레이드를 막는 가벼운 사전 검증.
	roomSession, err := handler.roomSessionService.Get(resolvedInvitation.RoomID)
	if err != nil {
		if errors.Is(err, roomsession.ErrNotFound) || errors.Is(err, roomsession.ErrExpired) {
			ginContext.JSON(nethttp.StatusNotFound, gin.H{"error": "room session not found or expired"})
			return
		}
		ginContext.JSON(nethttp.StatusInternalServerError, gin.H{"error": "failed to load room session"})
		return
	}
	if roomSession.Status == roomsession.StatusEnded {
		ginContext.JSON(nethttp.StatusGone, gin.H{"error": "session already ended"})
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
		RoomID: resolvedInvitation.RoomID.String(),
		Serial: resolvedInvitation.Serial.String(),
		Send:   make(chan []byte, 256),
	}

	peer := handler.hub.JoinRoom(client.RoomID, client)
	if peer != nil {
		// 양쪽이 모두 모인 첫 순간 — 세션을 active로 전이하고 대기 중이던 상대에게 알린다.
		if err := handler.roomSessionService.Activate(resolvedInvitation.RoomID); err != nil {
			log.Printf("activate room session %s: %v", resolvedInvitation.RoomID, err)
		}
		trySend(peer, marshalMessage(msgTypePeerJoined, nil))
	}

	done := make(chan struct{})
	go handler.writePump(client, done)
	go handler.readPump(client, done)
}

// readPump는 WebSocket 읽기 루프다. 종료 시 cleanup으로 정리한다.
func (handler *WebSocketHandler) readPump(client *hub.Client, done chan struct{}) {
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
		case msgTypeOffer, msgTypeAnswer, msgTypeICECandidate, msgTypeControlEvent:
			// TODO(step5/6): role 검증 + 전용 signaling/relay 패키지로 교체.
			// 현재는 시그널링/제어 메시지를 상대방에게 raw 바이트 그대로 전달한다.
			if peer := handler.hub.GetPeer(client); peer != nil {
				trySend(peer, rawMessage)
			}
		case msgTypeLeave:
			return
		default:
			sendError(client, "UNKNOWN_TYPE", "unknown message type: "+incoming.Type)
		}
	}
}

// writePump는 WebSocket 쓰기 루프다. Send 채널을 소비해 전송하며,
// done이 닫히면(상대 정리/연결 종료) 루프를 종료한다.
func (handler *WebSocketHandler) writePump(client *hub.Client, done chan struct{}) {
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

// cleanup은 disconnect 처리다. Hub에서 제거하고, 첫 disconnect라면 세션을 종료하고
// 상대에게 peer-left를 알린다.
func (handler *WebSocketHandler) cleanup(client *hub.Client, done chan struct{}) {
	close(done)
	peer := handler.hub.LeaveRoom(client)
	_ = client.Conn.Close()

	if peer == nil {
		return
	}

	if err := handler.roomSessionService.End(
		roomsession.RoomID(client.RoomID),
		serialnumber.SerialNumber(client.Serial),
	); err != nil {
		log.Printf("end session %s: %v", client.RoomID, err)
	}
	trySend(peer, marshalMessage(msgTypePeerLeft, nil))
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

// marshalMessage는 타입과 페이로드를 {"type":...,"payload":...} JSON 바이트로 직렬화한다.
// payload가 nil이면 payload 필드는 생략된다.
func marshalMessage(messageType string, payload any) []byte {
	message := Message{Type: messageType}
	if payload != nil {
		encodedPayload, err := json.Marshal(payload)
		if err != nil {
			encodedPayload = []byte("null")
		}
		message.Payload = encodedPayload
	}
	encoded, _ := json.Marshal(message)
	return encoded
}
