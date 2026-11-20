//go:build unit

package handler

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

type openAIImagesSizeRoutingAccountRepo struct {
	openAIImagesFailoverAccountRepo
}

func (r openAIImagesSizeRoutingAccountRepo) ListModelAvailabilityCandidates(_ context.Context, _ *int64, platforms []string, _ bool) ([]service.Account, error) {
	var out []service.Account
	for _, platform := range platforms {
		out = append(out, r.accountsForPlatform(platform)...)
	}
	return out, nil
}

func newOpenAIImagesSizeRoutingTestHandler(t *testing.T, accounts []service.Account, upstream service.HTTPUpstream) *OpenAIGatewayHandler {
	t.Helper()
	cfg := &config.Config{RunMode: config.RunModeSimple}
	gatewayService := service.NewOpenAIGatewayService(
		openAIImagesSizeRoutingAccountRepo{openAIImagesFailoverAccountRepo{accounts: accounts}},
		nil, nil, nil, nil, nil, nil,
		cfg,
		nil, nil, nil, nil, nil,
		upstream,
		nil, nil, nil, nil, nil, nil, nil, nil,
	)
	billingService := service.NewBillingCacheService(nil, nil, nil, nil, nil, nil, cfg, nil)
	t.Cleanup(billingService.Stop)
	handler := NewOpenAIGatewayHandler(
		gatewayService,
		service.NewConcurrencyService(nil),
		billingService,
		service.NewAPIKeyService(nil, nil, nil, nil, nil, nil, cfg),
		nil, nil, nil, nil,
		cfg,
	)
	handler.maxAccountSwitches = 10
	return handler
}

func serveOpenAIImagesSizeRoutingRequest(handler *OpenAIGatewayHandler, groupID int64, body string) (*httptest.ResponseRecorder, *gin.Context) {
	req := httptest.NewRequest(http.MethodPost, "/v1/images/generations", bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = req
	c.Set(string(middleware2.ContextKeyAPIKey), &service.APIKey{
		ID:      99,
		GroupID: &groupID,
		Group:   &service.Group{ID: groupID, AllowImageGeneration: true},
		User:    &service.User{ID: 100},
	})
	c.Set(string(middleware2.ContextKeyUser), middleware2.AuthSubject{UserID: 100, Concurrency: 0})
	handler.Images(c)
	return rec, c
}

func newOpenAIImagesSizeRoutingAccount(id int64, tiers ...any) service.Account {
	return service.Account{
		ID:          id,
		Name:        fmt.Sprintf("image-size-account-%d", id),
		Platform:    service.PlatformOpenAI,
		Type:        service.AccountTypeOAuth,
		Status:      service.StatusActive,
		Schedulable: true,
		Priority:    int(id),
		Credentials: map[string]any{"access_token": fmt.Sprintf("token-%d", id)},
		Extra:       map[string]any{service.AccountExtraOpenAIImageSizeTiers: tiers},
	}
}

func TestOpenAIGatewayHandlerImages_SizeTierMissingFromGroupReturns400(t *testing.T) {
	gin.SetMode(gin.TestMode)
	upstream := &openAIImagesFailoverHTTPUpstream{}
	handler := newOpenAIImagesSizeRoutingTestHandler(t, []service.Account{
		newOpenAIImagesSizeRoutingAccount(1, "1K", "2K"),
	}, upstream)

	rec, c := serveOpenAIImagesSizeRoutingRequest(handler, 3140, `{"model":"gpt-image-2","prompt":"draw a cat","size":"3840x2160"}`)

	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Equal(t, "invalid_request_error", gjson.GetBytes(rec.Body.Bytes(), "error.type").String())
	require.Contains(t, gjson.GetBytes(rec.Body.Bytes(), "error.message").String(), "4K")
	require.Empty(t, upstream.calls())
	reason, _ := c.Get(service.OpsClientBusinessLimitedReasonKey)
	require.Equal(t, service.OpsClientBusinessLimitedReasonLocalModelConfiguration, reason)
}

func TestOpenAIGatewayHandlerImages_SizeTierRoutesToAllowedAccounts(t *testing.T) {
	gin.SetMode(gin.TestMode)
	upstream := &openAIImagesFailoverHTTPUpstream{}
	handler := newOpenAIImagesSizeRoutingTestHandler(t, []service.Account{
		newOpenAIImagesSizeRoutingAccount(1, "1K", "2K"),
		newOpenAIImagesSizeRoutingAccount(2, "4K"),
		newOpenAIImagesSizeRoutingAccount(3, "4K"),
	}, upstream)

	rec, _ := serveOpenAIImagesSizeRoutingRequest(handler, 3141, `{"model":"gpt-image-1","prompt":"draw a cat","size":"2880x2880"}`)

	require.Equal(t, http.StatusBadGateway, rec.Code, "the stub upstream fails every attempt")
	require.Equal(t, []int64{2, 3}, upstream.calls(), "4K requests fail over only across 4K accounts")
}

func serveOpenAIResponsesSizeRoutingRequest(handler *OpenAIGatewayHandler, groupID int64, body string) (*httptest.ResponseRecorder, *gin.Context) {
	req := httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = req
	c.Set(string(middleware2.ContextKeyAPIKey), &service.APIKey{
		ID:      99,
		GroupID: &groupID,
		Group:   &service.Group{ID: groupID, Platform: service.PlatformOpenAI, AllowImageGeneration: true},
		User:    &service.User{ID: 100},
	})
	c.Set(string(middleware2.ContextKeyUser), middleware2.AuthSubject{UserID: 100, Concurrency: 0})
	handler.Responses(c)
	return rec, c
}

func TestOpenAIGatewayHandlerResponses_ImageToolSizeTierMissingFromGroupReturns400(t *testing.T) {
	gin.SetMode(gin.TestMode)
	upstream := &openAIImagesFailoverHTTPUpstream{}
	handler := newOpenAIImagesSizeRoutingTestHandler(t, []service.Account{
		newOpenAIImagesSizeRoutingAccount(1, "1K", "2K"),
	}, upstream)

	rec, _ := serveOpenAIResponsesSizeRoutingRequest(handler, 3142, `{"model":"gpt-5.4","stream":false,"input":"draw a cat","tools":[{"type":"image_generation","size":"3840x2160"}]}`)

	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Equal(t, "invalid_request_error", gjson.GetBytes(rec.Body.Bytes(), "error.type").String())
	require.Contains(t, gjson.GetBytes(rec.Body.Bytes(), "error.message").String(), "4K")
	require.Empty(t, upstream.calls())
}

func TestOpenAIGatewayHandlerResponses_TextRequestIgnoresImageSizeTiers(t *testing.T) {
	gin.SetMode(gin.TestMode)
	upstream := &openAIImagesFailoverHTTPUpstream{}
	handler := newOpenAIImagesSizeRoutingTestHandler(t, []service.Account{
		newOpenAIImagesSizeRoutingAccount(1, "4K"),
	}, upstream)

	_, _ = serveOpenAIResponsesSizeRoutingRequest(handler, 3143, `{"model":"gpt-5.4","stream":false,"input":"hello","size":"1024x1024"}`)

	require.NotEmpty(t, upstream.calls())
	require.Equal(t, int64(1), upstream.calls()[0])
}
