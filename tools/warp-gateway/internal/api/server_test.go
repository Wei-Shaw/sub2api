package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/tools/warp-gateway/internal/api"
	"github.com/Wei-Shaw/sub2api/tools/warp-gateway/internal/config"
	"github.com/Wei-Shaw/sub2api/tools/warp-gateway/internal/runtime"
	"github.com/Wei-Shaw/sub2api/tools/warp-gateway/internal/service"
	"github.com/Wei-Shaw/sub2api/tools/warp-gateway/internal/store"
)

var apiPortBase = atomic.Uint64{}

func setupAPI(t *testing.T) (http.Handler, *service.Manager) {
	t.Helper()
	dir := t.TempDir()
	base := 44000 + int(apiPortBase.Add(50))
	cfg := config.Default()
	cfg.DataDir = dir
	cfg.Runtime = "mock"
	cfg.ProbeURL = "mock://local"
	cfg.PortRangeStart = base
	cfg.PortRangeEnd = base + 40
	cfg.HealthInterval = time.Hour
	st, err := store.New(filepath.Join(dir, "state"), cfg.PortRangeStart, cfg.PortRangeEnd)
	if err != nil {
		t.Fatal(err)
	}
	mgr := service.NewManager(cfg, st, runtime.NewMockManager(), nil)
	t.Cleanup(func() {
		mgr.Shutdown(context.Background())
	})
	return api.NewServer(mgr, "test-token").Handler(), mgr
}

func TestAPICreatePoolAndSnapshot(t *testing.T) {
	h, _ := setupAPI(t)

	// unauthorized
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/instances", nil)
	h.ServeHTTP(rr, req)
	if rr.Code != 401 {
		t.Fatalf("want 401 got %d", rr.Code)
	}

	body := []byte(`{"name_prefix":"wp","count":2}`)
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/pools", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer test-token")
	req.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(rr, req)
	if rr.Code != 201 {
		t.Fatalf("create pool status=%d body=%s", rr.Code, rr.Body.String())
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/v1/pools/snapshot", nil)
	req.Header.Set("Authorization", "Bearer test-token")
	h.ServeHTTP(rr, req)
	if rr.Code != 200 {
		t.Fatalf("snapshot %d", rr.Code)
	}
	var snap service.PoolSnapshot
	if err := json.Unmarshal(rr.Body.Bytes(), &snap); err != nil {
		t.Fatal(err)
	}
	if snap.TotalCount != 2 {
		t.Fatalf("total=%d", snap.TotalCount)
	}
	if len(snap.SocksURLs) != 2 {
		t.Fatalf("socks urls=%v", snap.SocksURLs)
	}

	// healthz no auth? currently auth wraps all - document that healthz also needs token when set
	// For k8s we may want healthz open - leave as is for now.

	_ = context.Background()
}

func TestAPIRotateAndDelete(t *testing.T) {
	h, _ := setupAPI(t)
	auth := func(r *http.Request) {
		r.Header.Set("Authorization", "Bearer test-token")
	}

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/instances", bytes.NewReader([]byte(
		`{"name":"one","profile":{"mock_exit_ip":"203.0.113.50"}}`,
	)))
	auth(req)
	req.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(rr, req)
	if rr.Code != 201 {
		t.Fatalf("create %d %s", rr.Code, rr.Body.String())
	}
	var inst store.Instance
	_ = json.Unmarshal(rr.Body.Bytes(), &inst)

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/instances/"+inst.ID+"/rotate", bytes.NewReader([]byte(
		`{"profile":{"mock_exit_ip":"198.51.100.7"}}`,
	)))
	auth(req)
	req.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(rr, req)
	if rr.Code != 200 {
		t.Fatalf("rotate %d %s", rr.Code, rr.Body.String())
	}
	_ = json.Unmarshal(rr.Body.Bytes(), &inst)
	if inst.ExitIP != "198.51.100.7" {
		t.Fatalf("exit_ip=%s", inst.ExitIP)
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodDelete, "/v1/instances/"+inst.ID, nil)
	auth(req)
	h.ServeHTTP(rr, req)
	if rr.Code != 200 {
		t.Fatalf("delete %d", rr.Code)
	}
}
