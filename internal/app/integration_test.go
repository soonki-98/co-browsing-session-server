package app_test

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"

	"co-browsing-session-server/internal/app"
)

// 통합 테스트: POST /rooms 로 발급한 시리얼로 고객/상담사가 GET /ws에 접속하고,
// peer-joined / leave / peer-left / 세션 종료(invitation 삭제)까지의 end-to-end 경로를 검증한다.

func newTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	gin.SetMode(gin.TestMode)
	server := httptest.NewServer(app.New().Engine())
	t.Cleanup(server.Close)
	return server
}

// createRoom은 POST /rooms를 호출해 발급된 시리얼을 반환한다.
func createRoom(t *testing.T, server *httptest.Server) string {
	t.Helper()
	response, err := http.Post(server.URL+"/rooms", "application/json", nil)
	if err != nil {
		t.Fatalf("POST /rooms: %v", err)
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("POST /rooms status = %d, want 200", response.StatusCode)
	}

	var body struct {
		Data struct {
			SerialNumber string `json:"serial_number"`
		} `json:"data"`
	}
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatalf("decode /rooms response: %v", err)
	}
	if body.Data.SerialNumber == "" {
		t.Fatalf("발급된 serial_number가 비어 있다")
	}
	return body.Data.SerialNumber
}

// dialWS는 주어진 시리얼/역할로 WebSocket에 접속한다.
func dialWS(t *testing.T, server *httptest.Server, serial, role string) (*websocket.Conn, *http.Response, error) {
	t.Helper()
	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	return websocket.DefaultDialer.Dial(wsURL+"/ws?serial="+serial+"&role="+role, nil)
}

// readMessageType은 다음 수신 메시지의 type을 읽는다(읽기 타임아웃 포함).
func readMessageType(t *testing.T, conn *websocket.Conn) string {
	t.Helper()
	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, raw, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("read message: %v", err)
	}
	var message struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(raw, &message); err != nil {
		t.Fatalf("unmarshal message: %v", err)
	}
	return message.Type
}

// readMessage는 다음 수신 메시지의 원본 바이트를 반환한다(읽기 타임아웃 포함).
func readMessage(t *testing.T, conn *websocket.Conn) []byte {
	t.Helper()
	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, raw, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("read message: %v", err)
	}
	return raw
}

// readErrorCode는 다음 수신 메시지가 error 타입이라고 보고 payload.code를 반환한다.
func readErrorCode(t *testing.T, conn *websocket.Conn) string {
	t.Helper()
	raw := readMessage(t, conn)
	var message struct {
		Type    string `json:"type"`
		Payload struct {
			Code string `json:"code"`
		} `json:"payload"`
	}
	if err := json.Unmarshal(raw, &message); err != nil {
		t.Fatalf("unmarshal error message: %v", err)
	}
	if message.Type != "error" {
		t.Fatalf("error 메시지를 기대했다, got type %q (%s)", message.Type, raw)
	}
	return message.Payload.Code
}

// assertNoMessage는 짧은 읽기 데드라인으로 일정 시간 동안 아무 메시지도 오지 않음을 검증한다.
// "타임아웃 = 무수신"으로 본다. 타임아웃이 아닌 다른 에러나 실제 수신은 실패다.
func assertNoMessage(t *testing.T, conn *websocket.Conn) {
	t.Helper()
	_ = conn.SetReadDeadline(time.Now().Add(300 * time.Millisecond))
	_, raw, err := conn.ReadMessage()
	if err == nil {
		t.Fatalf("아무 메시지도 받지 않아야 한다, got %s", raw)
	}
	var netErr interface{ Timeout() bool }
	if !errors.As(err, &netErr) || !netErr.Timeout() {
		t.Fatalf("무수신(read timeout)을 기대했다, got %v", err)
	}
}

// joinPair는 같은 시리얼로 고객·상담사를 접속시키고 고객의 peer-joined를 소비한다.
func joinPair(t *testing.T, server *httptest.Server) (customer, agent *websocket.Conn) {
	t.Helper()
	serial := createRoom(t, server)

	customer, _, err := dialWS(t, server, serial, "customer")
	if err != nil {
		t.Fatalf("고객 접속 실패: %v", err)
	}
	t.Cleanup(func() { _ = customer.Close() })

	agent, _, err = dialWS(t, server, serial, "agent")
	if err != nil {
		t.Fatalf("상담사 접속 실패: %v", err)
	}
	t.Cleanup(func() { _ = agent.Close() })

	if got := readMessageType(t, customer); got != "peer-joined" {
		t.Fatalf("고객은 peer-joined를 받아야 한다, got %q", got)
	}
	return customer, agent
}

func TestWebSocket_HappyPath_PeerJoinedAndPeerLeft(t *testing.T) {
	server := newTestServer(t)
	serial := createRoom(t, server)

	customer, _, err := dialWS(t, server, serial, "customer")
	if err != nil {
		t.Fatalf("고객 접속 실패: %v", err)
	}
	defer func() { _ = customer.Close() }()

	agent, _, err := dialWS(t, server, serial, "agent")
	if err != nil {
		t.Fatalf("상담사 접속 실패: %v", err)
	}
	defer func() { _ = agent.Close() }()

	// 상담사 접속으로 양쪽이 모이면 고객 측에 peer-joined가 도착한다.
	if got := readMessageType(t, customer); got != "peer-joined" {
		t.Fatalf("고객은 peer-joined를 받아야 한다, got %q", got)
	}

	// 상담사가 leave → 고객 측에 peer-left 도착.
	if err := agent.WriteMessage(websocket.TextMessage, []byte(`{"type":"leave"}`)); err != nil {
		t.Fatalf("leave 전송 실패: %v", err)
	}
	if got := readMessageType(t, customer); got != "peer-left" {
		t.Fatalf("고객은 peer-left를 받아야 한다, got %q", got)
	}

	// 세션 종료 시 Invitation이 삭제되어 같은 시리얼 재접속은 404여야 한다.
	conn, response, err := dialWS(t, server, serial, "customer")
	if err == nil {
		_ = conn.Close()
		t.Fatalf("세션 종료 후 같은 시리얼 재접속은 실패해야 한다")
	}
	if response == nil || response.StatusCode != http.StatusNotFound {
		t.Fatalf("재접속은 404여야 한다, got %v", response)
	}
}

func TestWebSocket_RelaysSignalingBetweenPeers(t *testing.T) {
	server := newTestServer(t)
	serial := createRoom(t, server)

	customer, _, err := dialWS(t, server, serial, "customer")
	if err != nil {
		t.Fatalf("고객 접속 실패: %v", err)
	}
	defer func() { _ = customer.Close() }()

	agent, _, err := dialWS(t, server, serial, "agent")
	if err != nil {
		t.Fatalf("상담사 접속 실패: %v", err)
	}
	defer func() { _ = agent.Close() }()

	// 고객의 peer-joined 소비.
	if got := readMessageType(t, customer); got != "peer-joined" {
		t.Fatalf("peer-joined를 받아야 한다, got %q", got)
	}

	// 고객 → offer가 상담사에게 그대로 전달되는지(raw passthrough) 확인.
	if err := customer.WriteMessage(websocket.TextMessage, []byte(`{"type":"offer","payload":{"sdp":"x"}}`)); err != nil {
		t.Fatalf("offer 전송 실패: %v", err)
	}
	if got := readMessageType(t, agent); got != "offer" {
		t.Fatalf("상담사는 offer를 전달받아야 한다, got %q", got)
	}
}

func TestWebSocket_RejectsBadRequests(t *testing.T) {
	server := newTestServer(t)

	// serial/role 누락 → 400.
	if _, response, err := dialWS(t, server, "", ""); err == nil || response.StatusCode != http.StatusBadRequest {
		t.Fatalf("파라미터 누락은 400이어야 한다, got resp=%v err=%v", response, err)
	}

	// 존재하지 않는 시리얼 → 404.
	if _, response, err := dialWS(t, server, "ZZZZZZ", "customer"); err == nil || response.StatusCode != http.StatusNotFound {
		t.Fatalf("존재하지 않는 시리얼은 404여야 한다, got resp=%v err=%v", response, err)
	}
}

func TestWebSocket_UnknownMessageTypeKeepsConnection(t *testing.T) {
	server := newTestServer(t)
	serial := createRoom(t, server)

	customer, _, err := dialWS(t, server, serial, "customer")
	if err != nil {
		t.Fatalf("고객 접속 실패: %v", err)
	}
	defer func() { _ = customer.Close() }()

	if err := customer.WriteMessage(websocket.TextMessage, []byte(`{"type":"bogus"}`)); err != nil {
		t.Fatalf("메시지 전송 실패: %v", err)
	}
	// 연결은 유지되고 error 메시지가 돌아온다.
	if got := readMessageType(t, customer); got != "error" {
		t.Fatalf("알 수 없는 타입에는 error를 응답해야 한다, got %q", got)
	}
}

// ── 시그널링 중계 (docs/designs/화면공유/시그널링_중계.md Test Plan) ──────────────

// AC-1: 고객 offer → 상담원이 동일 메시지(원문 그대로) 수신.
func TestSignaling_CustomerOffer_RelaysToAgent(t *testing.T) {
	server := newTestServer(t)
	customer, agent := joinPair(t, server)

	offer := []byte(`{"type":"offer","payload":{"sdp":"v=0\r\no=customer"}}`)
	if err := customer.WriteMessage(websocket.TextMessage, offer); err != nil {
		t.Fatalf("offer 전송 실패: %v", err)
	}

	// AC-7: 원문 보존 — 상담원이 받은 바이트가 발신 바이트와 동일.
	if got := readMessage(t, agent); string(got) != string(offer) {
		t.Fatalf("상담원은 offer 원문을 그대로 받아야 한다\n want %s\n got  %s", offer, got)
	}
}

// AC-2: 상담원 answer → 고객이 동일 메시지 수신.
func TestSignaling_AgentAnswer_RelaysToCustomer(t *testing.T) {
	server := newTestServer(t)
	customer, agent := joinPair(t, server)

	answer := []byte(`{"type":"answer","payload":{"sdp":"v=0\r\no=agent"}}`)
	if err := agent.WriteMessage(websocket.TextMessage, answer); err != nil {
		t.Fatalf("answer 전송 실패: %v", err)
	}

	if got := readMessage(t, customer); string(got) != string(answer) {
		t.Fatalf("고객은 answer 원문을 그대로 받아야 한다\n want %s\n got  %s", answer, got)
	}
}

// AC-3: ice-candidate는 양방향으로 전달된다.
func TestSignaling_ICECandidate_RelaysBothWays(t *testing.T) {
	server := newTestServer(t)
	customer, agent := joinPair(t, server)

	// 고객 → 상담원
	fromCustomer := []byte(`{"type":"ice-candidate","payload":{"candidate":"c-from-customer"}}`)
	if err := customer.WriteMessage(websocket.TextMessage, fromCustomer); err != nil {
		t.Fatalf("고객 ice 전송 실패: %v", err)
	}
	if got := readMessage(t, agent); string(got) != string(fromCustomer) {
		t.Fatalf("상담원은 고객 ice를 그대로 받아야 한다\n want %s\n got  %s", fromCustomer, got)
	}

	// 상담원 → 고객
	fromAgent := []byte(`{"type":"ice-candidate","payload":{"candidate":"c-from-agent"}}`)
	if err := agent.WriteMessage(websocket.TextMessage, fromAgent); err != nil {
		t.Fatalf("상담원 ice 전송 실패: %v", err)
	}
	if got := readMessage(t, customer); string(got) != string(fromAgent) {
		t.Fatalf("고객은 상담원 ice를 그대로 받아야 한다\n want %s\n got  %s", fromAgent, got)
	}
}

// AC-5 / AC-8: 상담원이 offer 전송 → 발신 상담원이 INVALID_SENDER, 고객은 무수신.
func TestSignaling_AgentOffer_RejectedWithInvalidSender(t *testing.T) {
	server := newTestServer(t)
	customer, agent := joinPair(t, server)

	if err := agent.WriteMessage(websocket.TextMessage, []byte(`{"type":"offer","payload":{"sdp":"x"}}`)); err != nil {
		t.Fatalf("offer 전송 실패: %v", err)
	}

	if got := readErrorCode(t, agent); got != "INVALID_SENDER" {
		t.Fatalf("상담원 offer는 INVALID_SENDER로 거부돼야 한다, got %q", got)
	}
	// AC-8: 거부 시 상대(고객)는 아무것도 받지 않는다.
	assertNoMessage(t, customer)
}

// AC-6 / AC-8: 고객이 answer 전송 → 발신 고객이 INVALID_SENDER, 상담원은 무수신.
func TestSignaling_CustomerAnswer_RejectedWithInvalidSender(t *testing.T) {
	server := newTestServer(t)
	customer, agent := joinPair(t, server)

	if err := customer.WriteMessage(websocket.TextMessage, []byte(`{"type":"answer","payload":{"sdp":"x"}}`)); err != nil {
		t.Fatalf("answer 전송 실패: %v", err)
	}

	if got := readErrorCode(t, customer); got != "INVALID_SENDER" {
		t.Fatalf("고객 answer는 INVALID_SENDER로 거부돼야 한다, got %q", got)
	}
	assertNoMessage(t, agent)
}

// AC-4: 상대 미접속 상태에서 고객 offer → 발신 고객이 PEER_NOT_CONNECTED.
func TestSignaling_OfferWithoutPeer_RejectedWithPeerNotConnected(t *testing.T) {
	server := newTestServer(t)
	serial := createRoom(t, server)

	customer, _, err := dialWS(t, server, serial, "customer")
	if err != nil {
		t.Fatalf("고객 접속 실패: %v", err)
	}
	defer func() { _ = customer.Close() }()

	// 상담원 미접속 상태에서 offer 전송.
	if err := customer.WriteMessage(websocket.TextMessage, []byte(`{"type":"offer","payload":{"sdp":"x"}}`)); err != nil {
		t.Fatalf("offer 전송 실패: %v", err)
	}

	if got := readErrorCode(t, customer); got != "PEER_NOT_CONNECTED" {
		t.Fatalf("상대 미접속 offer는 PEER_NOT_CONNECTED로 거부돼야 한다, got %q", got)
	}
}

// ── 제어 이벤트 중계 (docs/designs/원격-제어/제어_이벤트_중계.md Test Plan) ──────────

// AC-1: 상담원 control-event(click) → 고객이 동일 payload + 보완된 timestamp 수신.
func TestControl_AgentClick_RelaysToCustomer(t *testing.T) {
	server := newTestServer(t)
	customer, agent := joinPair(t, server)

	send := []byte(`{"type":"control-event","payload":{"type":"click","x":320,"y":240}}`)
	if err := agent.WriteMessage(websocket.TextMessage, send); err != nil {
		t.Fatalf("control-event 전송 실패: %v", err)
	}

	raw := readMessage(t, customer)
	var message struct {
		Type    string `json:"type"`
		Payload struct {
			Type      string `json:"type"`
			X         *int   `json:"x"`
			Y         *int   `json:"y"`
			Timestamp int64  `json:"timestamp"`
		} `json:"payload"`
	}
	if err := json.Unmarshal(raw, &message); err != nil {
		t.Fatalf("control-event 수신 디코딩 실패: %v (%s)", err, raw)
	}
	if message.Type != "control-event" {
		t.Fatalf("고객은 control-event를 받아야 한다, got %q", message.Type)
	}
	if message.Payload.Type != "click" {
		t.Fatalf("payload.type은 click이어야 한다, got %q", message.Payload.Type)
	}
	if message.Payload.X == nil || *message.Payload.X != 320 || message.Payload.Y == nil || *message.Payload.Y != 240 {
		t.Fatalf("위치 x/y가 보존돼야 한다, got x=%v y=%v", message.Payload.X, message.Payload.Y)
	}
	// timestamp 누락 → 서버가 0이 아닌 값으로 보완.
	if message.Payload.Timestamp == 0 {
		t.Fatalf("누락된 timestamp는 서버가 보완해야 한다, got 0")
	}
}

// AC-4: 고객 control-event → 발신 고객이 FORBIDDEN, 상담원은 무수신.
func TestControl_CustomerEvent_RejectedWithForbidden(t *testing.T) {
	server := newTestServer(t)
	customer, agent := joinPair(t, server)

	if err := customer.WriteMessage(websocket.TextMessage, []byte(`{"type":"control-event","payload":{"type":"click","x":1,"y":2}}`)); err != nil {
		t.Fatalf("control-event 전송 실패: %v", err)
	}

	if got := readErrorCode(t, customer); got != "FORBIDDEN" {
		t.Fatalf("고객 control-event는 FORBIDDEN으로 거부돼야 한다, got %q", got)
	}
	assertNoMessage(t, agent)
}

// AC-5: 미허용 종류(type) → INVALID_EVENT_TYPE.
func TestControl_UnknownEventType_RejectedWithInvalidType(t *testing.T) {
	server := newTestServer(t)
	customer, agent := joinPair(t, server)

	if err := agent.WriteMessage(websocket.TextMessage, []byte(`{"type":"control-event","payload":{"type":"hover","x":1,"y":2}}`)); err != nil {
		t.Fatalf("control-event 전송 실패: %v", err)
	}

	if got := readErrorCode(t, agent); got != "INVALID_EVENT_TYPE" {
		t.Fatalf("미허용 종류는 INVALID_EVENT_TYPE로 거부돼야 한다, got %q", got)
	}
	// 거부는 상대에게 전달되지 않는다.
	assertNoMessage(t, customer)
}

// NFR: 거부 직후에도 연결이 유지되어 후속 정상 이벤트는 계속 전달된다.
func TestControl_RejectionKeepsConnection(t *testing.T) {
	server := newTestServer(t)
	customer, agent := joinPair(t, server)

	// 먼저 미허용 종류로 거부당한다.
	if err := agent.WriteMessage(websocket.TextMessage, []byte(`{"type":"control-event","payload":{"type":"hover"}}`)); err != nil {
		t.Fatalf("control-event 전송 실패: %v", err)
	}
	if got := readErrorCode(t, agent); got != "INVALID_EVENT_TYPE" {
		t.Fatalf("미허용 종류는 INVALID_EVENT_TYPE로 거부돼야 한다, got %q", got)
	}

	// 거부 직후 정상 이벤트는 그대로 고객에게 전달돼야 한다(연결 유지).
	if err := agent.WriteMessage(websocket.TextMessage, []byte(`{"type":"control-event","payload":{"type":"scroll","deltaY":5}}`)); err != nil {
		t.Fatalf("후속 control-event 전송 실패: %v", err)
	}
	if got := readMessageType(t, customer); got != "control-event" {
		t.Fatalf("거부 후에도 정상 이벤트는 전달돼야 한다, got %q", got)
	}
}

// ── TURN 자격증명 (docs/designs/화면공유/turn_자격증명_발급.md Test Plan) ─────────

// AC-1 / AC-3: GET /turn-credentials → 200 + {"data":{...}} 봉투, ttl==3600, uris≥1.
func TestTURN_Credentials_ReturnsEnvelope(t *testing.T) {
	server := newTestServer(t)

	response, err := http.Get(server.URL + "/turn-credentials")
	if err != nil {
		t.Fatalf("GET /turn-credentials: %v", err)
	}
	defer func() { _ = response.Body.Close() }()

	if response.StatusCode != http.StatusOK {
		t.Fatalf("GET /turn-credentials status = %d, want 200", response.StatusCode)
	}

	var body struct {
		Data struct {
			Username string   `json:"username"`
			Password string   `json:"password"`
			TTL      int      `json:"ttl"`
			URIs     []string `json:"uris"`
		} `json:"data"`
	}
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatalf("decode turn-credentials 응답: %v", err)
	}

	if body.Data.Username == "" || body.Data.Password == "" {
		t.Fatalf("username/password가 채워져야 한다, got %+v", body.Data)
	}
	if body.Data.TTL != 3600 {
		t.Fatalf("ttl은 3600이어야 한다, got %d", body.Data.TTL)
	}
	if len(body.Data.URIs) < 1 {
		t.Fatalf("uris는 하나 이상이어야 한다, got %v", body.Data.URIs)
	}
}

// AC-8: 환경변수 미설정에서도 기본값으로 200 발급.
func TestTURN_Credentials_DefaultsWhenUnset(t *testing.T) {
	t.Setenv("TURN_SECRET", "")
	t.Setenv("TURN_URIS", "")
	server := newTestServer(t)

	response, err := http.Get(server.URL + "/turn-credentials")
	if err != nil {
		t.Fatalf("GET /turn-credentials: %v", err)
	}
	defer func() { _ = response.Body.Close() }()

	if response.StatusCode != http.StatusOK {
		t.Fatalf("환경변수 미설정에서도 200이어야 한다, got %d", response.StatusCode)
	}

	var body struct {
		Data struct {
			URIs []string `json:"uris"`
		} `json:"data"`
	}
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatalf("decode turn-credentials 응답: %v", err)
	}
	if len(body.Data.URIs) < 1 {
		t.Fatalf("기본값으로도 uris≥1을 보장해야 한다, got %v", body.Data.URIs)
	}
}
