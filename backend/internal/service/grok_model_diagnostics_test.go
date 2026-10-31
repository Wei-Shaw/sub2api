//go:build unit

package service

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	"github.com/stretchr/testify/require"
)

func TestGrokModelDiagnosticsFreshExplicitMapping(t *testing.T) {
	account := grokDiagnosticExplicitAccount()
	credentialsBefore := cloneGrokDiagnosticValue(t, account.Credentials)
	scope := grokDiagnosticOfficialScope(t)
	now := time.Date(2026, 10, 25, 7, 0, 0, 0, time.UTC)
	snapshot := grokDiagnosticFreshSnapshot(scope, time.Date(2026, 10, 25, 2, 0, 0, 0, time.UTC))
	snapshotBefore := cloneGrokDiagnosticValue(t, snapshot)

	diag := EvaluateGrokModelDiagnostics(account, scope, snapshot, now)

	requireDiagnosticArrays(t, diag)
	require.Equal(t, "fresh", diag.State)
	require.Equal(t, "https://cli-chat-proxy.grok.com/v1/models", diag.Endpoint)
	require.Equal(t, "2026-10-25T02:00:00Z", diag.FetchedAt)
	require.Equal(t, "2026-10-25T08:00:00Z", diag.ExpiresAt)
	require.Equal(t, []string{"grok-4.5", "grok-4.6", "grok-4.7", "grok-4.7-build-fast"}, diag.Models)
	require.Nil(t, diag.LastAttempt)
	requireDiagnosticSorted(t, diag.Mappings)
	got := diagnosticByRequested(diag.Mappings)
	require.Equal(t, GrokModelDiagnosticItem{Requested: "friendly", Upstream: "grok-4.7", Status: "listed"}, got["friendly"])
	require.Equal(t, GrokModelDiagnosticItem{Requested: "grok-4.7-latest", Upstream: "grok-4.7", Status: "listed"}, got["grok-4.7-latest"])
	require.Equal(t, GrokModelDiagnosticItem{Requested: "composer-2.5", Upstream: "grok-composer-2.5-fast", Status: "not_listed"}, got["composer-2.5"])
	require.Equal(t, GrokModelDiagnosticItem{Requested: "custom-*", Status: "unverifiable"}, got["custom-*"])
	require.Equal(t, GrokModelDiagnosticItem{Requested: "grok-imagine-image", Upstream: "grok-imagine-image", Status: "out_of_scope"}, got["grok-imagine-image"])
	require.Equal(t, credentialsBefore, cloneGrokDiagnosticValue(t, account.Credentials))
	require.Equal(t, snapshotBefore, cloneGrokDiagnosticValue(t, snapshot))
}

func TestGrokModelDiagnosticsBareAliasDoesNotInventDefaultGeneration(t *testing.T) {
	now := time.Date(2026, 10, 25, 7, 0, 0, 0, time.UTC)
	scope := grokDiagnosticOfficialScope(t)
	aliases := map[string]string{
		"grok":             "grok",
		"grok-latest":      "grok-latest",
		"Grok":             "Grok",
		"GROK-LATEST":      "GROK-LATEST",
		"xai/grok":         "xai/grok",
		"x-ai/Grok-Latest": "x-ai/Grok-Latest",
	}

	t.Run("catalog has only the responses default", func(t *testing.T) {
		diag := evaluateGrokDiagnosticMapping(t, scope, []string{"grok-4.5"}, aliases, now)
		for requested, target := range aliases {
			item := diagnosticByRequested(diag.Mappings)[requested]
			require.Equal(t, "unverifiable", item.Status, requested)
			require.Equal(t, target, item.Upstream, requested)
			require.NotEqual(t, "grok-4.5", item.Upstream, requested)
			require.NotEqual(t, "grok-4.6", item.Upstream, requested)
		}
	})

	t.Run("catalog has only the builtin text default", func(t *testing.T) {
		diag := evaluateGrokDiagnosticMapping(t, scope, []string{"grok-4.6"}, aliases, now)
		for requested, target := range aliases {
			item := diagnosticByRequested(diag.Mappings)[requested]
			require.Equal(t, "unverifiable", item.Status, requested)
			require.Equal(t, target, item.Upstream, requested)
		}
	})

	t.Run("both generations still have no single forward target", func(t *testing.T) {
		diag := evaluateGrokDiagnosticMapping(t, scope, []string{"grok-4.5", "grok-4.6"}, aliases, now)
		for requested := range aliases {
			item := diagnosticByRequested(diag.Mappings)[requested]
			require.Equal(t, "unverifiable", item.Status, requested)
			require.NotEqual(t, "listed", item.Status, requested)
			require.NotEqual(t, "not_listed", item.Status, requested)
		}
	})

	t.Run("literal catalog grok is not grok-4.6", func(t *testing.T) {
		diag := evaluateGrokDiagnosticMapping(t, scope, []string{"grok"}, map[string]string{
			"pinned":   "grok-4.6",
			"friendly": "grok",
		}, now)
		got := diagnosticByRequested(diag.Mappings)
		require.Equal(t, GrokModelDiagnosticItem{Requested: "pinned", Upstream: "grok-4.6", Status: "not_listed"}, got["pinned"])
		require.Equal(t, GrokModelDiagnosticItem{Requested: "friendly", Upstream: "grok", Status: "unverifiable"}, got["friendly"])
	})

	t.Run("concrete aliases stay comparable", func(t *testing.T) {
		diag := evaluateGrokDiagnosticMapping(t, scope, []string{"grok-4.5", "grok-4.7"}, map[string]string{
			"pinned":   "grok-4.5",
			"latest":   "grok-4.7-latest",
			"composer": "composer-2.5",
		}, now)
		got := diagnosticByRequested(diag.Mappings)
		require.Equal(t, GrokModelDiagnosticItem{Requested: "pinned", Upstream: "grok-4.5", Status: "listed"}, got["pinned"])
		require.Equal(t, GrokModelDiagnosticItem{Requested: "latest", Upstream: "grok-4.7", Status: "listed"}, got["latest"])
		require.Equal(t, GrokModelDiagnosticItem{Requested: "composer", Upstream: "grok-composer-2.5-fast", Status: "not_listed"}, got["composer"])
	})
}

func TestGrokModelDiagnosticsCustomIDKeepsCase(t *testing.T) {
	scope, err := NewGrokCatalogScope(25, AccountTypeAPIKey, "https://relay.example.com/v1/models")
	require.NoError(t, err)
	now := time.Date(2026, 10, 25, 7, 0, 0, 0, time.UTC)

	t.Run("different case is not a catalog match", func(t *testing.T) {
		diag := evaluateGrokDiagnosticMapping(t, scope, []string{"custom-a", "other-id"}, map[string]string{
			"mixed": "Custom-A",
			"other": "Other-ID",
		}, now)
		got := diagnosticByRequested(diag.Mappings)
		require.Equal(t, GrokModelDiagnosticItem{Requested: "mixed", Upstream: "Custom-A", Status: "unverifiable"}, got["mixed"])
		require.NotEqual(t, "listed", got["mixed"].Status)
		require.NotEqual(t, "custom-a", got["mixed"].Upstream)
		require.Equal(t, GrokModelDiagnosticItem{Requested: "other", Upstream: "Other-ID", Status: "unverifiable"}, got["other"])
	})

	t.Run("same case matches", func(t *testing.T) {
		diag := evaluateGrokDiagnosticMapping(t, scope, []string{"Custom-A", "custom-a"}, map[string]string{
			"mixed": "Custom-A",
			"lower": "custom-a",
		}, now)
		got := diagnosticByRequested(diag.Mappings)
		require.Equal(t, GrokModelDiagnosticItem{Requested: "mixed", Upstream: "Custom-A", Status: "listed"}, got["mixed"])
		require.Equal(t, GrokModelDiagnosticItem{Requested: "lower", Upstream: "custom-a", Status: "listed"}, got["lower"])
	})
}

func evaluateGrokDiagnosticMapping(t *testing.T, scope *GrokCatalogScope, models []string, targets map[string]string, now time.Time) *GrokModelDiagnostics {
	t.Helper()
	mapping := make(map[string]any, len(targets))
	for key, target := range targets {
		mapping[key] = target
	}
	account := &Account{
		ID: scope.AccountID, Platform: PlatformGrok, Type: scope.AccountType,
		Credentials: map[string]any{"model_mapping": mapping},
	}
	snapshot := &GrokObservedModelsSnapshot{
		Models:    append([]string{}, models...),
		FetchedAt: now.Add(-time.Hour).UTC().Format(time.RFC3339Nano),
		Scope:     scope,
	}
	return EvaluateGrokModelDiagnostics(account, scope, snapshot, now)
}

func TestGrokModelDiagnosticsExplicitComposerTargetInCatalogIsListed(t *testing.T) {
	account := grokDiagnosticExplicitAccount()
	account.Credentials["model_mapping"] = map[string]any{"composer-2.5": "grok-4.7"}
	scope := grokDiagnosticOfficialScope(t)
	now := time.Date(2026, 10, 25, 7, 0, 0, 0, time.UTC)

	diag := EvaluateGrokModelDiagnostics(account, scope, grokDiagnosticFreshSnapshot(scope, now.Add(-time.Hour)), now)

	got := diagnosticByRequested(diag.Mappings)
	require.Equal(t, GrokModelDiagnosticItem{Requested: "composer-2.5", Upstream: "grok-4.7", Status: "listed"}, got["composer-2.5"])
	require.NotEqual(t, "not_listed", got["composer-2.5"].Status)
}

func TestGrokModelDiagnosticsBuildSuffixIsNotCatalogEquivalence(t *testing.T) {
	account := grokDiagnosticExplicitAccount()
	account.Credentials["model_mapping"] = map[string]any{
		"plain":      "grok-4.7",
		"build":      "grok-4.7-build",
		"build-fast": "grok-4.7-build-fast",
	}
	scope := grokDiagnosticOfficialScope(t)
	now := time.Date(2026, 10, 25, 7, 0, 0, 0, time.UTC)

	diag := EvaluateGrokModelDiagnostics(account, scope, grokDiagnosticFreshSnapshot(scope, now.Add(-time.Hour)), now)

	got := diagnosticByRequested(diag.Mappings)
	require.Equal(t, "listed", got["plain"].Status)
	require.Equal(t, "grok-4.7", got["plain"].Upstream)
	require.Equal(t, "unverifiable", got["build"].Status)
	require.NotEqual(t, "listed", got["build"].Status)
	require.NotEqual(t, "not_listed", got["build"].Status)
	require.Equal(t, "grok-4.7-build-fast", got["build-fast"].Upstream)
	require.Equal(t, "listed", got["build-fast"].Status)
}

func TestGrokModelDiagnosticsMediaAndVoiceStayOutOfScope(t *testing.T) {
	account := grokDiagnosticExplicitAccount()
	account.Credentials["model_mapping"] = map[string]any{
		"picture": "grok-imagine-image",
		"clip":    "grok-imagine-video",
		"legacy":  "grok-video-1.5",
		"speak":   "grok-voice-latest",
		"hear":    "xai/grok-tts",
	}
	scope := grokDiagnosticOfficialScope(t)
	now := time.Date(2026, 10, 25, 7, 0, 0, 0, time.UTC)

	diag := EvaluateGrokModelDiagnostics(account, scope, grokDiagnosticFreshSnapshot(scope, now.Add(-time.Hour)), now)

	for _, item := range diag.Mappings {
		require.Equal(t, "out_of_scope", item.Status, item.Requested)
		require.NotEqual(t, "not_listed", item.Status)
	}
	got := diagnosticByRequested(diag.Mappings)
	require.Equal(t, "grok-imagine-video", got["clip"].Upstream)
	require.Equal(t, "grok-video-1.5", got["legacy"].Upstream)
	require.Equal(t, "grok-voice-latest", got["speak"].Upstream)
	require.Equal(t, "grok-tts", got["hear"].Upstream)
}

func TestGrokModelDiagnosticsDefaultMappingIsReadOnly(t *testing.T) {
	account := &Account{ID: 25, Platform: PlatformGrok, Type: AccountTypeOAuth}
	scope := grokDiagnosticOfficialScope(t)
	now := time.Date(2026, 10, 25, 7, 0, 0, 0, time.UTC)

	diag := EvaluateGrokModelDiagnostics(account, scope, grokDiagnosticFreshSnapshot(scope, now.Add(-time.Hour)), now)

	require.Nil(t, account.Credentials)
	require.Nil(t, account.Extra)
	got := diagnosticByRequested(diag.Mappings)
	require.Equal(t, "listed", got["grok-4.7"].Status)
	require.Equal(t, "grok-4.7", got["grok-4.7"].Upstream)
	require.Equal(t, "not_listed", got["composer-2.5"].Status)
	require.Equal(t, "grok-composer-2.5-fast", got["composer-2.5"].Upstream)
	require.Equal(t, "out_of_scope", got["grok-imagine-image"].Status)
	require.NotEmpty(t, diag.Mappings)
}

func TestGrokModelDiagnosticsAPIKeyUsesOwnCatalog(t *testing.T) {
	scope, err := NewGrokCatalogScope(25, AccountTypeAPIKey, "https://relay.example.com/v1/models?token=query-secret")
	require.NoError(t, err)
	account := &Account{
		ID: 25, Platform: PlatformGrok, Type: AccountTypeAPIKey,
		Credentials: map[string]any{"model_mapping": map[string]any{
			"friendly": "grok-4.7",
			"older":    "grok-4.6",
			"private":  "private-model",
			"seen":     "seen-model",
			"picture":  "grok-imagine-image",
		}},
	}
	now := time.Date(2026, 10, 25, 7, 0, 0, 0, time.UTC)
	snapshot := &GrokObservedModelsSnapshot{
		Models:    []string{"grok-3", "seen-model"},
		FetchedAt: now.Add(-time.Hour).Format(time.RFC3339Nano),
		Scope:     scope,
	}

	diag := EvaluateGrokModelDiagnostics(account, scope, snapshot, now)

	require.Equal(t, "fresh", diag.State)
	require.Equal(t, "https://relay.example.com/v1/models", diag.Endpoint)
	require.NotContains(t, diag.Endpoint, "query-secret")
	require.NotContains(t, diag.Endpoint, "token")
	got := diagnosticByRequested(diag.Mappings)
	require.Equal(t, "not_listed", got["friendly"].Status)
	require.Equal(t, "not_listed", got["older"].Status)
	require.Equal(t, "unverifiable", got["private"].Status)
	require.Equal(t, "listed", got["seen"].Status)
	require.Equal(t, "seen-model", got["seen"].Upstream)
	require.Equal(t, "out_of_scope", got["picture"].Status)
	raw := grokDiagnosticJSON(t, diag)
	require.NotContains(t, raw, "query-secret")
}

func TestGrokModelDiagnosticsSourceMismatchDoesNotBorrowCatalog(t *testing.T) {
	account := grokDiagnosticExplicitAccount()
	now := time.Date(2026, 10, 25, 7, 0, 0, 0, time.UTC)
	base := grokDiagnosticOfficialScope(t)
	snapshot := grokDiagnosticFreshSnapshot(base, now.Add(-time.Hour))
	alternates := []*GrokCatalogScope{
		mustGrokDiagnosticScope(t, 26, AccountTypeOAuth, "https://cli-chat-proxy.grok.com/v1/models"),
		mustGrokDiagnosticScope(t, 25, AccountTypeAPIKey, "https://cli-chat-proxy.grok.com/v1/models"),
		mustGrokDiagnosticScope(t, 25, AccountTypeOAuth, "https://api.x.ai/v1/models"),
		mustGrokDiagnosticScope(t, 25, AccountTypeOAuth, "https://cli-chat-proxy.grok.com/v1/models?foo=1"),
	}
	for _, current := range alternates {
		diag := EvaluateGrokModelDiagnostics(account, current, snapshot, now)
		require.Equal(t, "source_changed", diag.State)
		require.Equal(t, snapshot.Models, diag.Models)
		require.Equal(t, current.Endpoint, diag.Endpoint)
		for _, item := range diag.Mappings {
			require.NotEqual(t, "listed", item.Status, item.Requested)
			require.NotEqual(t, "not_listed", item.Status, item.Requested)
		}
		require.Equal(t, "unverifiable", diagnosticByRequested(diag.Mappings)["friendly"].Status)
		require.Equal(t, "out_of_scope", diagnosticByRequested(diag.Mappings)["grok-imagine-image"].Status)
	}
}

func TestGrokModelDiagnosticsUntrustedSnapshotsDoNotClaimAbsence(t *testing.T) {
	account := grokDiagnosticExplicitAccount()
	scope := grokDiagnosticOfficialScope(t)
	now := time.Date(2026, 10, 25, 8, 0, 0, 0, time.UTC)
	fetched := time.Date(2026, 10, 25, 2, 0, 0, 0, time.UTC)
	models := []string{"grok-4.5", "grok-4.6", "grok-4.7", "grok-4.7-build-fast"}

	cases := []struct {
		name     string
		snapshot *GrokObservedModelsSnapshot
		state    string
	}{
		{name: "missing", snapshot: nil, state: "unknown"},
		{name: "legacy-without-scope", snapshot: &GrokObservedModelsSnapshot{Models: models, FetchedAt: fetched.Format(time.RFC3339Nano), Source: "upstream_v1_models"}, state: "unknown"},
		{name: "exactly-six-hours", snapshot: grokDiagnosticFreshSnapshot(scope, fetched), state: "stale"},
		{name: "older-than-six-hours", snapshot: grokDiagnosticFreshSnapshot(scope, fetched.Add(-time.Second)), state: "stale"},
		{name: "invalid-time", snapshot: &GrokObservedModelsSnapshot{Models: models, FetchedAt: "not-a-time", Scope: scope}, state: "unknown"},
		{name: "future-time", snapshot: grokDiagnosticFreshSnapshot(scope, now.Add(time.Minute)), state: "unknown"},
		{name: "empty-models", snapshot: &GrokObservedModelsSnapshot{Models: []string{" "}, FetchedAt: fetched.Format(time.RFC3339Nano), Scope: scope}, state: "unknown"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			diag := EvaluateGrokModelDiagnostics(account, scope, tc.snapshot, now)
			requireDiagnosticArrays(t, diag)
			require.Equal(t, tc.state, diag.State)
			for _, item := range diag.Mappings {
				require.NotEqual(t, "not_listed", item.Status, item.Requested)
			}
		})
	}

	justFresh := EvaluateGrokModelDiagnostics(account, scope, grokDiagnosticFreshSnapshot(scope, now.Add(-6*time.Hour+time.Nanosecond)), now)
	require.Equal(t, "fresh", justFresh.State)
	require.Equal(t, "not_listed", diagnosticByRequested(justFresh.Mappings)["composer-2.5"].Status)

	stale := EvaluateGrokModelDiagnostics(account, scope, grokDiagnosticFreshSnapshot(scope, fetched), now)
	require.Equal(t, "stale", stale.State)
	require.Equal(t, models, stale.Models)
	require.Equal(t, "2026-10-25T02:00:00Z", stale.FetchedAt)
	require.Equal(t, "2026-10-25T08:00:00Z", stale.ExpiresAt)
	require.Equal(t, "listed", diagnosticByRequested(stale.Mappings)["friendly"].Status)
	require.Equal(t, "unverifiable", diagnosticByRequested(stale.Mappings)["composer-2.5"].Status)
}

func TestGrokModelDiagnosticsFailedAttemptDoesNotReplaceFreshSuccess(t *testing.T) {
	account := grokDiagnosticExplicitAccount()
	scope := grokDiagnosticOfficialScope(t)
	now := time.Date(2026, 10, 25, 7, 0, 0, 0, time.UTC)
	successAt := time.Date(2026, 10, 25, 2, 0, 0, 0, time.UTC)
	snapshot := grokDiagnosticFreshSnapshot(scope, successAt)
	snapshot.LastAttempt = &GrokCatalogAttempt{
		StartedAt:  now.Add(-time.Minute).Format(time.RFC3339Nano),
		FinishedAt: now.Format(time.RFC3339Nano),
		Status:     "failed",
		ErrorCode:  "http_error",
		Scope:      scope,
	}

	diag := EvaluateGrokModelDiagnostics(account, scope, snapshot, now)

	require.Equal(t, "fresh", diag.State)
	require.Equal(t, "2026-10-25T02:00:00Z", diag.FetchedAt)
	require.NotEqual(t, diag.FetchedAt, diag.LastAttempt.FinishedAt)
	require.Equal(t, "failed", diag.LastAttempt.Status)
	require.Equal(t, "http_error", diag.LastAttempt.ErrorCode)
	require.Equal(t, "not_listed", diagnosticByRequested(diag.Mappings)["composer-2.5"].Status)
	require.Equal(t, "listed", diagnosticByRequested(diag.Mappings)["friendly"].Status)
	diag.LastAttempt.Status = "mutated"
	require.Equal(t, "failed", snapshot.LastAttempt.Status)
}

func TestGrokModelDiagnosticsUnknownWithoutCurrentScope(t *testing.T) {
	account := grokDiagnosticExplicitAccount()
	now := time.Date(2026, 10, 25, 7, 0, 0, 0, time.UTC)
	snapshot := grokDiagnosticFreshSnapshot(grokDiagnosticOfficialScope(t), now.Add(-time.Hour))

	diag := EvaluateGrokModelDiagnostics(account, nil, snapshot, now)

	require.Equal(t, "unknown", diag.State)
	require.Empty(t, diag.Endpoint)
	require.Equal(t, snapshot.Models, diag.Models)
	requireNoCatalogVerdict(t, diag.Mappings)
	raw := grokDiagnosticJSON(t, diag)
	require.Contains(t, raw, `"models":["grok-4.5","grok-4.6","grok-4.7","grok-4.7-build-fast"]`)
	require.NotContains(t, raw, `"models":null`)
	require.NotContains(t, raw, `"mappings":null`)
}

func TestGetGrokModelDiagnosticsReadsScopeWithoutTokenOrHTTP(t *testing.T) {
	upstream := &grokDiagnosticHTTPUpstream{}
	proxyID := int64(9)
	account := grokDiagnosticExplicitAccount()
	account.ProxyID = &proxyID
	account.Credentials["access_token"] = "access-secret"
	account.Credentials["refresh_token"] = "refresh-secret"
	now := time.Now().UTC().Add(-time.Hour)
	scope := grokDiagnosticOfficialScope(t)
	rememberGrokObservedSnapshot(account, grokDiagnosticFreshSnapshot(scope, now))
	svc := &AccountTestService{httpUpstream: upstream}

	diag := svc.GetGrokModelDiagnostics(context.Background(), account)

	require.Equal(t, "fresh", diag.State)
	require.Equal(t, "https://cli-chat-proxy.grok.com/v1/models", diag.Endpoint)
	require.Equal(t, "listed", diagnosticByRequested(diag.Mappings)["friendly"].Status)
	require.Zero(t, upstream.calls)
	raw := grokDiagnosticJSON(t, diag)
	require.NotContains(t, raw, "access-secret")
	require.NotContains(t, raw, "refresh-secret")
	require.Nil(t, svc.GetGrokModelDiagnostics(context.Background(), &Account{ID: 3, Platform: PlatformOpenAI, Type: AccountTypeOAuth}))
	require.Nil(t, svc.GetGrokModelDiagnostics(context.Background(), nil))
}

func TestGetGrokModelDiagnosticsUsesSettingBaseURLAndStripsSecrets(t *testing.T) {
	settings := NewSettingService(&grokBaseURLSettingRepoStub{values: map[string]string{
		SettingKeyGrokDefaultBaseURLMode: GrokDefaultBaseURLModeUSWest2,
	}}, nil)
	account := &Account{ID: 25, Platform: PlatformGrok, Type: AccountTypeOAuth, Credentials: map[string]any{
		"model_mapping": map[string]any{"friendly": "grok-4.7"},
		"access_token":  "access-secret",
	}}
	cliScope := grokDiagnosticOfficialScope(t)
	rememberGrokObservedSnapshot(account, grokDiagnosticFreshSnapshot(cliScope, time.Now().UTC().Add(-time.Hour)))
	svc := &AccountTestService{settingService: settings}

	diag := svc.GetGrokModelDiagnostics(context.Background(), account)

	require.Equal(t, "source_changed", diag.State)
	require.Equal(t, "https://us-west-2.api.x.ai/v1/models", diag.Endpoint)
	require.NotEqual(t, "not_listed", diagnosticByRequested(diag.Mappings)["friendly"].Status)
	require.NotEqual(t, "listed", diagnosticByRequested(diag.Mappings)["friendly"].Status)

	apiAccount := &Account{ID: 25, Platform: PlatformGrok, Type: AccountTypeAPIKey, Credentials: map[string]any{
		"api_key":       "sk-secret",
		"base_url":      "https://user:s3cret@relay.example.com/v1?access_token=query-secret",
		"model_mapping": map[string]any{"friendly": "grok-4.7"},
	}}
	apiSvc := &AccountTestService{cfg: &config.Config{Security: config.SecurityConfig{URLAllowlist: config.URLAllowlistConfig{Enabled: false}}}}
	apiDiag := apiSvc.GetGrokModelDiagnostics(context.Background(), apiAccount)
	require.Equal(t, "unknown", apiDiag.State)
	require.Equal(t, "https://relay.example.com/v1/models", apiDiag.Endpoint)
	raw := grokDiagnosticJSON(t, apiDiag)
	require.NotContains(t, raw, "s3cret")
	require.NotContains(t, raw, "query-secret")
	require.NotContains(t, raw, "sk-secret")
	require.NotContains(t, raw, "user:")
}

func TestGetGrokModelDiagnosticsInvalidScopeStaysUnknown(t *testing.T) {
	account := &Account{ID: 25, Platform: PlatformGrok, Type: AccountTypeAPIKey, Credentials: map[string]any{
		"api_key":  "sk-secret",
		"base_url": "https://blocked.example/v1",
	}}
	svc := &AccountTestService{cfg: &config.Config{Security: config.SecurityConfig{URLAllowlist: config.URLAllowlistConfig{
		Enabled: true, UpstreamHosts: []string{"api.x.ai"},
	}}}}

	diag := svc.GetGrokModelDiagnostics(context.Background(), account)

	require.Equal(t, "unknown", diag.State)
	require.Empty(t, diag.Endpoint)
	requireDiagnosticArrays(t, diag)
	require.NotContains(t, grokDiagnosticJSON(t, diag), "sk-secret")
}

func TestGrokModelDiagnosticsSnapshotDoesNotChangeMappingOrSupport(t *testing.T) {
	account := &Account{
		ID: 25, Platform: PlatformGrok, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true,
		Credentials: map[string]any{"model_mapping": map[string]any{
			"grok-4.6":     "grok-4.6",
			"composer-2.5": "composer-2.5",
			"custom-*":     "grok-4.7",
		}},
	}
	requests := []string{"grok-4.6", "composer-2.5", "custom-model", "grok-4.7", "gpt-5"}
	mappingBefore := account.GetModelMapping()
	supportBefore := map[string]bool{}
	for _, requested := range requests {
		supportBefore[requested] = account.IsModelSupported(requested)
	}
	mappedBefore := map[string]string{}
	for _, requested := range requests {
		mappedBefore[requested] = account.GetMappedModel(requested)
	}
	credentialsBefore := cloneGrokDiagnosticValue(t, account.Credentials)

	rememberGrokObservedSnapshot(account, grokDiagnosticFreshSnapshot(grokDiagnosticOfficialScope(t), time.Now().UTC()))
	_ = EvaluateGrokModelDiagnostics(account, grokDiagnosticOfficialScope(t), parseGrokObservedModels(account.Extra), time.Now().UTC())

	require.Equal(t, mappingBefore, account.GetModelMapping())
	require.Equal(t, credentialsBefore, cloneGrokDiagnosticValue(t, account.Credentials))
	for _, requested := range requests {
		require.Equal(t, supportBefore[requested], account.IsModelSupported(requested), requested)
		require.Equal(t, mappedBefore[requested], account.GetMappedModel(requested), requested)
	}
	require.NotContains(t, account.Credentials, "grok_observed_models")
}

func grokDiagnosticExplicitAccount() *Account {
	return &Account{
		ID: 25, Platform: PlatformGrok, Type: AccountTypeOAuth,
		Credentials: map[string]any{"model_mapping": map[string]any{
			"friendly":           "grok-4.7",
			"grok-4.7-latest":    "grok-4.7-latest",
			"composer-2.5":       "composer-2.5",
			"custom-*":           "grok-4.7",
			"grok-imagine-image": "grok-imagine-image",
		}},
	}
}

func grokDiagnosticOfficialScope(t *testing.T) *GrokCatalogScope {
	t.Helper()
	return mustGrokDiagnosticScope(t, 25, AccountTypeOAuth, "https://cli-chat-proxy.grok.com/v1/models")
}

func mustGrokDiagnosticScope(t *testing.T, accountID int64, accountType, modelsURL string) *GrokCatalogScope {
	t.Helper()
	scope, err := NewGrokCatalogScope(accountID, accountType, modelsURL)
	require.NoError(t, err)
	return scope
}

func grokDiagnosticFreshSnapshot(scope *GrokCatalogScope, fetched time.Time) *GrokObservedModelsSnapshot {
	cloned := *scope
	return &GrokObservedModelsSnapshot{
		Models:    []string{"grok-4.5", "grok-4.6", "grok-4.7", "grok-4.7-build-fast"},
		FetchedAt: fetched.UTC().Format(time.RFC3339Nano),
		Scope:     &cloned,
	}
}

func diagnosticByRequested(items []GrokModelDiagnosticItem) map[string]GrokModelDiagnosticItem {
	out := make(map[string]GrokModelDiagnosticItem, len(items))
	for _, item := range items {
		out[item.Requested] = item
	}
	return out
}

func requireDiagnosticSorted(t *testing.T, items []GrokModelDiagnosticItem) {
	t.Helper()
	for i := 1; i < len(items); i++ {
		require.LessOrEqual(t, items[i-1].Requested, items[i].Requested)
	}
}

func requireDiagnosticArrays(t *testing.T, diag *GrokModelDiagnostics) {
	t.Helper()
	require.NotNil(t, diag)
	require.NotNil(t, diag.Models)
	require.NotNil(t, diag.Mappings)
	raw := grokDiagnosticJSON(t, diag)
	require.NotContains(t, raw, `"models":null`)
	require.NotContains(t, raw, `"mappings":null`)
	require.Contains(t, raw, `"models":[`)
	require.Contains(t, raw, `"mappings":[`)
}

func requireNoCatalogVerdict(t *testing.T, items []GrokModelDiagnosticItem) {
	t.Helper()
	for _, item := range items {
		require.NotEqual(t, "listed", item.Status, item.Requested)
		require.NotEqual(t, "not_listed", item.Status, item.Requested)
	}
}

func grokDiagnosticJSON(t *testing.T, value any) string {
	t.Helper()
	raw, err := json.Marshal(value)
	require.NoError(t, err)
	return string(raw)
}

func cloneGrokDiagnosticValue(t *testing.T, value any) any {
	t.Helper()
	raw, err := json.Marshal(value)
	require.NoError(t, err)
	var cloned any
	require.NoError(t, json.Unmarshal(raw, &cloned))
	return cloned
}

type grokDiagnosticHTTPUpstream struct {
	calls int
}

func (u *grokDiagnosticHTTPUpstream) Do(*http.Request, string, int64, int) (*http.Response, error) {
	u.calls++
	return nil, errors.New("diagnostics must not call upstream")
}

func (u *grokDiagnosticHTTPUpstream) DoWithTLS(req *http.Request, proxyURL string, accountID int64, accountConcurrency int, _ *tlsfingerprint.Profile) (*http.Response, error) {
	return u.Do(req, proxyURL, accountID, accountConcurrency)
}
