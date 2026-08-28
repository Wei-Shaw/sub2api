package service

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestBuildAttachPlan_Phase3(t *testing.T) {
	snap := &WarpPoolSnapshot{
		Instances: []WarpInstance{
			{ID: "a", Name: "01", ListenHost: "127.0.0.1", ListenPort: 41001, Status: "running", ExitIP: "1.1.1.1"},
			{ID: "b", Name: "02", ListenHost: "127.0.0.1", ListenPort: 41002, Status: "unhealthy", ExitIP: "1.1.1.1"},
		},
		UnhealthyIDs: []string{"b"},
		DuplicateIPs: map[string][]string{"1.1.1.1": {"a", "b"}},
		HealthyCount: 1,
		TotalCount:   2,
	}
	plan := BuildAttachPlan(snap, "warp-pool")
	if plan.SuggestedGroupName != "warp-pool" {
		t.Fatal(plan.SuggestedGroupName)
	}
	if len(plan.ProxySpecs) != 2 {
		t.Fatalf("specs=%d", len(plan.ProxySpecs))
	}
	if plan.ProxySpecs[0].Protocol != "socks5h" {
		t.Fatal(plan.ProxySpecs[0].Protocol)
	}
	if len(plan.DetachProxyNames) == 0 {
		t.Fatal("expected detach for unhealthy")
	}
	if len(plan.DuplicateExitIPs["1.1.1.1"]) != 2 {
		t.Fatalf("dups=%v", plan.DuplicateExitIPs)
	}
}

func TestBuildAttachPlan_OnlyRunningInstancesAreActive(t *testing.T) {
	statuses := []string{"starting", "registered", "stopped", "error", "unhealthy", ""}
	instances := make([]WarpInstance, 0, len(statuses)+1)
	instances = append(instances, WarpInstance{ID: "running", Name: "running", ListenPort: 41000, Status: "running"})
	for i, status := range statuses {
		instances = append(instances, WarpInstance{ID: status, Name: status, ListenPort: 41001 + i, Status: status})
	}

	plan := BuildAttachPlan(&WarpPoolSnapshot{Instances: instances}, "warp-pool")
	if plan.ProxySpecs[0].Status != StatusActive {
		t.Fatalf("running instance status=%q", plan.ProxySpecs[0].Status)
	}
	for _, spec := range plan.ProxySpecs[1:] {
		if spec.Status != StatusError {
			t.Fatalf("non-running instance %q status=%q", spec.WarpID, spec.Status)
		}
	}
	if len(plan.DetachProxyNames) != len(statuses) {
		t.Fatalf("detach=%v", plan.DetachProxyNames)
	}
}

func TestWarpGatewayClientRejectsInsecureRemoteAndInvalidTLS(t *testing.T) {
	t.Run("remote HTTP", func(t *testing.T) {
		client := NewWarpGatewayClient(WarpGatewayConfig{Enabled: true, BaseURL: "http://warp.internal:8080", Token: "secret"})
		_, err := client.ListInstances(context.Background())
		if err == nil || !strings.Contains(err.Error(), "requires HTTPS") {
			t.Fatalf("err=%v", err)
		}
	})

	t.Run("missing client key", func(t *testing.T) {
		client := NewWarpGatewayClient(WarpGatewayConfig{Enabled: true, BaseURL: "https://warp.internal", TLSCertFile: "client.pem"})
		_, err := client.ListInstances(context.Background())
		if err == nil || !strings.Contains(err.Error(), "configured together") {
			t.Fatalf("err=%v", err)
		}
	})

	t.Run("invalid CA", func(t *testing.T) {
		dir := t.TempDir()
		caFile := filepath.Join(dir, "ca.pem")
		if err := os.WriteFile(caFile, []byte("not a certificate"), 0o600); err != nil {
			t.Fatal(err)
		}
		client := NewWarpGatewayClient(WarpGatewayConfig{Enabled: true, BaseURL: "https://warp.internal", TLSCAFile: caFile})
		_, err := client.ListInstances(context.Background())
		if err == nil || !strings.Contains(err.Error(), "no certificates found") {
			t.Fatalf("err=%v", err)
		}
	})
}

func TestWarpGatewayClientMutationDoesNotFollowRedirect(t *testing.T) {
	targetCalled := false
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		targetCalled = true
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(target.Close)
	redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL, http.StatusTemporaryRedirect)
	}))
	t.Cleanup(redirect.Close)

	client := NewWarpGatewayClient(WarpGatewayConfig{
		Enabled: true,
		BaseURL: redirect.URL,
		Token:   "secret",
		Timeout: time.Second,
	})
	_, err := client.CreatePoolEx(context.Background(), "warp", 1, true)
	if err == nil || !strings.Contains(err.Error(), "HTTP 307") {
		t.Fatalf("err=%v", err)
	}
	if targetCalled {
		t.Fatal("mutation request followed redirect to another endpoint")
	}
}

func TestBuildAttachPlan_PreservesSocksCredentials(t *testing.T) {
	plan := BuildAttachPlan(&WarpPoolSnapshot{Instances: []WarpInstance{{
		ID: "auth-1", Name: "auth", ListenHost: "127.0.0.1", ListenPort: 41009,
		Status: "running", SocksAuthUser: "user", SocksAuthPass: "secret",
	}}}, "warp-pool")
	if len(plan.ProxySpecs) != 1 {
		t.Fatalf("specs=%d", len(plan.ProxySpecs))
	}
	if plan.ProxySpecs[0].Username != "user" || plan.ProxySpecs[0].Password != "secret" {
		t.Fatalf("credentials were dropped: %#v", plan.ProxySpecs[0])
	}
}

func TestWarpGatewayClient_CreatePoolReturnsPartialCreated(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"error":   "member 2 failed",
			"created": []WarpInstance{{ID: "partial-1", Name: "warp-01", ListenPort: 41001}},
		})
	}))
	defer srv.Close()

	c := NewWarpGatewayClient(WarpGatewayConfig{Enabled: true, BaseURL: srv.URL, Timeout: time.Second})
	created, err := c.CreatePoolEx(context.Background(), "warp", 2, false)
	if err == nil {
		t.Fatal("expected partial creation error")
	}
	if len(created) != 1 || created[0].ID != "partial-1" {
		t.Fatalf("partial result lost: %#v", created)
	}
}

func TestBuildAttachPlan_DisambiguatesDuplicateInstanceNames(t *testing.T) {
	snap := &WarpPoolSnapshot{
		Instances: []WarpInstance{
			{ID: "a1", Name: "warp-01", ListenHost: "127.0.0.1", ListenPort: 20001, Status: "running"},
			{ID: "a2", Name: "warp-01", ListenHost: "127.0.0.1", ListenPort: 20002, Status: "running"},
		},
		TotalCount:   2,
		HealthyCount: 2,
	}
	plan := BuildAttachPlan(snap, "warp-pool")
	if len(plan.ProxySpecs) != 2 {
		t.Fatalf("specs=%d", len(plan.ProxySpecs))
	}
	if plan.ProxySpecs[0].Name == plan.ProxySpecs[1].Name {
		t.Fatalf("expected unique proxy names, both %q", plan.ProxySpecs[0].Name)
	}
	if plan.ProxySpecs[1].Name != "warp-warp-01-20002" {
		t.Fatalf("second name=%q", plan.ProxySpecs[1].Name)
	}
}

func TestWarpGatewayClient_ListAndSnapshot(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/instances", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"instances": []WarpInstance{{ID: "x", Name: "n", ListenPort: 41001, Status: "running"}},
		})
	})
	mux.HandleFunc("/v1/pools/snapshot", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(WarpPoolSnapshot{TotalCount: 1, HealthyCount: 1, SocksURLs: []string{"socks5h://127.0.0.1:41001"}})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	c := NewWarpGatewayClient(WarpGatewayConfig{
		Enabled: true,
		BaseURL: srv.URL,
		Timeout: 2 * time.Second,
	})
	list, err := c.ListInstances(context.Background())
	if err != nil || len(list) != 1 {
		t.Fatalf("list err=%v len=%d", err, len(list))
	}
	snap, err := c.PoolSnapshot(context.Background())
	if err != nil || snap.TotalCount != 1 {
		t.Fatalf("snap err=%v %#v", err, snap)
	}
	if list[0].SocksURL() != "socks5h://127.0.0.1:41001" {
		t.Fatal(list[0].SocksURL())
	}
}
