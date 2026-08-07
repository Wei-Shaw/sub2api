package handler

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func newUsageRecordTestPool(t *testing.T) *service.UsageRecordWorkerPool {
REDACTED
	pool := service.NewUsageRecordWorkerPoolWithOptions(service.UsageRecordWorkerPoolOptions{
		WorkerCount:           1,
		QueueSize:             8,
		TaskTimeout:           time.Second,
		OverflowPolicy:        "drop",
		OverflowSamplePercent: 0,
		AutoScaleEnabled:      false,
REDACTED)
	t.Cleanup(pool.Stop)
	return pool
REDACTED

func TestGatewayHandlerSubmitUsageRecordTask_WithPool(t *testing.T) {
	pool := newUsageRecordTestPool(t)
	h := &GatewayHandler{usageRecordWorkerPool: poolREDACTED

	done := make(chan struct{REDACTED)
	h.submitUsageRecordTask(context.Background(), func(ctx context.Context) {
		close(done)
REDACTED)

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("task not executed")
REDACTED
REDACTED

func TestGatewayHandlerSubmitUsageRecordTask_WithoutPoolSyncFallback(t *testing.T) {
	h := &GatewayHandler{REDACTED
	var called atomic.Bool

	h.submitUsageRecordTask(context.Background(), func(ctx context.Context) {
		if _, ok := ctx.Deadline(); !ok {
			t.Fatal("expected deadline in fallback context")
	REDACTED
		called.Store(true)
REDACTED)

	require.True(t, called.Load())
REDACTED

func TestGatewayHandlerSubmitUsageRecordTask_NilTask(t *testing.T) {
	h := &GatewayHandler{REDACTED
	require.NotPanics(t, func() {
		h.submitUsageRecordTask(context.Background(), nil)
REDACTED)
REDACTED

func TestGatewayHandlerSubmitUsageRecordTask_WithoutPool_TaskPanicRecovered(t *testing.T) {
	h := &GatewayHandler{REDACTED
	var called atomic.Bool

	require.NotPanics(t, func() {
		h.submitUsageRecordTask(context.Background(), func(ctx context.Context) {
			panic("usage task panic")
	REDACTED)
REDACTED)

	h.submitUsageRecordTask(context.Background(), func(ctx context.Context) {
		called.Store(true)
REDACTED)
	require.True(t, called.Load(), "panic 后后续任务应仍可执行")
REDACTED

func TestOpenAIGatewayHandlerSubmitUsageRecordTask_WithPool(t *testing.T) {
	pool := newUsageRecordTestPool(t)
	h := &OpenAIGatewayHandler{usageRecordWorkerPool: poolREDACTED

	done := make(chan struct{REDACTED)
	h.submitUsageRecordTask(context.Background(), func(ctx context.Context) {
		close(done)
REDACTED)

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("task not executed")
REDACTED
REDACTED

func TestOpenAIGatewayHandlerSubmitUsageRecordTask_WithoutPoolSyncFallback(t *testing.T) {
	h := &OpenAIGatewayHandler{REDACTED
	var called atomic.Bool

	h.submitUsageRecordTask(context.Background(), func(ctx context.Context) {
		if _, ok := ctx.Deadline(); !ok {
			t.Fatal("expected deadline in fallback context")
	REDACTED
		called.Store(true)
REDACTED)

	require.True(t, called.Load())
REDACTED

func TestOpenAIGatewayHandlerSubmitUsageRecordTask_NilTask(t *testing.T) {
	h := &OpenAIGatewayHandler{REDACTED
	require.NotPanics(t, func() {
		h.submitUsageRecordTask(context.Background(), nil)
REDACTED)
REDACTED

func TestOpenAIGatewayHandlerSubmitUsageRecordTask_WithoutPool_TaskPanicRecovered(t *testing.T) {
	h := &OpenAIGatewayHandler{REDACTED
	var called atomic.Bool

	require.NotPanics(t, func() {
		h.submitUsageRecordTask(context.Background(), func(ctx context.Context) {
			panic("usage task panic")
	REDACTED)
REDACTED)

	h.submitUsageRecordTask(context.Background(), func(ctx context.Context) {
		called.Store(true)
REDACTED)
	require.True(t, called.Load(), "panic 后后续任务应仍可执行")
REDACTED

func TestOpenAIGatewayHandlerSubmitMandatoryUsageRecordTask_DroppedTaskSyncFallback(t *testing.T) {
	pool := service.NewUsageRecordWorkerPoolWithOptions(service.UsageRecordWorkerPoolOptions{
		WorkerCount:           1,
		QueueSize:             1,
		TaskTimeout:           time.Second,
		OverflowPolicy:        "drop",
		OverflowSamplePercent: 0,
		AutoScaleEnabled:      false,
REDACTED)
	t.Cleanup(pool.Stop)
	h := &OpenAIGatewayHandler{usageRecordWorkerPool: poolREDACTED

	block := make(chan struct{REDACTED)
	release := make(chan struct{REDACTED)
	pool.Submit(func(ctx context.Context) {
		close(block)
		<-release
REDACTED)
	<-block
	pool.Submit(func(ctx context.Context) {REDACTED)

	var called atomic.Bool
	h.submitMandatoryUsageRecordTask(context.Background(), func(ctx context.Context) {
		called.Store(true)
REDACTED)
	close(release)

	require.True(t, called.Load(), "mandatory usage task must run synchronously when async submit is dropped")
REDACTED

func TestOpenAIGatewayHandlerSubmitOpenAIUsageRecordTask_ImageResultUsesMandatoryFallback(t *testing.T) {
	pool := service.NewUsageRecordWorkerPoolWithOptions(service.UsageRecordWorkerPoolOptions{
		WorkerCount:           1,
		QueueSize:             1,
		TaskTimeout:           time.Second,
		OverflowPolicy:        "drop",
		OverflowSamplePercent: 0,
		AutoScaleEnabled:      false,
REDACTED)
	t.Cleanup(pool.Stop)
	h := &OpenAIGatewayHandler{usageRecordWorkerPool: poolREDACTED

	block := make(chan struct{REDACTED)
	release := make(chan struct{REDACTED)
	pool.Submit(func(ctx context.Context) {
		close(block)
		<-release
REDACTED)
	<-block
	pool.Submit(func(ctx context.Context) {REDACTED)

	var called atomic.Bool
	h.submitOpenAIUsageRecordTask(context.Background(), &service.OpenAIForwardResult{ImageCount: 1REDACTED, func(ctx context.Context) {
		called.Store(true)
REDACTED)
	close(release)

	require.True(t, called.Load(), "image usage task must be mandatory when async submit is dropped")
REDACTED

func TestOpenAIGatewayHandlerSubmitOpenAIUsageRecordTask_SearchCountUsesMandatoryFallback(t *testing.T) {
	pool := service.NewUsageRecordWorkerPoolWithOptions(service.UsageRecordWorkerPoolOptions{
		WorkerCount:           1,
		QueueSize:             1,
		TaskTimeout:           time.Second,
		OverflowPolicy:        "drop",
		OverflowSamplePercent: 0,
		AutoScaleEnabled:      false,
REDACTED)
	t.Cleanup(pool.Stop)
	h := &OpenAIGatewayHandler{usageRecordWorkerPool: poolREDACTED

	block := make(chan struct{REDACTED)
	release := make(chan struct{REDACTED)
	pool.Submit(func(ctx context.Context) {
		close(block)
		<-release
REDACTED)
	<-block
	pool.Submit(func(ctx context.Context) {REDACTED)

	var called atomic.Bool
	h.submitOpenAIUsageRecordTask(context.Background(), &service.OpenAIForwardResult{SearchCount: 3REDACTED, func(ctx context.Context) {
		called.Store(true)
REDACTED)
	close(release)

	require.True(t, called.Load(), "search surcharge usage task must be mandatory when async submit is dropped")
REDACTED
