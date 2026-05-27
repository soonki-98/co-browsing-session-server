package roomsession

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

const SessionTTL = 10 * time.Minute

var (
	ErrNotFound          = errors.New("room session not found")
	ErrExpired           = errors.New("room session expired")
	ErrAlreadyExists     = errors.New("room session already exists")
	ErrInvalidTransition = errors.New("invalid status transition")
)

type RoomID string

func (roomID RoomID) String() string {
	return string(roomID)
}

func NewID() RoomID {
	return RoomID(uuid.NewString())
}

type RoomSession struct {
	ID        RoomID
	Status    Status
	StartedAt time.Time
	ExpiresAt time.Time
}

func New(roomID RoomID) *RoomSession {
	now := time.Now()
	return &RoomSession{
		ID:        roomID,
		Status:    StatusWaiting,
		StartedAt: now,
		ExpiresAt: now.Add(SessionTTL),
	}
}

func (roomSession *RoomSession) IsExpired(now time.Time) bool {
	return !roomSession.ExpiresAt.IsZero() && now.After(roomSession.ExpiresAt)
}

// Transition은 상태 전이 규칙을 검증한 뒤 적용한다.
// active로 진입하면 ExpiresAt을 zero로 만들어 무기한 유지한다.
func (roomSession *RoomSession) Transition(targetStatus Status) error {
	if !roomSession.Status.CanTransitionTo(targetStatus) {
		return ErrInvalidTransition
	}
	if targetStatus == StatusActive {
		roomSession.ExpiresAt = time.Time{}
	}
	roomSession.Status = targetStatus
	return nil
}
