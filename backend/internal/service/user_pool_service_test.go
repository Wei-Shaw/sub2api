//go:build unit

package service

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ── Stubs ─────────────────────────────────────────────────────────────────────

type poolRepoStub struct {
	mu sync.Mutex

	// Create
	createPool Pool
	createErr  error

	// GetByID
	getPool Pool
	getErr  error

	// Update
	updatePool Pool
	updateErr  error

	// SoftDelete
	deleteErr error

	// AddMembers
	addAdded   []int64
	addSkipped []int64
	addErr     error

	// RemoveMembers
	removeRemoved []int64
	removeErr     error

	// ListMembers
	members    []PoolMember
	membersErr error

	// MemberCount
	memberCount int

	// ReplaceGroupGrants
	replaceErr error

	// DeleteGroupGrant
	deleteGrantErr error

	// ListGroupGrants
	grants    []PoolGroupGrant
	grantsErr error

	// GetUserPools
	userPools    []Pool
	userPoolsErr error

	// GetUserPoolsBatch
	userPoolsBatch    map[int64][]Pool
	userPoolsBatchErr error

	// ListGroupGrantsBatch
	grantsBatch    map[int64][]PoolGroupGrant
	grantsBatchErr error
}

func (s *poolRepoStub) Create(_ context.Context, pool Pool) (Pool, error) {
	return s.createPool, s.createErr
}

func (s *poolRepoStub) List(_ context.Context, _ ListPoolsOptions) ([]Pool, int, error) {
	return nil, 0, nil
}

func (s *poolRepoStub) GetByID(_ context.Context, _ int64) (Pool, error) {
	return s.getPool, s.getErr
}

func (s *poolRepoStub) Update(_ context.Context, _ int64, pool Pool) (Pool, error) {
	return s.updatePool, s.updateErr
}

func (s *poolRepoStub) SoftDelete(_ context.Context, _ int64) error {
	return s.deleteErr
}

func (s *poolRepoStub) AddMembers(_ context.Context, _ int64, _ []int64) ([]int64, []int64, error) {
	return s.addAdded, s.addSkipped, s.addErr
}

func (s *poolRepoStub) RemoveMembers(_ context.Context, _ int64, _ []int64) ([]int64, error) {
	return s.removeRemoved, s.removeErr
}

func (s *poolRepoStub) ListMembers(_ context.Context, _ int64, _ ListMembersOptions) ([]PoolMember, int, error) {
	return s.members, len(s.members), s.membersErr
}

func (s *poolRepoStub) ListAllMemberIDs(_ context.Context, _ int64) ([]int64, error) {
	ids := make([]int64, len(s.members))
	for i, m := range s.members {
		ids[i] = m.UserID
	}
	return ids, s.membersErr
}

func (s *poolRepoStub) MemberCount(_ context.Context, _ int64) (int, error) {
	return s.memberCount, nil
}

func (s *poolRepoStub) ReplaceGroupGrants(_ context.Context, _ int64, grants []PoolGroupGrant) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.replaceErr == nil {
		s.grants = grants
	}
	return s.replaceErr
}

func (s *poolRepoStub) DeleteGroupGrant(_ context.Context, _ int64, _ int64) error {
	return s.deleteGrantErr
}

func (s *poolRepoStub) ListGroupGrants(_ context.Context, _ int64) ([]PoolGroupGrant, error) {
	return s.grants, s.grantsErr
}

func (s *poolRepoStub) GetUserPools(_ context.Context, _ int64) ([]Pool, error) {
	return s.userPools, s.userPoolsErr
}

func (s *poolRepoStub) GetUserPoolsBatch(_ context.Context, _ []int64) (map[int64][]Pool, error) {
	return s.userPoolsBatch, s.userPoolsBatchErr
}

func (s *poolRepoStub) ListGroupGrantsBatch(_ context.Context, _ []int64) (map[int64][]PoolGroupGrant, error) {
	return s.grantsBatch, s.grantsBatchErr
}

// poolOutboxStub captures enqueued events and implements CacheInvalidationOutboxRepository.
type poolOutboxStub struct {
	mu     sync.Mutex
	events []CacheInvalidationEvent
	err    error
}

func (s *poolOutboxStub) Enqueue(_ context.Context, event CacheInvalidationEvent) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events = append(s.events, event)
	return s.err
}

func (s *poolOutboxStub) ClaimReady(_ context.Context, _ string, _ int, _ time.Duration) ([]CacheInvalidationEvent, error) {
	return nil, nil
}

func (s *poolOutboxStub) MarkSucceeded(_ context.Context, _ int64) error { return nil }

func (s *poolOutboxStub) MarkFailed(_ context.Context, _ int64, _ error, _ time.Time) error {
	return nil
}

func (s *poolOutboxStub) MarkDead(_ context.Context, _ int64, _ error) error { return nil }

func (s *poolOutboxStub) RequeueStaleProcessing(_ context.Context, _ time.Time) (int64, error) {
	return 0, nil
}

// ── mockTransactionRunner ─────────────────────────────────────────────────────

// mockTransactionRunner executes fn synchronously with the same ctx (no real DB tx).
// Sufficient for unit tests where stubs do not depend on tx context propagation.
type mockTransactionRunner struct{}

func (m *mockTransactionRunner) RunInTx(ctx context.Context, fn func(context.Context) error) error {
	return fn(ctx)
}

// ── Test factory ──────────────────────────────────────────────────────────────

func newPoolSvc(repo *poolRepoStub, outbox *poolOutboxStub) *UserPoolService {
	return &UserPoolService{
		repo:       repo,
		outboxRepo: outbox,
		txRunner:   &mockTransactionRunner{},
	}
}

// ── Tests ─────────────────────────────────────────────────────────────────────

// TestUserPoolService_Create_EmptyName returns error for empty name.
func TestUserPoolService_Create_EmptyName(t *testing.T) {
	svc := newPoolSvc(&poolRepoStub{}, &poolOutboxStub{})
	_, err := svc.Create(context.Background(), "   ", "", "active")
	require.Error(t, err, "whitespace name must fail")
}

// TestUserPoolService_Create_OK delegates to repo on valid input.
func TestUserPoolService_Create_OK(t *testing.T) {
	expected := Pool{ID: 1, Name: "poolA", Status: "active"}
	repo := &poolRepoStub{createPool: expected}
	svc := newPoolSvc(repo, &poolOutboxStub{})

	got, err := svc.Create(context.Background(), "poolA", "", "active")
	require.NoError(t, err)
	assert.Equal(t, expected, got)
}

// TestUserPoolService_ReplaceGroupGrants_DuplicateGroup returns ErrDuplicateGrantGroup.
func TestUserPoolService_ReplaceGroupGrants_DuplicateGroup(t *testing.T) {
	svc := newPoolSvc(&poolRepoStub{}, &poolOutboxStub{})
	grants := []PoolGroupGrant{{GroupID: 5}, {GroupID: 5}}
	err := svc.ReplaceGroupGrants(context.Background(), 1, grants)
	require.ErrorIs(t, err, ErrDuplicateGrantGroup)
}

// TestUserPoolService_ReplaceGroupGrants_InvalidRate returns ErrPoolGrantRateInvalid.
func TestUserPoolService_ReplaceGroupGrants_InvalidRate(t *testing.T) {
	svc := newPoolSvc(&poolRepoStub{}, &poolOutboxStub{})
	neg := float64(-0.1)
	err := svc.ReplaceGroupGrants(context.Background(), 1, []PoolGroupGrant{{GroupID: 5, RateMultiplier: &neg}})
	require.ErrorIs(t, err, ErrPoolGrantRateInvalid)
}

// TestUserPoolService_ReplaceGroupGrants_InvalidRPM returns ErrPoolGrantRPMInvalid.
func TestUserPoolService_ReplaceGroupGrants_InvalidRPM(t *testing.T) {
	svc := newPoolSvc(&poolRepoStub{}, &poolOutboxStub{})
	neg := -1
	err := svc.ReplaceGroupGrants(context.Background(), 1, []PoolGroupGrant{{GroupID: 5, RPMOverride: &neg}})
	require.ErrorIs(t, err, ErrPoolGrantRPMInvalid)
}

// TestUserPoolService_ReplaceGroupGrants_ZeroRate rate_multiplier=0 also invalid.
func TestUserPoolService_ReplaceGroupGrants_ZeroRate(t *testing.T) {
	svc := newPoolSvc(&poolRepoStub{}, &poolOutboxStub{})
	zero := float64(0)
	err := svc.ReplaceGroupGrants(context.Background(), 1, []PoolGroupGrant{{GroupID: 5, RateMultiplier: &zero}})
	require.ErrorIs(t, err, ErrPoolGrantRateInvalid, "rate_multiplier=0 must be invalid")
}

// TestUserPoolService_ReplaceGroupGrants_DisabledGroup repo error propagates.
func TestUserPoolService_ReplaceGroupGrants_DisabledGroup(t *testing.T) {
	repo := &poolRepoStub{replaceErr: ErrPoolGrantGroupDisabled}
	svc := newPoolSvc(repo, &poolOutboxStub{})
	err := svc.ReplaceGroupGrants(context.Background(), 1, []PoolGroupGrant{{GroupID: 5}})
	require.ErrorIs(t, err, ErrPoolGrantGroupDisabled)
}

// TestUserPoolService_ReplaceGroupGrants_PublicGroup repo error propagates.
func TestUserPoolService_ReplaceGroupGrants_PublicGroup(t *testing.T) {
	repo := &poolRepoStub{replaceErr: ErrPoolGrantPublicGroupNotAllowed}
	svc := newPoolSvc(repo, &poolOutboxStub{})
	err := svc.ReplaceGroupGrants(context.Background(), 1, []PoolGroupGrant{{GroupID: 5}})
	require.ErrorIs(t, err, ErrPoolGrantPublicGroupNotAllowed)
}

// TestUserPoolService_RemoveMembers_EnqueuesStrictInvalidation outbox is populated.
func TestUserPoolService_RemoveMembers_EnqueuesStrictInvalidation(t *testing.T) {
	repo := &poolRepoStub{
		removeRemoved: []int64{10, 20},
		grants:        []PoolGroupGrant{{PoolID: 1, GroupID: 5}},
	}
	outbox := &poolOutboxStub{}
	svc := newPoolSvc(repo, outbox)

	removed, err := svc.RemoveMembers(context.Background(), 1, []int64{10, 20})
	require.NoError(t, err)
	assert.Equal(t, []int64{10, 20}, removed)

	outbox.mu.Lock()
	n := len(outbox.events)
	outbox.mu.Unlock()
	assert.Greater(t, n, 0, "outbox must have events for member removal with active grants")
}

// TestUserPoolService_Update_Disable_EnqueuesInvalidation pool disable triggers outbox.
func TestUserPoolService_Update_Disable_EnqueuesInvalidation(t *testing.T) {
	repo := &poolRepoStub{
		getPool:    Pool{ID: 1, Name: "p", Status: "active"},
		updatePool: Pool{ID: 1, Name: "p", Status: "disabled"},
		members:    []PoolMember{{PoolID: 1, UserID: 99}},
		grants:     []PoolGroupGrant{{PoolID: 1, GroupID: 5}},
	}
	outbox := &poolOutboxStub{}
	svc := newPoolSvc(repo, outbox)

	_, err := svc.Update(context.Background(), 1, "p", "", "disabled")
	require.NoError(t, err)

	outbox.mu.Lock()
	n := len(outbox.events)
	outbox.mu.Unlock()
	assert.Greater(t, n, 0, "disabling a pool with members+grants must enqueue cache invalidation")
}

// TestUserPoolService_Update_NoStatusChange_NoOutbox status unchanged means no outbox.
func TestUserPoolService_Update_NoStatusChange_NoOutbox(t *testing.T) {
	repo := &poolRepoStub{
		getPool:    Pool{ID: 1, Name: "p", Status: "active"},
		updatePool: Pool{ID: 1, Name: "p-new", Status: "active"},
	}
	outbox := &poolOutboxStub{}
	svc := newPoolSvc(repo, outbox)

	_, err := svc.Update(context.Background(), 1, "p-new", "", "active")
	require.NoError(t, err)

	outbox.mu.Lock()
	n := len(outbox.events)
	outbox.mu.Unlock()
	assert.Equal(t, 0, n, "status unchanged must not enqueue outbox events")
}

// TestUserPoolService_DeleteGroupGrant_InvalidGroupID returns ErrGroupGrantNotFound.
func TestUserPoolService_DeleteGroupGrant_InvalidGroupID(t *testing.T) {
	svc := newPoolSvc(&poolRepoStub{}, &poolOutboxStub{})
	err := svc.DeleteGroupGrant(context.Background(), 1, 0)
	require.ErrorIs(t, err, ErrGroupGrantNotFound)
}

// TestUserPoolService_RPM_Tightening_Detection verifies rpmTightened helper.
func TestUserPoolService_RPM_Tightening_Detection(t *testing.T) {
	cases := []struct {
		name     string
		old, new *int
		want     bool
	}{
		{"both nil", nil, nil, false},
		{"nil→0 (unlimited)", nil, poolIntPtr(0), false},
		{"nil→100 (restrictive)", nil, poolIntPtr(100), true},
		{"100→50 (tighter)", poolIntPtr(100), poolIntPtr(50), true},
		{"50→100 (looser)", poolIntPtr(50), poolIntPtr(100), false},
		{"50→nil (unlimited)", poolIntPtr(50), nil, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, rpmTightened(tc.old, tc.new))
		})
	}
}

func poolIntPtr(v int) *int { return &v }

// ── userRepoStub for AddMembersByFilter tests ────────────────────────────────

// poolTestUserRepo embeds userRepoStub and overrides ListWithFilters for AddMembersByFilter tests.
type poolTestUserRepo struct {
	userRepoStub
	calls int
	pages [][]User // page i returns pages[i]
	total int64
	err   error
}

func (r *poolTestUserRepo) ListWithFilters(_ context.Context, params pagination.PaginationParams, _ UserListFilters) ([]User, *pagination.PaginationResult, error) {
	r.calls++
	if r.err != nil {
		return nil, nil, r.err
	}
	idx := params.Page - 1
	var out []User
	if idx >= 0 && idx < len(r.pages) {
		out = r.pages[idx]
	}
	return out, &pagination.PaginationResult{Total: r.total, Page: params.Page, PageSize: params.PageSize}, nil
}

// ── TestAddMembersByFilter_TotalExceedsCap ────────────────────────────────────

// TestAddMembersByFilter_TotalExceedsCap verifies that when Total > 100000 the function
// returns an error after only one ListWithFilters call (no pagination loop).
func TestAddMembersByFilter_TotalExceedsCap(t *testing.T) {
	userRepo := &poolTestUserRepo{
		total: 200_000,
		pages: [][]User{{} /* page1 returns empty slice */},
	}
	repo := &poolRepoStub{
		grants: []PoolGroupGrant{{PoolID: 1, GroupID: 5}},
	}
	outbox := &poolOutboxStub{}
	svc := &UserPoolService{repo: repo, userRepo: userRepo, outboxRepo: outbox, txRunner: &mockTransactionRunner{}}

	added, skipped, matched, err := svc.AddMembersByFilter(context.Background(), 1, UserListFilters{Status: "active"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "matched too many users")
	assert.Equal(t, 0, added)
	assert.Equal(t, 0, skipped)
	assert.Equal(t, 200_000, matched)
	assert.Equal(t, 1, userRepo.calls, "ListWithFilters should be called exactly once")
}

// ── TestAddMembersByFilter_InvalidationNoExistingPool ────────────────────────

// TestAddMembersByFilter_InvalidationNoExistingPool verifies that users with no existing
// pool membership get an outbox event for every grant.
func TestAddMembersByFilter_InvalidationNoExistingPool(t *testing.T) {
	// Users 10 and 20; neither has existing pools.
	userRepo := &poolTestUserRepo{
		total: 2,
		pages: [][]User{{
			{ID: 10},
			{ID: 20},
		}},
	}
	repo := &poolRepoStub{
		addAdded: []int64{10, 20},
		grants:   []PoolGroupGrant{{PoolID: 1, GroupID: 5}, {PoolID: 1, GroupID: 6}},
		// GetUserPoolsBatch returns empty map → no existing pools.
		userPoolsBatch: map[int64][]Pool{},
		grantsBatch:    map[int64][]PoolGroupGrant{},
	}
	outbox := &poolOutboxStub{}
	svc := &UserPoolService{repo: repo, userRepo: userRepo, outboxRepo: outbox, txRunner: &mockTransactionRunner{}}

	added, skipped, matched, err := svc.AddMembersByFilter(context.Background(), 1, UserListFilters{Status: "active"})
	require.NoError(t, err)
	assert.Equal(t, 2, added)
	assert.Equal(t, 0, skipped)
	assert.Equal(t, 2, matched)

	outbox.mu.Lock()
	events := outbox.events
	outbox.mu.Unlock()

	// Should have 2 events: one per grant (groupID 5 and 6), each covering both users.
	assert.Equal(t, 2, len(events), "one outbox event per grant")
	for _, ev := range events {
		assert.Len(t, ev.Payload.AffectedUserIDs, 2)
	}
}

// ── TestAddMembersByFilter_InvalidationCorrectness ────────────────────────────

// TestAddMembersByFilter_InvalidationCorrectness verifies the minimum pool_id strategy:
// user already in pool 10 (higher ID) should trigger enqueue when current pool is 1 (lower ID).
func TestAddMembersByFilter_InvalidationCorrectness(t *testing.T) {
	// User 99 is already in pool 10 which has groupID 5.
	// Current pool is ID=1. Pool 1 < pool 10 → enqueue because current pool becomes effective.
	userRepo := &poolTestUserRepo{
		total: 1,
		pages: [][]User{{
			{ID: 99},
		}},
	}
	repo := &poolRepoStub{
		addAdded: []int64{99},
		grants:   []PoolGroupGrant{{PoolID: 1, GroupID: 5}},
		userPoolsBatch: map[int64][]Pool{
			99: {{ID: 10, Status: "active"}}, // user 99 is in pool 10
		},
		grantsBatch: map[int64][]PoolGroupGrant{
			10: {{PoolID: 10, GroupID: 5}}, // pool 10 grants groupID 5
		},
	}
	outbox := &poolOutboxStub{}
	svc := &UserPoolService{repo: repo, userRepo: userRepo, outboxRepo: outbox, txRunner: &mockTransactionRunner{}}

	_, _, _, err := svc.AddMembersByFilter(context.Background(), 1, UserListFilters{Status: "active"})
	require.NoError(t, err)

	outbox.mu.Lock()
	n := len(outbox.events)
	outbox.mu.Unlock()

	// Pool 1 < pool 10 → should enqueue (effective rate may change).
	assert.Equal(t, 1, n, "should enqueue: current pool ID 1 < existing pool ID 10")
}

// TestAddMembersByFilter_InvalidationNoEnqueueWhenPoolIDHigher verifies that when
// current pool has higher ID than existing pool, no enqueue happens for covered grants.
func TestAddMembersByFilter_InvalidationNoEnqueueWhenPoolIDHigher(t *testing.T) {
	// User 99 is already in pool 1 (lower ID). Current pool is ID=10. No enqueue expected.
	userRepo := &poolTestUserRepo{
		total: 1,
		pages: [][]User{{
			{ID: 99},
		}},
	}
	repo := &poolRepoStub{
		addAdded: []int64{99},
		grants:   []PoolGroupGrant{{PoolID: 10, GroupID: 5}},
		userPoolsBatch: map[int64][]Pool{
			99: {{ID: 1, Status: "active"}}, // user 99 is in pool 1
		},
		grantsBatch: map[int64][]PoolGroupGrant{
			1: {{PoolID: 1, GroupID: 5}}, // pool 1 grants groupID 5 — lower pool ID wins
		},
	}
	outbox := &poolOutboxStub{}
	svc := &UserPoolService{repo: repo, userRepo: userRepo, outboxRepo: outbox, txRunner: &mockTransactionRunner{}}

	_, _, _, err := svc.AddMembersByFilter(context.Background(), 10, UserListFilters{Status: "active"})
	require.NoError(t, err)

	outbox.mu.Lock()
	n := len(outbox.events)
	outbox.mu.Unlock()

	// Pool 10 > pool 1 → no enqueue (existing pool 1 remains effective).
	assert.Equal(t, 0, n, "should NOT enqueue: current pool ID 10 > existing pool ID 1")
}

// ── TestRemoveMembers_EnqueueFailRollback ─────────────────────────────────────

// poolRepoStubWithTracking records RemoveMembers calls and allows resetting removed state.
type poolRepoStubWithTracking struct {
	poolRepoStub
	removeCalled bool
}

func (s *poolRepoStubWithTracking) RemoveMembers(_ context.Context, _ int64, _ []int64) ([]int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.removeCalled = true
	return s.removeRemoved, s.removeErr
}

// failingOutboxStub returns an error on the first Enqueue call to simulate DB failure.
type failingOutboxStub struct {
	poolOutboxStub
	callCount int
	failAfter int // fail when callCount > failAfter
}

func (s *failingOutboxStub) Enqueue(_ context.Context, event CacheInvalidationEvent) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.callCount++
	if s.callCount > s.failAfter {
		return fmt.Errorf("outbox: simulated DB error")
	}
	s.events = append(s.events, event)
	return nil
}

// TestRemoveMembers_EnqueueFail_ReturnsError verifies that when the outbox Enqueue
// fails, RemoveMembers propagates the error rather than swallowing it.
// With a real DB transaction the DELETE would be rolled back; here we confirm that
// at minimum the error surfaces to the caller.
func TestRemoveMembers_EnqueueFail_ReturnsError(t *testing.T) {
	repo := &poolRepoStubWithTracking{
		poolRepoStub: poolRepoStub{
			removeRemoved: []int64{10, 20},
			grants:        []PoolGroupGrant{{PoolID: 1, GroupID: 5}},
		},
	}
	outbox := &failingOutboxStub{failAfter: -1} // fail on every Enqueue call

	svc := &UserPoolService{
		repo:       repo,
		outboxRepo: outbox,
		txRunner:   &mockTransactionRunner{},
	}

	_, err := svc.RemoveMembers(context.Background(), 1, []int64{10, 20})
	require.Error(t, err, "RemoveMembers must return error when outbox Enqueue fails")
	require.Contains(t, err.Error(), "simulated DB error")

	// With a real transaction the repo DELETE would have been rolled back.
	// With mockTransactionRunner (no real tx) we verify only that error is propagated.
	outbox.mu.Lock()
	enqueued := len(outbox.events)
	outbox.mu.Unlock()
	assert.Equal(t, 0, enqueued, "no events must be stored when enqueue fails")
}

// ── P0-4: Large pool pagination closure ───────────────────────────────────────

// poolRepoStubLarge is a poolRepoStub whose ListAllMemberIDs returns a fixed list of
// memberIDs supplied at construction, while ListMembers (capped at 200 by normPage)
// would only return the first page.  This lets us verify the batched path.
type poolRepoStubLarge struct {
	poolRepoStub
	allMemberIDs []int64
}

func (s *poolRepoStubLarge) ListAllMemberIDs(_ context.Context, _ int64) ([]int64, error) {
	return s.allMemberIDs, nil
}

// TestEnqueuePoolDisabled_LargePool verifies that a pool with 500 members (>normPage 200 cap)
// has all 500 user IDs covered across outbox events when the pool is disabled.
func TestEnqueuePoolDisabled_LargePool(t *testing.T) {
	const totalMembers = 500

	// Build member list.
	allIDs := make([]int64, totalMembers)
	for i := range allIDs {
		allIDs[i] = int64(i + 1)
	}

	repo := &poolRepoStubLarge{
		poolRepoStub: poolRepoStub{
			getPool:    Pool{ID: 1, Name: "large", Status: "active"},
			updatePool: Pool{ID: 1, Name: "large", Status: "disabled"},
			// ListMembers (used by old path) would only return 200 due to normPage.
			members: func() []PoolMember {
				m := make([]PoolMember, totalMembers)
				for i := range m {
					m[i] = PoolMember{PoolID: 1, UserID: int64(i + 1)}
				}
				return m
			}(),
			grants: []PoolGroupGrant{{PoolID: 1, GroupID: 5}},
		},
		allMemberIDs: allIDs,
	}

	outbox := &poolOutboxStub{}
	svc := &UserPoolService{repo: repo, outboxRepo: outbox, txRunner: &mockTransactionRunner{}}

	_, err := svc.Update(context.Background(), 1, "large", "", "disabled")
	require.NoError(t, err)

	outbox.mu.Lock()
	events := outbox.events
	outbox.mu.Unlock()

	require.NotEmpty(t, events, "must enqueue events for large pool disable")

	// Collect all user IDs across all events.
	seenIDs := make(map[int64]struct{})
	for _, ev := range events {
		for _, uid := range ev.Payload.AffectedUserIDs {
			seenIDs[uid] = struct{}{}
		}
		// Each batch must be ≤1000.
		assert.LessOrEqual(t, len(ev.Payload.AffectedUserIDs), 1000,
			"each event must contain at most 1000 user IDs")
	}

	assert.Equal(t, totalMembers, len(seenIDs),
		"all %d members must appear across outbox events", totalMembers)
}

// ── P0-5: Idempotency key uniqueness ─────────────────────────────────────────

// TestBuildIdempotencyKey_DifferentUserSets verifies that two events for the same
// (pool, group, reason, second) but different user sets produce different keys.
func TestBuildIdempotencyKey_DifferentUserSets(t *testing.T) {
	ts := time.Unix(1700000000, 0).UTC()
	key1 := buildIdempotencyKeyForPoolEvent(1, 5, ReasonPoolDisabled, []int64{100, 200, 300}, ts)
	key2 := buildIdempotencyKeyForPoolEvent(1, 5, ReasonPoolDisabled, []int64{400, 500, 600}, ts)

	assert.NotEqual(t, key1, key2,
		"different user sets must produce different idempotency keys")

	// Same user set but different order must produce identical key (sort-stable).
	key3 := buildIdempotencyKeyForPoolEvent(1, 5, ReasonPoolDisabled, []int64{300, 100, 200}, ts)
	assert.Equal(t, key1, key3,
		"same users in different order must produce the same idempotency key")
}

// ── P0-6: Rate change includes CacheTypeUserGroupRate + RatePairs ─────────────

// TestEnqueueRateChanged_IncludesRatePairs verifies that when ReplaceGroupGrants triggers
// a rate_changed diff, the generated outbox event contains CacheTypeUserGroupRate and
// the correct number of RatePairs.
func TestEnqueueRateChanged_IncludesRatePairs(t *testing.T) {
	old := 1.0
	newRate := 2.0

	repo := &poolRepoStub{
		// ListGroupGrants (old state) returns rate 1.0.
		grants: []PoolGroupGrant{{PoolID: 1, GroupID: 5, RateMultiplier: &old}},
		// ListAllMemberIDs returns 3 users.
		members: []PoolMember{
			{PoolID: 1, UserID: 10},
			{PoolID: 1, UserID: 20},
			{PoolID: 1, UserID: 30},
		},
	}
	outbox := &poolOutboxStub{}
	svc := newPoolSvc(repo, outbox)

	err := svc.ReplaceGroupGrants(context.Background(), 1, []PoolGroupGrant{
		{GroupID: 5, RateMultiplier: &newRate},
	})
	require.NoError(t, err)

	outbox.mu.Lock()
	events := outbox.events
	outbox.mu.Unlock()

	// Find the rate_changed event.
	var rateEvent *CacheInvalidationEvent
	for i := range events {
		if events[i].Reason == ReasonRateChanged {
			rateEvent = &events[i]
			break
		}
	}
	require.NotNil(t, rateEvent, "must have a rate_changed outbox event")

	// Must include CacheTypeUserGroupRate.
	assert.Contains(t, rateEvent.CacheTypes, CacheTypeUserGroupRate,
		"rate_changed event must include CacheTypeUserGroupRate")

	// RatePairs must cover all 3 users.
	assert.Len(t, rateEvent.Payload.RatePairs, 3,
		"RatePairs length must equal number of affected users")
	for _, rp := range rateEvent.Payload.RatePairs {
		assert.Equal(t, int64(5), rp.GroupID, "RatePair GroupID must match the grant group")
	}
}
