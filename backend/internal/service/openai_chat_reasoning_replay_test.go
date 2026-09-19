//go:build unit

package service

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func replayRecordFromCache(t *testing.T, cache *reasoningRecordingCache, callID string) chatReasoningReplayRecord {
	t.Helper()
	raw, ok := cache.snapshotSets()[chatReasoningReplayKeyPrefix+callID]
	require.True(t, ok, "expected cache record for %s", callID)
	var record chatReasoningReplayRecord
	require.NoError(t, json.Unmarshal([]byte(raw), &record))
	return record
}

// Each reasoning item is bound to the next completed tool call in stream
// order; reasoning after the last tool call (final message) is not stored.
func TestChatReasoningReplayRecorder_BindsReasoningToNextToolCall(t *testing.T) {
	cache := &reasoningRecordingCache{}
	svc := &OpenAIGatewayService{cache: cache}
	rec := newChatReasoningReplayRecorder(svc, 7, nil)

	rec.Observe(&apicompat.ResponsesStreamEvent{Type: "response.created", Response: &apicompat.ResponsesResponse{ID: "resp_1"}})
	rec.Observe(&apicompat.ResponsesStreamEvent{Type: "response.output_item.added", Item: &apicompat.ResponsesOutput{Type: "reasoning", ID: "rs_1"}})
	rec.Observe(&apicompat.ResponsesStreamEvent{Type: "response.output_item.done", Item: &apicompat.ResponsesOutput{Type: "reasoning", ID: "rs_1", EncryptedContent: "enc-1"}})
	rec.Observe(&apicompat.ResponsesStreamEvent{Type: "response.output_item.done", Item: &apicompat.ResponsesOutput{Type: "reasoning", ID: "rs_2", EncryptedContent: "enc-2"}})
	rec.Observe(&apicompat.ResponsesStreamEvent{Type: "response.output_item.done", Item: &apicompat.ResponsesOutput{Type: "function_call", CallID: "call_a", Name: "ls"}})
	rec.Observe(&apicompat.ResponsesStreamEvent{Type: "response.output_item.done", Item: &apicompat.ResponsesOutput{Type: "reasoning", ID: "rs_3", EncryptedContent: "enc-3"}})
	rec.Observe(&apicompat.ResponsesStreamEvent{Type: "response.output_item.done", Item: &apicompat.ResponsesOutput{Type: "custom_tool_call", CallID: "call_b", Name: "apply_patch"}})
	rec.Observe(&apicompat.ResponsesStreamEvent{Type: "response.output_item.done", Item: &apicompat.ResponsesOutput{Type: "reasoning", ID: "rs_4", EncryptedContent: "enc-4"}})
	rec.Observe(&apicompat.ResponsesStreamEvent{Type: "response.output_item.done", Item: &apicompat.ResponsesOutput{Type: "message", Role: "assistant"}})
	rec.Observe(&apicompat.ResponsesStreamEvent{Type: "response.completed"})
	rec.Finish()

	a := replayRecordFromCache(t, cache, "call_a")
	assert.Equal(t, int64(7), a.AccountID)
	assert.Equal(t, "resp_1", a.ResponseID)
	assert.Equal(t, []string{"enc-1", "enc-2"}, a.Items)
	assert.NotZero(t, a.CreatedAt)

	b := replayRecordFromCache(t, cache, "call_b")
	assert.Equal(t, []string{"enc-3"}, b.Items)

	assert.Len(t, cache.snapshotSets(), 2, "reasoning after the final tool call has no call_id and must not be stored")
	assert.Equal(t, 4, rec.reasoningSeen)
	assert.Equal(t, 2, rec.toolCallsSeen)
	assert.Equal(t, 2, rec.bound)
	assert.Equal(t, 2, rec.stored)
	assert.Len(t, rec.pending, 1)
}

// A parallel batch (one reasoning item, several function calls) binds the
// reasoning to the first call and marks the rest as covered by it, so the
// next lookup does not report them as misses.
func TestChatReasoningReplayRecorder_ParallelBatchMarksSiblings(t *testing.T) {
	cache := &reasoningRecordingCache{}
	svc := &OpenAIGatewayService{cache: cache}
	rec := newChatReasoningReplayRecorder(svc, 7, nil)
	rec.ObserveOutput("resp_par", []apicompat.ResponsesOutput{
		{Type: "reasoning", EncryptedContent: "enc-batch"},
		{Type: "function_call", CallID: "call_1"},
		{Type: "function_call", CallID: "call_2"},
		{Type: "function_call", CallID: "call_3"},
		{Type: "reasoning", EncryptedContent: "enc-second"},
		{Type: "function_call", CallID: "call_4"},
		{Type: "function_call", CallID: "call_5"},
	})
	rec.Finish()

	owner := replayRecordFromCache(t, cache, "call_1")
	assert.Equal(t, []string{"enc-batch"}, owner.Items)
	assert.Empty(t, owner.CoveredBy)
	for _, sibling := range []string{"call_2", "call_3"} {
		marker := replayRecordFromCache(t, cache, sibling)
		assert.Empty(t, marker.Items)
		assert.Equal(t, "call_1", marker.CoveredBy)
		assert.Equal(t, int64(7), marker.AccountID)
	}
	second := replayRecordFromCache(t, cache, "call_4")
	assert.Equal(t, []string{"enc-second"}, second.Items)
	assert.Equal(t, "call_4", replayRecordFromCache(t, cache, "call_5").CoveredBy)
	assert.Equal(t, 2, rec.bound)
	assert.Equal(t, 3, rec.covered)
	assert.Equal(t, 5, rec.stored)

	cache.getResp = cache.snapshotSets()
	hook, stats := svc.chatReasoningReplayLookup(7, nil)
	assert.Equal(t, []string{"enc-batch"}, hook("call_1"))
	assert.Nil(t, hook("call_2"))
	assert.Nil(t, hook("call_3"))
	assert.Equal(t, []string{"enc-second"}, hook("call_4"))
	assert.Nil(t, hook("call_5"))
	assert.Equal(t, 2, stats.Hits)
	assert.Equal(t, 3, stats.CoveredBySibling)
	assert.Equal(t, 0, stats.Misses)
	assert.Equal(t, 2, stats.Injected)
}

// Reasoning items without encrypted_content carry nothing replayable, and a
// tool call with no preceding reasoning writes nothing.
func TestChatReasoningReplayRecorder_IgnoresItemsWithoutCipher(t *testing.T) {
	cache := &reasoningRecordingCache{}
	rec := newChatReasoningReplayRecorder(&OpenAIGatewayService{cache: cache}, 1, nil)

	rec.Observe(&apicompat.ResponsesStreamEvent{Type: "response.output_item.done", Item: &apicompat.ResponsesOutput{Type: "reasoning", ID: "rs_1"}})
	rec.Observe(&apicompat.ResponsesStreamEvent{Type: "response.output_item.done", Item: &apicompat.ResponsesOutput{Type: "function_call", CallID: "call_a"}})
	rec.Observe(&apicompat.ResponsesStreamEvent{Type: "response.output_item.done", Item: &apicompat.ResponsesOutput{Type: "function_call", CallID: "call_b"}})
	rec.Finish()

	assert.Empty(t, cache.snapshotSets(), "no bound owner means no sibling markers either")
	assert.Equal(t, 1, rec.reasoningNoCipher)
	assert.Equal(t, 0, rec.bound)
	assert.Equal(t, 0, rec.covered)
}

// The buffered (stream=false) path feeds the terminal output list.
func TestChatReasoningReplayRecorder_ObserveOutput(t *testing.T) {
	cache := &reasoningRecordingCache{}
	rec := newChatReasoningReplayRecorder(&OpenAIGatewayService{cache: cache}, 3, nil)
	rec.ObserveOutput("resp_buffered", []apicompat.ResponsesOutput{
		{Type: "reasoning", ID: "rs_1", EncryptedContent: "enc-1"},
		{Type: "function_call", CallID: "call_x", Name: "ls"},
	})
	rec.Finish()

	record := replayRecordFromCache(t, cache, "call_x")
	assert.Equal(t, "resp_buffered", record.ResponseID)
	assert.Equal(t, []string{"enc-1"}, record.Items)
}

// A nil recorder is safe to drive (replay disabled for the request).
func TestChatReasoningReplayRecorder_NilSafe(t *testing.T) {
	var rec *chatReasoningReplayRecorder
	rec.Observe(&apicompat.ResponsesStreamEvent{Type: "response.output_item.done", Item: &apicompat.ResponsesOutput{Type: "reasoning", EncryptedContent: "x"}})
	rec.ObserveOutput("r", nil)
	rec.Finish()
}

// Lookup returns cached items only for the same account and counts outcomes.
func TestChatReasoningReplayLookup_AccountScopedHitsAndMisses(t *testing.T) {
	sameAccount, _ := json.Marshal(chatReasoningReplayRecord{AccountID: 7, ResponseID: "resp_1", Items: []string{"enc-1", "enc-2"}, CreatedAt: 1})
	otherAccount, _ := json.Marshal(chatReasoningReplayRecord{AccountID: 8, Items: []string{"enc-foreign"}, CreatedAt: 1})
	cache := &reasoningRecordingCache{getResp: map[string]string{
		chatReasoningReplayKeyPrefix + "call_same":      string(sameAccount),
		chatReasoningReplayKeyPrefix + "call_other":     string(otherAccount),
		chatReasoningReplayKeyPrefix + "call_malformed": "{not json",
		chatReasoningReplayKeyPrefix + "call_empty":     `{"account_id":7,"encrypted_items":[]}`,
	}}
	svc := &OpenAIGatewayService{cache: cache}

	hook, stats := svc.chatReasoningReplayLookup(7, nil)
	require.NotNil(t, hook)

	assert.Equal(t, []string{"enc-1", "enc-2"}, hook("call_same"))
	assert.Nil(t, hook("call_other"), "ciphertext from another account must never be injected")
	assert.Nil(t, hook("call_missing"))
	assert.Nil(t, hook("call_malformed"))
	assert.Nil(t, hook("call_empty"))
	assert.Nil(t, hook("  "))

	assert.Equal(t, 6, stats.Lookups)
	assert.Equal(t, 1, stats.Hits)
	assert.Equal(t, 1, stats.AccountMismatch)
	assert.Equal(t, 4, stats.Misses)
	assert.Equal(t, 2, stats.Injected)
}

// Round trip: what the recorder stores is what the lookup returns.
func TestChatReasoningReplay_RecorderLookupRoundTrip(t *testing.T) {
	cache := &reasoningRecordingCache{}
	svc := &OpenAIGatewayService{cache: cache}
	rec := newChatReasoningReplayRecorder(svc, 5, nil)
	rec.ObserveOutput("resp_rt", []apicompat.ResponsesOutput{
		{Type: "reasoning", EncryptedContent: "enc-rt"},
		{Type: "function_call", CallID: "call_rt"},
	})
	cache.getResp = cache.snapshotSets()

	hook, stats := svc.chatReasoningReplayLookup(5, nil)
	assert.Equal(t, []string{"enc-rt"}, hook("call_rt"))
	assert.Equal(t, 1, stats.Hits)
}

func TestChatReasoningReplayEnabled(t *testing.T) {
	cache := &reasoningRecordingCache{}
	svc := &OpenAIGatewayService{cache: cache}
	codex := &Account{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeOAuth}
	require.True(t, codex.UsesOpenAICodexProtocol(), "test fixture must be a codex oauth account")

	assert.True(t, svc.chatReasoningReplayEnabled(context.Background(), codex))
	assert.False(t, svc.chatReasoningReplayEnabled(withChatReasoningReplayDisabled(context.Background()), codex), "retry after upstream rejection must not inject again")
	assert.False(t, (&OpenAIGatewayService{}).chatReasoningReplayEnabled(context.Background(), codex), "no cache, no replay")
	assert.False(t, svc.chatReasoningReplayEnabled(context.Background(), nil))

	apiKey := &Account{ID: 2, Platform: PlatformOpenAI, Type: AccountTypeAPIKey}
	assert.False(t, svc.chatReasoningReplayEnabled(context.Background(), apiKey), "third-party API key upstreams do not return replayable codex ciphertext")
}

func TestIsOpenAIInvalidEncryptedContentError(t *testing.T) {
	assert.True(t, isOpenAIInvalidEncryptedContentError([]byte(`{"error":{"code":"invalid_encrypted_content","message":"x"}}`), "x"))
	assert.True(t, isOpenAIInvalidEncryptedContentError(nil, "The encrypted content CAIS... could not be Verified. Reason: Encrypted content could not be decrypted or parsed."))
	assert.False(t, isOpenAIInvalidEncryptedContentError([]byte(`{"error":{"code":"invalid_request_error","message":"Instructions are required"}}`), "Instructions are required"))
	assert.False(t, isOpenAIInvalidEncryptedContentError(nil, "encrypted content is fine"))
}
