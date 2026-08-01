package service

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
)

type memProxyRepo struct {
	nextID  int64
	proxies map[int64]*Proxy
}

func newMemProxyRepo() *memProxyRepo {
	return &memProxyRepo{nextID: 1, proxies: map[int64]*Proxy{}}
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
func (m *memProxyRepo) Delete(ctx context.Context, id int64) error { return nil }
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
	return 0, nil
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
	return nil, nil
}
func (m *memProxyRepo) CountByGroupID(ctx context.Context, groupID int64) (int64, error) {
	return 0, nil
}
func (m *memProxyRepo) UpdateHealthAudit(context.Context, int64, int, *time.Time, string) error { return nil }
func (m *memProxyRepo) GetHealthAudit(context.Context, int64) (int, *time.Time, string, error) {
	return 0, nil, "", nil
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
