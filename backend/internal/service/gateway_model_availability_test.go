//go:build unit

package service

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestDiagnoseModelAvailabilityForPlatform_NoModel_AlwaysAvailable(t *testing.T) {
	repo := &mockAccountRepoForPlatform{accounts: nil, accountsByID: map[int64]*Account{}}
	svc := &GatewayService{accountRepo: repo, cfg: testConfig()}

	diag := svc.DiagnoseModelAvailabilityForPlatform(context.Background(), nil, "", PlatformOpenAI)

	require.True(t, diag.HasAccountsInPool, "empty model must return HasAccountsInPool=true so caller stays on 503")
	require.True(t, diag.HasModelSupport, "empty model must return HasModelSupport=true so caller stays on 503")
}

func TestDiagnoseModelAvailabilityForPlatform_EmptyPlatform_AlwaysAvailable(t *testing.T) {
	repo := &mockAccountRepoForPlatform{accounts: nil, accountsByID: map[int64]*Account{}}
	svc := &GatewayService{accountRepo: repo, cfg: testConfig()}

	diag := svc.DiagnoseModelAvailabilityForPlatform(context.Background(), nil, "gpt-5", "")

	require.True(t, diag.HasAccountsInPool)
	require.True(t, diag.HasModelSupport, "empty platform must fall back to {true,true} so caller stays on 503")
}

func TestDiagnoseModelAvailabilityForPlatform_NilReceiver(t *testing.T) {
	var svc *GatewayService

	diag := svc.DiagnoseModelAvailabilityForPlatform(context.Background(), nil, "gpt-5", PlatformOpenAI)

	require.True(t, diag.HasAccountsInPool)
	require.True(t, diag.HasModelSupport)
}

func TestDiagnoseModelAvailabilityForPlatform_NoAccountsInPool(t *testing.T) {
	repo := &mockAccountRepoForPlatform{accounts: nil, accountsByID: map[int64]*Account{}}
	svc := &GatewayService{accountRepo: repo, cfg: testConfig()}

	diag := svc.DiagnoseModelAvailabilityForPlatform(context.Background(), nil, "gpt-5", PlatformOpenAI)

	require.False(t, diag.HasAccountsInPool)
	require.False(t, diag.HasModelSupport, "no accounts means no support; caller stays on 503 (empty-pool branch)")
}

func TestDiagnoseModelAvailabilityForPlatform_ExplicitMappingMatches(t *testing.T) {
	repo := &mockAccountRepoForPlatform{
		accounts: []Account{
			{
				ID:          1,
				Platform:    PlatformOpenAI,
				Status:      StatusActive,
				Schedulable: true,
				Credentials: map[string]any{
					"model_mapping": map[string]any{"gpt-5.1-codex-mini": "gpt-5.1-codex-mini"},
				},
			},
		},
		accountsByID: map[int64]*Account{},
	}
	for i := range repo.accounts {
		repo.accountsByID[repo.accounts[i].ID] = &repo.accounts[i]
	}
	svc := &GatewayService{accountRepo: repo, cfg: testConfig()}

	diag := svc.DiagnoseModelAvailabilityForPlatform(context.Background(), nil, "gpt-5.1-codex-mini", PlatformOpenAI)

	require.True(t, diag.HasAccountsInPool)
	require.True(t, diag.HasModelSupport)
}

func TestDiagnoseModelAvailabilityForPlatform_EmptyMappingAllowsAll(t *testing.T) {
	repo := &mockAccountRepoForPlatform{
		accounts: []Account{
			{ID: 1, Platform: PlatformOpenAI, Status: StatusActive, Schedulable: true /* no ModelMapping = allow all */},
		},
		accountsByID: map[int64]*Account{},
	}
	for i := range repo.accounts {
		repo.accountsByID[repo.accounts[i].ID] = &repo.accounts[i]
	}
	svc := &GatewayService{accountRepo: repo, cfg: testConfig()}

	diag := svc.DiagnoseModelAvailabilityForPlatform(context.Background(), nil, "gpt-5.1-codex-mini", PlatformOpenAI)

	require.True(t, diag.HasModelSupport, "empty model_mapping must be treated as 'allow all' (Account.IsModelSupported semantics)")
}

func TestDiagnoseModelAvailabilityForPlatform_WildcardMappingMatches(t *testing.T) {
	repo := &mockAccountRepoForPlatform{
		accounts: []Account{
			{
				ID:          1,
				Platform:    PlatformOpenAI,
				Status:      StatusActive,
				Schedulable: true,
				Credentials: map[string]any{
					"model_mapping": map[string]any{"*": "gpt-5"},
				},
			},
		},
		accountsByID: map[int64]*Account{},
	}
	for i := range repo.accounts {
		repo.accountsByID[repo.accounts[i].ID] = &repo.accounts[i]
	}
	svc := &GatewayService{accountRepo: repo, cfg: testConfig()}

	diag := svc.DiagnoseModelAvailabilityForPlatform(context.Background(), nil, "gpt-5.1-codex-mini", PlatformOpenAI)

	require.True(t, diag.HasModelSupport, "wildcard mapping must classify the request as 'serviceable'")
}

func TestDiagnoseModelAvailabilityForPlatform_NoMatchingModel_ReturnsNotFoundSignal(t *testing.T) {
	groupID := int64(42)
	repo := &mockAccountRepoForPlatform{
		accounts: []Account{
			{
				ID:          1,
				Platform:    PlatformOpenAI,
				Status:      StatusActive,
				Schedulable: true,
				AccountGroups: []AccountGroup{
					{GroupID: groupID},
				},
				Credentials: map[string]any{"model_mapping": map[string]any{"gpt-5": "gpt-5"}},
			},
			{
				ID:          2,
				Platform:    PlatformOpenAI,
				Status:      StatusActive,
				Schedulable: true,
				AccountGroups: []AccountGroup{
					{GroupID: groupID},
				},
				Credentials: map[string]any{"model_mapping": map[string]any{"gpt-5-mini": "gpt-5-mini"}},
			},
		},
		accountsByID: map[int64]*Account{},
	}
	for i := range repo.accounts {
		repo.accountsByID[repo.accounts[i].ID] = &repo.accounts[i]
	}
	svc := &GatewayService{accountRepo: repo, cfg: testConfig()}

	diag := svc.DiagnoseModelAvailabilityForPlatform(context.Background(), &groupID, "gpt-5.1-codex-mini", PlatformOpenAI)

	require.True(t, diag.HasAccountsInPool, "group has OpenAI accounts")
	require.False(t, diag.HasModelSupport, "no account mapping admits the requested model — handler should return 404")
}

func TestDiagnoseModelAvailabilityForPlatform_RateLimitedSupportingAccountRemainsConfigured(t *testing.T) {
	groupID := int64(42)
	cooldownUntil := time.Now().Add(time.Hour)
	repo := &mockAccountRepoForPlatform{
		accounts: []Account{
			{
				ID:                     1,
				Platform:               PlatformAnthropic,
				Status:                 StatusActive,
				Schedulable:            true,
				RateLimitResetAt:       &cooldownUntil,
				OverloadUntil:          &cooldownUntil,
				TempUnschedulableUntil: &cooldownUntil,
				AccountGroups:          []AccountGroup{{GroupID: groupID}},
				Credentials: map[string]any{
					"model_mapping": map[string]any{"claude-opus-4-8": "claude-opus-4-8"},
				},
			},
		},
		accountsByID: map[int64]*Account{},
	}
	require.False(t, repo.accounts[0].IsSchedulable(), "test account must be excluded from normal scheduling while cooling down")
	svc := &GatewayService{
		accountRepo:       repo,
		cfg:               testConfig(),
		schedulerSnapshot: &SchedulerSnapshotService{}, // diagnosis must bypass the transient-only snapshot
	}

	diag := svc.DiagnoseModelAvailabilityForPlatform(context.Background(), &groupID, "claude-opus-4-8", PlatformAnthropic)

	require.True(t, diag.HasAccountsInPool)
	require.True(t, diag.HasModelSupport, "a configured model remains supported while every matching account is temporarily cooling down")
	require.False(t, diag.AllMatchingAccountsRateLimited, "an account with overload and temp-unschedulable blockers is not limited solely by rate limit")
}

func TestDiagnoseModelAvailabilityForPlatform_AllMatchingAnthropicAccountsRateLimited(t *testing.T) {
	groupID := int64(44)
	now := time.Now()
	firstReset := now.Add(30 * time.Minute)
	secondReset := now.Add(90 * time.Minute)
	repo := &mockAccountRepoForPlatform{
		accounts: []Account{
			{
				ID:               1,
				Platform:         PlatformAnthropic,
				Type:             AccountTypeAPIKey,
				Status:           StatusActive,
				Schedulable:      true,
				RateLimitResetAt: &secondReset,
				AccountGroups:    []AccountGroup{{GroupID: groupID}},
				Credentials:      map[string]any{"model_mapping": map[string]any{"claude-sonnet-4-5": "claude-sonnet-4-5"}},
			},
			{
				ID:               2,
				Platform:         PlatformAnthropic,
				Type:             AccountTypeAPIKey,
				Status:           StatusActive,
				Schedulable:      true,
				RateLimitResetAt: &firstReset,
				AccountGroups:    []AccountGroup{{GroupID: groupID}},
				Credentials:      map[string]any{"model_mapping": map[string]any{"claude-sonnet-4-5": "claude-sonnet-4-5"}},
			},
		},
		accountsByID: map[int64]*Account{},
	}
	svc := &GatewayService{accountRepo: repo, cfg: testConfig()}

	diag := svc.DiagnoseModelAvailabilityForPlatform(context.Background(), &groupID, "claude-sonnet-4-5", PlatformAnthropic)

	require.True(t, diag.HasAccountsInPool)
	require.True(t, diag.HasModelSupport)
	require.True(t, diag.AllMatchingAccountsRateLimited)
	require.NotNil(t, diag.MinRateLimitResetAt)
	require.WithinDuration(t, firstReset, *diag.MinRateLimitResetAt, time.Second)
}

func TestDiagnoseModelAvailabilityForPlatform_MixedRateLimitAndOverloadStays503(t *testing.T) {
	groupID := int64(45)
	now := time.Now()
	resetAt := now.Add(30 * time.Minute)
	overloadUntil := now.Add(10 * time.Minute)
	repo := &mockAccountRepoForPlatform{
		accounts: []Account{
			{
				ID:               1,
				Platform:         PlatformAnthropic,
				Type:             AccountTypeAPIKey,
				Status:           StatusActive,
				Schedulable:      true,
				RateLimitResetAt: &resetAt,
				AccountGroups:    []AccountGroup{{GroupID: groupID}},
			},
			{
				ID:            2,
				Platform:      PlatformAnthropic,
				Type:          AccountTypeAPIKey,
				Status:        StatusActive,
				Schedulable:   true,
				OverloadUntil: &overloadUntil,
				AccountGroups: []AccountGroup{{GroupID: groupID}},
			},
		},
		accountsByID: map[int64]*Account{},
	}
	svc := &GatewayService{accountRepo: repo, cfg: testConfig()}

	diag := svc.DiagnoseModelAvailabilityForPlatform(context.Background(), &groupID, "claude-sonnet-4-5", PlatformAnthropic)

	require.True(t, diag.HasModelSupport)
	require.False(t, diag.AllMatchingAccountsRateLimited)
	require.Nil(t, diag.MinRateLimitResetAt)
}

func TestOpenAIDiagnoseModelAvailabilityForPlatform_RateLimitedSupportingAccountRemainsConfigured(t *testing.T) {
	groupID := int64(43)
	cooldownUntil := time.Now().Add(time.Hour)
	repo := &mockAccountRepoForPlatform{
		accounts: []Account{
			{
				ID:                     2,
				Platform:               PlatformOpenAI,
				Status:                 StatusActive,
				Schedulable:            true,
				RateLimitResetAt:       &cooldownUntil,
				OverloadUntil:          &cooldownUntil,
				TempUnschedulableUntil: &cooldownUntil,
				AccountGroups:          []AccountGroup{{GroupID: groupID}},
				Credentials: map[string]any{
					"model_mapping": map[string]any{"claude-opus-4-8": "claude-opus-4-8"},
				},
			},
		},
		accountsByID: map[int64]*Account{},
	}
	require.False(t, repo.accounts[0].IsSchedulable(), "test account must be excluded from normal scheduling while cooling down")
	svc := &OpenAIGatewayService{
		accountRepo:       repo,
		cfg:               testConfig(),
		schedulerSnapshot: &SchedulerSnapshotService{}, // diagnosis must bypass the transient-only snapshot
	}

	diag := svc.DiagnoseModelAvailabilityForPlatform(context.Background(), &groupID, "claude-opus-4-8", PlatformOpenAI)

	require.True(t, diag.HasAccountsInPool)
	require.True(t, diag.HasModelSupport, "OpenAI-compatible diagnosis must keep transiently limited supporting accounts in the configured pool")
	require.False(t, diag.AllMatchingAccountsRateLimited, "an account with overload and temp-unschedulable blockers is not limited solely by rate limit")
}

func TestCompositeModelAvailabilityDiagnosis_IsolatesTargetPlatformPools(t *testing.T) {
	groupID := int64(46)
	now := time.Now()
	anthropicReset := now.Add(20 * time.Minute)
	openAIReset := now.Add(40 * time.Minute)

	newServices := func(accounts []Account) (*GatewayService, *OpenAIGatewayService) {
		repo := &mockAccountRepoForPlatform{accounts: accounts, accountsByID: map[int64]*Account{}}
		return &GatewayService{accountRepo: repo, cfg: testConfig()}, &OpenAIGatewayService{accountRepo: repo, cfg: testConfig()}
	}
	newAccount := func(id int64, platform, model string, resetAt *time.Time) Account {
		return Account{
			ID:               id,
			Platform:         platform,
			Type:             AccountTypeAPIKey,
			Status:           StatusActive,
			Schedulable:      true,
			RateLimitResetAt: resetAt,
			AccountGroups:    []AccountGroup{{GroupID: groupID}},
			Credentials:      map[string]any{"model_mapping": map[string]any{model: model}},
		}
	}

	t.Run("Anthropic pool limited while OpenAI pool remains available", func(t *testing.T) {
		anthropicSvc, openAISvc := newServices([]Account{
			newAccount(1, PlatformAnthropic, "claude-sonnet-4-5", &anthropicReset),
			newAccount(2, PlatformAnthropic, "claude-sonnet-4-5", &anthropicReset),
			newAccount(3, PlatformOpenAI, "gpt-5.1", nil),
			newAccount(4, PlatformOpenAI, "gpt-5.1", nil),
		})

		anthropicDiag := anthropicSvc.DiagnoseModelAvailabilityForPlatform(context.Background(), &groupID, "claude-sonnet-4-5", PlatformAnthropic)
		openAIDiag := openAISvc.DiagnoseModelAvailabilityForPlatform(context.Background(), &groupID, "gpt-5.1", PlatformOpenAI)

		require.True(t, anthropicDiag.AllMatchingAccountsRateLimited)
		require.NotNil(t, anthropicDiag.MinRateLimitResetAt)
		require.WithinDuration(t, anthropicReset, *anthropicDiag.MinRateLimitResetAt, time.Second)
		require.True(t, openAIDiag.HasModelSupport)
		require.False(t, openAIDiag.AllMatchingAccountsRateLimited)
	})

	t.Run("OpenAI pool limited while Anthropic pool remains available", func(t *testing.T) {
		anthropicSvc, openAISvc := newServices([]Account{
			newAccount(1, PlatformAnthropic, "claude-sonnet-4-5", nil),
			newAccount(2, PlatformAnthropic, "claude-sonnet-4-5", nil),
			newAccount(3, PlatformOpenAI, "gpt-5.1", &openAIReset),
			newAccount(4, PlatformOpenAI, "gpt-5.1", &openAIReset),
		})

		anthropicDiag := anthropicSvc.DiagnoseModelAvailabilityForPlatform(context.Background(), &groupID, "claude-sonnet-4-5", PlatformAnthropic)
		openAIDiag := openAISvc.DiagnoseModelAvailabilityForPlatform(context.Background(), &groupID, "gpt-5.1", PlatformOpenAI)

		require.True(t, anthropicDiag.HasModelSupport)
		require.False(t, anthropicDiag.AllMatchingAccountsRateLimited)
		require.True(t, openAIDiag.AllMatchingAccountsRateLimited)
		require.NotNil(t, openAIDiag.MinRateLimitResetAt)
		require.WithinDuration(t, openAIReset, *openAIDiag.MinRateLimitResetAt, time.Second)
	})
}

func TestAccountOnlyRateLimitedUntil_UsesLatestLimitAndRejectsOtherBlockers(t *testing.T) {
	now := time.Now()
	accountReset := now.Add(20 * time.Minute)
	modelReset := now.Add(40 * time.Minute).UTC().Truncate(time.Second)
	account := &Account{
		Type:             AccountTypeAPIKey,
		RateLimitResetAt: &accountReset,
		Extra: map[string]any{
			"model_rate_limits": map[string]any{
				"claude-sonnet-4-5": map[string]any{"rate_limit_reset_at": modelReset.Format(time.RFC3339)},
			},
		},
	}

	resetAt, onlyRateLimited := accountOnlyRateLimitedUntil(context.Background(), account, "claude-sonnet-4-5", now)
	require.True(t, onlyRateLimited)
	require.WithinDuration(t, modelReset, resetAt, time.Second, "the account becomes usable only after both active limits reset")

	overloadUntil := now.Add(5 * time.Minute)
	account.OverloadUntil = &overloadUntil
	_, onlyRateLimited = accountOnlyRateLimitedUntil(context.Background(), account, "claude-sonnet-4-5", now)
	require.False(t, onlyRateLimited)
	account.OverloadUntil = nil

	tempUntil := now.Add(5 * time.Minute)
	account.TempUnschedulableUntil = &tempUntil
	_, onlyRateLimited = accountOnlyRateLimitedUntil(context.Background(), account, "claude-sonnet-4-5", now)
	require.False(t, onlyRateLimited)
	account.TempUnschedulableUntil = nil

	expiredAt := now.Add(-time.Minute)
	account.AutoPauseOnExpired = true
	account.ExpiresAt = &expiredAt
	_, onlyRateLimited = accountOnlyRateLimitedUntil(context.Background(), account, "claude-sonnet-4-5", now)
	require.False(t, onlyRateLimited)
	account.AutoPauseOnExpired = false

	account.Extra["quota_limit"] = 100.0
	account.Extra["quota_used"] = 100.0
	_, onlyRateLimited = accountOnlyRateLimitedUntil(context.Background(), account, "claude-sonnet-4-5", now)
	require.False(t, onlyRateLimited)
}

func TestDiagnoseModelAvailabilityForPlatform_WrongPlatformFiltersOut(t *testing.T) {
	// Group has only Anthropic accounts; user routes to OpenAI gateway.
	// Diagnosis must NOT see Anthropic accounts (listSchedulableAccounts filters
	// by platform), so HasAccountsInPool is false and the caller stays on 503.
	repo := &mockAccountRepoForPlatform{
		accounts: []Account{
			{
				ID:          1,
				Platform:    PlatformAnthropic,
				Status:      StatusActive,
				Schedulable: true,
				Credentials: map[string]any{"model_mapping": map[string]any{"claude-sonnet-4-5": "claude-sonnet-4-5"}},
			},
		},
		accountsByID: map[int64]*Account{},
	}
	for i := range repo.accounts {
		repo.accountsByID[repo.accounts[i].ID] = &repo.accounts[i]
	}
	svc := &GatewayService{accountRepo: repo, cfg: testConfig()}

	diag := svc.DiagnoseModelAvailabilityForPlatform(context.Background(), nil, "gpt-5", PlatformOpenAI)

	require.False(t, diag.HasAccountsInPool, "OpenAI route must not see Anthropic accounts in pool")
	require.False(t, diag.HasModelSupport)
}
