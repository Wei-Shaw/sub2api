package service

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseCyberPolicyUserAllowlist(t *testing.T) {
	ids, err := ParseCyberPolicyUserAllowlist("12, 34\n12\t56")
	require.NoError(t, err)
	require.Len(t, ids, 3)
	for _, raw := range []string{"0", "-1", "12,invalid", "9223372036854775808", strings.Repeat("1", 16385)} {
		ids, err := ParseCyberPolicyUserAllowlist(raw)
		require.Error(t, err)
		require.Nil(t, ids)
	}
	ids, err = ParseCyberPolicyUserAllowlist("")
	require.NoError(t, err)
	require.Empty(t, ids)
}

func TestCyberPolicyAllowlistCoversAllUserKeysAndRefreshes(t *testing.T) {
	repo := &fakeSettingRepo{vals: map[string]string{SettingKeyCyberPolicyUserAllowlist: "12"}}
	settings := &SettingService{settingRepo: repo}
	svc := &OpenAIGatewayService{settingService: settings}
	ctx := context.Background()
	// The allowlist also applies when session blocking is disabled.
	require.True(t, svc.CyberPolicyLogOnly(ctx, &APIKey{ID: 1, UserID: 12}))
	require.True(t, svc.CyberPolicyLogOnly(ctx, &APIKey{ID: 2, UserID: 12}))
	require.False(t, svc.CyberPolicyLogOnly(ctx, &APIKey{ID: 3, UserID: 34}))
	require.False(t, svc.CyberPolicyLogOnly(ctx, nil))
	repo.vals[SettingKeyCyberPolicyUserAllowlist] = "34"
	settings.cyberSessionBlockRuntimeCache.Store(&cachedCyberSessionBlockRuntime{})
	require.False(t, svc.CyberPolicyLogOnly(ctx, &APIKey{ID: 1, UserID: 12}))
	require.True(t, svc.CyberPolicyLogOnly(ctx, &APIKey{ID: 3, UserID: 34}))
}

func TestRecordCyberPolicyEventLogOnlyPreservesEvidenceWithoutSideEffects(t *testing.T) {
	repo := &banCountArgsTestRepo{}
	svc := NewContentModerationService(
		&contentModerationTestSettingRepo{values: map[string]string{SettingKeyRiskControlEnabled: "true"}},
		repo, nil, nil, nil, nil, nil, &EmailService{},
	)
	// An unconfigured email service must never be invoked in log-only mode.
	svc.RecordCyberPolicyEvent(context.Background(), CyberPolicyRecordInput{
		LogOnly: true, UserID: 12, UserEmail: "test@example.com", Model: "gpt-5",
		UpstreamMessage: "blocked", UpstreamBody: `{"error":{"code":"cyber_policy"}}`,
	})
	logs := repo.snapshotLogs()
	require.Len(t, logs, 1)
	require.Equal(t, ContentModerationModeCyberLogOnly, logs[0].Mode)
	require.Equal(t, ContentModerationActionCyberPolicy, logs[0].Action)
	require.True(t, logs[0].Flagged)
	require.Contains(t, logs[0].Error, "cyber_policy")
	require.False(t, logs[0].AutoBanned)
	require.False(t, logs[0].EmailSent)
	require.Zero(t, logs[0].ViolationCount)
	require.Empty(t, repo.snapshotCountCalls())
}
