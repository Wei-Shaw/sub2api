package service

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"strconv"
	"sync"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/google/uuid"
	"golang.org/x/sync/singleflight"
)

type groupQuotaUsageQuerier interface {
	QueryUsageWindows(context.Context, int64) (*OpenAIQuotaUsage, error)
}

type groupQuotaCacheEntry struct {
	revision, runtimeVersion, generation int64
	mappings                             map[string]GroupQuotaPolicyResolution
}

// GroupQuotaPolicyService keeps the gateway path entirely in memory. The
// background sampler reads quota only; it never resets or consumes credits.
type GroupQuotaPolicyService struct {
	repo        GroupQuotaPolicyRepository
	groups      GroupRepository
	accounts    AccountRepository
	quota       groupQuotaUsageQuerier
	leader      LeaderLockCache
	owner       string
	ctx         context.Context
	cancel      context.CancelFunc
	start, stop sync.Once
	wg          sync.WaitGroup
	mu          sync.RWMutex
	cache       map[int64]groupQuotaCacheEntry
	generation  int64
	sampleMu    sync.Mutex
	samples     map[int64]GroupQuotaPolicySnapshot
	sampling    singleflight.Group
	diagnostics *groupQuotaPolicyDiagnostics
}

func NewGroupQuotaPolicyService(repo GroupQuotaPolicyRepository, groups GroupRepository, accounts AccountRepository, quota *OpenAIQuotaService, leader LeaderLockCache) *GroupQuotaPolicyService {
	ctx, cancel := context.WithCancel(context.Background())
	var querier groupQuotaUsageQuerier
	if quota != nil {
		querier = quota
	}
	return &GroupQuotaPolicyService{repo: repo, groups: groups, accounts: accounts, quota: querier, leader: leader,
		owner: uuid.NewString(), ctx: ctx, cancel: cancel, cache: make(map[int64]groupQuotaCacheEntry), samples: make(map[int64]GroupQuotaPolicySnapshot)}
}

func (s *GroupQuotaPolicyService) eligibleAccounts(ctx context.Context, groupID int64) ([]GroupQuotaPolicyAccount, map[int64]bool, error) {
	group, err := s.groups.GetByIDLite(ctx, groupID)
	if err != nil {
		return nil, nil, err
	}
	if group == nil {
		return nil, nil, ErrGroupNotFound
	}
	if group.Platform != PlatformOpenAI {
		return nil, nil, infraerrors.BadRequest("GROUP_QUOTA_POLICY_PLATFORM", "quota policies currently support OpenAI groups only")
	}
	accounts, err := s.accounts.ListByGroup(ctx, groupID)
	if err != nil {
		return nil, nil, err
	}
	options, eligible := []GroupQuotaPolicyAccount{}, make(map[int64]bool)
	for i := range accounts {
		account := &accounts[i]
		if groupQuotaEligible(account) {
			eligible[account.ID] = true
			options = append(options, GroupQuotaPolicyAccount{ID: account.ID, Name: account.Name, Status: account.Status})
		}
	}
	return options, eligible, nil
}

func groupQuotaView(record *GroupQuotaPolicyRecord, accounts []GroupQuotaPolicyAccount) *GroupQuotaPolicyView {
	// Reads may show that data has become stale, but never invent an unpersisted
	// transition or event (in particular immediately after manual release).
	runtime := record.Runtime
	runtime.Rules = append([]GroupQuotaPolicyRuleState{}, runtime.Rules...)
	evaluated := evaluateGroupQuotaPolicy(record.Config, record.Runtime, time.Now())
	for i := range runtime.Rules {
		if i < len(evaluated.Rules) && evaluated.Rules[i].Status == "unknown" {
			runtime.Rules[i].Status = "unknown"
		}
	}
	return &GroupQuotaPolicyView{GroupID: record.GroupID, Revision: record.Revision, Config: record.Config,
		Runtime: runtime, Accounts: accounts}
}

func (s *GroupQuotaPolicyService) Get(ctx context.Context, groupID int64) (*GroupQuotaPolicyView, error) {
	options, _, err := s.eligibleAccounts(ctx, groupID)
	if err != nil {
		return nil, err
	}
	record, err := s.repo.Get(ctx, groupID)
	if err != nil {
		return nil, err
	}
	s.publish(record)
	return groupQuotaView(record, options), nil
}

func (s *GroupQuotaPolicyService) Save(ctx context.Context, groupID, revision int64, config GroupQuotaPolicyConfig) (*GroupQuotaPolicyView, error) {
	options, eligible, err := s.eligibleAccounts(ctx, groupID)
	if err != nil {
		return nil, err
	}
	if err := validateGroupQuotaPolicy(config, eligible); err != nil {
		return nil, err
	}
	if config.Rules == nil {
		config.Rules = []GroupQuotaPolicyRule{}
	}
	for attempt := 0; attempt < 3; attempt++ {
		record, err := s.repo.Get(ctx, groupID)
		if err != nil {
			return nil, err
		}
		if record.Revision != revision {
			return nil, ErrGroupQuotaPolicyConflict
		}
		record.Runtime = groupQuotaRuntimeForConfig(record, config)
		record.Config = config
		record.Runtime = evaluateGroupQuotaPolicy(config, record.Runtime, time.Now())
		groupQuotaAppendEvent(&record.Runtime, time.Now().Unix(), "", "config_saved", "Quota policy configuration saved")
		ok, err := s.repo.SaveConfig(ctx, record, record.Revision, record.RuntimeVersion)
		if err != nil {
			return nil, err
		}
		if !ok {
			continue
		}
		record.Revision++
		record.RuntimeVersion++
		s.publish(record)
		return groupQuotaView(record, options), nil
	}
	return nil, ErrGroupQuotaPolicyConflict
}

func (s *GroupQuotaPolicyService) Preview(ctx context.Context, groupID int64, config GroupQuotaPolicyConfig, model string) (*GroupQuotaPolicyPreview, error) {
	_, eligible, err := s.eligibleAccounts(ctx, groupID)
	if err != nil {
		return nil, err
	}
	if err := validateGroupQuotaPolicy(config, eligible); err != nil {
		return nil, err
	}
	record, err := s.repo.Get(ctx, groupID)
	if err != nil {
		return nil, err
	}
	runtime := evaluateGroupQuotaPolicy(config, groupQuotaRuntimeForConfig(record, config), time.Now())
	return &GroupQuotaPolicyPreview{Runtime: runtime, Resolution: resolveGroupQuotaPolicy(config, runtime, record.Revision, model)}, nil
}

func (s *GroupQuotaPolicyService) Release(ctx context.Context, groupID, revision int64, ruleID string) (*GroupQuotaPolicyView, error) {
	options, _, err := s.eligibleAccounts(ctx, groupID)
	if err != nil {
		return nil, err
	}
	for attempt := 0; attempt < 3; attempt++ {
		record, err := s.repo.Get(ctx, groupID)
		if err != nil {
			return nil, err
		}
		if record.Revision != revision {
			return nil, ErrGroupQuotaPolicyConflict
		}
		found := false
		for _, rule := range record.Config.Rules {
			if rule.ID == ruleID {
				found = true
				break
			}
		}
		if !found {
			return nil, infraerrors.NotFound("GROUP_QUOTA_POLICY_RULE_NOT_FOUND", "quota rule not found")
		}
		record.Runtime = evaluateGroupQuotaPolicy(record.Config, record.Runtime, time.Now())
		for i := range record.Runtime.Rules {
			state := &record.Runtime.Rules[i]
			if state.RuleID == ruleID {
				if !state.Active {
					return nil, infraerrors.Conflict("GROUP_QUOTA_POLICY_NOT_ACTIVE", "quota rule is not active")
				}
				state.Active, state.Suppressed, state.Status, state.ChangedAt = false, true, "suppressed", time.Now().Unix()
			}
		}
		groupQuotaAppendEvent(&record.Runtime, time.Now().Unix(), ruleID, "released", "Manually released; rearming requires quota recovery")
		ok, err := s.repo.SaveRuntime(ctx, record, record.Revision, record.RuntimeVersion)
		if err != nil {
			return nil, err
		}
		if !ok {
			continue
		}
		record.RuntimeVersion++
		s.publish(record)
		return groupQuotaView(record, options), nil
	}
	return nil, ErrGroupQuotaPolicyConflict
}

func (s *GroupQuotaPolicyService) Refresh(ctx context.Context, groupID int64) (*GroupQuotaPolicyView, error) {
	options, eligible, err := s.eligibleAccounts(ctx, groupID)
	if err != nil {
		return nil, err
	}
	record, err := s.repo.Get(ctx, groupID)
	if err != nil {
		return nil, err
	}
	if record.Revision == 0 {
		return groupQuotaView(record, options), nil
	}
	ctx, cancel := context.WithTimeout(ctx, 50*time.Second)
	defer cancel()
	ids := groupQuotaAccountIDs([]*GroupQuotaPolicyRecord{record})
	queryCtx, queryCancel := context.WithTimeout(ctx, 40*time.Second)
	samples := s.collect(queryCtx, ids, eligible)
	queryCancel()
	if err := s.persistSamples(ctx, groupID, samples); err != nil {
		return nil, err
	}
	return s.Get(ctx, groupID)
}

func (s *GroupQuotaPolicyService) Resolve(groupID int64, model string) GroupQuotaPolicyResolution {
	result := GroupQuotaPolicyResolution{SourceModel: model, TargetModel: model}
	if s == nil {
		return result
	}
	s.mu.RLock()
	entry, ok := s.cache[groupID]
	if ok {
		result.Revision = entry.revision
		if mapped, exists := entry.mappings[model]; exists {
			result = mapped
		}
	}
	s.mu.RUnlock()
	return result
}

func (s *GroupQuotaPolicyService) publish(record *GroupQuotaPolicyRecord) {
	entry := groupQuotaCacheEntry{revision: record.Revision, runtimeVersion: record.RuntimeVersion, mappings: make(map[string]GroupQuotaPolicyResolution)}
	for _, rule := range record.Config.Rules {
		for _, mapping := range rule.Mappings {
			if _, ok := entry.mappings[mapping.Source]; ok {
				continue
			}
			result := resolveGroupQuotaPolicy(record.Config, record.Runtime, record.Revision, mapping.Source)
			if result.Applied {
				entry.mappings[mapping.Source] = result
			}
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	previous := s.cache[record.GroupID]
	if previous.revision > record.Revision || (previous.revision == record.Revision && previous.runtimeVersion > record.RuntimeVersion) {
		return
	}
	s.generation++
	entry.generation = s.generation
	s.cache[record.GroupID] = entry
}

func (s *GroupQuotaPolicyService) syncRecords(ctx context.Context) ([]*GroupQuotaPolicyRecord, error) {
	s.mu.RLock()
	generation := s.generation
	s.mu.RUnlock()
	records, err := s.repo.List(ctx)
	if err != nil {
		return nil, err
	}
	present := make(map[int64]bool)
	for _, record := range records {
		present[record.GroupID] = true
		s.publish(record)
	}
	s.mu.Lock()
	for id, entry := range s.cache {
		if !present[id] && entry.generation <= generation {
			delete(s.cache, id)
		}
	}
	s.mu.Unlock()
	return records, nil
}

func (s *GroupQuotaPolicyService) Start() {
	if s == nil || s.repo == nil || s.quota == nil {
		return
	}
	s.start.Do(func() {
		ctx, cancel := context.WithTimeout(s.ctx, 5*time.Second)
		_, err := s.syncRecords(ctx)
		cancel()
		if err != nil {
			slog.Warn("group_quota_policy_initial_load_failed")
		}
		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			ticker := time.NewTicker(time.Minute)
			defer ticker.Stop()
			s.scan()
			for {
				select {
				case <-s.ctx.Done():
					return
				case <-ticker.C:
					s.scan()
				}
			}
		}()
	})
}

func (s *GroupQuotaPolicyService) Stop() {
	if s == nil {
		return
	}
	s.stop.Do(func() { s.cancel(); s.wg.Wait() })
}

func (s *GroupQuotaPolicyService) scan() {
	ctx, cancel := context.WithTimeout(s.ctx, 50*time.Second)
	defer cancel()
	records, err := s.syncRecords(ctx)
	if err != nil {
		slog.Warn("group_quota_policy_sync_failed")
		return
	}
	if len(records) == 0 {
		return
	}
	if s.leader != nil {
		acquired, err := s.leader.TryAcquireLeaderLock(ctx, "jobs:group-quota-policy", s.owner, 55*time.Second)
		if err != nil || !acquired {
			return
		}
		defer func() {
			releaseCtx, releaseCancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer releaseCancel()
			_ = s.leader.ReleaseLeaderLock(releaseCtx, "jobs:group-quota-policy", s.owner)
		}()
	}
	eligible := make(map[int64]bool)
	groupEligibility := make(map[int64]map[int64]bool)
	groupErrors := make(map[int64]bool)
	for _, record := range records {
		_, members, err := s.eligibleAccounts(ctx, record.GroupID)
		if err != nil {
			members = map[int64]bool{}
			groupErrors[record.GroupID] = true
		}
		groupEligibility[record.GroupID] = members
		for id := range members {
			eligible[id] = true
		}
	}
	queryCtx, queryCancel := context.WithTimeout(ctx, 40*time.Second)
	samples := s.collect(queryCtx, groupQuotaAccountIDs(records), eligible)
	queryCancel()
	for _, record := range records {
		if ctx.Err() != nil {
			return
		}
		perGroup := make(map[int64]GroupQuotaPolicySnapshot)
		for _, id := range groupQuotaAccountIDs([]*GroupQuotaPolicyRecord{record}) {
			if !groupEligibility[record.GroupID][id] {
				perGroup[id] = GroupQuotaPolicySnapshot{AccountID: id, CheckedAt: time.Now().Unix(), ErrorCode: "ACCOUNT_UNAVAILABLE"}
				if groupErrors[record.GroupID] {
					snapshot := perGroup[id]
					snapshot.ErrorCode = "ACCOUNT_LOOKUP_FAILED"
					perGroup[id] = snapshot
				}
			} else if snapshot, ok := samples[id]; ok {
				perGroup[id] = snapshot
			}
		}
		if err := s.persistSamples(ctx, record.GroupID, perGroup); err != nil {
			slog.Warn("group_quota_policy_persist_failed", "group_id", record.GroupID)
		}
	}
}

func groupQuotaAccountIDs(records []*GroupQuotaPolicyRecord) []int64 {
	seen := make(map[int64]bool)
	for _, record := range records {
		for _, rule := range record.Config.Rules {
			if rule.Mode != "disabled" {
				for _, id := range rule.AccountIDs {
					seen[id] = true
				}
			}
		}
	}
	ids := make([]int64, 0, len(seen))
	for id := range seen {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids
}

func (s *GroupQuotaPolicyService) collect(ctx context.Context, ids []int64, eligible map[int64]bool) map[int64]GroupQuotaPolicySnapshot {
	// Oldest checked accounts go first so a large group cannot starve later IDs
	// when the bounded cycle times out. Four requests at most are in flight.
	s.sampleMu.Lock()
	sort.SliceStable(ids, func(i, j int) bool { return s.samples[ids[i]].CheckedAt < s.samples[ids[j]].CheckedAt })
	s.sampleMu.Unlock()
	results := make(map[int64]GroupQuotaPolicySnapshot)
	var mu sync.Mutex
	var wg sync.WaitGroup
	jobs := make(chan int64)
	for worker := 0; worker < 4; worker++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for id := range jobs {
				if ctx.Err() != nil {
					continue
				}
				snapshot := GroupQuotaPolicySnapshot{AccountID: id, CheckedAt: time.Now().Unix(), ErrorCode: "ACCOUNT_UNAVAILABLE"}
				if eligible[id] {
					snapshot = s.sample(ctx, id)
				}
				mu.Lock()
				results[id] = snapshot
				mu.Unlock()
			}
		}()
	}
	for _, id := range ids {
		select {
		case jobs <- id:
		case <-ctx.Done():
		}
	}
	close(jobs)
	wg.Wait()
	return results
}

func (s *GroupQuotaPolicyService) sample(ctx context.Context, accountID int64) GroupQuotaPolicySnapshot {
	result, _, _ := s.sampling.Do(strconv.FormatInt(accountID, 10), func() (any, error) {
		s.sampleMu.Lock()
		cached, ok := s.samples[accountID]
		s.sampleMu.Unlock()
		if ok && time.Now().Unix()-cached.CheckedAt < 15 {
			return cached, nil
		}
		usage, err := s.quota.QueryUsageWindows(ctx, accountID)
		snapshot := groupQuotaSnapshot(accountID, usage, time.Now())
		if err != nil {
			snapshot.ErrorCode = infraerrors.Reason(err)
			if snapshot.ErrorCode == "" {
				snapshot.ErrorCode = "QUOTA_QUERY_FAILED"
			}
		}
		s.sampleMu.Lock()
		if old := s.samples[accountID]; snapshot.CheckedAt >= old.CheckedAt {
			s.samples[accountID] = snapshot
		}
		s.sampleMu.Unlock()
		return snapshot, nil
	})
	return result.(GroupQuotaPolicySnapshot)
}

func mergeGroupQuotaSamples(runtime GroupQuotaPolicyRuntime, samples map[int64]GroupQuotaPolicySnapshot, config GroupQuotaPolicyConfig) GroupQuotaPolicyRuntime {
	byID := make(map[int64]GroupQuotaPolicySnapshot)
	wanted := make(map[int64]bool)
	for _, rule := range config.Rules {
		for _, id := range rule.AccountIDs {
			wanted[id] = true
		}
	}
	for _, snapshot := range runtime.Snapshots {
		if wanted[snapshot.AccountID] {
			byID[snapshot.AccountID] = snapshot
		}
	}
	for id, incoming := range samples {
		if !wanted[id] {
			continue
		}
		old := byID[id]
		if incoming.CheckedAt < old.CheckedAt || (incoming.SampledAt > 0 && incoming.SampledAt < old.SampledAt) {
			continue
		}
		if incoming.ErrorCode != "" && old.SampledAt > 0 {
			old.ErrorCode, old.CheckedAt = incoming.ErrorCode, incoming.CheckedAt
			byID[id] = old
		} else {
			byID[id] = incoming
		}
	}
	runtime.Snapshots = make([]GroupQuotaPolicySnapshot, 0, len(byID))
	for _, snapshot := range byID {
		runtime.Snapshots = append(runtime.Snapshots, snapshot)
	}
	sort.Slice(runtime.Snapshots, func(i, j int) bool { return runtime.Snapshots[i].AccountID < runtime.Snapshots[j].AccountID })
	return runtime
}

func (s *GroupQuotaPolicyService) persistSamples(ctx context.Context, groupID int64, samples map[int64]GroupQuotaPolicySnapshot) error {
	for attempt := 0; attempt < 3; attempt++ {
		record, err := s.repo.Get(ctx, groupID)
		if err != nil {
			return err
		}
		if record.Revision == 0 {
			return nil
		}
		record.Runtime = mergeGroupQuotaSamples(record.Runtime, samples, record.Config)
		record.Runtime = evaluateGroupQuotaPolicy(record.Config, record.Runtime, time.Now())
		ok, err := s.repo.SaveRuntime(ctx, record, record.Revision, record.RuntimeVersion)
		if err != nil {
			return err
		}
		if !ok {
			continue
		}
		record.RuntimeVersion++
		s.publish(record)
		return nil
	}
	return fmt.Errorf("persist quota policy runtime: %w", ErrGroupQuotaPolicyConflict)
}
