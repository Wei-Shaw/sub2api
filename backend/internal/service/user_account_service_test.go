//go:build unit

package service

import (
	"context"
	"errors"
	"testing"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/imroc/req/v3"
	"github.com/stretchr/testify/require"
)

// --- test doubles ---

type userAccountSettingRepo struct {
	values map[string]string
}

func (r *userAccountSettingRepo) Get(ctx context.Context, key string) (*Setting, error) {
	return nil, errors.New("not implemented")
}
func (r *userAccountSettingRepo) GetValue(_ context.Context, key string) (string, error) {
	if r.values == nil {
		return "", errors.New("missing")
	}
	v, ok := r.values[key]
	if !ok {
		return "", errors.New("missing")
	}
	return v, nil
}
func (r *userAccountSettingRepo) GetMultiple(_ context.Context, keys []string) (map[string]string, error) {
	out := make(map[string]string, len(keys))
	for _, k := range keys {
		if r.values != nil {
			if v, ok := r.values[k]; ok {
				out[k] = v
			}
		}
	}
	return out, nil
}
func (r *userAccountSettingRepo) GetAll(context.Context) (map[string]string, error) {
	return r.values, nil
}
func (r *userAccountSettingRepo) Set(context.Context, string, string) error { return nil }
func (r *userAccountSettingRepo) SetMultiple(context.Context, map[string]string) error {
	return nil
}
func (r *userAccountSettingRepo) Delete(context.Context, string) error { return nil }

func newTestSettingService(enabled bool, max int) *SettingService {
	vals := map[string]string{
		SettingKeyUserOwnedAccountsEnabled: "false",
		SettingKeyMaxUserOwnedAccounts:     "10",
	}
	if enabled {
		vals[SettingKeyUserOwnedAccountsEnabled] = "true"
	}
	if max > 0 {
		vals[SettingKeyMaxUserOwnedAccounts] = itoa(max)
	} else if max == 0 {
		vals[SettingKeyMaxUserOwnedAccounts] = "0"
	}
	return &SettingService{settingRepo: &userAccountSettingRepo{values: vals}}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	// small int helper without strconv dependency clash in test
	const digits = "0123456789"
	if n < 0 {
		return "0"
	}
	var b [12]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = digits[n%10]
		n /= 10
	}
	return string(b[i:])
}

type userAccountRepoStub struct {
	accountRepoStub
	accounts     map[int64]*Account
	nextID       int64
	ownedCount   int
	createCalls  int
	addGroupCalls [][]int64
	deletedIDs   []int64
}

func newUserAccountRepoStub() *userAccountRepoStub {
	return &userAccountRepoStub{
		accounts: make(map[int64]*Account),
		nextID:   1,
	}
}

func (s *userAccountRepoStub) Create(_ context.Context, account *Account) error {
	s.createCalls++
	if account.ID == 0 {
		account.ID = s.nextID
		s.nextID++
	}
	cp := *account
	if account.Credentials != nil {
		cp.Credentials = cloneAnyMap(account.Credentials)
	}
	if account.OwnerUserID != nil {
		oid := *account.OwnerUserID
		cp.OwnerUserID = &oid
	}
	s.accounts[account.ID] = &cp
	return nil
}

func (s *userAccountRepoStub) GetByID(_ context.Context, id int64) (*Account, error) {
	a, ok := s.accounts[id]
	if !ok {
		return nil, ErrAccountNotFound
	}
	cp := *a
	if a.OwnerUserID != nil {
		oid := *a.OwnerUserID
		cp.OwnerUserID = &oid
	}
	if a.Credentials != nil {
		cp.Credentials = cloneAnyMap(a.Credentials)
	}
	cp.GroupIDs = append([]int64(nil), a.GroupIDs...)
	return &cp, nil
}

func (s *userAccountRepoStub) Update(_ context.Context, account *Account) error {
	if account == nil {
		return ErrAccountNilInput
	}
	cp := *account
	if account.OwnerUserID != nil {
		oid := *account.OwnerUserID
		cp.OwnerUserID = &oid
	}
	if account.Credentials != nil {
		cp.Credentials = cloneAnyMap(account.Credentials)
	}
	if account.Extra != nil {
		cp.Extra = cloneAnyMap(account.Extra)
	}
	cp.GroupIDs = append([]int64(nil), account.GroupIDs...)
	s.accounts[account.ID] = &cp
	return nil
}

func (s *userAccountRepoStub) UpdateExtra(_ context.Context, id int64, updates map[string]any) error {
	a, ok := s.accounts[id]
	if !ok {
		return ErrAccountNotFound
	}
	if a.Extra == nil {
		a.Extra = make(map[string]any)
	}
	for k, v := range updates {
		a.Extra[k] = v
	}
	return nil
}

func (s *userAccountRepoStub) Delete(_ context.Context, id int64) error {
	s.deletedIDs = append(s.deletedIDs, id)
	delete(s.accounts, id)
	return nil
}

func (s *userAccountRepoStub) AddGroups(_ context.Context, accountID int64, groupIDs []int64) error {
	s.addGroupCalls = append(s.addGroupCalls, append([]int64(nil), groupIDs...))
	a, ok := s.accounts[accountID]
	if !ok {
		// Create 后内存中已有
		return nil
	}
	set := make(map[int64]struct{})
	for _, id := range a.GroupIDs {
		set[id] = struct{}{}
	}
	for _, gid := range groupIDs {
		if _, exists := set[gid]; !exists {
			a.GroupIDs = append(a.GroupIDs, gid)
		}
	}
	return nil
}

func (s *userAccountRepoStub) RemoveGroups(_ context.Context, accountID int64, groupIDs []int64) error {
	a, ok := s.accounts[accountID]
	if !ok {
		return nil
	}
	rm := make(map[int64]struct{}, len(groupIDs))
	for _, id := range groupIDs {
		rm[id] = struct{}{}
	}
	out := make([]int64, 0, len(a.GroupIDs))
	for _, id := range a.GroupIDs {
		if _, drop := rm[id]; !drop {
			out = append(out, id)
		}
	}
	a.GroupIDs = out
	return nil
}

func (s *userAccountRepoStub) CountActiveOwned(_ context.Context, ownerUserID int64) (int, error) {
	if s.ownedCount > 0 {
		return s.ownedCount, nil
	}
	n := 0
	for _, a := range s.accounts {
		if a.OwnerUserID != nil && *a.OwnerUserID == ownerUserID {
			n++
		}
	}
	return n, nil
}

func (s *userAccountRepoStub) ListByOwnerUserID(_ context.Context, ownerUserID int64, params pagination.PaginationParams) ([]Account, *pagination.PaginationResult, error) {
	var out []Account
	for _, a := range s.accounts {
		if a.OwnerUserID != nil && *a.OwnerUserID == ownerUserID {
			out = append(out, *a)
		}
	}
	return out, &pagination.PaginationResult{Total: int64(len(out))}, nil
}

func (s *userAccountRepoStub) BulkUpdate(_ context.Context, ids []int64, updates AccountBulkUpdate) (int64, error) {
	for _, id := range ids {
		a, ok := s.accounts[id]
		if !ok {
			continue
		}
		if updates.Credentials != nil {
			if a.Credentials == nil {
				a.Credentials = map[string]any{}
			}
			for k, v := range updates.Credentials {
				a.Credentials[k] = v
			}
		}
		if updates.UpstreamPlan != nil {
			a.UpstreamPlan = *updates.UpstreamPlan
		}
	}
	return int64(len(ids)), nil
}

type userAccountProvisionerStub struct {
	groups      map[string]*Group // platform -> group
	ensureCalls []string
	afterCommit int
}

func (p *userAccountProvisionerStub) ProvisionPrivatePlatformGroups(context.Context, int64) (*ProvisionResult, error) {
	return &ProvisionResult{}, nil
}
func (p *userAccountProvisionerStub) EnsurePrivateGroupForPlatform(_ context.Context, userID int64, platform string) (*Group, *ProvisionResult, error) {
	p.ensureCalls = append(p.ensureCalls, platform)
	if p.groups == nil {
		p.groups = map[string]*Group{}
	}
	g, ok := p.groups[platform]
	if !ok {
		g = &Group{ID: int64(100 + len(p.groups)), Name: PrivateGroupName(userID, platform), Platform: platform, Status: StatusActive}
		p.groups[platform] = g
	}
	return g, &ProvisionResult{UserID: userID, EnsuredGroupIDs: []int64{g.ID}}, nil
}
func (p *userAccountProvisionerStub) RevokePrivatePlatformGroups(context.Context, int64) (*RevokeResult, error) {
	return &RevokeResult{}, nil
}
func (p *userAccountProvisionerStub) SyncPrivateSubscriptionExpiresAt(context.Context) (*SyncResult, error) {
	return &SyncResult{}, nil
}
func (p *userAccountProvisionerStub) AfterCommit(context.Context, *ProvisionResult) { p.afterCommit++ }
func (p *userAccountProvisionerStub) AfterRevokeCommit(context.Context, *RevokeResult) {
}

func newUserAccountServiceForTest(
	repo *userAccountRepoStub,
	prov *userAccountProvisionerStub,
	settings *SettingService,
) UserAccountService {
	var groups []*Group
	if prov != nil {
		for _, g := range prov.groups {
			groups = append(groups, g)
		}
	}
	return NewUserAccountService(repo, newRecomputeGroupRepo(groups...), prov, settings, nil, nil)
}

// --- tests ---

func TestUserAccountService_FlagOff_Returns403(t *testing.T) {
	repo := newUserAccountRepoStub()
	svc := newUserAccountServiceForTest(repo, &userAccountProvisionerStub{}, newTestSettingService(false, 10))

	_, err := svc.Create(context.Background(), 1, &CreateUserAccountInput{
		Name: "a", Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Visibility: VisibilityPrivate,
		Credentials: map[string]any{"api_key": "sk-x"},
	})
	require.Error(t, err)
	require.True(t, infraerrors.IsForbidden(err))
	require.Equal(t, "USER_OWNED_ACCOUNTS_DISABLED", infraerrors.Reason(err))
}

func TestUserAccountService_OverLimit_Returns400(t *testing.T) {
	repo := newUserAccountRepoStub()
	repo.ownedCount = 10
	svc := newUserAccountServiceForTest(repo, &userAccountProvisionerStub{}, newTestSettingService(true, 10))

	_, err := svc.Create(context.Background(), 1, &CreateUserAccountInput{
		Name: "a", Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Visibility: VisibilityPrivate,
		Credentials: map[string]any{"api_key": "sk-x"},
	})
	require.Error(t, err)
	require.True(t, infraerrors.IsBadRequest(err))
	require.Equal(t, "USER_ACCOUNT_LIMIT_EXCEEDED", infraerrors.Reason(err))
}

func TestUserAccountService_CreatePrivate_BindsPrivateGroup(t *testing.T) {
	repo := newUserAccountRepoStub()
	prov := &userAccountProvisionerStub{}
	svc := newUserAccountServiceForTest(repo, prov, newTestSettingService(true, 10))

	acc, err := svc.Create(context.Background(), 42, &CreateUserAccountInput{
		Name: "my-key", Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Visibility: VisibilityPrivate,
		Credentials: map[string]any{"api_key": "sk-test"},
	})
	require.NoError(t, err)
	require.NotNil(t, acc)
	require.Equal(t, VisibilityPrivate, acc.Visibility)
	require.NotNil(t, acc.OwnerUserID)
	require.Equal(t, int64(42), *acc.OwnerUserID)
	require.Len(t, prov.ensureCalls, 1)
	require.Equal(t, PlatformOpenAI, prov.ensureCalls[0])
	require.NotEmpty(t, repo.addGroupCalls)
	require.Contains(t, acc.GroupIDs, prov.groups[PlatformOpenAI].ID)
}

func TestUserAccountService_CreatePublicWithoutPlan_StaysPrivate(t *testing.T) {
	repo := newUserAccountRepoStub()
	prov := &userAccountProvisionerStub{}
	svc := newUserAccountServiceForTest(repo, prov, newTestSettingService(true, 10))

	// openai oauth without plan credentials → 无匹配共享池 → 强制 private（空档位 reason=plan_empty）
	acc, err := svc.Create(context.Background(), 7, &CreateUserAccountInput{
		Name: "oauth-acc", Platform: PlatformOpenAI, Type: AccountTypeOAuth, Visibility: VisibilityPublic,
		Credentials: map[string]any{"access_token": "tok", "refresh_token": "rt"},
	})
	require.NoError(t, err)
	require.Equal(t, VisibilityPrivate, acc.Visibility)
	require.Equal(t, VisibilityReasonPlanEmpty, acc.VisibilityReason)
	require.Empty(t, acc.UpstreamPlan)
}

func TestUserAccountService_CreatePublicWithPlan_Promotes(t *testing.T) {
	repo := newUserAccountRepoStub()
	// seed private group + matching share pool（升 public 要求至少命中一个共享池）
	g := &Group{ID: 501, Name: PrivateGroupName(7, PlatformOpenAI), Platform: PlatformOpenAI, Status: StatusActive}
	pool := &Group{
		ID: 502, Name: "openai-plus-pool", Platform: PlatformOpenAI,
		IsSharePool: true, UpstreamPlan: "plus", Status: StatusActive,
	}
	prov := &userAccountProvisionerStub{groups: map[string]*Group{PlatformOpenAI: g}}
	groupRepo := newRecomputeGroupRepo(g, pool)
	svc := NewUserAccountService(repo, groupRepo, prov, newTestSettingService(true, 10), nil, nil)

	acc, err := svc.Create(context.Background(), 7, &CreateUserAccountInput{
		Name: "oauth-plus", Platform: PlatformOpenAI, Type: AccountTypeOAuth, Visibility: VisibilityPublic,
		Credentials: map[string]any{
			"access_token": "tok",
			"plan_type":    "plus",
		},
	})
	require.NoError(t, err)
	// plus 在 DefaultGroupUpstreamPlansSeed(openai) 中 → 应升 public
	require.Equal(t, "plus", acc.UpstreamPlan)
	require.Equal(t, VisibilityPublic, acc.Visibility)
}

func TestEnsureUserOwnedOpenAIPrivacy_SetsMode(t *testing.T) {
	repo := newUserAccountRepoStub()
	acc := &Account{
		ID:       11,
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
		Credentials: map[string]any{
			"access_token": "tok",
		},
		Extra: map[string]any{},
	}
	repo.accounts[11] = acc

	// factory nil → no-op
	ensureUserOwnedOpenAIPrivacy(context.Background(), repo, nil, acc)
	require.Empty(t, acc.Extra["privacy_mode"])

	// factory 创建客户端失败 → disableOpenAITraining 返回 PrivacyModeFailed 并落库
	factory := PrivacyClientFactory(func(proxyURL string) (*req.Client, error) {
		return nil, errors.New("no client")
	})
	ensureUserOwnedOpenAIPrivacy(context.Background(), repo, factory, acc)
	require.Equal(t, PrivacyModeFailed, acc.Extra["privacy_mode"])

	// already training_off → skip（不覆盖）
	acc.Extra["privacy_mode"] = PrivacyModeTrainingOff
	ensureUserOwnedOpenAIPrivacy(context.Background(), repo, factory, acc)
	require.Equal(t, PrivacyModeTrainingOff, acc.Extra["privacy_mode"])
}

func TestUserAccountService_NonOwnerGet_404(t *testing.T) {
	repo := newUserAccountRepoStub()
	owner := int64(1)
	repo.accounts[9] = &Account{ID: 9, Name: "x", OwnerUserID: &owner, Visibility: VisibilityPrivate, Platform: PlatformOpenAI}
	svc := newUserAccountServiceForTest(repo, &userAccountProvisionerStub{}, newTestSettingService(true, 10))

	_, err := svc.Get(context.Background(), 2, 9)
	require.Error(t, err)
	require.True(t, infraerrors.IsNotFound(err))
}

func TestUserAccountService_OwnerGet_OK(t *testing.T) {
	repo := newUserAccountRepoStub()
	owner := int64(1)
	repo.accounts[9] = &Account{ID: 9, Name: "x", OwnerUserID: &owner, Visibility: VisibilityPrivate, Platform: PlatformOpenAI}
	svc := newUserAccountServiceForTest(repo, &userAccountProvisionerStub{}, newTestSettingService(true, 10))

	acc, err := svc.Get(context.Background(), 1, 9)
	require.NoError(t, err)
	require.Equal(t, int64(9), acc.ID)
}

func TestDeleteUser_CascadesOwnedAccounts(t *testing.T) {
	// 直接测 deleteOwnedAccountsForUser 逻辑（通过 adminServiceImpl + stub）
	repo := newUserAccountRepoStub()
	owner := int64(55)
	repo.accounts[1] = &Account{ID: 1, OwnerUserID: &owner, Name: "a1"}
	repo.accounts[2] = &Account{ID: 2, OwnerUserID: &owner, Name: "a2"}
	other := int64(99)
	repo.accounts[3] = &Account{ID: 3, OwnerUserID: &other, Name: "other"}

	svc := &adminServiceImpl{accountRepo: repo}
	err := svc.deleteOwnedAccountsForUser(context.Background(), owner)
	require.NoError(t, err)
	require.ElementsMatch(t, []int64{1, 2}, repo.deletedIDs)
	_, err = repo.GetByID(context.Background(), 3)
	require.NoError(t, err, "other user's account must remain")
}

func TestUserAccountService_OpenAIAPIKeyPublic_ForcePrivateUnsupported(t *testing.T) {
	repo := newUserAccountRepoStub()
	prov := &userAccountProvisionerStub{}
	svc := newUserAccountServiceForTest(repo, prov, newTestSettingService(true, 10))

	acc, err := svc.Create(context.Background(), 3, &CreateUserAccountInput{
		Name: "key", Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Visibility: VisibilityPublic,
		Credentials: map[string]any{"api_key": "sk-x", "plan_type": "plus"},
	})
	require.NoError(t, err)
	require.Equal(t, VisibilityPrivate, acc.Visibility)
	require.Equal(t, VisibilityReasonPlanProbeUnsupported, acc.VisibilityReason)
}
