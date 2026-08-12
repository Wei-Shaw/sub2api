package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestUpstreamStatusCodeFromMessage(t *testing.T) {
	tests := []struct {
		message string
		want    int
	}{
		{`submit: fal upstream error (HTTP 400, request_id=apiz-711cff6cc3981a78): {"detail":"不支持的参数: image_urls"}`, 400},
		{"status 429: rate limited", 0},
		{"submit: upstream unavailable", 0},
		{"HTTP 999 invalid", 0},
	}
	for _, test := range tests {
		require.Equal(t, test.want, upstreamStatusCodeFromMessage(test.message), test.message)
	}
}

func TestSanitizeVideoErrorReasonRemovesPlatformNames(t *testing.T) {
	tests := map[string]string{
		"status 400: apiz upstream failed: invalid input":     "status 400: upstream failed: invalid input",
		"submit: atlascloud upstream failed, quota exhausted": "submit: upstream failed, quota exhausted",
		"fal upstream error (HTTP 422): invalid request":      "upstream error (HTTP 422): invalid request",
		"FAL.AI: status 503: temporarily unavailable":         "status 503: temporarily unavailable",
		"status 400: invalid input":                           "status 400: invalid input",
	}
	for input, expected := range tests {
		require.Equal(t, expected, SanitizeVideoErrorReason(input))
	}
}
