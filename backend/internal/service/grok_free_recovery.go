package service

import (
	"strings"
	"sync"
	"time"
)

// Lightweight free-usage recovery hint: after free cool ends, keep a short
// "probe preferred" window so the next scheduling pass can prefer accounts that
// have waited out free-usage without immediately re-entering a tight 429 loop.
// Full async quota recovery queues (grok2api) are intentionally not ported.
type grokFreeRecoveryHint struct {
	CoolUntil  time.Time
	ProbeUntil time.Time
REDACTED

type grokFreeRecoveryStore struct {
	mu    sync.Mutex
	items map[int64]grokFreeRecoveryHint
REDACTED

var globalGrokFreeRecovery = &grokFreeRecoveryStore{
	items: make(map[int64]grokFreeRecoveryHint),
REDACTED

const grokFreeRecoveryProbeWindow = 30 * time.Minute

// markGrokFreeUsageRecovery records that account left free-usage cool and should
// be treated carefully until ProbeUntil.
func markGrokFreeUsageRecovery(accountID int64, coolUntil time.Time) {
	if accountID <= 0 {
		return
REDACTED
	now := time.Now()
	if coolUntil.IsZero() || !coolUntil.After(now) {
		coolUntil = now.Add(2 * time.Hour)
REDACTED
	globalGrokFreeRecovery.mu.Lock()
	defer globalGrokFreeRecovery.mu.Unlock()
	globalGrokFreeRecovery.items[accountID] = grokFreeRecoveryHint{
		CoolUntil:  coolUntil,
		ProbeUntil: coolUntil.Add(grokFreeRecoveryProbeWindow),
REDACTED
REDACTED

// isGrokFreeUsageCooling is true while still inside the free-usage cool window.
func isGrokFreeUsageCooling(accountID int64, now time.Time) bool {
	if accountID <= 0 {
		return false
REDACTED
	globalGrokFreeRecovery.mu.Lock()
	defer globalGrokFreeRecovery.mu.Unlock()
	h, ok := globalGrokFreeRecovery.items[accountID]
	if !ok {
		return false
REDACTED
	if now.After(h.ProbeUntil) {
		delete(globalGrokFreeRecovery.items, accountID)
		return false
REDACTED
	return now.Before(h.CoolUntil)
REDACTED

// isGrokFreeUsageProbePreferred is true after cool ends but before probe window
// expires — account is schedulable but may still be near free-usage edges.
func isGrokFreeUsageProbePreferred(accountID int64, now time.Time) bool {
	if accountID <= 0 {
		return false
REDACTED
	globalGrokFreeRecovery.mu.Lock()
	defer globalGrokFreeRecovery.mu.Unlock()
	h, ok := globalGrokFreeRecovery.items[accountID]
	if !ok {
		return false
REDACTED
	if now.After(h.ProbeUntil) {
		delete(globalGrokFreeRecovery.items, accountID)
		return false
REDACTED
	return !now.Before(h.CoolUntil) && now.Before(h.ProbeUntil)
REDACTED

// reasonIsGrokFreeUsage detects free-usage cool reasons written by our handlers.
func reasonIsGrokFreeUsage(reason string) bool {
	return strings.Contains(strings.ToLower(reason), "free usage")
REDACTED
