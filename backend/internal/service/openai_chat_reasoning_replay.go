package service

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// Chat Completions → Responses reasoning replay.
//
// Reasoning models keep their working state in Responses `reasoning` items.
// Under store=false the API returns them with encrypted_content and expects
// the caller to replay every reasoning item that preceded a function_call
// when it sends the function_call_output back. Codex clients do this
// natively. Chat Completions clients cannot: the protocol has no field for
// a reasoning item, so an assistant message with tool_calls comes back as
// text plus tool_calls and the reasoning that produced them is lost. The
// model then continues each tool round from scratch, which noticeably
// degrades multi-step agent quality even though every request succeeds.
//
// This file closes the gap on the gateway side:
//
//   - Response direction (chatReasoningReplayRecorder): while the Responses
//     stream is translated to Chat chunks, completed `reasoning` items are
//     held until the next completed function_call / custom_tool_call and
//     then stored under that call_id, together with the account that
//     produced them (encrypted_content is bound to the upstream account).
//   - Request direction (chatReasoningReplayLookup): when the next Chat
//     request replays the assistant tool_calls, the converter asks for the
//     cached items by call_id and inserts them as `reasoning` input items
//     right before the matching function_call. Items produced by another
//     account are skipped so the upstream never sees foreign ciphertext.
//
// The cache is best-effort: a miss leaves the request exactly as before.

const (
	// chatReasoningReplayKeyPrefix namespaces call_id keys inside the shared
	// reasoning cache (repository key becomes "reasoning_content:cc_tool_call:<call_id>").
	chatReasoningReplayKeyPrefix = "cc_tool_call:"
	chatReasoningReplayCacheTTL  = responsesReasoningCacheTTL
	chatReasoningReplayCacheOpTO = 2 * time.Second
	// chatReasoningReplayMaxItemsPerCall bounds the number of reasoning items
	// bound to one tool call; Codex emits one, occasionally two.
	chatReasoningReplayMaxItemsPerCall = 8
)

// chatReasoningReplayRecord is the JSON document stored per call_id.
//
// A Responses turn emits one reasoning item and then possibly several
// parallel function calls. The reasoning is bound to the first call of that
// batch (Items non-empty); every later call in the same batch gets a marker
// record with CoveredBy pointing at that first call and no Items, so a
// lookup can tell "already replayed via a sibling" apart from a real miss.
type chatReasoningReplayRecord struct {
	AccountID  int64    `json:"account_id"`
	ResponseID string   `json:"response_id,omitempty"`
	Items      []string `json:"encrypted_items,omitempty"`
	CoveredBy  string   `json:"covered_by,omitempty"`
	CreatedAt  int64    `json:"created_at"`
}

// chatReasoningReplayStats summarizes one request's lookup phase for logging.
type chatReasoningReplayStats struct {
	Lookups          int
	Hits             int
	Misses           int
	CoveredBySibling int // call ids whose reasoning was replayed via the first call of their batch
	AccountMismatch  int
	Injected         int
}

type chatReasoningReplayDisabledContextKey struct{}

// withChatReasoningReplayDisabled marks a retry that must not inject cached
// reasoning (used after the upstream rejected the injected ciphertext).
func withChatReasoningReplayDisabled(ctx context.Context) context.Context {
	return context.WithValue(ctx, chatReasoningReplayDisabledContextKey{}, true)
}

func chatReasoningReplayDisabled(ctx context.Context) bool {
	if ctx == nil {
		return false
	}
	disabled, _ := ctx.Value(chatReasoningReplayDisabledContextKey{}).(bool)
	return disabled
}

const chatReasoningReplayRecorderGinKey = "openai.chat_reasoning_replay.recorder"

func setChatReasoningReplayRecorder(c *gin.Context, rec *chatReasoningReplayRecorder) {
	if c == nil {
		return
	}
	if rec == nil {
		c.Set(chatReasoningReplayRecorderGinKey, nil)
		return
	}
	c.Set(chatReasoningReplayRecorderGinKey, rec)
}

func chatReasoningReplayRecorderFromContext(c *gin.Context) *chatReasoningReplayRecorder {
	if c == nil {
		return nil
	}
	v, ok := c.Get(chatReasoningReplayRecorderGinKey)
	if !ok || v == nil {
		return nil
	}
	rec, _ := v.(*chatReasoningReplayRecorder)
	return rec
}

// chatReasoningReplayEnabled reports whether the replay bridge applies to
// this request: only the Codex OAuth protocol returns encrypted reasoning we
// can replay, and a cache must be available.
func (s *OpenAIGatewayService) chatReasoningReplayEnabled(ctx context.Context, account *Account) bool {
	if s == nil || s.cache == nil || account == nil {
		return false
	}
	if !account.UsesOpenAICodexProtocol() {
		return false
	}
	return !chatReasoningReplayDisabled(ctx)
}

// chatReasoningReplayLookup returns the converter hook plus the stats it
// fills in. accountID scopes the hit: records written by another account are
// reported as AccountMismatch and not injected.
func (s *OpenAIGatewayService) chatReasoningReplayLookup(accountID int64, log *zap.Logger) (func(callID string) []string, *chatReasoningReplayStats) {
	stats := &chatReasoningReplayStats{}
	if s == nil || s.cache == nil {
		return nil, stats
	}
	if log == nil {
		log = logger.L()
	}
	hook := func(callID string) []string {
		stats.Lookups++
		record, ok := s.loadChatReasoningReplayRecord(callID)
		if !ok {
			stats.Misses++
			log.Debug("openai chat_completions: reasoning replay miss",
				zap.String("call_id", callID),
			)
			return nil
		}
		if len(record.Items) == 0 {
			// Sibling of a parallel batch: the reasoning item is injected in
			// front of record.CoveredBy, which precedes this call in the input.
			stats.CoveredBySibling++
			log.Debug("openai chat_completions: reasoning replay covered by sibling call",
				zap.String("call_id", callID),
				zap.String("covered_by", record.CoveredBy),
			)
			return nil
		}
		if record.AccountID != accountID {
			stats.AccountMismatch++
			log.Info("openai chat_completions: reasoning replay skipped, cached by another account",
				zap.String("call_id", callID),
				zap.Int64("cached_account_id", record.AccountID),
				zap.Int64("account_id", accountID),
			)
			return nil
		}
		stats.Hits++
		stats.Injected += len(record.Items)
		log.Debug("openai chat_completions: reasoning replay hit",
			zap.String("call_id", callID),
			zap.Int("items", len(record.Items)),
			zap.Int("encrypted_bytes", chatReasoningReplayBytes(record.Items)),
			zap.String("cached_response_id", record.ResponseID),
			zap.Int64("cached_age_seconds", time.Now().Unix()-record.CreatedAt),
		)
		return record.Items
	}
	return hook, stats
}

func (s *OpenAIGatewayService) loadChatReasoningReplayRecord(callID string) (*chatReasoningReplayRecord, bool) {
	callID = strings.TrimSpace(callID)
	if callID == "" || s == nil || s.cache == nil {
		return nil, false
	}
	ctx, cancel := context.WithTimeout(context.Background(), chatReasoningReplayCacheOpTO)
	defer cancel()
	raw, err := s.cache.GetReasoningContent(ctx, chatReasoningReplayKeyPrefix+callID)
	if err != nil {
		if err != ErrReasoningContentNotFound {
			logger.L().Warn("openai chat_completions: reasoning replay cache read failed",
				zap.Error(err),
				zap.String("call_id", callID),
			)
		}
		return nil, false
	}
	var record chatReasoningReplayRecord
	if err := json.Unmarshal([]byte(raw), &record); err != nil {
		logger.L().Warn("openai chat_completions: reasoning replay cache record malformed",
			zap.Error(err),
			zap.String("call_id", callID),
		)
		return nil, false
	}
	if len(record.Items) == 0 && strings.TrimSpace(record.CoveredBy) == "" {
		return nil, false
	}
	return &record, true
}

func (s *OpenAIGatewayService) storeChatReasoningReplayRecord(callID string, record *chatReasoningReplayRecord) error {
	if s == nil || s.cache == nil || record == nil || (len(record.Items) == 0 && record.CoveredBy == "") {
		return nil
	}
	callID = strings.TrimSpace(callID)
	if callID == "" {
		return nil
	}
	raw, err := json.Marshal(record)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), chatReasoningReplayCacheOpTO)
	defer cancel()
	return s.cache.SetReasoningContent(ctx, chatReasoningReplayKeyPrefix+callID, string(raw), chatReasoningReplayCacheTTL)
}

func chatReasoningReplayBytes(items []string) int {
	n := 0
	for _, item := range items {
		n += len(item)
	}
	return n
}

// chatReasoningReplayRecorder binds completed reasoning items to the next
// completed tool call in one upstream Responses stream and persists them.
type chatReasoningReplayRecorder struct {
	svc        *OpenAIGatewayService
	log        *zap.Logger
	accountID  int64
	responseID string

	pending     []string // encrypted reasoning items not yet bound to a call
	lastBoundID string   // first call of the current parallel batch (owner of the reasoning record)

	reasoningSeen     int // reasoning items with encrypted_content
	reasoningNoCipher int // reasoning items without encrypted_content (nothing to replay)
	toolCallsSeen     int
	bound             int // tool calls that got reasoning attached
	covered           int // sibling calls marked as covered by the batch owner
	stored            int // records written to cache
	storeFailures     int
	finished          bool
}

func newChatReasoningReplayRecorder(svc *OpenAIGatewayService, accountID int64, log *zap.Logger) *chatReasoningReplayRecorder {
	if log == nil {
		log = logger.L()
	}
	return &chatReasoningReplayRecorder{svc: svc, log: log, accountID: accountID}
}

// Observe inspects one Responses stream event.
func (r *chatReasoningReplayRecorder) Observe(event *apicompat.ResponsesStreamEvent) {
	if r == nil || event == nil {
		return
	}
	switch strings.TrimSpace(event.Type) {
	case "response.created":
		if event.Response != nil && event.Response.ID != "" {
			r.responseID = event.Response.ID
		}
	case "response.output_item.done":
		r.observeItem(event.Item)
	}
}

// ObserveOutput inspects a terminal (buffered) Responses output list.
func (r *chatReasoningReplayRecorder) ObserveOutput(responseID string, output []apicompat.ResponsesOutput) {
	if r == nil {
		return
	}
	if responseID != "" {
		r.responseID = responseID
	}
	for i := range output {
		r.observeItem(&output[i])
	}
}

func (r *chatReasoningReplayRecorder) observeItem(item *apicompat.ResponsesOutput) {
	if item == nil {
		return
	}
	switch item.Type {
	case "reasoning":
		encrypted := strings.TrimSpace(item.EncryptedContent)
		if encrypted == "" {
			r.reasoningNoCipher++
			return
		}
		r.reasoningSeen++
		if len(r.pending) < chatReasoningReplayMaxItemsPerCall {
			r.pending = append(r.pending, encrypted)
		}
	case "function_call", "custom_tool_call":
		r.toolCallsSeen++
		callID := strings.TrimSpace(item.CallID)
		if callID == "" {
			return
		}
		if len(r.pending) == 0 {
			if r.lastBoundID == "" || r.lastBoundID == callID {
				return
			}
			// Parallel sibling: no reasoning of its own, covered by the batch owner.
			marker := &chatReasoningReplayRecord{
				AccountID:  r.accountID,
				ResponseID: r.responseID,
				CoveredBy:  r.lastBoundID,
				CreatedAt:  time.Now().Unix(),
			}
			r.covered++
			if err := r.svc.storeChatReasoningReplayRecord(callID, marker); err != nil {
				r.storeFailures++
				r.log.Warn("openai chat_completions: reasoning replay sibling marker write failed",
					zap.Error(err),
					zap.String("call_id", callID),
					zap.String("covered_by", r.lastBoundID),
				)
				return
			}
			r.stored++
			return
		}
		record := &chatReasoningReplayRecord{
			AccountID:  r.accountID,
			ResponseID: r.responseID,
			Items:      r.pending,
			CreatedAt:  time.Now().Unix(),
		}
		r.pending = nil
		r.lastBoundID = callID
		r.bound++
		if err := r.svc.storeChatReasoningReplayRecord(callID, record); err != nil {
			r.storeFailures++
			r.log.Warn("openai chat_completions: reasoning replay cache write failed",
				zap.Error(err),
				zap.String("call_id", callID),
			)
			return
		}
		r.stored++
		r.log.Debug("openai chat_completions: reasoning replay cached",
			zap.String("call_id", callID),
			zap.Int64("account_id", r.accountID),
			zap.Int("items", len(record.Items)),
			zap.Int("encrypted_bytes", chatReasoningReplayBytes(record.Items)),
			zap.String("response_id", r.responseID),
		)
	}
}

// Finish logs a per-response summary. Reasoning left in pending belongs to a
// final assistant message (no following tool call); it has no call_id to
// hang on and is not replayable through Chat Completions today.
func (r *chatReasoningReplayRecorder) Finish() {
	if r == nil || r.finished {
		return
	}
	r.finished = true
	if r.reasoningSeen == 0 && r.reasoningNoCipher == 0 && r.toolCallsSeen == 0 {
		return
	}
	fields := []zap.Field{
		zap.String("response_id", r.responseID),
		zap.Int64("account_id", r.accountID),
		zap.Int("reasoning_items", r.reasoningSeen),
		zap.Int("reasoning_items_without_cipher", r.reasoningNoCipher),
		zap.Int("tool_calls", r.toolCallsSeen),
		zap.Int("tool_calls_bound", r.bound),
		zap.Int("tool_calls_covered_by_sibling", r.covered),
		zap.Int("records_stored", r.stored),
		zap.Int("store_failures", r.storeFailures),
		zap.Int("unbound_reasoning_items", len(r.pending)),
	}
	if r.storeFailures > 0 {
		r.log.Warn("openai chat_completions: reasoning replay recorded with cache failures", fields...)
		return
	}
	r.log.Debug("openai chat_completions: reasoning replay recorded", fields...)
}

// isOpenAIInvalidEncryptedContentError reports whether an upstream 400 body
// rejects replayed reasoning ciphertext. It matches the documented error code
// and the verify/decrypt message variants the ChatGPT backend emits.
func isOpenAIInvalidEncryptedContentError(respBody []byte, upstreamMsg string) bool {
	if strings.EqualFold(strings.TrimSpace(extractUpstreamErrorCode(respBody)), "invalid_encrypted_content") {
		return true
	}
	lower := strings.ToLower(upstreamMsg)
	if !strings.Contains(lower, "encrypted content") {
		return false
	}
	return strings.Contains(lower, "could not be verified") ||
		strings.Contains(lower, "could not be decrypted") ||
		strings.Contains(lower, "invalid_encrypted_content")
}
