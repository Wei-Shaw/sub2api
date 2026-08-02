package service

import (
	"context"
	"strings"
	"testing"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

// W1: free admin CRUD must not invent or mutate warp-* inventory outside WARP APIs.

func TestAdminService_CreateProxy_RejectsWarpManagedName(t *testing.T) {
	repo := newMemProxyRepo()
	svc := &adminServiceImpl{proxyRepo: repo}

	_, err := svc.CreateProxy(context.Background(), &CreateProxyInput{
		Name: "warp-auto-01", Protocol: "socks5", Host: "127.0.0.1", Port: 1080,
	})
	if err == nil {
		t.Fatal("expected reject for warp-* create")
	}
	if infraerrors.Reason(err) != "PROXY_WARP_MANAGED" {
		t.Fatalf("reason=%q want PROXY_WARP_MANAGED err=%v", infraerrors.Reason(err), err)
	}
	if len(repo.proxies) != 0 {
		t.Fatalf("must not persist warp-* proxy, n=%d", len(repo.proxies))
	}
}

func TestAdminService_UpdateProxy_RejectsWarpHostChange(t *testing.T) {
	repo := newMemProxyRepo()
	p := &Proxy{
		Name: "warp-node-1", Protocol: "socks5",
		Host: "10.0.0.1", Port: 1080, Status: StatusActive, FallbackMode: FallbackModeNone,
	}
	if err := repo.Create(context.Background(), p); err != nil {
		t.Fatal(err)
	}
	svc := &adminServiceImpl{proxyRepo: repo}

	_, err := svc.UpdateProxy(context.Background(), p.ID, &UpdateProxyInput{Host: "10.0.0.99"})
	if err == nil {
		t.Fatal("expected reject host change on warp-*")
	}
	if infraerrors.Reason(err) != "PROXY_WARP_MANAGED" {
		t.Fatalf("reason=%q want PROXY_WARP_MANAGED err=%v", infraerrors.Reason(err), err)
	}
	got, _ := repo.GetByID(context.Background(), p.ID)
	if got.Host != "10.0.0.1" {
		t.Fatalf("host must stay unchanged, got %s", got.Host)
	}
}

func TestAdminService_UpdateProxy_AllowsWarpStatusOnly(t *testing.T) {
	repo := newMemProxyRepo()
	p := &Proxy{
		Name: "warp-node-1", Protocol: "socks5",
		Host: "10.0.0.1", Port: 1080, Status: StatusActive, FallbackMode: FallbackModeNone,
	}
	if err := repo.Create(context.Background(), p); err != nil {
		t.Fatal(err)
	}
	svc := &adminServiceImpl{proxyRepo: repo}

	got, err := svc.UpdateProxy(context.Background(), p.ID, &UpdateProxyInput{Status: StatusInactive})
	if err != nil {
		t.Fatalf("status-only update on warp-* should be allowed: %v", err)
	}
	if got.Status != StatusInactive {
		t.Fatalf("status=%s want inactive", got.Status)
	}
}

func TestAdminService_UpdateProxy_RejectsRenameToWarp(t *testing.T) {
	repo := newMemProxyRepo()
	p := &Proxy{
		Name: "office", Protocol: "http",
		Host: "1.1.1.1", Port: 8080, Status: StatusActive, FallbackMode: FallbackModeNone,
	}
	if err := repo.Create(context.Background(), p); err != nil {
		t.Fatal(err)
	}
	svc := &adminServiceImpl{proxyRepo: repo}

	_, err := svc.UpdateProxy(context.Background(), p.ID, &UpdateProxyInput{Name: "warp-hijack"})
	if err == nil {
		t.Fatal("expected reject rename onto warp-*")
	}
	if infraerrors.Reason(err) != "PROXY_WARP_MANAGED" {
		t.Fatalf("reason=%q want PROXY_WARP_MANAGED", infraerrors.Reason(err))
	}
}

func TestAdminService_DeleteProxy_RejectsWarpManaged(t *testing.T) {
	repo := newMemProxyRepo()
	p := &Proxy{
		Name: "warp-managed-x", Protocol: "socks5",
		Host: "127.0.0.1", Port: 20001, Status: StatusActive,
	}
	if err := repo.Create(context.Background(), p); err != nil {
		t.Fatal(err)
	}
	svc := &adminServiceImpl{proxyRepo: repo}

	err := svc.DeleteProxy(context.Background(), p.ID)
	if err == nil {
		t.Fatal("expected reject delete of warp-*")
	}
	if infraerrors.Reason(err) != "PROXY_WARP_MANAGED" {
		t.Fatalf("reason=%q want PROXY_WARP_MANAGED err=%v", infraerrors.Reason(err), err)
	}
	if _, ok := repo.proxies[p.ID]; !ok {
		t.Fatal("warp-* proxy must still exist after rejected delete")
	}
}

func TestAdminService_BatchDeleteProxies_SkipsWarpManaged(t *testing.T) {
	repo := newMemProxyRepo()
	warp := &Proxy{Name: "warp-a", Protocol: "socks5", Host: "h", Port: 1, Status: StatusActive}
	manual := &Proxy{Name: "manual", Protocol: "http", Host: "h", Port: 2, Status: StatusActive}
	if err := repo.Create(context.Background(), warp); err != nil {
		t.Fatal(err)
	}
	if err := repo.Create(context.Background(), manual); err != nil {
		t.Fatal(err)
	}
	svc := &adminServiceImpl{proxyRepo: repo}

	res, err := svc.BatchDeleteProxies(context.Background(), []int64{warp.ID, manual.ID})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.DeletedIDs) != 1 || res.DeletedIDs[0] != manual.ID {
		t.Fatalf("deleted=%v want [%d]", res.DeletedIDs, manual.ID)
	}
	if len(res.Skipped) != 1 || res.Skipped[0].ID != warp.ID {
		t.Fatalf("skipped=%+v want warp id=%d", res.Skipped, warp.ID)
	}
	if !strings.Contains(res.Skipped[0].Reason, "PROXY_WARP_MANAGED") &&
		!strings.Contains(res.Skipped[0].Reason, "warp") {
		t.Fatalf("skip reason=%q", res.Skipped[0].Reason)
	}
	if _, ok := repo.proxies[warp.ID]; !ok {
		t.Fatal("warp-* must remain")
	}
	if _, ok := repo.proxies[manual.ID]; ok {
		t.Fatal("manual proxy should be deleted")
	}
}

func TestAdminService_UpdateProxy_AllowsWarpExpiresAt(t *testing.T) {
	repo := newMemProxyRepo()
	p := &Proxy{
		Name: "warp-node-exp", Protocol: "socks5",
		Host: "10.0.0.1", Port: 1080, Status: StatusActive, FallbackMode: FallbackModeNone,
	}
	if err := repo.Create(context.Background(), p); err != nil {
		t.Fatal(err)
	}
	svc := &adminServiceImpl{proxyRepo: repo}
	exp := time.Date(2031, 6, 1, 0, 0, 0, 0, time.UTC)

	got, err := svc.UpdateProxy(context.Background(), p.ID, &UpdateProxyInput{
		ExpiresAtProvided: true, ExpiresAt: &exp,
	})
	if err != nil {
		t.Fatalf("expires_at update on warp-* should be allowed: %v", err)
	}
	if got.ExpiresAt == nil || !got.ExpiresAt.Equal(exp) {
		t.Fatalf("expires_at=%v want %v", got.ExpiresAt, exp)
	}
}
