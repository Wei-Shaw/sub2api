package service

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/antigravity"
	"github.com/Wei-Shaw/sub2api/internal/pkg/geminicli"
	"github.com/Wei-Shaw/sub2api/internal/pkg/oauth"
	"github.com/Wei-Shaw/sub2api/internal/pkg/openai"
	"github.com/Wei-Shaw/sub2api/internal/pkg/xai"
	"github.com/stretchr/testify/require"
)

func TestRelayOAuthSessionsCannotCrossOwnersOrAdminScope(t *testing.T) {
	claudeStore := oauth.NewSessionStore()
	openaiStore := openai.NewSessionStore()
	geminiStore := geminicli.NewSessionStore()
	antigravityStore := antigravity.NewSessionStore()
	grokStore := xai.NewSessionStore()
	t.Cleanup(claudeStore.Stop)
	t.Cleanup(openaiStore.Stop)
	t.Cleanup(geminiStore.Stop)
	t.Cleanup(antigravityStore.Stop)
	t.Cleanup(grokStore.Stop)

	for _, owner := range []int64{0, 7} {
		claudeStore.Set("session", &oauth.OAuthSession{OwnerUserID: owner, CreatedAt: time.Now()})
		openaiStore.Set("session", &openai.OAuthSession{OwnerUserID: owner, CreatedAt: time.Now()})
		geminiStore.Set("session", &geminicli.OAuthSession{OwnerUserID: owner, CreatedAt: time.Now()})
		antigravityStore.Set("session", &antigravity.OAuthSession{OwnerUserID: owner, CreatedAt: time.Now()})
		grokStore.Set("session", &xai.OAuthSession{OwnerUserID: owner, CreatedAt: time.Now()})
		for _, caller := range []int64{0, 7, 8} {
			if caller == owner {
				continue
			}
			ctx := WithRelayOwner(context.Background(), caller)
			// No HTTP clients are configured: rejection must happen before a token request.
			_, err := (&OAuthService{sessionStore: claudeStore}).ExchangeCode(ctx, &ExchangeCodeInput{SessionID: "session"})
			require.ErrorIs(t, err, ErrGroupNotAllowed, "claude owner=%d caller=%d", owner, caller)
			_, err = (&OpenAIOAuthService{sessionStore: openaiStore}).ExchangeCode(ctx, &OpenAIExchangeCodeInput{SessionID: "session"})
			require.ErrorIs(t, err, ErrGroupNotAllowed, "openai owner=%d caller=%d", owner, caller)
			_, err = (&GeminiOAuthService{sessionStore: geminiStore}).ExchangeCode(ctx, &GeminiExchangeCodeInput{SessionID: "session"})
			require.ErrorIs(t, err, ErrGroupNotAllowed, "gemini owner=%d caller=%d", owner, caller)
			_, err = (&AntigravityOAuthService{sessionStore: antigravityStore}).ExchangeCode(ctx, &AntigravityExchangeCodeInput{SessionID: "session"})
			require.ErrorIs(t, err, ErrGroupNotAllowed, "antigravity owner=%d caller=%d", owner, caller)
			_, err = (&GrokOAuthService{sessionStore: grokStore}).ExchangeCode(ctx, &GrokExchangeCodeInput{SessionID: "session"})
			require.ErrorIs(t, err, ErrGroupNotAllowed, "grok owner=%d caller=%d", owner, caller)
		}
		_, ok := claudeStore.Get("session")
		require.True(t, ok)
		_, ok = openaiStore.Get("session")
		require.True(t, ok)
		_, ok = geminiStore.Get("session")
		require.True(t, ok)
		_, ok = antigravityStore.Get("session")
		require.True(t, ok)
		_, ok = grokStore.Get("session")
		require.True(t, ok)
	}
}

func TestRelayAccountControlsPreserveProtocolsButRejectAdminFields(t *testing.T) {
	require.NoError(t, ValidateRelayCredentials(AccountTypeOAuth, map[string]any{
		"access_token": "token", "chatgpt_account_id": "workspace", "client_id": "client",
		"header_override_enabled": true, "header_overrides": map[string]any{"x-custom": "value"},
		"model_mapping": map[string]any{"alias": "model"},
	}))
	extra, err := SanitizeRelayAccountExtra(map[string]any{
		"openai_passthrough": true, "openai_responses_mode": "chat",
		"parent_account_id": 99, "tls_fingerprint_profile_id": 3,
		"upstream_billing_rate_sync_enabled": true,
	})
	require.NoError(t, err)
	require.Equal(t, map[string]any{"openai_passthrough": true, "openai_responses_mode": "chat"}, extra)
	_, err = SanitizeRelayAccountExtra(map[string]any{"custom_base_url": "http://127.0.0.1"})
	require.Error(t, err)
}

type relayScopeGroupRepo struct{ GroupRepository }

func (relayScopeGroupRepo) GetByID(context.Context, int64) (*Group, error) {
	return &Group{ID: 1, OwnerUserID: relayTestID(7), Platform: PlatformComposite, Status: StatusActive}, nil
}

type relayScopeUserRepo struct{ UserRepository }

func (relayScopeUserRepo) GetByID(context.Context, int64) (*User, error) {
	return &User{ID: 7}, nil
}

func TestRelaySharedCreateSupportsAdminCredentialFormsAndForcesOwnership(t *testing.T) {
	s := &RelayService{groups: relayScopeGroupRepo{}, users: relayScopeUserRepo{}}
	for _, input := range []*CreateAccountInput{
		{Platform: PlatformAnthropic, Type: AccountTypeSetupToken, Credentials: map[string]any{"access_token": "token", "expires_in": 3600, "org_uuid": "org"}},
		{Platform: PlatformAnthropic, Type: AccountTypeBedrock, Credentials: map[string]any{"auth_mode": "sigv4", "aws_access_key_id": "key", "aws_secret_access_key": "secret", "aws_region": "us-east-1"}},
		{Platform: PlatformGemini, Type: AccountTypeServiceAccount, Credentials: map[string]any{"service_account_json": `{"client_email":"test@example.com","private_key":"own-key","project_id":"project"}`, "location": "global"}},
	} {
		input.Name = "own account"
		input.Concurrency = 3
		input.GroupIDs = []int64{1}
		input.OwnerUserID = relayTestID(8)
		input.ProxyID = relayTestID(9)
		rate := 5.0
		input.RateMultiplier = &rate
		require.NoError(t, s.prepareCreateAccount(context.Background(), 7, input))
		require.Equal(t, int64(7), *input.OwnerUserID)
		require.Nil(t, input.ProxyID)
		require.Nil(t, input.RateMultiplier)
		require.True(t, input.SkipDefaultGroupBind)
	}
}

func TestRelayVertexTokenCacheCannotReuseAnotherOwnersCredentials(t *testing.T) {
	key := &vertexServiceAccountKey{ClientEmail: "same@example.com", PrivateKeyID: "same-key"}
	admin := vertexServiceAccountCacheKey(&Account{ID: 1}, key)
	first := vertexServiceAccountCacheKey(&Account{ID: 2, OwnerUserID: relayTestID(7)}, key)
	second := vertexServiceAccountCacheKey(&Account{ID: 3, OwnerUserID: relayTestID(8)}, key)
	require.NotEqual(t, admin, first)
	require.NotEqual(t, first, second)
}

func TestRelayEditPreservesRuntimeStateButCanRemoveEditableControls(t *testing.T) {
	a := &Account{Type: AccountTypeAPIKey, Extra: map[string]any{
		"model_rate_limits": map[string]any{"model": "cooldown"},
		"quota_used":        12.0, "openai_passthrough": true, "quota_daily_limit": 20.0,
	}}
	input := &UpdateAccountInput{Extra: map[string]any{"quota_used": 0.0, "quota_daily_limit": 30.0, "tls_fingerprint_profile_id": 99}}
	require.NoError(t, (&RelayService{}).prepareUpdateAccount(context.Background(), 7, a, input))
	require.Equal(t, 12.0, input.Extra["quota_used"])
	require.Equal(t, a.Extra["model_rate_limits"], input.Extra["model_rate_limits"])
	require.Equal(t, 30.0, input.Extra["quota_daily_limit"])
	require.NotContains(t, input.Extra, "openai_passthrough")
	require.NotContains(t, input.Extra, "tls_fingerprint_profile_id")
}

func TestRelayEditCanDisableCredentialControlsWithoutLosingSecrets(t *testing.T) {
	a := &Account{Type: AccountTypeAPIKey, Credentials: map[string]any{"api_key": "keep", "pool_mode": true, "model_mapping": map[string]any{"old": "model"}}}
	input := &UpdateAccountInput{Credentials: map[string]any{"base_url": "https://api.example.com"}}
	require.NoError(t, (&RelayService{}).prepareUpdateAccount(context.Background(), 7, a, input))
	require.Equal(t, "keep", input.Credentials["api_key"])
	require.NotContains(t, input.Credentials, "pool_mode")
	require.NotContains(t, input.Credentials, "model_mapping")
}

func TestRelayReauthorizationCanPatchTokensWithoutChangingGroupsOrName(t *testing.T) {
	a := &Account{ID: 1, Name: "saved", Type: AccountTypeOAuth, Credentials: map[string]any{"access_token": "old", "refresh_token": "keep"}}
	input := &UpdateAccountInput{Credentials: map[string]any{"access_token": "new"}, ProxyID: relayTestID(3), Priority: new(int)}
	require.NoError(t, (&RelayService{}).prepareUpdateAccount(context.Background(), 7, a, input))
	require.Equal(t, "new", input.Credentials["access_token"])
	require.Equal(t, "keep", input.Credentials["refresh_token"])
	require.Nil(t, input.GroupIDs)
	require.Empty(t, input.Name)
	require.Nil(t, input.Priority)
	require.Nil(t, input.ProxyID)
}
