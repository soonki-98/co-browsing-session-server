package memory

import (
	"sync"

	"co-browsing-session-server/internal/domain/serialnumber"
	"co-browsing-session-server/internal/domain/session"
)

type SessionRepository struct {
	mu       sync.RWMutex
	sessions map[serialnumber.SerialNumber]*session.Session
}

func NewSessionRepository() *SessionRepository {
	return &SessionRepository{
		sessions: make(map[serialnumber.SerialNumber]*session.Session),
	}
}

func (r *SessionRepository) Create(s *session.Session) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exist := r.sessions[s.Serial]; exist {
		return session.ErrAlreadyExists
	}
	r.sessions[s.Serial] = s
	return nil
}

func (r *SessionRepository) Get(serial serialnumber.SerialNumber) (*session.Session, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	s, exist := r.sessions[serial]
	if !exist {
		return nil, session.ErrNotFound
	}
	return s, nil
}

func (r *SessionRepository) Save(s *session.Session) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exist := r.sessions[s.Serial]; !exist {
		return session.ErrNotFound
	}
	r.sessions[s.Serial] = s
	return nil
}

func (r *SessionRepository) Delete(serial serialnumber.SerialNumber) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exist := r.sessions[serial]; !exist {
		return session.ErrNotFound
	}
	delete(r.sessions, serial)
	return nil
}
