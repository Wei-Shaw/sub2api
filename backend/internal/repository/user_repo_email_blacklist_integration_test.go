//go:build integration

package repository

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func emailBlacklistProbeKeys(emails ...string) []string {
	keys := make([]string, 0, 2*len(emails))
	for _, email := range emails {
		for _, probe := range service.EmailAliasDedupProbes(email) {
			keys = append(keys, service.EmailAliasProbeKey(probe))
		}
	}
	return keys
}

func (s *UserRepoSuite) TestListActiveUsersByEmailAliasProbeKeys_MatchesInboxVariants() {
	dotted := s.mustCreateUser(&service.User{Email: "bl.target@gmail.com"})
	plusGooglemail := s.mustCreateUser(&service.User{Email: "bltarget+promo@googlemail.com"})
	upperRootDot := s.mustCreateUser(&service.User{Email: "BL.Tar.Get@Gmail.com."})
	nonGmailDotted := s.mustCreateUser(&service.User{Email: "bl.target@company.example"})
	s.mustCreateUser(&service.User{Email: "bltarget2@gmail.com"})
	s.mustCreateUser(&service.User{Email: "b.l.target@gmail.com", Status: service.StatusDisabled})
	s.mustCreateUser(&service.User{Email: "bltarget@gmail.com", Role: service.RoleAdmin})
	deleted := s.mustCreateUser(&service.User{Email: "bltar.get+x@gmail.com"})
	s.Require().NoError(s.repo.Delete(s.ctx, deleted.ID))

	users, err := s.repo.ListActiveUsersByEmailAliasProbeKeys(s.ctx,
		emailBlacklistProbeKeys("bltarget@googlemail.com", "bltarget@company.example"))
	s.Require().NoError(err)

	ids := make([]int64, 0, len(users))
	for _, user := range users {
		s.Require().Equal(service.StatusActive, user.Status)
		ids = append(ids, user.ID)
	}
	// 仓储层按去点形态过度匹配非 Gmail 域名（bl.target@company.example），由 service 层复核。
	s.Require().ElementsMatch([]int64{dotted.ID, plusGooglemail.ID, upperRootDot.ID, nonGmailDotted.ID}, ids)
}

func (s *UserRepoSuite) TestListActiveUsersByEmailAliasProbeKeys_EmptyKeys() {
	s.mustCreateUser(&service.User{Email: "bl-empty@gmail.com"})

	users, err := s.repo.ListActiveUsersByEmailAliasProbeKeys(s.ctx, nil)
	s.Require().NoError(err)
	s.Require().Empty(users)
}

func (s *UserRepoSuite) TestBatchDisableUsers_SkipsAdminDisabledAndDeleted() {
	active := s.mustCreateUser(&service.User{Email: "bl-disable-active@example.com"})
	admin := s.mustCreateUser(&service.User{Email: "bl-disable-admin@example.com", Role: service.RoleAdmin})
	already := s.mustCreateUser(&service.User{Email: "bl-disable-already@example.com", Status: service.StatusDisabled})
	deleted := s.mustCreateUser(&service.User{Email: "bl-disable-deleted@example.com"})
	s.Require().NoError(s.repo.Delete(s.ctx, deleted.ID))

	disabled, err := s.repo.BatchDisableUsers(s.ctx, []int64{active.ID, admin.ID, already.ID, deleted.ID, 99999999})
	s.Require().NoError(err)
	s.Require().Equal([]int64{active.ID}, disabled)

	got, err := s.repo.GetByID(s.ctx, active.ID)
	s.Require().NoError(err)
	s.Require().Equal(service.StatusDisabled, got.Status)

	gotAdmin, err := s.repo.GetByID(s.ctx, admin.ID)
	s.Require().NoError(err)
	s.Require().Equal(service.StatusActive, gotAdmin.Status)
}

// 批量禁用与单个禁用一样持久吊销 MCP 授权：恢复账号后旧授权与令牌不会复活，被跳过的用户授权不受影响。
func TestBatchDisableUsers_RevokesMCPGrantsPersistently(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	target := newMCPOAuthIntegrationFixture(t, ctx)
	skippedAdmin := newMCPOAuthIntegrationFixture(t, ctx)
	issued, err := target.oauth.Token(ctx, target.input, mcpIntegrationResource)
	require.NoError(t, err)

	users := NewUserRepository(integrationEntClient, integrationDB)
	admin, err := users.GetByID(ctx, skippedAdmin.userID)
	require.NoError(t, err)
	admin.Role = service.RoleAdmin
	require.NoError(t, users.Update(ctx, admin, service.UserUpdateFields{Role: true}))

	blacklistRepo, ok := users.(service.UserEmailBlacklistRepository)
	require.True(t, ok)
	disabled, err := blacklistRepo.BatchDisableUsers(ctx, []int64{target.userID, skippedAdmin.userID})
	require.NoError(t, err)
	require.Equal(t, []int64{target.userID}, disabled)

	requireRevoked := func(f mcpOAuthIntegrationFixture, revoked bool) {
		t.Helper()
		grant, err := f.repo.GetGrant(ctx, f.grantID)
		require.NoError(t, err)
		require.Equal(t, revoked, grant.RevokedAt != nil, f.grantID)
	}
	requireRevoked(target, true)
	requireRevoked(skippedAdmin, false)

	user, err := users.GetByID(ctx, target.userID)
	require.NoError(t, err)
	user.Status = service.StatusActive
	require.NoError(t, users.Update(ctx, user, service.UserUpdateFields{Status: true}))
	requireRevoked(target, true)
	_, err = target.oauth.Authenticate(ctx, issued.AccessToken)
	require.ErrorIs(t, err, service.ErrMCPOAuthInvalidToken)
}

// 授权创建持有用户行锁时发起批量禁用：禁用等锁释放后执行，并吊销对方刚提交的授权。
func TestBatchDisableUsers_RevokesGrantCommittedWhileWaitingForUserLock(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	f := newMCPOAuthIntegrationFixture(t, ctx)

	// 与 mcpOAuthRepository.CreateAuthorization 相同的加锁顺序：先锁用户行，再插入授权。
	lockTx, err := integrationDB.BeginTx(ctx, nil)
	require.NoError(t, err)
	defer func() { _ = lockTx.Rollback() }()
	_, err = lockTx.ExecContext(ctx, `SELECT id FROM users WHERE id=$1 FOR UPDATE`, f.userID)
	require.NoError(t, err)
	now := time.Now().UTC()
	const racingGrantID = "bl-race-grant-0000000000000000"
	_, err = lockTx.ExecContext(ctx, `
INSERT INTO mcp_oauth_grants (id, client_id, client_name, user_id, token_version, resource, scope, read_only, created_at, expires_at)
VALUES ($1, $2, 'racing client', $3, 1, $4, 'user', TRUE, $5, $6)`,
		racingGrantID, f.input.ClientID, f.userID, mcpIntegrationResource, now, now.Add(time.Hour))
	require.NoError(t, err)

	users := NewUserRepository(integrationEntClient, integrationDB).(service.UserEmailBlacklistRepository)
	type batchResult struct {
		disabled []int64
		err      error
	}
	done := make(chan batchResult, 1)
	go func() {
		disabled, err := users.BatchDisableUsers(ctx, []int64{f.userID})
		done <- batchResult{disabled: disabled, err: err}
	}()

	require.Eventually(t, func() bool {
		var waiting int
		err := integrationDB.QueryRowContext(ctx, `
SELECT COUNT(*) FROM pg_stat_activity
WHERE datname = current_database() AND wait_event_type = 'Lock' AND query LIKE '%UPDATE users SET status%'`).Scan(&waiting)
		return err == nil && waiting > 0
	}, 10*time.Second, 10*time.Millisecond, "batch disable should wait for the user row lock")
	require.NoError(t, lockTx.Commit())

	result := <-done
	require.NoError(t, result.err)
	require.Equal(t, []int64{f.userID}, result.disabled)
	for _, id := range []string{f.grantID, racingGrantID} {
		grant, err := f.repo.GetGrant(ctx, id)
		require.NoError(t, err)
		require.NotNil(t, grant.RevokedAt, id)
	}
}
