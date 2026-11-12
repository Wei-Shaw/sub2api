package service

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/model"
	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type requestTooLargeUpstream struct {
	anthropicHTTPUpstreamRecorder
	calls int
}

func (u *requestTooLargeUpstream) DoWithTLS(req *http.Request, proxyURL string, accountID int64, accountConcurrency int, profile *tlsfingerprint.Profile) (*http.Response, error) {
	u.calls++
	return u.anthropicHTTPUpstreamRecorder.DoWithTLS(req, proxyURL, accountID, accountConcurrency, profile)
}

func TestGatewayForward_RequestTooLarge(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, accountType := range []string{AccountTypeOAuth, AccountTypeAPIKey} {
		for _, stream := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/stream=%t", accountType, stream), func(t *testing.T) {
				rec := httptest.NewRecorder()
				c, _ := gin.CreateTestContext(rec)
				body := []byte(fmt.Sprintf(`{"model":"claude-sonnet-4-5","max_tokens":16,"stream":%t,"messages":[{"role":"user","content":"hello"}]}`, stream))
				c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(string(body)))
				parsed, err := ParseGatewayRequest(NewRequestBodyRef(body), PlatformAnthropic)
				require.NoError(t, err)
				upstream := &requestTooLargeUpstream{
					anthropicHTTPUpstreamRecorder: anthropicHTTPUpstreamRecorder{resp: &http.Response{
						StatusCode: http.StatusRequestEntityTooLarge,
						Header:     http.Header{"Content-Type": []string{"application/json"}},
						Body:       io.NopCloser(strings.NewReader(`{"error":{"type":"request_too_large","message":"Request exceeds the maximum allowed size of 32 MB"}}`)),
					}},
				}
				svc := &GatewayService{
					cfg:              &config.Config{},
					httpUpstream:     upstream,
					rateLimitService: &RateLimitService{},
				}
				account := &Account{
					ID: 1, Platform: PlatformAnthropic, Type: accountType,
					Credentials: map[string]any{"access_token": "test-oauth-token", "api_key": "test-api-key"},
					Status:      StatusActive, Schedulable: true, Concurrency: 1,
				}

				result, err := svc.Forward(context.Background(), c, account, parsed)

				require.Error(t, err)
				require.Nil(t, result)
				var failoverErr *UpstreamFailoverError
				require.NotErrorAs(t, err, &failoverErr)
				require.Equal(t, 1, upstream.calls, "a size rejection should not retry the same request")
				require.Equal(t, http.StatusRequestEntityTooLarge, rec.Code)
				require.Contains(t, rec.Header().Get("Content-Type"), "application/json")
				require.JSONEq(t, `{"type":"error","error":{"type":"request_too_large","message":"Request exceeds the upstream size limit"}}`, rec.Body.String())
				require.True(t, IsResponseCommitted(c))
				require.Equal(t, http.StatusRequestEntityTooLarge, c.GetInt(OpsUpstreamStatusCodeKey))
			})
		}
	}
}

func TestGatewayHandleErrorResponse_RequestTooLarge(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for name, body := range map[string]string{
		"structured": `{"error":{"type":"request_too_large","message":"Request exceeds the maximum allowed size of 32 MB"},"request_id":"private-upstream-id"}`,
		"html":       `<html><body>413 Request Entity Too Large: internal-proxy</body></html>`,
		"empty":      "",
		"malformed":  `{"error":`,
	} {
		for _, accountType := range []string{AccountTypeOAuth, AccountTypeAPIKey} {
			t.Run(name+"/"+accountType, func(t *testing.T) {
				rec := httptest.NewRecorder()
				c, _ := gin.CreateTestContext(rec)
				c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
				svc := &GatewayService{}
				resp := &http.Response{
					StatusCode: http.StatusRequestEntityTooLarge,
					Body:       io.NopCloser(strings.NewReader(body)),
					Header:     http.Header{},
				}
				account := &Account{ID: 1, Platform: PlatformAnthropic, Type: accountType}

				_, err := svc.handleErrorResponse(context.Background(), resp, c, account)

				require.Error(t, err)
				var failoverErr *UpstreamFailoverError
				require.NotErrorAs(t, err, &failoverErr)
				require.Equal(t, http.StatusRequestEntityTooLarge, rec.Code)
				require.JSONEq(t, `{"type":"error","error":{"type":"request_too_large","message":"Request exceeds the upstream size limit"}}`, rec.Body.String())
				require.True(t, IsResponseCommitted(c))
				require.Equal(t, http.StatusRequestEntityTooLarge, c.GetInt(OpsUpstreamStatusCodeKey))
				if name == "structured" {
					require.Equal(t, "Request exceeds the maximum allowed size of 32 MB", c.GetString(OpsUpstreamErrorMessageKey))
				}
			})
		}
	}
}

func TestGatewayHandleErrorResponse_RequestTooLargePassthroughRule(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)

	ruleSvc := &ErrorPassthroughService{}
	ruleSvc.setLocalCache([]*model.ErrorPassthroughRule{
		newNonFailoverPassthroughRule(http.StatusRequestEntityTooLarge, "maximum allowed size", http.StatusBadRequest, "Please reduce the request size"),
	})
	BindErrorPassthroughService(c, ruleSvc)
	svc := &GatewayService{}
	resp := &http.Response{
		StatusCode: http.StatusRequestEntityTooLarge,
		Body:       io.NopCloser(strings.NewReader(`{"error":{"type":"request_too_large","message":"Request exceeds the maximum allowed size of 32 MB"}}`)),
		Header:     http.Header{},
	}
	account := &Account{ID: 1, Platform: PlatformAnthropic, Type: AccountTypeOAuth}

	_, err := svc.handleErrorResponse(context.Background(), resp, c, account)

	require.Error(t, err)
	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.JSONEq(t, `{"type":"error","error":{"type":"upstream_error","message":"Please reduce the request size"}}`, rec.Body.String())
}
