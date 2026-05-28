package service

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// TestApplyCacheTTLOverrideToSSEBytes_MessageStart5m verifies the bedrock streaming
// rewrite path collapses 5m+1h breakdowns into the 5m bucket when cost_priority
// is chosen. Without this, Bedrock customers who select "cost_priority" would
// continue to see (and be billed for) the upstream 1h split.
func TestApplyCacheTTLOverrideToSSEBytes_MessageStart5m(t *testing.T) {
	in := []byte(`{"type":"message_start","message":{"usage":{"input_tokens":10,"cache_creation":{"ephemeral_5m_input_tokens":100,"ephemeral_1h_input_tokens":900}}}}`)

	out := applyCacheTTLOverrideToSSEBytes(in, "message_start", "5m")

	require.Equal(t, int64(1000), gjson.GetBytes(out, "message.usage.cache_creation.ephemeral_5m_input_tokens").Int())
	require.Equal(t, int64(0), gjson.GetBytes(out, "message.usage.cache_creation.ephemeral_1h_input_tokens").Int())
}

func TestApplyCacheTTLOverrideToSSEBytes_MessageDelta1h(t *testing.T) {
	in := []byte(`{"type":"message_delta","usage":{"cache_creation":{"ephemeral_5m_input_tokens":700,"ephemeral_1h_input_tokens":300}}}`)

	out := applyCacheTTLOverrideToSSEBytes(in, "message_delta", "1h")

	require.Equal(t, int64(1000), gjson.GetBytes(out, "usage.cache_creation.ephemeral_1h_input_tokens").Int())
	require.Equal(t, int64(0), gjson.GetBytes(out, "usage.cache_creation.ephemeral_5m_input_tokens").Int())
}

func TestApplyCacheTTLOverrideToSSEBytes_NoOpWhenAlreadyAtTarget(t *testing.T) {
	in := []byte(`{"type":"message_delta","usage":{"cache_creation":{"ephemeral_5m_input_tokens":1000,"ephemeral_1h_input_tokens":0}}}`)

	out := applyCacheTTLOverrideToSSEBytes(in, "message_delta", "5m")

	// Identical bytes — sjson should not have been called
	require.Equal(t, string(in), string(out))
}

func TestApplyCacheTTLOverrideToSSEBytes_IgnoresIrrelevantEventTypes(t *testing.T) {
	in := []byte(`{"type":"content_block_delta","delta":{"text":"hi"}}`)
	require.Equal(t, string(in), string(applyCacheTTLOverrideToSSEBytes(in, "content_block_delta", "5m")),
		"non-usage-bearing event types must be left untouched to avoid corrupting the stream")
}

func TestApplyCacheTTLOverrideToSSEBytes_NoOpWhenCacheCreationAbsent(t *testing.T) {
	in := []byte(`{"type":"message_delta","usage":{"output_tokens":5}}`)
	out := applyCacheTTLOverrideToSSEBytes(in, "message_delta", "1h")
	require.Equal(t, string(in), string(out))
}

func TestApplyCacheTTLOverrideToSSEBytes_NoOpWhenTotalIsZero(t *testing.T) {
	in := []byte(`{"type":"message_delta","usage":{"cache_creation":{"ephemeral_5m_input_tokens":0,"ephemeral_1h_input_tokens":0}}}`)
	out := applyCacheTTLOverrideToSSEBytes(in, "message_delta", "1h")
	require.Equal(t, string(in), string(out),
		"empty cache_creation must not be rewritten — would surface as a no-op change in the diff with no signal value")
}
