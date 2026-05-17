package store

import (
	"sync"
	"time"
)

type SessionStatus string

const (
	StatusWaiting SessionStatus = "waiting"
	StatusActive SessionStatus = "active"
	StatusEnded SessionStatus = "ended"
)

type Session struct {
	Serial string
	Status SessionStatus
	CustomerID string
	AgentID string
	CreateAt time.Time
	ExpiresAt time.Time
}

type SessionStore struct {
    mu       sync.RWMutex
    sessions map[string]*Session // key: serial number
}

func NewSessionStore() *SessionStore {
    return &SessionStore{
        sessions: make(map[string]*Session),
    }
}