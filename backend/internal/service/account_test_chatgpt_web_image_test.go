//go:build unit

package service

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/chatgptwebimage"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func webImageAdminTestAccount() *Account {
	reset := time.Now().Add(time.Hour)
	return &Account{ID: 78, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusError, ErrorMessage: "Codex quota exhausted", Schedulable: false, RateLimitResetAt: &reset,
		Credentials: map[string]any{"plan_type": "free", "access_token": "unused", "model_mapping": map[string]any{"paint": "gpt-image-2", "gpt-6-luna": "gpt-6-luna"}},
		Extra:       map[string]any{AccountExtraChatGPTWebImageEnabled: true, AccountExtraChatGPTWebImageRegularChat: true}}
}

func cacheAdminWebImageResult(t *testing.T, account *Account, opts AccountTestOptions, prompt string, imageModel ...string) {
	t.Helper()
	store, err := configuredWebImageJobStore()
	require.NoError(t, err)
	model := "paint"
	if len(imageModel) > 0 {
		model = imageModel[0]
	}
	key := adminWebImageTestJobKey(opts.ActorScope, opts.OperationID, account.ID, model, prompt)
	require.NoError(t, writeWebImageJob(filepath.Join(store.dir, key+".json"), webImageJobRecord{State: "complete", BillingID: "web-image:cached-test", Result: chatgptwebimage.Result{Images: []chatgptwebimage.Image{{Bytes: adminTestPNG(t), Format: "png"}}}}))
}

func TestAccountTestChatGPTWebImagePublicAliasCachedSSEAndNoRecovery(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Setenv("SUB2API_WEB_IMAGE_STATE_DIR", t.TempDir())
	account := webImageAdminTestAccount()
	before, err := json.Marshal(account)
	require.NoError(t, err)
	opts := AccountTestOptions{ActorScope: "admin:5", OperationID: "one-click"}
	cacheAdminWebImageResult(t, account, opts, "draw a cat")
	repo := &rateLimitClearRepoStub{getByIDAccount: account}
	svc := &AccountTestService{accountRepo: repo, openaiGatewayService: &OpenAIGatewayService{}}
	for i := 0; i < 2; i++ {
		rec := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rec)
		c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/78/test", nil)
		require.NoError(t, svc.TestAccountConnection(c, account.ID, "paint", "draw a cat", "", opts))
		require.False(t, AccountTestAllowsRecovery(c))
		require.Contains(t, rec.Body.String(), `"provider":"chatgpt_web"`)
		require.Contains(t, rec.Body.String(), `"model":"paint"`)
		require.NotContains(t, rec.Body.String(), `"actual_model"`, "unknown upstream image model must remain unknown")
		require.Contains(t, rec.Body.String(), `"capability_only":true`)
		require.Contains(t, rec.Body.String(), `"type":"image"`)
		require.Contains(t, rec.Body.String(), `data:image/png;base64,`)
		require.Contains(t, rec.Body.String(), `"success":true`)
		require.NotContains(t, rec.Body.String(), "Calling Codex")
	}
	after, err := json.Marshal(account)
	require.NoError(t, err)
	require.Equal(t, string(before), string(after))
	require.Zero(t, repo.clearErrorCalls)
	require.Zero(t, repo.clearRateLimitCalls)
	require.Zero(t, repo.clearModelRateLimitCalls)
	files, err := filepath.Glob(filepath.Join(os.Getenv("SUB2API_WEB_IMAGE_STATE_DIR"), "*.json"))
	require.NoError(t, err)
	require.Len(t, files, 1)
	record, err := readWebImageJob(files[0])
	require.NoError(t, err)
	require.False(t, record.Billed, "admin capability probes must not impersonate API-key billing")
}

func TestAccountTestChatGPTWebImageRejectsBeforeUpstream(t *testing.T) {
	cases := []struct {
		name, model string
		change      func(*Account)
		message     string
	}{
		{"disabled", "paint", func(a *Account) { a.Extra[AccountExtraChatGPTWebImageEnabled] = false }, "not enabled"},
		{"no history consent", "paint", func(a *Account) { a.Extra[AccountExtraChatGPTWebImageRegularChat] = false }, "explicit permission"},
		{"unconfigured", "gpt-image-2", func(a *Account) { delete(a.Extra, AccountExtraChatGPTWebImageEnabled) }, "not enabled"},
		{"invalid switch", "gpt-image-2.5-flare", func(a *Account) { a.Extra[AccountExtraChatGPTWebImageEnabled] = "true" }, "not enabled"},
		{"empty mapping still requires consent", "gpt-image-2", func(a *Account) {
			delete(a.Credentials, "model_mapping")
			delete(a.Extra, AccountExtraChatGPTWebImageRegularChat)
		}, "explicit permission"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			a := webImageAdminTestAccount()
			tc.change(a)
			repo := &rateLimitClearRepoStub{getByIDAccount: a}
			svc := &AccountTestService{accountRepo: repo} // nil gateway/upstream: policy must reject first
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest(http.MethodPost, "/test", nil)
			err := svc.TestAccountConnection(c, a.ID, tc.model, "cat", "")
			require.ErrorContains(t, err, tc.message)
			require.False(t, AccountTestAllowsRecovery(c))
			require.NotContains(t, rec.Body.String(), "Calling Codex")
			require.Zero(t, repo.clearErrorCalls)
		})
	}
}

func TestAccountTestCapabilityRecoveryCannotBeSpoofedByHeaders(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/test", strings.NewReader(`{"capability_only":false}`))
	c.Request.Header.Set("X-Image-Provider", "chatgpt_web")
	c.Request.Header.Set("X-Account-Test-Capability-Only", "true")
	require.True(t, AccountTestAllowsRecovery(c), "client headers cannot set internal test outcome")
	c.Set(accountTestCapabilityOnlyContextKey, true)
	c.Request.Header.Set("X-Account-Test-Capability-Only", "false")
	require.False(t, AccountTestAllowsRecovery(c))
}

func TestAdminWebImageTestJobScope(t *testing.T) {
	key := adminWebImageTestJobKey("admin:1", "click-a", 78, "paint", "cat")
	require.Equal(t, key, adminWebImageTestJobKey("admin:1", "click-a", 78, "paint", "cat"))
	for _, other := range []string{
		adminWebImageTestJobKey("admin:2", "click-a", 78, "paint", "cat"),
		adminWebImageTestJobKey("admin:1", "click-b", 78, "paint", "cat"),
		adminWebImageTestJobKey("admin:1", "click-a", 79, "paint", "cat"),
		adminWebImageTestJobKey("admin:1", "click-a", 78, "paint", "dog"),
		adminWebImageTestJobKey("admin:1", "click-a", 78, "other", "cat"),
	} {
		require.NotEqual(t, key, other)
	}
}

func TestScheduledWebImageTestSuccessDoesNotRecoverCodexState(t *testing.T) {
	t.Setenv("SUB2API_WEB_IMAGE_STATE_DIR", t.TempDir())
	a := webImageAdminTestAccount()
	opts := AccountTestOptions{ActorScope: "scheduled:3", OperationID: "run-a"}
	cacheAdminWebImageResult(t, a, opts, defaultOpenAIImageTestPrompt)
	repo := &rateLimitClearRepoStub{getByIDAccount: a}
	svc := &AccountTestService{accountRepo: repo, openaiGatewayService: &OpenAIGatewayService{}}
	result, err := svc.runTestBackground(context.Background(), a.ID, "paint", opts)
	require.NoError(t, err)
	require.Equal(t, "success", result.Status)
	require.True(t, result.capabilityOnly)
	runner := &ScheduledTestRunnerService{rateLimitSvc: NewRateLimitService(repo, nil, &config.Config{}, nil, nil)}
	runner.recoverAfterTest(context.Background(), &ScheduledTestPlan{ID: 3, AccountID: a.ID, AutoRecover: true}, result)
	require.Zero(t, repo.clearErrorCalls)
	require.Zero(t, repo.clearRateLimitCalls)
	require.Zero(t, repo.clearModelRateLimitCalls)
	require.Equal(t, StatusError, a.Status)
	require.False(t, a.Schedulable)
	require.True(t, scheduledTestAllowsRecovery(&ScheduledTestResult{Status: "success"}, true), "ordinary chat success retains existing recovery")
	require.False(t, scheduledTestAllowsRecovery(&ScheduledTestResult{Status: "failed"}, true))
	require.False(t, scheduledTestAllowsRecovery(&ScheduledTestResult{Status: "success"}, false))
}

func TestFetchOpenAIAccountModelsFreeWebImageEligibility(t *testing.T) {
	newCodexModelsOAuthCacheServer(t, `{"models":[{"slug":"gpt-6-luna"},{"slug":"gpt-image-2"}]}`)
	a := newCodexModelsTestAccount()
	a.Credentials["plan_type"] = "free"
	a.Credentials["model_mapping"] = map[string]any{"paint": "gpt-image-2", "gpt-6-luna": "gpt-6-luna"}
	a.Extra = map[string]any{AccountExtraChatGPTWebImageEnabled: true, AccountExtraChatGPTWebImageRegularChat: true}
	svc := &AccountTestService{openaiGatewayService: &OpenAIGatewayService{}}
	models, err := svc.FetchOpenAIAccountModels(context.Background(), a)
	require.NoError(t, err)
	require.ElementsMatch(t, append([]string{"paint", "gpt-6-luna"}, freeWebImageTestModelIDs()...), pickerModelIDs(models))
	delete(a.Extra, AccountExtraChatGPTWebImageEnabled)
	models, err = svc.FetchOpenAIAccountModels(context.Background(), a)
	require.NoError(t, err)
	require.Equal(t, []string{"gpt-6-luna"}, pickerModelIDs(models), "unconfigured Free must not advertise native image tests")
	a.Extra[AccountExtraChatGPTWebImageEnabled] = false
	models, err = svc.FetchOpenAIAccountModels(context.Background(), a)
	require.NoError(t, err)
	require.Equal(t, []string{"gpt-6-luna"}, pickerModelIDs(models))
}

func adminTestPNG(t *testing.T) []byte {
	t.Helper()
	var out bytes.Buffer
	require.NoError(t, png.Encode(&out, image.NewRGBA(image.Rect(0, 0, 2, 2))))
	return out.Bytes()
}

func TestAccountTestChatGPTWebImageRejectsInvalidResult(t *testing.T) {
	valid := chatgptwebimage.Image{Bytes: adminTestPNG(t), Format: "png"}
	require.NoError(t, validateAccountTestWebImageResult(chatgptwebimage.Result{Images: []chatgptwebimage.Image{valid}}))
	for _, images := range [][]chatgptwebimage.Image{nil, {valid, valid}, {{Format: "png"}}, {{Format: "jpeg", Bytes: valid.Bytes}}, {{Format: "png", Bytes: []byte("bad PNG")}}, {{Format: "png", Bytes: valid.Bytes[:len(valid.Bytes)-5]}}} {
		require.Error(t, validateAccountTestWebImageResult(chatgptwebimage.Result{Images: images}))
	}
}

func TestAccountTestUnconfiguredFreeRejectsNativeImageFallback(t *testing.T) {
	for _, extra := range []map[string]any{nil, {}, {AccountExtraChatGPTWebImageEnabled: "true"}} {
		a := webImageAdminTestAccount()
		a.Extra = extra
		delete(a.Credentials, "access_token")
		repo := &rateLimitClearRepoStub{getByIDAccount: a}
		svc := &AccountTestService{accountRepo: repo}
		rec := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rec)
		c.Request = httptest.NewRequest(http.MethodPost, "/test", nil)
		err := svc.TestAccountConnection(c, a.ID, "paint", "cat", "")
		require.ErrorContains(t, err, "not enabled")
		require.False(t, AccountTestAllowsRecovery(c))
		require.Contains(t, rec.Body.String(), "chatgpt_web")
		require.NotContains(t, rec.Body.String(), "Calling Codex")
		require.Zero(t, repo.clearErrorCalls)
	}
}

func TestAccountTestFreeNativeImagesIgnoreChatMappingAndDoNotRecover(t *testing.T) {
	for _, mapping := range []map[string]any{nil, {"gpt-6-luna": "gpt-6-luna"}, {"gpt-image-2.5-flare": "gpt-6-luna"}} {
		t.Run("manual_and_scheduled", func(t *testing.T) {
			t.Setenv("SUB2API_WEB_IMAGE_STATE_DIR", t.TempDir())
			a := webImageAdminTestAccount()
			a.Credentials["model_mapping"] = mapping
			model := "gpt-image-2.5-flare"
			opts := AccountTestOptions{ActorScope: "admin:5", OperationID: "native-image"}
			cacheAdminWebImageResult(t, a, opts, defaultOpenAIImageTestPrompt, model)
			repo := &rateLimitClearRepoStub{getByIDAccount: a}
			svc := &AccountTestService{accountRepo: repo, openaiGatewayService: &OpenAIGatewayService{}}
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest(http.MethodPost, "/test", nil)
			require.NoError(t, svc.TestAccountConnection(c, a.ID, model, "", "", opts))
			require.Contains(t, rec.Body.String(), `"provider":"chatgpt_web"`)
			require.Contains(t, rec.Body.String(), `"success":true`)
			require.False(t, AccountTestAllowsRecovery(c))
			result, err := svc.runTestBackground(context.Background(), a.ID, model, opts)
			require.NoError(t, err)
			require.Equal(t, "success", result.Status)
			require.True(t, result.capabilityOnly)
			require.Zero(t, repo.clearErrorCalls)
		})
	}
}
