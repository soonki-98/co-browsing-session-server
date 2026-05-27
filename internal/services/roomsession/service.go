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
	rsRepo  roomsession.Repository
	invRepo invitation.Repository
	gen     serialnumber.Generator
}

func NewService(rsRepo roomsession.Repository, invRepo invitation.Repository, gen serialnumber.Generator) *Service {
	return &Service{rsRepo: rsRepo, invRepo: invRepo, gen: gen}
}

// Create는 RoomSession과 Invitation을 atomic하게 만든다.
// Invitation 발급 실패 시 보상 트랜잭션으로 RoomSession을 롤백한다.
func (s *Service) Create(ctx context.Context) (*roomsession.RoomSession, *invitation.Invitation, error) {
	roomID := roomsession.NewID()
	rs := roomsession.New(roomID)

	if _, err := s.rsRepo.Create(rs); err != nil {
		return nil, nil, fmt.Errorf("create room session: %w", err)
	}

	for range createMaxRetries {
		serial := s.gen.Generate(serialLength)
		inv := invitation.New(serial, roomID)

		_, err := s.invRepo.Create(inv)
		if err == nil {
			return rs, inv, nil
		}
		if !errors.Is(err, invitation.ErrAlreadyExists) {
			_ = s.rsRepo.Delete(roomID)
			return nil, nil, fmt.Errorf("create invitation: %w", err)
		}
	}

	_ = s.rsRepo.Delete(roomID)
	return nil, nil, fmt.Errorf("create invitation: exhausted %d retries due to serial collisions", createMaxRetries)
}
