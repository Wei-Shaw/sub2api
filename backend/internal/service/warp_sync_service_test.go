package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
)

type memProxyRepo struct {
	nextID        int64
	proxies       map[int64]*Proxy
	accountCounts map[int64]int64
}

func newMemProxyRepo() *memProxyRepo {
	return &memProxyRepo{nextID: 1, proxies: map[int64]*Proxy{}, accountCounts: map[int64]int64{}}
}

func (m *memProxyRepo) Create(ctx context.Context, proxy *Proxy) error {
	proxy.ID = m.nextID
	m.nextID++
	cp := *proxy
	m.proxies[proxy.ID] = &cp
	return nil
}
func (m *memProxyRepo) GetByID(ctx context.Context, id int64) (*Proxy, error) {
	p, ok := m.proxies[id]
	if !ok {
		return nil, ErrProxyNotFound
	}
	cp := *p
	return &cp, nil
}
func (m *memProxyRepo) ListByIDs(ctx context.Context, ids []int64) ([]Proxy, error) {
	return nil, nil
}
func (m *memProxyRepo) Update(ctx context.Context, proxy *Proxy) error {
	cp := *proxy
	m.proxies[proxy.ID] = &cp
	return nil
}
func (m *memProxyRepo) Delete(ctx context.Context, id int64) error {
	delete(m.proxies, id)
	return nil
}
func (m *memProxyRepo) List(ctx context.Context, params pagination.PaginationParams) ([]Proxy, *pagination.PaginationResult, error) {
	return m.ListWithFilters(ctx, params, "", "", "")
}
func (m *memProxyRepo) ListWithFilters(ctx context.Context, params pagination.PaginationParams, protocol, status, search string) ([]Proxy, *pagination.PaginationResult, error) {
	out := make([]Proxy, 0, len(m.proxies))
	for _, p := range m.proxies {
		out = append(out, *p)
	}
	return out, &pagination.PaginationResult{Total: int64(len(out)), Page: 1, PageSize: params.PageSize}, nil
}
func (m *memProxyRepo) ListWithFiltersAndAccountCount(ctx context.Context, params pagination.PaginationParams, protocol, status, search string) ([]ProxyWithAccountCount, *pagination.PaginationResult, error) {
	return nil, nil, nil
}
func (m *memProxyRepo) ListActive(ctx context.Context) ([]Proxy, error) {
	out := make([]Proxy, 0)
	for _, p := range m.proxies {
		if p.Status == StatusActive {
			out = append(out, *p)
		}
	}
	return out, nil
}
func (m *memProxyRepo) ListActiveWithAccountCount(ctx context.Context) ([]ProxyWithAccountCount, error) {
	return nil, nil
}
func (m *memProxyRepo) ExistsByHostPortAuth(ctx context.Context, host string, port int, username, password string) (bool, error) {
	return false, nil
}
func (m *memProxyRepo) CountAccountsByProxyID(ctx context.Context, proxyID int64) (int64, error) {
	if m.accountCounts == nil {
		return 0, nil
	}
	return m.accountCounts[proxyID], nil
}
func (m *memProxyRepo) ListAccountSummariesByProxyID(ctx context.Context, proxyID int64) ([]ProxyAccountSummary, error) {
	return nil, nil
}
func (m *memProxyRepo) SweepExpiredProxies(ctx context.Context, now time.Time) (int64, error) {
	return 0, nil
}
func (m *memProxyRepo) ListAllForFallback(ctx context.Context) ([]Proxy, error) { return nil, nil }
func (m *memProxyRepo) CountExpired(ctx context.Context) (int64, error)         { return 0, nil }
func (m *memProxyRepo) CountExpiringSoon(ctx context.Context, now time.Time) (int64, error) {
	return 0, nil
}
func (m *memProxyRepo) ListByGroupID(ctx context.Context, groupID int64) ([]Proxy, error) {
	out := make([]Proxy, 0)
	for _, p := range m.proxies {
		if p.GroupID != nil && *p.GroupID == groupID {
			out = append(out, *p)
		}
	}
	return out, nil
}
func (m *memProxyRepo) CountByGroupID(ctx context.Context, groupID int64) (int64, error) {
	var n int64
	for _, p := range m.proxies {
		if p.GroupID != nil && *p.GroupID == groupID {
			n++
		}
	}
	return n, nil
}
func (m *memProxyRepo) UpdateHealthAudit(context.Context, int64, int, *time.Time, string) error {
	return nil
}
func (m *memProxyRepo) GetHealthAudit(context.Context, int64) (int, *time.Time, string, error) {
	return 0, nil, "", nil
}
func (m *memProxyRepo) CountHealthIsolated(context.Context) (int64, error)       { return 0, nil }
func (m *memProxyRepo) ListHealthIsolated(context.Context, int) ([]Proxy, error) { return nil, nil }
func (m *memProxyRepo) ListHealthIsolatedByID(context.Context, int64, int) ([]Proxy, error) {
	return nil, nil
}
func (m *memProxyRepo) ClearAccountProxyBindings(ctx context.Context, proxyID int64) (int64, error) {
	if m.accountCounts == nil {
		return 0, nil
	}
	n := m.accountCounts[proxyID]
	delete(m.accountCounts, proxyID)
	return n, nil
}
func (m *memProxyRepo) UpdateStatusWithHealthIsolation(_ context.Context, proxyID int64, status string, failCount int, lastHealthAt *time.Time, isolatedBy string, onlyIfStatus string, onlyIfIsolatedBy *string, _ bool) (bool, error) {
	p, ok := m.proxies[proxyID]
	if !ok {
		return false, ErrProxyNotFound
	}
	if onlyIfStatus != "" && p.Status != onlyIfStatus {
		return false, nil
	}
	if onlyIfIsolatedBy != nil {
		cur := p.HealthIsolatedBy
		if cur != *onlyIfIsolatedBy {
			return false, nil
		}
	}
	p.Status = status
	p.HealthFailCount = failCount
	p.HealthIsolatedBy = isolatedBy
	p.LastHealthAt = lastHealthAt
	return true, nil
}

type memGroupRepo struct {
	nextID  int64
	groups  map[int64]*ProxyGroup
	members map[int64][]int64
}

func newMemGroupRepo() *memGroupRepo {
	return &memGroupRepo{nextID: 1, groups: map[int64]*ProxyGroup{}, members: map[int64][]int64{}}
}
func (m *memGroupRepo) Create(ctx context.Context, group *ProxyGroup) error {
	group.ID = m.nextID
	m.nextID++
	cp := *group
	m.groups[group.ID] = &cp
	return nil
}
func (m *memGroupRepo) GetByID(ctx context.Context, id int64) (*ProxyGroup, error) {
	g, ok := m.groups[id]
	if !ok {
		return nil, ErrProxyGroupNotFound
	}
	cp := *g
	return &cp, nil
}
func (m *memGroupRepo) Update(ctx context.Context, group *ProxyGroup) error {
	cp := *group
	m.groups[group.ID] = &cp
	return nil
}
func (m *memGroupRepo) Delete(ctx context.Context, id int64) error { return nil }
func (m *memGroupRepo) List(ctx context.Context, params pagination.PaginationParams) ([]ProxyGroup, *pagination.PaginationResult, error) {
	out := make([]ProxyGroup, 0, len(m.groups))
	for _, g := range m.groups {
		out = append(out, *g)
	}
	return out, &pagination.PaginationResult{Total: int64(len(out))}, nil
}
func (m *memGroupRepo) ListActive(ctx context.Context) ([]ProxyGroup, error) {
	out := make([]ProxyGroup, 0)
	for _, g := range m.groups {
		if g.Status == StatusActive {
			out = append(out, *g)
		}
	}
	return out, nil
}
func (m *memGroupRepo) CountProxiesByGroupID(ctx context.Context, groupID int64) (int64, error) {
	return int64(len(m.members[groupID])), nil
}
func (m *memGroupRepo) CountAccountsByGroupID(ctx context.Context, groupID int64) (int64, error) {
	return 0, nil
}
func (m *memGroupRepo) SetGroupMembers(ctx context.Context, groupID int64, proxyIDs []int64) error {
	m.members[groupID] = append([]int64(nil), proxyIDs...)
	return nil
}

// setGroupMembersCallCount tracks SetMembers for drastic-refuse assertions.
func (m *memGroupRepo) memberCount(groupID int64) int {
	return len(m.members[groupID])
}

func TestMergeNonWarpGroupMembers(t *testing.T) {
	proxyRepo := newMemProxyRepo()
	gid := int64(7)
	// Manual non-warp member already in group.
	manual := &Proxy{Name: "office-proxy", Host: "10.0.0.1", Port: 1080, Status: StatusActive, GroupID: &gid}
	_ = proxyRepo.Create(context.Background(), manual)
	// Existing warp member (should not be double-kept via merge path alone).
	warpOld := &Proxy{Name: "warp-warp-01", Host: "127.0.0.1", Port: 20001, Status: StatusActive, GroupID: &gid, ManagedBy: warpProxyManagedBy, ExternalID: "old"}
	_ = proxyRepo.Create(context.Background(), warpOld)

	svc := &WarpSyncService{proxyRepo: proxyRepo}
	// New warp member set from sync plan (only the new warp id).
	merged, err := svc.mergeNonWarpGroupMembers(context.Background(), gid, []int64{warpOld.ID, 99})
	if err != nil {
		t.Fatal(err)
	}
	seen := map[int64]bool{}
	for _, id := range merged {
		seen[id] = true
	}
	if !seen[manual.ID] {
		t.Fatalf("expected non-warp member %d preserved, got %v", manual.ID, merged)
	}
	if !seen[warpOld.ID] || !seen[99] {
		t.Fatalf("expected warp members kept, got %v", merged)
	}
}

type noopResolver struct{}

func (noopResolver) ResolveProxy(ctx context.Context, groupID, accountID int64) (*Proxy, error) {
	return nil, nil
}
func (noopResolver) InvalidateGroup(groupID int64) {}

func TestWarpSyncService_SyncFromGateway(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/pools/snapshot", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(WarpPoolSnapshot{
			Instances: []WarpInstance{
				{ID: "i1", Name: "01", ListenHost: "127.0.0.1", ListenPort: 41001, Status: "running", ExitIP: "1.2.3.4"},
				{ID: "i2", Name: "02", ListenHost: "127.0.0.1", ListenPort: 41002, Status: "unhealthy", ExitIP: "1.2.3.4"},
			},
			UnhealthyIDs: []string{"i2"},
			DuplicateIPs: map[string][]string{"1.2.3.4": {"i1", "i2"}},
			HealthyCount: 1,
			TotalCount:   2,
		})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	client := NewWarpGatewayClient(WarpGatewayConfig{Enabled: true, BaseURL: srv.URL, Timeout: time.Second})
	proxyRepo := newMemProxyRepo()
	groupRepo := newMemGroupRepo()
	groupSvc := NewProxyGroupService(groupRepo, proxyRepo, noopResolver{})
	cfg := &config.Config{Warp: config.WarpConfig{
		Enabled:              true,
		AutoDetachUnhealthy:  true,
		AlertDuplicateExitIP: true,
		DefaultGroupName:     "warp-pool",
		Gateway:              config.WarpGatewayConfig{BaseURL: srv.URL},
	}}
	svc := NewWarpSyncService(cfg, client, proxyRepo, groupSvc, nil)

	res, err := svc.SyncFromGateway(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	if len(res.CreatedProxies) != 2 {
		t.Fatalf("created=%d", len(res.CreatedProxies))
	}
	if res.Group == nil || res.Group.Name != "warp-pool" {
		t.Fatalf("group=%+v", res.Group)
	}
	// only healthy member should be in group when auto-detach
	if len(res.MemberIDs) != 1 {
		t.Fatalf("members=%v detached=%v", res.MemberIDs, res.DetachedIDs)
	}
	if len(res.Alerts) == 0 {
		t.Fatal("expected duplicate ip alert")
	}

	// second sync updates, no new creates
	res2, err := svc.SyncFromGateway(context.Background(), "warp-pool")
	if err != nil {
		t.Fatal(err)
	}
	if len(res2.CreatedProxies) != 0 {
		t.Fatalf("expected 0 creates, got %d", len(res2.CreatedProxies))
	}
}

// Multi-batch gateway snapshots historically reused instance names (warp-01 twice
// on different ports). Sync must keep both endpoints in the proxy pool.
func TestWarpSyncFromGateway_DuplicateNamesDifferentPortsCreateBoth(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/pools/snapshot", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(WarpPoolSnapshot{
			Instances: []WarpInstance{
				{ID: "a1", Name: "warp-01", ListenHost: "127.0.0.1", ListenPort: 20001, Status: "running", ExitIP: "1.1.1.1"},
				{ID: "a2", Name: "warp-01", ListenHost: "127.0.0.1", ListenPort: 20002, Status: "running", ExitIP: "1.1.1.2"},
				{ID: "a3", Name: "warp-02", ListenHost: "127.0.0.1", ListenPort: 20003, Status: "running", ExitIP: "1.1.1.3"},
			},
			HealthyCount: 3,
			TotalCount:   3,
		})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	client := NewWarpGatewayClient(WarpGatewayConfig{Enabled: true, BaseURL: srv.URL, Timeout: time.Second})
	proxyRepo := newMemProxyRepo()
	groupRepo := newMemGroupRepo()
	groupSvc := NewProxyGroupService(groupRepo, proxyRepo, noopResolver{})
	cfg := &config.Config{Warp: config.WarpConfig{
		Enabled:             true,
		AutoDetachUnhealthy: false,
		DefaultGroupName:    "warp-pool",
		Gateway:             config.WarpGatewayConfig{BaseURL: srv.URL},
	}}
	svc := NewWarpSyncService(cfg, client, proxyRepo, groupSvc, nil)

	res, err := svc.SyncFromGateway(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	if len(res.CreatedProxies) != 3 {
		t.Fatalf("created=%d want 3 (duplicate names must not collapse)", len(res.CreatedProxies))
	}
	if len(res.MemberIDs) != 3 {
		t.Fatalf("members=%v want 3", res.MemberIDs)
	}
	names := map[string]struct{}{}
	for _, p := range res.CreatedProxies {
		if _, dup := names[p.Name]; dup {
			t.Fatalf("duplicate proxy name after sync: %q", p.Name)
		}
		names[p.Name] = struct{}{}
	}
}

func TestEnsureUniqueWarpProxyName(t *testing.T) {
	byName := map[string]Proxy{
		"warp-warp-01": {ID: 1, Name: "warp-warp-01", Host: "127.0.0.1", Port: 10001},
	}
	byKey := map[string]Proxy{
		"127.0.0.1:10001": byName["warp-warp-01"],
	}
	// Same endpoint keeps name.
	got := ensureUniqueWarpProxyName("warp-warp-01", "127.0.0.1:10001", byName, byKey)
	if got != "warp-warp-01" {
		t.Fatalf("same endpoint: %q", got)
	}
	// Different port must disambiguate.
	got = ensureUniqueWarpProxyName("warp-warp-01", "127.0.0.1:10002", byName, byKey)
	if got != "warp-warp-01-10002" {
		t.Fatalf("diff endpoint: %q", got)
	}
}

func TestWarpSync_OrphanDeleteUnbindsAccounts(t *testing.T) {
	// Gateway drops instance i-old; local still has warp-old bound to accounts.
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/pools/snapshot", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(WarpPoolSnapshot{
			Instances: []WarpInstance{
				{ID: "i-new", Name: "new", ListenHost: "127.0.0.1", ListenPort: 42001, Status: "running"},
			},
			HealthyCount: 1,
			TotalCount:   1,
		})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	client := NewWarpGatewayClient(WarpGatewayConfig{Enabled: true, BaseURL: srv.URL, Timeout: time.Second})
	proxyRepo := newMemProxyRepo()
	// Pre-seed an orphan warp proxy still referenced by accounts.
	orphan := &Proxy{Name: "warp-old", Protocol: "socks5", Host: "127.0.0.1", Port: 41999, Status: StatusActive, ManagedBy: warpProxyManagedBy, ExternalID: "i-old"}
	if err := proxyRepo.Create(context.Background(), orphan); err != nil {
		t.Fatal(err)
	}
	proxyRepo.accountCounts[orphan.ID] = 3

	groupRepo := newMemGroupRepo()
	groupSvc := NewProxyGroupService(groupRepo, proxyRepo, noopResolver{})
	cfg := &config.Config{Warp: config.WarpConfig{
		Enabled:          true,
		DefaultGroupName: "warp-pool",
		Gateway:          config.WarpGatewayConfig{BaseURL: srv.URL},
	}}
	svc := NewWarpSyncService(cfg, client, proxyRepo, groupSvc, nil)

	res, err := svc.SyncFromGateway(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	if len(res.DeletedProxies) != 1 || res.DeletedProxies[0].ID != orphan.ID {
		t.Fatalf("deleted=%+v want orphan id=%d", res.DeletedProxies, orphan.ID)
	}
	if _, ok := proxyRepo.proxies[orphan.ID]; ok {
		t.Fatal("orphan should be removed from repo")
	}
	if _, still := proxyRepo.accountCounts[orphan.ID]; still {
		t.Fatal("account bindings should be cleared")
	}
	foundAlert := false
	for _, a := range res.Alerts {
		if strings.Contains(a, "unbound") {
			foundAlert = true
			break
		}
	}
	if !foundAlert {
		t.Fatalf("expected unbind alert, got %v", res.Alerts)
	}
}

func TestWarpSync_DoesNotTakeOwnershipByNameOrEndpoint(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/pools/snapshot", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(WarpPoolSnapshot{
			Instances: []WarpInstance{{
				ID: "managed-1", Name: "managed", ListenHost: "127.0.0.1", ListenPort: 42101,
				Status: "running", SocksAuthUser: "warp-user", SocksAuthPass: "warp-pass",
			}},
			HealthyCount: 1,
			TotalCount:   1,
		})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	proxyRepo := newMemProxyRepo()
	manual := &Proxy{Name: "warp-manual", Protocol: "socks5", Host: "127.0.0.1", Port: 42101, Status: StatusActive}
	if err := proxyRepo.Create(context.Background(), manual); err != nil {
		t.Fatal(err)
	}
	groupRepo := newMemGroupRepo()
	groupSvc := NewProxyGroupService(groupRepo, proxyRepo, noopResolver{})
	client := NewWarpGatewayClient(WarpGatewayConfig{Enabled: true, BaseURL: srv.URL, Timeout: time.Second})
	svc := NewWarpSyncService(&config.Config{Warp: config.WarpConfig{
		Enabled: true, DefaultGroupName: "warp-pool", Gateway: config.WarpGatewayConfig{BaseURL: srv.URL},
	}}, client, proxyRepo, groupSvc, nil)

	res, err := svc.SyncFromGateway(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	if len(res.CreatedProxies) != 1 || len(res.UpdatedProxies) != 0 || len(res.DeletedProxies) != 0 {
		t.Fatalf("unexpected reconciliation: created=%v updated=%v deleted=%v", res.CreatedProxies, res.UpdatedProxies, res.DeletedProxies)
	}
	manualAfter := proxyRepo.proxies[manual.ID]
	if manualAfter == nil || manualAfter.ManagedBy != "" || manualAfter.Name != "warp-manual" {
		t.Fatalf("manual proxy was taken over: %#v", manualAfter)
	}
	managed := res.CreatedProxies[0]
	if managed.ManagedBy != warpProxyManagedBy || managed.ExternalID != "managed-1" ||
		managed.Username != "warp-user" || managed.Password != "warp-pass" {
		t.Fatalf("managed proxy metadata incomplete: %#v", managed)
	}
}

func TestWarpSync_AdoptsMarkedLegacyProxyByExactEndpoint(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/pools/snapshot", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(WarpPoolSnapshot{
			Instances: []WarpInstance{{
				ID: "gateway-legacy-1", Name: "legacy", ListenHost: "127.0.0.1", ListenPort: 42111,
				Status: "running", SocksAuthUser: "new-user", SocksAuthPass: "new-pass",
			}},
			HealthyCount: 1, TotalCount: 1,
		})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	proxyRepo := newMemProxyRepo()
	legacy := &Proxy{
		Name: "warp-legacy", Protocol: "socks5", Host: "127.0.0.1", Port: 42111,
		Status: StatusActive, ManagedBy: warpProxyManagedBy,
	}
	if err := proxyRepo.Create(context.Background(), legacy); err != nil {
		t.Fatal(err)
	}
	groupSvc := NewProxyGroupService(newMemGroupRepo(), proxyRepo, noopResolver{})
	svc := NewWarpSyncService(&config.Config{Warp: config.WarpConfig{
		Enabled: true, DefaultGroupName: "warp-pool", Gateway: config.WarpGatewayConfig{BaseURL: srv.URL},
	}}, NewWarpGatewayClient(WarpGatewayConfig{Enabled: true, BaseURL: srv.URL, Timeout: time.Second}), proxyRepo, groupSvc, nil)

	res, err := svc.SyncFromGateway(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	if len(res.CreatedProxies) != 0 || len(res.UpdatedProxies) != 1 {
		t.Fatalf("legacy adoption created=%v updated=%v", res.CreatedProxies, res.UpdatedProxies)
	}
	adopted := proxyRepo.proxies[legacy.ID]
	if adopted == nil || adopted.ExternalID != "gateway-legacy-1" || adopted.Username != "new-user" || adopted.Password != "new-pass" {
		t.Fatalf("legacy proxy not adopted in place: %#v", adopted)
	}
}

func TestWarpSync_ProcessSingleflightBusy(t *testing.T) {
	cfg := &config.Config{Warp: config.WarpConfig{Enabled: true, Gateway: config.WarpGatewayConfig{BaseURL: "http://127.0.0.1:1"}}}
	client := NewWarpGatewayClient(WarpGatewayConfig{Enabled: true, BaseURL: "http://127.0.0.1:1", Timeout: time.Millisecond})
	svc := NewWarpSyncService(cfg, client, newMemProxyRepo(), nil, nil)
	svc.syncMu.Lock()
	svc.syncActive = true
	svc.syncMu.Unlock()

	_, err := svc.SyncFromGateway(context.Background(), "")
	if !errors.Is(err, ErrWarpSyncBusy) {
		t.Fatalf("err=%v want ErrWarpSyncBusy", err)
	}
}

func TestWarpSync_LeaderLockContended(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/pools/snapshot", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(WarpPoolSnapshot{
			Instances:    []WarpInstance{{ID: "i1", Name: "01", ListenHost: "127.0.0.1", ListenPort: 1, Status: "running"}},
			HealthyCount: 1, TotalCount: 1,
		})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	lock := &fakeLeaderLockCache{}
	ok, err := lock.TryAcquireLeaderLock(context.Background(), warpSyncWorkerLockKey, "peer", time.Minute)
	if err != nil || !ok {
		t.Fatalf("seed lock: ok=%v err=%v", ok, err)
	}

	client := NewWarpGatewayClient(WarpGatewayConfig{Enabled: true, BaseURL: srv.URL, Timeout: time.Second})
	svc := NewWarpSyncService(&config.Config{Warp: config.WarpConfig{
		Enabled: true, DefaultGroupName: "warp-pool",
		Gateway: config.WarpGatewayConfig{BaseURL: srv.URL, ReconcileInterval: 15},
	}}, client, newMemProxyRepo(), nil, nil)
	svc.SetLeaderLock(lock, "local", nil)

	res, err := svc.SyncFromGateway(context.Background(), "")
	if !errors.Is(err, ErrWarpSyncBusy) {
		t.Fatalf("err=%v want ErrWarpSyncBusy (peer lock)", err)
	}
	if res != nil {
		t.Fatalf("peer lock must not return success result, got %+v", res)
	}
}

// Redis acquire error → 503 ServiceUnavailable, not 409 Busy.
func TestWarpSync_LeaderLockUnavailable(t *testing.T) {
	lock := &fakeLeaderLockCache{acquireErr: context.DeadlineExceeded}
	client := NewWarpGatewayClient(WarpGatewayConfig{Enabled: true, BaseURL: "http://127.0.0.1:1", Timeout: time.Millisecond})
	svc := NewWarpSyncService(&config.Config{Warp: config.WarpConfig{
		Enabled: true, DefaultGroupName: "warp-pool",
		Gateway: config.WarpGatewayConfig{BaseURL: "http://127.0.0.1:1"},
	}}, client, newMemProxyRepo(), nil, nil)
	svc.SetLeaderLock(lock, "local", nil)

	_, err := svc.SyncFromGateway(context.Background(), "")
	if !errors.Is(err, ErrWarpSyncLockUnavailable) {
		t.Fatalf("err=%v want ErrWarpSyncLockUnavailable", err)
	}
	if errors.Is(err, ErrWarpSyncBusy) {
		t.Fatal("Redis-down must not map to Busy")
	}

	_, err = svc.CreatePoolAndSyncEx(context.Background(), "warp", 1, "warp-pool", false)
	if !errors.Is(err, ErrWarpSyncLockUnavailable) {
		t.Fatalf("CreatePool err=%v want ErrWarpSyncLockUnavailable", err)
	}
}

// Empty successful gateway snapshot must not prune existing local warp-* rows.
func TestWarpSync_EmptyGatewaySnapshotRefusesPrune(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/pools/snapshot", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(WarpPoolSnapshot{
			Instances:    nil,
			HealthyCount: 0,
			TotalCount:   0,
		})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	client := NewWarpGatewayClient(WarpGatewayConfig{Enabled: true, BaseURL: srv.URL, Timeout: time.Second})
	proxyRepo := newMemProxyRepo()
	existing := &Proxy{Name: "warp-keep-me", Protocol: "socks5", Host: "127.0.0.1", Port: 43001, Status: StatusActive, ManagedBy: warpProxyManagedBy, ExternalID: "keep-me"}
	if err := proxyRepo.Create(context.Background(), existing); err != nil {
		t.Fatal(err)
	}
	groupRepo := newMemGroupRepo()
	groupSvc := NewProxyGroupService(groupRepo, proxyRepo, noopResolver{})
	cfg := &config.Config{Warp: config.WarpConfig{
		Enabled: true, DefaultGroupName: "warp-pool",
		Gateway: config.WarpGatewayConfig{BaseURL: srv.URL},
	}}
	svc := NewWarpSyncService(cfg, client, proxyRepo, groupSvc, nil)

	res, err := svc.SyncFromGateway(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	if len(res.DeletedProxies) != 0 {
		t.Fatalf("must not delete local warp-* on empty snapshot, deleted=%+v", res.DeletedProxies)
	}
	if _, ok := proxyRepo.proxies[existing.ID]; !ok {
		t.Fatal("local warp proxy must remain after empty gateway snapshot")
	}
	found := false
	for _, a := range res.Alerts {
		if strings.Contains(a, "empty gateway snapshot") && strings.Contains(a, "refusing to prune") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected empty-snapshot alert, got %v", res.Alerts)
	}
}

func TestWarpSync_SyncLockTTLMin90s(t *testing.T) {
	svc := &WarpSyncService{cfg: config.WarpConfig{
		Gateway: config.WarpGatewayConfig{ReconcileInterval: 15},
	}}
	ttl := svc.syncLockTTL()
	if ttl < 90*time.Second {
		t.Fatalf("syncLockTTL=%v want >= 90s", ttl)
	}
	if ttl > 5*time.Minute {
		t.Fatalf("syncLockTTL=%v want <= 5m", ttl)
	}

	svc2 := &WarpSyncService{cfg: config.WarpConfig{
		Gateway: config.WarpGatewayConfig{ReconcileInterval: 120},
	}}
	ttl2 := svc2.syncLockTTL()
	if ttl2 != 120*time.Second {
		t.Fatalf("syncLockTTL with reconcile 120s = %v want 120s", ttl2)
	}
}

func TestWarpSync_CreatePoolCountCap(t *testing.T) {
	cfg := &config.Config{Warp: config.WarpConfig{Enabled: true, Gateway: config.WarpGatewayConfig{BaseURL: "http://127.0.0.1:1"}}}
	client := NewWarpGatewayClient(WarpGatewayConfig{Enabled: true, BaseURL: "http://127.0.0.1:1", Timeout: time.Millisecond})
	svc := NewWarpSyncService(cfg, client, newMemProxyRepo(), nil, nil)
	_, err := svc.CreatePoolAndSyncEx(context.Background(), "warp", 51, "warp-pool", false)
	if err == nil {
		t.Fatal("expected count > 50 to fail")
	}
	if !strings.Contains(err.Error(), "50") && !strings.Contains(err.Error(), "WARP_POOL_COUNT_TOO_LARGE") {
		t.Fatalf("unexpected err: %v", err)
	}
}

// CreatePool must not hit gateway mutate when a peer already holds the leader lock.
func TestWarpSync_CreatePoolBusyBeforeMutate(t *testing.T) {
	createHits := 0
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/pools", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			createHits++
			t.Errorf("CreatePoolEx must not be called when peer holds lock")
			_ = json.NewEncoder(w).Encode(map[string]any{"instances": []WarpInstance{}})
			return
		}
		http.NotFound(w, r)
	})
	mux.HandleFunc("/v1/pools/snapshot", func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("snapshot must not be called when peer holds lock")
		_ = json.NewEncoder(w).Encode(WarpPoolSnapshot{})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	lock := &fakeLeaderLockCache{}
	ok, err := lock.TryAcquireLeaderLock(context.Background(), warpSyncWorkerLockKey, "peer", time.Minute)
	if err != nil || !ok {
		t.Fatalf("seed lock: ok=%v err=%v", ok, err)
	}

	client := NewWarpGatewayClient(WarpGatewayConfig{Enabled: true, BaseURL: srv.URL, Timeout: time.Second})
	svc := NewWarpSyncService(&config.Config{Warp: config.WarpConfig{
		Enabled: true, DefaultGroupName: "warp-pool",
		Gateway: config.WarpGatewayConfig{BaseURL: srv.URL},
	}}, client, newMemProxyRepo(), nil, nil)
	svc.SetLeaderLock(lock, "local", nil)

	_, err = svc.CreatePoolAndSyncEx(context.Background(), "warp", 2, "warp-pool", false)
	if !errors.Is(err, ErrWarpSyncBusy) {
		t.Fatalf("err=%v want ErrWarpSyncBusy", err)
	}
	if createHits != 0 {
		t.Fatalf("createHits=%d want 0 (mutate must not run before lock)", createHits)
	}
}

func TestIsDrasticWarpDrop(t *testing.T) {
	cases := []struct {
		local, spec int
		want        bool
	}{
		{3, 1, true},  // drop 2, half rule
		{4, 1, true},  // drop 3
		{10, 4, true}, // drop 6, 4*2 < 10
		{6, 1, true},
		{3, 2, false}, // drop 1, 2*2 not < 3
		{2, 1, false}, // half boundary: 1*2 >= 2
		{4, 2, false}, // exactly half
		{5, 0, false}, // empty handled by empty breaker
		{1, 1, false},
		{0, 1, false},
		{2, 0, false},
	}
	for _, tc := range cases {
		if got := isDrasticWarpDrop(tc.local, tc.spec); got != tc.want {
			t.Errorf("isDrasticWarpDrop(%d, %d)=%v want %v", tc.local, tc.spec, got, tc.want)
		}
	}
}

// Gateway returning far fewer specs than local warp-* must refuse orphan prune
// and skip SetMembers while still upserting the present specs (first streak round).
func TestWarpSync_DrasticDropRefusesOrphanPrune(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/pools/snapshot", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(WarpPoolSnapshot{
			Instances: []WarpInstance{
				{ID: "keep", Name: "keep", ListenHost: "127.0.0.1", ListenPort: 45001, Status: "running"},
			},
			HealthyCount: 1,
			TotalCount:   1,
		})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	client := NewWarpGatewayClient(WarpGatewayConfig{Enabled: true, BaseURL: srv.URL, Timeout: time.Second})
	proxyRepo := newMemProxyRepo()
	// Seed 6 local warp-* proxies (isDrasticWarpDrop: 1*2 < 6, drop 5 >= 2).
	seedIDs := make([]int64, 0, 6)
	for i := 1; i <= 6; i++ {
		p := &Proxy{
			Name:       fmt.Sprintf("warp-local-%02d", i),
			Protocol:   "socks5",
			Host:       "127.0.0.1",
			Port:       44000 + i,
			Status:     StatusActive,
			ManagedBy:  warpProxyManagedBy,
			ExternalID: fmt.Sprintf("local-%02d", i),
		}
		if err := proxyRepo.Create(context.Background(), p); err != nil {
			t.Fatal(err)
		}
		seedIDs = append(seedIDs, p.ID)
	}
	groupRepo := newMemGroupRepo()
	groupSvc := NewProxyGroupService(groupRepo, proxyRepo, noopResolver{})
	// Pre-seed managed group with all 6 members so we can assert SetMembers is not shrunk.
	createdGroup, err := groupSvc.Create(context.Background(), CreateProxyGroupInput{
		Name:     "warp-pool",
		Strategy: ProxyGroupStrategySticky,
		ProxyIDs: seedIDs,
	})
	if err != nil {
		t.Fatal(err)
	}
	if groupRepo.memberCount(createdGroup.ID) != 6 {
		t.Fatalf("precondition: want 6 members, got %d", groupRepo.memberCount(createdGroup.ID))
	}

	cfg := &config.Config{Warp: config.WarpConfig{
		Enabled: true, DefaultGroupName: "warp-pool", AutoDetachUnhealthy: false,
		Gateway: config.WarpGatewayConfig{BaseURL: srv.URL},
	}}
	svc := NewWarpSyncService(cfg, client, proxyRepo, groupSvc, nil)

	res, err := svc.SyncFromGateway(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	if len(res.DeletedProxies) != 0 {
		t.Fatalf("drastic drop must not delete orphans, deleted=%+v", res.DeletedProxies)
	}
	// Local inventory still has the 6 seeds (+ maybe 1 created for new port).
	if len(proxyRepo.proxies) < 6 {
		t.Fatalf("local proxies shrank under drastic drop: n=%d", len(proxyRepo.proxies))
	}
	// Member list must not be replaced with the shrunk gateway set on refuse rounds.
	if got := groupRepo.memberCount(createdGroup.ID); got != 6 {
		t.Fatalf("drastic refuse must skip SetMembers; members=%d want 6 (ids=%v)", got, groupRepo.members[createdGroup.ID])
	}
	foundAlert := false
	for _, a := range res.Alerts {
		if strings.Contains(a, "refusing orphan prune") && strings.Contains(a, "dropped from") {
			foundAlert = true
			break
		}
	}
	if !foundAlert {
		t.Fatalf("expected drastic-drop alert, got %v", res.Alerts)
	}
	// Present gateway endpoint should be created/updated (new host:port 45001).
	if len(res.CreatedProxies)+len(res.UpdatedProxies) < 1 {
		t.Fatalf("expected present spec upserted, created=%d updated=%d", len(res.CreatedProxies), len(res.UpdatedProxies))
	}
}

// W1: small pool 3 local + 1 gateway (drop 2) must refuse prune/SetMembers on first round.
func TestWarpSync_DrasticDropSmallPoolRefuses(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/pools/snapshot", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(WarpPoolSnapshot{
			Instances: []WarpInstance{
				// Match first seed host:port so localWarpN stays stable across rounds.
				{ID: "keep", Name: "small-01", ListenHost: "127.0.0.1", ListenPort: 48001, Status: "running"},
			},
			HealthyCount: 1,
			TotalCount:   1,
		})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	client := NewWarpGatewayClient(WarpGatewayConfig{Enabled: true, BaseURL: srv.URL, Timeout: time.Second})
	proxyRepo := newMemProxyRepo()
	seedIDs := make([]int64, 0, 3)
	for i := 1; i <= 3; i++ {
		externalID := fmt.Sprintf("small-%02d", i)
		if i == 1 {
			externalID = "keep"
		}
		p := &Proxy{
			Name:       fmt.Sprintf("warp-small-%02d", i),
			Protocol:   "socks5",
			Host:       "127.0.0.1",
			Port:       48000 + i,
			Status:     StatusActive,
			ManagedBy:  warpProxyManagedBy,
			ExternalID: externalID,
		}
		if err := proxyRepo.Create(context.Background(), p); err != nil {
			t.Fatal(err)
		}
		seedIDs = append(seedIDs, p.ID)
	}
	groupRepo := newMemGroupRepo()
	groupSvc := NewProxyGroupService(groupRepo, proxyRepo, noopResolver{})
	createdGroup, err := groupSvc.Create(context.Background(), CreateProxyGroupInput{
		Name: "warp-pool", Strategy: ProxyGroupStrategySticky, ProxyIDs: seedIDs,
	})
	if err != nil {
		t.Fatal(err)
	}

	cfg := &config.Config{Warp: config.WarpConfig{
		Enabled: true, DefaultGroupName: "warp-pool", AutoDetachUnhealthy: false,
		Gateway: config.WarpGatewayConfig{BaseURL: srv.URL},
	}}
	svc := NewWarpSyncService(cfg, client, proxyRepo, groupSvc, nil)

	res, err := svc.SyncFromGateway(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	if len(res.DeletedProxies) != 0 {
		t.Fatalf("small-pool drastic must not delete, deleted=%+v", res.DeletedProxies)
	}
	if len(proxyRepo.proxies) != 3 {
		t.Fatalf("small-pool inventory shrank: n=%d", len(proxyRepo.proxies))
	}
	if got := groupRepo.memberCount(createdGroup.ID); got != 3 {
		t.Fatalf("small-pool refuse must skip SetMembers; members=%d want 3", got)
	}
	found := false
	for _, a := range res.Alerts {
		if strings.Contains(a, "refusing orphan prune") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected refusing alert for 3→1, got %v", res.Alerts)
	}
}

// After drasticPruneConfirmRounds consistent shapes + matching confirm snapshot, allow prune.
func TestWarpSync_DrasticDropConfirmedAllowsPrune(t *testing.T) {
	mux := http.NewServeMux()
	// Stable drastic snapshot: keep first seed endpoint so (localWarpN, specN) shape holds.
	mux.HandleFunc("/v1/pools/snapshot", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(WarpPoolSnapshot{
			Instances: []WarpInstance{
				{ID: "keep", Name: "conf-01", ListenHost: "127.0.0.1", ListenPort: 49001, Status: "running"},
			},
			HealthyCount: 1,
			TotalCount:   1,
		})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	client := NewWarpGatewayClient(WarpGatewayConfig{Enabled: true, BaseURL: srv.URL, Timeout: time.Second})
	proxyRepo := newMemProxyRepo()
	seedIDs := make([]int64, 0, 6)
	for i := 1; i <= 6; i++ {
		externalID := fmt.Sprintf("conf-%02d", i)
		if i == 1 {
			externalID = "keep"
		}
		p := &Proxy{
			Name:       fmt.Sprintf("warp-conf-%02d", i),
			Protocol:   "socks5",
			Host:       "127.0.0.1",
			Port:       49000 + i,
			Status:     StatusActive,
			ManagedBy:  warpProxyManagedBy,
			ExternalID: externalID,
		}
		if err := proxyRepo.Create(context.Background(), p); err != nil {
			t.Fatal(err)
		}
		seedIDs = append(seedIDs, p.ID)
	}
	groupRepo := newMemGroupRepo()
	groupSvc := NewProxyGroupService(groupRepo, proxyRepo, noopResolver{})
	createdGroup, err := groupSvc.Create(context.Background(), CreateProxyGroupInput{
		Name: "warp-pool", Strategy: ProxyGroupStrategySticky, ProxyIDs: seedIDs,
	})
	if err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{Warp: config.WarpConfig{
		Enabled: true, DefaultGroupName: "warp-pool", AutoDetachUnhealthy: false,
		Gateway: config.WarpGatewayConfig{BaseURL: srv.URL},
	}}
	svc := NewWarpSyncService(cfg, client, proxyRepo, groupSvc, nil)

	// Rounds 1..N-1: refuse; round N: confirm snapshot matches → allow prune.
	for round := 1; round <= drasticPruneConfirmRounds; round++ {
		res, err := svc.SyncFromGateway(context.Background(), "")
		if err != nil {
			t.Fatalf("round %d: %v", round, err)
		}
		if round < drasticPruneConfirmRounds {
			if len(res.DeletedProxies) != 0 {
				t.Fatalf("round %d: must refuse prune, deleted=%+v", round, res.DeletedProxies)
			}
			if got := groupRepo.memberCount(createdGroup.ID); got != 6 {
				t.Fatalf("round %d: SetMembers must be skipped, members=%d", round, got)
			}
			continue
		}
		// Final armed round + confirm ok.
		if len(res.DeletedProxies) == 0 {
			t.Fatalf("round %d: expected orphan prune after confirm, alerts=%v", round, res.Alerts)
		}
		foundAllow := false
		for _, a := range res.Alerts {
			if strings.Contains(a, "allowing orphan prune") {
				foundAllow = true
				break
			}
		}
		if !foundAllow {
			t.Fatalf("round %d: expected allowing alert, got %v", round, res.Alerts)
		}
		// Only the kept endpoint (+ group shrunk via SetMembers).
		if len(proxyRepo.proxies) != 1 {
			t.Fatalf("round %d: want 1 local warp left, got %d", round, len(proxyRepo.proxies))
		}
		if got := groupRepo.memberCount(createdGroup.ID); got != 1 {
			t.Fatalf("round %d: want 1 group member after allow, got %d ids=%v", round, got, groupRepo.members[createdGroup.ID])
		}
		if svc.drasticStreak != 0 {
			t.Fatalf("round %d: streak should reset after allow, streak=%d", round, svc.drasticStreak)
		}
	}
}

// W2: when confirm snapshot shape differs, refuse prune and reset streak.
func TestWarpSync_DrasticDropConfirmMismatchRefuses(t *testing.T) {
	var calls atomic.Int32
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/pools/snapshot", func(w http.ResponseWriter, r *http.Request) {
		n := calls.Add(1)
		// Calls 1..drasticPruneConfirmRounds: main snapshots (drastic 1-of-6).
		// Call drasticPruneConfirmRounds+1: confirm on the armed round — recover full pool.
		if int(n) <= drasticPruneConfirmRounds {
			_ = json.NewEncoder(w).Encode(WarpPoolSnapshot{
				Instances: []WarpInstance{
					{ID: "keep", Name: "mm-01", ListenHost: "127.0.0.1", ListenPort: 50001, Status: "running"},
				},
				HealthyCount: 1,
				TotalCount:   1,
			})
			return
		}
		// Confirm mismatch: full inventory again (not drastic / different specN).
		insts := make([]WarpInstance, 0, 6)
		for i := 1; i <= 6; i++ {
			insts = append(insts, WarpInstance{
				ID: fmt.Sprintf("i%d", i), Name: fmt.Sprintf("mm-%02d", i),
				ListenHost: "127.0.0.1", ListenPort: 50000 + i, Status: "running",
			})
		}
		_ = json.NewEncoder(w).Encode(WarpPoolSnapshot{
			Instances: insts, HealthyCount: 6, TotalCount: 6,
		})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	client := NewWarpGatewayClient(WarpGatewayConfig{Enabled: true, BaseURL: srv.URL, Timeout: time.Second})
	proxyRepo := newMemProxyRepo()
	seedIDs := make([]int64, 0, 6)
	for i := 1; i <= 6; i++ {
		externalID := fmt.Sprintf("mm-%02d", i)
		if i == 1 {
			externalID = "keep"
		}
		p := &Proxy{
			Name:       fmt.Sprintf("warp-mm-%02d", i),
			Protocol:   "socks5",
			Host:       "127.0.0.1",
			Port:       50000 + i,
			Status:     StatusActive,
			ManagedBy:  warpProxyManagedBy,
			ExternalID: externalID,
		}
		if err := proxyRepo.Create(context.Background(), p); err != nil {
			t.Fatal(err)
		}
		seedIDs = append(seedIDs, p.ID)
	}
	groupRepo := newMemGroupRepo()
	groupSvc := NewProxyGroupService(groupRepo, proxyRepo, noopResolver{})
	createdGroup, err := groupSvc.Create(context.Background(), CreateProxyGroupInput{
		Name: "warp-pool", Strategy: ProxyGroupStrategySticky, ProxyIDs: seedIDs,
	})
	if err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{Warp: config.WarpConfig{
		Enabled: true, DefaultGroupName: "warp-pool", AutoDetachUnhealthy: false,
		Gateway: config.WarpGatewayConfig{BaseURL: srv.URL},
	}}
	svc := NewWarpSyncService(cfg, client, proxyRepo, groupSvc, nil)

	for round := 1; round <= drasticPruneConfirmRounds; round++ {
		res, err := svc.SyncFromGateway(context.Background(), "")
		if err != nil {
			t.Fatalf("round %d: %v", round, err)
		}
		if len(res.DeletedProxies) != 0 {
			t.Fatalf("round %d: must not prune, deleted=%+v", round, res.DeletedProxies)
		}
		if got := groupRepo.memberCount(createdGroup.ID); got != 6 {
			t.Fatalf("round %d: members must stay 6, got %d", round, got)
		}
		if round < drasticPruneConfirmRounds {
			continue
		}
		found := false
		for _, a := range res.Alerts {
			if strings.Contains(a, "drastic confirm mismatch") {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("round %d: expected confirm mismatch alert, got %v", round, res.Alerts)
		}
		if svc.drasticStreak != 0 {
			t.Fatalf("round %d: streak must reset on confirm mismatch, streak=%d", round, svc.drasticStreak)
		}
	}
	if len(proxyRepo.proxies) != 6 {
		t.Fatalf("inventory must stay intact after confirm mismatch: n=%d", len(proxyRepo.proxies))
	}
	// Confirm call must have been issued on the armed round.
	if got := calls.Load(); got < int32(drasticPruneConfirmRounds+1) {
		t.Fatalf("expected at least %d snapshot calls (incl confirm), got %d", drasticPruneConfirmRounds+1, got)
	}
}

// C3: TotalCount != specN must not advance drastic streak toward confirmed prune.
func TestWarpSync_DrasticDropInconsistentTotalCountRefuses(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/pools/snapshot", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(WarpPoolSnapshot{
			Instances: []WarpInstance{
				{ID: "keep", Name: "keep", ListenHost: "127.0.0.1", ListenPort: 47001, Status: "running"},
			},
			HealthyCount: 1,
			// Stale TotalCount pretends full pool still exists while Instances is truncated.
			TotalCount: 6,
		})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	client := NewWarpGatewayClient(WarpGatewayConfig{Enabled: true, BaseURL: srv.URL, Timeout: time.Second})
	proxyRepo := newMemProxyRepo()
	for i := 1; i <= 6; i++ {
		p := &Proxy{
			Name: fmt.Sprintf("warp-tot-%02d", i), Protocol: "socks5",
			Host: "127.0.0.1", Port: 47100 + i, Status: StatusActive,
			ManagedBy: warpProxyManagedBy, ExternalID: fmt.Sprintf("tot-%02d", i),
		}
		if err := proxyRepo.Create(context.Background(), p); err != nil {
			t.Fatal(err)
		}
	}
	groupRepo := newMemGroupRepo()
	groupSvc := NewProxyGroupService(groupRepo, proxyRepo, noopResolver{})
	cfg := &config.Config{Warp: config.WarpConfig{
		Enabled: true, DefaultGroupName: "warp-pool", AutoDetachUnhealthy: false,
		Gateway: config.WarpGatewayConfig{BaseURL: srv.URL},
	}}
	svc := NewWarpSyncService(cfg, client, proxyRepo, groupSvc, nil)

	// Even after many rounds, inconsistent TotalCount must not allow prune.
	for round := 0; round < drasticPruneConfirmRounds+2; round++ {
		res, err := svc.SyncFromGateway(context.Background(), "")
		if err != nil {
			t.Fatalf("round %d: %v", round, err)
		}
		if len(res.DeletedProxies) != 0 {
			t.Fatalf("round %d: must not prune on inconsistent TotalCount, deleted=%+v", round, res.DeletedProxies)
		}
		if svc.drasticStreak != 0 {
			t.Fatalf("round %d: streak must not advance on inconsistent TotalCount, streak=%d", round, svc.drasticStreak)
		}
		found := false
		for _, a := range res.Alerts {
			if strings.Contains(a, "TotalCount") && strings.Contains(a, "inconsistent") {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("round %d: expected TotalCount inconsistent alert, got %v", round, res.Alerts)
		}
	}
	if len(proxyRepo.proxies) < 6 {
		t.Fatalf("inventory shrank: n=%d", len(proxyRepo.proxies))
	}
}

// TotalCount>0 with empty Instances is inconsistent empty: refuse prune like empty breaker.
func TestWarpSync_InconsistentEmptyTotalCountRefusesPrune(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/pools/snapshot", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(WarpPoolSnapshot{
			Instances:    nil,
			HealthyCount: 0,
			TotalCount:   5,
		})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	client := NewWarpGatewayClient(WarpGatewayConfig{Enabled: true, BaseURL: srv.URL, Timeout: time.Second})
	proxyRepo := newMemProxyRepo()
	existing := &Proxy{Name: "warp-keep-inconsistent", Protocol: "socks5", Host: "127.0.0.1", Port: 46001, Status: StatusActive, ManagedBy: warpProxyManagedBy, ExternalID: "keep-inconsistent"}
	if err := proxyRepo.Create(context.Background(), existing); err != nil {
		t.Fatal(err)
	}
	groupRepo := newMemGroupRepo()
	groupSvc := NewProxyGroupService(groupRepo, proxyRepo, noopResolver{})
	cfg := &config.Config{Warp: config.WarpConfig{
		Enabled: true, DefaultGroupName: "warp-pool",
		Gateway: config.WarpGatewayConfig{BaseURL: srv.URL},
	}}
	svc := NewWarpSyncService(cfg, client, proxyRepo, groupSvc, nil)

	res, err := svc.SyncFromGateway(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	if len(res.DeletedProxies) != 0 {
		t.Fatalf("must not delete on inconsistent empty, deleted=%+v", res.DeletedProxies)
	}
	if _, ok := proxyRepo.proxies[existing.ID]; !ok {
		t.Fatal("local warp proxy must remain after inconsistent empty snapshot")
	}
	found := false
	for _, a := range res.Alerts {
		if strings.Contains(a, "TotalCount") && strings.Contains(a, "refusing to prune") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected TotalCount mismatch alert, got %v", res.Alerts)
	}
}

// W4: BindAccountsToGroup must fail-closed when SyncFromGateway errors (no warn-and-continue).
func TestWarpSync_BindAccountsToGroup_SyncFailClosed(t *testing.T) {
	mux := http.NewServeMux()
	// ensureGroup may not hit snapshot; SyncFromGateway will.
	mux.HandleFunc("/v1/pools/snapshot", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "gateway down", http.StatusBadGateway)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	client := NewWarpGatewayClient(WarpGatewayConfig{Enabled: true, BaseURL: srv.URL, Timeout: time.Second})
	proxyRepo := newMemProxyRepo()
	groupRepo := newMemGroupRepo()
	groupSvc := NewProxyGroupService(groupRepo, proxyRepo, noopResolver{})
	// Pre-create group so ensureGroup succeeds before sync fails.
	if _, err := groupSvc.Create(context.Background(), CreateProxyGroupInput{
		Name: "warp-pool", Strategy: ProxyGroupStrategySticky,
	}); err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{Warp: config.WarpConfig{
		Enabled: true, DefaultGroupName: "warp-pool",
		Gateway: config.WarpGatewayConfig{BaseURL: srv.URL},
	}}
	// Embedded nil AccountRepository satisfies the interface for compile-time;
	// sync fails before any account method is invoked.
	svc := NewWarpSyncService(cfg, client, proxyRepo, groupSvc, &struct{ AccountRepository }{})

	res, err := svc.BindAccountsToGroup(context.Background(), []int64{1, 2}, "warp-pool", false)
	if err == nil {
		t.Fatal("expected bind to fail when sync fails")
	}
	if res != nil {
		t.Fatalf("fail-closed must not return partial bind result, got %+v", res)
	}
	if !strings.Contains(err.Error(), "Bad Gateway") && !strings.Contains(strings.ToLower(err.Error()), "gateway") &&
		!strings.Contains(err.Error(), "502") && !strings.Contains(err.Error(), "snapshot") {
		// Accept any sync transport/API error; just ensure it is not silently ignored.
		t.Logf("bind returned sync error (ok): %v", err)
	}
}

// W4: peer leader lock during bind → ErrWarpSyncBusy (409), not silent continue.
func TestWarpSync_BindAccountsToGroup_SyncBusyFailClosed(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/pools/snapshot", func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("snapshot must not run when peer holds lock")
		_ = json.NewEncoder(w).Encode(WarpPoolSnapshot{})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	lock := &fakeLeaderLockCache{}
	ok, err := lock.TryAcquireLeaderLock(context.Background(), warpSyncWorkerLockKey, "peer", time.Minute)
	if err != nil || !ok {
		t.Fatalf("seed lock: ok=%v err=%v", ok, err)
	}

	client := NewWarpGatewayClient(WarpGatewayConfig{Enabled: true, BaseURL: srv.URL, Timeout: time.Second})
	proxyRepo := newMemProxyRepo()
	groupRepo := newMemGroupRepo()
	groupSvc := NewProxyGroupService(groupRepo, proxyRepo, noopResolver{})
	if _, err := groupSvc.Create(context.Background(), CreateProxyGroupInput{
		Name: "warp-pool", Strategy: ProxyGroupStrategySticky,
	}); err != nil {
		t.Fatal(err)
	}
	svc := NewWarpSyncService(&config.Config{Warp: config.WarpConfig{
		Enabled: true, DefaultGroupName: "warp-pool",
		Gateway: config.WarpGatewayConfig{BaseURL: srv.URL},
	}}, client, proxyRepo, groupSvc, &struct{ AccountRepository }{})
	svc.SetLeaderLock(lock, "local", nil)

	_, err = svc.BindAccountsToGroup(context.Background(), []int64{42}, "", false)
	if !errors.Is(err, ErrWarpSyncBusy) {
		t.Fatalf("err=%v want ErrWarpSyncBusy", err)
	}
}

// W3: CreatePoolAndSyncEx wraps sync failure after gateway create succeeded.
func TestWarpSync_CreatePoolAndSyncEx_SyncFailWrapped(t *testing.T) {
	createHits := 0
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/pools", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			createHits++
			_ = json.NewEncoder(w).Encode(map[string]any{
				"instances": []WarpInstance{
					{ID: "n1", Name: "01", ListenHost: "127.0.0.1", ListenPort: 48001, Status: "running"},
				},
			})
			return
		}
		http.NotFound(w, r)
	})
	mux.HandleFunc("/v1/pools/snapshot", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "snapshot boom", http.StatusInternalServerError)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	client := NewWarpGatewayClient(WarpGatewayConfig{Enabled: true, BaseURL: srv.URL, Timeout: time.Second})
	svc := NewWarpSyncService(&config.Config{Warp: config.WarpConfig{
		Enabled: true, DefaultGroupName: "warp-pool",
		Gateway: config.WarpGatewayConfig{BaseURL: srv.URL},
	}}, client, newMemProxyRepo(), nil, nil)

	_, err := svc.CreatePoolAndSyncEx(context.Background(), "warp", 1, "warp-pool", false)
	if err == nil {
		t.Fatal("expected sync failure after create")
	}
	if createHits != 1 {
		t.Fatalf("createHits=%d want 1", createHits)
	}
	if !strings.Contains(err.Error(), "gateway create succeeded but sync failed") {
		t.Fatalf("want W3 wrap message, got: %v", err)
	}
}

// HealthAllAndSync acquires leadership before HealthAll; peer lock → busy, no health hit.
func TestWarpSync_HealthAllAndSyncBusyBeforeHealth(t *testing.T) {
	healthHits := 0
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/health/all", func(w http.ResponseWriter, r *http.Request) {
		healthHits++
		t.Errorf("HealthAll must not run when peer holds lock")
		_ = json.NewEncoder(w).Encode(map[string]any{})
	})
	mux.HandleFunc("/v1/pools/snapshot", func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("snapshot must not run when peer holds lock")
		_ = json.NewEncoder(w).Encode(WarpPoolSnapshot{})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	lock := &fakeLeaderLockCache{}
	ok, err := lock.TryAcquireLeaderLock(context.Background(), warpSyncWorkerLockKey, "peer", time.Minute)
	if err != nil || !ok {
		t.Fatalf("seed lock: ok=%v err=%v", ok, err)
	}

	client := NewWarpGatewayClient(WarpGatewayConfig{Enabled: true, BaseURL: srv.URL, Timeout: time.Second})
	svc := NewWarpSyncService(&config.Config{Warp: config.WarpConfig{
		Enabled: true, DefaultGroupName: "warp-pool",
		Gateway: config.WarpGatewayConfig{BaseURL: srv.URL},
	}}, client, newMemProxyRepo(), nil, nil)
	svc.SetLeaderLock(lock, "local", nil)

	_, err = svc.HealthAllAndSync(context.Background(), "warp-pool")
	if !errors.Is(err, ErrWarpSyncBusy) {
		t.Fatalf("err=%v want ErrWarpSyncBusy", err)
	}
	if healthHits != 0 {
		t.Fatalf("healthHits=%d want 0", healthHits)
	}
}
