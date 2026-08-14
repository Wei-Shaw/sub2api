package service

import (
	"strings"
	"sync"
	"time"
)

const (
	openAIModelTransientFailureWindow = time.Minute
	openAIModelTransientShortCooldown = 10 * time.Second
	openAIModelTransientLongCooldown  = 45 * time.Second
	openAIModelTransientCircuitTTL    = 10 * time.Minute
	openAIModelTransientDefaultMax    = 4096
	openAIModelTransientMaxModelBytes = 512
	openAIModelTransientMaxRequestIDs = 64
)

type openAIAccountModelKey struct {
	AccountID int64
	Model     string
}

type openAIAccountModelTransientEntry struct {
	failureStreak         int
	lastFailure           time.Time
	blockUntil            time.Time
	lastTouched           time.Time
	requestIDs            map[string]struct{}
	durableStreak         int
	persisting            bool
	persisted             bool
	persistenceGeneration uint64
	mutationGeneration    uint64
}

type openAIAccountModelTransientDecision struct {
	FailureStreak         int
	Cooldown              time.Duration
	BlockUntil            time.Time
	Counted               bool
	OpenCircuit           bool
	PersistenceGeneration uint64
}

type openAIAccountModelTransientKeyLock struct {
	mu   sync.Mutex
	refs int
}

type openAIAccountModelTransientState struct {
	mu                        sync.Mutex
	entries                   map[openAIAccountModelKey]openAIAccountModelTransientEntry
	keyLocks                  map[openAIAccountModelKey]*openAIAccountModelTransientKeyLock
	maxEntries                int
	nextPersistenceGeneration uint64
	nextMutationGeneration    uint64
}

func newOpenAIAccountModelTransientState(maxEntries int) *openAIAccountModelTransientState {
	if maxEntries <= 0 {
		maxEntries = openAIModelTransientDefaultMax
	}
	return &openAIAccountModelTransientState{
		entries:    make(map[openAIAccountModelKey]openAIAccountModelTransientEntry),
		keyLocks:   make(map[openAIAccountModelKey]*openAIAccountModelTransientKeyLock),
		maxEntries: maxEntries,
	}
}

func (s *openAIAccountModelTransientState) lockKey(key openAIAccountModelKey) func() {
	s.mu.Lock()
	if s.keyLocks == nil {
		s.keyLocks = make(map[openAIAccountModelKey]*openAIAccountModelTransientKeyLock)
	}
	keyLock := s.keyLocks[key]
	if keyLock == nil {
		keyLock = &openAIAccountModelTransientKeyLock{}
		s.keyLocks[key] = keyLock
	}
	keyLock.refs++
	s.mu.Unlock()

	keyLock.mu.Lock()
	return func() {
		keyLock.mu.Unlock()
		s.mu.Lock()
		keyLock.refs--
		if keyLock.refs == 0 {
			delete(s.keyLocks, key)
		}
		if s.maxEntries > 0 && len(s.entries) > s.maxEntries {
			s.evictOldestLocked()
		}
		s.mu.Unlock()
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

func (s *openAIAccountModelTransientState) recordFailure(accountID int64, model string, now time.Time, requestID ...string) openAIAccountModelTransientDecision {
	key, ok := openAIAccountModelTransientKey(accountID, model)
	if s == nil || !ok {
		return openAIAccountModelTransientDecision{}
	}
	if now.IsZero() {
		now = time.Now()
	}

	unlockKey := s.lockKey(key)
	defer unlockKey()
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.entries == nil {
		s.entries = make(map[openAIAccountModelKey]openAIAccountModelTransientEntry)
	}
	if s.maxEntries <= 0 {
		s.maxEntries = openAIModelTransientDefaultMax
	}

	entry, exists := s.entries[key]
	if !exists {
		s.evictOldestLocked()
	}
	if !exists || entry.lastFailure.IsZero() || now.Sub(entry.lastFailure) > openAIModelTransientFailureWindow || now.Before(entry.lastFailure) {
		entry.failureStreak = 0
		entry.blockUntil = time.Time{}
		entry.requestIDs = nil
		entry.durableStreak = 0
		entry.persisting = false
		entry.persisted = false
		entry.persistenceGeneration = 0
	}
	currentRequestID := ""
	openCircuit := false
	if len(requestID) > 0 {
		currentRequestID = strings.TrimSpace(requestID[0])
	}
	if currentRequestID != "" {
		if _, duplicate := entry.requestIDs[currentRequestID]; duplicate {
			return openAIAccountModelTransientDecision{
				FailureStreak: entry.failureStreak,
				BlockUntil:    entry.blockUntil,
			}
		}
		if entry.requestIDs == nil {
			entry.requestIDs = make(map[string]struct{})
		}
		if len(entry.requestIDs) >= openAIModelTransientMaxRequestIDs {
			for oldestID := range entry.requestIDs {
				delete(entry.requestIDs, oldestID)
				break
			}
		}
		entry.requestIDs[currentRequestID] = struct{}{}
		if !entry.persisted && !entry.persisting {
			entry.durableStreak++
			if entry.durableStreak >= 3 {
				s.nextPersistenceGeneration++
				entry.persisting = true
				entry.persistenceGeneration = s.nextPersistenceGeneration
				openCircuit = true
			}
		}
	}
	entry.failureStreak++
	s.nextMutationGeneration++
	entry.mutationGeneration = s.nextMutationGeneration
	entry.lastFailure = now
	entry.lastTouched = now

	cooldown := time.Duration(0)
	switch {
	case entry.failureStreak >= 3:
		cooldown = openAIModelTransientLongCooldown
	case entry.failureStreak == 2:
		cooldown = openAIModelTransientShortCooldown
	}
	if cooldown > 0 {
		entry.blockUntil = now.Add(cooldown)
	} else {
		entry.blockUntil = time.Time{}
	}
	s.entries[key] = entry
	return openAIAccountModelTransientDecision{
		FailureStreak:         entry.failureStreak,
		Cooldown:              cooldown,
		BlockUntil:            entry.blockUntil,
		Counted:               true,
		OpenCircuit:           openCircuit,
		PersistenceGeneration: entry.persistenceGeneration,
	}
}

func (s *openAIAccountModelTransientState) mutationGeneration(accountID int64, model string) uint64 {
	key, ok := openAIAccountModelTransientKey(accountID, model)
	if s == nil || !ok {
		return 0
	}
	unlockKey := s.lockKey(key)
	defer unlockKey()
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.entries[key].mutationGeneration
}

func (s *openAIAccountModelTransientState) mutationGenerations(accountID int64) map[string]uint64 {
	if s == nil || accountID <= 0 {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	result := make(map[string]uint64)
	for key, entry := range s.entries {
		if key.AccountID == accountID {
			result[key.Model] = entry.mutationGeneration
		}
	}
	return result
}

func (s *openAIAccountModelTransientState) clearCircuitIfGeneration(
	accountID int64,
	model string,
	observedGeneration uint64,
	clear func() (bool, error),
) (bool, error) {
	key, ok := openAIAccountModelTransientKey(accountID, model)
	if s == nil || !ok {
		return clear()
	}
	unlockKey := s.lockKey(key)
	defer unlockKey()
	s.mu.Lock()
	if entry, exists := s.entries[key]; exists && entry.mutationGeneration != observedGeneration {
		s.mu.Unlock()
		return false, nil
	}
	s.mu.Unlock()
	cleared, err := clear()
	if err == nil && cleared {
		s.mu.Lock()
		delete(s.entries, key)
		s.mu.Unlock()
	}
	return cleared, err
}

func (s *openAIAccountModelTransientState) finishCircuitPersistence(accountID int64, model string, generation uint64, persisted bool) {
	key, ok := openAIAccountModelTransientKey(accountID, model)
	if s == nil || !ok {
		return
	}
	unlockKey := s.lockKey(key)
	defer unlockKey()
	s.mu.Lock()
	defer s.mu.Unlock()
	entry, exists := s.entries[key]
	if !exists || !entry.persisting || entry.persistenceGeneration != generation {
		return
	}
	entry.persisting = false
	entry.persisted = persisted
	s.entries[key] = entry
}

func (s *openAIAccountModelTransientState) recordSuccess(accountID int64, model string) {
	key, ok := openAIAccountModelTransientKey(accountID, model)
	if s == nil || !ok {
		return
	}
	unlockKey := s.lockKey(key)
	defer unlockKey()
	s.mu.Lock()
	delete(s.entries, key)
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

	unlockKey := s.lockKey(key)
	defer unlockKey()
	s.mu.Lock()
	defer s.mu.Unlock()
	entry, exists := s.entries[key]
	if !exists {
		return false
	}
	if !entry.lastFailure.IsZero() && now.Sub(entry.lastFailure) > openAIModelTransientFailureWindow {
		delete(s.entries, key)
		return false
	}
	entry.lastTouched = now
	s.entries[key] = entry
	return !entry.blockUntil.IsZero() && now.Before(entry.blockUntil)
}

func (s *openAIAccountModelTransientState) size() int {
	if s == nil {
		return 0
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.entries)
}

func (s *openAIAccountModelTransientState) evictOldestLocked() {
	if len(s.entries) < s.maxEntries {
		return
	}
	var oldestKey openAIAccountModelKey
	var oldestTime time.Time
	found := false
	for key, entry := range s.entries {
		if keyLock := s.keyLocks[key]; keyLock != nil && keyLock.refs > 0 {
			continue
		}
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
