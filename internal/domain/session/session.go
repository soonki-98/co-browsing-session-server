package session

import (
	"errors"
	"time"

	"co-browsing-session-server/internal/domain/serialnumber"
)

const TTL = 10 * time.Minute

var (
	ErrNotFound          = errors.New("session not found")
	ErrAlreadyExists     = errors.New("session already exists")
	ErrExpired           = errors.New("session expired")
	ErrInvalidTransition = errors.New("invalid status transition")
)

type Session struct {
	Serial     serialnumber.SerialNumber
	Status     Status
	CustomerID string
	AgentID    string
	CreateAt   time.Time
	ExpiresAt  time.Time
}

func New(serial serialnumber.SerialNumber) *Session {
	now := time.Now()
	return &Session{
		Serial:    serial,
		Status:    StatusWaiting,
		CreateAt:  now,
		ExpiresAt: now.Add(TTL),
	}
}

func (s *Session) IsExpired(now time.Time) bool {
	return !s.ExpiresAt.IsZero() && now.After(s.ExpiresAt)
}

// Transition은 상태 전이 규칙을 검증한 뒤 적용한다.
// active로 진입하면 만료 시각을 제거(zero)하여 무기한 유지한다.
func (s *Session) Transition(to Status) error {
	if !s.Status.CanTransitionTo(to) {
		return ErrInvalidTransition
	}
	if to == StatusActive {
		s.ExpiresAt = time.Time{}
	}
	s.Status = to
	return nil
}
