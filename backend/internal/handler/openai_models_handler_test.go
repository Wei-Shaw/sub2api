package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func performOrdinaryPinnedModelsRequest(t *testing.T, codex *OpenAIGatewayHandler, group *service.Group, path, etag string) *httptest.ResponseRecorder {
	t.Helper()
	h := &GatewayHandler{openAIGatewayService: codex.gatewayService, maxAccountSwitches: 3}
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, path, nil)
	c.Request.Header.Set("If-None-Match", etag)
	c.Set(string(middleware2.ContextKeyAPIKey), &service.APIKey{GroupID: &group.ID, Group: group})
	h.Models(c)
	return recorder
}

func ordinaryPinnedModelIDs(t *testing.T, recorder *httptest.ResponseRecorder) []string {
	t.Helper()
	var response gatewayModelsResponseForTest
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &response))
	require.NotNil(t, response.Data, "an empty catalog must be an array, not null")
	return modelIDsForTest(response.Data)
}

func TestOrdinaryPinnedModelsUsesSelectedAccountsAndFinalETag(t *testing.T) {
	accounts := []service.Account{
		newPinnedCodexAccount(1, service.StatusActive, true, false),
		newPinnedCodexAccount(2, service.StatusActive, true, true),
		newPinnedCodexAccount(3, service.StatusActive, true, false),
	}
	upstream := &codexModelsPinnedHTTPUpstream{bodies: map[int64]string{
		1: `{"data":[{"id":"unselected"}]}`,
		2: `{"data":[{"id":"special-model","owned_by":"first","created":123},{"id":"gpt-image-1"}]}`,
		3: `{"data":[{"id":"special-model","owned_by":"second"},{"id":"text-embedding-3-large"}]}`,
	}}
	h := newPinnedCodexTestHandler(accounts, upstream, 3)
	group := &service.Group{ID: 91, Platform: service.PlatformOpenAI,
		CodexModelsManifestConfig: service.GroupCodexModelsManifestConfig{Enabled: true, AccountIDs: []int64{2, 3}}}
	first := performOrdinaryPinnedModelsRequest(t, h, group, "/v1/models", "")
	require.Equal(t, http.StatusOK, first.Code, first.Body.String())
	require.Equal(t, []string{"special-model", "gpt-image-1", "text-embedding-3-large"}, ordinaryPinnedModelIDs(t, first))
	var response gatewayModelsResponseForTest
	require.NoError(t, json.Unmarshal(first.Body.Bytes(), &response))
	require.Equal(t, "model", response.Data[0].Object)
	require.Equal(t, "first", response.Data[0].OwnedBy)
	require.EqualValues(t, 123, response.Data[0].Created)
	require.Equal(t, []int64{2, 3}, upstream.accountIDs())
	alias := performOrdinaryPinnedModelsRequest(t, h, group, "/models", first.Header().Get("ETag"))
	require.Equal(t, http.StatusNotModified, alias.Code)
	require.Empty(t, alias.Body.String())
	require.Equal(t, []int64{2, 3}, upstream.accountIDs(), "fresh requests must not hit upstream")

	otherGroup := *group
	otherGroup.ID++
	otherGroup.ModelAllowlist = service.GroupModelAllowlist{Enabled: true, Models: []string{"text-embedding-3-large", "special-model"}}
	filtered := performOrdinaryPinnedModelsRequest(t, h, &otherGroup, "/v1/models", first.Header().Get("ETag"))
	require.Equal(t, http.StatusOK, filtered.Code)
	require.Equal(t, []string{"text-embedding-3-large", "special-model"}, ordinaryPinnedModelIDs(t, filtered))
	require.NotEqual(t, first.Header().Get("ETag"), filtered.Header().Get("ETag"))
	again := performOrdinaryPinnedModelsRequest(t, h, group, "/v1/models", "")
	require.Equal(t, first.Body.String(), again.Body.String(), "group filters must not modify account cache")
	require.Equal(t, []int64{2, 3}, upstream.accountIDs())
}

func TestOrdinaryPinnedModelsFailureAndEmptyPolicies(t *testing.T) {
	for _, tc := range []struct {
		name       string
		accountIDs []int64
		bodies     map[int64]string
		statuses   map[int64]int
		fallback   bool
		selected   []string
		wantStatus int
		wantIDs    []string
	}{
		{name: "partial failure", accountIDs: []int64{2, 3}, bodies: map[int64]string{2: `{"data":[{"id":"ok"}]}`}, statuses: map[int64]int{3: 503}, wantStatus: 200, wantIDs: []string{"ok"}},
		{name: "all failed", accountIDs: []int64{2, 3}, statuses: map[int64]int{2: 429, 3: 504}, wantStatus: 502},
		{name: "missing members", accountIDs: []int64{99}, wantStatus: 503},
		{name: "empty config", wantStatus: 503},
		{name: "fallback from missing members", accountIDs: []int64{99}, fallback: true, wantStatus: 200, wantIDs: []string{"from-scheduler"}},
		{name: "fallback from failure", accountIDs: []int64{2}, statuses: map[int64]int{2: 503}, fallback: true, wantStatus: 200, wantIDs: []string{"from-scheduler"}},
		{name: "valid empty", accountIDs: []int64{2}, bodies: map[int64]string{2: `{"data":[]}`}, fallback: true, wantStatus: 200},
		{name: "filtered empty", accountIDs: []int64{2}, bodies: map[int64]string{2: `{"data":[{"id":"ok"}]}`}, selected: []string{"absent"}, fallback: true, wantStatus: 200},
		{name: "invalid envelope", accountIDs: []int64{2}, bodies: map[int64]string{2: `{"error":"denied"}`}, wantStatus: 502},
		{name: "null data", accountIDs: []int64{2}, bodies: map[int64]string{2: `{"data":null}`}, wantStatus: 502},
	} {
		t.Run(tc.name, func(t *testing.T) {
			accounts := []service.Account{
				newPinnedCodexAccount(1, service.StatusActive, true, false),
				newPinnedCodexAccount(2, service.StatusActive, true, false),
				newPinnedCodexAccount(3, service.StatusActive, true, false),
			}
			bodies := map[int64]string{1: `{"data":[{"id":"from-scheduler"}]}`}
			for id, body := range tc.bodies {
				bodies[id] = body
			}
			upstream := &codexModelsPinnedHTTPUpstream{bodies: bodies, statuses: tc.statuses}
			h := newPinnedCodexTestHandler(accounts, upstream, 3)
			group := &service.Group{ID: 92, Platform: service.PlatformOpenAI,
				ModelAllowlist:            service.GroupModelAllowlist{Enabled: len(tc.selected) > 0, Models: tc.selected},
				CodexModelsManifestConfig: service.GroupCodexModelsManifestConfig{Enabled: true, AccountIDs: tc.accountIDs, FallbackToScheduler: tc.fallback}}
			recorder := performOrdinaryPinnedModelsRequest(t, h, group, "/v1/models", "")
			require.Equal(t, tc.wantStatus, recorder.Code, recorder.Body.String())
			if tc.wantStatus == 200 {
				require.Equal(t, len(tc.wantIDs), len(ordinaryPinnedModelIDs(t, recorder)))
				if len(tc.wantIDs) > 0 {
					require.Equal(t, tc.wantIDs, ordinaryPinnedModelIDs(t, recorder))
				} else {
					require.NotContains(t, upstream.accountIDs(), int64(1), "empty success must not trigger fallback")
				}
			}
		})
	}
}

func TestOrdinaryPinnedModelsSkipsPersistentlyUnavailableAccounts(t *testing.T) {
	accounts := []service.Account{
		newPinnedCodexAccount(1, service.StatusActive, true, true),
		newPinnedCodexAccount(2, service.StatusActive, false, false),
		newPinnedCodexAccount(3, service.StatusDisabled, true, false),
		newPinnedCodexAccount(4, service.StatusActive, true, false),
	}
	expired := time.Now().Add(-time.Hour)
	accounts[3].ExpiresAt = &expired
	accounts[3].AutoPauseOnExpired = true
	overloaded := time.Now().Add(time.Hour)
	accounts[0].OverloadUntil = &overloaded
	accounts[0].TempUnschedulableUntil = &overloaded
	upstream := &codexModelsPinnedHTTPUpstream{bodies: map[int64]string{1: `{"data":[{"id":"available"}]}`}}
	h := newPinnedCodexTestHandler(accounts, upstream, 3)
	group := &service.Group{ID: 93, Platform: service.PlatformOpenAI,
		CodexModelsManifestConfig: service.GroupCodexModelsManifestConfig{Enabled: true, AccountIDs: []int64{1, 2, 3, 4, 99}}}
	recorder := performOrdinaryPinnedModelsRequest(t, h, group, "/v1/models", "")
	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	require.Equal(t, []int64{1}, upstream.accountIDs())
}

func TestPinnedModelsAllowlistExpandsWildcardsForBothRepresentations(t *testing.T) {
	for _, codex := range []bool{false, true} {
		t.Run(map[bool]string{false: "ordinary", true: "codex"}[codex], func(t *testing.T) {
			accounts := []service.Account{newPinnedCodexAccount(1, service.StatusActive, true, false)}
			accounts[0].Credentials["model_mapping"] = map[string]any{
				"public-a": "gpt-5.5", "public-b": "gpt-5.5", "blocked": "gpt-5.5",
			}
			body := `{"data":[{"id":"gpt-5.5","owned_by":"provider"}]}`
			if codex {
				body = `{"models":[{"slug":"gpt-5.5","context_window":424242}]}`
			}
			upstream := &codexModelsPinnedHTTPUpstream{bodies: map[int64]string{1: body}}
			h := newPinnedCodexTestHandler(accounts, upstream, 3)
			group := &service.Group{ID: 95, Platform: service.PlatformOpenAI,
				ModelAllowlist:            service.GroupModelAllowlist{Enabled: true, Models: []string{"public-b", "public-*"}},
				CodexModelsManifestConfig: service.GroupCodexModelsManifestConfig{Enabled: true, AccountIDs: []int64{1}}}
			if codex {
				recorder := performPinnedCodexModelsRequest(t, h, group, "")
				require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
				require.Equal(t, []string{"public-b", "public-a"}, codexHandlerManifestSlugs(t, recorder))
				require.Contains(t, recorder.Body.String(), `"context_window":424242`)
			} else {
				recorder := performOrdinaryPinnedModelsRequest(t, h, group, "/v1/models", "")
				require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
				require.Equal(t, []string{"public-b", "public-a"}, ordinaryPinnedModelIDs(t, recorder))
				require.Contains(t, recorder.Body.String(), `"owned_by":"provider"`)
			}
		})
	}
}

func TestPinnedModelsMappingFollowsUpstreamDiscoveryForBothRepresentations(t *testing.T) {
	for _, codex := range []bool{false, true} {
		t.Run(map[bool]string{false: "ordinary", true: "codex"}[codex], func(t *testing.T) {
			accounts := []service.Account{
				newPinnedCodexAccount(1, service.StatusActive, true, false),
				newPinnedCodexAccount(2, service.StatusActive, true, false),
			}
			accounts[0].Credentials["model_mapping"] = map[string]any{"unselected-alias": "gpt-5.5"}
			accounts[1].Credentials["model_mapping"] = map[string]any{
				"public-alias": "gpt-5.5", "missing-alias": "missing-upstream", "custom-*": "gpt-5.5",
			}
			body := `{"data":[{"id":"gpt-5.5","owned_by":"provider","created":123},{"id":"unmapped-upstream"}]}`
			if codex {
				body = `{"models":[{"slug":"gpt-5.5","context_window":424242},{"slug":"unmapped-upstream"}]}`
			}
			upstream := &codexModelsPinnedHTTPUpstream{bodies: map[int64]string{2: body}}
			h := newPinnedCodexTestHandler(accounts, upstream, 3)
			group := &service.Group{ID: 94, Platform: service.PlatformOpenAI,
				ModelAllowlist:            service.GroupModelAllowlist{Enabled: true, Models: []string{"custom-concrete", "public-alias", "unselected-alias", "missing-alias"}},
				CodexModelsManifestConfig: service.GroupCodexModelsManifestConfig{Enabled: true, AccountIDs: []int64{2}}}
			var recorder *httptest.ResponseRecorder
			if codex {
				recorder = performPinnedCodexModelsRequest(t, h, group, "")
			} else {
				recorder = performOrdinaryPinnedModelsRequest(t, h, group, "/v1/models", "")
			}
			require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
			require.Equal(t, []int64{2}, upstream.accountIDs(), "mappings must not bypass pinned discovery")
			if codex {
				require.Equal(t, []string{"custom-concrete", "public-alias"}, codexHandlerManifestSlugs(t, recorder))
				require.Contains(t, recorder.Body.String(), `"context_window":424242`)
			} else {
				require.Equal(t, []string{"custom-concrete", "public-alias"}, ordinaryPinnedModelIDs(t, recorder))
				require.Contains(t, recorder.Body.String(), `"owned_by":"provider"`)
			}
		})
	}
}
