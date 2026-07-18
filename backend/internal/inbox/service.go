package inbox

import (
	"context"
	"encoding/json"
	"math"
	"sort"
	"time"
)

// 默认配置常量。
const (
	// defaultRetention 是消息保留期。超过此期限的消息由清理 job 删除，catchup 也不再返回。
	defaultRetention = 30 * 24 * time.Hour
	// defaultCatchupLimit 是单次 catchup 返回的最大消息数（分页页大小）。
	defaultCatchupLimit = 200
	// defaultUnackedLimit 是未读列表（推送 unacked / badge 计数）的最大条数。
	defaultUnackedLimit = 100
	// defaultBroadcastScan 是每次求值时扫描的广播候选上限（targeting 过滤前）。
	defaultBroadcastScan = 500
)

// maxSeqValue 作为区间上界代表"当前所有已分配 seq"，用于未读派生查询。
const maxSeqValue = math.MaxInt64

// Config 是 Service 的可调参数。零值字段回退到默认值。
type Config struct {
	Retention     time.Duration
	CatchupLimit  int
	UnackedLimit  int
	BroadcastScan int
}

func (c Config) withDefaults() Config {
	if c.Retention <= 0 {
		c.Retention = defaultRetention
	}
	if c.CatchupLimit <= 0 {
		c.CatchupLimit = defaultCatchupLimit
	}
	if c.UnackedLimit <= 0 {
		c.UnackedLimit = defaultUnackedLimit
	}
	if c.BroadcastScan <= 0 {
		c.BroadcastScan = defaultBroadcastScan
	}
	return c
}

// Service 是信箱的读取侧应用服务：catchup 补齐、ack 抬升水位、未读派生、推送帧构造。
// 写入侧（发布）见 Publisher。
type Service struct {
	repo    Repository
	seq     SeqSource
	attrs   AttributeProvider
	metrics Metrics
	cfg     Config
	now     func() time.Time
}

// NewService 构造读取侧服务。attrs / metrics 为 nil 时使用 noop 实现。
func NewService(repo Repository, seq SeqSource, attrs AttributeProvider, metrics Metrics, cfg Config) *Service {
	if attrs == nil {
		attrs = NewNoopAttributeProvider()
	}
	if metrics == nil {
		metrics = NewNoopMetrics()
	}
	return &Service{
		repo:    repo,
		seq:     seq,
		attrs:   attrs,
		metrics: metrics,
		cfg:     cfg.withDefaults(),
		now:     time.Now,
	}
}

// Catchup 拉取用户自 sinceClient 之后、保留期内、且高于服务端 ack 水位的消息。
//
// 首次访问（无水位记录）时懒初始化 acked_seq 为当前 seq 高水位，使新用户从"当下"
// 开始接收，不倒灌历史积压。返回的 AckedSeq 为服务端权威水位，客户端据此持久化
// local_ack_seq。
func (s *Service) Catchup(ctx context.Context, userID int64, sinceClient int64) (CatchupResult, error) {
	acked, found, err := s.repo.GetInboxState(ctx, userID)
	if err != nil {
		return CatchupResult{}, err
	}
	if !found {
		fresh, err := s.freshBaseline(ctx)
		if err != nil {
			return CatchupResult{}, err
		}
		if err := s.repo.InitInboxState(ctx, userID, fresh); err != nil {
			return CatchupResult{}, err
		}
		// 可能并发初始化，回读一次拿到权威水位。
		acked, _, err = s.repo.GetInboxState(ctx, userID)
		if err != nil {
			return CatchupResult{}, err
		}
	}

	// 客户端水位低于服务端 ack 水位时，以服务端为准（低于水位的消息视为已读）。
	since := sinceClient
	if acked > since {
		since = acked
	}

	limit := s.cfg.CatchupLimit
	cutoff := s.now().Add(-s.cfg.Retention)

	direct, err := s.repo.ListDirectSince(ctx, userID, since, limit+1)
	if err != nil {
		return CatchupResult{}, err
	}
	bcCandidates, err := s.repo.ListBroadcastsSince(ctx, since, cutoff, s.cfg.BroadcastScan)
	if err != nil {
		return CatchupResult{}, err
	}

	attrs, err := s.attrs.Fetch(ctx, userID)
	if err != nil {
		return CatchupResult{}, err
	}

	msgs := direct
	for _, b := range bcCandidates {
		if matchTargeting(b.Targeting, attrs) {
			msgs = append(msgs, broadcastToMessage(b))
		}
	}
	sort.Slice(msgs, func(i, j int) bool { return msgs[i].Seq < msgs[j].Seq })

	hasMore := false
	if len(msgs) > limit {
		hasMore = true
		msgs = msgs[:limit]
	}

	s.metrics.IncCatchup(len(msgs))
	return CatchupResult{Messages: msgs, AckedSeq: acked, HasMore: hasMore}, nil
}

// Ack 累积抬升用户已读水位到 seq。
func (s *Service) Ack(ctx context.Context, userID, seq int64) error {
	if seq <= 0 {
		return ErrInvalidAck
	}
	if err := s.repo.Ack(ctx, userID, seq); err != nil {
		return err
	}
	s.metrics.IncAck()
	return nil
}

// UnreadCount 返回用户未读消息数（单播 + 命中的广播），以及是否被 limit 截断。
// truncated=true 表示实际未读数不少于返回值（用于 "99+" 类展示）。
func (s *Service) UnreadCount(ctx context.Context, userID int64) (count int, truncated bool, err error) {
	seqs, truncated, err := s.UnackedSeqs(ctx, userID)
	if err != nil {
		return 0, false, err
	}
	return len(seqs), truncated, nil
}

// UnackedSeqs 派生用户当前未 ack 的消息 seq 列表（单播 + 命中广播），按 seq 升序，
// 上限 UnackedLimit。truncated 表示被截断。供未读 badge 与 WS 推送共用。
func (s *Service) UnackedSeqs(ctx context.Context, userID int64) (seqs []int64, truncated bool, err error) {
	acked, found, err := s.repo.GetInboxState(ctx, userID)
	if err != nil {
		return nil, false, err
	}
	if !found {
		return nil, false, nil
	}

	limit := s.cfg.UnackedLimit
	cutoff := s.now().Add(-s.cfg.Retention)

	directSeqs, err := s.repo.UnackedDirectSeqs(ctx, userID, acked, maxSeqValue, limit+1)
	if err != nil {
		return nil, false, err
	}
	bcCandidates, err := s.repo.UnackedBroadcasts(ctx, acked, maxSeqValue, cutoff, s.cfg.BroadcastScan)
	if err != nil {
		return nil, false, err
	}

	attrs, err := s.attrs.Fetch(ctx, userID)
	if err != nil {
		return nil, false, err
	}

	all := directSeqs
	for _, b := range bcCandidates {
		if matchTargeting(b.Targeting, attrs) {
			all = append(all, b.Seq)
		}
	}
	sort.Slice(all, func(i, j int) bool { return all[i] < all[j] })

	if len(all) > limit {
		return all[:limit], true, nil
	}
	return all, false, nil
}

// BuildPush 为一条新到达的消息构造 WS 下行帧，附带该用户当前未读 seq 列表。
func (s *Service) BuildPush(ctx context.Context, userID int64, m Message) (PushMessage, error) {
	unacked, truncated, err := s.UnackedSeqs(ctx, userID)
	if err != nil {
		return PushMessage{}, err
	}
	return PushMessage{
		Type:      PushTypeNotification,
		Seq:       m.Seq,
		Scope:     m.Scope,
		Namespace: m.Namespace,
		Unacked:   unacked,
		Truncated: truncated,
		Payload:   m.Payload,
		CreatedAt: m.CreatedAt,
	}, nil
}

// 管理端审计分页参数。
const (
	defaultAuditPageSize = 20
	maxAuditPageSize     = 100
)

// AuditPage 是管理端审计查询的分页结果（泛化于广播 / 单播）。
type AuditPage[T any] struct {
	Items    []T   `json:"items"`
	Total    int64 `json:"total"`
	Page     int   `json:"page"`
	PageSize int   `json:"page_size"`
}

// clampPaging 归一化分页参数：page>=1，pageSize∈[1,100] 默认 20，返回 limit/offset。
func clampPaging(page, pageSize int) (limit, offset, normPage, normSize int) {
	if page < 1 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = defaultAuditPageSize
	}
	if pageSize > maxAuditPageSize {
		pageSize = maxAuditPageSize
	}
	return pageSize, (page - 1) * pageSize, page, pageSize
}

// ListBroadcasts 管理端审计：按 namespace（空=全部）分页倒序列出广播。
func (s *Service) ListBroadcasts(ctx context.Context, namespace string, page, pageSize int) (AuditPage[Broadcast], error) {
	limit, offset, p, size := clampPaging(page, pageSize)
	items, total, err := s.repo.ListBroadcastsPaged(ctx, namespace, limit, offset)
	if err != nil {
		return AuditPage[Broadcast]{}, err
	}
	return AuditPage[Broadcast]{Items: items, Total: total, Page: p, PageSize: size}, nil
}

// ListDirectMessages 管理端审计：按 namespace（空=全部）与 userID（0=全部）分页倒序列出单播。
func (s *Service) ListDirectMessages(ctx context.Context, namespace string, userID int64, page, pageSize int) (AuditPage[DirectMessage], error) {
	limit, offset, p, size := clampPaging(page, pageSize)
	items, total, err := s.repo.ListDirectMessagesPaged(ctx, namespace, userID, limit, offset)
	if err != nil {
		return AuditPage[DirectMessage]{}, err
	}
	return AuditPage[DirectMessage]{Items: items, Total: total, Page: p, PageSize: size}, nil
}

// freshBaseline 为新用户分配一个"当下"的 seq 基线：取自 seq 分配器（单调），
// 保证 > 所有已存在 seq，从而新用户不会倒灌历史消息。分配器缺失时退化为 0。
func (s *Service) freshBaseline(ctx context.Context) (int64, error) {
	if s.seq == nil {
		return 0, nil
	}
	return s.seq.Next(ctx)
}

// broadcastToMessage 把广播行投影为统一的 Message 视图。
func broadcastToMessage(b Broadcast) Message {
	return Message{
		Seq:       b.Seq,
		Scope:     ScopeBroadcast,
		Namespace: b.Namespace,
		Payload:   b.Payload,
		CreatedAt: b.CreatedAt,
	}
}

// matchTargeting 解析并对用户属性求值 targeting。发布时已校验过，解析失败按不命中处理。
func matchTargeting(raw json.RawMessage, attrs map[string]any) bool {
	t, err := ParseTargeting(raw)
	if err != nil {
		return false
	}
	return t.Match(attrs)
}
