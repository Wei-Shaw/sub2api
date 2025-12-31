package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseGatewayRequest(t *testing.T) {
	body := []byte(`{"model":"claude-3-7-sonnet","stream":true,"metadata":{"user_id":"session_123e4567-e89b-12d3-a456-426614174000"REDACTED,"system":[{"type":"text","text":"hello","cache_control":{"type":"ephemeral"REDACTEDREDACTED],"messages":[{"content":"hi"REDACTED]REDACTED`)
	parsed, err := ParseGatewayRequest(body)
REDACTED
	require.Equal(t, "claude-3-7-sonnet", parsed.Model)
	require.True(t, parsed.Stream)
	require.Equal(t, "session_123e4567-e89b-12d3-a456-426614174000", parsed.MetadataUserID)
	require.True(t, parsed.HasSystem)
	require.NotNil(t, parsed.System)
	require.Len(t, parsed.Messages, 1)
REDACTED

func TestParseGatewayRequest_SystemNull(t *testing.T) {
	body := []byte(`{"model":"claude-3","system":nullREDACTED`)
	parsed, err := ParseGatewayRequest(body)
REDACTED
	require.False(t, parsed.HasSystem)
REDACTED

func TestParseGatewayRequest_InvalidModelType(t *testing.T) {
	body := []byte(`{"model":123REDACTED`)
	_, err := ParseGatewayRequest(body)
REDACTED
REDACTED

func TestParseGatewayRequest_InvalidStreamType(t *testing.T) {
	body := []byte(`{"stream":"true"REDACTED`)
	_, err := ParseGatewayRequest(body)
REDACTED
REDACTED
