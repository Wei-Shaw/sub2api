package service

// B2-③ 保存期无价门禁回归测试（ollama_cloud）：
//   ① mapping 目标含未定价名 → Create/Update 返回 400 MODEL_PRICING_MISSING 且
//      message 含该模型名；
//   ② 空 mapping 无 allowed_models 清单 → 400；
//   ③ 空 mapping + 显式清单（全部已定价，含真实 :tag 形态）→ 通过；
//   ④ 非 ollama 平台同样配置不受影响（回归）。
//
// 门禁同时接在两条保存路径上：
//   - AccountService.Create/Update（本文件，spec 权威位置）；
//   - adminServiceImpl.CreateAccount/UpdateAccount（生产 admin handler 实际走
//     的路径，见 TestAdminCreateAccount.../TestAdminUpdateAccount...）。

import (
	"context"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/stretchr/testify/require"
)

func newOllamaGateAccountService(repo AccountRepository) *AccountService {
	return NewAccountService(repo, nil, NewBillingService(&config.Config{}, nil))
}

func requireModelPricingMissing(t *testing.T, err error, wantNames ...string) {
	t.Helper()
	require.Error(t, err)
	require.Equal(t, 400, infraerrors.Code(err), "必须映射为 400: %v", err)
	require.Equal(t, "MODEL_PRICING_MISSING", infraerrors.Reason(err))
	for _, name := range wantNames {
		require.Contains(t, infraerrors.Message(err), name, "message 必须列出未定价模型名")
	}
}

func TestAccountServiceCreateOllamaCloudPricingGate(t *testing.T) {
	ctx := context.Background()

	t.Run("mapping目标含未定价名_400并列出该名", func(t *testing.T) {
		repo := &upstreamBillingProbeAccountRepo{accounts: make(map[int64]*Account)}
		svc := newOllamaGateAccountService(repo)
		_, err := svc.Create(ctx, CreateAccountRequest{
			Name:        "ollama-bad-mapping",
			Platform:    PlatformOllamaCloud,
			Type:        AccountTypeAPIKey,
			Credentials: map[string]any{"api_key": "k", "model_mapping": map[string]any{"alias": "totally-unpriced-model"}},
		})
		requireModelPricingMissing(t, err, "totally-unpriced-model")
		require.Empty(t, repo.accounts, "被拒绝的账号不得落库")
	})

	t.Run("空mapping且无清单_400", func(t *testing.T) {
		repo := &upstreamBillingProbeAccountRepo{accounts: make(map[int64]*Account)}
		svc := newOllamaGateAccountService(repo)
		_, err := svc.Create(ctx, CreateAccountRequest{
			Name:        "ollama-no-list",
			Platform:    PlatformOllamaCloud,
			Type:        AccountTypeAPIKey,
			Credentials: map[string]any{"api_key": "k"},
		})
		requireModelPricingMissing(t, err)
		require.Contains(t, infraerrors.Message(err), OllamaCloudAllowedModelsExtraKey)
		require.Empty(t, repo.accounts)
	})

	t.Run("空mapping加已定价清单_通过", func(t *testing.T) {
		repo := &upstreamBillingProbeAccountRepo{accounts: make(map[int64]*Account)}
		svc := newOllamaGateAccountService(repo)
		created, err := svc.Create(ctx, CreateAccountRequest{
			Name:        "ollama-with-list",
			Platform:    PlatformOllamaCloud,
			Type:        AccountTypeAPIKey,
			Credentials: map[string]any{"api_key": "k", "base_url": "https://ollama.com/v1"},
			Extra: map[string]any{
				OllamaCloudAllowedModelsExtraKey: []any{"gpt-oss:120b-cloud", "qwen3.5:397b"},
			},
		})
		require.NoError(t, err)
		require.NotNil(t, created)
		require.NotEmpty(t, repo.accounts)
	})

	t.Run("mapping目标全部已定价_通过", func(t *testing.T) {
		repo := &upstreamBillingProbeAccountRepo{accounts: make(map[int64]*Account)}
		svc := newOllamaGateAccountService(repo)
		_, err := svc.Create(ctx, CreateAccountRequest{
			Name:     "ollama-priced-mapping",
			Platform: PlatformOllamaCloud,
			Type:     AccountTypeAPIKey,
			Credentials: map[string]any{
				"api_key":       "k",
				"model_mapping": map[string]any{"big": "gpt-oss:120b", "coder": "qwen3.5:397b"},
			},
		})
		require.NoError(t, err)
	})

	t.Run("非ollama平台不受影响_回归", func(t *testing.T) {
		for _, platform := range []string{PlatformOpenAI, PlatformKimi, PlatformAnthropic} {
			repo := &upstreamBillingProbeAccountRepo{accounts: make(map[int64]*Account)}
			svc := newOllamaGateAccountService(repo)
			created, err := svc.Create(ctx, CreateAccountRequest{
				Name:        "legacy-" + platform,
				Platform:    platform,
				Type:        AccountTypeAPIKey,
				Credentials: map[string]any{"api_key": "k"},
			})
			require.NoError(t, err, "platform=%s 的空 mapping 账号保存不得被门禁拦截", platform)
			require.NotNil(t, created)
		}
	})
}

func TestAccountServiceUpdateOllamaCloudPricingGate(t *testing.T) {
	ctx := context.Background()

	newOllamaAccount := func(t *testing.T) (*upstreamBillingProbeAccountRepo, *AccountService, *Account) {
		t.Helper()
		repo := &upstreamBillingProbeAccountRepo{accounts: make(map[int64]*Account)}
		svc := newOllamaGateAccountService(repo)
		created, err := svc.Create(ctx, CreateAccountRequest{
			Name:     "ollama-update",
			Platform: PlatformOllamaCloud,
			Type:     AccountTypeAPIKey,
			Credentials: map[string]any{
				"api_key":       "k",
				"model_mapping": map[string]any{"big": "gpt-oss:120b"},
			},
		})
		require.NoError(t, err)
		return repo, svc, created
	}

	t.Run("改成未定价目标_400", func(t *testing.T) {
		_, svc, created := newOllamaAccount(t)
		_, err := svc.Update(ctx, created.ID, UpdateAccountRequest{
			Credentials: &map[string]any{"api_key": "k", "model_mapping": map[string]any{"big": "totally-unpriced-model"}},
		})
		requireModelPricingMissing(t, err, "totally-unpriced-model")
	})

	t.Run("清空mapping且无清单_400", func(t *testing.T) {
		_, svc, created := newOllamaAccount(t)
		_, err := svc.Update(ctx, created.ID, UpdateAccountRequest{
			Credentials: &map[string]any{"api_key": "k"},
		})
		requireModelPricingMissing(t, err)
	})

	t.Run("换成已定价清单_通过", func(t *testing.T) {
		repo, svc, created := newOllamaAccount(t)
		updated, err := svc.Update(ctx, created.ID, UpdateAccountRequest{
			Credentials: &map[string]any{"api_key": "k"},
			Extra: &map[string]any{
				OllamaCloudAllowedModelsExtraKey: []any{"gpt-oss:20b"},
			},
		})
		require.NoError(t, err)
		require.Equal(t, []any{"gpt-oss:20b"}, updated.Extra[OllamaCloudAllowedModelsExtraKey])
		require.NotNil(t, repo.accounts[created.ID])
	})

	t.Run("非ollama平台更新不受影响_回归", func(t *testing.T) {
		repo := &upstreamBillingProbeAccountRepo{accounts: make(map[int64]*Account)}
		svc := newOllamaGateAccountService(repo)
		created, err := svc.Create(ctx, CreateAccountRequest{
			Name:        "kimi-update",
			Platform:    PlatformKimi,
			Type:        AccountTypeAPIKey,
			Credentials: map[string]any{"api_key": "k", "model_mapping": map[string]any{"k2": "kimi-k2-0905-preview"}},
		})
		require.NoError(t, err)
		updated, err := svc.Update(ctx, created.ID, UpdateAccountRequest{
			Credentials: &map[string]any{"api_key": "k"},
		})
		require.NoError(t, err, "kimi 账号清空 mapping 的更新不得被门禁拦截")
		require.NotNil(t, updated)
	})
}

// 生产 admin handler 实际走的保存路径（adminServiceImpl.CreateAccount/UpdateAccount）
// 同样必须挂门禁——只测 AccountService 而 admin 路径漏挂，等于门禁空转。
func TestAdminCreateAccountOllamaCloudPricingGate(t *testing.T) {
	svc := &adminServiceImpl{accountRepo: &upstreamBillingProbeAccountRepo{accounts: make(map[int64]*Account)}, billingService: NewBillingService(&config.Config{}, nil)}
	_, err := svc.CreateAccount(context.Background(), &CreateAccountInput{
		Name:                 "ollama-admin-unpriced",
		Platform:             PlatformOllamaCloud,
		Type:                 AccountTypeAPIKey,
		Credentials:          map[string]any{"api_key": "k", "model_mapping": map[string]any{"alias": "totally-unpriced-model"}},
		SkipDefaultGroupBind: true,
	})
	requireModelPricingMissing(t, err, "totally-unpriced-model")

	created, err := svc.CreateAccount(context.Background(), &CreateAccountInput{
		Name:                 "ollama-admin-priced",
		Platform:             PlatformOllamaCloud,
		Type:                 AccountTypeAPIKey,
		Credentials:          map[string]any{"api_key": "k"},
		Extra:                map[string]any{OllamaCloudAllowedModelsExtraKey: []any{"gpt-oss:120b-cloud", "nemotron-3-super"}},
		SkipDefaultGroupBind: true,
	})
	require.NoError(t, err)
	require.NotNil(t, created)
}

func TestAdminUpdateAccountOllamaCloudPricingGate(t *testing.T) {
	repo := &upstreamBillingProbeAccountRepo{accounts: make(map[int64]*Account)}
	svc := &adminServiceImpl{accountRepo: repo, billingService: NewBillingService(&config.Config{}, nil)}

	created, err := svc.CreateAccount(context.Background(), &CreateAccountInput{
		Name:                 "ollama-admin-update",
		Platform:             PlatformOllamaCloud,
		Type:                 AccountTypeAPIKey,
		Credentials:          map[string]any{"api_key": "k"},
		Extra:                map[string]any{OllamaCloudAllowedModelsExtraKey: []any{"gpt-oss:120b-cloud"}},
		SkipDefaultGroupBind: true,
	})
	require.NoError(t, err)

	_, err = svc.UpdateAccount(context.Background(), created.ID, &UpdateAccountInput{
		Credentials: map[string]any{"api_key": "k", "model_mapping": map[string]any{"alias": "totally-unpriced-model"}},
	})
	requireModelPricingMissing(t, err, "totally-unpriced-model")

	updated, err := svc.UpdateAccount(context.Background(), created.ID, &UpdateAccountInput{
		Credentials: map[string]any{"api_key": "k", "model_mapping": map[string]any{"alias": "qwen3.5:397b"}},
	})
	require.NoError(t, err)
	require.NotNil(t, updated)
}

// gate helper 单元测试：清单值形态与非 ollama 平台短路。
func TestOllamaCloudOutboundModelNames(t *testing.T) {
	account := &Account{Platform: PlatformOllamaCloud, Credentials: map[string]any{
		"model_mapping": map[string]any{"a": "gpt-oss:120b", "b": "", "c": "qwen3.5:397b", "d": "gpt-oss:120b"},
	}}
	// map 迭代顺序不定，只断言集合内容（去空、去重）。
	require.ElementsMatch(t, []string{"gpt-oss:120b", "qwen3.5:397b"}, ollamaCloudOutboundModelNames(account))

	account = &Account{Platform: PlatformOllamaCloud, Extra: map[string]any{
		OllamaCloudAllowedModelsExtraKey: []any{"gpt-oss:20b", " "},
	}}
	require.Equal(t, []string{"gpt-oss:20b"}, ollamaCloudOutboundModelNames(account))

	account = &Account{Platform: PlatformOllamaCloud, Extra: map[string]any{
		OllamaCloudAllowedModelsExtraKey: []any{"gpt-oss:20b", 42},
	}}
	require.Nil(t, ollamaCloudOutboundModelNames(account), "含非字符串元素的清单视同未提供（拒绝语义）")

	account = &Account{Platform: PlatformOpenAI}
	require.Nil(t, ollamaCloudOutboundModelNames(account))
}
