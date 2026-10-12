package service

import (
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	// openAIModelTransientStreakTTL bounds how long a failure streak survives
	// without a new failure. It exists only so the map does not keep state for
	// account+model pairs that stopped being used; a streak is otherwise reset
	// by recordSuccess alone.
	//
	// It must stay well above the cooldowns. Resetting the streak on a short
	// wall-clock window makes the breaker's sensitivity depend on request rate:
	// a gateway called less often than the window never reaches streak 2, so a
	// broken upstream is never cooled down and every request pays a failed
	// attempt plus a failover before reaching a healthy account. Low-traffic
	// deployments were hit hardest, which is the opposite of what a breaker
	// should do.
	openAIModelTransientStreakTTL       = 30 * time.Minute
	openAIModelTransientShortCooldown   = 10 * time.Second
	openAIModelTransientLongCooldown    = 45 * time.Second
	openAIModelTransientTimeoutCooldown = 90 * time.Second
	openAIModelTransientDefaultMax      = 4096
	openAIModelTransientMaxModelBytes   = 512
)

type openAIAccountModelKey struct {
	AccountID int64
	Model     string
}

type openAIAccountModelTransientEntry struct {
	failureStreak int
	lastFailure   time.Time
	blockUntil    time.Time
	lastTouched   time.Time
	requireProbe  bool
	recovery      openAITransientRecovery
	reason        string
}

type openAIAccountModelTransientDecision struct {
	FailureStreak int
	Cooldown      time.Duration
	BlockUntil    time.Time
}

type OpenAIAccountModelTransientSnapshot struct {
	AccountID         int64      `json:"account_id"`
	Model             string     `json:"model"`
	FailureStreak     int        `json:"failure_streak"`
	LastFailureAt     time.Time  `json:"last_failure_at"`
	BlockUntil        time.Time  `json:"block_until"`
	RemainingSecs     int64      `json:"remaining_seconds"`
	Scope             string     `json:"scope,omitempty"`
	State             string     `json:"state,omitempty"`
	Reason            string     `json:"reason,omitempty"`
	ProbesInFlight    int        `json:"probes_in_flight,omitempty"`
	RecoverySuccesses int        `json:"recovery_successes,omitempty"`
	RecoveryRequired  int        `json:"recovery_required,omitempty"`
	ProbeStartedAt    *time.Time `json:"probe_started_at,omitempty"`
}

type openAIAccountModelTransientState struct {
	mu         sync.Mutex
	entries    map[openAIAccountModelKey]openAIAccountModelTransientEntry
	maxEntries int
	nextProbe  uint64
}

type openAITransientFailureOptions struct {
	requireProbe bool
	reason       string
	// Opt-in policy; zero values preserve OpenAI behavior.
	backoffBase      time.Duration
	backoffCap       time.Duration
	failureThreshold int
}

func newOpenAIAccountModelTransientState(maxEntries int) *openAIAccountModelTransientState {
	if maxEntries <= 0 {
		maxEntries = openAIModelTransientDefaultMax
	}
	return &openAIAccountModelTransientState{
		entries:    make(map[openAIAccountModelKey]openAIAccountModelTransientEntry),
		maxEntries: maxEntries,
	}
}

func normalizeOpenAIAccountModelTransientModel(model string) string {
	model = strings.TrimSpace(model)
	if len(model) > openAIModelTransientMaxModelBytes {
		return ""
	}
	return strings.ToLower(model)
}

func openAIAccountModelTransientKey(accountID int64, model string) (openAIAccountModelKey, bool) {
	model = normalizeOpenAIAccountModelTransientModel(model)
	if accountID <= 0 || model == "" {
		return openAIAccountModelKey{}, false
	}
	return openAIAccountModelKey{AccountID: accountID, Model: model}, true
}

func (s *openAIAccountModelTransientState) recordFailure(accountID int64, model string, now time.Time) openAIAccountModelTransientDecision {
	return s.recordFailureWithInitialCooldown(accountID, model, now, 0)
}

func (s *openAIAccountModelTransientState) recordFailureWithInitialCooldown(accountID int64, model string, now time.Time, initialCooldown time.Duration, options ...openAITransientFailureOptions) openAIAccountModelTransientDecision {
	key, ok := openAIAccountModelTransientKey(accountID, model)
	if s == nil || !ok {
		return openAIAccountModelTransientDecision{}
	}
	if now.IsZero() {
		now = time.Now()
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.entries == nil {
		s.entries = make(map[openAIAccountModelKey]openAIAccountModelTransientEntry)
	}
	if s.maxEntries <= 0 {
		s.maxEntries = openAIModelTransientDefaultMax
	}

	entry, exists := s.entries[key]
	if len(options) > 0 {
		entry.requireProbe = options[0].requireProbe
		entry.reason = options[0].reason
	}
	if entry.requireProbe && exists && now.Before(entry.blockUntil) {
		return openAIAccountModelTransientDecision{FailureStreak: entry.failureStreak, BlockUntil: entry.blockUntil}
	}
	if !exists {
		s.evictOldestLocked()
	}
	// The streak is cleared by recordSuccess. Only drop it here when the entry
	// is stale beyond the TTL, or when the clock moved backwards.
	if !exists || entry.lastFailure.IsZero() || now.Sub(entry.lastFailure) > openAIModelTransientStreakTTL || now.Before(entry.lastFailure) {
		entry.failureStreak = 0
		entry.blockUntil = time.Time{}
	}
	entry.failureStreak++
	entry.lastFailure = now
	entry.lastTouched = now
	entry.recovery = openAITransientRecovery{}
	entry.recovery.required = 1
	if entry.failureStreak >= 2 {
		entry.recovery.required = 2
	}

	cooldown := initialCooldown
	switch {
	case entry.failureStreak >= 3:
		cooldown = openAIModelTransientLongCooldown
	case entry.failureStreak == 2:
		cooldown = openAIModelTransientShortCooldown
	}
	if initialCooldown > cooldown {
		cooldown = initialCooldown
	}
	if len(options) > 0 && options[0].backoffBase > 0 {
		opts := options[0]
		threshold := max(1, opts.failureThreshold)
		if entry.failureStreak < threshold {
			cooldown = 0
			entry.requireProbe = false
		} else {
			cooldown = opts.backoffBase
			for n := threshold; n < entry.failureStreak && cooldown < opts.backoffCap; n++ {
				cooldown = min(cooldown*2, opts.backoffCap)
			}
			// Retry-After remains a floor, even above our local backoff cap.
			if initialCooldown > cooldown {
				cooldown = initialCooldown
			}
		}
	}
	if cooldown > 0 {
		entry.blockUntil = now.Add(cooldown)
	} else {
		entry.blockUntil = time.Time{}
	}
	s.entries[key] = entry
	return openAIAccountModelTransientDecision{
		FailureStreak: entry.failureStreak,
		Cooldown:      cooldown,
		BlockUntil:    entry.blockUntil,
	}
}

func (s *openAIAccountModelTransientState) recordSuccess(accountID int64, model string) {
	key, ok := openAIAccountModelTransientKey(accountID, model)
	if s == nil || !ok {
		return
	}
	s.mu.Lock()
	if entry, exists := s.entries[key]; exists && !entry.requireProbe {
		delete(s.entries, key)
	}
	s.mu.Unlock()
}

func (s *openAIAccountModelTransientState) isBlocked(accountID int64, model string, now time.Time) bool {
	key, ok := openAIAccountModelTransientKey(accountID, model)
	if s == nil || !ok {
		return false
	}
	if now.IsZero() {
		now = time.Now()
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	entry, exists := s.entries[key]
	if !exists {
		return false
	}
	if !entry.requireProbe && !entry.lastFailure.IsZero() && now.Sub(entry.lastFailure) > openAIModelTransientStreakTTL {
		delete(s.entries, key)
		return false
	}
	entry.lastTouched = now
	s.entries[key] = entry
	return (!entry.blockUntil.IsZero() && now.Before(entry.blockUntil)) || (entry.requireProbe && !entry.recovery.canReserve(now))
}

func (s *openAIAccountModelTransientState) reserveProbe(accountID int64, model string, now time.Time) (bool, uint64) {
	key, valid := openAIAccountModelTransientKey(accountID, model)
	if s == nil || !valid {
		return false, 0
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	entry, exists := s.entries[key]
	if !exists || !entry.requireProbe {
		return false, 0
	}
	if now.Before(entry.blockUntil) || !entry.recovery.canReserve(now) {
		return true, 0
	}
	s.nextProbe++
	entry.recovery.reserve(s.nextProbe, now)
	entry.lastTouched = now
	s.entries[key] = entry
	return false, s.nextProbe
}

func (s *openAIAccountModelTransientState) releaseProbe(accountID int64, model string, id uint64) {
	key, valid := openAIAccountModelTransientKey(accountID, model)
	if s == nil || !valid || id == 0 {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	entry, exists := s.entries[key]
	if exists {
		delete(entry.recovery.probes, id)
		s.entries[key] = entry
	}
}

func (s *openAIAccountModelTransientState) completeProbe(accountID int64, model string, id uint64, now time.Time) bool {
	key, valid := openAIAccountModelTransientKey(accountID, model)
	if s == nil || !valid || id == 0 {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	entry, exists := s.entries[key]
	if !exists || now.Before(entry.blockUntil) {
		return false
	}
	accepted, recovered := entry.recovery.success(id)
	if recovered {
		delete(s.entries, key)
	} else {
		s.entries[key] = entry
	}
	return accepted
}

func (s *openAIAccountModelTransientState) retryAt(accountID int64, model string, now time.Time) time.Time {
	key, valid := openAIAccountModelTransientKey(accountID, model)
	if s == nil || !valid {
		return time.Time{}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	entry, exists := s.entries[key]
	if !exists || !entry.requireProbe {
		return time.Time{}
	}
	return entry.recovery.retryAt(entry.blockUntil, now)
}

func (s *openAIAccountModelTransientState) size() int {
	if s == nil {
		return 0
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.entries)
}

func (s *openAIAccountModelTransientState) snapshot(now time.Time) []OpenAIAccountModelTransientSnapshot {
	if s == nil {
		return []OpenAIAccountModelTransientSnapshot{}
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	items := make([]OpenAIAccountModelTransientSnapshot, 0, len(s.entries))
	for key, entry := range s.entries {
		if !entry.requireProbe && (entry.lastFailure.IsZero() || now.Sub(entry.lastFailure) > openAIModelTransientStreakTTL) {
			delete(s.entries, key)
			continue
		}
		if !entry.requireProbe && (entry.blockUntil.IsZero() || !now.Before(entry.blockUntil)) {
			continue
		}

		remaining := int64(entry.blockUntil.Sub(now).Seconds())
		if remaining < 0 {
			remaining = 0
		}
		items = append(items, OpenAIAccountModelTransientSnapshot{
			AccountID:     key.AccountID,
			Model:         key.Model,
			FailureStreak: entry.failureStreak,
			LastFailureAt: entry.lastFailure.UTC(),
			BlockUntil:    entry.blockUntil.UTC(),
			RemainingSecs: remaining,
			Scope:         "account_model", State: entry.recovery.state(entry.blockUntil, now), Reason: entry.reason,
			ProbesInFlight: len(entry.recovery.probes), RecoverySuccesses: entry.recovery.successes,
			RecoveryRequired: entry.recovery.requiredSuccesses(), ProbeStartedAt: entry.recovery.oldestProbe(),
		})
	}

	sort.Slice(items, func(i, j int) bool {
		if items[i].BlockUntil.Equal(items[j].BlockUntil) {
			if items[i].AccountID == items[j].AccountID {
				return items[i].Model < items[j].Model
			}
			return items[i].AccountID < items[j].AccountID
		}
		return items[i].BlockUntil.Before(items[j].BlockUntil)
	})
	return items
}

func (s *openAIAccountModelTransientState) evictOldestLocked() {
	if len(s.entries) < s.maxEntries {
		return
	}
	var oldestKey openAIAccountModelKey
	var oldestTime time.Time
	found := false
	for key, entry := range s.entries {
		if !found || entry.lastTouched.Before(oldestTime) {
			oldestKey = key
			oldestTime = entry.lastTouched
			found = true
		}
	}
	if found {
		delete(s.entries, oldestKey)
	}
}
