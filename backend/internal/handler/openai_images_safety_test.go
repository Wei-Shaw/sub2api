//go:build unit

package handler

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

type openAIImagesStaticHTTPUpstream struct {
	service.HTTPUpstream
	status int
	body   string
}

func (u *openAIImagesStaticHTTPUpstream) Do(_ *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
	return &http.Response{
		StatusCode: u.status,
		Header: http.Header{
			"Content-Type": []string{"application/json"},
			"X-Request-Id": []string{"upstream-req-1"},
		},
		Body: io.NopCloser(bytes.NewBufferString(u.body)),
	}, nil
}

func runOpenAIImagesWithUpstream(t *testing.T, status int, upstreamBody string) (*httptest.ResponseRecorder, *contentModerationHandlerTestRepo) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	groupID := int64(8992)
	accountRepo := openAIImagesFailoverAccountRepo{accounts: []service.Account{{
		ID:          1,
		Name:        "image-apikey",
		Platform:    service.PlatformOpenAI,
		Type:        service.AccountTypeAPIKey,
		Status:      service.StatusActive,
		Schedulable: true,
		Credentials: map[string]any{"api_key": "sk-test"},
	}}}
	cfg := &config.Config{RunMode: config.RunModeSimple}
	gatewayService := service.NewOpenAIGatewayService(
		accountRepo, nil, nil, nil, nil, nil, nil, cfg, nil, nil, nil, nil, nil,
		&openAIImagesStaticHTTPUpstream{status: status, body: upstreamBody},
		nil, nil, nil, nil, nil, nil, nil, nil,
	)
	billingService := service.NewBillingCacheService(nil, nil, nil, nil, nil, nil, cfg, nil)
	t.Cleanup(billingService.Stop)
	h := NewOpenAIGatewayHandler(
		gatewayService,
		service.NewConcurrencyService(nil),
		billingService,
		service.NewAPIKeyService(nil, nil, nil, nil, nil, nil, cfg),
		nil, nil, nil, nil,
		cfg,
	)
	repo := &contentModerationHandlerTestRepo{}
	h.contentModerationService = service.NewContentModerationService(
		&contentModerationHandlerSettingRepo{values: map[string]string{
			service.SettingKeyRiskControlEnabled:      "true",
			service.SettingKeyContentModerationConfig: `{"enabled":false,"mode":"off","all_groups":true}`,
		}},
		repo, nil, nil, nil, nil, nil, nil,
	)

	body := []byte(`{"model":"gpt-image-1","prompt":"draw a forbidden thing","size":"1024x1024"}`)
	ctx := context.WithValue(context.Background(), ctxkey.RequestID, "gateway-req-8992")
	req := httptest.NewRequest(http.MethodPost, "/v1/images/generations", bytes.NewReader(body)).WithContext(ctx)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = req
	c.Set(string(middleware2.ContextKeyAPIKey), &service.APIKey{
		ID:      99,
		Name:    "issue-key",
		GroupID: &groupID,
		Group:   &service.Group{ID: groupID, Name: "img-group", AllowImageGeneration: true},
		User:    &service.User{ID: 100, Email: "user@example.com"},
	})
	c.Set(string(middleware2.ContextKeyUser), middleware2.AuthSubject{UserID: 100})

	h.Images(c)
	return rec, repo
}

func TestOpenAIGatewayHandlerImages_SafetySystemRejectionRecordedInRiskControl(t *testing.T) {
	const msg = "Your request was rejected by the safety system. If you believe this is an error, contact us at help.openai.com and include the request ID b553d2f5-3906-4f26-aa3a-e3bb7e24396c."
	rec, repo := runOpenAIImagesWithUpstream(t, http.StatusBadRequest, `{"error":{"message":"`+msg+`"}}`)

	require.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
	require.Contains(t, gjson.GetBytes(rec.Body.Bytes(), "error.message").String(), "safety system")

	require.Eventually(t, func() bool { return len(repo.logSnapshot()) == 1 }, 2*time.Second, 10*time.Millisecond)
	log := repo.logSnapshot()[0]
	require.Equal(t, service.ContentModerationActionImageSafety, log.Action)
	require.Equal(t, service.ContentModerationModeRiskControlLogOnly, log.Mode)
	require.True(t, log.Flagged)
	require.Equal(t, "gateway-req-8992", log.RequestID, "must join with the ops error log, not the upstream request id")
	require.Contains(t, log.Error, "upstream_request_id=upstream-req-1")
	require.NotNil(t, log.UserID)
	require.Equal(t, int64(100), *log.UserID)
	require.Equal(t, "user@example.com", log.UserEmail)
	require.Equal(t, "issue-key", log.APIKeyName)
	require.Equal(t, "img-group", log.GroupName)
	require.Equal(t, "gpt-image-1", log.Model)
	require.Equal(t, service.PlatformOpenAI, log.Provider)
	require.Equal(t, "draw a forbidden thing", log.InputExcerpt)
	require.Contains(t, log.Error, "safety system")
	require.Contains(t, log.Error, "upstream_status=400")
	require.False(t, log.AutoBanned)
	require.False(t, log.EmailSent)
}

func TestOpenAIGatewayHandlerImages_NonSafety400NotRecorded(t *testing.T) {
	rec, repo := runOpenAIImagesWithUpstream(t, http.StatusBadRequest, `{"error":{"message":"Invalid size","type":"invalid_request_error","param":"size"}}`)

	require.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
	require.Never(t, func() bool { return len(repo.logSnapshot()) > 0 }, 200*time.Millisecond, 10*time.Millisecond)
}
