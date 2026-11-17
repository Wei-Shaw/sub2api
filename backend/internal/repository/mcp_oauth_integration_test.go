//go:build integration

package repository

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

const mcpIntegrationResource = "https://platform.example/api/v1/mcp"

type mcpOAuthIntegrationFixture struct {
	oauth   *service.MCPOAuthService
	repo    service.MCPOAuthRepository
	input   service.MCPOAuthTokenInput
	userID  int64
	grantID string
}

// Use committed rows and the production repositories: OAuth owns its database
// transactions, so wrapping this fixture in testTx would hide lock behavior.
func newMCPOAuthIntegrationFixture(t *testing.T, ctx context.Context) mcpOAuthIntegrationFixture {
	t.Helper()
	user := mustCreateUser(t, testEntClient(t), &service.User{})
	repo := NewMCPOAuthRepository(integrationEntClient, integrationDB)
	oauth := service.NewMCPOAuthService(repo, NewUserRepository(integrationEntClient, integrationDB))
	t.Cleanup(func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_, err := integrationDB.ExecContext(cleanupCtx, `DELETE FROM users WHERE id=$1`, user.ID)
		require.NoError(t, err)
	})

	const redirectURI = "https://client.example/callback?channel=desktop"
	client, err := oauth.Register(ctx, service.MCPOAuthRegisterInput{
		ClientName: "Integration MCP client", RedirectURIs: []string{redirectURI},
		TokenEndpointAuthMethod: "none",
	})
	require.NoError(t, err)
	clientID := client.ClientID

	verifier := strings.Repeat("v", 64)
	challenge := sha256.Sum256([]byte(verifier))
	authorization := service.MCPOAuthAuthorizeInput{
		ResponseType: "code", ClientID: clientID, RedirectURI: redirectURI,
		Scope: "mcp", State: "integration-state", Resource: mcpIntegrationResource,
		CodeChallenge: base64.RawURLEncoding.EncodeToString(challenge[:]), CodeChallengeMethod: "S256",
	}
	preview, err := oauth.Preview(ctx, user.ID, authorization, mcpIntegrationResource)
	require.NoError(t, err)
	require.Equal(t, client.ClientName, preview.ClientName)
	require.Equal(t, redirectURI, preview.RedirectURI)
	redirect, err := oauth.Decide(ctx, user.ID, service.MCPOAuthConsentInput{
		Authorization: authorization, Approved: true,
	}, mcpIntegrationResource)
	require.NoError(t, err)
	callback, err := url.Parse(redirect)
	require.NoError(t, err)
	require.Equal(t, "desktop", callback.Query().Get("channel"))
	require.Equal(t, authorization.State, callback.Query().Get("state"))
	require.NotEmpty(t, callback.Query().Get("code"))
	grants, err := oauth.ListGrants(ctx, user.ID)
	require.NoError(t, err)
	require.Len(t, grants, 1)
	require.Equal(t, service.MCPScopeUser, grants[0].Scope)
	require.True(t, grants[0].ReadOnly)
	require.False(t, grants[0].Invalidated)
	require.Equal(t, clientID, grants[0].ClientID)
	require.Equal(t, client.ClientName, grants[0].ClientName)
	return mcpOAuthIntegrationFixture{
		oauth: oauth, repo: repo, userID: user.ID, grantID: grants[0].ID,
		input: service.MCPOAuthTokenInput{
			GrantType: "authorization_code", ClientID: clientID, RedirectURI: redirectURI,
			Code: callback.Query().Get("code"), CodeVerifier: verifier, Resource: mcpIntegrationResource,
		},
	}
}

func mcpIntegrationDigest(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

func requireMCPIntegrationInvalidGrant(t *testing.T, err error) {
	t.Helper()
	var oauthErr *service.MCPOAuthError
	require.ErrorAs(t, err, &oauthErr)
	require.Equal(t, "invalid_grant", oauthErr.Code)
}

func TestMCPOAuthIntegrationAuthorizationAndRefreshReplay(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	f := newMCPOAuthIntegrationFixture(t, ctx)
	// TestMain applies the real SQL migrations before this end-to-end flow.
	issued, err := f.oauth.Token(ctx, f.input, mcpIntegrationResource)
	require.NoError(t, err)
	require.NotEmpty(t, issued.RefreshToken)
	principal, err := f.oauth.Authenticate(ctx, issued.AccessToken)
	require.NoError(t, err)
	require.Equal(t, f.userID, principal.UserID)
	require.Equal(t, f.grantID, principal.GrantID)
	code, err := f.repo.GetCode(ctx, mcpIntegrationDigest(f.input.Code))
	require.NoError(t, err)
	require.NotNil(t, code.ConsumedAt)
	refresh, err := f.repo.GetCredential(ctx, mcpIntegrationDigest(issued.RefreshToken))
	require.NoError(t, err)
	require.Nil(t, refresh.UsedAt)

	// SDK clients omit resource on refresh; it must inherit the original binding.
	refreshInput := service.MCPOAuthTokenInput{
		GrantType: "refresh_token", ClientID: f.input.ClientID, RefreshToken: issued.RefreshToken,
	}
	rotated, err := f.oauth.Token(ctx, refreshInput, mcpIntegrationResource)
	require.NoError(t, err)
	require.NotEqual(t, issued.RefreshToken, rotated.RefreshToken)
	principal, err = f.oauth.Authenticate(ctx, rotated.AccessToken)
	require.NoError(t, err)
	require.Equal(t, mcpIntegrationResource, principal.Resource)
	refresh, err = f.repo.GetCredential(ctx, mcpIntegrationDigest(issued.RefreshToken))
	require.NoError(t, err)
	require.NotNil(t, refresh.UsedAt, "old refresh digest must remain for replay detection")

	_, err = f.oauth.Token(ctx, refreshInput, mcpIntegrationResource)
	requireMCPIntegrationInvalidGrant(t, err)
	grant, err := f.repo.GetGrant(ctx, f.grantID)
	require.NoError(t, err)
	require.NotNil(t, grant.RevokedAt, "replay revocation must commit despite invalid_grant")
	for _, raw := range []string{issued.AccessToken, rotated.AccessToken} {
		_, err = f.oauth.Authenticate(ctx, raw)
		require.ErrorIs(t, err, service.ErrMCPOAuthInvalidToken)
	}
	refreshInput.RefreshToken = rotated.RefreshToken
	_, err = f.oauth.Token(ctx, refreshInput, mcpIntegrationResource)
	requireMCPIntegrationInvalidGrant(t, err)
}

func TestMCPOAuthIntegrationConcurrentCodeExchange(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	f := newMCPOAuthIntegrationFixture(t, ctx)
	const attempts = 8
	type exchangeResult struct {
		tokens *service.MCPOAuthTokenResponse
		err    error
	}
	start := make(chan struct{})
	results := make(chan exchangeResult, attempts)
	var workers sync.WaitGroup
	for range attempts {
		workers.Add(1)
		go func() {
			defer workers.Done()
			<-start
			tokens, err := f.oauth.Token(ctx, f.input, mcpIntegrationResource)
			results <- exchangeResult{tokens: tokens, err: err}
		}()
	}
	close(start)
	workers.Wait()
	close(results)
	var issued *service.MCPOAuthTokenResponse
	successes := 0
	for result := range results {
		if result.err != nil {
			requireMCPIntegrationInvalidGrant(t, result.err)
			continue
		}
		successes++
		issued = result.tokens
	}
	require.Equal(t, 1, successes, "one authorization code must issue only one credential pair")
	require.NotNil(t, issued)
	var credentialCount int
	require.NoError(t, integrationDB.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM mcp_oauth_credentials WHERE grant_id=$1`, f.grantID).Scan(&credentialCount))
	require.Equal(t, 2, credentialCount, "losing transactions must not persist extra access or refresh tokens")
	grant, err := f.repo.GetGrant(ctx, f.grantID)
	require.NoError(t, err)
	require.NotNil(t, grant.RevokedAt, "the concurrent code replay must revoke the winning issuance")
	_, err = f.oauth.Authenticate(ctx, issued.AccessToken)
	require.ErrorIs(t, err, service.ErrMCPOAuthInvalidToken)
	_, err = f.oauth.Token(ctx, service.MCPOAuthTokenInput{
		GrantType: "refresh_token", ClientID: f.input.ClientID, RefreshToken: issued.RefreshToken,
	}, mcpIntegrationResource)
	requireMCPIntegrationInvalidGrant(t, err)
}

func TestMCPOAuthIntegrationAccountChangesRevokeGrantsPersistently(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	f := newMCPOAuthIntegrationFixture(t, ctx)
	users := NewUserRepository(integrationEntClient, integrationDB)
	user, err := users.GetByID(ctx, f.userID)
	require.NoError(t, err)
	user.Role = service.RoleAdmin
	require.NoError(t, users.Update(ctx, user, service.UserUpdateFields{Role: true}))
	now := time.Now().UTC()
	adminGrant := &service.MCPOAuthGrant{
		ID: "admin-" + strings.Repeat("g", 20), ClientID: f.input.ClientID, UserID: f.userID, TokenVersion: 1,
		Resource: mcpIntegrationResource, Scope: service.MCPScopeAdmin, ReadOnly: true, CreatedAt: now, ExpiresAt: now.Add(time.Hour),
	}
	require.NoError(t, f.repo.CreateAuthorization(ctx, adminGrant, &service.MCPOAuthCode{
		Hash: mcpIntegrationDigest(adminGrant.ID), GrantID: adminGrant.ID, ClientID: f.input.ClientID, RedirectURI: f.input.RedirectURI,
		Resource: mcpIntegrationResource, CodeChallenge: strings.Repeat("c", 43), ExpiresAt: now.Add(time.Minute),
	}, service.MCPOAuthGrantLimit))

	requireRevoked := func(id string, revoked bool) {
		t.Helper()
		grant, err := f.repo.GetGrant(ctx, id)
		require.NoError(t, err)
		require.Equal(t, revoked, grant.RevokedAt != nil, id)
	}
	// Demotion revokes administrator grants only; personal access remains.
	user.Role = service.RoleUser
	require.NoError(t, users.Update(ctx, user, service.UserUpdateFields{Role: true}))
	requireRevoked(adminGrant.ID, true)
	requireRevoked(f.grantID, false)

	// Disabling revokes everything, and re-enabling must not revive it.
	user.Status = service.StatusDisabled
	require.NoError(t, users.Update(ctx, user, service.UserUpdateFields{Status: true}))
	user.Status = service.StatusActive
	require.NoError(t, users.Update(ctx, user, service.UserUpdateFields{Status: true}))
	requireRevoked(f.grantID, true)
	_, err = f.oauth.Token(ctx, f.input, mcpIntegrationResource)
	requireMCPIntegrationInvalidGrant(t, err)
}

func TestMCPOAuthIntegrationClientIDKeyConvergesAcrossInstances(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	// Start without a key so the instances race to create it.
	_, err := integrationDB.ExecContext(ctx, `DELETE FROM security_secrets WHERE key=$1`, mcpOAuthClientIDKeyName)
	require.NoError(t, err)
	const instances = 8
	keys := make([][]byte, instances)
	errs := make([]error, instances)
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := 0; i < instances; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			keys[i], errs[i] = NewMCPOAuthRepository(integrationEntClient, integrationDB).ClientIDKey(ctx)
		}(i)
	}
	close(start)
	wg.Wait()
	for i := 0; i < instances; i++ {
		require.NoError(t, errs[i])
		require.GreaterOrEqual(t, len(keys[i]), 32)
		require.Equal(t, keys[0], keys[i])
	}

	// A client registered by one instance is accepted by another.
	users := NewUserRepository(integrationEntClient, integrationDB)
	client, err := service.NewMCPOAuthService(NewMCPOAuthRepository(integrationEntClient, integrationDB), users).Register(ctx, service.MCPOAuthRegisterInput{RedirectURIs: []string{"https://client.example/cb"}})
	require.NoError(t, err)
	_, err = service.NewMCPOAuthService(NewMCPOAuthRepository(integrationEntClient, integrationDB), users).Token(ctx, service.MCPOAuthTokenInput{GrantType: "authorization_code", ClientID: client.ClientID}, mcpIntegrationResource)
	requireMCPIntegrationInvalidGrant(t, err)
}
