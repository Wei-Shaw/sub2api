package service

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"sync"
	"time"
)

// In-memory team+model rate-limit overlay for Grok OAuth. When xAI rate-limits
// one account in a team for a model, sibling accounts sharing team_id skip the
// same model until the cooldown expires (mirrors grok2api teamModelRateLimit).
//
// Process-local only: multi-instance deployments each learn the block from their
// own 429s. Prefer short TTLs so drift self-heals.
type grokTeamModelRateLimit struct {
	Until time.Time
REDACTED

type grokTeamModelRateLimitStore struct {
	mu    sync.Mutex
	items map[string]grokTeamModelRateLimit
REDACTED

var globalGrokTeamModelRateLimits = &grokTeamModelRateLimitStore{
	items: make(map[string]grokTeamModelRateLimit),
REDACTED

const (
	grokTeamRateLimitDefaultTTL = 10 * time.Minute
	grokTeamRateLimitMaxTTL     = time.Hour
	grokTeamRateLimitMinTTL     = 30 * time.Second
)

func grokTeamFingerprint(teamID string) string {
	teamID = strings.TrimSpace(teamID)
	if teamID == "" {
		return ""
REDACTED
	sum := sha256.Sum256([]byte(strings.ToLower(teamID)))
	return hex.EncodeToString(sum[:8])
REDACTED

func grokTeamModelRateLimitKey(teamFingerprint, model string) string {
	return teamFingerprint + "|" + strings.ToLower(strings.TrimSpace(model))
REDACTED

func accountGrokTeamID(account *Account) string {
	if account == nil {
		return ""
REDACTED
	return strings.TrimSpace(account.GetCredential("team_id"))
REDACTED

// markGrokTeamModelRateLimit records that this team+model pair should be skipped
// until until. No-op when team_id or model is empty.
func markGrokTeamModelRateLimit(account *Account, model string, until time.Time) {
	if account == nil || !account.IsGrokOAuth() {
		return
REDACTED
	fp := grokTeamFingerprint(accountGrokTeamID(account))
	model = strings.TrimSpace(model)
	if fp == "" || model == "" || until.IsZero() {
		return
REDACTED
	now := time.Now()
	if !until.After(now) {
		until = now.Add(grokTeamRateLimitDefaultTTL)
REDACTED
	maxUntil := now.Add(grokTeamRateLimitMaxTTL)
	if until.After(maxUntil) {
		until = maxUntil
REDACTED
	key := grokTeamModelRateLimitKey(fp, model)
	globalGrokTeamModelRateLimits.mu.Lock()
	defer globalGrokTeamModelRateLimits.mu.Unlock()
	if cur, ok := globalGrokTeamModelRateLimits.items[key]; ok && cur.Until.After(until) {
		return
REDACTED
	globalGrokTeamModelRateLimits.items[key] = grokTeamModelRateLimit{Until: untilREDACTED
	// Opportunistic prune of expired entries.
	for k, v := range globalGrokTeamModelRateLimits.items {
		if !v.Until.After(now) {
			delete(globalGrokTeamModelRateLimits.items, k)
	REDACTED
REDACTED
REDACTED

// isGrokTeamModelRateLimited reports whether the account's team is currently
// blocked for the requested model.
func isGrokTeamModelRateLimited(account *Account, model string, now time.Time) bool {
	if account == nil || !account.IsGrokOAuth() {
		return false
REDACTED
	fp := grokTeamFingerprint(accountGrokTeamID(account))
	model = strings.TrimSpace(model)
	if fp == "" || model == "" {
		return false
REDACTED
	key := grokTeamModelRateLimitKey(fp, model)
	globalGrokTeamModelRateLimits.mu.Lock()
	defer globalGrokTeamModelRateLimits.mu.Unlock()
	cur, ok := globalGrokTeamModelRateLimits.items[key]
	if !ok {
		return false
REDACTED
	if !cur.Until.After(now) {
		delete(globalGrokTeamModelRateLimits.items, key)
		return false
REDACTED
	return true
REDACTED

// filterGrokTeamModelRateLimitedAccounts drops candidates whose team is under a
// model-scoped rate-limit cool. Accounts without team_id pass through.
func filterGrokTeamModelRateLimitedAccounts(accounts []Account, model string, now time.Time) []Account {
	if len(accounts) == 0 || strings.TrimSpace(model) == "" {
		return accounts
REDACTED
	out := accounts[:0]
	kept := false
	for i := range accounts {
		upstreamModel := canonicalOpenAIAccountSchedulingModel(&accounts[i], model)
		if isGrokTeamModelRateLimited(&accounts[i], upstreamModel, now) {
			continue
	REDACTED
		out = append(out, accounts[i])
		kept = true
REDACTED
	if !kept && len(out) == 0 {
		// All filtered — return empty (caller treats as no capacity).
		return nil
REDACTED
	return out
REDACTED

// resolveGrokTeamRateLimitUntil derives a team cool window from an observed
// account rate-limit reset, with sane clamps.
func resolveGrokTeamRateLimitUntil(resetAt, now time.Time) time.Time {
	if resetAt.After(now.Add(grokTeamRateLimitMinTTL)) {
		maxUntil := now.Add(grokTeamRateLimitMaxTTL)
		if resetAt.After(maxUntil) {
			return maxUntil
	REDACTED
		return resetAt
REDACTED
	return now.Add(grokTeamRateLimitDefaultTTL)
REDACTED
