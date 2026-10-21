package service

import (
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

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/tidwall/gjson"
)

const (
	gatewayCompactEnvelopePrefix   = "sub2api.deepseek.compact.v1."
	gatewayCompactCipherDomain     = "sub2api/deepseek-remote-compaction/v1"
	gatewayCompactMaxSummaryBytes  = 128 * 1024
	gatewayCompactMaxEnvelopeBytes = 256 * 1024
	gatewayCompactEnvelopeVersion  = 1
)

var errGatewayCompactInvalidEncryptedContent = errors.New("invalid compact encrypted_content")

const gatewayCompactCheckpointPreamble = "This is an automatically generated checkpoint condensing an earlier span of the conversation to free up context. Treat the captured context as established background and build on it without restating it. Continue the task directly from the messages that follow, without acknowledging this checkpoint."

const gatewayCompactInstruction = `You are performing a CONTEXT CHECKPOINT COMPACTION. Create a handoff summary for another LLM that will resume the task.

Include:
- Current progress and key decisions made
- Important context, constraints, or user preferences
- What remains to be done (clear next steps)
- Any critical data, examples, or references needed to continue

Be concise, structured, and focused on helping the next LLM seamlessly continue the work.`

type gatewayCompactEnvelope struct {
	Version    int    `json:"version"`
	Checkpoint string `json:"checkpoint"`
}

func frameGatewayCompactSummary(summary string) string {
	return gatewayCompactCheckpointPreamble + "\n\n<compacted-summary>" + summary + "</compacted-summary>"
}

func gatewayCompactUserAAD(ctx context.Context) ([]byte, error) {
	if ctx == nil {
		return nil, errors.New("authenticated user context is required for remote compaction")
	}
	userID, ok := ctx.Value(ctxkey.UserID).(int64)
	if !ok || userID <= 0 {
		return nil, errors.New("authenticated user context is required for remote compaction")
	}
	return []byte(gatewayCompactCipherDomain + ":user:" + strconv.FormatInt(userID, 10)), nil
}

func gatewayCompactAEAD(cfg *config.Config) (cipher.AEAD, error) {
	if cfg == nil || strings.TrimSpace(cfg.JWT.Secret) == "" {
		return nil, errors.New("remote compaction requires a persistent JWT secret")
	}
	key := sha256.Sum256([]byte(gatewayCompactCipherDomain + "\x00" + cfg.JWT.Secret))
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, fmt.Errorf("initialize gateway compact cipher: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("initialize gateway compact AEAD: %w", err)
	}
	return aead, nil
}

func sealGatewayCompactCheckpoint(ctx context.Context, cfg *config.Config, checkpoint string) (string, error) {
	if !utf8.ValidString(checkpoint) || len(checkpoint) > gatewayCompactMaxSummaryBytes {
		return "", errors.New("gateway compact checkpoint exceeds the supported size")
	}
	payload, err := marshalOpenAIUpstreamJSON(gatewayCompactEnvelope{
		Version:    gatewayCompactEnvelopeVersion,
		Checkpoint: checkpoint,
	})
	if err != nil {
		return "", fmt.Errorf("encode gateway compact checkpoint: %w", err)
	}
	aead, err := gatewayCompactAEAD(cfg)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("generate gateway compact nonce: %w", err)
	}
	aad, err := gatewayCompactUserAAD(ctx)
	if err != nil {
		return "", err
	}
	sealed := aead.Seal(nonce, nonce, payload, aad)
	envelope := gatewayCompactEnvelopePrefix + base64.RawURLEncoding.EncodeToString(sealed)
	if len(envelope) > gatewayCompactMaxEnvelopeBytes {
		return "", errors.New("gateway compact encrypted_content exceeds the supported size")
	}
	return envelope, nil
}

func openGatewayCompactCheckpoint(ctx context.Context, cfg *config.Config, envelope string) (string, error) {
	invalid := func() (string, error) { return "", errGatewayCompactInvalidEncryptedContent }
	if !strings.HasPrefix(envelope, gatewayCompactEnvelopePrefix) || len(envelope) > gatewayCompactMaxEnvelopeBytes {
		return invalid()
	}
	encoded := strings.TrimPrefix(envelope, gatewayCompactEnvelopePrefix)
	sealed, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return invalid()
	}
	aead, err := gatewayCompactAEAD(cfg)
	if err != nil || len(sealed) < aead.NonceSize()+aead.Overhead() {
		return invalid()
	}
	aad, err := gatewayCompactUserAAD(ctx)
	if err != nil {
		return invalid()
	}
	payload, err := aead.Open(nil, sealed[:aead.NonceSize()], sealed[aead.NonceSize():], aad)
	if err != nil || len(payload) > gatewayCompactMaxEnvelopeBytes || !utf8.Valid(payload) {
		return invalid()
	}
	var state gatewayCompactEnvelope
	if err := json.Unmarshal(payload, &state); err != nil ||
		state.Version != gatewayCompactEnvelopeVersion ||
		strings.TrimSpace(state.Checkpoint) == "" ||
		len(state.Checkpoint) > gatewayCompactMaxSummaryBytes ||
		!utf8.ValidString(state.Checkpoint) {
		return invalid()
	}
	return state.Checkpoint, nil
}

// RestoreGatewayCompactInput turns gateway-owned compaction items back into
// provider-neutral user checkpoints before policy inspection or upstream conversion.
func (s *GatewayService) RestoreGatewayCompactInput(ctx context.Context, body []byte) ([]byte, bool, error) {
	var cfg *config.Config
	if s != nil {
		cfg = s.cfg
	}
	return restoreGatewayCompactInput(ctx, cfg, body)
}

func restoreGatewayCompactInput(ctx context.Context, cfg *config.Config, body []byte) ([]byte, bool, error) {
	if len(body) == 0 || !gjson.ValidBytes(body) {
		return body, false, nil
	}
	inputResult := gjson.GetBytes(body, "input")
	if !inputResult.IsArray() {
		return body, false, nil
	}
	hasOwnedCompaction := false
	inputResult.ForEach(func(_, item gjson.Result) bool {
		t := item.Get("type").String()
		if (t == "compaction" || t == "compaction_summary") &&
			strings.HasPrefix(item.Get("encrypted_content").String(), gatewayCompactEnvelopePrefix) {
			hasOwnedCompaction = true
			return false
		}
		return true
	})
	if !hasOwnedCompaction {
		return body, false, nil
	}

	var request map[string]json.RawMessage
	if err := json.Unmarshal(body, &request); err != nil {
		return body, false, nil
	}
	inputRaw, ok := request["input"]
	if !ok || len(bytes.TrimSpace(inputRaw)) == 0 || bytes.TrimSpace(inputRaw)[0] != '[' {
		return body, false, nil
	}
	var items []json.RawMessage
	if err := json.Unmarshal(inputRaw, &items); err != nil {
		return body, false, nil
	}

	changed := false
	for i, raw := range items {
		var item map[string]json.RawMessage
		if err := json.Unmarshal(raw, &item); err != nil {
			continue
		}
		var itemType string
		if err := json.Unmarshal(item["type"], &itemType); err != nil ||
			(itemType != "compaction" && itemType != "compaction_summary") {
			continue
		}
		var encryptedContent string
		if err := json.Unmarshal(item["encrypted_content"], &encryptedContent); err != nil {
			continue
		}
		if !strings.HasPrefix(encryptedContent, gatewayCompactEnvelopePrefix) {
			continue
		}
		checkpoint, err := openGatewayCompactCheckpoint(ctx, cfg, encryptedContent)
		if err != nil {
			return nil, false, err
		}
		restored, err := marshalOpenAIUpstreamJSON(map[string]any{
			"type": "message",
			"role": "user",
			"content": []any{map[string]any{
				"type": "input_text",
				"text": checkpoint,
			}},
		})
		if err != nil {
			return nil, false, err
		}
		items[i] = restored
		changed = true
	}
	if !changed {
		return body, false, nil
	}
	restoredInput, err := marshalOpenAIUpstreamJSON(items)
	if err != nil {
		return nil, false, err
	}
	request["input"] = restoredInput
	restoredBody, err := marshalOpenAIUpstreamJSON(request)
	if err != nil {
		return nil, false, err
	}
	return restoredBody, true, nil
}

func isGatewayResponsesCompactionRequest(c *gin.Context, body []byte) bool {
	return isOpenAIResponsesCompactPath(c) || HasCompactionTriggerInInput(body)
}

func isGatewayNativeCompactionV2Request(c *gin.Context, body []byte, clientStream bool) bool {
	return !isOpenAIResponsesCompactPath(c) && clientStream && HasCompactionTriggerInInput(body)
}

func rewriteGatewayResponsesCompactionRequest(body []byte) ([]byte, error) {
	var request map[string]json.RawMessage
	if err := json.Unmarshal(body, &request); err != nil {
		return nil, fmt.Errorf("parse compact request: %w", err)
	}
	if previousRaw, ok := request["previous_response_id"]; ok {
		var previous *string
		if err := json.Unmarshal(previousRaw, &previous); err != nil {
			return nil, errors.New("remote compaction previous_response_id must be a string or null")
		}
		if previous != nil && strings.TrimSpace(*previous) != "" {
			return nil, errors.New("remote compaction does not support previous_response_id")
		}
	}
	inputRaw, ok := request["input"]
	if !ok {
		return nil, errors.New("remote compaction requires input")
	}

	var items []json.RawMessage
	trimmedInput := bytes.TrimSpace(inputRaw)
	if len(trimmedInput) > 0 && trimmedInput[0] == '"' {
		var text string
		if err := json.Unmarshal(trimmedInput, &text); err != nil || strings.TrimSpace(text) == "" {
			return nil, errors.New("remote compaction input string must not be empty")
		}
		msg, err := marshalOpenAIUpstreamJSON(map[string]any{
			"type": "message",
			"role": "user",
			"content": []any{map[string]any{
				"type": "input_text",
				"text": text,
			}},
		})
		if err != nil {
			return nil, err
		}
		items = []json.RawMessage{msg}
	} else if err := json.Unmarshal(inputRaw, &items); err != nil {
		return nil, errors.New("remote compaction input must be a string or array")
	}

	triggerCount := 0
	history := make([]json.RawMessage, 0, len(items))
	for _, raw := range items {
		if strings.TrimSpace(gjson.GetBytes(raw, "type").String()) == "compaction_trigger" {
			triggerCount++
			continue
		}
		history = append(history, raw)
	}
	if triggerCount > 1 {
		return nil, errors.New("remote compaction requires at most one compaction_trigger")
	}
	if len(history) == 0 {
		return nil, errors.New("remote compaction has no conversation history to summarize")
	}

	promptItem, err := marshalOpenAIUpstreamJSON(map[string]any{
		"type": "message",
		"role": "user",
		"content": []any{map[string]any{
			"type": "input_text",
			"text": gatewayCompactInstruction,
		}},
	})
	if err != nil {
		return nil, fmt.Errorf("encode compact checkpoint prompt: %w", err)
	}
	upstreamInput, err := marshalOpenAIUpstreamJSON(append(history, promptItem))
	if err != nil {
		return nil, fmt.Errorf("encode compact input: %w", err)
	}

	request["input"] = upstreamInput
	delete(request, "tools")
	delete(request, "tool_choice")

	return marshalOpenAIUpstreamJSON(request)
}

func extractAnthropicResponseSummaryText(resp *apicompat.AnthropicResponse) string {
	if resp == nil {
		return ""
	}
	var sb strings.Builder
	for _, block := range resp.Content {
		if block.Type == "text" && block.Text != "" {
			if sb.Len() > 0 {
				sb.WriteString("\n")
			}
			sb.WriteString(block.Text)
		}
	}
	return strings.TrimSpace(sb.String())
}

func buildGatewayCompactResponsesJSON(responseID, itemID, model, encryptedContent string, usage ClaudeUsage) ([]byte, error) {
	totalTokens := usage.InputTokens + usage.OutputTokens + usage.CacheReadInputTokens + usage.CacheCreationInputTokens
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
			"output_tokens_details": map[string]any{
				"reasoning_tokens": 0,
			},
			"total_tokens": totalTokens,
		},
	})
}

func writeGatewayCompactResponse(
	ctx context.Context,
	c *gin.Context,
	cfg *config.Config,
	originalModel string,
	summary string,
	usage ClaudeUsage,
	clientStream bool,
) error {
	checkpoint := frameGatewayCompactSummary(summary)
	encryptedContent, err := sealGatewayCompactCheckpoint(ctx, cfg, checkpoint)
	if err != nil {
		return err
	}
	responseID := "resp_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	itemID := "cmp_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	finalJSON, err := buildGatewayCompactResponsesJSON(responseID, itemID, originalModel, encryptedContent, usage)
	if err != nil {
		return err
	}

	MarkResponseCommitted(c)
	if clientStream {
		payload, ok := buildOpenAICompactSSEPayload(finalJSON)
		if !ok {
			return errors.New("failed to build compact SSE payload")
		}
		header := c.Writer.Header()
		header.Set("Content-Type", "text/event-stream")
		header.Set("Cache-Control", "no-cache")
		header.Set("Connection", "keep-alive")
		header.Set("X-Accel-Buffering", "no")
		c.Writer.WriteHeader(http.StatusOK)
		_, _ = c.Writer.Write(payload)
		c.Writer.Flush()
		return nil
	}
	c.Writer.Header().Set("Content-Type", "application/json; charset=utf-8")
	c.Data(http.StatusOK, "application/json; charset=utf-8", finalJSON)
	return nil
}
