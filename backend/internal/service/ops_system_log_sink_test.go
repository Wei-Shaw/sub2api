package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
)

func TestOpsSystemLogSink_ShouldIndex(t *testing.T) {
	sink := &OpsSystemLogSink{REDACTED

	cases := []struct {
		name  string
		event *logger.LogEvent
		want  bool
REDACTED{
		{
			name:  "warn level",
			event: &logger.LogEvent{Level: "warn", Component: "app"REDACTED,
			want:  true,
	REDACTED,
		{
			name:  "error level",
			event: &logger.LogEvent{Level: "error", Component: "app"REDACTED,
			want:  true,
	REDACTED,
		{
			name:  "access component",
			event: &logger.LogEvent{Level: "info", Component: "http.access"REDACTED,
			want:  true,
	REDACTED,
		{
			name: "access component from fields (real zap path)",
			event: &logger.LogEvent{
				Level:     "info",
				Component: "",
				Fields:    map[string]any{"component": "http.access"REDACTED,
		REDACTED,
			want: true,
	REDACTED,
		{
			name:  "audit component",
			event: &logger.LogEvent{Level: "info", Component: "audit.log_config_change"REDACTED,
			want:  true,
	REDACTED,
		{
			name: "audit component from fields (real zap path)",
			event: &logger.LogEvent{
				Level:     "info",
				Component: "",
				Fields:    map[string]any{"component": "audit.log_config_change"REDACTED,
		REDACTED,
			want: true,
	REDACTED,
		{
			name:  "plain info",
			event: &logger.LogEvent{Level: "info", Component: "app"REDACTED,
			want:  false,
	REDACTED,
REDACTED

	for _, tc := range cases {
		if got := sink.shouldIndex(tc.event); got != tc.want {
			t.Fatalf("%s: shouldIndex()=%v, want %v", tc.name, got, tc.want)
	REDACTED
REDACTED
REDACTED

func TestOpsSystemLogSink_WriteLogEvent_ShouldDropWhenQueueFull(t *testing.T) {
	sink := &OpsSystemLogSink{
		queue: make(chan *logger.LogEvent, 1),
REDACTED

	sink.WriteLogEvent(&logger.LogEvent{Level: "warn", Component: "app"REDACTED)
	sink.WriteLogEvent(&logger.LogEvent{Level: "warn", Component: "app"REDACTED)

	if got := len(sink.queue); got != 1 {
		t.Fatalf("queue len = %d, want 1", got)
REDACTED
	if dropped := atomic.LoadUint64(&sink.droppedCount); dropped != 1 {
		t.Fatalf("droppedCount = %d, want 1", dropped)
REDACTED
REDACTED

func TestOpsSystemLogSink_Health(t *testing.T) {
	sink := &OpsSystemLogSink{
		queue: make(chan *logger.LogEvent, 10),
REDACTED
	sink.lastError.Store("db timeout")
	atomic.StoreUint64(&sink.droppedCount, 3)
	atomic.StoreUint64(&sink.writeFailed, 2)
	atomic.StoreUint64(&sink.writtenCount, 5)
	atomic.StoreUint64(&sink.totalDelayNs, uint64(5000000)) // 5ms total -> avg 1ms
	sink.queue <- &logger.LogEvent{Level: "warn", Component: "app"REDACTED
	sink.queue <- &logger.LogEvent{Level: "warn", Component: "app"REDACTED

	health := sink.Health()
	if health.QueueDepth != 2 {
		t.Fatalf("queue depth = %d, want 2", health.QueueDepth)
REDACTED
	if health.QueueCapacity != 10 {
		t.Fatalf("queue capacity = %d, want 10", health.QueueCapacity)
REDACTED
	if health.DroppedCount != 3 {
		t.Fatalf("dropped = %d, want 3", health.DroppedCount)
REDACTED
	if health.WriteFailed != 2 {
		t.Fatalf("write failed = %d, want 2", health.WriteFailed)
REDACTED
	if health.WrittenCount != 5 {
		t.Fatalf("written = %d, want 5", health.WrittenCount)
REDACTED
	if health.AvgWriteDelayMs != 1 {
		t.Fatalf("avg delay ms = %d, want 1", health.AvgWriteDelayMs)
REDACTED
	if health.LastError != "db timeout" {
		t.Fatalf("last error = %q, want db timeout", health.LastError)
REDACTED
REDACTED

func TestOpsSystemLogSink_StartStopAndFlushSuccess(t *testing.T) {
	done := make(chan struct{REDACTED, 1)
	var captured []*OpsInsertSystemLogInput
	repo := &opsRepoMock{
		BatchInsertSystemLogsFn: func(_ context.Context, inputs []*OpsInsertSystemLogInput) (int64, error) {
			captured = append(captured, inputs...)
			select {
			case done <- struct{REDACTED{REDACTED:
			default:
		REDACTED
			return int64(len(inputs)), nil
	REDACTED,
REDACTED

	sink := NewOpsSystemLogSink(repo)
	sink.batchSize = 1
	sink.flushInterval = 10 * time.Millisecond
	sink.Start()
	defer sink.Stop()

	sink.WriteLogEvent(&logger.LogEvent{
		Time:      time.Now().UTC(),
		Level:     "warn",
		Component: "http.access",
		Message:   `authorization="Bearer sk-test-123"`,
		Fields: map[string]any{
			"component":         "http.access",
			"request_id":        "req-1",
			"client_request_id": "creq-1",
			"user_id":           "12",
			"api_key_id":        int64(56),
			"account_id":        json.Number("34"),
			"platform":          "openai",
			"model":             "gpt-5",
	REDACTED,
REDACTED)

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatalf("timeout waiting for sink flush")
REDACTED

	if len(captured) != 1 {
		t.Fatalf("captured len = %d, want 1", len(captured))
REDACTED
	item := captured[0]
	if item.RequestID != "req-1" || item.ClientRequestID != "creq-1" {
		t.Fatalf("unexpected request ids: %+v", item)
REDACTED
	if item.UserID == nil || *item.UserID != 12 {
		t.Fatalf("unexpected user_id: %+v", item.UserID)
REDACTED
	if item.APIKeyID == nil || *item.APIKeyID != 56 {
		t.Fatalf("unexpected api_key_id: %+v", item.APIKeyID)
REDACTED
	if item.AccountID == nil || *item.AccountID != 34 {
		t.Fatalf("unexpected account_id: %+v", item.AccountID)
REDACTED
	if strings.TrimSpace(item.Message) == "" {
		t.Fatalf("message should not be empty")
REDACTED
	// writtenCount is incremented after BatchInsertSystemLogsFn returns,
	// so poll briefly to avoid a race between the done signal and the atomic add.
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if sink.Health().WrittenCount > 0 {
			break
	REDACTED
		time.Sleep(time.Millisecond)
REDACTED
	health := sink.Health()
	if health.WrittenCount == 0 {
		t.Fatalf("written_count should be >0")
REDACTED
REDACTED

func TestOpsSystemLogSink_FlushFailureUpdatesHealth(t *testing.T) {
	repo := &opsRepoMock{
		BatchInsertSystemLogsFn: func(_ context.Context, inputs []*OpsInsertSystemLogInput) (int64, error) {
			return 0, errors.New("db unavailable")
	REDACTED,
REDACTED
	sink := NewOpsSystemLogSink(repo)
	sink.batchSize = 1
	sink.flushInterval = 10 * time.Millisecond
	sink.Start()
	defer sink.Stop()

	sink.WriteLogEvent(&logger.LogEvent{
		Time:      time.Now().UTC(),
		Level:     "warn",
		Component: "app",
		Message:   "boom",
		Fields:    map[string]any{REDACTED,
REDACTED)

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		health := sink.Health()
		if health.WriteFailed > 0 {
			if !strings.Contains(health.LastError, "db unavailable") {
				t.Fatalf("unexpected last error: %s", health.LastError)
		REDACTED
			return
	REDACTED
		time.Sleep(20 * time.Millisecond)
REDACTED
	t.Fatalf("write_failed_count not updated")
REDACTED

func TestOpsSystemLogSink_StopFlushUsesActiveContextAndDrainsQueue(t *testing.T) {
	var inserted int64
	var canceledCtxCalls int64
	repo := &opsRepoMock{
		BatchInsertSystemLogsFn: func(ctx context.Context, inputs []*OpsInsertSystemLogInput) (int64, error) {
			if err := ctx.Err(); err != nil {
				atomic.AddInt64(&canceledCtxCalls, 1)
				return 0, err
		REDACTED
			atomic.AddInt64(&inserted, int64(len(inputs)))
			return int64(len(inputs)), nil
	REDACTED,
REDACTED

	sink := NewOpsSystemLogSink(repo)
	sink.batchSize = 200
	sink.flushInterval = time.Hour
	sink.Start()

	sink.WriteLogEvent(&logger.LogEvent{
		Time:      time.Now().UTC(),
		Level:     "warn",
		Component: "app",
		Message:   "pending-on-shutdown",
		Fields:    map[string]any{"component": "http.access"REDACTED,
REDACTED)

	sink.Stop()

	if got := atomic.LoadInt64(&inserted); got != 1 {
		t.Fatalf("inserted = %d, want 1", got)
REDACTED
	if got := atomic.LoadInt64(&canceledCtxCalls); got != 0 {
		t.Fatalf("canceled ctx calls = %d, want 0", got)
REDACTED
	health := sink.Health()
	if health.WrittenCount != 1 {
		t.Fatalf("written_count = %d, want 1", health.WrittenCount)
REDACTED
REDACTED

type stringerValue string

func (s stringerValue) String() string { return string(s) REDACTED

func TestOpsSystemLogSink_HelperFunctions(t *testing.T) {
	src := map[string]any{"a": 1REDACTED
	cloned := copyMap(src)
	src["a"] = 2
	v, ok := cloned["a"].(int)
	if !ok || v != 1 {
		t.Fatalf("copyMap should create copy")
REDACTED
	if got := asString(stringerValue(" hello ")); got != "hello" {
		t.Fatalf("asString stringer = %q", got)
REDACTED
	if got := asString(fmt.Errorf("x")); got != "" {
		t.Fatalf("asString error should be empty, got %q", got)
REDACTED
	if got := asString(123); got != "" {
		t.Fatalf("asString non-string should be empty, got %q", got)
REDACTED

	cases := []struct {
		in   any
		want int64
		ok   bool
REDACTED{
		{in: 5, want: 5, ok: trueREDACTED,
		{in: int64(6), want: 6, ok: trueREDACTED,
		{in: float64(7), want: 7, ok: trueREDACTED,
		{in: json.Number("8"), want: 8, ok: trueREDACTED,
		{in: "9", want: 9, ok: trueREDACTED,
		{in: "0", ok: falseREDACTED,
		{in: -1, ok: falseREDACTED,
		{in: "abc", ok: falseREDACTED,
REDACTED
	for _, tc := range cases {
		got := asInt64Ptr(tc.in)
		if tc.ok {
			if got == nil || *got != tc.want {
				t.Fatalf("asInt64Ptr(%v) = %+v, want %d", tc.in, got, tc.want)
		REDACTED
	REDACTED else if got != nil {
			t.Fatalf("asInt64Ptr(%v) should be nil, got %d", tc.in, *got)
	REDACTED
REDACTED
REDACTED
