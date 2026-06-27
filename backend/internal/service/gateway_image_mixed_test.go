//go:build unit

package service

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// newImageMixedRepo 构造 openai + fal 混合候选池测试用的账号仓储。
func newImageMixedRepo(accounts []Account) *mockAccountRepoForPlatform {
	repo := &mockAccountRepoForPlatform{
		accounts:     accounts,
		accountsByID: map[int64]*Account{},
	}
	for i := range repo.accounts {
		repo.accountsByID[repo.accounts[i].ID] = &repo.accounts[i]
	}
	return repo
}

// falImageAccount 构造一个支持指定 model+api 的 fal 账号。
// modelAPIs 形如 {"gpt-image-2": "openai/gpt-image-2", "gpt-image-2-edit": "openai/gpt-image-2/edit"}。
func falImageAccount(id int64, priority int, modelMapping map[string]any) Account {
	return Account{
		ID:          id,
		Platform:    PlatformFal,
		Type:        AccountTypeAPIKey,
		Priority:    priority,
		Status:      StatusActive,
		Schedulable: true,
		Credentials: map[string]any{"model_mapping": modelMapping},
	}
}

// openaiImageAccount 构造一个 API Key 类型的 openai 账号（支持 basic/native 图片能力）。
func openaiImageAccount(id int64, priority int) Account {
	return Account{
		ID:          id,
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Priority:    priority,
		Status:      StatusActive,
		Schedulable: true,
	}
}

func TestGatewayService_SelectImageAccountMixed(t *testing.T) {
	ctx := context.Background()
	const t2iModel = "gpt-image-2"

	newSvc := func(repo *mockAccountRepoForPlatform, cache *mockGatewayCacheForPlatform) *GatewayService {
		if cache == nil {
			cache = &mockGatewayCacheForPlatform{}
		}
		return &GatewayService{
			accountRepo: repo,
			cache:       cache,
			cfg:         testConfig(),
		}
	}

	t.Run("fal优先级更高时混合池选中fal", func(t *testing.T) {
		repo := newImageMixedRepo([]Account{
			openaiImageAccount(1, 2),
			falImageAccount(2, 1, map[string]any{t2iModel: "openai/gpt-image-2"}),
		})
		svc := newSvc(repo, nil)

		acc, err := svc.SelectImageAccountMixed(ctx, nil, "", t2iModel, nil, OpenAIImagesCapabilityBasic, "", "")
		require.NoError(t, err)
		require.NotNil(t, acc)
		require.Equal(t, int64(2), acc.ID, "fal 优先级更高应被选中")
		require.Equal(t, PlatformFal, acc.Platform)
	})

	t.Run("同优先级均未使用时优先openai保持稳定", func(t *testing.T) {
		repo := newImageMixedRepo([]Account{
			openaiImageAccount(1, 1),
			falImageAccount(2, 1, map[string]any{t2iModel: "openai/gpt-image-2"}),
		})
		svc := newSvc(repo, nil)

		acc, err := svc.SelectImageAccountMixed(ctx, nil, "", t2iModel, nil, OpenAIImagesCapabilityBasic, "", "")
		require.NoError(t, err)
		require.NotNil(t, acc)
		require.Equal(t, int64(1), acc.ID, "同优先级且均未使用时 openai 先入池应保持稳定")
		require.Equal(t, PlatformOpenAI, acc.Platform)
	})

	t.Run("同优先级按最久未用选号", func(t *testing.T) {
		now := time.Now()
		earlier := now.Add(-time.Hour)
		openai := openaiImageAccount(1, 1)
		openai.LastUsedAt = &now
		fal := falImageAccount(2, 1, map[string]any{t2iModel: "openai/gpt-image-2"})
		fal.LastUsedAt = &earlier

		repo := newImageMixedRepo([]Account{openai, fal})
		svc := newSvc(repo, nil)

		acc, err := svc.SelectImageAccountMixed(ctx, nil, "", t2iModel, nil, OpenAIImagesCapabilityBasic, "", "")
		require.NoError(t, err)
		require.NotNil(t, acc)
		require.Equal(t, int64(2), acc.ID, "同优先级时最久未用的 fal 账号应被选中")
	})

	t.Run("failover排除openai后跨平台切到fal", func(t *testing.T) {
		repo := newImageMixedRepo([]Account{
			openaiImageAccount(1, 1),
			falImageAccount(2, 2, map[string]any{t2iModel: "openai/gpt-image-2"}),
		})
		svc := newSvc(repo, nil)

		// 首选 openai（优先级更高）
		acc, err := svc.SelectImageAccountMixed(ctx, nil, "", t2iModel, nil, OpenAIImagesCapabilityBasic, "", "")
		require.NoError(t, err)
		require.Equal(t, int64(1), acc.ID)

		// 排除 openai 后混合池天然切到 fal
		excluded := map[int64]struct{}{1: {}}
		acc, err = svc.SelectImageAccountMixed(ctx, nil, "", t2iModel, excluded, OpenAIImagesCapabilityBasic, "", "")
		require.NoError(t, err)
		require.NotNil(t, acc)
		require.Equal(t, int64(2), acc.ID, "openai 被排除后应跨平台切到 fal")
		require.Equal(t, PlatformFal, acc.Platform)
	})

	t.Run("fal不支持请求模型时跳过选openai", func(t *testing.T) {
		repo := newImageMixedRepo([]Account{
			// fal 优先级更高，但只映射了 gpt-image-2，不支持 dall-e-3
			falImageAccount(2, 1, map[string]any{t2iModel: "openai/gpt-image-2"}),
			openaiImageAccount(1, 2),
		})
		svc := newSvc(repo, nil)

		acc, err := svc.SelectImageAccountMixed(ctx, nil, "", "dall-e-3", nil, OpenAIImagesCapabilityBasic, "", "")
		require.NoError(t, err)
		require.NotNil(t, acc)
		require.Equal(t, int64(1), acc.ID, "fal 不支持请求模型时应跳过，由 openai 兜底")
		require.Equal(t, PlatformOpenAI, acc.Platform)
	})

	t.Run("edit请求时fal仅文生图被跳过选openai", func(t *testing.T) {
		repo := newImageMixedRepo([]Account{
			// fal 优先级更高，但只支持文生图（无 edit api 段）
			falImageAccount(2, 1, map[string]any{t2iModel: "openai/gpt-image-2"}),
			openaiImageAccount(1, 2),
		})
		svc := newSvc(repo, nil)

		acc, err := svc.SelectImageAccountMixed(ctx, nil, "", t2iModel, nil, OpenAIImagesCapabilityNative, "edit", "")
		require.NoError(t, err)
		require.NotNil(t, acc)
		require.Equal(t, int64(1), acc.ID, "edit 请求时仅支持文生图的 fal 账号应被跳过")
	})

	t.Run("edit请求时支持edit的fal账号被选中", func(t *testing.T) {
		repo := newImageMixedRepo([]Account{
			falImageAccount(2, 1, map[string]any{
				t2iModel:           "openai/gpt-image-2",
				t2iModel + "-edit": "openai/gpt-image-2/edit",
			}),
			openaiImageAccount(1, 2),
		})
		svc := newSvc(repo, nil)

		acc, err := svc.SelectImageAccountMixed(ctx, nil, "", t2iModel, nil, OpenAIImagesCapabilityNative, "edit", "")
		require.NoError(t, err)
		require.NotNil(t, acc)
		require.Equal(t, int64(2), acc.ID, "支持 edit 的 fal 账号应被选中")
		require.Equal(t, PlatformFal, acc.Platform)
	})

	t.Run("无可用账号返回ErrNoAvailableAccounts", func(t *testing.T) {
		repo := newImageMixedRepo(nil)
		svc := newSvc(repo, nil)

		acc, err := svc.SelectImageAccountMixed(ctx, nil, "", t2iModel, nil, OpenAIImagesCapabilityBasic, "", "")
		require.Nil(t, acc)
		require.ErrorIs(t, err, ErrNoAvailableAccounts)
	})

	t.Run("粘性会话跨平台命中fal", func(t *testing.T) {
		repo := newImageMixedRepo([]Account{
			// openai 优先级更高，但粘性会话绑定到 fal
			openaiImageAccount(1, 1),
			falImageAccount(2, 2, map[string]any{t2iModel: "openai/gpt-image-2"}),
		})
		cache := &mockGatewayCacheForPlatform{
			sessionBindings: map[string]int64{"session-img": 2},
		}
		svc := newSvc(repo, cache)

		acc, err := svc.SelectImageAccountMixed(ctx, nil, "session-img", t2iModel, nil, OpenAIImagesCapabilityBasic, "", "")
		require.NoError(t, err)
		require.NotNil(t, acc)
		require.Equal(t, int64(2), acc.ID, "粘性会话命中应允许跨平台返回 fal 账号")
		require.Equal(t, PlatformFal, acc.Platform)
	})

	t.Run("被排除的粘性会话账号回退普通选号", func(t *testing.T) {
		repo := newImageMixedRepo([]Account{
			openaiImageAccount(1, 1),
			falImageAccount(2, 2, map[string]any{t2iModel: "openai/gpt-image-2"}),
		})
		cache := &mockGatewayCacheForPlatform{
			sessionBindings: map[string]int64{"session-img": 2},
		}
		svc := newSvc(repo, cache)

		// fal(2) 被排除，粘性命中失效，回退优先级选号选中 openai(1)
		excluded := map[int64]struct{}{2: {}}
		acc, err := svc.SelectImageAccountMixed(ctx, nil, "session-img", t2iModel, excluded, OpenAIImagesCapabilityBasic, "", "")
		require.NoError(t, err)
		require.NotNil(t, acc)
		require.Equal(t, int64(1), acc.ID)
		require.Equal(t, PlatformOpenAI, acc.Platform)
	})
}
