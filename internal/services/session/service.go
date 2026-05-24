package session

import (
	"context"
	"errors"
	"fmt"

	"co-browsing-session-server/internal/domain/serialnumber"
	sessiondomain "co-browsing-session-server/internal/domain/session"
)

const (
	createMaxRetries = 5
	serialLength     = 6
)

type Service struct {
	repo sessiondomain.Repository
	gen  serialnumber.Generator
}

func NewService(repo sessiondomain.Repository, gen serialnumber.Generator) *Service {
	return &Service{repo: repo, gen: gen}
}

// Create는 새 Session을 만든다. serial 충돌이 발생하면 최대 createMaxRetries회 재시도한다.
func (s *Service) Create(ctx context.Context) (*sessiondomain.Session, error) {
	for range createMaxRetries {
		serial := s.gen.Generate(serialLength)
		newSession := sessiondomain.New(serial)

		if _, err := s.repo.Create(newSession); err == nil {
			return newSession, nil
		} else if !errors.Is(err, sessiondomain.ErrAlreadyExists) {
			return nil, fmt.Errorf("create session: %w", err)
		}
	}
	return nil, fmt.Errorf("create session: exhausted %d retries due to serial collisions", createMaxRetries)
}
