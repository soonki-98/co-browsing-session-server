package session

import (
	"co-browsing-session-server/internal/domain"
	"time"
)

func NewSessionStore() *SessionStore {
	return &SessionStore{
		sessions: make(map[domain.SerialNumber]*domain.Session),
	}
}

func (s *SessionStore) Create(session *domain.Session) (*domain.Session, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exist := s.sessions[session.Serial]; exist {
		return nil, ErrSessionExists
	}

	s.sessions[session.Serial] = session

	return session, nil
}

func (s *SessionStore) Get(serial domain.SerialNumber) (*domain.Session, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	value, exist := s.sessions[serial]

	if !exist {
		return nil, ErrSessionNotFound
	}

	if !value.ExpiresAt.IsZero() && time.Now().After(value.ExpiresAt) {
		return nil, ErrSessionExpired
	}

	return value, nil
}

func (s *SessionStore) UpdateStatus(serial domain.SerialNumber, status domain.SessionStatus) (*domain.Session, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	value, exist := s.sessions[serial]

	if !exist {
		return nil, ErrSessionNotFound
	}

	if !value.ExpiresAt.IsZero() && time.Now().After(value.ExpiresAt) {
		return nil, ErrSessionExpired
	}

	if !isValidStatus(status) {
		return nil, ErrInvalidTransition
	}

	if !isValidStatusTransition(value.Status, status) {
		return nil, ErrInvalidTransition
	}

	if status == domain.StatusActive {
		value.ExpiresAt = time.Time{}
	}
	value.Status = status
	return value, nil
}

func (s *SessionStore) Delete(serial domain.SerialNumber) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, exist := s.sessions[serial]

	if !exist {
		return ErrSessionNotFound
	}

	delete(s.sessions, serial)

	return nil
}
