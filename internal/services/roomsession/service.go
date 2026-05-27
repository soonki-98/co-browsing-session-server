package roomsession

import (
	"context"
	"errors"
	"fmt"

	"co-browsing-session-server/internal/domain/invitation"
	"co-browsing-session-server/internal/domain/roomsession"
	"co-browsing-session-server/internal/domain/serialnumber"
)

const (
	createMaxRetries = 5
	serialLength     = 6
)

type Service struct {
	roomSessionRepository roomsession.Repository
	invitationRepository  invitation.Repository
	serialNumberGenerator serialnumber.Generator
}

func NewService(
	roomSessionRepository roomsession.Repository,
	invitationRepository invitation.Repository,
	serialNumberGenerator serialnumber.Generator,
) *Service {
	return &Service{
		roomSessionRepository: roomSessionRepository,
		invitationRepository:  invitationRepository,
		serialNumberGenerator: serialNumberGenerator,
	}
}

// Create는 RoomSession과 Invitation을 atomic하게 만든다.
// Invitation 발급 실패 시 보상 트랜잭션으로 RoomSession을 롤백한다.
func (service *Service) Create(requestContext context.Context) (*roomsession.RoomSession, *invitation.Invitation, error) {
	roomID := roomsession.NewID()
	newRoomSession := roomsession.New(roomID)

	if _, err := service.roomSessionRepository.Create(newRoomSession); err != nil {
		return nil, nil, fmt.Errorf("create room session: %w", err)
	}

	for range createMaxRetries {
		serial := service.serialNumberGenerator.Generate(serialLength)
		newInvitation := invitation.New(serial, roomID)

		_, err := service.invitationRepository.Create(newInvitation)
		if err == nil {
			return newRoomSession, newInvitation, nil
		}
		if !errors.Is(err, invitation.ErrAlreadyExists) {
			_ = service.roomSessionRepository.Delete(roomID)
			return nil, nil, fmt.Errorf("create invitation: %w", err)
		}
	}

	_ = service.roomSessionRepository.Delete(roomID)
	return nil, nil, fmt.Errorf("create invitation: exhausted %d retries due to serial collisions", createMaxRetries)
}
