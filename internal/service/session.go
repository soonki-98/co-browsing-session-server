package service

import (
	"co-browsing-session-server/internal/domain"
	"co-browsing-session-server/internal/repository/session"
	"errors"
)

func CreateSession(store *session.SessionStore) (*domain.Session, error) {
	const maxRetries = 5

	for range maxRetries {
		serial := domain.GenerateRandomSerialNumber(6)
		newSession := domain.CreateSession(serial)

		s, err := store.Create(newSession)

		if err == nil {
			return s, nil
		}
		if !errors.Is(err, session.ErrSessionExists) {
			return nil, err
		}
	}

	return nil, errors.New("failed to create session")
}
