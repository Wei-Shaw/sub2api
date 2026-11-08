package handler

import (
	"encoding/json"
	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/gemini"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestGeminiNativeModelsUsesAccountMappings(t *testing.T) {
	for _, tt := range []struct {
		name                     string
		forced, mixed, allowlist bool
		status                   int
	}{
		{"mixed", false, true, false, 200},
		{"mixed allowlist", false, true, true, 200},
		{"disabled mixed", false, false, false, 503},
		{"forced without mixed opt-in", true, false, false, 200},
		{"forced allowlist", true, false, true, 200},
	} {
		t.Run(tt.name, func(t *testing.T) {
			groupID := int64(45)
			repo := &geminiAllowlistAccountRepoStub{gatewayModelsAccountRepoStub: gatewayModelsAccountRepoStub{byGroup: map[int64][]service.Account{
				groupID: {{ID: 1, Platform: service.PlatformAntigravity,
					Extra:       map[string]any{"mixed_scheduling": tt.mixed},
					Credentials: map[string]any{"model_mapping": map[string]any{"gemini-synced-custom": "gemini-3.8-flash-high", "claude-custom": "claude-sonnet-4-6"}}}},
			}}}
			h := &GatewayHandler{geminiCompatService: service.NewGeminiMessagesCompatService(repo, nil, nil, nil, nil, nil, nil, nil, nil)}
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest(http.MethodGet, "/v1beta/models", nil)
			c.Set(string(middleware.ContextKeyAPIKey), &service.APIKey{GroupID: &groupID, Group: &service.Group{ID: groupID, Platform: service.PlatformGemini,
				ModelAllowlist: service.GroupModelAllowlist{Enabled: tt.allowlist, Models: []string{"gemini-synced-custom"}},
			}})
			if tt.forced {
				c.Set(string(middleware.ContextKeyForcePlatform), service.PlatformAntigravity)
			}
			h.GeminiV1BetaListModels(c)
			require.Equal(t, tt.status, rec.Code, rec.Body.String())
			if tt.status != 200 {
				return
			}
			var got gemini.ModelsListResponse
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
			names := []string{}
			for _, model := range got.Models {
				names = append(names, model.Name)
				require.Contains(t, model.SupportedGenerationMethods, "generateContent")
			}
			require.Contains(t, names, "models/gemini-synced-custom")
			require.NotContains(t, names, "models/claude-custom")
			require.NotContains(t, names, "models/gemini-2.0-flash")
			if tt.allowlist {
				require.Equal(t, []string{"models/gemini-synced-custom"}, names)
			}
		})
	}
}

func TestAppendUpstreamGeminiModelsPreservesMetadata(t *testing.T) {
	body := []byte(`{"models":[{"name":"models/gemini-native","inputTokenLimit":123,"custom":{"a":true}}],"nextPageToken":"next","other":42}`)
	extra := []gemini.Model{gemini.FallbackModel("gemini-native"), gemini.FallbackModel("gemini-synced-custom"), gemini.FallbackModel("gemini-synced-custom")}
	merged, ok := appendUpstreamGeminiModels(body, extra)
	require.True(t, ok)
	require.JSONEq(t, `{"models":[{"name":"models/gemini-native","inputTokenLimit":123,"custom":{"a":true}},{"name":"models/gemini-synced-custom","supportedGenerationMethods":["generateContent","streamGenerateContent"]}],"nextPageToken":"next","other":42}`, string(merged))
	filtered, dropped, ok := filterUpstreamGeminiModelsBody(merged, service.GroupModelAllowlist{Enabled: true, Models: []string{"gemini-synced-*"}})
	require.True(t, ok)
	require.True(t, dropped)
	require.NotContains(t, string(filtered), "gemini-native")
	require.Contains(t, string(filtered), "gemini-synced-custom")
	for _, invalid := range []string{`null`, `not-json`, `{"error":"bad"}`, `{"models":{}}`} {
		got, ok := appendUpstreamGeminiModels([]byte(invalid), extra)
		require.False(t, ok)
		require.Equal(t, invalid, string(got))
	}
}

// Exercise the real handler's native-upstream branch, including scope fallback.
type geminiMixedModelsUpstream struct {
	service.HTTPUpstream
	status int
	body   string
}

func (u *geminiMixedModelsUpstream) Do(_ *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
	return &http.Response{StatusCode: u.status, Header: http.Header{"Content-Type": {"application/json"}, "X-Request-Id": {"native-id"}}, Body: io.NopCloser(strings.NewReader(u.body))}, nil
}
func TestGeminiNativeModelsMergesNativeUpstream(t *testing.T) {
	for _, tt := range []struct {
		name           string
		status         int
		body           string
		expectedNative string
	}{
		{"native", 200, `{"models":[{"name":"models/gemini-native","inputTokenLimit":123}],"nextPageToken":"next"}`, "models/gemini-native"},
		{"scope fallback", 403, `{"error":"insufficient authentication scopes"}`, "models/gemini-3.5-flash"},
		{"upstream error", 429, `{"error":"rate limited"}`, ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			id := int64(46)
			repo := &geminiAllowlistAccountRepoStub{gatewayModelsAccountRepoStub: gatewayModelsAccountRepoStub{byGroup: map[int64][]service.Account{id: {
				{ID: 1, Platform: service.PlatformGemini, Type: service.AccountTypeAPIKey, Credentials: map[string]any{"api_key": "test"}},
				{ID: 2, Platform: service.PlatformAntigravity, Extra: map[string]any{"mixed_scheduling": true}, Credentials: map[string]any{"model_mapping": map[string]any{"gemini-synced-custom": "gemini-3.8-flash-high"}}},
			}}}}
			upstream := &geminiMixedModelsUpstream{status: tt.status, body: tt.body}
			h := &GatewayHandler{geminiCompatService: service.NewGeminiMessagesCompatService(repo, nil, nil, nil, nil, nil, upstream, nil, &config.Config{})}
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest(http.MethodGet, "/v1beta/models", nil)
			c.Set(string(middleware.ContextKeyAPIKey), &service.APIKey{GroupID: &id, Group: &service.Group{ID: id, Platform: service.PlatformGemini}})
			h.GeminiV1BetaListModels(c)
			if tt.status == 429 {
				require.Equal(t, 429, rec.Code)
				require.JSONEq(t, tt.body, rec.Body.String())
				return
			}
			require.Equal(t, 200, rec.Code, rec.Body.String())
			require.Contains(t, rec.Body.String(), tt.expectedNative)
			require.Contains(t, rec.Body.String(), "models/gemini-synced-custom")
			if tt.status == 200 {
				require.Contains(t, rec.Body.String(), `"inputTokenLimit":123`)
				require.Contains(t, rec.Body.String(), `"nextPageToken":"next"`)
				require.Equal(t, "native-id", rec.Header().Get("X-Request-Id"))
			}
		})
	}
}

type geminiModelsUpstreamCounter struct {
	service.HTTPUpstream
	calls int
}

func (u *geminiModelsUpstreamCounter) Do(_ *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
	u.calls++
	return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(`{"models":[{"name":"gemini-4-argon-high"}]}`))}, nil
}

type geminiDeclaredModelsFixture struct {
	t        *testing.T
	groupID  int64
	handler  *GatewayHandler
	upstream *geminiModelsUpstreamCounter
}

func newGeminiDeclaredModelsFixture(t *testing.T, accounts []service.Account) *geminiDeclaredModelsFixture {
	groupID := int64(47)
	repo := &geminiAllowlistAccountRepoStub{gatewayModelsAccountRepoStub: gatewayModelsAccountRepoStub{byGroup: map[int64][]service.Account{groupID: accounts}}}
	upstream := &geminiModelsUpstreamCounter{}
	return &geminiDeclaredModelsFixture{
		t:        t,
		groupID:  groupID,
		handler:  &GatewayHandler{geminiCompatService: service.NewGeminiMessagesCompatService(repo, nil, nil, nil, nil, nil, upstream, nil, &config.Config{})},
		upstream: upstream,
	}
}

func (f *geminiDeclaredModelsFixture) context(path, model string, allowlist service.GroupModelAllowlist) (*gin.Context, *httptest.ResponseRecorder) {
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodGet, path, nil)
	if model != "" {
		c.Params = gin.Params{{Key: "model", Value: model}}
	}
	c.Set(string(middleware.ContextKeyAPIKey), &service.APIKey{GroupID: &f.groupID, Group: &service.Group{ID: f.groupID, Platform: service.PlatformGemini, ModelAllowlist: allowlist}})
	return c, rec
}

func (f *geminiDeclaredModelsFixture) list(allowlist service.GroupModelAllowlist) []string {
	c, rec := f.context("/v1beta/models", "", allowlist)
	f.handler.GeminiV1BetaListModels(c)
	require.Equal(f.t, http.StatusOK, rec.Code, rec.Body.String())
	var got gemini.ModelsListResponse
	require.NoError(f.t, json.Unmarshal(rec.Body.Bytes(), &got))
	names := []string{}
	for _, model := range got.Models {
		names = append(names, model.Name)
	}
	return names
}

func (f *geminiDeclaredModelsFixture) get(model string) *httptest.ResponseRecorder {
	c, rec := f.context("/v1beta/models/"+model, model, service.GroupModelAllowlist{})
	f.handler.GeminiV1BetaGetModel(c)
	return rec
}

// 分组内 Gemini 账号都声明了 model_mapping 时，列表与详情都按声明的模型应答，不透传选中账号的上游列表。
func TestGeminiNativeModelsUsesDeclaredMappings(t *testing.T) {
	f := newGeminiDeclaredModelsFixture(t, []service.Account{
		{ID: 1, Platform: service.PlatformGemini, Type: service.AccountTypeAPIKey, Priority: 10, Credentials: map[string]any{"api_key": "a", "model_mapping": map[string]any{"gemini-4-argon": "gemini-4-argon"}}},
		{ID: 2, Platform: service.PlatformGemini, Type: service.AccountTypeAPIKey, Priority: 11, Credentials: map[string]any{"api_key": "b", "model_mapping": map[string]any{"gemini-2.5-flash": "gemini-2.5-flash", "gemini-3.1-pro-preview": "gemini-3.1-pro-preview", "gemini-3.8-*": "gemini-3.8-flash"}}},
		{ID: 3, Platform: service.PlatformAntigravity, Extra: map[string]any{"mixed_scheduling": true}, Credentials: map[string]any{"model_mapping": map[string]any{"gemini-synced-custom": "gemini-3.8-flash-high"}}},
	})

	names := f.list(service.GroupModelAllowlist{})
	require.Subset(t, names, []string{"models/gemini-2.5-flash", "models/gemini-3.1-pro-preview", "models/gemini-3.8-*", "models/gemini-4-argon", "models/gemini-synced-custom"})
	require.NotContains(t, names, "models/gemini-4-argon-high")

	// 白名单与 /v1/models 同口径：通配符映射键展开为白名单中的具体模型。
	require.Equal(t, []string{"models/gemini-2.5-flash", "models/gemini-3.8-flash", "models/gemini-synced-custom"},
		f.list(service.GroupModelAllowlist{Enabled: true, Models: []string{"gemini-2.5-flash", "gemini-3.8-flash", "gemini-synced-custom"}}))

	for model, status := range map[string]int{
		"gemini-4-argon":                     http.StatusOK,
		"gemini-3.8-flash":                   http.StatusOK,
		"gemini-3.1-pro-preview-customtools": http.StatusOK,
		"gemini-synced-custom":               http.StatusOK,
		"gemini-4-argon-high":                http.StatusNotFound,
	} {
		rec := f.get(model)
		require.Equal(t, status, rec.Code, model+": "+rec.Body.String())
		if status == http.StatusOK {
			require.JSONEq(t, `{"name":"models/`+model+`","supportedGenerationMethods":["generateContent","streamGenerateContent"]}`, rec.Body.String())
		} else {
			require.Contains(t, rec.Body.String(), "models/"+model+" is not found")
		}
	}
	require.Zero(t, f.upstream.calls)
}

// 分组内有 Gemini 账号未配置映射（接受任意模型）时，模型接口仍由选中账号的上游应答。
func TestGeminiNativeModelsUnmappedAccountKeepsUpstream(t *testing.T) {
	f := newGeminiDeclaredModelsFixture(t, []service.Account{
		{ID: 1, Platform: service.PlatformGemini, Type: service.AccountTypeAPIKey, Priority: 10, Credentials: map[string]any{"api_key": "a", "model_mapping": map[string]any{"gemini-4-argon": "gemini-4-argon"}}},
		{ID: 2, Platform: service.PlatformGemini, Type: service.AccountTypeAPIKey, Priority: 11, Credentials: map[string]any{"api_key": "b"}},
	})

	require.Equal(t, []string{"gemini-4-argon-high"}, f.list(service.GroupModelAllowlist{}))
	rec := f.get("gemini-2.5-pro")
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.Equal(t, 2, f.upstream.calls)
}
