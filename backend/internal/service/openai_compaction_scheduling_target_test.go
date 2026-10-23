package service

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type openAICompactionDecisionRepo struct {
	AccountRepository
	accounts map[int64]*Account
}

func (r *openAICompactionDecisionRepo) GetByID(_ context.Context, id int64) (*Account, error) {
	if r == nil || r.accounts[id] == nil {
		return nil, fmt.Errorf("account %d not found", id)
	}
	return r.accounts[id], nil
}

func recordOpenAICompactionTargetEvidence(t *testing.T, svc *OpenAIGatewayService, account *Account, model, kind, verdict string) OpenAICompactionProbeTarget {
	t.Helper()
	target, err := svc.resolveOpenAICompactionTarget(context.Background(), account, model, kind, false)
	require.NoError(t, err)
	updates, err := BuildOpenAICompactionProbeExtraUpdates(account.Extra, target, OpenAICompactionProbeObservation{
		Verdict: verdict, CheckedAt: time.Now().UTC(), Status: 200,
	})
	require.NoError(t, err)
	mergeAccountExtra(account, updates)
	return target
}

func newOpenAICompactionDecisionAccount() *Account {
	return &Account{
		ID: 81001, Platform: PlatformDeepseek, Type: AccountTypeAPIKey,
		Status: StatusActive, Schedulable: true, Concurrency: 1,
		Credentials: map[string]any{
			"api_key":      "fixture-value",
			"account_mode": AccountModePayG, "api_protocol": APIProtocolResponses,
			"base_url": "https://relay.example.test/v1",
		},
		Extra: map[string]any{OpenAICompactSummaryEnabledExtraKey: true},
	}
}

func TestOpenAICompactionDecisionUsesExactTargetEvidencePerModelAndKind(t *testing.T) {
	svc := &OpenAIGatewayService{}
	account := newOpenAICompactionDecisionAccount()
	recordOpenAICompactionTargetEvidence(t, svc, account, "native-model", openAICompactionKindV2, OpenAICompactionProbeVerdictSupported)
	recordOpenAICompactionTargetEvidence(t, svc, account, "plain-model", openAICompactionKindV2, OpenAICompactionProbeVerdictUnsupported)

	native := svc.openAICompactionRequestDecision(context.Background(), account, "native-model", openAICompactionKindV2, true)
	require.Equal(t, openAICompactionExecutionNative, native.Execution)
	require.Equal(t, openAICompactionTierVerified, native.Tier)
	require.True(t, native.VerdictKnown)
	require.Equal(t, OpenAICompactionProbeVerdictSupported, native.Verdict)

	summary := svc.openAICompactionRequestDecision(context.Background(), account, "plain-model", openAICompactionKindV2, true)
	require.Equal(t, openAICompactionExecutionSummary, summary.Execution)
	require.True(t, summary.VerdictKnown)
	require.Equal(t, OpenAICompactionProbeVerdictUnsupported, summary.Verdict)

	legacy := svc.openAICompactionRequestDecision(context.Background(), account, "native-model", openAICompactionKindLegacy, true)
	require.Equal(t, openAICompactionExecutionSummary, legacy.Execution, "v2 evidence must not promote the legacy target")
	require.False(t, legacy.VerdictKnown)
}

func TestOpenAICompactionDecisionHonorsModesAndProtocolBoundary(t *testing.T) {
	svc := &OpenAIGatewayService{}
	account := newOpenAICompactionDecisionAccount()
	recordOpenAICompactionTargetEvidence(t, svc, account, "native-model", openAICompactionKindV2, OpenAICompactionProbeVerdictSupported)

	account.Extra["openai_compact_mode"] = OpenAICompactModeForceOff
	forceOff := svc.openAICompactionRequestDecision(context.Background(), account, "native-model", openAICompactionKindV2, true)
	require.Equal(t, openAICompactionExecutionSummary, forceOff.Execution)

	chat := newOpenAICompactionDecisionAccount()
	chat.Credentials["api_protocol"] = APIProtocolChatCompletions
	chat.Extra[OpenAICompactSummaryEnabledExtraKey] = false
	chat.Extra["openai_compact_mode"] = OpenAICompactModeForceOn
	forcedChat := svc.openAICompactionRequestDecision(context.Background(), chat, "chat-model", openAICompactionKindV2, true)
	require.Equal(t, openAICompactionExecutionUnsupported, forcedChat.Execution)
	require.Equal(t, "native_compaction_unsupported", forcedChat.Reason)

	chat.Extra[OpenAICompactSummaryEnabledExtraKey] = true
	summary := svc.openAICompactionRequestDecision(context.Background(), chat, "chat-model", openAICompactionKindV2, true)
	require.Equal(t, openAICompactionExecutionSummary, summary.Execution)
}

func TestOpenAICompactionLegacySummaryBypassesChatResponsesGate(t *testing.T) {
	ctx := context.Background()
	account := newOpenAICompactionDecisionAccount()
	account.ID = 81007
	account.Platform = PlatformOpenAI
	account.Credentials["api_protocol"] = APIProtocolChatCompletions
	account.Extra = map[string]any{OpenAICompactSummaryEnabledExtraKey: true, "openai_responses_supported": false}
	require.Empty(t, openAICompatibleAccountEligibilityFailureReasonBeforeProfit(
		ctx, account, PlatformOpenAI, "chat-model", true, OpenAIEndpointCapabilityResponses,
	))
	require.True(t, accountSupportsOpenAICapabilities(account, OpenAIEndpointCapabilityResponses, "", true))
	require.Equal(t, openAICompactionExecutionSummary,
		(&OpenAIGatewayService{}).openAICompactionRequestDecision(ctx, account, "chat-model", openAICompactionKindLegacy, true).Execution)

	for _, kind := range []string{openAICompactionKindLegacy, openAICompactionKindV2} {
		require.Equal(t, openAICompactionExecutionSummary,
			(&OpenAIGatewayService{}).openAICompactionRequestDecision(ctx, account, "chat-model", kind, true).Execution)
	}

	account.Extra[OpenAICompactSummaryEnabledExtraKey] = false
	require.NotEmpty(t, openAICompatibleAccountEligibilityFailureReasonBeforeProfit(
		ctx, account, PlatformOpenAI, "chat-model", true, OpenAIEndpointCapabilityResponses,
	))
	require.False(t, accountSupportsOpenAICapabilities(account, OpenAIEndpointCapabilityResponses, "", true))
	for _, kind := range []string{openAICompactionKindLegacy, openAICompactionKindV2} {
		require.Equal(t, openAICompactionExecutionUnsupported,
			(&OpenAIGatewayService{}).openAICompactionRequestDecision(ctx, account, "chat-model", kind, true).Execution)
	}
}

func TestOpenAICompactionLegacySummarySelectsChatAccount(t *testing.T) {
	for _, advanced := range []bool{false, true} {
		t.Run(map[bool]string{false: "legacy", true: "advanced"}[advanced], func(t *testing.T) {
			account := newOpenAICompactionDecisionAccount()
			account.ID = 81008
			account.Platform = PlatformOpenAI
			account.Credentials["api_protocol"] = APIProtocolChatCompletions
			account.Extra = map[string]any{OpenAICompactSummaryEnabledExtraKey: true, "openai_responses_supported": false}
			groupID := int64(81008)
			selectAccount := func(summaryEnabled, requireCompact bool) (*AccountSelectionResult, error) {
				account.Extra[OpenAICompactSummaryEnabledExtraKey] = summaryEnabled
				svc := newOpenAICompactionSchedulerTestService([]Account{*account}, advanced)
				selection, _, err := svc.SelectAccountWithSchedulerForCapability(
					context.Background(), &groupID, "", "", "chat-model", nil, OpenAIUpstreamTransportAny,
					OpenAIEndpointCapabilityResponses, requireCompact, false, true, PlatformOpenAI,
				)
				return selection, err
			}

			selection, err := selectAccount(true, true)
			require.NoError(t, err)
			require.NotNil(t, selection)
			require.Equal(t, account.ID, selection.Account.ID)

			selection, err = selectAccount(false, true)
			require.Error(t, err)
			require.Nil(t, selection)

			selection, err = selectAccount(true, false)
			require.Error(t, err, "ordinary Responses capability must remain enforced")
			require.Nil(t, selection)
		})
	}
}

func TestOpenAICompactionLegacySummaryUsesLoadBatchEntry(t *testing.T) {
	for _, advanced := range []bool{false, true} {
		t.Run(map[bool]string{false: "legacy", true: "advanced"}[advanced], func(t *testing.T) {
			account := newOpenAICompactionDecisionAccount()
			account.ID = 81009
			account.Platform = PlatformOpenAI
			account.Credentials["api_protocol"] = APIProtocolChatCompletions
			account.Extra = map[string]any{OpenAICompactSummaryEnabledExtraKey: true, "openai_responses_supported": false}
			svc := newOpenAICompactionSchedulerTestService([]Account{*account}, advanced)
			svc.cfg.Gateway.Scheduling.LoadBatchEnabled = true
			groupID := int64(81009)

			selection, _, err := svc.SelectAccountWithSchedulerForCapability(
				context.Background(), &groupID, "", "", "chat-model", nil, OpenAIUpstreamTransportAny,
				OpenAIEndpointCapabilityResponses, true, false, true, PlatformOpenAI,
			)
			require.NoError(t, err)
			require.NotNil(t, selection)
			require.Equal(t, account.ID, selection.Account.ID)
		})
	}
}

func TestOpenAICompactionDecisionInvalidatesChangedEndpoint(t *testing.T) {
	svc := &OpenAIGatewayService{}
	account := newOpenAICompactionDecisionAccount()
	recordOpenAICompactionTargetEvidence(t, svc, account, "native-model", openAICompactionKindV2, OpenAICompactionProbeVerdictSupported)
	changed := account.CloneForTest()
	changed.Credentials["base_url"] = "https://relay-two.example.test/v1"

	decision := svc.openAICompactionRequestDecision(context.Background(), changed, "native-model", openAICompactionKindV2, true)
	require.False(t, decision.VerdictKnown)
	require.Equal(t, openAICompactionExecutionSummary, decision.Execution)
}

func TestOpenAICompactionHistoricalFlagDoesNotCrossKinds(t *testing.T) {
	svc := &OpenAIGatewayService{}
	account := &Account{
		ID: 81002, Platform: PlatformOpenAI, Type: AccountTypeAPIKey,
		Credentials: map[string]any{
			"api_key":  "fixture-value",
			"base_url": "https://api.openai.com/v1",
		},
		Extra: map[string]any{"openai_compact_supported": false, "openai_responses_supported": true},
	}
	recordOpenAICompactionTargetEvidence(t, svc, account, "gpt-5.4", openAICompactionKindV2, OpenAICompactionProbeVerdictSupported)

	v2 := svc.openAICompactionRequestDecision(context.Background(), account, "gpt-5.4", openAICompactionKindV2, true)
	require.Equal(t, openAICompactionExecutionNative, v2.Execution)
	require.Equal(t, openAICompactionTierVerified, v2.Tier)

	legacy := svc.openAICompactionRequestDecision(context.Background(), account, "gpt-5.4", openAICompactionKindLegacy, true)
	require.Equal(t, openAICompactionExecutionNative, legacy.Execution)
	require.Equal(t, openAICompactionTierUnknown, legacy.Tier)
	require.Equal(t, ProviderCapabilityUnknown, legacy.ProviderCapability)
}

func TestOpenAICompactionHistoricalFlagCannotDecideUnprobedTarget(t *testing.T) {
	svc := &OpenAIGatewayService{}
	base := func(supported bool) *Account {
		return &Account{
			ID: 81003, Platform: PlatformOpenAI, Type: AccountTypeAPIKey,
			Credentials: map[string]any{
				"api_key":  "fixture-value",
				"base_url": "https://api.openai.com/v1", "api_protocol": APIProtocolResponses,
			},
			Extra: map[string]any{"openai_compact_supported": supported, "openai_responses_supported": true},
		}
	}
	for _, supported := range []bool{false, true} {
		for _, kind := range []string{openAICompactionKindLegacy, openAICompactionKindV2} {
			account := base(supported)
			account.Credentials["base_url"] = "https://new-relay.example.test/v1"
			decision := svc.openAICompactionRequestDecision(context.Background(), account, "unprobed-model", kind, true)
			require.False(t, decision.VerdictKnown)
			require.Equal(t, openAICompactionExecutionNative, decision.Execution)
			require.Equal(t, openAICompactionTierUnknown, decision.Tier)
			account.Extra[OpenAICompactSummaryEnabledExtraKey] = true
			decision = svc.openAICompactionRequestDecision(context.Background(), account, "unprobed-model", kind, true)
			require.Equal(t, openAICompactionExecutionSummary, decision.Execution)
		}
	}
}

func TestOpenAICompactionSchedulersRankVerifiedBeforeUnknown(t *testing.T) {
	svc := &OpenAIGatewayService{}
	fallback := newOpenAICompactionDecisionAccount()
	fallback.ID = 81005
	fallback.Platform = PlatformOpenAI
	fallback.Extra = map[string]any{"openai_compact_supported": true, "openai_responses_supported": true}
	verified := fallback.CloneForTest()
	verified.ID = 81006
	recordOpenAICompactionTargetEvidence(t, svc, verified, "gpt-5.4", openAICompactionKindV2, OpenAICompactionProbeVerdictSupported)
	require.Equal(t, openAICompactionTierVerified, svc.openAICompactSupportTier(verified, "gpt-5.4", openAICompactionKindV2))
	require.Equal(t, openAICompactionTierUnknown, svc.openAICompactSupportTier(fallback, "gpt-5.4", openAICompactionKindV2))

	ordered := svc.prioritizeOpenAICompactAccounts([]*Account{fallback, verified}, "gpt-5.4", openAICompactionKindV2)
	require.Equal(t, verified.ID, ordered[0].ID)

	planCandidates := []openAIAccountCandidateScore{
		{account: fallback, loadInfo: &AccountLoadInfo{AccountID: fallback.ID}, loadKnown: true},
		{account: verified, loadInfo: &AccountLoadInfo{AccountID: verified.ID}, loadKnown: true},
	}
	scheduler := &defaultOpenAIAccountScheduler{service: svc, stats: newOpenAIAccountRuntimeStats()}
	order := scheduler.buildOpenAISelectionOrder(OpenAIAccountScheduleRequest{
		RequestedModel:     "gpt-5.4",
		RequiredCapability: OpenAIEndpointCapabilityRemoteCompactionV2,
	}, openAIAccountLoadPlan{allCandidates: planCandidates, candidates: planCandidates, candidateCount: len(planCandidates), topK: 2})
	require.NotEmpty(t, order)
	require.Equal(t, verified.ID, order[0].account.ID)
}

func TestOpenAICompactionDecisionRehydratesExactEvidenceAfterColdCatalog(t *testing.T) {
	svc := &OpenAIGatewayService{}
	account := &Account{
		ID: 81004, Platform: PlatformCommandCode, Type: AccountTypeAPIKey,
		Credentials: map[string]any{
			"api_key":      "fixture-value",
			"api_protocol": APIProtocolAdaptive, "account_mode": AccountModePayG,
			"api_base_urls": map[string]any{
				APIProtocolResponses:       "https://aggregate.example.test/native",
				APIProtocolChatCompletions: "https://aggregate.example.test/native",
				APIProtocolAnthropic:       "https://aggregate.example.test/native",
			},
		},
		Extra: map[string]any{OpenAICompactSummaryEnabledExtraKey: true},
	}
	target, err := svc.resolveOpenAICompactionTargetForProtocol(context.Background(), account, "cold-model", openAICompactionKindV2, APIProtocolResponses)
	require.NoError(t, err)
	updates, err := BuildOpenAICompactionProbeExtraUpdates(account.Extra, target, OpenAICompactionProbeObservation{
		Verdict: OpenAICompactionProbeVerdictSupported, CheckedAt: time.Now().UTC(), Status: 200,
	})
	require.NoError(t, err)
	mergeAccountExtra(account, updates)

	cold := svc.openAICompactionRequestDecision(context.Background(), account, "cold-model", openAICompactionKindV2, true)
	require.Equal(t, openAICompactionExecutionSummary, cold.Execution, "candidate projection must remain unknown")
	require.False(t, cold.VerdictKnown)

	native := svc.openAICompactionRequestDecisionWithProtocol(context.Background(), account, "cold-model", openAICompactionKindV2, true, APIProtocolResponses)
	require.Equal(t, openAICompactionExecutionNative, native.Execution)
	require.Equal(t, openAICompactionTierVerified, native.Tier)
	require.Equal(t, APIProtocolResponses, native.Target.Protocol)
	require.True(t, native.VerdictKnown)

	changedCatalog := svc.openAICompactionRequestDecisionWithProtocol(context.Background(), account, "cold-model", openAICompactionKindV2, true, APIProtocolChatCompletions)
	require.Equal(t, openAICompactionExecutionSummary, changedCatalog.Execution)
	require.False(t, changedCatalog.VerdictKnown)
}

func TestOpenAICompactionDecisionShadowRequiresSelectedParentIdentity(t *testing.T) {
	svc := &OpenAIGatewayService{}
	parentID := int64(81011)
	parent := &Account{
		ID: parentID, Platform: PlatformOpenAI, Type: AccountTypeOAuth,
		Credentials: map[string]any{"chatgpt_account_id": "chatgpt-a"},
	}
	shadow := &Account{
		ID: 81010, Platform: PlatformOpenAI, Type: AccountTypeOAuth, ParentAccountID: &parentID,
		Extra: map[string]any{OpenAICompactSummaryEnabledExtraKey: true},
	}
	target, err := svc.resolveOpenAICompactionTargetWithCredential(context.Background(), shadow, parent, "gpt-5.4", openAICompactionKindV2, false)
	require.NoError(t, err)
	updates, err := BuildOpenAICompactionProbeExtraUpdates(shadow.Extra, target, OpenAICompactionProbeObservation{
		Verdict: OpenAICompactionProbeVerdictSupported, CheckedAt: time.Now().UTC(), Status: 200,
	})
	require.NoError(t, err)
	mergeAccountExtra(shadow, updates)

	projected := svc.openAICompactionRequestDecision(context.Background(), shadow, "gpt-5.4", openAICompactionKindV2, false)
	require.Equal(t, openAICompactionExecutionSummary, projected.Execution, "shadow projection lacks parent credential identity")
	require.False(t, projected.VerdictKnown)

	svc.accountRepo = &openAICompactionDecisionRepo{accounts: map[int64]*Account{parentID: parent}}
	selected := svc.openAICompactionRequestDecision(context.Background(), shadow, "gpt-5.4", openAICompactionKindV2, true)
	require.Equal(t, openAICompactionExecutionNative, selected.Execution)
	require.Equal(t, openAICompactionTierVerified, selected.Tier)

	reauthoredParent := parent.CloneForTest()
	reauthoredParent.Credentials["chatgpt_account_id"] = "chatgpt-b"
	svc.accountRepo = &openAICompactionDecisionRepo{accounts: map[int64]*Account{parentID: reauthoredParent}}
	reauthorized := svc.openAICompactionRequestDecision(context.Background(), shadow, "gpt-5.4", openAICompactionKindV2, true)
	require.Equal(t, openAICompactionExecutionSummary, reauthorized.Execution)
	require.False(t, reauthorized.VerdictKnown)

	svc.accountRepo = &openAICompactionDecisionRepo{accounts: map[int64]*Account{}}
	missing := svc.openAICompactionRequestDecision(context.Background(), shadow, "gpt-5.4", openAICompactionKindV2, true)
	require.Equal(t, openAICompactionExecutionSummary, missing.Execution)
	require.False(t, missing.VerdictKnown)
}
