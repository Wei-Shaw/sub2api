//go:build unit

package service

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestAnthropicCompatAccountErrorPolicy(t *testing.T) {
	for _, tc := range []struct {
		name         string
		status       int
		credentials  map[string]any
		wantError    int
		wantTemp     int
		wantFailover bool
	}{
		{
			name: "custom gone", status: http.StatusGone,
			credentials: map[string]any{
				"custom_error_codes_enabled": true,
				"custom_error_codes":         []any{float64(http.StatusGone)},
			},
			wantError: 1, wantFailover: true,
		},
		{
			name: "pool custom gone", status: http.StatusGone,
			credentials: map[string]any{
				"pool_mode":                    true,
				"pool_mode_retry_status_codes": []any{float64(http.StatusGone)},
				"custom_error_codes_enabled":   true,
				"custom_error_codes":           []any{float64(http.StatusGone)},
			},
			wantError: 1, wantFailover: true,
		},
		{
			name: "pool temporary gone", status: http.StatusGone,
			credentials: map[string]any{
				"pool_mode":                    true,
				"pool_mode_retry_status_codes": []any{float64(http.StatusGone)},
				"temp_unschedulable_enabled":   true,
				"temp_unschedulable_rules": []any{map[string]any{
					"scope":            "account",
					"error_code":       float64(http.StatusGone),
					"keywords":         []any{"Service moved"},
					"duration_minutes": float64(10),
				}},
			},
			wantTemp: 1, wantFailover: true,
		},
		{name: "unconfigured gone", status: http.StatusGone},
		{
			name: "excluded custom code", status: http.StatusGone,
			credentials: map[string]any{
				"custom_error_codes_enabled": true,
				"custom_error_codes":         []any{float64(http.StatusNotFound)},
			},
		},
		{name: "request error", status: http.StatusBadRequest},
		{name: "default server failover", status: http.StatusInternalServerError, wantFailover: true},
	} {
		for _, chat := range []bool{false, true} {
			for _, stream := range []bool{false, true} {
				name := tc.name + "/chat=" + strconv.FormatBool(chat) + "/stream=" + strconv.FormatBool(stream)
				t.Run(name, func(t *testing.T) {
					account := &Account{ID: 1, Platform: PlatformAnthropic, Type: AccountTypeAPIKey,
						Credentials: map[string]any{"api_key": "fixture-key"}}
					for key, value := range tc.credentials {
						account.Credentials[key] = value
					}
					repo := &rateLimitAccountRepoStub{}
					upstream := &anthropicHTTPUpstreamRecorder{resp: &http.Response{
						StatusCode: tc.status, Header: http.Header{},
						Body: io.NopCloser(strings.NewReader(`{"type":"error","error":{"type":"invalid_request_error","message":"Service moved"}}`)),
					}}
					svc := &GatewayService{
						cfg: &config.Config{}, httpUpstream: upstream,
						rateLimitService: NewRateLimitService(repo, nil, &config.Config{}, nil, nil),
					}
					body := `{"model":"claude-sonnet-4-5","input":"hello","stream":` + strconv.FormatBool(stream) + `}`
					path := "/v1/responses"
					if chat {
						body = `{"model":"claude-sonnet-4-5","messages":[{"role":"user","content":"hello"}],"stream":` + strconv.FormatBool(stream) + `}`
						path = "/v1/chat/completions"
					}
					rec := httptest.NewRecorder()
					c, _ := gin.CreateTestContext(rec)
					c.Request = httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
					var err error
					if chat {
						_, err = svc.ForwardAsChatCompletions(context.Background(), c, account, []byte(body), nil)
					} else {
						_, err = svc.ForwardAsResponses(context.Background(), c, account, []byte(body), nil)
					}
					require.Error(t, err)
					require.Equal(t, tc.wantError, repo.setErrorCalls)
					require.Equal(t, tc.wantTemp, repo.tempCalls)
					var failoverErr *UpstreamFailoverError
					if tc.wantFailover {
						require.ErrorAs(t, err, &failoverErr)
						require.Equal(t, tc.status, failoverErr.StatusCode)
						require.False(t, failoverErr.RetryableOnSameAccount)
						require.False(t, c.Writer.Written(), "leave the response available for the handler to try another account")
						require.Empty(t, rec.Body.String())
					} else {
						require.NotErrorAs(t, err, &failoverErr)
						require.Equal(t, tc.status, rec.Code)
						require.Contains(t, rec.Body.String(), "Service moved")
					}
				})
			}
		}
	}
}
