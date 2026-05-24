package service

import (
	"co-browsing-session-server/internal/model"
	"co-browsing-session-server/internal/repository/session"
	"errors"
)

func CreateSession(store *session.SessionStore) (*model.Session, error) {
	const maxRetries = 5

	for range maxRetries {
		serial := generateRandomSerialNumber(6)
		s, err := store.Create(serial)
		if err == nil {
			return s, nil
		}
		if !errors.Is(err, session.ErrSessionExists) {
			return nil, err
		}
	}

	return nil, errors.New("failed to create session")
}
