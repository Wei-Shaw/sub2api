//go:build unit

package service

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func modelProbeTestConfig() *config.Config {
	return &config.Config{
		Security: config.SecurityConfig{
			URLAllowlist: config.URLAllowlistConfig{
				AllowInsecureHTTP: true,
			},
		},
	}
}

func TestAccountTestService_ModelProbeOpenAIListModels(t *testing.T) {
	upstream := &queuedHTTPUpstream{responses: []*http.Response{
		newJSONResponse(http.StatusOK, `{"object":"list","data":[{"id":"gpt-5.4","object":"model","owned_by":"openai"},{"id":"gpt-5.4-openai-compact","object":"model","owned_by":"openai"}]}`),
	}}
	svc := &AccountTestService{httpUpstream: upstream, cfg: modelProbeTestConfig()}

	result, err := svc.ProbeModelList(context.Background(), ModelProbeListInput{
		Platform: PlatformOpenAI,
		BaseURL:  "https://one.example.com",
		APIKey:   "sk-test-secret",
	})

	require.NoError(t, err)
	require.Len(t, result.Models, 2)
	require.Equal(t, "gpt-5.4", result.Models[0].ID)
	require.Equal(t, "gpt-5.4-openai-compact", result.Models[1].ID)
	require.Len(t, upstream.requests, 1)
	req := upstream.requests[0]
	require.Equal(t, http.MethodGet, req.Method)
	require.Equal(t, "https://one.example.com/v1/models", req.URL.String())
	require.Equal(t, "Bearer sk-test-secret", req.Header.Get("Authorization"))
}

func TestAccountTestService_ModelProbeOpenAIResponsesMinimalRequest(t *testing.T) {
	upstream := &queuedHTTPUpstream{responses: []*http.Response{
		newJSONResponse(http.StatusOK, `{"id":"resp_123","output":[]}`),
	}}
	svc := &AccountTestService{httpUpstream: upstream, cfg: modelProbeTestConfig()}

	result, err := svc.ProbeModels(context.Background(), ModelProbeTestInput{
		Platform: PlatformOpenAI,
		BaseURL:  "https://one.example.com",
		APIKey:   "sk-test-secret",
		Mode:     ModelProbeModeOpenAIResponses,
		Models:   []string{"gpt-5.4"},
	})

	require.NoError(t, err)
	require.Len(t, result.Results, 1)
	require.True(t, result.Results[0].OK)
	require.Equal(t, http.StatusOK, result.Results[0].Status)
	require.Len(t, upstream.requests, 1)
	req := upstream.requests[0]
	require.Equal(t, http.MethodPost, req.Method)
	require.Equal(t, "https://one.example.com/v1/responses", req.URL.String())

	body, err := io.ReadAll(req.Body)
	require.NoError(t, err)
	require.JSONEq(t, `{"model":"gpt-5.4","input":"ping","max_output_tokens":1}`, string(body))
}

func TestAccountTestService_ModelProbeAnthropicListUsesBuiltInCandidates(t *testing.T) {
	upstream := &queuedHTTPUpstream{}
	svc := &AccountTestService{httpUpstream: upstream, cfg: modelProbeTestConfig()}

	result, err := svc.ProbeModelList(context.Background(), ModelProbeListInput{
		Platform: PlatformAnthropic,
		BaseURL:  "https://invalid.example.com",
		APIKey:   "sk-ant-test",
	})

	require.NoError(t, err)
	require.NotEmpty(t, result.Models)
	require.Equal(t, "anthropic", result.Models[0].OwnedBy)
	require.Empty(t, upstream.requests)
}

func TestAccountTestService_ModelProbeOpenAIChatCompletionsMinimalRequest(t *testing.T) {
	upstream := &queuedHTTPUpstream{responses: []*http.Response{
		newJSONResponse(http.StatusOK, `{"id":"chatcmpl_123","choices":[]}`),
	}}
	svc := &AccountTestService{httpUpstream: upstream, cfg: modelProbeTestConfig()}

	result, err := svc.ProbeModels(context.Background(), ModelProbeTestInput{
		Platform: PlatformOpenAI,
		BaseURL:  "https://one.example.com/v1",
		APIKey:   "sk-test-secret",
		Mode:     ModelProbeModeOpenAIChatCompletions,
		Models:   []string{"gpt-5.4"},
	})

	require.NoError(t, err)
	require.True(t, result.Results[0].OK)
	require.Len(t, upstream.requests, 1)
	req := upstream.requests[0]
	require.Equal(t, "https://one.example.com/v1/chat/completions", req.URL.String())

	body, err := io.ReadAll(req.Body)
	require.NoError(t, err)
	require.JSONEq(t, `{"model":"gpt-5.4","messages":[{"role":"user","content":"ping"}],"max_completion_tokens":1}`, string(body))
}

func TestAccountTestService_ModelProbeOpenAIChatCompletionsFallsBackToMaxTokens(t *testing.T) {
	upstream := &queuedHTTPUpstream{responses: []*http.Response{
		newJSONResponse(http.StatusBadRequest, `{"error":{"message":"Unknown parameter: max_completion_tokens"}}`),
		newJSONResponse(http.StatusOK, `{"id":"chatcmpl_123","choices":[]}`),
	}}
	svc := &AccountTestService{httpUpstream: upstream, cfg: modelProbeTestConfig()}

	result, err := svc.ProbeModels(context.Background(), ModelProbeTestInput{
		Platform: PlatformOpenAI,
		BaseURL:  "https://one.example.com/v1",
		APIKey:   "sk-test-secret",
		Mode:     ModelProbeModeOpenAIChatCompletions,
		Models:   []string{"gpt-5.4"},
	})

	require.NoError(t, err)
	require.Len(t, result.Results, 1)
	require.True(t, result.Results[0].OK)
	require.Len(t, upstream.requests, 2)

	firstBody, err := io.ReadAll(upstream.requests[0].Body)
	require.NoError(t, err)
	require.JSONEq(t, `{"model":"gpt-5.4","messages":[{"role":"user","content":"ping"}],"max_completion_tokens":1}`, string(firstBody))

	secondBody, err := io.ReadAll(upstream.requests[1].Body)
	require.NoError(t, err)
	require.JSONEq(t, `{"model":"gpt-5.4","messages":[{"role":"user","content":"ping"}],"max_tokens":1}`, string(secondBody))
}

func TestAccountTestService_ModelProbeErrorDoesNotExposeAPIKey(t *testing.T) {
	upstream := &queuedHTTPUpstream{responses: []*http.Response{
		newJSONResponse(http.StatusUnauthorized, `{"error":{"message":"invalid key sk-test-secret"}}`),
	}}
	svc := &AccountTestService{httpUpstream: upstream, cfg: modelProbeTestConfig()}

	result, err := svc.ProbeModels(context.Background(), ModelProbeTestInput{
		Platform: PlatformOpenAI,
		BaseURL:  "https://one.example.com",
		APIKey:   "sk-test-secret",
		Mode:     ModelProbeModeOpenAIResponses,
		Models:   []string{"gpt-5.4"},
	})

	require.NoError(t, err)
	require.Len(t, result.Results, 1)
	require.False(t, result.Results[0].OK)
	require.Equal(t, http.StatusUnauthorized, result.Results[0].Status)
	require.NotContains(t, result.Results[0].Error, "sk-test-secret")
	require.Contains(t, strings.ToLower(result.Results[0].Error), "invalid key")
}

func TestAccountTestService_ModelProbeErrorDoesNotExposePartialAPIKey(t *testing.T) {
	upstream := &queuedHTTPUpstream{responses: []*http.Response{
		newJSONResponse(http.StatusUnauthorized, `{"error":{"message":"invalid key sk-test-sec..."}}`),
	}}
	svc := &AccountTestService{httpUpstream: upstream, cfg: modelProbeTestConfig()}

	result, err := svc.ProbeModels(context.Background(), ModelProbeTestInput{
		Platform: PlatformOpenAI,
		BaseURL:  "https://one.example.com",
		APIKey:   "sk-test-secret",
		Mode:     ModelProbeModeOpenAIResponses,
		Models:   []string{"gpt-5.4"},
	})

	require.NoError(t, err)
	require.Len(t, result.Results, 1)
	require.False(t, result.Results[0].OK)
	require.NotContains(t, result.Results[0].Error, "sk-test")
	require.Contains(t, strings.ToLower(result.Results[0].Error), "invalid key")
}
