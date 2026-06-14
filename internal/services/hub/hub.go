package hub

import (
	"sync"

	"github.com/gorilla/websocket"
)

// Role은 한 LiveRoom 안에서 클라이언트가 맡는 역할이다.
type Role string

const (
	RoleCustomer Role = "customer"
	RoleAgent    Role = "agent"
)

// Client는 WebSocket 연결 하나를 나타낸다.
// RoomID와 Serial은 opaque string으로만 다룬다 — Hub는 도메인 타입을 알지 못한다.
type Client struct {
	Conn   *websocket.Conn
	Role   Role
	RoomID string      // 속한 LiveRoom의 ID (= roomsession.RoomID 값)
	Serial string      // disconnect 시 Invitation 삭제에 사용 (= invitation.Serial 값)
	Send   chan []byte // 이 클라이언트에게 보낼 메시지 채널 (버퍼드)
}

// LiveRoom은 WS 연결 중인 클라이언트 한 쌍을 담는 휘발 컨테이너다.
// 영속 entity인 roomsession.RoomSession과 같은 ID를 공유하지만,
// 서버 재시작 시 사라져도 무방하다 — 실제 WS 연결과 운명을 같이 한다.
type LiveRoom struct {
	ID       string  // = roomsession.RoomID 값
	Customer *Client // nil이면 아직 미접속
	Agent    *Client // nil이면 아직 미접속
}

// Hub는 모든 LiveRoom을 룸 단위로 관리하는 중앙 주소록이다.
// 도메인이나 인프라를 참조하지 않는 self-contained 컴포넌트다.
type Hub struct {
	mutex     sync.RWMutex
	liveRooms map[string]*LiveRoom // key: RoomID
}

func NewHub() *Hub {
	return &Hub{
		liveRooms: make(map[string]*LiveRoom),
	}
}

// JoinRoom은 클라이언트를 LiveRoom에 추가한다. LiveRoom이 없으면 생성한다.
// 동일 role이 이미 접속 중이면 기존 연결을 Close()한 뒤 교체한다.
// 반환값은 상대 role 클라이언트이며, 아직 미접속이면 nil이다.
func (hub *Hub) JoinRoom(roomID string, client *Client) (peer *Client) {
	hub.mutex.Lock()
	defer hub.mutex.Unlock()

	liveRoom, exists := hub.liveRooms[roomID]
	if !exists {
		liveRoom = &LiveRoom{ID: roomID}
		hub.liveRooms[roomID] = liveRoom
	}

	switch client.Role {
	case RoleCustomer:
		if liveRoom.Customer != nil && liveRoom.Customer.Conn != nil {
			_ = liveRoom.Customer.Conn.Close()
		}
		liveRoom.Customer = client
		return liveRoom.Agent
	case RoleAgent:
		if liveRoom.Agent != nil && liveRoom.Agent.Conn != nil {
			_ = liveRoom.Agent.Conn.Close()
		}
		liveRoom.Agent = client
		return liveRoom.Customer
	}
	return nil
}

// LeaveRoom은 클라이언트를 LiveRoom에서 제거한다.
// 양쪽 슬롯이 모두 비면 LiveRoom 자체를 삭제한다.
// 반환값은 상대방 클라이언트로, peer-left 알림 전송에 쓰인다 (nil 가능).
func (hub *Hub) LeaveRoom(client *Client) (peer *Client) {
	hub.mutex.Lock()
	defer hub.mutex.Unlock()

	liveRoom, exists := hub.liveRooms[client.RoomID]
	if !exists {
		return nil
	}

	switch client.Role {
	case RoleCustomer:
		// 이미 새 연결로 교체된 경우 자기 자신이 아닐 수 있으므로 동일성 확인.
		if liveRoom.Customer == client {
			liveRoom.Customer = nil
		}
		peer = liveRoom.Agent
	case RoleAgent:
		if liveRoom.Agent == client {
			liveRoom.Agent = nil
		}
		peer = liveRoom.Customer
	}

	if liveRoom.Customer == nil && liveRoom.Agent == nil {
		delete(hub.liveRooms, client.RoomID)
	}
	return peer
}

// GetRoom은 roomID에 해당하는 LiveRoom을 반환한다. 없으면 nil이다.
func (hub *Hub) GetRoom(roomID string) *LiveRoom {
	hub.mutex.RLock()
	defer hub.mutex.RUnlock()
	return hub.liveRooms[roomID]
}

// GetPeer는 특정 클라이언트의 상대방을 반환한다. 없으면 nil이다.
func (hub *Hub) GetPeer(client *Client) *Client {
	hub.mutex.RLock()
	defer hub.mutex.RUnlock()

	liveRoom, exists := hub.liveRooms[client.RoomID]
	if !exists {
		return nil
	}
	switch client.Role {
	case RoleCustomer:
		return liveRoom.Agent
	case RoleAgent:
		return liveRoom.Customer
	}
	return nil
}
