//go:build unit

package service

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsOpenAIWSIngressTurnRetryable_NilError(t *testing.T) {
	assert.False(t, isOpenAIWSIngressTurnRetryable(nil))
}

func TestIsOpenAIWSIngressTurnRetryable_NonTurnError(t *testing.T) {
	assert.False(t, isOpenAIWSIngressTurnRetryable(errors.New("random error")))
}

func TestIsOpenAIWSIngressTurnRetryable_ContextCanceled(t *testing.T) {
	err := wrapOpenAIWSIngressTurnError("read_upstream", context.Canceled, false)
	assert.False(t, isOpenAIWSIngressTurnRetryable(err))
}

func TestIsOpenAIWSIngressTurnRetryable_ContextDeadline(t *testing.T) {
	err := wrapOpenAIWSIngressTurnError("read_upstream", context.DeadlineExceeded, false)
	assert.False(t, isOpenAIWSIngressTurnRetryable(err))
}

func TestIsOpenAIWSIngressTurnRetryable_WroteDownstream(t *testing.T) {
	err := wrapOpenAIWSIngressTurnError("read_upstream", errors.New("timeout"), true)
	assert.False(t, isOpenAIWSIngressTurnRetryable(err))
}

func TestIsOpenAIWSIngressTurnRetryable_ReadUpstreamRetryable(t *testing.T) {
	err := wrapOpenAIWSIngressTurnError("read_upstream", errors.New("connection reset"), false)
	assert.True(t, isOpenAIWSIngressTurnRetryable(err))
}

func TestIsOpenAIWSIngressTurnRetryable_WriteUpstreamRetryable(t *testing.T) {
	err := wrapOpenAIWSIngressTurnError("write_upstream", errors.New("broken pipe"), false)
	assert.True(t, isOpenAIWSIngressTurnRetryable(err))
}

func TestOpenAIWSIngressTurnRetryReason_NilError(t *testing.T) {
	assert.Equal(t, "unknown", openAIWSIngressTurnRetryReason(nil))
}

func TestOpenAIWSIngressTurnRetryReason_NonTurnError(t *testing.T) {
	assert.Equal(t, "unknown", openAIWSIngressTurnRetryReason(errors.New("random")))
}

func TestOpenAIWSIngressTurnRetryReason_ExtractsStage(t *testing.T) {
	err := wrapOpenAIWSIngressTurnError("read_upstream", errors.New("timeout"), false)
	reason := openAIWSIngressTurnRetryReason(err)
	assert.Equal(t, "read_upstream", reason)
}

func TestIsOpenAIWSIngressPreviousResponseNotFound_True(t *testing.T) {
	err := wrapOpenAIWSIngressTurnError("previous_response_not_found", errors.New("not found"), false)
	assert.True(t, isOpenAIWSIngressPreviousResponseNotFound(err))
}

func TestIsOpenAIWSIngressPreviousResponseNotFound_WroteDownstream(t *testing.T) {
	err := wrapOpenAIWSIngressTurnError("previous_response_not_found", errors.New("not found"), true)
	assert.False(t, isOpenAIWSIngressPreviousResponseNotFound(err))
}

func TestIsOpenAIWSIngressPreviousResponseNotFound_DifferentStage(t *testing.T) {
	err := wrapOpenAIWSIngressTurnError("read_upstream", errors.New("not found"), false)
	assert.False(t, isOpenAIWSIngressPreviousResponseNotFound(err))
}

func TestApplyOpenAIWSRetryPayloadStrategy_NilPayload(t *testing.T) {
	strategy, removed := applyOpenAIWSRetryPayloadStrategy(nil, 1)
	assert.Equal(t, "empty", strategy)
	assert.Empty(t, removed)
}

func TestApplyOpenAIWSRetryPayloadStrategy_FirstAttempt(t *testing.T) {
	payload := map[string]any{
		"model": "gpt-5",
		"input": "hello",
		"include": []string{"reasoning"},
	}
	strategy, removed := applyOpenAIWSRetryPayloadStrategy(payload, 1)
	assert.Equal(t, "full", strategy)
	assert.Empty(t, removed)
	// payload should be unchanged
	assert.Contains(t, payload, "include")
}

func TestApplyOpenAIWSRetryPayloadStrategy_SecondAttempt(t *testing.T) {
	payload := map[string]any{
		"model":            "gpt-5",
		"input":            "hello",
		"include":          []string{"reasoning"},
		"prompt_cache_key": "pcache_123",
	}
	strategy, removed := applyOpenAIWSRetryPayloadStrategy(payload, 2)
	assert.NotEqual(t, "full", strategy)
	// prompt_cache_key should be preserved
	assert.Contains(t, payload, "prompt_cache_key")
	// include should be removed
	assert.Contains(t, removed, "include")
}
