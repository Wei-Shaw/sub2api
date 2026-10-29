//go:build unit

package service

import (
	"fmt"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/stretchr/testify/require"
)

func TestDescribeInvalidJSON_TruncatedBody(t *testing.T) {
	// Simulates a body cut off mid-stream (e.g. partially consumed by middleware).
	body := []byte(`{"model":"claude-sonnet-4-6","messages":[{"role":"user","content":"hi`)

	err := DescribeInvalidJSON(body)

	require.Error(t, err)
	require.Contains(t, err.Error(), fmt.Sprintf("len=%d", len(body)))
	require.Contains(t, err.Error(), "unexpected end of JSON input")
}

func TestDescribeInvalidJSON_InvalidCharacterWithOffset(t *testing.T) {
	body := []byte(`{"model": bad}`)

	err := DescribeInvalidJSON(body)

	require.Error(t, err)
	require.Contains(t, err.Error(), "offset=11")
	require.Contains(t, err.Error(), "invalid character")
}

func TestDescribeInvalidJSON_DoesNotLeakBodyContent(t *testing.T) {
	secret := "sk-super-secret-value"
	body := []byte(`{"api_key":"` + secret + `","broken":`)

	err := DescribeInvalidJSON(body)

	require.Error(t, err)
	require.NotContains(t, err.Error(), secret)
}

func TestParseGatewayRequest_InvalidJSONErrorIsDiagnostic(t *testing.T) {
	body := []byte(`{"model":"claude-sonnet-4-6","messages":[`)

	_, err := ParseGatewayRequest(NewRequestBodyRef(body), domain.PlatformAnthropic)

	require.Error(t, err)
	require.True(t, strings.HasPrefix(err.Error(), "invalid json (len="), "error should carry diagnostics, got: %s", err.Error())
}
