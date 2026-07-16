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
REDACTED

type openAIAccountModelTransientEntry struct {
	failureStreak int
	lastFailure   time.Time
	blockUntil    time.Time
	lastTouched   time.Time
REDACTED

type openAIAccountModelTransientDecision struct {
	FailureStreak int
	Cooldown      time.Duration
	BlockUntil    time.Time
REDACTED

type openAIAccountModelTransientState struct {
	mu         sync.Mutex
	entries    map[openAIAccountModelKey]openAIAccountModelTransientEntry
	maxEntries int
REDACTED

func newOpenAIAccountModelTransientState(maxEntries int) *openAIAccountModelTransientState {
	if maxEntries <= 0 {
		maxEntries = openAIModelTransientDefaultMax
REDACTED
	return &openAIAccountModelTransientState{
		entries:    make(map[openAIAccountModelKey]openAIAccountModelTransientEntry),
		maxEntries: maxEntries,
REDACTED
REDACTED

func normalizeOpenAIAccountModelTransientModel(model string) string {
	model = strings.TrimSpace(model)
	if len(model) > openAIModelTransientMaxModelBytes {
		return ""
REDACTED
	return strings.ToLower(model)
REDACTED

func openAIAccountModelTransientKey(accountID int64, model string) (openAIAccountModelKey, bool) {
	model = normalizeOpenAIAccountModelTransientModel(model)
	if accountID <= 0 || model == "" {
		return openAIAccountModelKey{REDACTED, false
REDACTED
	return openAIAccountModelKey{AccountID: accountID, Model: modelREDACTED, true
REDACTED

func (s *openAIAccountModelTransientState) recordFailure(accountID int64, model string, now time.Time) openAIAccountModelTransientDecision {
	key, ok := openAIAccountModelTransientKey(accountID, model)
	if s == nil || !ok {
		return openAIAccountModelTransientDecision{REDACTED
REDACTED
	if now.IsZero() {
		now = time.Now()
REDACTED

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.entries == nil {
		s.entries = make(map[openAIAccountModelKey]openAIAccountModelTransientEntry)
REDACTED
	if s.maxEntries <= 0 {
		s.maxEntries = openAIModelTransientDefaultMax
REDACTED

	entry, exists := s.entries[key]
	if !exists {
		s.evictOldestLocked()
REDACTED
	if !exists || entry.lastFailure.IsZero() || now.Sub(entry.lastFailure) > openAIModelTransientFailureWindow || now.Before(entry.lastFailure) {
		entry.failureStreak = 0
		entry.blockUntil = time.Time{REDACTED
REDACTED
	entry.failureStreak++
	entry.lastFailure = now
	entry.lastTouched = now

	cooldown := time.Duration(0)
	switch {
	case entry.failureStreak >= 3:
		cooldown = openAIModelTransientLongCooldown
	case entry.failureStreak == 2:
		cooldown = openAIModelTransientShortCooldown
REDACTED
	if cooldown > 0 {
		entry.blockUntil = now.Add(cooldown)
REDACTED else {
		entry.blockUntil = time.Time{REDACTED
REDACTED
	s.entries[key] = entry
	return openAIAccountModelTransientDecision{
		FailureStreak: entry.failureStreak,
		Cooldown:      cooldown,
		BlockUntil:    entry.blockUntil,
REDACTED
REDACTED

func (s *openAIAccountModelTransientState) recordSuccess(accountID int64, model string) {
	key, ok := openAIAccountModelTransientKey(accountID, model)
	if s == nil || !ok {
		return
REDACTED
	s.mu.Lock()
	delete(s.entries, key)
	s.mu.Unlock()
REDACTED

func (s *openAIAccountModelTransientState) isBlocked(accountID int64, model string, now time.Time) bool {
	key, ok := openAIAccountModelTransientKey(accountID, model)
	if s == nil || !ok {
		return false
REDACTED
	if now.IsZero() {
		now = time.Now()
REDACTED

	s.mu.Lock()
	defer s.mu.Unlock()
	entry, exists := s.entries[key]
	if !exists {
		return false
REDACTED
	if !entry.lastFailure.IsZero() && now.Sub(entry.lastFailure) > openAIModelTransientFailureWindow {
		delete(s.entries, key)
		return false
REDACTED
	entry.lastTouched = now
	s.entries[key] = entry
	return !entry.blockUntil.IsZero() && now.Before(entry.blockUntil)
REDACTED

func (s *openAIAccountModelTransientState) size() int {
	if s == nil {
		return 0
REDACTED
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.entries)
REDACTED

func (s *openAIAccountModelTransientState) evictOldestLocked() {
	if len(s.entries) < s.maxEntries {
		return
REDACTED
	var oldestKey openAIAccountModelKey
	var oldestTime time.Time
	found := false
	for key, entry := range s.entries {
		if !found || entry.lastTouched.Before(oldestTime) {
			oldestKey = key
			oldestTime = entry.lastTouched
			found = true
	REDACTED
REDACTED
	if found {
		delete(s.entries, oldestKey)
REDACTED
REDACTED
