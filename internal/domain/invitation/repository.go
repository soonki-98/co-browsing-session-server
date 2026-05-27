package invitation

import "co-browsing-session-server/internal/domain/serialnumber"

// Repository는 Invitation aggregate의 저장 계약(port)이다.
// ResolveBySerial은 read-on-check — 만료된 항목을 발견하면 삭제 후 ErrExpired를 반환한다.
type Repository interface {
	Create(i *Invitation) (*Invitation, error)
	ResolveBySerial(s serialnumber.SerialNumber) (*Invitation, error)
	Delete(s serialnumber.SerialNumber) error
}
