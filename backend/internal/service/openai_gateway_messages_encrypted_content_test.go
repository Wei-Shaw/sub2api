//go:build unit

package service

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/openai_compat"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestForwardAsAnthropic_OpenAIInvalidEncryptedContentRetriesWithoutForeignReasoning(t *testing.T) {
	gin.SetMode(gin.TestMode)

	body := []byte(`{
		"model":"claude-sonnet-4-5",
		"max_tokens":32,
		"messages":[
			{"role":"user","content":"first"},
			{"role":"assistant","content":[
				{"type":"thinking","thinking":"keep this summary","signature":"CAIS.AQ=="},
				{"type":"text","text":"first answer"}
			]},
			{"role":"user","content":"continue"}
		],
		"stream":false
	}`)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	completed := strings.Join([]string{
		`data: {"type":"response.completed","response":{"id":"resp_recovered","object":"response","model":"gpt-5.4","status":"completed","output":[{"type":"message","id":"msg_1","role":"assistant","status":"completed","content":[{"type":"output_text","text":"recovered"}]}],"usage":{"input_tokens":5,"output_tokens":2,"total_tokens":7}}}`,
		"",
		"data: [DONE]",
		"",
	}, "\n")
	upstream := &httpUpstreamRecorder{responses: []*http.Response{
		{
			StatusCode: http.StatusBadRequest,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body: io.NopCloser(strings.NewReader(
				`{"error":{"type":"invalid_request_error","message":"The encrypted content CAIS.AQ== could not be Verified. Reason: Encrypted content could not be decrypted or parsed."}}`,
			)),
		},
		{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"text/event-stream"}, "x-request-id": []string{"rid_recovered"}},
			Body:       io.NopCloser(strings.NewReader(completed)),
		},
	}}
	svc := &OpenAIGatewayService{
		cfg:          rawChatCompletionsTestConfig(),
		httpUpstream: upstream,
	}
	account := rawChatCompletionsTestAccount()
	account.Extra = map[string]any{
		openai_compat.ExtraKeyResponsesMode:      string(openai_compat.ResponsesSupportModeAuto),
		openai_compat.ExtraKeyResponsesSupported: true,
	}

	result, err := svc.ForwardAsAnthropic(context.Background(), c, account, body, "", "gpt-5.4")

	require.NoError(t, err)
	require.NotNil(t, result)
	require.Len(t, upstream.bodies, 2)
	require.Equal(t, "CAIS.AQ==", gjson.GetBytes(upstream.bodies[0], `input.#(type=="reasoning").encrypted_content`).String())
	require.False(t, gjson.GetBytes(upstream.bodies[1], `input.#(type=="reasoning").encrypted_content`).Exists())
	require.False(t, gjson.GetBytes(upstream.bodies[1], `input.#(type=="reasoning")`).Exists())
	require.Equal(t, "first answer", gjson.GetBytes(upstream.bodies[1], `input.#(role=="assistant").content.0.text`).String())
	require.Contains(t, string(upstream.bodies[1]), `"text":"continue"`)
	require.Equal(t, "recovered", gjson.Get(rec.Body.String(), "content.0.text").String())
}

func TestIsOpenAIInvalidEncryptedContentResponse(t *testing.T) {
	tests := []struct {
		name string
		body string
		want bool
	}{
		{
			name: "error code",
			body: `{"error":{"code":"invalid_encrypted_content","message":"bad encrypted content"}}`,
			want: true,
		},
		{
			name: "verification message without code",
			body: `{"error":{"message":"The encrypted content CAIS.AQ== could not be Verified. Reason: Encrypted content could not bedecrypted or parsed."}}`,
			want: true,
		},
		{
			name: "unrelated bad request",
			body: `{"error":{"code":"invalid_request_error","message":"input is too long"}}`,
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, isOpenAIInvalidEncryptedContentResponse(http.StatusBadRequest, []byte(tt.body)))
		})
	}
	require.False(t, isOpenAIInvalidEncryptedContentResponse(http.StatusInternalServerError, []byte(tests[0].body)))
}
