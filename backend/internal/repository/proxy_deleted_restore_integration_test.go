//go:build integration

package repository

import (
	"context"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/lib/pq"
	"github.com/stretchr/testify/require"
	"testing"
)

func (s *ProxyExpirySuite) TestRevertRejectsDeletedOrigin() {
	original := s.mkProxy("deleted-original", service.FallbackModeDirect, nil, nil)
	backup := s.mkProxy("current-backup", service.FallbackModeNone, nil, nil)
	account := s.mkAccountWithProxy(backup)
	_, err := s.tx.ExecContext(s.ctx, `UPDATE accounts SET proxy_fallback_origin_id=$1 WHERE id=$2`, original, account)
	s.Require().NoError(err)
	count, err := s.repo.CountAccountsByProxyID(s.ctx, original)
	s.Require().NoError(err)
	s.Require().Zero(count)
	s.Require().NoError(s.repo.Delete(s.ctx, original))
	repo := newAccountRepositoryWithSQL(s.tx.Client(), s.tx, nil)
	err = repo.RevertProxyFallback(s.ctx, account)
	s.ErrorIs(err, service.ErrProxyNotFound)
	s.Equal(&backup, s.accountProxyID(account))
	var origin *int64
	s.Require().NoError(scanSingleRow(s.ctx, s.tx, `SELECT proxy_fallback_origin_id FROM accounts WHERE id=$1`, []any{account}, &origin))
	s.Equal(&original, origin)
	var countAfter int64
	s.Require().NoError(scanSingleRow(s.ctx, s.tx, `SELECT COUNT(*) FROM scheduler_outbox WHERE event_type=$1 AND account_id=$2`, []any{service.SchedulerOutboxEventAccountChanged, account}, &countAfter))
	s.Zero(countAfter, "failed restore must not enqueue an account change")
}
func (s *ProxyExpirySuite) TestRevertWithoutOriginKeepsExistingError() {
	proxy := s.mkProxy("not-fallback", service.FallbackModeNone, nil, nil)
	account := s.mkAccountWithProxy(proxy)
	repo := newAccountRepositoryWithSQL(s.tx.Client(), s.tx, nil)
	s.ErrorIs(repo.RevertProxyFallback(s.ctx, account), service.ErrAccountNotInFallback)
}

// FOR SHARE must protect deleted_at, not just the foreign-key identity.
func TestRevertProxyHoldsLockAgainstSoftDelete(t *testing.T) {
	ctx := context.Background()
	client := testEntClient(t)
	proxies := newProxyRepositoryWithSQL(client, integrationDB)
	original := &service.Proxy{Name: "locked-origin", Protocol: "http", Host: "127.0.0.1", Port: 8080, Status: service.StatusActive, FallbackMode: service.FallbackModeNone}
	require.NoError(t, proxies.Create(ctx, original))
	account := mustCreateAccount(t, client, &service.Account{Name: "locked-restore", Platform: service.PlatformOpenAI, Type: service.AccountTypeAPIKey, Credentials: map[string]any{"api_key": "test"}, Extra: map[string]any{}})
	_, err := integrationDB.ExecContext(ctx, `UPDATE accounts SET proxy_fallback_origin_id=$1 WHERE id=$2`, original.ID, account.ID)
	require.NoError(t, err)
	restoreTx := testEntTx(t)
	repo := newAccountRepositoryWithSQL(restoreTx.Client(), restoreTx, nil)
	require.NoError(t, repo.RevertProxyFallback(ctx, account.ID))
	deleteTx, err := integrationDB.BeginTx(ctx, nil)
	require.NoError(t, err)
	defer func() { _ = deleteTx.Rollback() }()
	_, err = deleteTx.ExecContext(ctx, `SET LOCAL lock_timeout='100ms'`)
	require.NoError(t, err)
	_, err = deleteTx.ExecContext(ctx, `UPDATE proxies SET deleted_at=NOW() WHERE id=$1`, original.ID)
	var pgErr *pq.Error
	require.ErrorAs(t, err, &pgErr)
	require.Equal(t, pq.ErrorCode("55P03"), pgErr.Code, "soft deletion must wait for restore transaction")
}
