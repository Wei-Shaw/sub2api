package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
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
	h, mgr := setupAPI(t)
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
	req = httptest.NewRequest(http.MethodPost, "/v1/instances/"+inst.ID+"/rotate", nil)
	auth(req)
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("empty rotate status=%d body=%s", rr.Code, rr.Body.String())
	}
	unchanged, err := mgr.GetRaw(inst.ID)
	if err != nil || unchanged.Profile.MockExitIP != "198.51.100.7" {
		t.Fatalf("empty rotate changed profile: instance=%#v err=%v", unchanged, err)
	}

	for _, tc := range []struct {
		name string
		body string
	}{
		{name: "malformed", body: `{"profile":`},
		{name: "trailing JSON", body: `{}` + `{}`},
	} {
		t.Run("rotate rejects "+tc.name, func(t *testing.T) {
			rr := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/v1/instances/"+inst.ID+"/rotate", strings.NewReader(tc.body))
			auth(req)
			h.ServeHTTP(rr, req)
			if rr.Code != http.StatusBadRequest {
				t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
			}
			got, err := mgr.GetRaw(inst.ID)
			if err != nil || got.Profile.MockExitIP != "198.51.100.7" {
				t.Fatalf("manager was invoked: instance=%#v err=%v", got, err)
			}
		})
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodDelete, "/v1/instances/"+inst.ID, strings.NewReader(`{} garbage`))
	auth(req)
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("malformed delete status=%d body=%s", rr.Code, rr.Body.String())
	}
	if _, err := mgr.GetRaw(inst.ID); err != nil {
		t.Fatalf("malformed delete invoked manager: %v", err)
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodDelete, "/v1/instances/"+inst.ID, nil)
	auth(req)
	h.ServeHTTP(rr, req)
	if rr.Code != 200 {
		t.Fatalf("delete %d", rr.Code)
	}
}

func TestPoolSnapshotReturnsSocksCredentialsButListDoesNot(t *testing.T) {
	h, _ := setupAPI(t)
	auth := func(r *http.Request) { r.Header.Set("Authorization", "Bearer test-token") }
	body := []byte(`{"name":"credentials","auto_start":false,"socks_auth_user":"user","socks_auth_pass":"pass","profile":{"private_key":"private"}}`)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/instances", bytes.NewReader(body))
	auth(req)
	req.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("create status=%d body=%s", rr.Code, rr.Body.String())
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/v1/pools/snapshot", nil)
	auth(req)
	h.ServeHTTP(rr, req)
	var snapshot service.PoolSnapshot
	if err := json.Unmarshal(rr.Body.Bytes(), &snapshot); err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Instances) != 1 || snapshot.Instances[0].SocksAuthUser != "user" || snapshot.Instances[0].SocksAuthPass != "pass" {
		t.Fatalf("snapshot did not preserve SOCKS credentials: %#v", snapshot.Instances)
	}
	if snapshot.Instances[0].Profile.PrivateKey != "***" {
		t.Fatalf("snapshot leaked WARP private key: %#v", snapshot.Instances[0].Profile)
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/v1/instances", nil)
	auth(req)
	h.ServeHTTP(rr, req)
	var listed struct {
		Instances []store.Instance `json:"instances"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &listed); err != nil {
		t.Fatal(err)
	}
	if len(listed.Instances) != 1 || listed.Instances[0].SocksAuthPass != "***" {
		t.Fatalf("ordinary list exposed SOCKS password: %#v", listed.Instances)
	}
}
