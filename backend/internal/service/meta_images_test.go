//go:build unit

package service

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func metaImagesTestContext(t *testing.T, body []byte) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	req := httptest.NewRequest(http.MethodPost, "/v1/images/generations", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "codex_cli_rs/0.1")
	req.Header.Set("Originator", "codex_cli_rs")
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = req
	return c, rec
}

func TestMetaForwardImagesCallsMuseImagesAPIWithGatewayIdentity(t *testing.T) {
	body := []byte(`{"model":"muse-image-1.0","prompt":"a red apple","n":1}`)
	c, rec := metaImagesTestContext(t, body)
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}, "X-Request-Id": []string{"req_meta_img"}},
		Body: io.NopCloser(strings.NewReader(
			`{"created":1794294456,"data":[{"b64_json":"UklGRg=="}],"output_format":"webp","background":"opaque","size":"1920x1280","usage":{"input_tokens":9697,"output_tokens":628,"total_tokens":10325}}`,
		)),
	}}
	svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream}
	parsed, err := svc.ParseOpenAIImagesRequest(c, body)
	require.NoError(t, err)
	account := metaTestAccount(map[string]any{"api_key": "sk-muse", "base_url": "https://relay.example/v1"}, AccountTypeOAuth)
	account.Schedulable = true
	account.Status = StatusActive

	result, err := svc.ForwardImages(context.Background(), c, account, body, parsed, "")

	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, 1, result.ImageCount)
	require.Equal(t, 9697, result.Usage.InputTokens)
	require.Equal(t, 628, result.Usage.OutputTokens)
	require.Equal(t, "muse-image-1.0", result.UpstreamModel)
	require.NotNil(t, upstream.lastReq)
	require.Equal(t, "https://relay.example/v1/images/generations", upstream.lastReq.URL.String())
	require.Equal(t, "Bearer sk-muse", upstream.lastReq.Header.Get("Authorization"))
	require.True(t, strings.HasPrefix(upstream.lastReq.Header.Get("User-Agent"), "muse-build/"))
	require.Equal(t, "tbh:exec", upstream.lastReq.Header.Get("X-Client-Id"))
	require.NotEmpty(t, upstream.lastReq.Header.Get("X-Tbh-Session-Id"))
	require.Empty(t, upstream.lastReq.Header.Get("Originator"))
	require.Equal(t, "muse-image-1.0", gjson.GetBytes(upstream.lastBody, "model").String())
	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "UklGRg==", gjson.Get(rec.Body.String(), "data.0.b64_json").String())
	require.Equal(t, "webp", gjson.Get(rec.Body.String(), "output_format").String())
}

func TestMetaForwardImagesQuotaResetRateLimitsAccount(t *testing.T) {
	body := []byte(`{"model":"muse-image-1.0","prompt":"a red apple"}`)
	c, _ := metaImagesTestContext(t, body)
	resetAt := time.Now().Add(37 * time.Minute).Truncate(time.Second)
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusTooManyRequests,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body: io.NopCloser(strings.NewReader(
			`{"error":{"code":"rate_limit_exceeded","message":"Usage limit exceeded","resets_at":` + strconv.FormatInt(resetAt.Unix(), 10) + `}}`,
		)),
	}}
	repo := &rateLimitAccountRepoStub{}
	svc := &OpenAIGatewayService{
		cfg:              &config.Config{},
		httpUpstream:     upstream,
		rateLimitService: NewRateLimitService(repo, nil, &config.Config{}, nil, nil),
	}
	parsed, err := svc.ParseOpenAIImagesRequest(c, body)
	require.NoError(t, err)
	account := metaTestAccount(map[string]any{"api_key": "sk-muse"}, AccountTypeOAuth)

	result, err := svc.ForwardImages(context.Background(), c, account, body, parsed, "")

	require.Nil(t, result)
	var failoverErr *UpstreamFailoverError
	require.ErrorAs(t, err, &failoverErr)
	require.Equal(t, http.StatusTooManyRequests, failoverErr.StatusCode)
	require.Equal(t, 1, repo.rateLimitedCalls)
	require.Equal(t, account.ID, repo.lastRateLimitedID)
	require.WithinDuration(t, resetAt, repo.lastRateLimitedAt, 2*time.Second)
}

func TestMetaForwardImagesRejectsNonMuseUpstreamModelWithoutCallingUpstream(t *testing.T) {
	body := []byte(`{"model":"gpt-image-1","prompt":"a red apple"}`)
	c, _ := metaImagesTestContext(t, body)
	upstream := &httpUpstreamRecorder{}
	svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream}
	parsed, err := svc.ParseOpenAIImagesRequest(c, body)
	require.NoError(t, err)
	account := metaTestAccount(map[string]any{"api_key": "sk-muse"}, AccountTypeAPIKey)

	_, err = svc.ForwardImages(context.Background(), c, account, body, parsed, "")

	require.ErrorContains(t, err, "Meta image model")
	require.Nil(t, upstream.lastReq)
}

func TestMetaAccountsServeEveryImagesCapability(t *testing.T) {
	for _, accountType := range []string{AccountTypeOAuth, AccountTypeAPIKey} {
		account := metaTestAccount(map[string]any{"api_key": "sk-muse"}, accountType)
		for _, capability := range []OpenAIImagesCapability{OpenAIImagesCapabilityBasic, OpenAIImagesCapabilityNative, OpenAIImagesCapabilityAPIKey} {
			require.True(t, account.SupportsOpenAIImageCapability(capability), "type=%s capability=%s", accountType, capability)
		}
	}
}
