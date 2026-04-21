//go:build unit

package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---- stubs ----

type stubQuotaSettings struct {
	enabled        bool
	defaultEnabled bool
	getBoolErr     error
}

func (s *stubQuotaSettings) GetBool(ctx context.Context, key string) (bool, error) {
	if s.getBoolErr != nil {
		return false, s.getBoolErr
	}
	switch key {
	case SettingKeyUsageLimitEnabled:
		return s.enabled, nil
	case SettingKeyDefaultUsageLimitEnabled:
		return s.defaultEnabled, nil
	}
	return false, nil
}

type stubQuotaUserRepo struct {
	user *User
	err  error
}

func (s *stubQuotaUserRepo) GetByID(context.Context, int64) (*User, error) {
	if s.err != nil {
		return nil, s.err
	}
	if s.user == nil {
		return &User{}, nil
	}
	clone := *s.user
	return &clone, nil
}

// 以下为 UserRepository 其它方法的占位实现（测试中不使用）
func (s *stubQuotaUserRepo) Create(context.Context, *User) error               { return nil }
func (s *stubQuotaUserRepo) GetByEmail(context.Context, string) (*User, error) { return &User{}, nil }
func (s *stubQuotaUserRepo) GetFirstAdmin(context.Context) (*User, error)      { return &User{}, nil }
func (s *stubQuotaUserRepo) Update(context.Context, *User) error               { return nil }
func (s *stubQuotaUserRepo) Delete(context.Context, int64) error               { return nil }
func (s *stubQuotaUserRepo) List(context.Context, pagination.PaginationParams) ([]User, *pagination.PaginationResult, error) {
	return nil, nil, nil
}
func (s *stubQuotaUserRepo) ListWithFilters(context.Context, pagination.PaginationParams, UserListFilters) ([]User, *pagination.PaginationResult, error) {
	return nil, nil, nil
}
func (s *stubQuotaUserRepo) UpdateBalance(context.Context, int64, float64) error { return nil }
func (s *stubQuotaUserRepo) DeductBalance(context.Context, int64, float64) error { return nil }
func (s *stubQuotaUserRepo) UpdateConcurrency(context.Context, int64, int) error { return nil }
func (s *stubQuotaUserRepo) ExistsByEmail(context.Context, string) (bool, error) { return false, nil }
func (s *stubQuotaUserRepo) RemoveGroupFromAllowedGroups(context.Context, int64) (int64, error) {
	return 0, nil
}
func (s *stubQuotaUserRepo) AddGroupToAllowedGroups(context.Context, int64, int64) error {
	return nil
}
func (s *stubQuotaUserRepo) RemoveGroupFromUserAllowedGroups(context.Context, int64, int64) error {
	return nil
}
func (s *stubQuotaUserRepo) UpdateTotpSecret(context.Context, int64, *string) error { return nil }
func (s *stubQuotaUserRepo) EnableTotp(context.Context, int64) error                { return nil }
func (s *stubQuotaUserRepo) DisableTotp(context.Context, int64) error               { return nil }
func (s *stubQuotaUserRepo) UpdateUsageLimit(context.Context, int64, *bool, *float64) error {
	return nil
}

type stubQuotaRuleRepo struct {
	rules      []*QuotaRule
	listErr    error
	created    *QuotaRule
	createErr  error
	replaceErr error
	// replaceAllCalls 记录 ReplaceAll 的入参，方便测试断言
	replaceAllCalls [][]CreateRuleRequest
}

func (s *stubQuotaRuleRepo) ListByUser(context.Context, int64) ([]*QuotaRule, error) {
	if s.listErr != nil {
		return nil, s.listErr
	}
	return s.rules, nil
}
func (s *stubQuotaRuleRepo) GetByIDForUser(_ context.Context, userID, ruleID int64) (*QuotaRule, error) {
	for _, r := range s.rules {
		if r.ID == ruleID && r.UserID == userID {
			return r, nil
		}
	}
	return nil, ErrQuotaRuleNotFound
}
func (s *stubQuotaRuleRepo) Create(_ context.Context, userID int64, req CreateRuleRequest) (*QuotaRule, error) {
	if s.createErr != nil {
		return nil, s.createErr
	}
	r := &QuotaRule{
		ID:            int64(len(s.rules) + 1),
		UserID:        userID,
		GroupIDs:      req.GroupIDs,
		DailyLimitUSD: req.DailyLimitUSD,
		Period:        QuotaPeriodDaily,
	}
	s.rules = append(s.rules, r)
	s.created = r
	return r, nil
}
func (s *stubQuotaRuleRepo) Update(context.Context, int64, int64, UpdateRuleRequest) (*QuotaRule, error) {
	return nil, nil
}
func (s *stubQuotaRuleRepo) Delete(context.Context, int64, int64) error { return nil }

func (s *stubQuotaRuleRepo) ReplaceAll(_ context.Context, userID int64, reqs []CreateRuleRequest) ([]*QuotaRule, error) {
	// 拷贝入参，防止后续被调用方改写影响断言
	snapshot := make([]CreateRuleRequest, len(reqs))
	copy(snapshot, reqs)
	s.replaceAllCalls = append(s.replaceAllCalls, snapshot)
	if s.replaceErr != nil {
		return nil, s.replaceErr
	}
	// 模拟事务语义：整体替换
	out := make([]*QuotaRule, 0, len(reqs))
	s.rules = s.rules[:0]
	for i, req := range reqs {
		period := req.Period
		if period == "" {
			period = QuotaPeriodDaily
		}
		r := &QuotaRule{
			ID:            int64(i + 1),
			UserID:        userID,
			GroupIDs:      req.GroupIDs,
			DailyLimitUSD: req.DailyLimitUSD,
			Period:        period,
		}
		s.rules = append(s.rules, r)
		out = append(out, r)
	}
	return out, nil
}

type stubGroupRepo struct {
	groups map[int64]*Group
}

func (s *stubGroupRepo) GetByIDLite(_ context.Context, id int64) (*Group, error) {
	g, ok := s.groups[id]
	if !ok {
		return nil, ErrGroupNotFound
	}
	return g, nil
}

// 以下为 GroupRepository 其他方法的占位实现
func (s *stubGroupRepo) Create(context.Context, *Group) error { return nil }
func (s *stubGroupRepo) GetByID(_ context.Context, id int64) (*Group, error) {
	return s.GetByIDLite(nil, id)
}
func (s *stubGroupRepo) Update(context.Context, *Group) error                  { return nil }
func (s *stubGroupRepo) Delete(context.Context, int64) error                   { return nil }
func (s *stubGroupRepo) DeleteCascade(context.Context, int64) ([]int64, error) { return nil, nil }
func (s *stubGroupRepo) List(context.Context, pagination.PaginationParams) ([]Group, *pagination.PaginationResult, error) {
	return nil, nil, nil
}
func (s *stubGroupRepo) ListWithFilters(context.Context, pagination.PaginationParams, string, string, string, *bool) ([]Group, *pagination.PaginationResult, error) {
	return nil, nil, nil
}
func (s *stubGroupRepo) ListActive(context.Context) ([]Group, error) { return nil, nil }
func (s *stubGroupRepo) ListActiveByPlatform(context.Context, string) ([]Group, error) {
	return nil, nil
}
func (s *stubGroupRepo) ExistsByName(context.Context, string) (bool, error) { return false, nil }
func (s *stubGroupRepo) GetAccountCount(context.Context, int64) (int64, int64, error) {
	return 0, 0, nil
}
func (s *stubGroupRepo) DeleteAccountGroupsByGroupID(context.Context, int64) (int64, error) {
	return 0, nil
}
func (s *stubGroupRepo) GetAccountIDsByGroupIDs(context.Context, []int64) ([]int64, error) {
	return nil, nil
}
func (s *stubGroupRepo) BindAccountsToGroup(context.Context, int64, []int64) error { return nil }
func (s *stubGroupRepo) UpdateSortOrders(context.Context, []GroupSortOrderUpdate) error {
	return nil
}

type stubQuotaUserWriter struct {
	called      bool
	lastEnabled *bool
	lastLimit   *float64
	err         error
}

func (s *stubQuotaUserWriter) UpdateUsageLimit(_ context.Context, _ int64, enabled *bool, limit *float64) error {
	s.called = true
	s.lastEnabled = enabled
	s.lastLimit = limit
	return s.err
}

// ---- 测试用例 ----

func newTestQuotaService(t *testing.T, settings *stubQuotaSettings, user *User, rules []*QuotaRule) *quotaService {
	t.Helper()
	return &quotaService{
		ruleRepo:   &stubQuotaRuleRepo{rules: rules},
		userRepo:   &stubQuotaUserRepo{user: user},
		userWriter: &stubQuotaUserWriter{},
		groupRepo:  &stubGroupRepo{groups: map[int64]*Group{}},
		settings:   settings,
		cache:      nil,
	}
}

func TestResolve_GlobalDisabledReturnsEnabledFalse(t *testing.T) {
	settings := &stubQuotaSettings{enabled: false}
	svc := newTestQuotaService(t, settings, &User{ID: 1}, nil)
	r, err := svc.Resolve(context.Background(), 1)
	require.NoError(t, err)
	assert.False(t, r.Enabled)
}

func TestResolve_UserOverrideTrue(t *testing.T) {
	settings := &stubQuotaSettings{enabled: true, defaultEnabled: false}
	truePtr := true
	u := &User{ID: 1, UsageLimitEnabled: &truePtr}
	svc := newTestQuotaService(t, settings, u, nil)
	r, err := svc.Resolve(context.Background(), 1)
	require.NoError(t, err)
	assert.True(t, r.Enabled)
}

func TestResolve_UserOverrideFalse(t *testing.T) {
	settings := &stubQuotaSettings{enabled: true, defaultEnabled: true}
	falsePtr := false
	u := &User{ID: 1, UsageLimitEnabled: &falsePtr}
	svc := newTestQuotaService(t, settings, u, nil)
	r, err := svc.Resolve(context.Background(), 1)
	require.NoError(t, err)
	assert.False(t, r.Enabled)
}

func TestResolve_UserOverrideNilFollowsDefault(t *testing.T) {
	// nil override + defaultEnabled=true → enabled=true
	settings := &stubQuotaSettings{enabled: true, defaultEnabled: true}
	limit := 10.0
	u := &User{ID: 1, UsageLimitEnabled: nil, DailyUsageLimitUSD: &limit}
	svc := newTestQuotaService(t, settings, u, nil)
	r, err := svc.Resolve(context.Background(), 1)
	require.NoError(t, err)
	assert.True(t, r.Enabled)
	require.NotNil(t, r.DailyLimit)
	assert.Equal(t, 10.0, *r.DailyLimit)
}

func TestResolve_UserNotFoundReturnsDisabled(t *testing.T) {
	settings := &stubQuotaSettings{enabled: true}
	svc := &quotaService{
		ruleRepo:   &stubQuotaRuleRepo{},
		userRepo:   &stubQuotaUserRepo{err: ErrUserNotFound},
		userWriter: &stubQuotaUserWriter{},
		groupRepo:  &stubGroupRepo{},
		settings:   settings,
	}
	r, err := svc.Resolve(context.Background(), 99)
	require.NoError(t, err)
	assert.False(t, r.Enabled)
}

func TestMatchRule_HitAndMiss(t *testing.T) {
	resolved := &ResolvedQuota{
		Enabled: true,
		Rules: []QuotaRule{
			{ID: 10, GroupIDs: []int64{1, 2}, DailyLimitUSD: 5},
			{ID: 11, GroupIDs: []int64{3}, DailyLimitUSD: 3},
		},
	}
	svc := &quotaService{}
	assert.Equal(t, int64(10), svc.MatchRule(resolved, 1).ID)
	assert.Equal(t, int64(10), svc.MatchRule(resolved, 2).ID)
	assert.Equal(t, int64(11), svc.MatchRule(resolved, 3).ID)
	assert.Nil(t, svc.MatchRule(resolved, 999))
}

func TestMatchRule_NotEnabled(t *testing.T) {
	resolved := &ResolvedQuota{Enabled: false, Rules: []QuotaRule{{ID: 1, GroupIDs: []int64{1}}}}
	svc := &quotaService{}
	assert.Nil(t, svc.MatchRule(resolved, 1))
}

func TestCreateRule_RejectsOverlap(t *testing.T) {
	settings := &stubQuotaSettings{enabled: true, defaultEnabled: true}
	existing := []*QuotaRule{{ID: 1, UserID: 7, GroupIDs: []int64{10, 20}, DailyLimitUSD: 5, Period: QuotaPeriodDaily}}
	svc := &quotaService{
		ruleRepo:   &stubQuotaRuleRepo{rules: existing},
		userRepo:   &stubQuotaUserRepo{user: &User{ID: 7}},
		userWriter: &stubQuotaUserWriter{},
		groupRepo:  &stubGroupRepo{groups: map[int64]*Group{10: {ID: 10}, 20: {ID: 20}, 30: {ID: 30}}},
		settings:   settings,
	}
	_, err := svc.CreateRule(context.Background(), 7, CreateRuleRequest{
		GroupIDs:      []int64{20, 30},
		DailyLimitUSD: 1,
	})
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrRuleGroupsOverlap))
}

func TestCreateRule_RejectsSubscriptionGroup(t *testing.T) {
	settings := &stubQuotaSettings{enabled: true, defaultEnabled: true}
	svc := &quotaService{
		ruleRepo:   &stubQuotaRuleRepo{},
		userRepo:   &stubQuotaUserRepo{user: &User{ID: 1}},
		userWriter: &stubQuotaUserWriter{},
		groupRepo: &stubGroupRepo{groups: map[int64]*Group{
			5: {ID: 5, SubscriptionType: SubscriptionTypeSubscription},
		}},
		settings: settings,
	}
	_, err := svc.CreateRule(context.Background(), 1, CreateRuleRequest{
		GroupIDs:      []int64{5},
		DailyLimitUSD: 1,
	})
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrRuleGroupSubscription))
}

func TestCreateRule_RejectsMissingGroup(t *testing.T) {
	settings := &stubQuotaSettings{enabled: true, defaultEnabled: true}
	svc := &quotaService{
		ruleRepo:   &stubQuotaRuleRepo{},
		userRepo:   &stubQuotaUserRepo{user: &User{ID: 1}},
		userWriter: &stubQuotaUserWriter{},
		groupRepo:  &stubGroupRepo{groups: map[int64]*Group{}},
		settings:   settings,
	}
	_, err := svc.CreateRule(context.Background(), 1, CreateRuleRequest{
		GroupIDs:      []int64{99},
		DailyLimitUSD: 1,
	})
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrRuleGroupNotFound))
}

func TestCreateRule_RejectsNonPositiveLimit(t *testing.T) {
	settings := &stubQuotaSettings{enabled: true, defaultEnabled: true}
	svc := &quotaService{
		ruleRepo:   &stubQuotaRuleRepo{},
		userRepo:   &stubQuotaUserRepo{user: &User{ID: 1}},
		userWriter: &stubQuotaUserWriter{},
		groupRepo:  &stubGroupRepo{groups: map[int64]*Group{1: {ID: 1}}},
		settings:   settings,
	}
	_, err := svc.CreateRule(context.Background(), 1, CreateRuleRequest{
		GroupIDs:      []int64{1},
		DailyLimitUSD: 0,
	})
	require.Error(t, err)
}

func TestCreateRule_NormalizesGroupIDsAndSucceeds(t *testing.T) {
	settings := &stubQuotaSettings{enabled: true, defaultEnabled: true}
	ruleRepo := &stubQuotaRuleRepo{}
	svc := &quotaService{
		ruleRepo:   ruleRepo,
		userRepo:   &stubQuotaUserRepo{user: &User{ID: 1}},
		userWriter: &stubQuotaUserWriter{},
		groupRepo:  &stubGroupRepo{groups: map[int64]*Group{1: {ID: 1}, 2: {ID: 2}}},
		settings:   settings,
	}
	// 提交带重复 + 乱序
	rule, err := svc.CreateRule(context.Background(), 1, CreateRuleRequest{
		GroupIDs:      []int64{2, 1, 2},
		DailyLimitUSD: 3,
	})
	require.NoError(t, err)
	assert.Equal(t, []int64{1, 2}, rule.GroupIDs)
}

func TestReplaceUserRules_Success(t *testing.T) {
	settings := &stubQuotaSettings{enabled: true, defaultEnabled: true}
	ruleRepo := &stubQuotaRuleRepo{
		rules: []*QuotaRule{{ID: 100, UserID: 7, GroupIDs: []int64{10}, DailyLimitUSD: 5, Period: QuotaPeriodDaily}},
	}
	svc := &quotaService{
		ruleRepo:   ruleRepo,
		userRepo:   &stubQuotaUserRepo{user: &User{ID: 7}},
		userWriter: &stubQuotaUserWriter{},
		groupRepo: &stubGroupRepo{groups: map[int64]*Group{
			20: {ID: 20}, 30: {ID: 30}, 40: {ID: 40},
		}},
		settings: settings,
	}
	out, err := svc.ReplaceUserRules(context.Background(), 7, []CreateRuleRequest{
		{GroupIDs: []int64{30, 20}, DailyLimitUSD: 3},
		{GroupIDs: []int64{40}, DailyLimitUSD: 2},
	})
	require.NoError(t, err)
	require.Len(t, out, 2)
	// 第一次调用的入参记录应存在，且分组归一化（去重升序）
	require.Len(t, ruleRepo.replaceAllCalls, 1)
	assert.Equal(t, []int64{20, 30}, ruleRepo.replaceAllCalls[0][0].GroupIDs)
	assert.Equal(t, []int64{40}, ruleRepo.replaceAllCalls[0][1].GroupIDs)
}

func TestReplaceUserRules_BatchInternalOverlapRollsBack(t *testing.T) {
	// 批次内部两条规则共享 group 20 → 应在调用 ReplaceAll 前返回错误，不落库
	settings := &stubQuotaSettings{enabled: true, defaultEnabled: true}
	ruleRepo := &stubQuotaRuleRepo{}
	svc := &quotaService{
		ruleRepo:   ruleRepo,
		userRepo:   &stubQuotaUserRepo{user: &User{ID: 7}},
		userWriter: &stubQuotaUserWriter{},
		groupRepo: &stubGroupRepo{groups: map[int64]*Group{
			10: {ID: 10}, 20: {ID: 20}, 30: {ID: 30},
		}},
		settings: settings,
	}
	_, err := svc.ReplaceUserRules(context.Background(), 7, []CreateRuleRequest{
		{GroupIDs: []int64{10, 20}, DailyLimitUSD: 3},
		{GroupIDs: []int64{20, 30}, DailyLimitUSD: 2},
	})
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrRuleGroupsOverlap))
	// ReplaceAll 不应被调用（校验失败阶段整体回滚）
	assert.Empty(t, ruleRepo.replaceAllCalls)
}

func TestReplaceUserRules_RepoErrorPropagates(t *testing.T) {
	settings := &stubQuotaSettings{enabled: true, defaultEnabled: true}
	sentinelErr := errors.New("tx rollback")
	ruleRepo := &stubQuotaRuleRepo{replaceErr: sentinelErr}
	svc := &quotaService{
		ruleRepo:   ruleRepo,
		userRepo:   &stubQuotaUserRepo{user: &User{ID: 7}},
		userWriter: &stubQuotaUserWriter{},
		groupRepo:  &stubGroupRepo{groups: map[int64]*Group{1: {ID: 1}}},
		settings:   settings,
	}
	_, err := svc.ReplaceUserRules(context.Background(), 7, []CreateRuleRequest{
		{GroupIDs: []int64{1}, DailyLimitUSD: 3},
	})
	require.Error(t, err)
	assert.True(t, errors.Is(err, sentinelErr))
}

func TestFormatQuotaAmount(t *testing.T) {
	assert.Equal(t, "1.00000000", formatQuotaAmount(1.0))
	assert.Equal(t, "0.12345678", formatQuotaAmount(0.12345678))
}

func TestResolvedQuotaJSONRoundTrip(t *testing.T) {
	limit := 10.0
	original := &ResolvedQuota{
		UserID:     7,
		Enabled:    true,
		DailyLimit: &limit,
		ResolvedAt: time.Unix(1700000000, 0).UTC(),
		Rules: []QuotaRule{
			{ID: 1, GroupIDs: []int64{1, 2}, DailyLimitUSD: 3, Period: QuotaPeriodDaily},
		},
	}
	raw, err := EncodeResolvedQuota(original)
	require.NoError(t, err)
	var decoded ResolvedQuota
	require.NoError(t, DecodeResolvedQuota(raw, &decoded))
	assert.Equal(t, original.UserID, decoded.UserID)
	assert.Equal(t, original.Enabled, decoded.Enabled)
	require.NotNil(t, decoded.DailyLimit)
	assert.Equal(t, *original.DailyLimit, *decoded.DailyLimit)
}

func TestQuotaExceededTotalErrorMetadata(t *testing.T) {
	t0 := time.Unix(1700000000, 0).UTC()
	err := QuotaExceededTotalError(10, 9.5, t0)
	require.Error(t, err)
	// 错误需能被 errors.Is 识别为 ErrQuotaExceeded
	assert.True(t, errors.Is(err, ErrQuotaExceeded))
}
