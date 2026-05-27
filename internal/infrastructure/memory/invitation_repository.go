package memory

import (
	"sync"
	"time"

	"co-browsing-session-server/internal/domain/invitation"
	"co-browsing-session-server/internal/domain/serialnumber"
)

type InvitationRepository struct {
	mutex       sync.Mutex
	invitations map[serialnumber.SerialNumber]*invitation.Invitation
}

func NewInvitationRepository() *InvitationRepository {
	return &InvitationRepository{
		invitations: make(map[serialnumber.SerialNumber]*invitation.Invitation),
	}
}

func (invitationRepository *InvitationRepository) Create(newInvitation *invitation.Invitation) (*invitation.Invitation, error) {
	invitationRepository.mutex.Lock()
	defer invitationRepository.mutex.Unlock()

	if _, exists := invitationRepository.invitations[newInvitation.Serial]; exists {
		return nil, invitation.ErrAlreadyExists
	}
	invitationRepository.invitations[newInvitation.Serial] = newInvitation
	return newInvitation, nil
}

func (invitationRepository *InvitationRepository) ResolveBySerial(serial serialnumber.SerialNumber) (*invitation.Invitation, error) {
	invitationRepository.mutex.Lock()
	defer invitationRepository.mutex.Unlock()

	storedInvitation, exists := invitationRepository.invitations[serial]
	if !exists {
		return nil, invitation.ErrNotFound
	}
	if storedInvitation.IsExpired(time.Now()) {
		delete(invitationRepository.invitations, serial)
		return nil, invitation.ErrExpired
	}
	return storedInvitation, nil
}

func (invitationRepository *InvitationRepository) Delete(serial serialnumber.SerialNumber) error {
	invitationRepository.mutex.Lock()
	defer invitationRepository.mutex.Unlock()

	if _, exists := invitationRepository.invitations[serial]; !exists {
		return invitation.ErrNotFound
	}
	delete(invitationRepository.invitations, serial)
	return nil
}
