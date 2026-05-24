package model

import "time"

type SessionStatus string

const (
	StatusWaiting SessionStatus = "waiting"
	StatusActive SessionStatus = "active"
	StatusEnded SessionStatus = "ended"
)

type Session struct {
	Serial string // 6자리 시리얼 번호 (PK)
	Status SessionStatus
	CustomerID string // WS 연결 시 할당
	AgentID string // WS 연결 시 할당
	CreateAt time.Time
	ExpiresAt time.Time
}
