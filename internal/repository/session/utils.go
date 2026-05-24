package session

import (
	"co-browsing-session-server/internal/domain"
)

func isValidStatus(status domain.SessionStatus) bool {
	return status == domain.StatusActive || status == domain.StatusEnded || status == domain.StatusWaiting
}

func isValidStatusTransition(before domain.SessionStatus, after domain.SessionStatus) bool {
	result := false

	switch before {
	case domain.StatusWaiting:
		return after == domain.StatusActive || after == domain.StatusEnded
	case domain.StatusActive:
		return after == domain.StatusEnded
	}

	return result
}
