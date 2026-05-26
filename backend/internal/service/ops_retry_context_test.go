package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNewOpsRetryContext_SetsHTTPTransportAndRequestHeaders(t *testing.T) {
	errorLog := &OpsErrorLogDetail{
		OpsErrorLog: OpsErrorLog{
			RequestPath: "/openai/v1/responses",
		},
		UserAgent: "ops-retry-agent/1.0",
		RequestHeaders: `{
			"anthropic-beta":"beta-v1",
			"ANTHROPIC-VERSION":"2023-06-01",
			"authorization":"Bearer should-not-forward"
		}`,
	}

	c, w := newOpsRetryContext(context.Background(), errorLog)
	require.NotNil(t, c)
	require.NotNil(t, w)
	require.NotNil(t, c.Request)

	require.Equal(t, "/openai/v1/responses", c.Request.URL.Path)
	require.Equal(t, "application/json", c.Request.Header.Get("Content-Type"))
	require.Equal(t, "ops-retry-agent/1.0", c.Request.Header.Get("User-Agent"))
	require.Equal(t, "beta-v1", c.Request.Header.Get("anthropic-beta"))
	require.Equal(t, "2023-06-01", c.Request.Header.Get("anthropic-version"))
	require.Empty(t, c.Request.Header.Get("authorization"), "未在白名单内的敏感头不应被重放")
	require.Equal(t, OpenAIClientTransportHTTP, GetOpenAIClientTransport(c))
}

func TestNewOpsRetryContext_InvalidHeadersJSONStillSetsHTTPTransport(t *testing.T) {
	errorLog := &OpsErrorLogDetail{
		RequestHeaders: "{invalid-json",
	}

	c, _ := newOpsRetryContext(context.Background(), errorLog)
	require.NotNil(t, c)
	require.NotNil(t, c.Request)
	require.Equal(t, "/", c.Request.URL.Path)
	require.Equal(t, OpenAIClientTransportHTTP, GetOpenAIClientTransport(c))
}
