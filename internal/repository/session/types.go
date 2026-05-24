package session

import (
	"co-browsing-session-server/internal/model"
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
	sessions map[string]*model.Session // key: serial number
}
