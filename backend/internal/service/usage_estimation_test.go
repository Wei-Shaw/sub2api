//go:build unit

package service

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFinalizeOpenAIUsageEstimate_MissingUsageGetsBillableFallback(t *testing.T) {
	result := &OpenAIForwardResult{RequestBytes: 401, ObservedResponseBytes: 81}

	finalizeOpenAIUsageEstimate(result)

	require.Equal(t, UsageSourceEstimated, result.UsageSource)
	require.Equal(t, 101, result.Usage.InputTokens)
	require.Equal(t, 21, result.Usage.OutputTokens)
}

func TestFinalizeOpenAIUsageEstimate_UpstreamUsageIsAuthoritative(t *testing.T) {
	result := &OpenAIForwardResult{
		Usage:        OpenAIUsage{InputTokens: 123, OutputTokens: 45},
		UsagePresent: true,
		RequestBytes: 9999,
	}

	finalizeOpenAIUsageEstimate(result)

	require.Equal(t, UsageSourceUpstream, result.UsageSource)
	require.Equal(t, 123, result.Usage.InputTokens)
	require.Equal(t, 45, result.Usage.OutputTokens)
}

func TestFinalizeClaudeUsageEstimate_ExplicitZeroUsageIsNotRewritten(t *testing.T) {
	result := &ForwardResult{UsagePresent: true, RequestBytes: 400}

	finalizeClaudeUsageEstimate(result)

	require.Equal(t, UsageSourceUpstream, result.UsageSource)
	require.Zero(t, result.Usage.InputTokens)
	require.Zero(t, result.Usage.OutputTokens)
}

func TestRequestBodyBytesForUsageEstimate_DoesNotCountInlineImagePayload(t *testing.T) {
	body := []byte(`{"model":"gpt-5","input":[{"role":"user","content":[{"type":"input_text","text":"hello"},{"type":"input_image","image_url":"data:image/png;base64,AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"}]}]}`)

	measured := requestBodyBytesForUsageEstimate(body)

	require.Greater(t, measured, len("hello"))
	require.Less(t, measured, len(body)-20)
}

func TestFinalizeOpenAIUsageEstimate_DoesNotEstimateFailedRequest(t *testing.T) {
	result := &OpenAIForwardResult{ObservedResponseBytes: 120}

	(&OpenAIGatewayService{}).finalizeUsageEstimate(context.Background(), nil, nil, 400, result, errors.New("upstream failed"))

	require.Empty(t, result.UsageSource)
	require.Zero(t, result.Usage.InputTokens)
	require.Zero(t, result.Usage.OutputTokens)
}
