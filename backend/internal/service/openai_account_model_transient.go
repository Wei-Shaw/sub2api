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
	openAIModelTransientDefaultMax    = 4096
	openAIModelTransientMaxModelBytes = 512
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
}

type openAIAccountModelTransientDecision struct {
	FailureStreak int
	Cooldown      time.Duration
	BlockUntil    time.Time
}

type openAIAccountModelTransientState struct {
	mu         sync.Mutex
	entries    map[openAIAccountModelKey]openAIAccountModelTransientEntry
	maxEntries int
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
	if !exists {
		s.evictOldestLocked()
	}
	if !exists || entry.lastFailure.IsZero() || now.Sub(entry.lastFailure) > openAIModelTransientFailureWindow || now.Before(entry.lastFailure) {
		entry.failureStreak = 0
		entry.blockUntil = time.Time{}
	}
	entry.failureStreak++
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
