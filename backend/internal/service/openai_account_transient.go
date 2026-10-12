package service

import (
	"fmt"
	"sort"
	"sync"
	"time"
)

const openAIAccountTransientStreakTTL = 30 * time.Minute

var openAIAccountTransientBackoff = [...]time.Duration{
	10 * time.Second,
	45 * time.Second,
	2 * time.Minute,
	5 * time.Minute,
	15 * time.Minute,
}

type openAIAccountTransientEntry struct {
	failureStreak  int
	lastFailure    time.Time
	blockedUntil   time.Time
	probeStartedAt time.Time
	recovery       openAITransientRecovery
	reason         string
}

type openAIAccountTransientDecision struct {
	FailureStreak int
	Cooldown      time.Duration
	BlockUntil    time.Time
}

type openAIAccountTransientState struct {
	mu           sync.Mutex
	entries      map[int64]openAIAccountTransientEntry
	nextProbe    uint64
	observations map[int64][]openAIAccountFailureObservation
	waiters      map[int64]int
	waiterTotal  int
}

type openAIAccountFailureObservation struct {
	model string
	at    time.Time
}

func newOpenAIAccountTransientState() *openAIAccountTransientState {
	return &openAIAccountTransientState{entries: make(map[int64]openAIAccountTransientEntry)}
}

func (s *openAIAccountTransientState) entryLocked(accountID int64, now time.Time) (openAIAccountTransientEntry, bool) {
	entry, exists := s.entries[accountID]
	if !exists {
		return entry, false
	}
	lastActivity := entry.lastFailure
	if entry.probeStartedAt.After(lastActivity) {
		lastActivity = entry.probeStartedAt
	}
	if now.Before(lastActivity) {
		entry.blockedUntil = now
		entry.recovery = openAITransientRecovery{}
		s.entries[accountID] = entry
	}
	return entry, true
}

func (s *openAIAccountTransientState) recordFailure(accountID int64, now time.Time, reason ...string) openAIAccountTransientDecision {
	if s == nil || accountID <= 0 {
		return openAIAccountTransientDecision{}
	}
	if now.IsZero() {
		now = time.Now()
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.recordFailureLocked(accountID, now, reason...)
}

func (s *openAIAccountTransientState) recordFailureLocked(accountID int64, now time.Time, reason ...string) openAIAccountTransientDecision {
	if s.entries == nil {
		s.entries = make(map[int64]openAIAccountTransientEntry)
	}
	entry, exists := s.entryLocked(accountID, now)
	if now.Sub(entry.lastFailure) > openAIAccountTransientStreakTTL {
		entry.failureStreak = 0
	}
	// Requests already in flight belong to the same failed window. They must
	// neither extend it nor turn one traffic burst into a 15-minute lockout.
	if exists && now.Before(entry.blockedUntil) {
		return openAIAccountTransientDecision{FailureStreak: entry.failureStreak, BlockUntil: entry.blockedUntil}
	}
	if !exists && len(s.entries) >= openAIModelTransientDefaultMax {
		var oldestID int64
		var oldest time.Time
		for id, item := range s.entries {
			if oldest.IsZero() || item.lastFailure.Before(oldest) {
				oldestID, oldest = id, item.lastFailure
			}
		}
		delete(s.entries, oldestID)
	}
	if entry.failureStreak < len(openAIAccountTransientBackoff) {
		entry.failureStreak++
	}
	entry.lastFailure = now
	entry.recovery = openAITransientRecovery{}
	entry.recovery.required = 1
	if entry.failureStreak >= 2 {
		entry.recovery.required = 2
	}
	entry.reason = "upstream_connection"
	if len(reason) > 0 {
		entry.reason = reason[0]
	}
	entry.probeStartedAt = time.Time{}
	cooldown := openAIAccountTransientBackoff[entry.failureStreak-1]
	entry.blockedUntil = now.Add(cooldown)
	s.entries[accountID] = entry
	return openAIAccountTransientDecision{
		FailureStreak: entry.failureStreak,
		Cooldown:      cooldown,
		BlockUntil:    entry.blockedUntil,
	}
}

func (s *openAIAccountTransientState) isBlocked(accountID int64, now time.Time) bool {
	if s == nil {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	entry, exists := s.entryLocked(accountID, now)
	return exists && (now.Before(entry.blockedUntil) || !entry.recovery.canReserve(now))
}

func (s *openAIAccountTransientState) hasEntry(accountID int64, now time.Time) bool {
	if s == nil {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	_, exists := s.entryLocked(accountID, now)
	return exists
}

func (s *openAIAccountTransientState) reserveProbe(accountID int64, now time.Time) (blocked bool, probeID uint64) {
	if s == nil {
		return false, 0
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	entry, exists := s.entryLocked(accountID, now)
	if !exists {
		return false, 0
	}
	if now.Before(entry.blockedUntil) || !entry.recovery.canReserve(now) {
		return true, 0
	}
	s.nextProbe++
	entry.recovery.reserve(s.nextProbe, now)
	entry.probeStartedAt = now
	s.entries[accountID] = entry
	return false, s.nextProbe
}

func (s *openAIAccountTransientState) releaseProbe(accountID int64, probeID uint64) {
	if s == nil || probeID == 0 {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	entry, exists := s.entries[accountID]
	// A delayed release from an earlier request cannot release a newer probe.
	if !exists {
		return
	}
	delete(entry.recovery.probes, probeID)
	s.entries[accountID] = entry
}

func (s *openAIAccountTransientState) recordSuccess(accountID int64, now time.Time, probeID ...uint64) bool {
	if s == nil {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	entry, exists := s.entries[accountID]
	if exists && !now.Before(entry.blockedUntil) && len(probeID) > 0 {
		accepted, recovered := entry.recovery.success(probeID[0])
		if recovered {
			delete(s.entries, accountID)
		} else {
			s.entries[accountID] = entry
		}
		return accepted
	}
	return false
}

// A successful request breaks evidence of an account-wide outage, but cannot
// clear an existing circuit. That requires the matching recovery probes.
func (s *openAIAccountTransientState) observeSuccess(accountID int64) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.observations, accountID)
}

func (s *openAIAccountTransientState) observeModelFailure(accountID int64, model string, status int, now time.Time) openAIAccountTransientDecision {
	if s == nil || model == "" {
		return openAIAccountTransientDecision{}
	}
	s.mu.Lock()
	if s.observations == nil {
		s.observations = make(map[int64][]openAIAccountFailureObservation)
	}
	if len(s.observations) >= openAIModelTransientDefaultMax {
		for id, items := range s.observations {
			if len(items) == 0 || now.Sub(items[len(items)-1].at) > time.Minute {
				delete(s.observations, id)
			}
		}
		if len(s.observations) >= openAIModelTransientDefaultMax {
			s.mu.Unlock()
			return openAIAccountTransientDecision{}
		}
	}
	items := s.observations[accountID]
	kept := make([]openAIAccountFailureObservation, 0, 4)
	models := make(map[string]bool)
	duplicate := false
	for _, item := range items {
		if now.Sub(item.at) <= time.Minute {
			kept = append(kept, item)
			models[item.model] = true
			if item.model == model && now.Sub(item.at) < 5*time.Second {
				duplicate = true
			}
		}
	}
	if !duplicate {
		kept = append(kept, openAIAccountFailureObservation{model, now})
		models[model] = true
	}
	trip := len(kept) >= 3 && len(models) >= 2
	if trip {
		delete(s.observations, accountID)
	} else {
		s.observations[accountID] = kept
	}
	s.mu.Unlock()
	if trip {
		return s.recordFailure(accountID, now, fmt.Sprintf("cross_model_%d", status))
	}
	return openAIAccountTransientDecision{}
}

func (s *openAIAccountTransientState) retryAt(accountID int64, now time.Time) time.Time {
	s.mu.Lock()
	defer s.mu.Unlock()
	entry, exists := s.entryLocked(accountID, now)
	if !exists {
		return time.Time{}
	}
	return entry.recovery.retryAt(entry.blockedUntil, now)
}

func (s *openAIAccountTransientState) snapshot(now time.Time) []OpenAIAccountModelTransientSnapshot {
	s.mu.Lock()
	defer s.mu.Unlock()
	items := make([]OpenAIAccountModelTransientSnapshot, 0)
	for id := range s.entries {
		entry, exists := s.entryLocked(id, now)
		if !exists {
			continue
		}
		remaining := int64(entry.blockedUntil.Sub(now).Seconds())
		if remaining < 0 {
			remaining = 0
		}
		items = append(items, OpenAIAccountModelTransientSnapshot{
			AccountID: id, Model: "*", FailureStreak: entry.failureStreak,
			LastFailureAt: entry.lastFailure.UTC(), BlockUntil: entry.blockedUntil.UTC(),
			RemainingSecs: remaining,
			Scope:         "account", State: entry.recovery.state(entry.blockedUntil, now), Reason: entry.reason,
			ProbesInFlight: len(entry.recovery.probes), RecoverySuccesses: entry.recovery.successes,
			RecoveryRequired: entry.recovery.requiredSuccesses(), ProbeStartedAt: entry.recovery.oldestProbe(),
		})
	}
	sort.Slice(items, func(i, j int) bool { return items[i].AccountID < items[j].AccountID })
	return items
}

func (s *openAIAccountTransientState) reserveWait(groupID int64) (func(), bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.waiterTotal >= 64 || s.waiters[groupID] >= 8 {
		return nil, false
	}
	if s.waiters == nil {
		s.waiters = make(map[int64]int)
	}
	s.waiters[groupID]++
	s.waiterTotal++
	var once sync.Once
	return func() {
		once.Do(func() {
			s.mu.Lock()
			defer s.mu.Unlock()
			s.waiters[groupID]--
			s.waiterTotal--
			if s.waiters[groupID] == 0 {
				delete(s.waiters, groupID)
			}
		})
	}, true
}
