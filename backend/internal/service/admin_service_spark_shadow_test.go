//go:build unit

package service

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/stretchr/testify/require"
)

// sparkShadowRepoStub 是 AccountRepository 的内存测试桩，
// 专为 CreateShadow 单元测试设计。
// 嵌入 mockAccountRepoForGemini（由 gemini_multiplatform_test.go 提供所有 stub 方法），
// 并覆盖测试所需的核心方法。
type sparkShadowRepoStub struct {
	mockAccountRepoForGemini
	nextID   int64
	accounts map[int64]*Account
	groupsOf map[int64][]int64 // accountID → []groupIDs
}

func newSparkShadowRepoStub() *sparkShadowRepoStub {
	return &sparkShadowRepoStub{
		nextID:   0,
		accounts: make(map[int64]*Account),
		groupsOf: make(map[int64][]int64),
		mockAccountRepoForGemini: mockAccountRepoForGemini{
			accountsByID: make(map[int64]*Account),
		},
	}
}

func (s *sparkShadowRepoStub) Create(_ context.Context, account *Account) error {
	s.nextID++
	account.ID = s.nextID
	cp := *account
	s.accounts[account.ID] = &cp
	s.mockAccountRepoForGemini.accountsByID[account.ID] = &cp
	return nil
}

func (s *sparkShadowRepoStub) GetByID(_ context.Context, id int64) (*Account, error) {
	acc, ok := s.accounts[id]
	if !ok {
		return nil, ErrAccountNotFound
	}
	return acc, nil
}

func (s *sparkShadowRepoStub) ListShadowsByParent(_ context.Context, parentID int64) ([]*Account, error) {
	var result []*Account
	for _, acc := range s.accounts {
		if acc.ParentAccountID != nil && *acc.ParentAccountID == parentID && acc.QuotaDimension == QuotaDimensionSpark {
			cp := *acc
			result = append(result, &cp)
		}
	}
	return result, nil
}

func (s *sparkShadowRepoStub) BindGroups(_ context.Context, accountID int64, groupIDs []int64) error {
	s.groupsOf[accountID] = append(s.groupsOf[accountID], groupIDs...)
	return nil
}

func (s *sparkShadowRepoStub) ListSchedulableByGroupID(_ context.Context, groupID int64) ([]Account, error) {
	var result []Account
	for accID, groups := range s.groupsOf {
		for _, gid := range groups {
			if gid == groupID {
				if acc, ok := s.accounts[accID]; ok {
					result = append(result, *acc)
				}
				break
			}
		}
	}
	return result, nil
}

// ListWithFilters は mockAccountRepoForGemini にないが AccountRepository が要求する。
// 親の mockAccountRepoForGemini の nil 実装が継承されるため、ここでは省略可。

// ── 追加 stub（AccountRepository に必要な残りのメソッド）──────────────────
func (s *sparkShadowRepoStub) ExistsByID(_ context.Context, id int64) (bool, error) {
	_, ok := s.accounts[id]
	return ok, nil
}
func (s *sparkShadowRepoStub) Update(_ context.Context, account *Account) error {
	if _, ok := s.accounts[account.ID]; !ok {
		return ErrAccountNotFound
	}
	cp := *account
	s.accounts[account.ID] = &cp
	s.mockAccountRepoForGemini.accountsByID[account.ID] = &cp
	return nil
}

func (s *sparkShadowRepoStub) Delete(_ context.Context, id int64) error {
	delete(s.accounts, id)
	delete(s.mockAccountRepoForGemini.accountsByID, id)
	return nil
}
func (s *sparkShadowRepoStub) BatchUpdateLastUsed(_ context.Context, _ map[int64]time.Time) error {
	return nil
}
func (s *sparkShadowRepoStub) ListByGroup(_ context.Context, _ int64) ([]Account, error) {
	return nil, nil
}
func (s *sparkShadowRepoStub) ListWithFilters(_ context.Context, _ pagination.PaginationParams, _, _, _, _ string, _ int64, _ string) ([]Account, *pagination.PaginationResult, error) {
	return nil, nil, nil
}

// TestCreateShadow はメインのシナリオを検証する。
//
// Test 1 — 基本生成: ParentAccountID / QuotaDimension / 默认 spark model_mapping / 无 auth token / ProxyID 継承
// Test 2 — 一母一影: 二度目の生成はエラー
func TestCreateShadow(t *testing.T) {
	ctx := context.Background()
	repo := newSparkShadowRepoStub()
	svc := &adminServiceImpl{accountRepo: repo}

	proxyID := int64(7)
	parent := &Account{
		Name:     "p",
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
		Status:   StatusActive,
		ProxyID:  &proxyID,
		Credentials: map[string]any{
			"refresh_token":      "RT",
			"chatgpt_account_id": "org-x",
		},
	}
	require.NoError(t, repo.Create(ctx, parent))

	// Test 1: 基本生成
	shadow, err := svc.CreateShadow(ctx, parent.ID, ShadowOptions{Name: "p-spark", Priority: 50})
	require.NoError(t, err)
	require.NotNil(t, shadow)
	require.Equal(t, parent.ID, *shadow.ParentAccountID)
	require.Equal(t, QuotaDimensionSpark, shadow.QuotaDimension)
	require.Equal(t, defaultSparkShadowModelMapping(), shadow.Credentials["model_mapping"],
		"影子默认带 spark 恒等变体映射")
	require.Nil(t, shadow.Credentials["refresh_token"], "影子不得持有 auth token")
	require.Nil(t, shadow.Credentials["access_token"], "影子不得持有 auth token")
	require.Equal(t, parent.ProxyID, shadow.ProxyID)

	// Test 2: 一母一影 — 再作成は拒否
	_, err = svc.CreateShadow(ctx, parent.ID, ShadowOptions{Name: "dup"})
	require.Error(t, err)
}

func TestCreateShadowInheritsParentEffectiveOpenAILongContextBillingValue(t *testing.T) {
	tests := []struct {
		name        string
		parentExtra map[string]any
		want        bool
	}{
		{name: "missing parent value defaults disabled", want: false},
		{name: "explicit parent opt-out is inherited", parentExtra: map[string]any{openAILongContextBillingEnabledKey: false}, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := newSparkShadowRepoStub()
			svc := &adminServiceImpl{accountRepo: repo}
			parent := &Account{
				Name:        "parent",
				Platform:    PlatformOpenAI,
				Type:        AccountTypeOAuth,
				Status:      StatusActive,
				Credentials: map[string]any{"access_token": "token"},
				Extra:       tt.parentExtra,
			}
			require.NoError(t, repo.Create(context.Background(), parent))

			shadow, err := svc.CreateShadow(context.Background(), parent.ID, ShadowOptions{Name: "shadow"})

			require.NoError(t, err)
			require.Equal(t, tt.want, shadow.Extra[openAILongContextBillingEnabledKey])
			require.Equal(t, tt.want, shadow.IsOpenAILongContextBillingEnabled())
		})
	}
}

// TestCreateShadow_BindGroups は BindGroups の後置呼び出しを検証する。
// 影子账号が指定グループに属し、ListSchedulableByGroupID で取得可能であること。
func TestCreateShadow_BindGroups(t *testing.T) {
	ctx := context.Background()
	repo := newSparkShadowRepoStub()
	svc := &adminServiceImpl{accountRepo: repo}

	parent := &Account{
		Name:     "parent",
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
		Status:   StatusActive,
		Credentials: map[string]any{
			"chatgpt_account_id": "org-y",
		},
	}
	require.NoError(t, repo.Create(ctx, parent))

	const testGroupID = int64(42)
	shadow, err := svc.CreateShadow(ctx, parent.ID, ShadowOptions{
		Name:     "p-spark",
		GroupIDs: []int64{testGroupID},
	})
	require.NoError(t, err)
	require.NotNil(t, shadow)
	require.Equal(t, []int64{testGroupID}, shadow.GroupIDs, "CreateShadow should backfill GroupIDs into the returned shadow")

	accounts, err := repo.ListSchedulableByGroupID(ctx, testGroupID)
	require.NoError(t, err)
	require.Len(t, accounts, 1)
	require.Equal(t, shadow.ID, accounts[0].ID)
}

// TestDeleteAccount_CascadeToShadow verifies that deleting a parent account also
// deletes its spark shadow account.
// TestCreateShadow_InheritsParentConcurrency 验证外审 F3:未指定并发时
// 影子继承母账号并发,避免 Concurrency=0 被限流器当作"无限并发"。
func TestCreateShadow_InheritsParentConcurrency(t *testing.T) {
	ctx := context.Background()

	t.Run("unspecified_inherits_parent", func(t *testing.T) {
		repo := newSparkShadowRepoStub()
		svc := &adminServiceImpl{accountRepo: repo}
		parent := &Account{
			Name: "conc-parent", Platform: PlatformOpenAI, Type: AccountTypeOAuth,
			Status: StatusActive, Concurrency: 3,
			Credentials: map[string]any{"chatgpt_account_id": "org-c"},
		}
		require.NoError(t, repo.Create(ctx, parent))

		shadow, err := svc.CreateShadow(ctx, parent.ID, ShadowOptions{Name: "conc-shadow"})
		require.NoError(t, err)
		require.Equal(t, 3, shadow.Concurrency, "未指定并发应继承母账号(非 0=无限)")
		require.Equal(t, 3, repo.accounts[shadow.ID].Concurrency)
	})

	t.Run("explicit_positive_kept", func(t *testing.T) {
		repo := newSparkShadowRepoStub()
		svc := &adminServiceImpl{accountRepo: repo}
		parent := &Account{
			Name: "conc-parent2", Platform: PlatformOpenAI, Type: AccountTypeOAuth,
			Status: StatusActive, Concurrency: 3,
			Credentials: map[string]any{"chatgpt_account_id": "org-c2"},
		}
		require.NoError(t, repo.Create(ctx, parent))

		shadow, err := svc.CreateShadow(ctx, parent.ID, ShadowOptions{Name: "conc-shadow2", Concurrency: 2})
		require.NoError(t, err)
		require.Equal(t, 2, shadow.Concurrency, "显式正并发应保留")
	})
}

// TestCreateShadow_InheritsParentPriorityWhenOmitted 验证外审第5轮 P1:未指定优先级时
// 影子继承母账号 priority,而非直写 0 抢到最高调度优先级(repo SetPriority 绕过 ent 默认 50,
// 调度比较数值越小越优先;前端一键创建只传 name 即触发该路径)。
func TestCreateShadow_InheritsParentPriorityWhenOmitted(t *testing.T) {
	ctx := context.Background()

	t.Run("unspecified_inherits_parent", func(t *testing.T) {
		repo := newSparkShadowRepoStub()
		svc := &adminServiceImpl{accountRepo: repo}
		parent := &Account{
			Name: "prio-parent", Platform: PlatformOpenAI, Type: AccountTypeOAuth,
			Status: StatusActive, Priority: 30,
			Credentials: map[string]any{"chatgpt_account_id": "org-p"},
		}
		require.NoError(t, repo.Create(ctx, parent))

		// 模拟前端一键创建:只传 name,priority 省略=0。
		shadow, err := svc.CreateShadow(ctx, parent.ID, ShadowOptions{Name: "prio-shadow"})
		require.NoError(t, err)
		require.Equal(t, 30, shadow.Priority, "未指定优先级应继承母账号(而非 0=最高优先级)")
		require.Equal(t, 30, repo.accounts[shadow.ID].Priority)
	})

	t.Run("explicit_positive_kept", func(t *testing.T) {
		repo := newSparkShadowRepoStub()
		svc := &adminServiceImpl{accountRepo: repo}
		parent := &Account{
			Name: "prio-parent2", Platform: PlatformOpenAI, Type: AccountTypeOAuth,
			Status: StatusActive, Priority: 30,
			Credentials: map[string]any{"chatgpt_account_id": "org-p2"},
		}
		require.NoError(t, repo.Create(ctx, parent))

		shadow, err := svc.CreateShadow(ctx, parent.ID, ShadowOptions{Name: "prio-shadow2", Priority: 7})
		require.NoError(t, err)
		require.Equal(t, 7, shadow.Priority, "显式正优先级应保留")
	})
}

// TestPersistAccountCredentials_SkipsShadow 验证外审第6轮 P1:凭据写入唯一汇聚点
// persistAccountCredentials 对 spark 影子早返 no-op,任何上游路径都无法把凭据落到影子行。
func TestPersistAccountCredentials_SkipsShadow(t *testing.T) {
	ctx := context.Background()
	repo := newSparkShadowRepoStub()
	parentID := int64(1)
	shadow := &Account{
		Name: "shadow", Platform: PlatformOpenAI, Type: AccountTypeOAuth,
		Status: StatusActive, Credentials: map[string]any{},
		ParentAccountID: &parentID, QuotaDimension: QuotaDimensionSpark,
	}
	require.NoError(t, repo.Create(ctx, shadow)) // 回填 shadow.ID,使「漏判则 Update 成功」可被检出

	err := persistAccountCredentials(ctx, repo, shadow, map[string]any{"access_token": "LEAK", "refresh_token": "LEAK"})
	require.NoError(t, err)
	require.Empty(t, shadow.Credentials, "影子凭据不可被写入(传入对象)")
	require.Empty(t, repo.accounts[shadow.ID].Credentials, "影子凭据不可被写入(仓储)")
}

// TestResolveCredentialAccount_RejectsParentShadow 验证外审第6轮 P2 防御:畸形数据/手工 DB
// 写出的「影子→影子」链,凭据解析必须 fail-closed 而非停在无凭据的一级影子。
func TestResolveCredentialAccount_RejectsParentShadow(t *testing.T) {
	ctx := context.Background()
	repo := newSparkShadowRepoStub()

	grandparent := &Account{
		Name: "gp", Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive,
		Credentials: map[string]any{"refresh_token": "RT"},
	}
	require.NoError(t, repo.Create(ctx, grandparent))
	// parent 本身是影子(非法二级结构)
	parentShadow := &Account{
		Name: "parent-shadow", Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive,
		Credentials: map[string]any{}, ParentAccountID: &grandparent.ID, QuotaDimension: QuotaDimensionSpark,
	}
	require.NoError(t, repo.Create(ctx, parentShadow))
	child := &Account{
		Name: "child-shadow", Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive,
		Credentials: map[string]any{}, ParentAccountID: &parentShadow.ID, QuotaDimension: QuotaDimensionSpark,
	}
	require.NoError(t, repo.Create(ctx, child))

	_, err := resolveCredentialAccount(ctx, repo, child)
	require.Error(t, err, "父账号本身是影子时凭据解析应拒绝(fail-closed)")
}

// TestPersistOpenAI429PlanType_SkipsShadow 验证外审第7轮 P1:429 plan_type 同步走 BulkUpdate 直写
// (不经 persistAccountCredentials),必须对影子早返,否则会把 plan_type 写进影子 credentials。
func TestPersistOpenAI429PlanType_SkipsShadow(t *testing.T) {
	ctx := context.Background()
	body := []byte(`{"error":{"type":"usage_limit_reached","plan_type":"pro"}}`)
	parentID := int64(1)

	t.Run("shadow_skipped", func(t *testing.T) {
		repo := newSparkShadowRepoStub()
		shadow := &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth, Credentials: map[string]any{}, ParentAccountID: &parentID}
		persistOpenAI429PlanType(ctx, repo, shadow, body)
		require.Empty(t, shadow.Credentials, "影子不可被写入 plan_type 凭据")
	})

	t.Run("normal_account_writes", func(t *testing.T) {
		repo := newSparkShadowRepoStub()
		normal := &Account{ID: 9, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Credentials: map[string]any{}}
		persistOpenAI429PlanType(ctx, repo, normal, body)
		require.Equal(t, "pro", normal.Credentials["plan_type"], "普通账号应写入 plan_type(反向对照,证明 body 有效、写路径通)")
	})
}

// updateExtraSpyRepo 记录 UpdateExtra 是否被调用,用于验证影子 codex_* 快照守卫。
type updateExtraSpyRepo struct {
	*sparkShadowRepoStub
	updateExtraCalled bool
}

func (r *updateExtraSpyRepo) UpdateExtra(_ context.Context, _ int64, _ map[string]any) error {
	r.updateExtraCalled = true
	return nil
}

// TestPersistOpenAICodexSnapshot_SkipsShadow 验证外审第7轮 P1:影子 codex_* 仅由 QueryUsage
// (/wham/usage bengalfox)更新,不能被 429 路径的 x-codex-* 全局头快照污染。
func TestPersistOpenAICodexSnapshot_SkipsShadow(t *testing.T) {
	headers := http.Header{}
	headers.Set("x-codex-primary-used-percent", "50")
	parentID := int64(1)

	t.Run("shadow_skipped", func(t *testing.T) {
		spy := &updateExtraSpyRepo{sparkShadowRepoStub: newSparkShadowRepoStub()}
		s := &RateLimitService{accountRepo: spy}
		shadow := &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth, ParentAccountID: &parentID}
		s.persistOpenAICodexSnapshot(context.Background(), shadow, headers)
		require.False(t, spy.updateExtraCalled, "影子不应写 codex_* 头快照")
	})

	t.Run("normal_account_writes", func(t *testing.T) {
		spy := &updateExtraSpyRepo{sparkShadowRepoStub: newSparkShadowRepoStub()}
		s := &RateLimitService{accountRepo: spy}
		normal := &Account{ID: 9, Platform: PlatformOpenAI, Type: AccountTypeOAuth}
		s.persistOpenAICodexSnapshot(context.Background(), normal, headers)
		require.True(t, spy.updateExtraCalled, "普通账号应写 codex_* 头快照(反向对照)")
	})
}

// TestResetAccountQuota_RejectsShadow 验证外审第7轮 P2:通用 reset-quota 对影子明确 400 拒绝
// (影子不持自有配额,语义不一致),且母账号仍可正常重置。
func TestResetAccountQuota_RejectsShadow(t *testing.T) {
	ctx := context.Background()
	repo := newSparkShadowRepoStub()
	svc := &adminServiceImpl{accountRepo: repo}
	parent := &Account{
		Name: "rq-parent", Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive,
		Credentials: map[string]any{"chatgpt_account_id": "o"},
	}
	require.NoError(t, repo.Create(ctx, parent))
	shadow, err := svc.CreateShadow(ctx, parent.ID, ShadowOptions{Name: "rq-shadow"})
	require.NoError(t, err)

	err = svc.ResetAccountQuota(ctx, shadow.ID)
	require.Error(t, err)
	require.Equal(t, http.StatusBadRequest, infraerrors.Code(err), "影子 reset-quota 应 400")

	require.NoError(t, svc.ResetAccountQuota(ctx, parent.ID), "母账号 reset-quota 应放行")
}

// sparkShadowGroupRepoStub 嵌入 groupRepoStub(其余方法 panic),仅覆写
// ListActiveByPlatform 以供 F4 默认绑组测试。
type sparkShadowGroupRepoStub struct {
	groupRepoStub
	groups []Group
}

func (s *sparkShadowGroupRepoStub) ListActiveByPlatform(_ context.Context, _ string) ([]Group, error) {
	return s.groups, nil
}

// TestCreateShadow_DefaultGroupBinding 验证外审 F4:未指定 group_ids 时
// 影子回落绑定 openai-default 组(否则无组、组内路由选不到)。
func TestCreateShadow_DefaultGroupBinding(t *testing.T) {
	ctx := context.Background()
	repo := newSparkShadowRepoStub()
	groupRepo := &sparkShadowGroupRepoStub{
		groups: []Group{
			{ID: 99, Name: PlatformOpenAI + "-default"},
			{ID: 7, Name: "some-other-group"},
		},
	}
	svc := &adminServiceImpl{accountRepo: repo, groupRepo: groupRepo}

	parent := &Account{
		Name: "grp-parent", Platform: PlatformOpenAI, Type: AccountTypeOAuth,
		Status: StatusActive, Credentials: map[string]any{"chatgpt_account_id": "org-g"},
	}
	require.NoError(t, repo.Create(ctx, parent))

	shadow, err := svc.CreateShadow(ctx, parent.ID, ShadowOptions{Name: "grp-shadow"})
	require.NoError(t, err)
	require.Equal(t, []int64{99}, repo.groupsOf[shadow.ID], "未指定分组应回落绑定 openai-default(id=99)")
}

// TestCreateShadow_InheritsParentGroups 验证外审 G1:未指定 group_ids 时
// 影子继承母账号当前分组(而非仅 openai-default),以便母在自定义组时影子也可路由。
func TestCreateShadow_InheritsParentGroups(t *testing.T) {
	ctx := context.Background()
	repo := newSparkShadowRepoStub()
	// groupRepo 故意提供 openai-default,以证明「继承母分组」优先于「回落 openai-default」。
	groupRepo := &sparkShadowGroupRepoStub{groups: []Group{{ID: 99, Name: PlatformOpenAI + "-default"}}}
	svc := &adminServiceImpl{accountRepo: repo, groupRepo: groupRepo}

	parent := &Account{
		Name: "grp-parent", Platform: PlatformOpenAI, Type: AccountTypeOAuth,
		Status: StatusActive, GroupIDs: []int64{11, 22},
		Credentials: map[string]any{"chatgpt_account_id": "org-grp"},
	}
	require.NoError(t, repo.Create(ctx, parent))

	shadow, err := svc.CreateShadow(ctx, parent.ID, ShadowOptions{Name: "grp-shadow"})
	require.NoError(t, err)
	require.Equal(t, []int64{11, 22}, repo.groupsOf[shadow.ID], "未指定分组应继承母账号分组,而非 openai-default")
}

// TestCreateShadow_RejectsShadowAsParent 验证外审 G6:不允许把影子当母创建二级影子。
func TestCreateShadow_RejectsShadowAsParent(t *testing.T) {
	ctx := context.Background()
	repo := newSparkShadowRepoStub()
	svc := &adminServiceImpl{accountRepo: repo}

	parent := &Account{
		Name: "real-parent", Platform: PlatformOpenAI, Type: AccountTypeOAuth,
		Status: StatusActive, Credentials: map[string]any{"chatgpt_account_id": "org-x"},
	}
	require.NoError(t, repo.Create(ctx, parent))
	firstShadow, err := svc.CreateShadow(ctx, parent.ID, ShadowOptions{Name: "first-shadow"})
	require.NoError(t, err)

	// 把一级影子当母 → 必须被拒(400)。
	_, err = svc.CreateShadow(ctx, firstShadow.ID, ShadowOptions{Name: "second-shadow"})
	require.Error(t, err)
	require.Equal(t, http.StatusBadRequest, infraerrors.Code(err), "影子当母应返回 400")
}

// TestCreateShadow_StructuredErrors 验证外审 G3:可预期业务错误返回结构化 4xx 而非 500。
func TestCreateShadow_StructuredErrors(t *testing.T) {
	ctx := context.Background()

	t.Run("non_oauth_parent_400", func(t *testing.T) {
		repo := newSparkShadowRepoStub()
		svc := &adminServiceImpl{accountRepo: repo}
		parent := &Account{Name: "apikey-parent", Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive}
		require.NoError(t, repo.Create(ctx, parent))
		_, err := svc.CreateShadow(ctx, parent.ID, ShadowOptions{Name: "s"})
		require.Error(t, err)
		require.Equal(t, http.StatusBadRequest, infraerrors.Code(err), "非 OAuth 母账号应 400")
	})

	t.Run("duplicate_409", func(t *testing.T) {
		repo := newSparkShadowRepoStub()
		svc := &adminServiceImpl{accountRepo: repo}
		parent := &Account{Name: "p", Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Credentials: map[string]any{"chatgpt_account_id": "o"}}
		require.NoError(t, repo.Create(ctx, parent))
		_, err := svc.CreateShadow(ctx, parent.ID, ShadowOptions{Name: "s1"})
		require.NoError(t, err)
		_, err = svc.CreateShadow(ctx, parent.ID, ShadowOptions{Name: "s2"})
		require.Error(t, err)
		require.Equal(t, http.StatusConflict, infraerrors.Code(err), "重复创建应 409")
	})
}

// TestUpdateAccount_RejectsTypeChangeOnShadow 验证外审 G7:影子 type 不可被普通更新改坏。
func TestUpdateAccount_RejectsTypeChangeOnShadow(t *testing.T) {
	ctx := context.Background()
	repo := newSparkShadowRepoStub()
	svc := &adminServiceImpl{accountRepo: repo}
	parent := &Account{
		Name: "type-parent", Platform: PlatformOpenAI, Type: AccountTypeOAuth,
		Status: StatusActive, Credentials: map[string]any{"chatgpt_account_id": "org-t"},
	}
	require.NoError(t, repo.Create(ctx, parent))
	shadow, err := svc.CreateShadow(ctx, parent.ID, ShadowOptions{Name: "type-shadow"})
	require.NoError(t, err)

	// 试图把影子 type 改成 apikey → 必须被拒(400)。
	_, err = svc.UpdateAccount(ctx, shadow.ID, &UpdateAccountInput{Type: AccountTypeAPIKey})
	require.Error(t, err)
	require.Equal(t, http.StatusBadRequest, infraerrors.Code(err), "改影子 type 应 400")
	require.Equal(t, AccountTypeOAuth, repo.accounts[shadow.ID].Type, "影子 type 必须保持 oauth")

	// 传入相同 type(oauth)为 no-op,应允许。
	_, err = svc.UpdateAccount(ctx, shadow.ID, &UpdateAccountInput{Type: AccountTypeOAuth})
	require.NoError(t, err, "传入相同 type 应允许")
}

// TestBulkUpdateAccounts_RejectsCredentialWriteToShadow 验证外审 G5:批量更新携带凭据时
// 目标含影子必须被拒(与单账号 UpdateAccount 守卫对齐,堵住 bulk 绕过)。
func TestBulkUpdateAccounts_RejectsCredentialWriteToShadow(t *testing.T) {
	ctx := context.Background()
	repo := newSparkShadowRepoStub()
	svc := &adminServiceImpl{accountRepo: repo}
	parent := &Account{
		Name: "bulk-parent", Platform: PlatformOpenAI, Type: AccountTypeOAuth,
		Status: StatusActive, Credentials: map[string]any{"chatgpt_account_id": "org-b", "access_token": "t"},
	}
	require.NoError(t, repo.Create(ctx, parent))
	shadow, err := svc.CreateShadow(ctx, parent.ID, ShadowOptions{Name: "bulk-shadow"})
	require.NoError(t, err)

	_, err = svc.BulkUpdateAccounts(ctx, &BulkUpdateAccountsInput{
		AccountIDs:  []int64{shadow.ID},
		Credentials: map[string]any{"access_token": "leaked"},
	})
	require.Error(t, err, "批量给影子写凭据必须被拒")
	require.Equal(t, http.StatusBadRequest, infraerrors.Code(err), "应 400")
	// Credentials 允许持有 model_mapping(CreateShadow 写入的默认值),该不变量只约束
	// 鉴权凭据不可泄露到影子——不能整体断言 Credentials 为空。
	require.Empty(t, repo.accounts[shadow.ID].GetOpenAIAccessToken(), "影子 access_token 必须保持为空 —— 批量写入未生效")
}

func TestDeleteAccount_CascadeToShadow(t *testing.T) {
	ctx := context.Background()
	repo := newSparkShadowRepoStub()
	svc := &adminServiceImpl{accountRepo: repo}

	parent := &Account{
		Name:        "cascade-parent",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeOAuth,
		Status:      StatusActive,
		Credentials: map[string]any{"chatgpt_account_id": "org-cascade"},
	}
	require.NoError(t, repo.Create(ctx, parent))

	shadow, err := svc.CreateShadow(ctx, parent.ID, ShadowOptions{Name: "cascade-shadow"})
	require.NoError(t, err)
	shadowID := shadow.ID

	// Both accounts exist before delete.
	_, ok := repo.accounts[parent.ID]
	require.True(t, ok)
	_, ok = repo.accounts[shadowID]
	require.True(t, ok)

	require.NoError(t, svc.DeleteAccount(ctx, parent.ID))

	// Parent is gone.
	_, ok = repo.accounts[parent.ID]
	require.False(t, ok, "parent account should be deleted")
	// Shadow is also gone (cascade).
	_, ok = repo.accounts[shadowID]
	require.False(t, ok, "shadow account should be cascade-deleted")
}

// TestUpdateAccount_PropagatesProxyToShadow verifies that updating a parent
// account's ProxyID propagates the new value to its spark shadow.
func TestUpdateAccount_PropagatesProxyToShadow(t *testing.T) {
	ctx := context.Background()
	repo := newSparkShadowRepoStub()
	svc := &adminServiceImpl{accountRepo: repo}

	oldProxy := int64(7)
	parent := &Account{
		Name:        "proxy-parent",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeOAuth,
		Status:      StatusActive,
		ProxyID:     &oldProxy,
		Credentials: map[string]any{"chatgpt_account_id": "org-proxy"},
	}
	require.NoError(t, repo.Create(ctx, parent))

	shadow, err := svc.CreateShadow(ctx, parent.ID, ShadowOptions{Name: "proxy-shadow"})
	require.NoError(t, err)
	shadowID := shadow.ID

	// Update parent's ProxyID.
	newProxy := int64(42)
	_, err = svc.UpdateAccount(ctx, parent.ID, &UpdateAccountInput{ProxyID: &newProxy})
	require.NoError(t, err)

	// Shadow must carry the new ProxyID.
	storedShadow, ok := repo.accounts[shadowID]
	require.True(t, ok)
	require.NotNil(t, storedShadow.ProxyID)
	require.Equal(t, newProxy, *storedShadow.ProxyID)
}

// TestUpdateAccount_RejectsCredentialWriteToShadow 验证安全不变量「影子绝不持有鉴权凭据」
// 在通用更新路径(UpdateAccount,被 edit/re-auth/refresh/batch 共用)上也被守住:
// 对影子写入 access_token/refresh_token 必须被拒绝,且影子的 access_token/refresh_token
// 保持为空(Credentials 本身允许持有 CreateShadow 写入的 model_mapping,故不能断言整体为空)。
func TestUpdateAccount_RejectsCredentialWriteToShadow(t *testing.T) {
	ctx := context.Background()
	repo := newSparkShadowRepoStub()
	svc := &adminServiceImpl{accountRepo: repo}

	parent := &Account{
		Name:        "cred-parent",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeOAuth,
		Status:      StatusActive,
		Credentials: map[string]any{"access_token": "parent-secret", "refresh_token": "parent-rt"},
	}
	require.NoError(t, repo.Create(ctx, parent))

	shadow, err := svc.CreateShadow(ctx, parent.ID, ShadowOptions{Name: "cred-shadow"})
	require.NoError(t, err)
	require.Empty(t, shadow.GetOpenAIAccessToken(), "前提:影子创建后不持有 access_token")
	require.Empty(t, shadow.GetOpenAIRefreshToken(), "前提:影子创建后不持有 refresh_token")

	// 试图给影子写入凭据 → 必须被拒绝。
	_, err = svc.UpdateAccount(ctx, shadow.ID, &UpdateAccountInput{
		Credentials: map[string]any{"access_token": "leaked", "refresh_token": "leaked-rt"},
	})
	require.Error(t, err, "对影子写入凭据必须被拒绝")

	// 结构化 4xx(非裸 error→500)。
	require.Equal(t, http.StatusBadRequest, infraerrors.Code(err), "应映射为 400 而非 500")

	// 影子的 access_token/refresh_token 仍为空 —— 凭据未被写入。
	storedShadow, ok := repo.accounts[shadow.ID]
	require.True(t, ok)
	require.Empty(t, storedShadow.GetOpenAIAccessToken(), "影子 access_token 必须保持为空 —— 凭据未被写入")
	require.Empty(t, storedShadow.GetOpenAIRefreshToken(), "影子 refresh_token 必须保持为空 —— 凭据未被写入")

	// 对照:不带凭据的字段更新(如 Priority)仍应成功。
	newPriority := 5
	_, err = svc.UpdateAccount(ctx, shadow.ID, &UpdateAccountInput{Priority: &newPriority})
	require.NoError(t, err, "影子的非凭据字段更新应正常")
}

// TestBulkUpdateAccounts_PropagatesProxyToShadow verifies that bulk-updating
// accounts' ProxyID propagates the new value to each account's spark shadow.
func TestBulkUpdateAccounts_PropagatesProxyToShadow(t *testing.T) {
	ctx := context.Background()
	repo := newSparkShadowRepoStub()
	svc := &adminServiceImpl{accountRepo: repo}

	oldProxy := int64(7)
	parent := &Account{
		Name:        "bulk-parent",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeOAuth,
		Status:      StatusActive,
		ProxyID:     &oldProxy,
		Credentials: map[string]any{"chatgpt_account_id": "org-bulk"},
	}
	require.NoError(t, repo.Create(ctx, parent))

	shadow, err := svc.CreateShadow(ctx, parent.ID, ShadowOptions{Name: "bulk-shadow"})
	require.NoError(t, err)
	shadowID := shadow.ID

	// Bulk update parent's ProxyID.
	newProxy := int64(99)
	_, err = svc.BulkUpdateAccounts(ctx, &BulkUpdateAccountsInput{
		AccountIDs: []int64{parent.ID},
		ProxyID:    &newProxy,
	})
	require.NoError(t, err)

	// Shadow must carry the new ProxyID.
	storedShadow, ok := repo.accounts[shadowID]
	require.True(t, ok)
	require.NotNil(t, storedShadow.ProxyID)
	require.Equal(t, newProxy, *storedShadow.ProxyID)
}

// ── 外审 P1/P2 加固:专用测试桩 ───────────────────────────────────────────

// raceCreateRepoStub 模拟并发竞态:对影子的 Create 撞一母一影唯一索引(返回错误),
// 且复查时另一并发请求的影子已存在 → CreateShadow 应映射为结构化 409(外审 A/P1)。
type raceCreateRepoStub struct {
	*sparkShadowRepoStub
}

func (s *raceCreateRepoStub) Create(ctx context.Context, account *Account) error {
	if account.ParentAccountID != nil {
		// 模拟另一并发请求已抢先建成影子:注入底层 map,本次 Create 撞唯一索引失败。
		s.sparkShadowRepoStub.nextID++
		phantom := *account
		phantom.ID = s.sparkShadowRepoStub.nextID
		s.sparkShadowRepoStub.accounts[phantom.ID] = &phantom
		return errors.New(`duplicate key value violates unique constraint "uq_accounts_spark_shadow_per_parent"`)
	}
	return s.sparkShadowRepoStub.Create(ctx, account)
}

// bindFailRepoStub 让 BindGroups 失败,用于验证绑组失败时补偿删除刚建的影子(外审 C/P1)。
type bindFailRepoStub struct {
	*sparkShadowRepoStub
}

func (s *bindFailRepoStub) BindGroups(_ context.Context, _ int64, _ []int64) error {
	return errors.New("simulated bind failure")
}

// sparkShadowValidatingGroupRepoStub 实现 groupExistenceBatchReader(ExistsByIDs),
// 使 validateGroupIDsExist 走批量存在性校验路径。
type sparkShadowValidatingGroupRepoStub struct {
	groupRepoStub
	existing map[int64]bool
}

func (s *sparkShadowValidatingGroupRepoStub) ExistsByIDs(_ context.Context, ids []int64) (map[int64]bool, error) {
	out := make(map[int64]bool, len(ids))
	for _, id := range ids {
		out[id] = s.existing[id]
	}
	return out, nil
}

// TestCreateShadow_DefaultsNameFromParent 验证外审 E/P2:空 name 不应 500,
// 而是默认 "<母账号名> (Spark)"。
func TestCreateShadow_DefaultsNameFromParent(t *testing.T) {
	ctx := context.Background()
	repo := newSparkShadowRepoStub()
	svc := &adminServiceImpl{accountRepo: repo}
	parent := &Account{
		Name: "mum", Platform: PlatformOpenAI, Type: AccountTypeOAuth,
		Status: StatusActive, Credentials: map[string]any{"chatgpt_account_id": "o"},
	}
	require.NoError(t, repo.Create(ctx, parent))

	shadow, err := svc.CreateShadow(ctx, parent.ID, ShadowOptions{Name: "   "})
	require.NoError(t, err, "空/空白 name 不应 500,应默认命名")
	require.Equal(t, "mum (Spark)", shadow.Name)
}

// TestCreateShadow_ConcurrentCreateReturns409 验证外审 A/P1:并发竞态下预查放行后
// Create 撞唯一索引,应映射结构化 409 而非裸 500。
func TestCreateShadow_ConcurrentCreateReturns409(t *testing.T) {
	ctx := context.Background()
	base := newSparkShadowRepoStub()
	repo := &raceCreateRepoStub{sparkShadowRepoStub: base}
	svc := &adminServiceImpl{accountRepo: repo}
	parent := &Account{
		Name: "p", Platform: PlatformOpenAI, Type: AccountTypeOAuth,
		Status: StatusActive, Credentials: map[string]any{"chatgpt_account_id": "o"},
	}
	require.NoError(t, base.Create(ctx, parent))

	_, err := svc.CreateShadow(ctx, parent.ID, ShadowOptions{Name: "s"})
	require.Error(t, err)
	require.Equal(t, http.StatusConflict, infraerrors.Code(err), "并发竞态撞唯一索引应映射 409 而非 500")
}

// TestCreateShadow_InvalidGroupRejectedNoOrphan 验证外审 C/P1:显式无效分组应在
// 创建前被拒,不留孤儿影子。
func TestCreateShadow_InvalidGroupRejectedNoOrphan(t *testing.T) {
	ctx := context.Background()
	repo := newSparkShadowRepoStub()
	groupRepo := &sparkShadowValidatingGroupRepoStub{existing: map[int64]bool{7: true}}
	svc := &adminServiceImpl{accountRepo: repo, groupRepo: groupRepo}
	parent := &Account{
		Name: "p", Platform: PlatformOpenAI, Type: AccountTypeOAuth,
		Status: StatusActive, Credentials: map[string]any{"chatgpt_account_id": "o"},
	}
	require.NoError(t, repo.Create(ctx, parent))

	_, err := svc.CreateShadow(ctx, parent.ID, ShadowOptions{Name: "s", GroupIDs: []int64{999}})
	require.Error(t, err, "无效分组应在创建前被拒")

	shadows, qerr := repo.ListShadowsByParent(ctx, parent.ID)
	require.NoError(t, qerr)
	require.Empty(t, shadows, "无效分组应在创建前被拒,不应建出影子")
}

// TestCreateShadow_BindFailureRollsBackShadow 验证外审 C/P1:绑组失败时补偿删除
// 刚建的影子,不留孤儿(否则一母一影唯一索引会挡住重试)。
func TestCreateShadow_BindFailureRollsBackShadow(t *testing.T) {
	ctx := context.Background()
	base := newSparkShadowRepoStub()
	repo := &bindFailRepoStub{sparkShadowRepoStub: base}
	groupRepo := &sparkShadowValidatingGroupRepoStub{existing: map[int64]bool{7: true}}
	svc := &adminServiceImpl{accountRepo: repo, groupRepo: groupRepo}
	parent := &Account{
		Name: "p", Platform: PlatformOpenAI, Type: AccountTypeOAuth,
		Status: StatusActive, Credentials: map[string]any{"chatgpt_account_id": "o"},
	}
	require.NoError(t, base.Create(ctx, parent))

	_, err := svc.CreateShadow(ctx, parent.ID, ShadowOptions{Name: "s", GroupIDs: []int64{7}})
	require.Error(t, err, "绑组失败应返回错误")

	shadows, qerr := base.ListShadowsByParent(ctx, parent.ID)
	require.NoError(t, qerr)
	require.Empty(t, shadows, "绑组失败后应补偿删除影子,不留孤儿")
}

// TestUpdateAccount_RejectsParentTypeChangeWithShadow 验证外审 D/P1:母账号有 spark 影子时,
// 不能把 type 改出 OpenAI OAuth(否则影子被调度后透传凭据解析必失败)。
func TestUpdateAccount_RejectsParentTypeChangeWithShadow(t *testing.T) {
	ctx := context.Background()
	repo := newSparkShadowRepoStub()
	svc := &adminServiceImpl{accountRepo: repo}
	parent := &Account{
		Name: "p", Platform: PlatformOpenAI, Type: AccountTypeOAuth,
		Status: StatusActive, Credentials: map[string]any{"chatgpt_account_id": "o"},
	}
	require.NoError(t, repo.Create(ctx, parent))
	_, err := svc.CreateShadow(ctx, parent.ID, ShadowOptions{Name: "s"})
	require.NoError(t, err)

	_, err = svc.UpdateAccount(ctx, parent.ID, &UpdateAccountInput{Type: AccountTypeAPIKey})
	require.Error(t, err)
	require.Equal(t, http.StatusBadRequest, infraerrors.Code(err), "母账号有影子时改 type 出 oauth 应 400")
	require.Equal(t, AccountTypeOAuth, repo.accounts[parent.ID].Type, "母账号 type 必须保持 oauth")

	// 对照:把 type 设为相同 oauth(no-op)应允许。
	_, err = svc.UpdateAccount(ctx, parent.ID, &UpdateAccountInput{Type: AccountTypeOAuth})
	require.NoError(t, err, "传入相同 type(no-op)应允许")
}

// TestUpdateAccount_IgnoresProxyChangeOnShadow 验证外审 B/P1:影子 proxy 恒继承母账号,
// 普通更新不得独立改动。
func TestUpdateAccount_IgnoresProxyChangeOnShadow(t *testing.T) {
	ctx := context.Background()
	repo := newSparkShadowRepoStub()
	svc := &adminServiceImpl{accountRepo: repo}
	parentProxy := int64(7)
	parent := &Account{
		Name: "p", Platform: PlatformOpenAI, Type: AccountTypeOAuth,
		Status: StatusActive, ProxyID: &parentProxy,
		Credentials: map[string]any{"chatgpt_account_id": "o"},
	}
	require.NoError(t, repo.Create(ctx, parent))
	shadow, err := svc.CreateShadow(ctx, parent.ID, ShadowOptions{Name: "s"})
	require.NoError(t, err)
	require.NotNil(t, repo.accounts[shadow.ID].ProxyID)
	require.Equal(t, parentProxy, *repo.accounts[shadow.ID].ProxyID, "前提:影子继承母 proxy=7")

	newProxy := int64(42)
	_, err = svc.UpdateAccount(ctx, shadow.ID, &UpdateAccountInput{ProxyID: &newProxy})
	require.NoError(t, err, "影子的非 proxy 字段更新仍应成功")
	require.NotNil(t, repo.accounts[shadow.ID].ProxyID)
	require.Equal(t, parentProxy, *repo.accounts[shadow.ID].ProxyID, "影子 proxy 不应被独立改动,恒继承母账号")
}

func TestUpdateAccount_ShadowAllowsModelMappingAndGroupUpdate(t *testing.T) {
	ctx := context.Background()
	repo := newSparkShadowRepoStub()
	groupRepo := &sparkShadowValidatingGroupRepoStub{existing: map[int64]bool{7: true}}
	svc := &adminServiceImpl{accountRepo: repo, groupRepo: groupRepo}
	parentID := int64(1)
	parent := &Account{
		ID:       parentID,
		Name:     "p",
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
		Status:   StatusActive,
		Credentials: map[string]any{
			"access_token":       "parent-token",
			"chatgpt_account_id": "org-parent",
		},
	}
	require.NoError(t, repo.Create(ctx, parent))
	shadow := &Account{
		Name:            "s",
		Platform:        PlatformOpenAI,
		Type:            AccountTypeOAuth,
		Status:          StatusActive,
		ParentAccountID: &parentID,
		QuotaDimension:  QuotaDimensionSpark,
		Credentials:     map[string]any{},
	}
	require.NoError(t, repo.Create(ctx, shadow))

	groupIDs := []int64{7}
	updated, err := svc.UpdateAccount(ctx, shadow.ID, &UpdateAccountInput{
		Credentials: map[string]any{
			"model_mapping": map[string]any{
				"gpt-5.3-codex-spark": "gpt-5.3-codex-spark",
			},
		},
		GroupIDs: &groupIDs,
	})

	require.NoError(t, err)
	require.Equal(t, []int64{7}, repo.groupsOf[shadow.ID])
	require.Equal(t, map[string]any{"gpt-5.3-codex-spark": "gpt-5.3-codex-spark"}, updated.Credentials["model_mapping"])
	require.Empty(t, updated.GetOpenAIAccessToken(), "影子账号不可持有母账号 access_token")
}

func TestUpdateAccount_ShadowEmptyCredentialsClearsModelMapping(t *testing.T) {
	ctx := context.Background()
	repo := newSparkShadowRepoStub()
	svc := &adminServiceImpl{accountRepo: repo}
	parentID := int64(1)
	shadow := &Account{
		Name:            "s",
		Platform:        PlatformOpenAI,
		Type:            AccountTypeOAuth,
		Status:          StatusActive,
		ParentAccountID: &parentID,
		QuotaDimension:  QuotaDimensionSpark,
		Credentials: map[string]any{
			"model_mapping": map[string]any{
				"gpt-5.3-codex-spark": "gpt-5.3-codex-spark",
			},
		},
	}
	require.NoError(t, repo.Create(ctx, shadow))

	updated, err := svc.UpdateAccount(ctx, shadow.ID, &UpdateAccountInput{
		Credentials: map[string]any{},
	})

	require.NoError(t, err)
	require.Empty(t, updated.Credentials)
	require.Empty(t, repo.accounts[shadow.ID].Credentials)
}

func TestUpdateAccount_ShadowRejectsAuthCredentials(t *testing.T) {
	ctx := context.Background()
	repo := newSparkShadowRepoStub()
	svc := &adminServiceImpl{accountRepo: repo}
	parentID := int64(1)
	shadow := &Account{
		Name:            "s",
		Platform:        PlatformOpenAI,
		Type:            AccountTypeOAuth,
		Status:          StatusActive,
		ParentAccountID: &parentID,
		QuotaDimension:  QuotaDimensionSpark,
		Credentials:     map[string]any{},
	}
	require.NoError(t, repo.Create(ctx, shadow))

	_, err := svc.UpdateAccount(ctx, shadow.ID, &UpdateAccountInput{
		Credentials: map[string]any{"access_token": "leak"},
	})

	require.Error(t, err)
	require.Equal(t, http.StatusBadRequest, infraerrors.Code(err))
	require.Empty(t, repo.accounts[shadow.ID].Credentials)
}

// TestBulkUpdateAccounts_RejectsProxyChangeOnShadow 验证外审第4轮 P1:批量更新携带 proxy 且
// 目标含影子必须被拒(与单账号 UpdateAccount 守卫对齐,堵住 bulk 绕过"proxy 恒继承母账号")。
func TestBulkUpdateAccounts_RejectsProxyChangeOnShadow(t *testing.T) {
	ctx := context.Background()
	repo := newSparkShadowRepoStub()
	svc := &adminServiceImpl{accountRepo: repo}
	parentProxy := int64(7)
	parent := &Account{
		Name: "p", Platform: PlatformOpenAI, Type: AccountTypeOAuth,
		Status: StatusActive, ProxyID: &parentProxy,
		Credentials: map[string]any{"chatgpt_account_id": "o"},
	}
	require.NoError(t, repo.Create(ctx, parent))
	shadow, err := svc.CreateShadow(ctx, parent.ID, ShadowOptions{Name: "s"})
	require.NoError(t, err)

	newProxy := int64(42)
	_, err = svc.BulkUpdateAccounts(ctx, &BulkUpdateAccountsInput{
		AccountIDs: []int64{shadow.ID},
		ProxyID:    &newProxy,
	})
	require.Error(t, err, "批量给影子改 proxy 必须被拒")
	require.Equal(t, http.StatusBadRequest, infraerrors.Code(err), "应 400")
	require.NotNil(t, repo.accounts[shadow.ID].ProxyID)
	require.Equal(t, parentProxy, *repo.accounts[shadow.ID].ProxyID, "影子 proxy 必须保持继承母账号")
}

// TestForceOpenAIPrivacy_SkipsShadow 验证外审第4轮:影子隐私设置跳过(由母账号管理),
// 早返不触碰任何依赖(svc 无 deps,若未守卫会 nil panic)。
func TestForceOpenAIPrivacy_SkipsShadow(t *testing.T) {
	svc := &adminServiceImpl{}
	pid := int64(1)
	shadow := &Account{ID: 2, Platform: PlatformOpenAI, Type: AccountTypeOAuth, ParentAccountID: &pid}
	require.Equal(t, "", svc.ForceOpenAIPrivacy(context.Background(), shadow), "影子隐私设置应跳过")
}
