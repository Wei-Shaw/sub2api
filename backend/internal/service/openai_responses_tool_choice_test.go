//go:build unit

package service

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestOpenAIGatewayService_NormalizesResponsesFunctionToolChoiceForAPIKeyUpstreams(t *testing.T) {
	tests := []struct {
		name        string
		passthrough bool
	}{
		{name: "standard forwarding"},
		{name: "passthrough forwarding", passthrough: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			upstream := &httpUpstreamRecorder{resp: &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"application/json"}},
				Body: io.NopCloser(strings.NewReader(
					`{"id":"resp_tool_choice","model":"gpt-5.4","output":[],"usage":{"input_tokens":1,"output_tokens":1,"total_tokens":2}}`,
				)),
			}}
			svc := newOpenAIImageGenerationControlTestService(upstream)
			c, _ := newOpenAIImageGenerationControlTestContext(true, "compatibility-test/1.0")
			account := newOpenAIImageGenerationControlTestAccount()
			if tt.passthrough {
				account.Extra = map[string]any{"openai_passthrough": true}
			}

			body := []byte(`{
				"model":"gpt-5.4",
				"instructions":"test function tool choice compatibility",
				"input":"check the weather",
				"stream":false,
				"tools":[{"type":"function","name":"get_weather","description":"Get weather","parameters":{"type":"object"}}],
				"tool_choice":{"type":"function","function":{"name":"get_weather"},"provider_extension":{"enabled":true}}
			}`)

			result, err := svc.Forward(context.Background(), c, account, body)
			require.NoError(t, err)
			require.NotNil(t, result)
			require.NotNil(t, upstream.lastReq)
			require.Equal(t, "function", gjson.GetBytes(upstream.lastBody, "tool_choice.type").String())
			require.Equal(t, "get_weather", gjson.GetBytes(upstream.lastBody, "tool_choice.name").String())
			require.False(t, gjson.GetBytes(upstream.lastBody, "tool_choice.function").Exists())
			require.True(t, gjson.GetBytes(upstream.lastBody, "tool_choice.provider_extension.enabled").Bool())
			require.Equal(t, "get_weather", gjson.GetBytes(upstream.lastBody, "tools.0.name").String())
		})
	}
}
