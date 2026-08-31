package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type codexModelsFailoverAccountRepo struct {
	service.AccountRepository
	accounts []service.Account
}

func (r codexModelsFailoverAccountRepo) GetByID(_ context.Context, id int64) (*service.Account, error) {
	for i := range r.accounts {
		if r.accounts[i].ID == id {
			account := r.accounts[i]
			return &account, nil
		}
	}
	return nil, service.ErrNoAvailableAccounts
}

func (r codexModelsFailoverAccountRepo) ListSchedulableByPlatform(_ context.Context, platform string) ([]service.Account, error) {
	accounts := make([]service.Account, 0, len(r.accounts))
	for _, account := range r.accounts {
		if account.Platform == platform {
			accounts = append(accounts, account)
		}
	}
	return accounts, nil
}

func (r codexModelsFailoverAccountRepo) ListSchedulableByGroupID(_ context.Context, _ int64) ([]service.Account, error) {
	return append([]service.Account(nil), r.accounts...), nil
}

func (r codexModelsFailoverAccountRepo) ListByGroup(_ context.Context, _ int64) ([]service.Account, error) {
	return append([]service.Account(nil), r.accounts...), nil
}

type codexModelsFailoverHTTPUpstream struct {
	service.HTTPUpstream
	mu          sync.Mutex
	accountIDs  []int64
	firstErr    error
	firstStatus int
	firstBody   string
	statuses    map[int64]int
}

func (u *codexModelsFailoverHTTPUpstream) Do(_ *http.Request, _ string, accountID int64, _ int) (*http.Response, error) {
	u.mu.Lock()
	u.accountIDs = append(u.accountIDs, accountID)
	u.mu.Unlock()

	status, hasStatus := u.statuses[accountID]
	if accountID == 1 || hasStatus {
		if u.firstErr != nil {
			return nil, u.firstErr
		}
		if u.firstBody != "" && !hasStatus {
			return &http.Response{
				StatusCode: http.StatusOK,
				Status:     "200 OK",
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader(u.firstBody)),
			}, nil
		}
		if !hasStatus {
			status = u.firstStatus
		}
		if status == 0 {
			status = http.StatusServiceUnavailable
		}
		return &http.Response{
			StatusCode: status,
			Status:     http.StatusText(status),
			Header:     make(http.Header),
			Body: io.NopCloser(strings.NewReader(
				`{"error":{"message":"No available OpenAI accounts","type":"upstream_error"}}`,
			)),
		}, nil
	}
	return &http.Response{
		StatusCode: http.StatusOK,
		Status:     "200 OK",
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(`{"models":[{"slug":"gpt-5.6-sol"}]}`)),
	}, nil
}

func (u *codexModelsFailoverHTTPUpstream) calls() []int64 {
	u.mu.Lock()
	defer u.mu.Unlock()
	return append([]int64(nil), u.accountIDs...)
}

func TestCodexModelsCanceledRequestDoesNotWriteResponse(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/models", nil).WithContext(ctx)

	h := &OpenAIGatewayHandler{}
	h.CodexModels(c)

	if c.Writer.Written() {
		t.Fatalf("canceled request wrote an HTTP response: status=%d body=%q", recorder.Code, recorder.Body.String())
	}
}

func TestCodexModelsAppliesLocalFiltersBeforeClientETag(t *testing.T) {
	gin.SetMode(gin.TestMode)
	groupID := int64(43)
	repo := &codexModelsFailoverAccountRepo{accounts: []service.Account{
		{
			ID:          1,
			Name:        "custom-openai",
			Platform:    service.PlatformOpenAI,
			Type:        service.AccountTypeAPIKey,
			Status:      service.StatusActive,
			Schedulable: true,
			Concurrency: 1,
			Credentials: map[string]any{
				"api_key":  "sk-test",
				"base_url": "https://upstream.example/v1",
			},
		},
	}}
	upstream := &codexModelsFailoverHTTPUpstream{
		firstBody: `{"object":"list","data":[{"id":"codex-auto-review"},{"id":"gpt-5.6"}]}`,
	}
	gatewayService := service.NewOpenAIGatewayService(
		repo,
		nil, nil, nil, nil, nil, nil, &config.Config{RunMode: config.RunModeSimple}, nil, nil, nil, nil, nil,
		upstream,
		nil, nil, nil, nil, nil, nil, nil, nil,
	)
	handler := &OpenAIGatewayHandler{gatewayService: gatewayService}
	group := &service.Group{
		ID:       groupID,
		Platform: service.PlatformOpenAI,
		ModelsListConfig: service.GroupModelsListConfig{
			Enabled: true,
			Models:  []string{"codex-auto-review", "gpt-5.6"},
		},
	}

	first := performCodexModelsRequestForGroup(t, handler, group, "")
	if first.Code != http.StatusOK {
		t.Fatalf("first status: got %d, want %d; body=%s", first.Code, http.StatusOK, first.Body.String())
	}
	if body := first.Body.String(); !strings.Contains(body, "codex-auto-review") || !strings.Contains(body, "gpt-5.6") {
		t.Fatalf("first body did not include the explicitly selected models: %s", body)
	}
	oldETag := first.Header().Get("ETag")
	if oldETag == "" {
		t.Fatal("first response did not include an ETag")
	}

	group.ModelsListConfig.Enabled = false
	second := performCodexModelsRequestForGroup(t, handler, group, oldETag)
	if second.Code != http.StatusOK {
		t.Fatalf("second status: got %d, want %d; body=%s", second.Code, http.StatusOK, second.Body.String())
	}
	if body := second.Body.String(); strings.Contains(body, "codex-auto-review") || !strings.Contains(body, "gpt-5.6") {
		t.Fatalf("second body was not the filtered manifest: %s", body)
	}
	if newETag := second.Header().Get("ETag"); newETag == "" || newETag == oldETag {
		t.Fatalf("second ETag: got %q, want a new final-body ETag", newETag)
	}

	third := performCodexModelsRequestForGroup(t, handler, group, second.Header().Get("ETag"))
	if third.Code != http.StatusNotModified {
		t.Fatalf("third status: got %d, want %d; body=%s", third.Code, http.StatusNotModified, third.Body.String())
	}
	if third.Body.Len() != 0 {
		t.Fatalf("third body: got %q, want empty", third.Body.String())
	}
}

func TestCodexModelsAPIKeyCacheDoesNotLeakGroupFilters(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := &codexModelsFailoverAccountRepo{accounts: []service.Account{
		{
			ID:          1,
			Name:        "shared-api-key",
			Platform:    service.PlatformOpenAI,
			Type:        service.AccountTypeAPIKey,
			Status:      service.StatusActive,
			Schedulable: true,
			Concurrency: 1,
			Credentials: map[string]any{
				"api_key":  "sk-shared",
				"base_url": "https://upstream.example/v1",
			},
		},
	}}
	upstream := &codexModelsFailoverHTTPUpstream{
		firstBody: `{"object":"list","data":[{"id":"model-a"},{"id":"model-b"}]}`,
	}
	gatewayService := service.NewOpenAIGatewayService(
		repo,
		nil, nil, nil, nil, nil, nil, &config.Config{RunMode: config.RunModeSimple}, nil, nil, nil, nil, nil,
		upstream,
		nil, nil, nil, nil, nil, nil, nil, nil,
	)
	handler := &OpenAIGatewayHandler{gatewayService: gatewayService}
	groupA := &service.Group{
		ID:       91,
		Platform: service.PlatformOpenAI,
		ModelsListConfig: service.GroupModelsListConfig{
			Enabled: true,
			Models:  []string{"model-a"},
		},
	}
	groupB := &service.Group{
		ID:       92,
		Platform: service.PlatformOpenAI,
		ModelsListConfig: service.GroupModelsListConfig{
			Enabled: true,
			Models:  []string{"model-b"},
		},
	}

	firstA := performCodexModelsRequestForGroup(t, handler, groupA, "")
	require.Equal(t, http.StatusOK, firstA.Code, firstA.Body.String())
	require.Equal(t, []string{"model-a"}, codexHandlerManifestSlugs(t, firstA))

	firstB := performCodexModelsRequestForGroup(t, handler, groupB, "")
	require.Equal(t, http.StatusOK, firstB.Code, firstB.Body.String())
	require.Equal(t, []string{"model-b"}, codexHandlerManifestSlugs(t, firstB))

	etagA := firstA.Header().Get("ETag")
	require.NotEmpty(t, etagA)

	var wg sync.WaitGroup
	results := make([]*httptest.ResponseRecorder, 8)
	for i := range results {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			if index%2 == 0 {
				results[index] = performCodexModelsRequestForGroup(t, handler, groupA, etagA)
				return
			}
			results[index] = performCodexModelsRequestForGroup(t, handler, groupB, "")
		}(i)
	}
	wg.Wait()

	sawGroupB := false
	for _, recorder := range results {
		require.NotNil(t, recorder)
		switch recorder.Code {
		case http.StatusNotModified:
			require.Empty(t, recorder.Body.Bytes())
		case http.StatusOK:
			slugs := codexHandlerManifestSlugs(t, recorder)
			if len(slugs) == 1 && slugs[0] == "model-b" {
				sawGroupB = true
				continue
			}
			require.Equal(t, []string{"model-a"}, slugs)
		default:
			t.Fatalf("unexpected status %d body=%s", recorder.Code, recorder.Body.String())
		}
	}
	require.True(t, sawGroupB)
}

// Scenario: OpenAI 分组内混用 OAuth 和第三方 API Key 时，管理员模型配置优先。
func TestCodexModelsUsesConfiguredModelsBeforeUpstreamDiscovery(t *testing.T) {
	gin.SetMode(gin.TestMode)
	groupID := int64(44)
	repo := &codexModelsFailoverAccountRepo{accounts: []service.Account{
		{
			ID:          1,
			Name:        "ark-compatible",
			Platform:    service.PlatformOpenAI,
			Type:        service.AccountTypeAPIKey,
			Status:      service.StatusActive,
			Schedulable: true,
			Priority:    0,
			Concurrency: 1,
			Credentials: map[string]any{
				"api_key":  "sk-ark",
				"base_url": "https://ark.example/v1",
				"model_mapping": map[string]any{
					"glm-5.3": "glm-5.3",
				},
			},
		},
		{
			ID:          2,
			Name:        "chatgpt-oauth",
			Platform:    service.PlatformOpenAI,
			Type:        service.AccountTypeOAuth,
			Status:      service.StatusActive,
			Schedulable: true,
			Priority:    1,
			Concurrency: 1,
			Credentials: map[string]any{
				"access_token": "oauth-test",
			},
		},
	}}
	upstream := &codexModelsFailoverHTTPUpstream{firstStatus: http.StatusNotFound}
	gatewayService := service.NewOpenAIGatewayService(
		repo,
		nil, nil, nil, nil, nil, nil, &config.Config{RunMode: config.RunModeSimple}, nil, nil, nil, nil, nil,
		upstream,
		nil, nil, nil, nil, nil, nil, nil, nil,
	)
	handler := &OpenAIGatewayHandler{gatewayService: gatewayService}

	recorder := performCodexModelsRequestForGroup(t, handler, &service.Group{
		ID:       groupID,
		Platform: service.PlatformOpenAI,
	}, "")

	if got := upstream.calls(); len(got) != 0 {
		t.Fatalf("upstream account calls: got %v, want none", got)
	}
	if recorder.Code != http.StatusOK {
		t.Fatalf("status: got %d, want %d; body=%s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	var envelope struct {
		Models []map[string]any `json:"models"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("decode body: %v; body=%s", err, recorder.Body.String())
	}
	if len(envelope.Models) != 1 || envelope.Models[0]["slug"] != "glm-5.3" {
		t.Fatalf("models: got %v, want only glm-5.3", envelope.Models)
	}
	if _, ok := envelope.Models[0]["supported_reasoning_levels"]; !ok {
		t.Fatalf("configured model is missing the Codex descriptor contract: %v", envelope.Models[0])
	}
}

func TestCompositeCodexModelsReusesExistingManifestSelection(t *testing.T) {
	handler, upstream, groupID := newCodexModelsFailoverTestHandler(http.StatusServiceUnavailable)

	recorder := performCodexModelsRequestForPlatform(t, handler, groupID, service.PlatformComposite)

	if got, want := upstream.calls(), []int64{1, 2}; !equalInt64Slices(got, want) {
		t.Fatalf("upstream account calls: got %v, want %v", got, want)
	}
	if recorder.Code != http.StatusOK {
		t.Fatalf("status: got %d, want %d; body=%s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
}

func TestCodexModelsFailsOverFromRetryableUpstreamStatus(t *testing.T) {
	retryableStatuses := []int{
		http.StatusTooManyRequests,
		http.StatusInternalServerError,
		http.StatusBadGateway,
		http.StatusServiceUnavailable,
		http.StatusGatewayTimeout,
	}
	for _, status := range retryableStatuses {
		t.Run(http.StatusText(status), func(t *testing.T) {
			handler, upstream, groupID := newCodexModelsFailoverTestHandler(status)
			recorder := performCodexModelsRequest(t, handler, groupID)

			if got, want := upstream.calls(), []int64{1, 2}; !equalInt64Slices(got, want) {
				t.Fatalf("upstream account calls: got %v, want %v", got, want)
			}
			if recorder.Code != http.StatusOK {
				t.Fatalf("status: got %d, want %d; body=%s", recorder.Code, http.StatusOK, recorder.Body.String())
			}
			requireCompleteCodexModelsHandlerResponse(t, recorder, "gpt-5.6-sol")
		})
	}
}

// Scenario: an API-key upstream without /models is excluded only for this discovery request.
func TestCodexModelsFailsOverWhenAPIKeyModelsEndpointIsUnavailable(t *testing.T) {
	for _, status := range []int{http.StatusNotFound, http.StatusMethodNotAllowed} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			handler, upstream, groupID := newCodexModelsFailoverTestHandler(status)
			recorder := performCodexModelsRequest(t, handler, groupID)

			if got, want := upstream.calls(), []int64{1, 2}; !equalInt64Slices(got, want) {
				t.Fatalf("upstream account calls: got %v, want %v", got, want)
			}
			if recorder.Code != http.StatusOK {
				t.Fatalf("status: got %d, want %d; body=%s", recorder.Code, http.StatusOK, recorder.Body.String())
			}
		})
	}
}

func TestCodexModelsFailsOverFromUpstreamTransportError(t *testing.T) {
	handler, upstream, groupID := newCodexModelsFailoverTestHandler(http.StatusServiceUnavailable)
	upstream.firstErr = &net.OpError{
		Op:  "read",
		Net: "tcp",
		Err: errors.New("connection reset"),
	}
	recorder := performCodexModelsRequest(t, handler, groupID)

	if got, want := upstream.calls(), []int64{1, 2}; !equalInt64Slices(got, want) {
		t.Fatalf("upstream account calls: got %v, want %v", got, want)
	}
	if recorder.Code != http.StatusOK {
		t.Fatalf("status: got %d, want %d; body=%s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
}

func TestCodexModelsFailsOverFromInvalidManifestEnvelope(t *testing.T) {
	handler, upstream, groupID := newCodexModelsFailoverTestHandler(http.StatusOK)
	upstream.firstBody = `{"object":"list","data":[]}`
	recorder := performCodexModelsRequest(t, handler, groupID)

	if got, want := upstream.calls(), []int64{1, 2}; !equalInt64Slices(got, want) {
		t.Fatalf("upstream account calls: got %v, want %v", got, want)
	}
	if recorder.Code != http.StatusOK {
		t.Fatalf("status: got %d, want %d; body=%s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	requireCompleteCodexModelsHandlerResponse(t, recorder, "gpt-5.6-sol")
}

func TestCodexModelsDoesNotFailOverFromPermanentUpstreamStatus(t *testing.T) {
	statuses := []int{
		http.StatusBadRequest,
		http.StatusUnauthorized,
		http.StatusForbidden,
		600,
	}
	for _, status := range statuses {
		t.Run(fmt.Sprintf("status_%d", status), func(t *testing.T) {
			handler, upstream, groupID := newCodexModelsFailoverTestHandler(status)
			recorder := performCodexModelsRequest(t, handler, groupID)

			if got, want := upstream.calls(), []int64{1}; !equalInt64Slices(got, want) {
				t.Fatalf("upstream account calls: got %v, want %v", got, want)
			}
			if recorder.Code != http.StatusBadGateway {
				t.Fatalf("status: got %d, want %d; body=%s", recorder.Code, http.StatusBadGateway, recorder.Body.String())
			}
		})
	}
}

func TestCodexModelsDoesNotFailOverFromUpstreamConfigurationError(t *testing.T) {
	handler, upstream, groupID := newCodexModelsFailoverTestHandler(http.StatusServiceUnavailable)
	upstream.firstErr = errors.New("invalid proxy URL")
	recorder := performCodexModelsRequest(t, handler, groupID)

	if got, want := upstream.calls(), []int64{1}; !equalInt64Slices(got, want) {
		t.Fatalf("upstream account calls: got %v, want %v", got, want)
	}
	if recorder.Code != http.StatusBadGateway {
		t.Fatalf("status: got %d, want %d; body=%s", recorder.Code, http.StatusBadGateway, recorder.Body.String())
	}
}

func TestCodexModelsReturnsLastUpstreamErrorWhenAccountsAreExhausted(t *testing.T) {
	handler, upstream, groupID := newCodexModelsFailoverTestHandler(http.StatusServiceUnavailable)
	upstream.statuses = map[int64]int{
		1: http.StatusServiceUnavailable,
		2: http.StatusGatewayTimeout,
	}
	recorder := performCodexModelsRequest(t, handler, groupID)

	if got, want := upstream.calls(), []int64{1, 2}; !equalInt64Slices(got, want) {
		t.Fatalf("upstream account calls: got %v, want %v", got, want)
	}
	if recorder.Code != http.StatusBadGateway {
		t.Fatalf("status: got %d, want %d; body=%s", recorder.Code, http.StatusBadGateway, recorder.Body.String())
	}
	if body := recorder.Body.String(); !strings.Contains(body, "upstream error 504") {
		t.Fatalf("body does not preserve the last upstream error: %s", body)
	}
}

func TestCodexModelsHonorsAccountSwitchLimit(t *testing.T) {
	handler, upstream, groupID := newCodexModelsFailoverTestHandlerWithAccountCount(http.StatusServiceUnavailable, 4, 2)
	upstream.statuses = map[int64]int{
		1: http.StatusServiceUnavailable,
		2: http.StatusBadGateway,
		3: http.StatusGatewayTimeout,
		4: http.StatusInternalServerError,
	}
	recorder := performCodexModelsRequest(t, handler, groupID)

	if got, want := upstream.calls(), []int64{1, 2, 3}; !equalInt64Slices(got, want) {
		t.Fatalf("upstream account calls: got %v, want %v", got, want)
	}
	if recorder.Code != http.StatusBadGateway {
		t.Fatalf("status: got %d, want %d; body=%s", recorder.Code, http.StatusBadGateway, recorder.Body.String())
	}
	if body := recorder.Body.String(); !strings.Contains(body, "upstream error 504") {
		t.Fatalf("body does not preserve the limit-ending upstream error: %s", body)
	}
}

func newCodexModelsFailoverTestHandler(firstStatus int) (*OpenAIGatewayHandler, *codexModelsFailoverHTTPUpstream, int64) {
	return newCodexModelsFailoverTestHandlerWithAccountCount(firstStatus, 2, 3)
}

func newCodexModelsFailoverTestHandlerWithAccountCount(firstStatus, accountCount, maxSwitches int) (*OpenAIGatewayHandler, *codexModelsFailoverHTTPUpstream, int64) {
	gin.SetMode(gin.TestMode)
	groupID := int64(42)
	accounts := make([]service.Account, 0, accountCount)
	for i := 1; i <= accountCount; i++ {
		accounts = append(accounts, service.Account{
			ID:          int64(i),
			Name:        fmt.Sprintf("upstream-%d", i),
			Platform:    service.PlatformOpenAI,
			Type:        service.AccountTypeAPIKey,
			Status:      service.StatusActive,
			Schedulable: true,
			Priority:    i - 1,
			Concurrency: 1,
			Credentials: map[string]any{
				"api_key":  fmt.Sprintf("sk-%d", i),
				"base_url": fmt.Sprintf("https://upstream-%d.example/v1", i),
			},
		})
	}
	upstream := &codexModelsFailoverHTTPUpstream{firstStatus: firstStatus}
	cfg := &config.Config{RunMode: config.RunModeSimple}
	gatewayService := service.NewOpenAIGatewayService(
		codexModelsFailoverAccountRepo{accounts: accounts},
		nil, nil, nil, nil, nil, nil, cfg, nil, nil, nil, nil, nil,
		upstream,
		nil, nil, nil, nil, nil, nil, nil, nil,
	)
	return &OpenAIGatewayHandler{gatewayService: gatewayService, maxAccountSwitches: maxSwitches}, upstream, groupID
}

func performCodexModelsRequest(t *testing.T, handler *OpenAIGatewayHandler, groupID int64) *httptest.ResponseRecorder {
	return performCodexModelsRequestForGroup(t, handler, &service.Group{ID: groupID, Platform: service.PlatformOpenAI}, "")
}

func performCodexModelsRequestForPlatform(t *testing.T, handler *OpenAIGatewayHandler, groupID int64, platform string) *httptest.ResponseRecorder {
	return performCodexModelsRequestForGroup(t, handler, &service.Group{ID: groupID, Platform: platform}, "")
}

func performCodexModelsRequestForGroup(t *testing.T, handler *OpenAIGatewayHandler, group *service.Group, etag string) *httptest.ResponseRecorder {
	t.Helper()
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/models?client_version=0.144.0", nil)
	if etag != "" {
		c.Request.Header.Set("If-None-Match", etag)
	}
	c.Set(string(middleware2.ContextKeyAPIKey), &service.APIKey{
		GroupID: &group.ID,
		Group:   group,
	})

	handler.CodexModels(c)
	return recorder
}

func codexHandlerManifestSlugs(t *testing.T, recorder *httptest.ResponseRecorder) []string {
	t.Helper()

	var envelope struct {
		Models []struct {
			Slug string `json:"slug"`
		} `json:"models"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("decode body: %v; body=%s", err, recorder.Body.String())
	}
	slugs := make([]string, 0, len(envelope.Models))
	for _, model := range envelope.Models {
		slugs = append(slugs, model.Slug)
	}
	return slugs
}

func requireCompleteCodexModelsHandlerResponse(t *testing.T, recorder *httptest.ResponseRecorder, slug string) {
	t.Helper()

	var envelope struct {
		Models []map[string]any `json:"models"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("decode body: %v; body=%s", err, recorder.Body.String())
	}
	if len(envelope.Models) != 1 {
		t.Fatalf("models count: got %d, want 1; body=%s", len(envelope.Models), recorder.Body.String())
	}
	model := envelope.Models[0]
	if got := model["slug"]; got != slug {
		t.Fatalf("slug: got %v, want %q", got, slug)
	}
	if levels, ok := model["supported_reasoning_levels"].([]any); !ok || len(levels) == 0 {
		t.Fatalf("supported_reasoning_levels must be populated: %v", model["supported_reasoning_levels"])
	}
	if messages, ok := model["model_messages"].(map[string]any); !ok || messages["instructions_template"] == "" {
		t.Fatalf("model_messages.instructions_template must be populated: %v", model["model_messages"])
	}
	if policy, ok := model["truncation_policy"].(map[string]any); !ok || len(policy) == 0 {
		t.Fatalf("truncation_policy must be populated: %v", model["truncation_policy"])
	}
	modalities, ok := model["input_modalities"].([]any)
	if !ok || len(modalities) != 2 || modalities[0] != "text" || modalities[1] != "image" {
		t.Fatalf("known GPT model modalities: got %v, want [text image]", model["input_modalities"])
	}
}

func equalInt64Slices(got, want []int64) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}
