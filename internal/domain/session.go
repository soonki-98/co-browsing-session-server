package domain

import (
	"co-browsing-session-server/internal/model"
	"time"
)

const SessionTTL = 10 * time.Minute

func CreateSession(serial string) *model.Session {
	session := &model.Session{
		Serial:    serial,
		Status:    model.StatusWaiting,
		CreateAt:  time.Now(),
		ExpiresAt: time.Now().Add(SessionTTL),
	}

	return session
}
