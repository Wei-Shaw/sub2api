package service

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
)

// ── TransactionRunner ─────────────────────────────────────────────────────────

// TransactionRunner executes a function inside an Ent database transaction.
// The provided ctx carries the Ent Tx so that repositories using txAwareSQLExecutor
// automatically participate in the same transaction.
type TransactionRunner interface {
	RunInTx(ctx context.Context, fn func(ctx context.Context) error) error
}

// ── UserPoolService ───────────────────────────────────────────────────────────

// UserPoolService implements Pool domain operations including cache invalidation
// via the CacheInvalidationOutbox pattern.
type UserPoolService struct {
	repo       UserPoolRepository
	userRepo   UserRepository
	outboxRepo CacheInvalidationOutboxRepository
	txRunner   TransactionRunner
}

// NewUserPoolService constructs a UserPoolService.
func NewUserPoolService(
	repo UserPoolRepository,
	userRepo UserRepository,
	outboxRepo CacheInvalidationOutboxRepository,
	txRunner TransactionRunner,
) *UserPoolService {
	return &UserPoolService{
		repo:       repo,
		userRepo:   userRepo,
		outboxRepo: outboxRepo,
		txRunner:   txRunner,
	}
}

// ── CRUD ──────────────────────────────────────────────────────────────────────

// Create creates a new Pool.  name must be non-empty.
func (s *UserPoolService) Create(ctx context.Context, name, description, status string) (Pool, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return Pool{}, ErrPoolNameConflict // name 必须非空，用 conflict 表示唯一性约束
	}
	if status == "" {
		status = "active"
	}
	return s.repo.Create(ctx, Pool{Name: name, Description: description, Status: status})
}

// List returns a paginated list of Pools.
func (s *UserPoolService) List(ctx context.Context, opts ListPoolsOptions) ([]Pool, int, error) {
	return s.repo.List(ctx, opts)
}

// GetByID returns a Pool by ID.
func (s *UserPoolService) GetByID(ctx context.Context, id int64) (Pool, error) {
	return s.repo.GetByID(ctx, id)
}

// Update updates Pool name/description/status.
// status changes (active → disabled or disabled → active) trigger cache invalidation.
func (s *UserPoolService) Update(ctx context.Context, id int64, name, description, status string) (Pool, error) {
	// Fetch current state for diff.
	current, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return Pool{}, err
	}

	name = strings.TrimSpace(name)
	if name == "" {
		name = current.Name
	}
	if status == "" {
		status = current.Status
	}

	var updated Pool
	txErr := s.txRunner.RunInTx(ctx, func(txCtx context.Context) error {
		var uErr error
		updated, uErr = s.repo.Update(txCtx, id, Pool{Name: name, Description: description, Status: status})
		if uErr != nil {
			return uErr
		}
		// On active → disabled: enqueue strict cache invalidation for all pool members.
		if current.Status == "active" && updated.Status == "disabled" {
			return s.enqueuePoolDisabledInvalidation(txCtx, id)
		}
		return nil
	})
	if txErr != nil {
		return Pool{}, txErr
	}
	return updated, nil
}

// Delete soft-deletes a Pool.
// The invalidation enqueue and the soft-delete run in the same transaction so that
// if the outbox write fails the soft-delete is rolled back.
func (s *UserPoolService) Delete(ctx context.Context, id int64) error {
	return s.txRunner.RunInTx(ctx, func(txCtx context.Context) error {
		// Enqueue cache invalidation before soft-delete so we can still read members/grants
		// inside the same transaction (reads use txCtx).
		if err := s.enqueuePoolDisabledInvalidation(txCtx, id); err != nil {
			return err
		}
		return s.repo.SoftDelete(txCtx, id)
	})
}

// ── AddMembers ────────────────────────────────────────────────────────────────

// AddMembers adds users to a Pool and enqueues cache invalidation as needed.
func (s *UserPoolService) AddMembers(ctx context.Context, poolID int64, userIDs []int64) (added []int64, skipped []int64, err error) {
	// Capture pre-state (pool grants) for diff.
	grants, err := s.repo.ListGroupGrants(ctx, poolID)
	if err != nil {
		return nil, nil, err
	}

	added, skipped, err = s.repo.AddMembers(ctx, poolID, userIDs)
	if err != nil {
		return nil, nil, err
	}

	// Enqueue best-effort auth invalidation for newly added users.
	if len(added) > 0 && len(grants) > 0 {
		for _, uid := range added {
			s.enqueueAddMemberInvalidation(ctx, poolID, uid, grants)
		}
	}

	return added, skipped, nil
}

// ── RemoveMembers ─────────────────────────────────────────────────────────────

// RemoveMembers removes users from a Pool and enqueues strict cache invalidation.
// The removal and outbox write run inside the same transaction: if the enqueue fails
// the DELETE is rolled back.
func (s *UserPoolService) RemoveMembers(ctx context.Context, poolID int64, userIDs []int64) (removed []int64, err error) {
	// Capture grants before entering the transaction (read-only, no tx needed).
	grants, err := s.repo.ListGroupGrants(ctx, poolID)
	if err != nil {
		return nil, err
	}

	txErr := s.txRunner.RunInTx(ctx, func(txCtx context.Context) error {
		var rErr error
		removed, rErr = s.repo.RemoveMembers(txCtx, poolID, userIDs)
		if rErr != nil {
			return rErr
		}
		if len(removed) > 0 && len(grants) > 0 {
			return s.enqueueRemoveMemberInvalidation(txCtx, poolID, removed, grants)
		}
		return nil
	})
	if txErr != nil {
		return nil, txErr
	}
	return removed, nil
}

// ── ListMembers ───────────────────────────────────────────────────────────────

// ListMembers returns a paginated list of Pool members.
func (s *UserPoolService) ListMembers(ctx context.Context, poolID int64, opts ListMembersOptions) ([]PoolMember, int, error) {
	return s.repo.ListMembers(ctx, poolID, opts)
}

// ── ReplaceGroupGrants ────────────────────────────────────────────────────────

// ReplaceGroupGrants atomically replaces all group grants for a Pool.
// Validates grant parameters before writing to DB.
func (s *UserPoolService) ReplaceGroupGrants(ctx context.Context, poolID int64, grants []PoolGroupGrant) error {
	// Service-layer validation (mirrors repo validation to surface 422 before DB hit).
	seen := make(map[int64]struct{})
	for _, g := range grants {
		if g.GroupID <= 0 {
			return ErrGroupGrantNotFound
		}
		if _, dup := seen[g.GroupID]; dup {
			return ErrDuplicateGrantGroup
		}
		seen[g.GroupID] = struct{}{}
		if g.RateMultiplier != nil && *g.RateMultiplier <= 0 {
			return ErrPoolGrantRateInvalid
		}
		if g.RPMOverride != nil && *g.RPMOverride < 0 {
			return ErrPoolGrantRPMInvalid
		}
	}

	// Capture pre-state for diff.
	oldGrants, err := s.repo.ListGroupGrants(ctx, poolID)
	if err != nil {
		return err
	}

	// Get all affected member IDs for cache invalidation scope (read-only, before tx).
	// Uses ListAllMemberIDs to bypass the normPage 200-row cap.
	allUserIDs, err := s.repo.ListAllMemberIDs(ctx, poolID)
	if err != nil {
		return err
	}

	return s.txRunner.RunInTx(ctx, func(txCtx context.Context) error {
		if err := s.repo.ReplaceGroupGrants(txCtx, poolID, grants); err != nil {
			return err
		}
		// Compute diff and enqueue outbox for revoked/tightened permissions.
		return s.enqueueGrantReplacementInvalidation(txCtx, poolID, allUserIDs, oldGrants, grants)
	})
}

// ── DeleteGroupGrant ──────────────────────────────────────────────────────────

// DeleteGroupGrant removes a specific group grant from a Pool and invalidates caches.
func (s *UserPoolService) DeleteGroupGrant(ctx context.Context, poolID, groupID int64) error {
	if groupID <= 0 {
		return ErrGroupGrantNotFound
	}

	// Capture all member IDs before entering transaction (read-only).
	// Uses ListAllMemberIDs to bypass the normPage 200-row cap.
	allUserIDs, err := s.repo.ListAllMemberIDs(ctx, poolID)
	if err != nil {
		return err
	}

	return s.txRunner.RunInTx(ctx, func(txCtx context.Context) error {
		if err := s.repo.DeleteGroupGrant(txCtx, poolID, groupID); err != nil {
			return err
		}
		if len(allUserIDs) > 0 {
			return s.enqueueStrictAuthInvalidationBatched(txCtx, poolID, groupID, allUserIDs, ReasonPoolGrantRemoved)
		}
		return nil
	})
}

// ── ListGroupGrants ───────────────────────────────────────────────────────────

// ListGroupGrants returns active standard group grants for a Pool.
func (s *UserPoolService) ListGroupGrants(ctx context.Context, poolID int64) ([]PoolGroupGrant, error) {
	return s.repo.ListGroupGrants(ctx, poolID)
}

// ── GetUserPools ──────────────────────────────────────────────────────────────

// GetUserPools returns all Pools a user belongs to.
func (s *UserPoolService) GetUserPools(ctx context.Context, userID int64) ([]Pool, error) {
	return s.repo.GetUserPools(ctx, userID)
}

// ── Cache invalidation helpers ────────────────────────────────────────────────

// enqueuePoolDisabledInvalidation enqueues strict auth cache invalidation for all
// members of a pool when the pool is disabled or deleted.
// Returns an error so callers inside RunInTx can trigger rollback.
func (s *UserPoolService) enqueuePoolDisabledInvalidation(ctx context.Context, poolID int64) error {
	// Use ListAllMemberIDs to bypass the normPage 200-row cap.
	userIDs, err := s.repo.ListAllMemberIDs(ctx, poolID)
	if err != nil {
		return fmt.Errorf("user_pool: enqueuePoolDisabledInvalidation: list members: %w", err)
	}
	grants, err := s.repo.ListGroupGrants(ctx, poolID)
	if err != nil {
		return fmt.Errorf("user_pool: enqueuePoolDisabledInvalidation: list grants: %w", err)
	}

	if len(userIDs) == 0 || len(grants) == 0 {
		return nil
	}

	for _, g := range grants {
		if err := s.enqueueStrictAuthInvalidationBatched(ctx, poolID, g.GroupID, userIDs, ReasonPoolDisabled); err != nil {
			return err
		}
	}
	return nil
}

// enqueueAddMemberInvalidation enqueues best-effort auth invalidation for a newly
// added pool member.  Uses minimum pool_id logic to determine effective grant scope.
// Errors are logged and swallowed: Add is a relaxing operation so eventual consistency is fine.
func (s *UserPoolService) enqueueAddMemberInvalidation(ctx context.Context, poolID, userID int64, grants []PoolGroupGrant) {
	existingPools, err := s.repo.GetUserPools(ctx, userID)
	if err != nil {
		slog.Warn("user_pool: enqueueAddMemberInvalidation: get_user_pools failed",
			"pool_id", poolID, "user_id", userID, "error", err)
		return
	}

	if len(existingPools) == 0 {
		// User had no pools before; all grants are new.
		for _, g := range grants {
			if err := s.enqueueBestEffortAuthInvalidation(ctx, poolID, g.GroupID, []int64{userID}); err != nil {
				slog.Warn("user_pool: enqueueAddMemberInvalidation: enqueue failed",
					"pool_id", poolID, "user_id", userID, "group_id", g.GroupID, "error", err)
			}
		}
		return
	}

	// Build union of all existing pool group IDs.
	existingGroupIDs := make(map[int64]bool)
	minExistingPoolID := int64(-1)
	for _, p := range existingPools {
		if p.ID == poolID {
			continue // skip current pool (just added)
		}
		if minExistingPoolID < 0 || p.ID < minExistingPoolID {
			minExistingPoolID = p.ID
		}
		eg, _ := s.repo.ListGroupGrants(ctx, p.ID)
		for _, g := range eg {
			existingGroupIDs[g.GroupID] = true
		}
	}

	for _, g := range grants {
		if existingGroupIDs[g.GroupID] {
			// Group covered by existing pool. If current pool has lower ID it may affect effective rate.
			if minExistingPoolID > 0 && poolID < minExistingPoolID {
				if err := s.enqueueBestEffortAuthInvalidation(ctx, poolID, g.GroupID, []int64{userID}); err != nil {
					slog.Warn("user_pool: enqueueAddMemberInvalidation: enqueue failed",
						"pool_id", poolID, "user_id", userID, "group_id", g.GroupID, "error", err)
				}
			}
		} else {
			// New group grant for this user.
			if err := s.enqueueBestEffortAuthInvalidation(ctx, poolID, g.GroupID, []int64{userID}); err != nil {
				slog.Warn("user_pool: enqueueAddMemberInvalidation: enqueue failed",
					"pool_id", poolID, "user_id", userID, "group_id", g.GroupID, "error", err)
			}
		}
	}
}

// enqueueRemoveMemberInvalidation enqueues strict auth invalidation for removed members.
// Returns the first error encountered so callers inside RunInTx can trigger rollback.
func (s *UserPoolService) enqueueRemoveMemberInvalidation(ctx context.Context, poolID int64, removedUserIDs []int64, grants []PoolGroupGrant) error {
	for _, g := range grants {
		if err := s.enqueueStrictAuthInvalidation(ctx, poolID, g.GroupID, removedUserIDs, ReasonPoolMemberRemoved); err != nil {
			return err
		}
	}
	return nil
}

// enqueueGrantReplacementInvalidation computes the diff between old and new grants
// and enqueues appropriate outbox events.
// userIDs is the full list of pool members (already fetched via ListAllMemberIDs).
// Returns the first error so callers inside RunInTx can trigger rollback.
func (s *UserPoolService) enqueueGrantReplacementInvalidation(
	ctx context.Context,
	poolID int64,
	userIDs []int64,
	oldGrants, newGrants []PoolGroupGrant,
) error {
	if len(userIDs) == 0 {
		return nil
	}

	oldMap := make(map[int64]PoolGroupGrant)
	for _, g := range oldGrants {
		oldMap[g.GroupID] = g
	}
	newMap := make(map[int64]PoolGroupGrant)
	for _, g := range newGrants {
		newMap[g.GroupID] = g
	}

	// Removed grants → permission revoked → strict invalidation (batched).
	for gid := range oldMap {
		if _, exists := newMap[gid]; !exists {
			if err := s.enqueueStrictAuthInvalidationBatched(ctx, poolID, gid, userIDs, ReasonPoolGrantRemoved); err != nil {
				return err
			}
		}
	}

	// Changed grants → check for tightening.
	for gid, ng := range newMap {
		og, existed := oldMap[gid]
		if !existed {
			// New grant → best-effort.
			if err := s.enqueueBestEffortAuthInvalidation(ctx, poolID, gid, userIDs); err != nil {
				return err
			}
			continue
		}
		// Check RPM tightening.
		if rpmTightened(og.RPMOverride, ng.RPMOverride) {
			if err := s.enqueueStrictAuthInvalidationBatched(ctx, poolID, gid, userIDs, ReasonRPMTightened); err != nil {
				return err
			}
			continue
		}
		// Check rate change.
		if rateChanged(og.RateMultiplier, ng.RateMultiplier) {
			if err := s.enqueueStrictAuthInvalidationBatched(ctx, poolID, gid, userIDs, ReasonRateChanged); err != nil {
				return err
			}
		}
	}
	return nil
}

// enqueueStrictAuthInvalidationBatched splits userIDs into batches of ≤1000 and
// calls enqueueStrictAuthInvalidation for each batch.  This ensures outbox events
// stay within a manageable size regardless of pool cardinality.
// Returns the first error encountered so callers inside RunInTx can trigger rollback.
func (s *UserPoolService) enqueueStrictAuthInvalidationBatched(ctx context.Context, poolID, groupID int64, userIDs []int64, reason string) error {
	const batchSize = 1000
	for i := 0; i < len(userIDs); i += batchSize {
		end := i + batchSize
		if end > len(userIDs) {
			end = len(userIDs)
		}
		if err := s.enqueueStrictAuthInvalidation(ctx, poolID, groupID, userIDs[i:end], reason); err != nil {
			return err
		}
	}
	return nil
}

// enqueueStrictAuthInvalidation writes a strict auth cache invalidation event to the outbox.
// When reason is ReasonRateChanged, it also populates CacheTypeUserGroupRate and RatePairs
// so the worker invalidates the user_group_rate cache layer as well.
// Returns an error so callers inside RunInTx can propagate it and trigger rollback.
func (s *UserPoolService) enqueueStrictAuthInvalidation(ctx context.Context, poolID, groupID int64, userIDs []int64, reason string) error {
	if len(userIDs) == 0 {
		return nil
	}

	cacheTypes := []string{CacheTypeAuthSnapshot}
	var ratePairs []RatePair

	// P0-6: rate_multiplier changes must also invalidate user_group_rate cache.
	if reason == ReasonRateChanged {
		cacheTypes = append(cacheTypes, CacheTypeUserGroupRate)
		ratePairs = make([]RatePair, len(userIDs))
		for i, uid := range userIDs {
			ratePairs[i] = RatePair{UserID: uid, GroupID: groupID}
		}
	}

	poolIDAgg := poolID
	event := CacheInvalidationEvent{
		EventType:     EventTypeAuthCacheInvalidate,
		AggregateType: "user_pool",
		AggregateID:   &poolIDAgg,
		Reason:        reason,
		CacheTypes:    cacheTypes,
		MaxAttempts:   12,
		Payload: EventPayload{
			SchemaVersion:   EventPayloadSchemaVersion,
			AffectedUserIDs: userIDs,
			RatePairs:       ratePairs,
		},
		IdempotencyKey: buildIdempotencyKeyForPoolEvent(poolID, groupID, reason, userIDs, time.Now()),
	}

	return s.outboxRepo.Enqueue(ctx, event)
}

// enqueueBestEffortAuthInvalidation writes a best-effort auth cache invalidation event.
// Used for grant additions (no permission revocation) where eventual consistency is acceptable.
// Returns an error so callers can decide whether to propagate or swallow it.
func (s *UserPoolService) enqueueBestEffortAuthInvalidation(ctx context.Context, poolID, groupID int64, userIDs []int64) error {
	if len(userIDs) == 0 {
		return nil
	}

	poolIDAgg := poolID
	event := CacheInvalidationEvent{
		EventType:     EventTypeAuthCacheInvalidate,
		AggregateType: "user_pool",
		AggregateID:   &poolIDAgg,
		Reason:        ReasonPoolGrantReplaced,
		CacheTypes:    []string{CacheTypeAuthSnapshot},
		MaxAttempts:   3, // best-effort: fewer retries
		Payload: EventPayload{
			SchemaVersion:   EventPayloadSchemaVersion,
			AffectedUserIDs: userIDs,
		},
	}

	return s.outboxRepo.Enqueue(ctx, event)
}

// rpmTightened returns true if the RPM override became more restrictive.
func rpmTightened(old, new *int) bool {
	if old == nil && new == nil {
		return false
	}
	if old == nil {
		// Was unlimited (nil), now restricted.
		return *new > 0
	}
	if new == nil {
		// Was restricted, now unlimited.
		return false
	}
	// Both non-nil: tightened if new is smaller and non-zero.
	return *new > 0 && *new < *old
}

// rateChanged returns true if the rate multiplier changed (for diff purposes).
func rateChanged(old, new *float64) bool {
	if old == nil && new == nil {
		return false
	}
	if old == nil || new == nil {
		return true
	}
	return *old != *new
}

// buildIdempotencyKeyForPoolEvent builds a deterministic idempotency key for pool outbox events.
// The key includes a sha256 digest (first 16 hex chars) of the sorted user IDs so that
// two events with the same (pool, group, reason, second) but different affected users
// are never silently merged by the ON CONFLICT DO NOTHING constraint.
func buildIdempotencyKeyForPoolEvent(poolID, groupID int64, reason string, userIDs []int64, ts time.Time) string {
	// Second-level granularity — avoids the minute-level coalescing bug.
	sec := ts.UTC().Unix()

	// Stable digest: sort user IDs, then SHA-256 over big-endian encoding.
	sorted := append([]int64(nil), userIDs...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
	h := sha256.New()
	for _, id := range sorted {
		var buf [8]byte
		binary.BigEndian.PutUint64(buf[:], uint64(id))
		_, _ = h.Write(buf[:])
	}
	digest := hex.EncodeToString(h.Sum(nil))[:16]

	return fmt.Sprintf("pool:%d:group:%d:%s:%d:%s", poolID, groupID, reason, sec, digest)
}

// ── AddMembersByFilter ────────────────────────────────────────────────────────

const (
	addByFilterMaxMatched = 100_000
	addByFilterBatchSize  = 1000
	addByFilterPageSize   = 1000
)

// AddMembersByFilter resolves filter to user IDs, then adds them to the pool.
// Returns added/skipped/matched counts and any error.
// Invalidation is computed in-memory to avoid N+1 DB queries.
func (s *UserPoolService) AddMembersByFilter(
	ctx context.Context,
	poolID int64,
	filters UserListFilters,
) (added int, skipped int, matched int, err error) {
	// Disable subscription loading — not needed and expensive for large datasets.
	falseBool := false
	filters.IncludeSubscriptions = &falseBool

	// Step 1: fetch current pool grants (1 query total).
	poolGrants, err := s.repo.ListGroupGrants(ctx, poolID)
	if err != nil {
		return 0, 0, 0, err
	}

	// Step 2: paginate users, fast-fail on Total > cap before iterating.
	var allIDs []int64
	page := 1
	for {
		params := pagination.PaginationParams{Page: page, PageSize: addByFilterPageSize}
		users, result, qErr := s.userRepo.ListWithFilters(ctx, params, filters)
		if qErr != nil {
			return 0, 0, 0, qErr
		}
		// Quick-fail on first page using Total.
		if page == 1 && result.Total > addByFilterMaxMatched {
			return 0, 0, int(result.Total), ErrPoolMatchedTooManyUsers
		}
		for _, u := range users {
			allIDs = append(allIDs, u.ID)
		}
		// Safety net in case Total was inaccurate.
		if len(allIDs) > addByFilterMaxMatched {
			return 0, 0, 0, ErrPoolMatchedTooManyUsers
		}
		if len(allIDs) >= int(result.Total) || len(users) < addByFilterPageSize {
			break
		}
		page++
	}

	matched = len(allIDs)
	if matched == 0 || len(poolGrants) == 0 {
		// Nothing to add or no grants to invalidate — still insert members.
		for i := 0; i < len(allIDs); i += addByFilterBatchSize {
			end := i + addByFilterBatchSize
			if end > len(allIDs) {
				end = len(allIDs)
			}
			batchAdded, batchSkipped, bErr := s.repo.AddMembers(ctx, poolID, allIDs[i:end])
			if bErr != nil {
				return added, skipped, matched, bErr
			}
			added += len(batchAdded)
			skipped += len(batchSkipped)
		}
		return added, skipped, matched, nil
	}

	// Step 3: batch insert and collect truly-added user IDs.
	var totalAdded []int64
	for i := 0; i < len(allIDs); i += addByFilterBatchSize {
		end := i + addByFilterBatchSize
		if end > len(allIDs) {
			end = len(allIDs)
		}
		batchAdded, batchSkipped, bErr := s.repo.AddMembers(ctx, poolID, allIDs[i:end])
		if bErr != nil {
			return added, skipped, matched, bErr
		}
		added += len(batchAdded)
		skipped += len(batchSkipped)
		totalAdded = append(totalAdded, batchAdded...)
	}

	if len(totalAdded) == 0 {
		return added, skipped, matched, nil
	}

	// Step 4: batch-fetch existing pool memberships for all added users (1 query).
	userPoolsMap, err := s.repo.GetUserPoolsBatch(ctx, totalAdded)
	if err != nil {
		slog.Warn("user_pool: AddMembersByFilter: GetUserPoolsBatch failed", "pool_id", poolID, "error", err)
		return added, skipped, matched, nil
	}

	// Step 5: collect all distinct existing pool IDs (excluding current pool).
	existingPoolIDSet := make(map[int64]struct{})
	for _, pools := range userPoolsMap {
		for _, p := range pools {
			if p.ID != poolID {
				existingPoolIDSet[p.ID] = struct{}{}
			}
		}
	}

	// Step 6: batch-fetch grants for all existing pools (1 query).
	existingPoolIDs := make([]int64, 0, len(existingPoolIDSet))
	for pid := range existingPoolIDSet {
		existingPoolIDs = append(existingPoolIDs, pid)
	}
	existingGrantsMap, err := s.repo.ListGroupGrantsBatch(ctx, existingPoolIDs)
	if err != nil {
		slog.Warn("user_pool: AddMembersByFilter: ListGroupGrantsBatch failed", "pool_id", poolID, "error", err)
		return added, skipped, matched, nil
	}

	// Step 7: in-memory invalidation decision — 1:1 replica of enqueueAddMemberInvalidation logic,
	// aggregated per (groupID) to batch users into fewer outbox rows.
	// enqueue[groupID] -> []userID
	enqueueMap := make(map[int64][]int64)

	for _, uid := range totalAdded {
		existingPools := userPoolsMap[uid] // nil if user had no pools before

		if len(existingPools) == 0 {
			// User had no pools before; all grants are new.
			for _, g := range poolGrants {
				enqueueMap[g.GroupID] = append(enqueueMap[g.GroupID], uid)
			}
			continue
		}

		// Build existing group coverage and minimum existing pool ID for this user.
		existingGroupIDs := make(map[int64]bool)
		minExistingPoolID := int64(-1)
		for _, p := range existingPools {
			if p.ID == poolID {
				continue // skip current pool (just added)
			}
			if minExistingPoolID < 0 || p.ID < minExistingPoolID {
				minExistingPoolID = p.ID
			}
			for _, eg := range existingGrantsMap[p.ID] {
				existingGroupIDs[eg.GroupID] = true
			}
		}

		for _, g := range poolGrants {
			if existingGroupIDs[g.GroupID] {
				// Group already covered. Enqueue only if current pool would become effective (lower ID).
				if minExistingPoolID > 0 && poolID < minExistingPoolID {
					enqueueMap[g.GroupID] = append(enqueueMap[g.GroupID], uid)
				}
			} else {
				// New group grant for this user.
				enqueueMap[g.GroupID] = append(enqueueMap[g.GroupID], uid)
			}
		}
	}

	// Step 8: emit outbox events — one call per (poolID, groupID) covering all affected users.
	// Best-effort: log and continue on enqueue failure.
	for groupID, userIDs := range enqueueMap {
		if err := s.enqueueBestEffortAuthInvalidation(ctx, poolID, groupID, userIDs); err != nil {
			slog.Warn("user_pool: AddMembersByFilter: enqueue failed",
				"pool_id", poolID, "group_id", groupID, "error", err)
		}
	}

	return added, skipped, matched, nil
}
