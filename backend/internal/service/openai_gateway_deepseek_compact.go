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
	deepSeekCompactionModeKey     = "deepseek_compaction_bridge_mode"
	deepSeekCompactPreparedKey    = "deepseek_remote_compaction_prepared"
	deepSeekResponsesValidatedKey = "deepseek_responses_input_validated"

	deepSeekCompactEnvelopePrefix      = "sub2api.deepseek.compact.v1."
	deepSeekCompactCipherDomain        = "sub2api/deepseek-remote-compaction/v1"
	deepSeekCompactMaxSummaryBytes     = 128 * 1024
	deepSeekCompactMaxEnvelopeBytes    = 256 * 1024
	deepSeekCompactMaxSSEBytes         = 8 * 1024 * 1024
	deepSeekCompactMaxSSELineBytes     = 1024 * 1024
	deepSeekCompactSummaryMaxTokens    = 8192
	deepSeekCompactEnvelopeVersion     = 1
	deepSeekResponsesMaxJSONDepth      = 256
	deepSeekCompactInvalidStateMessage = "invalid DeepSeek compact encrypted_content"
)

type DeepSeekCompactionMode string

const (
	DeepSeekCompactionModeNone          DeepSeekCompactionMode = ""
	DeepSeekCompactionModeRemoteV2SSE   DeepSeekCompactionMode = "remote_v2_sse"
	DeepSeekCompactionModeLegacyBodySSE DeepSeekCompactionMode = "legacy_body_sse"
	DeepSeekCompactionModeLegacyUnary   DeepSeekCompactionMode = "legacy_unary"
)

var (
	ErrDeepSeekCompactInvalidEncryptedContent = errors.New(deepSeekCompactInvalidStateMessage)
	ErrDeepSeekResponsesDuplicateJSONKey      = errors.New("DeepSeek Responses request contains duplicate JSON object keys")
	ErrDeepSeekResponsesNonCanonicalJSONKey   = errors.New("DeepSeek Responses request contains non-canonical JSON object keys")
	ErrDeepSeekCompactRequestTooLarge         = errors.New("DeepSeek Responses request exceeds the supported size")
)

const deepSeekCompactCheckpointPreamble = "This is an automatically generated checkpoint condensing an earlier span of the conversation to free up context. Treat the captured context as established background and build on it without restating it. Continue the task directly from the messages that follow, without acknowledging this checkpoint."

const deepSeekCompactInstruction = `You are performing a CONTEXT CHECKPOINT COMPACTION. Create a handoff summary for another LLM that will resume the task.

Include:
- Current progress and key decisions made
- Important context, constraints, or user preferences
- What remains to be done (clear next steps)
- Any critical data, examples, or references needed to continue

Be concise, structured, and focused on helping the next LLM seamlessly continue the work.`

type deepSeekCompactEnvelope struct {
	Version    int    `json:"version"`
	Checkpoint string `json:"checkpoint"`
}

type deepSeekCompactStreamResult struct {
	Summary        string
	Usage          OpenAIUsage
	TotalTokens    int
	ResponseID     string
	FirstTokenMs   *int
	Completed      bool
	UpstreamFailed bool
}

type deepSeekCompactPreparedRequest struct {
	SourceHash    [32]byte
	ResponsesBody []byte
}

// MarkDeepSeekCompaction records a server-validated bridge decision. The mode
// also preserves the client wire: remote/body streaming clients receive SSE,
// while legacy /responses/compact clients receive unary Responses JSON.
func MarkDeepSeekCompaction(c *gin.Context, mode DeepSeekCompactionMode) {
	if c == nil {
		return
	}
	switch mode {
	case DeepSeekCompactionModeRemoteV2SSE, DeepSeekCompactionModeLegacyBodySSE:
		c.Set(deepSeekCompactionModeKey, mode)
		MarkOpenAICompactClientStream(c)
	case DeepSeekCompactionModeLegacyUnary:
		c.Set(deepSeekCompactionModeKey, mode)
	}
}

// ClearDeepSeekCompaction resets request-scoped state before reusing a Gin
// context for the next turn of a Responses WebSocket connection.
func ClearDeepSeekCompaction(c *gin.Context) {
	if c == nil {
		return
	}
	c.Set(deepSeekCompactionModeKey, DeepSeekCompactionModeNone)
	c.Set(deepSeekCompactPreparedKey, nil)
	c.Set(deepSeekResponsesValidatedKey, false)
	clearOpenAICompactClientStream(c)
}

// MarkDeepSeekRemoteCompactionV2 keeps the existing call site contract for the
// current Codex remote_compaction_v2 streaming wire.
func MarkDeepSeekRemoteCompactionV2(c *gin.Context) {
	MarkDeepSeekCompaction(c, DeepSeekCompactionModeRemoteV2SSE)
}

func DeepSeekCompactionModeFromContext(c *gin.Context) DeepSeekCompactionMode {
	if c == nil {
		return DeepSeekCompactionModeNone
	}
	value, _ := c.Get(deepSeekCompactionModeKey)
	mode, _ := value.(DeepSeekCompactionMode)
	return mode
}

func IsDeepSeekCompactionMarked(c *gin.Context) bool {
	return DeepSeekCompactionModeFromContext(c) != DeepSeekCompactionModeNone
}

func IsDeepSeekRemoteCompactionV2Marked(c *gin.Context) bool {
	return IsDeepSeekCompactionMarked(c)
}

// MarkDeepSeekResponsesInputValidated avoids repeating the strict input scan
// after the handler has restored gateway-owned state before policy inspection.
func MarkDeepSeekResponsesInputValidated(c *gin.Context) {
	if c != nil {
		c.Set(deepSeekResponsesValidatedKey, true)
	}
}

func IsDeepSeekResponsesInputValidated(c *gin.Context) bool {
	if c == nil {
		return false
	}
	value, _ := c.Get(deepSeekResponsesValidatedKey)
	validated, _ := value.(bool)
	return validated
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

type deepSeekResponsesJSONScanMode uint8

const (
	deepSeekResponsesJSONScanGeneric deepSeekResponsesJSONScanMode = iota
	deepSeekResponsesJSONScanRoot
	deepSeekResponsesJSONScanInput
	deepSeekResponsesJSONScanInputItem
	deepSeekResponsesJSONScanContent
	deepSeekResponsesJSONScanContentItem
)

type deepSeekResponsesJSONScan struct {
	hasCompactState bool
	strict          bool
}

var deepSeekResponsesRootJSONKeys = [...]string{
	"model", "instructions", "input", "max_output_tokens", "temperature", "top_p", "stream", "tools",
	"include", "store", "parallel_tool_calls", "reasoning", "text", "tool_choice", "service_tier",
	"prompt_cache_key", "previous_response_id", "user",
}

var deepSeekResponsesInputItemJSONKeys = [...]string{
	"type", "role", "content", "encrypted_content", "call_id", "name", "arguments", "id", "output",
	"namespace", "summary", "text", "input", "tools",
}

var deepSeekResponsesContentItemJSONKeys = [...]string{"type", "text", "image_url"}

func deepSeekResponsesCanonicalJSONKey(key string, candidates []string) (string, uint64, bool) {
	for index, candidate := range candidates {
		if strings.EqualFold(key, candidate) {
			return candidate, uint64(1) << index, true
		}
	}
	return "", 0, false
}

func scanDeepSeekResponsesJSON(body []byte) (bool, error) {
	return scanDeepSeekResponsesJSONWithMode(body, true)
}

// probeDeepSeekResponsesCompactionState performs a streaming, fixed-state
// probe. Provider-native fields remain opaque unless an input item identifies
// itself as compaction state, at which point the caller runs the strict scan.
// Root model, input, and stream stay strict even for ordinary requests because
// routing, policy inspection, and wire selection consume them before the
// original JSON reaches upstream.
func probeDeepSeekResponsesCompactionState(body []byte) (bool, error) {
	return scanDeepSeekResponsesJSONWithMode(body, false)
}

func scanDeepSeekResponsesJSONWithMode(body []byte, strict bool) (bool, error) {
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	scan := &deepSeekResponsesJSONScan{strict: strict}
	if err := consumeDeepSeekResponsesJSONValue(decoder, deepSeekResponsesJSONScanRoot, 0, scan); err != nil {
		return false, err
	}
	if _, err := decoder.Token(); err != io.EOF {
		if err == nil {
			return false, errors.New("DeepSeek Responses request contains trailing JSON data")
		}
		return false, err
	}
	return scan.hasCompactState, nil
}

func consumeDeepSeekResponsesJSONValue(
	decoder *json.Decoder,
	mode deepSeekResponsesJSONScanMode,
	depth int,
	scan *deepSeekResponsesJSONScan,
) error {
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	return consumeDeepSeekResponsesJSONToken(decoder, token, mode, depth, scan)
}

func consumeDeepSeekResponsesJSONToken(
	decoder *json.Decoder,
	token json.Token,
	mode deepSeekResponsesJSONScanMode,
	depth int,
	scan *deepSeekResponsesJSONScan,
) error {
	delim, ok := token.(json.Delim)
	if !ok {
		return nil
	}
	if depth >= deepSeekResponsesMaxJSONDepth {
		return errors.New("DeepSeek Responses request exceeds the supported JSON nesting depth")
	}
	switch delim {
	case '{':
		switch mode {
		case deepSeekResponsesJSONScanInput:
			mode = deepSeekResponsesJSONScanInputItem
		case deepSeekResponsesJSONScanContent:
			mode = deepSeekResponsesJSONScanContentItem
		}
		var seen uint64
		for decoder.More() {
			keyToken, err := decoder.Token()
			if err != nil {
				return err
			}
			key, ok := keyToken.(string)
			if !ok {
				return errors.New("DeepSeek Responses request contains a non-string object key")
			}
			canonical := ""
			bit := uint64(0)
			recognized := false
			switch mode {
			case deepSeekResponsesJSONScanRoot:
				canonical, bit, recognized = deepSeekResponsesCanonicalJSONKey(key, deepSeekResponsesRootJSONKeys[:])
			case deepSeekResponsesJSONScanInputItem:
				canonical, bit, recognized = deepSeekResponsesCanonicalJSONKey(key, deepSeekResponsesInputItemJSONKeys[:])
			case deepSeekResponsesJSONScanContentItem:
				canonical, bit, recognized = deepSeekResponsesCanonicalJSONKey(key, deepSeekResponsesContentItemJSONKeys[:])
			}
			strictKey := scan.strict || (mode == deepSeekResponsesJSONScanRoot &&
				(canonical == "model" || canonical == "input" || canonical == "stream"))
			if recognized && strictKey {
				if key != canonical {
					return ErrDeepSeekResponsesNonCanonicalJSONKey
				}
				if seen&bit != 0 {
					return ErrDeepSeekResponsesDuplicateJSONKey
				}
				seen |= bit
			}

			valueMode := deepSeekResponsesJSONScanGeneric
			if mode == deepSeekResponsesJSONScanRoot && canonical == "input" {
				valueMode = deepSeekResponsesJSONScanInput
			} else if mode == deepSeekResponsesJSONScanInputItem &&
				(canonical == "content" || canonical == "summary" || canonical == "output") {
				valueMode = deepSeekResponsesJSONScanContent
			}
			if mode == deepSeekResponsesJSONScanInputItem && canonical == "type" {
				valueToken, err := decoder.Token()
				if err != nil {
					return err
				}
				if itemType, ok := valueToken.(string); ok &&
					(itemType == "compaction" || itemType == "compaction_summary") {
					scan.hasCompactState = true
				}
				if err := consumeDeepSeekResponsesJSONToken(decoder, valueToken, valueMode, depth+1, scan); err != nil {
					return err
				}
				continue
			}
			if err := consumeDeepSeekResponsesJSONValue(decoder, valueMode, depth+1, scan); err != nil {
				return err
			}
		}
		closing, err := decoder.Token()
		if err != nil {
			return err
		}
		if closing != json.Delim('}') {
			return errors.New("DeepSeek Responses request has an invalid object boundary")
		}
	case '[':
		childMode := deepSeekResponsesJSONScanGeneric
		switch mode {
		case deepSeekResponsesJSONScanInput:
			childMode = deepSeekResponsesJSONScanInputItem
		case deepSeekResponsesJSONScanContent:
			childMode = deepSeekResponsesJSONScanContentItem
		}
		for decoder.More() {
			if err := consumeDeepSeekResponsesJSONValue(decoder, childMode, depth+1, scan); err != nil {
				return err
			}
		}
		closing, err := decoder.Token()
		if err != nil {
			return err
		}
		if closing != json.Delim(']') {
			return errors.New("DeepSeek Responses request has an invalid array boundary")
		}
	default:
		return errors.New("DeepSeek Responses request has an invalid JSON boundary")
	}
	return nil
}

// RestoreDeepSeekCompactInput keeps the dedicated DeepSeek fail-closed
// contract for callers that do not already know the effective target.
func (s *OpenAIGatewayService) RestoreDeepSeekCompactInput(ctx context.Context, body []byte) ([]byte, bool, error) {
	return s.RestoreDeepSeekCompactInputForTarget(ctx, body, PlatformDeepSeek)
}

// RestoreDeepSeekCompactInputForTarget turns gateway-owned compaction items
// back into provider-neutral user checkpoints before provider dispatch. A
// DeepSeek target rejects foreign opaque state; other targets preserve it.
func (s *OpenAIGatewayService) RestoreDeepSeekCompactInputForTarget(ctx context.Context, body []byte, targetPlatform string) ([]byte, bool, error) {
	rejectForeign := targetPlatform == PlatformDeepSeek
	if s.deepSeekCompactRequestTooLarge(body) {
		return nil, false, ErrDeepSeekCompactRequestTooLarge
	}
	if len(body) == 0 || !gjson.ValidBytes(body) {
		return body, false, nil
	}
	hasCompactState, err := probeDeepSeekResponsesCompactionState(body)
	if err != nil {
		// DeepSeek fail-closed rejects ambiguous root keys. Other targets leave
		// duplicate/non-canonical model keys to allowlist / upstream JSON
		// semantics so identical duplicates stay legal and conflicting ones
		// surface as model-not-allowed rather than compact errors.
		if !rejectForeign &&
			(errors.Is(err, ErrDeepSeekResponsesDuplicateJSONKey) ||
				errors.Is(err, ErrDeepSeekResponsesNonCanonicalJSONKey)) {
			return body, false, nil
		}
		return nil, false, err
	}
	if !hasCompactState {
		return body, false, nil
	}
	if _, err := scanDeepSeekResponsesJSON(body); err != nil {
		return nil, false, err
	}
	var request map[string]json.RawMessage
	if err := json.Unmarshal(body, &request); err != nil {
		return body, false, nil
	}
	inputRaw, ok := request["input"]
	if !ok || len(bytes.TrimSpace(inputRaw)) == 0 || bytes.TrimSpace(inputRaw)[0] != '[' {
		return nil, false, ErrDeepSeekCompactInvalidEncryptedContent
	}
	var items []json.RawMessage
	if err := json.Unmarshal(inputRaw, &items); err != nil {
		return nil, false, ErrDeepSeekCompactInvalidEncryptedContent
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
		if err := json.Unmarshal(item["encrypted_content"], &encryptedContent); err != nil || strings.TrimSpace(encryptedContent) == "" {
			if rejectForeign {
				return nil, false, ErrDeepSeekCompactInvalidEncryptedContent
			}
			continue
		}
		if !strings.HasPrefix(encryptedContent, deepSeekCompactEnvelopePrefix) {
			if rejectForeign {
				return nil, false, ErrDeepSeekCompactInvalidEncryptedContent
			}
			continue
		}
		checkpoint, err := s.openDeepSeekCompactCheckpoint(ctx, encryptedContent)
		if err != nil {
			return nil, false, ErrDeepSeekCompactInvalidEncryptedContent
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
			return nil, false, ErrDeepSeekCompactInvalidEncryptedContent
		}
		items[i] = restored
		changed = true
	}
	if !changed {
		return body, false, nil
	}
	restoredInput, err := marshalOpenAIUpstreamJSON(items)
	if err != nil {
		return nil, false, ErrDeepSeekCompactInvalidEncryptedContent
	}
	request["input"] = restoredInput
	restoredBody, err := marshalOpenAIUpstreamJSON(request)
	if err != nil {
		return nil, false, ErrDeepSeekCompactInvalidEncryptedContent
	}
	return restoredBody, true, nil
}

// NormalizeDeepSeekLegacyCompactRequest converts the standalone
// /responses/compact request shape into the internal body-signal shape used by
// the native Responses bridge. Callers must restore gateway-owned checkpoints
// first so policy inspection and this normalization operate on plaintext.
func (s *OpenAIGatewayService) NormalizeDeepSeekLegacyCompactRequest(c *gin.Context, body []byte) ([]byte, error) {
	if s.deepSeekCompactRequestTooLarge(body) {
		return nil, ErrDeepSeekCompactRequestTooLarge
	}
	if !IsDeepSeekResponsesInputValidated(c) {
		if _, err := scanDeepSeekResponsesJSON(body); err != nil {
			return nil, err
		}
	}

	normalized, _, err := normalizeOpenAICompactRequestBody(body)
	if err != nil {
		return nil, fmt.Errorf("normalize DeepSeek legacy compact request: %w", err)
	}
	var request map[string]json.RawMessage
	if err := json.Unmarshal(normalized, &request); err != nil {
		return nil, fmt.Errorf("parse DeepSeek legacy compact request: %w", err)
	}
	inputRaw, ok := request["input"]
	if !ok {
		return nil, errors.New("DeepSeek legacy compact request requires input")
	}
	var items []json.RawMessage
	trimmedInput := bytes.TrimSpace(inputRaw)
	if len(trimmedInput) > 0 && trimmedInput[0] == '"' {
		var text string
		if err := json.Unmarshal(trimmedInput, &text); err != nil || strings.TrimSpace(text) == "" {
			return nil, errors.New("DeepSeek legacy compact input string must not be empty")
		}
		message, err := marshalOpenAIUpstreamJSON(map[string]any{
			"type": "message",
			"role": "user",
			"content": []any{map[string]any{
				"type": "input_text",
				"text": text,
			}},
		})
		if err != nil {
			return nil, fmt.Errorf("encode DeepSeek legacy compact string input: %w", err)
		}
		items = []json.RawMessage{message}
	} else if err := json.Unmarshal(inputRaw, &items); err != nil {
		return nil, errors.New("DeepSeek legacy compact input must be a string or array")
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
	if triggerIndex >= 0 && triggerIndex != len(items)-1 {
		return nil, errors.New("DeepSeek remote compaction requires a final compaction_trigger")
	}
	if triggerIndex < 0 {
		trigger, _ := marshalOpenAIUpstreamJSON(map[string]any{"type": "compaction_trigger"})
		items = append(items, trigger)
	}
	encodedInput, err := marshalOpenAIUpstreamJSON(items)
	if err != nil {
		return nil, fmt.Errorf("encode DeepSeek legacy compact input: %w", err)
	}
	request["input"] = encodedInput
	encoded, err := marshalOpenAIUpstreamJSON(request)
	if err != nil {
		return nil, fmt.Errorf("encode DeepSeek legacy compact request: %w", err)
	}
	if s.deepSeekCompactRequestTooLarge(encoded) {
		return nil, ErrDeepSeekCompactRequestTooLarge
	}
	return encoded, nil
}

// PrepareDeepSeekRemoteCompactionRequest performs account-independent
// conversion after the user concurrency and billing gates but before account
// scheduling. Forward reuses the cached body and only applies account model
// mapping, avoiding a second conversion of a potentially large history.
func (s *OpenAIGatewayService) PrepareDeepSeekRemoteCompactionRequest(c *gin.Context, body []byte) error {
	if s.deepSeekCompactRequestTooLarge(body) {
		return ErrDeepSeekCompactRequestTooLarge
	}
	if _, err := scanDeepSeekResponsesJSON(body); err != nil {
		return err
	}
	model := strings.TrimSpace(gjson.GetBytes(body, "model").String())
	if model == "" {
		return errors.New("DeepSeek remote compaction requires a model")
	}
	responsesBody, err := deepSeekCompactResponsesRequest(body, model)
	if err != nil {
		return err
	}
	if c != nil {
		c.Set(deepSeekCompactPreparedKey, deepSeekCompactPreparedRequest{
			SourceHash:    sha256.Sum256(body),
			ResponsesBody: responsesBody,
		})
	}
	return nil
}

func (s *OpenAIGatewayService) deepSeekCompactRequestTooLarge(body []byte) bool {
	return s != nil && s.cfg != nil && s.cfg.Gateway.MaxBodySize > 0 && int64(len(body)) > s.cfg.Gateway.MaxBodySize
}

func preparedDeepSeekCompactResponsesRequest(c *gin.Context, sourceBody []byte, upstreamModel string) ([]byte, bool) {
	if c == nil {
		return nil, false
	}
	value, ok := c.Get(deepSeekCompactPreparedKey)
	if !ok {
		return nil, false
	}
	prepared, ok := value.(deepSeekCompactPreparedRequest)
	if !ok || prepared.SourceHash != sha256.Sum256(sourceBody) || len(prepared.ResponsesBody) == 0 {
		return nil, false
	}
	responsesBody := prepared.ResponsesBody
	if currentModel := strings.TrimSpace(gjson.GetBytes(responsesBody, "model").String()); currentModel != upstreamModel {
		responsesBody = ReplaceModelInBody(responsesBody, upstreamModel)
	}
	return responsesBody, true
}

func deepSeekCompactContentContainsImage(raw json.RawMessage) bool {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) || trimmed[0] == '"' {
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
		default:
			if len(pending) > 0 {
				return errors.New("DeepSeek remote compaction contains an interrupted tool call/result sequence")
			}
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

func (s *OpenAIGatewayService) readDeepSeekCompactResponsesStream(c *gin.Context, resp *http.Response, account *Account, startTime time.Time) (deepSeekCompactStreamResult, error) {
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
	var cyberMark *CyberPolicyMark
	defer func() {
		if cyberMark != nil {
			cyberMark.UpstreamInTok = result.Usage.InputTokens
			cyberMark.UpstreamOutTok = result.Usage.OutputTokens
			MarkOpsCyberPolicy(c, *cyberMark)
		}
	}()

	setProtocolErr := func(err error) {
		if protocolErr == nil {
			protocolErr = err
		}
		result.UpstreamFailed = true
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
		payloadBytes := redactDeepSeekAPIKey(account, []byte(payload))
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
		if usage, ok := extractOpenAIUsageFromJSONBytes(payloadBytes); ok && hasBillableOpenAIUsage(usage) {
			result.Usage = usage
			result.TotalTokens = 0
			totalTokens := int(gjson.GetBytes(payloadBytes, "response.usage.total_tokens").Int())
			if totalTokens <= 0 {
				totalTokens = int(gjson.GetBytes(payloadBytes, "usage.total_tokens").Int())
			}
			if totalTokens > 0 {
				result.TotalTokens = totalTokens
			}
		}
		if hit, code, message := detectOpenAICyberPolicy(payloadBytes); hit {
			cyberMark = &CyberPolicyMark{Code: code, Message: message, Body: truncateString(string(payloadBytes), 4096), UpstreamStatus: http.StatusOK}
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
	streamInterval := time.Duration(0)
	if s.cfg != nil && s.cfg.Gateway.StreamDataIntervalTimeout > 0 {
		streamInterval = time.Duration(s.cfg.Gateway.StreamDataIntervalTimeout) * time.Second
	}
	var scanStopErr error
	scanErr := scanDeepSeekSSELinesWithIdleTimeout(scanner, resp.Body, streamInterval, func(rawLine string) bool {
		line := strings.TrimSuffix(rawLine, "\r")
		rawBytes += len(line) + 1
		if rawBytes > deepSeekCompactMaxSSEBytes {
			scanStopErr = errors.New("DeepSeek compact stream exceeds the supported size")
			return false
		}
		if line == "" {
			processEvent()
			return true
		}
		if strings.HasPrefix(line, "event:") {
			currentEventType = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
			return true
		}
		if strings.HasPrefix(line, "data:") {
			data := strings.TrimPrefix(strings.TrimPrefix(line, "data:"), " ")
			dataLines = append(dataLines, data)
		}
		return true
	})
	if scanErr != nil {
		result.UpstreamFailed = true
		if errors.Is(scanErr, errDeepSeekSSEDataIntervalTimeout) {
			return result, fmt.Errorf("DeepSeek compact stream data interval timeout: %w", scanErr)
		}
		return result, fmt.Errorf("read DeepSeek compact stream: %w", scanErr)
	}
	if scanStopErr != nil {
		result.UpstreamFailed = true
		return result, scanStopErr
	}
	if len(dataLines) > 0 {
		setProtocolErr(errors.New("DeepSeek compact stream ended before a blank-line-dispatched terminal event"))
	}
	if terminalCount != 1 {
		setProtocolErr(fmt.Errorf("DeepSeek compact stream requires exactly one terminal event, got %d", terminalCount))
	}
	result.Completed = terminalCount == 1 && terminalType == "response.completed"
	if !hasBillableOpenAIUsage(result.Usage) {
		if protocolErr != nil {
			return result, fmt.Errorf("%w: %s", protocolErr, deepSeekMissingUsageMsg)
		}
		return result, errors.New(deepSeekMissingUsageMsg)
	}
	if result.TotalTokens <= 0 {
		result.TotalTokens = result.Usage.InputTokens + result.Usage.OutputTokens
	}
	if protocolErr != nil {
		return result, protocolErr
	}
	result.Summary = string(redactDeepSeekAPIKey(account, []byte(result.Summary)))
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
			"output_tokens_details": map[string]any{
				"reasoning_tokens": usage.ReasoningTokens,
			},
			"total_tokens": totalTokens,
		},
	})
}

func (s *OpenAIGatewayService) handleCommittedDeepSeekCompactHTTPError(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	resp *http.Response,
	body []byte,
	upstreamModel string,
) (*OpenAIForwardResult, error) {
	body = redactDeepSeekAPIKey(account, body)
	message := redactDeepSeekAPIKeyString(account, strings.TrimSpace(extractUpstreamErrorMessage(body)))
	detail := ""
	if s.cfg != nil && s.cfg.Gateway.LogUpstreamErrorBody {
		maxBytes := s.cfg.Gateway.LogUpstreamErrorBodyMaxBytes
		if maxBytes <= 0 {
			maxBytes = 2048
		}
		detail = truncateString(string(body), maxBytes)
	}
	setOpsUpstreamError(c, resp.StatusCode, message, detail)
	if hit, code, cyberMessage := detectOpenAICyberPolicy(body); hit {
		MarkOpsCyberPolicy(c, CyberPolicyMark{
			Code: code, Message: cyberMessage, Body: truncateString(string(body), 4096), UpstreamStatus: resp.StatusCode,
		})
	} else {
		s.handleOpenAIAccountUpstreamError(ctx, account, resp.StatusCode, resp.Header, body, upstreamModel)
	}
	appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
		Platform:           account.Platform,
		AccountID:          account.ID,
		AccountName:        account.Name,
		UpstreamStatusCode: resp.StatusCode,
		UpstreamRequestID:  openAICompatibleUpstreamRequestID(resp.Header),
		Kind:               "http_error",
		Message:            message,
		Detail:             detail,
	})
	writeOpenAICompactSSEFailure(c, resp.StatusCode, body)
	return nil, fmt.Errorf("non-streaming openai protocol error: DeepSeek compact upstream HTTP %d", resp.StatusCode)
}

type deepSeekCompactExecution struct {
	Result      *OpenAIForwardResult
	FinalJSON   []byte
	Headers     http.Header
	RequestBody []byte
}

type deepSeekCompactUpstreamHTTPError struct {
	StatusCode  int
	Headers     http.Header
	Body        []byte
	Message     string
	RequestBody []byte
}

func (e *deepSeekCompactUpstreamHTTPError) Error() string {
	if e == nil {
		return ""
	}
	return fmt.Sprintf("DeepSeek compact upstream HTTP %d: %s", e.StatusCode, strings.TrimSpace(e.Message))
}

type deepSeekCompactUpstreamSender func(responsesBody []byte) (*http.Response, error)

// executeDeepSeekRemoteCompaction is the transport-neutral compaction core used
// by unary HTTP, HTTP SSE and Responses WebSocket clients. Callers provide the
// cancellable upstream sender and own only their final downstream wire.
func (s *OpenAIGatewayService) executeDeepSeekRemoteCompaction(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	body []byte,
	originalModel string,
	billingModel string,
	upstreamModel string,
	clientStream bool,
	send deepSeekCompactUpstreamSender,
) (deepSeekCompactExecution, error) {
	var execution deepSeekCompactExecution
	startTime := time.Now()
	if s.deepSeekCompactRequestTooLarge(body) {
		return execution, ErrDeepSeekCompactRequestTooLarge
	}
	if _, err := deepSeekCompactUserAAD(ctx); err != nil {
		return execution, err
	}
	if _, err := s.deepSeekCompactAEAD(); err != nil {
		return execution, err
	}
	responsesBody, prepared := preparedDeepSeekCompactResponsesRequest(c, body, upstreamModel)
	if !prepared {
		var err error
		responsesBody, err = deepSeekCompactResponsesRequest(body, upstreamModel)
		if err != nil {
			return execution, err
		}
	}
	execution.RequestBody = responsesBody
	if send == nil {
		return execution, errors.New("DeepSeek compact upstream sender is nil")
	}
	SetActualOpenAIUpstreamEndpoint(c, deepSeekResponsesEndpoint)
	upstreamStart := time.Now()
	resp, err := send(responsesBody)
	SetOpsLatencyMs(c, OpsUpstreamLatencyMsKey, time.Since(upstreamStart).Milliseconds())
	if err != nil {
		return execution, err
	}
	defer func() { _ = resp.Body.Close() }()
	sanitizeDeepSeekResponseHeadersInPlace(account, resp.Header)
	if resp.StatusCode >= http.StatusBadRequest {
		respBody, upstreamMsg := s.readOpenAIUpstreamError(resp)
		if hit, _, _ := detectOpenAICyberPolicy(respBody); !hit {
			if failoverErr := s.failoverOpenAIUpstreamHTTPError(ctx, c, account, resp, respBody, upstreamMsg, upstreamModel); failoverErr != nil {
				return execution, failoverErr
			}
		}
		return execution, &deepSeekCompactUpstreamHTTPError{
			StatusCode:  resp.StatusCode,
			Headers:     resp.Header.Clone(),
			Body:        redactDeepSeekAPIKey(account, respBody),
			Message:     redactDeepSeekAPIKeyString(account, upstreamMsg),
			RequestBody: responsesBody,
		}
	}

	streamResult, streamErr := s.readDeepSeekCompactResponsesStream(c, resp, account, startTime)
	requestID := openAICompatibleUpstreamRequestID(resp.Header)
	terminalEvent := ""
	if streamResult.Completed && !streamResult.UpstreamFailed {
		terminalEvent = "response.completed"
	}
	result := &OpenAIForwardResult{
		RequestID:                     requestID,
		ResponseID:                    streamResult.ResponseID,
		Usage:                         streamResult.Usage,
		Model:                         originalModel,
		BillingModel:                  billingModel,
		UpstreamModel:                 upstreamModel,
		UpstreamResponseModel:         observedUpstreamResponseModel(c),
		UpstreamResponseModelConflict: observedUpstreamResponseModelConflict(c),
		UpstreamEndpoint:              deepSeekResponsesEndpoint,
		ReasoningEffort:               optionalTrimmedStringPtr(gjson.GetBytes(responsesBody, "reasoning.effort").String()),
		RequestKind:                   UsageRequestKindCompact,
		UpstreamTerminalEvent:         terminalEvent,
		Stream:                        clientStream,
		OpenAIWSMode:                  false,
		Duration:                      time.Since(startTime),
		FirstTokenMs:                  streamResult.FirstTokenMs,
		ResponseHeaders:               resp.Header.Clone(),
	}
	execution.Result = result
	execution.Headers = resp.Header.Clone()
	if ctx.Err() != nil {
		return execution, context.Cause(ctx)
	}
	if streamErr != nil {
		if result.HasBillableTokenUsage() {
			return execution, streamErr
		}
		return deepSeekCompactExecution{}, s.newOpenAIStreamFailoverError(c, account, false, requestID, nil, streamErr.Error(), resp.Header)
	}

	checkpoint := frameDeepSeekCompactSummary(streamResult.Summary)
	encryptedContent, err := s.sealDeepSeekCompactCheckpoint(ctx, checkpoint)
	if err != nil {
		return execution, err
	}
	responseID := "resp_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	itemID := "cmp_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	finalJSON, err := deepSeekCompactResponsesJSON(responseID, itemID, originalModel, encryptedContent, streamResult.Usage, streamResult.TotalTokens)
	if err != nil {
		return execution, fmt.Errorf("encode DeepSeek compact response: %w", err)
	}
	result.ResponseID = responseID
	execution.FinalJSON = finalJSON
	s.bindHTTPResponseAccount(ctx, c, account, responseID)
	return execution, nil
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
	token := account.GetDeepSeekAPIKey()
	if token == "" {
		return nil, fmt.Errorf("account %d missing api_key", account.ID)
	}
	targetURL, err := s.deepSeekEndpointURL(account, deepSeekResponsesEndpoint)
	if err != nil {
		return nil, err
	}
	clientStream := openAICompactClientWantsStream(c)
	execution, err := s.executeDeepSeekRemoteCompaction(
		ctx,
		c,
		account,
		body,
		originalModel,
		billingModel,
		upstreamModel,
		clientStream,
		func(responsesBody []byte) (*http.Response, error) {
			return s.sendCCUpstreamRequest(ctx, c, account, targetURL, responsesBody, true, token, "", "")
		},
	)
	if err != nil {
		var upstreamHTTPError *deepSeekCompactUpstreamHTTPError
		if !errors.As(err, &upstreamHTTPError) {
			return execution.Result, err
		}
		resp := &http.Response{
			StatusCode: upstreamHTTPError.StatusCode,
			Header:     upstreamHTTPError.Headers.Clone(),
			Body:       io.NopCloser(bytes.NewReader(upstreamHTTPError.Body)),
		}
		if StopOpenAICompactSSEKeepaliveCommitted(c) {
			result, handleErr := s.handleCommittedDeepSeekCompactHTTPError(ctx, c, account, resp, upstreamHTTPError.Body, upstreamModel)
			if result != nil {
				result.RequestKind = UsageRequestKindCompact
			}
			return result, handleErr
		}
		result, handleErr := s.handleErrorResponse(ctx, resp, c, account, upstreamHTTPError.RequestBody, billingModel)
		if result != nil {
			result.RequestKind = UsageRequestKindCompact
		}
		return result, handleErr
	}
	result := execution.Result
	syntheticResp := &http.Response{StatusCode: http.StatusOK, Header: execution.Headers.Clone()}
	s.writeDeepSeekResponsesHeaders(c, syntheticResp, clientStream)
	if clientStream {
		if !writeOpenAICompactSSEBridge(c, http.StatusOK, execution.FinalJSON) {
			return result, errors.New("failed to synthesize DeepSeek remote compaction response")
		}
	} else {
		c.Header("Content-Type", "application/json")
		c.Data(http.StatusOK, "application/json", execution.FinalJSON)
	}
	return result, nil
}
