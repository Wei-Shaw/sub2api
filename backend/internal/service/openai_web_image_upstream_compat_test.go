package service

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestFreeWebImageUpstreamCompatCatalogSurvivesRetiredChatOnlyMapping(t *testing.T) {
	for _, publicName := range []string{"gpt-5.5", "chat"} {
		for _, enabled := range []bool{true, false} {
			t.Run(publicName+"/enabled="+map[bool]string{true: "true", false: "false"}[enabled], func(t *testing.T) {
				a := webImagePolicyAccount(map[string]any{publicName: "gpt-5.5"})
				a.ID = 270
				a.Extra[AccountExtraChatGPTWebImageEnabled] = enabled
				groupID := int64(101)
				repo := &modelsListAccountRepoStub{byGroup: map[int64][]Account{groupID: {*a}}, all: []Account{*a}}
				models := (&GatewayService{accountRepo: repo}).GetAvailableModels(context.Background(), &groupID, PlatformOpenAI)
				require.NotNil(t, models, "an empty filtered catalog must not restore default chat models")
				require.NotContains(t, models, publicName)
				if enabled {
					require.ElementsMatch(t, freeWebImageTestModelIDs(), models, "retired text entries must not hide independently enabled web images")
				} else {
					require.Empty(t, models)
				}
			})
		}
	}
}

func TestFreeWebImageUpstreamCompatSupportEntryPointsSharePermission(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*Account)
		allow  bool
	}{
		{"enabled", func(*Account) {}, true},
		{"disabled", func(a *Account) { a.Extra[AccountExtraChatGPTWebImageEnabled] = false }, false},
		{"unconfigured", func(a *Account) { a.Extra = nil }, false},
		{"no_consent", func(a *Account) { a.Extra[AccountExtraChatGPTWebImageRegularChat] = false }, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a := webImagePolicyAccount(map[string]any{
				"gpt-image-2.5-flare": "gpt-5.5", // Legacy native image mappings are ignored by the web path.
				"paint":               "gpt-image-2",
				"chat":                "gpt-5.5",
				"live-chat":           "gpt-6-luna",
			})
			tc.change(a)
			for _, model := range []string{"gpt-image-2.5-flare", "gpt-image-1", "paint"} {
				require.Equal(t, tc.allow, a.IsModelSupported(model), model)
				require.Equal(t, tc.allow, supportsOpenAIRequestModel(a, model, false), model)
			}
			require.False(t, a.IsModelSupported("chat"))
			require.False(t, supportsOpenAIRequestModel(a, "chat", false))
			require.True(t, a.IsModelSupported("live-chat"), "image permission must not change ordinary live chat support")
			require.True(t, supportsOpenAIRequestModel(a, "live-chat", false))
		})
	}
}

func TestFreeWebImageUpstreamCompatCompactUsesActualMappedTarget(t *testing.T) {
	for _, tc := range []struct {
		name, ordinary, compact string
		wantOrdinary            bool
		wantCompact             bool
	}{
		{"only_compact_retired", "gpt-6-luna", "gpt-5.5", true, false},
		{"compact_replaces_retired", "gpt-5.5", "gpt-6-luna", false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a := webImageSchedulingAccount(270)
			a.Credentials["model_mapping"] = map[string]any{"chat": tc.ordinary}
			a.Credentials["compact_model_mapping"] = map[string]any{"chat": tc.compact}
			require.Equal(t, tc.wantOrdinary, a.IsModelSupported("chat"))
			for _, compact := range []bool{false, true} {
				want := tc.wantOrdinary
				if compact {
					want = tc.wantCompact
				}
				require.Equal(t, want, supportsOpenAIRequestModel(&a, "chat", compact))
				ok, reason := (&defaultOpenAIAccountScheduler{}).isAccountRequestCompatibleReason(context.Background(), &a, OpenAIAccountScheduleRequest{RequestedModel: "chat", RequireCompact: compact})
				require.Equal(t, want, ok, reason)
				reason = openAICompatibleAccountEligibilityFailureReasonBeforeProfit(context.Background(), &a, PlatformOpenAI, "chat", compact, "")
				require.Equal(t, want, reason == "", reason)
			}
		})
	}
}

func TestFreeWebImageUpstreamCompatProjectionRetiresAfterAliases(t *testing.T) {
	for _, codex := range []bool{false, true} {
		field, idField := "data", "id"
		if codex {
			field, idField = "models", "slug"
		}
		t.Run(field, func(t *testing.T) {
			a := webImagePolicyAccount(map[string]any{
				"legacy":      "gpt-5.5",
				"gpt-5.5":     "gpt-6.1-sol",
				"gpt-6.1-sol": "gpt-5.5",
			})
			body, err := json.Marshal(map[string]any{field: []map[string]any{
				{idField: "gpt-5.5", "source": "retired"},
				{idField: "gpt-6.1-sol", "source": "live"},
			}, "unknown_top": true})
			require.NoError(t, err)
			original := string(body)
			projected, err := projectAccountModelsBody(body, a, nil, codex)
			require.NoError(t, err)
			require.Equal(t, original, string(body), "the cached upstream catalog must stay unchanged")
			require.True(t, gjson.GetBytes(projected, "unknown_top").Bool())
			ids := make([]string, 0)
			for _, entry := range gjson.GetBytes(projected, field).Array() {
				id := entry.Get(idField).String()
				ids = append(ids, id)
				if id == "gpt-5.5" {
					require.Equal(t, "live", entry.Get("source").String(), "the old public name must retain metadata from its live one-hop target")
				}
			}
			want := []string{"gpt-5.5"}
			if !codex {
				want = append(want, freeWebImageTestModelIDs()...)
			}
			require.ElementsMatch(t, want, ids)
		})
	}
}

func TestFreeWebImageUpstreamCompatSizeRoutingRetainsConsentAndChatMappingIndependence(t *testing.T) {
	for _, mode := range openAIImageSizeSchedulerModes {
		for _, model := range []string{"gpt-image-2.6-flare", "gpt-image-2.6-sunburst"} {
			for _, tc := range []struct {
				name, size string
				wantID     int64
			}{
				{"explicit_1k", "1024x1024", 270},
				{"explicit_4k_replaces_sticky_1k", "3840x2160", 271},
				{"auto_prefers_4k", "auto", 271},
			} {
				t.Run(mode.name+"/"+model+"/"+tc.name, func(t *testing.T) {
					resetOpenAIAdvancedSchedulerSettingCacheForTest()
					oneK, fourK, noConsent := webImageSchedulingAccount(270), webImageSchedulingAccount(271), webImageSchedulingAccount(272)
					oneK.Extra[AccountExtraOpenAIImageSizeTiers] = []string{ImageBillingSize1K}
					fourK.Extra[AccountExtraOpenAIImageSizeTiers] = []string{ImageBillingSize4K}
					fourK.Priority = 10
					noConsent.Priority = -10
					noConsent.Extra[AccountExtraChatGPTWebImageRegularChat] = false
					accounts := []Account{oneK, fourK, noConsent}
					for i := range accounts {
						// Image compatibility entries remain independent of the chat
						// allowlist, including newly added upstream image names.
						accounts[i].Credentials["model_mapping"] = map[string]any{"chat": "gpt-6-luna"}
					}
					cache := &schedulerTestGatewayCache{}
					session := ""
					if tc.name == "explicit_4k_replaces_sticky_1k" {
						session = "web-image-size"
						cache.sessionBindings = map[string]int64{"openai:" + session: oneK.ID}
					}
					var acquired []int64
					svc := newOpenAIImageSizeSchedulerTestService(mode, accounts, cache, schedulerTestConcurrencyCache{acquiredIDs: &acquired})
					groupID := int64(101)
					ctx := WithOpenAIImageSizeRouting(context.Background(), tc.size)
					selection, _, err := svc.SelectAccountWithSchedulerForImages(ctx, &groupID, session, model, nil, OpenAIImagesCapabilityNative)
					require.NoError(t, err)
					require.NotNil(t, selection)
					defer selection.ReleaseFunc()
					require.Equal(t, tc.wantID, selection.Account.ID)
					require.Equal(t, []int64{tc.wantID}, acquired, "unconsented and incompatible-size accounts must not acquire a slot")
				})
			}
		}
	}
}
