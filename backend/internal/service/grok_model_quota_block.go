package service

import (
	"strconv"
	"strings"
	"sync"
	"time"
)

// Process-local per-account model soft-blocks for Grok free-usage that names a
// model (e.g. "used all free usage for model grok-4.5"). Sibling models on the
// same account stay schedulable. Multi-instance: each process learns from its
// own upstream errors.
type grokModelQuotaBlock struct {
	Until time.Time
REDACTED

type grokModelQuotaBlockStore struct {
	mu    sync.Mutex
	items map[string]grokModelQuotaBlock // key: accountID|model
REDACTED

var globalGrokModelQuotaBlocks = &grokModelQuotaBlockStore{
	items: make(map[string]grokModelQuotaBlock),
REDACTED

const (
	grokModelQuotaBlockDefaultTTL = 2 * time.Hour
	grokModelQuotaBlockMaxTTL     = 6 * time.Hour
	grokModelQuotaBlockMinTTL     = 20 * time.Minute
)

func grokModelQuotaBlockKey(accountID int64, model string) string {
	return strings.TrimSpace(strings.ToLower(model)) + "|" + strconv.FormatInt(accountID, 10)
REDACTED

// markGrokModelQuotaBlock soft-blocks accountID for model until the given time.
func markGrokModelQuotaBlock(accountID int64, model string, until time.Time) {
	model = strings.TrimSpace(model)
	if accountID <= 0 || model == "" || until.IsZero() {
		return
REDACTED
	now := time.Now()
	if !until.After(now.Add(grokModelQuotaBlockMinTTL)) {
		until = now.Add(grokModelQuotaBlockDefaultTTL)
REDACTED
	if max := now.Add(grokModelQuotaBlockMaxTTL); until.After(max) {
		until = max
REDACTED
	key := grokModelQuotaBlockKey(accountID, model)
	globalGrokModelQuotaBlocks.mu.Lock()
	defer globalGrokModelQuotaBlocks.mu.Unlock()
	if cur, ok := globalGrokModelQuotaBlocks.items[key]; ok && cur.Until.After(until) {
		return
REDACTED
	globalGrokModelQuotaBlocks.items[key] = grokModelQuotaBlock{Until: untilREDACTED
	for k, v := range globalGrokModelQuotaBlocks.items {
		if !v.Until.After(now) {
			delete(globalGrokModelQuotaBlocks.items, k)
	REDACTED
REDACTED
REDACTED

// isGrokModelQuotaBlocked reports whether this account cannot serve model now.
func isGrokModelQuotaBlocked(accountID int64, model string, now time.Time) bool {
	model = strings.TrimSpace(model)
	if accountID <= 0 || model == "" {
		return false
REDACTED
	key := grokModelQuotaBlockKey(accountID, model)
	globalGrokModelQuotaBlocks.mu.Lock()
	defer globalGrokModelQuotaBlocks.mu.Unlock()
	cur, ok := globalGrokModelQuotaBlocks.items[key]
	if !ok {
		return false
REDACTED
	if !cur.Until.After(now) {
		delete(globalGrokModelQuotaBlocks.items, key)
		return false
REDACTED
	return true
REDACTED

func filterGrokModelQuotaBlockedAccounts(accounts []Account, model string, now time.Time) []Account {
	if len(accounts) == 0 || strings.TrimSpace(model) == "" {
		return accounts
REDACTED
	out := make([]Account, 0, len(accounts))
	for i := range accounts {
		upstreamModel := canonicalOpenAIAccountSchedulingModel(&accounts[i], model)
		if isGrokModelQuotaBlocked(accounts[i].ID, upstreamModel, now) {
			continue
	REDACTED
		out = append(out, accounts[i])
REDACTED
	return out
REDACTED

// isGrokModelSpecificFreeUsage is true when free-usage exhaustion is scoped to
// a named model (account may still serve other models).
func isGrokModelSpecificFreeUsage(low, model string) bool {
	model = strings.ToLower(strings.TrimSpace(model))
	if model == "" || low == "" {
		return false
REDACTED
	if strings.Contains(low, "for model") || strings.Contains(low, "模型") {
		return true
REDACTED
	// "used all the included free usage for model grok-4.5"
	if strings.Contains(low, "free usage") && strings.Contains(low, model) {
		return true
REDACTED
	return false
REDACTED
