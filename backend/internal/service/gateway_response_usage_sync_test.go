package service

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestHandleNonStreamingResponse_UsageAlignedWithClaudeMaxSimulation(t *testing.T) {
	gin.SetMode(gin.TestMode)

	svc := &GatewayService{
		cfg:              &config.Config{},
		rateLimitService: &RateLimitService{},
	}

	account := &Account{
		ID:       11,
		Platform: PlatformAnthropic,
		Type:     AccountTypeOAuth,
		Extra: map[string]any{
			"cache_ttl_override_enabled": true,
			"cache_ttl_override_target":  "5m",
		},
	}
	group := &Group{
		ID:                       99,
		Platform:                 PlatformAnthropic,
		SimulateClaudeMaxEnabled: true,
	}
	parsed := &ParsedRequest{
		Model: "claude-sonnet-4",
		Messages: []any{
			map[string]any{
				"role": "user",
				"content": []any{
					map[string]any{
						"type":          "text",
						"text":          "long cached context",
						"cache_control": map[string]any{"type": "ephemeral"},
					},
					map[string]any{
						"type": "text",
						"text": "new user question",
					},
				},
			},
		},
	}

	upstreamBody := []byte(`{"id":"msg_1","model":"claude-sonnet-4","usage":{"input_tokens":120,"output_tokens":8}}`)
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       ioNopCloserBytes(upstreamBody),
	}

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewReader(nil))
	c.Set("api_key", &APIKey{Group: group})
	requestCtx := withClaudeMaxResponseRewriteContext(context.Background(), c, parsed)

	usage, err := svc.handleNonStreamingResponse(requestCtx, resp, c, account, "claude-sonnet-4", "claude-sonnet-4")
	require.NoError(t, err)
	require.NotNil(t, usage)

	var rendered struct {
		Usage ClaudeUsage `json:"usage"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &rendered))
	rendered.Usage.CacheCreation5mTokens = int(gjson.GetBytes(rec.Body.Bytes(), "usage.cache_creation.ephemeral_5m_input_tokens").Int())
	rendered.Usage.CacheCreation1hTokens = int(gjson.GetBytes(rec.Body.Bytes(), "usage.cache_creation.ephemeral_1h_input_tokens").Int())

	require.Equal(t, rendered.Usage.InputTokens, usage.InputTokens)
	require.Equal(t, rendered.Usage.OutputTokens, usage.OutputTokens)
	require.Equal(t, rendered.Usage.CacheCreationInputTokens, usage.CacheCreationInputTokens)
	require.Equal(t, rendered.Usage.CacheCreation5mTokens, usage.CacheCreation5mTokens)
	require.Equal(t, rendered.Usage.CacheCreation1hTokens, usage.CacheCreation1hTokens)
	require.Equal(t, rendered.Usage.CacheReadInputTokens, usage.CacheReadInputTokens)

	require.Greater(t, usage.CacheCreation1hTokens, 0)
	require.Equal(t, 0, usage.CacheCreation5mTokens)
	require.Less(t, usage.InputTokens, 120)
}

func TestHandleNonStreamingResponse_ClaudeMaxDisabled_NoSimulationIntercept(t *testing.T) {
	gin.SetMode(gin.TestMode)

	svc := &GatewayService{
		cfg:              &config.Config{},
		rateLimitService: &RateLimitService{},
	}

	account := &Account{
		ID:       12,
		Platform: PlatformAnthropic,
		Type:     AccountTypeOAuth,
		Extra: map[string]any{
			"cache_ttl_override_enabled": true,
			"cache_ttl_override_target":  "5m",
		},
	}
	group := &Group{
		ID:                       100,
		Platform:                 PlatformAnthropic,
		SimulateClaudeMaxEnabled: false,
	}
	parsed := &ParsedRequest{
		Model: "claude-sonnet-4",
		Messages: []any{
			map[string]any{
				"role": "user",
				"content": []any{
					map[string]any{
						"type":          "text",
						"text":          "long cached context",
						"cache_control": map[string]any{"type": "ephemeral"},
					},
					map[string]any{
						"type": "text",
						"text": "new user question",
					},
				},
			},
		},
	}

	upstreamBody := []byte(`{"id":"msg_2","model":"claude-sonnet-4","usage":{"input_tokens":120,"output_tokens":8}}`)
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       ioNopCloserBytes(upstreamBody),
	}

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewReader(nil))
	c.Set("api_key", &APIKey{Group: group})
	requestCtx := withClaudeMaxResponseRewriteContext(context.Background(), c, parsed)

	usage, err := svc.handleNonStreamingResponse(requestCtx, resp, c, account, "claude-sonnet-4", "claude-sonnet-4")
	require.NoError(t, err)
	require.NotNil(t, usage)

	require.Equal(t, 120, usage.InputTokens)
	require.Equal(t, 0, usage.CacheCreationInputTokens)
	require.Equal(t, 0, usage.CacheCreation5mTokens)
	require.Equal(t, 0, usage.CacheCreation1hTokens)
}

func ioNopCloserBytes(b []byte) *readCloserFromBytes {
	return &readCloserFromBytes{Reader: bytes.NewReader(b)}
}

type readCloserFromBytes struct {
	*bytes.Reader
}

func (r *readCloserFromBytes) Close() error {
	return nil
}
