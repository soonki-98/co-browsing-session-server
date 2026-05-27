package invitation

import (
	"errors"
	"time"

	"co-browsing-session-server/internal/domain/roomsession"
	"co-browsing-session-server/internal/domain/serialnumber"
)

const InvitationTTL = 10 * time.Minute

var (
	ErrNotFound      = errors.New("invitation not found")
	ErrExpired       = errors.New("invitation expired")
	ErrAlreadyExists = errors.New("invitation already exists")
)

type Invitation struct {
	Serial    serialnumber.SerialNumber
	RoomID    roomsession.RoomID
	IssuedAt  time.Time
	ExpiresAt time.Time
}

func New(serial serialnumber.SerialNumber, roomID roomsession.RoomID) *Invitation {
	now := time.Now()
	return &Invitation{
		Serial:    serial,
		RoomID:    roomID,
		IssuedAt:  now,
		ExpiresAt: now.Add(InvitationTTL),
	}
}

func (i *Invitation) IsExpired(now time.Time) bool {
	return now.After(i.ExpiresAt)
}
