package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/muse"
)

// MuseRuntimeService manages accepted-work ownership independently of the HTTP
// request slot. A qualified provider transport is required before public wiring.
type MuseRuntimeService struct{ store muse.RuntimeStore }

func NewMuseRuntimeService(store muse.RuntimeStore) *MuseRuntimeService {
	return &MuseRuntimeService{store: store}
}

func (s *MuseRuntimeService) BindVerifiedWorkspace(ctx context.Context, identity muse.Identity) (*muse.Workspace, error) {
	return s.store.Bind(ctx, identity)
}

func (s *MuseRuntimeService) Reserve(ctx context.Context, input muse.Reservation) (*muse.Turn, muse.Lease, error) {
	var nonce [16]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return nil, muse.Lease{}, err
	}
	input.TurnID = "muse_turn_" + hex.EncodeToString(nonce[:])
	return s.store.Reserve(ctx, input)
}

// BeginSubmission must commit before invoking any provider-side effect.
func (s *MuseRuntimeService) BeginSubmission(ctx context.Context, lease muse.Lease, probeReference ...string) error {
	if len(probeReference) > 1 {
		return muse.ErrInvalid
	}
	reference := ""
	if len(probeReference) == 1 {
		reference = probeReference[0]
	}
	return s.store.Advance(ctx, lease, muse.Reserved, muse.Submitting, reference)
}

func (s *MuseRuntimeService) Advance(ctx context.Context, lease muse.Lease, from, to muse.State, providerTurnID string) error {
	return s.store.Advance(ctx, lease, from, to, providerTurnID)
}

func (s *MuseRuntimeService) Renew(ctx context.Context, lease muse.Lease, duration time.Duration) error {
	return s.store.Renew(ctx, lease, duration)
}

func (s *MuseRuntimeService) ClaimRecovery(ctx context.Context, workspaceID int64, worker string, duration time.Duration) (*muse.Turn, muse.Lease, error) {
	return s.store.ClaimRecovery(ctx, workspaceID, worker, duration)
}

func (s *MuseRuntimeService) GetTurn(ctx context.Context, id string, actor muse.Actor) (*muse.Turn, error) {
	return s.store.GetTurn(ctx, id, actor)
}
