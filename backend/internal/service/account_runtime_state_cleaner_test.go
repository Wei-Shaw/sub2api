package service

import (
	"testing"
	"time"
)

func TestCompositeAccountRuntimeStateCleanerDeletesAllAccountState(t *testing.T) {
	const accountID int64 = 77
	usageCache := NewUsageCache()
	usageCache.apiCache.Store(accountID, &apiUsageCache{timestamp: time.Now()})
	stats := newOpenAIAccountRuntimeStats()
	stats.loadOrCreate(accountID)
	openAIGateway := &OpenAIGatewayService{
		openaiAccountStats:    stats,
		codexSnapshotThrottle: newAccountWriteThrottle(time.Minute),
	}
	openAIGateway.openaiWSFallbackUntil.Store(accountID, time.Now().Add(time.Minute))
	openAIGateway.openaiAccountRuntimeBlockUntil.Store(accountID, time.Now().Add(time.Minute))
	openAIGateway.openaiCompatSessionResponses.Store("77\x001\x00session", openAICompatSessionResponseBinding{ExpiresAt: time.Now().Add(time.Minute)})
	openAIGateway.openaiCompatAnthropicDigestSessions.Store("77|1|digest", openAICompatAnthropicDigestBinding{ExpiresAt: time.Now().Add(time.Minute)})
	openAIGateway.codexSnapshotThrottle.Allow(accountID, time.Now())
	antigravity := &AntigravityTokenProvider{}
	antigravity.markBackfillAttempted(accountID)
	rateLimit := &RateLimitService{usageCache: map[int64]*geminiUsageCacheEntry{accountID: {cachedAt: time.Now()}}}
	cleaner := ProvideAccountRuntimeStateCleaner(usageCache, openAIGateway, antigravity, rateLimit)

	cleaner.DeleteAccountRuntimeState(accountID)
	if _, ok := usageCache.apiCache.Load(accountID); ok {
		t.Fatal("usage cache retained deleted account")
	}
	if stats.size() != 0 {
		t.Fatal("scheduler stats retained deleted account")
	}
	if _, ok := openAIGateway.openaiWSFallbackUntil.Load(accountID); ok {
		t.Fatal("WS fallback state retained deleted account")
	}
	if _, ok := openAIGateway.openaiAccountRuntimeBlockUntil.Load(accountID); ok {
		t.Fatal("runtime block state retained deleted account")
	}
	if openAIGateway.openaiCompatSessionResponses.Len() != 0 || openAIGateway.openaiCompatAnthropicDigestSessions.Len() != 0 {
		t.Fatal("compatibility session state retained deleted account")
	}
	if _, ok := antigravity.backfillCooldown.Load(accountID); ok {
		t.Fatal("Antigravity cooldown retained deleted account")
	}
	if _, ok := rateLimit.usageCache[accountID]; ok {
		t.Fatal("rate-limit usage cache retained deleted account")
	}
}

func TestRateLimitGeminiUsageCacheBoundsAndExpiresEntries(t *testing.T) {
	service := &RateLimitService{usageCache: make(map[int64]*geminiUsageCacheEntry)}
	now := time.Now()
	for accountID := int64(1); accountID <= geminiUsageCacheMaxEntries+1; accountID++ {
		service.setGeminiUsageTotals(accountID, now, now, GeminiUsageTotals{})
	}
	if got := len(service.usageCache); got > geminiUsageCacheMaxEntries {
		t.Fatalf("expected at most %d entries, got %d", geminiUsageCacheMaxEntries, got)
	}

	service.usageCache[999999] = &geminiUsageCacheEntry{windowStart: now, cachedAt: now.Add(-geminiPrecheckCacheTTL)}
	if _, ok := service.getGeminiUsageTotals(999999, now, now); ok {
		t.Fatal("expired Gemini usage cache unexpectedly hit")
	}
	if _, retained := service.usageCache[999999]; retained {
		t.Fatal("expired Gemini usage cache entry was retained")
	}
}
