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

// 门禁触发收窄回归：缺 mapping/清单的存量 legacy 账号不得被维护性更新锁死，
// 只有可能改变「可出站模型集合」或影响调度准入的更新才重验。
func TestAccountServiceUpdateOllamaCloudPricingGateOnlyOnModelSetOrSchedulingChange(t *testing.T) {
	ctx := context.Background()

	seedLegacyAccount := func(t *testing.T) (*upstreamBillingProbeAccountRepo, *AccountService, *Account) {
		t.Helper()
		repo := &upstreamBillingProbeAccountRepo{accounts: make(map[int64]*Account)}
		svc := newOllamaGateAccountService(repo)
		// legacy 形态：无 model_mapping、无 allowed_models 清单。Create 路径有门禁，
		// 直接经 repo 落库模拟迁移过来的存量账号。
		account := &Account{
			Platform:    PlatformOllamaCloud,
			Type:        AccountTypeAPIKey,
			Name:        "ollama-legacy",
			Status:      StatusActive,
			Schedulable: true,
			Credentials: map[string]any{"api_key": "k"},
		}
		require.NoError(t, repo.Create(ctx, account))
		return repo, svc, account
	}

	t.Run("legacy账号仅改name_必须成功", func(t *testing.T) {
		_, svc, account := seedLegacyAccount(t)
		updated, err := svc.Update(ctx, account.ID, UpdateAccountRequest{Name: strPtr("ollama-legacy-renamed")})
		require.NoError(t, err, "维护性更新（仅改 name）不得被 MODEL_PRICING_MISSING 拦截")
		require.Equal(t, "ollama-legacy-renamed", updated.Name)
	})

	t.Run("legacy账号改notes与并发数_放行", func(t *testing.T) {
		_, svc, account := seedLegacyAccount(t)
		concurrency := 5
		updated, err := svc.Update(ctx, account.ID, UpdateAccountRequest{
			Notes:       strPtr("migrated account"),
			Concurrency: &concurrency,
		})
		require.NoError(t, err)
		require.Equal(t, "migrated account", *updated.Notes)
		require.Equal(t, 5, updated.Concurrency)
	})

	t.Run("legacy账号加未定价mapping_仍400", func(t *testing.T) {
		_, svc, account := seedLegacyAccount(t)
		_, err := svc.Update(ctx, account.ID, UpdateAccountRequest{
			Credentials: &map[string]any{"api_key": "k", "model_mapping": map[string]any{"big": "totally-unpriced-model"}},
		})
		requireModelPricingMissing(t, err, "totally-unpriced-model")
	})

	t.Run("legacy账号加未定价清单_仍400", func(t *testing.T) {
		_, svc, account := seedLegacyAccount(t)
		_, err := svc.Update(ctx, account.ID, UpdateAccountRequest{
			Extra: &map[string]any{OllamaCloudAllowedModelsExtraKey: []any{"totally-unpriced-model"}},
		})
		requireModelPricingMissing(t, err, "totally-unpriced-model")
	})

	t.Run("不可调度改可调度_必须校验", func(t *testing.T) {
		repo, svc, account := seedLegacyAccount(t)
		// 停用态（手动开关仍开）：经 Update 把 status 改回 active 即「启用」，
		// 必须先通过门禁。
		repo.accounts[account.ID].Status = StatusDisabled
		_, err := svc.Update(ctx, account.ID, UpdateAccountRequest{Status: strPtr(StatusActive)})
		requireModelPricingMissing(t, err)
	})

	t.Run("停用legacy账号_放行", func(t *testing.T) {
		_, svc, account := seedLegacyAccount(t)
		updated, err := svc.Update(ctx, account.ID, UpdateAccountRequest{Status: strPtr(StatusDisabled)})
		require.NoError(t, err, "停用（退出调度准入）属于维护性更新，不得被拦截")
		require.Equal(t, StatusDisabled, updated.Status)
	})

	t.Run("非ollama平台行为不变_回归", func(t *testing.T) {
		repo := &upstreamBillingProbeAccountRepo{accounts: make(map[int64]*Account)}
		svc := newOllamaGateAccountService(repo)
		created, err := svc.Create(ctx, CreateAccountRequest{
			Name:        "kimi-legacy-update",
			Platform:    PlatformKimi,
			Type:        AccountTypeAPIKey,
			Credentials: map[string]any{"api_key": "k"},
		})
		require.NoError(t, err)
		updated, err := svc.Update(ctx, created.ID, UpdateAccountRequest{
			Name:  strPtr("kimi-renamed"),
			Notes: strPtr("notes only"),
		})
		require.NoError(t, err, "非 ollama 平台的维护性更新行为必须保持不变")
		require.Equal(t, "kimi-renamed", updated.Name)
	})
}

// 门禁触发收窄在生产 admin handler 路径（adminServiceImpl.UpdateAccount）同样生效：
// 全对象 PUT 编辑会原样带回 credentials/extra，集合未变时不得重验锁死 legacy 账号。
func TestAdminUpdateAccountOllamaCloudPricingGateOnlyOnModelSetOrSchedulingChange(t *testing.T) {
	ctx := context.Background()
	repo := &upstreamBillingProbeAccountRepo{accounts: make(map[int64]*Account)}
	svc := &adminServiceImpl{accountRepo: repo, billingService: NewBillingService(&config.Config{}, nil)}

	account := &Account{
		Platform:    PlatformOllamaCloud,
		Type:        AccountTypeAPIKey,
		Name:        "ollama-admin-legacy",
		Status:      StatusActive,
		Schedulable: true,
		Credentials: map[string]any{"api_key": "k"},
	}
	require.NoError(t, repo.Create(ctx, account))

	t.Run("仅改name_必须成功", func(t *testing.T) {
		updated, err := svc.UpdateAccount(ctx, account.ID, &UpdateAccountInput{Name: "ollama-admin-legacy-renamed"})
		require.NoError(t, err, "admin 路径维护性更新（仅改 name）不得被拦截")
		require.Equal(t, "ollama-admin-legacy-renamed", updated.Name)
	})

	t.Run("全对象PUT带回原credentials与extra_不重验放行", func(t *testing.T) {
		updated, err := svc.UpdateAccount(ctx, account.ID, &UpdateAccountInput{
			Name:        "ollama-admin-legacy",
			Credentials: map[string]any{"api_key": "k"},
			Extra:       map[string]any{"remark": "unchanged set"},
		})
		require.NoError(t, err, "可出站集合未变的全对象 PUT 不得触发门禁")
		require.NotNil(t, updated)
	})

	t.Run("加未定价mapping_仍400", func(t *testing.T) {
		_, err := svc.UpdateAccount(ctx, account.ID, &UpdateAccountInput{
			Credentials: map[string]any{"api_key": "k", "model_mapping": map[string]any{"alias": "totally-unpriced-model"}},
		})
		requireModelPricingMissing(t, err, "totally-unpriced-model")
	})

	t.Run("不可调度改可调度_必须校验", func(t *testing.T) {
		repo.accounts[account.ID].Status = StatusDisabled
		_, err := svc.UpdateAccount(ctx, account.ID, &UpdateAccountInput{Status: StatusActive})
		requireModelPricingMissing(t, err)
	})
}

// ─────────────────────────────────────────────────────────────────────────────
// 评审收口回归：三条此前绕过门禁的 admin 写路径（BulkUpdateAccounts /
// DuplicateAccount / SetAccountSchedulable）。定位是「门禁不可绕过」的一致性——
// 运行时白名单与转发前定价预检已兜底最坏情形，本组测试锁的是保存期拦截行为
// 本身：不合规整批拒绝且不落库、错误点名账号、维护性更新与非 ollama 平台放行。
// ─────────────────────────────────────────────────────────────────────────────

// ollamaGateSchedulableCall 记录一次 SetSchedulable 调用，用于断言「拒绝时未触达 DB」。
type ollamaGateSchedulableCall struct {
	accountID   int64
	schedulable bool
}

// ollamaGateAdminRepoStub 在 upstreamBillingProbeAccountRepo 之上补齐本组回归所需的
// 两个写入口：SetSchedulable（记录调用）与 CreateWithAccountGroups（复制路径的原子创建）。
type ollamaGateAdminRepoStub struct {
	*upstreamBillingProbeAccountRepo
	schedulableCalls []ollamaGateSchedulableCall
}

func newOllamaGateAdminRepoStub() *ollamaGateAdminRepoStub {
	return &ollamaGateAdminRepoStub{
		upstreamBillingProbeAccountRepo: &upstreamBillingProbeAccountRepo{accounts: make(map[int64]*Account)},
	}
}

func (r *ollamaGateAdminRepoStub) SetSchedulable(_ context.Context, id int64, schedulable bool) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if account := r.accounts[id]; account != nil {
		account.Schedulable = schedulable
	}
	r.schedulableCalls = append(r.schedulableCalls, ollamaGateSchedulableCall{accountID: id, schedulable: schedulable})
	return nil
}

func (r *ollamaGateAdminRepoStub) CreateWithAccountGroups(ctx context.Context, account *Account, _ []AccountGroup) error {
	return r.Create(ctx, account)
}

// seedOllamaGateAccount 直接经 repo 落库一个账号（绕过 Create 路径门禁），
// 用于模拟「门禁上线前迁移过来的存量账号」。
func seedOllamaGateAccount(t *testing.T, repo *ollamaGateAdminRepoStub, mutate func(*Account)) *Account {
	t.Helper()
	account := &Account{
		Platform:    PlatformOllamaCloud,
		Type:        AccountTypeAPIKey,
		Name:        "ollama-gate-seed",
		Status:      StatusActive,
		Schedulable: false,
		Credentials: map[string]any{"api_key": "k"},
	}
	if mutate != nil {
		mutate(account)
	}
	require.NoError(t, repo.Create(context.Background(), account))
	return account
}

func TestAdminBulkUpdateAccountsOllamaCloudPricingGate(t *testing.T) {
	ctx := context.Background()
	newSvc := func(t *testing.T) (*ollamaGateAdminRepoStub, *adminServiceImpl) {
		t.Helper()
		repo := newOllamaGateAdminRepoStub()
		svc := &adminServiceImpl{accountRepo: repo, billingService: NewBillingService(&config.Config{}, nil)}
		return repo, svc
	}

	t.Run("bulk写入未定价mapping_整批拒绝点名账号且不落库", func(t *testing.T) {
		repo, svc := newSvc(t)
		first := seedOllamaGateAccount(t, repo, func(a *Account) { a.Name = "ollama-bulk-a" })
		second := seedOllamaGateAccount(t, repo, func(a *Account) {
			a.Name = "ollama-bulk-b"
			a.Extra = map[string]any{OllamaCloudAllowedModelsExtraKey: []any{"gpt-oss:120b-cloud"}}
		})
		_, err := svc.BulkUpdateAccounts(ctx, &BulkUpdateAccountsInput{
			AccountIDs:  []int64{first.ID, second.ID},
			Credentials: map[string]any{"api_key": "k", "model_mapping": map[string]any{"alias": "totally-unpriced-model"}},
		})
		requireModelPricingMissing(t, err, "totally-unpriced-model")
		// 批量操作要能定位：错误信息必须点名每个不合规账号。
		require.Contains(t, infraerrors.Message(err), first.Name)
		require.Contains(t, infraerrors.Message(err), second.Name)
		// 整批拒绝：任何账号都不得写入。
		require.Empty(t, repo.bulkUpdates, "被拒绝的批量更新不得触达 DB")
	})

	t.Run("bulk写入已定价mapping_通过", func(t *testing.T) {
		repo, svc := newSvc(t)
		account := seedOllamaGateAccount(t, repo, nil)
		_, err := svc.BulkUpdateAccounts(ctx, &BulkUpdateAccountsInput{
			AccountIDs:  []int64{account.ID},
			Credentials: map[string]any{"api_key": "k", "model_mapping": map[string]any{"big": "gpt-oss:120b"}},
		})
		require.NoError(t, err)
		require.Len(t, repo.bulkUpdates, 1)
	})

	t.Run("bulk启用legacy无配置账号_拒绝且不落库", func(t *testing.T) {
		repo, svc := newSvc(t)
		first := seedOllamaGateAccount(t, repo, func(a *Account) { a.Name = "ollama-bulk-legacy-a" })
		second := seedOllamaGateAccount(t, repo, func(a *Account) { a.Name = "ollama-bulk-legacy-b" })
		_, err := svc.BulkUpdateAccounts(ctx, &BulkUpdateAccountsInput{
			AccountIDs:  []int64{first.ID, second.ID},
			Schedulable: boolPtr(true),
		})
		requireModelPricingMissing(t, err)
		require.Contains(t, infraerrors.Message(err), first.Name)
		require.Contains(t, infraerrors.Message(err), second.Name)
		require.Empty(t, repo.bulkUpdates)
	})

	t.Run("bulk停用_放行", func(t *testing.T) {
		repo, svc := newSvc(t)
		account := seedOllamaGateAccount(t, repo, func(a *Account) {
			a.Name = "ollama-bulk-disable"
			a.Schedulable = true
		})
		_, err := svc.BulkUpdateAccounts(ctx, &BulkUpdateAccountsInput{
			AccountIDs:  []int64{account.ID},
			Schedulable: boolPtr(false),
		})
		require.NoError(t, err, "停用（退出调度准入）属于维护性更新，不得被拦截")
		require.Len(t, repo.bulkUpdates, 1)
	})

	t.Run("bulk仅改name_legacy账号放行", func(t *testing.T) {
		repo, svc := newSvc(t)
		account := seedOllamaGateAccount(t, repo, func(a *Account) { a.Name = "ollama-bulk-legacy" })
		_, err := svc.BulkUpdateAccounts(ctx, &BulkUpdateAccountsInput{
			AccountIDs: []int64{account.ID},
			Name:       "ollama-bulk-legacy-renamed",
		})
		require.NoError(t, err, "维护性更新（仅改 name）不得被 MODEL_PRICING_MISSING 锁死")
		require.Len(t, repo.bulkUpdates, 1)
	})

	t.Run("非ollama平台bulk不受影响_回归", func(t *testing.T) {
		repo, svc := newSvc(t)
		account := seedOllamaGateAccount(t, repo, func(a *Account) {
			a.Name = "kimi-bulk"
			a.Platform = PlatformKimi
			a.Schedulable = false
		})
		_, err := svc.BulkUpdateAccounts(ctx, &BulkUpdateAccountsInput{
			AccountIDs: []int64{account.ID},
			Credentials: map[string]any{
				"api_key":       "k",
				"model_mapping": map[string]any{"k2": "totally-unpriced-model"},
			},
			Schedulable: boolPtr(true),
		})
		require.NoError(t, err, "非 ollama 平台的批量更新行为必须保持不变")
		require.Len(t, repo.bulkUpdates, 1)
	})
}

func TestAdminDuplicateAccountOllamaCloudPricingGate(t *testing.T) {
	ctx := context.Background()
	newSvc := func(t *testing.T) (*ollamaGateAdminRepoStub, *adminServiceImpl) {
		t.Helper()
		repo := newOllamaGateAdminRepoStub()
		svc := &adminServiceImpl{accountRepo: repo, accountDuplicateRepo: repo, billingService: NewBillingService(&config.Config{}, nil)}
		return repo, svc
	}

	t.Run("复制不合规账号_拒绝且不落库", func(t *testing.T) {
		repo, svc := newSvc(t)
		source := seedOllamaGateAccount(t, repo, func(a *Account) {
			a.Name = "ollama-dup-bad"
			a.Credentials = map[string]any{"api_key": "k", "model_mapping": map[string]any{"alias": "totally-unpriced-model"}}
		})
		_, err := svc.DuplicateAccount(ctx, source.ID, "", "")
		requireModelPricingMissing(t, err, "totally-unpriced-model")
		require.Len(t, repo.accounts, 1, "被拒绝的复制不得落库")
	})

	t.Run("复制无配置legacy账号_拒绝", func(t *testing.T) {
		repo, svc := newSvc(t)
		source := seedOllamaGateAccount(t, repo, func(a *Account) { a.Name = "ollama-dup-legacy" })
		_, err := svc.DuplicateAccount(ctx, source.ID, "", "")
		requireModelPricingMissing(t, err)
		require.Len(t, repo.accounts, 1)
	})

	t.Run("复制已配置清单账号_通过且保持暂停", func(t *testing.T) {
		repo, svc := newSvc(t)
		source := seedOllamaGateAccount(t, repo, func(a *Account) {
			a.Name = "ollama-dup-good"
			a.Extra = map[string]any{OllamaCloudAllowedModelsExtraKey: []any{"gpt-oss:120b-cloud"}}
		})
		duplicate, err := svc.DuplicateAccount(ctx, source.ID, "", "")
		require.NoError(t, err)
		require.NotNil(t, duplicate)
		require.Len(t, repo.accounts, 2)
		require.False(t, duplicate.Schedulable, "复制产物必须保持暂停，等人工复核后再启用")
	})

	t.Run("非ollama平台复制不受影响_回归", func(t *testing.T) {
		repo, svc := newSvc(t)
		source := seedOllamaGateAccount(t, repo, func(a *Account) {
			a.Name = "kimi-dup"
			a.Platform = PlatformKimi
			a.Credentials = map[string]any{"api_key": "k", "model_mapping": map[string]any{"k2": "totally-unpriced-model"}}
		})
		duplicate, err := svc.DuplicateAccount(ctx, source.ID, "", "")
		require.NoError(t, err, "非 ollama 平台的复制行为必须保持不变")
		require.NotNil(t, duplicate)
		require.Len(t, repo.accounts, 2)
	})
}

func TestAdminSetAccountSchedulableOllamaCloudPricingGate(t *testing.T) {
	ctx := context.Background()
	newSvc := func(t *testing.T) (*ollamaGateAdminRepoStub, *adminServiceImpl) {
		t.Helper()
		repo := newOllamaGateAdminRepoStub()
		svc := &adminServiceImpl{accountRepo: repo, billingService: NewBillingService(&config.Config{}, nil)}
		return repo, svc
	}

	t.Run("启用无配置legacy账号_拒绝且不落库", func(t *testing.T) {
		repo, svc := newSvc(t)
		account := seedOllamaGateAccount(t, repo, func(a *Account) { a.Name = "ollama-enable-legacy" })
		_, err := svc.SetAccountSchedulable(ctx, account.ID, true)
		requireModelPricingMissing(t, err)
		require.Empty(t, repo.schedulableCalls, "被拒绝的启用不得触达 DB")
	})

	t.Run("停用_放行", func(t *testing.T) {
		repo, svc := newSvc(t)
		account := seedOllamaGateAccount(t, repo, func(a *Account) {
			a.Name = "ollama-disable"
			a.Schedulable = true
		})
		updated, err := svc.SetAccountSchedulable(ctx, account.ID, false)
		require.NoError(t, err, "停用（true→false）属于维护性操作，不得被拦截")
		require.NotNil(t, updated)
		require.Len(t, repo.schedulableCalls, 1)
		require.False(t, repo.schedulableCalls[0].schedulable)
	})

	t.Run("启用未定价mapping账号_拒绝", func(t *testing.T) {
		repo, svc := newSvc(t)
		account := seedOllamaGateAccount(t, repo, func(a *Account) {
			a.Name = "ollama-enable-unpriced"
			a.Credentials = map[string]any{"api_key": "k", "model_mapping": map[string]any{"alias": "totally-unpriced-model"}}
		})
		_, err := svc.SetAccountSchedulable(ctx, account.ID, true)
		requireModelPricingMissing(t, err, "totally-unpriced-model")
		require.Empty(t, repo.schedulableCalls)
	})

	t.Run("启用已定价账号_通过", func(t *testing.T) {
		repo, svc := newSvc(t)
		account := seedOllamaGateAccount(t, repo, func(a *Account) {
			a.Name = "ollama-enable-priced"
			a.Extra = map[string]any{OllamaCloudAllowedModelsExtraKey: []any{"gpt-oss:120b-cloud"}}
		})
		updated, err := svc.SetAccountSchedulable(ctx, account.ID, true)
		require.NoError(t, err)
		require.NotNil(t, updated)
		require.Len(t, repo.schedulableCalls, 1)
	})

	t.Run("非ollama平台启用不受影响_回归", func(t *testing.T) {
		repo, svc := newSvc(t)
		account := seedOllamaGateAccount(t, repo, func(a *Account) {
			a.Name = "kimi-enable"
			a.Platform = PlatformKimi
			a.Credentials = map[string]any{"api_key": "k", "model_mapping": map[string]any{"k2": "totally-unpriced-model"}}
		})
		updated, err := svc.SetAccountSchedulable(ctx, account.ID, true)
		require.NoError(t, err, "非 ollama 平台的手动启用行为必须保持不变")
		require.NotNil(t, updated)
		require.Len(t, repo.schedulableCalls, 1)
	})
}
