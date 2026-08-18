package service

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func longContextAuthTestAPIKey(enabled bool) *APIKey {
	groupID := int64(5)
	return &APIKey{
		ID:      13,
		UserID:  9,
		GroupID: &groupID,
		Name:    "long-context-auth-roundtrip",
		Status:  StatusActive,
		User: &User{
			ID:          9,
			Email:       "long-context@test.local",
			Status:      StatusActive,
			Concurrency: 5,
		},
		Group: &Group{
			ID:                        groupID,
			Name:                      "pro-7",
			Platform:                  PlatformOpenAI,
			Status:                    StatusActive,
			Hydrated:                  true,
			RateMultiplier:            1,
			SubscriptionType:          SubscriptionTypeStandard,
			LongContextPricingEnabled: enabled,
		},
	}
}

func TestAPIKeyAuthSnapshotLongContextPricingRoundtrip(t *testing.T) {
	svc := &APIKeyService{}
	apiKey := longContextAuthTestAPIKey(true)

	snapshot := svc.snapshotFromAPIKey(context.Background(), apiKey)
	require.NotNil(t, snapshot)
	require.Equal(t, apiKeyAuthSnapshotVersion, snapshot.Version)
	require.True(t, snapshot.Group.LongContextPricingEnabled)

	payload, err := json.Marshal(&APIKeyAuthCacheEntry{Snapshot: snapshot})
	require.NoError(t, err)
	var restored APIKeyAuthCacheEntry
	require.NoError(t, json.Unmarshal(payload, &restored))

	materialized, used, err := svc.applyAuthCacheEntry(apiKey.Key, &restored)
	require.NoError(t, err)
	require.True(t, used)
	require.NotNil(t, materialized.Group)
	require.True(t, materialized.Group.LongContextPricingEnabled)

	offKey := longContextAuthTestAPIKey(false)
	offSnapshot := svc.snapshotFromAPIKey(context.Background(), offKey)
	offRestored := svc.snapshotToAPIKey(offKey.Key, offSnapshot)
	require.False(t, offRestored.Group.LongContextPricingEnabled)
}

func TestAPIKeyAuthSnapshotOldVersionWithoutLongContextIsEvicted(t *testing.T) {
	svc := &APIKeyService{}
	snapshot := svc.snapshotFromAPIKey(context.Background(), longContextAuthTestAPIKey(true))
	require.NotNil(t, snapshot)
	snapshot.Version = 19
	snapshot.Group.LongContextPricingEnabled = false

	materialized, used, err := svc.applyAuthCacheEntry("sk-old-longctx", &APIKeyAuthCacheEntry{Snapshot: snapshot})
	require.NoError(t, err)
	require.False(t, used, "缺长上下文字段的旧快照必须淘汰回源，避免零值 false 关掉官方阶梯")
	require.Nil(t, materialized)
}

func TestAPIKeyAuthSnapshotLongContextRoundtripKeepsOfficialLadder(t *testing.T) {
	svc := &APIKeyService{}
	billing := NewBillingService(&config.Config{}, nil)
	resolver := NewModelPricingResolver(nil, billing)
	tokens := UsageTokens{InputTokens: 250000, OutputTokens: 1000}

	materialized := svc.snapshotToAPIKey("sk-longctx", svc.snapshotFromAPIKey(context.Background(), longContextAuthTestAPIKey(true)))
	enabled, err := billing.CalculateCostUnified(CostInput{
		Model: "grok-4.5", Group: materialized.Group, Tokens: tokens, RateMultiplier: 1, Resolver: resolver,
	})
	require.NoError(t, err)
	require.True(t, enabled.LongContextBillingApplied)

	disabledKey := longContextAuthTestAPIKey(false)
	disabled := svc.snapshotToAPIKey("sk-shortctx", svc.snapshotFromAPIKey(context.Background(), disabledKey))
	off, err := billing.CalculateCostUnified(CostInput{
		Model: "grok-4.5", Group: disabled.Group, Tokens: tokens, RateMultiplier: 1, Resolver: resolver,
	})
	require.NoError(t, err)
	require.False(t, off.LongContextBillingApplied)
	require.InDelta(t, off.InputCost*2, enabled.InputCost, 1e-12)
	require.InDelta(t, off.OutputCost*2, enabled.OutputCost, 1e-12)
}

func TestLegacyAuthSnapshotOmittingLongContextFlagDisablesOfficialLadder(t *testing.T) {
	// 复现 0.1.176 起的线上事故：快照 JSON 不含该字段时，热路径 Group 零值为 false。
	legacyJSON := []byte(`{"snapshot":{"version":20,"api_key_id":13,"user_id":9,"group_id":5,"name":"legacy","status":"active","user":{"id":9,"status":"active","role":"","balance":0,"concurrency":0,"email":"","username":"","balance_notify_enabled":false,"balance_notify_threshold_type":"","total_recharged":0,"rpm_limit":0},"group":{"id":5,"name":"pro-7","platform":"openai","is_exclusive":false,"status":"active","subscription_type":"standard","rate_multiplier":1}}}`)

	var entry APIKeyAuthCacheEntry
	require.NoError(t, json.Unmarshal(legacyJSON, &entry))
	require.NotNil(t, entry.Snapshot)
	require.NotNil(t, entry.Snapshot.Group)
	require.False(t, entry.Snapshot.Group.LongContextPricingEnabled)

	svc := &APIKeyService{}
	materialized, used, err := svc.applyAuthCacheEntry("sk-legacy", &entry)
	require.NoError(t, err)
	require.True(t, used)
	require.False(t, materialized.Group.LongContextPricingEnabled)

	billing := NewBillingService(&config.Config{}, nil)
	resolver := NewModelPricingResolver(nil, billing)
	cost, err := billing.CalculateCostUnified(CostInput{
		Model: "grok-4.5", Group: materialized.Group, Tokens: UsageTokens{InputTokens: 250000, OutputTokens: 1000}, RateMultiplier: 1, Resolver: resolver,
	})
	require.NoError(t, err)
	require.False(t, cost.LongContextBillingApplied)
}
