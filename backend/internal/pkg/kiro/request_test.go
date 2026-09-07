package kiro

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
	"github.com/stretchr/testify/require"
)

func baseOpts() Options {
	return Options{
		ModelID:        "claude-sonnet-4.6",
		ConversationID: "conv-1",
		ProfileArn:     "arn:aws:codewhisperer:::profile/ABC",
		ToolDescMaxLen: 10000,
	}
}

func TestBuildRequestFixedFields(t *testing.T) {
	t.Parallel()

	req := &apicompat.AnthropicRequest{
		Messages: []apicompat.AnthropicMessage{
			{Role: "user", Content: rawJSON(t, `"hello"`)},
		},
	}

	out, err := BuildRequest(req, baseOpts())
	require.NoError(t, err)
	require.Equal(t, "MANUAL", out.ConversationState.ChatTriggerType)
	require.Equal(t, "conv-1", out.ConversationState.ConversationID)
	require.Equal(t, "arn:aws:codewhisperer:::profile/ABC", out.ProfileArn)

	um := out.ConversationState.CurrentMessage.UserInputMessage
	require.Equal(t, "hello", um.Content)
	require.Equal(t, "claude-sonnet-4.6", um.ModelID)
	require.Equal(t, "AI_EDITOR", um.Origin)
	require.Empty(t, out.ConversationState.History)
}

// TestBuildRequestSystemPrependedToFirstUser 覆盖 spec §6.3 的头号有损项：
// Kiro 没有 system 角色，system 必须拼进第一条 user message。
func TestBuildRequestSystemPrependedToFirstUser(t *testing.T) {
	t.Parallel()

	req := &apicompat.AnthropicRequest{
		System: rawJSON(t, `"SYSTEM RULES"`),
		Messages: []apicompat.AnthropicMessage{
			{Role: "user", Content: rawJSON(t, `"first"`)},
			{Role: "assistant", Content: rawJSON(t, `"ack"`)},
			{Role: "user", Content: rawJSON(t, `"second"`)},
		},
	}

	out, err := BuildRequest(req, baseOpts())
	require.NoError(t, err)

	// 有 history 时，system 拼到 history 里的第一条 user，而不是 current。
	require.Len(t, out.ConversationState.History, 2)
	firstUser := out.ConversationState.History[0].UserInputMessage
	require.NotNil(t, firstUser)
	require.True(t, strings.HasPrefix(firstUser.Content, "SYSTEM RULES"))
	require.Contains(t, firstUser.Content, "first")

	// current 保持原样。
	require.Equal(t, "second", out.ConversationState.CurrentMessage.UserInputMessage.Content)
}

func TestBuildRequestSystemGoesToCurrentWhenNoHistory(t *testing.T) {
	t.Parallel()

	req := &apicompat.AnthropicRequest{
		System: rawJSON(t, `"SYSTEM RULES"`),
		Messages: []apicompat.AnthropicMessage{
			{Role: "user", Content: rawJSON(t, `"only"`)},
		},
	}

	out, err := BuildRequest(req, baseOpts())
	require.NoError(t, err)
	require.Empty(t, out.ConversationState.History)

	content := out.ConversationState.CurrentMessage.UserInputMessage.Content
	require.True(t, strings.HasPrefix(content, "SYSTEM RULES"))
	require.Contains(t, content, "only")
}

// TestBuildRequestTrailingAssistantBecomesContinue 覆盖 assistant prefill 的有损转换。
func TestBuildRequestTrailingAssistantBecomesContinue(t *testing.T) {
	t.Parallel()

	req := &apicompat.AnthropicRequest{
		Messages: []apicompat.AnthropicMessage{
			{Role: "user", Content: rawJSON(t, `"q"`)},
			{Role: "assistant", Content: rawJSON(t, `"partial answer"`)},
		},
	}

	out, err := BuildRequest(req, baseOpts())
	require.NoError(t, err)
	require.Equal(t, "Continue", out.ConversationState.CurrentMessage.UserInputMessage.Content)

	last := out.ConversationState.History[len(out.ConversationState.History)-1]
	require.NotNil(t, last.AssistantResponseMessage)
	require.Equal(t, "partial answer", last.AssistantResponseMessage.Content)
}

func TestBuildRequestToolsConvertedAndSanitized(t *testing.T) {
	t.Parallel()

	req := &apicompat.AnthropicRequest{
		Messages: []apicompat.AnthropicMessage{
			{Role: "user", Content: rawJSON(t, `"go"`)},
		},
		Tools: []apicompat.AnthropicTool{{
			Name:        "Read",
			Description: "read a file",
			InputSchema: rawJSON(t, `{"type":"object","additionalProperties":false,"required":[],"properties":{"p":{"type":"string"}}}`),
		}},
	}

	out, err := BuildRequest(req, baseOpts())
	require.NoError(t, err)

	ctx := out.ConversationState.CurrentMessage.UserInputMessage.UserInputMessageContext
	require.NotNil(t, ctx)
	require.Len(t, ctx.Tools, 1)

	spec := ctx.Tools[0].ToolSpecification
	require.Equal(t, "Read", spec.Name)
	require.Equal(t, "read a file", spec.Description)
	require.NotContains(t, spec.InputSchema.JSON, "additionalProperties", "schema 必须经过 SanitizeSchema")
	require.NotContains(t, spec.InputSchema.JSON, "required")
}

func TestBuildRequestLongToolDescriptionMovedToSystem(t *testing.T) {
	t.Parallel()

	longDesc := strings.Repeat("x", 200)
	opts := baseOpts()
	opts.ToolDescMaxLen = 50

	req := &apicompat.AnthropicRequest{
		Messages: []apicompat.AnthropicMessage{
			{Role: "user", Content: rawJSON(t, `"go"`)},
		},
		Tools: []apicompat.AnthropicTool{{
			Name:        "Bash",
			Description: longDesc,
			InputSchema: rawJSON(t, `{"type":"object"}`),
		}},
	}

	out, err := BuildRequest(req, opts)
	require.NoError(t, err)

	content := out.ConversationState.CurrentMessage.UserInputMessage.Content
	require.Contains(t, content, "## Tool: Bash", "长描述应移入 system prompt")
	require.Contains(t, content, longDesc)

	spec := out.ConversationState.CurrentMessage.UserInputMessage.UserInputMessageContext.Tools[0].ToolSpecification
	require.Contains(t, spec.Description, "Full documentation in system prompt")
	require.NotContains(t, spec.Description, longDesc)
}

// TestBuildRequestStripsToolContentWhenNoTools 覆盖：请求未声明 tools 时，
// 历史里的工具调用/结果必须清空，否则上游会因引用未声明的工具而拒绝。
func TestBuildRequestStripsToolContentWhenNoTools(t *testing.T) {
	t.Parallel()

	req := &apicompat.AnthropicRequest{
		Messages: []apicompat.AnthropicMessage{
			{Role: "user", Content: rawJSON(t, `"q"`)},
			{Role: "assistant", Content: rawJSON(t, `[{"type":"tool_use","id":"tu_1","name":"Read","input":{}}]`)},
			{Role: "user", Content: rawJSON(t, `[{"type":"tool_result","tool_use_id":"tu_1","content":"r"}]`)},
		},
	}

	out, err := BuildRequest(req, baseOpts())
	require.NoError(t, err)

	// 3 条消息规整后 current 是最后一条，history 保留前两条 —— 先断言结构本身
	// 存在，否则若整段 history 或 assistant 条目消失，下面的空值检查也会误判通过。
	require.Len(t, out.ConversationState.History, 2)

	var assistantEntry *AssistantResponseMessage
	for _, h := range out.ConversationState.History {
		if h.AssistantResponseMessage != nil {
			assistantEntry = h.AssistantResponseMessage
		}
	}
	require.NotNil(t, assistantEntry, "history 中必须存在 assistant 条目")
	require.Empty(t, assistantEntry.ToolUses)

	ctx := out.ConversationState.CurrentMessage.UserInputMessage.UserInputMessageContext
	if ctx != nil {
		require.Empty(t, ctx.ToolResults)
	}
}

// TestBuildRequestMalformedToolSchemaDegradesGracefully 覆盖：InputSchema 是
// 非法 JSON 时，processTools 里被忽略的 json.Unmarshal 错误必须安全退化成
// SanitizeSchema(nil) 的 {}，而不是让整个请求出错，也不能让 payload marshal
// 成 null（那会让上游 400）。
func TestBuildRequestMalformedToolSchemaDegradesGracefully(t *testing.T) {
	t.Parallel()

	req := &apicompat.AnthropicRequest{
		Messages: []apicompat.AnthropicMessage{
			{Role: "user", Content: rawJSON(t, `"go"`)},
		},
		Tools: []apicompat.AnthropicTool{{
			Name:        "Broken",
			InputSchema: json.RawMessage("{not json"),
		}},
	}

	out, err := BuildRequest(req, baseOpts())
	require.NoError(t, err)

	spec := out.ConversationState.CurrentMessage.UserInputMessage.UserInputMessageContext.Tools[0].ToolSpecification
	require.NotNil(t, spec.InputSchema.JSON)
	require.Empty(t, spec.InputSchema.JSON)

	// 整个 payload 必须能正常 marshal；若退化成 nil map，会 marshal 成
	// "json":null 而不是 "json":{}，上游会因此拒绝请求。
	data, err := json.Marshal(out)
	require.NoError(t, err)
	require.NotContains(t, string(data), `"json":null`)
	require.Contains(t, string(data), `"json":{}`)
}

func TestBuildRequestToolResultsMappedWithStatus(t *testing.T) {
	t.Parallel()

	req := &apicompat.AnthropicRequest{
		Messages: []apicompat.AnthropicMessage{
			{Role: "user", Content: rawJSON(t, `"q"`)},
			{Role: "assistant", Content: rawJSON(t, `[{"type":"tool_use","id":"tu_1","name":"Read","input":{}}]`)},
			{Role: "user", Content: rawJSON(t, `[
				{"type":"tool_result","tool_use_id":"tu_1","content":"ok"},
				{"type":"tool_result","tool_use_id":"tu_2","content":"bad","is_error":true}
			]`)},
		},
		Tools: []apicompat.AnthropicTool{{Name: "Read", InputSchema: rawJSON(t, `{"type":"object"}`)}},
	}

	out, err := BuildRequest(req, baseOpts())
	require.NoError(t, err)

	ctx := out.ConversationState.CurrentMessage.UserInputMessage.UserInputMessageContext
	require.NotNil(t, ctx)
	require.Len(t, ctx.ToolResults, 2)
	require.Equal(t, "success", ctx.ToolResults[0].Status)
	require.Equal(t, "ok", ctx.ToolResults[0].Content[0].Text)
	require.Equal(t, "error", ctx.ToolResults[1].Status)
}

func TestBuildRequestImagesMapped(t *testing.T) {
	t.Parallel()

	req := &apicompat.AnthropicRequest{
		Messages: []apicompat.AnthropicMessage{{
			Role: "user",
			Content: rawJSON(t, `[
				{"type":"text","text":"see"},
				{"type":"image","source":{"type":"base64","media_type":"image/png","data":"QUJD"}}
			]`),
		}},
	}

	out, err := BuildRequest(req, baseOpts())
	require.NoError(t, err)

	imgs := out.ConversationState.CurrentMessage.UserInputMessage.Images
	require.Len(t, imgs, 1)
	require.Equal(t, "png", imgs[0].Format)
	require.Equal(t, "QUJD", imgs[0].Source.Bytes)
}

func TestBuildRequestEmptyContentBecomesContinue(t *testing.T) {
	t.Parallel()

	req := &apicompat.AnthropicRequest{
		Messages: []apicompat.AnthropicMessage{
			{Role: "user", Content: rawJSON(t, `""`)},
		},
	}

	out, err := BuildRequest(req, baseOpts())
	require.NoError(t, err)
	require.Equal(t, "Continue", out.ConversationState.CurrentMessage.UserInputMessage.Content)
}

func TestBuildRequestFakeThinkingInjection(t *testing.T) {
	t.Parallel()

	req := &apicompat.AnthropicRequest{
		Messages: []apicompat.AnthropicMessage{
			{Role: "user", Content: rawJSON(t, `"solve it"`)},
		},
	}

	opts := baseOpts()
	opts.FakeThinking = true
	opts.FakeThinkingMaxTokens = 4000

	out, err := BuildRequest(req, opts)
	require.NoError(t, err)

	content := out.ConversationState.CurrentMessage.UserInputMessage.Content
	require.Contains(t, content, "<thinking_mode>enabled</thinking_mode>")
	require.Contains(t, content, "<max_thinking_length>4000</max_thinking_length>")
	require.Contains(t, content, "solve it")

	// 关闭时不得注入。
	opts.FakeThinking = false
	out, err = BuildRequest(req, opts)
	require.NoError(t, err)
	require.NotContains(t, out.ConversationState.CurrentMessage.UserInputMessage.Content, "<thinking_mode>")
}

func TestBuildRequestNoUsableMessages(t *testing.T) {
	t.Parallel()

	_, err := BuildRequest(&apicompat.AnthropicRequest{}, baseOpts())
	require.ErrorIs(t, err, ErrNoMessages)

	// 全是 assistant → EnsureFirstIsUser 清空 → 同样是 ErrNoMessages。
	_, err = BuildRequest(&apicompat.AnthropicRequest{
		Messages: []apicompat.AnthropicMessage{{Role: "assistant", Content: rawJSON(t, `"x"`)}},
	}, baseOpts())
	require.ErrorIs(t, err, ErrNoMessages)
}
