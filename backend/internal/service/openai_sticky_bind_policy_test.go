package service

import (
	"context"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func TestBindStickySessionAfterAdmissionWithPolicy(t *testing.T) {
	groupID := int64(9)
	gated := context.WithValue(context.Background(), openAIProfitControlGateCtxKey{}, &openAIProfitControlGate{groupID: groupID})
	cases := []struct {
		name     string
		policy   StickyBindPolicy
		ctx      context.Context
		existing int64
		want     int64
	}{
		{"legacy_no_gate_overwrites", StickyBindPolicyLegacy, context.Background(), 1, 2},
		{"legacy_gate_keeps_other", StickyBindPolicyLegacy, gated, 1, 1},
		{"legacy_gate_binds_when_empty", StickyBindPolicyLegacy, gated, 0, 2},
		{"preserve_no_gate_keeps_other", StickyBindPolicyPreserve, context.Background(), 1, 1},
		{"preserve_gate_keeps_other", StickyBindPolicyPreserve, gated, 1, 1},
		{"preserve_binds_when_empty", StickyBindPolicyPreserve, context.Background(), 0, 2},
		{"preserve_refreshes_same_account", StickyBindPolicyPreserve, context.Background(), 2, 2},
		{"migrate_no_gate_overwrites", StickyBindPolicyMigrate, context.Background(), 1, 2},
		{"migrate_gate_overwrites", StickyBindPolicyMigrate, gated, 1, 2},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cache := &schedulerTestGatewayCache{sessionBindings: map[string]int64{}}
			if tc.existing > 0 {
				cache.sessionBindings["openai:s"] = tc.existing
			}
			svc := &OpenAIGatewayService{cache: cache, cfg: &config.Config{}}
			require.NoError(t, svc.BindStickySessionAfterAdmissionWithPolicy(tc.ctx, &groupID, "s", 2, tc.policy))
			require.Equal(t, tc.want, cache.sessionBindings["openai:s"])
		})
	}
}

func TestBindStickySessionAfterProfitAdmission_IsLegacyPolicy(t *testing.T) {
	groupID := int64(9)
	cache := &schedulerTestGatewayCache{sessionBindings: map[string]int64{"openai:s": 1}}
	svc := &OpenAIGatewayService{cache: cache, cfg: &config.Config{}}
	require.NoError(t, svc.BindStickySessionAfterProfitAdmission(context.Background(), &groupID, "s", 2))
	require.Equal(t, int64(2), cache.sessionBindings["openai:s"], "HTTP 未配利润控制时等待后换到新账号仍绑到新账号，与改前一致")
}

func TestRefreshStickySessionTTL_RefreshesPrimaryKey(t *testing.T) {
	groupID := int64(9)
	cache := &schedulerTestGatewayCache{sessionBindings: map[string]int64{"openai:s": 1}}
	cfg := &config.Config{}
	cfg.Gateway.OpenAIWS.StickySessionTTLSeconds = 3600
	svc := &OpenAIGatewayService{cache: cache, cfg: cfg}
	require.NoError(t, svc.RefreshStickySessionTTL(context.Background(), &groupID, "s"))
	require.Equal(t, 1, cache.refreshedSessions["openai:s"])
	require.NoError(t, svc.RefreshStickySessionTTL(context.Background(), &groupID, ""), "空哈希直接返回")
	require.Equal(t, 1, cache.refreshedSessions["openai:s"])
}
