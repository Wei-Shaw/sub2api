package service

import (
	"context"
	"errors"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

// TestOpenAIGatewayService_SelectAccountWithScheduler_CompactPrefersSupportedOverUnknown
// 验证 compact 调度时显式支持 (tier=2) 优先于未探测 (tier=1)。
func TestOpenAIGatewayService_SelectAccountWithScheduler_CompactPrefersSupportedOverUnknown(t *testing.T) {
	resetOpenAIAdvancedSchedulerSettingCacheForTest()

	ctx := context.Background()
	groupID := int64(91001)
	accounts := []Account{
		{
			ID:          71001,
			Platform:    PlatformOpenAI,
			Type:        AccountTypeAPIKey,
			Status:      StatusActive,
			Schedulable: true,
			Concurrency: 1,
			Priority:    0,
			Credentials: map[string]any{"api_protocol": APIProtocolResponses},
			Extra:       map[string]any{}, // unknown
		},
		{
			ID:          71002,
			Platform:    PlatformOpenAI,
			Type:        AccountTypeAPIKey,
			Status:      StatusActive,
			Schedulable: true,
			Concurrency: 1,
			Priority:    0,
			Credentials: map[string]any{"api_protocol": APIProtocolResponses},
			Extra:       map[string]any{},
		},
	}
	accounts[1].Credentials["api_key"] = "fixture-value"
	recordOpenAICompactionTargetEvidence(t, &OpenAIGatewayService{}, &accounts[1], "gpt-5.4", openAICompactionKindLegacy, OpenAICompactionProbeVerdictSupported)
	cfg := &config.Config{}
	cfg.Gateway.Scheduling.LoadBatchEnabled = false
	svc := &OpenAIGatewayService{
		accountRepo:        schedulerTestOpenAIAccountRepo{accounts: accounts},
		cache:              &schedulerTestGatewayCache{},
		cfg:                cfg,
		concurrencyService: NewConcurrencyService(schedulerTestConcurrencyCache{}),
	}

	selection, _, err := svc.SelectAccountWithScheduler(
		ctx,
		&groupID,
		"",
		"",
		"gpt-5.4",
		nil,
		OpenAIUpstreamTransportAny,
		true,
	)
	require.NoError(t, err)
	require.NotNil(t, selection)
	require.NotNil(t, selection.Account)
	require.Equal(t, int64(71002), selection.Account.ID, "compact-supported account should win over unknown")
}

// TestOpenAIGatewayService_SelectAccountWithScheduler_CompactRejectsExplicitlyUnsupported
// 验证 force_off / 已探测不支持 (tier=0) 的账号不会被 compact 请求选中。
func TestOpenAIGatewayService_SelectAccountWithScheduler_CompactRejectsExplicitlyUnsupported(t *testing.T) {
	resetOpenAIAdvancedSchedulerSettingCacheForTest()

	ctx := context.Background()
	groupID := int64(91002)
	accounts := []Account{
		{
			ID:          71010,
			Platform:    PlatformOpenAI,
			Type:        AccountTypeAPIKey,
			Status:      StatusActive,
			Schedulable: true,
			Concurrency: 1,
			Priority:    0,
			Credentials: map[string]any{"api_protocol": APIProtocolResponses},
			Extra:       map[string]any{"openai_compact_mode": OpenAICompactModeForceOff},
		},
		{
			ID:          71011,
			Platform:    PlatformOpenAI,
			Type:        AccountTypeAPIKey,
			Status:      StatusActive,
			Schedulable: true,
			Concurrency: 1,
			Priority:    0,
			Credentials: map[string]any{"api_protocol": APIProtocolResponses},
			Extra:       map[string]any{},
		},
	}
	accounts[1].Credentials["api_key"] = "fixture-value"
	recordOpenAICompactionTargetEvidence(t, &OpenAIGatewayService{}, &accounts[1], "gpt-5.4", openAICompactionKindLegacy, OpenAICompactionProbeVerdictUnsupported)
	cfg := &config.Config{}
	cfg.Gateway.Scheduling.LoadBatchEnabled = false
	svc := &OpenAIGatewayService{
		accountRepo:        schedulerTestOpenAIAccountRepo{accounts: accounts},
		cache:              &schedulerTestGatewayCache{},
		cfg:                cfg,
		concurrencyService: NewConcurrencyService(schedulerTestConcurrencyCache{}),
	}

	selection, _, err := svc.SelectAccountWithScheduler(
		ctx,
		&groupID,
		"",
		"",
		"gpt-5.4",
		nil,
		OpenAIUpstreamTransportAny,
		true,
	)
	require.Error(t, err)
	require.True(t, errors.Is(err, ErrNoAvailableCompactAccounts), "compact-only accounts should rejected explicitly unsupported and return compact error")
	require.Nil(t, selection)
}

func newOpenAICompactionSchedulerTestService(accounts []Account, advanced bool) *OpenAIGatewayService {
	cfg := &config.Config{}
	cfg.Gateway.Scheduling.LoadBatchEnabled = false
	svc := &OpenAIGatewayService{
		accountRepo:        schedulerTestOpenAIAccountRepo{accounts: accounts},
		cache:              &schedulerTestGatewayCache{},
		cfg:                cfg,
		concurrencyService: NewConcurrencyService(schedulerTestConcurrencyCache{}),
	}
	if advanced {
		svc.rateLimitService = newOpenAIAdvancedSchedulerRateLimitService("true")
	}
	return svc
}

func selectOpenAICompactionSchedulerTestAccount(t *testing.T, svc *OpenAIGatewayService, groupID int64, requireCompact bool) (*AccountSelectionResult, error) {
	t.Helper()
	selection, _, err := svc.SelectAccountWithSchedulerForCapability(
		context.Background(),
		&groupID,
		"",
		"",
		"gpt-5.6-sol",
		nil,
		OpenAIUpstreamTransportAny,
		OpenAIEndpointCapabilityResponses,
		requireCompact,
		false,
		false,
	)
	return selection, err
}

func TestOpenAIGatewayService_SelectAccountWithScheduler_OrdinaryResponsesIgnoresCompactProbe(t *testing.T) {
	for _, advanced := range []bool{false, true} {
		t.Run(map[bool]string{false: "legacy_scheduler", true: "advanced_scheduler"}[advanced], func(t *testing.T) {
			resetOpenAIAdvancedSchedulerSettingCacheForTest()
			svc := newOpenAICompactionSchedulerTestService([]Account{{
				ID:          71012,
				Platform:    PlatformOpenAI,
				Type:        AccountTypeAPIKey,
				Status:      StatusActive,
				Schedulable: true,
				Concurrency: 1,
				Extra: map[string]any{
					"openai_compact_supported":   false,
					"openai_responses_supported": true,
				},
			}}, advanced)

			selection, err := selectOpenAICompactionSchedulerTestAccount(t, svc, 91007, false)
			require.NoError(t, err)
			require.NotNil(t, selection)
			require.Equal(t, int64(71012), selection.Account.ID)
		})
	}
}

func TestOpenAIGatewayService_SelectAccountWithScheduler_OrdinaryResponsesAllowsCompactForceOff(t *testing.T) {
	for _, advanced := range []bool{false, true} {
		t.Run(map[bool]string{false: "legacy_scheduler", true: "advanced_scheduler"}[advanced], func(t *testing.T) {
			resetOpenAIAdvancedSchedulerSettingCacheForTest()
			svc := newOpenAICompactionSchedulerTestService([]Account{{
				ID:          71013,
				Platform:    PlatformOpenAI,
				Type:        AccountTypeAPIKey,
				Status:      StatusActive,
				Schedulable: true,
				Concurrency: 1,
				Extra: map[string]any{
					"openai_compact_mode":        OpenAICompactModeForceOff,
					"openai_responses_supported": true,
				},
			}}, advanced)

			selection, err := selectOpenAICompactionSchedulerTestAccount(t, svc, 91008, false)
			require.NoError(t, err)
			require.NotNil(t, selection)
			require.Equal(t, int64(71013), selection.Account.ID)
		})
	}
}

func TestOpenAIGatewayService_SelectAccountWithScheduler_NativeCompactionRequiresResponsesCapability(t *testing.T) {
	for _, advanced := range []bool{false, true} {
		t.Run(map[bool]string{false: "legacy_scheduler", true: "advanced_scheduler"}[advanced], func(t *testing.T) {
			resetOpenAIAdvancedSchedulerSettingCacheForTest()
			svc := newOpenAICompactionSchedulerTestService([]Account{{
				ID:          71014,
				Platform:    PlatformOpenAI,
				Type:        AccountTypeAPIKey,
				Status:      StatusActive,
				Schedulable: true,
				Concurrency: 1,
				Extra: map[string]any{
					"openai_compact_mode":        OpenAICompactModeForceOn,
					"openai_responses_supported": false,
				},
			}}, advanced)

			selection, err := selectOpenAICompactionSchedulerTestAccount(t, svc, 91009, false)
			require.ErrorIs(t, err, ErrNoAvailableAccounts)
			require.NotErrorIs(t, err, ErrNoAvailableCompactAccounts)
			require.NotContains(t, err.Error(), "/responses/compact")
			require.Nil(t, selection)
		})
	}
}

func TestOpenAIGatewayService_SelectAccountWithScheduler_LegacyCompactionKeepsCompactEligibility(t *testing.T) {
	for _, advanced := range []bool{false, true} {
		t.Run(map[bool]string{false: "legacy_scheduler", true: "advanced_scheduler"}[advanced], func(t *testing.T) {
			resetOpenAIAdvancedSchedulerSettingCacheForTest()
			accounts := []Account{
				{
					ID:          71015,
					Platform:    PlatformOpenAI,
					Type:        AccountTypeAPIKey,
					Status:      StatusActive,
					Schedulable: true,
					Concurrency: 1,
					Extra: map[string]any{
						"openai_compact_supported":   false,
						"openai_responses_supported": true,
					},
				},
				{
					ID:          71016,
					Platform:    PlatformOpenAI,
					Type:        AccountTypeAPIKey,
					Status:      StatusActive,
					Schedulable: true,
					Concurrency: 1,
					Extra: map[string]any{
						"openai_compact_mode":        OpenAICompactModeForceOff,
						"openai_responses_supported": true,
					},
				},
			}
			accounts[0].Credentials = map[string]any{"api_key": "fixture-value", "api_protocol": APIProtocolResponses}
			recordOpenAICompactionTargetEvidence(t, &OpenAIGatewayService{}, &accounts[0], "gpt-5.6-sol", openAICompactionKindLegacy, OpenAICompactionProbeVerdictUnsupported)
			svc := newOpenAICompactionSchedulerTestService(accounts, advanced)

			selection, err := selectOpenAICompactionSchedulerTestAccount(t, svc, 91010, true)
			require.ErrorIs(t, err, ErrNoAvailableCompactAccounts)
			require.Contains(t, err.Error(), "/responses/compact")
			require.Nil(t, selection)
		})
	}
}

func selectAdvancedLegacyCompactFromSnapshot(t *testing.T, snapshotAccount, dbAccount Account) (*AccountSelectionResult, error) {
	t.Helper()
	resetOpenAIAdvancedSchedulerSettingCacheForTest()
	groupID := int64(91011)
	svc := newOpenAICompactionSchedulerTestService([]Account{dbAccount}, true)
	svc.schedulerSnapshot = &SchedulerSnapshotService{cache: &openAISnapshotCacheStub{
		accountsByID: map[int64]*Account{snapshotAccount.ID: &snapshotAccount},
	}}
	scheduler := &defaultOpenAIAccountScheduler{service: svc, stats: newOpenAIAccountRuntimeStats()}
	req := OpenAIAccountScheduleRequest{
		GroupID:            &groupID,
		Platform:           PlatformOpenAI,
		RequestedModel:     "gpt-5.6-sol",
		RequiredTransport:  OpenAIUpstreamTransportAny,
		RequiredCapability: OpenAIEndpointCapabilityResponses,
		RequireCompact:     true,
	}
	filtered := []*Account{&snapshotAccount}
	loadMap := map[int64]*AccountLoadInfo{
		snapshotAccount.ID: {AccountID: snapshotAccount.ID},
	}
	budget := newOpenAISelectionProbeBudget()
	budget.enableLimit()
	attempt := scheduler.trySelectByLoadBalancePool(context.Background(), req, filtered, loadMap, budget)
	if attempt.result != nil {
		return attempt.result, nil
	}
	selection, _, _, _, err := scheduler.finishLoadBalanceSelectionFallback(
		context.Background(), req, attempt, budget, openAISelectionFilterStats{},
	)
	return selection, err
}

func TestOpenAIGatewayService_AdvancedLegacyCompactKeepsSnapshotUnsupportedDiagnostic(t *testing.T) {
	stale := Account{
		ID: 71017, Platform: PlatformOpenAI, Type: AccountTypeAPIKey,
		Status: StatusActive, Schedulable: true, Concurrency: 1, GroupIDs: []int64{91011},
		Credentials: map[string]any{"api_key": "fixture-provider-key", "api_protocol": APIProtocolResponses, "base_url": "https://api.openai.com"},
		Extra:       map[string]any{"openai_compact_supported": false, "openai_responses_supported": true},
	}
	decisionSvc := &OpenAIGatewayService{}
	recordOpenAICompactionTargetEvidence(t, decisionSvc, &stale, "gpt-5.6-sol", openAICompactionKindV2, OpenAICompactionProbeVerdictSupported)
	recordOpenAICompactionTargetEvidence(t, decisionSvc, &stale, "gpt-5.6-sol", openAICompactionKindLegacy, OpenAICompactionProbeVerdictUnsupported)
	dbAccount := *stale.CloneForTest()

	selection, err := selectAdvancedLegacyCompactFromSnapshot(t, stale, dbAccount)
	require.Nil(t, selection)
	require.ErrorIs(t, err, ErrNoAvailableCompactAccounts)
	require.Contains(t, err.Error(), "/responses/compact")
}

func TestOpenAIGatewayService_AdvancedLegacyCompactPreservesDBCompactRejection(t *testing.T) {
	stale := Account{
		ID: 71019, Platform: PlatformOpenAI, Type: AccountTypeAPIKey,
		Status: StatusActive, Schedulable: true, Concurrency: 1, GroupIDs: []int64{91011},
		Credentials: map[string]any{"api_key": "fixture-provider-key", "api_protocol": APIProtocolResponses, "base_url": "https://api.openai.com"},
		Extra:       map[string]any{"openai_responses_supported": true},
	}
	decisionSvc := &OpenAIGatewayService{}
	recordOpenAICompactionTargetEvidence(t, decisionSvc, &stale, "gpt-5.6-sol", openAICompactionKindV2, OpenAICompactionProbeVerdictSupported)
	dbAccount := *stale.CloneForTest()
	recordOpenAICompactionTargetEvidence(t, decisionSvc, &dbAccount, "gpt-5.6-sol", openAICompactionKindLegacy, OpenAICompactionProbeVerdictUnsupported)

	selection, err := selectAdvancedLegacyCompactFromSnapshot(t, stale, dbAccount)
	require.Nil(t, selection)
	require.ErrorIs(t, err, ErrNoAvailableCompactAccounts)
	require.Contains(t, err.Error(), "/responses/compact")
}

func TestOpenAIGatewayService_AdvancedLegacyCompactRecoversFromFreshDB(t *testing.T) {
	stale := Account{
		ID: 71018, Platform: PlatformOpenAI, Type: AccountTypeAPIKey,
		Status: StatusActive, Schedulable: true, Concurrency: 1, GroupIDs: []int64{91011},
		Credentials: map[string]any{"api_key": "fixture-provider-key", "api_protocol": APIProtocolResponses, "base_url": "https://api.openai.com"},
		Extra:       map[string]any{"openai_compact_supported": false, "openai_responses_supported": true},
	}
	decisionSvc := &OpenAIGatewayService{}
	recordOpenAICompactionTargetEvidence(t, decisionSvc, &stale, "gpt-5.6-sol", openAICompactionKindLegacy, OpenAICompactionProbeVerdictUnsupported)
	dbAccount := *stale.CloneForTest()
	dbAccount.Extra["openai_compact_supported"] = true
	recordOpenAICompactionTargetEvidence(t, decisionSvc, &dbAccount, "gpt-5.6-sol", openAICompactionKindLegacy, OpenAICompactionProbeVerdictSupported)
	decision := (&OpenAIGatewayService{}).openAICompactionRequestDecision(
		context.Background(), &dbAccount, "gpt-5.6-sol", openAICompactionKindLegacy, true,
	)
	require.True(t, decision.Allowed(), "fresh DB legacy compact should be selectable: %+v", decision)
	debugGroupID := int64(91011)
	debugSvc := newOpenAICompactionSchedulerTestService([]Account{dbAccount}, true)
	debugSvc.schedulerSnapshot = &SchedulerSnapshotService{cache: &openAISnapshotCacheStub{
		accountsByID: map[int64]*Account{stale.ID: &stale},
	}}
	fresh := debugSvc.resolveFreshSchedulableOpenAIAccount(
		context.Background(), &stale, PlatformOpenAI, "gpt-5.6-sol", true, OpenAIEndpointCapabilityResponses,
	)
	require.NotNil(t, fresh, "snapshot account should remain eligible for DB recheck")
	fresh = debugSvc.recheckSelectedOpenAIAccountFromDB(
		context.Background(), fresh, &debugGroupID, PlatformOpenAI, "gpt-5.6-sol", true, OpenAIEndpointCapabilityResponses,
	)
	require.NotNil(t, fresh, "fresh DB compact capability should survive recheck")

	selection, err := selectAdvancedLegacyCompactFromSnapshot(t, stale, dbAccount)
	require.NoError(t, err)
	require.NotNil(t, selection)
	require.NotNil(t, selection.Account)
	require.Equal(t, stale.ID, selection.Account.ID)
	require.Equal(t, false, stale.Extra["openai_compact_supported"])
	require.Equal(t, true, selection.Account.Extra["openai_compact_supported"])
	if selection.ReleaseFunc != nil {
		selection.ReleaseFunc()
	}
}

// TestOpenAIGatewayService_SelectAccountWithScheduler_CompactRequiresResponsesCapability
// prevents a confirmed Chat Completions-only API key from silently dropping the
// compaction trigger through the Responses-to-Chat fallback.
func TestOpenAIGatewayService_SelectAccountWithScheduler_CompactRequiresResponsesCapability(t *testing.T) {
	resetOpenAIAdvancedSchedulerSettingCacheForTest()

	ctx := context.Background()
	groupID := int64(91005)
	accounts := []Account{{
		ID:          71050,
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Status:      StatusActive,
		Schedulable: true,
		Concurrency: 1,
		Extra: map[string]any{
			"openai_compact_supported":   true,
			"openai_responses_supported": false,
		},
	}}
	cfg := &config.Config{}
	cfg.Gateway.Scheduling.LoadBatchEnabled = false
	svc := &OpenAIGatewayService{
		accountRepo:        schedulerTestOpenAIAccountRepo{accounts: accounts},
		cache:              &schedulerTestGatewayCache{},
		cfg:                cfg,
		concurrencyService: NewConcurrencyService(schedulerTestConcurrencyCache{}),
	}

	selection, _, err := svc.SelectAccountWithSchedulerForCapability(
		ctx,
		&groupID,
		"",
		"",
		"gpt-5.4",
		nil,
		OpenAIUpstreamTransportAny,
		OpenAIEndpointCapabilityResponses,
		true,
		false,
		false,
	)
	require.Error(t, err)
	require.True(t, errors.Is(err, ErrNoAvailableAccounts))
	require.Nil(t, selection)
}

func TestOpenAIGatewayService_SelectAccountWithScheduler_CompactSkipsChatOnlyAccount(t *testing.T) {
	resetOpenAIAdvancedSchedulerSettingCacheForTest()

	ctx := context.Background()
	groupID := int64(91006)
	accounts := []Account{
		{
			ID:          71060,
			Platform:    PlatformOpenAI,
			Type:        AccountTypeAPIKey,
			Status:      StatusActive,
			Schedulable: true,
			Concurrency: 1,
			Priority:    10,
			Extra: map[string]any{
				"openai_compact_supported":   true,
				"openai_responses_supported": false,
			},
		},
		{
			ID:          71061,
			Platform:    PlatformOpenAI,
			Type:        AccountTypeAPIKey,
			Status:      StatusActive,
			Schedulable: true,
			Concurrency: 1,
			Priority:    0,
			Extra: map[string]any{
				"openai_compact_supported":   true,
				"openai_responses_supported": true,
			},
		},
	}
	cfg := &config.Config{}
	cfg.Gateway.Scheduling.LoadBatchEnabled = false
	svc := &OpenAIGatewayService{
		accountRepo:        schedulerTestOpenAIAccountRepo{accounts: accounts},
		cache:              &schedulerTestGatewayCache{},
		cfg:                cfg,
		concurrencyService: NewConcurrencyService(schedulerTestConcurrencyCache{}),
	}

	selection, _, err := svc.SelectAccountWithSchedulerForCapability(
		ctx,
		&groupID,
		"",
		"",
		"gpt-5.6-sol",
		nil,
		OpenAIUpstreamTransportAny,
		OpenAIEndpointCapabilityResponses,
		true,
		false,
		false,
	)
	require.NoError(t, err)
	require.NotNil(t, selection)
	require.NotNil(t, selection.Account)
	require.Equal(t, int64(71061), selection.Account.ID)
}

// TestOpenAIGatewayService_SelectAccountWithScheduler_CompactFallsBackToUnknown
// 验证当没有"已知支持"账号时，compact 请求会回退到"未探测"账号。
func TestOpenAIGatewayService_SelectAccountWithScheduler_CompactFallsBackToUnknown(t *testing.T) {
	resetOpenAIAdvancedSchedulerSettingCacheForTest()

	ctx := context.Background()
	groupID := int64(91003)
	accounts := []Account{
		{
			ID:          71020,
			Platform:    PlatformOpenAI,
			Type:        AccountTypeAPIKey,
			Status:      StatusActive,
			Schedulable: true,
			Concurrency: 1,
			Priority:    0,
			Credentials: map[string]any{"api_protocol": APIProtocolResponses},
			Extra:       map[string]any{},
		},
		{
			ID:          71021,
			Platform:    PlatformOpenAI,
			Type:        AccountTypeAPIKey,
			Status:      StatusActive,
			Schedulable: true,
			Concurrency: 1,
			Priority:    0,
			Credentials: map[string]any{"api_protocol": APIProtocolResponses},
			Extra:       map[string]any{}, // unknown -> tier=1
		},
	}
	accounts[0].Credentials["api_key"] = "fixture-value"
	recordOpenAICompactionTargetEvidence(t, &OpenAIGatewayService{}, &accounts[0], "gpt-5.4", openAICompactionKindLegacy, OpenAICompactionProbeVerdictUnsupported)
	cfg := &config.Config{}
	cfg.Gateway.Scheduling.LoadBatchEnabled = false
	svc := &OpenAIGatewayService{
		accountRepo:        schedulerTestOpenAIAccountRepo{accounts: accounts},
		cache:              &schedulerTestGatewayCache{},
		cfg:                cfg,
		concurrencyService: NewConcurrencyService(schedulerTestConcurrencyCache{}),
	}

	selection, _, err := svc.SelectAccountWithScheduler(
		ctx,
		&groupID,
		"",
		"",
		"gpt-5.4",
		nil,
		OpenAIUpstreamTransportAny,
		true,
	)
	require.NoError(t, err)
	require.NotNil(t, selection)
	require.NotNil(t, selection.Account)
	require.Equal(t, int64(71021), selection.Account.ID, "unknown account should be picked when no supported account available")
}

// TestOpenAIGatewayService_SelectAccountWithScheduler_CompactAllowsGrok verifies
// that OpenAI-compatible compact routing does not reject Grok accounts.
func TestOpenAIGatewayService_SelectAccountWithScheduler_CompactAllowsGrok(t *testing.T) {
	resetOpenAIAdvancedSchedulerSettingCacheForTest()

	ctx := context.Background()
	groupID := int64(91004)
	accounts := []Account{
		{
			ID:          71030,
			Platform:    PlatformGrok,
			Type:        AccountTypeOAuth,
			Status:      StatusActive,
			Schedulable: true,
			Concurrency: 1,
			Priority:    0,
			Credentials: map[string]any{
				"model_mapping": map[string]any{"grok-4.5": "grok-4.5"},
			},
		},
	}
	cfg := &config.Config{}
	cfg.Gateway.Scheduling.LoadBatchEnabled = false
	svc := &OpenAIGatewayService{
		accountRepo:        schedulerTestOpenAIAccountRepo{accounts: accounts},
		cache:              &schedulerTestGatewayCache{},
		cfg:                cfg,
		concurrencyService: NewConcurrencyService(schedulerTestConcurrencyCache{}),
	}

	selection, _, err := svc.SelectAccountWithSchedulerForCapability(
		ctx,
		&groupID,
		"",
		"",
		"grok-4.5",
		nil,
		OpenAIUpstreamTransportAny,
		OpenAIEndpointCapabilityChatCompletions,
		true,
		false,
		true,
		PlatformGrok,
	)
	require.NoError(t, err)
	require.NotNil(t, selection)
	require.NotNil(t, selection.Account)
	require.Equal(t, int64(71030), selection.Account.ID)
}

// TestOpenAICompactSupportTier 验证 target-scoped tier 分类逻辑。
func TestOpenAICompactSupportTier(t *testing.T) {
	tests := []struct {
		name    string
		account *Account
		want    int
	}{
		{name: "nil", account: nil, want: 0},
		{name: "non openai", account: &Account{Platform: PlatformAnthropic}, want: 0},
		{name: "grok", account: &Account{Platform: PlatformGrok}, want: openAICompactionTierVerified},
		{name: "openai unknown", account: &Account{Platform: PlatformOpenAI, Extra: map[string]any{}}, want: openAICompactionTierUnknown},
		{name: "historical supported remains unknown", account: &Account{Platform: PlatformOpenAI, Extra: map[string]any{"openai_compact_supported": true}}, want: openAICompactionTierUnknown},
		{name: "historical unsupported remains unknown", account: &Account{Platform: PlatformOpenAI, Extra: map[string]any{"openai_compact_supported": false}}, want: openAICompactionTierUnknown},
		{name: "force on remains an attempt", account: &Account{Platform: PlatformOpenAI, Extra: map[string]any{"openai_compact_mode": OpenAICompactModeForceOn}}, want: openAICompactionTierUnknown},
		{name: "force off overrides probe true", account: &Account{Platform: PlatformOpenAI, Extra: map[string]any{"openai_compact_mode": OpenAICompactModeForceOff, "openai_compact_supported": true}}, want: openAICompactionTierUnsupported},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := (&OpenAIGatewayService{}).openAICompactSupportTier(tt.account, "", openAICompactionKindV2); got != tt.want {
				t.Fatalf("openAICompactSupportTier(...) = %d, want %d", got, tt.want)
			}
		})
	}
}
