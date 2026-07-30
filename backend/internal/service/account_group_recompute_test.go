//go:build unit

package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

// recomputeAccountRepo 记录 Add/Remove 调用，支持内存态绑定与列表扫描。
type recomputeAccountRepo struct {
	accountRepoStub
	accounts       map[int64]*Account
	bindings       map[int64][]int64 // accountID -> groupIDs
	removeCalls    []removeCall
	addCalls       []addCall
	recomputeStub  bool // 若 true，Recompute 路径不改 bindings（由外层 spy）
	skipGetRefresh bool
}

type removeCall struct {
	accountID int64
	groupIDs  []int64
}

type addCall struct {
	accountID int64
	groupIDs  []int64
}

func newRecomputeAccountRepo(accs ...*Account) *recomputeAccountRepo {
	r := &recomputeAccountRepo{
		accounts: make(map[int64]*Account),
		bindings: make(map[int64][]int64),
	}
	for _, a := range accs {
		if a == nil {
			continue
		}
		cp := *a
		cp.GroupIDs = append([]int64(nil), a.GroupIDs...)
		r.accounts[a.ID] = &cp
		r.bindings[a.ID] = append([]int64(nil), a.GroupIDs...)
	}
	return r
}

func (r *recomputeAccountRepo) GetByID(_ context.Context, id int64) (*Account, error) {
	a := r.accounts[id]
	if a == nil {
		return nil, ErrAccountNotFound
	}
	cp := *a
	cp.GroupIDs = append([]int64(nil), r.bindings[id]...)
	return &cp, nil
}

func (r *recomputeAccountRepo) AddGroups(_ context.Context, accountID int64, groupIDs []int64) error {
	r.addCalls = append(r.addCalls, addCall{accountID: accountID, groupIDs: append([]int64(nil), groupIDs...)})
	set := make(map[int64]struct{})
	for _, id := range r.bindings[accountID] {
		set[id] = struct{}{}
	}
	for _, gid := range groupIDs {
		if _, ok := set[gid]; ok {
			continue
		}
		r.bindings[accountID] = append(r.bindings[accountID], gid)
		set[gid] = struct{}{}
	}
	if a := r.accounts[accountID]; a != nil {
		a.GroupIDs = append([]int64(nil), r.bindings[accountID]...)
	}
	return nil
}

func (r *recomputeAccountRepo) RemoveGroups(_ context.Context, accountID int64, groupIDs []int64) error {
	r.removeCalls = append(r.removeCalls, removeCall{accountID: accountID, groupIDs: append([]int64(nil), groupIDs...)})
	rm := make(map[int64]struct{}, len(groupIDs))
	for _, gid := range groupIDs {
		rm[gid] = struct{}{}
	}
	cur := r.bindings[accountID]
	next := make([]int64, 0, len(cur))
	for _, id := range cur {
		if _, ok := rm[id]; !ok {
			next = append(next, id)
		}
	}
	r.bindings[accountID] = next
	if a := r.accounts[accountID]; a != nil {
		a.GroupIDs = append([]int64(nil), next...)
	}
	return nil
}

func (r *recomputeAccountRepo) ListOwnerAccountsBoundToGroup(_ context.Context, groupID int64) ([]*Account, error) {
	out := make([]*Account, 0)
	for id, gids := range r.bindings {
		for _, g := range gids {
			if g != groupID {
				continue
			}
			a := r.accounts[id]
			if a == nil || a.OwnerUserID == nil {
				continue
			}
			cp := *a
			cp.GroupIDs = append([]int64(nil), gids...)
			out = append(out, &cp)
			break
		}
	}
	return out, nil
}

func (r *recomputeAccountRepo) ListPublicOwnerAccountsByPlatformPlan(_ context.Context, platform, plan string) ([]*Account, error) {
	out := make([]*Account, 0)
	for _, a := range r.accounts {
		if a.OwnerUserID == nil {
			continue
		}
		if a.Visibility != VisibilityPublic {
			continue
		}
		if a.Platform != platform || a.UpstreamPlan != plan {
			continue
		}
		cp := *a
		cp.GroupIDs = append([]int64(nil), r.bindings[a.ID]...)
		out = append(out, &cp)
	}
	return out, nil
}

func (r *recomputeAccountRepo) CountActiveOwned(_ context.Context, ownerUserID int64) (int, error) {
	n := 0
	for _, a := range r.accounts {
		if a.OwnerUserID != nil && *a.OwnerUserID == ownerUserID {
			n++
		}
	}
	return n, nil
}

// recomputeGroupRepo 内存组仓。
type recomputeGroupRepo struct {
	groupRepoNoop
	byID   map[int64]*Group
	byName map[string]*Group
	// ensureCalls 若被误调用 Ensure 路径不会走 groupRepo.Create；这里用 GetByName 计数。
	getByNameCalls []string
}

func newRecomputeGroupRepo(groups ...*Group) *recomputeGroupRepo {
	r := &recomputeGroupRepo{
		byID:   make(map[int64]*Group),
		byName: make(map[string]*Group),
	}
	for _, g := range groups {
		if g == nil {
			continue
		}
		cp := *g
		r.byID[g.ID] = &cp
		r.byName[g.Name] = &cp
	}
	return r
}

func (r *recomputeGroupRepo) GetByName(_ context.Context, name string) (*Group, error) {
	r.getByNameCalls = append(r.getByNameCalls, name)
	g := r.byName[name]
	if g == nil {
		return nil, ErrGroupNotFound
	}
	cp := *g
	return &cp, nil
}

func (r *recomputeGroupRepo) GetByID(_ context.Context, id int64) (*Group, error) {
	g := r.byID[id]
	if g == nil {
		return nil, ErrGroupNotFound
	}
	cp := *g
	return &cp, nil
}

func (r *recomputeGroupRepo) ListByIDs(_ context.Context, ids []int64) ([]Group, error) {
	out := make([]Group, 0, len(ids))
	for _, id := range ids {
		if g := r.byID[id]; g != nil {
			out = append(out, *g)
		}
	}
	return out, nil
}

func (r *recomputeGroupRepo) ListSharePoolMatches(_ context.Context, platform, plan string) ([]Group, error) {
	out := make([]Group, 0)
	for _, g := range r.byID {
		if !g.IsSharePool || g.IsExclusive || g.Status != StatusActive {
			continue
		}
		if g.Platform != platform || g.UpstreamPlan != plan {
			continue
		}
		if IsPrivateGroupName(g.Name) {
			continue
		}
		out = append(out, *g)
	}
	return out, nil
}

func (r *recomputeGroupRepo) Create(context.Context, *Group) error {
	panic("unexpected Create call — recompute must not Ensure private groups")
}

func ownerID(id int64) *int64 { return &id }

func TestOnSharePoolGroupChange_TrueToFalse_CallsRemoveGroups(t *testing.T) {
	owner := int64(7)
	private := &Group{
		ID: 100, Name: PrivateGroupName(owner, PlatformOpenAI), Platform: PlatformOpenAI,
		IsExclusive: true, Status: StatusActive,
	}
	pool := &Group{
		ID: 200, Name: "openai-plus-pool", Platform: PlatformOpenAI,
		IsSharePool: true, UpstreamPlan: "plus", Status: StatusActive,
	}
	adminExtra := &Group{
		ID: 300, Name: "ops-manual", Platform: PlatformOpenAI, Status: StatusActive,
	}
	acc := &Account{
		ID: 1, OwnerUserID: &owner, Platform: PlatformOpenAI,
		Visibility: VisibilityPublic, UpstreamPlan: "plus",
		GroupIDs: []int64{100, 200, 300},
	}
	accountRepo := newRecomputeAccountRepo(acc)
	groupRepo := newRecomputeGroupRepo(private, pool, adminExtra)
	rec := NewAccountGroupRecomputer(accountRepo, groupRepo)

	before := cloneGroupShallow(pool)
	after := cloneGroupShallow(pool)
	after.IsSharePool = false
	// 模拟 DB 已写入 after 态（ListSharePoolMatches / currentManaged 读仓）
	groupRepo.byID[200] = after
	groupRepo.byName[after.Name] = after

	err := rec.OnSharePoolGroupChange(context.Background(), before, after)
	require.NoError(t, err)

	// MUST：显式 RemoveGroups 含 pool 200
	require.NotEmpty(t, accountRepo.removeCalls)
	foundForced := false
	for _, c := range accountRepo.removeCalls {
		if c.accountID == 1 {
			for _, gid := range c.groupIDs {
				if gid == 200 {
					foundForced = true
				}
			}
		}
	}
	require.True(t, foundForced, "RemoveGroups must be called for share pool group even when recompute predicate would miss it")

	// admin 非池绑定 300 必须保留
	require.Contains(t, accountRepo.bindings[1], int64(300))
	// 池绑定已卸
	require.NotContains(t, accountRepo.bindings[1], int64(200))
	// 私有组保留/补回
	require.Contains(t, accountRepo.bindings[1], int64(100))
}

func TestOnSharePoolGroupChange_FalseToTrue_AbsorbsMatchingPublic(t *testing.T) {
	owner := int64(9)
	private := &Group{
		ID: 101, Name: PrivateGroupName(owner, PlatformOpenAI), Platform: PlatformOpenAI,
		IsExclusive: true, Status: StatusActive,
	}
	pool := &Group{
		ID: 201, Name: "openai-plus-pool", Platform: PlatformOpenAI,
		IsSharePool: false, UpstreamPlan: "plus", Status: StatusActive,
	}
	acc := &Account{
		ID: 2, OwnerUserID: &owner, Platform: PlatformOpenAI,
		Visibility: VisibilityPublic, UpstreamPlan: "plus",
		GroupIDs: []int64{101},
	}
	accountRepo := newRecomputeAccountRepo(acc)
	groupRepo := newRecomputeGroupRepo(private, pool)
	// 开池后 group 状态
	after := cloneGroupShallow(pool)
	after.IsSharePool = true
	groupRepo.byID[201] = after
	groupRepo.byName[after.Name] = after

	rec := NewAccountGroupRecomputer(accountRepo, groupRepo)
	before := cloneGroupShallow(pool)
	err := rec.OnSharePoolGroupChange(context.Background(), before, after)
	require.NoError(t, err)

	require.Contains(t, accountRepo.bindings[2], int64(201), "absorb must attach matching public account")
	require.Contains(t, accountRepo.bindings[2], int64(101))
}

func TestOnSharePoolGroupChange_PlanChange_UnbindsOldBindsNew(t *testing.T) {
	ownerPlus := int64(1)
	ownerTeam := int64(2)
	privPlus := &Group{ID: 111, Name: PrivateGroupName(ownerPlus, PlatformOpenAI), Platform: PlatformOpenAI, IsExclusive: true, Status: StatusActive}
	privTeam := &Group{ID: 112, Name: PrivateGroupName(ownerTeam, PlatformOpenAI), Platform: PlatformOpenAI, IsExclusive: true, Status: StatusActive}
	pool := &Group{
		ID: 220, Name: "pool", Platform: PlatformOpenAI,
		IsSharePool: true, UpstreamPlan: "plus", Status: StatusActive,
	}
	accPlus := &Account{
		ID: 10, OwnerUserID: &ownerPlus, Platform: PlatformOpenAI,
		Visibility: VisibilityPublic, UpstreamPlan: "plus", GroupIDs: []int64{111, 220},
	}
	accTeam := &Account{
		ID: 11, OwnerUserID: &ownerTeam, Platform: PlatformOpenAI,
		Visibility: VisibilityPublic, UpstreamPlan: "team", GroupIDs: []int64{112},
	}
	accountRepo := newRecomputeAccountRepo(accPlus, accTeam)
	groupRepo := newRecomputeGroupRepo(privPlus, privTeam, pool)

	before := cloneGroupShallow(pool)
	after := cloneGroupShallow(pool)
	after.UpstreamPlan = "team"
	groupRepo.byID[220] = after
	groupRepo.byName[after.Name] = after

	rec := NewAccountGroupRecomputer(accountRepo, groupRepo)
	err := rec.OnSharePoolGroupChange(context.Background(), before, after)
	require.NoError(t, err)

	require.NotContains(t, accountRepo.bindings[10], int64(220), "plus account unbound after plan change")
	require.Contains(t, accountRepo.bindings[11], int64(220), "team account absorbed after plan change")
}

func TestRecomputeManagedLinks_DoesNotEnsureWhenPrivateMissing(t *testing.T) {
	owner := int64(3)
	pool := &Group{
		ID: 230, Name: "pool", Platform: PlatformOpenAI,
		IsSharePool: true, UpstreamPlan: "plus", Status: StatusActive,
	}
	acc := &Account{
		ID: 20, OwnerUserID: &owner, Platform: PlatformOpenAI,
		Visibility: VisibilityPublic, UpstreamPlan: "plus",
		GroupIDs: []int64{230},
	}
	accountRepo := newRecomputeAccountRepo(acc)
	// 无私有组
	groupRepo := newRecomputeGroupRepo(pool)
	rec := NewAccountGroupRecomputer(accountRepo, groupRepo)

	err := rec.RecomputeManagedLinks(context.Background(), acc)
	require.NoError(t, err)

	// GetByName 被调用且未 Create（Create panics）
	require.Contains(t, groupRepo.getByNameCalls, PrivateGroupName(owner, PlatformOpenAI))
	// public 仍可绑池；无私有 ID
	require.Contains(t, accountRepo.bindings[20], int64(230))
}

func TestRecomputeManagedLinks_TogglePrivateRemovesSharePoolLinks(t *testing.T) {
	owner := int64(5)
	private := &Group{
		ID: 140, Name: PrivateGroupName(owner, PlatformGrok), Platform: PlatformGrok,
		IsExclusive: true, Status: StatusActive,
	}
	pool := &Group{
		ID: 240, Name: "grok-pool", Platform: PlatformGrok,
		IsSharePool: true, UpstreamPlan: "super", Status: StatusActive,
	}
	adminExtra := &Group{ID: 340, Name: "ops", Platform: PlatformGrok, Status: StatusActive}
	acc := &Account{
		ID: 30, OwnerUserID: &owner, Platform: PlatformGrok,
		Visibility: VisibilityPublic, UpstreamPlan: "super",
		GroupIDs: []int64{140, 240, 340},
	}
	accountRepo := newRecomputeAccountRepo(acc)
	groupRepo := newRecomputeGroupRepo(private, pool, adminExtra)
	rec := NewAccountGroupRecomputer(accountRepo, groupRepo)

	// toggle → private
	acc.Visibility = VisibilityPrivate
	accountRepo.accounts[30].Visibility = VisibilityPrivate

	err := rec.RecomputeManagedLinks(context.Background(), acc)
	require.NoError(t, err)

	require.Contains(t, accountRepo.bindings[30], int64(140), "private group kept")
	require.NotContains(t, accountRepo.bindings[30], int64(240), "share pool removed")
	require.Contains(t, accountRepo.bindings[30], int64(340), "admin non-pool kept")
}

func TestOnSharePoolGroupChange_ForcedUnlinkEvenIfRecomputeIsNoop(t *testing.T) {
	// 关池时即使 bound 列表返回账号，RemoveGroups 必须被调用（门禁：不可只靠 recompute）。
	owner := int64(8)
	pool := &Group{
		ID: 250, Name: "pool", Platform: PlatformOpenAI,
		IsSharePool: true, UpstreamPlan: "plus", Status: StatusActive,
	}
	acc := &Account{
		ID: 40, OwnerUserID: &owner, Platform: PlatformOpenAI,
		Visibility: VisibilityPublic, UpstreamPlan: "plus", GroupIDs: []int64{250},
	}
	accountRepo := newRecomputeAccountRepo(acc)
	// 无私有组 + 关池后 ListSharePoolMatches 为空 → recompute 因 currentManaged 不再认该组，toRemove 为空
	// （模拟「仅 recompute 不够」）；但 Step A 仍应 RemoveGroups。
	groupRepo := newRecomputeGroupRepo(pool)
	rec := NewAccountGroupRecomputer(accountRepo, groupRepo)

	before := cloneGroupShallow(pool)
	after := cloneGroupShallow(pool)
	after.IsSharePool = false
	groupRepo.byID[250] = after
	groupRepo.byName[after.Name] = after

	// 先手动模拟 recompute 谓词失效：is_share_pool=false 后 currentManaged 不含 250
	// Step A 仍必须卸链
	err := rec.OnSharePoolGroupChange(context.Background(), before, after)
	require.NoError(t, err)
	require.NotEmpty(t, accountRepo.removeCalls, "RemoveGroups must run on true→false path")
	require.NotContains(t, accountRepo.bindings[40], int64(250))
}

func TestIsSharePoolCandidate(t *testing.T) {
	require.False(t, isSharePoolCandidate(nil))
	require.False(t, isSharePoolCandidate(&Group{IsSharePool: true, Status: StatusActive})) // plan empty
	require.False(t, isSharePoolCandidate(&Group{IsSharePool: true, UpstreamPlan: "plus", Status: StatusDisabled}))
	require.False(t, isSharePoolCandidate(&Group{IsSharePool: true, UpstreamPlan: "plus", Status: StatusActive, IsExclusive: true}))
	require.False(t, isSharePoolCandidate(&Group{
		IsSharePool: true, UpstreamPlan: "plus", Status: StatusActive,
		Name: PrivateGroupName(1, PlatformOpenAI),
	}))
	require.True(t, isSharePoolCandidate(&Group{
		IsSharePool: true, UpstreamPlan: "plus", Status: StatusActive, Name: "pool",
	}))
}
