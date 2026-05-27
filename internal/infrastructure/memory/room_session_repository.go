package memory

import (
	"sync"
	"time"

	"co-browsing-session-server/internal/domain/roomsession"
)

type RoomSessionRepository struct {
	mutex    sync.Mutex
	sessions map[roomsession.RoomID]*roomsession.RoomSession
}

func NewRoomSessionRepository() *RoomSessionRepository {
	return &RoomSessionRepository{
		sessions: make(map[roomsession.RoomID]*roomsession.RoomSession),
	}
}

func (roomSessionRepository *RoomSessionRepository) Create(roomSession *roomsession.RoomSession) (*roomsession.RoomSession, error) {
	roomSessionRepository.mutex.Lock()
	defer roomSessionRepository.mutex.Unlock()

	if _, exists := roomSessionRepository.sessions[roomSession.ID]; exists {
		return nil, roomsession.ErrAlreadyExists
	}
	roomSessionRepository.sessions[roomSession.ID] = roomSession
	return roomSession, nil
}

func (roomSessionRepository *RoomSessionRepository) Get(roomID roomsession.RoomID) (*roomsession.RoomSession, error) {
	roomSessionRepository.mutex.Lock()
	defer roomSessionRepository.mutex.Unlock()

	storedSession, exists := roomSessionRepository.sessions[roomID]
	if !exists {
		return nil, roomsession.ErrNotFound
	}
	if storedSession.IsExpired(time.Now()) {
		delete(roomSessionRepository.sessions, roomID)
		return nil, roomsession.ErrExpired
	}
	return storedSession, nil
}

func (roomSessionRepository *RoomSessionRepository) Update(roomSession *roomsession.RoomSession) (*roomsession.RoomSession, error) {
	roomSessionRepository.mutex.Lock()
	defer roomSessionRepository.mutex.Unlock()

	if _, exists := roomSessionRepository.sessions[roomSession.ID]; !exists {
		return nil, roomsession.ErrNotFound
	}
	roomSessionRepository.sessions[roomSession.ID] = roomSession
	return roomSession, nil
}

func (roomSessionRepository *RoomSessionRepository) Delete(roomID roomsession.RoomID) error {
	roomSessionRepository.mutex.Lock()
	defer roomSessionRepository.mutex.Unlock()

	if _, exists := roomSessionRepository.sessions[roomID]; !exists {
		return roomsession.ErrNotFound
	}
	delete(roomSessionRepository.sessions, roomID)
	return nil
}
