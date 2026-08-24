package service

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func TestResolveModelsListReadLimit(t *testing.T) {
	t.Run("nil config uses default", func(t *testing.T) {
		require.Equal(t, config.DefaultModelsListReadMaxBytes, resolveModelsListReadLimit(nil))
REDACTED)

	t.Run("configured limit wins", func(t *testing.T) {
		cfg := &config.Config{REDACTED
		cfg.Gateway.ModelsListReadMaxBytes = 16 << 20
		require.Equal(t, int64(16<<20), resolveModelsListReadLimit(cfg))
REDACTED)

	t.Run("non-positive limit uses default", func(t *testing.T) {
		cfg := &config.Config{REDACTED
		require.Equal(t, config.DefaultModelsListReadMaxBytes, resolveModelsListReadLimit(cfg))
REDACTED)
REDACTED
