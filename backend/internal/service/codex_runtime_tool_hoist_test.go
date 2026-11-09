package service

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// codexRuntimeToolsBody 复刻 Codex 0.166 的真实形状：工具全部在
// input[].additional_tools 里，exec 是嵌在 namespace "functions" 下的 custom 工具。
const codexRuntimeToolsBody = `{
  "model": "gpt-6.1-sol",
  "stream": true,
  "input": [
    {"type":"additional_tools","role":"developer","tools":[
      {"type":"namespace","name":"functions","tools":[
        {"type":"custom","name":"exec","description":"Run JavaScript code"},
        {"type":"function","name":"wait","parameters":{"type":"object"}}
      ]},
      {"type":"namespace","name":"clock","tools":[
        {"type":"function","name":"sleep","parameters":{"type":"object"}}
      ]}
    ]},
    {"type":"message","role":"user","content":"run date -u"}
  ]
}`

func TestHoistCodexRuntimeToolsLiftsAdditionalToolsAndNamespaceCustom(t *testing.T) {
	out, changed, err := hoistCodexRuntimeTools([]byte(codexRuntimeToolsBody))
	require.NoError(t, err)
	require.True(t, changed)

	require.False(t, gjson.GetBytes(out, `input.#(type=="additional_tools")`).Exists(),
		"additional_tools 不能被原样发给上游")

	tools := gjson.GetBytes(out, "tools").Array()
	require.Len(t, tools, 3, "两个 namespace + 提升上来的 custom exec")

	// custom exec 提到顶层，名字保持原样，Codex 回程把它还原成 custom_tool_call。
	var exec gjson.Result
	var functionsNs gjson.Result
	for _, tool := range tools {
		switch tool.Get("name").String() {
		case "exec":
			exec = tool
		case "functions":
			functionsNs = tool
		}
	}
	require.True(t, exec.Exists(), "custom exec 必须被提升到顶层")
	require.Equal(t, "custom", exec.Get("type").String())
	require.Equal(t, "Run JavaScript code", exec.Get("description").String())

	// namespace 里只应剩下 function 子工具。
	require.True(t, functionsNs.Exists())
	children := functionsNs.Get("tools").Array()
	require.Len(t, children, 1)
	require.Equal(t, "wait", children[0].Get("name").String())
}

func TestHoistCodexRuntimeToolsDedupesRepeatedAdditionalTools(t *testing.T) {
	body := `{
      "input": [
        {"type":"additional_tools","tools":[{"type":"function","name":"wait"}]},
        {"type":"message","role":"user","content":"hi"},
        {"type":"additional_tools","tools":[{"type":"function","name":"wait"},{"type":"namespace","name":"functions","tools":[{"type":"custom","name":"exec"}]}]}
      ]
    }`
	out, changed, err := hoistCodexRuntimeTools([]byte(body))
	require.NoError(t, err)
	require.True(t, changed)

	// 子工具全部被提走的 functions namespace 一并移除，不向上游发空声明。
	names := []string{}
	for _, tool := range gjson.GetBytes(out, "tools").Array() {
		names = append(names, tool.Get("name").String())
	}
	require.Equal(t, []string{"wait", "exec"}, names)
}

func TestHoistCodexRuntimeToolsKeepsCollidingNamespaceCustom(t *testing.T) {
	body := `{
      "tools": [{"type":"custom","name":"exec"}],
      "input": [
        {"type":"additional_tools","tools":[
          {"type":"namespace","name":"functions","tools":[{"type":"custom","name":"exec"}]}
        ]}
      ]
    }`
	out, changed, err := hoistCodexRuntimeTools([]byte(body))
	require.NoError(t, err)
	require.True(t, changed)

	// 顶层已有同名工具：namespace 子工具原样保留，避免产生无法区分的重复声明。
	topLevelExec := 0
	var namespaceChildNames []string
	for _, tool := range gjson.GetBytes(out, "tools").Array() {
		switch tool.Get("name").String() {
		case "exec":
			topLevelExec++
		case "functions":
			for _, child := range tool.Get("tools").Array() {
				namespaceChildNames = append(namespaceChildNames, child.Get("name").String())
			}
		}
	}
	require.Equal(t, 1, topLevelExec, "不应重复声明顶层 exec")
	require.Equal(t, []string{"exec"}, namespaceChildNames, "namespace 子工具应原样保留")
}

func TestHoistCodexRuntimeToolsNoopWithoutCodexShape(t *testing.T) {
	body := []byte(`{"model":"gpt-6.1-sol","input":[{"type":"message","role":"user","content":"hi"}],"tools":[{"type":"function","name":"f"}]}`)
	out, changed, err := hoistCodexRuntimeTools(body)
	require.NoError(t, err)
	require.False(t, changed)
	require.Equal(t, string(body), string(out))
}

func TestHoistCodexRuntimeToolsLeavesInvalidJSONAlone(t *testing.T) {
	body := []byte(`{"additional_tools": oops`)
	out, changed, err := hoistCodexRuntimeTools(body)
	require.NoError(t, err)
	require.False(t, changed)
	require.Equal(t, string(body), string(out))
}

func TestCodexNamespaceToolsEnabled(t *testing.T) {
	require.False(t, (*Account)(nil).CodexNamespaceToolsEnabled())

	require.False(t, (&Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey}).CodexNamespaceToolsEnabled())
	require.False(t, (&Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Extra: map[string]any{"other": true}}).CodexNamespaceToolsEnabled())
	require.False(t, (&Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Extra: map[string]any{ExtraKeyCodexNamespaceTools: false}}).CodexNamespaceToolsEnabled())
	require.True(t, (&Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Extra: map[string]any{ExtraKeyCodexNamespaceTools: true}}).CodexNamespaceToolsEnabled())

	// OAuth 上游原生支持 additional_tools / namespace，误配也不改写请求体。
	require.False(t, (&Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth, Extra: map[string]any{ExtraKeyCodexNamespaceTools: true}}).CodexNamespaceToolsEnabled())

	// 非 openai 平台即使误配也不生效。
	require.False(t, (&Account{Platform: PlatformAnthropic, Extra: map[string]any{ExtraKeyCodexNamespaceTools: true}}).CodexNamespaceToolsEnabled())
}

// TestHoistedCodexToolsLowerToChatFunction 验证提升后的请求在 CC 回退路径上
// 能把 exec 降级成 function 工具发给上游，并在回程被识别为 custom 工具。
func TestHoistedCodexToolsLowerToChatFunction(t *testing.T) {
	out, changed, err := hoistCodexRuntimeTools([]byte(codexRuntimeToolsBody))
	require.NoError(t, err)
	require.True(t, changed)

	var req apicompat.ResponsesRequest
	require.NoError(t, json.Unmarshal(out, &req))
	effective, err := apicompat.EffectiveResponsesTools(&req)
	require.NoError(t, err)
	require.True(t, apicompat.CustomToolNames(effective)["exec"], "回程需要按 custom 还原 exec")

	chatReq, err := apicompat.ResponsesToChatCompletionsRequest(&req)
	require.NoError(t, err)

	var lowered bool
	for _, tool := range chatReq.Tools {
		if tool.Function == nil || tool.Function.Name != "exec" {
			continue
		}
		lowered = true
		require.Equal(t, "function", tool.Type)
		require.Contains(t, string(tool.Function.Parameters), "input")
	}
	require.True(t, lowered, "exec 必须以 function 工具形式出现在上游请求里")
}

// TestHoistedCodexToolsAdaptForWSFunctionUpstream 验证 WS 桥（只读顶层 tools）
// 也能拿到 exec，并登记为需要回程还原的 custom 工具。
func TestHoistedCodexToolsAdaptForWSFunctionUpstream(t *testing.T) {
	out, changed, err := hoistCodexRuntimeTools([]byte(codexRuntimeToolsBody))
	require.NoError(t, err)
	require.True(t, changed)

	var body map[string]any
	require.NoError(t, decodeOpenAIJSONUseNumber(out, &body))
	adapter, adapted, err := apicompat.AdaptResponsesClientTools(body)
	require.NoError(t, err)
	require.True(t, adapted)
	require.True(t, adapter.CustomTools["exec"], "WS 桥回程需要按 custom 还原 exec")

	tools, _ := body["tools"].([]any)
	var found bool
	for _, raw := range tools {
		tool, _ := raw.(map[string]any)
		if stringValue(tool["name"]) == "exec" && stringValue(tool["type"]) == "function" {
			found = true
		}
	}
	require.True(t, found, "exec 必须以 function 工具形式出现在上游请求里")
}

func TestOpenAIWSHTTPBridgeEnabled(t *testing.T) {
	require.False(t, (*Account)(nil).IsOpenAIWSHTTPBridgeEnabled())
	require.False(t, (&Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey}).IsOpenAIWSHTTPBridgeEnabled())
	require.False(t, (&Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Extra: map[string]any{"openai_ws_http_bridge": false}}).IsOpenAIWSHTTPBridgeEnabled())
	require.True(t, (&Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Extra: map[string]any{"openai_ws_http_bridge": true}}).IsOpenAIWSHTTPBridgeEnabled())
	require.False(t, (&Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth, Extra: map[string]any{"openai_ws_http_bridge": true}}).IsOpenAIWSHTTPBridgeEnabled())
	require.False(t, (&Account{Platform: PlatformAnthropic, Extra: map[string]any{"openai_ws_http_bridge": true}}).IsOpenAIWSHTTPBridgeEnabled())
}

// TestHoistCodexRuntimeToolsKeepsInputNonEmpty 覆盖 Codex 首包只带
// additional_tools 的场景：移除后 input 不能变成空数组，否则上游 400
// "Input items array must not be empty"。
func TestHoistCodexRuntimeToolsKeepsInputNonEmpty(t *testing.T) {
	body := []byte(`{"input":[{"type":"additional_tools","tools":[{"type":"custom","name":"exec"}]}]}`)
	out, changed, err := hoistCodexRuntimeTools(body)
	require.NoError(t, err)
	require.True(t, changed)
	require.Len(t, gjson.GetBytes(out, "input").Array(), 1, "input 不能被清空")
	require.Len(t, gjson.GetBytes(out, "tools").Array(), 1, "工具仍要提升到顶层")
	require.Empty(t, gjson.GetBytes(out, "input.0.tools").Array(), "保留的 item 必须清空 tools，避免与顶层重复声明")
}

// 首包只带 additional_tools 时，提升后的请求必须仍能转成 Chat Completions：
// 保留的 item 若仍带工具，EffectiveResponsesTools 会把同名工具合并两次，
// 转换以 duplicate top-level executable tool name 拒绝。
func TestHoistedAdditionalToolsOnlyInputConvertsToChat(t *testing.T) {
	body := []byte(`{"model":"compat-model","input":[{"type":"additional_tools","tools":[
	  {"type":"function","name":"wait","parameters":{"type":"object"}},
	  {"type":"custom","name":"exec"},
	  {"type":"namespace","name":"functions","tools":[{"type":"custom","name":"apply_patch"},{"type":"function","name":"read","parameters":{"type":"object"}}]}
	]}]}`)
	out, changed, err := hoistCodexRuntimeTools(body)
	require.NoError(t, err)
	require.True(t, changed)

	var req apicompat.ResponsesRequest
	require.NoError(t, json.Unmarshal(out, &req))
	effective, err := apicompat.EffectiveResponsesTools(&req)
	require.NoError(t, err)
	require.Len(t, effective, 4, "wait + exec + functions namespace + apply_patch，各只一次")
	chatReq, err := apicompat.ResponsesToChatCompletionsRequest(&req)
	require.NoError(t, err)
	var names []string
	for _, tool := range chatReq.Tools {
		names = append(names, tool.Function.Name)
	}
	require.ElementsMatch(t, []string{"wait", "exec", "functions__read", "apply_patch"}, names)

	// 再次提升（WS 桥多轮重放）不再改写。
	again, changedAgain, err := hoistCodexRuntimeTools(out)
	require.NoError(t, err)
	require.False(t, changedAgain)
	require.Equal(t, string(out), string(again))
}

// 非默认命名空间的 custom 工具不能提升：回程 custom_tool_call 只带裸名，Codex
// 会把缺省 namespace 补成 "functions" 路由，扩展命名空间的工具会找不到处理器。
func TestHoistKeepsNonDefaultNamespaceCustomTools(t *testing.T) {
	body := []byte(`{"input":[
	  {"type":"additional_tools","tools":[{"type":"namespace","name":"ext","tools":[{"type":"custom","name":"apply"},{"type":"function","name":"lookup"}]}]},
	  {"type":"message","role":"user","content":"hi"}
	]}`)
	out, changed, err := hoistCodexRuntimeTools(body)
	require.NoError(t, err)
	require.True(t, changed, "additional_tools 仍要提升到顶层")
	require.False(t, hasTopLevelTool(out, "custom", "apply"), "ext/apply 不能被提成顶层裸名")
	require.Equal(t, []string{"apply", "lookup"}, namespaceChildren(t, out, "ext"))
}

// 同名 namespace 出现在顶层 tools 与 input[].additional_tools 时，必须按子工具
// 合并而不是整体丢弃（review 反馈：增量下发的常见形态）。
func TestHoistMergesSameNamespaceAcrossSources(t *testing.T) {
	body := []byte(`{
	  "model":"compat-model","stream":false,
	  "tools":[{"type":"namespace","name":"functions","tools":[{"type":"function","name":"read","parameters":{"type":"object"}}]}],
	  "input":[
	    {"type":"additional_tools","role":"developer","tools":[{"type":"namespace","name":"functions","tools":[{"type":"function","name":"write","parameters":{"type":"object"}}]}]},
	    {"type":"message","role":"user","content":"hi"}
	  ]
	}`)
	out, changed, err := hoistCodexRuntimeTools(body)
	require.NoError(t, err)
	require.True(t, changed)
	require.Equal(t, []string{"read", "write"}, namespaceChildren(t, out, "functions"))
}

// 多个 additional_tools item 声明同一个 namespace 时同样要合并。
func TestHoistMergesNamespaceAcrossAdditionalToolsItems(t *testing.T) {
	body := []byte(`{
	  "input":[
	    {"type":"additional_tools","tools":[{"type":"namespace","name":"functions","tools":[{"type":"function","name":"read"}]}]},
	    {"type":"message","role":"user","content":"hi"},
	    {"type":"additional_tools","tools":[{"type":"namespace","name":"functions","tools":[{"type":"function","name":"write"}]}]}
	  ]
	}`)
	out, changed, err := hoistCodexRuntimeTools(body)
	require.NoError(t, err)
	require.True(t, changed)
	require.Equal(t, []string{"read", "write"}, namespaceChildren(t, out, "functions"))
}

// 后声明的同名 namespace 里的 custom 工具（如 exec）也必须被提升到顶层，
// 否则本 PR 想修的场景在这类形状下依然丢工具。
func TestHoistLiftsCustomFromMergedNamespace(t *testing.T) {
	body := []byte(`{
	  "tools":[{"type":"namespace","name":"functions","tools":[{"type":"function","name":"read"}]}],
	  "input":[
	    {"type":"additional_tools","tools":[{"type":"namespace","name":"functions","tools":[{"type":"custom","name":"exec"}]}]},
	    {"type":"message","role":"user","content":"hi"}
	  ]
	}`)
	out, changed, err := hoistCodexRuntimeTools(body)
	require.NoError(t, err)
	require.True(t, changed)
	require.Equal(t, []string{"read"}, namespaceChildren(t, out, "functions"))
	require.True(t, hasTopLevelTool(out, "custom", "exec"), "exec 必须提升到顶层")
}

func hasTopLevelTool(body []byte, typ, name string) bool {
	for _, tool := range gjson.GetBytes(body, "tools").Array() {
		if tool.Get("type").String() == typ && tool.Get("name").String() == name {
			return true
		}
	}
	return false
}

func namespaceChildren(t *testing.T, body []byte, ns string) []string {
	t.Helper()
	var names []string
	for _, tool := range gjson.GetBytes(body, "tools").Array() {
		if tool.Get("name").String() != ns || tool.Get("type").String() != "namespace" {
			continue
		}
		for _, child := range tool.Get("tools").Array() {
			names = append(names, child.Get("name").String())
		}
	}
	return names
}

// HTTP /responses 透传（OpenAI API Key 账号）同样要提升：既有客户端工具适配只读
// 顶层 tools，不提升的话 Codex 0.166 的 exec 一个都到不了上游。
func TestOpenAIPassthroughHoistsCodexRuntimeTools(t *testing.T) {
	gin.SetMode(gin.TestMode)
	body := []byte(codexRuntimeToolsBody)
	body, err := sjson.SetBytes(body, "stream", false)
	require.NoError(t, err)

	run := func(extra map[string]any) (*httpUpstreamRecorder, *httptest.ResponseRecorder) {
		recorder := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(recorder)
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))
		upstream := &httpUpstreamRecorder{resp: &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body: io.NopCloser(strings.NewReader(`{"id":"resp_hoist","status":"completed","output":[
				{"type":"function_call","id":"i1","call_id":"c1","name":"exec","arguments":"{\"input\":\"date -u\"}"},
				{"type":"function_call","id":"i2","call_id":"c2","name":"functions__wait","arguments":"{}"}],"usage":{}}`)),
		}}
		svc := openAIClientToolsTestService(upstream)
		account := &Account{ID: 8592, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Credentials: map[string]any{"api_key": "test-key"}, Extra: extra}
		result, err := svc.forwardOpenAIPassthrough(context.Background(), c, account, body, body, "gpt-6.1-sol", false, nil, false, time.Now())
		require.NoError(t, err)
		require.NotNil(t, result)
		return upstream, recorder
	}

	// 开关关闭：不提升，additional_tools 原样到达上游。
	upstream, _ := run(nil)
	require.True(t, gjson.GetBytes(upstream.lastBody, `input.#(type=="additional_tools")`).Exists())
	require.False(t, gjson.GetBytes(upstream.lastBody, "tools").Exists())

	upstream, recorder := run(map[string]any{ExtraKeyCodexNamespaceTools: true})
	require.False(t, gjson.GetBytes(upstream.lastBody, `input.#(type=="additional_tools")`).Exists())
	require.True(t, hasTopLevelTool(upstream.lastBody, "function", "exec"), "exec 降级为 function 发给上游")
	require.True(t, hasTopLevelTool(upstream.lastBody, "function", "functions__wait"), "namespace function 子工具摊平")

	out := recorder.Body.String()
	require.Equal(t, "custom_tool_call", gjson.Get(out, "output.0.type").String())
	require.Equal(t, "exec", gjson.Get(out, "output.0.name").String())
	require.Equal(t, "date -u", gjson.Get(out, "output.0.input").String())
	require.Equal(t, "function_call", gjson.Get(out, "output.1.type").String())
	require.Equal(t, "wait", gjson.Get(out, "output.1.name").String())
	require.Equal(t, "functions", gjson.Get(out, "output.1.namespace").String())
}
