package service

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/tools/warp-gateway/internal/config"
	"github.com/Wei-Shaw/sub2api/tools/warp-gateway/internal/register"
	"github.com/Wei-Shaw/sub2api/tools/warp-gateway/internal/runtime"
	"github.com/Wei-Shaw/sub2api/tools/warp-gateway/internal/store"
)

type stopErrorRuntime struct{ handle *stopErrorHandle }

func (r *stopErrorRuntime) Name() string { return "stop-error" }
func (r *stopErrorRuntime) Start(context.Context, *store.Instance) (runtime.Handle, error) {
	return r.handle, nil
}

type observableRuntime struct {
	starts atomic.Int32
	mu     sync.Mutex
	latest *observableHandle
}

func (r *observableRuntime) Name() string { return "mock" }
func (r *observableRuntime) Start(ctx context.Context, _ *store.Instance) (runtime.Handle, error) {
	h := &observableHandle{runCtx: ctx, done: make(chan struct{})}
	r.mu.Lock()
	r.latest = h
	r.mu.Unlock()
	r.starts.Add(1)
	return h, nil
}

func (r *observableRuntime) handle() *observableHandle {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.latest
}

type observableHandle struct {
	runCtx        context.Context
	done          chan struct{}
	once          sync.Once
	errMu         sync.Mutex
	err           error
	stopSawActive atomic.Bool
}

func (h *observableHandle) LocalAddr() string     { return "127.0.0.1:1" }
func (h *observableHandle) Done() <-chan struct{} { return h.done }
func (h *observableHandle) Err() error {
	h.errMu.Lock()
	defer h.errMu.Unlock()
	return h.err
}
func (h *observableHandle) Stop(context.Context) error {
	select {
	case <-h.runCtx.Done():
	default:
		h.stopSawActive.Store(true)
	}
	h.once.Do(func() { close(h.done) })
	return nil
}
func (h *observableHandle) crash(err error) {
	h.errMu.Lock()
	h.err = err
	h.errMu.Unlock()
	h.once.Do(func() { close(h.done) })
}

func newInternalManager(t *testing.T, rt runtime.Manager) *Manager {
	t.Helper()
	dir := t.TempDir()
	cfg := config.Default()
	cfg.DataDir = dir
	cfg.ProbeURL = "mock://local"
	cfg.PortRangeStart, cfg.PortRangeEnd = 42501, 42510
	st, err := store.New(filepath.Join(dir, "state"), cfg.PortRangeStart, cfg.PortRangeEnd)
	if err != nil {
		t.Fatal(err)
	}
	return NewManager(cfg, st, rt, nil)
}

func TestShutdownPreservesDesiredRunningAndReconcileRestarts(t *testing.T) {
	rt := &observableRuntime{}
	mgr := newInternalManager(t, rt)
	inst, err := mgr.Create(context.Background(), CreateRequest{Name: "survives-shutdown"})
	if err != nil {
		t.Fatal(err)
	}
	mgr.Shutdown(context.Background())
	stopped, err := mgr.store.Get(inst.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stopped.DesiredState != store.DesiredRunning || stopped.Status != store.StatusStopped {
		t.Fatalf("shutdown state=%s/%s", stopped.Status, stopped.DesiredState)
	}

	restartedRuntime := &observableRuntime{}
	restarted := NewManager(mgr.cfg, mgr.store, restartedRuntime, nil)
	restarted.Reconcile(context.Background())
	got, err := restarted.store.Get(inst.ID)
	if err != nil || got.Status != store.StatusRunning || restartedRuntime.starts.Load() != 1 {
		t.Fatalf("reconcile did not restore instance: state=%#v starts=%d err=%v", got, restartedRuntime.starts.Load(), err)
	}
	restarted.Shutdown(context.Background())
}

func TestUnexpectedRuntimeExitIsRemovedAndRestarted(t *testing.T) {
	rt := &observableRuntime{}
	mgr := newInternalManager(t, rt)
	inst, err := mgr.Create(context.Background(), CreateRequest{Name: "restart-on-exit"})
	if err != nil {
		t.Fatal(err)
	}
	first := rt.handle()
	first.crash(errors.New("boom"))
	deadline := time.Now().Add(3 * time.Second)
	for rt.starts.Load() < 2 && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if rt.starts.Load() < 2 {
		t.Fatalf("runtime was not restarted; starts=%d", rt.starts.Load())
	}
	mgr.mu.Lock()
	current := mgr.handles[inst.ID]
	mgr.mu.Unlock()
	if current == nil || current == first {
		t.Fatalf("stale handle retained after restart: %#v", current)
	}
	mgr.Shutdown(context.Background())
}

func TestStopGracefullyStopsBeforeCancellingRunContext(t *testing.T) {
	rt := &observableRuntime{}
	mgr := newInternalManager(t, rt)
	inst, err := mgr.Create(context.Background(), CreateRequest{Name: "graceful-stop"})
	if err != nil {
		t.Fatal(err)
	}
	h := rt.handle()
	if err := mgr.Stop(context.Background(), inst.ID); err != nil {
		t.Fatal(err)
	}
	if !h.stopSawActive.Load() {
		t.Fatal("runtime context was cancelled before Handle.Stop")
	}
}

func TestHealthCheckUsesInstanceLifecycleLock(t *testing.T) {
	mgr := newInternalManager(t, &observableRuntime{})
	inst, err := mgr.Create(context.Background(), CreateRequest{Name: "health-lock"})
	if err != nil {
		t.Fatal(err)
	}
	unlock := mgr.lockInstance(inst.ID)
	done := make(chan error, 1)
	go func() { done <- mgr.HealthCheck(context.Background(), inst.ID) }()
	select {
	case err := <-done:
		t.Fatalf("HealthCheck bypassed lifecycle lock: %v", err)
	case <-time.After(100 * time.Millisecond):
	}
	unlock()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	mgr.Shutdown(context.Background())
}

type stopErrorHandle struct{ err error }

func (h *stopErrorHandle) LocalAddr() string          { return "127.0.0.1:1" }
func (h *stopErrorHandle) Stop(context.Context) error { return h.err }
func (h *stopErrorHandle) Done() <-chan struct{}      { return make(chan struct{}) }
func (h *stopErrorHandle) Err() error                 { return nil }

func TestStopFailureKeepsHandleAndDoesNotReportStopped(t *testing.T) {
	dir := t.TempDir()
	cfg := config.Default()
	cfg.DataDir = dir
	cfg.ProbeURL = "mock://local"
	cfg.PortRangeStart, cfg.PortRangeEnd = 42401, 42410
	st, err := store.New(filepath.Join(dir, "state"), cfg.PortRangeStart, cfg.PortRangeEnd)
	if err != nil {
		t.Fatal(err)
	}
	h := &stopErrorHandle{err: errors.New("stop failed")}
	mgr := NewManager(cfg, st, &stopErrorRuntime{handle: h}, nil)
	inst, err := mgr.Create(context.Background(), CreateRequest{Name: "stop-failure"})
	if err != nil {
		t.Fatal(err)
	}
	if err := mgr.Stop(context.Background(), inst.ID); !errors.Is(err, h.err) {
		t.Fatalf("Stop error=%v, want %v", err, h.err)
	}
	if mgr.handles[inst.ID] != h {
		t.Fatal("failed runtime handle was discarded")
	}
	got, err := mgr.store.Get(inst.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status == store.StatusStopped || got.LastError == "" {
		t.Fatalf("failed stop persisted as %s with error %q", got.Status, got.LastError)
	}
}

func TestCreatePoolRetainsPartialRegistrationsAsInstances(t *testing.T) {
	dir := t.TempDir()
	cfg := config.Default()
	cfg.DataDir = dir
	cfg.Runtime = "sing-box"
	cfg.PortRangeStart, cfg.PortRangeEnd = 42301, 42310
	st, err := store.New(filepath.Join(dir, "state"), cfg.PortRangeStart, cfg.PortRangeEnd)
	if err != nil {
		t.Fatal(err)
	}
	mgr := NewManager(cfg, st, runtime.NewSingBoxManager("unused", dir), nil)
	mgr.registerMany = func(context.Context, int) ([]register.Result, error) {
		return []register.Result{{Profile: store.Profile{
			PrivateKey: "private", DeviceID: "registered-device", AccessToken: "token",
			Peers: []store.PeerConfig{{PublicKey: "peer", Endpoint: "example.test:2408"}},
		}}}, errors.New("registration interrupted")
	}
	autoStart := false
	created, err := mgr.CreatePool(context.Background(), CreatePoolRequest{
		NamePrefix: "registered", Count: 2, Register: true, AutoStart: &autoStart,
	})
	if err == nil {
		t.Fatal("expected partial registration error")
	}
	var partial *PoolCreateError
	if !errors.As(err, &partial) || partial.Registered != 1 || partial.Created != 1 {
		t.Fatalf("partial error=%#v", partial)
	}
	if len(created) != 1 {
		t.Fatalf("created=%d", len(created))
	}
	retained, getErr := mgr.GetRaw(created[0].ID)
	if getErr != nil || retained.Profile.DeviceID != "registered-device" || retained.Profile.AccessToken != "token" {
		t.Fatalf("registered resource was not retained: instance=%#v err=%v", retained, getErr)
	}
}
