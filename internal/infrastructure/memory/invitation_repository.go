package memory

import (
	"sync"
	"time"

	"co-browsing-session-server/internal/domain/invitation"
	"co-browsing-session-server/internal/domain/serialnumber"
)

type InvitationRepository struct {
	mu          sync.Mutex
	invitations map[serialnumber.SerialNumber]*invitation.Invitation
}

func NewInvitationRepository() *InvitationRepository {
	return &InvitationRepository{
		invitations: make(map[serialnumber.SerialNumber]*invitation.Invitation),
	}
}

func (r *InvitationRepository) Create(i *invitation.Invitation) (*invitation.Invitation, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exist := r.invitations[i.Serial]; exist {
		return nil, invitation.ErrAlreadyExists
	}
	r.invitations[i.Serial] = i
	return i, nil
}

func (r *InvitationRepository) ResolveBySerial(s serialnumber.SerialNumber) (*invitation.Invitation, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	inv, exist := r.invitations[s]
	if !exist {
		return nil, invitation.ErrNotFound
	}
	if inv.IsExpired(time.Now()) {
		delete(r.invitations, s)
		return nil, invitation.ErrExpired
	}
	return inv, nil
}

func (r *InvitationRepository) Delete(s serialnumber.SerialNumber) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exist := r.invitations[s]; !exist {
		return invitation.ErrNotFound
	}
	delete(r.invitations, s)
	return nil
}
