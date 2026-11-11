//go:build unit

package service

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestGrokMediaUpstreamErrorFields(t *testing.T) {
	for _, tt := range []struct {
		name        string
		status      int
		body        string
		wantMessage string
		wantCode    string
	}{
		{"flat error", 400, `{"code":"invalid_image","error":"Image could not be decoded."}`, "Image could not be decoded.", "invalid_image"},
		{"nested error", 400, `{"code":"outer","error":{"code":"invalid_size","message":"Invalid size."}}`, "Invalid size.", "invalid_size"},
		{"message without code", 400, `{"error":"Invalid size."}`, "Invalid size.", ""},
		{"redact code query", 400, `{"error":"Invalid size.","code":"https://example.test?key=secret&n=1"}`, "Invalid size.", "https://example.test?key=***&n=1"},
		{"unknown object", 400, `{"error":{"debug":"private diagnostic"},"code":{"debug":"private code"}}`, "xAI upstream returned status 400", ""},
		{"content refusal", 403, `{"code":"permission-denied","error":"Content violates usage guidelines."}`, "Content violates usage guidelines.", "permission-denied"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/edits", nil)
			repo := &grokQuotaAccountRepo{}
			svc := &OpenAIGatewayService{accountRepo: repo}
			account := &Account{ID: 1, Platform: PlatformGrok, Type: AccountTypeOAuth}
			_, err := svc.handleGrokMediaErrorResponse(context.Background(), &http.Response{
				StatusCode: tt.status, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(tt.body)),
			}, c, account, "request-id", "grok-imagine-image")
			require.Error(t, err)
			var failover *UpstreamFailoverError
			require.False(t, errors.As(err, &failover))
			require.True(t, IsResponseCommitted(c))
			require.Equal(t, tt.status, recorder.Code)
			require.Equal(t, tt.wantMessage, gjson.GetBytes(recorder.Body.Bytes(), "error.message").String())
			require.Equal(t, tt.wantCode, gjson.GetBytes(recorder.Body.Bytes(), "error.code").String())
			if tt.wantCode == "" {
				require.False(t, gjson.GetBytes(recorder.Body.Bytes(), "error.code").Exists())
			}
			require.NotContains(t, recorder.Body.String(), "secret")
			require.NotContains(t, recorder.Body.String(), "private")
			require.Zero(t, repo.tempUnschedCalls)
			require.Zero(t, repo.rateLimitedCalls)
			require.Zero(t, repo.updateCalls)
		})
	}
}

func TestGrokMediaErrorRewritesKeepPriority(t *testing.T) {
	for _, tt := range []struct {
		name            string
		passthroughRule bool
		passthroughBody bool
	}{
		{name: "account error code filter"},
		{name: "configured response rule", passthroughRule: true},
		{name: "configured message passthrough", passthroughRule: true, passthroughBody: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/edits", nil)
			account := &Account{ID: 1, Platform: PlatformGrok, Type: AccountTypeAPIKey, Credentials: map[string]any{
				"custom_error_codes_enabled": true, "custom_error_codes": []any{float64(http.StatusTooManyRequests)},
			}}
			wantStatus, wantMessage := http.StatusInternalServerError, "Upstream gateway error"
			if tt.passthroughRule {
				rules := &ErrorPassthroughService{}
				rule := newNonFailoverPassthroughRule(http.StatusBadRequest, "invalid_image", http.StatusUnprocessableEntity, "Configured error")
				rule.PassthroughBody = tt.passthroughBody
				rules.setLocalCache([]*model.ErrorPassthroughRule{rule})
				BindErrorPassthroughService(c, rules)
				wantStatus, wantMessage = http.StatusUnprocessableEntity, "Configured error"
				if tt.passthroughBody {
					wantMessage = "Original error: https://example.test?access_token=***"
				}
			}
			svc := &OpenAIGatewayService{accountRepo: &grokQuotaAccountRepo{}}
			_, err := svc.handleGrokMediaErrorResponse(context.Background(), &http.Response{
				StatusCode: http.StatusBadRequest, Header: http.Header{},
				Body: io.NopCloser(strings.NewReader(`{"code":"invalid_image","error":"Original error: https://example.test?access_token=secret"}`)),
			}, c, account, "request-id", "grok-imagine-image")
			require.Error(t, err)
			require.Equal(t, wantStatus, recorder.Code)
			require.Equal(t, wantMessage, gjson.GetBytes(recorder.Body.Bytes(), "error.message").String())
			require.False(t, gjson.GetBytes(recorder.Body.Bytes(), "error.code").Exists())
		})
	}
}

func TestGrokMediaRetryableErrorKeepsOriginalBody(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/edits", nil)
	body := `{"code":"temporarily_unavailable","error":"Try again later"}`
	svc := &OpenAIGatewayService{accountRepo: &grokQuotaAccountRepo{}}
	_, err := svc.handleGrokMediaErrorResponse(context.Background(), &http.Response{
		StatusCode: http.StatusServiceUnavailable, Header: http.Header{"X-Request-Id": {"request-id"}},
		Body: io.NopCloser(strings.NewReader(body)),
	}, c, &Account{ID: 1, Platform: PlatformGrok, Type: AccountTypeOAuth}, "request-id", "grok-imagine-image")
	var failover *UpstreamFailoverError
	require.ErrorAs(t, err, &failover)
	require.Equal(t, http.StatusServiceUnavailable, failover.StatusCode)
	require.Equal(t, body, string(failover.ResponseBody))
	require.Equal(t, "request-id", failover.ResponseHeaders.Get("X-Request-Id"))
	require.False(t, IsResponseCommitted(c))
	require.False(t, c.Writer.Written())
	require.Empty(t, recorder.Body.String())
}
