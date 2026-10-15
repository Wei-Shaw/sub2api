package service

import (
	"context"
	"testing"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

type proxyGroupServiceRepoStub struct {
	proxy *Proxy
	err   error
}

func (s *proxyGroupServiceRepoStub) List(context.Context, int, int, string, string) ([]ProxyGroup, int64, error) {
	return nil, 0, nil
}

func (s *proxyGroupServiceRepoStub) ListAll(context.Context) ([]ProxyGroup, error) {
	return nil, nil
}

func (s *proxyGroupServiceRepoStub) GetByID(context.Context, int64) (*ProxyGroup, error) {
	return nil, nil
}

func (s *proxyGroupServiceRepoStub) Create(context.Context, *ProxyGroup, []int64) error {
	return nil
}

func (s *proxyGroupServiceRepoStub) Update(context.Context, *ProxyGroup, []int64) error {
	return nil
}

func (s *proxyGroupServiceRepoStub) Delete(context.Context, int64) error {
	return nil
}

func (s *proxyGroupServiceRepoStub) CountAccountsByGroupID(context.Context, int64) (int64, error) {
	return 0, nil
}

func (s *proxyGroupServiceRepoStub) SelectRandomAvailableProxy(context.Context, int64) (*Proxy, error) {
	return s.proxy, s.err
}

func TestProxyGroupServiceResolveAccountProxy(t *testing.T) {
	proxy := &Proxy{ID: 42, Name: "proxy-42"}
	svc := NewProxyGroupService(&proxyGroupServiceRepoStub{proxy: proxy})
	defer SetDefaultProxyGroupResolver(nil)
	groupID := int64(7)
	account := &Account{ProxyGroupID: &groupID}

	if err := svc.ResolveAccountProxy(context.Background(), account); err != nil {
		t.Fatalf("resolve account proxy: %v", err)
	}
	if account.ProxyID == nil || *account.ProxyID != proxy.ID {
		t.Fatalf("proxy_id = %v, want %d", account.ProxyID, proxy.ID)
	}
	if account.Proxy != proxy {
		t.Fatalf("resolved proxy pointer was not attached to account")
	}
}

func TestProxyGroupServiceResolveAccountProxyNoAvailableProxy(t *testing.T) {
	svc := NewProxyGroupService(&proxyGroupServiceRepoStub{})
	defer SetDefaultProxyGroupResolver(nil)
	groupID := int64(7)
	account := &Account{ProxyGroupID: &groupID}

	err := svc.ResolveAccountProxy(context.Background(), account)
	if infraerrors.Reason(err) != "PROXY_GROUP_NO_AVAILABLE_PROXY" {
		t.Fatalf("reason = %q, want PROXY_GROUP_NO_AVAILABLE_PROXY", infraerrors.Reason(err))
	}
	if account.ProxyID != nil || account.Proxy != nil {
		t.Fatalf("account proxy binding changed on failed resolution")
	}
}
