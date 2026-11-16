//go:build unit

package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGrokUpstreamErrorMessageFormats(t *testing.T) {
	for _, tt := range []struct {
		name string
		body string
		want string
	}{
		{"xai string error", `{"code":"invalid_image","error":"Image could not be decoded."}`, "Image could not be decoded."},
		{"nested message", `{"error":{"message":"nested"},"detail":"detail","message":"message"}`, "nested"},
		{"embedded nested message", `{"error":{"message":"{\"error\":{\"message\":\"embedded\"}}"}}`, "embedded"},
		{"detail retains priority", `{"error":"xai","detail":"detail","message":"message"}`, "detail"},
		{"message retains priority", `{"error":"xai","message":"message"}`, "message"},
		{"blank legacy fields", `{"error":"xai","detail":" ","message":" "}`, "xai"},
		{"unknown error object", `{"error":{"debug":"private diagnostic"}}`, ""},
		{"error array", `{"error":["private diagnostic"]}`, ""},
		{"numeric error", `{"error":400}`, ""},
		{"null error", `{"error":null}`, ""},
		{"plain text", `upstream internal diagnostic`, ""},
		{"empty", ``, ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, grokUpstreamErrorMessage([]byte(tt.body)))
		})
	}
}

func TestGrokUpstreamErrorCodeFormats(t *testing.T) {
	for _, tt := range []struct {
		name string
		body string
		want string
	}{
		{"xai string code", `{"code":" invalid_image ","error":"Image could not be decoded."}`, "invalid_image"},
		{"numeric code", `{"code":400,"error":"Bad input"}`, "400"},
		{"nested code retains priority", `{"error":{"code":"nested"},"code":"top"}`, "nested"},
		{"embedded code retains priority", `{"error":{"message":"{\"error\":{\"code\":\"embedded\"}}"},"code":"top"}`, "embedded"},
		{"embedded code with suffix", `{"error":{"message":"{\"error\":{\"code\":\"embedded\"}} (upstream)"},"code":"top"}`, "embedded"},
		{"embedded message without code", `{"error":{"message":"{\"error\":{\"message\":\"reason\"}}"},"code":"top"}`, "top"},
		{"blank nested code", `{"error":{"code":" "},"code":"top"}`, "top"},
		{"object code", `{"code":{"private":"diagnostic"}}`, ""},
		{"array code", `{"code":["diagnostic"]}`, ""},
		{"boolean code", `{"code":true}`, ""},
		{"null code", `{"code":null}`, ""},
		{"no code", `{"error":{"message":"reason"}}`, ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, grokUpstreamErrorCode([]byte(tt.body)))
		})
	}
}

// The shared extractors drive failover, model-not-found cooldown and account
// disable decisions on every platform; xAI's flat shape must stay Grok-only.
func TestSharedUpstreamErrorExtractorsIgnoreXAIFlatShape(t *testing.T) {
	flat := []byte(`{"code":"invalid_image","error":"Image could not be decoded."}`)
	require.Empty(t, extractUpstreamErrorMessage(flat))
	require.Empty(t, extractUpstreamErrorCode(flat))

	require.Empty(t, extractUpstreamErrorCode([]byte(`{"object":"error","message":"m","code":400}`)))
	require.Empty(t, extractUpstreamErrorCode([]byte(`{"error":{"message":"m"},"code":"token_revoked"}`)))
}

func TestOpenAICompatibleModelNotFoundIgnoresTopLevelCode(t *testing.T) {
	for _, body := range []string{
		`{"object":"error","message":"model not found: foo","type":"BadRequestError","param":null,"code":400}`,
		`{"code":"invalid_argument","message":"Unknown model: foo"}`,
		`{"error":{"message":"The model is not supported"},"code":"bad_request"}`,
	} {
		t.Run(body, func(t *testing.T) {
			require.True(t, isOpenAICompatibleModelNotFoundBody([]byte(body)))
			require.True(t, IsOpenAICompatibleModelNotFound400([]byte(body)))
		})
	}
}
