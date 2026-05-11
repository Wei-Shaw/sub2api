package service

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestBuildDynamicToolMap_BelowThreshold(t *testing.T) {
	// Parrot 行为：tools 数量 ≤ 5 时不做动态映射。
	names := []string{"bash", "edit", "read", "write", "search"}
	require.Nil(t, buildDynamicToolMap(names))
}

func TestBuildDynamicToolMap_AboveThresholdIsStable(t *testing.T) {
	// Parrot 不变量：同一组 tool_names 在同进程内映射稳定（保证 cache 命中）。
	names := []string{"alpha", "beta", "gamma", "delta", "epsilon", "zeta"}
	a := buildDynamicToolMap(names)
	b := buildDynamicToolMap(names)
	require.NotNil(t, a)
	require.Equal(t, a, b, "same input tool_names must yield identical mapping")
	require.Len(t, a, 6)
	for _, name := range names {
		require.Contains(t, a, name)
		require.NotEqual(t, name, a[name])
	}
}

func TestSanitizeToolName_StaticPrefix(t *testing.T) {
	require.Equal(t, "cc_sess_list", sanitizeToolName("sessions_list", nil))
	require.Equal(t, "cc_ses_get", sanitizeToolName("session_get", nil))
	require.Equal(t, "bash", sanitizeToolName("bash", nil))
}

func TestSanitizeToolName_DynamicTakesPrecedence(t *testing.T) {
	dyn := map[string]string{"sessions_list": "analyze_ses00"}
	got := sanitizeToolName("sessions_list", dyn)
	require.Equal(t, "analyze_ses00", got, "dynamic mapping wins over static prefix")
}

func TestRestoreToolNamesInBytes_LongestFirst(t *testing.T) {
	// 当假名 "abc_12" 是另一个更长假名的子串（真实场景极少但算法必须防御）时，
	// 长的必须先替换。本测试用显式构造的映射来验证排序不变量。
	rw := &ToolNameRewrite{
		Forward: map[string]string{"foo": "abc_12", "bar": "abc_12_ext"},
		Reverse: map[string]string{"abc_12": "foo", "abc_12_ext": "bar"},
	}
	// 手工构造 ReverseOrdered：长的在前
	rw.ReverseOrdered = [][2]string{
		{"abc_12_ext", "bar"},
		{"abc_12", "foo"},
	}
	data := []byte(`{"tool":"abc_12_ext","other":"abc_12"}`)
	restored := string(restoreToolNamesInBytes(data, rw))
	require.Equal(t, `{"tool":"bar","other":"foo"}`, restored)
}

func TestRestoreToolNamesInBytes_StaticPrefixRollback(t *testing.T) {
	data := []byte(`{"name":"sessions_list","id":"cc_ses_xyz"}`)
	got := string(restoreToolNamesInBytes(data, nil))
	require.Equal(t, `{"name":"sessions_list","id":"session_xyz"}`, got)
}

func TestApplyToolNameRewriteToBody_RenamesToolsAndToolChoice(t *testing.T) {
	body := []byte(`{"tools":[{"name":"sessions_list","input_schema":{}},{"name":"session_get","input_schema":{}},{"name":"web_search","type":"web_search_20250305"}],"tool_choice":{"type":"tool","name":"sessions_list"}}`)
	rw := buildToolNameRewriteFromBody(body)
	require.NotNil(t, rw)
	require.Contains(t, rw.Forward, "sessions_list")
	require.Contains(t, rw.Forward, "session_get")
	// web_search 是 server tool，不参与工具名改写
	require.NotContains(t, rw.Forward, "web_search")

	out := applyToolNameRewriteToBody(body, rw)

	// tools[0].name 和 tools[1].name 被改写，tools[2].name 保持不变
	require.Equal(t, "cc_sess_list", gjson.GetBytes(out, "tools.0.name").String())
	require.Equal(t, "cc_ses_get", gjson.GetBytes(out, "tools.1.name").String())
	require.Equal(t, "web_search", gjson.GetBytes(out, "tools.2.name").String())

	// tool_choice.name 被同步改写
	require.Equal(t, "cc_sess_list", gjson.GetBytes(out, "tool_choice.name").String())
	require.Equal(t, "tool", gjson.GetBytes(out, "tool_choice.type").String())
}

func TestApplyToolNameRewriteToBody_RenamesToolUseInMessages(t *testing.T) {
	// sessions_list 通过静态前缀规则改写为 cc_sess_list
	// web_search 是 server tool（type != ""），不参与工具名改写
	// messages 中的 tool_use.name 必须同步改写，才能和 tools[] 保持一致
	body := []byte(`{"tools":[{"name":"sessions_list","input_schema":{}},{"name":"web_search","type":"web_search_20250305"}],"messages":[{"role":"user","content":[{"type":"text","text":"hi"}]},{"role":"assistant","content":[{"type":"tool_use","id":"tu_01","name":"sessions_list","input":{}},{"type":"text","text":"thinking"}]},{"role":"user","content":[{"type":"tool_result","tool_use_id":"tu_01","content":"ok"}]}]}`)
	rw := buildToolNameRewriteFromBody(body)
	require.NotNil(t, rw)
	require.Equal(t, "cc_sess_list", rw.Forward["sessions_list"])

	out := applyToolNameRewriteToBody(body, rw)

	// tools[0].name 被改写
	require.Equal(t, "cc_sess_list", gjson.GetBytes(out, "tools.0.name").String())
	// tools[1].name 是 server tool，保持不变
	require.Equal(t, "web_search", gjson.GetBytes(out, "tools.1.name").String())
	// messages[1].content[0].name 是 tool_use，必须同步改写以匹配 tools[]
	require.Equal(t, "cc_sess_list", gjson.GetBytes(out, "messages.1.content.0.name").String())
	// messages[1].content[1] 是 text，保持不变
	require.Equal(t, "thinking", gjson.GetBytes(out, "messages.1.content.1.text").String())
	// messages[2].content[0] 是 tool_result，不包含 name 字段，保持不变
	require.Equal(t, "ok", gjson.GetBytes(out, "messages.2.content.0.content").String())
}

func TestApplyToolNameRewriteToBody_RenamesToolUseWithDynamicMapping(t *testing.T) {
	body := []byte(`{"tools":[{"name":"alpha_search","input_schema":{}},{"name":"beta_lookup","input_schema":{}},{"name":"gamma_fetch","input_schema":{}},{"name":"delta_update","input_schema":{}},{"name":"epsilon_parse","input_schema":{}},{"name":"zeta_render","input_schema":{}},{"name":"web_search","type":"web_search_20250305"}],"tool_choice":{"type":"tool","name":"gamma_fetch"},"messages":[{"role":"assistant","content":[{"type":"tool_use","id":"tu_dyn","name":"gamma_fetch","input":{}},{"type":"tool_use","id":"tu_srv","name":"web_search","input":{}},{"type":"text","text":"done"}]},{"role":"user","content":[{"type":"tool_result","tool_use_id":"tu_dyn","content":"ok"}]}]}`)
	rw := buildToolNameRewriteFromBody(body)
	require.NotNil(t, rw)
	require.Len(t, rw.Forward, 6)

	fakeGamma := rw.Forward["gamma_fetch"]
	require.NotEmpty(t, fakeGamma)
	require.NotEqual(t, "gamma_fetch", fakeGamma)
	require.NotContains(t, rw.Forward, "web_search")

	out := applyToolNameRewriteToBody(body, rw)

	// 动态映射会改写 tools[]、tool_choice 和历史 tool_use 中的同一个工具名
	require.Equal(t, fakeGamma, gjson.GetBytes(out, "tools.2.name").String())
	require.Equal(t, fakeGamma, gjson.GetBytes(out, "tool_choice.name").String())
	require.Equal(t, fakeGamma, gjson.GetBytes(out, "messages.0.content.0.name").String())
	// server tool 不参与动态映射，历史 tool_use 中同名引用也保持不变
	require.Equal(t, "web_search", gjson.GetBytes(out, "tools.6.name").String())
	require.Equal(t, "web_search", gjson.GetBytes(out, "messages.0.content.1.name").String())
	// tool_result 依靠 tool_use_id 关联，不需要 name 字段
	require.Equal(t, "ok", gjson.GetBytes(out, "messages.1.content.0.content").String())
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

func TestBuildToolNameRewriteFromBody_ReverseOrderedByLengthDesc(t *testing.T) {
	// 超过阈值触发动态映射，验证 ReverseOrdered 按假名长度倒序排列
	body := []byte(`{"tools":[
        {"name":"t1","input_schema":{}},
        {"name":"t2","input_schema":{}},
        {"name":"t3","input_schema":{}},
        {"name":"t4","input_schema":{}},
        {"name":"t5","input_schema":{}},
        {"name":"t6","input_schema":{}}
    ]}`)
	rw := buildToolNameRewriteFromBody(body)
	require.NotNil(t, rw)
	require.NotEmpty(t, rw.ReverseOrdered)
	for i := 1; i < len(rw.ReverseOrdered); i++ {
		require.GreaterOrEqual(t, len(rw.ReverseOrdered[i-1][0]), len(rw.ReverseOrdered[i][0]),
			"ReverseOrdered must be sorted by fake-name length descending")
	}
}

func TestRestoreToolNamesInBytes_NoMapping_NoStaticMatch_IsNoop(t *testing.T) {
	data := []byte("plain text without any tool names")
	require.Equal(t, string(data), string(restoreToolNamesInBytes(data, nil)))
}

// Ensure the fake name format follows Parrot's "{prefix}{name[:3]}{i:02d}".
func TestBuildDynamicToolMap_FakeNameShape(t *testing.T) {
	names := []string{"alphabet", "bravo", "charlie", "delta", "echo", "foxtrot"}
	m := buildDynamicToolMap(names)
	require.NotNil(t, m)
	for _, name := range names {
		fake, ok := m[name]
		require.True(t, ok)
		// fake = prefix + head3 + "%02d"
		// ends with two decimal digits
		require.Regexp(t, `^[a-z]+_[a-z0-9]{1,3}\d{2}$`, fake)
		head := name
		if len(head) > 3 {
			head = head[:3]
		}
		require.True(t, strings.Contains(fake, head), "fake %q should contain head3 %q of %q", fake, head, name)
	}
}
