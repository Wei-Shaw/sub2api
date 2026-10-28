//go:build unit

package service

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	"github.com/stretchr/testify/require"
)

const (
	grokCatalogSyncSecretToken   = "oauth-access-token"
	grokCatalogSyncSecretRefresh = "oauth-refresh-token"
	grokCatalogFallbackWarning   = "The upstream model list was unavailable; configured IDs were used only for capability enrichment."
	grokCatalogSaveFailure       = "Failed to save upstream model catalog"
	grokCatalogMetadataFailure   = "Failed to save upstream model metadata"
)

func TestGrokCatalogSyncSavesIDOnlyModelsWhenMetadataRegistryFails(t *testing.T) {
	account := grokOAuthModelSyncTestAccount("https://cli-chat-proxy.grok.com/v1")
	account.Credentials["model_mapping"] = map[string]any{"alias": "grok-configured"}
	account.Extra = map[string]any{"note": "keep"}
	credentialsBefore := cloneGrokCatalogMemoryValue(account.Credentials)
	repo := newGrokCatalogMemoryRepo(account)
	upstream := newGrokCatalogUpstream(http.StatusOK, `{"data":[{"model":"grok-4.7"}]}`, http.StatusBadGateway, `{"error":"registry-down"}`)
	svc := &AccountTestService{
		accountRepo:       repo,
		httpUpstream:      upstream,
		cfg:               upstreamModelSyncTestConfig(),
		grokTokenProvider: NewGrokTokenProvider(nil, nil),
	}

	catalog, err := svc.SyncUpstreamModelCatalog(context.Background(), account)
	require.NoError(t, err)
	require.Equal(t, "upstream", catalog.DiscoverySource)
	require.Equal(t, []string{"grok-4.7"}, catalog.Models)
	require.Contains(t, catalog.Warnings, UpstreamModelSyncWarning{
		Code:    UpstreamModelMetadataIncompleteCode,
		Message: "Model IDs were synced, but capability metadata is incomplete.",
	})
	require.Equal(t, []string{"grok-4.7"}, parseGrokObservedModels(account.Extra).Models)
	require.Equal(t, "keep", account.Extra["note"])
	require.Equal(t, credentialsBefore, cloneGrokCatalogMemoryValue(account.Credentials))

	stored, err := repo.GetByID(context.Background(), account.ID)
	require.NoError(t, err)
	snap := parseGrokObservedModels(stored.Extra)
	require.Equal(t, []string{"grok-4.7"}, snap.Models)
	require.NotContains(t, snap.Models, "grok-configured")
	require.Equal(t, "keep", stored.Extra["note"])
	require.Equal(t, grokCatalogStatusSuccess, snap.LastAttempt.Status)
	require.Empty(t, snap.LastAttempt.ErrorCode)
	require.Equal(t, "https://cli-chat-proxy.grok.com/v1/models", snap.Scope.Endpoint)
	require.Equal(t, account.ID, snap.Scope.AccountID)
	require.Equal(t, account.Type, snap.Scope.AccountType)
	scope, err := NewGrokCatalogScope(account.ID, account.Type, upstream.modelsRequests()[0].URL.String())
	require.NoError(t, err)
	require.Equal(t, scope.Fingerprint, snap.Scope.Fingerprint)
	require.Equal(t, 1, repo.casCalls)
	require.Zero(t, repo.updateExtraCalls)
	require.Len(t, upstream.modelsRequests(), 1)
	require.Len(t, upstream.registryRequests(), 1)
	storedJSON := grokCatalogJSON(t, stored.Extra)
	require.NotContains(t, storedJSON, grokCatalogSyncSecretToken)
	require.NotContains(t, storedJSON, grokCatalogSyncSecretRefresh)
	require.NotContains(t, storedJSON, "registry-down")
}

func TestGrokCatalogSyncConfiguredFallbackDoesNotRefreshSuccess(t *testing.T) {
	for _, status := range []int{http.StatusNotFound, http.StatusMethodNotAllowed} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			account := grokCatalogAccountWithSuccess([]string{"grok-4.7"}, time.Now().Add(-time.Hour).UTC())
			account.Credentials["model_mapping"] = map[string]any{
				"alias":    "grok-configured",
				"wildcard": "grok-*",
			}
			previous := parseGrokObservedModels(account.Extra)
			repo := newGrokCatalogMemoryRepo(account)
			upstream := newGrokCatalogUpstream(status, `{"error":"hidden-upstream-body"}`, http.StatusBadGateway, `{"error":"registry-down"}`)
			svc := grokCatalogSyncService(repo, upstream)

			catalog, err := svc.SyncUpstreamModelCatalog(context.Background(), account)
			require.NoError(t, err)
			require.Equal(t, "configured_fallback", catalog.DiscoverySource)
			require.Equal(t, []string{"grok-configured"}, catalog.Models)
			require.Contains(t, catalog.Warnings, UpstreamModelSyncWarning{
				Code:    "grok_model_catalog_configured_fallback",
				Message: grokCatalogFallbackWarning,
			})

			stored, err := repo.GetByID(context.Background(), account.ID)
			require.NoError(t, err)
			snap := parseGrokObservedModels(stored.Extra)
			require.Equal(t, previous.Models, snap.Models)
			require.Equal(t, previous.FetchedAt, snap.FetchedAt)
			require.Equal(t, previous.SuccessStartedAt, snap.SuccessStartedAt)
			require.Equal(t, previous.Scope.Fingerprint, snap.Scope.Fingerprint)
			require.Equal(t, grokCatalogStatusConfiguredFallback, snap.LastAttempt.Status)
			require.Equal(t, "http_error", snap.LastAttempt.ErrorCode)
			require.NotContains(t, grokCatalogJSON(t, stored.Extra), "hidden-upstream-body")
			require.NotContains(t, snap.Models, "grok-configured")
		})
	}
}

func TestGrokCatalogSyncKeepsSuccessOnHTTPErrors(t *testing.T) {
	for _, status := range []int{http.StatusUnauthorized, http.StatusForbidden, http.StatusTooManyRequests, http.StatusInternalServerError} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			account := grokCatalogAccountWithSuccess([]string{"grok-4.7"}, time.Now().Add(-time.Hour).UTC())
			previous := parseGrokObservedModels(account.Extra)
			repo := newGrokCatalogMemoryRepo(account)
			upstream := newGrokCatalogUpstream(status, `{"error":"hidden-upstream-body"}`, http.StatusOK, `{}`)
			svc := grokCatalogSyncService(repo, upstream)

			_, err := svc.SyncUpstreamModelCatalog(context.Background(), account)
			requireGrokHTTPSyncError(t, err, status)
			snap := grokCatalogStoredSnapshot(t, repo, account.ID)
			requireGrokSuccessPreserved(t, previous, snap)
			require.Equal(t, grokCatalogStatusFailed, snap.LastAttempt.Status)
			require.Equal(t, "http_error", snap.LastAttempt.ErrorCode)
			require.NotContains(t, grokCatalogJSON(t, snap), "hidden-upstream-body")
			require.NotContains(t, err.Error(), "hidden-upstream-body")
		})
	}
}

func TestGrokCatalogSyncHTTPErrorSurvivesObservationSaveFailure(t *testing.T) {
	account := grokCatalogAccountWithSuccess([]string{"grok-4.7"}, time.Now().Add(-time.Hour).UTC())
	previous := parseGrokObservedModels(account.Extra)
	repo := &grokCatalogCASErrorRepo{grokCatalogMemoryRepo: newGrokCatalogMemoryRepo(account), err: errors.New("database unavailable")}
	upstream := newGrokCatalogUpstream(http.StatusForbidden, `{"error":"hidden-upstream-body"}`, http.StatusOK, `{}`)
	svc := grokCatalogSyncService(repo, upstream)

	_, err := svc.SyncUpstreamModelCatalog(context.Background(), account)
	requireGrokHTTPSyncError(t, err, http.StatusForbidden)
	require.NotContains(t, syncSafeMessage(t, err), "database unavailable")
	snap := grokCatalogStoredSnapshot(t, repo, account.ID)
	require.Equal(t, previous.Models, snap.Models)
	require.Equal(t, previous.FetchedAt, snap.FetchedAt)
}

func TestGrokCatalogSyncKeepsSuccessOnEmptyMalformedAndOversize(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		limit   int64
		message string
		code    string
	}{
		{
			name:    "empty",
			body:    `{"data":[]}`,
			message: "Upstream returned no supported models",
			code:    "empty_catalog",
		},
		{
			name:    "malformed",
			body:    `not-json`,
			message: "Upstream model list response was not valid JSON",
			code:    "invalid_response",
		},
		{
			name:    "error envelope",
			body:    `{"error":{"message":"hidden-upstream-body"}}`,
			message: "Upstream model list response was not valid JSON",
			code:    "invalid_response",
		},
		{
			name:    "too large",
			body:    `{"data":[{"model":"grok-overflow"}]}` + strings.Repeat(" ", 80),
			limit:   int64(len(`{"data":[{"model":"grok-overflow"}]}`)),
			message: "Upstream model list response is too large",
			code:    "too_large",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			account := grokCatalogAccountWithSuccess([]string{"grok-4.7"}, time.Now().Add(-time.Hour).UTC())
			previous := parseGrokObservedModels(account.Extra)
			repo := newGrokCatalogMemoryRepo(account)
			upstream := newGrokCatalogUpstream(http.StatusOK, tt.body, http.StatusOK, `{}`)
			cfg := upstreamModelSyncTestConfig()
			if tt.limit > 0 {
				cfg.Gateway.ModelsListReadMaxBytes = tt.limit
			}
			svc := grokCatalogSyncService(repo, upstream)
			svc.cfg = cfg

			_, err := svc.SyncUpstreamModelCatalog(context.Background(), account)
			require.Equal(t, tt.message, syncSafeMessage(t, err))
			require.Equal(t, UpstreamModelSyncErrorUpstream, syncKind(t, err))
			snap := grokCatalogStoredSnapshot(t, repo, account.ID)
			requireGrokSuccessPreserved(t, previous, snap)
			require.Equal(t, grokCatalogStatusFailed, snap.LastAttempt.Status)
			require.Equal(t, tt.code, snap.LastAttempt.ErrorCode)
			require.NotContains(t, snap.Models, "grok-overflow")
			require.NotContains(t, grokCatalogJSON(t, snap), "hidden-upstream-body")
		})
	}
}

func TestGrokCatalogSyncReportsCatalogSaveFailureAsInternal(t *testing.T) {
	account := grokCatalogAccountWithSuccess([]string{"grok-old"}, time.Now().Add(-time.Hour).UTC())
	repo := &grokCatalogCASErrorRepo{grokCatalogMemoryRepo: newGrokCatalogMemoryRepo(account), err: errors.New("database unavailable")}
	upstream := newGrokCatalogUpstream(http.StatusOK, `{"data":[{"model":"grok-4.7"}]}`, http.StatusOK, `{}`)
	svc := grokCatalogSyncService(repo, upstream)

	catalog, err := svc.SyncUpstreamModelCatalog(context.Background(), account)
	require.Nil(t, catalog)
	require.Equal(t, grokCatalogSaveFailure, syncSafeMessage(t, err))
	require.Equal(t, UpstreamModelSyncErrorInternal, syncKind(t, err))
	require.NotContains(t, syncSafeMessage(t, err), "database unavailable")
	snap := grokCatalogStoredSnapshot(t, repo, account.ID)
	require.Equal(t, []string{"grok-old"}, snap.Models)
}

func TestGrokCatalogSyncReportsMetadataSaveFailureWithoutRollback(t *testing.T) {
	account := grokCatalogAccountWithSuccess([]string{"grok-old"}, time.Now().Add(-time.Hour).UTC())
	repo := &upstreamModelMetadataRepoStub{
		account: account,
		err:     errors.New("metadata database unavailable"),
	}
	body := `{"data":[{"model":"grok-4.7","reasoning":true,"supported_reasoning_levels":["low","high"],"input_modalities":["text"],"context_window":128000}]}`
	upstream := newGrokCatalogUpstream(http.StatusOK, body, http.StatusOK, `{}`)
	svc := grokCatalogSyncService(repo, upstream)

	catalog, err := svc.SyncUpstreamModelCatalog(context.Background(), account)
	require.Nil(t, catalog)
	require.Equal(t, grokCatalogMetadataFailure, syncSafeMessage(t, err))
	require.Equal(t, UpstreamModelSyncErrorInternal, syncKind(t, err))
	snap := parseGrokObservedModels(account.Extra)
	require.Equal(t, []string{"grok-4.7"}, snap.Models)
	require.Equal(t, grokCatalogStatusSuccess, snap.LastAttempt.Status)
	require.NotEqual(t, "metadata database unavailable", syncSafeMessage(t, err))
	require.Greater(t, repo.casCalls, 0)
}

func TestGrokCatalogSyncPreviewAndPureReadDoNotPersist(t *testing.T) {
	account := grokOAuthModelSyncTestAccount("https://cli-chat-proxy.grok.com/v1")
	account.ID = 0
	repo := newGrokCatalogMemoryRepo(grokOAuthModelSyncTestAccount("https://cli-chat-proxy.grok.com/v1"))
	upstream := newGrokCatalogUpstream(http.StatusOK, `{"data":[{"model":"grok-4.7"}]}`, http.StatusBadGateway, `{"error":"registry-down"}`)
	svc := grokCatalogSyncService(repo, upstream)

	catalog, err := svc.SyncUpstreamModelCatalog(context.Background(), account)
	require.NoError(t, err)
	require.Equal(t, "upstream", catalog.DiscoverySource)
	require.Equal(t, []string{"grok-4.7"}, catalog.Models)
	require.Zero(t, repo.casCalls)
	require.Nil(t, parseGrokObservedModels(account.Extra))

	readAccount := grokOAuthModelSyncTestAccount("https://cli-chat-proxy.grok.com/v1")
	readRepo := newGrokCatalogMemoryRepo(readAccount)
	readUpstream := newGrokCatalogUpstream(http.StatusOK, `{"data":[{"id":"display","model":"grok-4.7"}]}`, http.StatusOK, `{}`)
	readSvc := grokCatalogSyncService(readRepo, readUpstream)
	before := len(readUpstream.requests)
	result, err := readSvc.fetchUpstreamModelListWithObservation(context.Background(), readAccount)
	require.NoError(t, err)
	require.Equal(t, []string{"grok-4.7"}, result.Models)
	require.NotNil(t, result.GrokObservation)
	require.Equal(t, []string{"grok-4.7"}, result.GrokObservation.Models)
	require.Equal(t, grokCatalogStatusSuccess, result.GrokObservation.Attempt.Status)
	require.Equal(t, "https://cli-chat-proxy.grok.com/v1/models", result.GrokObservation.Attempt.Scope.Endpoint)
	models, fetchErr := readSvc.FetchUpstreamSupportedModels(context.Background(), readAccount)
	require.NoError(t, fetchErr)
	require.Equal(t, []string{"grok-4.7"}, models)
	require.Zero(t, readRepo.casCalls)
	require.Zero(t, readRepo.updateExtraCalls)
	require.Empty(t, readUpstream.registryRequests())
	require.Len(t, readUpstream.modelsRequests(), 2)
	require.Equal(t, before+2, len(readUpstream.requests))
}

func TestGrokCatalogSyncNonGrokOmitsDiscoverySource(t *testing.T) {
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body: io.NopCloser(strings.NewReader(`{"data":[{
			"id":"gpt-test",
			"reasoning":false,
			"input_modalities":["text"],
			"context_window":128000
		}]}`)),
	}}
	repo := &upstreamModelMetadataRepoStub{}
	svc := &AccountTestService{accountRepo: repo, httpUpstream: upstream, cfg: upstreamModelSyncTestConfig()}
	catalog, err := svc.SyncUpstreamModelCatalog(context.Background(), &Account{
		ID: 44, Platform: PlatformOpenAI, Type: AccountTypeAPIKey,
		Credentials: map[string]any{"api_key": "key", "base_url": "https://provider.example/v1"},
	})
	require.NoError(t, err)
	require.Empty(t, catalog.DiscoverySource)
	require.NotContains(t, grokCatalogJSON(t, catalog), "discovery_source")
	require.Zero(t, repo.casCalls)
}

func TestGrokCatalogSyncManualIgnoresFreshTTL(t *testing.T) {
	account := grokOAuthModelSyncTestAccount("https://cli-chat-proxy.grok.com/v1")
	scope, err := NewGrokCatalogScope(account.ID, account.Type, "https://cli-chat-proxy.grok.com/v1/models")
	require.NoError(t, err)
	seedGrokCatalogSuccess(account, []string{"grok-old"}, time.Now().Add(-time.Minute).UTC(), scope)
	repo := newGrokCatalogMemoryRepo(account)
	upstream := newGrokCatalogUpstream(http.StatusOK, `{"data":[{"model":"grok-4.7"}]}`, http.StatusBadGateway, `{"error":"registry-down"}`)
	svc := grokCatalogSyncService(repo, upstream)

	catalog, err := svc.SyncUpstreamModelCatalog(context.Background(), account)
	require.NoError(t, err)
	require.Equal(t, []string{"grok-4.7"}, catalog.Models)
	require.Equal(t, []string{"grok-4.7"}, grokCatalogStoredSnapshot(t, repo, account.ID).Models)
	require.Len(t, upstream.modelsRequests(), 1)
}

func TestGrokCatalogSyncConfigErrorsDoNotFabricateSuccess(t *testing.T) {
	t.Run("missing token provider", func(t *testing.T) {
		account := grokCatalogAccountWithSuccess([]string{"grok-4.7"}, time.Now().Add(-time.Hour).UTC())
		delete(account.Credentials, "access_token")
		previous := parseGrokObservedModels(account.Extra)
		repo := newGrokCatalogMemoryRepo(account)
		upstream := newGrokCatalogUpstream(http.StatusOK, `{"data":[{"model":"grok-new"}]}`, http.StatusOK, `{}`)
		svc := &AccountTestService{accountRepo: repo, httpUpstream: upstream, cfg: upstreamModelSyncTestConfig()}

		_, err := svc.SyncUpstreamModelCatalog(context.Background(), account)
		require.Equal(t, "Grok token provider is not configured", syncSafeMessage(t, err))
		require.Equal(t, UpstreamModelSyncErrorConfiguration, syncKind(t, err))
		require.Empty(t, upstream.requests)
		snap := grokCatalogStoredSnapshot(t, repo, account.ID)
		requireGrokSuccessPreserved(t, previous, snap)
		require.Equal(t, grokCatalogStatusFailed, snap.LastAttempt.Status)
		require.Equal(t, "configuration", snap.LastAttempt.ErrorCode)
		require.Nil(t, snap.LastAttempt.Scope)
		require.NotContains(t, grokCatalogJSON(t, snap), grokCatalogSyncSecretRefresh)
	})

	t.Run("invalid base url", func(t *testing.T) {
		account := grokCatalogAccountWithSuccess([]string{"grok-4.7"}, time.Now().Add(-time.Hour).UTC())
		account.Credentials["base_url"] = "http://insecure.example.test/v1"
		previous := parseGrokObservedModels(account.Extra)
		repo := newGrokCatalogMemoryRepo(account)
		upstream := newGrokCatalogUpstream(http.StatusOK, `{"data":[{"model":"grok-new"}]}`, http.StatusOK, `{}`)
		svc := grokCatalogSyncService(repo, upstream)

		_, err := svc.SyncUpstreamModelCatalog(context.Background(), account)
		require.Equal(t, "Invalid Grok base URL", syncSafeMessage(t, err))
		require.Equal(t, UpstreamModelSyncErrorConfiguration, syncKind(t, err))
		require.Empty(t, upstream.requests)
		snap := grokCatalogStoredSnapshot(t, repo, account.ID)
		requireGrokSuccessPreserved(t, previous, snap)
		require.Equal(t, grokCatalogStatusFailed, snap.LastAttempt.Status)
		require.Equal(t, "configuration", snap.LastAttempt.ErrorCode)
		require.Nil(t, snap.LastAttempt.Scope)
	})
}

func TestGrokCatalogSyncObservationTimeIsStableAcrossCASRetry(t *testing.T) {
	account := grokOAuthModelSyncTestAccount("https://cli-chat-proxy.grok.com/v1")
	inner := newGrokCatalogMemoryRepo(account)
	inner.conflictsRemaining = 1
	repo := &grokCatalogCASRecordRepo{grokCatalogMemoryRepo: inner}
	upstream := newGrokCatalogUpstream(http.StatusOK, `{"data":[{"model":"grok-4.7"}]}`, http.StatusBadGateway, `{"error":"registry-down"}`)
	svc := grokCatalogSyncService(repo, upstream)

	_, err := svc.SyncUpstreamModelCatalog(context.Background(), account)
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(repo.nextPayloads()), 2)
	first := grokCatalogPayloadTimes(t, repo.nextPayloads()[0])
	second := grokCatalogPayloadTimes(t, repo.nextPayloads()[1])
	require.Equal(t, first, second)
	require.NotEmpty(t, first.started)
}

func TestGrokCatalogSyncKeepsAPIKeyAndCustomURLIdentity(t *testing.T) {
	t.Run("api key uses its own base url", func(t *testing.T) {
		account := &Account{
			ID:       21,
			Platform: PlatformGrok,
			Type:     AccountTypeAPIKey,
			Credentials: map[string]any{
				"api_key":  "xai-key",
				"base_url": "https://xai.example.com/v1",
			},
		}
		repo := newGrokCatalogMemoryRepo(account)
		upstream := newGrokCatalogUpstream(http.StatusOK, `{"data":[{"model":"grok-4.7"}]}`, http.StatusBadGateway, `{"error":"registry-down"}`)
		svc := &AccountTestService{accountRepo: repo, httpUpstream: upstream, cfg: upstreamModelSyncTestConfig()}

		catalog, err := svc.SyncUpstreamModelCatalog(context.Background(), account)
		require.NoError(t, err)
		require.Equal(t, "upstream", catalog.DiscoverySource)
		req := upstream.modelsRequests()[0]
		require.Equal(t, "https://xai.example.com/v1/models", req.URL.String())
		require.Equal(t, "Bearer xai-key", req.Header.Get("Authorization"))
		require.Empty(t, req.Header.Get("X-Grok-Client-Version"))
		require.Empty(t, req.Header.Get("X-UserID"))
		snap := grokCatalogStoredSnapshot(t, repo, account.ID)
		require.Equal(t, "https://xai.example.com/v1/models", snap.Scope.Endpoint)
		require.Equal(t, AccountTypeAPIKey, snap.Scope.AccountType)
	})

	t.Run("custom oauth url does not add cli identity", func(t *testing.T) {
		account := grokOAuthModelSyncTestAccount("https://custom.example.test/v1")
		account.Credentials["header_override_enabled"] = true
		account.Credentials["header_overrides"] = map[string]any{"X-Account-Trace": "trace-1"}
		proxyID := int64(4)
		account.ProxyID = &proxyID
		account.Proxy = &Proxy{Protocol: "http", Host: "proxy.example.test", Port: 8080}
		repo := newGrokCatalogMemoryRepo(account)
		upstream := newGrokCatalogUpstream(http.StatusOK, `{"data":[{"model":"grok-4.7"}]}`, http.StatusBadGateway, `{"error":"registry-down"}`)
		svc := grokCatalogSyncService(repo, upstream)

		_, err := svc.SyncUpstreamModelCatalog(context.Background(), account)
		require.NoError(t, err)
		req := upstream.modelsRequests()[0]
		require.Equal(t, "https://custom.example.test/v1/models", req.URL.String())
		require.Empty(t, req.Header.Get("X-UserID"))
		require.Empty(t, req.Header.Get("X-Email"))
		require.NotEmpty(t, req.Header.Get("X-Grok-Client-Version"))
		require.Equal(t, []string{"trace-1"}, headerValuesEqualFold(req.Header, "X-Account-Trace"))
		require.Equal(t, account.Proxy.URL(), upstream.proxyURLs[0])
		require.Equal(t, "https://custom.example.test/v1/models", grokCatalogStoredSnapshot(t, repo, account.ID).Scope.Endpoint)
	})
}

func TestGrokCatalogBackgroundSkipsOnlyFreshMatchingScope(t *testing.T) {
	account := grokOAuthModelSyncTestAccount("")
	scope, err := NewGrokCatalogScope(account.ID, account.Type, "https://cli-chat-proxy.grok.com/v1/models")
	require.NoError(t, err)
	seedGrokCatalogSuccess(account, []string{"grok-4.7"}, time.Now().Add(-time.Minute).UTC(), scope)
	repo := newGrokCatalogMemoryRepo(account)
	upstream := newGrokCatalogUpstream(http.StatusOK, `{"data":[{"model":"grok-new"}]}`, http.StatusOK, `{}`)
	svc := &GrokQuotaService{accountRepo: repo, httpUpstream: upstream, cfg: upstreamModelSyncTestConfig()}

	require.NoError(t, svc.syncGrokObservedModels(context.Background(), account))
	require.Empty(t, upstream.requests)
	require.Zero(t, repo.casCalls)
	require.Equal(t, []string{"grok-4.7"}, grokCatalogStoredSnapshot(t, repo, account.ID).Models)
}

func TestGrokCatalogBackgroundFetchesWhenSnapshotCannotBeReused(t *testing.T) {
	officialScope := func(account *Account) *GrokCatalogScope {
		t.Helper()
		scope, err := NewGrokCatalogScope(account.ID, account.Type, "https://cli-chat-proxy.grok.com/v1/models")
		require.NoError(t, err)
		return scope
	}
	tests := []struct {
		name  string
		setup func(account *Account)
	}{
		{
			name: "stale",
			setup: func(account *Account) {
				seedGrokCatalogSuccess(account, []string{"grok-old"}, time.Now().Add(-(grokObservedModelsTTL + time.Minute)).UTC(), officialScope(account))
			},
		},
		{
			name: "missing scope",
			setup: func(account *Account) {
				seedGrokCatalogSuccess(account, []string{"grok-old"}, time.Now().Add(-time.Minute).UTC(), nil)
			},
		},
		{
			name: "source changed",
			setup: func(account *Account) {
				other, err := NewGrokCatalogScope(account.ID, account.Type, "https://old.example.test/v1/models")
				require.NoError(t, err)
				seedGrokCatalogSuccess(account, []string{"grok-old"}, time.Now().Add(-time.Minute).UTC(), other)
			},
		},
		{
			name: "future timestamp",
			setup: func(account *Account) {
				seedGrokCatalogSuccess(account, []string{"grok-old"}, time.Now().Add(-time.Hour).UTC(), officialScope(account))
				observed := account.Extra[grokObservedModelsExtraKey].(map[string]any)
				observed["fetched_at"] = time.Now().Add(24 * time.Hour).UTC().Format(time.RFC3339Nano)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			account := grokOAuthModelSyncTestAccount("")
			tt.setup(account)
			repo := newGrokCatalogMemoryRepo(account)
			upstream := newGrokCatalogUpstream(http.StatusOK, `{"data":[{"model":"grok-4.7"}]}`, http.StatusOK, `{}`)
			svc := &GrokQuotaService{accountRepo: repo, httpUpstream: upstream, cfg: upstreamModelSyncTestConfig()}

			require.NoError(t, svc.syncGrokObservedModels(context.Background(), account))
			require.Len(t, upstream.modelsRequests(), 1)
			require.Empty(t, upstream.registryRequests())
			require.Equal(t, 1, repo.casCalls)
			require.Zero(t, repo.updateExtraCalls)
			snap := grokCatalogStoredSnapshot(t, repo, account.ID)
			require.Equal(t, []string{"grok-4.7"}, snap.Models)
			require.Equal(t, "https://cli-chat-proxy.grok.com/v1/models", snap.Scope.Endpoint)
			require.Equal(t, grokCatalogStatusSuccess, snap.LastAttempt.Status)
		})
	}
}

func TestGrokCatalogBackgroundNoTokenAndConfigAreSafe(t *testing.T) {
	t.Run("no token", func(t *testing.T) {
		account := grokCatalogAccountWithSuccess([]string{"grok-4.7"}, time.Now().Add(-time.Hour).UTC())
		delete(account.Credentials, "access_token")
		account.Credentials["refresh_token"] = "refresh-secret-value"
		previous := parseGrokObservedModels(account.Extra)
		repo := newGrokCatalogMemoryRepo(account)
		upstream := newGrokCatalogUpstream(http.StatusOK, `{"data":[{"model":"grok-new"}]}`, http.StatusOK, `{}`)
		svc := &GrokQuotaService{
			accountRepo:   repo,
			httpUpstream:  upstream,
			cfg:           upstreamModelSyncTestConfig(),
			tokenProvider: NewGrokTokenProvider(nil, nil),
		}

		err := svc.syncGrokObservedModels(context.Background(), account)
		require.Equal(t, "No Grok access token is available", syncSafeMessage(t, err))
		require.Equal(t, UpstreamModelSyncErrorConfiguration, syncKind(t, err))
		require.NotContains(t, err.Error(), "refresh-secret-value")
		require.Empty(t, upstream.requests)
		snap := grokCatalogStoredSnapshot(t, repo, account.ID)
		requireGrokSuccessPreserved(t, previous, snap)
		require.Equal(t, grokCatalogStatusFailed, snap.LastAttempt.Status)
		require.Equal(t, "configuration", snap.LastAttempt.ErrorCode)
		require.NotContains(t, grokCatalogJSON(t, snap), "refresh-secret-value")
		require.NotContains(t, grokCatalogJSON(t, snap), grokCatalogSyncSecretRefresh)
	})

	t.Run("rejected custom url", func(t *testing.T) {
		account := grokCatalogAccountWithSuccess([]string{"grok-4.7"}, time.Now().Add(-time.Hour).UTC())
		account.Credentials["base_url"] = "https://blocked.example.test/v1"
		previous := parseGrokObservedModels(account.Extra)
		repo := newGrokCatalogMemoryRepo(account)
		upstream := newGrokCatalogUpstream(http.StatusOK, `{"data":[{"model":"grok-new"}]}`, http.StatusOK, `{}`)
		cfg := upstreamModelSyncTestConfig()
		cfg.Security.URLAllowlist.Enabled = true
		cfg.Security.URLAllowlist.UpstreamHosts = []string{"allowed.example.test"}
		svc := &GrokQuotaService{accountRepo: repo, httpUpstream: upstream, cfg: cfg}

		err := svc.syncGrokObservedModels(context.Background(), account)
		require.ErrorContains(t, err, "base URL rejected by URL security policy")
		require.Equal(t, UpstreamModelSyncErrorConfiguration, syncKind(t, err))
		require.NotContains(t, err.Error(), grokCatalogSyncSecretToken)
		require.Empty(t, upstream.requests)
		snap := grokCatalogStoredSnapshot(t, repo, account.ID)
		requireGrokSuccessPreserved(t, previous, snap)
		require.Equal(t, grokCatalogStatusFailed, snap.LastAttempt.Status)
		require.Equal(t, "configuration", snap.LastAttempt.ErrorCode)
		require.Nil(t, snap.LastAttempt.Scope)
	})
}

func TestGrokCatalogBackgroundKeepsSuccessOnHTTPAndBodyFailures(t *testing.T) {
	t.Run("http", func(t *testing.T) {
		for _, status := range []int{http.StatusForbidden, http.StatusTooManyRequests, http.StatusInternalServerError} {
			t.Run(http.StatusText(status), func(t *testing.T) {
				account := grokCatalogAccountWithSuccess([]string{"grok-4.7"}, time.Now().Add(-(grokObservedModelsTTL + time.Hour)).UTC())
				previous := parseGrokObservedModels(account.Extra)
				repo := newGrokCatalogMemoryRepo(account)
				upstream := newGrokCatalogUpstream(status, `{"error":"hidden-upstream-body"}`, http.StatusOK, `{}`)
				svc := &GrokQuotaService{accountRepo: repo, httpUpstream: upstream, cfg: upstreamModelSyncTestConfig()}

				err := svc.syncGrokObservedModels(context.Background(), account)
				requireGrokHTTPSyncError(t, err, status)
				snap := grokCatalogStoredSnapshot(t, repo, account.ID)
				requireGrokSuccessPreserved(t, previous, snap)
				require.Equal(t, grokCatalogStatusFailed, snap.LastAttempt.Status)
				require.Equal(t, "http_error", snap.LastAttempt.ErrorCode)
				require.NotContains(t, grokCatalogJSON(t, snap), "hidden-upstream-body")
			})
		}
	})

	t.Run("body", func(t *testing.T) {
		tests := []struct {
			name    string
			body    string
			limit   int64
			message string
			code    string
		}{
			{name: "empty", body: `{"data":[]}`, message: "Upstream returned no supported models", code: "empty_catalog"},
			{name: "malformed", body: `not-json`, message: "Upstream model list response was not valid JSON", code: "invalid_response"},
			{
				name:    "too large",
				body:    `{"data":[{"model":"grok-overflow"}]}` + strings.Repeat(" ", 80),
				limit:   int64(len(`{"data":[{"model":"grok-overflow"}]}`)),
				message: "Upstream model list response is too large",
				code:    "too_large",
			},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				account := grokCatalogAccountWithSuccess([]string{"grok-4.7"}, time.Now().Add(-(grokObservedModelsTTL + time.Hour)).UTC())
				previous := parseGrokObservedModels(account.Extra)
				repo := newGrokCatalogMemoryRepo(account)
				upstream := newGrokCatalogUpstream(http.StatusOK, tt.body, http.StatusOK, `{}`)
				cfg := upstreamModelSyncTestConfig()
				if tt.limit > 0 {
					cfg.Gateway.ModelsListReadMaxBytes = tt.limit
				}
				svc := &GrokQuotaService{accountRepo: repo, httpUpstream: upstream, cfg: cfg}

				err := svc.syncGrokObservedModels(context.Background(), account)
				require.Equal(t, tt.message, syncSafeMessage(t, err))
				snap := grokCatalogStoredSnapshot(t, repo, account.ID)
				requireGrokSuccessPreserved(t, previous, snap)
				require.Equal(t, tt.code, snap.LastAttempt.ErrorCode)
				require.NotContains(t, snap.Models, "grok-overflow")
			})
		}
	})
}

func TestGrokCatalogBackgroundUsesAccountProxy(t *testing.T) {
	account := grokOAuthModelSyncTestAccount("")
	proxyID := int64(8)
	account.ProxyID = &proxyID
	repo := newGrokCatalogMemoryRepo(account)
	upstream := newGrokCatalogUpstream(http.StatusOK, `{"data":[{"model":"grok-4.7"}]}`, http.StatusOK, `{}`)
	proxy := &Proxy{ID: proxyID, Protocol: "http", Host: "proxy.example.test", Port: 9090}
	svc := &GrokQuotaService{
		accountRepo:  repo,
		httpUpstream: upstream,
		cfg:          upstreamModelSyncTestConfig(),
		proxyRepo:    grokCatalogProxyRepo{proxy: proxy},
	}

	require.NoError(t, svc.syncGrokObservedModels(context.Background(), account))
	require.Equal(t, proxy.URL(), upstream.proxyURLs[0])
	require.Equal(t, "grok-user-id", upstream.modelsRequests()[0].Header.Get("X-UserID"))
}

func TestGrokCatalogBackgroundLateResultDoesNotOverwriteNewerSuccess(t *testing.T) {
	account := grokOAuthModelSyncTestAccount("https://cli-chat-proxy.grok.com/v1")
	repo := newGrokCatalogMemoryRepo(account)
	started := make(chan struct{}, 1)
	release := make(chan struct{})
	upstream := newGrokCatalogUpstream(http.StatusOK, `{"data":[{"model":"older"}]}`, http.StatusBadGateway, `{"error":"registry-down"}`)
	upstream.blockFirst = true
	upstream.started = started
	upstream.release = release
	upstream.secondBody = `{"data":[{"model":"newer"}]}`

	stale := cloneGrokCatalogMemoryAccount(account)
	done := make(chan error, 1)
	go func() {
		done <- (&GrokQuotaService{
			accountRepo:  repo,
			httpUpstream: upstream,
			cfg:          upstreamModelSyncTestConfig(),
		}).syncGrokObservedModels(context.Background(), stale)
	}()

	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("older catalog request did not start")
	}

	manualUpstreamAccount := cloneGrokCatalogMemoryAccount(account)
	_, err := grokCatalogSyncService(repo, upstream).SyncUpstreamModelCatalog(context.Background(), manualUpstreamAccount)
	require.NoError(t, err)
	close(release)

	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("older catalog request did not finish")
	}

	snap := grokCatalogStoredSnapshot(t, repo, account.ID)
	require.Equal(t, []string{"newer"}, snap.Models)
	require.NotContains(t, snap.Models, "older")
	require.Equal(t, []string{"newer"}, parseGrokObservedModels(manualUpstreamAccount.Extra).Models)
}

type grokCatalogUpstream struct {
	mu           sync.Mutex
	modelsStatus int
	modelsBody   string
	secondBody   string
	modelsSeen   int
	registryCode int
	registryBody string
	requests     []*http.Request
	proxyURLs    []string
	blockFirst   bool
	started      chan struct{}
	release      chan struct{}
}

func newGrokCatalogUpstream(modelsStatus int, modelsBody string, registryCode int, registryBody string) *grokCatalogUpstream {
	return &grokCatalogUpstream{
		modelsStatus: modelsStatus,
		modelsBody:   modelsBody,
		registryCode: registryCode,
		registryBody: registryBody,
	}
}

func (u *grokCatalogUpstream) Do(req *http.Request, proxyURL string, _ int64, _ int) (*http.Response, error) {
	u.mu.Lock()
	u.requests = append(u.requests, req)
	u.proxyURLs = append(u.proxyURLs, proxyURL)
	registry := req.URL != nil && req.URL.Host == "models.dev"
	body := u.modelsBody
	status := u.modelsStatus
	block := false
	if !registry {
		u.modelsSeen++
		if u.modelsSeen == 1 {
			block = u.blockFirst
		}
		if u.modelsSeen > 1 && u.secondBody != "" {
			body = u.secondBody
			status = http.StatusOK
		}
	} else {
		body = u.registryBody
		status = u.registryCode
	}
	started := u.started
	release := u.release
	u.mu.Unlock()

	if registry {
		return grokCatalogHTTPResponse(status, body), nil
	}
	if block {
		started <- struct{}{}
		<-release
	}
	return grokCatalogHTTPResponse(status, body), nil
}

func (u *grokCatalogUpstream) DoWithTLS(req *http.Request, proxyURL string, accountID int64, accountConcurrency int, _ *tlsfingerprint.Profile) (*http.Response, error) {
	return u.Do(req, proxyURL, accountID, accountConcurrency)
}

func (u *grokCatalogUpstream) modelsRequests() []*http.Request {
	u.mu.Lock()
	defer u.mu.Unlock()
	return grokCatalogFilterRequests(u.requests, false)
}

func (u *grokCatalogUpstream) registryRequests() []*http.Request {
	u.mu.Lock()
	defer u.mu.Unlock()
	return grokCatalogFilterRequests(u.requests, true)
}

func grokCatalogFilterRequests(requests []*http.Request, registry bool) []*http.Request {
	filtered := make([]*http.Request, 0)
	for _, req := range requests {
		if req == nil || req.URL == nil {
			continue
		}
		if (req.URL.Host == "models.dev") == registry {
			filtered = append(filtered, req)
		}
	}
	return filtered
}

func grokCatalogHTTPResponse(status int, body string) *http.Response {
	if status == 0 {
		status = http.StatusOK
	}
	return &http.Response{
		StatusCode: status,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

type grokCatalogGetter interface {
	GetByID(context.Context, int64) (*Account, error)
}

func grokCatalogSyncService(repo AccountRepository, upstream HTTPUpstream) *AccountTestService {
	return &AccountTestService{
		accountRepo:       repo,
		httpUpstream:      upstream,
		cfg:               upstreamModelSyncTestConfig(),
		grokTokenProvider: NewGrokTokenProvider(nil, nil),
	}
}

func grokCatalogAccountWithSuccess(models []string, fetched time.Time) *Account {
	account := grokOAuthModelSyncTestAccount("https://cli-chat-proxy.grok.com/v1")
	scope, err := NewGrokCatalogScope(account.ID, account.Type, "https://cli-chat-proxy.grok.com/v1/models")
	if err != nil {
		panic(err)
	}
	seedGrokCatalogSuccess(account, models, fetched, scope)
	return account
}

func seedGrokCatalogSuccess(account *Account, models []string, fetched time.Time, scope *GrokCatalogScope) {
	fetchedAt := fetched.UTC().Format(time.RFC3339Nano)
	startedAt := fetched.Add(-time.Second).UTC().Format(time.RFC3339Nano)
	snap := GrokObservedModelsSnapshot{
		Models:           append([]string(nil), models...),
		FetchedAt:        fetchedAt,
		SuccessStartedAt: startedAt,
		Source:           "upstream_v1_models",
		Scope:            scope,
		LastAttempt: &GrokCatalogAttempt{
			StartedAt:  startedAt,
			FinishedAt: fetchedAt,
			Status:     grokCatalogStatusSuccess,
			Scope:      scope,
		},
	}
	raw, err := json.Marshal(snap)
	if err != nil {
		panic(err)
	}
	var asMap map[string]any
	if err := json.Unmarshal(raw, &asMap); err != nil {
		panic(err)
	}
	if account.Extra == nil {
		account.Extra = map[string]any{}
	}
	account.Extra[grokObservedModelsExtraKey] = asMap
}

func grokCatalogStoredSnapshot(t *testing.T, repo grokCatalogGetter, id int64) *GrokObservedModelsSnapshot {
	t.Helper()
	stored, err := repo.GetByID(context.Background(), id)
	require.NoError(t, err)
	snap := parseGrokObservedModels(stored.Extra)
	require.NotNil(t, snap)
	return snap
}

func requireGrokSuccessPreserved(t *testing.T, previous, snap *GrokObservedModelsSnapshot) {
	t.Helper()
	require.Equal(t, previous.Models, snap.Models)
	require.Equal(t, previous.FetchedAt, snap.FetchedAt)
	require.Equal(t, previous.SuccessStartedAt, snap.SuccessStartedAt)
	require.Equal(t, previous.Scope.Fingerprint, snap.Scope.Fingerprint)
}

func requireGrokHTTPSyncError(t *testing.T, err error, status int) {
	t.Helper()
	require.Equal(t, "Upstream model list request failed with HTTP "+strconv.Itoa(status), syncSafeMessage(t, err))
	require.Equal(t, UpstreamModelSyncErrorUpstream, syncKind(t, err))
	require.Equal(t, status, upstreamModelSyncStatusCode(err))
}

func syncSafeMessage(t *testing.T, err error) string {
	t.Helper()
	var syncErr *UpstreamModelSyncError
	require.ErrorAs(t, err, &syncErr)
	return syncErr.SafeMessage()
}

func syncKind(t *testing.T, err error) UpstreamModelSyncErrorKind {
	t.Helper()
	var syncErr *UpstreamModelSyncError
	require.ErrorAs(t, err, &syncErr)
	return syncErr.Kind
}

func grokCatalogJSON(t *testing.T, value any) string {
	t.Helper()
	raw, err := json.Marshal(value)
	require.NoError(t, err)
	return string(raw)
}

type grokCatalogCASErrorRepo struct {
	*grokCatalogMemoryRepo
	err error
}

func (r *grokCatalogCASErrorRepo) CompareAndSwapGrokObservedModels(ctx context.Context, id int64, expected, next json.RawMessage) (bool, error) {
	if r.err != nil {
		return false, r.err
	}
	return r.grokCatalogMemoryRepo.CompareAndSwapGrokObservedModels(ctx, id, expected, next)
}

type grokCatalogCASRecordRepo struct {
	*grokCatalogMemoryRepo
	mu    sync.Mutex
	nexts []json.RawMessage
}

func (r *grokCatalogCASRecordRepo) CompareAndSwapGrokObservedModels(ctx context.Context, id int64, expected, next json.RawMessage) (bool, error) {
	r.mu.Lock()
	r.nexts = append(r.nexts, append(json.RawMessage(nil), next...))
	r.mu.Unlock()
	return r.grokCatalogMemoryRepo.CompareAndSwapGrokObservedModels(ctx, id, expected, next)
}

func (r *grokCatalogCASRecordRepo) nextPayloads() []json.RawMessage {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]json.RawMessage(nil), r.nexts...)
}

type grokCatalogPayloadClock struct {
	started  string
	finished string
	success  string
}

func grokCatalogPayloadTimes(t *testing.T, raw json.RawMessage) grokCatalogPayloadClock {
	t.Helper()
	var payload struct {
		SuccessStartedAt string `json:"success_started_at"`
		LastAttempt      struct {
			StartedAt  string `json:"started_at"`
			FinishedAt string `json:"finished_at"`
		} `json:"last_attempt"`
	}
	require.NoError(t, json.Unmarshal(raw, &payload))
	return grokCatalogPayloadClock{
		started:  payload.LastAttempt.StartedAt,
		finished: payload.LastAttempt.FinishedAt,
		success:  payload.SuccessStartedAt,
	}
}

type grokCatalogProxyRepo struct {
	ProxyRepository
	proxy *Proxy
}

func (r grokCatalogProxyRepo) GetByID(context.Context, int64) (*Proxy, error) {
	return r.proxy, nil
}
