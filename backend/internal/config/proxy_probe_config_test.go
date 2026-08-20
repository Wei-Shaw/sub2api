//go:build unit

package config

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNormalizeProxyProbeURLs(t *testing.T) {
	t.Parallel()

	got, err := normalizeProxyProbeURLs([]ProbeURLConfig{
		{URL: " https://chatgpt.com/cdn-cgi/trace ", Parser: " CHATGPT-TRACE "REDACTED,
		{URL: "https://api64.ipify.org?format=json", Parser: "ipify"REDACTED,
REDACTED)
REDACTED
	require.Equal(t, []ProbeURLConfig{
		{URL: "https://chatgpt.com/cdn-cgi/trace", Parser: "chatgpt-trace"REDACTED,
		{URL: "https://api64.ipify.org?format=json", Parser: "ipify"REDACTED,
REDACTED, got)
REDACTED

func TestNormalizeProxyProbeURLsRejectsInvalidEntries(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		target  ProbeURLConfig
		wantErr string
REDACTED{
		{name: "missing URL", target: ProbeURLConfig{Parser: "ipify"REDACTED, wantErr: "url is required"REDACTED,
		{name: "missing parser", target: ProbeURLConfig{URL: "https://example.com"REDACTED, wantErr: "parser is required"REDACTED,
		{name: "unknown parser", target: ProbeURLConfig{URL: "https://example.com", Parser: "ip_api"REDACTED, wantErr: "unsupported parser"REDACTED,
		{name: "relative URL", target: ProbeURLConfig{URL: "/cdn-cgi/trace", Parser: "chatgpt-trace"REDACTED, wantErr: "invalid url"REDACTED,
		{name: "unsupported scheme", target: ProbeURLConfig{URL: "ftp://example.com/file", Parser: "ipify"REDACTED, wantErr: "scheme must be http or https"REDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, err := normalizeProxyProbeURLs([]ProbeURLConfig{tt.targetREDACTED)
			require.ErrorContains(t, err, tt.wantErr)
	REDACTED)
REDACTED
REDACTED
