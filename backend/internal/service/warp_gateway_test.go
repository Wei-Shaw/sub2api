package service

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
