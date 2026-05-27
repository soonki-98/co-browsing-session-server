package roomsession

type Status string

const (
	StatusWaiting Status = "waiting"
	StatusActive  Status = "active"
	StatusEnded   Status = "ended"
)

func (status Status) IsValid() bool {
	switch status {
	case StatusWaiting, StatusActive, StatusEnded:
		return true
	}
	return false
}

func (status Status) CanTransitionTo(targetStatus Status) bool {
	if !targetStatus.IsValid() {
		return false
	}
	switch status {
	case StatusWaiting:
		return targetStatus == StatusActive || targetStatus == StatusEnded
	case StatusActive:
		return targetStatus == StatusEnded
	}
	return false
}
