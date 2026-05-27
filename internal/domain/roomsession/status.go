package roomsession

type Status string

const (
	StatusWaiting Status = "waiting"
	StatusActive  Status = "active"
	StatusEnded   Status = "ended"
)

func (s Status) IsValid() bool {
	switch s {
	case StatusWaiting, StatusActive, StatusEnded:
		return true
	}
	return false
}

func (s Status) CanTransitionTo(to Status) bool {
	if !to.IsValid() {
		return false
	}
	switch s {
	case StatusWaiting:
		return to == StatusActive || to == StatusEnded
	case StatusActive:
		return to == StatusEnded
	}
	return false
}
