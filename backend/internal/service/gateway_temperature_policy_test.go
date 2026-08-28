package service

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestGatewayBuildUpstreamRequestAppliesFinalTemperaturePolicy(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tests := []struct {
		name        string
		credentials map[string]any
		wantExists  bool
		wantValue   float64
	}{
		{
			name:        "override replaces Claude OAuth default",
			credentials: map[string]any{"temperature_mode": "override", "temperature": 0.2},
			wantExists:  true,
			wantValue:   0.2,
		},
		{
			name:        "omit removes Claude OAuth default",
			credentials: map[string]any{"temperature_mode": "omit"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
			account := &Account{
				ID:          1,
				Platform:    PlatformAnthropic,
				Type:        AccountTypeOAuth,
				Credentials: tt.credentials,
			}

			req, wireBody, err := (&GatewayService{}).buildUpstreamRequest(
				context.Background(), c, account,
				[]byte(`{"model":"claude-sonnet-4-5","temperature":1,"messages":[]}`),
				"token", "oauth", "claude-sonnet-4-5", false, false,
			)

			require.NoError(t, err)
			require.NotNil(t, req)
			value := gjson.GetBytes(wireBody, "temperature")
			require.Equal(t, tt.wantExists, value.Exists())
			if tt.wantExists {
				require.InDelta(t, tt.wantValue, value.Float(), 1e-9)
			}
		})
	}
}

func TestOpenAIChatTemperaturePolicyRespectsResponsesModelCapabilities(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tests := []struct {
		name            string
		requestModel    string
		upstreamModel   string
		wantTemperature bool
	}{
		{name: "supported model keeps account override", requestModel: "gpt-4.1", upstreamModel: "gpt-4.1", wantTemperature: true},
		{name: "gpt 5.6 keeps account override", requestModel: "gpt-5.6-terra", upstreamModel: "gpt-5.6-terra", wantTemperature: true},
		{name: "legacy gpt model mapped to gpt 5.6 keeps account override", requestModel: "gpt-5.4", upstreamModel: "gpt-5.6-sol", wantTemperature: true},
		{name: "reasoning model removes unsupported override", requestModel: "gpt-5.4", upstreamModel: "gpt-5.4"},
		{name: "mapped reasoning model removes unsupported override", requestModel: "gpt-4.1", upstreamModel: "gpt-5.4"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := []byte(`{"model":"` + tt.requestModel + `","messages":[{"role":"user","content":"hello"}],"stream":false}`)
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
			c.Request.Header.Set("Content-Type", "application/json")

			upstream := &httpUpstreamRecorder{resp: &http.Response{
				StatusCode: http.StatusBadRequest,
				Header:     http.Header{"Content-Type": []string{"application/json"}},
				Body:       io.NopCloser(strings.NewReader(`{"error":{"message":"stop after request capture"}}`)),
			}}
			svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream}
			account := &Account{
				ID:          2,
				Name:        "openai-api-key",
				Platform:    PlatformOpenAI,
				Type:        AccountTypeAPIKey,
				Concurrency: 1,
				Credentials: map[string]any{
					"api_key":          "sk-test",
					"temperature_mode": "override",
					"temperature":      0.2,
				},
				Extra: map[string]any{"openai_responses_supported": true},
			}

			result, err := svc.ForwardAsChatCompletions(context.Background(), c, account, body, "", tt.upstreamModel)

			require.Error(t, err)
			require.Nil(t, result)
			temperature := gjson.GetBytes(upstream.lastBody, "temperature")
			require.Equal(t, tt.wantTemperature, temperature.Exists())
			if tt.wantTemperature {
				require.InDelta(t, 0.2, temperature.Float(), 1e-9)
			}
		})
	}
}

func TestOpenAIMessagesTemperaturePolicyUsesMappedModelCapabilities(t *testing.T) {
	gin.SetMode(gin.TestMode)
	body := []byte(`{"model":"claude-sonnet-4","max_tokens":1024,"messages":[{"role":"user","content":"hello"}],"temperature":0.9}`)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusBadRequest,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(`{"error":{"message":"stop after request capture"}}`)),
	}}
	svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream}
	account := &Account{
		ID:          2,
		Name:        "openai-api-key",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Concurrency: 1,
		Credentials: map[string]any{
			"api_key":          "sk-test",
			"temperature_mode": "override",
			"temperature":      0.2,
		},
		Extra: map[string]any{"openai_responses_supported": true},
	}

	result, err := svc.ForwardAsAnthropic(context.Background(), c, account, body, "", "gpt-5.4")

	require.Error(t, err)
	require.Nil(t, result)
	require.Equal(t, "gpt-5.4", gjson.GetBytes(upstream.lastBody, "model").String())
	require.False(t, gjson.GetBytes(upstream.lastBody, "temperature").Exists())
}

func TestOpenAIMessagesTemperaturePolicyKeepsGPT56Override(t *testing.T) {
	gin.SetMode(gin.TestMode)
	body := []byte(`{"model":"gpt-5.4","max_tokens":1024,"messages":[{"role":"user","content":"hello"}]}`)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusBadRequest,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(`{"error":{"message":"stop after request capture"}}`)),
	}}
	svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream}
	account := &Account{
		ID:          2,
		Name:        "openai-api-key",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Concurrency: 1,
		Credentials: map[string]any{
			"api_key":          "sk-test",
			"temperature_mode": "override",
			"temperature":      0.2,
		},
		Extra: map[string]any{"openai_responses_supported": true},
	}

	result, err := svc.ForwardAsAnthropic(context.Background(), c, account, body, "", "gpt-5.6-terra")

	require.Error(t, err)
	require.Nil(t, result)
	require.Equal(t, "gpt-5.6-terra", gjson.GetBytes(upstream.lastBody, "model").String())
	require.InDelta(t, 0.2, gjson.GetBytes(upstream.lastBody, "temperature").Float(), 1e-9)
}

func TestOpenAIChatResponsesShapeTemperaturePolicyUsesMappedGPT56(t *testing.T) {
	gin.SetMode(gin.TestMode)
	body := []byte(`{"model":"gpt-5.4","input":"hello","stream":false}`)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusBadRequest,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(`{"error":{"message":"stop after request capture"}}`)),
	}}
	svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream}
	account := &Account{
		ID:          2,
		Name:        "openai-api-key",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Concurrency: 1,
		Credentials: map[string]any{
			"api_key":          "sk-test",
			"temperature_mode": "override",
			"temperature":      0.2,
		},
		Extra: map[string]any{"openai_responses_supported": true},
	}

	result, err := svc.ForwardAsChatCompletions(context.Background(), c, account, body, "", "gpt-5.6-sol")

	require.Error(t, err)
	require.Nil(t, result)
	require.Equal(t, "gpt-5.6-sol", gjson.GetBytes(upstream.lastBody, "model").String())
	require.InDelta(t, 0.2, gjson.GetBytes(upstream.lastBody, "temperature").Float(), 1e-9)
}

func TestOpenAIResponsesTemperaturePolicyRespectsModelCapabilities(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tests := []struct {
		name            string
		model           string
		mappedModel     string
		wantTemperature bool
	}{
		{name: "supported model keeps account override", model: "gpt-4.1", wantTemperature: true},
		{name: "gpt 5.6 keeps account override", model: "gpt-5.6-luna", wantTemperature: true},
		{name: "legacy gpt model mapped to gpt 5.6 keeps account override", model: "gpt-5.4", mappedModel: "gpt-5.6-terra", wantTemperature: true},
		{name: "reasoning model removes unsupported override", model: "gpt-5.4"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := []byte(`{"model":"` + tt.model + `","input":"hello","stream":false}`)
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))
			c.Request.Header.Set("Content-Type", "application/json")

			upstream := &httpUpstreamRecorder{resp: &http.Response{
				StatusCode: http.StatusBadRequest,
				Header:     http.Header{"Content-Type": []string{"application/json"}},
				Body:       io.NopCloser(strings.NewReader(`{"error":{"message":"stop after request capture"}}`)),
			}}
			svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream}
			credentials := map[string]any{
				"api_key":          "sk-test",
				"temperature_mode": "override",
				"temperature":      0.2,
			}
			if tt.mappedModel != "" {
				credentials["model_mapping"] = map[string]any{tt.model: tt.mappedModel}
			}
			account := &Account{
				ID:          2,
				Name:        "openai-api-key",
				Platform:    PlatformOpenAI,
				Type:        AccountTypeAPIKey,
				Concurrency: 1,
				Credentials: credentials,
				Extra:       map[string]any{"openai_responses_supported": true},
			}

			result, err := svc.Forward(context.Background(), c, account, body)

			require.Error(t, err)
			require.Nil(t, result)
			temperature := gjson.GetBytes(upstream.lastBody, "temperature")
			require.Equal(t, tt.wantTemperature, temperature.Exists())
			if tt.wantTemperature {
				require.InDelta(t, 0.2, temperature.Float(), 1e-9)
			}
		})
	}
}
