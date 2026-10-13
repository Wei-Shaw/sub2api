package service

import (
	"context"
	"encoding/json"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/stretchr/testify/require"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

type automationTestRepo struct {
	AccountVaultRotationRepository
	AccountVaultAutomationRepository
	settings VaultAutomationSettings
	health   VaultAccountHealth
	claimed  string
	queues   int
}

func (r *automationTestRepo) LinkedAutomationAccounts(context.Context) ([]VaultAutomationAccount, error) {
	return []VaultAutomationAccount{{AccountID: 7, GatewayAccountID: 100}}, nil
}
func (r *automationTestRepo) AutomationHealth(context.Context, []int64) ([]VaultAccountHealth, error) {
	return []VaultAccountHealth{r.health}, nil
}
func (r *automationTestRepo) SetConcurrency(context.Context, int64, int) error { return nil }

func TestVaultManualRecoveryRefreshesExpiryAndQueuesOnlyTerminalRejections(t *testing.T) {
	for _, tc := range []struct {
		name                      string
		status                    int
		body                      string
		prior                     string
		expired                   bool
		queued, refreshed, failed int
	}{
		{"expired_but_valid", 200, `{"rate_limit":{"primary_window":{"used_percent":7,"limit_window_seconds":2592000,"reset_at":2000000000}}}`, "", true, 0, 1, 0},
		{"revoked", 401, `{"error":{"code":"token_revoked"}}`, "token_revoked", false, 1, 0, 0},
		{"automatic_limit_retry", 401, `{"error":{"code":"token_revoked"}}`, "automatic_attempt_limited", false, 1, 0, 0},
		{"expired_403", 403, `{"error":{"code":"token_revoked"}}`, "", true, 0, 0, 1},
		{"expired_429", 429, `{"error":{"code":"token_revoked"}}`, "", true, 0, 0, 1},
		{"expired_generic401", 401, `{"detail":"Unauthorized"}`, "", true, 0, 0, 1},
		{"healthy_is_untouched", 401, `{"error":{"code":"token_revoked"}}`, "", false, 0, 0, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			usage, account, _, calls := automationTestUsage(t, tc.status, tc.body)
			if tc.expired {
				account.Credentials["expires_at"] = time.Now().Add(-time.Hour).Format(time.RFC3339)
			}
			repo := &automationTestRepo{health: VaultAccountHealth{AccountID: 7, ErrorCode: tc.prior}}
			s, _, _ := rotationTestSetup(t)
			s.vault.keyVerified = true
			s.repo, s.users = repo, &automationTestUsers{}
			s.automation = &vaultAutomation{repo: repo, usage: usage, rotation: s}
			result, err := s.ReauthorizeInvalid(context.Background(), 9)
			require.NoError(t, err)
			require.Equal(t, tc.queued, repo.queues)
			require.Equal(t, tc.refreshed, result.Refreshed)
			require.Equal(t, tc.failed, result.Failed)
			if tc.name == "healthy_is_untouched" {
				require.Zero(t, *calls)
			}
		})
	}
}

func (r *automationTestRepo) AutomationSettings(context.Context) (*VaultAutomationSettings, error) {
	s := r.settings
	return &s, nil
}
func (r *automationTestRepo) SaveAutomationHealth(_ context.Context, h VaultAccountHealth) error {
	r.health = h
	return nil
}
func (r *automationTestRepo) ClaimReauthorization(_ context.Context, _ int64, hash string, _ time.Time) (bool, error) {
	if r.claimed == hash {
		return false, nil
	}
	r.claimed = hash
	return true, nil
}
func (r *automationTestRepo) QueueSession(_ context.Context, _, _ int64, id string, _ time.Time) (*AccountVaultRotationJob, string, error) {
	r.queues++
	return &AccountVaultRotationJob{ID: id, Status: "queued"}, "queued", nil
}

type automationTestUsers struct{ UserRepository }

func (r *automationTestUsers) GetByID(context.Context, int64) (*User, error) {
	return &User{ID: 9, Role: RoleAdmin, Status: StatusActive, TokenVersion: 1}, nil
}

func automationTestUsage(t *testing.T, status int, body string) (*AccountUsageService, *Account, *stubQuotaAccountRepo, *int) {
	t.Helper()
	account := &Account{ID: 100, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Credentials: map[string]any{"chatgpt_account_id": "synthetic-workspace", "access_token": "synthetic-token"}}
	repo := &stubQuotaAccountRepo{accounts: map[int64]*Account{100: account}}
	tokens := &stubQuotaTokenCache{tokens: map[string]string{OpenAITokenCacheKey(account): "synthetic-token"}}
	calls := new(int)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*calls++
		require.Equal(t, http.MethodGet, r.Method)
		require.Equal(t, "/backend-api/wham/usage", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	quota := NewOpenAIQuotaService(repo, nil, NewOpenAITokenProvider(repo, tokens, nil), newQuotaRedirectingFactory(srv), nil)
	return &AccountUsageService{accountRepo: repo, openAIQuotaService: quota}, account, repo, calls
}

func TestVaultAutomationQueuesOnceAndNeverOnNetworkOrPolicyFailures(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		body   string
		want   int
	}{
		{"revoked", 401, `{"error":{"code":"token_revoked","message":"synthetic-secret-canary"}}`, 1},
		{"generic401", 401, `{"detail":"Unauthorized"}`, 0},
		{"policy403", 403, `{"error":{"code":"token_revoked"}}`, 0},
		{"rate429", 429, `{"error":{"code":"token_revoked"}}`, 0},
		{"gateway502", 502, `{"error":{"code":"token_revoked"}}`, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			usage, _, _, _ := automationTestUsage(t, tc.status, tc.body)
			repo := &automationTestRepo{settings: VaultAutomationSettings{Enabled: true, ActorID: 9, TokenVersion: 1}}
			a := &vaultAutomation{repo: repo, usage: usage, rotation: &AccountVaultRotationService{repo: repo, users: &automationTestUsers{}}}
			v := VaultAutomationAccount{AccountID: 7, GatewayAccountID: 100}
			a.check(context.Background(), v, VaultAccountHealth{}, 9, true)
			a.check(context.Background(), v, VaultAccountHealth{}, 9, true)
			require.Equal(t, tc.want, repo.queues)
			require.Nil(t, repo.health.Quota)
			if tc.want == 1 {
				require.Equal(t, "manual_required", repo.health.Status)
				require.NotEmpty(t, repo.claimed)
			}
			raw, _ := json.Marshal(repo.health)
			require.NotContains(t, string(raw), "synthetic-secret-canary")
		})
	}
}
func TestVaultAutomationDisabledOrPausedDoesNotQueue(t *testing.T) {
	for _, paused := range []bool{false, true} {
		usage, _, _, calls := automationTestUsage(t, 401, `{"error":{"code":"token_revoked"}}`)
		repo := &automationTestRepo{settings: VaultAutomationSettings{Enabled: false, ActorID: 9, TokenVersion: 1}}
		a := &vaultAutomation{repo: repo, usage: usage, rotation: &AccountVaultRotationService{repo: repo, users: &automationTestUsers{}}}
		a.check(context.Background(), VaultAutomationAccount{AccountID: 7, GatewayAccountID: 100, PausedJob: paused}, VaultAccountHealth{}, 9, true)
		require.Zero(t, repo.queues)
		if paused {
			require.Zero(t, *calls)
			require.Equal(t, "manual_required", repo.health.Status)
		}
	}
}
func TestVaultLiveQuotaUsesActualWindowAndPreservesFailedSnapshot(t *testing.T) {
	usage, account, repo, calls := automationTestUsage(t, 200, `{"rate_limit":{"primary_window":{"used_percent":7,"limit_window_seconds":2592000,"reset_at":2000000000,"reset_after_seconds":300}}}`)
	q, err := usage.queryLiveQuota(context.Background(), 100)
	require.NoError(t, err)
	require.Equal(t, 1, *calls)
	info := usageFromLiveQuota(q, "live")
	require.Nil(t, info.SevenDay)
	require.Nil(t, info.FiveHour)
	require.NotNil(t, info.ThirtyDay)
	require.Equal(t, 7.0, info.ThirtyDay.Utilization)
	account.Extra = map[string]any{"openai_live_quota": q}
	cached, err := usage.getOpenAILiveUsage(context.Background(), account, false)
	require.NoError(t, err)
	require.Equal(t, "cached", cached.QuotaQueryStatus)
	require.Equal(t, q.FetchedAt, *cached.UpdatedAt)
	require.Equal(t, 1, *calls)
	require.NotContains(t, repo.extraUpdates[100], "access_token")
	failing, _, failedRepo, _ := automationTestUsage(t, 401, `{"error":{"code":"token_revoked"}}`)
	result, err := failing.getOpenAILiveUsage(context.Background(), account, true)
	require.Error(t, err)
	require.Nil(t, result)
	require.Equal(t, 502, infraerrors.Code(err), "upstream 401 must not log the administrator out")
	require.NotContains(t, failedRepo.extraUpdates[100], "openai_live_quota")
	transient, _, transientRepo, _ := automationTestUsage(t, 500, `{"detail":"temporary error"}`)
	stale, err := transient.getOpenAILiveUsage(context.Background(), account, true)
	require.NoError(t, err)
	require.Equal(t, "stale", stale.QuotaQueryStatus)
	require.Equal(t, q.FetchedAt, *stale.UpdatedAt)
	require.NotEmpty(t, stale.Error)
	require.NotContains(t, transientRepo.extraUpdates[100], "openai_live_quota")
}
func TestVaultAutomaticRefreshFailureClassification(t *testing.T) {
	for _, tc := range []struct{ message, want string }{
		{"refresh failed: invalid_grant", "invalid_grant"},
		{"refresh_token_reused", "refresh_token_reused"},
		{"openai access_token expired and refresh_token is missing", "expired_without_refresh_token"},
		{"invalid_client invalid_grant", ""},
		{"upstream timeout", ""},
	} {
		require.Equal(t, tc.want, revokedCredentialError(infraerrors.New(502, "OPENAI_QUOTA_TOKEN_UNAVAILABLE", tc.message)))
	}
}
