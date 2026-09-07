package kiro

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
)

// ErrNoMessages 表示规整后没有任何可发送的消息。
var ErrNoMessages = errors.New("kiro: no usable messages after normalization")

// defaultOrigin 是 Kiro IDE 路径的 origin；API Key 路径由调用方传 KIRO_CLI。
const defaultOrigin = "AI_EDITOR"

// continuePlaceholder 用于顶替空内容或 assistant 结尾的场景。
// Kiro 不接受空 content，也没有 assistant prefill 语义。
const continuePlaceholder = "Continue"

// Request 是 generateAssistantResponse 的请求体。
type Request struct {
	ConversationState ConversationState `json:"conversationState"`
	ProfileArn        string            `json:"profileArn,omitempty"`
}

// ConversationState 承载完整会话。
type ConversationState struct {
	ChatTriggerType string         `json:"chatTriggerType"`
	ConversationID  string         `json:"conversationId"`
	CurrentMessage  CurrentMessage `json:"currentMessage"`
	History         []HistoryEntry `json:"history,omitempty"`
}

// CurrentMessage 是本轮要发送的消息。
type CurrentMessage struct {
	UserInputMessage UserInputMessage `json:"userInputMessage"`
}

// UserInputMessage 是 Kiro 的用户消息形态。注意它没有 system 角色，
// 也没有 temperature / top_p / max_tokens 等采样参数的槽位。
type UserInputMessage struct {
	Content                 string                   `json:"content"`
	ModelID                 string                   `json:"modelId"`
	Origin                  string                   `json:"origin"`
	Images                  []KiroImage              `json:"images,omitempty"`
	UserInputMessageContext *UserInputMessageContext `json:"userInputMessageContext,omitempty"`
}

// KiroImage 是 Kiro 的图片形态：格式名 + base64 字节。
type KiroImage struct {
	Format string      `json:"format"`
	Source ImageSource `json:"source"`
}

// ImageSource 承载 base64 字节。
type ImageSource struct {
	Bytes string `json:"bytes"`
}

// UserInputMessageContext 携带工具声明与工具结果。
type UserInputMessageContext struct {
	Tools       []Tool           `json:"tools,omitempty"`
	ToolResults []KiroToolResult `json:"toolResults,omitempty"`
}

// Tool 是一条工具声明。
type Tool struct {
	ToolSpecification ToolSpecification `json:"toolSpecification"`
}

// ToolSpecification 是工具的名称/描述/入参 schema。
type ToolSpecification struct {
	Name        string      `json:"name"`
	Description string      `json:"description"`
	InputSchema InputSchema `json:"inputSchema"`
}

// InputSchema 包住 JSON Schema。
type InputSchema struct {
	JSON map[string]any `json:"json"`
}

// KiroToolResult 是一个工具执行结果。
type KiroToolResult struct {
	ToolUseID string              `json:"toolUseId"`
	Status    string              `json:"status"`
	Content   []ToolResultContent `json:"content"`
}

// ToolResultContent 是工具结果的一段文本。
type ToolResultContent struct {
	Text string `json:"text"`
}

// HistoryEntry 是历史里的一条消息，两个字段互斥。
type HistoryEntry struct {
	UserInputMessage         *UserInputMessage         `json:"userInputMessage,omitempty"`
	AssistantResponseMessage *AssistantResponseMessage `json:"assistantResponseMessage,omitempty"`
}

// AssistantResponseMessage 是历史里的助手消息。
type AssistantResponseMessage struct {
	Content  string        `json:"content"`
	ToolUses []KiroToolUse `json:"toolUses,omitempty"`
}

// KiroToolUse 是历史里的一次工具调用。
type KiroToolUse struct {
	Name      string          `json:"name"`
	Input     json.RawMessage `json:"input"`
	ToolUseID string          `json:"toolUseId"`
}

// Options 控制请求构造的可变部分。
type Options struct {
	// ModelID 是已解析好的 Kiro 上游模型名。
	ModelID string
	// ConversationID 必须与粘性会话一致；换账号时调用方需重新生成。
	ConversationID string
	// ProfileArn 对 API Key 账号应留空。
	ProfileArn string
	// Origin 为空时默认 AI_EDITOR；API Key 路径传 KIRO_CLI。
	Origin string
	// FakeThinking 开启后向 prompt 注入思考指令，并由 StreamTranslator 剥离。
	FakeThinking bool
	// FakeThinkingMaxTokens 是注入指令里声明的思考预算。
	FakeThinkingMaxTokens int
	// ToolDescMaxLen 超过此长度的工具描述移入 system prompt。
	ToolDescMaxLen int
}

// BuildRequest 把 Anthropic 请求转换成 Kiro 的 conversationState。
// 转换是有损的，完整清单见设计文档 §6.3。
func BuildRequest(req *apicompat.AnthropicRequest, opts Options) (*Request, error) {
	if req == nil {
		return nil, ErrNoMessages
	}

	// 1-2. 工具预处理：长描述移入 system，schema 清洗。
	tools, toolDocs := processTools(req.Tools, opts.ToolDescMaxLen)

	// 3. system 拼接。
	systemText, err := FlattenSystem(req.System)
	if err != nil {
		return nil, err
	}
	if toolDocs != "" {
		if systemText == "" {
			systemText = strings.TrimSpace(toolDocs)
		} else {
			systemText += toolDocs
		}
	}

	// 4. 消息规整链。
	msgs, err := FromAnthropic(req)
	if err != nil {
		return nil, err
	}
	if len(tools) == 0 {
		msgs = StripToolContent(msgs)
	}
	msgs = MergeAdjacent(msgs)
	msgs = EnsureFirstIsUser(msgs)
	msgs = EnsureAlternating(msgs)
	if len(msgs) == 0 {
		return nil, ErrNoMessages
	}

	origin := opts.Origin
	if origin == "" {
		origin = defaultOrigin
	}

	// 5. history 构造（除最后一条外）。system 拼到 history 首条 user。
	historyMsgs := msgs[:len(msgs)-1]
	current := msgs[len(msgs)-1]

	if systemText != "" && len(historyMsgs) > 0 {
		for i := range historyMsgs {
			if historyMsgs[i].Role == "user" {
				historyMsgs[i].Text = joinSystem(systemText, historyMsgs[i].Text)
				break
			}
		}
	}

	history := buildHistory(historyMsgs, opts.ModelID, origin)

	// 6. current message；assistant 结尾时移入 history 并顶替为 Continue。
	currentContent := current.Text
	if systemText != "" && len(historyMsgs) == 0 {
		currentContent = joinSystem(systemText, currentContent)
	}

	if current.Role == "assistant" {
		history = append(history, HistoryEntry{
			AssistantResponseMessage: assistantEntry(current, currentContent),
		})
		current = Msg{Role: "user"}
		currentContent = continuePlaceholder
	}

	if strings.TrimSpace(currentContent) == "" {
		currentContent = continuePlaceholder
	}

	// 7. 假思考注入。
	if opts.FakeThinking && current.Role == "user" {
		currentContent = injectThinking(currentContent, opts.FakeThinkingMaxTokens)
	}

	// 8. images / toolResults / tools。
	userInput := UserInputMessage{
		Content: currentContent,
		ModelID: opts.ModelID,
		Origin:  origin,
		Images:  toKiroImages(current.Images),
	}
	if ctx := buildContext(tools, current.ToolResults); ctx != nil {
		userInput.UserInputMessageContext = ctx
	}

	// 9. 固定字段。
	out := &Request{
		ConversationState: ConversationState{
			ChatTriggerType: "MANUAL",
			ConversationID:  opts.ConversationID,
			CurrentMessage:  CurrentMessage{UserInputMessage: userInput},
			History:         history,
		},
		ProfileArn: opts.ProfileArn,
	}
	return out, nil
}

func joinSystem(system, content string) string {
	if content == "" {
		return system
	}
	return system + "\n\n" + content
}

// processTools 清洗 schema，并把超长描述移入 system prompt 文档段。
func processTools(tools []apicompat.AnthropicTool, maxLen int) ([]Tool, string) {
	if len(tools) == 0 {
		return nil, ""
	}

	out := make([]Tool, 0, len(tools))
	var docs []string

	for _, tool := range tools {
		var schema map[string]any
		if len(tool.InputSchema) > 0 {
			// 解析失败时退化为空对象，好过让上游 400。
			_ = json.Unmarshal(tool.InputSchema, &schema)
		}

		desc := tool.Description
		if desc == "" {
			desc = "Tool: " + tool.Name
		}
		if maxLen > 0 && len(desc) > maxLen {
			docs = append(docs, fmt.Sprintf("## Tool: %s\n\n%s", tool.Name, desc))
			desc = fmt.Sprintf("[Full documentation in system prompt under '## Tool: %s']", tool.Name)
		}

		out = append(out, Tool{ToolSpecification: ToolSpecification{
			Name:        tool.Name,
			Description: desc,
			InputSchema: InputSchema{JSON: SanitizeSchema(schema)},
		}})
	}

	var toolDocs string
	if len(docs) > 0 {
		toolDocs = "\n\n---\n# Tool Documentation\n\n" + strings.Join(docs, "\n\n---\n\n")
	}
	return out, toolDocs
}

func buildHistory(msgs []Msg, modelID, origin string) []HistoryEntry {
	if len(msgs) == 0 {
		return nil
	}

	history := make([]HistoryEntry, 0, len(msgs))
	for _, m := range msgs {
		if m.Role == "assistant" {
			history = append(history, HistoryEntry{
				AssistantResponseMessage: assistantEntry(m, m.Text),
			})
			continue
		}

		content := m.Text
		if content == "" {
			content = "(empty)"
		}
		entry := &UserInputMessage{
			Content: content,
			ModelID: modelID,
			Origin:  origin,
			Images:  toKiroImages(m.Images),
		}
		if ctx := buildContext(nil, m.ToolResults); ctx != nil {
			entry.UserInputMessageContext = ctx
		}
		history = append(history, HistoryEntry{UserInputMessage: entry})
	}
	return history
}

func assistantEntry(m Msg, content string) *AssistantResponseMessage {
	if content == "" {
		content = "(empty)"
	}
	out := &AssistantResponseMessage{Content: content}
	for _, tc := range m.ToolCalls {
		input := tc.Input
		if len(input) == 0 {
			input = json.RawMessage("{}")
		}
		out.ToolUses = append(out.ToolUses, KiroToolUse{
			Name:      tc.Name,
			Input:     input,
			ToolUseID: tc.ID,
		})
	}
	return out
}

func buildContext(tools []Tool, results []ToolResult) *UserInputMessageContext {
	if len(tools) == 0 && len(results) == 0 {
		return nil
	}

	ctx := &UserInputMessageContext{Tools: tools}
	for _, r := range results {
		status := "success"
		if r.IsError {
			status = "error"
		}
		text := r.Text
		if text == "" {
			text = "(empty result)"
		}
		ctx.ToolResults = append(ctx.ToolResults, KiroToolResult{
			ToolUseID: r.ToolUseID,
			Status:    status,
			Content:   []ToolResultContent{{Text: text}},
		})
	}
	return ctx
}

func toKiroImages(images []Image) []KiroImage {
	if len(images) == 0 {
		return nil
	}
	out := make([]KiroImage, 0, len(images))
	for _, img := range images {
		out = append(out, KiroImage{
			Format: img.Format,
			Source: ImageSource{Bytes: img.Data},
		})
	}
	return out
}

// injectThinking 把思考指令注入到用户内容之前。
// Kiro 没有原生 reasoning，这是四份参考实现一致采用的替代方案。
func injectThinking(content string, maxTokens int) string {
	if maxTokens <= 0 {
		maxTokens = 4000
	}
	const instruction = `Think step by step. Make sure you fully understand what is being asked, ` +
		`consider multiple approaches, think about edge cases, challenge your assumptions, ` +
		`and verify your reasoning before concluding. ` +
		`Your response MUST begin with the literal text "<thinking>" as the very first characters ` +
		`— do not write any words, greetings, or acknowledgements before it. ` +
		`Write your reasoning inside <thinking>...</thinking> tags, close the tag, then give your final answer.`

	return fmt.Sprintf(
		"<thinking_mode>enabled</thinking_mode>\n<max_thinking_length>%d</max_thinking_length>\n<thinking_instruction>%s</thinking_instruction>\n\n%s",
		maxTokens, instruction, content)
}
