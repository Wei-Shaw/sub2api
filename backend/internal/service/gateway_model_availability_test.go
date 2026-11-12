//go:build unit

package service

import (
	"context"
	"errors"
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
}

func TestDiagnoseModelAvailabilityForPlatform_TracksAllSupportingAccountsBalanceState(t *testing.T) {
	groupID := int64(44)
	until := time.Now().Add(time.Hour)
	expired := time.Now().Add(-time.Hour)
	blocked := Account{
		ID: 1, Platform: PlatformDeepseek, Type: AccountTypeAPIKey,
		Status: StatusActive, Schedulable: true,
		AccountGroups:           []AccountGroup{{GroupID: groupID}},
		Credentials:             map[string]any{"model_mapping": map[string]any{"deepseek-flash": "deepseek-flash"}},
		Extra:                   map[string]any{cnExtraKey(PlatformDeepseek, cnBalanceExtraSuffixLow): true},
		TempUnschedulableUntil:  &until,
		TempUnschedulableReason: cnBalanceLowReason("insufficient balance"),
	}

	for _, tc := range []struct {
		name string
		new  func(AccountRepository) ModelAvailabilityDiagnoser
	}{
		{
			name: "gateway",
			new: func(repo AccountRepository) ModelAvailabilityDiagnoser {
				return &GatewayService{accountRepo: repo, cfg: testConfig()}
			},
		},
		{
			name: "openai-compatible",
			new: func(repo AccountRepository) ModelAvailabilityDiagnoser {
				return &OpenAIGatewayService{accountRepo: repo, cfg: testConfig()}
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, state := range []struct {
				name       string
				mutate     func(*Account)
				allLowWant bool
			}{
				{"all supporting accounts balance blocked", func(*Account) {}, true},
				{"block without cached balance", func(a *Account) { a.Extra = nil }, true},
				{"bare balance reason", func(a *Account) { a.TempUnschedulableReason = cnBalanceLowReasonPrefix }, true},
				{"recovered account with stale balance flag", func(a *Account) { a.TempUnschedulableUntil = nil }, false},
				{"expired block with stale balance flag", func(a *Account) { a.TempUnschedulableUntil = &expired }, false},
				{"different cooldown owner", func(a *Account) { a.TempUnschedulableReason = "upstream unavailable" }, false},
				{"prefix collision", func(a *Account) { a.TempUnschedulableReason = "cn_balance_lower" }, false},
				{"manually disabled account", func(a *Account) { a.Schedulable = false; a.TempUnschedulableUntil = nil }, true},
				{"other model", func(a *Account) {
					a.TempUnschedulableUntil = nil
					a.Credentials = map[string]any{"model_mapping": map[string]any{"other-model": "other-model"}}
				}, true},
				{"other platform", func(a *Account) { a.Platform = PlatformOpenAI; a.TempUnschedulableUntil = nil }, true},
				{"other group", func(a *Account) {
					a.AccountGroups = []AccountGroup{{GroupID: groupID + 1}}
					a.TempUnschedulableUntil = nil
				}, true},
			} {
				t.Run(state.name, func(t *testing.T) {
					other := blocked
					other.ID = 2
					state.mutate(&other)
					for _, accounts := range [][]Account{{blocked, other}, {other, blocked}} {
						repo := &mockAccountRepoForPlatform{accounts: accounts, accountsByID: map[int64]*Account{}}
						svc := tc.new(repo)

						diag := svc.DiagnoseModelAvailabilityForPlatform(context.Background(), &groupID, "deepseek-flash", PlatformDeepseek)

						require.True(t, diag.HasAccountsInPool)
						require.True(t, diag.HasModelSupport)
						require.Equal(t, state.allLowWant, diag.AllModelSupportAccountsInsufficientBalance)
						unsupported := svc.DiagnoseModelAvailabilityForPlatform(context.Background(), &groupID, "missing-model", PlatformDeepseek)
						require.False(t, unsupported.AllModelSupportAccountsInsufficientBalance)
					}
				})
			}
		})
	}
}

func TestDiagnoseModelAvailabilityForPlatform_LookupFailureDoesNotClaimBalanceExhaustion(t *testing.T) {
	repo := &countingCodexModelsAccountRepo{availabilityErr: errors.New("database unavailable")}
	for _, svc := range []ModelAvailabilityDiagnoser{
		&GatewayService{accountRepo: repo, cfg: testConfig()},
		&OpenAIGatewayService{accountRepo: repo, cfg: testConfig()},
	} {
		diag := svc.DiagnoseModelAvailabilityForPlatform(context.Background(), nil, "deepseek-flash", PlatformDeepseek)

		require.True(t, diag.HasModelSupport)
		require.False(t, diag.AllModelSupportAccountsInsufficientBalance)
	}
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
