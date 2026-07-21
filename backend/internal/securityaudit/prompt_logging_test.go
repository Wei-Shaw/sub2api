package securityaudit

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestPromptAuditLogAllowlistAndErrorsDoNotLeakCanarySecrets(t *testing.T) {
	const canary = "PROMPT_AUDIT_CANARY_SECRET_DO_NOT_PERSIST"
	var output bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&output, nil)))
	t.Cleanup(func() { slog.SetDefault(previous) })

	LogWarn(EventConfigReloadDegraded, map[string]any{
		"status":     "degraded",
		"error_code": "config_reload_failed",
		"error_kind": "Authorization: Bearer " + canary,
		"token":      canary,
		"body":       canary,
		"base_url":   "https://guard.example.test/path?api_key=" + canary,
		"raw_prompt": "prompt " + canary,
	})
	require.NotContains(t, output.String(), canary)
	require.NotContains(t, output.String(), "api_key=")
	require.Contains(t, output.String(), EventConfigReloadDegraded)

	beforeUnknown := output.Len()
	LogWarn("prompt_audit.typo_event", map[string]any{"status": "failed"})
	require.Equal(t, beforeUnknown, output.Len(), "events outside the stable dictionary must not be emitted")
	require.Len(t, knownLogEvents, 27)

	_, err := NormalizeBaseURL("https://guard.example.test/path?token=" + canary)
	require.Error(t, err)
	require.NotContains(t, err.Error(), canary)
}

func TestPromptGuardFailureLogUsesCompleteAllowlistedContextAndNoSideEffects(t *testing.T) {
	var output bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&output, nil)))
	t.Cleanup(func() { slog.SetDefault(previous) })
	groupID := int64(9)
	snapshot := PromptSnapshot{
		RequestID: "req-1", UserID: 2, APIKeyID: 3, GroupID: &groupID,
		Provider: "openai", Protocol: "openai_chat", Endpoint: "/v1/chat/completions",
		Model: "gpt-test", Stage: "http",
	}
	logGuardFailure(snapshot, ActiveConfig{ConfigVersion: 7}, DecisionUnavailable, ErrorCodeUnavailable, "guard-1", 25*time.Millisecond)

	var entry map[string]any
	require.NoError(t, json.Unmarshal(output.Bytes(), &entry))
	for key := range snapshotLogFields(snapshot) {
		require.Contains(t, entry, key)
	}
	require.EqualValues(t, 7, entry["config_version"])
	require.Equal(t, ErrorCodeUnavailable, entry["error_code"])
	require.Equal(t, false, entry["upstream_dispatched"])
	require.Equal(t, false, entry["billing_preconsumed"])
	require.EqualValues(t, 25, entry["latency_ms"])
}
