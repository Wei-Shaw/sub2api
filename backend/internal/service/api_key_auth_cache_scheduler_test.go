package service

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/stretchr/testify/require"
)

func TestAPIKeyAuthSnapshotSchedulerRoundtrip(t *testing.T) {
	for _, cfg := range []GroupSchedulerConfig{{}, {Strategy: "high_availability", SelectionMode: "strict_health", FirstByteFailover: true, StickyBindingMode: "rebind_on_failover", ProbeBypassSticky: true, MaxAccountSwitches: 5, SameAccountRetryAttempts: 2}} {
		t.Run(cfg.Strategy, func(t *testing.T) {
			svc := &APIKeyService{}
			key := profitAuthTestAPIKey()
			key.Group.Scheduler = cfg
			key.Group.ForceOpenAIUltrafast = true
			data, err := json.Marshal(&APIKeyAuthCacheEntry{Snapshot: svc.snapshotFromAPIKey(context.Background(), key)})
			require.NoError(t, err)
			var entry APIKeyAuthCacheEntry
			require.NoError(t, json.Unmarshal(data, &entry))
			got, used, err := svc.applyAuthCacheEntry(key.Key, &entry)
			require.NoError(t, err)
			require.True(t, used)
			require.NotNil(t, got.Group)
			require.Equal(t, cfg, got.Group.Scheduler)
			require.True(t, got.Group.ForceOpenAIUltrafast)
		})
	}
	svc := &APIKeyService{}
	for _, version := range []int{27, 28} {
		got, used, err := svc.applyAuthCacheEntry("legacy", &APIKeyAuthCacheEntry{Snapshot: &APIKeyAuthSnapshot{Version: version}})
		require.NoError(t, err)
		require.False(t, used)
		require.Nil(t, got)
	}
}

func TestOpenAIGroupSchedulerConfigUsesMatchingAuthGroup(t *testing.T) {
	for _, tc := range []struct {
		name     string
		id       int64
		hydrated bool
		calls    int32
		mode     string
	}{
		{"matching", 39101, true, 0, "strict_health"},
		{"different_group", 39102, true, 1, "weighted"},
		{"unhydrated", 39101, false, 1, "weighted"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			id := int64(39101)
			repo := &countingPrivacyGroupRepo{group: &Group{ID: id, Scheduler: GroupSchedulerConfig{SelectionMode: "weighted"}}}
			svc := &OpenAIGatewayService{schedulerSnapshot: &SchedulerSnapshotService{groupRepo: repo}}
			ctx := context.WithValue(context.Background(), ctxkey.Group, &Group{ID: tc.id, Name: "auth", Platform: PlatformOpenAI, Status: StatusActive, Hydrated: tc.hydrated, Scheduler: GroupSchedulerConfig{Strategy: "high_availability", SelectionMode: "strict_health"}})
			cfg, ok := svc.openAIGroupSchedulerConfig(ctx, &id)
			require.True(t, ok)
			require.Equal(t, tc.mode, cfg.SelectionMode)
			require.Equal(t, tc.calls, repo.calls.Load())
		})
	}
}
