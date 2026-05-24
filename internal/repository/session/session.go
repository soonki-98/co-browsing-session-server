package session

import (
	"co-browsing-session-server/internal/model"
	"time"
)

func NewSessionStore() *SessionStore {
	return &SessionStore{
		sessions: make(map[string]*model.Session),
	}
}

func (s *SessionStore) Create(serial string) (*model.Session, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exist := s.sessions[serial]; exist {
		return nil, ErrSessionExists
	}

	session := &model.Session{
		Serial:    serial,
		Status:    model.StatusWaiting,
		CreateAt:  time.Now(),
		ExpiresAt: time.Now().Add(SessionTTL),
	}

	s.sessions[serial] = session

	return session, nil
}
