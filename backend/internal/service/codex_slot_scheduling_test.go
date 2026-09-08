package service

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

type codexConversationTestRepo struct {
	*codexProfileGatewayAccountRepo
	mu      sync.Mutex
	slots   []CodexResolvedDeviceSlot
	pins    map[string]CodexResolvedDeviceSlot
	bindErr error
	winner  *CodexResolvedDeviceSlot
}

func (r *codexConversationTestRepo) FindCodexConversationBinding(_ context.Context, accountID, keyID int64, os CodexOSClass, surface CodexClientSurface, hash string) (*CodexResolvedDeviceSlot, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	slot, ok := r.pins[fmt.Sprintf("%d/%d/%s/%s/%s", accountID, keyID, os, surface, hash)]
	if !ok {
		return nil, nil
	}
	return &slot, nil
}
func (r *codexConversationTestRepo) BindCodexConversationSlot(_ context.Context, accountID, keyID int64, os CodexOSClass, surface CodexClientSurface, hash string, slotID int64) (*CodexResolvedDeviceSlot, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.bindErr != nil {
		return nil, r.bindErr
	}
	if r.winner != nil {
		result := *r.winner
		return &result, nil
	}
	key := fmt.Sprintf("%d/%d/%s/%s/%s", accountID, keyID, os, surface, hash)
	if slot, ok := r.pins[key]; ok {
		return &slot, nil
	}
	for _, slot := range r.slots {
		if slot.SlotID == slotID {
			slot.APIKeyID = keyID
			slot.BindingID = slotID + 100
			if r.pins == nil {
				r.pins = make(map[string]CodexResolvedDeviceSlot)
			}
			r.pins[key] = slot
			return &slot, nil
		}
	}
	return nil, errors.New("missing slot")
}
func (r *codexConversationTestRepo) RefreshCodexConversationBinding(context.Context, int64) error {
	return nil
}
func (r *codexConversationTestRepo) ListCodexDeviceSlots(context.Context, int64, CodexOSClass, CodexClientSurface, bool) ([]CodexResolvedDeviceSlot, error) {
	return r.slots, nil
}

func codexSlotSchedulingFixture(t *testing.T) (*OpenAIGatewayService, *Account, *codexConversationTestRepo, *CodexResolvedDeviceSlot) {
	t.Helper()
	account := codexProfileTestAccount(t, 201, CodexOSWindows, CodexSurfaceDesktop, CodexArchX8664, false)
	account.CodexIdentityPolicy.SessionPolicy = CodexSessionPolicySpec{Mode: CodexSessionDeviceShared, MaxActiveConversationsPerSlot: 1, DisableCrossKeyContinuation: true}
	slots := make([]CodexResolvedDeviceSlot, 3)
	for i := range slots {
		slots[i] = CodexResolvedDeviceSlot{AccountID: account.ID, APIKeyID: 101, ProfileID: 301, SlotID: int64(401 + i), OSClass: CodexOSWindows, CanonicalSurface: CodexSurfaceDesktop, Architecture: CodexArchX8664, CatalogVersion: 1, SlotIndex: i, Epoch: 4, State: "active", PolicyVersion: 1}
	}
	slots[1].ClientVersionMode = CodexClientVersionPinned
	slots[1].ClientVersion = "0.200.1"
	proxy := &Proxy{ID: 701}
	slots[1].ProxyID = &proxy.ID
	slots[1].Proxy = proxy
	repo := &codexConversationTestRepo{slots: slots, codexProfileGatewayAccountRepo: &codexProfileGatewayAccountRepo{resolvedSlots: map[int64]*CodexResolvedDeviceSlot{account.ID: &slots[0]}}}
	svc := &OpenAIGatewayService{accountRepo: repo, concurrencyService: NewConcurrencyService(&codexDeviceLeaseCache{})}
	return svc, account, repo, &slots[0]
}

func TestCodexSlotSchedulingPinsConversationsAndUsesIdleSlot(t *testing.T) {
	svc, account, repo, preferred := codexSlotSchedulingFixture(t)
	ctx := context.Background()
	request := codexProfileRequest{ConversationHash: "A"}
	one, leaseA, err := svc.acquireCodexConversationSlot(ctx, account, request, preferred, nil)
	require.NoError(t, err)
	defer leaseA.Release()
	require.Equal(t, int64(401), one.SlotID)
	request.ConversationHash = "B"
	two, leaseB, err := svc.acquireCodexConversationSlot(ctx, account, request, preferred, nil)
	require.NoError(t, err)
	defer leaseB.Release()
	require.Equal(t, int64(402), two.SlotID)
	require.Equal(t, "0.200.1", two.ClientVersion)
	require.Equal(t, int64(701), two.Proxy.ID)
	// Existing A must not move to free slot 3 merely because its slot is full.
	_, _, err = svc.acquireCodexConversationSlot(ctx, account, codexProfileRequest{ConversationHash: "A"}, preferred, nil)
	require.ErrorIs(t, err, ErrCodexDeviceSessionBusy)
	// A different key never inherits B's conversation pin.
	other := *preferred
	other.APIKeyID = 202
	three, leaseC, err := svc.acquireCodexConversationSlot(ctx, account, request, &other, nil)
	require.NoError(t, err)
	defer leaseC.Release()
	require.Equal(t, int64(403), three.SlotID)
	_, _, err = svc.acquireCodexConversationSlot(ctx, account, codexProfileRequest{ConversationHash: "D"}, preferred, nil)
	require.ErrorIs(t, err, ErrCodexDeviceSessionBusy)
	leaseA.Release()
	leaseB.Release()
	again, lease, err := svc.acquireCodexConversationSlot(ctx, account, request, preferred, []byte(`{"previous_response_id":"resp_b"}`))
	require.NoError(t, err)
	defer lease.Release()
	require.Equal(t, two.SlotID, again.SlotID)
	require.Equal(t, int64(401), repo.resolvedSlots[account.ID].SlotID, "key preference must not change")
}

func TestCodexSlotSchedulingDoesNotGuessContinuationOrDeviceFamily(t *testing.T) {
	svc, account, repo, preferred := codexSlotSchedulingFixture(t)
	ctx := context.Background()
	_, hold, err := svc.acquireCodexConversationSlot(ctx, account, codexProfileRequest{}, preferred, nil)
	require.NoError(t, err)
	defer hold.Release()
	for _, tc := range []struct{ hash, body string }{{"", ""}, {"new", `{"previous_response_id":"resp_unknown"}`}} {
		_, _, err := svc.acquireCodexConversationSlot(ctx, account, codexProfileRequest{ConversationHash: tc.hash}, preferred, []byte(tc.body))
		require.ErrorIs(t, err, ErrCodexDeviceSessionBusy)
	}
	for _, change := range []func(*CodexResolvedDeviceSlot){
		func(s *CodexResolvedDeviceSlot) { s.CanonicalSurface = CodexSurfaceCLI },
		func(s *CodexResolvedDeviceSlot) { s.Architecture = CodexArchARM64 },
		func(s *CodexResolvedDeviceSlot) { s.Epoch++ },
		func(s *CodexResolvedDeviceSlot) { s.State = "draining" },
		func(s *CodexResolvedDeviceSlot) { s.ProfileID++ },
		func(s *CodexResolvedDeviceSlot) { s.AccountID++ },
	} {
		candidate := *preferred
		candidate.SlotID = 402
		change(&candidate)
		repo.slots = []CodexResolvedDeviceSlot{*preferred, candidate}
		_, _, err := svc.acquireCodexConversationSlot(ctx, account, codexProfileRequest{ConversationHash: "new"}, preferred, nil)
		require.ErrorIs(t, err, ErrCodexDeviceSessionBusy)
	}
}

func TestCodexSlotSchedulingReleasesLosingOrFailedLease(t *testing.T) {
	for _, race := range []bool{false, true} {
		t.Run(fmt.Sprint(race), func(t *testing.T) {
			svc, account, repo, preferred := codexSlotSchedulingFixture(t)
			ctx := context.Background()
			if race {
				repo.winner = &repo.slots[1]
				hold, ok, err := svc.concurrencyService.AcquireCodexDeviceConversationLease(ctx, "402", 1)
				require.NoError(t, err)
				require.True(t, ok)
				defer hold.Release()
			} else {
				repo.bindErr = errors.New("injected binding failure")
			}
			_, _, err := svc.acquireCodexConversationSlot(ctx, account, codexProfileRequest{ConversationHash: "new"}, preferred, nil)
			require.Error(t, err)
			lease, ok, err := svc.concurrencyService.AcquireCodexDeviceConversationLease(ctx, "401", 1)
			require.NoError(t, err)
			require.True(t, ok, "temporary candidate lease must be released")
			lease.Release()
		})
	}
}

func TestCodexSlotSchedulingGatewayUsesSelectedVersionAndProxy(t *testing.T) {
	svc, account, _, _ := codexSlotSchedulingFixture(t)
	bodyA := []byte(`{"model":"gpt-5","prompt_cache_key":"A"}`)
	bodyB := []byte(`{"model":"gpt-5","prompt_cache_key":"B"}`)
	cA := newCodexProfileGatewayContext(t, 7, 101, bodyA)
	svc.GenerateSessionHash(cA, bodyA)
	_, err := svc.PrepareCodexProfileAttempt(cA.Request.Context(), cA, account, bodyA)
	require.NoError(t, err)
	defer svc.ReleaseCodexProfileAttempt(cA, account)
	cB := newCodexProfileGatewayContext(t, 7, 101, bodyB)
	svc.GenerateSessionHash(cB, bodyB)
	prepared, err := svc.PrepareCodexProfileAttempt(cB.Request.Context(), cB, account, bodyB)
	require.NoError(t, err)
	defer svc.ReleaseCodexProfileAttempt(cB, account)
	plan := stagedCodexIdentityAttemptPlan(cB, account)
	require.Equal(t, 1, plan.Slot.Index)
	require.Equal(t, "0.200.1", plan.Profile.Version)
	require.Equal(t, int64(701), *prepared.ProxyID)
}
