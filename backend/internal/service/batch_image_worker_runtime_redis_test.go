//go:build unit

package service_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/repository"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func TestBatchImageWorkerRuntime_StartupDoesNotCreateRedisBatchImageKeys(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() {
		_ = rdb.Close()
	})

	cfg := &config.Config{BatchImage: config.BatchImageConfig{
		QueueEnabled:                true,
		QueueReadyKey:               "batch_image:queue:ready",
		QueueDelayedKey:             "batch_image:queue:delayed",
		QueueActiveKey:              "batch_image:queue:active",
		InflightKeyPrefix:           "batch_image:queue:inflight:",
		LockKeyPrefix:               "batch_image:queue:lock:",
		InflightTTLSeconds:          60,
		JobLockTTLSeconds:           60,
		DelayedMoverIntervalSeconds: 60,
		RecoveryIntervalSeconds:     60,
		StaleActiveAfterSeconds:     60,
		DelayedMoveLimit:            10,
		RecoverLimit:                10,
	}}
	queue := repository.NewBatchImageQueue(rdb, cfg)
	worker := service.NewBatchImageWorker(queue, noopBatchImageProcessor{}, service.NewBatchImageWorkerOptionsFromConfig(cfg))
	runtime := service.NewBatchImageWorkerRuntime(worker, cfg)

	runtime.Start()
	require.Eventually(t, runtime.Running, time.Second, 10*time.Millisecond)
	runtime.Stop()

	for _, key := range mr.Keys() {
		require.False(t, strings.HasPrefix(key, "batch_image:"), "unexpected Redis key created at startup: %s", key)
	}
}

type noopBatchImageProcessor struct{}

func (noopBatchImageProcessor) Process(context.Context, string) (service.BatchImageProcessResult, error) {
	return service.BatchImageProcessResult{}, nil
}
