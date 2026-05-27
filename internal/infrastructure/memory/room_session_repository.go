package memory

import (
	"sync"
	"time"

	"co-browsing-session-server/internal/domain/roomsession"
)

type RoomSessionRepository struct {
	mu       sync.Mutex
	sessions map[roomsession.RoomID]*roomsession.RoomSession
}

func NewRoomSessionRepository() *RoomSessionRepository {
	return &RoomSessionRepository{
		sessions: make(map[roomsession.RoomID]*roomsession.RoomSession),
	}
}

func (r *RoomSessionRepository) Create(s *roomsession.RoomSession) (*roomsession.RoomSession, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exist := r.sessions[s.ID]; exist {
		return nil, roomsession.ErrAlreadyExists
	}
	r.sessions[s.ID] = s
	return s, nil
}

func (r *RoomSessionRepository) Get(id roomsession.RoomID) (*roomsession.RoomSession, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	s, exist := r.sessions[id]
	if !exist {
		return nil, roomsession.ErrNotFound
	}
	if s.IsExpired(time.Now()) {
		delete(r.sessions, id)
		return nil, roomsession.ErrExpired
	}
	return s, nil
}

func (r *RoomSessionRepository) Update(s *roomsession.RoomSession) (*roomsession.RoomSession, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exist := r.sessions[s.ID]; !exist {
		return nil, roomsession.ErrNotFound
	}
	r.sessions[s.ID] = s
	return s, nil
}

func (r *RoomSessionRepository) Delete(id roomsession.RoomID) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exist := r.sessions[id]; !exist {
		return roomsession.ErrNotFound
	}
	delete(r.sessions, id)
	return nil
}
