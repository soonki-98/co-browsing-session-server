package hub

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/gorilla/websocket"
)

// newTestConn은 실제 *websocket.Conn 하나를 만들어 반환한다.
// JoinRoom 교체 시 호출되는 Conn.Close() 동작을 실제 연결로 검증하기 위해 사용한다.
func newTestConn(t *testing.T) *websocket.Conn {
	t.Helper()

	upgrader := websocket.Upgrader{}
	server := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
		serverConn, err := upgrader.Upgrade(responseWriter, request, nil)
		if err != nil {
			t.Errorf("upgrade: %v", err)
			return
		}
		// 서버 측 연결은 테스트 동안 그대로 둔다. 닫힘은 클라이언트 측에서 관찰한다.
		_ = serverConn
	}))
	t.Cleanup(server.Close)

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	clientConn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	t.Cleanup(func() { _ = clientConn.Close() })
	return clientConn
}

func TestJoinRoom_CustomerFirstThenAgent(t *testing.T) {
	hub := NewHub()

	customer := &Client{Role: RoleCustomer, RoomID: "room-1"}
	if peer := hub.JoinRoom("room-1", customer); peer != nil {
		t.Fatalf("고객 단독 접속 시 peer는 nil이어야 한다, got %+v", peer)
	}

	agent := &Client{Role: RoleAgent, RoomID: "room-1"}
	peer := hub.JoinRoom("room-1", agent)
	if peer != customer {
		t.Fatalf("상담사 접속 시 peer는 고객 Client여야 한다, got %+v", peer)
	}

	if got := hub.GetPeer(customer); got != agent {
		t.Fatalf("고객의 peer는 상담사여야 한다, got %+v", got)
	}
	if got := hub.GetPeer(agent); got != customer {
		t.Fatalf("상담사의 peer는 고객이어야 한다, got %+v", got)
	}
}

func TestJoinRoom_ReplacesSameRoleAndClosesOldConn(t *testing.T) {
	hub := NewHub()

	oldConn := newTestConn(t)
	first := &Client{Role: RoleCustomer, RoomID: "room-1", Conn: oldConn}
	hub.JoinRoom("room-1", first)

	second := &Client{Role: RoleCustomer, RoomID: "room-1", Conn: newTestConn(t)}
	hub.JoinRoom("room-1", second)

	// 슬롯이 새 클라이언트로 교체되었는지 확인.
	liveRoom := hub.GetRoom("room-1")
	if liveRoom.Customer != second {
		t.Fatalf("재접속 시 고객 슬롯이 새 클라이언트로 교체되어야 한다")
	}

	// 기존 연결은 Close()되어 더 이상 쓰기가 불가능해야 한다.
	if err := oldConn.WriteMessage(websocket.TextMessage, []byte("ping")); err == nil {
		t.Fatalf("교체된 기존 연결은 닫혀 있어야 한다 (write가 실패해야 함)")
	}
}

func TestLeaveRoom_ReturnsPeerAndDeletesEmptyRoom(t *testing.T) {
	hub := NewHub()

	customer := &Client{Role: RoleCustomer, RoomID: "room-1"}
	agent := &Client{Role: RoleAgent, RoomID: "room-1"}
	hub.JoinRoom("room-1", customer)
	hub.JoinRoom("room-1", agent)

	if peer := hub.LeaveRoom(customer); peer != agent {
		t.Fatalf("고객 퇴장 시 peer로 상담사를 반환해야 한다, got %+v", peer)
	}
	// 한쪽만 남았으므로 룸은 유지된다.
	if hub.GetRoom("room-1") == nil {
		t.Fatalf("한쪽이 남아 있으면 룸은 유지되어야 한다")
	}

	if peer := hub.LeaveRoom(agent); peer != nil {
		t.Fatalf("마지막 퇴장 시 peer는 nil이어야 한다, got %+v", peer)
	}
	if hub.GetRoom("room-1") != nil {
		t.Fatalf("양쪽 모두 나간 후 룸은 삭제되어야 한다")
	}
}

func TestLeaveRoom_StaleClientDoesNotEvictReplacement(t *testing.T) {
	hub := NewHub()

	first := &Client{Role: RoleCustomer, RoomID: "room-1"}
	hub.JoinRoom("room-1", first)
	second := &Client{Role: RoleCustomer, RoomID: "room-1"}
	hub.JoinRoom("room-1", second)

	// 교체된 옛 클라이언트의 LeaveRoom은 현재 슬롯(second)을 비우면 안 된다.
	hub.LeaveRoom(first)
	liveRoom := hub.GetRoom("room-1")
	if liveRoom == nil || liveRoom.Customer != second {
		t.Fatalf("stale 클라이언트의 퇴장이 교체된 새 클라이언트를 제거해서는 안 된다")
	}
}

func TestHub_ConcurrentJoinLeave(t *testing.T) {
	hub := NewHub()

	const goroutines = 50
	roomIDs := []string{"room-a", "room-b", "room-c"}
	roles := []Role{RoleCustomer, RoleAgent}

	var waitGroup sync.WaitGroup
	for index := 0; index < goroutines; index++ {
		waitGroup.Add(1)
		go func(index int) {
			defer waitGroup.Done()
			roomID := roomIDs[index%len(roomIDs)]
			client := &Client{
				Role:   roles[index%len(roles)],
				RoomID: roomID,
			}
			hub.JoinRoom(roomID, client)
			hub.GetRoom(roomID)
			hub.GetPeer(client)
			hub.LeaveRoom(client)
		}(index)
	}
	waitGroup.Wait()
}
