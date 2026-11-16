package handler

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

type quotaPolicyResolverFunc func(int64, string) service.GroupQuotaPolicyResolution

func (f quotaPolicyResolverFunc) Resolve(groupID int64, model string) service.GroupQuotaPolicyResolution {
	return f(groupID, model)
}

func TestOpenAIGroupQuotaPolicyScopeAndDisabledPreserveModel(t *testing.T) {
	groupID := int64(17)
	key := func(platform string) *service.APIKey {
		return &service.APIKey{GroupID: &groupID, Group: &service.Group{ID: groupID, Platform: platform}}
	}
	for _, tc := range []struct {
		name    string
		key     *service.APIKey
		compact bool
	}{
		{"missing key", nil, false},
		{"missing group", &service.APIKey{GroupID: &groupID}, false},
		{"missing group id", &service.APIKey{Group: key(service.PlatformOpenAI).Group}, false},
		{"composite", key(service.PlatformComposite), false},
		{"anthropic", key(service.PlatformAnthropic), false},
		{"compact", key(service.PlatformOpenAI), true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := &OpenAIGatewayHandler{groupQuotaPolicy: quotaPolicyResolverFunc(func(int64, string) service.GroupQuotaPolicyResolution {
				t.Fatal("out-of-scope requests must not resolve quota policy")
				return service.GroupQuotaPolicyResolution{}
			})}
			result := h.resolveOpenAIGroupQuotaPolicy(tc.key, "channel-model", tc.compact)
			require.False(t, result.Applied)
			require.Equal(t, "channel-model", result.TargetModel)
		})
	}
	h := &OpenAIGatewayHandler{}
	require.Equal(t, "original", h.resolveOpenAIGroupQuotaPolicy(key(service.PlatformOpenAI), "original", false).TargetModel)
	h.groupQuotaPolicy = quotaPolicyResolverFunc(func(int64, string) service.GroupQuotaPolicyResolution {
		return service.GroupQuotaPolicyResolution{}
	})
	require.Equal(t, "original", h.resolveOpenAIGroupQuotaPolicy(key(service.PlatformOpenAI), "original", false).TargetModel)
	h.SetGroupQuotaPolicyService(nil)
	require.Nil(t, h.groupQuotaPolicy)
}

func TestOpenAIGroupQuotaPolicyFreezesAfterChannelWithoutChangingBillingIdentity(t *testing.T) {
	groupID := int64(41)
	key := &service.APIKey{GroupID: &groupID, Group: &service.Group{ID: groupID, Platform: service.PlatformOpenAI}}
	channel := service.ChannelMappingResult{Mapped: true, MappedModel: "channel-alias", ChannelID: 9, BillingModelSource: "channel_mapped"}
	originalChannel := channel
	calls := 0
	target := "policy-target"
	h := &OpenAIGatewayHandler{groupQuotaPolicy: quotaPolicyResolverFunc(func(gid int64, model string) service.GroupQuotaPolicyResolution {
		calls++
		require.Equal(t, groupID, gid)
		require.Equal(t, "channel-alias", model)
		return service.GroupQuotaPolicyResolution{Applied: true, SourceModel: model, TargetModel: target, RuleID: "rule-1", Revision: 3}
	})}
	policy := h.resolveOpenAIGroupQuotaPolicy(key, openAIChannelForwardModel(channel, "public-model"), false)
	target = "changed-after-admission"
	body := []byte(`{"model":"public-model","input":"hello","max_output_tokens":100}`)
	mapped := openAIModelMappedBody(body, channel.Mapped || policy.Applied, policy.TargetModel, func(body []byte, model string) []byte {
		out, err := sjson.SetBytes(body, "model", model)
		require.NoError(t, err)
		return out
	})
	require.Equal(t, 1, calls)
	require.Equal(t, "policy-target", gjson.GetBytes(mapped, "model").String())
	require.Equal(t, "public-model", gjson.GetBytes(body, "model").String())
	require.Equal(t, originalChannel, channel)
	estimate := quotaPolicyInflightEstimate("public-model", body, policy)
	require.Equal(t, "public-model", estimate.Model)
	require.Equal(t, "policy-target", estimate.AdditionalUpstreamModel)
	require.Equal(t, 100, estimate.MaxTokens)
	require.Equal(t, tokenInflightEstimate("public-model", body), quotaPolicyInflightEstimate("public-model", body, service.GroupQuotaPolicyResolution{}))
}

func TestOpenAIGroupQuotaPolicyWSTurnBoundary(t *testing.T) {
	account := &service.Account{Platform: service.PlatformOpenAI, Type: service.AccountTypeAPIKey, Credentials: map[string]any{
		"model_mapping": map[string]any{"base": "gpt-5.1", "low": "gpt-5-mini", "low-alias": "gpt-5-mini"},
	}}
	snapshot := func(model string, applied bool) *openAIWSTurnChannelMappingSnapshot {
		return &openAIWSTurnChannelMappingSnapshot{forwardModel: model, quotaPolicy: service.GroupQuotaPolicyResolution{Applied: applied, TargetModel: model}}
	}
	for _, tc := range []struct {
		name          string
		before, after *openAIWSTurnChannelMappingSnapshot
		reconnect     bool
	}{
		{"activation", snapshot("base", false), snapshot("low", true), true},
		{"recovery", snapshot("low", true), snapshot("base", false), true},
		{"same target alias", snapshot("low", true), snapshot("low-alias", true), false},
		{"policy revision only", snapshot("low", true), snapshot("low", true), false},
		{"same target recovery", snapshot("low", true), snapshot("low-alias", false), false},
		{"unsupported target", snapshot("low", true), snapshot("unsupported", true), true},
		{"no policy retains existing model switch handling", snapshot("base", false), snapshot("low", false), false},
		{"no earlier turn", nil, snapshot("low", true), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.reconnect, openAIQuotaPolicyTurnRequiresReconnect(account, tc.before, tc.after))
		})
	}
	require.False(t, shouldReportOpenAIWSProxyAccountFailure(newOpenAIWSUnsupportedModelSwitchError("low")), "policy boundary reconnect must not penalize a healthy account")
}

func TestOpenAIGroupQuotaPolicyWSRetryPreservesCurrentTurn(t *testing.T) {
	for _, sameRequestedModel := range []bool{false, true} {
		name := "later client model"
		if sameRequestedModel {
			name = "same client model with later policy alias"
		}
		t.Run(name, func(t *testing.T) {
			initial := &openAIWSTurnChannelMappingSnapshot{
				turn: 1, requestedModel: "original", forwardModel: "initial-target",
				mapping:     service.ChannelMappingResult{Mapped: true, MappedModel: "initial-channel", BillingModelSource: "requested"},
				quotaPolicy: service.GroupQuotaPolicyResolution{Applied: true, TargetModel: "initial-target", RuleID: "initial-rule", Revision: 1},
			}
			requestedModel := "later-model"
			if sameRequestedModel {
				requestedModel = initial.requestedModel
			}
			current := &openAIWSTurnChannelMappingSnapshot{
				turn: 4, requestedModel: requestedModel, forwardModel: "current-target",
				mapping:     service.ChannelMappingResult{Mapped: true, MappedModel: "current-channel", BillingModelSource: "channel_mapped"},
				quotaPolicy: service.GroupQuotaPolicyResolution{Applied: true, TargetModel: "current-target", RuleID: "current-rule", Revision: 7},
			}
			currentBody, err := sjson.SetBytes([]byte(`{"type":"response.create","input":[]}`), "model", requestedModel)
			require.NoError(t, err)
			body, safe := openAIWSNextAttemptMessage([]byte(`{"model":"original"}`), currentBody, true)
			require.True(t, safe)
			attempt := openAIWSTurnMappingForRetry(current, initial)
			require.Equal(t, 1, attempt.turn, "the restarted proxy begins at turn one")
			require.Equal(t, 4, current.turn, "published turn snapshots remain immutable")
			require.Equal(t, gjson.GetBytes(body, "model").String(), attempt.requestedModel)
			require.Equal(t, "current-target", attempt.forwardModel, "rescheduling uses the current turn's target")
			require.Equal(t, current.mapping, attempt.mapping, "billing retains this turn's channel identity")
			require.Equal(t, current.quotaPolicy, attempt.quotaPolicy, "retry must not reevaluate policy")
			usage := attempt.mapping.ToUsageFields(attempt.requestedModel, attempt.forwardModel)
			require.Equal(t, requestedModel, usage.OriginalModel, "retried turns retain their own requested-model billing identity")
			require.Equal(t, "current-channel", usage.ChannelMappedModel)
			require.Equal(t, requestedModel+"→current-channel→current-target", usage.ModelMappingChain)
			secondRetry := openAIWSTurnMappingForRetry(attempt, initial)
			require.Equal(t, attempt, secondRetry, "repeated retries retain the same frozen decision")
			require.NotSame(t, attempt, secondRetry)
		})
	}
}
