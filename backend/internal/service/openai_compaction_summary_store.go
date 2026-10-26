package service

import (
	"bytes"
	"context"
	"encoding/hex"
	"fmt"
	"log/slog"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

const (
	openAICompactionSummarySecretKey = "compaction_summary_key_v1"
	openAICompactionSummaryKeyBytes  = 32
)

type openAICompactionSummarySecretStore interface {
	GetSecuritySecret(ctx context.Context, key string) (string, bool, error)
	GetOrCreateGeneratedSecuritySecret(ctx context.Context, key string, byteLength int) (string, bool, error)
}

func (s *OpenAIGatewayService) openAICompactionSummaryCipherKey(ctx context.Context, create bool) ([]byte, error) {
	if s == nil {
		return nil, fmt.Errorf("%w: gateway service is unavailable", ErrOpenAICompactionSummaryKey)
	}
	s.compactionSummaryKeyMu.Lock()
	defer s.compactionSummaryKeyMu.Unlock()
	if len(s.compactionSummaryKey) == openAICompactionSummaryKeyBytes {
		return append([]byte(nil), s.compactionSummaryKey...), nil
	}
	store, ok := s.accountRepo.(openAICompactionSummarySecretStore)
	if !ok || store == nil {
		return nil, fmt.Errorf("%w: persisted secret store is unavailable", ErrOpenAICompactionSummaryKey)
	}

	var (
		value string
		found bool
		err   error
	)
	if create {
		value, _, err = store.GetOrCreateGeneratedSecuritySecret(ctx, openAICompactionSummarySecretKey, openAICompactionSummaryKeyBytes)
		found = err == nil
	} else {
		value, found, err = store.GetSecuritySecret(ctx, openAICompactionSummarySecretKey)
	}
	if err != nil {
		slog.ErrorContext(ctx, "openai_compaction_summary_key_load_failed")
		return nil, fmt.Errorf("%w: persisted secret is unavailable", ErrOpenAICompactionSummaryKey)
	}
	if !found {
		return nil, fmt.Errorf("%w: %s is not initialized", ErrOpenAICompactionSummaryKey, openAICompactionSummarySecretKey)
	}
	key, err := hex.DecodeString(strings.TrimSpace(value))
	if err != nil || len(key) != openAICompactionSummaryKeyBytes {
		return nil, fmt.Errorf("%w: stored key must be %d hex-encoded bytes", ErrOpenAICompactionSummaryKey, openAICompactionSummaryKeyBytes)
	}
	s.compactionSummaryKey = append([]byte(nil), key...)
	return key, nil
}

func (s *OpenAIGatewayService) EnsureOpenAICompactionSummaryKey(ctx context.Context) error {
	_, err := s.openAICompactionSummaryCipherKey(ctx, true)
	return err
}

// RestoreOpenAICompactionSummaryInput validates and restores only ciphertexts
// produced by this gateway. Provider-native compaction items are left byte-for-
// byte intact so the existing account-affinity and replay rules remain the only
// owners of native upstream state.
func (s *OpenAIGatewayService) RestoreOpenAICompactionSummaryInput(ctx context.Context, c *gin.Context, body []byte) ([]byte, bool, error) {
	if len(body) == 0 {
		return body, false, nil
	}
	if !gjson.ValidBytes(body) {
		return body, false, fmt.Errorf("%w: request JSON is invalid", ErrOpenAICompactionSummaryInvalid)
	}
	if err := ValidateOpenAICompactionInputShape(body); err != nil {
		return body, false, err
	}
	input := gjson.GetBytes(body, "input")
	if !input.Exists() || (!input.IsArray() && !input.IsObject()) {
		return body, false, nil
	}

	type restoreTarget struct {
		index      int
		item       gjson.Result
		ciphertext string
	}
	targets := make([]restoreTarget, 0, 1)
	inspectItem := func(index int, item gjson.Result) error {
		if !item.IsObject() {
			return nil
		}
		encrypted := item.Get("encrypted_content")
		if encrypted.Type != gjson.String || !looksLikeOpenAICompactionSummaryCiphertext(encrypted.String()) {
			return nil
		}
		itemType := strings.TrimSpace(item.Get("type").String())
		if !isOpenAICompactionType(itemType) {
			return fmt.Errorf("%w: gateway ciphertext is not a compaction item", ErrOpenAICompactionSummaryInvalid)
		}
		ciphertext := strings.TrimSpace(encrypted.String())
		if !IsOpenAICompactionSummaryCiphertext(ciphertext) {
			return fmt.Errorf("%w: unsupported version or key id", ErrOpenAICompactionSummaryInvalid)
		}
		targets = append(targets, restoreTarget{index: index, item: item, ciphertext: ciphertext})
		return nil
	}
	if input.IsArray() {
		index := 0
		var inspectErr error
		input.ForEach(func(_, item gjson.Result) bool {
			if err := inspectItem(index, item); err != nil {
				inspectErr = err
				return false
			}
			index++
			return true
		})
		if inspectErr != nil {
			return body, false, inspectErr
		}
	} else if err := inspectItem(-1, input); err != nil {
		return body, false, err
	}
	if len(targets) == 0 {
		return body, false, nil
	}

	apiKey := getAPIKeyFromContext(c)
	if apiKey == nil {
		return body, false, fmt.Errorf("%w: authenticated API key is unavailable", ErrOpenAICompactionSummaryInvalid)
	}
	scope, err := NewOpenAICompactionOwnerScope(apiKey.UserID, apiKey.GroupID)
	if err != nil {
		return body, false, err
	}
	key, err := s.openAICompactionSummaryCipherKey(ctx, false)
	if err != nil {
		return body, false, err
	}

	replacements := make(map[int][]byte, len(targets))
	for _, target := range targets {
		summary, err := OpenOpenAICompactionSummary(key, scope, target.ciphertext)
		if err != nil {
			return body, false, err
		}
		message, err := marshalOpenAIUpstreamJSON(map[string]any{
			"type": "message",
			"role": "user",
			"content": []any{map[string]any{
				"type": "input_text",
				"text": "<conversation_summary>\n" + summary + "\n</conversation_summary>",
			}},
		})
		if err != nil {
			return body, false, fmt.Errorf("encode restored compaction summary item: %w", err)
		}
		replacements[target.index] = message
	}
	if input.IsObject() {
		restored, err := sjson.SetRawBytes(body, "input", replacements[-1])
		if err != nil {
			return body, false, fmt.Errorf("replace restored compaction summary input: %w", err)
		}
		return restored, true, nil
	}

	var rebuilt bytes.Buffer
	rebuilt.Grow(len(input.Raw))
	copyFrom := 0
	index := 0
	input.ForEach(func(_, item gjson.Result) bool {
		replacement, replace := replacements[index]
		index++
		if !replace {
			return true
		}
		itemStart := item.Index - input.Index
		_, _ = rebuilt.WriteString(input.Raw[copyFrom:itemStart])
		_, _ = rebuilt.Write(replacement)
		copyFrom = itemStart + len(item.Raw)
		return true
	})
	_, _ = rebuilt.WriteString(input.Raw[copyFrom:])
	encoded, err := sjson.SetRawBytes(body, "input", rebuilt.Bytes())
	if err != nil {
		return body, false, fmt.Errorf("replace restored compaction summary input: %w", err)
	}
	return encoded, true, nil
}
