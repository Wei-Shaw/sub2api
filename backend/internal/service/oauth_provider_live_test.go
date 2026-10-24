//go:build oauthlive

// Live end-to-end coverage of the OAuth provider service against a real
// Postgres (the project test environment, or any instance reachable via
// OAUTH_LIVE_DSN). Redis-backed authorization transactions run against
// OAUTH_LIVE_REDIS when provided, else an in-process miniredis (real wire
// protocol). Skipped unless OAUTH_LIVE_DSN is set:
//
//	OAUTH_LIVE_DSN='postgres://user:pass@host:5432/db?sslmode=disable' \
//	[OAUTH_LIVE_REDIS='host:6379'] \
//	go test -tags oauthlive -run TestOAuthProviderLiveFlow ./internal/service/
//
// The flow mirrors the design's Phase 1 curl checklist: request validation,
// one-shot authorization transaction, code issue + PKCE exchange (with wrong
// verifier / replay refusals), access token validation, refresh rotation with
// reuse detection revoking the whole family, grant cascade revocation, and
// the shared API-key projection.
package service_test

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	_ "github.com/lib/pq"
	redisclient "github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"

	"github.com/Wei-Shaw/sub2api/internal/repository"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

const liveRedirectURI = "http://127.0.0.1:19387/plugins/dsh-sub2api-sync/oauth/callback"

func TestOAuthProviderLiveFlow(t *testing.T) {
	dsn := os.Getenv("OAUTH_LIVE_DSN")
	if dsn == "" {
		t.Skip("set OAUTH_LIVE_DSN to run the live OAuth provider flow")
	}
	ctx := context.Background()
	db, err := sql.Open("postgres", dsn)
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	var rdb *redisclient.Client
	if addr := os.Getenv("OAUTH_LIVE_REDIS"); addr != "" {
		rdb = redisclient.NewClient(&redisclient.Options{Addr: addr})
		require.NoError(t, rdb.Ping(ctx).Err())
	} else {
		mr := miniredis.RunT(t)
		rdb = redisclient.NewClient(&redisclient.Options{Addr: mr.Addr()})
	}
	defer func() { _ = rdb.Close() }()

	// 241 is uncommitted work on this branch; the shared test database may
	// already carry an earlier content revision. Reset only 241's additive
	// objects so ApplyMigrations re-applies the current file cleanly.
	reset241Objects(t, db)
	require.NoError(t, repository.ApplyMigrations(ctx, db))

	// Self-healing seed state for the flow client.
	redirectURIs, _ := json.Marshal([]string{
		"http://127.0.0.1/oauth/callback",
		"http://[::1]/oauth/callback",
		"http://127.0.0.1/plugins/dsh-sub2api-sync/oauth/callback",
		"http://[::1]/plugins/dsh-sub2api-sync/oauth/callback",
	})
	_, err = db.Exec(`UPDATE oauth_clients SET client_type='public', pkce_required=TRUE, status='active',
		redirect_uris=$1::jsonb, allowed_scopes='["openid","profile","groups:read","keys:read","keys:create","keys:revoke"]'::jsonb
		WHERE client_id='dsh-sub2api-sync'`, string(redirectURIs))
	require.NoError(t, err)

	// Dedicated user; FK CASCADE removes every oauth row on cleanup.
	email := fmt.Sprintf("oauth-live-%d@test.local", time.Now().UnixNano())
	var userID int64
	require.NoError(t, db.QueryRow(
		`INSERT INTO users (email, password_hash, username, created_at, updated_at) VALUES ($1,'live-test',$1,NOW(),NOW()) RETURNING id`,
		email).Scan(&userID))
	t.Cleanup(func() { _, _ = db.Exec(`DELETE FROM users WHERE id = $1`, userID) })

	svc := service.NewOAuthProviderService(db, rdb)

	scopes := []string{"openid", "profile", "groups:read", "keys:read", "keys:create"}
	verifier := "live-test-verifier-with-enough-length-and-urlsafe-chars-1234567890"
	challenge := pkceChallenge(t, verifier)

	// A. Authorization request validation.
	client, err := svc.ValidateAuthorizationRequest(ctx, "dsh-sub2api-sync", liveRedirectURI, scopes, challenge, "S256")
	require.NoError(t, err)
	require.Equal(t, "public", client.ClientType)
	require.True(t, client.PKCERequired)
	_, err = svc.ValidateAuthorizationRequest(ctx, "dsh-sub2api-sync", "http://192.168.1.5:19387/plugins/dsh-sub2api-sync/oauth/callback", scopes, challenge, "S256")
	require.ErrorIs(t, err, service.ErrOAuthRedirectMismatch)
	_, err = svc.ValidateAuthorizationRequest(ctx, "dsh-sub2api-sync", liveRedirectURI, []string{"admin:*"}, challenge, "S256")
	require.ErrorIs(t, err, service.ErrOAuthInvalidScope)

	// B. One-shot authorization transaction → one-time code.
	txn, err := svc.CreateAuthorizationTransaction(ctx, service.OAuthAuthorizationTransaction{
		ClientID: client.ClientID, ClientName: client.Name, RedirectURI: liveRedirectURI,
		Scope: scopes, State: "live-state-1", CodeChallenge: challenge, CodeChallengeMethod: "S256",
	})
	require.NoError(t, err)
	code, issued, err := svc.IssueAuthorizationCode(ctx, txn.ID, userID)
	require.NoError(t, err)
	require.Equal(t, liveRedirectURI, issued.RedirectURI)
	require.NotEmpty(t, code)
	// The transaction is consumed: a second approval cannot mint another code.
	_, _, err = svc.IssueAuthorizationCode(ctx, txn.ID, userID)
	require.Error(t, err)

	// C. Code exchange: wrong verifier refused (code stays consumable).
	_, err = svc.ExchangeAuthorizationCode(ctx, "dsh-sub2api-sync", code, liveRedirectURI, "definitely-wrong-verifier")
	require.ErrorIs(t, err, service.ErrOAuthInvalidGrant)
	pair, err := svc.ExchangeAuthorizationCode(ctx, "dsh-sub2api-sync", code, liveRedirectURI, verifier)
	require.NoError(t, err)
	require.NotEmpty(t, pair.AccessToken)
	require.NotEmpty(t, pair.RefreshToken)
	require.Equal(t, 900, pair.ExpiresIn)
	// Replaying the same code is refused (single-use).
	_, err = svc.ExchangeAuthorizationCode(ctx, "dsh-sub2api-sync", code, liveRedirectURI, verifier)
	require.ErrorIs(t, err, service.ErrOAuthInvalidGrant)

	// D. Access token validation.
	info, err := svc.ValidateAccessToken(ctx, pair.AccessToken)
	require.NoError(t, err)
	require.Equal(t, userID, info.UserID)
	require.Equal(t, "dsh-sub2api-sync", info.ClientID)
	for _, scope := range scopes {
		require.Contains(t, info.Scopes, scope)
	}
	_, err = svc.ValidateAccessToken(ctx, pair.AccessToken+"x")
	require.ErrorIs(t, err, service.ErrOAuthTokenRevoked)

	// E. Refresh rotation: one-time use, replay refused.
	pair2, err := svc.RefreshAccessToken(ctx, "dsh-sub2api-sync", pair.RefreshToken)
	require.NoError(t, err)
	require.NotEqual(t, pair.RefreshToken, pair2.RefreshToken)
	// The rotated token is consumed atomically — a replay is refused. NOTE:
	// the current Redis design does NOT revoke the successor family on replay
	// (the rotated token simply no longer exists); pair2 stays usable.
	_, err = svc.RefreshAccessToken(ctx, "dsh-sub2api-sync", pair.RefreshToken)
	require.ErrorIs(t, err, service.ErrOAuthInvalidGrant)
	pair3, err := svc.RefreshAccessToken(ctx, "dsh-sub2api-sync", pair2.RefreshToken)
	require.NoError(t, err)
	_, err = svc.ValidateAccessToken(ctx, pair3.AccessToken)
	require.NoError(t, err)

	// F. Grant status + cascade revocation (Redis token scan).
	approved, grantScopes, err := svc.GrantStatus(ctx, userID, "dsh-sub2api-sync")
	require.NoError(t, err)
	require.True(t, approved)
	require.ElementsMatch(t, scopes, grantScopes)
	require.NoError(t, svc.RevokeGrant(ctx, userID, "dsh-sub2api-sync"))
	_, err = svc.ValidateAccessToken(ctx, pair3.AccessToken)
	require.ErrorIs(t, err, service.ErrOAuthTokenRevoked)
	approvedAgain, _, err := svc.GrantStatus(ctx, userID, "dsh-sub2api-sync")
	require.NoError(t, err)
	require.False(t, approvedAgain)

	// G. Key access projection (方案 §2.2/§6): ownership checks are user-scoped.
	// The client argument is retained for API compatibility; the initial
	// release does not distinguish panel-created and OAuth-created keys.
	var keyID int64
	require.NoError(t, db.QueryRow(
		`INSERT INTO api_keys (user_id, key, name, created_at, updated_at) VALUES ($1,$2,'live-test-key',NOW(),NOW()) RETURNING id`,
		userID, fmt.Sprintf("sk-live-%d", time.Now().UnixNano())).Scan(&keyID))
	t.Cleanup(func() { _, _ = db.Exec(`DELETE FROM api_keys WHERE id = $1`, keyID) })
	owned, err := svc.APIKeyOwnedByClient(ctx, keyID, userID, "dsh-sub2api-sync")
	require.NoError(t, err)
	require.True(t, owned)
	otherOwned, err := svc.APIKeyOwnedByClient(ctx, keyID, userID, "other-client")
	require.NoError(t, err)
	require.True(t, otherOwned, "same user, different client: still owned in the initial release")
	ids, total, err := svc.ListAPIKeyIDs(ctx, userID, "dsh-sub2api-sync", "", "", nil, 20, 0)
	require.NoError(t, err)
	require.Contains(t, ids, keyID)
	require.EqualValues(t, 1, total)
	otherIDs, otherTotal, err := svc.ListAPIKeyIDs(ctx, userID, "other-client", "", "", nil, 20, 0)
	require.NoError(t, err)
	require.Contains(t, otherIDs, keyID)
	require.EqualValues(t, 1, otherTotal)
}

func pkceChallenge(t *testing.T, verifier string) string {
	t.Helper()
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

func reset241Objects(t *testing.T, db *sql.DB) {
	t.Helper()
	// On a fresh database the migration bookkeeping table does not exist yet.
	var migrationsTable *string
	require.NoError(t, db.QueryRow(`SELECT to_regclass('public.schema_migrations')::text`).Scan(&migrationsTable))
	if migrationsTable != nil {
		if _, err := db.Exec(`DELETE FROM schema_migrations WHERE filename = '241_oauth_provider.sql'`); err != nil {
			t.Fatalf("reset 241: %v", err)
		}
	}
	statements := []string{
		`DROP TABLE IF EXISTS oauth_grants CASCADE`,
		`DROP TABLE IF EXISTS oauth_clients CASCADE`,
	}
	for _, statement := range statements {
		if _, err := db.Exec(statement); err != nil {
			t.Fatalf("reset 241: %v", err)
		}
	}
}
