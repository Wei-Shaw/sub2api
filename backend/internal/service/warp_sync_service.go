package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
)

// ErrWarpSyncBusy is returned when process-local sync is already running or the
// peer leader lock could not be acquired (another instance is syncing) — 409.
var ErrWarpSyncBusy = infraerrors.Conflict("WARP_SYNC_BUSY", "warp sync already running")

// ErrWarpSyncLockUnavailable is returned when the leader-lock backend (Redis)
// errors on acquire — peer may still hold the lock; do not run mutate/sync — 503.
var ErrWarpSyncLockUnavailable = infraerrors.ServiceUnavailable(
	"WARP_SYNC_LOCK_UNAVAILABLE", "warp sync leader lock backend unavailable")

// WarpSyncService syncs warp-gateway instances into proxies + proxy_groups (auto 落库).
type WarpSyncService struct {
	cfg         config.WarpConfig
	client      *WarpGatewayClient
	proxyRepo   ProxyRepository
	groupSvc    *ProxyGroupService
	accountRepo AccountRepository
	log         *slog.Logger

	// Shared leader lock for worker tick + admin Sync so multi-instance and
	// concurrent manual syncs do not race on upsert/orphan prune.
	lock       LeaderLockCache
	db         *sql.DB
	instanceID string
	syncMu     sync.Mutex
	syncActive bool

	// drasticStreak counts consecutive drastic-drop snapshots with the same
	// (localWarpN, specN) shape. After drasticPruneConfirmRounds, orphan prune
	// is allowed so intentional shrink is not permanently blocked.
	drasticStreak   int
	drasticLocalN   int
	drasticSpecN    int
	drasticStreakMu sync.Mutex
}

// drasticPruneConfirmRounds is how many consecutive drastic-drop observations
// are required before orphan prune is permitted (process-local counter).
const (
	drasticPruneConfirmRounds = 3
	warpProxyManagedBy        = "warp-gateway"
)

// isDrasticWarpDrop reports whether gateway returned far fewer specs than local
// warp-* inventory. Protects small pools (e.g. 3→1 drop 2) as well as large ones:
//   - relative: specs less than half of local
//   - absolute: would remove at least 2 warps
//
// 3→1 drastic; 4→1 drastic; 10→4 drastic; 3→2 not; 2→1 not.
func isDrasticWarpDrop(localWarpN, specN int) bool {
	return specN > 0 && localWarpN >= 2 && specN*2 < localWarpN && (localWarpN-specN) >= 2
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

// SetLeaderLock wires the distributed lock shared with WarpSyncWorker.
// db is only used when lock is nil (advisory single-flight). Redis error → skip.
func (s *WarpSyncService) SetLeaderLock(lock LeaderLockCache, instanceID string, db *sql.DB) {
	if s == nil {
		return
	}
	s.lock = lock
	s.db = db
	if instanceID != "" {
		s.instanceID = instanceID
	}
}

// ProvideWarpGatewayClient builds the HTTP client from app config.
func ProvideWarpGatewayClient(cfg *config.Config) (*WarpGatewayClient, error) {
	if cfg == nil {
		return newWarpGatewayClient(WarpGatewayConfig{})
	}
	timeout := time.Duration(cfg.Warp.Gateway.TimeoutMS) * time.Millisecond
	if timeout <= 0 {
		timeout = 3 * time.Second
	}
	return newWarpGatewayClient(WarpGatewayConfig{
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
	DeletedProxies []Proxy                `json:"deleted_proxies,omitempty"`
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
// Gateway mutate + sync share one leadership lock so a peer holding the lock
// cannot leave a partial fork (create succeeded, sync skipped).
func (s *WarpSyncService) CreatePoolAndSyncEx(ctx context.Context, namePrefix string, count int, groupName string, register bool) (*WarpSyncResult, error) {
	if !s.Enabled() {
		return nil, fmt.Errorf("warp gateway is disabled")
	}
	if count <= 0 {
		return nil, fmt.Errorf("count must be > 0")
	}
	// Soft cap protects gateway + DB from accidental bulk create storms.
	if count > 50 {
		return nil, infraerrors.BadRequest("WARP_POOL_COUNT_TOO_LARGE", "count must be <= 50")
	}
	return s.withSyncLeadershipTTL(ctx, s.createLockTTL(count, register), func(ctx context.Context) (*WarpSyncResult, error) {
		if ctx.Err() != nil {
			return nil, fmt.Errorf("leadership lost before create: %w", ctx.Err())
		}
		created, createErr := s.client.CreatePoolEx(ctx, namePrefix, count, register)
		if createErr != nil && len(created) == 0 {
			if ctx.Err() != nil {
				return nil, fmt.Errorf("gateway create may have completed but leadership lost/cancelled before sync: %w", ctx.Err())
			}
			return nil, createErr
		}
		// Create may have committed on gateway; if leadership/parent canceled before
		// sync, surface partial success so callers do not assume full roll-forward.
		if err := ctx.Err(); err != nil {
			return nil, fmt.Errorf("gateway create may have completed but leadership lost/cancelled before sync: %w", err)
		}
		res, err := s.syncFromGatewayLocked(ctx, groupName)
		if err != nil {
			// W3: create already committed on gateway — surface partial success clearly.
			return nil, fmt.Errorf("gateway create succeeded but sync failed (instances may exist on gateway): %w", err)
		}
		if createErr != nil {
			res.Alerts = append(res.Alerts, fmt.Sprintf("gateway pool creation partially succeeded (%d created): %v", len(created), createErr))
		}
		return res, nil
	})
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
	// W4: fail-closed — bind must not proceed on a stale/empty pool when sync fails
	// (including ErrWarpSyncBusy → 409). Previously Warn-and-continue left accounts
	// pointed at an unsynced group.
	if _, err := s.SyncFromGateway(ctx, groupName); err != nil {
		return nil, err
	}
	// re-load group after sync
	if g2, err := s.ensureGroup(ctx, groupName); err == nil {
		group = g2
	}
	// Require at least one member so bind does not attach accounts to an empty pool.
	if s.proxyRepo != nil {
		n, cerr := s.proxyRepo.CountByGroupID(ctx, group.ID)
		if cerr != nil {
			return nil, fmt.Errorf("count warp group members: %w", cerr)
		}
		if n == 0 {
			return nil, infraerrors.BadRequest("WARP_GROUP_EMPTY", "warp pool group has no members after sync; create/sync pool first")
		}
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
// HealthAll and sync share one leadership lock so peer contention returns
// ErrWarpSyncBusy before either mutates local inventory.
func (s *WarpSyncService) HealthAllAndSync(ctx context.Context, groupName string) (*WarpSyncResult, error) {
	if !s.Enabled() {
		return nil, fmt.Errorf("warp gateway is disabled")
	}
	return s.withSyncLeadership(ctx, func(ctx context.Context) (*WarpSyncResult, error) {
		if ctx.Err() != nil {
			return nil, fmt.Errorf("leadership lost before health: %w", ctx.Err())
		}
		if _, _, _, err := s.client.HealthAll(ctx); err != nil {
			if ctx.Err() != nil {
				return nil, fmt.Errorf("gateway health may have completed but leadership lost/cancelled before sync: %w", ctx.Err())
			}
			return nil, err
		}
		if err := ctx.Err(); err != nil {
			return nil, fmt.Errorf("gateway health may have completed but leadership lost/cancelled before sync: %w", err)
		}
		return s.syncFromGatewayLocked(ctx, groupName)
	})
}

// RotateAndSync rotates one gateway instance and re-syncs the pool.
func (s *WarpSyncService) RotateAndSync(ctx context.Context, instanceID, groupName string) (*WarpSyncResult, error) {
	if !s.Enabled() {
		return nil, fmt.Errorf("warp gateway is disabled")
	}
	return s.withSyncLeadership(ctx, func(ctx context.Context) (*WarpSyncResult, error) {
		if ctx.Err() != nil {
			return nil, fmt.Errorf("leadership lost before rotate: %w", ctx.Err())
		}
		if _, err := s.client.Rotate(ctx, instanceID); err != nil {
			if ctx.Err() != nil {
				return nil, fmt.Errorf("gateway rotate may have completed but leadership lost/cancelled before sync: %w", ctx.Err())
			}
			return nil, err
		}
		if err := ctx.Err(); err != nil {
			return nil, fmt.Errorf("gateway rotate may have completed but leadership lost/cancelled before sync: %w", err)
		}
		return s.syncFromGatewayLocked(ctx, groupName)
	})
}

// DeleteInstanceAndSync deletes a gateway instance (optionally CF unregister) then syncs proxies.
func (s *WarpSyncService) DeleteInstanceAndSync(ctx context.Context, instanceID, groupName string, deregisterCloudflare bool) (*WarpSyncResult, error) {
	if !s.Enabled() {
		return nil, fmt.Errorf("warp gateway is disabled")
	}
	if strings.TrimSpace(instanceID) == "" {
		return nil, fmt.Errorf("instance id required")
	}
	return s.withSyncLeadership(ctx, func(ctx context.Context) (*WarpSyncResult, error) {
		if ctx.Err() != nil {
			return nil, fmt.Errorf("leadership lost before delete: %w", ctx.Err())
		}
		if err := s.client.DeleteInstance(ctx, instanceID, deregisterCloudflare); err != nil {
			if ctx.Err() != nil {
				return nil, fmt.Errorf("gateway delete may have completed but leadership lost/cancelled before sync: %w", ctx.Err())
			}
			return nil, err
		}
		if err := ctx.Err(); err != nil {
			return nil, fmt.Errorf("gateway delete may have completed but leadership lost/cancelled before sync: %w", err)
		}
		return s.syncFromGatewayLocked(ctx, groupName)
	})
}

// SyncFromGateway pulls snapshot and upserts proxies + group membership.
// Process-local singleflight + shared leader lock gate worker ticks and admin
// Sync so concurrent runs cannot double-create or race orphan prune.
func (s *WarpSyncService) SyncFromGateway(ctx context.Context, groupName string) (*WarpSyncResult, error) {
	if !s.Enabled() {
		return nil, fmt.Errorf("warp gateway is disabled")
	}
	if s.proxyRepo == nil {
		return nil, fmt.Errorf("proxy repository not configured")
	}
	return s.withSyncLeadership(ctx, func(ctx context.Context) (*WarpSyncResult, error) {
		return s.syncFromGatewayLocked(ctx, groupName)
	})
}

// withSyncLeadership runs fn while holding process singleflight + leader lock.
// fn should call syncFromGatewayLocked or gateway mutate+sync.
// Returns ErrWarpSyncBusy (409) before any gateway mutate when the lock is held.
func (s *WarpSyncService) withSyncLeadership(ctx context.Context, fn func(ctx context.Context) (*WarpSyncResult, error)) (*WarpSyncResult, error) {
	return s.withSyncLeadershipTTL(ctx, s.syncLockTTL(), fn)
}

// withSyncLeadershipTTL is like withSyncLeadership but uses an explicit lock TTL
// (e.g. register create whose HTTP timeout far exceeds the default sync TTL).
// When the Redis lock implements LeaderLockRefresher, a heartbeat renews TTL
// every ttl/3 so long create+sync cannot be overtaken by a peer.
func (s *WarpSyncService) withSyncLeadershipTTL(ctx context.Context, ttl time.Duration, fn func(ctx context.Context) (*WarpSyncResult, error)) (*WarpSyncResult, error) {
	if s == nil {
		return nil, fmt.Errorf("warp sync service is nil")
	}
	if !s.Enabled() {
		return nil, fmt.Errorf("warp gateway is disabled")
	}
	if ttl <= 0 {
		ttl = s.syncLockTTL()
	}

	s.syncMu.Lock()
	if s.syncActive {
		s.syncMu.Unlock()
		return nil, ErrWarpSyncBusy
	}
	s.syncActive = true
	s.syncMu.Unlock()
	defer func() {
		s.syncMu.Lock()
		s.syncActive = false
		s.syncMu.Unlock()
	}()

	owner := s.instanceID
	if owner == "" {
		owner = "warp-sync"
	}
	// Independent short context so a long parent deadline (or cancelled admin
	// request) does not leave acquire hanging or race release.
	lockCtx, lockCancel := context.WithTimeout(context.Background(), 2*time.Second)
	release, ok, unavail := tryAcquireSingletonLeaderLockEx(lockCtx, s.lock, s.db, warpSyncWorkerLockKey, owner, ttl)
	lockCancel()
	if !ok {
		// Peer busy 409 vs Redis unavailable 503 — see tryAcquireSingletonLeaderLockEx.
		if unavail {
			s.log.Warn("warp sync skipped: leader lock backend unavailable")
			return nil, ErrWarpSyncLockUnavailable
		}
		s.log.Debug("warp sync skipped: leader lock held by peer")
		return nil, ErrWarpSyncBusy
	}
	if release != nil {
		defer release()
	}
	// Heartbeat ctx cancels when leadership is lost mid-sync so fn aborts.
	// CreatePoolEx / HealthAll / Rotate / DeleteInstance all receive hbCtx via fn.
	hbCtx, stopHB := startLeaderLockHeartbeat(ctx, s.lock, warpSyncWorkerLockKey, owner, ttl)
	defer stopHB()

	result, err := fn(hbCtx)
	if err != nil && hbCtx.Err() != nil && (errors.Is(err, context.Canceled) || errors.Is(hbCtx.Err(), context.Canceled)) {
		return result, fmt.Errorf("warp sync aborted: leadership lost: %w", err)
	}
	return result, err
}

// syncLockTTL is the leader-lock hold time for one withSyncLeadership critical section.
// Must exceed worst-case HealthAll+Sync (worker tick timeout aligns to this) with headroom.
// Floor 90s; ceiling 5m; prefers ReconcileInterval when larger than the floor.
// Heartbeat renews while held; ceiling is a crash-safety bound between renewals.
func (s *WarpSyncService) syncLockTTL() time.Duration {
	sec := 90
	if s.cfg.Gateway.ReconcileInterval > 0 {
		sec = s.cfg.Gateway.ReconcileInterval
	}
	ttl := time.Duration(sec) * time.Second
	if ttl < 90*time.Second {
		ttl = 90 * time.Second
	}
	if ttl > 5*time.Minute {
		ttl = 5 * time.Minute
	}
	return ttl
}

// createLockTTL covers CreatePoolEx HTTP timeout (especially register=true) plus
// a sync budget so leadership cannot expire mid-create while a peer HealthAllAndSyncs.
// Mirrors WarpGatewayClient.CreatePoolEx timeout selection.
func (s *WarpSyncService) createLockTTL(count int, register bool) time.Duration {
	base := s.syncLockTTL()
	if count < 1 {
		count = 1
	}
	createTO := 30 * time.Second
	if s.client != nil && s.client.cfg.Timeout > 0 {
		createTO = s.client.cfg.Timeout
	}
	if register {
		// Same formula as WarpGatewayClient.CreatePoolEx for real WARP handshakes.
		regTO := time.Duration(30+count*25) * time.Second
		if createTO < 120*time.Second {
			createTO = regTO
		}
		if createTO < regTO {
			createTO = regTO
		}
	}
	// Snapshot + upsert + prune headroom after gateway returns.
	syncBudget := 2*time.Minute + time.Duration(count)*2*time.Second
	ttl := createTO + syncBudget + 30*time.Second
	if ttl < base {
		ttl = base
	}
	// Hard cap: count=50 register is ~21min create; allow up to 30m + heartbeat.
	if ttl > 30*time.Minute {
		ttl = 30 * time.Minute
	}
	return ttl
}

func (s *WarpSyncService) syncFromGatewayLocked(ctx context.Context, groupName string) (*WarpSyncResult, error) {
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

	// Names/endpoints are used only for collision avoidance. Reconciliation
	// ownership is keyed exclusively by the persisted controller resource ID.
	index, err := s.indexProxies(ctx)
	if err != nil {
		return nil, err
	}
	byKey, byName, byExternalID := index.byKey, index.byName, index.byExternalID

	localWarpN := index.managedCount

	// Circuit breaker: successful empty gateway snapshot must not wipe local
	// warp-* inventory (transient gateway bug / empty body / wrong env).
	// Also treat TotalCount>0 with empty Instances as inconsistent empty.
	instancesEmpty := snap == nil || len(snap.Instances) == 0
	gatewayEmpty := instancesEmpty || len(plan.ProxySpecs) == 0
	inconsistentEmpty := snap != nil && snap.TotalCount > 0 && len(snap.Instances) == 0
	if gatewayEmpty {
		if localWarpN > 0 {
			var msg string
			if inconsistentEmpty {
				msg = fmt.Sprintf("inconsistent empty gateway snapshot: TotalCount=%d but Instances empty; refusing to prune %d local warp-* proxies", snap.TotalCount, localWarpN)
			} else {
				msg = fmt.Sprintf("empty gateway snapshot: refusing to prune %d local warp-* proxies", localWarpN)
			}
			result.Alerts = append(result.Alerts, msg)
			s.log.Warn(msg)
			// Skip orphan prune and SetMembers that would drop all warp members.
			return result, nil
		}
		if inconsistentEmpty {
			msg := fmt.Sprintf("inconsistent empty gateway snapshot: TotalCount=%d but Instances empty", snap.TotalCount)
			result.Alerts = append(result.Alerts, msg)
			s.log.Warn(msg)
			return result, nil
		}
	}

	// Drastic drop: gateway returned far fewer specs than local warp-* inventory.
	// Still upsert present specs. Refuse orphan prune + SetMembers replace for the
	// first few consecutive identical drastic observations so a flaky snapshot
	// cannot wipe inventory or shrink group membership used for routing.
	// After drasticPruneConfirmRounds with TotalCount==specN, take a second fresh
	// gateway snapshot before destructive prune/SetMembers (W2 confirm).
	//
	// C3: streak is process-local under leader singleflight (only the leader runs
	// sync). Leadership move resets streak → delayed prune (safe). TotalCount must
	// match len(ProxySpecs) before a round counts toward confirmed shrink so a
	// truncated Instances array with stale TotalCount cannot arm prune.
	allowOrphanPrune := true
	allowMemberReplace := true
	specN := len(plan.ProxySpecs)
	if isDrasticWarpDrop(localWarpN, specN) {
		totalConsistent := snap != nil && snap.TotalCount == specN
		needConfirmSnapshot := false
		s.drasticStreakMu.Lock()
		if !totalConsistent {
			// Shape mismatch: refuse prune/replace; do not advance streak.
			allowOrphanPrune = false
			allowMemberReplace = false
			tc := 0
			if snap != nil {
				tc = snap.TotalCount
			}
			msg := fmt.Sprintf("gateway snapshot dropped from %d local warp-* to %d specs (TotalCount=%d inconsistent); refusing orphan prune and member replace",
				localWarpN, specN, tc)
			result.Alerts = append(result.Alerts, msg)
			s.log.Warn(msg)
		} else {
			if s.drasticLocalN == localWarpN && s.drasticSpecN == specN {
				s.drasticStreak++
			} else {
				s.drasticStreak = 1
				s.drasticLocalN = localWarpN
				s.drasticSpecN = specN
			}
			streak := s.drasticStreak
			if streak < drasticPruneConfirmRounds {
				allowOrphanPrune = false
				allowMemberReplace = false
				msg := fmt.Sprintf("gateway snapshot dropped from %d local warp-* to %d specs; refusing orphan prune (%d/%d consistent rounds)",
					localWarpN, specN, streak, drasticPruneConfirmRounds)
				result.Alerts = append(result.Alerts, msg)
				s.log.Warn(msg)
			} else {
				// Streak armed — confirm with a fresh gateway snapshot before prune.
				needConfirmSnapshot = true
			}
		}
		s.drasticStreakMu.Unlock()

		if needConfirmSnapshot {
			// W2: second snapshot before allow prune / SetMembers.
			snap2, cerr := s.client.PoolSnapshot(ctx)
			if cerr != nil {
				allowOrphanPrune = false
				allowMemberReplace = false
				msg := fmt.Sprintf("gateway snapshot dropped from %d local warp-* to %d specs; drastic confirm snapshot failed; refusing prune: %v",
					localWarpN, specN, cerr)
				result.Alerts = append(result.Alerts, msg)
				s.log.Warn(msg)
				// Keep streak so next tick retries confirm.
			} else {
				plan2 := BuildAttachPlan(snap2, groupName)
				specN2 := len(plan2.ProxySpecs)
				confirmOK := snap2 != nil &&
					isDrasticWarpDrop(localWarpN, specN2) &&
					snap2.TotalCount == specN2 &&
					specN2 == specN
				if !confirmOK {
					allowOrphanPrune = false
					allowMemberReplace = false
					// Shape changed on confirm — reset streak (prefer re-arm from scratch).
					s.drasticStreakMu.Lock()
					s.drasticStreak = 0
					s.drasticLocalN = 0
					s.drasticSpecN = 0
					s.drasticStreakMu.Unlock()
					tc2 := 0
					if snap2 != nil {
						tc2 = snap2.TotalCount
					}
					msg := fmt.Sprintf("gateway snapshot dropped from %d local warp-* to %d specs; drastic confirm mismatch (confirmSpecs=%d TotalCount=%d); refusing prune",
						localWarpN, specN, specN2, tc2)
					result.Alerts = append(result.Alerts, msg)
					s.log.Warn(msg)
				} else {
					// Confirmed shrink: allow prune + SetMembers; reset streak so a later drop re-arms.
					s.drasticStreakMu.Lock()
					s.drasticStreak = 0
					s.drasticLocalN = 0
					s.drasticSpecN = 0
					s.drasticStreakMu.Unlock()
					msg := fmt.Sprintf("gateway snapshot dropped from %d local warp-* to %d specs; allowing orphan prune after %d consistent rounds",
						localWarpN, specN, drasticPruneConfirmRounds)
					result.Alerts = append(result.Alerts, msg)
					s.log.Warn(msg)
				}
			}
		}
	} else {
		s.drasticStreakMu.Lock()
		s.drasticStreak = 0
		s.drasticLocalN = 0
		s.drasticSpecN = 0
		s.drasticStreakMu.Unlock()
	}

	memberIDs := make([]int64, 0, len(plan.ProxySpecs))
	seenMember := map[int64]struct{}{}
	keepExternalIDs := map[string]struct{}{}
	for _, spec := range plan.ProxySpecs {
		if strings.TrimSpace(spec.ExternalID) == "" {
			return nil, fmt.Errorf("warp gateway instance %q is missing external id", spec.Name)
		}
		key := proxyHostPortKey(spec.Host, spec.Port)
		existing, exists := byExternalID[spec.ExternalID]
		if !exists {
			// Migration 194 marks only rows from the legacy auto-managed group.
			// Bind their stable external ID only on an exact endpoint match.
			if legacy, ok := index.legacyByKey[key]; ok {
				existing, exists = legacy, true
			}
		}
		if exists {
			oldKey := proxyHostPortKey(existing.Host, existing.Port)
			if current, ok := byKey[oldKey]; ok && current.ID == existing.ID {
				delete(byKey, oldKey)
			}
			if current, ok := byName[existing.Name]; ok && current.ID == existing.ID {
				delete(byName, existing.Name)
			}
		}
		// Ensure proxy name is unique among this sync pass + existing rows when
		// the same gateway name is reused for a different host:port (legacy bug).
		spec.Name = ensureUniqueWarpProxyName(spec.Name, key, byName, byKey)
		keepExternalIDs[spec.ExternalID] = struct{}{}
		var p *Proxy
		if exists {
			needUpdate := existing.Name != spec.Name || existing.Protocol != spec.Protocol ||
				existing.Host != spec.Host || existing.Port != spec.Port ||
				existing.Username != spec.Username || existing.Password != spec.Password ||
				existing.Status != spec.Status || existing.ManagedBy != warpProxyManagedBy ||
				existing.ExternalID != spec.ExternalID
			if needUpdate {
				existing.Name = spec.Name
				existing.Protocol = spec.Protocol
				existing.Host = spec.Host
				existing.Port = spec.Port
				existing.Username = spec.Username
				existing.Password = spec.Password
				existing.Status = spec.Status
				existing.ManagedBy = warpProxyManagedBy
				existing.ExternalID = spec.ExternalID
				if err := s.proxyRepo.Update(ctx, &existing); err != nil {
					return nil, fmt.Errorf("update proxy %s: %w", spec.Name, err)
				}
				result.UpdatedProxies = append(result.UpdatedProxies, existing)
			}
			p = &existing
			byName[spec.Name] = existing
		} else {
			// A matching endpoint/name is not ownership proof. Always create a
			// controller-owned row for a previously unseen gateway instance.
			created, err := s.proxyRepoCreate(ctx, spec)
			if err != nil {
				return nil, err
			}
			result.CreatedProxies = append(result.CreatedProxies, *created)
			p = created
		}
		byKey[key] = *p
		byName[p.Name] = *p
		byExternalID[spec.ExternalID] = *p
		delete(index.legacyByKey, key)

		// Healthy/running members stay in group; unhealthy optionally excluded.
		include := p.Status == StatusActive
		if !s.cfg.AutoDetachUnhealthy {
			include = true
		}
		if include {
			if _, dup := seenMember[p.ID]; !dup {
				seenMember[p.ID] = struct{}{}
				memberIDs = append(memberIDs, p.ID)
			}
		} else {
			result.DetachedIDs = append(result.DetachedIDs, p.ID)
		}
	}

	// Prune only rows carrying explicit WARP controller ownership.
	// Soft-delete leaves accounts.proxy_id dangling unless we unbind first;
	// also skip (with alert) when count fails rather than deleting blind.
	// Drastic-drop rounds skip prune while still upserting present specs.
	if allowOrphanPrune {
		for externalID, p := range byExternalID {
			if _, ok := keepExternalIDs[externalID]; ok {
				continue
			}
			if err := s.deleteOrphanWarpProxy(ctx, p, result); err != nil {
				s.log.Warn("delete orphan warp proxy failed", "name", p.Name, "id", p.ID, "err", err)
				continue
			}
		}
	}

	// Ensure group exists and set members.
	// W2: when drastic refuse (allowMemberReplace=false), skip SetMembers so
	// routing keeps existing warp-* group members (same safety as empty path).
	if s.groupSvc != nil {
		group, err := s.ensureGroup(ctx, groupName)
		if err != nil {
			return nil, err
		}
		if allowMemberReplace {
			// Preserve non-warp members that operators mixed into the managed group.
			// WARP sync owns only warp-* rows; full replace would silently drop others.
			memberIDs, err = s.mergeNonWarpGroupMembers(ctx, group.ID, memberIDs)
			if err != nil {
				return nil, err
			}
			withMembers, err := s.groupSvc.SetMembers(ctx, group.ID, memberIDs)
			if err != nil {
				return nil, fmt.Errorf("set group members: %w", err)
			}
			result.Group = withMembers
			// Soft consistency check: gateway healthy count vs group members.
			if snap != nil && snap.HealthyCount > 0 && len(memberIDs) < snap.HealthyCount {
				msg := fmt.Sprintf("warp sync member count %d < gateway healthy %d (possible index gap or detach)", len(memberIDs), snap.HealthyCount)
				result.Alerts = append(result.Alerts, msg)
				s.log.Warn(msg)
			}
		} else {
			// Keep existing membership; still surface group for callers.
			if with, gerr := s.groupSvc.GetByID(ctx, group.ID); gerr == nil {
				result.Group = with
			} else {
				result.Group = &ProxyGroupWithProxies{ProxyGroup: *group}
			}
		}
	}
	result.MemberIDs = memberIDs
	return result, nil
}

// mergeNonWarpGroupMembers keeps existing group members that are not warp-*
// managed proxies so SetMembers does not eject manually added endpoints.
func (s *WarpSyncService) mergeNonWarpGroupMembers(ctx context.Context, groupID int64, warpMemberIDs []int64) ([]int64, error) {
	if s.proxyRepo == nil || groupID <= 0 {
		return warpMemberIDs, nil
	}
	existing, err := s.proxyRepo.ListByGroupID(ctx, groupID)
	if err != nil {
		return nil, fmt.Errorf("list group members before warp set: %w", err)
	}
	out := make([]int64, 0, len(warpMemberIDs)+len(existing))
	seen := map[int64]struct{}{}
	for _, id := range warpMemberIDs {
		if id <= 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	for i := range existing {
		p := existing[i]
		if p.ManagedBy == warpProxyManagedBy {
			continue
		}
		if _, ok := seen[p.ID]; ok {
			continue
		}
		seen[p.ID] = struct{}{}
		out = append(out, p.ID)
	}
	return out, nil
}

// deleteOrphanWarpProxy unbinds accounts (and backup refs) then soft-deletes a
// warp-* proxy that disappeared from the gateway. In-use rows are unbound rather
// than skipped so sticky accounts do not keep a dead SOCKS endpoint forever.
func (s *WarpSyncService) deleteOrphanWarpProxy(ctx context.Context, p Proxy, result *WarpSyncResult) error {
	if s.proxyRepo == nil || result == nil {
		return nil
	}
	if atomicRepo, ok := s.proxyRepo.(interface {
		DeleteWithAccountBindings(context.Context, int64) (int64, error)
	}); ok {
		unbound, err := atomicRepo.DeleteWithAccountBindings(ctx, p.ID)
		if err != nil {
			return err
		}
		if unbound > 0 {
			msg := fmt.Sprintf("orphan warp proxy %s (id=%d): unbound %d account(s) before delete", p.Name, p.ID, unbound)
			result.Alerts = append(result.Alerts, msg)
			s.log.Warn(msg)
		}
		result.DeletedProxies = append(result.DeletedProxies, p)
		return nil
	}
	count, err := s.proxyRepo.CountAccountsByProxyID(ctx, p.ID)
	if err != nil {
		msg := fmt.Sprintf("orphan warp proxy %s (id=%d): count accounts failed: %v", p.Name, p.ID, err)
		result.Alerts = append(result.Alerts, msg)
		return err
	}
	if count > 0 {
		unbound, cerr := s.proxyRepo.ClearAccountProxyBindings(ctx, p.ID)
		if cerr != nil {
			msg := fmt.Sprintf("orphan warp proxy %s (id=%d): clear %d account binding(s) failed: %v", p.Name, p.ID, count, cerr)
			result.Alerts = append(result.Alerts, msg)
			return cerr
		}
		msg := fmt.Sprintf("orphan warp proxy %s (id=%d): unbound %d account(s) before delete", p.Name, p.ID, unbound)
		result.Alerts = append(result.Alerts, msg)
		s.log.Warn(msg)
	} else {
		// Still clear backup_proxy_id self-refs even when no accounts are bound.
		if _, cerr := s.proxyRepo.ClearAccountProxyBindings(ctx, p.ID); cerr != nil {
			s.log.Debug("clear orphan proxy backup refs failed", "id", p.ID, "err", cerr)
		}
	}
	if err := s.proxyRepo.Delete(ctx, p.ID); err != nil {
		return err
	}
	result.DeletedProxies = append(result.DeletedProxies, p)
	return nil
}

func (s *WarpSyncService) proxyRepoCreate(ctx context.Context, spec WarpProxySpec) (*Proxy, error) {
	p := &Proxy{
		Name:       spec.Name,
		Protocol:   spec.Protocol,
		Host:       spec.Host,
		Port:       spec.Port,
		Username:   spec.Username,
		Password:   spec.Password,
		Status:     spec.Status,
		ManagedBy:  warpProxyManagedBy,
		ExternalID: spec.ExternalID,
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
	// Also search non-active via paginated List (may exceed one page).
	for page := 1; page <= 50; page++ {
		groups, pageResult, err := s.groupSvc.List(ctx, pagination.PaginationParams{Page: page, PageSize: 200})
		if err != nil {
			break
		}
		for i := range groups {
			if groups[i].Name != name {
				continue
			}
			g := groups[i].ProxyGroup
			if g.Status != StatusActive {
				st := StatusActive
				if _, err := s.groupSvc.Update(ctx, g.ID, UpdateProxyGroupInput{Status: &st}); err != nil {
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
		if pageResult == nil || int64(page*200) >= pageResult.Total || len(groups) == 0 {
			break
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

type warpProxyIndex struct {
	byKey        map[string]Proxy
	byName       map[string]Proxy
	byExternalID map[string]Proxy
	legacyByKey  map[string]Proxy
	managedCount int
}

type warpManagedProxyLister interface {
	ListByManagedBy(context.Context, string) ([]Proxy, error)
}

func (s *WarpSyncService) indexProxies(ctx context.Context) (warpProxyIndex, error) {
	index := warpProxyIndex{
		byKey: make(map[string]Proxy), byName: make(map[string]Proxy),
		byExternalID: make(map[string]Proxy), legacyByKey: make(map[string]Proxy),
	}
	seenManaged := make(map[int64]struct{})
	add := func(p Proxy) {
		index.byKey[proxyHostPortKey(p.Host, p.Port)] = p
		index.byName[p.Name] = p
		if p.ManagedBy != warpProxyManagedBy {
			return
		}
		if _, seen := seenManaged[p.ID]; !seen {
			seenManaged[p.ID] = struct{}{}
			index.managedCount++
		}
		if p.ExternalID != "" {
			index.byExternalID[p.ExternalID] = p
			return
		}
		index.legacyByKey[proxyHostPortKey(p.Host, p.Port)] = p
	}

	// Production repositories expose an ownership query. One active scan remains
	// for display-name/endpoint collision avoidance; inactive owned rows come from
	// the targeted query instead of repeated full-table COUNT + pagination.
	if lister, ok := s.proxyRepo.(warpManagedProxyLister); ok {
		owned, err := lister.ListByManagedBy(ctx, warpProxyManagedBy)
		if err != nil {
			return warpProxyIndex{}, err
		}
		for _, p := range owned {
			add(p)
		}
		active, err := s.proxyRepo.ListActive(ctx)
		if err != nil {
			return warpProxyIndex{}, err
		}
		for _, p := range active {
			if p.ManagedBy == warpProxyManagedBy {
				continue
			}
			index.byKey[proxyHostPortKey(p.Host, p.Port)] = p
			index.byName[p.Name] = p
		}
		return index, nil
	}

	// Page through all rows so owned proxies remain discoverable even if an
	// operator renamed or disabled one between reconciliation rounds.
	const pageSize = 1000
	for page := 1; page <= 100; page++ {
		list, pageResult, listErr := s.proxyRepo.ListWithFilters(ctx, pagination.PaginationParams{
			Page: page, PageSize: pageSize, SortBy: "id", SortOrder: "asc",
		}, "", "", "")
		if listErr != nil {
			if page == 1 {
				// fallback ListActive only on first-page failure
				active, aerr := s.proxyRepo.ListActive(ctx)
				if aerr != nil {
					return warpProxyIndex{}, aerr
				}
				for _, p := range active {
					add(p)
				}
				return index, nil
			}
			return warpProxyIndex{}, listErr
		}
		for _, p := range list {
			add(p)
		}
		if len(list) < pageSize {
			break
		}
		if pageResult != nil && int64(page*pageSize) >= pageResult.Total {
			break
		}
	}
	return index, nil
}

func proxyHostPortKey(host string, port int) string {
	return strings.ToLower(strings.TrimSpace(host)) + ":" + fmt.Sprintf("%d", port)
}

// ensureUniqueWarpProxyName keeps proxy names unique when gateway reuses a display
// name for a different SOCKS endpoint. Prefer the planned name; on collision with
// a different host:port, append -<port> (and numeric suffix if still taken).
func ensureUniqueWarpProxyName(name, hostPortKey string, byName, byKey map[string]Proxy) string {
	name = strings.TrimSpace(name)
	if name == "" {
		name = "warp-unnamed"
	}
	if existing, ok := byName[name]; !ok {
		return name
	} else if proxyHostPortKey(existing.Host, existing.Port) == hostPortKey {
		return name
	}
	// Collision with a different endpoint.
	base := name
	// Prefer port disambiguation from hostPortKey ("host:port").
	port := ""
	if i := strings.LastIndex(hostPortKey, ":"); i >= 0 && i+1 < len(hostPortKey) {
		port = hostPortKey[i+1:]
	}
	if port != "" {
		cand := fmt.Sprintf("%s-%s", base, port)
		if existing, ok := byName[cand]; !ok || proxyHostPortKey(existing.Host, existing.Port) == hostPortKey {
			return cand
		}
		base = cand
	}
	for i := 2; i < 10000; i++ {
		cand := fmt.Sprintf("%s-%d", base, i)
		if existing, ok := byName[cand]; !ok || proxyHostPortKey(existing.Host, existing.Port) == hostPortKey {
			return cand
		}
	}
	return fmt.Sprintf("%s-%d", base, time.Now().UnixNano()%100000)
}
