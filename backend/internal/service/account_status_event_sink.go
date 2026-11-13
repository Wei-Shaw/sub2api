package service

import (
	"context"
	"log"
	"strings"
	"sync/atomic"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
)

// 账号状态变更「事件出口」。
//
// 为什么用全局 sink 而不是构造注入：状态转移发生在 RateLimitService 与
// TokenRefreshService 的 notifyAccountSchedulingBlocked（v0.2.19 上共 31 个调用点，分布在 8 个文件），
// 给它们各塞一个 OpsRepository 需要改 wire 装配；而事件写入本身是 best-effort
// 的旁路，用 sink 可以把改动面压到「一个注册点 + 两个出口」，且默认 no-op，
// 不会影响任何单测构造的 service 实例。
//
// 约束：sink 必须异步、有超时、失败只记日志 —— 它绝不能阻塞或失败请求路径。

// AccountStatusEventSink 由 OpsService 实现。
type AccountStatusEventSink interface {
	RecordAccountStatusEvents(ctx context.Context, events []*OpsAccountStatusEventRecord) error
}

var accountStatusEventSink atomic.Value // 保存 AccountStatusEventSink

// SetAccountStatusEventSink 注册事件出口（由 NewOpsService 调用）。
func SetAccountStatusEventSink(sink AccountStatusEventSink) {
	if sink == nil {
		accountStatusEventSink = atomic.Value{}
		return
	}
	accountStatusEventSink.Store(sink)
}

func currentAccountStatusEventSink() AccountStatusEventSink {
	v, ok := accountStatusEventSink.Load().(AccountStatusEventSink)
	if !ok {
		return nil
	}
	return v
}

// accountStatusEventTimeout 限制旁路写入耗时，避免慢库拖住调用方。
const accountStatusEventTimeout = 5 * time.Second

// EmitAccountStatusEvent 异步落一条状态变更事件。调用方不需要（也不应该）等待。
func EmitAccountStatusEvent(ev *OpsAccountStatusEventRecord) {
	if ev == nil {
		return
	}
	sink := currentAccountStatusEventSink()
	if sink == nil {
		return
	}
	if ev.CreatedAt.IsZero() {
		ev.CreatedAt = time.Now()
	}
	if ev.Source == "" {
		ev.Source = "classify"
	}

	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("[Ops] account status event sink panic: %v", r)
			}
		}()
		ctx, cancel := context.WithTimeout(context.Background(), accountStatusEventTimeout)
		defer cancel()
		if err := sink.RecordAccountStatusEvents(ctx, []*OpsAccountStatusEventRecord{ev}); err != nil {
			log.Printf("[Ops] account status event write failed: account=%d to=%s reason=%s err=%v",
				ev.AccountID, ev.ToState, derefString(ev.Reason), err)
		}
	}()
}

// requestIDFromContext 取出本次请求的 request_id（由中间件写入 ctx）。
//
// 让「账号为什么被摘」能回指到具体请求：状态事件带上 request_id 后，排障时可以
// 从一条报错请求直接看到它触发了哪次账号下线，反过来也行。后台任务（token 刷新等）
// 没有请求上下文，返回空串，事件照常写入。
func requestIDFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	if v, ok := ctx.Value(ctxkey.RequestID).(string); ok {
		return strings.TrimSpace(v)
	}
	return ""
}

// accountStatusEventFromBlocking 把「这个账号被挡到 until 为止」翻译成状态事件。
//
// 语义映射（与 internal/upstream.State 对齐）：
//   - until 为零值  → 该账号被摘除（invalid），需要刷新凭据或人工处理
//   - until 非零值  → 冷却（cooling），到期自动回到可调度
func accountStatusEventFromBlocking(ctx context.Context, account *Account, until time.Time, reason, source string) *OpsAccountStatusEventRecord {
	if account == nil || account.ID <= 0 {
		return nil
	}
	ev := &OpsAccountStatusEventRecord{
		AccountID: account.ID,
		Source:    source,
		CreatedAt: time.Now(),
	}
	if requestID := requestIDFromContext(ctx); requestID != "" {
		ev.RequestID = &requestID
	}
	if r := reason; r != "" {
		ev.Reason = &r
	}
	// from_state 记录持久化状态（active/disabled/error），to_state 记录调度语义状态，
	// 两者刻意分开：库里的 status 是"配置态"，冷却/摘除是"调度态"。
	if st := account.Status; st != "" {
		s := st
		ev.FromState = &s
	}
	if until.IsZero() {
		ev.ToState = "invalid"
	} else {
		ev.ToState = "cooling"
		t := until
		ev.CoolingUntil = &t
	}
	return ev
}

func derefString(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
