package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand/v2"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

const gatewayStreamHeartbeatBytesKey = "gateway_stream_heartbeat_bytes"

// requireAPIKeyQueueCapability records a group-gated capability that this
// request actually uses, so a queued key wait re-checks exactly that capability
// and ignores unrelated group permission changes.
func requireAPIKeyQueueCapability(c *gin.Context, capability service.APIKeyQueueCapability) {
	if c == nil || c.Request == nil || capability == 0 {
		return
	}
	c.Request = c.Request.WithContext(service.WithAPIKeyQueueCapability(c.Request.Context(), capability))
}

func recordGatewayStreamHeartbeat(c *gin.Context, written int) {
	if c == nil || written <= 0 {
		return
	}
	total, _ := c.Get(gatewayStreamHeartbeatBytesKey)
	bytes, _ := total.(int)
	c.Set(gatewayStreamHeartbeatBytesKey, bytes+written)
}

func gatewayStreamHasOnlyHeartbeats(c *gin.Context) bool {
	if c == nil || c.Writer == nil {
		return false
	}
	value, ok := c.Get(gatewayStreamHeartbeatBytesKey)
	if !ok {
		return false
	}
	heartbeatBytes, _ := value.(int)
	return heartbeatBytes > 0 && c.Writer.Size() == heartbeatBytes
}

// claudeCodeValidator is a singleton validator for Claude Code client detection
var claudeCodeValidator = service.NewClaudeCodeValidator()

// SetClaudeCodeClientContext 检查请求是否来自 Claude Code 客户端，并设置到 context 中
// 返回更新后的 context
func SetClaudeCodeClientContext(c *gin.Context, body []byte, parsedReq *service.ParsedRequest) {
	if c == nil || c.Request == nil {
		return
	}
	ua := c.GetHeader("User-Agent")
	// Fast path：非 Claude CLI UA 直接判定 false，避免热路径二次 JSON 反序列化。
	if !claudeCodeValidator.ValidateUserAgent(ua) {
		ctx := service.SetClaudeCodeClient(c.Request.Context(), false)
		c.Request = c.Request.WithContext(ctx)
		return
	}

	isClaudeCode := false
	if !strings.Contains(c.Request.URL.Path, "messages") {
		// 与 Validate 行为一致：非 messages 路径 UA 命中即可视为 Claude Code 客户端。
		isClaudeCode = true
	} else {
		// 仅在确认为 Claude CLI 且 messages 路径时再做 body 解析。
		bodyMap := claudeCodeBodyMapFromParsedRequest(parsedReq)
		if bodyMap == nil && len(body) > 0 {
			_ = json.Unmarshal(body, &bodyMap)
		}
		isClaudeCode = claudeCodeValidator.Validate(c.Request, bodyMap)
	}

	// 更新 request context
	ctx := service.SetClaudeCodeClient(c.Request.Context(), isClaudeCode)

	// 仅在确认为 Claude Code 客户端时提取版本号写入 context
	if isClaudeCode {
		if version := claudeCodeValidator.ExtractVersion(ua); version != "" {
			ctx = service.SetClaudeCodeVersion(ctx, version)
		}
	}

	c.Request = c.Request.WithContext(ctx)
}

func claudeCodeBodyMapFromParsedRequest(parsedReq *service.ParsedRequest) map[string]any {
	if parsedReq == nil {
		return nil
	}
	bodyMap := map[string]any{
		"model": parsedReq.Model,
	}
	// 探测识别（max_tokens=1）需要看到该字段，复用已解析请求时一并带上。
	if parsedReq.MaxTokens > 0 {
		bodyMap["max_tokens"] = parsedReq.MaxTokens
	}
	if parsedReq.HasSystem {
		if system, ok := parsedReq.SystemValue(); ok {
			bodyMap["system"] = system
		} else {
			bodyMap["system"] = nil
		}
	}
	if parsedReq.MetadataUserID != "" {
		bodyMap["metadata"] = map[string]any{"user_id": parsedReq.MetadataUserID}
	}
	return bodyMap
}

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
	// defaultPingInterval 流式响应等待时发送 ping 的默认间隔
	defaultPingInterval = 10 * time.Second
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
	SSEPingFormatClaude SSEPingFormat = "data: {\"type\": \"ping\"}\n\n"
	// SSEPingFormatNone indicates no ping should be sent (e.g., OpenAI has no ping spec)
	SSEPingFormatNone SSEPingFormat = ""
	// SSEPingFormatComment is an SSE comment ping for OpenAI/Codex CLI clients
	SSEPingFormatComment SSEPingFormat = ":\n\n"
)

// ConcurrencyError represents a concurrency limit error with context
type ConcurrencyError struct {
	SlotType  string
	IsTimeout bool
}

func (e *ConcurrencyError) Error() string {
	if e.IsTimeout {
		return fmt.Sprintf("timeout waiting for %s concurrency slot", e.SlotType)
	}
	return fmt.Sprintf("%s concurrency limit reached", e.SlotType)
}

type WaitQueueFullError struct {
	SlotType string
}

func (e *WaitQueueFullError) Error() string {
	return "Too many pending requests, please retry later"
}

// ConcurrencyHelper provides common concurrency slot management for gateway handlers
type ConcurrencyHelper struct {
	concurrencyService *service.ConcurrencyService
	pingFormat         SSEPingFormat
	pingInterval       time.Duration
}

// NewConcurrencyHelper creates a new ConcurrencyHelper
func NewConcurrencyHelper(concurrencyService *service.ConcurrencyService, pingFormat SSEPingFormat, pingInterval time.Duration) *ConcurrencyHelper {
	if pingInterval <= 0 {
		pingInterval = defaultPingInterval
	}
	return &ConcurrencyHelper{
		concurrencyService: concurrencyService,
		pingFormat:         pingFormat,
		pingInterval:       pingInterval,
	}
}

// wrapReleaseOnDone ensures release runs at most once and still triggers on context cancellation.
// 用于避免客户端断开或上游超时导致的并发槽位泄漏。
// 优化：基于 context.AfterFunc 注册回调，避免每请求额外守护 goroutine。
func wrapReleaseOnDone(ctx context.Context, releaseFunc func()) func() {
	if releaseFunc == nil {
		return nil
	}
	// Enforced leases must outlive cancellation until the forwarding owner joins
	// upstream. Releasing on ctx.Done would admit a replacement during shutdown.
	if service.HasAPIKeyAdmissionOwner(ctx) {
		return sync.OnceFunc(releaseFunc)
	}
	var once sync.Once
	releaseOnce := func() {
		once.Do(releaseFunc)
	}
	stop := context.AfterFunc(ctx, releaseOnce)

	return func() {
		_ = stop()
		releaseOnce()
	}
}

// IncrementWaitCount increments the wait count for a user
func (h *ConcurrencyHelper) IncrementWaitCount(ctx context.Context, userID int64, maxWait int) (bool, error) {
	return h.concurrencyService.IncrementWaitCount(ctx, userID, maxWait)
}

// DecrementWaitCount decrements the wait count for a user
func (h *ConcurrencyHelper) DecrementWaitCount(ctx context.Context, userID int64) {
	h.concurrencyService.DecrementWaitCount(ctx, userID)
}

// IncrementAccountWaitCount increments the wait count for an account
func (h *ConcurrencyHelper) IncrementAccountWaitCount(ctx context.Context, accountID int64, maxWait int) (bool, error) {
	return h.concurrencyService.IncrementAccountWaitCount(ctx, accountID, maxWait)
}

// DecrementAccountWaitCount decrements the wait count for an account
func (h *ConcurrencyHelper) DecrementAccountWaitCount(ctx context.Context, accountID int64) {
	h.concurrencyService.DecrementAccountWaitCount(ctx, accountID)
}

// TryAcquireUserSlot 尝试立即获取用户并发槽位。
// 返回值: (releaseFunc, acquired, error)
func (h *ConcurrencyHelper) TryAcquireUserSlot(ctx context.Context, userID int64, maxConcurrency int) (func(), bool, error) {
	result, err := h.concurrencyService.AcquireUserSlot(ctx, userID, maxConcurrency)
	if err != nil {
		return nil, false, err
	}
	if !result.Acquired {
		return nil, false, nil
	}
	return result.ReleaseFunc, true, nil
}

// AcquireLiveUserSlot is the owned user reservation used by Live creation. The
// returned RequestID is the exact ordinary member that the Live transfer moves
// into the joint lease; the ReleaseFunc still owns it until that transfer.
func (h *ConcurrencyHelper) AcquireLiveUserSlot(ctx context.Context, userID int64, maxConcurrency int) (*service.AcquireResult, error) {
	if h == nil || h.concurrencyService == nil {
		if maxConcurrency > 0 {
			return nil, fmt.Errorf("concurrency service is unavailable")
		}
		return &service.AcquireResult{Acquired: true, ReleaseFunc: func() {}}, nil
	}
	return h.concurrencyService.AcquireUserSlot(ctx, userID, maxConcurrency)
}

func (h *ConcurrencyHelper) TryAcquireUserSlotForAPIKey(ctx context.Context, userID int64, maxConcurrency int, apiKeyID int64, keyLimit int) (func(), bool, error) {
	// Key admission (including its bounded queue wait) happens before any user
	// slot so a waiting request does not hold user capacity.
	keyRelease, err := h.withAPIKeySlot(ctx, apiKeyID, keyLimit, nil)
	if err != nil {
		return nil, false, err
	}
	releaseFunc, acquired, err := h.TryAcquireUserSlot(ctx, userID, maxConcurrency)
	if err != nil {
		keyRelease()
		return nil, false, err
	}
	if !acquired {
		keyRelease()
		return nil, false, nil
	}
	return wrapReleaseOnDone(ctx, sync.OnceFunc(func() {
		releaseFunc()
		keyRelease()
	})), true, nil
}

// WS acquisition still observes cancellation. Once acquired, tracking belongs
// to the turn owner, which releases only after upstream forwarding has stopped.
// Key waiting happens before the user slot for the same reason as HTTP.
func (h *ConcurrencyHelper) TryAcquireWSUserSlotForAPIKey(ctx context.Context, userID int64, maxConcurrency int, apiKeyID int64, keyLimit int) (func(), bool, error) {
	keyRelease, err := h.withAPIKeySlot(ctx, apiKeyID, keyLimit, nil)
	if err != nil {
		return nil, false, err
	}
	releaseFunc, acquired, err := h.TryAcquireUserSlot(ctx, userID, maxConcurrency)
	if err != nil {
		keyRelease()
		return nil, false, err
	}
	if !acquired {
		keyRelease()
		return nil, false, nil
	}
	return sync.OnceFunc(func() {
		releaseFunc()
		keyRelease()
	}), true, nil
}

// AcquireOpenAIWSIngressLease bounds the whole client WebSocket lifecycle,
// independently from per-turn user and account slots.
func (h *ConcurrencyHelper) AcquireOpenAIWSIngressLease(ctx context.Context, apiKeyID int64, maxConnections int) (*service.OpenAIWSIngressLease, bool, error) {
	if h == nil || h.concurrencyService == nil {
		return nil, false, fmt.Errorf("concurrency service is unavailable")
	}
	return h.concurrencyService.AcquireOpenAIWSIngressLease(ctx, apiKeyID, maxConnections)
}

// TryAcquireAccountSlot 尝试立即获取账号并发槽位。
// 返回值: (releaseFunc, acquired, error)
func (h *ConcurrencyHelper) TryAcquireAccountSlot(ctx context.Context, accountID int64, maxConcurrency int) (func(), bool, error) {
	result, err := h.concurrencyService.AcquireAccountSlot(ctx, accountID, maxConcurrency)
	if err != nil {
		return nil, false, err
	}
	if !result.Acquired {
		return nil, false, nil
	}
	return result.ReleaseFunc, true, nil
}

// AcquireUserSlotWithWait acquires a user concurrency slot, waiting if necessary.
// For streaming requests, sends ping events during the wait.
// streamStarted is updated if streaming response has begun.
func (h *ConcurrencyHelper) AcquireUserSlotWithWait(c *gin.Context, userID int64, maxConcurrency int, apiKeyID int64, keyLimit int, isStream bool, streamStarted *bool) (func(), error) {
	return h.acquireUserSlotWithWaitTimeout(c, userID, maxConcurrency, apiKeyID, keyLimit, maxConcurrencyWait, isStream, streamStarted)
}

func (h *ConcurrencyHelper) acquireUserSlotWithWaitTimeout(c *gin.Context, userID int64, maxConcurrency int, apiKeyID int64, keyLimit int, timeout time.Duration, isStream bool, streamStarted *bool) (func(), error) {
	ctx := c.Request.Context()
	// Reserve key capacity before any user queue can emit an SSE heartbeat.
	keyRelease, err := h.withAPIKeySlot(ctx, apiKeyID, keyLimit, nil)
	if err != nil {
		return nil, err
	}
	transferred := false
	defer func() {
		if !transferred {
			keyRelease()
		}
	}()
	combine := func(userRelease func()) func() {
		transferred = true
		return wrapReleaseOnDone(ctx, sync.OnceFunc(func() {
			if userRelease != nil {
				userRelease()
			}
			keyRelease()
		}))
	}

	// Try to acquire immediately
	releaseFunc, acquired, err := h.TryAcquireUserSlot(ctx, userID, maxConcurrency)
	if err != nil {
		return nil, err
	}

	if acquired {
		return combine(releaseFunc), nil
	}

	queueLimit := service.CalculateMaxWait(maxConcurrency) - maxConcurrency
	if queueLimit < 1 {
		queueLimit = 1
	}
	canWait, err := h.IncrementWaitCount(ctx, userID, queueLimit)
	if err != nil {
		return nil, err
	}
	if !canWait {
		return nil, &WaitQueueFullError{SlotType: "user"}
	}
	defer h.DecrementWaitCount(ctx, userID)

	// Need to wait - handle streaming ping if needed
	releaseFunc, err = h.waitForSlotWithPingTimeout(c, "user", userID, maxConcurrency, timeout, isStream, streamStarted, false)
	if err != nil {
		return nil, err
	}
	return combine(releaseFunc), nil
}

// ReserveAPIKeySlotWithWait is the owned-reservation variant used by Live
// creation, which transfers the key member instead of releasing it.
func (h *ConcurrencyHelper) ReserveAPIKeySlotWithWait(ctx context.Context, apiKeyID int64, keyLimit int) (*service.APIKeySlotReservation, error) {
	if h == nil || h.concurrencyService == nil {
		if keyLimit != 0 {
			return nil, fmt.Errorf("API key concurrency admission unavailable")
		}
		return nil, nil
	}
	reservation, err := h.concurrencyService.ReserveAPIKeySlotWithWait(ctx, apiKeyID, keyLimit)
	if err != nil {
		return nil, err
	}
	if reservation == nil {
		return nil, &ConcurrencyError{SlotType: "API key"}
	}
	return reservation, nil
}

// ReserveWSAPIKeySlotWithWait is the WS turn admission variant: every turn runs
// one bounded fresh authorization check before deciding capacity, so unlimited
// and queue-disabled keys still see per-turn revocations and expansions. The
// refreshed limit is the one used for capacity, and only an enforced (limit > 0)
// result touches Redis admission; limit 0 stays stats-only.
func (h *ConcurrencyHelper) ReserveWSAPIKeySlotWithWait(ctx context.Context, apiKeyID int64, keyLimit int) (*service.APIKeySlotReservation, error) {
	if h == nil || h.concurrencyService == nil {
		if keyLimit != 0 {
			return nil, fmt.Errorf("API key concurrency admission unavailable")
		}
		return nil, nil
	}
	refreshed, revalidated, err := h.concurrencyService.RevalidateAPIKeyQueueTurn(ctx)
	if err != nil {
		return nil, err
	}
	if revalidated {
		keyLimit = refreshed
	}
	reservation, err := h.concurrencyService.ReserveAPIKeySlotWithWait(ctx, apiKeyID, keyLimit)
	if err != nil {
		return nil, err
	}
	if reservation == nil {
		return nil, &ConcurrencyError{SlotType: "API key"}
	}
	return reservation, nil
}

// RevalidateTurnAuth runs the installed queue revalidator once under the
// bounded admission context, or returns nil when no fresh gate is installed.
// WS frame hooks call it ahead of payload parsing to refresh permissions for
// the current frame; it performs no moderation, slot or pricing work.
func (h *ConcurrencyHelper) RevalidateTurnAuth(ctx context.Context) error {
	if h == nil || h.concurrencyService == nil {
		return nil
	}
	_, _, err := h.concurrencyService.RevalidateAPIKeyQueueTurn(ctx)
	return err
}

// AcquireAPIKeySlot covers HTTP forwarding endpoints without a user wait queue.
func (h *ConcurrencyHelper) AcquireAPIKeySlot(ctx context.Context, apiKeyID int64, keyLimit int) (func(), error) {
	if h == nil || h.concurrencyService == nil {
		if keyLimit != 0 {
			return nil, fmt.Errorf("API key concurrency admission unavailable")
		}
		return func() {}, nil
	}
	release, err := h.withAPIKeySlot(ctx, apiKeyID, keyLimit, nil)
	return wrapReleaseOnDone(ctx, release), err
}

func (h *ConcurrencyHelper) withAPIKeySlot(ctx context.Context, apiKeyID int64, keyLimit int, releaseFunc func()) (func(), error) {
	// The key-level queue (when configured) waits here, before user/account
	// admission and without writing any response header.
	result, err := h.concurrencyService.AcquireAPIKeySlotWithWait(ctx, apiKeyID, keyLimit)
	if err != nil || !result.Acquired {
		if releaseFunc != nil {
			releaseFunc()
		}
		if err != nil {
			return nil, err
		}
		return nil, &ConcurrencyError{SlotType: "API key"}
	}
	return sync.OnceFunc(func() {
		if releaseFunc != nil {
			releaseFunc()
		}
		if result.ReleaseFunc != nil {
			result.ReleaseFunc()
		}
	}), nil
}

// AcquireAccountSlotWithWait acquires an account concurrency slot, waiting if necessary.
// For streaming requests, sends ping events during the wait.
// streamStarted is updated if streaming response has begun.
func (h *ConcurrencyHelper) AcquireAccountSlotWithWait(c *gin.Context, accountID int64, maxConcurrency int, isStream bool, streamStarted *bool) (func(), error) {
	ctx := c.Request.Context()

	// Try to acquire immediately
	releaseFunc, acquired, err := h.TryAcquireAccountSlot(ctx, accountID, maxConcurrency)
	if err != nil {
		return nil, err
	}

	if acquired {
		return releaseFunc, nil
	}

	// Need to wait - handle streaming ping if needed
	return h.waitForSlotWithPing(c, "account", accountID, maxConcurrency, isStream, streamStarted)
}

// waitForSlotWithPing waits for a concurrency slot, sending ping events for streaming requests.
// streamStarted pointer is updated when streaming begins (for proper error handling by caller).
func (h *ConcurrencyHelper) waitForSlotWithPing(c *gin.Context, slotType string, id int64, maxConcurrency int, isStream bool, streamStarted *bool) (func(), error) {
	return h.waitForSlotWithPingTimeout(c, slotType, id, maxConcurrency, maxConcurrencyWait, isStream, streamStarted, false)
}

// waitForSlotWithPingTimeout waits for a concurrency slot with a custom timeout.
func (h *ConcurrencyHelper) waitForSlotWithPingTimeout(c *gin.Context, slotType string, id int64, maxConcurrency int, timeout time.Duration, isStream bool, streamStarted *bool, tryImmediate bool) (func(), error) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), timeout)
	defer cancel()

	acquireSlot := func() (*service.AcquireResult, error) {
		if slotType == "user" {
			return h.concurrencyService.AcquireUserSlot(ctx, id, maxConcurrency)
		}
		return h.concurrencyService.AcquireAccountSlot(ctx, id, maxConcurrency)
	}

	if tryImmediate {
		result, err := acquireSlot()
		if err != nil {
			return nil, err
		}
		if result.Acquired {
			return result.ReleaseFunc, nil
		}
	}

	// Determine if ping is needed (streaming + ping format defined)
	needPing := isStream && h.pingFormat != ""

	var flusher http.Flusher
	if needPing {
		var ok bool
		flusher, ok = c.Writer.(http.Flusher)
		if !ok {
			return nil, fmt.Errorf("streaming not supported")
		}
	}

	// Only create ping ticker if ping is needed
	var pingCh <-chan time.Time
	if needPing {
		pingTicker := time.NewTicker(h.pingInterval)
		defer pingTicker.Stop()
		pingCh = pingTicker.C
	}

	backoff := initialBackoff
	timer := time.NewTimer(backoff)
	defer timer.Stop()

	for {
		select {
		case <-ctx.Done():
			if parentErr := c.Request.Context().Err(); parentErr != nil {
				return nil, parentErr
			}
			return nil, &ConcurrencyError{
				SlotType:  slotType,
				IsTimeout: true,
			}

		case <-pingCh:
			// Send ping to keep connection alive
			if !*streamStarted {
				c.Header("Content-Type", "text/event-stream")
				c.Header("Cache-Control", "no-cache")
				c.Header("Connection", "keep-alive")
				c.Header("X-Accel-Buffering", "no")
				*streamStarted = true
			}
			written, err := fmt.Fprint(c.Writer, string(h.pingFormat))
			if err != nil {
				return nil, err
			}
			recordGatewayStreamHeartbeat(c, written)
			flusher.Flush()

		case <-timer.C:
			// Try to acquire slot
			result, err := acquireSlot()
			if err != nil {
				return nil, err
			}

			if result.Acquired {
				return result.ReleaseFunc, nil
			}
			backoff = nextBackoff(backoff)
			timer.Reset(backoff)
		}
	}
}

// AcquireAccountSlotWithWaitTimeout acquires an account slot with a custom timeout (keeps SSE ping).
func (h *ConcurrencyHelper) AcquireAccountSlotWithWaitTimeout(c *gin.Context, accountID int64, maxConcurrency int, timeout time.Duration, isStream bool, streamStarted *bool) (func(), error) {
	return h.waitForSlotWithPingTimeout(c, "account", accountID, maxConcurrency, timeout, isStream, streamStarted, true)
}

// nextBackoff 计算下一次退避时间
// 性能优化：使用指数退避 + 随机抖动，避免惊群效应
// current: 当前退避时间
// 返回值：下一次退避时间（100ms ~ 2s 之间）
func nextBackoff(current time.Duration) time.Duration {
	// 指数退避：当前时间 * 1.5
	next := time.Duration(float64(current) * backoffMultiplier)
	if next > maxBackoff {
		next = maxBackoff
	}
	// 添加 ±20% 的随机抖动（jitter 范围 0.8 ~ 1.2）
	// 抖动可以分散多个请求的重试时间点，避免同时冲击 Redis
	jitter := 0.8 + rand.Float64()*0.4
	jittered := time.Duration(float64(next) * jitter)
	if jittered < initialBackoff {
		return initialBackoff
	}
	if jittered > maxBackoff {
		return maxBackoff
	}
	return jittered
}
