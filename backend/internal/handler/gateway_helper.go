package handler

import (
	"context"
	"fmt"
	"math/rand"
	"net/http"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// 并发槽位等待相关常量
//
// 性能优化说明：
// 原实现使用固定间隔（100ms）轮询并发槽位，存在以下问题：
// 1. 高并发时频繁轮询增加 Redis 压力
// 2. 固定间隔可能导致多个请求同时重试（惊群效应）
//
// 新实现使用指数退避 + 抖动算法：
// 1. 初始退避 100ms，每次乘以 1.5，最大 2s
// 2. 添加 ±20% 的随机抖动，分散重试时间点
// 3. 减少 Redis 压力，避免惊群效应
const (
	// maxConcurrencyWait 等待并发槽位的最大时间
	maxConcurrencyWait = 30 * time.Second
	// pingInterval 流式响应等待时发送 ping 的间隔
	pingInterval = 15 * time.Second
	// initialBackoff 初始退避时间
	initialBackoff = 100 * time.Millisecond
	// backoffMultiplier 退避时间乘数（指数退避）
	backoffMultiplier = 1.5
	// maxBackoff 最大退避时间
	maxBackoff = 2 * time.Second
)

// SSEPingFormat defines the format of SSE ping events for different platforms
type SSEPingFormat string

const (
	// SSEPingFormatClaude is the Claude/Anthropic SSE ping format
	SSEPingFormatClaude SSEPingFormat = "data: {\"type\": \"ping\"REDACTED\n\n"
	// SSEPingFormatNone indicates no ping should be sent (e.g., OpenAI has no ping spec)
	SSEPingFormatNone SSEPingFormat = ""
)

// ConcurrencyError represents a concurrency limit error with context
type ConcurrencyError struct {
	SlotType  string
	IsTimeout bool
REDACTED

func (e *ConcurrencyError) Error() string {
	if e.IsTimeout {
		return fmt.Sprintf("timeout waiting for %s concurrency slot", e.SlotType)
REDACTED
	return fmt.Sprintf("%s concurrency limit reached", e.SlotType)
REDACTED

// ConcurrencyHelper provides common concurrency slot management for gateway handlers
type ConcurrencyHelper struct {
	concurrencyService *service.ConcurrencyService
	pingFormat         SSEPingFormat
REDACTED

// NewConcurrencyHelper creates a new ConcurrencyHelper
func NewConcurrencyHelper(concurrencyService *service.ConcurrencyService, pingFormat SSEPingFormat) *ConcurrencyHelper {
	return &ConcurrencyHelper{
		concurrencyService: concurrencyService,
		pingFormat:         pingFormat,
REDACTED
REDACTED

// IncrementWaitCount increments the wait count for a user
func (h *ConcurrencyHelper) IncrementWaitCount(ctx context.Context, userID int64, maxWait int) (bool, error) {
	return h.concurrencyService.IncrementWaitCount(ctx, userID, maxWait)
REDACTED

// DecrementWaitCount decrements the wait count for a user
func (h *ConcurrencyHelper) DecrementWaitCount(ctx context.Context, userID int64) {
	h.concurrencyService.DecrementWaitCount(ctx, userID)
REDACTED

// IncrementAccountWaitCount increments the wait count for an account
func (h *ConcurrencyHelper) IncrementAccountWaitCount(ctx context.Context, accountID int64, maxWait int) (bool, error) {
	return h.concurrencyService.IncrementAccountWaitCount(ctx, accountID, maxWait)
REDACTED

// DecrementAccountWaitCount decrements the wait count for an account
func (h *ConcurrencyHelper) DecrementAccountWaitCount(ctx context.Context, accountID int64) {
	h.concurrencyService.DecrementAccountWaitCount(ctx, accountID)
REDACTED

// AcquireUserSlotWithWait acquires a user concurrency slot, waiting if necessary.
// For streaming requests, sends ping events during the wait.
// streamStarted is updated if streaming response has begun.
func (h *ConcurrencyHelper) AcquireUserSlotWithWait(c *gin.Context, userID int64, maxConcurrency int, isStream bool, streamStarted *bool) (func(), error) {
	ctx := c.Request.Context()

	// Try to acquire immediately
	result, err := h.concurrencyService.AcquireUserSlot(ctx, userID, maxConcurrency)
	if err != nil {
		return nil, err
REDACTED

	if result.Acquired {
		return result.ReleaseFunc, nil
REDACTED

	// Need to wait - handle streaming ping if needed
	return h.waitForSlotWithPing(c, "user", userID, maxConcurrency, isStream, streamStarted)
REDACTED

// AcquireAccountSlotWithWait acquires an account concurrency slot, waiting if necessary.
// For streaming requests, sends ping events during the wait.
// streamStarted is updated if streaming response has begun.
func (h *ConcurrencyHelper) AcquireAccountSlotWithWait(c *gin.Context, accountID int64, maxConcurrency int, isStream bool, streamStarted *bool) (func(), error) {
	ctx := c.Request.Context()

	// Try to acquire immediately
	result, err := h.concurrencyService.AcquireAccountSlot(ctx, accountID, maxConcurrency)
	if err != nil {
		return nil, err
REDACTED

	if result.Acquired {
		return result.ReleaseFunc, nil
REDACTED

	// Need to wait - handle streaming ping if needed
	return h.waitForSlotWithPing(c, "account", accountID, maxConcurrency, isStream, streamStarted)
REDACTED

// waitForSlotWithPing waits for a concurrency slot, sending ping events for streaming requests.
// streamStarted pointer is updated when streaming begins (for proper error handling by caller).
func (h *ConcurrencyHelper) waitForSlotWithPing(c *gin.Context, slotType string, id int64, maxConcurrency int, isStream bool, streamStarted *bool) (func(), error) {
	return h.waitForSlotWithPingTimeout(c, slotType, id, maxConcurrency, maxConcurrencyWait, isStream, streamStarted)
REDACTED

// waitForSlotWithPingTimeout waits for a concurrency slot with a custom timeout.
func (h *ConcurrencyHelper) waitForSlotWithPingTimeout(c *gin.Context, slotType string, id int64, maxConcurrency int, timeout time.Duration, isStream bool, streamStarted *bool) (func(), error) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), timeout)
	defer cancel()

	// Determine if ping is needed (streaming + ping format defined)
	needPing := isStream && h.pingFormat != ""

	var flusher http.Flusher
	if needPing {
		var ok bool
		flusher, ok = c.Writer.(http.Flusher)
		if !ok {
			return nil, fmt.Errorf("streaming not supported")
	REDACTED
REDACTED

	// Only create ping ticker if ping is needed
	var pingCh <-chan time.Time
	if needPing {
		pingTicker := time.NewTicker(pingInterval)
		defer pingTicker.Stop()
		pingCh = pingTicker.C
REDACTED

	backoff := initialBackoff
	timer := time.NewTimer(backoff)
	defer timer.Stop()
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))

	for {
		select {
		case <-ctx.Done():
			return nil, &ConcurrencyError{
				SlotType:  slotType,
				IsTimeout: true,
		REDACTED

		case <-pingCh:
			// Send ping to keep connection alive
			if !*streamStarted {
				c.Header("Content-Type", "text/event-stream")
				c.Header("Cache-Control", "no-cache")
				c.Header("Connection", "keep-alive")
				c.Header("X-Accel-Buffering", "no")
				*streamStarted = true
		REDACTED
			if _, err := fmt.Fprint(c.Writer, string(h.pingFormat)); err != nil {
				return nil, err
		REDACTED
			flusher.Flush()

		case <-timer.C:
			// Try to acquire slot
			var result *service.AcquireResult
			var err error

			if slotType == "user" {
				result, err = h.concurrencyService.AcquireUserSlot(ctx, id, maxConcurrency)
		REDACTED else {
				result, err = h.concurrencyService.AcquireAccountSlot(ctx, id, maxConcurrency)
		REDACTED

			if err != nil {
				return nil, err
		REDACTED

			if result.Acquired {
				return result.ReleaseFunc, nil
		REDACTED
			backoff = nextBackoff(backoff, rng)
			timer.Reset(backoff)
	REDACTED
REDACTED
REDACTED

// AcquireAccountSlotWithWaitTimeout acquires an account slot with a custom timeout (keeps SSE ping).
func (h *ConcurrencyHelper) AcquireAccountSlotWithWaitTimeout(c *gin.Context, accountID int64, maxConcurrency int, timeout time.Duration, isStream bool, streamStarted *bool) (func(), error) {
	return h.waitForSlotWithPingTimeout(c, "account", accountID, maxConcurrency, timeout, isStream, streamStarted)
REDACTED

// nextBackoff 计算下一次退避时间
// 性能优化：使用指数退避 + 随机抖动，避免惊群效应
// current: 当前退避时间
// rng: 随机数生成器（可为 nil，此时不添加抖动）
// 返回值：下一次退避时间（100ms ~ 2s 之间）
func nextBackoff(current time.Duration, rng *rand.Rand) time.Duration {
	// 指数退避：当前时间 * 1.5
	next := time.Duration(float64(current) * backoffMultiplier)
	if next > maxBackoff {
		next = maxBackoff
REDACTED
	if rng == nil {
		return next
REDACTED
	// 添加 ±20% 的随机抖动（jitter 范围 0.8 ~ 1.2）
	// 抖动可以分散多个请求的重试时间点，避免同时冲击 Redis
	jitter := 0.8 + rng.Float64()*0.4
	jittered := time.Duration(float64(next) * jitter)
	if jittered < initialBackoff {
		return initialBackoff
REDACTED
	if jittered > maxBackoff {
		return maxBackoff
REDACTED
	return jittered
REDACTED
