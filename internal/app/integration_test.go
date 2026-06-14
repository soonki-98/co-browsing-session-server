package app_test

import (
	"encoding/json"
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
