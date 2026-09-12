package service

import (
	"context"
	"database/sql"
	"log/slog"
	"math"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

type subscriptionResetQuotaReader interface {
	QueryUsageSnapshot(context.Context, int64) (*OpenAIQuotaUsage, error)
}

type resetPollSchedule struct {
	next     time.Time
	failures int
}

// SubscriptionResetMonitor has its own durable baselines. It never consumes
// the history ingester's processed_at cursor or interprets timer expiry as a
// reset. One bounded poll is shared across every group referencing the account.
type SubscriptionResetMonitor struct {
	repo             SubscriptionResetObserverRepository
	accounts         AccountRepository
	quota            subscriptionResetQuotaReader
	leader           LeaderLockCache
	db               *sql.DB
	owner            string
	ctx              context.Context
	cancel           context.CancelFunc
	mu               sync.Mutex
	started, stopped bool
	wg               sync.WaitGroup
	schedule         map[int64]resetPollSchedule
	now              func() time.Time
}

func NewSubscriptionResetMonitor(repo SubscriptionResetObserverRepository, accounts AccountRepository, quota subscriptionResetQuotaReader, leader LeaderLockCache, db *sql.DB) *SubscriptionResetMonitor {
	ctx, cancel := context.WithCancel(context.Background())
	return &SubscriptionResetMonitor{repo: repo, accounts: accounts, quota: quota, leader: leader, db: db, owner: uuid.NewString(), ctx: ctx, cancel: cancel, schedule: make(map[int64]resetPollSchedule), now: time.Now}
}

func (m *SubscriptionResetMonitor) Start() {
	if m == nil || m.repo == nil || m.accounts == nil || m.quota == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.started || m.stopped {
		return
	}
	m.started = true
	m.wg.Add(1)
	go func() {
		defer m.wg.Done()
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-m.ctx.Done():
				return
			case <-ticker.C:
				m.runOnce(m.ctx)
			}
		}
	}()
}

func (m *SubscriptionResetMonitor) Stop() {
	if m == nil {
		return
	}
	m.mu.Lock()
	m.stopped = true
	m.cancel()
	m.mu.Unlock()
	m.wg.Wait()
}

func (m *SubscriptionResetMonitor) runOnce(parent context.Context) {
	ctx, cancel := context.WithTimeout(parent, 25*time.Second)
	defer cancel()
	release, acquired := tryAcquireSingletonLeaderLock(ctx, m.leader, m.db, "jobs:subscription-reset-observer", m.owner, 35*time.Second)
	if !acquired {
		return
	}
	defer release()
	policies, err := m.repo.ListEnabled(ctx)
	if err != nil {
		slog.Warn("subscription_reset: list policies failed", "error", err)
		return
	}
	refs := make(map[int64][]*SubscriptionResetPolicy)
	for _, policy := range policies {
		for _, id := range policy.AccountIDs {
			refs[id] = append(refs[id], policy)
		}
	}
	for id := range m.schedule {
		if _, ok := refs[id]; !ok {
			delete(m.schedule, id)
		}
	}
	ids := make([]int64, 0, len(refs))
	now := m.now()
	for id := range refs {
		if !m.schedule[id].next.After(now) {
			ids = append(ids, id)
		}
	}
	sort.Slice(ids, func(i, j int) bool {
		a, b := m.schedule[ids[i]].next, m.schedule[ids[j]].next
		if a.Equal(b) {
			return ids[i] < ids[j]
		}
		return a.Before(b)
	})
	if len(ids) > 40 {
		ids = ids[:40]
	}
	for i, id := range ids {
		if i > 0 {
			timer := time.NewTimer(500 * time.Millisecond)
			select {
			case <-ctx.Done():
				timer.Stop()
				return
			case <-timer.C:
			}
		}
		if ctx.Err() != nil {
			return
		}
		sample, account := m.fetchSample(ctx, id)
		for _, policy := range refs[id] {
			copySample := *sample
			if account == nil || !slices.Contains(account.GroupIDs, policy.GroupID) {
				copySample.Error = "account_not_in_group"
			}
			if err := m.repo.RecordSample(ctx, policy.GroupID, policy.Version, &copySample); err != nil {
				slog.Warn("subscription_reset: observation write failed", "group_id", policy.GroupID, "account_id", id, "error", err)
			}
		}
		delay := 2 * time.Minute
		state := m.schedule[id]
		if sample.Error != "" {
			state.failures++
			delay = time.Minute * time.Duration(1<<min(state.failures, 4))
		} else {
			state.failures = 0
			if sample.ResetAt != nil && sample.ResetAt.Before(m.now().Add(10*time.Minute)) {
				delay = 30 * time.Second
			}
		}
		state.next = m.now().Add(delay)
		m.schedule[id] = state
	}
}

func (m *SubscriptionResetMonitor) fetchSample(ctx context.Context, id int64) (*SubscriptionResetSample, *Account) {
	sample := &SubscriptionResetSample{AccountID: id, ObservedAt: m.now()}
	account, err := m.accounts.GetByID(ctx, id)
	if err != nil || !IsOpenAIQuotaHistoryAccount(account) {
		sample.Error = "account_unavailable"
		return sample, account
	}
	if account.Status != StatusActive {
		sample.Error = "account_disabled"
		return sample, account
	}
	usage, err := m.quota.QueryUsageSnapshot(ctx, id)
	sample.ObservedAt = m.now()
	if err != nil || usage == nil {
		sample.Error = "quota_query_failed"
		return sample, account
	}
	// Refresh after the quota GET to capture local reset-credit markers written
	// concurrently and changes to identity/group membership during the request.
	latest, err := m.accounts.GetByID(ctx, id)
	if err != nil || !IsOpenAIQuotaHistoryAccount(latest) || latest.Status != StatusActive {
		sample.Error = "account_unavailable"
		return sample, latest
	}
	account = latest
	populateSubscriptionResetSample(sample, usage)
	if marker, ok := account.Extra["codex_history_reset_at"].(string); ok {
		if at, err := time.Parse(time.RFC3339Nano, marker); err == nil {
			sample.LocalResetAt = &at
		}
	}
	return sample, account
}

func populateSubscriptionResetSample(sample *SubscriptionResetSample, usage *OpenAIQuotaUsage) {
	if usage == nil || strings.TrimSpace(usage.UserID) == "" || strings.TrimSpace(usage.AccountID) == "" {
		sample.Error = "quota_identity_unknown"
		return
	}
	sample.Subject = &SubscriptionResetQuotaSubject{UserID: usage.UserID, AccountID: usage.AccountID, Scope: "codex:global"}
	sample.PlanType = usage.PlanType
	if usage.RateLimit == nil {
		sample.Error = "quota_window_unknown"
		return
	}
	var selected *OpenAIRateLimitWindow
	for _, window := range []*OpenAIRateLimitWindow{usage.RateLimit.PrimaryWindow, usage.RateLimit.SecondaryWindow} {
		if window != nil && window.LimitWindowSeconds == 604800 {
			if selected != nil {
				sample.Error = "quota_window_ambiguous"
				return
			}
			selected = window
		}
	}
	if selected == nil || !selected.observationFieldsPresent || selected.ResetAt <= 0 || math.IsNaN(selected.UsedPercent) || math.IsInf(selected.UsedPercent, 0) || selected.UsedPercent < 0 || selected.UsedPercent > 100 {
		sample.Error = "quota_window_unknown"
		return
	}
	reset := time.Unix(selected.ResetAt, 0).UTC()
	minutes := int(selected.LimitWindowSeconds / 60)
	used := selected.UsedPercent
	sample.ResetAt = &reset
	sample.WindowMinutes = &minutes
	sample.UsedPercent = &used
}
