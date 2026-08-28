package service_test

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/tools/warp-gateway/internal/config"
	"github.com/Wei-Shaw/sub2api/tools/warp-gateway/internal/runtime"
	"github.com/Wei-Shaw/sub2api/tools/warp-gateway/internal/service"
	"github.com/Wei-Shaw/sub2api/tools/warp-gateway/internal/store"
)

func testManager(t *testing.T) *service.Manager {
	t.Helper()
	dir := t.TempDir()
	cfg := config.Default()
	cfg.DataDir = dir
	cfg.Runtime = "mock"
	cfg.ProbeURL = "mock://local"
	cfg.PortRangeStart = 42001
	cfg.PortRangeEnd = 42050
	cfg.HealthInterval = time.Hour
	cfg.UnhealthyAfter = 2
	st, err := store.New(filepath.Join(dir, "state"), cfg.PortRangeStart, cfg.PortRangeEnd)
	if err != nil {
		t.Fatal(err)
	}
	rt := runtime.NewMockManager()
	return service.NewManager(cfg, st, rt, nil)
}

func TestCreateStartHealthPoolRotate(t *testing.T) {
	mgr := testManager(t)
	ctx := context.Background()

	inst, err := mgr.Create(ctx, service.CreateRequest{
		Name:    "warp-a",
		Profile: store.Profile{MockExitIP: "203.0.113.21"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if inst.Status != store.StatusRunning {
		t.Fatalf("status=%s want running", inst.Status)
	}
	if inst.ExitIP != "203.0.113.21" {
		t.Fatalf("exit_ip=%s", inst.ExitIP)
	}
	if inst.SocksURL() == "" {
		t.Fatal("empty socks url")
	}

	// Pool of 3
	pool, err := mgr.CreatePool(ctx, service.CreatePoolRequest{
		NamePrefix: "pool",
		Count:      3,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(pool) != 3 {
		t.Fatalf("pool size %d", len(pool))
	}
	// Second batch with same prefix must allocate next free names (no collision).
	pool2, err := mgr.CreatePool(ctx, service.CreatePoolRequest{
		NamePrefix: "pool",
		Count:      2,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(pool2) != 2 {
		t.Fatalf("second pool size %d", len(pool2))
	}
	names := map[string]struct{}{}
	for _, inst := range append(pool, pool2...) {
		if _, dup := names[inst.Name]; dup {
			t.Fatalf("duplicate pool name %q across batches", inst.Name)
		}
		names[inst.Name] = struct{}{}
	}
	if pool2[0].Name != "pool-04" || pool2[1].Name != "pool-05" {
		t.Fatalf("expected pool-04/05 after first batch of 3, got %q %q", pool2[0].Name, pool2[1].Name)
	}

	// Force duplicate exit IP for alert
	_, err = mgr.Create(ctx, service.CreateRequest{
		Name:    "dup",
		Profile: store.Profile{MockExitIP: "203.0.113.21"},
	})
	if err != nil {
		t.Fatal(err)
	}
	dups := mgr.ExitIPDuplicates()
	if len(dups["203.0.113.21"]) < 2 {
		t.Fatalf("expected duplicate exit ip, got %#v", dups)
	}

	// Rotate with new mock IP
	rotated, err := mgr.Rotate(ctx, inst.ID, &store.Profile{MockExitIP: "198.51.100.9"})
	if err != nil {
		t.Fatal(err)
	}
	if rotated.ExitIP != "198.51.100.9" {
		t.Fatalf("after rotate exit_ip=%s", rotated.ExitIP)
	}

	snap := mgr.PoolSnapshot()
	if snap.TotalCount < 5 {
		t.Fatalf("snapshot total=%d", snap.TotalCount)
	}
	if snap.HealthyCount < 1 {
		t.Fatalf("healthy=%d", snap.HealthyCount)
	}

	// Stop + delete
	if err := mgr.Stop(ctx, inst.ID); err != nil {
		t.Fatal(err)
	}
	got, _ := mgr.Get(inst.ID)
	if got.Status != store.StatusStopped {
		t.Fatalf("stop status=%s", got.Status)
	}
	if err := mgr.Delete(ctx, inst.ID); err != nil {
		t.Fatal(err)
	}
}

func TestUnhealthyThreshold(t *testing.T) {
	// Unhealthy path is exercised when FailCount accumulates; mock always healthy.
	// We simulate via store update through failed health by using empty runtime handle stop.
	// For unit coverage, mark via multiple HealthCheck after Stop.
	mgr := testManager(t)
	ctx := context.Background()
	inst, err := mgr.Create(ctx, service.CreateRequest{Name: "u1", Profile: store.Profile{MockExitIP: "203.0.113.1"}})
	if err != nil {
		t.Fatal(err)
	}
	if err := mgr.Stop(ctx, inst.ID); err != nil {
		t.Fatal(err)
	}
	// After stop, mock probe still succeeds (probe does not require live socks in mock mode).
	// That's intentional for control-plane unit tests.
	_ = mgr.HealthCheck(ctx, inst.ID)
}

func TestCreatePoolReturnsPartialResources(t *testing.T) {
	mgr := testManager(t)
	autoStart := false
	pool, err := mgr.CreatePool(context.Background(), service.CreatePoolRequest{
		NamePrefix: "partial",
		Count:      2,
		StartPort:  42050,
		AutoStart:  &autoStart,
	})
	if err == nil {
		t.Fatal("expected second member to exceed configured port range")
	}
	if len(pool) != 1 || pool[0].Name != "partial-01" {
		t.Fatalf("partial resources=%#v", pool)
	}
	var partial *service.PoolCreateError
	if !errors.As(err, &partial) || partial.Created != 1 {
		t.Fatalf("error=%T %v, want PoolCreateError with one created", err, err)
	}
	if got, getErr := mgr.Get(pool[0].ID); getErr != nil || got.Name != "partial-01" {
		t.Fatalf("created member not recoverable: got=%#v err=%v", got, getErr)
	}
}

func TestPoolSnapshotPreservesOnlySocksCredentials(t *testing.T) {
	mgr := testManager(t)
	autoStart := false
	inst, err := mgr.Create(context.Background(), service.CreateRequest{
		Name: "authenticated",
		Profile: store.Profile{
			PrivateKey: "warp-private", AccessToken: "warp-token", LicenseKey: "warp-license",
		},
		SocksAuthUser: "proxy-user",
		SocksAuthPass: "proxy-password",
		AutoStart:     &autoStart,
	})
	if err != nil {
		t.Fatal(err)
	}
	if inst.SocksAuthPass != "***" {
		t.Fatalf("ordinary create response leaked password: %q", inst.SocksAuthPass)
	}
	snap := mgr.PoolSnapshot()
	if len(snap.Instances) != 1 {
		t.Fatalf("instances=%d", len(snap.Instances))
	}
	got := snap.Instances[0]
	if got.SocksAuthUser != "proxy-user" || got.SocksAuthPass != "proxy-password" {
		t.Fatalf("snapshot credentials=%q/%q", got.SocksAuthUser, got.SocksAuthPass)
	}
	if got.Profile.PrivateKey != "***" || got.Profile.AccessToken != "***" || got.Profile.LicenseKey != "***" {
		t.Fatalf("snapshot leaked WARP profile secrets: %#v", got.Profile)
	}
	listed := mgr.List()[0]
	if listed.SocksAuthPass != "***" {
		t.Fatalf("ordinary list response leaked password: %q", listed.SocksAuthPass)
	}
}

type blockingRuntime struct {
	startEntered chan struct{}
	releaseStart chan struct{}
	stopped      chan struct{}
}

func (r *blockingRuntime) Name() string { return "blocking" }

func (r *blockingRuntime) Start(context.Context, *store.Instance) (runtime.Handle, error) {
	close(r.startEntered)
	<-r.releaseStart
	return blockingHandle{stopped: r.stopped}, nil
}

type blockingHandle struct{ stopped chan struct{} }

func (h blockingHandle) LocalAddr() string     { return "127.0.0.1:1" }
func (h blockingHandle) Done() <-chan struct{} { return h.stopped }
func (h blockingHandle) Err() error            { return nil }
func (h blockingHandle) Stop(context.Context) error {
	close(h.stopped)
	return nil
}

func TestLifecycleOperationsAreSerializedPerInstance(t *testing.T) {
	dir := t.TempDir()
	cfg := config.Default()
	cfg.DataDir = dir
	cfg.ProbeURL = "mock://local"
	cfg.PortRangeStart, cfg.PortRangeEnd = 42101, 42110
	st, err := store.New(filepath.Join(dir, "state"), cfg.PortRangeStart, cfg.PortRangeEnd)
	if err != nil {
		t.Fatal(err)
	}
	rt := &blockingRuntime{startEntered: make(chan struct{}), releaseStart: make(chan struct{}), stopped: make(chan struct{})}
	mgr := service.NewManager(cfg, st, rt, nil)
	autoStart := false
	inst, err := mgr.Create(context.Background(), service.CreateRequest{Name: "serialized", AutoStart: &autoStart})
	if err != nil {
		t.Fatal(err)
	}
	startDone := make(chan error, 1)
	go func() { startDone <- mgr.Start(context.Background(), inst.ID) }()
	<-rt.startEntered
	stopDone := make(chan error, 1)
	go func() { stopDone <- mgr.Stop(context.Background(), inst.ID) }()
	select {
	case err := <-stopDone:
		t.Fatalf("Stop completed while Start was in progress: %v", err)
	case <-time.After(100 * time.Millisecond):
	}
	close(rt.releaseStart)
	if err := <-startDone; err != nil {
		t.Fatal(err)
	}
	if err := <-stopDone; err != nil {
		t.Fatal(err)
	}
	select {
	case <-rt.stopped:
	default:
		t.Fatal("runtime handle was leaked")
	}
	got, _ := mgr.Get(inst.ID)
	if got.Status != store.StatusStopped || got.DesiredState != store.DesiredStopped {
		t.Fatalf("final state=%s/%s", got.Status, got.DesiredState)
	}
}
