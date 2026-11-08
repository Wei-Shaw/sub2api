package service

// 「由 sub2api 压缩」（accounts.extra.openai_compact_mode = "sub2api"）。
//
// Codex 的远程压缩 v2 要求上游产出一个 compaction item，其中 encrypted_content
// 是上游后端生成的密文状态——兼容上游（如 DeepSeek）不实现这套协议，压缩请求
// 只能 503。
//
// 本模式把压缩搬到 sub2api 内部完成：
//  1. 收到压缩请求（/v1/responses/compact 或 input 里带 compaction_trigger），
//     把历史摊平成文本，调一次上游模型要一段摘要；
//  2. 把摘要封进 compaction_summary item 的 encrypted_content 返回给客户端。
//     客户端从不解密该字段，只会原样回放；
//  3. 后续回合 rehydrateS2ACompactionItems 认出自家 item（id 前缀 cmp_s2a_），
//     把摘要还原成一条 developer 消息再转发给上游。
//
// 因此整个过程不依赖上游的加密状态，能力完全由 sub2api 提供。开关默认关闭，
// 只有显式选择该模式的账号才会进入这条链路。

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

const (
	// compactS2AIDPrefix 是 sub2api 自产压缩 item 的 id 前缀，用于回放识别。
	compactS2AIDPrefix = "cmp_s2a_"
	// compactS2APayloadPrefix 是 encrypted_content 的自描述前缀。
	compactS2APayloadPrefix = "s2a1:"
	// compactS2ASummaryLimit 限制喂给摘要模型的历史长度（字符）。
	compactS2ASummaryLimit = 400_000
)

const compactS2ASystemPrompt = `You compact an AI agent conversation so the agent can continue from the summary alone.

Produce a dense, structured handover note covering:
- The user's goal and any explicit constraints or preferences
- Decisions made and rejected approaches (with reasons)
- Current state of work: files touched, commands run, results
- Open questions, next steps, and anything needed to resume safely

Preserve exact identifiers (paths, commands, error strings, config keys). Do not invent facts.
Reply with the handover note only.`

// UsesSub2APICompact 报告账号是否启用「由 sub2api 压缩」。
func (a *Account) UsesSub2APICompact() bool {
	return a != nil && a.IsOpenAI() && a.GetOpenAICompactMode() == OpenAICompactModeSub2API
}

// IsOpenAICompactRequest 判断本次请求是否需要做压缩（路径或 compaction_trigger）。
func IsOpenAICompactRequest(c *gin.Context, body []byte) bool {
	return isOpenAIResponsesCompactPath(c) || HasCompactionTriggerInInput(body)
}

// forwardOpenAICompactViaSub2API 在本地完成压缩，不把压缩请求发给上游。
func (s *OpenAIGatewayService) forwardOpenAICompactViaSub2API(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	body []byte,
	startTime time.Time,
) (*OpenAIForwardResult, error) {
	requestedModel := strings.TrimSpace(gjson.GetBytes(body, "model").String())
	if requestedModel == "" {
		writeOpenAIResponsesFallbackError(c, http.StatusBadRequest, "invalid_request_error", "model is required")
		return nil, fmt.Errorf("missing model in compact request")
	}
	stream := gjson.GetBytes(body, "stream").Bool()

	history := compactS2AFlattenInput(gjson.GetBytes(body, "input"))
	if strings.TrimSpace(history) == "" {
		writeOpenAIResponsesFallbackError(c, http.StatusBadRequest, "invalid_request_error", "input is empty")
		return nil, fmt.Errorf("empty input for compact request")
	}

	upstreamModel := normalizeOpenAIModelForUpstream(account, requestedModel)
	ccBody, err := json.Marshal(map[string]any{
		"model":  upstreamModel,
		"stream": false,
		"messages": []any{
			map[string]any{"role": "system", "content": compactS2ASystemPrompt},
			map[string]any{"role": "user", "content": "[Conversation transcript]\n\n" + history + "\n\n[End of transcript]\n\nWrite the handover note now."},
		},
	})
	if err != nil {
		writeOpenAIResponsesFallbackError(c, http.StatusInternalServerError, "api_error", "Failed to build compact request")
		return nil, fmt.Errorf("marshal s2a compact request: %w", err)
	}

	apiKey, targetURL, err := s.resolveCCFallbackTarget(account)
	if err != nil {
		writeOpenAIResponsesFallbackError(c, http.StatusBadGateway, "api_error", err.Error())
		return nil, err
	}
	resp, err := s.sendCCUpstreamRequest(ctx, c, account, targetURL, ccBody, false, apiKey, account.GetOpenAIUserAgent(), "")
	if err != nil {
		writeOpenAIResponsesFallbackError(c, http.StatusBadGateway, "api_error", "Failed to reach upstream")
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= 400 {
		return s.handleErrorResponse(ctx, resp, c, account, ccBody, requestedModel)
	}

	ccResp, usage, err := s.readCCUpstreamJSONResponse(c, resp, writeOpenAIResponsesFallbackError)
	if err != nil {
		return nil, err
	}
	summary := compactS2AResponseText(ccResp)
	if strings.TrimSpace(summary) == "" {
		writeOpenAIResponsesFallbackError(c, http.StatusBadGateway, "api_error", "upstream returned an empty compaction summary")
		return nil, fmt.Errorf("empty compaction summary from upstream")
	}

	item := compactS2ABuildItem(summary, upstreamModel)
	responseID := "resp_" + item.ID
	if stream {
		compactS2AWriteStream(c, requestedModel, responseID, item)
	} else {
		compactS2AWriteJSON(c, requestedModel, responseID, item, usage)
	}

	return &OpenAIForwardResult{
		RequestID:     responseID,
		ResponseID:    responseID,
		Usage:         usage,
		Model:         requestedModel,
		BillingModel:  requestedModel,
		UpstreamModel: upstreamModel,
		Stream:        stream,
		Duration:      time.Since(startTime),
	}, nil
}

// compactS2AFlattenInput 把 Responses input 摊平成可读文本。
func compactS2AFlattenInput(input gjson.Result) string {
	if !input.IsArray() {
		if input.Type == gjson.String {
			return truncateCompactS2AText(input.String())
		}
		return ""
	}
	var b strings.Builder
	input.ForEach(func(_, item gjson.Result) bool {
		switch strings.TrimSpace(item.Get("type").String()) {
		case "message":
			role := strings.TrimSpace(item.Get("role").String())
			text := compactS2AMessageText(item.Get("content"))
			if text != "" {
				b.WriteString(strings.ToUpper(role))
				b.WriteString(": ")
				b.WriteString(text)
				b.WriteString("\n\n")
			}
		case "function_call", "custom_tool_call":
			name := item.Get("name").String()
			payload := item.Get("arguments").String()
			if payload == "" {
				payload = item.Get("input").String()
			}
			b.WriteString("[tool call] " + name + "(" + payload + ")\n\n")
		case "function_call_output", "custom_tool_call_output":
			b.WriteString("[tool result] " + item.Get("output").String() + "\n\n")
		case "compaction", "compaction_summary":
			if summary, ok := compactS2ADecodeItem(item); ok {
				b.WriteString("[earlier compacted context]\n" + summary + "\n\n")
			}
		}
		return true
	})
	return truncateCompactS2AText(b.String())
}

func compactS2AMessageText(content gjson.Result) string {
	if content.Type == gjson.String {
		return content.String()
	}
	var parts []string
	for _, part := range content.Array() {
		if text := part.Get("text").String(); text != "" {
			parts = append(parts, text)
		}
	}
	return strings.Join(parts, "\n")
}

// truncateCompactS2AText 超长时保留头部与尾部，中间省略：压缩要的是全貌，
// 头尾通常分别是任务设定与最新进展。
func truncateCompactS2AText(text string) string {
	if len(text) <= compactS2ASummaryLimit {
		return text
	}
	head := compactS2ASummaryLimit * 3 / 10
	tail := compactS2ASummaryLimit - head
	return text[:head] + "\n\n[... middle of conversation omitted ...]\n\n" + text[len(text)-tail:]
}

func compactS2AResponseText(resp *apicompat.ChatCompletionsResponse) string {
	if resp == nil || len(resp.Choices) == 0 {
		return ""
	}
	raw := bytes.TrimSpace(resp.Choices[0].Message.Content)
	if len(raw) == 0 {
		return ""
	}
	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		return strings.TrimSpace(text)
	}
	var parts []struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal(raw, &parts); err == nil {
		var b strings.Builder
		for _, part := range parts {
			b.WriteString(part.Text)
		}
		return strings.TrimSpace(b.String())
	}
	return ""
}

// compactS2ABuildItem 构造发回客户端的 compaction_summary item。
func compactS2ABuildItem(summary, model string) compactS2AItem {
	payload, _ := json.Marshal(map[string]any{"v": 1, "summary": summary, "model": model})
	encoded := compactS2APayloadPrefix + base64.RawURLEncoding.EncodeToString(payload)
	sum := sha256.Sum256([]byte(encoded))
	return compactS2AItem{
		Type:             "compaction_summary",
		ID:               compactS2AIDPrefix + hex.EncodeToString(sum[:12]),
		Status:           "completed",
		EncryptedContent: encoded,
		Summary:          summary,
	}
}

type compactS2AItem struct {
	Type             string `json:"type"`
	ID               string `json:"id"`
	Status           string `json:"status,omitempty"`
	EncryptedContent string `json:"encrypted_content"`
	Summary          string `json:"-"`
}

func (i compactS2AItem) marshal() map[string]any {
	out := map[string]any{
		"type":              i.Type,
		"id":                i.ID,
		"encrypted_content": i.EncryptedContent,
	}
	if i.Status != "" {
		out["status"] = i.Status
	}
	return out
}

// compactS2ADecodeItem 解出自家压缩 item 的摘要。
func compactS2ADecodeItem(item gjson.Result) (string, bool) {
	if !strings.HasPrefix(item.Get("id").String(), compactS2AIDPrefix) {
		return "", false
	}
	encoded := item.Get("encrypted_content").String()
	if !strings.HasPrefix(encoded, compactS2APayloadPrefix) {
		return "", false
	}
	raw, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(encoded, compactS2APayloadPrefix))
	if err != nil {
		return "", false
	}
	var payload struct {
		Summary string `json:"summary"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil || strings.TrimSpace(payload.Summary) == "" {
		return "", false
	}
	return payload.Summary, true
}

// rehydrateS2ACompactionItems 把 input 里自己产出的压缩 item 还原成 developer
// 消息。无法识别的压缩 item 原样保留，交给后续链路处理。
func rehydrateS2ACompactionItems(body []byte) ([]byte, bool) {
	if len(body) == 0 || !strings.Contains(string(body), compactS2AIDPrefix) {
		return body, false
	}
	var root map[string]any
	if err := decodeOpenAIJSONUseNumber(body, &root); err != nil {
		return body, false
	}
	input, ok := root["input"].([]any)
	if !ok {
		return body, false
	}
	changed := false
	for idx, raw := range input {
		item, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		id, _ := item["id"].(string)
		if !strings.HasPrefix(id, compactS2AIDPrefix) {
			continue
		}
		encoded, _ := item["encrypted_content"].(string)
		if !strings.HasPrefix(encoded, compactS2APayloadPrefix) {
			continue
		}
		decoded, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(encoded, compactS2APayloadPrefix))
		if err != nil {
			continue
		}
		var payload struct {
			Summary string `json:"summary"`
		}
		if err := json.Unmarshal(decoded, &payload); err != nil || strings.TrimSpace(payload.Summary) == "" {
			continue
		}
		input[idx] = map[string]any{
			"type": "message",
			"role": "developer",
			"content": []any{
				map[string]any{
					"type": "input_text",
					"text": "[Compacted earlier context]\n\n" + payload.Summary,
				},
			},
		}
		changed = true
	}
	if !changed {
		return body, false
	}
	root["input"] = input
	rebuilt, err := marshalOpenAIUpstreamJSON(root)
	if err != nil {
		return body, false
	}
	return rebuilt, true
}

func compactS2AWriteJSON(c *gin.Context, model, responseID string, item compactS2AItem, usage OpenAIUsage) {
	c.JSON(http.StatusOK, map[string]any{
		"id":         responseID,
		"object":     "response",
		"created_at": time.Now().Unix(),
		"status":     "completed",
		"model":      model,
		"output":     []any{item.marshal()},
		"usage": map[string]any{
			"input_tokens":  usage.InputTokens,
			"output_tokens": usage.OutputTokens,
			"total_tokens":  usage.InputTokens + usage.OutputTokens,
		},
	})
}

func compactS2AWriteStream(c *gin.Context, model, responseID string, item compactS2AItem) {
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Status(http.StatusOK)
	send := func(eventType string, payload map[string]any) {
		payload["type"] = eventType
		data, err := json.Marshal(payload)
		if err != nil {
			return
		}
		_, _ = c.Writer.WriteString("event: " + eventType + "\ndata: " + string(data) + "\n\n")
		c.Writer.Flush()
	}
	created := map[string]any{
		"id": responseID, "object": "response", "created_at": time.Now().Unix(),
		"status": "in_progress", "model": model, "output": []any{},
	}
	send("response.created", map[string]any{"response": created})
	send("response.output_item.added", map[string]any{"output_index": 0, "item": item.marshal()})
	send("response.output_item.done", map[string]any{"output_index": 0, "item": item.marshal()})
	completed := map[string]any{
		"id": responseID, "object": "response", "created_at": time.Now().Unix(),
		"status": "completed", "model": model, "output": []any{item.marshal()},
	}
	send("response.completed", map[string]any{"response": completed})
}
