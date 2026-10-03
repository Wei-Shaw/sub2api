package admin

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

type previewMetadataRepo struct {
	service.AccountRepository
	writes int
}

func (r *previewMetadataRepo) UpdateExtra(context.Context, int64, map[string]any) error {
	r.writes++
	return nil
}

func TestAccountHandlerSyncUpstreamModelsPreviewEdit(t *testing.T) {
	for _, tc := range []struct {
		name      string
		key       string
		proxy     any
		wantKey   string
		wantProxy string
		adaptive  bool
	}{
		{"new key and saved proxy", "new-key", nil, "new-key", "http://saved-proxy.example:8080", false},
		{"blank key keeps saved secret", "", nil, "saved-secret", "http://saved-proxy.example:8080", false},
		{"clear proxy", "new-key", 0, "new-key", "", false},
		{"change proxy", "new-key", 8, "new-key", "http://draft-proxy.example:8081", false},
		{"adaptive protocol uses draft endpoint", "new-key", nil, "new-key", "http://saved-proxy.example:8080", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			proxyID := int64(7)
			svc := &availableModelsAdminService{
				stubAdminService: newStubAdminService(),
				account: service.Account{
					ID: 46, Platform: service.PlatformOpenAI, Type: service.AccountTypeAPIKey,
					Concurrency: 3,
					Credentials: map[string]any{
						"api_key": "saved-secret", "base_url": "https://old.example/v1",
						"header_override_enabled": true,
						"header_overrides":        map[string]any{"X-Test-Header": "preserved"},
						"model_mapping":           map[string]any{"old-model": "old-model"},
					},
					Extra:   map[string]any{"untouched": true},
					ProxyID: &proxyID,
					Proxy:   &service.Proxy{ID: 7, Protocol: "http", Host: "saved-proxy.example", Port: 8080},
				},
			}
			svc.proxies = []service.Proxy{{ID: 8, Protocol: "http", Host: "draft-proxy.example", Port: 8081}}
			if tc.adaptive {
				svc.account.Platform = service.PlatformKimi
				svc.account.Credentials["api_protocol"] = "adaptive"
				svc.account.Credentials["api_base_urls"] = map[string]any{"chat_completions": "https://old.example/v1"}
			}
			before, err := json.Marshal(svc.account)
			require.NoError(t, err)
			upstream := &syncUpstreamHTTPUpstream{resp: &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"application/json"}},
				Body: io.NopCloser(strings.NewReader(`{"models":[{
					"slug":"draft-model", "default_reasoning_level":"high",
					"supported_reasoning_levels":[{"effort":"low"},{"effort":"high"}],
					"input_modalities":["text"], "context_window":128000
				}]}`)),
			}}
			repo := &previewMetadataRepo{}
			router := setupSyncUpstreamModelsRouter(svc, upstream, repo)
			payload := map[string]any{
				"account_id": 46, "platform": "openai", "type": "apikey",
				"base_url": "https://draft.example/v1", "api_key": tc.key,
				"model_mapping": map[string]string{},
			}
			if tc.adaptive {
				payload["platform"] = "kimi"
				payload["api_protocol"] = "adaptive"
				payload["account_mode"] = "payg"
				payload["api_base_urls"] = map[string]string{"chat_completions": "https://draft.example/v1"}
			}
			if tc.proxy != nil {
				payload["proxy_id"] = tc.proxy
			}
			body, err := json.Marshal(payload)
			require.NoError(t, err)
			req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/models/sync-upstream-preview", strings.NewReader(string(body)))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)
			require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
			require.Len(t, upstream.requests, 1)
			require.Equal(t, "https://draft.example/v1/models", upstream.requests[0].URL.String())
			require.Equal(t, "Bearer "+tc.wantKey, upstream.requests[0].Header.Get("Authorization"))
			require.Equal(t, []string{"preserved"}, upstream.requests[0].Header["x-test-header"])
			require.Equal(t, []string{tc.wantProxy}, upstream.proxyURLs)
			require.Equal(t, []int64{0}, upstream.accountIDs)
			require.Zero(t, repo.writes, "preview must not persist complete capability metadata")
			after, err := json.Marshal(svc.account)
			require.NoError(t, err)
			require.JSONEq(t, string(before), string(after), "preview must not mutate the saved account")
			require.NotContains(t, rec.Body.String(), tc.wantKey)
		})
	}
}

func TestAccountHandlerSyncUpstreamModelsPreviewRejectsInvalidDraft(t *testing.T) {
	for _, body := range []string{
		`{"platform":"openai","type":"apikey","api_key":""}`,
		`{"platform":"openai","type":"apikey","api_key":"   "}`,
		`{"account_id":-1,"platform":"openai","type":"apikey","api_key":"key"}`,
		`{"account_id":46,"platform":"anthropic","type":"apikey","api_key":"key"}`,
		`{"account_id":46,"platform":"openai","type":"oauth","api_key":"key"}`,
		`{"account_id":46,"platform":"openai","type":"apikey","proxy_id":-1}`,
	} {
		t.Run(body, func(t *testing.T) {
			svc := &availableModelsAdminService{
				stubAdminService: newStubAdminService(),
				account: service.Account{ID: 46, Platform: service.PlatformOpenAI, Type: service.AccountTypeAPIKey,
					Credentials: map[string]any{"api_key": "saved-secret"}},
			}
			upstream := &syncUpstreamHTTPUpstream{}
			router := setupSyncUpstreamModelsRouter(svc, upstream)
			req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/models/sync-upstream-preview", strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)
			require.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
			require.Empty(t, upstream.requests)
			require.NotContains(t, rec.Body.String(), "saved-secret")
		})
	}
}
