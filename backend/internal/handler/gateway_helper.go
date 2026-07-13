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

	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

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

type AdmissionMode uint8

const (
	AdmissionModeWait AdmissionMode = iota
	AdmissionModeFailFast
)

type UserAdmissionRequest struct {
	UserID         int64
	MaxConcurrency int
	Mode           AdmissionMode
	Stream         bool
	StreamStarted  *bool
}

type AccountWaitPolicy uint8

const (
	AccountWaitPolicyUntracked AccountWaitPolicy = iota
	AccountWaitPolicyTracked
	AccountWaitPolicyRetryThenTracked
)

type AccountAdmissionRequest struct {
	Selection  *service.AccountSelectionResult
	WaitPolicy AccountWaitPolicy
}

type RequestAdmission struct {
	mu             sync.Mutex
	helper         *ConcurrencyHelper
	context        *gin.Context
	mode           AdmissionMode
	stream         bool
	streamStarted  *bool
	userRelease    func()
	accountRelease func()
}

type AccountUnavailableError struct{}

func (e *AccountUnavailableError) Error() string {
	return "account concurrency slot unavailable"
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

// Begin acquires the request-scoped user slot and owns its release lifecycle.
func (h *ConcurrencyHelper) Begin(c *gin.Context, request UserAdmissionRequest) (*RequestAdmission, error) {
	streamStarted := request.StreamStarted
	if streamStarted == nil {
		streamStarted = new(bool)
	}

	var releaseFunc func()
	var err error
	if request.Mode == AdmissionModeFailFast {
		var acquired bool
		releaseFunc, acquired, err = h.tryAcquireUserSlot(c.Request.Context(), request.UserID, request.MaxConcurrency)
		if err == nil && !acquired {
			return nil, &ConcurrencyError{SlotType: "user"}
		}
		if err == nil {
			releaseFunc = h.withAPIKeySlotFromGin(c, releaseFunc)
		}
	} else {
		releaseFunc, err = h.acquireUserSlotWithWait(
			c,
			request.UserID,
			request.MaxConcurrency,
			request.Stream,
			streamStarted,
		)
	}
	if err != nil {
		return nil, err
	}

	return &RequestAdmission{
		helper:        h,
		context:       c,
		mode:          request.Mode,
		stream:        request.Stream,
		streamStarted: streamStarted,
		userRelease:   wrapReleaseOnDone(c.Request.Context(), releaseFunc),
	}, nil
}

// AdmitAccount adopts a scheduler-owned slot or waits for the selected account.
func (a *RequestAdmission) AdmitAccount(request AccountAdmissionRequest) error {
	selection := request.Selection
	if a == nil || a.helper == nil || a.context == nil || selection == nil || selection.Account == nil {
		return &AccountUnavailableError{}
	}

	a.ReleaseAccount()
	if selection.Acquired {
		a.setAccountRelease(selection.ReleaseFunc)
		return nil
	}
	if selection.WaitPlan == nil {
		return &AccountUnavailableError{}
	}

	var releaseFunc func()
	if a.mode == AdmissionModeFailFast {
		acquiredRelease, acquired, err := a.helper.tryAcquireAccountSlot(
			a.context.Request.Context(),
			selection.Account.ID,
			selection.WaitPlan.MaxConcurrency,
		)
		if err != nil {
			return err
		}
		if !acquired {
			return &AccountUnavailableError{}
		}
		releaseFunc = acquiredRelease
	} else {
		if request.WaitPolicy == AccountWaitPolicyRetryThenTracked {
			acquiredRelease, acquired, err := a.helper.tryAcquireAccountSlot(
				a.context.Request.Context(),
				selection.Account.ID,
				selection.WaitPlan.MaxConcurrency,
			)
			if err != nil {
				return err
			}
			if acquired {
				a.setAccountRelease(acquiredRelease)
				return nil
			}
		}

		trackedWait := request.WaitPolicy != AccountWaitPolicyUntracked
		if trackedWait {
			canWait, err := a.helper.incrementAccountWaitCount(
				a.context.Request.Context(),
				selection.Account.ID,
				selection.WaitPlan.MaxWaiting,
			)
			if err != nil {
				return err
			}
			if !canWait {
				return &WaitQueueFullError{SlotType: "account"}
			}
			defer a.helper.decrementAccountWaitCount(a.context.Request.Context(), selection.Account.ID)
		}

		var err error
		releaseFunc, err = a.helper.acquireAccountSlotWithWaitTimeout(
			a.context,
			selection.Account.ID,
			selection.WaitPlan.MaxConcurrency,
			selection.WaitPlan.Timeout,
			a.stream,
			a.streamStarted,
		)
		if err != nil {
			return err
		}
	}

	a.setAccountRelease(releaseFunc)
	return nil
}

func (a *RequestAdmission) setAccountRelease(releaseFunc func()) {
	wrapped := wrapReleaseOnDone(a.context.Request.Context(), releaseFunc)
	a.mu.Lock()
	a.accountRelease = wrapped
	a.mu.Unlock()
}

// ReleaseAccount releases only the selected account slot, preserving the user slot.
func (a *RequestAdmission) ReleaseAccount() {
	if a == nil {
		return
	}
	a.mu.Lock()
	releaseFunc := a.accountRelease
	a.accountRelease = nil
	a.mu.Unlock()
	if releaseFunc != nil {
		releaseFunc()
	}
}

// Close releases all concurrency resources owned by this request.
func (a *RequestAdmission) Close() {
	if a == nil {
		return
	}
	a.mu.Lock()
	accountRelease := a.accountRelease
	userRelease := a.userRelease
	a.accountRelease = nil
	a.userRelease = nil
	a.mu.Unlock()
	if accountRelease != nil {
		accountRelease()
	}
	if userRelease != nil {
		userRelease()
	}
}

// wrapReleaseOnDone ensures release runs at most once and still triggers on context cancellation.
// 用于避免客户端断开或上游超时导致的并发槽位泄漏。
// 优化：基于 context.AfterFunc 注册回调，避免每请求额外守护 goroutine。
func wrapReleaseOnDone(ctx context.Context, releaseFunc func()) func() {
	if releaseFunc == nil {
		return nil
	}
	var once sync.Once
	release := func() {
		once.Do(releaseFunc)
	}
	stop := context.AfterFunc(ctx, release)
	return func() {
		_ = stop()
		release()
	}
}

func (h *ConcurrencyHelper) incrementWaitCount(ctx context.Context, userID int64, maxWait int) (bool, error) {
	return h.concurrencyService.IncrementWaitCount(ctx, userID, maxWait)
}

func (h *ConcurrencyHelper) decrementWaitCount(ctx context.Context, userID int64) {
	h.concurrencyService.DecrementWaitCount(ctx, userID)
}

func (h *ConcurrencyHelper) incrementAccountWaitCount(ctx context.Context, accountID int64, maxWait int) (bool, error) {
	return h.concurrencyService.IncrementAccountWaitCount(ctx, accountID, maxWait)
}

func (h *ConcurrencyHelper) decrementAccountWaitCount(ctx context.Context, accountID int64) {
	h.concurrencyService.DecrementAccountWaitCount(ctx, accountID)
}

func (h *ConcurrencyHelper) tryAcquireUserSlot(ctx context.Context, userID int64, maxConcurrency int) (func(), bool, error) {
	result, err := h.concurrencyService.AcquireUserSlot(ctx, userID, maxConcurrency)
	if err != nil {
		return nil, false, err
	}
	if !result.Acquired {
		return nil, false, nil
	}
	return result.ReleaseFunc, true, nil
}

func (h *ConcurrencyHelper) tryAcquireAccountSlot(ctx context.Context, accountID int64, maxConcurrency int) (func(), bool, error) {
	result, err := h.concurrencyService.AcquireAccountSlot(ctx, accountID, maxConcurrency)
	if err != nil {
		return nil, false, err
	}
	if !result.Acquired {
		return nil, false, nil
	}
	return result.ReleaseFunc, true, nil
}

func (h *ConcurrencyHelper) acquireUserSlotWithWait(c *gin.Context, userID int64, maxConcurrency int, isStream bool, streamStarted *bool) (func(), error) {
	return h.acquireUserSlotWithWaitTimeout(c, userID, maxConcurrency, maxConcurrencyWait, isStream, streamStarted)
}

func (h *ConcurrencyHelper) acquireUserSlotWithWaitTimeout(c *gin.Context, userID int64, maxConcurrency int, timeout time.Duration, isStream bool, streamStarted *bool) (func(), error) {
	ctx := c.Request.Context()

	// Try to acquire immediately
	releaseFunc, acquired, err := h.tryAcquireUserSlot(ctx, userID, maxConcurrency)
	if err != nil {
		return nil, err
	}

	if acquired {
		return h.withAPIKeySlotFromGin(c, releaseFunc), nil
	}

	queueLimit := service.CalculateMaxWait(maxConcurrency) - maxConcurrency
	if queueLimit < 1 {
		queueLimit = 1
	}
	canWait, err := h.incrementWaitCount(ctx, userID, queueLimit)
	if err != nil {
		return nil, err
	}
	if !canWait {
		return nil, &WaitQueueFullError{SlotType: "user"}
	}
	defer h.decrementWaitCount(ctx, userID)

	// Need to wait - handle streaming ping if needed
	releaseFunc, err = h.waitForSlotWithPingTimeout(c, "user", userID, maxConcurrency, timeout, isStream, streamStarted, false)
	if err != nil {
		return nil, err
	}
	return h.withAPIKeySlotFromGin(c, releaseFunc), nil
}

func (h *ConcurrencyHelper) withAPIKeySlotFromGin(c *gin.Context, releaseFunc func()) func() {
	if c == nil {
		return releaseFunc
	}
	apiKey, ok := middleware2.GetAPIKeyFromContext(c)
	if !ok || apiKey == nil {
		return releaseFunc
	}
	return h.withAPIKeySlot(c.Request.Context(), apiKey.ID, releaseFunc)
}

func (h *ConcurrencyHelper) withAPIKeySlot(ctx context.Context, apiKeyID int64, releaseFunc func()) func() {
	if h == nil || h.concurrencyService == nil || apiKeyID <= 0 {
		return releaseFunc
	}
	apiKeyReleaseFunc := h.concurrencyService.TrackAPIKeySlot(ctx, apiKeyID)
	return func() {
		if releaseFunc != nil {
			releaseFunc()
		}
		if apiKeyReleaseFunc != nil {
			apiKeyReleaseFunc()
		}
	}
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
			if _, err := fmt.Fprint(c.Writer, string(h.pingFormat)); err != nil {
				return nil, err
			}
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

func (h *ConcurrencyHelper) acquireAccountSlotWithWaitTimeout(c *gin.Context, accountID int64, maxConcurrency int, timeout time.Duration, isStream bool, streamStarted *bool) (func(), error) {
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
