package service

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
)

const (
	usageBillingLaneShardCount = 64
	// usageBillingLaneMaxBatchSize 是单个合并事务最多包含的命令数。
	usageBillingLaneMaxBatchSize        = 128
	usageBillingLaneFallbackLogInterval = time.Minute
)

var errUsageBillingLaneBatchOutcomeMismatch = errors.New("usage billing batch outcome count mismatch")

// UsageBillingLaneCombiner 是按用户合并计费事务的 UsageBillingRepository。
//
// 每个用户一条通道：通道空闲时命令立即按单条 Apply 结算；结算进行中到达的命令排队，
// 上一轮结束后整批（至多 maxBatch 条）交给 ApplyBatch 在一个事务内结算。每个调用方阻塞等待
// 并拿到自己那条命令的结果。ApplyBatch 返回 error 时整批逐条 Apply。
// 每次结算（单条、整批、整批回退后的逐条）的超时从开始结算时计算，排队时间不计入。
//
// 批量生图的冻结/结算/释放直接调用底层仓储。
// 通道只在本进程内合并同一用户的计费；结算的原子性与幂等性由底层仓储的事务提供。
type UsageBillingLaneCombiner struct {
	repo     UsageBillingBatchRepository
	maxBatch int

	shards [usageBillingLaneShardCount]usageBillingLaneShard
	wg     sync.WaitGroup

	singleApplies   atomic.Uint64
	batches         atomic.Uint64
	batchedCommands atomic.Uint64
	batchFallbacks  atomic.Uint64
	maxBatchSeen    atomic.Uint64
	lastFallbackLog atomic.Int64
}

// usageBillingLaneShard 保护一组用户的通道。通道在 lanes 中存在当且仅当它的结算 goroutine 在运行。
type usageBillingLaneShard struct {
	mu     sync.Mutex
	lanes  map[int64]*usageBillingLane
	closed bool
}

type usageBillingLane struct {
	pending []*usageBillingLaneRequest
}

type usageBillingLaneRequest struct {
	ctx       context.Context
	cmd       *UsageBillingCommand
	done      chan UsageBillingBatchOutcome
	delivered bool
}

// UsageBillingLaneStats 是通道合并器的累计计数。
type UsageBillingLaneStats struct {
	SingleApplies   uint64
	Batches         uint64
	BatchedCommands uint64
	BatchFallbacks  uint64
	MaxBatchSize    uint64
}

// NewUsageBillingLaneCombiner 创建通道合并器。
func NewUsageBillingLaneCombiner(repo UsageBillingBatchRepository) *UsageBillingLaneCombiner {
	return newUsageBillingLaneCombiner(repo, usageBillingLaneMaxBatchSize)
}

func newUsageBillingLaneCombiner(repo UsageBillingBatchRepository, maxBatch int) *UsageBillingLaneCombiner {
	c := &UsageBillingLaneCombiner{repo: repo, maxBatch: maxBatch}
	for i := range c.shards {
		c.shards[i].lanes = make(map[int64]*usageBillingLane)
	}
	return c
}

// Apply 结算一条计费命令：进入该用户的通道并等待结果，结果与直接调用底层 Apply 一致。
// 缺少 request_id 的命令与 Stop 之后的命令直接调用底层 Apply。
func (c *UsageBillingLaneCombiner) Apply(ctx context.Context, cmd *UsageBillingCommand) (*UsageBillingApplyResult, error) {
	if cmd == nil {
		return c.repo.Apply(ctx, cmd)
	}
	cmd.Normalize()
	if cmd.RequestID == "" {
		return c.repo.Apply(ctx, cmd)
	}
	if ctx == nil {
		ctx = context.Background()
	}
	req := &usageBillingLaneRequest{ctx: ctx, cmd: cmd, done: make(chan UsageBillingBatchOutcome, 1)}
	if !c.enqueue(req) {
		return c.repo.Apply(ctx, cmd)
	}
	// 入队后命令可能已在执行中的事务里，必须等到结果，不能以单条 Apply 另行结算。
	out := <-req.done
	return out.Result, out.Err
}

func (c *UsageBillingLaneCombiner) ReserveBatchImageBalance(ctx context.Context, cmd *BatchImageBalanceHoldCommand) (*BatchImageBalanceHoldResult, error) {
	return c.repo.ReserveBatchImageBalance(ctx, cmd)
}

func (c *UsageBillingLaneCombiner) CaptureBatchImageBalance(ctx context.Context, cmd *BatchImageBalanceHoldCommand) (*BatchImageBalanceHoldResult, error) {
	return c.repo.CaptureBatchImageBalance(ctx, cmd)
}

func (c *UsageBillingLaneCombiner) ReleaseBatchImageBalance(ctx context.Context, cmd *BatchImageBalanceHoldCommand) (*BatchImageBalanceHoldResult, error) {
	return c.repo.ReleaseBatchImageBalance(ctx, cmd)
}

// Stop 拒收新命令（之后的 Apply 直接调用底层仓储），并等待所有通道结算完已排队的命令。可重复调用。
func (c *UsageBillingLaneCombiner) Stop() {
	if c == nil {
		return
	}
	for i := range c.shards {
		shard := &c.shards[i]
		shard.mu.Lock()
		shard.closed = true
		shard.mu.Unlock()
	}
	c.wg.Wait()
	stats := c.Stats()
	logger.LegacyPrintf("service.usage_billing_lane", "[UsageBillingLane] stopped: single=%d batches=%d batched_commands=%d fallbacks=%d max_batch=%d",
		stats.SingleApplies, stats.Batches, stats.BatchedCommands, stats.BatchFallbacks, stats.MaxBatchSize)
}

// Stats 返回累计计数。
func (c *UsageBillingLaneCombiner) Stats() UsageBillingLaneStats {
	return UsageBillingLaneStats{
		SingleApplies:   c.singleApplies.Load(),
		Batches:         c.batches.Load(),
		BatchedCommands: c.batchedCommands.Load(),
		BatchFallbacks:  c.batchFallbacks.Load(),
		MaxBatchSize:    c.maxBatchSeen.Load(),
	}
}

func (c *UsageBillingLaneCombiner) shardFor(userID int64) *usageBillingLaneShard {
	return &c.shards[uint64(userID)%usageBillingLaneShardCount]
}

// enqueue 把命令追加到用户通道；通道不存在时创建并启动结算 goroutine。已停止时返回 false。
func (c *UsageBillingLaneCombiner) enqueue(req *usageBillingLaneRequest) bool {
	userID := req.cmd.UserID
	shard := c.shardFor(userID)
	shard.mu.Lock()
	defer shard.mu.Unlock()
	if shard.closed {
		return false
	}
	if lane, ok := shard.lanes[userID]; ok {
		lane.pending = append(lane.pending, req)
		return true
	}
	lane := &usageBillingLane{pending: []*usageBillingLaneRequest{req}}
	shard.lanes[userID] = lane
	c.wg.Add(1)
	go c.drain(shard, userID, lane)
	return true
}

// drain 循环取出通道中排队的命令结算，队列为空时在同一把锁内移除通道并退出。
func (c *UsageBillingLaneCombiner) drain(shard *usageBillingLaneShard, userID int64, lane *usageBillingLane) {
	defer c.wg.Done()
	for {
		batch := c.take(shard, userID, lane)
		if len(batch) == 0 {
			return
		}
		c.execute(batch)
	}
}

func (c *UsageBillingLaneCombiner) take(shard *usageBillingLaneShard, userID int64, lane *usageBillingLane) []*usageBillingLaneRequest {
	shard.mu.Lock()
	defer shard.mu.Unlock()
	n := len(lane.pending)
	if n == 0 {
		delete(shard.lanes, userID)
		return nil
	}
	if n <= c.maxBatch {
		batch := lane.pending
		lane.pending = nil
		return batch
	}
	batch := lane.pending[:c.maxBatch:c.maxBatch]
	lane.pending = append([]*usageBillingLaneRequest(nil), lane.pending[c.maxBatch:]...)
	return batch
}

// execute 结算一批命令并把结果交给各自的调用方；任何路径（含 panic）下每个请求都恰好收到一次结果。
func (c *UsageBillingLaneCombiner) execute(batch []*usageBillingLaneRequest) {
	defer func() {
		if r := recover(); r != nil {
			logger.LegacyPrintf("service.usage_billing_lane", "[UsageBillingLane] ALERT: panic while settling user=%d batch=%d: %v",
				batch[0].cmd.UserID, len(batch), r)
			err := fmt.Errorf("usage billing lane panic: %v", r)
			for _, req := range batch {
				deliverUsageBillingLaneResult(req, nil, err)
			}
		}
	}()

	if len(batch) == 1 {
		c.singleApplies.Add(1)
		c.applyOne(batch[0], time.Now().Add(postUsageBillingTimeout))
		return
	}

	c.recordBatch(len(batch))
	cmds := make([]*UsageBillingCommand, len(batch))
	for i, req := range batch {
		cmds[i] = req.cmd
	}
	outcomes, err := c.applyBatch(batch[0].ctx, cmds)
	if err == nil && len(outcomes) != len(batch) {
		err = errUsageBillingLaneBatchOutcomeMismatch
	}
	if err == nil {
		for i, req := range batch {
			deliverUsageBillingLaneResult(req, outcomes[i].Result, outcomes[i].Err)
		}
		return
	}

	c.logBatchFallback(batch[0].cmd.UserID, len(batch), err)
	// 整批逐条结算共用一个截止时间，数据库不可用时整批在一个超时内结束。
	deadline := time.Now().Add(postUsageBillingTimeout)
	for _, req := range batch {
		c.applyOne(req, deadline)
	}
}

// applyOne 以单条 Apply 结算一个请求并交付结果。上下文保留请求上下文中的值，截止时间为 deadline；
// panic 视同该请求返回 error。
func (c *UsageBillingLaneCombiner) applyOne(req *usageBillingLaneRequest, deadline time.Time) {
	ctx, cancel := context.WithDeadline(context.WithoutCancel(req.ctx), deadline)
	defer cancel()
	res, err := func() (res *UsageBillingApplyResult, err error) {
		defer func() {
			if r := recover(); r != nil {
				logger.LegacyPrintf("service.usage_billing_lane", "[UsageBillingLane] ALERT: panic while applying user=%d request=%s: %v",
					req.cmd.UserID, req.cmd.RequestID, r)
				res, err = nil, fmt.Errorf("usage billing apply panic: %v", r)
			}
		}()
		return c.repo.Apply(ctx, req.cmd)
	}()
	deliverUsageBillingLaneResult(req, res, err)
}

// applyBatch 调用底层 ApplyBatch，panic 视同返回 error（事务已由底层回滚）。
func (c *UsageBillingLaneCombiner) applyBatch(ctx context.Context, cmds []*UsageBillingCommand) (outcomes []UsageBillingBatchOutcome, err error) {
	batchCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), postUsageBillingTimeout)
	defer cancel()
	defer func() {
		if r := recover(); r != nil {
			logger.LegacyPrintf("service.usage_billing_lane", "[UsageBillingLane] ALERT: panic while settling user=%d batch=%d: %v",
				cmds[0].UserID, len(cmds), r)
			outcomes, err = nil, fmt.Errorf("usage billing batch panic: %v", r)
		}
	}()
	return c.repo.ApplyBatch(batchCtx, cmds)
}

// logBatchFallback 计数并记录一次整批回退；日志每分钟至多一条，带累计回退次数。
func (c *UsageBillingLaneCombiner) logBatchFallback(userID int64, size int, err error) {
	total := c.batchFallbacks.Add(1)
	now := time.Now().UnixNano()
	last := c.lastFallbackLog.Load()
	if now-last < int64(usageBillingLaneFallbackLogInterval) || !c.lastFallbackLog.CompareAndSwap(last, now) {
		return
	}
	logger.LegacyPrintf("service.usage_billing_lane", "[UsageBillingLane] batch settle failed, applying one by one: user=%d batch=%d total_fallbacks=%d err=%v",
		userID, size, total, err)
}

func (c *UsageBillingLaneCombiner) recordBatch(size int) {
	c.batches.Add(1)
	c.batchedCommands.Add(uint64(size))
	for {
		prev := c.maxBatchSeen.Load()
		if uint64(size) <= prev || c.maxBatchSeen.CompareAndSwap(prev, uint64(size)) {
			return
		}
	}
}

// deliverUsageBillingLaneResult 把结果交给请求方；已交付过的请求忽略。只在通道的结算 goroutine 中调用。
func deliverUsageBillingLaneResult(req *usageBillingLaneRequest, res *UsageBillingApplyResult, err error) {
	if req.delivered {
		return
	}
	req.delivered = true
	if err != nil {
		res = nil
	}
	req.done <- UsageBillingBatchOutcome{Result: res, Err: err}
}
