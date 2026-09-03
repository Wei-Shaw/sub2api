package service

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// 已核实的模型不能共享同一套代码模式和压缩协议；未知型号不能仅凭 GPT 前缀开启能力。
func TestConfiguredCodexToolCapabilitiesByModel(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		model string
		patch any
		hash  any
		mode  any
		lite  bool
	}{
		{"gpt-5.6-sol", "freeform", "3000", "code_mode_only", true},
		{"gpt-5.6-terra", "freeform", "3000", "code_mode_only", true},
		{"gpt-5.6-luna", "freeform", "3000", "code_mode_only", true},
		{"gpt-5.6", "freeform", "3000", "code_mode_only", true},
		{"codex-auto-review", "freeform", "3000", "code_mode_only", true},
		{"gpt-5.5", "freeform", "2911", nil, false},
		{"gpt-5.4", "freeform", "2911", nil, false},
		{"gpt-5.4-mini", "freeform", "2911", nil, false},
		{"gpt-5.3-codex-spark", "freeform", "2911", nil, false},
		{"gpt-5.2", "freeform", nil, nil, false},
		{"openai/GPT_5.4_MINI", "freeform", "2911", nil, false},
		{"gpt-5.5-2026-04-23", "freeform", "2911", nil, false},
		{"gpt-5.4-high", "freeform", "2911", nil, false},
		{"gpt-5.4-unknown", nil, nil, nil, false},
		{"gpt-5.4-nano", nil, nil, nil, false},
		{"gpt-5.5-pro", nil, nil, nil, false},
		{"gpt-5.7", nil, nil, nil, false},
		{"company-coder", nil, nil, nil, false},
		{"claude-sonnet-4-6", nil, nil, nil, false},
	} {
		t.Run(tt.model, func(t *testing.T) {
			body, err := BuildCodexModelsManifest([]string{tt.model})
			require.NoError(t, err)
			model := decodeCodexManifestModels(t, body)[0]
			require.Equal(t, tt.patch, model["apply_patch_tool_type"])
			require.Equal(t, tt.hash, model["comp_hash"])
			require.Equal(t, tt.mode, model["tool_mode"])
			require.Equal(t, tt.lite, model["use_responses_lite"])
		})
	}
}

// 经同步快照往返后仍保留显式 null/false，且不能把上游任意字段作为本地提示词保存。
func TestSyncUpstreamModelCatalogPreservesCodexToolCapabilities(t *testing.T) {
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Body: io.NopCloser(strings.NewReader(`{"models":[{
			"slug":"future-coder","reasoning":false,"input_modalities":["text"],"context_window":64000,
			"apply_patch_tool_type":"function","comp_hash":"provider-v2","tool_mode":null,
			"use_responses_lite":false,"model_messages":{"instructions_template":"do not copy"}
		}]}`)),
	}}
	repo := &upstreamModelMetadataRepoStub{}
	account := newCodexModelsAPIKeyTestAccount("https://provider.example/v1")
	svc := &AccountTestService{accountRepo: repo, httpUpstream: upstream, cfg: upstreamModelSyncTestConfig()}
	catalog, err := svc.SyncUpstreamModelCatalog(context.Background(), account)
	require.NoError(t, err)
	require.Empty(t, catalog.Warnings)
	require.Len(t, upstream.requests, 1)
	require.NotNil(t, repo.updates)
	metadata, ok := account.GetUpstreamModelMetadata("future-coder")
	require.True(t, ok)
	body, err := json.Marshal(metadata)
	require.NoError(t, err)
	require.Contains(t, string(body), `"apply_patch_tool_type":"function"`)
	require.Contains(t, string(body), `"tool_mode":null`)
	require.Contains(t, string(body), `"use_responses_lite":false`)
	require.NotContains(t, string(body), "instructions_template")

	account.Credentials["model_mapping"] = map[string]any{"my-coder": "future-coder"}
	manifest, err := buildCodexModelsManifestForAccounts(PlatformOpenAI, []string{"my-coder"}, []Account{*account}, nil, true)
	require.NoError(t, err)
	model := decodeCodexManifestModels(t, manifest)[0]
	require.Equal(t, "my-coder", model["slug"])
	require.Equal(t, "my-coder", model["display_name"])
	require.Equal(t, "function", model["apply_patch_tool_type"])
	require.Equal(t, "provider-v2", model["comp_hash"])
	require.Nil(t, model["tool_mode"])
	require.Equal(t, false, model["use_responses_lite"])
}

func codexToolCapabilityAccount(t *testing.T, id int64, alias, target, fields string) Account {
	t.Helper()
	account := Account{ID: id, Platform: PlatformOpenAI, Type: AccountTypeAPIKey,
		Credentials: map[string]any{"model_mapping": map[string]any{alias: target}},
	}
	if fields != "" {
		_, metadata, err := extractUpstreamModelCatalog([]byte(fmt.Sprintf(`{"models":[{"slug":%q,%s}]}`, target, fields)), false)
		require.NoError(t, err)
		account.SetUpstreamModelMetadataSnapshot(UpstreamModelMetadataSnapshot{Models: metadata})
	}
	return account
}

// 同名别名可能命中不同模型；能力必须与账号顺序无关，缺失和显式禁用都不能被型号默认值覆盖。
func TestGroupCodexToolCapabilitiesIntersectMappedTargets(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name    string
		alias   string
		targets []string
		fields  []string
		patch   any
		hash    any
		mode    any
		lite    bool
	}{
		{"known alias", "my-coder", []string{"gpt-5.4"}, []string{""}, "freeform", "2911", nil, false},
		{"mixed known models", "my-coder", []string{"gpt-5.6-sol", "gpt-5.5"}, []string{"", ""}, "freeform", nil, nil, false},
		{"unknown mapped target", "gpt-5.6-sol", []string{"gpt-5.6-sol", "unknown-coder"}, []string{"", ""}, nil, nil, nil, false},
		{"upstream overrides defaults", "my-coder", []string{"gpt-5.6-sol"}, []string{`"apply_patch_tool_type":"function","comp_hash":null,"tool_mode":null,"use_responses_lite":false`}, "function", nil, nil, false},
		{"explicitly disabled", "gpt-5.6-sol", []string{"gpt-5.6-sol", "gpt-5.6-sol"}, []string{"", `"apply_patch_tool_type":null,"comp_hash":null,"tool_mode":null,"use_responses_lite":false`}, nil, nil, nil, false},
		{"future model", "my-coder", []string{"future-a", "future-b"}, []string{`"apply_patch_tool_type":"freeform","comp_hash":"future-v1","tool_mode":"code_mode_only","use_responses_lite":true`, `"apply_patch_tool_type":"freeform","comp_hash":"future-v1","tool_mode":"code_mode_only","use_responses_lite":true`}, "freeform", "future-v1", "code_mode_only", true},
		{"incompatible patch formats", "my-coder", []string{"future-a", "future-b"}, []string{`"apply_patch_tool_type":"freeform","tool_mode":"code_mode_only","use_responses_lite":true`, `"apply_patch_tool_type":"function","tool_mode":"code_mode_only","use_responses_lite":true`}, nil, nil, nil, false},
		{"missing future metadata", "my-coder", []string{"future-a", "future-b"}, []string{`"apply_patch_tool_type":"freeform","tool_mode":"code_mode_only","use_responses_lite":true`, ""}, nil, nil, nil, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			accounts := make([]Account, len(tt.targets))
			for i, target := range tt.targets {
				accounts[i] = codexToolCapabilityAccount(t, int64(i+1), tt.alias, target, tt.fields[i])
			}
			for range 2 {
				body, err := buildCodexModelsManifestForAccounts(PlatformOpenAI, []string{tt.alias}, accounts, nil, true)
				require.NoError(t, err)
				model := decodeCodexManifestModels(t, body)[0]
				require.Equal(t, tt.patch, model["apply_patch_tool_type"])
				require.Equal(t, tt.hash, model["comp_hash"])
				require.Equal(t, tt.mode, model["tool_mode"])
				require.Equal(t, tt.lite, model["use_responses_lite"])
				accounts[0], accounts[len(accounts)-1] = accounts[len(accounts)-1], accounts[0]
			}
		})
	}
}

// 原生目录及普通 /v1/models 转换都遵循：实时上游 > 已同步快照 > 已核实型号默认值。
func TestCompleteAPIKeyCodexToolCapabilitiesPreserveUpstreamPrecedence(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name      string
		body      string
		converted bool
		patch     any
		hash      any
		mode      any
		lite      bool
	}{
		{"native explicit null", `{"models":[{"slug":"gpt-5.5","apply_patch_tool_type":null,"comp_hash":null,"tool_mode":null,"use_responses_lite":false}]}`, false, nil, nil, nil, false},
		{"native synced fallback", `{"models":[{"slug":"gpt-5.5"}]}`, false, "function", "synced-v1", "code_mode_only", true},
		{"converted synced fallback", `{"data":[{"id":"gpt-5.5"}]}`, true, "function", "synced-v1", "code_mode_only", true},
		{"converted live fields", `{"data":[{"id":"gpt-5.5","apply_patch_tool_type":"freeform","comp_hash":"live-v2","tool_mode":null,"use_responses_lite":false}]}`, true, "freeform", "live-v2", nil, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			account := codexToolCapabilityAccount(t, 1, "gpt-5.5", "gpt-5.5", `"apply_patch_tool_type":"function","comp_hash":"synced-v1","tool_mode":"code_mode_only","use_responses_lite":true`)
			account.Credentials["base_url"] = "https://provider.example/v1"
			manifest := &CodexModelsManifest{Body: []byte(tt.body), upstreamSourceBody: []byte(tt.body), convertedFromOpenAIModelList: tt.converted}
			svc := &OpenAIGatewayService{}
			require.NoError(t, svc.CompleteAPIKeyCodexModelsManifestForClient(manifest, &account))
			model := decodeCodexManifestModels(t, manifest.Body)[0]
			require.Equal(t, tt.patch, model["apply_patch_tool_type"])
			require.Equal(t, tt.hash, model["comp_hash"])
			require.Equal(t, tt.mode, model["tool_mode"])
			require.Equal(t, tt.lite, model["use_responses_lite"])
			require.Equal(t, codexModelsManifestBodyETag(manifest.Body), manifest.ETag)
		})
	}
}
