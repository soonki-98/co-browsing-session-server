package store

import (
	"co-browsing-session-server/internal/model"
	"sync"
)

type SessionStore struct {
	mu       sync.RWMutex
	sessions map[string]*model.Session // key: serial number
}

func NewSessionStore() *SessionStore {
	return &SessionStore{
		sessions: make(map[string]*model.Session),
	}
}
 