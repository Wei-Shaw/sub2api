//go:build unit

package service

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// 本文件锁死 asteria-control（外部控制面）对 API Key 编辑路径的两条依赖：
//
//  1. 改名不会重新生成 key 值——控制面把 key 明文发给客户端后只靠改名做标记，
//     key 值一变客户端凭据就静默失效。
//  2. 只带 expires_at 的部分更新不碰配额与限速的累计值——控制面按租约续期时
//     并发计费递增的 quota_used / usage_* 不能被旧快照覆盖。
//
// 将来 sub2api 升级或换中转，只要这两条变了这里就红。

// contractAPIKeyRepoStub 在 updateFieldsAPIKeyRepoStub 之上模拟真实仓储的两个行为：
//   - GetByID 返回快照后，立刻有一笔并发计费把 quota_used / usage_5h 原子递增；
//   - Update 只把掩码声明的列写回库里那一行（与 repository.Update 的 builder 分支一致）。
type contractAPIKeyRepoStub struct {
	updateFieldsAPIKeyRepoStub
	concurrentCharge float64
}

func (s *contractAPIKeyRepoStub) GetByID(ctx context.Context, id int64) (*APIKey, error) {
	snapshot, err := s.updateFieldsAPIKeyRepoStub.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	// 快照拿走之后才递增，模拟"编辑请求与计费请求交错"。
	s.key.QuotaUsed += s.concurrentCharge
	s.key.Usage5h += s.concurrentCharge
	return snapshot, nil
}

func (s *contractAPIKeyRepoStub) Update(ctx context.Context, key *APIKey, fields APIKeyUpdateFields) error {
	if err := s.updateFieldsAPIKeyRepoStub.Update(ctx, key, fields); err != nil {
		return err
	}
	if fields.Name {
		s.key.Name = key.Name
	}
	if fields.Status {
		s.key.Status = key.Status
	}
	if fields.Quota {
		s.key.Quota = key.Quota
	}
	if fields.QuotaUsed {
		s.key.QuotaUsed = key.QuotaUsed
	}
	if fields.RateLimits {
		s.key.RateLimit5h, s.key.RateLimit1d, s.key.RateLimit7d = key.RateLimit5h, key.RateLimit1d, key.RateLimit7d
	}
	if fields.RateLimitUsage {
		s.key.Usage5h, s.key.Usage1d, s.key.Usage7d = key.Usage5h, key.Usage1d, key.Usage7d
	}
	if fields.ExpiresAt {
		s.key.ExpiresAt = key.ExpiresAt
	}
	return nil
}

func newContractAPIKeyService(key *APIKey, concurrentCharge float64) (*APIKeyService, *contractAPIKeyRepoStub) {
	repo := &contractAPIKeyRepoStub{
		updateFieldsAPIKeyRepoStub: updateFieldsAPIKeyRepoStub{key: key},
		concurrentCharge:           concurrentCharge,
	}
	return &APIKeyService{apiKeyRepo: repo}, repo
}

// asteria-control 依赖这个事实：改名（只传 name）不会重新生成 key 值。
// 锁两层：Update 掩码只含 Name；APIKeyUpdateFields 根本没有 Key 位，
// 所以 repository.Update 没有任何分支能 SetKey。
func TestAsteriaContract_RenameDoesNotRotateKey(t *testing.T) {
	const keyValue = "sk-asteria-issued"
	svc, repo := newContractAPIKeyService(&APIKey{
		ID: 1, UserID: 7, Key: keyValue, Name: "old-name", Status: StatusActive,
	}, 0)

	name := "new-name"
	updated, err := svc.Update(context.Background(), 1, 7, UpdateAPIKeyRequest{Name: &name})
	require.NoError(t, err)

	require.Equal(t, []APIKeyUpdateFields{{Name: true}}, repo.updateFields)
	require.Equal(t, keyValue, updated.Key, "返回值里的 key 不能变")
	require.Equal(t, keyValue, repo.key.Key, "库里那一行的 key 不能变")
	require.Equal(t, "new-name", repo.key.Name)

	// 结构性保证：掩码类型没有 Key 位，仓储层无从写 key 列。
	_, hasKeyBit := reflect.TypeOf(APIKeyUpdateFields{}).FieldByName("Key")
	require.False(t, hasKeyBit, "APIKeyUpdateFields 多出 Key 位意味着 Update 可能改写 key 值，asteria-control 的凭据会静默失效")
}

// asteria-control 依赖这个事实：只带 expires_at 的部分更新，掩码只含 ExpiresAt，
// 并发递增的 quota_used / usage_5h 与 quota / rate_limit_* 阈值都不被覆盖。
func TestAsteriaContract_ExpiresAtOnlyLeavesUsageAndLimitsUntouched(t *testing.T) {
	newExpiry := time.Now().Add(30 * 24 * time.Hour).Truncate(time.Second)
	oldExpiry := time.Now().Add(24 * time.Hour)

	tests := []struct {
		name       string
		req        UpdateAPIKeyRequest
		wantExpiry *time.Time
	}{
		{
			name:       "set expires_at",
			req:        UpdateAPIKeyRequest{ExpiresAt: &newExpiry},
			wantExpiry: &newExpiry,
		},
		{
			// handler 把 expires_at="" 翻译成 ClearExpiration=true（api_key_handler.go Update）。
			name:       "clear expires_at",
			req:        UpdateAPIKeyRequest{ClearExpiration: true},
			wantExpiry: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			const charge = 2.5
			svc, repo := newContractAPIKeyService(&APIKey{
				ID: 1, UserID: 7, Key: "sk-test", Status: StatusActive,
				Quota: 100, QuotaUsed: 30,
				RateLimit5h: 10, RateLimit1d: 20, RateLimit7d: 30,
				Usage5h: 12, Usage1d: 13, Usage7d: 14,
				ExpiresAt: &oldExpiry,
			}, charge)

			_, err := svc.Update(context.Background(), 1, 7, tt.req)
			require.NoError(t, err)

			require.Equal(t, []APIKeyUpdateFields{{ExpiresAt: true}}, repo.updateFields)

			// 并发计费的递增必须留在库里，而不是被 GetByID 时的旧快照写回。
			require.InDelta(t, 30+charge, repo.key.QuotaUsed, 1e-9, "quota_used 被旧快照覆盖")
			require.InDelta(t, 12+charge, repo.key.Usage5h, 1e-9, "usage_5h 被旧快照覆盖")
			require.Equal(t, 13.0, repo.key.Usage1d)
			require.Equal(t, 14.0, repo.key.Usage7d)

			// 阈值列同样不动。
			require.Equal(t, 100.0, repo.key.Quota)
			require.Equal(t, 10.0, repo.key.RateLimit5h)
			require.Equal(t, 20.0, repo.key.RateLimit1d)
			require.Equal(t, 30.0, repo.key.RateLimit7d)

			if tt.wantExpiry == nil {
				require.Nil(t, repo.key.ExpiresAt)
			} else {
				require.NotNil(t, repo.key.ExpiresAt)
				require.True(t, tt.wantExpiry.Equal(*repo.key.ExpiresAt))
			}
		})
	}
}
