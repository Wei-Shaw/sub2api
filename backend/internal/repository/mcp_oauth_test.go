//go:build unit

package repository

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/ent/securitysecret"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestMCPOAuthRepositoryClientIDKeyIsPersistedAndCached(t *testing.T) {
	ctx := context.Background()
	client := newSecuritySecretTestClient(t)
	repo := NewMCPOAuthRepository(client, nil)
	key, err := repo.ClientIDKey(ctx)
	require.NoError(t, err)
	require.Len(t, key, 64)
	other, err := NewMCPOAuthRepository(client, nil).ClientIDKey(ctx)
	require.NoError(t, err)
	require.Equal(t, key, other, "instances must share the stored key")
	_, err = client.SecuritySecret.Delete().Where(securitysecret.KeyEQ(mcpOAuthClientIDKeyName)).Exec(ctx)
	require.NoError(t, err)
	cached, err := repo.ClientIDKey(ctx)
	require.NoError(t, err)
	require.Equal(t, key, cached, "a loaded key is served from memory")
}

func TestMCPOAuthRepositoryClientIDKeyDoesNotCacheInvalidSecret(t *testing.T) {
	ctx := context.Background()
	client := newSecuritySecretTestClient(t)
	require.NoError(t, client.SecuritySecret.Create().SetKey(mcpOAuthClientIDKeyName).SetValue("short").Exec(ctx))
	repo := NewMCPOAuthRepository(client, nil)
	_, err := repo.ClientIDKey(ctx)
	require.Error(t, err)
	valid := strings.Repeat("c", 64)
	require.NoError(t, client.SecuritySecret.Update().Where(securitysecret.KeyEQ(mcpOAuthClientIDKeyName)).SetValue(valid).Exec(ctx))
	key, err := repo.ClientIDKey(ctx)
	require.NoError(t, err)
	require.Equal(t, []byte(valid), key)
}

func TestMCPOAuthRepositoryConsumptionCommitsReplayRevocation(t *testing.T) {
	for _, rotating := range []bool{false, true} {
		t.Run(map[bool]string{false: "code", true: "refresh"}[rotating], func(t *testing.T) {
			db, mock, err := sqlmock.New()
			require.NoError(t, err)
			defer db.Close()
			repo := NewMCPOAuthRepository(nil, db)
			now := time.Now().UTC()
			used := now.Add(-time.Minute)
			mock.ExpectBegin()
			mock.ExpectQuery(`SELECT revoked_at,expires_at FROM mcp_oauth_grants WHERE id=\$1 FOR UPDATE`).WithArgs("grant").
				WillReturnRows(sqlmock.NewRows([]string{"revoked_at", "expires_at"}).AddRow(nil, now.Add(time.Hour)))
			if rotating {
				mock.ExpectQuery(`SELECT expires_at,used_at FROM mcp_oauth_credentials WHERE key_hash=\$1 AND grant_id=\$2 AND kind='refresh' FOR UPDATE`).WithArgs("old-hash", "grant").
					WillReturnRows(sqlmock.NewRows([]string{"expires_at", "used_at"}).AddRow(now.Add(time.Hour), used))
			} else {
				mock.ExpectQuery(`SELECT expires_at,consumed_at FROM mcp_oauth_codes WHERE code_hash=\$1 AND grant_id=\$2 FOR UPDATE`).WithArgs("old-hash", "grant").
					WillReturnRows(sqlmock.NewRows([]string{"expires_at", "consumed_at"}).AddRow(now.Add(time.Hour), used))
			}
			mock.ExpectExec(`UPDATE mcp_oauth_grants SET revoked_at=\$2 WHERE id=\$1`).WithArgs("grant", now).WillReturnResult(sqlmock.NewResult(0, 1))
			// The security change MUST commit despite returning an error to OAuth.
			mock.ExpectCommit()
			if rotating {
				err = repo.RotateRefresh(context.Background(), "old-hash", "grant", now, nil, nil)
			} else {
				err = repo.ExchangeCode(context.Background(), "old-hash", "grant", now, nil, nil)
			}
			require.ErrorIs(t, err, service.ErrMCPOAuthNotFound)
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestMCPOAuthRepositoryConsumptionAndIssuanceAreOneTransaction(t *testing.T) {
	for _, rotating := range []bool{false, true} {
		for _, failInsert := range []bool{false, true} {
			t.Run(map[bool]string{false: "code", true: "refresh"}[rotating]+map[bool]string{false: "/commit", true: "/rollback"}[failInsert], func(t *testing.T) {
				db, mock, err := sqlmock.New()
				require.NoError(t, err)
				defer db.Close()
				repo := NewMCPOAuthRepository(nil, db)
				now := time.Now().UTC()
				access := &service.MCPOAuthCredential{Hash: "access-digest", GrantID: "grant", Kind: "access", ExpiresAt: now.Add(10 * time.Minute)}
				refresh := &service.MCPOAuthCredential{Hash: "refresh-digest", GrantID: "grant", Kind: "refresh", ExpiresAt: now.Add(time.Hour)}
				mock.ExpectBegin()
				mock.ExpectQuery(`SELECT revoked_at,expires_at FROM mcp_oauth_grants WHERE id=\$1 FOR UPDATE`).WithArgs("grant").
					WillReturnRows(sqlmock.NewRows([]string{"revoked_at", "expires_at"}).AddRow(nil, now.Add(time.Hour)))
				if rotating {
					mock.ExpectQuery(`SELECT expires_at,used_at FROM mcp_oauth_credentials`).WithArgs("old-hash", "grant").WillReturnRows(sqlmock.NewRows([]string{"expires_at", "used_at"}).AddRow(now.Add(time.Hour), nil))
					mock.ExpectExec(`UPDATE mcp_oauth_credentials SET used_at=\$2 WHERE key_hash=\$1`).WithArgs("old-hash", now).WillReturnResult(sqlmock.NewResult(0, 1))
				} else {
					mock.ExpectQuery(`SELECT expires_at,consumed_at FROM mcp_oauth_codes`).WithArgs("old-hash", "grant").WillReturnRows(sqlmock.NewRows([]string{"expires_at", "consumed_at"}).AddRow(now.Add(time.Hour), nil))
					mock.ExpectExec(`UPDATE mcp_oauth_codes SET consumed_at=\$2 WHERE code_hash=\$1`).WithArgs("old-hash", now).WillReturnResult(sqlmock.NewResult(0, 1))
				}
				mock.ExpectExec(`INSERT INTO mcp_oauth_credentials`).WithArgs(access.Hash, "grant", "access", access.ExpiresAt).WillReturnResult(sqlmock.NewResult(0, 1))
				insert := mock.ExpectExec(`INSERT INTO mcp_oauth_credentials`).WithArgs(refresh.Hash, "grant", "refresh", refresh.ExpiresAt)
				injected := errors.New("database unavailable")
				if failInsert {
					insert.WillReturnError(injected)
					mock.ExpectRollback()
				} else {
					insert.WillReturnResult(sqlmock.NewResult(0, 1))
					mock.ExpectCommit()
				}
				if rotating {
					err = repo.RotateRefresh(context.Background(), "old-hash", "grant", now, access, refresh)
				} else {
					err = repo.ExchangeCode(context.Background(), "old-hash", "grant", now, access, refresh)
				}
				if failInsert {
					require.ErrorIs(t, err, injected)
				} else {
					require.NoError(t, err)
				}
				require.NoError(t, mock.ExpectationsWereMet())
			})
		}
	}
}

func TestMCPOAuthRepositoryRejectsExpiredAndRevokedFamilies(t *testing.T) {
	for _, revoked := range []bool{false, true} {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		repo := NewMCPOAuthRepository(nil, db)
		now := time.Now().UTC()
		var revokedAt *time.Time
		expiry := now.Add(-time.Second)
		if revoked {
			revokedAt, expiry = &now, now.Add(time.Hour)
		}
		mock.ExpectBegin()
		mock.ExpectQuery(`SELECT revoked_at,expires_at FROM mcp_oauth_grants WHERE id=\$1 FOR UPDATE`).WithArgs("grant").WillReturnRows(sqlmock.NewRows([]string{"revoked_at", "expires_at"}).AddRow(revokedAt, expiry))
		mock.ExpectRollback()
		err = repo.RotateRefresh(context.Background(), "old-hash", "grant", now, nil, nil)
		require.ErrorIs(t, err, service.ErrMCPOAuthNotFound)
		require.NoError(t, mock.ExpectationsWereMet())
		mock.ExpectClose()
		require.NoError(t, db.Close())
	}
}

func TestMCPOAuthRepositoryAuthorizationSerializesLimit(t *testing.T) {
	for _, atLimit := range []bool{false, true} {
		t.Run(map[bool]string{false: "create", true: "at limit"}[atLimit], func(t *testing.T) {
			db, mock, err := sqlmock.New()
			require.NoError(t, err)
			defer db.Close()
			repo := NewMCPOAuthRepository(nil, db)
			now := time.Now().UTC()
			grant := &service.MCPOAuthGrant{ID: "grant", ClientID: "client", ClientName: "Client", UserID: 7, TokenVersion: 42, Resource: "https://example.com/mcp", Scope: "user", ReadOnly: true, CreatedAt: now, ExpiresAt: now.Add(time.Hour)}
			code := &service.MCPOAuthCode{Hash: "code-digest", GrantID: grant.ID, ClientID: grant.ClientID, RedirectURI: "https://client.example/cb", Resource: grant.Resource, CodeChallenge: "challenge", ExpiresAt: now.Add(time.Minute)}
			count := 19
			if atLimit {
				count = 20
			}
			// Bounded retention runs before the owner row is locked.
			mock.ExpectExec(`DELETE FROM mcp_oauth_grants WHERE id IN \(SELECT id FROM mcp_oauth_grants WHERE expires_at<\$1 ORDER BY expires_at LIMIT 100 FOR UPDATE SKIP LOCKED\)`).WithArgs(now.Add(-7 * 24 * time.Hour)).WillReturnResult(sqlmock.NewResult(0, 2))
			mock.ExpectBegin()
			mock.ExpectQuery(`SELECT id FROM users WHERE id=\$1 AND deleted_at IS NULL FOR UPDATE`).WithArgs(int64(7)).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(7))
			mock.ExpectQuery(`SELECT COUNT\(\*\) FROM mcp_oauth_grants WHERE user_id=\$1 AND revoked_at IS NULL AND expires_at>\$2`).WithArgs(int64(7), now).WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(count))
			if atLimit {
				mock.ExpectRollback()
				require.ErrorIs(t, repo.CreateAuthorization(context.Background(), grant, code, 20), service.ErrMCPOAuthGrantLimit)
			} else {
				mock.ExpectExec(`INSERT INTO mcp_oauth_grants`).WithArgs(grant.ID, grant.ClientID, grant.ClientName, grant.UserID, grant.TokenVersion, grant.Resource, grant.Scope, grant.ReadOnly, grant.CreatedAt, grant.ExpiresAt).WillReturnResult(sqlmock.NewResult(0, 1))
				mock.ExpectExec(`INSERT INTO mcp_oauth_codes`).WithArgs(code.Hash, code.GrantID, code.ClientID, code.RedirectURI, code.Resource, code.CodeChallenge, code.ExpiresAt).WillReturnResult(sqlmock.NewResult(0, 1))
				mock.ExpectCommit()
				require.NoError(t, repo.CreateAuthorization(context.Background(), grant, code, 20))
			}
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestMCPOAuthRepositoryOwnerAndRevocationBoundaries(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	repo := NewMCPOAuthRepository(nil, db)
	now := time.Now().UTC()
	mock.ExpectExec(`UPDATE mcp_oauth_grants SET revoked_at=COALESCE\(revoked_at,\$3\) WHERE id=\$1 AND user_id=\$2`).WithArgs("grant", int64(8), now).WillReturnResult(sqlmock.NewResult(0, 0))
	require.ErrorIs(t, repo.RevokeGrant(context.Background(), 8, "grant", now), service.ErrMCPOAuthNotFound)
	mock.ExpectExec(`UPDATE mcp_oauth_grants SET last_used_at=\$2 WHERE id=\$1 AND revoked_at IS NULL AND expires_at>\$2`).WithArgs("grant", now).WillReturnResult(sqlmock.NewResult(0, 0))
	require.ErrorIs(t, repo.TouchGrant(context.Background(), "grant", now), service.ErrMCPOAuthNotFound)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestMCPOAuthRepositoryRevokeUserGrantsCoversEveryActiveGrant(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	repo := NewMCPOAuthRepository(nil, db)
	now := time.Now().UTC()
	mock.ExpectExec(`UPDATE mcp_oauth_grants SET revoked_at=\$2 WHERE user_id=\$1 AND revoked_at IS NULL$`).WithArgs(int64(8), now).WillReturnResult(sqlmock.NewResult(0, 0))
	require.NoError(t, repo.RevokeUserGrants(context.Background(), 8, now), "no active grants is not an error")
	mock.ExpectExec(`UPDATE mcp_oauth_grants SET revoked_at=\$2 WHERE user_id=\$1 AND revoked_at IS NULL AND scope='admin'`).WithArgs(int64(8), now).WillReturnResult(sqlmock.NewResult(0, 3))
	require.NoError(t, revokeMCPGrants(context.Background(), db, 8, true, now))
	require.NoError(t, mock.ExpectationsWereMet())
}
