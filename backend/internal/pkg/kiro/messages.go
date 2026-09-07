package kiro

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
)

var (
	// ErrUnsupportedImageSource 表示图片不是 base64 内联字节。
	// Kiro 只接受内联字节；URL 形态若由网关代下载会引入 SSRF 面，因此直接拒绝。
	ErrUnsupportedImageSource = errors.New("kiro: only base64 image sources are supported")
	// ErrUnsupportedBlock 表示出现了 Kiro 无对应语义的内容块（如 document/PDF）。
	ErrUnsupportedBlock = errors.New("kiro: unsupported content block")
)

// Image 是 Kiro 形态的图片：格式名 + base64 字节。
type Image struct {
	Format string
	Data   string
}

// ToolCall 是助手发起的一次工具调用。
type ToolCall struct {
	ID    string
	Name  string
	Input json.RawMessage
}

// ToolResult 是用户回传的一个工具结果。
type ToolResult struct {
	ToolUseID string
	Text      string
	IsError   bool
}

// Msg 是协议无关的中间消息形态，仅供本包内部使用。
type Msg struct {
	Role        string
	Text        string
	Images      []Image
	ToolCalls   []ToolCall
	ToolResults []ToolResult
}

// FlattenSystem 把 Anthropic 的 system 字段（string 或 content block 数组）
// 压平成单个字符串。Kiro 没有 system 角色，调用方需把结果拼进首条 user message。
func FlattenSystem(raw json.RawMessage) (string, error) {
	if len(raw) == 0 {
		return "", nil
	}

	var asString string
	if err := json.Unmarshal(raw, &asString); err == nil {
		return asString, nil
	}

	var blocks []apicompat.AnthropicContentBlock
	if err := json.Unmarshal(raw, &blocks); err != nil {
		return "", fmt.Errorf("kiro: decode system: %w", err)
	}

	parts := make([]string, 0, len(blocks))
	for _, b := range blocks {
		if b.Type == "text" && b.Text != "" {
			parts = append(parts, b.Text)
		}
	}
	return strings.Join(parts, "\n\n"), nil
}

// FromAnthropic 把 Anthropic 消息数组转成中间形态。
// 不做角色规整 —— 那是 MergeAdjacent / EnsureFirstIsUser / EnsureAlternating 的职责。
func FromAnthropic(req *apicompat.AnthropicRequest) ([]Msg, error) {
	if req == nil {
		return nil, nil
	}

	msgs := make([]Msg, 0, len(req.Messages))
	for i, m := range req.Messages {
		msg, err := convertMessage(m)
		if err != nil {
			return nil, fmt.Errorf("kiro: message[%d]: %w", i, err)
		}
		msgs = append(msgs, msg)
	}
	return msgs, nil
}

func convertMessage(m apicompat.AnthropicMessage) (Msg, error) {
	out := Msg{Role: m.Role}

	// content 可能是裸字符串。
	var asString string
	if err := json.Unmarshal(m.Content, &asString); err == nil {
		out.Text = asString
		return out, nil
	}

	var blocks []apicompat.AnthropicContentBlock
	if err := json.Unmarshal(m.Content, &blocks); err != nil {
		return Msg{}, fmt.Errorf("decode content: %w", err)
	}

	var texts []string
	for _, b := range blocks {
		switch b.Type {
		case "text":
			if b.Text != "" {
				texts = append(texts, b.Text)
			}

		case "thinking", "redacted_thinking":
			// Kiro 没有原生 reasoning，历史里的 thinking 块直接丢弃，
			// 不回传给上游（无 signature 的思考块回传只会引发校验问题）。

		case "image":
			img, err := convertImage(b.Source)
			if err != nil {
				return Msg{}, err
			}
			out.Images = append(out.Images, img)

		case "tool_use":
			out.ToolCalls = append(out.ToolCalls, ToolCall{
				ID:    b.ID,
				Name:  b.Name,
				Input: b.Input,
			})

		case "tool_result":
			text, err := flattenToolResultContent(b.Content)
			if err != nil {
				return Msg{}, err
			}
			out.ToolResults = append(out.ToolResults, ToolResult{
				ToolUseID: b.ToolUseID,
				Text:      text,
				IsError:   b.IsError,
			})

		default:
			return Msg{}, fmt.Errorf("%w: %q", ErrUnsupportedBlock, b.Type)
		}
	}

	out.Text = strings.Join(texts, "\n\n")
	return out, nil
}

func convertImage(src *apicompat.AnthropicImageSource) (Image, error) {
	if src == nil || src.Type != "base64" {
		return Image{}, ErrUnsupportedImageSource
	}

	format := src.MediaType
	if idx := strings.Index(format, "/"); idx >= 0 {
		format = format[idx+1:]
	}
	if format == "" {
		format = "jpeg"
	}

	return Image{Format: format, Data: src.Data}, nil
}

// flattenToolResultContent 把 tool_result 的 content（string 或 block 数组）压平成文本。
func flattenToolResultContent(raw json.RawMessage) (string, error) {
	if len(raw) == 0 {
		return "", nil
	}

	var asString string
	if err := json.Unmarshal(raw, &asString); err == nil {
		return asString, nil
	}

	var blocks []apicompat.AnthropicContentBlock
	if err := json.Unmarshal(raw, &blocks); err != nil {
		return "", fmt.Errorf("decode tool_result content: %w", err)
	}

	parts := make([]string, 0, len(blocks))
	for _, b := range blocks {
		if b.Type == "text" && b.Text != "" {
			parts = append(parts, b.Text)
		}
	}
	return strings.Join(parts, "\n\n"), nil
}

// MergeAdjacent 合并连续同角色的消息。
// 文本用空行拼接，图片 / 工具调用 / 工具结果按序追加 —— 任何一项丢失都会让上游
// 看到不完整的工具轮次。
func MergeAdjacent(msgs []Msg) []Msg {
	if len(msgs) == 0 {
		return msgs
	}

	out := make([]Msg, 0, len(msgs))
	for _, m := range msgs {
		if len(out) == 0 || out[len(out)-1].Role != m.Role {
			out = append(out, m)
			continue
		}

		prev := &out[len(out)-1]
		switch {
		case prev.Text == "":
			prev.Text = m.Text
		case m.Text != "":
			prev.Text += "\n\n" + m.Text
		}
		prev.Images = append(prev.Images, m.Images...)
		prev.ToolCalls = append(prev.ToolCalls, m.ToolCalls...)
		prev.ToolResults = append(prev.ToolResults, m.ToolResults...)
	}
	return out
}

// EnsureFirstIsUser 丢弃开头的 assistant 消息 —— Kiro 要求会话以 user 开始。
func EnsureFirstIsUser(msgs []Msg) []Msg {
	for i, m := range msgs {
		if m.Role == "user" {
			return msgs[i:]
		}
	}
	return nil
}

// EnsureAlternating 保证 user/assistant 严格交替，必要时插入占位消息。
// MergeAdjacent 之后理论上不会触发，保留为防御性不变式。
func EnsureAlternating(msgs []Msg) []Msg {
	if len(msgs) < 2 {
		return msgs
	}

	out := make([]Msg, 0, len(msgs))
	out = append(out, msgs[0])
	for _, m := range msgs[1:] {
		if out[len(out)-1].Role == m.Role {
			filler := "assistant"
			if m.Role == "assistant" {
				filler = "user"
			}
			out = append(out, Msg{Role: filler, Text: "(continued)"})
		}
		out = append(out, m)
	}
	return out
}

// StripToolContent 清空所有工具调用与结果。
// 请求未声明 tools 时必须调用 —— 否则上游会因「引用了未声明的工具」而拒绝。
func StripToolContent(msgs []Msg) []Msg {
	out := make([]Msg, len(msgs))
	copy(out, msgs)
	for i := range out {
		out[i].ToolCalls = nil
		out[i].ToolResults = nil
	}
	return out
}
