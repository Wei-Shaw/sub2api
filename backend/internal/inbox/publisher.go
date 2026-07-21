package inbox

import (
	"context"
	"encoding/json"
	"errors"
	"regexp"
	"time"
)

// maxSeqRetries 是分配-插入循环中因 seq 主键冲突而重试的最大次数。冲突极罕见（需
// 恰好撞上同一毫秒同一计数），3 次足以覆盖。
const maxSeqRetries = 3

// dedupKeyPattern 限制 dedup_key 字符集，仅允许可见 ASCII 子集，避免控制字符 / 空白
// 渗入索引键。
var dedupKeyPattern = regexp.MustCompile(`^[A-Za-z0-9_.:@=/+-]+$`)

// Notifier 在消息成功落库后被回调，用于触发实时 fan-out（WebSocket 推送）。v1 由
// WS hub / Redis pub/sub 实现；未接线时为 nil，发布仍然成功——客户端会在下次
// catchup 时补齐，实时性降级但不丢消息。
type Notifier interface {
	// NotifyDirect 通知：给 userID 新增了一条单播消息 m。
	NotifyDirect(ctx context.Context, userID int64, m Message)
	// NotifyBroadcast 通知：新增了一条广播消息 b（接收方由订阅端按 targeting 过滤）。
	NotifyBroadcast(ctx context.Context, b Broadcast)
}

// Publisher 是各业务模块发布站内消息的统一入口（发布 SDK）。实现负责入参校验、
// seq 分配、幂等落库与实时通知。
type Publisher interface {
	// PublishToUser 发布一条单播消息给指定用户。
	//   - created=true：新写入。
	//   - created=false, err=nil：dedup 幂等命中，已存在，未重复写入。
	PublishToUser(ctx context.Context, in PublishDirectInput) (created bool, seq int64, err error)

	// PublishBroadcast 发布一条广播消息。语义同上，dedup 维度为 (namespace, dedup_key)。
	PublishBroadcast(ctx context.Context, in PublishBroadcastInput) (created bool, seq int64, err error)
}

type publisher struct {
	alloc    SeqSource
	repo     Repository
	notifier Notifier // 可空
	metrics  Metrics
}

// NewPublisher 构造发布 SDK（noop metrics）。notifier 可为 nil（实时推送降级为纯 catchup）。
func NewPublisher(alloc SeqSource, repo Repository, notifier Notifier) Publisher {
	return NewPublisherWithMetrics(alloc, repo, notifier, nil)
}

// NewPublisherWithMetrics 构造发布 SDK 并注入可观测性打点。metrics 为 nil 时用 noop。
func NewPublisherWithMetrics(alloc SeqSource, repo Repository, notifier Notifier, metrics Metrics) Publisher {
	if metrics == nil {
		metrics = NewNoopMetrics()
	}
	return &publisher{alloc: alloc, repo: repo, notifier: notifier, metrics: metrics}
}

func (p *publisher) PublishToUser(ctx context.Context, in PublishDirectInput) (bool, int64, error) {
	if in.RecipientID <= 0 {
		return false, 0, ErrInvalidRecipient
	}
	if err := validateEnvelope(in.Namespace, in.DedupKey, in.Payload); err != nil {
		return false, 0, err
	}

	created, seq, err := p.allocAndInsert(ctx, func(seq int64) (bool, error) {
		return p.repo.InsertDirectMessage(ctx, seq, in)
	})
	if err != nil {
		return false, 0, err
	}
	if created {
		p.metrics.IncPublished(ScopeDirect)
		if p.notifier != nil {
			p.notifier.NotifyDirect(ctx, in.RecipientID, Message{
				Seq:       seq,
				Scope:     ScopeDirect,
				Namespace: in.Namespace,
				Payload:   in.Payload,
				CreatedAt: time.Now().UTC(),
			})
		}
	}
	return created, seq, nil
}

func (p *publisher) PublishBroadcast(ctx context.Context, in PublishBroadcastInput) (bool, int64, error) {
	if err := validateEnvelope(in.Namespace, in.DedupKey, in.Payload); err != nil {
		return false, 0, err
	}
	if err := ValidateTargeting(in.Targeting); err != nil {
		return false, 0, err
	}

	created, seq, err := p.allocAndInsert(ctx, func(seq int64) (bool, error) {
		return p.repo.InsertBroadcast(ctx, seq, in)
	})
	if err != nil {
		return false, 0, err
	}
	if created {
		p.metrics.IncPublished(ScopeBroadcast)
		if p.notifier != nil {
			p.notifier.NotifyBroadcast(ctx, Broadcast{
				Seq:       seq,
				Namespace: in.Namespace,
				DedupKey:  in.DedupKey,
				Targeting: in.Targeting,
				Payload:   in.Payload,
				CreatedAt: time.Now().UTC(),
			})
		}
	}
	return created, seq, nil
}

// allocAndInsert 执行分配-插入循环：分配 seq 后尝试落库，遇到主键冲突（ErrSeqConflict）
// 换 seq 重试，最多 maxSeqRetries 次；耗尽则返回 ErrSeqExhausted。
func (p *publisher) allocAndInsert(ctx context.Context, insert func(seq int64) (bool, error)) (bool, int64, error) {
	for attempt := 0; attempt < maxSeqRetries; attempt++ {
		seq, err := p.alloc.Next(ctx)
		if err != nil {
			return false, 0, err
		}
		created, err := insert(seq)
		switch {
		case errors.Is(err, ErrSeqConflict):
			p.metrics.IncSeqAllocRetry()
			continue // 主键冲突，换 seq 重试
		case err != nil:
			return false, 0, err
		default:
			return created, seq, nil
		}
	}
	return false, 0, ErrSeqExhausted
}

// validateEnvelope 校验所有消息共有的信封字段：namespace、dedup_key、payload。
func validateEnvelope(namespace, dedupKey string, payload json.RawMessage) error {
	if namespace == "" || len(namespace) > MaxNamespaceLen {
		return ErrInvalidNamespace
	}
	if dedupKey == "" || len(dedupKey) > MaxDedupKeyLen || !dedupKeyPattern.MatchString(dedupKey) {
		return ErrInvalidDedupKey
	}
	if len(payload) == 0 || !json.Valid(payload) {
		return ErrInvalidPayload
	}
	if len(payload) > MaxPayloadBytes {
		return ErrPayloadTooLarge
	}
	return nil
}
