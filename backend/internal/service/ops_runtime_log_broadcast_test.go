package service

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
)

// fakeRuntimeLogBroadcaster 是 RuntimeLogBroadcaster 的内存实现，
// 用于验证 UpdateRuntimeLogConfig / ResetRuntimeLogConfig 会触发跨实例广播，
// 以及订阅端会在收到广播时从 DB 重新读取并应用新配置。
type fakeRuntimeLogBroadcaster struct {
	publishCount atomic.Int32
	mu           sync.Mutex
	handlers     []func()
}

func (f *fakeRuntimeLogBroadcaster) Publish(ctx context.Context) error {
	f.publishCount.Add(1)
	f.mu.Lock()
	handlers := append([]func(){}, f.handlers...)
	f.mu.Unlock()
	for _, h := range handlers {
		h()
	}
	return nil
}

func (f *fakeRuntimeLogBroadcaster) Subscribe(_ context.Context, handler func()) {
	if handler == nil {
		return
	}
	f.mu.Lock()
	f.handlers = append(f.handlers, handler)
	f.mu.Unlock()
}

func newLoggerForBroadcastTest(t *testing.T) {
	t.Helper()
	if err := logger.Init(logger.InitOptions{
		Level:       "info",
		Format:      "json",
		ServiceName: "sub2api",
		Environment: "test",
		Output: logger.OutputOptions{
			ToStdout: true,
			ToFile:   false,
		},
	}); err != nil {
		t.Fatalf("init logger: %v", err)
	}
}

func TestUpdateRuntimeLogConfig_TriggersBroadcast(t *testing.T) {
	newLoggerForBroadcastTest(t)

	repo := newRuntimeSettingRepoStub()
	broadcaster := &fakeRuntimeLogBroadcaster{}
	svc := &OpsService{
		settingRepo:           repo,
		cfg:                   &config.Config{Log: config.LogConfig{Level: "info", Caller: true, StacktraceLevel: "error", Sampling: config.LogSamplingConfig{Initial: 100, Thereafter: 100}}},
		runtimeLogBroadcaster: broadcaster,
	}

	if _, err := svc.UpdateRuntimeLogConfig(context.Background(), &OpsRuntimeLogConfig{
		Level:           "warn",
		SamplingInitial: 100,
		SamplingNext:    100,
		Caller:          true,
		StacktraceLevel: "error",
		RetentionDays:   30,
	}, 42); err != nil {
		t.Fatalf("UpdateRuntimeLogConfig: %v", err)
	}
	if got := broadcaster.publishCount.Load(); got != 1 {
		t.Fatalf("expected 1 broadcast after update, got %d", got)
	}
}

func TestResetRuntimeLogConfig_TriggersBroadcast(t *testing.T) {
	newLoggerForBroadcastTest(t)

	repo := newRuntimeSettingRepoStub()
	broadcaster := &fakeRuntimeLogBroadcaster{}
	svc := &OpsService{
		settingRepo:           repo,
		cfg:                   &config.Config{Log: config.LogConfig{Level: "info", Caller: true, StacktraceLevel: "error", Sampling: config.LogSamplingConfig{Initial: 100, Thereafter: 100}}},
		runtimeLogBroadcaster: broadcaster,
	}

	if _, err := svc.ResetRuntimeLogConfig(context.Background(), 7); err != nil {
		t.Fatalf("ResetRuntimeLogConfig: %v", err)
	}
	if got := broadcaster.publishCount.Load(); got != 1 {
		t.Fatalf("expected 1 broadcast after reset, got %d", got)
	}
}

func TestStartRuntimeLogSubscriber_AppliesConfigOnBroadcast(t *testing.T) {
	newLoggerForBroadcastTest(t)

	repo := newRuntimeSettingRepoStub()
	broadcaster := &fakeRuntimeLogBroadcaster{}
	svc := &OpsService{
		settingRepo:           repo,
		cfg:                   &config.Config{Log: config.LogConfig{Level: "info", Caller: true, StacktraceLevel: "error", Sampling: config.LogSamplingConfig{Initial: 100, Thereafter: 100}}},
		runtimeLogBroadcaster: broadcaster,
	}

	// 起始等级设为 info。
	if err := applyOpsRuntimeLogConfig(defaultOpsRuntimeLogConfig(svc.cfg)); err != nil {
		t.Fatalf("prime logger: %v", err)
	}
	if logger.CurrentLevel() != "info" {
		t.Fatalf("prelude level = %q, want info", logger.CurrentLevel())
	}

	// 模拟另一节点已经把 debug 配置写入 DB（本节点还未感知）。
	repo.values[SettingKeyOpsRuntimeLogConfig] = `{"level":"debug","enable_sampling":false,"sampling_initial":100,"sampling_thereafter":100,"caller":true,"stacktrace_level":"error","retention_days":30}`

	// 启动订阅并触发一次广播 —— 订阅 handler 应从 DB 拉最新配置并 apply。
	svc.StartRuntimeLogSubscriber(context.Background())
	if err := broadcaster.Publish(context.Background()); err != nil {
		t.Fatalf("publish: %v", err)
	}

	// Publish 在 fake 里同步调用 handler，此处直接断言即可；给一个很短的宽限窗口以防未来改成异步。
	deadline := time.Now().Add(200 * time.Millisecond)
	for logger.CurrentLevel() != "debug" && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if got := logger.CurrentLevel(); got != "debug" {
		t.Fatalf("logger level after broadcast = %q, want debug", got)
	}
}

func TestNotifyRuntimeLogConfigChanged_NoBroadcasterIsNoop(t *testing.T) {
	// 未注入 broadcaster 时不能 panic，且不影响正常返回。
	svc := &OpsService{}
	svc.notifyRuntimeLogConfigChanged(context.Background())
}
