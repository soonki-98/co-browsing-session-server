package session

import "co-browsing-session-server/internal/model"

func isValidStatus(status model.SessionStatus) bool {
	return status == model.StatusActive || status == model.StatusEnded || status == model.StatusWaiting
}

func isValidStatusTransition(before model.SessionStatus, after model.SessionStatus) bool {
	result := false

	switch before {
	case model.StatusWaiting:
		return after == model.StatusActive || after == model.StatusEnded
	case model.StatusActive:
		return after == model.StatusEnded
	}

	return result
}
