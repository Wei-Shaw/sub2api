package config

import (
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"
)

func TestOpenAIAsyncConfigBounds(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	setOpenAIAsyncDefaults()
	var cfg struct {
		Async OpenAIAsyncConfig `mapstructure:"openai_async"`
	}
	require.NoError(t, viper.Unmarshal(&cfg))
	require.False(t, cfg.Async.Enabled)
	cfg.Async.Enabled = true
	require.NoError(t, cfg.Async.Validate())
	for _, change := range []func(*OpenAIAsyncConfig){func(c *OpenAIAsyncConfig) { c.MaxBatchRequests = 50001 }, func(c *OpenAIAsyncConfig) { c.MaxOutputBytes = 1 }, func(c *OpenAIAsyncConfig) { c.MaxOutputBytes = int64(c.MaxBatchRequests) * 1024 }, func(c *OpenAIAsyncConfig) { c.MaxFileBytes = 513 << 20 }, func(c *OpenAIAsyncConfig) { c.WorkerConcurrency = 0 }} {
		bad := cfg.Async
		change(&bad)
		require.Error(t, bad.Validate())
	}
}
