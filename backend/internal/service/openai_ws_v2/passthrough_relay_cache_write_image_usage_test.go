package openai_ws_v2

import (
	"context"
	"testing"
	"time"

	coderws "github.com/coder/websocket"
	"github.com/stretchr/testify/require"
)

type cacheWriteImageUsageCase struct {
	name    string
	payload string
	want    Usage
}

func cacheWriteImageUsageCases() []cacheWriteImageUsageCase {
	return []cacheWriteImageUsageCase{
		{
			name:    "input details explicit zero",
			payload: `{"type":"response.completed","response":{"id":"resp_input_zero","usage":{"input_tokens":100,"output_tokens":60,"input_tokens_details":{"cached_tokens":10,"image_tokens":40,"cache_write_tokens":0},"output_tokens_details":{"image_tokens":10}},"tool_usage":{"image_gen":{"input_tokens_details":{"image_tokens":900},"output_tokens_details":{"image_tokens":500}}}}}`,
			want:    Usage{InputTokens: 100, OutputTokens: 60, CacheReadInputTokens: 10, ImageInputTokens: 40, ImageOutputTokens: 10, CacheCreationInputTokensPresent: true},
		},
		{
			name:    "input details explicit null",
			payload: `{"type":"response.completed","response":{"id":"resp_input_null","usage":{"input_tokens":100,"output_tokens":60,"input_tokens_details":{"cached_tokens":10,"image_tokens":40,"cache_write_tokens":null},"output_tokens_details":{"image_tokens":10}}}}`,
			want:    Usage{InputTokens: 100, OutputTokens: 60, CacheReadInputTokens: 10, ImageInputTokens: 40, ImageOutputTokens: 10, CacheCreationInputTokensPresent: true},
		},
		{
			name:    "prompt details explicit zero",
			payload: `{"type":"response.completed","response":{"id":"resp_prompt_zero","usage":{"prompt_tokens":100,"completion_tokens":60,"prompt_tokens_details":{"cached_tokens":10,"image_tokens":7,"cache_write_tokens":0},"completion_tokens_details":{"image_tokens":2}}}}`,
			want:    Usage{InputTokens: 100, OutputTokens: 60, CacheReadInputTokens: 10, ImageInputTokens: 7, ImageOutputTokens: 2, CacheCreationInputTokensPresent: true},
		},
		{
			name:    "prompt details explicit null",
			payload: `{"type":"response.completed","response":{"id":"resp_prompt_null","usage":{"prompt_tokens":100,"completion_tokens":60,"prompt_tokens_details":{"cached_tokens":10,"image_tokens":7,"cache_write_tokens":null},"completion_tokens_details":{"image_tokens":2}}}}`,
			want:    Usage{InputTokens: 100, OutputTokens: 60, CacheReadInputTokens: 10, ImageInputTokens: 7, ImageOutputTokens: 2, CacheCreationInputTokensPresent: true},
		},
		{
			name:    "hosted image missing cache write",
			payload: `{"type":"response.completed","response":{"id":"resp_hosted_missing","usage":{"input_tokens":100,"output_tokens":60,"input_tokens_details":{"cached_tokens":10}},"tool_usage":{"image_gen":{"input_tokens_details":{"image_tokens":80},"output_tokens_details":{"image_tokens":50}}}}}`,
			want:    Usage{InputTokens: 100, OutputTokens: 60, CacheReadInputTokens: 10, ImageInputTokens: 80, ImageOutputTokens: 50},
		},
		{
			name:    "hosted image explicit zero",
			payload: `{"type":"response.completed","response":{"id":"resp_hosted_zero","usage":{"input_tokens":100,"output_tokens":60,"cache_creation_input_tokens":0,"input_tokens_details":{"cached_tokens":10}},"tool_usage":{"image_gen":{"input_tokens_details":{"image_tokens":80},"output_tokens_details":{"image_tokens":50}}}}}`,
			want:    Usage{InputTokens: 100, OutputTokens: 60, CacheReadInputTokens: 10, ImageInputTokens: 80, ImageOutputTokens: 50, CacheCreationInputTokensPresent: true},
		},
		{
			name:    "top level hosted image explicit null",
			payload: `{"type":"response.completed","response":{"id":"resp_hosted_null"},"usage":{"input_tokens":100,"output_tokens":60,"cache_write_tokens":null,"input_tokens_details":{"cached_tokens":10}},"tool_usage":{"image_gen":{"input_tokens_details":{"image_tokens":80},"output_tokens_details":{"image_tokens":50}}}}`,
			want:    Usage{InputTokens: 100, OutputTokens: 60, CacheReadInputTokens: 10, ImageInputTokens: 80, ImageOutputTokens: 50, CacheCreationInputTokensPresent: true},
		},
		{
			name:    "image and positive cache write",
			payload: `{"type":"response.completed","response":{"id":"resp_positive","usage":{"input_tokens":100,"output_tokens":60,"input_tokens_details":{"cached_tokens":10,"image_tokens":20,"cache_write_tokens":16},"output_tokens_details":{"image_tokens":5}}}}`,
			want:    Usage{InputTokens: 100, OutputTokens: 60, CacheReadInputTokens: 10, ImageInputTokens: 20, ImageOutputTokens: 5, CacheCreationInputTokens: 16, CacheCreationInputTokensPresent: true},
		},
		{
			name:    "text missing cache write",
			payload: `{"type":"response.completed","response":{"id":"resp_text","usage":{"input_tokens":3,"output_tokens":4}}}`,
			want:    Usage{InputTokens: 3, OutputTokens: 4},
		},
	}
}

func relayCacheWriteImageUsage(t *testing.T, cases []cacheWriteImageUsageCase) (RelayResult, []RelayTurnResult) {
	t.Helper()
	frames := make([]passthroughTestFrame, 0, len(cases))
	for _, tc := range cases {
		frames = append(frames, passthroughTestFrame{msgType: coderws.MessageText, payload: []byte(tc.payload)})
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	var turns []RelayTurnResult
	result, relayExit := Relay(ctx, newPassthroughTestFrameConn(nil, false), newPassthroughTestFrameConn(frames, true),
		[]byte(`{"type":"response.create","model":"gpt-5.4","input":[]}`),
		RelayOptions{OnTurnComplete: func(turn RelayTurnResult) { turns = append(turns, turn) }})
	require.Nil(t, relayExit)
	require.Len(t, turns, len(cases))
	return result, turns
}

func TestRelay_CacheWritePresenceAndImageUsage(t *testing.T) {
	t.Parallel()
	for _, tc := range cacheWriteImageUsageCases() {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			result, turns := relayCacheWriteImageUsage(t, []cacheWriteImageUsageCase{tc})
			require.Equal(t, tc.want, turns[0].Usage, "both image counters and authoritative cache-write presence must survive the terminal frame")
			require.Equal(t, tc.want, result.Usage, "the aggregate must preserve explicit zero/null presence and leave a missing field absent")
		})
	}
}

func TestRelay_CacheWritePresenceAndImageUsageAcrossTurns(t *testing.T) {
	t.Parallel()
	cases := cacheWriteImageUsageCases()
	result, turns := relayCacheWriteImageUsage(t, cases)
	for i, tc := range cases {
		require.Equal(t, tc.want, turns[i].Usage, "turn %d (%s) must not inherit image tokens or cache-write presence from an earlier turn", i+1, tc.name)
	}
	require.Equal(t, Usage{
		InputTokens: 803, OutputTokens: 484, CacheReadInputTokens: 80,
		ImageInputTokens: 354, ImageOutputTokens: 179,
		CacheCreationInputTokens: 16, CacheCreationInputTokensPresent: true,
	}, result.Usage, "aggregate counters add once per turn and cache-write presence is retained across text-only turns")
}

func TestRelay_CacheWritePresenceAndImageUsageFromProgressiveFrames(t *testing.T) {
	t.Parallel()
	state := &relayState{}
	parseUsageAndAccumulate(state, []byte(`{"type":"response.in_progress","response":{"usage":{"input_tokens":100,"output_tokens":60,"input_tokens_details":{"image_tokens":40,"cache_write_tokens":16},"output_tokens_details":{"image_tokens":10}}}}`), "response.in_progress", nil)
	parseUsageAndAccumulate(state, []byte(`{"type":"response.in_progress","response":{"usage":{"input_tokens_details":{"cache_write_tokens":0}}}}`), "response.in_progress", nil)
	turn := finalizeRelayTurnUsage(state)
	want := Usage{InputTokens: 100, OutputTokens: 60, ImageInputTokens: 40, ImageOutputTokens: 10, CacheCreationInputTokens: 16, CacheCreationInputTokensPresent: true}
	require.Equal(t, want, turn, "progressive zero must not erase positive cache-write or image usage")
	require.Equal(t, want, state.usage)
	require.Equal(t, Usage{}, state.turnUsage, "settlement resets image counters and presence together")
}
