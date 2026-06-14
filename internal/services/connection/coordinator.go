package connection

import (
	"errors"
	"fmt"

	"co-browsing-session-server/internal/domain/invitation"
	"co-browsing-session-server/internal/domain/roomsession"
	"co-browsing-session-server/internal/domain/serialnumber"
	"co-browsing-session-server/internal/services/hub"
	rssvc "co-browsing-session-server/internal/services/roomsession"
)

// 유즈케이스 경계 에러 — interfaces 레이어가 트랜스포트 상태 코드로 변환한다.
// 도메인 에러 sentinel을 interfaces가 직접 알 필요가 없도록 여기서 한 번 변환한다.
var (
	ErrInvitationInvalid = errors.New("invitation not found or expired") // → 404
	ErrSessionEnded      = errors.New("session already ended")           // → 410
)

// Target은 Resolve가 돌려주는, 업그레이드 후 Client 생성에 필요한 식별자다.
// RoomID/Serial은 트랜스포트로 넘어가므로 opaque string으로 노출한다.
type Target struct {
	RoomID string
	Serial string
}

// Coordinator는 라이브 연결(Hub)과 영속 상태(RoomSession + Invitation)를 묶어
// 참가자의 접속/해제 유즈케이스를 오케스트레이션한다.
// 영속 상태만 다루는 roomsession.Service와 달리, 휘발 Hub까지 함께 조율하는 상위 흐름이다.
type Coordinator struct {
	hub                  *hub.Hub
	invitationRepository invitation.Repository
	roomSessionService   *rssvc.Service
}

func NewCoordinator(
	roomHub *hub.Hub,
	invitationRepository invitation.Repository,
	roomSessionService *rssvc.Service,
) *Coordinator {
	return &Coordinator{
		hub:                  roomHub,
		invitationRepository: invitationRepository,
		roomSessionService:   roomSessionService,
	}
}

// Resolve는 업그레이드 전에 시리얼을 검증하고 접속 대상을 돌려준다.
// 잘못된/만료된 초대는 ErrInvitationInvalid, 이미 종료된 세션은 ErrSessionEnded를 반환한다.
func (coordinator *Coordinator) Resolve(serial string) (Target, error) {
	resolvedInvitation, err := coordinator.invitationRepository.ResolveBySerial(serialnumber.SerialNumber(serial))
	if err != nil {
		if errors.Is(err, invitation.ErrNotFound) || errors.Is(err, invitation.ErrExpired) {
			return Target{}, ErrInvitationInvalid
		}
		return Target{}, fmt.Errorf("resolve invitation: %w", err)
	}

	// 불필요한 업그레이드를 막는 가벼운 사전 검증.
	roomSession, err := coordinator.roomSessionService.Get(resolvedInvitation.RoomID)
	if err != nil {
		if errors.Is(err, roomsession.ErrNotFound) || errors.Is(err, roomsession.ErrExpired) {
			return Target{}, ErrInvitationInvalid
		}
		return Target{}, fmt.Errorf("get room session: %w", err)
	}
	if roomSession.Status == roomsession.StatusEnded {
		return Target{}, ErrSessionEnded
	}

	return Target{
		RoomID: resolvedInvitation.RoomID.String(),
		Serial: resolvedInvitation.Serial.String(),
	}, nil
}

// Join은 클라이언트를 Hub에 등록한다. 양쪽이 모두 모이면 세션을 active로 전이시킨다.
// 반환값은 알림 대상(대기 중이던 상대방)이며, 아직 미접속이면 nil이다.
// active 전이 실패는 치명적이지 않으므로 peer는 그대로 반환하고 에러를 함께 돌려준다(호출자가 로깅).
func (coordinator *Coordinator) Join(client *hub.Client) (peer *hub.Client, err error) {
	peer = coordinator.hub.JoinRoom(client.RoomID, client)
	if peer == nil {
		return nil, nil
	}
	if activateErr := coordinator.roomSessionService.Activate(roomsession.RoomID(client.RoomID)); activateErr != nil {
		return peer, fmt.Errorf("activate room session: %w", activateErr)
	}
	return peer, nil
}

// Leave는 클라이언트를 Hub에서 제거한다. 첫 disconnect라면 세션을 종료(ended 전이 + Invitation 삭제)한다.
// 반환값은 알림 대상(상대방)이며, 없으면 nil이다.
func (coordinator *Coordinator) Leave(client *hub.Client) (peer *hub.Client, err error) {
	peer = coordinator.hub.LeaveRoom(client)
	if peer == nil {
		return nil, nil
	}
	if endErr := coordinator.roomSessionService.End(
		roomsession.RoomID(client.RoomID),
		serialnumber.SerialNumber(client.Serial),
	); endErr != nil {
		return peer, fmt.Errorf("end room session: %w", endErr)
	}
	return peer, nil
}

// Peer는 메시지 릴레이 대상(상대방)을 반환한다. 없으면 nil이다.
func (coordinator *Coordinator) Peer(client *hub.Client) *hub.Client {
	return coordinator.hub.GetPeer(client)
}
