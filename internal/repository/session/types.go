package session

import (
	"co-browsing-session-server/internal/domain"
	"errors"
	"sync"
)

var (
	ErrSessionNotFound   = errors.New("session not found")
	ErrSessionExpired    = errors.New("session expired")
	ErrSessionExists     = errors.New("session already exists")
	ErrInvalidTransition = errors.New("invalid status transition")
)

type SessionStore struct {
	mu       sync.RWMutex
	sessions map[domain.SerialNumber]*domain.Session // key: serial number
}
