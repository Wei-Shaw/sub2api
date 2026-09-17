package handler

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

func newSlotWaitObservedLogger() (*zap.Logger, *observer.ObservedLogs) {
	core, logs := observer.New(zap.InfoLevel)
	return zap.New(core), logs
}

func slotWaitLoggedFields(t *testing.T, logs *observer.ObservedLogs) map[string]any {
	t.Helper()
	entries := logs.All()
	require.Len(t, entries, 1)
	fields := map[string]any{}
	for _, f := range entries[0].Context {
		fields[f.Key] = f.Integer
	}
	return fields
}

func TestLogOpenAIWSSlotWaited_SkipsImmediateAcquire(t *testing.T) {
	logger, logs := newSlotWaitObservedLogger()

	// 立即命中时耗时在微秒级，不该产生日志，否则每个 turn 都会刷一条。
	logOpenAIWSSlotWaited(logger, "openai.websocket_turn_account_slot_waited", 3, 77, 200*time.Microsecond)
	require.Equal(t, 0, logs.Len())

	// 阈值边界下方同样不记录。
	logOpenAIWSSlotWaited(logger, "openai.websocket_turn_account_slot_waited", 3, 77, openAIWSSlotWaitLogThreshold-time.Millisecond)
	require.Equal(t, 0, logs.Len())
}

func TestLogOpenAIWSSlotWaited_RecordsRealWait(t *testing.T) {
	logger, logs := newSlotWaitObservedLogger()

	logOpenAIWSSlotWaited(logger, "openai.websocket_turn_account_slot_waited", 3, 77, 1500*time.Millisecond)

	entries := logs.All()
	require.Len(t, entries, 1)
	require.Equal(t, "openai.websocket_turn_account_slot_waited", entries[0].Message)
	require.Equal(t, zap.InfoLevel, entries[0].Level)

	fields := slotWaitLoggedFields(t, logs)
	require.Equal(t, int64(77), fields["account_id"])
	require.Equal(t, int64(3), fields["turn"])
	// 等待时长必须落进日志，否则无法校准 turn_slot_wait_timeout_seconds。
	require.Equal(t, int64(1500*time.Millisecond), fields["waited"])
}

func TestLogOpenAIWSSlotWaited_FirstTurnOmitsTurnField(t *testing.T) {
	logger, logs := newSlotWaitObservedLogger()

	// turn==0 表示握手期准入，没有轮次可言，不应写出误导性的 turn=0。
	logOpenAIWSSlotWaited(logger, "openai.websocket_account_slot_waited", 0, 74, time.Second)

	fields := slotWaitLoggedFields(t, logs)
	require.Equal(t, int64(74), fields["account_id"])
	_, hasTurn := fields["turn"]
	require.False(t, hasTurn)
}

func TestLogOpenAIWSSlotWaited_NilLoggerIsSafe(t *testing.T) {
	// reqLog 在部分早退路径上可能尚未装配，这里不能 panic。
	require.NotPanics(t, func() {
		logOpenAIWSSlotWaited(nil, "openai.websocket_turn_account_slot_waited", 1, 77, time.Minute)
	})
}
