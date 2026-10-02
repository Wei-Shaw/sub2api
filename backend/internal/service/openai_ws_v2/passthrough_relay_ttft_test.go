package openai_ws_v2

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestObserveUpstreamMessageEmptyEventsDoNotStartTTFT(t *testing.T) {
	start := time.Unix(100, 0)
	now := start
	state := &relayState{}
	state.setPendingTurnStartedAt(start)
	observe := func(data string) observedUpstreamEvent {
		now = now.Add(time.Second)
		return observeUpstreamMessage(state, []byte(data), start, func() time.Time { return now }, nil)
	}
	for _, data := range []string{
		`{"type":"response.output_text.delta","delta":"","SSE-Keep-Alive":true}`,
		`{"type":"response.created","response":{"id":"resp_1"}}`,
		`{"type":"response.in_progress","response":{"id":"resp_1"}}`,
		`{"type":"response.output_text.delta","response_id":"resp_1","delta":""}`,
		`{"type":"response.output_text.delta","delta":{}}`,
		`{"type":"response.output_item.added","item":{"type":"reasoning","summary":[]}}`,
		`{"type":"response.output_text.done","text":""}`,
	} {
		observe(data)
		require.Nil(t, state.firstTokenMs, data)
		if state.activeTurn != nil {
			require.Nil(t, state.activeTurn.firstTokenMs, data)
		}
	}
	terminal := observe(`{"type":"response.completed","response":{"id":"resp_1","usage":{"input_tokens":2,"output_tokens":1}}}`)
	require.True(t, terminal.terminal)
	require.Nil(t, terminal.firstToken)
	require.Nil(t, state.firstTokenMs)
	require.Equal(t, Usage{InputTokens: 2, OutputTokens: 1}, terminal.usage)
}

func TestObserveUpstreamMessageTTFTPerTurnWithoutResponseID(t *testing.T) {
	start := time.Unix(100, 0)
	now := start
	state := &relayState{}
	observe := func(data string) observedUpstreamEvent {
		return observeUpstreamMessage(state, []byte(data), start, func() time.Time { return now }, nil)
	}
	for _, id := range []string{"resp_1", "resp_2"} {
		state.setPendingTurnStartedAt(now)
		observe(`{"type":"response.created","response":{"id":"` + id + `"}}`)
		now = now.Add(time.Second)
		observe(`{"type":"response.output_text.delta","delta":" "}`)
		require.NotNil(t, state.activeTurn.firstTokenMs)
		require.Equal(t, 1000, *state.activeTurn.firstTokenMs)
		now = now.Add(time.Second)
		observe(`{"type":"response.output_text.delta","delta":"later"}`)
		terminal := observe(`{"type":"response.completed","response":{"id":"` + id + `","usage":{"input_tokens":2,"output_tokens":1}}}`)
		require.NotNil(t, terminal.firstToken)
		require.Equal(t, 1000, *terminal.firstToken)
	}
	require.Equal(t, 1000, *state.firstTokenMs)
	require.Equal(t, Usage{InputTokens: 4, OutputTokens: 2}, state.usage)
}

func TestObserveUpstreamMessageTTFTUsesResponseID(t *testing.T) {
	start := time.Unix(100, 0)
	now := start
	state := &relayState{}
	observe := func(data string) {
		observeUpstreamMessage(state, []byte(data), start, func() time.Time { return now }, nil)
	}
	state.setPendingTurnStartedAt(now)
	observe(`{"type":"response.created","response":{"id":"resp_old"}}`)
	now = now.Add(time.Second)
	state.setPendingTurnStartedAt(now)
	observe(`{"type":"response.created","response":{"id":"resp_current"}}`)
	current := state.activeTurn
	now = now.Add(time.Second)
	observe(`{"type":"response.output_text.delta","response_id":"resp_old","delta":"old output"}`)
	require.Nil(t, current.firstTokenMs, "old response must not start the active turn's TTFT")
	require.Equal(t, 2000, *state.turnTimingByID["resp_old"].firstTokenMs)
	now = now.Add(time.Second)
	observe(`{"type":"response.output_text.delta","response_id":"resp_current","delta":""}`)
	require.Nil(t, current.firstTokenMs)
	now = now.Add(time.Second)
	observe(`{"type":"response.output_text.delta","response_id":"resp_current","delta":"new output"}`)
	require.Equal(t, 3000, *current.firstTokenMs)
	now = now.Add(time.Second)
	observe(`{"type":"response.output_text.delta","response_id":"resp_old","delta":"later old output"}`)
	observe(`{"type":"response.output_text.delta","response_id":"resp_current","delta":"later new output"}`)
	require.Same(t, current, state.activeTurn)
	require.Equal(t, 3000, *current.firstTokenMs)
	require.Equal(t, 2000, *state.firstTokenMs)
}
