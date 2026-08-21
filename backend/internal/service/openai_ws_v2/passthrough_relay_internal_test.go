package openai_ws_v2

import (
	"context"
	"errors"
	"io"
	"net"
	"sync/atomic"
	"testing"
	"time"

	coderws "github.com/coder/websocket"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestRunEntry_DelegatesRelay(t *testing.T) {
	t.Parallel()

	clientConn := newPassthroughTestFrameConn(nil, false)
	upstreamConn := newPassthroughTestFrameConn([]passthroughTestFrame{
		{
			msgType: coderws.MessageText,
			payload: []byte(`{"type":"response.completed","response":{"id":"resp_entry","usage":{"input_tokens":1,"output_tokens":1REDACTEDREDACTEDREDACTED`),
	REDACTED,
REDACTED, true)

	result, relayExit := RunEntry(EntryInput{
		Ctx:                context.Background(),
		ClientConn:         clientConn,
		UpstreamConn:       upstreamConn,
		FirstClientMessage: []byte(`{"type":"response.create","model":"gpt-4o","input":[]REDACTED`),
REDACTED)
	require.Nil(t, relayExit)
	require.Equal(t, "resp_entry", result.RequestID)
REDACTED

func TestRunClientToUpstream_ErrorPaths(t *testing.T) {
	t.Parallel()

	t.Run("read client eof", func(t *testing.T) {
		t.Parallel()

		exitCh := make(chan relayExitSignal, 1)
		runClientToUpstream(
			context.Background(),
			newPassthroughTestFrameConn(nil, true),
			nil,
			func(_ coderws.MessageType, _ []byte) error { return nil REDACTED,
			func() {REDACTED,
			nil,
			nil,
			exitCh,
		)
		sig := <-exitCh
		require.Equal(t, "read_client", sig.stage)
		require.True(t, sig.graceful)
REDACTED)

	t.Run("write upstream failed", func(t *testing.T) {
		t.Parallel()

		exitCh := make(chan relayExitSignal, 1)
		runClientToUpstream(
			context.Background(),
			newPassthroughTestFrameConn([]passthroughTestFrame{
				{msgType: coderws.MessageText, payload: []byte(`{"x":1REDACTED`)REDACTED,
		REDACTED, true),
			nil,
			func(_ coderws.MessageType, _ []byte) error { return errors.New("boom") REDACTED,
			func() {REDACTED,
			nil,
			nil,
			exitCh,
		)
		sig := <-exitCh
		require.Equal(t, "write_upstream", sig.stage)
		require.False(t, sig.graceful)
REDACTED)

	t.Run("forwarded counter and trace callback", func(t *testing.T) {
		t.Parallel()

		exitCh := make(chan relayExitSignal, 1)
		forwarded := &atomic.Int64{REDACTED
		traces := make([]RelayTraceEvent, 0, 2)
		runClientToUpstream(
			context.Background(),
			newPassthroughTestFrameConn([]passthroughTestFrame{
				{msgType: coderws.MessageText, payload: []byte(`{"x":1REDACTED`)REDACTED,
		REDACTED, true),
			nil,
			func(_ coderws.MessageType, _ []byte) error { return nil REDACTED,
			func() {REDACTED,
			forwarded,
			func(event RelayTraceEvent) {
				traces = append(traces, event)
		REDACTED,
			exitCh,
		)
		sig := <-exitCh
		require.Equal(t, "read_client", sig.stage)
		require.Equal(t, int64(1), forwarded.Load())
		require.NotEmpty(t, traces)
REDACTED)
REDACTED

func TestRunUpstreamToClient_ErrorAndDropPaths(t *testing.T) {
	t.Parallel()

	t.Run("read upstream eof", func(t *testing.T) {
		t.Parallel()

		exitCh := make(chan relayExitSignal, 1)
		drop := &atomic.Bool{REDACTED
		drop.Store(false)
		runUpstreamToClient(
			context.Background(),
			newPassthroughTestFrameConn(nil, true),
			func(_ coderws.MessageType, _ []byte) error { return nil REDACTED,
			time.Now(),
			time.Now,
			&relayState{REDACTED,
			nil,
			nil,
			nil,
			nil,
			nil,
			nil,
			drop,
			nil,
			nil,
			func() {REDACTED,
			nil,
			exitCh,
		)
		sig := <-exitCh
		require.Equal(t, "read_upstream", sig.stage)
		require.True(t, sig.graceful)
REDACTED)

	t.Run("write client failed", func(t *testing.T) {
		t.Parallel()

		exitCh := make(chan relayExitSignal, 1)
		drop := &atomic.Bool{REDACTED
		drop.Store(false)
		runUpstreamToClient(
			context.Background(),
			newPassthroughTestFrameConn([]passthroughTestFrame{
				{msgType: coderws.MessageText, payload: []byte(`{"type":"response.output_text.delta","delta":"x"REDACTED`)REDACTED,
		REDACTED, true),
			func(_ coderws.MessageType, _ []byte) error { return errors.New("write failed") REDACTED,
			time.Now(),
			time.Now,
			&relayState{REDACTED,
			nil,
			nil,
			nil,
			nil,
			nil,
			nil,
			drop,
			nil,
			nil,
			func() {REDACTED,
			nil,
			exitCh,
		)
		sig := <-exitCh
		require.Equal(t, "write_client", sig.stage)
REDACTED)

	t.Run("drop downstream and stop on terminal", func(t *testing.T) {
		t.Parallel()

		exitCh := make(chan relayExitSignal, 1)
		drop := &atomic.Bool{REDACTED
		drop.Store(true)
		dropped := &atomic.Int64{REDACTED
		runUpstreamToClient(
			context.Background(),
			newPassthroughTestFrameConn([]passthroughTestFrame{
				{
					msgType: coderws.MessageText,
					payload: []byte(`{"type":"response.completed","response":{"id":"resp_drop","usage":{"input_tokens":1,"output_tokens":1REDACTEDREDACTEDREDACTED`),
			REDACTED,
		REDACTED, true),
			func(_ coderws.MessageType, _ []byte) error { return nil REDACTED,
			time.Now(),
			time.Now,
			&relayState{REDACTED,
			nil,
			nil,
			nil,
			nil,
			nil,
			nil,
			drop,
			nil,
			dropped,
			func() {REDACTED,
			nil,
			exitCh,
		)
		sig := <-exitCh
		require.Equal(t, "drain_terminal", sig.stage)
		require.True(t, sig.graceful)
		require.Equal(t, int64(1), dropped.Load())
REDACTED)
REDACTED

func TestRunIdleWatchdog_NoTimeoutWhenDisabled(t *testing.T) {
	t.Parallel()

	exitCh := make(chan relayExitSignal, 1)
	lastActivity := &atomic.Int64{REDACTED
	lastActivity.Store(time.Now().UnixNano())
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go runIdleWatchdog(ctx, time.Now, 0, lastActivity, nil, exitCh)
	select {
	case <-exitCh:
		t.Fatal("unexpected idle timeout signal")
	case <-time.After(200 * time.Millisecond):
REDACTED
REDACTED

func TestHelperFunctionsCoverage(t *testing.T) {
	t.Parallel()

	require.Equal(t, "text", relayMessageTypeString(coderws.MessageText))
	require.Equal(t, "binary", relayMessageTypeString(coderws.MessageBinary))
	require.Contains(t, relayMessageTypeString(coderws.MessageType(99)), "unknown(")

	require.Equal(t, "", relayErrorString(nil))
	require.Equal(t, "x", relayErrorString(errors.New("x")))

	require.True(t, isDisconnectError(io.EOF))
	require.True(t, isDisconnectError(net.ErrClosed))
	require.True(t, isDisconnectError(context.Canceled))
	require.True(t, isDisconnectError(coderws.CloseError{Code: coderws.StatusGoingAwayREDACTED))
	require.True(t, isDisconnectError(errors.New("broken pipe")))
	require.False(t, isDisconnectError(errors.New("unrelated")))

	require.True(t, isTokenEvent("response.output_text.delta"))
	require.True(t, isTokenEvent("response.output_audio.delta"))
	require.False(t, isTokenEvent("response.completed"))
	require.False(t, isTokenEvent(""))
	require.False(t, isTokenEvent("response.created"))

	require.Equal(t, 2*time.Second, minDuration(2*time.Second, 5*time.Second))
	require.Equal(t, 2*time.Second, minDuration(5*time.Second, 2*time.Second))
	require.Equal(t, 5*time.Second, minDuration(0, 5*time.Second))
	require.Equal(t, 2*time.Second, minDuration(2*time.Second, 0))

	ch := make(chan relayExitSignal, 1)
	ch <- relayExitSignal{stage: "ok"REDACTED
	sig, ok := waitRelayExit(ch, 10*time.Millisecond)
	require.True(t, ok)
	require.Equal(t, "ok", sig.stage)
	ch <- relayExitSignal{stage: "ok2"REDACTED
	sig, ok = waitRelayExit(ch, 0)
	require.True(t, ok)
	require.Equal(t, "ok2", sig.stage)
	_, ok = waitRelayExit(ch, 10*time.Millisecond)
	require.False(t, ok)

	n, ok := parseUsageIntField(gjson.Get(`{"n":3REDACTED`, "n"), true)
	require.True(t, ok)
	require.Equal(t, 3, n)
	_, ok = parseUsageIntField(gjson.Get(`{"n":"x"REDACTED`, "n"), true)
	require.False(t, ok)
	n, ok = parseUsageIntField(gjson.Result{REDACTED, false)
	require.True(t, ok)
	require.Equal(t, 0, n)
	_, ok = parseUsageIntField(gjson.Result{REDACTED, true)
	require.False(t, ok)
REDACTED

func TestParseUsageAndEnrichCoverage(t *testing.T) {
	t.Parallel()

	state := &relayState{REDACTED
	parseUsageAndAccumulate(state, []byte(`{"type":"response.completed","response":{"usage":{"input_tokens":"bad"REDACTEDREDACTEDREDACTED`), "response.completed", nil)
	require.Equal(t, 0, state.usage.InputTokens)

	parseUsageAndAccumulate(
		state,
		[]byte(`{"type":"response.completed","response":{"usage":{"input_tokens":9,"output_tokens":"bad","input_tokens_details":{"cached_tokens":2REDACTEDREDACTEDREDACTEDREDACTED`),
		"response.completed",
		nil,
	)
	require.Equal(t, 0, state.usage.InputTokens, "部分字段解析失败时不应累加 usage")
	require.Equal(t, 0, state.usage.OutputTokens)
	require.Equal(t, 0, state.usage.CacheReadInputTokens)

	parseUsageAndAccumulate(
		state,
		[]byte(`{"type":"response.completed","response":{"usage":{"input_tokens_details":{"cached_tokens":2REDACTEDREDACTEDREDACTEDREDACTED`),
		"response.completed",
		nil,
	)
	require.Equal(t, 0, state.usage.InputTokens, "必填 usage 字段缺失时不应累加 usage")
	require.Equal(t, 0, state.usage.OutputTokens)
	require.Equal(t, 0, state.usage.CacheReadInputTokens)

	parseUsageAndAccumulate(state, []byte(`{"type":"response.completed","response":{"usage":{"input_tokens":2,"output_tokens":1,"input_tokens_details":{"cached_tokens":1,"cache_write_tokens":4REDACTED,"output_tokens_details":{"image_tokens":3REDACTEDREDACTEDREDACTEDREDACTED`), "response.completed", nil)
	finalizeRelayTurnUsage(state)
	require.Equal(t, 2, state.usage.InputTokens)
	require.Equal(t, 1, state.usage.OutputTokens)
	require.Equal(t, 1, state.usage.CacheReadInputTokens)
	require.Equal(t, 4, state.usage.CacheCreationInputTokens)
	require.Equal(t, 3, state.usage.ImageOutputTokens)

	result := &RelayResult{REDACTED
	enrichResult(result, state, 5*time.Millisecond)
	require.Equal(t, state.usage.InputTokens, result.Usage.InputTokens)
	require.Equal(t, state.usage.CacheCreationInputTokens, result.Usage.CacheCreationInputTokens)
	require.Equal(t, state.usage.ImageOutputTokens, result.Usage.ImageOutputTokens)
	require.Equal(t, 5*time.Millisecond, result.Duration)
	parseUsageAndAccumulate(state, []byte(`{"type":"response.in_progress","response":{"usage":{"input_tokens":9REDACTEDREDACTEDREDACTED`), "response.in_progress", nil)
	require.Equal(t, 2, state.usage.InputTokens)
	enrichResult(nil, state, 0)
REDACTED

func TestParseUsageAndAccumulateIncludesIndependentReasoningTokens(t *testing.T) {
	t.Parallel()

	state := &relayState{REDACTED
	got := parseUsageAndAccumulate(
		state,
		[]byte(`{"type":"response.completed","response":{"usage":{"input_tokens":32,"output_tokens":9,"total_tokens":151,"output_tokens_details":{"reasoning_tokens":110REDACTEDREDACTEDREDACTEDREDACTED`),
		"response.completed",
		nil,
	)
	require.Equal(t, 32, got.InputTokens)
	require.Equal(t, 119, got.OutputTokens)

	state = &relayState{REDACTED
	got = parseUsageAndAccumulate(
		state,
		[]byte(`{"type":"response.completed","response":{"usage":{"input_tokens":32,"output_tokens":119,"total_tokens":151,"output_tokens_details":{"reasoning_tokens":110REDACTEDREDACTEDREDACTEDREDACTED`),
		"response.completed",
		nil,
	)
	require.Equal(t, 119, got.OutputTokens, "inclusive Responses output must not double-count reasoning")
REDACTED

func TestParseUsageAndAccumulateAcceptsChatUsageAliases(t *testing.T) {
	t.Parallel()

	state := &relayState{REDACTED
	got := parseUsageAndAccumulate(
		state,
		[]byte(`{"type":"response.done","response":{"usage":{"prompt_tokens":12,"completion_tokens":6,"prompt_tokens_details":{"cached_tokens":4REDACTED,"completion_tokens_details":{"image_tokens":2REDACTEDREDACTEDREDACTEDREDACTED`),
		"response.done",
		nil,
	)
	finalizeRelayTurnUsage(state)
	require.Equal(t, 12, got.InputTokens)
	require.Equal(t, 6, got.OutputTokens)
	require.Equal(t, 4, got.CacheReadInputTokens)
	require.Equal(t, 2, got.ImageOutputTokens)
	require.Equal(t, got, state.usage)
REDACTED

func TestRelayUsageTerminalWithoutUsageKeepsFallback(t *testing.T) {
	t.Parallel()

	state := &relayState{REDACTED
	parseUsageAndAccumulate(state, []byte(`{"type":"response.in_progress","usage":{"input_tokens":9,"output_tokens":4REDACTEDREDACTED`), "response.in_progress", nil)
	parseUsageAndAccumulate(state, []byte(`{"type":"response.completed","response":{"id":"resp_1"REDACTEDREDACTED`), "response.completed", nil)

	turnUsage := finalizeRelayTurnUsage(state)
	require.Equal(t, Usage{InputTokens: 9, OutputTokens: 4REDACTED, turnUsage)
	require.Equal(t, turnUsage, state.usage)
REDACTED

func TestRelayUsageTerminalZeroKeepsFallback(t *testing.T) {
	t.Parallel()

	state := &relayState{REDACTED
	parseUsageAndAccumulate(state, []byte(`{"type":"response.in_progress","usage":{"input_tokens":9,"output_tokens":4REDACTEDREDACTED`), "response.in_progress", nil)
	parseUsageAndAccumulate(state, []byte(`{"type":"response.completed","usage":{"input_tokens":0,"output_tokens":0REDACTEDREDACTED`), "response.completed", nil)

	require.Equal(t, Usage{InputTokens: 9, OutputTokens: 4REDACTED, finalizeRelayTurnUsage(state))
	require.Equal(t, Usage{InputTokens: 9, OutputTokens: 4REDACTED, state.usage)
REDACTED

func TestRelayUsageTerminalNonZeroReplacesFallbackAsWhole(t *testing.T) {
	t.Parallel()

	state := &relayState{REDACTED
	parseUsageAndAccumulate(state, []byte(`{"type":"response.in_progress","usage":{"input_tokens":9,"output_tokens":4,"input_tokens_details":{"cached_tokens":2REDACTEDREDACTEDREDACTED`), "response.in_progress", nil)
	parseUsageAndAccumulate(state, []byte(`{"type":"response.completed","usage":{"input_tokens":3,"output_tokens":0,"input_tokens_details":{"cached_tokens":0REDACTEDREDACTEDREDACTED`), "response.completed", nil)

	require.Equal(t, Usage{InputTokens: 3REDACTED, finalizeRelayTurnUsage(state))
	require.Equal(t, Usage{InputTokens: 3REDACTED, state.usage)
REDACTED

func TestObserveUpstreamMessageBareErrorClearsTurnStateAndFinalizesUsageOnce(t *testing.T) {
	t.Parallel()

	now := time.Unix(200, 0)
	state := &relayState{REDACTED
	state.setPendingTurnStartedAt(now.Add(-2 * time.Second))
	timing := openAIWSRelayGetOrInitTurnTiming(state, "resp_active", now.Add(-time.Second))
	require.NotNil(t, timing)
	state.setPendingTurnStartedAt(now.Add(-500 * time.Millisecond))
	parseUsageAndAccumulate(state, []byte(`{"type":"response.in_progress","usage":{"input_tokens":9,"output_tokens":4REDACTEDREDACTED`), "response.in_progress", nil)

	observed := observeUpstreamMessage(
		state,
		[]byte(`{"type":"error","usage":{"input_tokens":3,"output_tokens":1REDACTED,"error":{"message":"failed"REDACTEDREDACTED`),
		now.Add(-3*time.Second),
		func() time.Time { return now REDACTED,
		nil,
	)

	require.False(t, observed.terminal, "bare error settlement is deferred in case response.failed follows")
	require.Equal(t, "error", observed.eventType)
	observed = finalizePendingBareError(state, now)
	require.True(t, observed.terminal)
	require.Equal(t, Usage{InputTokens: 3, OutputTokens: 1REDACTED, observed.usage)
	require.Equal(t, observed.usage, state.usage)
	require.Equal(t, Usage{REDACTED, state.turnUsage)
	require.Nil(t, state.activeTurn)
	require.Empty(t, state.turnTimingByID)
	// The error is bound to the active turn id, so settlement must not consume a
	// later pending start that belongs to the next turn.
	require.Equal(t, now.Add(-500*time.Millisecond), state.consumePendingTurnStartedAt())

	// Re-observing and settling a later terminal without usage must not re-add the prior turn.
	observeUpstreamMessage(state, []byte(`{"type":"error","error":{"message":"again"REDACTEDREDACTED`), now, func() time.Time { return now REDACTED, nil)
	finalizePendingBareError(state, now)
	require.Equal(t, Usage{InputTokens: 3, OutputTokens: 1REDACTED, state.usage)
REDACTED

func TestObserveUpstreamMessageErrorThenFailedSettlesUsageOnce(t *testing.T) {
	t.Parallel()

	now := time.Unix(300, 0)
	state := &relayState{REDACTED
	state.setPendingTurnStartedAt(now.Add(-time.Second))
	openAIWSRelayGetOrInitTurnTiming(state, "resp_1", now)
	parseUsageAndAccumulate(state, []byte(`{"type":"response.in_progress","usage":{"input_tokens":9,"output_tokens":4REDACTEDREDACTED`), "response.in_progress", nil)

	errorObserved := observeUpstreamMessage(
		state,
		[]byte(`{"type":"error","error":{"code":"server_is_overloaded","message":"overloaded"REDACTEDREDACTED`),
		now,
		func() time.Time { return now REDACTED,
		nil,
	)
	require.False(t, errorObserved.terminal)
	require.NotNil(t, state.pendingBareError)
	require.Equal(t, Usage{REDACTED, state.usage)

	failedObserved := observeUpstreamMessage(
		state,
		[]byte(`{"type":"response.failed","response":{"id":"resp_1","usage":{"input_tokens":7,"output_tokens":2REDACTEDREDACTEDREDACTED`),
		now,
		func() time.Time { return now REDACTED,
		nil,
	)
	require.True(t, failedObserved.terminal)
	require.Equal(t, Usage{InputTokens: 7, OutputTokens: 2REDACTED, failedObserved.usage)
	require.Equal(t, failedObserved.usage, state.usage)
	require.Nil(t, state.pendingBareError)
REDACTED

func TestObserveUpstreamMessageBareErrorBeforeNextCompletedKeepsBothTurns(t *testing.T) {
	t.Parallel()

	now := time.Unix(400, 0)
	state := &relayState{REDACTED
	parseUsageAndAccumulate(state, []byte(`{"type":"response.in_progress","usage":{"input_tokens":5,"output_tokens":1REDACTEDREDACTED`), "response.in_progress", nil)
	bare := observeUpstreamMessage(
		state,
		[]byte(`{"type":"error","error":{"message":"first turn failed"REDACTEDREDACTED`),
		now,
		func() time.Time { return now REDACTED,
		nil,
	)
	require.False(t, bare.terminal)
	bare = finalizePendingBareError(state, now)
	require.True(t, bare.terminal)
	require.Equal(t, Usage{InputTokens: 5, OutputTokens: 1REDACTED, bare.usage)

	completed := observeUpstreamMessage(
		state,
		[]byte(`{"type":"response.completed","response":{"id":"resp_next","usage":{"input_tokens":3,"output_tokens":2REDACTEDREDACTEDREDACTED`),
		now,
		func() time.Time { return now REDACTED,
		nil,
	)
	require.True(t, completed.terminal)
	require.Equal(t, Usage{InputTokens: 3, OutputTokens: 2REDACTED, completed.usage)
	require.Equal(t, Usage{InputTokens: 8, OutputTokens: 3REDACTED, state.usage)
REDACTED

func TestOpenAICacheCreationTokensFromUsageNestedZeroWins(t *testing.T) {
	t.Parallel()

	usage := gjson.Parse(`{"input_tokens_details":{"cache_write_tokens":0REDACTED,"cache_creation_input_tokens":19REDACTED`)
	require.Zero(t, openAICacheCreationTokensFromUsage(usage))
REDACTED

func TestEmitTurnCompleteCoverage(t *testing.T) {
	t.Parallel()

	// 非 terminal 事件不应触发。
	called := 0
	emitTurnComplete(func(turn RelayTurnResult) {
		called++
REDACTED, &relayState{requestModel: "gpt-5"REDACTED, observedUpstreamEvent{
		terminal:   false,
		eventType:  "response.output_text.delta",
		responseID: "resp_ignored",
		usage:      Usage{InputTokens: 1REDACTED,
REDACTED)
	require.Equal(t, 0, called)

	// 非 error 终态缺少 response_id 时不应触发。
	emitTurnComplete(func(turn RelayTurnResult) {
		called++
REDACTED, &relayState{requestModel: "gpt-5"REDACTED, observedUpstreamEvent{
		terminal:  true,
		eventType: "response.completed",
REDACTED)
	require.Equal(t, 0, called)

	// Bare error legitimately has no response_id; it must still settle the turn
	// or a later completed turn would suppress the adapter's aggregate fallback.
	var bareError RelayTurnResult
	emitTurnComplete(func(turn RelayTurnResult) {
		called++
		bareError = turn
REDACTED, &relayState{requestModel: "gpt-5"REDACTED, observedUpstreamEvent{
		terminal:  true,
		eventType: "error",
		usage:     Usage{InputTokens: 4, OutputTokens: 1REDACTED,
REDACTED)
	require.Equal(t, 1, called)
	require.Empty(t, bareError.RequestID)
	require.Equal(t, "error", bareError.TerminalEventType)
	require.Equal(t, Usage{InputTokens: 4, OutputTokens: 1REDACTED, bareError.Usage)

	// terminal 且 response_id 存在，应该触发；state=nil 时 model 为空串。
	var got RelayTurnResult
	emitTurnComplete(func(turn RelayTurnResult) {
		called++
		got = turn
REDACTED, nil, observedUpstreamEvent{
		terminal:   true,
		eventType:  "response.completed",
		responseID: "resp_emit",
		usage:      Usage{InputTokens: 2, OutputTokens: 3REDACTED,
REDACTED)
	require.Equal(t, 2, called)
	require.Equal(t, "resp_emit", got.RequestID)
	require.Equal(t, "response.completed", got.TerminalEventType)
	require.Equal(t, 2, got.Usage.InputTokens)
	require.Equal(t, 3, got.Usage.OutputTokens)
	require.Equal(t, "", got.RequestModel)
REDACTED

func TestIsDisconnectErrorCoverage_CloseStatusesAndMessageBranches(t *testing.T) {
	t.Parallel()

	require.True(t, isDisconnectError(coderws.CloseError{Code: coderws.StatusNormalClosureREDACTED))
	require.True(t, isDisconnectError(coderws.CloseError{Code: coderws.StatusNoStatusRcvdREDACTED))
	require.True(t, isDisconnectError(coderws.CloseError{Code: coderws.StatusAbnormalClosureREDACTED))
	require.True(t, isDisconnectError(errors.New("connection reset by peer")))
	require.False(t, isDisconnectError(errors.New("   ")))
REDACTED

func TestIsTokenEventCoverageBranches(t *testing.T) {
	t.Parallel()

	require.False(t, isTokenEvent("response.in_progress"))
	require.False(t, isTokenEvent("response.output_item.added"))
	require.True(t, isTokenEvent("response.output_audio.delta"))
	require.True(t, isTokenEvent("response.function_call_arguments.delta"))
	require.True(t, isTokenEvent("response.reasoning_summary_text.delta"))
	require.True(t, isTokenEvent("response.output_text.done"))
	require.True(t, isTokenEvent("response.function_call_arguments.done"))
	require.False(t, isTokenEvent("response.output"))
	require.False(t, isTokenEvent("response.output_audio.done"))
	require.False(t, isTokenEvent("response.content_part.done"))
	require.False(t, isTokenEvent("response.output_item.done"))
	require.False(t, isTokenEvent("response.output_text.annotation.added"))
	require.False(t, isTokenEvent("response.done"))
REDACTED

func TestTerminalAndTokenEventSetsAreDisjoint(t *testing.T) {
	t.Parallel()

	for _, eventType := range []string{
		"response.completed",
		"response.done",
		"response.failed",
		"response.incomplete",
		"response.cancelled",
		"response.canceled",
REDACTED {
		require.True(t, isTerminalEvent(eventType), eventType)
		require.False(t, isTokenEvent(eventType), eventType)
REDACTED
REDACTED

func TestShouldParseUsageTerminalEvents(t *testing.T) {
	t.Parallel()

	for _, eventType := range []string{
		"response.completed",
		"response.done",
		"response.failed",
		"response.incomplete",
		"response.cancelled",
		"response.canceled",
REDACTED {
		require.True(t, shouldParseUsage(eventType), eventType)
REDACTED
	require.False(t, shouldParseUsage("response.output_text.delta"))
	require.False(t, shouldParseUsage(""))
REDACTED

func TestRelayTurnTimingHelpersCoverage(t *testing.T) {
	t.Parallel()

	now := time.Unix(100, 0)
	// nil state
	require.Nil(t, openAIWSRelayGetOrInitTurnTiming(nil, "resp_nil", now))
	_, ok := openAIWSRelayDeleteTurnTiming(nil, "resp_nil")
	require.False(t, ok)

	state := &relayState{REDACTED
	timing := openAIWSRelayGetOrInitTurnTiming(state, "resp_a", now)
	require.NotNil(t, timing)
	require.Equal(t, now, timing.startAt)

	// 再次获取返回同一条 timing
	timing2 := openAIWSRelayGetOrInitTurnTiming(state, "resp_a", now.Add(5*time.Second))
	require.NotNil(t, timing2)
	require.Equal(t, now, timing2.startAt)

	// 删除存在键
	deleted, ok := openAIWSRelayDeleteTurnTiming(state, "resp_a")
	require.True(t, ok)
	require.Equal(t, now, deleted.startAt)

	// 删除不存在键
	_, ok = openAIWSRelayDeleteTurnTiming(state, "resp_a")
	require.False(t, ok)
REDACTED

func TestObserveUpstreamMessage_ResponseModelIsTurnLocalAndTerminalWins(t *testing.T) {
	t.Parallel()

	state := &relayState{requestModel: "gpt-5.6-sol"REDACTED
	startAt := time.Unix(0, 0)
	now := startAt
	nowFn := func() time.Time {
		now = now.Add(5 * time.Millisecond)
		return now
REDACTED

	created := observeUpstreamMessage(
		state,
		[]byte(`{"type":"response.created","response":{"id":"resp_1","model":"gpt-5.5"REDACTEDREDACTED`),
		startAt,
		nowFn,
		nil,
	)
	require.False(t, created.terminal)

	completed := observeUpstreamMessage(
		state,
		[]byte(`{"type":"response.completed","response":{"id":"resp_1","model":"gpt-5.4","usage":{"input_tokens":1,"output_tokens":2REDACTEDREDACTEDREDACTED`),
		startAt,
		nowFn,
		nil,
	)
	require.True(t, completed.terminal)
	require.Equal(t, "gpt-5.4", completed.responseModel)
	require.True(t, completed.responseConflict)

	var firstTurn RelayTurnResult
	emitTurnComplete(func(turn RelayTurnResult) { firstTurn = turn REDACTED, state, completed)
	require.Equal(t, "gpt-5.4", firstTurn.ResponseModel)
	require.True(t, firstTurn.ResponseModelConflict)

	observeUpstreamMessage(
		state,
		[]byte(`{"type":"response.created","response":{"id":"resp_2","model":"gpt-5.3"REDACTEDREDACTED`),
		startAt,
		nowFn,
		nil,
	)
	second := observeUpstreamMessage(
		state,
		[]byte(`{"type":"response.completed","response":{"id":"resp_2","model":"GPT-5.3","usage":{"input_tokens":3,"output_tokens":4REDACTEDREDACTEDREDACTED`),
		startAt,
		nowFn,
		nil,
	)
	require.Equal(t, "GPT-5.3", second.responseModel)
	require.False(t, second.responseConflict, "the previous turn must not contaminate this turn")
REDACTED

func TestObserveUpstreamMessage_ResponseIDFallbackPolicy(t *testing.T) {
	t.Parallel()

	state := &relayState{requestModel: "gpt-5"REDACTED
	startAt := time.Unix(0, 0)
	now := startAt
	nowFn := func() time.Time {
		now = now.Add(5 * time.Millisecond)
		return now
REDACTED

	// 非 terminal：仅有顶层 id，不应把 event id 当成 response_id。
	observed := observeUpstreamMessage(
		state,
		[]byte(`{"type":"response.output_text.delta","id":"evt_123","delta":"hi"REDACTED`),
		startAt,
		nowFn,
		nil,
	)
	require.False(t, observed.terminal)
	require.Equal(t, "", observed.responseID)

	// terminal：允许兜底用顶层 id（用于兼容少数字段变体）。
	observed = observeUpstreamMessage(
		state,
		[]byte(`{"type":"response.completed","id":"resp_fallback","response":{"usage":{"input_tokens":1,"output_tokens":1REDACTEDREDACTEDREDACTED`),
		startAt,
		nowFn,
		nil,
	)
	require.True(t, observed.terminal)
	require.Equal(t, "resp_fallback", observed.responseID)
REDACTED
