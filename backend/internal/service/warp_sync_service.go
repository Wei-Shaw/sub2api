package service

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
)

// WarpSyncService syncs warp-gateway instances into proxies + proxy_groups (auto 落库).
type WarpSyncService struct {
	cfg         config.WarpConfig
	client      *WarpGatewayClient
	proxyRepo   ProxyRepository
	groupSvc    *ProxyGroupService
	accountRepo AccountRepository
	log         *slog.Logger
}

func NewWarpSyncService(
	cfg *config.Config,
	client *WarpGatewayClient,
	proxyRepo ProxyRepository,
	groupSvc *ProxyGroupService,
	accountRepo AccountRepository,
) *WarpSyncService {
	var wcfg config.WarpConfig
	if cfg != nil {
		wcfg = cfg.Warp
	}
	return &WarpSyncService{
		cfg:         wcfg,
		client:      client,
		proxyRepo:   proxyRepo,
		groupSvc:    groupSvc,
		accountRepo: accountRepo,
		log:         slog.Default().With("component", "warp_sync"),
	}
}

// ProvideWarpGatewayClient builds the HTTP client from app config.
func ProvideWarpGatewayClient(cfg *config.Config) *WarpGatewayClient {
	if cfg == nil {
		return NewWarpGatewayClient(WarpGatewayConfig{})
	}
	timeout := time.Duration(cfg.Warp.Gateway.TimeoutMS) * time.Millisecond
	if timeout <= 0 {
		timeout = 3 * time.Second
	}
	return NewWarpGatewayClient(WarpGatewayConfig{
		Enabled:               cfg.Warp.Enabled,
		BaseURL:               cfg.Warp.Gateway.BaseURL,
		Token:                 cfg.Warp.Gateway.Token,
		Timeout:               timeout,
		ReconcileInterval:     time.Duration(cfg.Warp.Gateway.ReconcileInterval) * time.Second,
		TLSCAFile:             cfg.Warp.Gateway.TLSCAFile,
		TLSCertFile:           cfg.Warp.Gateway.TLSCertFile,
		TLSKeyFile:            cfg.Warp.Gateway.TLSKeyFile,
		TLSInsecureSkipVerify: cfg.Warp.Gateway.TLSInsecureSkipVerify,
	})
}

// WarpSyncResult is returned by SyncFromGateway / CreatePoolAndSync.
type WarpSyncResult struct {
	Snapshot       *WarpPoolSnapshot      `json:"snapshot"`
	Plan           WarpPoolAttachPlan     `json:"plan"`
	CreatedProxies []Proxy                `json:"created_proxies"`
	UpdatedProxies []Proxy                `json:"updated_proxies"`
	Group          *ProxyGroupWithProxies `json:"group,omitempty"`
	MemberIDs      []int64                `json:"member_ids"`
	DetachedIDs    []int64                `json:"detached_ids"`
	Alerts         []string               `json:"alerts,omitempty"`
}

func (s *WarpSyncService) Enabled() bool {
	return s != nil && s.client != nil && s.client.Enabled()
}

// Snapshot proxies gateway pool view.
func (s *WarpSyncService) Snapshot(ctx context.Context) (*WarpPoolSnapshot, error) {
	if !s.Enabled() {
		return nil, fmt.Errorf("warp gateway is disabled (set warp.enabled=true and gateway.base_url)")
	}
	return s.client.PoolSnapshot(ctx)
}

// ListInstances lists gateway instances.
func (s *WarpSyncService) ListInstances(ctx context.Context) ([]WarpInstance, error) {
	if !s.Enabled() {
		return nil, fmt.Errorf("warp gateway is disabled")
	}
	return s.client.ListInstances(ctx)
}

// CreatePoolAndSync creates N instances on gateway then syncs into DB + group.
// When register is true (or gateway runtime is sing-box), free WARP profiles are auto-registered.
func (s *WarpSyncService) CreatePoolAndSync(ctx context.Context, namePrefix string, count int, groupName string) (*WarpSyncResult, error) {
	return s.CreatePoolAndSyncEx(ctx, namePrefix, count, groupName, false)
}

// CreatePoolAndSyncEx adds register flag for real Cloudflare WARP pools.
func (s *WarpSyncService) CreatePoolAndSyncEx(ctx context.Context, namePrefix string, count int, groupName string, register bool) (*WarpSyncResult, error) {
	if !s.Enabled() {
		return nil, fmt.Errorf("warp gateway is disabled")
	}
	if count <= 0 {
		return nil, fmt.Errorf("count must be > 0")
	}
	if _, err := s.client.CreatePoolEx(ctx, namePrefix, count, register); err != nil {
		return nil, err
	}
	return s.SyncFromGateway(ctx, groupName)
}

// RegisterProfilesAndSync registers free WARP profiles into a new pool then syncs.
func (s *WarpSyncService) RegisterProfilesAndSync(ctx context.Context, namePrefix string, count int, groupName string) (*WarpSyncResult, error) {
	return s.CreatePoolAndSyncEx(ctx, namePrefix, count, groupName, true)
}

// WarpBindAccountsResult is returned by BindAccountsToGroup.
type WarpBindAccountsResult struct {
	GroupID    int64    `json:"group_id"`
	GroupName  string   `json:"group_name"`
	UpdatedIDs []int64  `json:"updated_ids"`
	SkippedIDs []int64  `json:"skipped_ids,omitempty"`
	Failed     []string `json:"failed,omitempty"`
}

// BindAccountsToGroup sets proxy_group_id for the given accounts to the WARP pool group.
// If groupName is empty, uses DefaultGroupName / warp-pool.
// If accountIDs is empty and bindAllActive is true, binds all active accounts.
func (s *WarpSyncService) BindAccountsToGroup(ctx context.Context, accountIDs []int64, groupName string, bindAllActive bool) (*WarpBindAccountsResult, error) {
	if !s.Enabled() {
		return nil, fmt.Errorf("warp gateway is disabled")
	}
	if s.accountRepo == nil {
		return nil, fmt.Errorf("account repository not configured")
	}
	if groupName == "" {
		groupName = s.cfg.DefaultGroupName
	}
	if groupName == "" {
		groupName = "warp-pool"
	}
	// Ensure group exists (sync first if needed).
	group, err := s.ensureGroup(ctx, groupName)
	if err != nil {
		return nil, err
	}
	// Ensure group has members from latest gateway snapshot.
	if _, err := s.SyncFromGateway(ctx, groupName); err != nil {
		s.log.Warn("bind: sync before bind failed", "err", err)
	}
	// re-load group after sync
	if g2, err := s.ensureGroup(ctx, groupName); err == nil {
		group = g2
	}

	ids := accountIDs
	if bindAllActive && len(ids) == 0 {
		active, err := s.accountRepo.ListActive(ctx)
		if err != nil {
			return nil, err
		}
		for _, a := range active {
			ids = append(ids, a.ID)
		}
	}
	if len(ids) == 0 {
		return nil, fmt.Errorf("no account ids provided")
	}

	result := &WarpBindAccountsResult{
		GroupID:   group.ID,
		GroupName: group.Name,
	}
	gid := group.ID
	for _, id := range ids {
		acc, err := s.accountRepo.GetByID(ctx, id)
		if err != nil {
			result.Failed = append(result.Failed, fmt.Sprintf("%d: %v", id, err))
			continue
		}
		if acc.ProxyGroupID != nil && *acc.ProxyGroupID == gid {
			result.SkippedIDs = append(result.SkippedIDs, id)
			continue
		}
		// Clear single proxy so group takes effect.
		acc.ProxyID = nil
		acc.ProxyGroupID = &gid
		if err := s.accountRepo.Update(ctx, acc); err != nil {
			result.Failed = append(result.Failed, fmt.Sprintf("%d: %v", id, err))
			continue
		}
		result.UpdatedIDs = append(result.UpdatedIDs, id)
	}
	return result, nil
}

// HealthAllAndSync runs gateway health then syncs (unhealthy → detach if configured).
func (s *WarpSyncService) HealthAllAndSync(ctx context.Context, groupName string) (*WarpSyncResult, error) {
	if !s.Enabled() {
		return nil, fmt.Errorf("warp gateway is disabled")
	}
	if _, _, _, err := s.client.HealthAll(ctx); err != nil {
		return nil, err
	}
	return s.SyncFromGateway(ctx, groupName)
}

// RotateAndSync rotates one gateway instance and re-syncs the pool.
func (s *WarpSyncService) RotateAndSync(ctx context.Context, instanceID, groupName string) (*WarpSyncResult, error) {
	if !s.Enabled() {
		return nil, fmt.Errorf("warp gateway is disabled")
	}
	if _, err := s.client.Rotate(ctx, instanceID); err != nil {
		return nil, err
	}
	return s.SyncFromGateway(ctx, groupName)
}

// SyncFromGateway pulls snapshot and upserts proxies + group membership.
func (s *WarpSyncService) SyncFromGateway(ctx context.Context, groupName string) (*WarpSyncResult, error) {
	if !s.Enabled() {
		return nil, fmt.Errorf("warp gateway is disabled")
	}
	if s.proxyRepo == nil {
		return nil, fmt.Errorf("proxy repository not configured")
	}
	if groupName == "" {
		groupName = s.cfg.DefaultGroupName
	}
	if groupName == "" {
		groupName = "warp-pool"
	}

	snap, err := s.client.PoolSnapshot(ctx)
	if err != nil {
		return nil, err
	}
	plan := BuildAttachPlan(snap, groupName)
	result := &WarpSyncResult{Snapshot: snap, Plan: plan}

	if s.cfg.AlertDuplicateExitIP && len(plan.DuplicateExitIPs) > 0 {
		for ip, ids := range plan.DuplicateExitIPs {
			msg := fmt.Sprintf("duplicate exit_ip %s shared by instances %v", ip, ids)
			result.Alerts = append(result.Alerts, msg)
			s.log.Warn(msg)
		}
	}

	// Index existing proxies by host:port and name.
	byKey, byName, err := s.indexProxies(ctx)
	if err != nil {
		return nil, err
	}

	memberIDs := make([]int64, 0, len(plan.ProxySpecs))
	for _, spec := range plan.ProxySpecs {
		key := proxyHostPortKey(spec.Host, spec.Port)
		var p *Proxy
		if existing, ok := byKey[key]; ok {
			// update status/name
			needUpdate := existing.Name != spec.Name || existing.Protocol != spec.Protocol || existing.Status != spec.Status
			if needUpdate {
				existing.Name = spec.Name
				existing.Protocol = spec.Protocol
				existing.Status = spec.Status
				if err := s.proxyRepo.Update(ctx, &existing); err != nil {
					return nil, fmt.Errorf("update proxy %s: %w", spec.Name, err)
				}
				result.UpdatedProxies = append(result.UpdatedProxies, existing)
			}
			p = &existing
			byName[spec.Name] = existing
		} else if existing, ok := byName[spec.Name]; ok {
			existing.Host = spec.Host
			existing.Port = spec.Port
			existing.Protocol = spec.Protocol
			existing.Status = spec.Status
			if err := s.proxyRepo.Update(ctx, &existing); err != nil {
				return nil, fmt.Errorf("update proxy by name %s: %w", spec.Name, err)
			}
			result.UpdatedProxies = append(result.UpdatedProxies, existing)
			p = &existing
			byKey[key] = existing
		} else {
			created, err := s.proxyRepoCreate(ctx, spec)
			if err != nil {
				return nil, err
			}
			result.CreatedProxies = append(result.CreatedProxies, *created)
			p = created
			byKey[key] = *created
			byName[spec.Name] = *created
		}

		// Healthy/running members stay in group; unhealthy optionally excluded.
		include := p.Status == StatusActive
		if !s.cfg.AutoDetachUnhealthy {
			include = true
		}
		if include {
			memberIDs = append(memberIDs, p.ID)
		} else {
			result.DetachedIDs = append(result.DetachedIDs, p.ID)
		}
	}

	// Ensure group exists and set members.
	if s.groupSvc != nil {
		group, err := s.ensureGroup(ctx, groupName)
		if err != nil {
			return nil, err
		}
		withMembers, err := s.groupSvc.SetMembers(ctx, group.ID, memberIDs)
		if err != nil {
			return nil, fmt.Errorf("set group members: %w", err)
		}
		result.Group = withMembers
	}
	result.MemberIDs = memberIDs
	return result, nil
}

func (s *WarpSyncService) proxyRepoCreate(ctx context.Context, spec WarpProxySpec) (*Proxy, error) {
	p := &Proxy{
		Name:     spec.Name,
		Protocol: spec.Protocol,
		Host:     spec.Host,
		Port:     spec.Port,
		Status:   spec.Status,
	}
	if p.Status == "" {
		p.Status = StatusActive
	}
	if err := s.proxyRepo.Create(ctx, p); err != nil {
		return nil, fmt.Errorf("create proxy %s: %w", spec.Name, err)
	}
	return p, nil
}

func (s *WarpSyncService) ensureGroup(ctx context.Context, name string) (*ProxyGroup, error) {
	if s.groupSvc == nil {
		return nil, fmt.Errorf("proxy group service not configured")
	}
	active, err := s.groupSvc.ListActive(ctx)
	if err != nil {
		return nil, err
	}
	for i := range active {
		if active[i].Name == name {
			return &active[i], nil
		}
	}
	// Also search non-active via List
	groups, _, err := s.groupSvc.List(ctx, pagination.PaginationParams{Page: 1, PageSize: 200})
	if err == nil {
		for i := range groups {
			if groups[i].Name == name {
				g := groups[i].ProxyGroup
				if g.Status != StatusActive {
					// reactivate
					st := StatusActive
					_, err := s.groupSvc.Update(ctx, g.ID, UpdateProxyGroupInput{Status: &st})
					if err != nil {
						return nil, err
					}
					got, err := s.groupSvc.GetByID(ctx, g.ID)
					if err != nil {
						return nil, err
					}
					return &got.ProxyGroup, nil
				}
				return &g, nil
			}
		}
	}
	created, err := s.groupSvc.Create(ctx, CreateProxyGroupInput{
		Name:            name,
		Description:     "Cloudflare WARP proxy pool (auto-managed by warp-gateway sync)",
		Strategy:        ProxyGroupStrategySticky,
		StickyByAccount: true,
	})
	if err != nil {
		return nil, fmt.Errorf("create proxy group %q: %w", name, err)
	}
	return &created.ProxyGroup, nil
}

func (s *WarpSyncService) indexProxies(ctx context.Context) (byKey map[string]Proxy, byName map[string]Proxy, err error) {
	byKey = make(map[string]Proxy)
	byName = make(map[string]Proxy)
	// Prefer filtered list for warp- prefix; fall back to paging.
	list, _, listErr := s.proxyRepo.ListWithFilters(ctx, pagination.PaginationParams{Page: 1, PageSize: 1000}, "", "", "warp-")
	if listErr != nil {
		// fallback ListActive only
		active, aerr := s.proxyRepo.ListActive(ctx)
		if aerr != nil {
			return nil, nil, aerr
		}
		list = active
	}
	for _, p := range list {
		byKey[proxyHostPortKey(p.Host, p.Port)] = p
		byName[p.Name] = p
	}
	// Also index all active (in case name filter missed)
	active, aerr := s.proxyRepo.ListActive(ctx)
	if aerr == nil {
		for _, p := range active {
			byKey[proxyHostPortKey(p.Host, p.Port)] = p
			byName[p.Name] = p
		}
	}
	return byKey, byName, nil
}

func proxyHostPortKey(host string, port int) string {
	return strings.ToLower(strings.TrimSpace(host)) + ":" + fmt.Sprintf("%d", port)
}
