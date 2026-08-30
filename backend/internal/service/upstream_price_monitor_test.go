package service

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/stretchr/testify/require"
)

func TestDefaultUpstreamPriceMonitorConfigIsDisabledObserveOnly(t *testing.T) {
	cfg := domain.DefaultUpstreamPriceMonitorConfig()
	require.False(t, cfg.Enabled)
	require.Equal(t, domain.UpstreamPriceMonitorModeObserve, cfg.Mode)
	require.Equal(t, 15, cfg.IntervalMinutes)
	require.InDelta(t, 1.20, cfg.Markup, 1e-12)
	require.Len(t, cfg.DomesticModels, 18)
	require.NoError(t, normalizeAndValidateUpstreamPriceMonitorConfig(&cfg))
}

func TestNormalizeUpstreamPriceMonitorConfigAcceptsExplicitlyManagedNewModel(t *testing.T) {
	cfg := domain.DefaultUpstreamPriceMonitorConfig()
	cfg.DomesticModels = []string{"new-domestic/model-v1"}
	require.NoError(t, normalizeAndValidateUpstreamPriceMonitorConfig(&cfg))
	require.Equal(t, []string{"new-domestic/model-v1"}, cfg.DomesticModels)

	cfg.DomesticModels = []string{"invalid model"}
	require.ErrorIs(t, normalizeAndValidateUpstreamPriceMonitorConfig(&cfg), ErrUpstreamPriceMonitorInvalidConfig)
}

func TestNormalizeUpstreamPriceMonitorConfigPreservesExplicitEmptyScope(t *testing.T) {
	cfg := domain.DefaultUpstreamPriceMonitorConfig()
	cfg.DomesticModels = []string{}
	require.NoError(t, normalizeAndValidateUpstreamPriceMonitorConfig(&cfg))
	require.Empty(t, cfg.DomesticModels)
}

func TestUpstreamPriceModelDiscoveryClassifiesAndIntersectsPerAccount(t *testing.T) {
	require.True(t, isLikelyDomesticUpstreamModel("deepseek-new-v1"))
	require.False(t, isLikelyDomesticUpstreamModel("gpt-foreign-v1"))
	managed := []string{"deepseek-new-v1", "MiniMax-M3"}
	require.Equal(t, []string{"deepseek-new-v1"}, intersectUpstreamPriceModels(managed, map[string]struct{}{"deepseek-new-v1": {}}))
	require.Equal(t, managed, intersectUpstreamPriceModels(managed, nil))
}

func TestUpstreamPriceAccountingMatchIgnoresLocalCost(t *testing.T) {
	remote := domain.UpstreamPriceUsageCounters{Requests: 2, InputTokens: 100, OutputTokens: 20, CacheReadTokens: 40, ActualCost: 0.2}
	local := remote
	local.ActualCost = 99
	require.True(t, equalUpstreamPriceAccountingCounters(remote, local))
	local.CacheReadTokens++
	require.False(t, equalUpstreamPriceAccountingCounters(remote, local))
}

func TestSolveUpstreamTokenPricesRequiresFullRankAndFindsCacheRead(t *testing.T) {
	rows := []domain.UpstreamPriceObservation{
		{InputTokens: 1_000_000, OutputTokens: 100_000, ActualCost: 0.2 + 0.03},
		{InputTokens: 100_000, OutputTokens: 1_000_000, ActualCost: 0.02 + 0.3},
		{InputTokens: 200_000, OutputTokens: 100_000, CacheReadTokens: 1_000_000, ActualCost: 0.04 + 0.03 + 0.05},
	}
	prices, count, err := SolveUpstreamTokenPrices(rows)
	require.NoError(t, err)
	require.Equal(t, 3, count)
	require.InDelta(t, 0.2, *prices.InputPerMillion, 1e-9)
	require.InDelta(t, 0.3, *prices.OutputPerMillion, 1e-9)
	require.InDelta(t, 0.05, *prices.CacheReadPerMillion, 1e-9)
	require.Nil(t, prices.CacheWritePerMillion)

	_, _, err = SolveUpstreamTokenPrices(rows[:1])
	require.ErrorIs(t, err, ErrUpstreamPriceInsufficientRank)
}

func TestSolveUpstreamTokenPricesRejectsIllConditionedTrafficWindows(t *testing.T) {
	rows := []domain.UpstreamPriceObservation{
		{InputTokens: 1_000_000, OutputTokens: 1_000_000, ActualCost: 0.5},
		{InputTokens: 1_000_000, OutputTokens: 1_000_001, ActualCost: 0.5000003},
	}
	_, _, err := SolveUpstreamTokenPrices(rows)
	require.ErrorIs(t, err, ErrUpstreamPriceInsufficientRank)
}

func TestParseX5M5XUsageModelStats(t *testing.T) {
	body := []byte(`{"model_stats":[{"model":"MiniMax-M3","requests":2,"input_tokens":100,"output_tokens":20,"cache_creation_tokens":0,"cache_read_tokens":80,"actual_cost":0.000042}]}`)
	models, err := parseX5M5XUsageModelStats(body)
	require.NoError(t, err)
	require.Equal(t, domain.UpstreamPriceUsageCounters{
		Requests: 2, InputTokens: 100, OutputTokens: 20, CacheReadTokens: 80, ActualCost: 0.000042,
	}, models["minimax-m3"])
}

func TestParseX5M5XModelListRejectsPartialOrDuplicateSnapshots(t *testing.T) {
	models, err := parseX5M5XModelList([]byte(`{"data":[{"id":"MiniMax-M3"},{"id":"new-domestic-v1"}]}`))
	require.NoError(t, err)
	require.Equal(t, []string{"MiniMax-M3", "new-domestic-v1"}, models)

	_, err = parseX5M5XModelList([]byte(`{"data":[{"id":"MiniMax-M3"},{"id":"minimax-m3"}]}`))
	require.Error(t, err)
	_, err = parseX5M5XModelList([]byte(`{"data":[{}]}`))
	require.Error(t, err)
}

func TestUsageCheckpointJSONNeverContainsCredential(t *testing.T) {
	checkpoint := domain.UpstreamPriceUsageCheckpoint{AccountIdentityHash: "hash-only", LedgerDate: "2026-08-30"}
	raw, err := json.Marshal(checkpoint)
	require.NoError(t, err)
	require.NotContains(t, string(raw), "api_key")
	require.NotContains(t, string(raw), "authorization")
}

func TestUpstreamPriceBillingContextIgnoresObservationTimestamp(t *testing.T) {
	first := &domain.UpstreamPriceBillingSnapshot{
		ResolvedRateMultiplier: 0.5, EffectiveRateMultiplier: 0.6, AppliedPeakMultiplier: 1.2,
		ObservedAt: time.Date(2026, 8, 30, 1, 0, 0, 0, time.UTC),
	}
	second := *first
	second.ObservedAt = first.ObservedAt.Add(15 * time.Minute)

	firstHash, firstKey := upstreamPriceBillingContext(first)
	secondHash, secondKey := upstreamPriceBillingContext(&second)
	require.Equal(t, firstHash, secondHash)
	require.Equal(t, firstKey, secondKey)

	second.EffectiveRateMultiplier = 0.7
	changedHash, changedKey := upstreamPriceBillingContext(&second)
	require.NotEqual(t, firstHash, changedHash)
	require.NotEqual(t, firstKey, changedKey)
}

func TestUpstreamPriceRunApplyScopeReadsPersistedJSONSummary(t *testing.T) {
	run := &domain.UpstreamPriceMonitorRun{Summary: map[string]any{
		"account_ids":                 []any{float64(7), float64(8)},
		"account_ledger_hashes":       map[string]any{"7": "hash-7", "8": "hash-8"},
		"account_identity_hashes":     map[string]any{"7": "identity-7", "8": "identity-8"},
		"channel_ids":                 []any{float64(9), float64(3), float64(9)},
		"display_multiplier_decimals": float64(3),
		"snapshot_max_age_minutes":    float64(60),
		"config_updated_at":           "2026-08-30T01:02:03Z",
		"model_catalog_revision":      float64(12),
	}}
	scope, ok := ReadUpstreamPriceRunApplyScope(run)
	require.True(t, ok)
	require.Equal(t, []int64{7, 8}, scope.AccountIDs)
	require.Equal(t, map[int64]string{7: "hash-7", 8: "hash-8"}, scope.AccountLedgerHashes)
	require.Equal(t, map[int64]string{7: "identity-7", 8: "identity-8"}, scope.AccountIdentityHashes)
	require.Equal(t, []int64{3, 9}, scope.ChannelIDs)
	require.Equal(t, 3, scope.DisplayMultiplierDecimals)
	require.Equal(t, 60, scope.MaxAgeMinutes)
	require.Equal(t, time.Date(2026, 8, 30, 1, 2, 3, 0, time.UTC), scope.ConfigUpdatedAt)
	require.Equal(t, int64(12), scope.ModelCatalogRevision)
}

func TestUpstreamPriceCredentialLedgerHashExcludesLocalAccountID(t *testing.T) {
	first := &Account{ID: 1, Type: AccountTypeAPIKey, Credentials: map[string]any{
		"api_key": "test-key-not-real", "base_url": "https://us-api.example.invalid",
	}}
	second := &Account{ID: 2, Type: AccountTypeAPIKey, Credentials: map[string]any{
		"api_key": "test-key-not-real", "base_url": "https://us-api.example.invalid",
	}}
	require.Equal(t, UpstreamPriceCredentialLedgerHash(first), UpstreamPriceCredentialLedgerHash(second))
	require.NotEqual(t, UpstreamPriceAccountIdentityHash(first), UpstreamPriceAccountIdentityHash(second))
}

func TestMissingUpstreamPriceProbeModelsPrefersTrustedNaturalEvidence(t *testing.T) {
	models := []string{"MiniMax-M3", "glm-5.3"}
	input, output, cacheWrite, cacheRead := 0.2, 0.8, 0.4, 0.05
	evidence := []domain.UpstreamPriceEvidence{
		{AccountID: 7, ModelName: "minimax-m3", BillingMode: DisplayBillingModeToken, Status: domain.UpstreamPriceEvidenceStatusTrusted,
			Prices: domain.UpstreamPriceVector{InputPerMillion: &input, OutputPerMillion: &output, CacheWritePerMillion: &cacheWrite, CacheReadPerMillion: &cacheRead}},
		{AccountID: 7, ModelName: "glm-5.3", BillingMode: DisplayBillingModeToken, Status: domain.UpstreamPriceEvidenceStatusPending},
	}
	require.Equal(t, []string{"glm-5.3"}, missingUpstreamPriceProbeModels(evidence, 7, models))
}

func TestUpstreamPriceActiveProbeSpecsReuseOnlyTheCachePrefix(t *testing.T) {
	specs := upstreamPriceActiveProbeSpecs("MiniMax-M3")
	require.Len(t, specs, 7)
	require.NotEqual(t, specs[0].SessionID, specs[1].SessionID)
	require.Equal(t, specs[3].SessionID, specs[4].SessionID)
	require.Equal(t, specs[3].SystemPrompt, specs[4].SystemPrompt)
	require.NotEmpty(t, specs[3].SystemPrompt)
	require.True(t, specs[5].ExplicitCache)
	require.Equal(t, specs[5].SessionID, specs[6].SessionID)
}

func TestActiveProbeRequiresAnExclusiveConcurrencyLeaseBackend(t *testing.T) {
	svc := &UpstreamPriceMonitorService{}
	release, acquired, err := svc.acquireUpstreamPriceProbeSlot(context.Background(), &Account{ID: 7, Concurrency: 1})
	require.NoError(t, err)
	require.False(t, acquired)
	require.NotPanics(t, release)
}
