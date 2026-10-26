package service

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestSupportsRemoteCompactionV2UsesProviderEndpointCapability(t *testing.T) {
	tests := []struct {
		name        string
		platform    string
		credentials map[string]any
		extra       map[string]any
		want        bool
	}{
		{
			name:     "official deepseek responses endpoint is unsupported",
			platform: PlatformDeepseek,
			credentials: map[string]any{
				"account_mode": "payg",
				"api_protocol": APIProtocolResponses,
			},
			want: false,
		},
		{
			name:     "custom deepseek endpoint remains unknown and routable",
			platform: PlatformDeepseek,
			credentials: map[string]any{
				"account_mode": "payg",
				"api_protocol": APIProtocolResponses,
				"base_url":     "https://relay.example.test",
			},
			want: true,
		},
		{
			name:     "openai force off remains unsupported",
			platform: PlatformOpenAI,
			extra: map[string]any{
				"openai_compact_mode": OpenAICompactModeForceOff,
			},
			want: false,
		},
		{
			name:     "unprobed openai remains compatible",
			platform: PlatformOpenAI,
			want:     true,
		},
		{
			name:     "compact force on does not override responses force off",
			platform: PlatformOpenAI,
			extra:    map[string]any{"openai_compact_mode": OpenAICompactModeForceOn, "openai_responses_mode": "force_chat_completions"},
		},
		{
			name:        "compact does not bypass chat capability restrictions",
			platform:    PlatformOpenAI,
			credentials: map[string]any{"openai_capabilities": []any{"embeddings"}},
		},
		{
			name:        "custom deepseek chat only cannot handle native compact",
			platform:    PlatformDeepseek,
			credentials: map[string]any{"api_protocol": APIProtocolChatCompletions, "base_url": "https://relay.example.test"},
		},
		{
			name:        "custom deepseek anthropic cannot handle native compact",
			platform:    PlatformDeepseek,
			credentials: map[string]any{"api_protocol": APIProtocolAnthropic, "base_url": "https://relay.example.test"},
		},
		{
			name:        "adaptive uses responses endpoint independently of chat endpoint",
			platform:    PlatformDeepseek,
			credentials: map[string]any{"api_protocol": APIProtocolAdaptive, "base_url": "https://relay.example.test"},
		},
		{
			name:        "adaptive custom responses endpoint stays unknown",
			platform:    PlatformDeepseek,
			credentials: map[string]any{"api_protocol": APIProtocolAdaptive, "api_base_urls": map[string]any{APIProtocolResponses: "https://relay.example.test"}},
			want:        true,
		},
		{
			name:     "grok native pass through keeps existing behavior",
			platform: PlatformGrok,
			want:     true,
		},
		{
			name:        "official deepseek explicit default port",
			platform:    PlatformDeepseek,
			credentials: map[string]any{"api_protocol": APIProtocolResponses, "base_url": "https://API.DEEPSEEK.COM:443/"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			account := &Account{Platform: tt.platform, Type: AccountTypeAPIKey, Credentials: tt.credentials, Extra: tt.extra}
			if got := account.SupportsOpenAIEndpointCapability(OpenAIEndpointCapabilityRemoteCompactionV2); got != tt.want {
				t.Fatalf("remote compaction capability = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSummaryCompactionCapabilityAllowsExplicitNonNativeAccount(t *testing.T) {
	account := &Account{
		Platform: PlatformDeepseek,
		Type:     AccountTypeAPIKey,
		Extra: map[string]any{
			"api_protocol":                      APIProtocolChatCompletions,
			OpenAICompactSummaryEnabledExtraKey: true,
		},
	}
	require.False(t, account.SupportsOpenAIEndpointCapability(OpenAIEndpointCapabilityRemoteCompactionV2))
	require.True(t, account.SupportsOpenAICompactionRequest())
	require.True(t, supportsOpenAIRequestProtocol(account, "deepseek-chat", OpenAIEndpointCapabilityRemoteCompactionV2))
	require.Equal(t, openAICompactionTierUnknown, (&OpenAIGatewayService{}).openAICompactSupportTier(account, "deepseek-chat", openAICompactionKindV2))
	require.True(t, accountSupportsOpenAICapabilities(account, OpenAIEndpointCapabilityRemoteCompactionV2, "", false))

	account.Extra[OpenAICompactSummaryEnabledExtraKey] = false
	require.False(t, account.SupportsOpenAICompactionRequest())
	require.Equal(t, openAICompactionTierUnsupported, (&OpenAIGatewayService{}).openAICompactSupportTier(account, "deepseek-chat", openAICompactionKindV2))
}

func TestSchedulerRemoteCompactionCapability(t *testing.T) {
	for _, engine := range []string{"legacy", "advanced", "load_batch"} {
		for _, tc := range []struct {
			name        string
			platform    string
			credentials map[string]any
			extra       map[string]any
			capability  OpenAIEndpointCapability
			want        bool
		}{
			{"official deepseek compact", PlatformDeepseek, map[string]any{"api_protocol": APIProtocolResponses}, nil, OpenAIEndpointCapabilityRemoteCompactionV2, false},
			{"official deepseek summary", PlatformDeepseek, map[string]any{"api_protocol": APIProtocolResponses}, map[string]any{OpenAICompactSummaryEnabledExtraKey: true}, OpenAIEndpointCapabilityRemoteCompactionV2, true},
			{"official deepseek ordinary", PlatformDeepseek, map[string]any{"api_protocol": APIProtocolResponses}, nil, OpenAIEndpointCapabilityChatCompletions, true},
			{"custom deepseek compact unknown", PlatformDeepseek, map[string]any{"api_protocol": APIProtocolResponses, "base_url": "https://relay.example.test"}, nil, OpenAIEndpointCapabilityRemoteCompactionV2, true},
			{"aggregator chat only", PlatformOpenCodeGo, map[string]any{"api_protocol": APIProtocolChatCompletions}, nil, OpenAIEndpointCapabilityRemoteCompactionV2, false},
			{"aggregator chat summary", PlatformOpenCodeGo, map[string]any{"api_protocol": APIProtocolChatCompletions}, map[string]any{OpenAICompactSummaryEnabledExtraKey: true}, OpenAIEndpointCapabilityRemoteCompactionV2, true},
			{"aggregator anthropic only", PlatformOpenCodeGo, map[string]any{"api_protocol": APIProtocolAnthropic}, nil, OpenAIEndpointCapabilityRemoteCompactionV2, false},
			{"aggregator normal chat conversion", PlatformOpenCodeGo, map[string]any{"api_protocol": APIProtocolChatCompletions}, nil, OpenAIEndpointCapabilityChatCompletions, true},
			{"aggregator mapped protocol", PlatformOpenCodeGo, map[string]any{
				"model_mapping":         map[string]any{"public-model": "gpt-native"},
				"compact_model_mapping": map[string]any{"public-model": "legacy-summary"},
				"protocol_rules":        []any{map[string]any{"pattern": "gpt-native", "protocol": APIProtocolResponses}},
			}, nil, OpenAIEndpointCapabilityRemoteCompactionV2, true},
			{"aggregator mapped chat protocol", PlatformOpenCodeGo, map[string]any{
				"model_mapping":  map[string]any{"public-model": "chat-only"},
				"protocol_rules": []any{map[string]any{"pattern": "chat-only", "protocol": APIProtocolChatCompletions}},
			}, nil, OpenAIEndpointCapabilityRemoteCompactionV2, false},
		} {
			t.Run(engine+"/"+tc.name, func(t *testing.T) {
				resetOpenAIAdvancedSchedulerSettingCacheForTest()
				t.Cleanup(resetOpenAIAdvancedSchedulerSettingCacheForTest)
				groupID := int64(93001)
				svc := newOpenAICompactionSchedulerTestService([]Account{{
					ID: 73001, Platform: tc.platform, Type: AccountTypeAPIKey,
					Status: StatusActive, Schedulable: true, Concurrency: 1,
					Credentials: tc.credentials, Extra: tc.extra,
				}}, engine == "advanced")
				svc.cfg.Gateway.Scheduling.LoadBatchEnabled = engine == "load_batch"
				model := "public-model"
				if tc.platform == PlatformDeepseek {
					model = "deepseek-v4-flash"
				}
				if tc.want {
					reason := openAICompatibleAccountEligibilityFailureReasonBeforeProfit(context.Background(), &Account{
						Platform: tc.platform, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true,
						Concurrency: 1, Credentials: tc.credentials, Extra: tc.extra,
					}, tc.platform, model, false, tc.capability)
					require.Empty(t, reason)
				}
				selection, _, err := svc.SelectAccountWithSchedulerForCapability(
					context.Background(), &groupID, "", "", model, nil,
					OpenAIUpstreamTransportAny, tc.capability, false, false, false, tc.platform,
				)
				if !tc.want {
					require.ErrorIs(t, err, ErrNoAvailableAccounts)
					require.Nil(t, selection)
					return
				}
				require.NoError(t, err)
				require.NotNil(t, selection)
				require.Equal(t, int64(73001), selection.Account.ID)
				if selection.ReleaseFunc != nil {
					selection.ReleaseFunc()
				}
			})
		}
	}
}

// The repository projection round-trip test is the other half of this
// regression: these are the only compaction fields the cache carries.
func TestCompactionSchedulerReadsProjectedSummaryAccount(t *testing.T) {
	for _, engine := range []string{"legacy", "advanced"} {
		for _, protocol := range []string{APIProtocolChatCompletions, APIProtocolAnthropic} {
			t.Run(engine+"/"+protocol, func(t *testing.T) {
				resetOpenAIAdvancedSchedulerSettingCacheForTest()
				t.Cleanup(resetOpenAIAdvancedSchedulerSettingCacheForTest)

				account := Account{
					ID:          73002,
					Platform:    PlatformOpenCodeGo,
					Type:        AccountTypeAPIKey,
					Status:      StatusActive,
					Schedulable: true,
					Concurrency: 1,
					Credentials: map[string]any{"api_protocol": protocol},
					Extra: map[string]any{
						OpenAICompactSummaryEnabledExtraKey: true,
						"openai_compact_mode":               OpenAICompactModeForceOff,
						"openai_compact_supported":          false,
					},
				}
				payload, err := json.Marshal(account)
				require.NoError(t, err)
				var projected Account
				require.NoError(t, json.Unmarshal(payload, &projected))

				svc := newOpenAICompactionSchedulerTestService([]Account{projected}, engine == "advanced")
				groupID := int64(93002)
				selection, _, err := svc.SelectAccountWithSchedulerForCapability(
					context.Background(), &groupID, "", "", "gpt-5.6-sol", nil,
					OpenAIUpstreamTransportAny, OpenAIEndpointCapabilityRemoteCompactionV2,
					false, false, false, PlatformOpenCodeGo,
				)
				require.NoError(t, err)
				require.NotNil(t, selection)
				require.Equal(t, int64(73002), selection.Account.ID)
				require.True(t, selection.Account.SupportsOpenAICompactionRequest())
				if selection.ReleaseFunc != nil {
					selection.ReleaseFunc()
				}
			})
		}
	}
}

func TestNativeCompactionSkipsIncompatibleStickyAccount(t *testing.T) {
	for _, advanced := range []bool{false, true} {
		t.Run(map[bool]string{false: "legacy", true: "advanced"}[advanced], func(t *testing.T) {
			resetOpenAIAdvancedSchedulerSettingCacheForTest()
			t.Cleanup(resetOpenAIAdvancedSchedulerSettingCacheForTest)
			accounts := []Account{
				{ID: 73011, Platform: PlatformOpenCodeGo, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true, Concurrency: 1, Credentials: map[string]any{"api_protocol": APIProtocolChatCompletions}},
				{ID: 73012, Platform: PlatformOpenCodeGo, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true, Concurrency: 1, Credentials: map[string]any{"api_protocol": APIProtocolResponses}},
			}
			svc := newOpenAICompactionSchedulerTestService(accounts, advanced)
			svc.cache = &schedulerTestGatewayCache{sessionBindings: map[string]int64{"compact-sticky": 73011}}
			groupID := int64(93011)
			selection, _, err := svc.SelectAccountWithSchedulerForCapability(context.Background(), &groupID, "", "compact-sticky", "gpt-5.6-sol", nil,
				OpenAIUpstreamTransportAny, OpenAIEndpointCapabilityRemoteCompactionV2, false, false, false, PlatformOpenCodeGo)
			require.NoError(t, err)
			require.NotNil(t, selection)
			require.Equal(t, int64(73012), selection.Account.ID)
			if selection.ReleaseFunc != nil {
				selection.ReleaseFunc()
			}
		})
	}
}

func TestNativeCompactionPrefersProbedOpenAIAccount(t *testing.T) {
	for _, engine := range []string{"legacy", "advanced", "load_batch"} {
		t.Run(engine, func(t *testing.T) {
			resetOpenAIAdvancedSchedulerSettingCacheForTest()
			t.Cleanup(resetOpenAIAdvancedSchedulerSettingCacheForTest)
			accounts := []Account{
				{ID: 73021, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true, Concurrency: 1, Priority: 0},
				{ID: 73022, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true, Concurrency: 1, Priority: 100, Credentials: map[string]any{"api_key": "fixture-value"}},
			}
			recordOpenAICompactionTargetEvidence(t, &OpenAIGatewayService{}, &accounts[1], "gpt-5.6-sol", openAICompactionKindV2, OpenAICompactionProbeVerdictSupported)
			svc := newOpenAICompactionSchedulerTestService(accounts, engine == "advanced")
			svc.cfg.Gateway.Scheduling.LoadBatchEnabled = engine == "load_batch"
			groupID := int64(93021)
			selection, _, err := svc.SelectAccountWithSchedulerForCapability(context.Background(), &groupID, "", "", "gpt-5.6-sol", nil,
				OpenAIUpstreamTransportAny, OpenAIEndpointCapabilityRemoteCompactionV2, false, false, false, PlatformOpenAI)
			require.NoError(t, err)
			require.NotNil(t, selection)
			require.Equal(t, int64(73022), selection.Account.ID)
			if selection.ReleaseFunc != nil {
				selection.ReleaseFunc()
			}
		})
	}
}

func TestNativeCompactionUsesMappedModelCatalogProtocol(t *testing.T) {
	account := &Account{ID: 73999, Platform: PlatformCommandCode, Type: AccountTypeAPIKey, Credentials: map[string]any{
		"model_mapping": map[string]any{"public-model": "custom-native"},
		"api_base_urls": map[string]any{APIProtocolChatCompletions: "https://compact-catalog.example.test"},
	}}
	key := modelProtocolCatalogKey(account, buildOpenAIModelsURL(account.GetCNProtocolBaseURL(APIProtocolChatCompletions)))
	t.Cleanup(func() {
		upstreamModelProtocols.mu.Lock()
		delete(upstreamModelProtocols.entries, key)
		upstreamModelProtocols.mu.Unlock()
	})
	for _, protocol := range []string{APIProtocolResponses, APIProtocolAnthropic, APIProtocolChatCompletions} {
		upstreamModelProtocols.store(key, map[string][]string{"custom-native": {protocol}}, nil, time.Now())
		require.Equal(t, protocol == APIProtocolResponses,
			supportsOpenAIRequestProtocol(account, "public-model", OpenAIEndpointCapabilityRemoteCompactionV2))
	}
	upstreamModelProtocols.store(key, map[string][]string{"custom-native": {APIProtocolChatCompletions}}, nil, time.Now().Add(-2*modelProtocolCatalogTTL))
	require.True(t, supportsOpenAIRequestProtocol(account, "public-model", OpenAIEndpointCapabilityRemoteCompactionV2), "stale negative catalog must allow a new observation in Forward")
}

func TestNativeCompactionColdCatalogDoesNotProbeCandidates(t *testing.T) {
	for _, advanced := range []bool{false, true} {
		t.Run(map[bool]string{false: "legacy", true: "advanced"}[advanced], func(t *testing.T) {
			resetOpenAIAdvancedSchedulerSettingCacheForTest()
			t.Cleanup(resetOpenAIAdvancedSchedulerSettingCacheForTest)
			accounts := make([]Account, 3)
			for i := range accounts {
				accounts[i] = Account{ID: int64(74001 + i), Platform: PlatformCommandCode, Type: AccountTypeAPIKey,
					Status: StatusActive, Schedulable: true, Concurrency: 1, Credentials: map[string]any{
						"api_base_urls": map[string]any{APIProtocolChatCompletions: "https://compact-cold-catalog.example.test"},
					}}
			}
			upstream := &httpUpstreamRecorder{err: errors.New("candidate filtering must not fetch a model catalog")}
			svc := newOpenAICompactionSchedulerTestService(accounts, advanced)
			svc.httpUpstream = upstream
			groupID := int64(94001)
			selection, _, err := svc.SelectAccountWithSchedulerForCapability(context.Background(), &groupID, "", "", "new-native-model", nil,
				OpenAIUpstreamTransportAny, OpenAIEndpointCapabilityRemoteCompactionV2, false, false, false, PlatformCommandCode)
			require.NoError(t, err, "cold catalog is unknown, not proof of incompatibility")
			require.NotNil(t, selection)
			require.Empty(t, upstream.requests)
			if selection.ReleaseFunc != nil {
				selection.ReleaseFunc()
			}
		})
	}
}

func TestForwardRejectsUnsupportedNativeCompactionBeforeUpstream(t *testing.T) {
	for _, account := range []Account{
		{Platform: PlatformDeepseek, Type: AccountTypeAPIKey, Credentials: map[string]any{"api_protocol": APIProtocolResponses}},
		{Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Extra: map[string]any{"openai_responses_supported": false}},
		{Platform: PlatformOpenCodeGo, Type: AccountTypeAPIKey, Credentials: map[string]any{"api_protocol": APIProtocolChatCompletions}},
		{Platform: PlatformOpenCodeGo, Type: AccountTypeAPIKey, Credentials: map[string]any{"api_protocol": APIProtocolAnthropic}},
	} {
		t.Run(account.Platform+"/"+account.GetAPIProtocol(), func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
			MarkOpenAINativeCompactionV2(c)
			upstream := &httpUpstreamRecorder{}
			svc := &OpenAIGatewayService{httpUpstream: upstream, cfg: &config.Config{}}
			result, err := svc.Forward(context.Background(), c, &account, []byte(`{"model":"gpt-5.6-sol","stream":true,"input":[{"type":"compaction_trigger"}]}`))
			require.Error(t, err)
			require.Nil(t, result)
			var failoverErr *UpstreamFailoverError
			require.ErrorAs(t, err, &failoverErr)
			require.Equal(t, OpenAINativeCompactionUnsupportedReason, failoverErr.Reason)
			require.True(t, failoverErr.ShouldRetryNextAccount())
			require.False(t, failoverErr.RetryableOnSameAccount)
			require.False(t, failoverErr.ShouldReportAccountScheduleFailure())
			_, _, healthFailure := classifyOpenAIAPIKeyHealthFailure(err)
			require.False(t, healthFailure)
			require.False(t, c.Writer.Written(), "local refusal must leave the response unwritten for bounded failover")
			require.Empty(t, upstream.requests)
		})
	}
}
