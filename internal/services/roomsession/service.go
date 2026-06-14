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

// Get은 RoomSession을 조회한다. WS 핸들러가 업그레이드 전 ended/만료 여부를
// 가볍게 사전 검증하는 용도다. read-on-check 동작은 리포지토리에 위임한다.
func (service *Service) Get(roomID roomsession.RoomID) (*roomsession.RoomSession, error) {
	return service.roomSessionRepository.Get(roomID)
}

// Activate는 양쪽이 모두 접속한 순간 RoomSession을 active로 전이시킨다.
// 이미 active/ended라 전이가 불가한 경우(ErrInvalidTransition)는 멱등하게 무시한다.
func (service *Service) Activate(roomID roomsession.RoomID) error {
	roomSession, err := service.roomSessionRepository.Get(roomID)
	if err != nil {
		return fmt.Errorf("get room session: %w", err)
	}

	if err := roomSession.Transition(roomsession.StatusActive); err != nil {
		if errors.Is(err, roomsession.ErrInvalidTransition) {
			return nil
		}
		return fmt.Errorf("transition to active: %w", err)
	}

	if _, err := service.roomSessionRepository.Update(roomSession); err != nil {
		return fmt.Errorf("update room session: %w", err)
	}
	return nil
}

// End는 한쪽이 끊긴 순간 RoomSession을 ended로 전이시키고 Invitation을 명시 삭제한다.
// 세션이 이미 없거나 만료/ended인 경우에도 Invitation 삭제는 best-effort로 수행한다(defense in depth).
func (service *Service) End(roomID roomsession.RoomID, serial serialnumber.SerialNumber) error {
	roomSession, err := service.roomSessionRepository.Get(roomID)
	if err != nil {
		if errors.Is(err, roomsession.ErrNotFound) || errors.Is(err, roomsession.ErrExpired) {
			_ = service.invitationRepository.Delete(serial)
			return nil
		}
		return fmt.Errorf("get room session: %w", err)
	}

	if err := roomSession.Transition(roomsession.StatusEnded); err != nil {
		if !errors.Is(err, roomsession.ErrInvalidTransition) {
			return fmt.Errorf("transition to ended: %w", err)
		}
		// 이미 ended — Invitation만 삭제하고 정상 종료.
	} else if _, err := service.roomSessionRepository.Update(roomSession); err != nil {
		return fmt.Errorf("update room session: %w", err)
	}

	_ = service.invitationRepository.Delete(serial)
	return nil
}
