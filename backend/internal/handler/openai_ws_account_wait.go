package handler

import (
	"context"
	"errors"
	"sync/atomic"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	coderws "github.com/coder/websocket"
	"go.uber.org/zap"
)

const (
	openAIWSAccountWaitPhaseInitial    = "initial"
	openAIWSAccountWaitPhaseSubsequent = "subsequent"
	openAIWSAccountWaitPhaseRetry      = "retry"
)

type openAIWSAccountWaitBudget struct {
	deadline     time.Time
	turn         int
	requestSent  atomic.Bool
	continuation bool
	// waitDisabled 为真时用户槽与账号槽都不排队、拿不到立即断开：非 openai 平台保持上游行为。
	waitDisabled bool
}

func (h *OpenAIGatewayHandler) shouldYieldWSAccountSlot(ctx context.Context, accountID int64, turn int, log *zap.Logger) bool {
	limit := h.gatewayService.OpenAIContinuationBurstLimit()
	reason, err := h.concurrencyHelper.concurrencyService.HeldAccountSlotYieldReason(ctx, accountID, limit)
	if err != nil {
		log.Warn("openai.websocket_account_admission_state_failed", zap.Int64("account_id", accountID), zap.Error(err))
		return limit > 0
	}
	if reason != "" {
		log.Debug("openai.websocket_account_slot_yielded", zap.Int64("account_id", accountID), zap.Int("turn", turn), zap.String("reason", reason))
	}
	return reason != ""
}

func (b *openAIWSAccountWaitBudget) nextTurn() {
	b.turn++
	b.deadline = time.Time{}
	b.continuation = true
}

// 透传后续轮的帧在准入前一直由网关持有、尚未写入上游，可与其他模式一样排队等槽。
func (b *openAIWSAccountWaitBudget) canWait(mode, phase string) bool {
	if b.waitDisabled || mode == service.OpenAIWSIngressModeOff {
		return false
	}
	if mode != service.OpenAIWSIngressModePassthrough || phase == openAIWSAccountWaitPhaseSubsequent {
		return true
	}
	return !b.continuation && !b.requestSent.Load()
}

func (b *openAIWSAccountWaitBudget) waitDeadline(timeout time.Duration, retryDeadline time.Time) time.Time {
	deadline := time.Now().Add(timeout)
	if b.deadline.IsZero() || deadline.Before(b.deadline) {
		b.deadline = deadline
	}
	if !retryDeadline.IsZero() && retryDeadline.Before(b.deadline) {
		return retryDeadline
	}
	return b.deadline
}

func (b *openAIWSAccountWaitBudget) expired() bool {
	return !b.deadline.IsZero() && !time.Now().Before(b.deadline)
}

func openAIWSAccountBusyError() error {
	return service.NewOpenAIWSClientCloseError(coderws.StatusTryAgainLater, "account is busy, please retry later", nil)
}

func closeWSAccountAdmission(ctx context.Context, conn *coderws.Conn, err error) {
	if errors.Is(context.Cause(ctx), service.ErrOpenAIWSIngressLeaseLost) {
		closeOpenAIClientWS(conn, coderws.StatusTryAgainLater, "websocket ingress capacity lease lost; please reconnect")
		return
	}
	var closeErr *service.OpenAIWSClientCloseError
	if errors.As(err, &closeErr) {
		closeOpenAIClientWS(conn, closeErr.StatusCode(), closeErr.Reason())
		return
	}
	closeOpenAIClientWS(conn, coderws.StatusGoingAway, "websocket request canceled")
}

// enterAccountWaitQueue 按等待计划的类别选计数键：续聊类进 wait:account:cont，其余进 wait:account。WS 与 HTTP 共用。
func (h *OpenAIGatewayHandler) enterAccountWaitQueue(ctx context.Context, accountID int64, plan *service.AccountWaitPlan) (bool, func(), error) {
	if plan.Class == service.AccountWaitClassContinuation {
		canWait, err := h.concurrencyHelper.IncrementAccountContinuationWaitCount(ctx, accountID, plan.MaxWaiting)
		return canWait, func() { h.concurrencyHelper.DecrementAccountContinuationWaitCount(ctx, accountID) }, err
	}
	canWait, err := h.concurrencyHelper.IncrementAccountWaitCount(ctx, accountID, plan.MaxWaiting)
	return canWait, func() { h.concurrencyHelper.DecrementAccountWaitCount(ctx, accountID) }, err
}

func (h *OpenAIGatewayHandler) acquireWSAccountSlot(ctx context.Context, account *service.Account, maxConcurrency int, plan *service.AccountWaitPlan, budget *openAIWSAccountWaitBudget, mode, phase string, retryDeadline time.Time, log *zap.Logger) (func(), error) {
	release, _, err := h.acquireWSAccountSlotLease(ctx, account, maxConcurrency, plan, budget, mode, phase, retryDeadline, log)
	return release, err
}

func (h *OpenAIGatewayHandler) acquireWSAccountSlotLease(ctx context.Context, account *service.Account, maxConcurrency int, plan *service.AccountWaitPlan, budget *openAIWSAccountWaitBudget, mode, phase string, retryDeadline time.Time, log *zap.Logger) (func(), func(context.Context) (bool, error), error) {
	if ctx.Err() != nil {
		return nil, nil, context.Cause(ctx)
	}
	if budget.expired() {
		return nil, nil, openAIWSAccountBusyError()
	}
	planClass := service.AccountWaitClassLegacy
	if plan != nil {
		planClass = plan.Class
	}
	fast, err := h.concurrencyHelper.TryAcquireAccountSlotForPlan(ctx, account.ID, maxConcurrency, planClass, h.gatewayService.OpenAIContinuationBurstLimit())
	if err != nil {
		return nil, nil, service.NewOpenAIWSClientCloseError(coderws.StatusInternalError, "failed to acquire account concurrency slot", err)
	}
	if fast != nil && fast.Acquired {
		if ctx.Err() != nil {
			fast.ReleaseFunc()
			return nil, nil, context.Cause(ctx)
		}
		return fast.ReleaseFunc, fast.ReuseFunc, nil
	}
	if !account.IsOpenAI() || !budget.canWait(mode, phase) || plan == nil || plan.Timeout <= 0 || plan.MaxWaiting <= 0 {
		return nil, nil, openAIWSAccountBusyError()
	}
	deadline := budget.waitDeadline(plan.Timeout, retryDeadline)
	if !time.Now().Before(deadline) {
		return nil, nil, openAIWSAccountBusyError()
	}
	canWait, leaveQueue, err := h.enterAccountWaitQueue(ctx, account.ID, plan)
	if err != nil {
		return nil, nil, service.NewOpenAIWSClientCloseError(coderws.StatusInternalError, "failed to enter account wait queue", err)
	}
	if !canWait {
		log.Info("openai.websocket_account_wait_finished", zap.Int64("account_id", account.ID), zap.String("mode", mode), zap.String("phase", phase), zap.String("class", plan.Class.String()), zap.String("reason", "queue_full"))
		return nil, nil, openAIWSAccountBusyError()
	}
	defer leaveQueue()
	observedCtx, endObservation := service.BeginOpenAIWSAccountWait(ctx)
	defer endObservation()
	waitCtx, cancel := context.WithDeadline(observedCtx, deadline)
	defer cancel()
	started := time.Now()
	log.Info("openai.websocket_account_wait_started", zap.Int64("account_id", account.ID), zap.String("mode", mode), zap.String("phase", phase), zap.Int("turn", budget.turn), zap.Int("max_concurrency", maxConcurrency), zap.Int("max_waiting", plan.MaxWaiting), zap.String("class", plan.Class.String()), zap.Int64("budget_ms", time.Until(deadline).Milliseconds()))
	result, err := waitForConcurrencySlotResult(waitCtx, func() (*service.AcquireResult, error) {
		return h.concurrencyHelper.TryAcquireAccountSlotForPlan(waitCtx, account.ID, maxConcurrency, plan.Class, h.gatewayService.OpenAIContinuationBurstLimit())
	}, nil, nil)
	if err == nil && waitCtx.Err() != nil {
		if result != nil && result.ReleaseFunc != nil {
			result.ReleaseFunc()
		}
		result = nil
		err = context.Cause(waitCtx)
	}
	reason := "acquired"
	if err != nil {
		switch {
		case ctx.Err() != nil:
			reason = "canceled"
		case observedCtx.Err() != nil:
			reason = "client_closed"
		case errors.Is(err, context.DeadlineExceeded):
			reason = "wait_timeout"
		default:
			reason = "acquire_error"
		}
	}
	log.Info("openai.websocket_account_wait_finished", zap.Int64("account_id", account.ID), zap.String("mode", mode), zap.String("phase", phase), zap.Int("turn", budget.turn), zap.Int64("wait_ms", time.Since(started).Milliseconds()), zap.String("class", plan.Class.String()), zap.String("reason", reason))
	if err == nil {
		return result.ReleaseFunc, result.ReuseFunc, nil
	}
	if ctx.Err() != nil {
		return nil, nil, context.Cause(ctx)
	}
	if observedCtx.Err() != nil {
		cause := context.Cause(observedCtx)
		var closeErr *service.OpenAIWSClientCloseError
		if errors.As(cause, &closeErr) {
			return nil, nil, closeErr
		}
		return nil, nil, service.NewOpenAIWSClientCloseError(coderws.StatusGoingAway, "websocket client disconnected while waiting", cause)
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return nil, nil, openAIWSAccountBusyError()
	}
	return nil, nil, service.NewOpenAIWSClientCloseError(coderws.StatusInternalError, "failed to acquire account concurrency slot", err)
}

// openAIWSUserWaitTimeout 沿用 HTTP 路径的用户槽等待上限；测试缩短用。
var openAIWSUserWaitTimeout = maxConcurrencyWait

// canWaitUser 决定用户槽拿不到时能否排队。首轮在选号之前抢用户槽，账号模式尚未解析，
// 首帧一直由网关持有、未发出，因此不看模式一律可等；后续轮与重试阶段沿用账号槽的规则。
func (b *openAIWSAccountWaitBudget) canWaitUser(mode, phase string) bool {
	if b.waitDisabled {
		return false
	}
	if phase == openAIWSAccountWaitPhaseInitial {
		return true
	}
	return b.canWait(mode, phase)
}

func openAIWSUserBusyError() error {
	return service.NewOpenAIWSClientCloseError(coderws.StatusTryAgainLater, "too many concurrent requests, please retry later", nil)
}

// acquireWSUserSlot 与 acquireWSAccountSlot 同构：先试抢，允许等时按 HTTP 常量排队。
// 用户槽等待独立计时，不占 budget 的轮级截止时间，否则账号槽的 120 秒会被压到 30 秒以内。
// beforeWait 在真正进入等待前调用，供后续轮先释放挂起的账号槽（不持账号槽等用户槽）。
func (h *OpenAIGatewayHandler) acquireWSUserSlot(ctx context.Context, userID int64, maxConcurrency int, apiKeyID int64, budget *openAIWSAccountWaitBudget, mode, phase string, beforeWait func(), log *zap.Logger) (func(), error) {
	if ctx.Err() != nil {
		return nil, context.Cause(ctx)
	}
	release, acquired, err := h.concurrencyHelper.TryAcquireUserSlotForAPIKey(ctx, userID, maxConcurrency, apiKeyID)
	if err != nil {
		return nil, service.NewOpenAIWSClientCloseError(coderws.StatusInternalError, "failed to acquire user concurrency slot", err)
	}
	if acquired {
		return release, nil
	}
	if !budget.canWaitUser(mode, phase) {
		return nil, openAIWSUserBusyError()
	}
	queueLimit := service.CalculateMaxWait(maxConcurrency) - maxConcurrency
	if queueLimit < 1 {
		queueLimit = 1
	}
	canWait, err := h.concurrencyHelper.IncrementWaitCount(ctx, userID, queueLimit)
	if err != nil {
		return nil, service.NewOpenAIWSClientCloseError(coderws.StatusInternalError, "failed to enter user wait queue", err)
	}
	if !canWait {
		log.Info("openai.websocket_user_wait_finished", zap.Int64("user_id", userID), zap.String("mode", mode), zap.String("phase", phase), zap.String("reason", "queue_full"))
		return nil, openAIWSUserBusyError()
	}
	defer h.concurrencyHelper.DecrementWaitCount(ctx, userID)
	if beforeWait != nil {
		beforeWait()
	}
	observedCtx, endObservation := service.BeginOpenAIWSAccountWait(ctx)
	defer endObservation()
	waitCtx, cancel := context.WithTimeout(observedCtx, openAIWSUserWaitTimeout)
	defer cancel()
	started := time.Now()
	log.Info("openai.websocket_user_wait_started", zap.Int64("user_id", userID), zap.String("mode", mode), zap.String("phase", phase), zap.Int("turn", budget.turn), zap.Int("max_concurrency", maxConcurrency), zap.Int("max_waiting", queueLimit), zap.Int64("budget_ms", openAIWSUserWaitTimeout.Milliseconds()))
	release, err = waitForConcurrencySlot(waitCtx, func() (*service.AcquireResult, error) {
		return h.concurrencyHelper.concurrencyService.AcquireUserSlot(waitCtx, userID, maxConcurrency)
	}, nil, nil)
	if err == nil && waitCtx.Err() != nil {
		if release != nil {
			release()
		}
		err = context.Cause(waitCtx)
	}
	reason := "acquired"
	if err != nil {
		switch {
		case ctx.Err() != nil:
			reason = "canceled"
		case observedCtx.Err() != nil:
			reason = "client_closed"
		case errors.Is(err, context.DeadlineExceeded):
			reason = "wait_timeout"
		default:
			reason = "acquire_error"
		}
	}
	log.Info("openai.websocket_user_wait_finished", zap.Int64("user_id", userID), zap.String("mode", mode), zap.String("phase", phase), zap.Int("turn", budget.turn), zap.Int64("wait_ms", time.Since(started).Milliseconds()), zap.String("reason", reason))
	if err == nil {
		return h.concurrencyHelper.withAPIKeySlot(ctx, apiKeyID, release), nil
	}
	if ctx.Err() != nil {
		return nil, context.Cause(ctx)
	}
	if observedCtx.Err() != nil {
		cause := context.Cause(observedCtx)
		var closeErr *service.OpenAIWSClientCloseError
		if errors.As(cause, &closeErr) {
			return nil, closeErr
		}
		return nil, service.NewOpenAIWSClientCloseError(coderws.StatusGoingAway, "websocket client disconnected while waiting", cause)
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return nil, openAIWSUserBusyError()
	}
	return nil, service.NewOpenAIWSClientCloseError(coderws.StatusInternalError, "failed to acquire user concurrency slot", err)
}
