//go:build unit

package service

import (
	"context"
	"errors"
	"sync"
	"time"
)

// 本文件汇总 service_quota_* 单元测试共享的 fake 实现，避免在 two_phase /
// validation / monitor / field_validation / reset_residual / deferred_batch 等
// 多个 *_test.go 中各自维护一份近乎相同的 ServiceQuotaRuleRepository / ServiceQuotaLimiter
// 实现（参考 CLAUDE.md「测试 fixture 也要复用」原则）。
//
// 设计要点：
//   - fakeServiceQuotaRepo：rules 用于 List；accounts/groups/channels 用于 Fetch*Scope；
//     写路径（Create/Update/Delete）默认返回 not-supported error，由调用方按需断言不被触发。
//   - fakeServiceQuotaLimiter：内存版限流器，counters 与 concurrency 均为 map；并暴露
//     acquireCalls / incrementCalls / snapshotManyCalls 等调用记录字段，方便测试断言。
//     snapshots 与 manyErr 用于 monitor 路径模拟"已存在 key 的快照"或注入错误。

// ─── ServiceQuotaRuleRepository ───

// fakeServiceQuotaRepo 提供一个无 DB 依赖的 ServiceQuotaRuleRepository 实现。
//
// 调用约定：
//   - List 返回 rules 的浅拷贝；
//   - FetchAccountScope/FetchGroupScope/FetchChannelScope 在对应 map 命中时返回数据，
//     未命中时返回 nil（与生产实现一致——"未找到"用 nil 表示，不返回 error）；
//   - Create/Update/Delete 默认返回 error（测试不触及写路径，触及即被显式断言）；
//   - FetchPathIDsByOwner 默认返回 nil/nil（多数测试不关心残留 path 清理）。
type fakeServiceQuotaRepo struct {
	rules    []*ServiceQuotaRule
	accounts map[int64]*AccountScopeInfo
	groups   map[int64]*GroupScopeInfo
	channels map[int64]*ChannelScopeInfo
}

func (f *fakeServiceQuotaRepo) List(_ context.Context, _ ServiceQuotaListFilter) ([]*ServiceQuotaRule, error) {
	return append([]*ServiceQuotaRule(nil), f.rules...), nil
}

func (f *fakeServiceQuotaRepo) Create(_ context.Context, _ ServiceQuotaRuleInput) (*ServiceQuotaRule, error) {
	return nil, errors.New("fakeServiceQuotaRepo: Create not supported")
}

func (f *fakeServiceQuotaRepo) Update(_ context.Context, _ int64, _ ServiceQuotaRuleInput) (*ServiceQuotaRule, error) {
	return nil, errors.New("fakeServiceQuotaRepo: Update not supported")
}

func (f *fakeServiceQuotaRepo) Delete(_ context.Context, _ int64) error {
	return errors.New("fakeServiceQuotaRepo: Delete not supported")
}

func (f *fakeServiceQuotaRepo) FetchAccountScope(_ context.Context, id int64) (*AccountScopeInfo, error) {
	return f.accounts[id], nil
}

func (f *fakeServiceQuotaRepo) FetchGroupScope(_ context.Context, id int64) (*GroupScopeInfo, error) {
	return f.groups[id], nil
}

func (f *fakeServiceQuotaRepo) FetchChannelScope(_ context.Context, id int64) (*ChannelScopeInfo, error) {
	return f.channels[id], nil
}

func (f *fakeServiceQuotaRepo) FetchPathIDsByOwner(_ context.Context, _ string, _ int64) ([]int64, error) {
	return nil, nil
}

// ─── ServiceQuotaLimiter ───

// fakeServiceQuotaLimiter 是内存版 ServiceQuotaLimiter 实现。
//
// 调用约定：
//   - counters / concurrency 是 fixed-window 计数 + concurrency 槽位的最小 in-memory 模拟；
//   - snapshots 是 monitor 测试预置的"key→snapshot"映射，让 SnapshotMany 能直接返回固定结果；
//   - manyErr 非 nil 时 SnapshotMany 直接返回该错误，方便测试 fail-soft 路径；
//   - acquireCalls / incrementCalls / snapshotManyCalls 记录调用，便于断言路径命中情况。
type fakeServiceQuotaLimiter struct {
	mu          sync.Mutex
	counters    map[string]float64
	concurrency map[string]map[string]struct{} // key -> set of members
	snapshots   map[string]LimiterSnapshot     // monitor 测试预置的 key→snapshot
	manyErr     error                          // SnapshotMany 注入的错误（fail-soft 测试）

	acquireCalls      []string
	incrementCalls    []string
	snapshotManyCalls [][]SnapshotKey
}

func newFakeLimiter() *fakeServiceQuotaLimiter {
	return &fakeServiceQuotaLimiter{
		counters:    map[string]float64{},
		concurrency: map[string]map[string]struct{}{},
		snapshots:   map[string]LimiterSnapshot{},
	}
}

func (f *fakeServiceQuotaLimiter) Current(_ context.Context, key string, _ time.Duration, _ string) (float64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.counters[key], nil
}

func (f *fakeServiceQuotaLimiter) Increment(_ context.Context, key string, delta float64, _ time.Duration, _ string) (float64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.counters[key] += delta
	f.incrementCalls = append(f.incrementCalls, key)
	return f.counters[key], nil
}

func (f *fakeServiceQuotaLimiter) Acquire(_ context.Context, key, member string, limit int64) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.acquireCalls = append(f.acquireCalls, key)
	set, ok := f.concurrency[key]
	if !ok {
		set = map[string]struct{}{}
		f.concurrency[key] = set
	}
	if int64(len(set)) >= limit {
		return false, nil
	}
	set[member] = struct{}{}
	return true, nil
}

func (f *fakeServiceQuotaLimiter) Release(_ context.Context, key, member string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if set, ok := f.concurrency[key]; ok {
		delete(set, member)
	}
	return nil
}

// Snapshot 优先返回 snapshots 中的预置值（monitor 路径），否则按 counters 推导。
func (f *fakeServiceQuotaLimiter) Snapshot(_ context.Context, key string, _ time.Duration, _ string) (LimiterSnapshot, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if v, ok := f.snapshots[key]; ok {
		return v, nil
	}
	if v, ok := f.counters[key]; ok {
		return LimiterSnapshot{Current: v, Exists: true}, nil
	}
	return LimiterSnapshot{}, nil
}

// SnapshotMany 同时记录调用 + 优先用 snapshots 预置值，再退化到 counters。
func (f *fakeServiceQuotaLimiter) SnapshotMany(_ context.Context, keys []SnapshotKey) ([]LimiterSnapshot, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.snapshotManyCalls = append(f.snapshotManyCalls, append([]SnapshotKey(nil), keys...))
	if f.manyErr != nil {
		return nil, f.manyErr
	}
	if len(keys) == 0 {
		return nil, nil
	}
	out := make([]LimiterSnapshot, len(keys))
	for i, k := range keys {
		if v, ok := f.snapshots[k.Key]; ok {
			out[i] = v
			continue
		}
		if v, ok := f.counters[k.Key]; ok {
			out[i] = LimiterSnapshot{Current: v, Exists: true}
		}
	}
	return out, nil
}

// Reset 模拟 DEL：从内存 map 中清掉对应 key 的计数与并发集合。
func (f *fakeServiceQuotaLimiter) Reset(_ context.Context, key string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.counters, key)
	delete(f.concurrency, key)
	delete(f.snapshots, key)
	return nil
}

// ResetPattern 模拟 SCAN+DEL：对 counters / concurrency / snapshots 三张表
// 都按 pattern 删除命中的 key。
func (f *fakeServiceQuotaLimiter) ResetPattern(_ context.Context, pattern string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for k := range f.counters {
		if matchesGlobPattern(k, pattern) {
			delete(f.counters, k)
		}
	}
	for k := range f.concurrency {
		if matchesGlobPattern(k, pattern) {
			delete(f.concurrency, k)
		}
	}
	for k := range f.snapshots {
		if matchesGlobPattern(k, pattern) {
			delete(f.snapshots, k)
		}
	}
	return nil
}
