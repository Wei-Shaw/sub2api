package soraerror

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestIsCloudflareChallengeResponse(t *testing.T) {
	headers := make(http.Header)
	headers.Set("cf-mitigated", "challenge")
	require.True(t, IsCloudflareChallengeResponse(http.StatusForbidden, headers, []byte(`{"ok":falseREDACTED`)))

	require.True(t, IsCloudflareChallengeResponse(http.StatusTooManyRequests, nil, []byte(`<!DOCTYPE html><title>Just a moment...</title><script>window._cf_chl_opt={REDACTED;</script>`)))
	require.False(t, IsCloudflareChallengeResponse(http.StatusBadGateway, nil, []byte(`<!DOCTYPE html><title>Just a moment...</title>`)))
REDACTED

func TestExtractCloudflareRayID(t *testing.T) {
	headers := make(http.Header)
	headers.Set("cf-ray", "9d01b0e9ecc35829-SEA")
	require.Equal(t, "9d01b0e9ecc35829-SEA", ExtractCloudflareRayID(headers, nil))

	body := []byte(`<script>window._cf_chl_opt={cRay: '9cff2d62d83bb98d'REDACTED;</script>`)
	require.Equal(t, "9cff2d62d83bb98d", ExtractCloudflareRayID(nil, body))
REDACTED

func TestExtractUpstreamErrorCodeAndMessage(t *testing.T) {
	code, msg := ExtractUpstreamErrorCodeAndMessage([]byte(`{"error":{"code":"cf_shield_429","message":"rate limited"REDACTEDREDACTED`))
	require.Equal(t, "cf_shield_429", code)
	require.Equal(t, "rate limited", msg)

	code, msg = ExtractUpstreamErrorCodeAndMessage([]byte(`{"code":"unsupported_country_code","message":"not available"REDACTED`))
	require.Equal(t, "unsupported_country_code", code)
	require.Equal(t, "not available", msg)

	code, msg = ExtractUpstreamErrorCodeAndMessage([]byte(`plain text`))
	require.Equal(t, "", code)
	require.Equal(t, "plain text", msg)
REDACTED

func TestFormatCloudflareChallengeMessage(t *testing.T) {
	headers := make(http.Header)
	headers.Set("cf-ray", "9d03b68c086027a1-SEA")
	msg := FormatCloudflareChallengeMessage("blocked", headers, nil)
	require.Equal(t, "blocked (cf-ray: 9d03b68c086027a1-SEA)", msg)
REDACTED
