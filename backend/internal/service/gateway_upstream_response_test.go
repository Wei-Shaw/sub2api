package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestExtractUpstreamErrorMessageFormats(t *testing.T) {
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
			require.Equal(t, tt.want, extractUpstreamErrorMessage([]byte(tt.body)))
		})
	}
}

func TestExtractUpstreamErrorCodeFormats(t *testing.T) {
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
			require.Equal(t, tt.want, extractUpstreamErrorCode([]byte(tt.body)))
		})
	}
}
