//go:build unit

package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNormalizeUpstreamPlanFromProbe_OpenAI(t *testing.T) {
	require.Equal(t, "free", NormalizeUpstreamPlanFromProbe(PlatformOpenAI, "Free"))
	require.Equal(t, "plus", NormalizeUpstreamPlanFromProbe(PlatformOpenAI, " PLUS "))
	require.Equal(t, "team", NormalizeUpstreamPlanFromProbe(PlatformOpenAI, "team"))
	require.Equal(t, "pro", NormalizeUpstreamPlanFromProbe(PlatformOpenAI, "Pro"))
	require.Equal(t, "", NormalizeUpstreamPlanFromProbe(PlatformOpenAI, "self_serve_business_usage_based"))
	require.Equal(t, "", NormalizeUpstreamPlanFromProbe(PlatformOpenAI, ""))
}

func TestNormalizeUpstreamPlanFromProbe_Grok(t *testing.T) {
	require.Equal(t, "free", NormalizeUpstreamPlanFromProbe(PlatformGrok, "free"))
	require.Equal(t, "basic", NormalizeUpstreamPlanFromProbe(PlatformGrok, "Basic"))
	require.Equal(t, "supergrok", NormalizeUpstreamPlanFromProbe(PlatformGrok, "SuperGrok"))
	require.Equal(t, "supergrok", NormalizeUpstreamPlanFromProbe(PlatformGrok, "super_grok"))
	require.Equal(t, "supergrokheavy", NormalizeUpstreamPlanFromProbe(PlatformGrok, "supergrokheavy"))
	require.Equal(t, "supergrokheavy", NormalizeUpstreamPlanFromProbe(PlatformGrok, "SuperGrok Heavy"))
	require.Equal(t, "", NormalizeUpstreamPlanFromProbe(PlatformGrok, "enterprise-unknown"))
}

func TestNormalizeUpstreamPlanFromProbe_Antigravity(t *testing.T) {
	require.Equal(t, "free-tier", NormalizeUpstreamPlanFromProbe(PlatformAntigravity, "Free"))
	require.Equal(t, "free-tier", NormalizeUpstreamPlanFromProbe(PlatformAntigravity, "free"))
	require.Equal(t, "free-tier", NormalizeUpstreamPlanFromProbe(PlatformAntigravity, "free-tier"))
	require.Equal(t, "g1-pro-tier", NormalizeUpstreamPlanFromProbe(PlatformAntigravity, "Pro"))
	require.Equal(t, "g1-pro-tier", NormalizeUpstreamPlanFromProbe(PlatformAntigravity, "g1-pro-tier"))
	require.Equal(t, "g1-ultra-tier", NormalizeUpstreamPlanFromProbe(PlatformAntigravity, "Ultra"))
	require.Equal(t, "", NormalizeUpstreamPlanFromProbe(PlatformAntigravity, "Abnormal"))
	require.Equal(t, "", NormalizeUpstreamPlanFromProbe(PlatformAntigravity, "unknown-tier"))
}

func TestNormalizeUpstreamPlanFromProbe_AnthropicGeminiEmpty(t *testing.T) {
	require.Equal(t, "", NormalizeUpstreamPlanFromProbe(PlatformAnthropic, "pro"))
	require.Equal(t, "", NormalizeUpstreamPlanFromProbe(PlatformGemini, "ultra"))
}

// applyPlanRepo 记录 BulkUpdate/Update，用于 ApplyProbedPlan 单测。
type applyPlanRepo struct {
	accountRepoStub
	accounts    map[int64]*Account
	bulkCalls   []AccountBulkUpdate
	updateCalls int
}

func newApplyPlanRepo(accs ...*Account) *applyPlanRepo {
	r := &applyPlanRepo{accounts: make(map[int64]*Account)}
	for _, a := range accs {
		if a == nil {
			continue
		}
		cp := *a
		if a.Credentials != nil {
			cp.Credentials = shallowCopyMap(a.Credentials)
		}
		r.accounts[a.ID] = &cp
	}
	return r
}

func (r *applyPlanRepo) GetByID(_ context.Context, id int64) (*Account, error) {
	a := r.accounts[id]
	if a == nil {
		return nil, ErrAccountNotFound
	}
	cp := *a
	if a.Credentials != nil {
		cp.Credentials = shallowCopyMap(a.Credentials)
	}
	return &cp, nil
}

func (r *applyPlanRepo) BulkUpdate(_ context.Context, ids []int64, updates AccountBulkUpdate) (int64, error) {
	r.bulkCalls = append(r.bulkCalls, updates)
	for _, id := range ids {
		a := r.accounts[id]
		if a == nil {
			continue
		}
		if a.Credentials == nil {
			a.Credentials = make(map[string]any)
		}
		for k, v := range updates.Credentials {
			a.Credentials[k] = v
		}
		if updates.UpstreamPlan != nil {
			a.UpstreamPlan = *updates.UpstreamPlan
		}
	}
	return int64(len(ids)), nil
}

func (r *applyPlanRepo) Update(_ context.Context, account *Account) error {
	r.updateCalls++
	if account == nil {
		return nil
	}
	cp := *account
	if account.Credentials != nil {
		cp.Credentials = shallowCopyMap(account.Credentials)
	}
	r.accounts[account.ID] = &cp
	return nil
}

func (r *applyPlanRepo) AddGroups(context.Context, int64, []int64) error    { return nil }
func (r *applyPlanRepo) RemoveGroups(context.Context, int64, []int64) error { return nil }

// recomputeSpy 记录 RecomputeManagedLinks 调用。
type recomputeSpy struct {
	calls []*Account
	err   error
}

func (s *recomputeSpy) RecomputeManagedLinks(_ context.Context, account *Account) error {
	s.calls = append(s.calls, account)
	return s.err
}

func TestApplyProbedPlan_UpdatesColumnAndCredentials(t *testing.T) {
	acc := &Account{
		ID:       1,
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
		Credentials: map[string]any{
			"access_token": "t",
			"plan_type":    "plus",
		},
		UpstreamPlan: "plus",
	}
	repo := newApplyPlanRepo(acc)

	err := ApplyProbedPlan(context.Background(), repo, nil, acc, "pro")
	require.NoError(t, err)
	require.Equal(t, "pro", acc.UpstreamPlan)
	require.Equal(t, "pro", acc.Credentials["plan_type"])
	require.Len(t, repo.bulkCalls, 1)
	require.Equal(t, "pro", repo.bulkCalls[0].Credentials["plan_type"])
	require.NotNil(t, repo.bulkCalls[0].UpstreamPlan)
	require.Equal(t, "pro", *repo.bulkCalls[0].UpstreamPlan)
}

func TestApplyProbedPlan_OwnedAccountCallsRecompute(t *testing.T) {
	owner := int64(42)
	acc := &Account{
		ID:          2,
		Platform:    PlatformOpenAI,
		OwnerUserID: &owner,
		Visibility:  VisibilityPublic,
		Credentials: map[string]any{"plan_type": "plus"},
		UpstreamPlan: "plus",
	}
	repo := newApplyPlanRepo(acc)
	spy := &recomputeSpy{}

	err := ApplyProbedPlan(context.Background(), repo, spy, acc, "team")
	require.NoError(t, err)
	require.Equal(t, "team", acc.UpstreamPlan)
	require.Len(t, spy.calls, 1)
	require.Equal(t, int64(2), spy.calls[0].ID)
}

func TestApplyProbedPlan_SystemAccountNoRecompute(t *testing.T) {
	acc := &Account{
		ID:           3,
		Platform:     PlatformOpenAI,
		OwnerUserID:  nil, // 系统号
		Credentials:  map[string]any{},
		UpstreamPlan: "",
	}
	repo := newApplyPlanRepo(acc)
	spy := &recomputeSpy{}

	err := ApplyProbedPlan(context.Background(), repo, spy, acc, "plus")
	require.NoError(t, err)
	require.Equal(t, "plus", acc.UpstreamPlan)
	require.Empty(t, spy.calls, "系统号不得触发 recompute")
}

func TestApplyProbedPlan_NoopWhenUnchanged(t *testing.T) {
	acc := &Account{
		ID:           4,
		Platform:     PlatformOpenAI,
		Credentials:  map[string]any{"plan_type": "pro"},
		UpstreamPlan: "pro",
	}
	repo := newApplyPlanRepo(acc)
	spy := &recomputeSpy{}

	err := ApplyProbedPlan(context.Background(), repo, spy, acc, "Pro")
	require.NoError(t, err)
	require.Empty(t, repo.bulkCalls)
	require.Zero(t, repo.updateCalls)
	require.Empty(t, spy.calls)
}

func TestApplyProbedPlan_UnknownRawClearsColumnButAuditsRaw(t *testing.T) {
	acc := &Account{
		ID:           5,
		Platform:     PlatformOpenAI,
		Credentials:  map[string]any{"plan_type": "plus"},
		UpstreamPlan: "plus",
	}
	repo := newApplyPlanRepo(acc)

	err := ApplyProbedPlan(context.Background(), repo, nil, acc, "self_serve_business")
	require.NoError(t, err)
	require.Equal(t, "", acc.UpstreamPlan)
	require.Equal(t, "self_serve_business", acc.Credentials["plan_type"])
	require.Len(t, repo.bulkCalls, 1)
	require.NotNil(t, repo.bulkCalls[0].UpstreamPlan)
	require.Equal(t, "", *repo.bulkCalls[0].UpstreamPlan)
}

func TestApplyProbedPlan_GrokUsesSubscriptionTier(t *testing.T) {
	acc := &Account{
		ID:       6,
		Platform: PlatformGrok,
		Credentials: map[string]any{
			"subscription_tier": "free",
		},
	}
	repo := newApplyPlanRepo(acc)

	err := ApplyProbedPlan(context.Background(), repo, nil, acc, "SuperGrok")
	require.NoError(t, err)
	require.Equal(t, "supergrok", acc.UpstreamPlan)
	require.Equal(t, "SuperGrok", acc.Credentials["subscription_tier"])
}

func TestApplyProbedPlan_AntigravityMapsTiers(t *testing.T) {
	acc := &Account{
		ID:          7,
		Platform:    PlatformAntigravity,
		Credentials: map[string]any{},
	}
	repo := newApplyPlanRepo(acc)

	err := ApplyProbedPlan(context.Background(), repo, nil, acc, "Pro")
	require.NoError(t, err)
	require.Equal(t, "g1-pro-tier", acc.UpstreamPlan)
	require.Equal(t, "Pro", acc.Credentials["plan_type"])
}

func TestApplyProbedPlan_SkipsShadow(t *testing.T) {
	parent := int64(1)
	acc := &Account{
		ID:              8,
		Platform:        PlatformOpenAI,
		ParentAccountID: &parent,
		Credentials:     map[string]any{},
	}
	repo := newApplyPlanRepo(acc)

	err := ApplyProbedPlan(context.Background(), repo, nil, acc, "pro")
	require.NoError(t, err)
	require.Empty(t, acc.Credentials)
	require.Empty(t, repo.bulkCalls)
}

func TestPersistOpenAI429PlanType_ViaApplyProbedPlan(t *testing.T) {
	body := []byte(`{"error":{"type":"usage_limit_reached","plan_type":"plus"}}`)
	owner := int64(9)
	acc := &Account{
		ID:          10,
		Platform:    PlatformOpenAI,
		Type:        AccountTypeOAuth,
		OwnerUserID: &owner,
		Credentials: map[string]any{"plan_type": "free"},
		UpstreamPlan: "free",
	}
	repo := newApplyPlanRepo(acc)
	spy := &recomputeSpy{}

	persistOpenAI429PlanType(context.Background(), repo, spy, acc, body)
	require.Equal(t, "plus", acc.UpstreamPlan)
	require.Equal(t, "plus", acc.Credentials["plan_type"])
	require.Len(t, spy.calls, 1)
}

func TestExtractProbedPlanRaw(t *testing.T) {
	acc := &Account{
		Platform:    PlatformOpenAI,
		Credentials: map[string]any{"chatgpt_plan_type": "team"},
	}
	require.Equal(t, "team", ExtractProbedPlanRaw(acc))

	acc2 := &Account{
		Platform:    PlatformGrok,
		Credentials: map[string]any{"subscription_tier": "basic"},
	}
	require.Equal(t, "basic", ExtractProbedPlanRaw(acc2))
}
