package app_test

import (
	"context"
	"errors"
	"testing"

	"co-browsing-session-server/internal/domain/invitation"
	"co-browsing-session-server/internal/domain/roomsession"
	"co-browsing-session-server/internal/domain/serialnumber"
	"co-browsing-session-server/internal/infrastructure/memory"
	"co-browsing-session-server/internal/services/connection"
	"co-browsing-session-server/internal/services/hub"
	roomsessionsvc "co-browsing-session-server/internal/services/roomsession"
)

// connection.Coordinator 단위 테스트는 실제 인프라(memory)로 와이어링이 필요하다.
// services 레이어 테스트는 depguard 규칙상 infrastructure를 import할 수 없으므로,
// 모든 레이어를 조립할 수 있는 composition root(app)에서 검증한다.

type coordinatorFixture struct {
	coordinator           *connection.Coordinator
	roomSessionRepository roomsession.Repository
	invitationRepository  invitation.Repository
	roomSessionService    *roomsessionsvc.Service
}

func newCoordinatorFixture() coordinatorFixture {
	generator := serialnumber.NewRandomGenerator()
	roomSessionRepository := memory.NewRoomSessionRepository()
	invitationRepository := memory.NewInvitationRepository()
	roomSessionService := roomsessionsvc.NewService(roomSessionRepository, invitationRepository, generator)
	roomHub := hub.NewHub()

	return coordinatorFixture{
		coordinator:           connection.NewCoordinator(roomHub, invitationRepository, roomSessionService),
		roomSessionRepository: roomSessionRepository,
		invitationRepository:  invitationRepository,
		roomSessionService:    roomSessionService,
	}
}

func TestCoordinator_Resolve_ValidSerial(t *testing.T) {
	fixture := newCoordinatorFixture()
	createdRoomSession, createdInvitation, err := fixture.roomSessionService.Create(context.Background())
	if err != nil {
		t.Fatalf("create room: %v", err)
	}

	target, err := fixture.coordinator.Resolve(createdInvitation.Serial.String())
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if target.RoomID != createdRoomSession.ID.String() {
		t.Fatalf("RoomID 불일치: got %q want %q", target.RoomID, createdRoomSession.ID.String())
	}
	if target.Serial != createdInvitation.Serial.String() {
		t.Fatalf("Serial 불일치: got %q want %q", target.Serial, createdInvitation.Serial.String())
	}
}

func TestCoordinator_Resolve_UnknownSerial(t *testing.T) {
	fixture := newCoordinatorFixture()
	if _, err := fixture.coordinator.Resolve("ZZZZZZ"); !errors.Is(err, connection.ErrInvitationInvalid) {
		t.Fatalf("존재하지 않는 시리얼은 ErrInvitationInvalid여야 한다, got %v", err)
	}
}

func TestCoordinator_Resolve_EndedSession(t *testing.T) {
	fixture := newCoordinatorFixture()
	createdRoomSession, createdInvitation, err := fixture.roomSessionService.Create(context.Background())
	if err != nil {
		t.Fatalf("create room: %v", err)
	}

	// 세션을 ended로 만들되 Invitation은 남겨 둔다(방어적 분기 재현).
	stored, err := fixture.roomSessionRepository.Get(createdRoomSession.ID)
	if err != nil {
		t.Fatalf("get room session: %v", err)
	}
	if err := stored.Transition(roomsession.StatusEnded); err != nil {
		t.Fatalf("transition: %v", err)
	}
	if _, err := fixture.roomSessionRepository.Update(stored); err != nil {
		t.Fatalf("update: %v", err)
	}

	if _, err := fixture.coordinator.Resolve(createdInvitation.Serial.String()); !errors.Is(err, connection.ErrSessionEnded) {
		t.Fatalf("종료된 세션은 ErrSessionEnded여야 한다, got %v", err)
	}
}

func TestCoordinator_JoinActivatesAndLeaveEnds(t *testing.T) {
	fixture := newCoordinatorFixture()
	createdRoomSession, createdInvitation, err := fixture.roomSessionService.Create(context.Background())
	if err != nil {
		t.Fatalf("create room: %v", err)
	}
	roomID := createdRoomSession.ID.String()
	serial := createdInvitation.Serial.String()

	customer := &hub.Client{Role: hub.RoleCustomer, RoomID: roomID, Serial: serial}
	agent := &hub.Client{Role: hub.RoleAgent, RoomID: roomID, Serial: serial}

	// 고객 단독 접속 → peer 없음.
	if peer, err := fixture.coordinator.Join(customer); err != nil || peer != nil {
		t.Fatalf("고객 단독 Join은 (nil, nil)이어야 한다, got peer=%v err=%v", peer, err)
	}

	// 상담사 접속 → peer는 고객, 세션은 active로 전이.
	peer, err := fixture.coordinator.Join(agent)
	if err != nil {
		t.Fatalf("Join(agent): %v", err)
	}
	if peer != customer {
		t.Fatalf("상담사 Join의 peer는 고객이어야 한다")
	}
	activated, err := fixture.roomSessionService.Get(createdRoomSession.ID)
	if err != nil {
		t.Fatalf("get after activate: %v", err)
	}
	if activated.Status != roomsession.StatusActive {
		t.Fatalf("세션은 active여야 한다, got %q", activated.Status)
	}

	// 상담사 퇴장 → peer는 고객, 세션 ended + Invitation 삭제.
	leftPeer, err := fixture.coordinator.Leave(agent)
	if err != nil {
		t.Fatalf("Leave(agent): %v", err)
	}
	if leftPeer != customer {
		t.Fatalf("상담사 Leave의 peer는 고객이어야 한다")
	}
	ended, err := fixture.roomSessionService.Get(createdRoomSession.ID)
	if err != nil {
		t.Fatalf("get after end: %v", err)
	}
	if ended.Status != roomsession.StatusEnded {
		t.Fatalf("세션은 ended여야 한다, got %q", ended.Status)
	}
	if _, err := fixture.invitationRepository.ResolveBySerial(createdInvitation.Serial); !errors.Is(err, invitation.ErrNotFound) {
		t.Fatalf("종료 후 Invitation은 삭제되어야 한다, got %v", err)
	}
}
