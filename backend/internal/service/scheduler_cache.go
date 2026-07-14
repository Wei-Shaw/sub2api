package service

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"
)

const (
	SchedulerModeSingle = "single"
	SchedulerModeMixed  = "mixed"
	SchedulerModeForced = "forced"

	SchedulerEngineLegacy = "legacy"
	SchedulerEngineV2     = "v2"

	SchedulerEngineStatusDisabled = "disabled"
	SchedulerEngineStatusBuilding = "building"
	SchedulerEngineStatusActive   = "active"
	SchedulerEngineStatusFailed   = "failed"

	DefaultSchedulerCandidateFetchLimit = 64
	DefaultSchedulerCandidateScanLimit  = 256
	MinSchedulerCandidateFetchLimit     = 1
	MaxSchedulerCandidateFetchLimit     = 4096
	MaxSchedulerCandidateScanLimit      = 65536
	// SchedulerCandidateIndexVersion must be incremented whenever an index-time
	// score factor or its source metadata changes. Old indexes then become cache
	// misses and are rebuilt before use.
	SchedulerCandidateIndexVersion = "2"
)

func ValidateSchedulerV2Limits(candidateLimit, scanLimit int) error {
	if candidateLimit < MinSchedulerCandidateFetchLimit || candidateLimit > MaxSchedulerCandidateFetchLimit {
		return fmt.Errorf("scheduler v2 candidate limit must be between %d and %d", MinSchedulerCandidateFetchLimit, MaxSchedulerCandidateFetchLimit)
	}
	if scanLimit < candidateLimit {
		return fmt.Errorf("scheduler v2 scan limit must be greater than or equal to candidate limit")
	}
	if scanLimit > MaxSchedulerCandidateScanLimit {
		return fmt.Errorf("scheduler v2 scan limit must not exceed %d", MaxSchedulerCandidateScanLimit)
	}
	return nil
}

type SchedulerBucket struct {
	GroupID  int64
	Platform string
	Mode     string
}

type SchedulerEngineState struct {
	Engine         string
	Status         string
	LastError      string
	CandidateLimit int
	ScanLimit      int
}

func (s SchedulerEngineState) V2Enabled() bool {
	return s.Engine == SchedulerEngineV2
}

type SchedulerCandidatePage struct {
	Accounts   []*Account
	NextOffset int64
	Done       bool
}

func (b SchedulerBucket) String() string {
	return fmt.Sprintf("%d:%s:%s", b.GroupID, b.Platform, b.Mode)
}

func ParseSchedulerBucket(raw string) (SchedulerBucket, bool) {
	parts := strings.Split(raw, ":")
	if len(parts) != 3 {
		return SchedulerBucket{}, false
	}
	groupID, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return SchedulerBucket{}, false
	}
	if parts[1] == "" || parts[2] == "" {
		return SchedulerBucket{}, false
	}
	return SchedulerBucket{
		GroupID:  groupID,
		Platform: parts[1],
		Mode:     parts[2],
	}, true
}

// SchedulerCache 负责调度快照与账号快照的缓存读写。
type SchedulerCache interface {
	// GetSnapshot 读取快照并返回命中与否（ready + active + 数据完整）。
	GetSnapshot(ctx context.Context, bucket SchedulerBucket) ([]*Account, bool, error)
	// SetSnapshot 写入快照并切换激活版本。
	SetSnapshot(ctx context.Context, bucket SchedulerBucket, accounts []Account) error
	// GetAccount 获取单账号快照。
	GetAccount(ctx context.Context, accountID int64) (*Account, error)
	// SetAccount 写入单账号快照（包含不可调度状态）。
	SetAccount(ctx context.Context, account *Account) error
	// DeleteAccount 删除单账号快照。
	DeleteAccount(ctx context.Context, accountID int64) error
	// UpdateLastUsed 批量更新账号的最后使用时间。
	UpdateLastUsed(ctx context.Context, updates map[int64]time.Time) error
	// TryLockBucket 尝试获取分桶重建锁。
	TryLockBucket(ctx context.Context, bucket SchedulerBucket, ttl time.Duration) (bool, error)
	// UnlockBucket 释放分桶重建锁。
	UnlockBucket(ctx context.Context, bucket SchedulerBucket) error
	// ListBuckets 返回已注册的分桶集合。
	ListBuckets(ctx context.Context) ([]SchedulerBucket, error)
	// GetOutboxWatermark 读取 outbox 水位。
	GetOutboxWatermark(ctx context.Context) (int64, error)
	// SetOutboxWatermark 保存 outbox 水位。
	SetOutboxWatermark(ctx context.Context, id int64) error
}

// SchedulerV2Cache is optional so legacy test doubles and deployments can keep
// implementing SchedulerCache. Enabling v2 without this capability fails
// closed and never falls back to the legacy snapshot or database path.
type SchedulerV2Cache interface {
	SchedulerCache
	GetSchedulerEngineState(ctx context.Context) (SchedulerEngineState, error)
	SetSchedulerEngineState(ctx context.Context, state SchedulerEngineState) error
	CompareAndSetSchedulerEngineState(ctx context.Context, expectedEngine, expectedStatus string, state SchedulerEngineState) (bool, error)
	SetSchedulerV2Limits(ctx context.Context, candidateLimit, scanLimit int) error
	GetCandidatePage(ctx context.Context, bucket SchedulerBucket, offset int64, limit int) (SchedulerCandidatePage, bool, error)
	SetCandidateIndex(ctx context.Context, bucket SchedulerBucket, accounts []Account) error
	ReplaceAccountCandidates(ctx context.Context, account *Account, buckets []SchedulerBucket) error
	DeleteCandidateAccount(ctx context.Context, accountID int64) error
	UpdateCandidateLastUsed(ctx context.Context, updates map[int64]time.Time) error
	InvalidateLegacySnapshots(ctx context.Context) error
}
