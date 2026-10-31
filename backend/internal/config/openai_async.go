package config

import (
	"fmt"

	"github.com/spf13/viper"
)

type OpenAIAsyncConfig struct {
	Enabled                   bool  `mapstructure:"enabled"`
	MaxFileBytes              int64 `mapstructure:"max_file_bytes"`
	UserTemporaryStorageBytes int64 `mapstructure:"user_temporary_storage_bytes"`
	UserStorageBytes          int64 `mapstructure:"user_storage_bytes"`
	FileTTLSeconds            int64 `mapstructure:"file_ttl_seconds"`
	MaxBatchRequests          int   `mapstructure:"max_batch_requests"`
	MaxActiveBatches          int   `mapstructure:"max_active_batches"`
	MaxLineBytes              int   `mapstructure:"max_line_bytes"`
	MaxOutputBytes            int64 `mapstructure:"max_output_bytes"`
	MaxResponseBytes          int64 `mapstructure:"max_response_bytes"`
	WorkerConcurrency         int   `mapstructure:"worker_concurrency"`
	RequestTimeoutSeconds     int   `mapstructure:"request_timeout_seconds"`
}

func setOpenAIAsyncDefaults() {
	viper.SetDefault("openai_async.enabled", false)
	viper.SetDefault("openai_async.max_file_bytes", 32<<20)
	viper.SetDefault("openai_async.user_storage_bytes", 1<<30)
	viper.SetDefault("openai_async.user_temporary_storage_bytes", 768<<20)
	viper.SetDefault("openai_async.file_ttl_seconds", 2592000)
	viper.SetDefault("openai_async.max_batch_requests", 1000)
	viper.SetDefault("openai_async.max_active_batches", 5)
	viper.SetDefault("openai_async.max_line_bytes", 1<<20)
	viper.SetDefault("openai_async.max_output_bytes", 128<<20)
	viper.SetDefault("openai_async.max_response_bytes", 8<<20)
	viper.SetDefault("openai_async.worker_concurrency", 2)
	viper.SetDefault("openai_async.request_timeout_seconds", 300)
}

func (c OpenAIAsyncConfig) Validate() error {
	if !c.Enabled {
		return nil
	}
	if c.MaxFileBytes < 1 || c.MaxFileBytes > 512<<20 || c.UserStorageBytes < c.MaxFileBytes || c.FileTTLSeconds < 3600 || c.FileTTLSeconds > 2592000 {
		return fmt.Errorf("openai_async: invalid file size, storage quota, or retention")
	}
	if c.MaxBatchRequests < 1 || c.MaxBatchRequests > 50000 || c.MaxActiveBatches < 1 || c.MaxActiveBatches > 100 || c.MaxLineBytes < 4096 || c.MaxLineBytes > 32<<20 {
		return fmt.Errorf("openai_async: invalid batch admission limits")
	}
	if c.UserTemporaryStorageBytes < c.MaxOutputBytes || c.UserTemporaryStorageBytes > c.UserStorageBytes || c.MaxOutputBytes > c.UserStorageBytes {
		return fmt.Errorf("openai_async: temporary storage must cover an output reservation and fit within user storage")
	}
	if c.MaxOutputBytes < int64(c.MaxBatchRequests)*4096 || c.MaxOutputBytes > 512<<20 || c.MaxResponseBytes < 1024 || c.MaxResponseBytes > c.MaxOutputBytes {
		return fmt.Errorf("openai_async: invalid output limits")
	}
	if c.WorkerConcurrency < 1 || c.WorkerConcurrency > 32 || c.RequestTimeoutSeconds < 1 || c.RequestTimeoutSeconds > 3600 {
		return fmt.Errorf("openai_async: invalid worker limits")
	}
	return nil
}
