package service

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
	"unsafe"
)

// 本文件只使用构造函数、enqueueAsync/enqueueRecord、计数器与真实 worker 等既有接口，
// 用来钉住异步审核队列对外可见的契约：空闲实例不得预留整条队列，QueueSize 仍是
// 丢弃阈值，零值服务入队仍是 no-op，真实 worker 消费不丢不重。

const contentModerationQueueChildProcessEnv = "SUB2API_CONTENT_MODERATION_QUEUE_CHILD"

// TestContentModerationIdleServiceDoesNotReserveQueue 是确定性的结构断言：
// 构造后、首次入队前不得已分配任务队列。旧实现在构造时就创建
// maxContentModerationQueueSize 深度的 channel。
func TestContentModerationIdleServiceDoesNotReserveQueue(t *testing.T) {
	svc := NewContentModerationService(nil, nil, nil, nil, nil, nil, nil, nil)
	if reserved := cap(svc.asyncQueue); reserved != 0 {
		t.Fatalf("new ContentModerationService reserved a %d-slot task queue (%d bytes) before any enqueue, want none",
			reserved, reserved*int(unsafe.Sizeof(contentModerationTask{})))
	}
}

// TestContentModerationIdleServiceMemoryIsBounded 测量空闲实例的常驻堆增量。
//
// 旧实现每个实例构造时占用 sizeof(contentModerationTask) * maxContentModerationQueueSize
// （约 28.8 MB），与是否启用异步审核无关。HeapAlloc 是进程级计数，而本包其它测试会留下
// 常驻的审核 worker，因此测量放在只运行本用例的子进程里完成，父进程只传播结果。
func TestContentModerationIdleServiceMemoryIsBounded(t *testing.T) {
	if os.Getenv(contentModerationQueueChildProcessEnv) != "1" {
		runContentModerationQueueTestInChildProcess(t, "TestContentModerationIdleServiceMemoryIsBounded")
		return
	}

	const instances = 64
	// 旧实现约 28.8 MB/实例；64 KiB 给分配器噪声留足余量，同时仍能识别任何预分配。
	const maxIdleBytesPerInstance = 64 * 1024

	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)
	services := make([]*ContentModerationService, 0, instances)
	for range instances {
		services = append(services, NewContentModerationService(nil, nil, nil, nil, nil, nil, nil, nil))
	}
	runtime.GC()
	runtime.ReadMemStats(&after)
	runtime.KeepAlive(services)

	perInstance := int64(0)
	if after.HeapAlloc > before.HeapAlloc {
		perInstance = int64(after.HeapAlloc-before.HeapAlloc) / instances
	}
	t.Logf("idle ContentModerationService retains %d heap bytes/instance over %d instances", perInstance, instances)
	if perInstance > maxIdleBytesPerInstance {
		t.Fatalf("idle ContentModerationService retains %d bytes/instance, want <= %d (a preallocated %d-slot queue is %d bytes)",
			perInstance, maxIdleBytesPerInstance, maxContentModerationQueueSize,
			maxContentModerationQueueSize*int(unsafe.Sizeof(contentModerationTask{})))
	}
}

// runContentModerationQueueTestInChildProcess 在新的测试进程里只运行 name 指定的用例，
// 子进程失败或超时都会让父用例失败。
func runContentModerationQueueTestInChildProcess(t *testing.T, name string) {
	t.Helper()
	executable, err := os.Executable()
	if err != nil {
		t.Fatalf("locate test binary: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, executable, "-test.run=^"+name+"$", "-test.count=1", "-test.v")
	cmd.Env = append(os.Environ(), contentModerationQueueChildProcessEnv+"=1")
	output, runErr := cmd.CombinedOutput()
	if runErr != nil {
		t.Fatalf("isolated %s failed: %v\n%s", name, runErr, output)
	}
	if !strings.Contains(string(output), "--- PASS: "+name) {
		t.Fatalf("isolated child did not run %s\n%s", name, output)
	}
	t.Logf("isolated child output:\n%s", output)
}

// TestContentModerationQueueSizeStillBoundsPendingTasks 钉住丢弃语义：待处理任务数达到
// QueueSize 即丢弃并计数；配置调大后可以继续接收；零值服务（未经构造函数）入队仍被忽略。
func TestContentModerationQueueSizeStillBoundsPendingTasks(t *testing.T) {
	cfg := defaultContentModerationConfig()
	cfg.QueueSize = 8
	svc := NewContentModerationService(nil, nil, nil, nil, nil, nil, nil, nil)
	for i := range 20 {
		svc.enqueueAsync(ContentModerationCheckInput{}, cfg, ContentModerationInput{}, fmt.Sprintf("bounded-%d", i))
	}
	if got := svc.asyncEnqueued.Load(); got != 8 {
		t.Fatalf("asyncEnqueued = %d for QueueSize=8, want 8", got)
	}
	if got := svc.asyncDropped.Load(); got != 12 {
		t.Fatalf("asyncDropped = %d for QueueSize=8, want 12", got)
	}
	if got := len(svc.asyncQueue); got != 8 {
		t.Fatalf("pending tasks = %d, want 8", got)
	}
	svc.enqueueRecord(ContentModerationCheckInput{}, cfg, &ContentModerationLog{RequestID: "record-over-limit"}, "", false, false)
	if got := svc.asyncDropped.Load(); got != 13 {
		t.Fatalf("asyncDropped = %d after a record enqueue on a full queue, want 13", got)
	}

	large := defaultContentModerationConfig()
	large.QueueSize = 4096
	grown := NewContentModerationService(nil, nil, nil, nil, nil, nil, nil, nil)
	for i := range 4096 {
		grown.enqueueAsync(ContentModerationCheckInput{}, large, ContentModerationInput{}, fmt.Sprintf("large-%d", i))
	}
	if got := grown.asyncEnqueued.Load(); got != 4096 {
		t.Fatalf("asyncEnqueued = %d for QueueSize=4096, want 4096", got)
	}
	if got := grown.asyncDropped.Load(); got != 0 {
		t.Fatalf("asyncDropped = %d below QueueSize=4096, want 0", got)
	}

	// 运行期调大 QueueSize 后，已到达旧上限的队列必须能继续接收。
	raised := defaultContentModerationConfig()
	raised.QueueSize = 8192
	for i := range 4096 {
		grown.enqueueAsync(ContentModerationCheckInput{}, raised, ContentModerationInput{}, fmt.Sprintf("raised-%d", i))
	}
	grown.enqueueAsync(ContentModerationCheckInput{}, raised, ContentModerationInput{}, "raised-over-limit")
	if got := grown.asyncEnqueued.Load(); got != 8192 {
		t.Fatalf("asyncEnqueued = %d after raising QueueSize to 8192, want 8192", got)
	}
	if got := grown.asyncDropped.Load(); got != 1 {
		t.Fatalf("asyncDropped = %d after exceeding QueueSize=8192, want 1", got)
	}
	if got := len(grown.asyncQueue); got != 8192 {
		t.Fatalf("pending tasks = %d, want 8192", got)
	}

	zero := &ContentModerationService{}
	zero.enqueueAsync(ContentModerationCheckInput{}, cfg, ContentModerationInput{}, "zero")
	zero.enqueueRecord(ContentModerationCheckInput{}, cfg, &ContentModerationLog{RequestID: "zero"}, "", false, false)
	if zero.asyncEnqueued.Load() != 0 || zero.asyncDropped.Load() != 0 || zero.asyncQueue != nil {
		t.Fatalf("zero-value service must ignore enqueues: enqueued=%d dropped=%d queue=%v",
			zero.asyncEnqueued.Load(), zero.asyncDropped.Load(), zero.asyncQueue != nil)
	}
}

// TestContentModerationWorkerConsumesConcurrentQueue 走构造函数启动的真实 worker：
// 4 个生产者并发写入 400 条记录任务，每条必须恰好落库一次，队列最终清空。
func TestContentModerationWorkerConsumesConcurrentQueue(t *testing.T) {
	cfg := defaultContentModerationConfig()
	cfg.Enabled = true
	cfg.Mode = ContentModerationModePreBlock
	cfg.WorkerCount = 4
	cfg.QueueSize = 4096
	svc, repo := newContentModerationModelFilterTestService(t, cfg)

	const producers = 4
	const perProducer = 100
	const total = producers * perProducer
	var group sync.WaitGroup
	for p := range producers {
		group.Add(1)
		go func(producer int) {
			defer group.Done()
			for i := range perProducer {
				svc.enqueueRecord(
					ContentModerationCheckInput{},
					cfg,
					&ContentModerationLog{
						RequestID: fmt.Sprintf("worker-%d-%d", producer, i),
						Mode:      ContentModerationModePreBlock,
						Action:    ContentModerationActionAllow,
					},
					"",
					false,
					false,
				)
			}
		}(p)
	}
	group.Wait()

	deadline := time.Now().Add(10 * time.Second)
	for svc.asyncProcessed.Load() < total && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}

	if got := svc.asyncEnqueued.Load(); got != total {
		t.Fatalf("asyncEnqueued = %d, want %d", got, total)
	}
	if got := svc.asyncProcessed.Load(); got != total {
		t.Fatalf("asyncProcessed = %d, want %d", got, total)
	}
	logs := repo.snapshotLogs()
	if len(logs) != total {
		t.Fatalf("persisted logs = %d, want %d", len(logs), total)
	}
	seen := make(map[string]struct{}, len(logs))
	for _, log := range logs {
		if _, exists := seen[log.RequestID]; exists {
			t.Fatalf("duplicate persisted request id %q", log.RequestID)
		}
		seen[log.RequestID] = struct{}{}
	}
}
