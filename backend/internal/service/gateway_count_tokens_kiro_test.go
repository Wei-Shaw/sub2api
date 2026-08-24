//go:build unit

package service

import (
	"bytes"
	"context"
	"image/color"
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

func TestGatewayServiceForwardCountTokensKiroDirectUsesLocalEstimate(t *testing.T) {
	gin.SetMode(gin.TestMode)

	body := []byte(`{
		"model":"claude-sonnet-4-6",
		"system":[{"type":"text","text":"You are helpful."}],
		"messages":[
			{"role":"assistant","content":[{"type":"tool_use","id":"tool_1","name":"lookup","input":{"city":"Shanghai"}}]},
			{"role":"user","content":[{"type":"tool_result","tool_use_id":"tool_1","content":[{"type":"text","text":"sunny"}]}]}
		],
		"tools":[{"name":"lookup","description":"Look up weather","input_schema":{"type":"object","properties":{"city":{"type":"string"}}}}]
	}`)
	want := estimateKiroInputTokens(context.Background(), body)
	require.Greater(t, want, 1)

	accounts := []*Account{
		{
			ID:       101,
			Platform: PlatformKiro,
			Type:     AccountTypeOAuth,
		},
		{
			ID:       102,
			Platform: PlatformKiro,
			Type:     AccountTypeAPIKey,
		},
	}

	for _, account := range accounts {
		t.Run(string(account.Type), func(t *testing.T) {
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages/count_tokens", bytes.NewReader(body))
			parsed, err := ParseGatewayRequest(NewRequestBodyRef(body), PlatformAnthropic)
			require.NoError(t, err)

			// Missing credentials and upstream dependencies make this test fail if the
			// request ever leaves the local Kiro count_tokens path.
			err = (&GatewayService{}).ForwardCountTokens(context.Background(), c, account, parsed)

			require.NoError(t, err)
			require.Equal(t, http.StatusOK, rec.Code)
			require.JSONEq(t, `{"input_tokens":`+strconv.Itoa(want)+`}`, rec.Body.String())
		})
	}
}

func TestGatewayServiceForwardCountTokensKiroDirectClampsEstimateToOne(t *testing.T) {
	gin.SetMode(gin.TestMode)

	body := []byte(`{"model":"claude-sonnet-4-6","messages":[]}`)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages/count_tokens", bytes.NewReader(body))
	parsed, err := ParseGatewayRequest(NewRequestBodyRef(body), PlatformAnthropic)
	require.NoError(t, err)

	err = (&GatewayService{}).ForwardCountTokens(context.Background(), c, &Account{
		ID:       103,
		Platform: PlatformKiro,
		Type:     AccountTypeOAuth,
	}, parsed)

	require.NoError(t, err)
	require.Equal(t, http.StatusOK, rec.Code)
	require.JSONEq(t, `{"input_tokens":1}`, rec.Body.String())
}

func TestGatewayServiceForwardCountTokensKiroImageUsesVisualEstimate(t *testing.T) {
	gin.SetMode(gin.TestMode)

	dataURL := kiroPNGDataURL(t, 512, 512, color.RGBA{R: 25, G: 50, B: 75, A: 255})
	body := []byte(`{"model":"claude-sonnet-4-6","messages":[{"role":"user","content":[{"type":"text","text":"describe"},{"type":"image_url","image_url":{"url":"` + dataURL + `"}}]}]}`)
	want := estimateKiroInputTokens(context.Background(), body)
	require.GreaterOrEqual(t, want, 350)
	require.Less(t, want, len(dataURL)/2, "base64 payload must not be counted as text")

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages/count_tokens", bytes.NewReader(body))
	parsed, err := ParseGatewayRequest(NewRequestBodyRef(body), PlatformAnthropic)
	require.NoError(t, err)

	err = (&GatewayService{}).ForwardCountTokens(context.Background(), c, &Account{
		ID:       105,
		Platform: PlatformKiro,
		Type:     AccountTypeOAuth,
	}, parsed)

	require.NoError(t, err)
	require.Equal(t, http.StatusOK, rec.Code)
	require.JSONEq(t, `{"input_tokens":`+strconv.Itoa(want)+`}`, rec.Body.String())
}

func TestGatewayServiceForwardCountTokensKiroAPIKeyBaseURLUsesUpstream(t *testing.T) {
	gin.SetMode(gin.TestMode)

	body := []byte(`{"model":"claude-sonnet-4-6","messages":[{"role":"user","content":"hello"}]}`)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages/count_tokens", bytes.NewReader(body))
	parsed, err := ParseGatewayRequest(NewRequestBodyRef(body), PlatformAnthropic)
	require.NoError(t, err)

	upstream := &anthropicHTTPUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(`{"input_tokens":77}`)),
	}}
	svc := &GatewayService{
		cfg: &config.Config{Security: config.SecurityConfig{URLAllowlist: config.URLAllowlistConfig{
			Enabled:           false,
			AllowInsecureHTTP: true,
		}}},
		httpUpstream:        upstream,
		tlsFPProfileService: &TLSFingerprintProfileService{},
	}
	account := &Account{
		ID:          104,
		Platform:    PlatformKiro,
		Type:        AccountTypeAPIKey,
		Concurrency: 1,
		Credentials: map[string]any{
			"api_key":  "relay-key",
			"base_url": "http://kiro-relay.example",
		},
	}

	err = svc.ForwardCountTokens(context.Background(), c, account, parsed)

	require.NoError(t, err)
	require.Equal(t, http.StatusOK, rec.Code)
	require.JSONEq(t, `{"input_tokens":77}`, rec.Body.String())
	require.NotNil(t, upstream.lastReq)
	require.Equal(t, "http://kiro-relay.example/v1/messages/count_tokens?beta=true", upstream.lastReq.URL.String())
	require.Equal(t, "relay-key", upstream.lastReq.Header.Get("x-api-key"))
}
