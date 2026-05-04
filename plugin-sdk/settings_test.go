package pluginsdk

import (
	"context"
	"encoding/json"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	pb "github.com/Wei-Shaw/sub2api/plugin-sdk/proto/pluginsdk"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// TestSettingsClient_StoreIfNewer_KeepsHigherRevision 是 T14 Fix 5 的回归测试。
//
// 场景：Get RPC 与 watch 的 applyEvent 同时往 cache 写入。Get 拿到的是更早
// (revision=5) 的快照，而 watch 紧接着用 revision=10 的事件抢先写入 cache；
// Get 完成 RPC 后试图用旧 revision 覆盖，必须被 storeIfNewer 拒绝，最终缓存
// 留下 revision=10 的最新版本。
func TestSettingsClient_StoreIfNewer_KeepsHigherRevision(t *testing.T) {
	c := &settingsClient{}

	// 模拟 watch 流先到，把 revision=10 写入。
	c.storeIfNewer("k", &cachedSetting{
		value:     json.RawMessage(`{"v":10}`),
		revision:  10,
		exists:    true,
		fetchedAt: time.Now(),
	})

	// Get RPC 后到达，携带 revision=5 的旧值——必须被拒绝。
	c.storeIfNewer("k", &cachedSetting{
		value:     json.RawMessage(`{"v":5}`),
		revision:  5,
		exists:    true,
		fetchedAt: time.Now(),
	})

	v, ok := c.cache.Load("k")
	if !ok {
		t.Fatalf("expected cache to contain key %q", "k")
	}
	got := v.(*cachedSetting)
	if got.revision != 10 {
		t.Fatalf("expected revision=10 to win, got revision=%d (value=%s)", got.revision, string(got.value))
	}
	if string(got.value) != `{"v":10}` {
		t.Fatalf("expected newer value to be retained, got %s", string(got.value))
	}
}

// TestSettingsClient_StoreIfNewer_AcceptsHigherRevision 验证当新写入的
// revision 严格更大时正常覆盖旧条目。
func TestSettingsClient_StoreIfNewer_AcceptsHigherRevision(t *testing.T) {
	c := &settingsClient{}

	c.storeIfNewer("k", &cachedSetting{
		value:    json.RawMessage(`{"v":1}`),
		revision: 1,
		exists:   true,
	})
	c.storeIfNewer("k", &cachedSetting{
		value:    json.RawMessage(`{"v":2}`),
		revision: 2,
		exists:   true,
	})

	v, _ := c.cache.Load("k")
	got := v.(*cachedSetting)
	if got.revision != 2 {
		t.Fatalf("expected revision=2 after monotonic update, got revision=%d", got.revision)
	}
}

// TestSettingsClient_StoreIfNewer_ConcurrentRace 模拟并发 Get / applyEvent
// 写入：100 次 watch 推送 (revision 单调递增 1..100) 与 100 次 Get 写入
// (revision 都是 50，故意制造"已过期"的写入) 并发跑；最终 cache 必须收敛
// 到 revision=100，绝不可能停在 50。
func TestSettingsClient_StoreIfNewer_ConcurrentRace(t *testing.T) {
	c := &settingsClient{}

	var wg sync.WaitGroup
	wg.Add(2)

	// watch 路径：递增 revision 写入。
	go func() {
		defer wg.Done()
		for i := int64(1); i <= 100; i++ {
			c.storeIfNewer("k", &cachedSetting{
				value:    json.RawMessage(`"watch"`),
				revision: i,
				exists:   true,
			})
		}
	}()

	// Get 路径：100 次都用 revision=50 (模拟一个滞后的 RPC 响应)。
	go func() {
		defer wg.Done()
		for i := 0; i < 100; i++ {
			c.storeIfNewer("k", &cachedSetting{
				value:    json.RawMessage(`"get"`),
				revision: 50,
				exists:   true,
			})
		}
	}()

	wg.Wait()
	v, _ := c.cache.Load("k")
	got := v.(*cachedSetting)
	if got.revision != 100 {
		t.Fatalf("expected concurrent winner revision=100, got revision=%d (value=%s)",
			got.revision, string(got.value))
	}
}

// stubSettingsExtClient is a tiny mock of pb.SettingsExtensionClient that
// returns the configured error from every Watch call. It is sufficient
// for verifying the LoopWithRetryClass classification behaviour because
// the watch stream never has to actually open.
type stubSettingsExtClient struct {
	watchErr   error
	watchCalls atomic.Int64
}

func (s *stubSettingsExtClient) Get(ctx context.Context, _ *pb.SettingsGetRequest, _ ...grpc.CallOption) (*pb.SettingsGetResponse, error) {
	return &pb.SettingsGetResponse{Exists: false}, nil
}

func (s *stubSettingsExtClient) Set(ctx context.Context, _ *pb.SettingsSetRequest, _ ...grpc.CallOption) (*pb.SettingsSetResponse, error) {
	return &pb.SettingsSetResponse{Revision: 1}, nil
}

func (s *stubSettingsExtClient) Watch(ctx context.Context, _ *pb.SettingsWatchRequest, _ ...grpc.CallOption) (grpc.ServerStreamingClient[pb.SettingsChangeEvent], error) {
	s.watchCalls.Add(1)
	return nil, s.watchErr
}

// TestRunWatchLoop_PermissionDeniedExitsFast is the T31 regression for
// settings.go: the host's SettingsExtension rejects callers that did not
// declare the `settings.own.read` capability with PermissionDenied; the
// watch loop must classify it fatal and exit instead of reconnecting
// forever. Observable signal: c.watchDone closes promptly.
func TestRunWatchLoop_PermissionDeniedExitsFast(t *testing.T) {
	stub := &stubSettingsExtClient{
		watchErr: status.Error(codes.PermissionDenied, "missing settings.own.read"),
	}
	c := newSettingsClient(stub, "p1", nil)
	c.watchDone = make(chan struct{})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	go func() {
		c.runWatchLoop(ctx)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("runWatchLoop did not exit on PermissionDenied")
	}
	if got := stub.watchCalls.Load(); got != 1 {
		t.Fatalf("Watch called %d times, want exactly 1 (no retry on fatal)", got)
	}
	// watchDone must also be closed (run loop signal for stopWatchLoop).
	select {
	case <-c.watchDone:
	default:
		t.Fatal("watchDone not closed after fatal exit")
	}
}
