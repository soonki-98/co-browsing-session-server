package service

import (
	"co-browsing-session-server/internal/model"
	"co-browsing-session-server/internal/repository/session"
)

func CreateSession(sessionStore *session.SessionStore) (*model.Session, error) {
	const SERIAL_NUMBER_LENGTH = 6

	serialNumber := generateRandomSerialNumber(SERIAL_NUMBER_LENGTH)

	s, err := sessionStore.Create(serialNumber)

	if err != nil {
		return nil, err
	}

	return s, nil
}
