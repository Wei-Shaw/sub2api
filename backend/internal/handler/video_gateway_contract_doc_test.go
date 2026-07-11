package handler

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/require"
)

func TestVideoGatewayContractDocumentsCurrentQCanvasBoundary(t *testing.T) {
	path := filepath.Clean(filepath.Join("..", "..", "..", "docs", "api", "video-gateway-contract.md"))
	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	require.True(t, utf8.Valid(raw), "video gateway contract must be valid UTF-8")
	content := string(raw)
	require.Contains(t, content, "内部 mock 可演示")
	require.Contains(t, content, "QCanvas")
	require.NotContains(t, content, "鐘舵")

	for _, endpoint := range []string{
		"`/api/v1/video/tasks`",
		"`/v1/video/tasks`",
		"`/v1/video/tasks/{id}`",
		"`/v1/video/tasks/{id}/cancel`",
	} {
		require.True(t, strings.Contains(content, endpoint), "contract is missing endpoint %s", endpoint)
	}

	for _, field := range []string{
		"`id`",
		"`status`",
		"`result_url`",
		"`error_message`",
		"`provider`",
		"`mock_only`",
		"`provider_boundary`",
		"`real_provider_dispatch_count`",
		"`usage.total_tokens`",
		"`actual_resolution`",
		"`actual_duration`",
		"`settlement_status`",
		"`delivery_status`",
	} {
		require.True(t, strings.Contains(content, field), "contract is missing field %s", field)
	}

	for _, boundary := range []string{
		"api-key-video-mock-only",
		"api-key-video-seedance-production",
		"api-key-video-seedance-tiny-trial",
	} {
		require.Contains(t, content, boundary)
	}

	for _, reason := range []string{
		"VIDEO_PRODUCTION_NOT_AUTHORIZED",
		"VIDEO_PROVIDER_DISABLED",
		"VIDEO_TRIAL_BLOCKED",
		"VIDEO_TRIAL_LIMIT_EXCEEDED",
		"IDEMPOTENCY_KEY_CONFLICT",
	} {
		require.Contains(t, content, reason)
	}

	require.Contains(t, content, "billing reservation")
	require.Contains(t, content, "持久交付")
	require.Contains(t, content, "非生产 READY")
}
