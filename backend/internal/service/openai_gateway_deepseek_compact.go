package service

import (
	"bufio"
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/tidwall/gjson"
)

const (
	deepSeekResponsesEndpoint          = "/responses"
	deepSeekCompactEnvelopePrefix      = "sub2api.deepseek.compact.v1."
	deepSeekCompactCipherDomain        = "sub2api/deepseek-remote-compaction/v1"
	deepSeekCompactMaxSummaryBytes     = 128 * 1024
	deepSeekCompactMaxEnvelopeBytes    = 256 * 1024
	deepSeekCompactMaxSSEBytes         = 8 * 1024 * 1024
	deepSeekCompactMaxSSELineBytes     = 1024 * 1024
	deepSeekCompactSummaryMaxTokens    = 8192
	deepSeekCompactEnvelopeVersion     = 1
	deepSeekCompactMissingUsageMsg     = "DeepSeek compact upstream returned a successful response without billable usage"
	deepSeekCompactInvalidStateMessage = "invalid DeepSeek compact encrypted_content"
	deepSeekCompactCheckpointPreamble  = "This is an automatically generated checkpoint condensing an earlier span of the conversation to free up context. Treat the captured context as established background and build on it without restating it. Continue the task directly from the messages that follow, without acknowledging this checkpoint."
	deepSeekCompactInstruction         = `You are performing a CONTEXT CHECKPOINT COMPACTION. Create a handoff summary for another LLM that will resume the task.

Include:
- Current progress and key decisions made
- Important context, constraints, or user preferences
- What remains to be done (clear next steps)
- Any critical data, examples, or references needed to continue

Be concise, structured, and focused on helping the next LLM seamlessly continue the work.`
)

var ErrDeepSeekCompactInvalidEncryptedContent = errors.New(deepSeekCompactInvalidStateMessage)

type deepSeekCompactEnvelope struct {
	Version    int    `json:"version"`
	Checkpoint string `json:"checkpoint"`
}

type deepSeekCompactStreamResult struct {
	Summary      string
	Usage        OpenAIUsage
	TotalTokens  int
	ResponseID   string
	FirstTokenMs *int
}

func frameDeepSeekCompactSummary(summary string) string {
	return deepSeekCompactCheckpointPreamble + "\n\n<compacted-summary>" + summary + "</compacted-summary>"
}

func deepSeekCompactUserAAD(ctx context.Context) ([]byte, error) {
	if ctx == nil {
		return nil, errors.New("authenticated user context is required for DeepSeek remote compaction")
	}
	userID, ok := ctx.Value(ctxkey.UserID).(int64)
	if !ok || userID <= 0 {
		return nil, errors.New("authenticated user context is required for DeepSeek remote compaction")
	}
	return []byte(deepSeekCompactCipherDomain + ":user:" + strconv.FormatInt(userID, 10)), nil
}

func (s *OpenAIGatewayService) deepSeekCompactAEAD() (cipher.AEAD, error) {
	if s == nil || s.cfg == nil || strings.TrimSpace(s.cfg.JWT.Secret) == "" {
		return nil, errors.New("DeepSeek remote compaction requires a persistent JWT secret")
	}
	key := sha256.Sum256([]byte(deepSeekCompactCipherDomain + "\x00" + s.cfg.JWT.Secret))
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, fmt.Errorf("initialize DeepSeek compact cipher: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("initialize DeepSeek compact AEAD: %w", err)
	}
	return aead, nil
}

func (s *OpenAIGatewayService) sealDeepSeekCompactCheckpoint(ctx context.Context, checkpoint string) (string, error) {
	if !utf8.ValidString(checkpoint) || len(checkpoint) > deepSeekCompactMaxSummaryBytes {
		return "", errors.New("DeepSeek compact checkpoint exceeds the supported size")
	}
	payload, err := marshalOpenAIUpstreamJSON(deepSeekCompactEnvelope{
		Version:    deepSeekCompactEnvelopeVersion,
		Checkpoint: checkpoint,
	})
	if err != nil {
		return "", fmt.Errorf("encode DeepSeek compact checkpoint: %w", err)
	}
	aead, err := s.deepSeekCompactAEAD()
	if err != nil {
		return "", err
	}
	nonce := make([]byte, aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("generate DeepSeek compact nonce: %w", err)
	}
	aad, err := deepSeekCompactUserAAD(ctx)
	if err != nil {
		return "", err
	}
	sealed := aead.Seal(nonce, nonce, payload, aad)
	envelope := deepSeekCompactEnvelopePrefix + base64.RawURLEncoding.EncodeToString(sealed)
	if len(envelope) > deepSeekCompactMaxEnvelopeBytes {
		return "", errors.New("DeepSeek compact encrypted_content exceeds the supported size")
	}
	return envelope, nil
}

func (s *OpenAIGatewayService) openDeepSeekCompactCheckpoint(ctx context.Context, envelope string) (string, error) {
	invalid := func() (string, error) { return "", ErrDeepSeekCompactInvalidEncryptedContent }
	if !strings.HasPrefix(envelope, deepSeekCompactEnvelopePrefix) || len(envelope) > deepSeekCompactMaxEnvelopeBytes {
		return invalid()
	}
	encoded := strings.TrimPrefix(envelope, deepSeekCompactEnvelopePrefix)
	sealed, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return invalid()
	}
	aead, err := s.deepSeekCompactAEAD()
	if err != nil || len(sealed) < aead.NonceSize()+aead.Overhead() {
		return invalid()
	}
	aad, err := deepSeekCompactUserAAD(ctx)
	if err != nil {
		return invalid()
	}
	payload, err := aead.Open(nil, sealed[:aead.NonceSize()], sealed[aead.NonceSize():], aad)
	if err != nil || len(payload) > deepSeekCompactMaxEnvelopeBytes || !utf8.Valid(payload) {
		return invalid()
	}
	var state deepSeekCompactEnvelope
	if err := json.Unmarshal(payload, &state); err != nil ||
		state.Version != deepSeekCompactEnvelopeVersion ||
		strings.TrimSpace(state.Checkpoint) == "" ||
		len(state.Checkpoint) > deepSeekCompactMaxSummaryBytes ||
		!utf8.ValidString(state.Checkpoint) {
		return invalid()
	}
	return state.Checkpoint, nil
}

func (s *OpenAIGatewayService) deepSeekResponsesCompactUpstream(account *Account) (targetURL, token string, err error) {
	if account == nil || !isDeepSeekResponsesUpstream(account) {
		return "", "", errors.New("deepseek compact requires a DeepSeek Responses upstream")
	}
	token = strings.TrimSpace(account.GetOpenAIProtocolAPIKey())
	if token == "" {
		return "", "", fmt.Errorf("account %d missing api_key", account.ID)
	}
	baseURL := strings.TrimSpace(account.GetOpenAIBaseURL())
	if baseURL == "" {
		return "", "", fmt.Errorf("account %d missing base_url", account.ID)
	}
	validated, err := s.validateUpstreamBaseURL(baseURL)
	if err != nil {
		return "", "", fmt.Errorf("invalid deepseek compact base_url: %w", err)
	}
	return buildOpenAIResponsesURLForPlatform(account.Platform, validated), token, nil
}

// RestoreDeepSeekCompactInput 把本网关签发的 compaction item 还原成 DeepSeek
// 能读的 user 检查点。foreign 的 OpenAI/Grok blob 在 DeepSeek 主机上直接丢掉，
// 否则上游会看到它不认识的 type=compaction。
func (s *OpenAIGatewayService) RestoreDeepSeekCompactInput(ctx context.Context, body []byte) ([]byte, bool, error) {
	if len(body) == 0 || !gjson.ValidBytes(body) {
		return body, false, nil
	}
	input := gjson.GetBytes(body, "input")
	if !input.IsArray() {
		return body, false, nil
	}
	hasCompact := false
	input.ForEach(func(_, item gjson.Result) bool {
		itemType := strings.TrimSpace(item.Get("type").String())
		if itemType == "compaction" || itemType == "compaction_summary" {
			hasCompact = true
			return false
		}
		return true
	})
	if !hasCompact {
		return body, false, nil
	}

	var request map[string]any
	if err := decodeOpenAIJSONUseNumber(body, &request); err != nil {
		return body, false, nil
	}
	items, ok := request["input"].([]any)
	if !ok {
		return body, false, ErrDeepSeekCompactInvalidEncryptedContent
	}
	rewritten := make([]any, 0, len(items))
	changed := false
	for _, raw := range items {
		item, ok := raw.(map[string]any)
		if !ok {
			rewritten = append(rewritten, raw)
			continue
		}
		itemType := strings.TrimSpace(firstNonEmptyString(item["type"]))
		if itemType != "compaction" && itemType != "compaction_summary" {
			rewritten = append(rewritten, raw)
			continue
		}
		encrypted, _ := item["encrypted_content"].(string)
		if !strings.HasPrefix(encrypted, deepSeekCompactEnvelopePrefix) {
			changed = true
			continue
		}
		checkpoint, err := s.openDeepSeekCompactCheckpoint(ctx, encrypted)
		if err != nil {
			return nil, false, ErrDeepSeekCompactInvalidEncryptedContent
		}
		rewritten = append(rewritten, map[string]any{
			"type": "message",
			"role": "user",
			"content": []any{map[string]any{
				"type": "input_text",
				"text": checkpoint,
			}},
		})
		changed = true
	}
	if !changed {
		return body, false, nil
	}
	request["input"] = rewritten
	restored, err := marshalOpenAIUpstreamJSON(request)
	if err != nil {
		return nil, false, ErrDeepSeekCompactInvalidEncryptedContent
	}
	return restored, true, nil
}

func deepSeekCompactContentContainsImage(raw json.RawMessage) bool {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) || (len(trimmed) > 0 && trimmed[0] == '"') {
		return false
	}
	parts := []json.RawMessage{trimmed}
	if trimmed[0] == '[' {
		if err := json.Unmarshal(trimmed, &parts); err != nil {
			return false
		}
	}
	for _, part := range parts {
		partType := strings.ToLower(strings.TrimSpace(gjson.GetBytes(part, "type").String()))
		switch partType {
		case "image", "input_image", "output_image", "image_url", "image_generation_call":
			return true
		}
	}
	return false
}

func deepSeekCompactInputItemContainsImage(raw json.RawMessage) bool {
	itemType := strings.ToLower(strings.TrimSpace(gjson.GetBytes(raw, "type").String()))
	switch itemType {
	case "image", "input_image", "output_image", "image_url", "image_generation_call":
		return true
	case "message", "":
		return deepSeekCompactContentContainsImage(json.RawMessage(gjson.GetBytes(raw, "content").Raw))
	case "function_call_output", "custom_tool_call_output", "tool_search_output":
		return deepSeekCompactContentContainsImage(json.RawMessage(gjson.GetBytes(raw, "output").Raw))
	default:
		return false
	}
}

func validateDeepSeekCompactToolPairs(items []json.RawMessage) error {
	pending := make(map[string]struct{})
	for _, raw := range items {
		itemType := strings.TrimSpace(gjson.GetBytes(raw, "type").String())
		switch itemType {
		case "function_call", "custom_tool_call", "tool_search_call":
			callID := strings.TrimSpace(gjson.GetBytes(raw, "call_id").String())
			if callID == "" {
				return errors.New("DeepSeek remote compaction contains a tool call without call_id")
			}
			if _, exists := pending[callID]; exists {
				return errors.New("DeepSeek remote compaction contains duplicate tool call ids")
			}
			pending[callID] = struct{}{}
		case "function_call_output", "custom_tool_call_output", "tool_search_output":
			callID := strings.TrimSpace(gjson.GetBytes(raw, "call_id").String())
			if _, exists := pending[callID]; callID == "" || !exists {
				return errors.New("DeepSeek remote compaction contains an unpaired tool result")
			}
			delete(pending, callID)
		}
	}
	if len(pending) > 0 {
		return errors.New("DeepSeek remote compaction contains an unanswered tool call")
	}
	return nil
}

func deepSeekCompactResponsesRequest(body []byte, upstreamModel string) ([]byte, error) {
	var source map[string]json.RawMessage
	if err := json.Unmarshal(body, &source); err != nil {
		return nil, fmt.Errorf("parse DeepSeek compact request: %w", err)
	}
	if previousRaw, ok := source["previous_response_id"]; ok {
		var previous *string
		if err := json.Unmarshal(previousRaw, &previous); err != nil {
			return nil, errors.New("DeepSeek remote compaction previous_response_id must be a string or null")
		}
		if previous != nil && strings.TrimSpace(*previous) != "" {
			return nil, errors.New("DeepSeek remote compaction does not support previous_response_id")
		}
	}
	inputRaw, ok := source["input"]
	if !ok {
		return nil, errors.New("DeepSeek remote compaction requires input")
	}
	var items []json.RawMessage
	if err := json.Unmarshal(inputRaw, &items); err != nil {
		return nil, errors.New("DeepSeek remote compaction input must be an array")
	}
	triggerIndex := -1
	for i, raw := range items {
		if strings.TrimSpace(gjson.GetBytes(raw, "type").String()) != "compaction_trigger" {
			continue
		}
		if triggerIndex >= 0 {
			return nil, errors.New("DeepSeek remote compaction requires exactly one compaction_trigger")
		}
		triggerIndex = i
	}
	if triggerIndex < 0 || triggerIndex != len(items)-1 {
		return nil, errors.New("DeepSeek remote compaction requires a final compaction_trigger")
	}
	if triggerIndex == 0 {
		return nil, errors.New("DeepSeek remote compaction has no conversation history to summarize")
	}
	history := items[:triggerIndex]
	for _, raw := range history {
		if deepSeekCompactInputItemContainsImage(raw) {
			return nil, errors.New("DeepSeek remote compaction does not support image content")
		}
	}
	if err := validateDeepSeekCompactToolPairs(history); err != nil {
		return nil, err
	}

	promptItem, err := marshalOpenAIUpstreamJSON(map[string]any{
		"type": "message",
		"role": "user",
		"content": []any{map[string]any{
			"type": "input_text",
			"text": deepSeekCompactInstruction,
		}},
	})
	if err != nil {
		return nil, fmt.Errorf("encode DeepSeek compact checkpoint prompt: %w", err)
	}
	upstreamInput, err := marshalOpenAIUpstreamJSON(append(history, promptItem))
	if err != nil {
		return nil, fmt.Errorf("encode DeepSeek compact input: %w", err)
	}

	modelRaw, _ := json.Marshal(upstreamModel)
	request := map[string]json.RawMessage{
		"model":             modelRaw,
		"input":             upstreamInput,
		"stream":            json.RawMessage("true"),
		"max_output_tokens": json.RawMessage(strconv.Itoa(deepSeekCompactSummaryMaxTokens)),
	}
	if instructions, ok := source["instructions"]; ok {
		request["instructions"] = instructions
	}
	if reasoningRaw, ok := source["reasoning"]; ok {
		var reasoning map[string]json.RawMessage
		if err := json.Unmarshal(reasoningRaw, &reasoning); err != nil {
			return nil, errors.New("DeepSeek remote compaction reasoning must be an object")
		}
		if effortRaw, ok := reasoning["effort"]; ok {
			var effort string
			if err := json.Unmarshal(effortRaw, &effort); err != nil || strings.TrimSpace(effort) == "" {
				return nil, errors.New("DeepSeek remote compaction reasoning.effort must be a non-empty string")
			}
			encodedReasoning, err := marshalOpenAIUpstreamJSON(map[string]json.RawMessage{"effort": effortRaw})
			if err != nil {
				return nil, fmt.Errorf("encode DeepSeek compact reasoning: %w", err)
			}
			request["reasoning"] = encodedReasoning
		}
	}
	encoded, err := marshalOpenAIUpstreamJSON(request)
	if err != nil {
		return nil, fmt.Errorf("encode DeepSeek compact Responses request: %w", err)
	}
	return encoded, nil
}

func deepSeekCompactTerminalSummary(response gjson.Result) (string, error) {
	output := response.Get("output")
	if !output.Exists() || !output.IsArray() {
		return "", errors.New("DeepSeek compact response.completed has invalid output")
	}
	items := output.Array()
	assistantSeen := false
	var summary strings.Builder
	for index, item := range items {
		switch strings.TrimSpace(item.Get("type").String()) {
		case "reasoning":
			if assistantSeen {
				return "", errors.New("DeepSeek compact response has reasoning after its assistant message")
			}
		case "message":
			if assistantSeen || index != len(items)-1 ||
				strings.TrimSpace(item.Get("role").String()) != "assistant" ||
				strings.TrimSpace(item.Get("status").String()) != "completed" {
				return "", errors.New("DeepSeek compact response must end with one completed assistant message")
			}
			assistantSeen = true
			content := item.Get("content")
			if !content.Exists() || !content.IsArray() || len(content.Array()) == 0 {
				return "", errors.New("DeepSeek compact assistant message has invalid content")
			}
			for _, part := range content.Array() {
				if strings.TrimSpace(part.Get("type").String()) != "output_text" {
					return "", errors.New("DeepSeek compact assistant message contains non-output_text content")
				}
				text := part.Get("text")
				if !text.Exists() || text.Type != gjson.String {
					return "", errors.New("DeepSeek compact assistant output_text has invalid text")
				}
				if summary.Len()+len(text.String()) > deepSeekCompactMaxSummaryBytes {
					return "", errors.New("DeepSeek compact summary exceeds the supported size")
				}
				_, _ = summary.WriteString(text.String())
			}
		default:
			return "", errors.New("DeepSeek compact response contains unsupported output")
		}
	}
	if !assistantSeen || strings.TrimSpace(summary.String()) == "" {
		return "", errors.New("DeepSeek compaction produced no text summary content")
	}
	return summary.String(), nil
}

func deepSeekCompactHasBillableUsage(usage OpenAIUsage) bool {
	return usage.InputTokens > 0 || usage.OutputTokens > 0
}

func (s *OpenAIGatewayService) readDeepSeekCompactResponsesStream(c *gin.Context, resp *http.Response, startTime time.Time) (deepSeekCompactStreamResult, error) {
	var result deepSeekCompactStreamResult
	if resp == nil || resp.Body == nil {
		return result, errors.New("DeepSeek compact stream has no response body")
	}
	observer := upstreamResponseModelObserverFromContext(c)
	if observer == nil {
		observer = beginUpstreamResponseModelObservation(c)
	}

	var dataLines []string
	currentEventType := ""
	terminalCount := 0
	terminalType := ""
	var protocolErr error
	var rawBytes int

	setProtocolErr := func(err error) {
		if protocolErr == nil {
			protocolErr = err
		}
	}
	processEvent := func() {
		headerType := currentEventType
		currentEventType = ""
		if len(dataLines) == 0 {
			return
		}
		payload := strings.TrimSpace(strings.Join(dataLines, "\n"))
		dataLines = dataLines[:0]
		if payload == "" {
			return
		}
		if terminalCount > 0 {
			setProtocolErr(errors.New("DeepSeek compact stream contains data after its terminal event"))
		}
		if payload == "[DONE]" {
			setProtocolErr(errors.New("DeepSeek Responses compact stream must not contain [DONE]"))
			return
		}
		payloadBytes := []byte(payload)
		if !gjson.ValidBytes(payloadBytes) {
			setProtocolErr(errors.New("DeepSeek compact stream returned malformed JSON data"))
			return
		}
		eventType := strings.TrimSpace(gjson.GetBytes(payloadBytes, "type").String())
		if eventType == "" {
			eventType = headerType
		} else if headerType != "" && headerType != eventType {
			setProtocolErr(errors.New("DeepSeek compact stream event type does not match its payload"))
		}
		observer.ObserveOpenAI(payloadBytes, eventType)
		if usage, ok := extractOpenAIUsageFromJSONBytes(payloadBytes); ok && deepSeekCompactHasBillableUsage(usage) {
			result.Usage = usage
			totalTokens := int(gjson.GetBytes(payloadBytes, "response.usage.total_tokens").Int())
			if totalTokens <= 0 {
				totalTokens = int(gjson.GetBytes(payloadBytes, "usage.total_tokens").Int())
			}
			result.TotalTokens = totalTokens
		}
		errorPayload := gjson.GetBytes(payloadBytes, "error")
		responseError := gjson.GetBytes(payloadBytes, "response.error")
		if eventType == "error" ||
			(errorPayload.Exists() && errorPayload.Type != gjson.Null) ||
			(responseError.Exists() && responseError.Type != gjson.Null) {
			setProtocolErr(errors.New("DeepSeek compact stream returned an error event"))
		}

		switch eventType {
		case "response.output_text.delta":
			delta := gjson.GetBytes(payloadBytes, "delta")
			if !delta.Exists() || delta.Type != gjson.String {
				setProtocolErr(errors.New("DeepSeek compact output_text delta has invalid text"))
			} else if delta.String() != "" && result.FirstTokenMs == nil {
				elapsed := int(time.Since(startTime).Milliseconds())
				result.FirstTokenMs = &elapsed
			}
		case "response.completed", "response.failed", "response.incomplete", "response.cancelled", "response.canceled":
			terminalCount++
			terminalType = eventType
			response := gjson.GetBytes(payloadBytes, "response")
			result.ResponseID = strings.TrimSpace(response.Get("id").String())
			if eventType != "response.completed" {
				setProtocolErr(fmt.Errorf("DeepSeek compact upstream returned %s", eventType))
				return
			}
			if strings.TrimSpace(response.Get("status").String()) != "completed" {
				setProtocolErr(errors.New("DeepSeek compact response.completed has non-completed status"))
			}
			summary, err := deepSeekCompactTerminalSummary(response)
			if err != nil {
				setProtocolErr(err)
			} else {
				result.Summary = summary
			}
		case "response.created", "response.in_progress", "response.output_item.added",
			"response.output_item.done", "response.content_part.added", "response.content_part.done",
			"response.output_text.done", "response.reasoning_summary_text.delta",
			"response.reasoning_summary_text.done", "response.reasoning_summary_part.added",
			"response.reasoning_summary_part.done", "response.reasoning_text.delta",
			"response.reasoning_text.done":
		case "error":
		default:
			setProtocolErr(fmt.Errorf("DeepSeek compact stream returned unsupported event %q", eventType))
		}
	}

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), deepSeekCompactMaxSSELineBytes)
	for scanner.Scan() {
		line := strings.TrimSuffix(scanner.Text(), "\r")
		rawBytes += len(line) + 1
		if rawBytes > deepSeekCompactMaxSSEBytes {
			return result, errors.New("DeepSeek compact stream exceeds the supported size")
		}
		if line == "" {
			processEvent()
			continue
		}
		if strings.HasPrefix(line, "event:") {
			currentEventType = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
			continue
		}
		if strings.HasPrefix(line, "data:") {
			data := strings.TrimPrefix(strings.TrimPrefix(line, "data:"), " ")
			dataLines = append(dataLines, data)
		}
	}
	if err := scanner.Err(); err != nil {
		return result, fmt.Errorf("read DeepSeek compact stream: %w", err)
	}
	if len(dataLines) > 0 {
		setProtocolErr(errors.New("DeepSeek compact stream ended before a blank-line-dispatched terminal event"))
	}
	if terminalCount != 1 {
		setProtocolErr(fmt.Errorf("DeepSeek compact stream requires exactly one terminal event, got %d", terminalCount))
	}
	if !deepSeekCompactHasBillableUsage(result.Usage) {
		if protocolErr != nil {
			return result, fmt.Errorf("%w: %s", protocolErr, deepSeekCompactMissingUsageMsg)
		}
		return result, errors.New(deepSeekCompactMissingUsageMsg)
	}
	if result.TotalTokens <= 0 {
		result.TotalTokens = result.Usage.InputTokens + result.Usage.OutputTokens
	}
	if protocolErr != nil {
		return result, protocolErr
	}
	if terminalType != "response.completed" {
		return result, errors.New("DeepSeek compact stream did not complete")
	}
	return result, nil
}

func deepSeekCompactResponsesJSON(responseID, itemID, model, encryptedContent string, usage OpenAIUsage, totalTokens int) ([]byte, error) {
	if totalTokens <= 0 {
		totalTokens = usage.InputTokens + usage.OutputTokens
	}
	return marshalOpenAIUpstreamJSON(map[string]any{
		"id":         responseID,
		"object":     "response",
		"created_at": time.Now().Unix(),
		"model":      model,
		"status":     "completed",
		"output": []any{map[string]any{
			"id":                itemID,
			"type":              "compaction",
			"status":            "completed",
			"encrypted_content": encryptedContent,
		}},
		"usage": map[string]any{
			"input_tokens": usage.InputTokens,
			"input_tokens_details": map[string]any{
				"cached_tokens": usage.CacheReadInputTokens,
			},
			"output_tokens": usage.OutputTokens,
			"total_tokens":  totalTokens,
		},
	})
}

// shouldSynthesizeDeepSeekRemoteCompactionRequest 判断这次请求是否必须由网关合成
// Codex remote compact v2 的 compaction item。
//
// DeepSeek /responses 不会产出 type=compaction。上游 handler 对 openai 平台
// 一律按 OpenAI native v2 透传：compaction_trigger 原样打到 DeepSeek，客户端
// 看到 reasoning+message 两条输出、0 条 compaction，于是报
// "remote compaction v2 expected exactly one compaction output item, got 0 from 2 output items"。
func shouldSynthesizeDeepSeekRemoteCompactionRequest(c *gin.Context, account *Account, body []byte) bool {
	if account == nil || !isDeepSeekResponsesUpstream(account) || !HasCompactionTriggerInInput(body) {
		return false
	}
	if isOpenAINativeCompactionV2(c) || gjson.GetBytes(body, "stream").Bool() {
		return true
	}
	return isOpenAIResponsesCompactPath(c)
}

func (s *OpenAIGatewayService) maybeForwardDeepSeekRemoteCompaction(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	body []byte,
) (*OpenAIForwardResult, bool, error) {
	if !shouldSynthesizeDeepSeekRemoteCompactionRequest(c, account, body) {
		return nil, false, nil
	}
	if gjson.GetBytes(body, "stream").Bool() || isOpenAINativeCompactionV2(c) {
		MarkOpenAICompactClientStream(c)
	}
	originalModel := strings.TrimSpace(gjson.GetBytes(body, "model").String())
	if originalModel == "" {
		return nil, true, fmt.Errorf("parse DeepSeek Responses request: model is required")
	}
	billingModel := resolveOpenAIForwardModel(account, originalModel, "")
	upstreamModel := normalizeOpenAIModelForUpstream(account, billingModel)
	result, err := s.forwardDeepSeekRemoteCompactionV2(ctx, c, account, body, originalModel, billingModel, upstreamModel)
	return result, true, err
}

func (s *OpenAIGatewayService) forwardDeepSeekRemoteCompactionV2(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	body []byte,
	originalModel string,
	billingModel string,
	upstreamModel string,
) (*OpenAIForwardResult, error) {
	targetURL, token, err := s.deepSeekResponsesCompactUpstream(account)
	if err != nil {
		return nil, err
	}
	if _, err := deepSeekCompactUserAAD(ctx); err != nil {
		return nil, err
	}
	if _, err := s.deepSeekCompactAEAD(); err != nil {
		return nil, err
	}
	responsesBody, err := deepSeekCompactResponsesRequest(body, upstreamModel)
	if err != nil {
		return nil, err
	}
	SetActualOpenAIUpstreamEndpoint(c, deepSeekResponsesEndpoint)
	startTime := time.Now()
	resp, err := s.sendCCUpstreamRequest(ctx, c, account, targetURL, responsesBody, true, token, "", "")
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= http.StatusBadRequest {
		return s.handleErrorResponse(ctx, resp, c, account, responsesBody, billingModel)
	}

	streamResult, streamErr := s.readDeepSeekCompactResponsesStream(c, resp, startTime)
	if streamErr != nil {
		return nil, streamErr
	}
	checkpoint := frameDeepSeekCompactSummary(streamResult.Summary)
	encryptedContent, err := s.sealDeepSeekCompactCheckpoint(ctx, checkpoint)
	if err != nil {
		return nil, err
	}
	responseID := "resp_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	itemID := "cmp_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	finalJSON, err := deepSeekCompactResponsesJSON(responseID, itemID, originalModel, encryptedContent, streamResult.Usage, streamResult.TotalTokens)
	if err != nil {
		return nil, fmt.Errorf("encode DeepSeek compact response: %w", err)
	}
	s.bindHTTPResponseAccount(ctx, c, account, responseID)

	clientStream := openAICompactClientWantsStream(c)
	if clientStream {
		if !writeOpenAICompactSSEBridge(c, http.StatusOK, finalJSON) {
			return nil, errors.New("failed to synthesize DeepSeek remote compaction response")
		}
	} else {
		c.Header("Content-Type", "application/json")
		c.Data(http.StatusOK, "application/json", finalJSON)
	}

	return &OpenAIForwardResult{
		RequestID:             resp.Header.Get("x-request-id"),
		ResponseID:            responseID,
		Usage:                 streamResult.Usage,
		Model:                 originalModel,
		BillingModel:          billingModel,
		UpstreamModel:         upstreamModel,
		UpstreamResponseModel: observedUpstreamResponseModel(c),
		UpstreamEndpoint:      deepSeekResponsesEndpoint,
		ReasoningEffort:       optionalTrimmedStringPtr(gjson.GetBytes(responsesBody, "reasoning.effort").String()),
		Stream:                clientStream,
		UpstreamTerminalEvent: "response.completed",
		ResponseHeaders:       resp.Header.Clone(),
		Duration:              time.Since(startTime),
		FirstTokenMs:          streamResult.FirstTokenMs,
	}, nil
}
