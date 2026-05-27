package roomsession

// Repository는 RoomSession aggregate의 저장 계약(port)이다.
// Get은 read-on-check — 만료된 세션을 발견하면 삭제 후 ErrExpired를 반환한다.
type Repository interface {
	Create(s *RoomSession) (*RoomSession, error)
	Get(id RoomID) (*RoomSession, error)
	Update(s *RoomSession) (*RoomSession, error)
	Delete(id RoomID) error
}
