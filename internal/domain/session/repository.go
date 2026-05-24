package session

import "co-browsing-session-server/internal/domain/serialnumber"

// Repository는 Session aggregate의 저장 계약(port)이다.
// 구현체는 infrastructure 레이어에 둔다.
type Repository interface {
	Create(s *Session) error
	Get(serial serialnumber.SerialNumber) (*Session, error)
	Save(s *Session) error
	Delete(serial serialnumber.SerialNumber) error
}
