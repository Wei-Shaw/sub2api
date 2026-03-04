package service

import (
	"bytes"
	"errors"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func TestResolveUpstreamResponseReadLimit(t *testing.T) {
	t.Run("use default when config missing", func(t *testing.T) {
		require.Equal(t, defaultUpstreamResponseReadMaxBytes, resolveUpstreamResponseReadLimit(nil))
REDACTED)

	t.Run("use configured value", func(t *testing.T) {
		cfg := &config.Config{REDACTED
		cfg.Gateway.UpstreamResponseReadMaxBytes = 1234
		require.Equal(t, int64(1234), resolveUpstreamResponseReadLimit(cfg))
REDACTED)
REDACTED

func TestReadUpstreamResponseBodyLimited(t *testing.T) {
	t.Run("within limit", func(t *testing.T) {
		body, err := readUpstreamResponseBodyLimited(bytes.NewReader([]byte("ok")), 2)
	REDACTED
		require.Equal(t, []byte("ok"), body)
REDACTED)

	t.Run("exceeds limit", func(t *testing.T) {
		body, err := readUpstreamResponseBodyLimited(bytes.NewReader([]byte("toolong")), 3)
		require.Nil(t, body)
	REDACTED
		require.True(t, errors.Is(err, ErrUpstreamResponseBodyTooLarge))
REDACTED)
REDACTED
