package repository

import (
	"context"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

type stubProxyGroupResolver struct {
	selected      *service.Proxy
	err           error
	calls         int
	lastGroupID   int64
	lastAccountID int64
}

func (s *stubProxyGroupResolver) ResolveProxy(_ context.Context, groupID, accountID int64) (*service.Proxy, error) {
	s.calls++
	s.lastGroupID = groupID
	s.lastAccountID = accountID
	return s.selected, s.err
}

func (s *stubProxyGroupResolver) InvalidateGroup(int64) {}

func TestApplyProxyGroupSelection_PriorityAndNoProxyIDWrite(t *testing.T) {
	t.Parallel()

	groupID := int64(11)
	proxyID := int64(22)
	selected := &service.Proxy{ID: 7, Protocol: "http", Host: "pool.example", Port: 8080}
	resolver := &stubProxyGroupResolver{selected: selected}
	repo := &accountRepository{proxyGroupResolver: resolver}

	t.Run("proxy_id 优先：已有 Proxy 时不调 resolver", func(t *testing.T) {
		t.Parallel()
		r := &stubProxyGroupResolver{selected: selected}
		repo := &accountRepository{proxyGroupResolver: r}
		existing := &service.Proxy{ID: proxyID, Protocol: "http", Host: "single", Port: 1}
		acc := &service.Account{
			ID:           1,
			ProxyID:      &proxyID,
			ProxyGroupID: &groupID,
			Proxy:        existing,
		}
		repo.applyProxyGroupSelection(context.Background(), acc)
		require.Equal(t, 0, r.calls)
		require.Equal(t, existing, acc.Proxy)
		require.Equal(t, &proxyID, acc.ProxyID)
	})

	t.Run("仅 proxy_group_id：填入 Proxy 且不写 ProxyID", func(t *testing.T) {
		t.Parallel()
		r := &stubProxyGroupResolver{selected: selected}
		repo := &accountRepository{proxyGroupResolver: r}
		acc := &service.Account{
			ID:           42,
			ProxyGroupID: &groupID,
		}
		repo.applyProxyGroupSelection(context.Background(), acc)
		require.Equal(t, 1, r.calls)
		require.Equal(t, groupID, r.lastGroupID)
		require.Equal(t, int64(42), r.lastAccountID)
		require.Equal(t, selected, acc.Proxy)
		require.Nil(t, acc.ProxyID, "C1: MUST NOT write ProxyID")
	})

	t.Run("无 group 且无 resolver 时 no-op", func(t *testing.T) {
		t.Parallel()
		acc := &service.Account{ID: 3}
		repo.applyProxyGroupSelection(context.Background(), acc)
		require.Nil(t, acc.Proxy)
		require.Nil(t, acc.ProxyID)
	})

	t.Run("resolver 失败不 panic 且不写 Proxy", func(t *testing.T) {
		t.Parallel()
		r := &stubProxyGroupResolver{err: service.ErrProxyGroupNotFound}
		repo := &accountRepository{proxyGroupResolver: r}
		acc := &service.Account{ID: 9, ProxyGroupID: &groupID}
		repo.applyProxyGroupSelection(context.Background(), acc)
		require.Equal(t, 1, r.calls)
		require.Nil(t, acc.Proxy)
		require.Nil(t, acc.ProxyID)
	})
}
