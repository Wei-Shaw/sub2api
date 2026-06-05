//go:build unit

package service

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	pkgerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

// 共享的 fakeServiceQuotaRepo 在 service_quota_fakes_test.go 中定义；本文件仅
// 通过 fixture 注入 channel/account/group 的 scope 信息，让 validatePathLinkage
// 单测脱离真实 DB。

// ptrInt64/ptrString 在同 package 已存在（payment_config_plans_validation_test.go / admin_service_group_test.go），复用即可。

// extractAppErrorMetadata 把 ApplicationError 的 reason（业务错误码）+ metadata 拿出来比对，
// 避免依赖具体 message 文案。
func extractAppErrorMetadata(t *testing.T, err error) (string, map[string]string) {
	t.Helper()
	require.Error(t, err)
	var appErr *pkgerrors.ApplicationError
	require.True(t, errors.As(err, &appErr), "expected ApplicationError, got %T: %v", err, err)
	return appErr.Reason, appErr.Metadata
}

// TestValidatePathLinkage_ChannelPlatformMismatch：channel 不服务 path 指定 platform 时报错。
func TestValidatePathLinkage_ChannelPlatformMismatch(t *testing.T) {
	t.Parallel()
	repo := &fakeServiceQuotaRepo{
		channels: map[int64]*ChannelScopeInfo{
			11: {Platforms: []string{"openai"}, GroupIDs: []int64{}},
		},
	}
	svc := &serviceQuotaService{repo: repo}

	err := svc.validatePathLinkage(context.Background(), ServiceQuotaPathInput{
		Platform:  ptrString("anthropic"),
		ChannelID: ptrInt64(11),
	})
	code, meta := extractAppErrorMetadata(t, err)
	require.Equal(t, "SERVICE_QUOTA_SCOPE_MISMATCH", code)
	require.Equal(t, "channel_platform", meta["entity"])
	require.Equal(t, "anthropic", meta["expected_platform"])
	require.Equal(t, "openai", meta["channel_platforms"])
}

// TestValidatePathLinkage_ChannelPlatformMatchesAfterFold：大小写不敏感，path 平台 ANTHROPIC 等价于 anthropic。
func TestValidatePathLinkage_ChannelPlatformMatchesAfterFold(t *testing.T) {
	t.Parallel()
	repo := &fakeServiceQuotaRepo{
		channels: map[int64]*ChannelScopeInfo{
			12: {Platforms: []string{"anthropic"}, GroupIDs: []int64{}},
		},
	}
	svc := &serviceQuotaService{repo: repo}

	err := svc.validatePathLinkage(context.Background(), ServiceQuotaPathInput{
		Platform:  ptrString("ANTHROPIC"),
		ChannelID: ptrInt64(12),
	})
	require.NoError(t, err)
}

// TestValidatePathLinkage_ChannelGroupMismatch：channel 不包含 path 指定 group 时报错。
func TestValidatePathLinkage_ChannelGroupMismatch(t *testing.T) {
	t.Parallel()
	repo := &fakeServiceQuotaRepo{
		channels: map[int64]*ChannelScopeInfo{
			21: {Platforms: []string{"anthropic"}, GroupIDs: []int64{301, 302}},
		},
		groups: map[int64]*GroupScopeInfo{
			999: {Platform: "anthropic"},
		},
	}
	svc := &serviceQuotaService{repo: repo}

	err := svc.validatePathLinkage(context.Background(), ServiceQuotaPathInput{
		Platform:  ptrString("anthropic"),
		ChannelID: ptrInt64(21),
		GroupID:   ptrInt64(999),
	})
	code, meta := extractAppErrorMetadata(t, err)
	require.Equal(t, "SERVICE_QUOTA_SCOPE_MISMATCH", code)
	require.Equal(t, "channel_group", meta["entity"])
	require.Equal(t, "21", meta["channel_id"])
	require.Equal(t, "999", meta["group_id"])
}

// TestValidatePathLinkage_ChannelBothMismatch：channel 既不服务 platform 也不含 group，
// 应该先在 platform 维度短路报错（按校验顺序）。
func TestValidatePathLinkage_ChannelBothMismatch(t *testing.T) {
	t.Parallel()
	repo := &fakeServiceQuotaRepo{
		channels: map[int64]*ChannelScopeInfo{
			31: {Platforms: []string{"openai"}, GroupIDs: []int64{500}},
		},
		groups: map[int64]*GroupScopeInfo{
			999: {Platform: "anthropic"},
		},
	}
	svc := &serviceQuotaService{repo: repo}

	err := svc.validatePathLinkage(context.Background(), ServiceQuotaPathInput{
		Platform:  ptrString("anthropic"),
		ChannelID: ptrInt64(31),
		GroupID:   ptrInt64(999),
	})
	code, meta := extractAppErrorMetadata(t, err)
	require.Equal(t, "SERVICE_QUOTA_SCOPE_MISMATCH", code)
	// platform mismatch 先被捕获（顺序敏感），group mismatch 不会被报告。
	require.Equal(t, "channel_platform", meta["entity"])
}

// TestValidatePathLinkage_ChannelHappyPath：channel 同时服务 platform 且包含 group 时通过校验。
func TestValidatePathLinkage_ChannelHappyPath(t *testing.T) {
	t.Parallel()
	repo := &fakeServiceQuotaRepo{
		channels: map[int64]*ChannelScopeInfo{
			41: {Platforms: []string{"anthropic", "openai"}, GroupIDs: []int64{500, 501}},
		},
		groups: map[int64]*GroupScopeInfo{
			500: {Platform: "anthropic"},
		},
	}
	svc := &serviceQuotaService{repo: repo}

	err := svc.validatePathLinkage(context.Background(), ServiceQuotaPathInput{
		Platform:  ptrString("anthropic"),
		ChannelID: ptrInt64(41),
		GroupID:   ptrInt64(500),
	})
	require.NoError(t, err)
}

// TestValidatePathLinkage_ChannelNotFound：channel id 不存在时报 SERVICE_QUOTA_SCOPE_CHANNEL_NOT_FOUND。
func TestValidatePathLinkage_ChannelNotFound(t *testing.T) {
	t.Parallel()
	repo := &fakeServiceQuotaRepo{
		channels: map[int64]*ChannelScopeInfo{}, // 空 → 任意 id 查不到
	}
	svc := &serviceQuotaService{repo: repo}

	err := svc.validatePathLinkage(context.Background(), ServiceQuotaPathInput{
		ChannelID: ptrInt64(404),
	})
	code, meta := extractAppErrorMetadata(t, err)
	require.Equal(t, "SERVICE_QUOTA_SCOPE_CHANNEL_NOT_FOUND", code)
	require.Equal(t, "404", meta["channel_id"])
}

// TestValidatePathLinkage_ChannelNilSkipped：未指定 channel_id 时不查 channel scope，
// 也不会因为 channel 没有 model_pricing 而误报。
func TestValidatePathLinkage_ChannelNilSkipped(t *testing.T) {
	t.Parallel()
	repo := &fakeServiceQuotaRepo{}
	svc := &serviceQuotaService{repo: repo}

	require.NoError(t, svc.validatePathLinkage(context.Background(), ServiceQuotaPathInput{
		Platform: ptrString("anthropic"),
	}))
}

// TestValidatePathLinkage_AccountChecksStillRun：原有 account/group 校验在加 channel 维度后仍生效。
func TestValidatePathLinkage_AccountChecksStillRun(t *testing.T) {
	t.Parallel()
	repo := &fakeServiceQuotaRepo{
		accounts: map[int64]*AccountScopeInfo{
			77: {Platform: "openai", GroupIDs: []int64{1}},
		},
	}
	svc := &serviceQuotaService{repo: repo}

	// account.platform=openai 与 path.platform=anthropic 不匹配 → 旧校验仍生效。
	err := svc.validatePathLinkage(context.Background(), ServiceQuotaPathInput{
		Platform:  ptrString("anthropic"),
		AccountID: ptrInt64(77),
	})
	code, meta := extractAppErrorMetadata(t, err)
	require.Equal(t, "SERVICE_QUOTA_SCOPE_MISMATCH", code)
	require.Equal(t, "account_platform", meta["entity"])
}
