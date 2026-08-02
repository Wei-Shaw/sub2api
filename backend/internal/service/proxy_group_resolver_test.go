//go:build unit

package service

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/stretchr/testify/require"
)

// memGenStore is an in-process ProxyGroupCacheVersionStore for unit tests.
type memGenStore struct {
	mu        sync.Mutex
	m         map[int64]int64
	getErr    error // when set, GetGeneration returns this error (fail-closed path)
	bumpErr   error // when set, BumpGeneration returns this error (retry/force-miss path)
	getCalls  int
	bumpCalls int
}

func newMemGenStore() *memGenStore {
	return &memGenStore{m: make(map[int64]int64)}
}

func (s *memGenStore) BumpGeneration(_ context.Context, groupID int64) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.bumpCalls++
	if s.bumpErr != nil {
		return 0, s.bumpErr
	}
	s.m[groupID]++
	return s.m[groupID], nil
}

func (s *memGenStore) GetGeneration(_ context.Context, groupID int64) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.getCalls++
	if s.getErr != nil {
		return 0, s.getErr
	}
	return s.m[groupID], nil
}

type stubProxyGroupRepo struct {
	group *ProxyGroup
	err   error
}

func (s *stubProxyGroupRepo) Create(context.Context, *ProxyGroup) error { panic("unused") }
func (s *stubProxyGroupRepo) GetByID(context.Context, int64) (*ProxyGroup, error) {
	return s.group, s.err
}
func (s *stubProxyGroupRepo) Update(context.Context, *ProxyGroup) error { panic("unused") }
func (s *stubProxyGroupRepo) Delete(context.Context, int64) error       { panic("unused") }
func (s *stubProxyGroupRepo) List(context.Context, pagination.PaginationParams) ([]ProxyGroup, *pagination.PaginationResult, error) {
	panic("unused")
}
func (s *stubProxyGroupRepo) ListActive(context.Context) ([]ProxyGroup, error) { panic("unused") }
func (s *stubProxyGroupRepo) CountProxiesByGroupID(context.Context, int64) (int64, error) {
	panic("unused")
}
func (s *stubProxyGroupRepo) CountAccountsByGroupID(context.Context, int64) (int64, error) {
	panic("unused")
}
func (s *stubProxyGroupRepo) SetGroupMembers(context.Context, int64, []int64) error {
	panic("unused")
}

type stubProxyRepoForGroup struct {
	ProxyRepository
	members []Proxy
}

func (s *stubProxyRepoForGroup) ListByGroupID(context.Context, int64) ([]Proxy, error) {
	return s.members, nil
}

func TestDefaultProxyGroupResolver_RoundRobinAndCache(t *testing.T) {
	t.Parallel()
	now := time.Now()
	group := &ProxyGroup{ID: 9, Name: "pool", Strategy: ProxyGroupStrategyRoundRobin, Status: StatusActive}
	members := []Proxy{
		{ID: 1, Status: StatusActive},
		{ID: 2, Status: StatusActive},
		{ID: 3, Status: StatusActive},
	}
	r := NewDefaultProxyGroupResolver(
		&stubProxyGroupRepo{group: group},
		&stubProxyRepoForGroup{members: members},
	)
	r.now = func() time.Time { return now }

	seen := map[int64]int{}
	for i := 0; i < 6; i++ {
		p, err := r.ResolveProxy(context.Background(), 9, 100)
		require.NoError(t, err)
		require.NotNil(t, p)
		seen[p.ID]++
	}
	require.Equal(t, 2, seen[1])
	require.Equal(t, 2, seen[2])
	require.Equal(t, 2, seen[3])
}

func TestDefaultProxyGroupResolver_StickyDoesNotWriteProxyID(t *testing.T) {
	t.Parallel()
	group := &ProxyGroup{ID: 1, StickyByAccount: true, Status: StatusActive}
	members := []Proxy{
		{ID: 10, Status: StatusActive},
		{ID: 20, Status: StatusActive},
	}
	r := NewDefaultProxyGroupResolver(
		&stubProxyGroupRepo{group: group},
		&stubProxyRepoForGroup{members: members},
	)
	first, err := r.ResolveProxy(context.Background(), 1, 42)
	require.NoError(t, err)
	require.NotNil(t, first)
	for i := 0; i < 5; i++ {
		p, err := r.ResolveProxy(context.Background(), 1, 42)
		require.NoError(t, err)
		require.Equal(t, first.ID, p.ID)
	}
}

func TestDefaultProxyGroupResolver_NoHealthyMembers(t *testing.T) {
	t.Parallel()
	now := time.Now()
	expired := now.Add(-time.Hour)
	group := &ProxyGroup{ID: 1, Status: StatusActive, Strategy: ProxyGroupStrategyRoundRobin}
	r := NewDefaultProxyGroupResolver(
		&stubProxyGroupRepo{group: group},
		&stubProxyRepoForGroup{members: []Proxy{{ID: 1, Status: StatusActive, ExpiresAt: &expired}}},
	)
	r.now = func() time.Time { return now }
	p, err := r.ResolveProxy(context.Background(), 1, 1)
	require.NoError(t, err)
	require.Nil(t, p)
}

// mutableMembersRepo allows tests to change ListByGroupID results mid-run.
type mutableMembersRepo struct {
	ProxyRepository
	mu      sync.Mutex
	members []Proxy
}

func (s *mutableMembersRepo) ListByGroupID(context.Context, int64) ([]Proxy, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Proxy, len(s.members))
	copy(out, s.members)
	return out, nil
}

func (s *mutableMembersRepo) setMembers(members []Proxy) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.members = members
}

func TestDefaultProxyGroupResolver_InvalidateGroupBumpsGenAndCrossInstanceSeesFresh(t *testing.T) {
	t.Parallel()
	now := time.Now()
	group := &ProxyGroup{ID: 42, Name: "pool", Strategy: ProxyGroupStrategyRoundRobin, Status: StatusActive}
	membersA := []Proxy{{ID: 1, Status: StatusActive}, {ID: 2, Status: StatusActive}}
	membersB := []Proxy{{ID: 3, Status: StatusActive}} // after membership change

	store := newMemGenStore()
	repo := &mutableMembersRepo{members: membersA}
	groupRepo := &stubProxyGroupRepo{group: group}

	// Instance A: loads and caches membersA
	a := NewDefaultProxyGroupResolverWithVersions(groupRepo, repo, store)
	a.now = func() time.Time { return now }
	p, err := a.ResolveProxy(context.Background(), 42, 1)
	require.NoError(t, err)
	require.NotNil(t, p)
	require.Equal(t, int64(1), p.ID)

	// Instance B shares the same generation store (simulates multi-instance Redis).
	b := NewDefaultProxyGroupResolverWithVersions(groupRepo, repo, store)
	b.now = func() time.Time { return now }
	p, err = b.ResolveProxy(context.Background(), 42, 1)
	require.NoError(t, err)
	require.Equal(t, int64(1), p.ID) // still cached/fresh from DB as membersA

	// Membership changes in DB; instance A invalidates (bumps gen + local drop).
	repo.setMembers(membersB)
	a.InvalidateGroup(42)
	gen, err := store.GetGeneration(context.Background(), 42)
	require.NoError(t, err)
	require.Equal(t, int64(1), gen)

	// Instance A sees fresh members immediately (local cache cleared).
	p, err = a.ResolveProxy(context.Background(), 42, 1)
	require.NoError(t, err)
	require.NotNil(t, p)
	require.Equal(t, int64(3), p.ID)

	// Instance B still has TTL-valid local cache of membersA, but gen mismatch
	// forces reload → sees membersB.
	p, err = b.ResolveProxy(context.Background(), 42, 1)
	require.NoError(t, err)
	require.NotNil(t, p)
	require.Equal(t, int64(3), p.ID)
}

func TestDefaultProxyGroupResolver_NilVersionStoreStillWorks(t *testing.T) {
	t.Parallel()
	group := &ProxyGroup{ID: 1, Status: StatusActive, Strategy: ProxyGroupStrategyRoundRobin}
	r := NewDefaultProxyGroupResolver(
		&stubProxyGroupRepo{group: group},
		&stubProxyRepoForGroup{members: []Proxy{{ID: 7, Status: StatusActive}}},
	)
	r.InvalidateGroup(1) // must not panic with nil versions
	p, err := r.ResolveProxy(context.Background(), 1, 1)
	require.NoError(t, err)
	require.NotNil(t, p)
	require.Equal(t, int64(7), p.ID)
}

func TestDefaultProxyGroupResolver_GetGenerationErrorForcesMiss(t *testing.T) {
	t.Parallel()
	now := time.Now()
	group := &ProxyGroup{ID: 7, Name: "pool", Strategy: ProxyGroupStrategyRoundRobin, Status: StatusActive}
	membersA := []Proxy{{ID: 1, Status: StatusActive}}
	membersB := []Proxy{{ID: 2, Status: StatusActive}}

	store := newMemGenStore()
	repo := &mutableMembersRepo{members: membersA}
	groupRepo := &stubProxyGroupRepo{group: group}

	r := NewDefaultProxyGroupResolverWithVersions(groupRepo, repo, store)
	r.now = func() time.Time { return now }

	// Prime cache with membersA (gen=0).
	p, err := r.ResolveProxy(context.Background(), 7, 1)
	require.NoError(t, err)
	require.NotNil(t, p)
	require.Equal(t, int64(1), p.ID)

	// Membership changes in DB; gen store starts failing.
	// Fail-closed: must NOT serve TTL-valid stale hit via gen==0 equality.
	repo.setMembers(membersB)
	store.getErr = errGenStoreDown

	p, err = r.ResolveProxy(context.Background(), 7, 1)
	require.NoError(t, err)
	require.NotNil(t, p)
	require.Equal(t, int64(2), p.ID, "GetGeneration error must force reload, not return hit with gen=0")
}

// TestDefaultProxyGroupResolver_BumpFailRetriesAndForceMiss verifies that a
// failed BumpGeneration is retried, then records a force-miss window so load()
// reloads from DB even when gen is unchanged and a local snap could reappear.
func TestDefaultProxyGroupResolver_BumpFailRetriesAndForceMiss(t *testing.T) {
	t.Parallel()
	now := time.Now()
	group := &ProxyGroup{ID: 55, Name: "pool", Strategy: ProxyGroupStrategyRoundRobin, Status: StatusActive}
	membersA := []Proxy{{ID: 1, Status: StatusActive}}
	membersB := []Proxy{{ID: 2, Status: StatusActive}}
	membersC := []Proxy{{ID: 3, Status: StatusActive}}

	store := newMemGenStore()
	store.bumpErr = errGenStoreDown
	repo := &mutableMembersRepo{members: membersA}
	groupRepo := &stubProxyGroupRepo{group: group}

	r := NewDefaultProxyGroupResolverWithVersions(groupRepo, repo, store)
	r.now = func() time.Time { return now }

	// Prime local cache (gen=0).
	p, err := r.ResolveProxy(context.Background(), 55, 1)
	require.NoError(t, err)
	require.Equal(t, int64(1), p.ID)

	// Membership changes; invalidate with bump always failing.
	repo.setMembers(membersB)
	beforeBumps := store.bumpCalls
	r.InvalidateGroup(55)
	require.Equal(t, beforeBumps+proxyGroupBumpAttempts, store.bumpCalls,
		"BumpGeneration must be retried %d times on failure", proxyGroupBumpAttempts)

	// Force-miss window active: gen still 0, but load must not stick to a
	// refilled snap across membership changes within the force-miss TTL.
	p, err = r.ResolveProxy(context.Background(), 55, 1)
	require.NoError(t, err)
	require.Equal(t, int64(2), p.ID)

	// Change members again while force-miss is still active — must see C, not
	// a TTL-valid cached B (gen still matches, would otherwise hit).
	repo.setMembers(membersC)
	p, err = r.ResolveProxy(context.Background(), 55, 1)
	require.NoError(t, err)
	require.Equal(t, int64(3), p.ID, "force-miss after bump fail must skip cache hit")

	// After force-miss window expires, normal TTL caching resumes.
	r.now = func() time.Time { return now.Add(proxyGroupBumpFailForceTTL + time.Second) }
	// Prime under expired force-miss (clears bumpFailed entry on check).
	p, err = r.ResolveProxy(context.Background(), 55, 1)
	require.NoError(t, err)
	require.Equal(t, int64(3), p.ID)

	// With force-miss cleared and TTL still valid from the previous fill at
	// advanced "now", a further membership change without invalidate would be
	// served from cache — confirm force-miss entry is gone so hit path works.
	_, stillForced := r.bumpFailed.Load(int64(55))
	require.False(t, stillForced, "expired force-miss entry must be cleared")
}

// errGenStoreDown simulates Redis/version-store outage.
var errGenStoreDown = context.DeadlineExceeded

type recordingResolver struct {
	invalidated []int64
}

func (r *recordingResolver) ResolveProxy(context.Context, int64, int64) (*Proxy, error) {
	return nil, nil
}
func (r *recordingResolver) InvalidateGroup(groupID int64) {
	r.invalidated = append(r.invalidated, groupID)
}

func TestAdminService_UpdateProxy_StatusChangeInvalidatesGroup(t *testing.T) {
	t.Parallel()
	gid := int64(99)
	repo := &updatingProxyRepoStub{
		proxyRepoStub: &proxyRepoStub{},
		proxy: &Proxy{
			ID: 5, Protocol: "http", Host: "p.example", Port: 8080,
			Status: StatusActive, GroupID: &gid, FallbackMode: FallbackModeNone,
		},
	}
	resolver := &recordingResolver{}
	svc := &adminServiceImpl{proxyRepo: repo, proxyGroupResolver: resolver}

	_, err := svc.UpdateProxy(context.Background(), 5, &UpdateProxyInput{Status: StatusInactive})
	require.NoError(t, err)
	require.Equal(t, []int64{99}, resolver.invalidated)
}

func TestAdminService_UpdateProxy_StatusChangeClearsHealthAudit(t *testing.T) {
	t.Parallel()
	gid := int64(99)
	repo := &updatingProxyRepoStub{
		proxyRepoStub: &proxyRepoStub{},
		proxy: &Proxy{
			ID: 5, Protocol: "http", Host: "p.example", Port: 8080,
			Status: StatusActive, GroupID: &gid, FallbackMode: FallbackModeNone,
		},
	}
	health := &memHealthCache{data: map[int64]*ProxyHealthMeta{
		5: {FailCount: 3, IsolatedBy: ProxyHealthIsolatedByHealth, IsolatedAt: 12345},
	}}
	svc := &adminServiceImpl{proxyRepo: repo, proxyHealth: health}

	_, err := svc.UpdateProxy(context.Background(), 5, &UpdateProxyInput{Status: StatusInactive})
	require.NoError(t, err)
	require.Len(t, repo.healthAuditCalls, 1)
	require.Equal(t, int64(5), repo.healthAuditCalls[0].proxyID)
	require.Equal(t, 0, repo.healthAuditCalls[0].failCount)
	require.Nil(t, repo.healthAuditCalls[0].lastHealthAt)
	require.Equal(t, "", repo.healthAuditCalls[0].isolatedBy)

	meta, err := health.GetProxyHealth(context.Background(), 5)
	require.NoError(t, err)
	require.NotNil(t, meta)
	require.Equal(t, "", meta.IsolatedBy)
	require.Equal(t, int64(0), meta.IsolatedAt)
	require.Equal(t, 0, meta.FailCount)
}

func TestAdminService_UpdateProxy_NameChangeInvalidatesGroup(t *testing.T) {
	t.Parallel()
	gid := int64(99)
	repo := &updatingProxyRepoStub{
		proxyRepoStub: &proxyRepoStub{},
		proxy: &Proxy{
			ID: 5, Protocol: "http", Host: "p.example", Port: 8080,
			Status: StatusActive, GroupID: &gid, FallbackMode: FallbackModeNone,
		},
	}
	resolver := &recordingResolver{}
	svc := &adminServiceImpl{proxyRepo: repo, proxyGroupResolver: resolver}

	_, err := svc.UpdateProxy(context.Background(), 5, &UpdateProxyInput{Name: "renamed"})
	require.NoError(t, err)
	require.Equal(t, []int64{99}, resolver.invalidated)
	require.Empty(t, repo.healthAuditCalls) // name-only must not clear health audit
}

func TestAdminService_UpdateProxy_HostChangeInvalidatesGroup(t *testing.T) {
	t.Parallel()
	gid := int64(55)
	repo := &updatingProxyRepoStub{
		proxyRepoStub: &proxyRepoStub{},
		proxy: &Proxy{
			ID: 5, Protocol: "http", Host: "old.example", Port: 8080,
			Status: StatusActive, GroupID: &gid, FallbackMode: FallbackModeNone,
		},
	}
	resolver := &recordingResolver{}
	svc := &adminServiceImpl{proxyRepo: repo, proxyGroupResolver: resolver}

	_, err := svc.UpdateProxy(context.Background(), 5, &UpdateProxyInput{Host: "new.example"})
	require.NoError(t, err)
	require.Equal(t, "new.example", repo.proxy.Host)
	require.Equal(t, []int64{55}, resolver.invalidated)
}

func TestAdminService_DeleteProxy_InvalidatesGroup(t *testing.T) {
	t.Parallel()
	gid := int64(77)
	repo := &deletingProxyRepoStub{
		proxyRepoStub: &proxyRepoStub{},
		proxy: &Proxy{
			ID: 8, Name: "office-proxy", Protocol: "http", Host: "p.example", Port: 8080,
			Status: StatusActive, GroupID: &gid,
		},
	}
	resolver := &recordingResolver{}
	svc := &adminServiceImpl{proxyRepo: repo, proxyGroupResolver: resolver}

	require.NoError(t, svc.DeleteProxy(context.Background(), 8))
	require.Equal(t, []int64{77}, resolver.invalidated)
	require.True(t, repo.deleted)
}

type deletingProxyRepoStub struct {
	*proxyRepoStub
	proxy   *Proxy
	deleted bool
}

func (s *deletingProxyRepoStub) GetByID(context.Context, int64) (*Proxy, error) {
	if s.proxy == nil {
		return nil, ErrProxyNotFound
	}
	copy := *s.proxy
	return &copy, nil
}

func (s *deletingProxyRepoStub) CountAccountsByProxyID(context.Context, int64) (int64, error) {
	return 0, nil
}

func (s *deletingProxyRepoStub) ClearAccountProxyBindings(_ context.Context, id int64) (int64, error) {
	if s.proxyRepoStub != nil {
		return s.proxyRepoStub.ClearAccountProxyBindings(context.Background(), id)
	}
	return 0, nil
}

func (s *deletingProxyRepoStub) Delete(context.Context, int64) error {
	s.deleted = true
	return nil
}
