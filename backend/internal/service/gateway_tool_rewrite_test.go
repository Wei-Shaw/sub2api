package service

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func buildTestToolNameRewrite(body []byte) *ToolNameRewrite {
	return buildToolNameRewriteFromBodyWithServerName(body, "tools")
}

func TestBuildMCPToolMap_StableForServerName(t *testing.T) {
	names := []string{"alpha_search", "beta_lookup", "gamma_fetch"}
	first := buildMCPToolMap(names, "tools")
	second := buildMCPToolMap(names, "tools")

	require.Equal(t, first, second)
	require.Len(t, first, len(names))
	for _, name := range names {
		alias := first[name]
		require.True(t, isMCPToolName(alias), alias)
		require.True(t, strings.HasPrefix(alias, "mcp__tools__"), alias)
		require.Contains(t, alias, name)
	}
}

func TestBuildMCPToolMap_SanitizesAndAvoidsCollisions(t *testing.T) {
	names := []string{"a name", "a/name", strings.Repeat("long", 40)}
	mapping := buildMCPToolMap(names, "tools")

	require.Len(t, mapping, len(names))
	for _, name := range names {
		require.True(t, isMCPToolName(mapping[name]), mapping[name])
		require.LessOrEqual(t, len(mapping[name]), 64)
	}
	require.NotEqual(t, mapping["a name"], mapping["a/name"])
	require.Contains(t, mapping[strings.Repeat("long", 40)], "_")
}

func TestBuildMCPToolMap_PreservesExistingMCPNames(t *testing.T) {
	names := []string{"mcp__files__read", "read"}
	mapping := buildMCPToolMap(names, "tools")

	require.NotContains(t, mapping, "mcp__files__read")
	require.Equal(t, "mcp__tools__read", mapping["read"])
}

func TestRestoreToolNamesInBytes_ExactNames(t *testing.T) {
	rw := &ToolNameRewrite{Reverse: map[string]string{"abc_12": "foo", "abc_12_ext": "bar"}}
	data := []byte(`{"type":"message","content":[{"type":"tool_use","name":"abc_12_ext"},{"type":"tool_use","name":"abc_12"},{"type":"tool_use","name":"abc_12_other"}]}`)
	restored := string(restoreToolNamesInBytes(data, rw))
	require.Equal(t, `{"type":"message","content":[{"type":"tool_use","name":"bar"},{"type":"tool_use","name":"foo"},{"type":"tool_use","name":"abc_12_other"}]}`, restored)
}

func TestRestoreToolNamesInBytes_NoMappingIsNoop(t *testing.T) {
	data := []byte(`{"name":"sessions_list","id":"cc_ses_xyz"}`)
	require.Equal(t, string(data), string(restoreToolNamesInBytes(data, nil)))
}

func TestApplyToolNameRewriteToBody_RenamesToolsAndToolChoice(t *testing.T) {
	body := []byte(`{"tools":[{"name":"sessions_list","input_schema":{}},{"name":"session_get","input_schema":{}},{"name":"web_search","type":"web_search_20250305"}],"tool_choice":{"type":"tool","name":"sessions_list"}}`)
	rw := buildTestToolNameRewrite(body)
	require.NotNil(t, rw)
	require.Contains(t, rw.Forward, "sessions_list")
	require.Contains(t, rw.Forward, "session_get")
	require.NotContains(t, rw.Forward, "web_search")

	out := applyToolNameRewriteToBody(body, rw)
	require.Equal(t, "mcp__tools__sessions_list", gjson.GetBytes(out, "tools.0.name").String())
	require.Equal(t, "mcp__tools__session_get", gjson.GetBytes(out, "tools.1.name").String())
	require.Equal(t, "web_search", gjson.GetBytes(out, "tools.2.name").String())
	require.Equal(t, "mcp__tools__sessions_list", gjson.GetBytes(out, "tool_choice.name").String())
	require.Equal(t, "tool", gjson.GetBytes(out, "tool_choice.type").String())
}

func TestApplyToolNameRewriteToBody_RenamesToolUseInMessages(t *testing.T) {
	body := []byte(`{"tools":[{"name":"sessions_list","input_schema":{}},{"name":"web_search","type":"web_search_20250305"}],"messages":[{"role":"user","content":[{"type":"text","text":"hi"}]},{"role":"assistant","content":[{"type":"tool_use","id":"tu_01","name":"sessions_list","input":{}},{"type":"text","text":"thinking"}]},{"role":"user","content":[{"type":"tool_result","tool_use_id":"tu_01","content":"ok"}]}]}`)
	rw := buildTestToolNameRewrite(body)
	require.NotNil(t, rw)
	require.Equal(t, "mcp__tools__sessions_list", rw.Forward["sessions_list"])

	out := applyToolNameRewriteToBody(body, rw)
	require.Equal(t, "mcp__tools__sessions_list", gjson.GetBytes(out, "tools.0.name").String())
	require.Equal(t, "web_search", gjson.GetBytes(out, "tools.1.name").String())
	require.Equal(t, "mcp__tools__sessions_list", gjson.GetBytes(out, "messages.1.content.0.name").String())
	require.Equal(t, "thinking", gjson.GetBytes(out, "messages.1.content.1.text").String())
	require.Equal(t, "ok", gjson.GetBytes(out, "messages.2.content.0.content").String())
}

func TestApplyToolNameRewriteToBody_RenamesAllToolCounts(t *testing.T) {
	body := []byte(`{"tools":[{"name":"alpha_search","input_schema":{}},{"name":"beta_lookup","input_schema":{}},{"name":"gamma_fetch","input_schema":{}},{"name":"delta_update","input_schema":{}},{"name":"epsilon_parse","input_schema":{}},{"name":"zeta_render","input_schema":{}},{"name":"web_search","type":"web_search_20250305"}],"tool_choice":{"type":"tool","name":"gamma_fetch"},"messages":[{"role":"assistant","content":[{"type":"tool_use","id":"tu_dyn","name":"gamma_fetch","input":{}},{"type":"tool_use","id":"tu_srv","name":"web_search","input":{}},{"type":"text","text":"done"}]}]}`)
	rw := buildTestToolNameRewrite(body)
	require.NotNil(t, rw)
	require.Len(t, rw.Forward, 6)

	fakeGamma := rw.Forward["gamma_fetch"]
	require.Equal(t, "mcp__tools__gamma_fetch", fakeGamma)
	out := applyToolNameRewriteToBody(body, rw)
	require.Equal(t, fakeGamma, gjson.GetBytes(out, "tools.2.name").String())
	require.Equal(t, fakeGamma, gjson.GetBytes(out, "tool_choice.name").String())
	require.Equal(t, fakeGamma, gjson.GetBytes(out, "messages.0.content.0.name").String())
	require.Equal(t, "web_search", gjson.GetBytes(out, "tools.6.name").String())
	require.Equal(t, "web_search", gjson.GetBytes(out, "messages.0.content.1.name").String())
}

func TestApplyToolsLastCacheBreakpoint_InjectsDefault(t *testing.T) {
	body := []byte(`{"tools":[{"name":"a","input_schema":{}},{"name":"b","input_schema":{}}]}`)
	out := applyToolsLastCacheBreakpoint(body)
	require.Equal(t, "ephemeral", gjson.GetBytes(out, "tools.1.cache_control.type").String())
	require.Equal(t, "5m", gjson.GetBytes(out, "tools.1.cache_control.ttl").String())
	// First tool untouched
	require.False(t, gjson.GetBytes(out, "tools.0.cache_control").Exists())
}

func TestApplyToolsLastCacheBreakpoint_PassesThroughClientTTL(t *testing.T) {
	body := []byte(`{"tools":[{"name":"a","input_schema":{},"cache_control":{"type":"ephemeral","ttl":"1h"}}]}`)
	out := applyToolsLastCacheBreakpoint(body)
	// User-provided ttl must be preserved.
	require.Equal(t, "1h", gjson.GetBytes(out, "tools.0.cache_control.ttl").String())
}

func TestApplyToolsLastCacheBreakpoint_StripsDeferredToolCacheControl(t *testing.T) {
	body := []byte(`{"tools":[{"name":"a","custom":{"defer_loading":true},"cache_control":{"type":"ephemeral","ttl":"1h"}},{"name":"b","custom":{"defer_loading":true}}]}`)
	out := applyToolsLastCacheBreakpoint(body)

	require.False(t, gjson.GetBytes(out, "tools.0.cache_control").Exists())
	require.False(t, gjson.GetBytes(out, "tools.1.cache_control").Exists())
}

func TestApplyToolsLastCacheBreakpoint_SkipsDeferredFinalTool(t *testing.T) {
	body := []byte(`{"tools":[{"name":"a","input_schema":{}},{"name":"b","defer_loading":true}]}`)
	out := applyToolsLastCacheBreakpoint(body)

	require.Equal(t, "ephemeral", gjson.GetBytes(out, "tools.0.cache_control.type").String())
	require.Equal(t, "5m", gjson.GetBytes(out, "tools.0.cache_control.ttl").String())
	require.False(t, gjson.GetBytes(out, "tools.1.cache_control").Exists())
}

func TestApplyToolsLastCacheBreakpoint_OnlyLiteralTrueIsDeferred(t *testing.T) {
	body := []byte(`{"tools":[{"name":"custom-true","custom":{"defer_loading":true},"cache_control":{"type":"ephemeral"}},{"name":"top-level-true","defer_loading":true,"cache_control":{"type":"ephemeral"}},{"name":"false","defer_loading":false,"cache_control":{"type":"ephemeral"}},{"name":"string","defer_loading":"true","cache_control":{"type":"ephemeral"}},{"name":"number","defer_loading":1,"cache_control":{"type":"ephemeral"}},{"name":"object","defer_loading":{},"cache_control":{"type":"ephemeral"}}]}`)
	out := stripDeferredToolCacheControl(body)

	require.False(t, gjson.GetBytes(out, "tools.0.cache_control").Exists())
	require.False(t, gjson.GetBytes(out, "tools.1.cache_control").Exists())
	for idx := 2; idx < 6; idx++ {
		require.Equal(t, "ephemeral", gjson.GetBytes(out, fmt.Sprintf("tools.%d.cache_control.type", idx)).String())
	}
}

func TestStripMessageCacheControl(t *testing.T) {
	body := []byte(`{"messages":[{"role":"user","content":[{"type":"text","text":"hi","cache_control":{"type":"ephemeral"}}]}]}`)
	out := stripMessageCacheControl(body)
	require.False(t, gjson.GetBytes(out, "messages.0.content.0.cache_control").Exists())
}

func TestAddMessageCacheBreakpoints_LastMessageOnly(t *testing.T) {
	body := []byte(`{"messages":[{"role":"user","content":[{"type":"text","text":"hello"}]}]}`)
	out := addMessageCacheBreakpoints(body)
	require.Equal(t, "ephemeral", gjson.GetBytes(out, "messages.0.content.0.cache_control.type").String())
	require.Equal(t, "5m", gjson.GetBytes(out, "messages.0.content.0.cache_control.ttl").String())
}

func TestAddMessageCacheBreakpoints_SecondToLastUserTurn(t *testing.T) {
	// Parrot 不变量：messages ≥ 4 时才打第二个断点，且位置是"倒数第二个 user turn"。
	body := []byte(`{"messages":[
        {"role":"user","content":[{"type":"text","text":"q1"}]},
        {"role":"assistant","content":[{"type":"text","text":"a1"}]},
        {"role":"user","content":[{"type":"text","text":"q2"}]},
        {"role":"assistant","content":[{"type":"text","text":"a2"}]}
    ]}`)
	out := addMessageCacheBreakpoints(body)
	// 最后一条 assistant 被打断点
	require.Equal(t, "ephemeral", gjson.GetBytes(out, "messages.3.content.0.cache_control.type").String())
	// 倒数第二个 user turn = index 0（唯一另一个 user）
	require.Equal(t, "ephemeral", gjson.GetBytes(out, "messages.0.content.0.cache_control.type").String())
	// 其他不打断点
	require.False(t, gjson.GetBytes(out, "messages.1.content.0.cache_control").Exists())
	require.False(t, gjson.GetBytes(out, "messages.2.content.0.cache_control").Exists())
}

func TestAddMessageCacheBreakpoints_StringContentPromoted(t *testing.T) {
	body := []byte(`{"messages":[{"role":"user","content":"hi"}]}`)
	out := addMessageCacheBreakpoints(body)
	// content 升级成数组
	require.True(t, gjson.GetBytes(out, "messages.0.content").IsArray())
	require.Equal(t, "text", gjson.GetBytes(out, "messages.0.content.0.type").String())
	require.Equal(t, "hi", gjson.GetBytes(out, "messages.0.content.0.text").String())
	require.Equal(t, "5m", gjson.GetBytes(out, "messages.0.content.0.cache_control.ttl").String())
}

func TestRewriteMessageCacheControlIfEnabled_DefaultKeepsClientAnchors(t *testing.T) {
	body := []byte(`{"messages":[
		{"role":"user","content":[{"type":"text","text":"stable","cache_control":{"type":"ephemeral","ttl":"1h"}}]},
		{"role":"assistant","content":[{"type":"text","text":"ok"}]},
		{"role":"user","content":[{"type":"text","text":"latest","cache_control":{"type":"ephemeral","ttl":"5m"}}]}
	]}`)

	out := (&GatewayService{}).rewriteMessageCacheControlIfEnabled(context.Background(), body)

	require.JSONEq(t, string(body), string(out))
	require.Equal(t, "1h", gjson.GetBytes(out, "messages.0.content.0.cache_control.ttl").String())
	require.Equal(t, "5m", gjson.GetBytes(out, "messages.2.content.0.cache_control.ttl").String())
}

func TestRewriteMessageCacheControlIfEnabled_OptInPreservesLegacyRewrite(t *testing.T) {
	body := []byte(`{"messages":[
		{"role":"user","content":[{"type":"text","text":"stable","cache_control":{"type":"ephemeral","ttl":"1h"}}]},
		{"role":"assistant","content":[{"type":"text","text":"ok"}]},
		{"role":"user","content":[{"type":"text","text":"latest","cache_control":{"type":"ephemeral","ttl":"1h"}}]},
		{"role":"assistant","content":[{"type":"text","text":"done"}]}
	]}`)
	repo := &gatewayTTLSettingRepo{data: map[string]string{
		SettingKeyRewriteMessageCacheControl: "true",
	}}
	gatewayForwardingCache.Store(&cachedGatewayForwardingSettings{})
	svc := &GatewayService{settingService: NewSettingService(repo, &config.Config{})}

	out := svc.rewriteMessageCacheControlIfEnabled(context.Background(), body)

	require.Equal(t, "5m", gjson.GetBytes(out, "messages.0.content.0.cache_control.ttl").String())
	require.False(t, gjson.GetBytes(out, "messages.2.content.0.cache_control").Exists())
	require.Equal(t, "5m", gjson.GetBytes(out, "messages.3.content.0.cache_control.ttl").String())
}

func TestBuildToolNameRewriteFromBody_ReverseRoundTrip(t *testing.T) {
	body := []byte(`{"tools":[{"name":"read"},{"name":"read_file"},{"name":"mcp__tools__read"}]}`)
	rw := buildTestToolNameRewrite(body)
	require.NotNil(t, rw)
	require.Len(t, rw.Forward, 2)
	require.Len(t, rw.Reverse, 2)
	for original, alias := range rw.Forward {
		require.Equal(t, original, rw.Reverse[alias])
		require.NotEqual(t, "mcp__tools__read", alias)
	}
}

func TestApplyToolNameRewriteToBody_Spans(t *testing.T) {
	body := []byte(`{"tools":[{"name":"sessions_\"quoted"},{"name":"session_last","defer_loading":true,"cache_control":{"type":"ephemeral"}}],"tool_choice":{"type":"tool","name":"sessions_\"quoted"},"messages":[{"content":[{"type":"tool_use","name":"sessions_\"quoted","input":{"name":"sessions_\"quoted"}},{"type":"tool_use"}]}],"metadata":{"name":"sessions_\"quoted"}}`)
	rw := buildTestToolNameRewrite(body)
	out := applyToolNameRewriteToBody(body, rw)
	require.True(t, gjson.ValidBytes(out))
	for _, path := range []string{"tools.0.name", "tool_choice.name", "messages.0.content.0.name"} {
		require.Equal(t, rw.Forward[`sessions_"quoted`], gjson.GetBytes(out, path).String())
	}
	require.Equal(t, `sessions_"quoted`, gjson.GetBytes(out, "messages.0.content.0.input.name").String())
	require.Equal(t, `sessions_"quoted`, gjson.GetBytes(out, "metadata.name").String())
	require.False(t, gjson.GetBytes(out, "tools.1.cache_control").Exists())
	require.Equal(t, "5m", gjson.GetBytes(out, "tools.0.cache_control.ttl").String())
	require.False(t, gjson.GetBytes(out, "messages.0.content.1.name").Exists())
}

func TestApplyToolNameRewriteToBody_DuplicateNames(t *testing.T) {
	body := []byte(`{"tools":[{"name":"sessions_first","name":"sessions_second"}],"messages":[{"content":[{"type":"tool_use","name":"sessions_first","name":"sessions_second"}]}]}`)
	rw := buildTestToolNameRewrite(body)
	out := applyToolNameRewriteToBody(body, rw)
	require.True(t, gjson.ValidBytes(out))
	require.Equal(t, 2, strings.Count(string(out), `"name":"`+rw.Forward["sessions_first"]+`","name":"sessions_second"`))
}

func TestApplyToolNameRewriteToBody_NonStringReferences(t *testing.T) {
	body := []byte(`{"tools":[{"name":"123"}],"tool_choice":{"type":"tool","name":123},"messages":[{"content":[{"type":"tool_use","name":123},{"type":"tool_use","name":true}]}]}`)
	rw := &ToolNameRewrite{Forward: map[string]string{"123": "replacement", "true": "also_replaced"}}
	out := applyToolNameRewriteToBody(body, rw)
	require.True(t, gjson.ValidBytes(out))
	require.Equal(t, "replacement", gjson.GetBytes(out, "tools.0.name").String())
	require.Equal(t, `"replacement"`, gjson.GetBytes(out, "tool_choice.name").Raw)
	require.Equal(t, `"replacement"`, gjson.GetBytes(out, "messages.0.content.0.name").Raw)
	require.Equal(t, `"also_replaced"`, gjson.GetBytes(out, "messages.0.content.1.name").Raw)
}

func makeLongToolHistory() []byte {
	var b strings.Builder
	_, _ = b.WriteString(`{"tools":[{"name":"sessions_list"}],"tool_choice":{"type":"tool","name":"sessions_list"},"messages":[`)
	for i := 0; i < 1000; i++ {
		if i > 0 {
			_ = b.WriteByte(',')
		}
		_, _ = b.WriteString(`{"content":[{"type":"tool_use","name":"sessions_list","input":{"name":"sessions_list"}}]}`)
	}
	_, _ = b.WriteString(`]}`)
	return []byte(b.String())
}

func TestApplyToolNameRewriteToBody_LongHistory(t *testing.T) {
	body := makeLongToolHistory()
	out := applyToolNameRewriteToBody(body, buildTestToolNameRewrite(body))
	require.True(t, gjson.ValidBytes(out))
	require.Equal(t, "mcp__tools__sessions_list", gjson.GetBytes(out, "messages.999.content.0.name").String())
	require.Equal(t, "sessions_list", gjson.GetBytes(out, "messages.999.content.0.input.name").String())
}

func BenchmarkApplyToolNameRewriteToBody_LongHistory(b *testing.B) {
	body := makeLongToolHistory()
	rw := buildTestToolNameRewrite(body)
	b.ReportAllocs()
	b.SetBytes(int64(len(body)))
	for i := 0; i < b.N; i++ {
		_ = applyToolNameRewriteToBody(body, rw)
	}
}

// Issue #1963: Claude Code style tool search returns tool_reference blocks in
// tool_result.content. They must follow the renamed tools[] entries.
func TestApplyToolNameRewriteToBody_ToolResultToolReference(t *testing.T) {
	body := []byte(`{"tools":[{"name":"Agent","input_schema":{}},{"name":"Bash","input_schema":{}},{"name":"Edit","input_schema":{}},{"name":"Read","input_schema":{}},{"name":"Skill","input_schema":{}},{"name":"ToolSearch","input_schema":{}},{"name":"WebFetch","input_schema":{},"defer_loading":true},{"name":"Write","input_schema":{}}],"messages":[{"role":"assistant","content":[{"type":"tool_use","id":"toolu_01","name":"ToolSearch","input":{"query":"select:WebFetch"}}]},{"role":"user","content":[{"type":"tool_result","tool_use_id":"toolu_01","content":[{"type":"tool_reference","tool_name":"WebFetch"}]}]}]}`)
	rw := buildTestToolNameRewrite(body)
	require.NotNil(t, rw)
	fakeWebFetch := rw.Forward["WebFetch"]
	require.NotEmpty(t, fakeWebFetch)

	out := applyToolNameRewriteToBody(body, rw)

	require.True(t, gjson.ValidBytes(out))
	require.Equal(t, fakeWebFetch, gjson.GetBytes(out, "tools.6.name").String())
	require.Equal(t, rw.Forward["ToolSearch"], gjson.GetBytes(out, "messages.0.content.0.name").String())
	require.Equal(t, fakeWebFetch, gjson.GetBytes(out, "messages.1.content.0.content.0.tool_name").String())
	require.Equal(t, "select:WebFetch", gjson.GetBytes(out, "messages.0.content.0.input.query").String())
}

func TestApplyToolNameRewriteToBody_ToolSearchResultReferences(t *testing.T) {
	body := []byte(`{"tools":[{"type":"tool_search_tool_regex_20251119","name":"tool_search_tool_regex"},{"name":"get_weather","input_schema":{},"defer_loading":true},{"name":"search_files","input_schema":{},"defer_loading":true},{"name":"alpha","input_schema":{}},{"name":"beta","input_schema":{}},{"name":"gamma","input_schema":{}},{"name":"delta","input_schema":{}}],"messages":[{"role":"user","content":"weather?"},{"role":"assistant","content":[{"type":"server_tool_use","id":"srvtoolu_01","name":"tool_search_tool_regex","input":{"pattern":"weather"}},{"type":"tool_search_tool_result","tool_use_id":"srvtoolu_01","content":{"type":"tool_search_tool_search_result","tool_references":[{"type":"tool_reference","tool_name":"get_weather"},{"type":"tool_reference","tool_name":"search_files"}]}},{"type":"tool_use","id":"toolu_01","name":"get_weather","input":{"location":"SF"}}]}]}`)
	rw := buildTestToolNameRewrite(body)
	require.NotNil(t, rw)
	require.Len(t, rw.Forward, 6)
	require.NotContains(t, rw.Forward, "tool_search_tool_regex")

	out := applyToolNameRewriteToBody(body, rw)

	require.True(t, gjson.ValidBytes(out))
	refs := "messages.1.content.1.content.tool_references"
	require.Equal(t, rw.Forward["get_weather"], gjson.GetBytes(out, refs+".0.tool_name").String())
	require.Equal(t, rw.Forward["search_files"], gjson.GetBytes(out, refs+".1.tool_name").String())
	require.Equal(t, rw.Forward["get_weather"], gjson.GetBytes(out, "messages.1.content.2.name").String())
	require.Equal(t, "tool_search_tool_regex", gjson.GetBytes(out, "tools.0.name").String())
	require.Equal(t, "tool_search_tool_regex", gjson.GetBytes(out, "messages.1.content.0.name").String())
}

func TestApplyToolNameRewriteToBody_ToolReferencesUseMCPMapping(t *testing.T) {
	body := []byte(`{"tools":[{"name":"sessions_list","input_schema":{}},{"name":"session_get","input_schema":{}},{"name":"bash","input_schema":{}}],"messages":[{"role":"assistant","content":[{"type":"tool_search_tool_result","tool_use_id":"srvtoolu_01","content":{"type":"tool_search_tool_search_result","tool_references":[{"type":"tool_reference","tool_name":"session_get"},{"type":"tool_reference","tool_name":"bash"}]}}]},{"role":"user","content":[{"type":"tool_result","tool_use_id":"toolu_01","content":[{"type":"tool_reference","tool_name":"sessions_list"}]}]}]}`)
	rw := buildTestToolNameRewrite(body)
	require.NotNil(t, rw)
	require.Len(t, rw.Forward, 3)

	out := applyToolNameRewriteToBody(body, rw)

	require.True(t, gjson.ValidBytes(out))
	refs := "messages.0.content.0.content.tool_references"
	require.Equal(t, rw.Forward["session_get"], gjson.GetBytes(out, refs+".0.tool_name").String())
	require.Equal(t, rw.Forward["bash"], gjson.GetBytes(out, refs+".1.tool_name").String())
	require.Equal(t, rw.Forward["sessions_list"], gjson.GetBytes(out, "messages.1.content.0.content.0.tool_name").String())
}

func TestApplyToolNameRewriteToBody_ToolReferencesKeepUnknownAndServerNames(t *testing.T) {
	body := []byte(`{"tools":[{"type":"web_search_20250305","name":"web_search"},{"type":"tool_search_tool_bm25_20251119","name":"tool_search_tool_bm25"},{"name":"alpha","input_schema":{}},{"name":"beta","input_schema":{}},{"name":"gamma","input_schema":{}},{"name":"delta","input_schema":{}},{"name":"epsilon","input_schema":{}},{"name":"zeta","input_schema":{}}],"messages":[{"role":"assistant","content":[{"type":"tool_search_tool_result","tool_use_id":"srvtoolu_01","content":{"type":"tool_search_tool_search_result","tool_references":[{"type":"tool_reference","tool_name":"web_search"},{"type":"tool_reference","tool_name":"missing_tool"}]}}]},{"role":"user","content":[{"type":"tool_result","tool_use_id":"toolu_01","content":[{"type":"tool_reference","tool_name":"tool_search_tool_bm25"},{"type":"tool_reference","tool_name":"Alpha"}]}]}]}`)
	rw := buildTestToolNameRewrite(body)
	require.NotNil(t, rw)
	require.Len(t, rw.Forward, 6)

	out := applyToolNameRewriteToBody(body, rw)

	require.True(t, gjson.ValidBytes(out))
	require.Equal(t, gjson.GetBytes(body, "messages").Raw, gjson.GetBytes(out, "messages").Raw)
}

func TestApplyToolNameRewriteToBody_ToolReferencesLeaveOpaquePayloadsByteIdentical(t *testing.T) {
	names := []string{"WebFetch", "Read", "Bash", "Grep", "Glob", "Write"}
	// @Name@ marks the only spans that may change. Every other byte, including
	// look-alike tool_reference objects outside protocol containers, must survive.
	template := `{"tools":[{"type":"tool_search_tool_regex_20251119","name":"tool_search_tool_regex"},` +
		`{"name":"@WebFetch@","input_schema":{"type":"object","properties":{"tool_name":{"type":"string","default":"WebFetch"}},"examples":[{"type":"tool_reference","tool_name":"WebFetch"}]},"defer_loading":true},` +
		`{"name":"@Read@","input_schema":{}},{"name":"@Bash@","input_schema":{}},{"name":"@Grep@","input_schema":{}},{"name":"@Glob@","input_schema":{}},` +
		`{"name":"@Write@","input_schema":{},"cache_control":{"type":"ephemeral","ttl":"5m"}}],` +
		`"messages":[{"role":"user","content":"Find WebFetch"},` +
		`{"role":"assistant","content":[` +
		`{"type":"thinking","thinking":"use tool_reference WebFetch","signature":"sig-WebFetch"},` +
		`{"type":"text","text":"{\"type\":\"tool_reference\",\"tool_name\":\"WebFetch\"}"},` +
		`{"type":"tool_reference","tool_name":"WebFetch"},` +
		`{"type":"server_tool_use","id":"srvtoolu_01","name":"tool_search_tool_regex","input":{"pattern":"WebFetch"}},` +
		`{"type":"tool_search_tool_result","tool_use_id":"srvtoolu_01","content":{"type":"tool_search_tool_search_result","tool_references":[{"type":"tool_reference","tool_name":"@Glob@"},{"type":"tool_reference","tool_name":"@Write@"}]}},` +
		`{"type":"tool_search_tool_result","tool_use_id":"srvtoolu_02","content":{"type":"tool_search_tool_result_error","error_code":"unavailable","tool_references":[{"type":"tool_reference","tool_name":"WebFetch"}]}},` +
		`{"type":"tool_use","id":"toolu_01","name":"@Read@","input":{"type":"tool_reference","tool_name":"WebFetch","content":[{"type":"tool_reference","tool_name":"WebFetch"}]}}]},` +
		`{"role":"user","content":[` +
		`{"type":"tool_result","tool_use_id":"toolu_01","content":[{"type":"tool_reference","tool_name":"@WebFetch@"},{"type":"text","text":"WebFetch","tool_name":"WebFetch"},{"type":"tool_reference","tool_name":"@Grep@"},{"type":"search_result","source":"s","title":"t","content":[{"type":"tool_reference","tool_name":"WebFetch"}]},{"type":"tool_reference","tool_name":"@WebFetch@"}]},` +
		`{"type":"tool_result","tool_use_id":"toolu_02","content":"{\"type\":\"tool_reference\",\"tool_name\":\"WebFetch\"}"},` +
		`{"type":"tool_result","tool_use_id":"toolu_03","content":{"type":"tool_reference","tool_name":"WebFetch"}},` +
		`{"type":"mcp_tool_result","tool_use_id":"mcptoolu_01","content":[{"type":"tool_reference","tool_name":"WebFetch"}]}]}]}`

	realPairs := make([]string, 0, len(names)*2)
	for _, name := range names {
		realPairs = append(realPairs, "@"+name+"@", name)
	}
	body := []byte(strings.NewReplacer(realPairs...).Replace(template))
	require.True(t, gjson.ValidBytes(body))
	rw := buildTestToolNameRewrite(body)
	require.NotNil(t, rw)
	require.Len(t, rw.Forward, len(names))

	fakePairs := make([]string, 0, len(names)*2)
	for _, name := range names {
		fakePairs = append(fakePairs, "@"+name+"@", rw.Forward[name])
	}
	want := strings.NewReplacer(fakePairs...).Replace(template)

	require.Equal(t, want, string(applyToolNameRewriteToBody(body, rw)))
}

func TestApplyToolNameRewriteToBody_EscapedToolReferenceNames(t *testing.T) {
	staticBody := []byte(`{"tools":[{"name":"sessions_\"quoted","input_schema":{}}],"messages":[{"role":"assistant","content":[{"type":"tool_search_tool_result","tool_use_id":"srvtoolu_01","content":{"type":"tool_search_tool_search_result","tool_references":[{"type":"tool_reference","tool_name":"sessions_\"quoted"}]}}]},{"role":"user","content":[{"type":"tool_result","tool_use_id":"toolu_01","content":[{"type":"tool_reference","tool_name":"sessions_\u0022quoted"}]}]}]}`)
	staticRW := buildTestToolNameRewrite(staticBody)
	out := applyToolNameRewriteToBody(staticBody, staticRW)
	require.True(t, gjson.ValidBytes(out))
	require.Equal(t, staticRW.Forward[`sessions_"quoted`], gjson.GetBytes(out, "messages.0.content.0.content.tool_references.0.tool_name").String())
	require.Equal(t, `"`+staticRW.Forward[`sessions_"quoted`]+`"`, gjson.GetBytes(out, "messages.1.content.0.content.0.tool_name").Raw)

	dynamicBody := []byte(`{"tools":[{"name":"WebFetch","input_schema":{}},{"name":"alpha","input_schema":{}},{"name":"beta","input_schema":{}},{"name":"gamma","input_schema":{}},{"name":"delta","input_schema":{}},{"name":"epsilon","input_schema":{}}],"messages":[{"role":"user","content":[{"type":"tool_result","tool_use_id":"toolu_01","content":[{"type":"tool_reference","tool_name":"Web\u0046etch"}]}]}]}`)
	rw := buildTestToolNameRewrite(dynamicBody)
	require.NotNil(t, rw)
	out = applyToolNameRewriteToBody(dynamicBody, rw)
	require.True(t, gjson.ValidBytes(out))
	require.Equal(t, `"`+rw.Forward["WebFetch"]+`"`, gjson.GetBytes(out, "messages.0.content.0.content.0.tool_name").Raw)
}
