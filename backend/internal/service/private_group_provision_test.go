//go:build unit

package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/stretchr/testify/require"
)

func TestPrivateGroupName_AndIsPrivateGroupName(t *testing.T) {
	name := PrivateGroupName(42, PlatformAnthropic)
	require.Equal(t, "private-42-anthropic", name)
	require.True(t, IsPrivateGroupName(name))
	require.True(t, IsPrivateGroupNameForUser(name, 42))
	require.False(t, IsPrivateGroupNameForUser(name, 43))

	require.False(t, IsPrivateGroupName("private-42-composite"))
	require.False(t, IsPrivateGroupName("private-x-anthropic"))
	require.False(t, IsPrivateGroupName("ops-group"))
	require.False(t, IsPrivateGroupName("private-42-ANTHROPIC"))

	uid, platform, ok := ParsePrivateGroupName(name)
	require.True(t, ok)
	require.Equal(t, int64(42), uid)
	require.Equal(t, PlatformAnthropic, platform)
}

func TestPrivateGroupPlatforms_EqualsAllowedQuotaPlatforms(t *testing.T) {
	got := privateGroupPlatforms()
	require.Equal(t, AllowedQuotaPlatforms, got)
	got[0] = "mutated"
	require.NotEqual(t, got[0], AllowedQuotaPlatforms[0])
}

func TestBuildPrivateGroup_ExplicitMultipliers(t *testing.T) {
	g := buildPrivateGroup(7, PlatformGrok)
	require.Equal(t, "private-7-grok", g.Name)
	require.Equal(t, SubscriptionTypeSubscription, g.SubscriptionType)
	require.True(t, g.IsExclusive)
	require.False(t, g.IsSharePool, "private groups must never be share pools")
	require.Equal(t, 1.0, g.RateMultiplier)
	require.Equal(t, 1.0, g.ImageRateMultiplier)
	require.Equal(t, 1.0, g.VideoRateMultiplier)
	require.Equal(t, 1.0, g.PeakRateMultiplier)
	require.Equal(t, defaultBatchImageDiscountMultiplier, g.BatchImageDiscountMultiplier)
	require.Equal(t, defaultBatchImageHoldMultiplier, g.BatchImageHoldMultiplier)
	require.Equal(t, 365, g.DefaultValidityDays)
	require.True(t, g.AllowImageGeneration, "grok defaults allow image")
	require.True(t, g.MCPXMLInject)
	require.Contains(t, g.Description, "user_id=7")
	require.Contains(t, g.Description, "platform=grok")

	anthropic := buildPrivateGroup(1, PlatformAnthropic)
	require.False(t, anthropic.AllowImageGeneration)
}

func TestMergePrivateAllowedGroupIDs(t *testing.T) {
	privateGroups := []Group{
		{ID: 101, Name: PrivateGroupName(5, PlatformAnthropic)},
		{ID: 102, Name: PrivateGroupName(5, PlatformOpenAI)},
		{ID: 999, Name: "ops-exclusive"},
	}
	merged := mergePrivateAllowedGroupIDs([]int64{1, 2, 101}, privateGroups, 5)
	require.Contains(t, merged, int64(1))
	require.Contains(t, merged, int64(2))
	require.Contains(t, merged, int64(101))
	require.Contains(t, merged, int64(102))
	require.NotContains(t, merged, int64(999))
}

func TestFilterOutPrivateGroups(t *testing.T) {
	in := []Group{
		{ID: 1, Name: "ops"},
		{ID: 2, Name: PrivateGroupName(1, PlatformGemini)},
	}
	out := filterOutPrivateGroups(in)
	require.Len(t, out, 1)
	require.Equal(t, int64(1), out[0].ID)
}


func TestValidatePrivateGroupIdentityUpdate(t *testing.T) {
	private := &Group{
		ID:               10,
		Name:             PrivateGroupName(3, PlatformAnthropic),
		Platform:         PlatformAnthropic,
		SubscriptionType: SubscriptionTypeSubscription,
		IsExclusive:      true,
	}
	ops := &Group{
		ID:               11,
		Name:             "ops-group",
		Platform:         PlatformOpenAI,
		SubscriptionType: SubscriptionTypeStandard,
		IsExclusive:      false,
	}

	// non-private: no lock
	require.NoError(t, validatePrivateGroupIdentityUpdate(ops, &UpdateGroupInput{Name: "renamed"}))

	// same-value submit allowed
	trueVal := true
	require.NoError(t, validatePrivateGroupIdentityUpdate(private, &UpdateGroupInput{
		Name:             private.Name,
		Platform:         private.Platform,
		SubscriptionType: private.SubscriptionType,
		IsExclusive:      &trueVal,
	}))

	err := validatePrivateGroupIdentityUpdate(private, &UpdateGroupInput{Name: "hacked"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "name")

	err = validatePrivateGroupIdentityUpdate(private, &UpdateGroupInput{Platform: PlatformOpenAI})
	require.Error(t, err)
	require.Contains(t, err.Error(), "platform")

	err = validatePrivateGroupIdentityUpdate(private, &UpdateGroupInput{SubscriptionType: SubscriptionTypeStandard})
	require.Error(t, err)
	require.Contains(t, err.Error(), "subscription_type")

	falseVal := false
	err = validatePrivateGroupIdentityUpdate(private, &UpdateGroupInput{IsExclusive: &falseVal})
	require.Error(t, err)
	require.Contains(t, err.Error(), "is_exclusive")

	sharePoolTrue := true
	err = validatePrivateGroupIdentityUpdate(private, &UpdateGroupInput{IsSharePool: &sharePoolTrue})
	require.Error(t, err)
	require.Contains(t, err.Error(), "share pool")

	// clearing / keeping false is allowed
	sharePoolFalse := false
	require.NoError(t, validatePrivateGroupIdentityUpdate(private, &UpdateGroupInput{IsSharePool: &sharePoolFalse}))
}

type stubGroupRepoForProvision struct {
	byName          map[string]*Group
	created         []*Group
	createErr       error
	nextID          int64
	deleted         []int64
	deleteFailID    int64 // 非 0 时 DeleteCascade 对该 ID 返回错误
	enqueueIDs      []int64
	listActive      []Group
	listExcl        []Group
	listByIDsFn     func([]int64) ([]Group, error)
	listActiveCalls int
	listExclCalls   int
	listByIDsCalls  int
}

func (s *stubGroupRepoForProvision) Create(_ context.Context, g *Group) error {
	if s.createErr != nil {
		return s.createErr
	}
	if s.byName == nil {
		s.byName = map[string]*Group{}
	}
	if s.nextID == 0 {
		s.nextID = 100
	}
	s.nextID++
	g.ID = s.nextID
	cp := *g
	s.byName[g.Name] = &cp
	s.created = append(s.created, &cp)
	return nil
}
func (s *stubGroupRepoForProvision) GetByID(context.Context, int64) (*Group, error) {
	return nil, ErrGroupNotFound
}
func (s *stubGroupRepoForProvision) GetByIDLite(context.Context, int64) (*Group, error) {
	return nil, ErrGroupNotFound
}
func (s *stubGroupRepoForProvision) GetByName(_ context.Context, name string) (*Group, error) {
	if s.byName != nil {
		if g, ok := s.byName[name]; ok {
			cp := *g
			return &cp, nil
		}
	}
	return nil, ErrGroupNotFound
}
func (s *stubGroupRepoForProvision) Update(context.Context, *Group) error { return nil }
func (s *stubGroupRepoForProvision) Delete(context.Context, int64) error  { return nil }
func (s *stubGroupRepoForProvision) DeleteCascade(_ context.Context, id int64) ([]int64, error) {
	if s.deleteFailID != 0 && id == s.deleteFailID {
		return nil, errors.New("cascade failed")
	}
	s.deleted = append(s.deleted, id)
	for k, g := range s.byName {
		if g.ID == id {
			delete(s.byName, k)
		}
	}
	return []int64{}, nil
}
func (s *stubGroupRepoForProvision) List(context.Context, pagination.PaginationParams) ([]Group, *pagination.PaginationResult, error) {
	return nil, nil, nil
}
func (s *stubGroupRepoForProvision) ListWithFilters(context.Context, pagination.PaginationParams, string, string, string, *bool, bool) ([]Group, *pagination.PaginationResult, error) {
	return nil, nil, nil
}
func (s *stubGroupRepoForProvision) ListActive(context.Context) ([]Group, error) {
	s.listActiveCalls++
	return s.listActive, nil
}
func (s *stubGroupRepoForProvision) ListActiveExcludingPrivate(context.Context) ([]Group, error) {
	s.listExclCalls++
	return s.listExcl, nil
}
func (s *stubGroupRepoForProvision) ListActiveByPlatform(context.Context, string) ([]Group, error) {
	return nil, nil
}
func (s *stubGroupRepoForProvision) ListByIDs(_ context.Context, ids []int64) ([]Group, error) {
	s.listByIDsCalls++
	if s.listByIDsFn != nil {
		return s.listByIDsFn(ids)
	}
	return nil, nil
}
func (s *stubGroupRepoForProvision) ExistsByName(context.Context, string) (bool, error) {
	return false, nil
}
func (s *stubGroupRepoForProvision) GetAccountCount(context.Context, int64) (int64, int64, error) {
	return 0, 0, nil
}
func (s *stubGroupRepoForProvision) DeleteAccountGroupsByGroupID(context.Context, int64) (int64, error) {
	return 0, nil
}
func (s *stubGroupRepoForProvision) GetAccountIDsByGroupIDs(context.Context, []int64) ([]int64, error) {
	return nil, nil
}
func (s *stubGroupRepoForProvision) BindAccountsToGroup(context.Context, int64, []int64) error {
	return nil
}
func (s *stubGroupRepoForProvision) UpdateSortOrders(context.Context, []GroupSortOrderUpdate) error {
	return nil
}
func (s *stubGroupRepoForProvision) EnqueueGroupChanged(_ context.Context, groupID int64) error {
	s.enqueueIDs = append(s.enqueueIDs, groupID)
	return nil
}

type stubSubEnsure struct {
	calls         int
	deferFlags    []bool
	err           error
	syncCalls     int
	syncUpdated   int64
	syncPairs     []privateSubUserGroup
	syncErr       error
	syncExpiresAt time.Time
	syncStatus    string
}

func (s *stubSubEnsure) EnsureSubscriptionWithExpiresAt(
	_ context.Context, _, groupID int64, _ time.Time, _ string, deferCache bool,
) (*UserSubscription, error) {
	s.calls++
	s.deferFlags = append(s.deferFlags, deferCache)
	if s.err != nil {
		return nil, s.err
	}
	return &UserSubscription{GroupID: groupID, Status: SubscriptionStatusActive}, nil
}
func (s *stubSubEnsure) InvalidateSubCache(_, _ int64) {}
func (s *stubSubEnsure) BulkSyncPrivateSubscriptionExpires(
	_ context.Context, expiresAt time.Time, status string,
) (int64, []privateSubUserGroup, error) {
	s.syncCalls++
	s.syncExpiresAt = expiresAt
	s.syncStatus = status
	if s.syncErr != nil {
		return 0, nil, s.syncErr
	}
	return s.syncUpdated, s.syncPairs, nil
}

// userRepoStubForProvision 在 userRepoStub 上允许 AddGroupToAllowedGroups。
type userRepoStubForProvision struct {
	userRepoStub
	allowedAdds []int64
}

func (s *userRepoStubForProvision) AddGroupToAllowedGroups(_ context.Context, _ int64, groupID int64) error {
	s.allowedAdds = append(s.allowedAdds, groupID)
	return nil
}

func newTestProvisioner(user *User, groupRepo *stubGroupRepoForProvision, sub *stubSubEnsure) *privateGroupProvisioner {
	if groupRepo == nil {
		groupRepo = &stubGroupRepoForProvision{byName: map[string]*Group{}}
	}
	if sub == nil {
		sub = &stubSubEnsure{}
	}
	userRepo := &userRepoStubForProvision{userRepoStub: userRepoStub{user: user}}
	cfg := &config.Config{}
	settings := NewSettingService(&settingRepoStub{values: map[string]string{}}, cfg)
	return &privateGroupProvisioner{
		groupRepo:      groupRepo,
		userRepo:       userRepo,
		subEnsure:      sub,
		settingService: settings,
	}
}

func TestCanProvisionPrivateGroups(t *testing.T) {
	require.True(t, CanProvisionPrivateGroups(RoleUser))
	require.True(t, CanProvisionPrivateGroups(RoleAdmin))
	require.False(t, CanProvisionPrivateGroups("guest"))
	require.False(t, CanProvisionPrivateGroups(""))
}

func TestProvision_CreatesGroupsForAdminRole(t *testing.T) {
	groupRepo := &stubGroupRepoForProvision{byName: map[string]*Group{}, nextID: 300}
	sub := &stubSubEnsure{}
	p := newTestProvisioner(&User{ID: 1, Role: RoleAdmin}, groupRepo, sub)
	result, err := p.ProvisionPrivatePlatformGroups(context.Background(), 1)
	require.NoError(t, err)
	require.Len(t, result.CreatedGroupIDs, len(AllowedQuotaPlatforms))
	require.Len(t, result.EnsuredGroupIDs, len(AllowedQuotaPlatforms))
	require.Len(t, groupRepo.created, len(AllowedQuotaPlatforms))
}

func TestProvision_CreatesFiveGroupsForUser(t *testing.T) {
	groupRepo := &stubGroupRepoForProvision{byName: map[string]*Group{}, nextID: 200}
	sub := &stubSubEnsure{}
	p := newTestProvisioner(&User{ID: 9, Role: RoleUser}, groupRepo, sub)

	result, err := p.ProvisionPrivatePlatformGroups(context.Background(), 9)
	require.NoError(t, err)
	require.Len(t, result.CreatedGroupIDs, len(AllowedQuotaPlatforms))
	require.Len(t, result.EnsuredGroupIDs, len(AllowedQuotaPlatforms))
	require.Len(t, groupRepo.created, len(AllowedQuotaPlatforms))
	require.Equal(t, len(AllowedQuotaPlatforms), sub.calls)

	for _, g := range groupRepo.created {
		require.True(t, IsPrivateGroupName(g.Name))
		require.Equal(t, defaultBatchImageDiscountMultiplier, g.BatchImageDiscountMultiplier)
		require.Equal(t, defaultBatchImageHoldMultiplier, g.BatchImageHoldMultiplier)
		require.Equal(t, 1.0, g.RateMultiplier)
	}

	result2, err := p.ProvisionPrivatePlatformGroups(context.Background(), 9)
	require.NoError(t, err)
	require.Empty(t, result2.CreatedGroupIDs)
	require.Len(t, result2.EnsuredGroupIDs, len(AllowedQuotaPlatforms))
	require.Len(t, groupRepo.created, len(AllowedQuotaPlatforms))
}

func TestProvision_FailsClosedOnEnsureError(t *testing.T) {
	groupRepo := &stubGroupRepoForProvision{byName: map[string]*Group{}}
	sub := &stubSubEnsure{err: errors.New("ensure boom")}
	p := newTestProvisioner(&User{ID: 3, Role: RoleUser}, groupRepo, sub)
	_, err := p.ProvisionPrivatePlatformGroups(context.Background(), 3)
	require.Error(t, err)
	require.Contains(t, err.Error(), "ensure private subscription")
}

func TestRevoke_DeletesPrivateGroups(t *testing.T) {
	groupRepo := &stubGroupRepoForProvision{byName: map[string]*Group{}}
	g1 := buildPrivateGroup(4, PlatformAnthropic)
	g1.ID = 11
	g2 := buildPrivateGroup(4, PlatformOpenAI)
	g2.ID = 12
	groupRepo.byName[g1.Name] = g1
	groupRepo.byName[g2.Name] = g2

	p := newTestProvisioner(&User{ID: 4, Role: RoleUser}, groupRepo, &stubSubEnsure{})
	result, err := p.RevokePrivatePlatformGroups(context.Background(), 4)
	require.NoError(t, err)
	require.Len(t, result.DeletedGroupIDs, 2)
	require.ElementsMatch(t, []int64{11, 12}, result.DeletedGroupIDs)
}

type recordingProvisioner struct {
	provisioned []int64
	revoked     []int64
}

func (r *recordingProvisioner) ProvisionPrivatePlatformGroups(_ context.Context, userID int64) (*ProvisionResult, error) {
	r.provisioned = append(r.provisioned, userID)
	return &ProvisionResult{UserID: userID, CreatedGroupIDs: []int64{1}, EnsuredGroupIDs: []int64{1, 2}}, nil
}
func (r *recordingProvisioner) RevokePrivatePlatformGroups(_ context.Context, userID int64) (*RevokeResult, error) {
	r.revoked = append(r.revoked, userID)
	return &RevokeResult{UserID: userID}, nil
}
func (r *recordingProvisioner) SyncPrivateSubscriptionExpiresAt(context.Context) (*SyncResult, error) {
	return &SyncResult{Updated: 0}, nil
}
func (r *recordingProvisioner) AfterCommit(context.Context, *ProvisionResult)     {}
func (r *recordingProvisioner) AfterRevokeCommit(context.Context, *RevokeResult) {}

func TestAdminCreateUser_RejectsPrivateGroupsWithoutEntClient(t *testing.T) {
	// privateGroups 已注入但无 entClient → 拒绝创建，避免非原子半成品。
	rec := &recordingProvisioner{}
	svc := &adminServiceImpl{
		userRepo:      &userRepoStub{nextID: 50},
		privateGroups: rec,
	}
	_, err := svc.CreateUser(context.Background(), &CreateUserInput{
		Email:    "u@test.com",
		Password: "password123",
		Role:     RoleUser,
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "requires entClient")
	require.Empty(t, rec.provisioned)
}

func TestAdminCreateUser_AdminRoleProvisionsPrivateGroups(t *testing.T) {
	// admin 与 user 同等供给；无 entClient 时直接测 ProvisionPrivatePlatformGroups。
	groupRepo := &stubGroupRepoForProvision{byName: map[string]*Group{}, nextID: 510}
	p := newTestProvisioner(&User{ID: 51, Role: RoleAdmin}, groupRepo, &stubSubEnsure{})
	result, err := p.ProvisionPrivatePlatformGroups(context.Background(), 51)
	require.NoError(t, err)
	require.Len(t, result.CreatedGroupIDs, len(AllowedQuotaPlatforms))
	require.Len(t, groupRepo.created, len(AllowedQuotaPlatforms))
}

func TestAdminDeleteUser_RevokesPrivateGroups(t *testing.T) {
	rec := &recordingProvisioner{}
	svc := &adminServiceImpl{
		userRepo:      &userRepoStub{user: &User{ID: 8, Role: RoleUser, Status: StatusActive}},
		privateGroups: rec,
	}
	require.NoError(t, svc.DeleteUser(context.Background(), 8))
	require.Equal(t, []int64{8}, rec.revoked)
}

func TestUpdateUser_PreservesPrivateAllowedGroups(t *testing.T) {
	userRepo := &userRepoStub{
		user: &User{ID: 10, Email: "u@t.com", Role: RoleUser, Status: StatusActive, AllowedGroups: []int64{1, 101, 102}},
	}
	groupRepo := &stubGroupRepoForProvision{byName: map[string]*Group{
		PrivateGroupName(10, PlatformAnthropic): {ID: 101, Name: PrivateGroupName(10, PlatformAnthropic)},
		PrivateGroupName(10, PlatformOpenAI):    {ID: 102, Name: PrivateGroupName(10, PlatformOpenAI)},
	}}
	svc := &adminServiceImpl{userRepo: userRepo, groupRepo: groupRepo}
	allowed := []int64{1}
	updated, err := svc.UpdateUser(context.Background(), 10, &UpdateUserInput{AllowedGroups: &allowed})
	require.NoError(t, err)
	require.Contains(t, updated.AllowedGroups, int64(1))
	require.Contains(t, updated.AllowedGroups, int64(101))
	require.Contains(t, updated.AllowedGroups, int64(102))
}

func TestUpdateUser_AllowedMergeFailsOnGetByNameError(t *testing.T) {
	userRepo := &userRepoStub{
		user: &User{ID: 10, Email: "u@t.com", Role: RoleUser, Status: StatusActive, AllowedGroups: []int64{1, 101}},
	}
	groupRepo := &stubGroupRepoForProvision{
		byName: map[string]*Group{},
		// GetByName 默认 not-found；用 getByNameErr 注入
	}
	// 覆盖 GetByName 行为：对 anthropic 返回瞬时错误
	failing := &stubGroupRepoGetByNameFail{
		stubGroupRepoForProvision: *groupRepo,
		failName:                  PrivateGroupName(10, PlatformAnthropic),
		failErr:                   errors.New("db timeout"),
	}
	svc := &adminServiceImpl{userRepo: userRepo, groupRepo: failing}
	allowed := []int64{1}
	_, err := svc.UpdateUser(context.Background(), 10, &UpdateUserInput{AllowedGroups: &allowed})
	require.Error(t, err)
	require.Contains(t, err.Error(), "load private group for allowed merge")
	// 未写入 Update，避免抹掉 private allowed
	require.Empty(t, userRepo.updated)
}

type stubGroupRepoGetByNameFail struct {
	stubGroupRepoForProvision
	failName string
	failErr  error
}

func (s *stubGroupRepoGetByNameFail) GetByName(ctx context.Context, name string) (*Group, error) {
	if name == s.failName {
		return nil, s.failErr
	}
	return s.stubGroupRepoForProvision.GetByName(ctx, name)
}

func TestRevoke_ContinuesAfterSinglePlatformFailure(t *testing.T) {
	groupRepo := &stubGroupRepoForProvision{byName: map[string]*Group{}}
	gOK := buildPrivateGroup(4, PlatformOpenAI)
	gOK.ID = 12
	groupRepo.byName[gOK.Name] = gOK

	// anthropic GetByName 将在 cascade 路径外：用 delete fail 模拟中途失败
	// 先放 anthropic 组，DeleteCascade 对 id=11 失败
	gFail := buildPrivateGroup(4, PlatformAnthropic)
	gFail.ID = 11
	groupRepo.byName[gFail.Name] = gFail
	groupRepo.deleteFailID = 11

	p := newTestProvisioner(&User{ID: 4, Role: RoleUser}, groupRepo, &stubSubEnsure{})
	result, err := p.RevokePrivatePlatformGroups(context.Background(), 4)
	require.Error(t, err)
	// openai 仍应被删除
	require.Contains(t, result.DeletedGroupIDs, int64(12))
	require.NotContains(t, result.DeletedGroupIDs, int64(11))
}

func TestAdminGetAllGroups_UsesExcludingPrivate(t *testing.T) {
	groupRepo := &stubGroupRepoForProvision{
		listExcl:   []Group{{ID: 1, Name: "ops"}},
		listActive: []Group{{ID: 1, Name: "ops"}, {ID: 2, Name: PrivateGroupName(1, PlatformAnthropic)}},
	}
	svc := &adminServiceImpl{groupRepo: groupRepo}
	groups, err := svc.GetAllGroups(context.Background())
	require.NoError(t, err)
	require.Equal(t, 1, groupRepo.listExclCalls)
	require.Equal(t, 0, groupRepo.listActiveCalls)
	require.Len(t, groups, 1)
	require.Equal(t, "ops", groups[0].Name)
}

type userSubRepoStubForAvailable struct {
	subs []UserSubscription
}

func (s *userSubRepoStubForAvailable) Create(context.Context, *UserSubscription) error { return nil }
func (s *userSubRepoStubForAvailable) GetByID(context.Context, int64) (*UserSubscription, error) {
	return nil, ErrSubscriptionNotFound
}
func (s *userSubRepoStubForAvailable) GetByIDIncludeDeleted(context.Context, int64) (*UserSubscription, error) {
	return nil, ErrSubscriptionNotFound
}
func (s *userSubRepoStubForAvailable) GetByUserIDAndGroupID(context.Context, int64, int64) (*UserSubscription, error) {
	return nil, ErrSubscriptionNotFound
}
func (s *userSubRepoStubForAvailable) GetActiveByUserIDAndGroupID(context.Context, int64, int64) (*UserSubscription, error) {
	return nil, ErrSubscriptionNotFound
}
func (s *userSubRepoStubForAvailable) Update(context.Context, *UserSubscription) error { return nil }
func (s *userSubRepoStubForAvailable) Delete(context.Context, int64) error             { return nil }
func (s *userSubRepoStubForAvailable) Restore(context.Context, int64, string) (*UserSubscription, error) {
	return nil, nil
}
func (s *userSubRepoStubForAvailable) ListByUserID(context.Context, int64) ([]UserSubscription, error) {
	return s.subs, nil
}
func (s *userSubRepoStubForAvailable) ListActiveByUserID(context.Context, int64) ([]UserSubscription, error) {
	return s.subs, nil
}
func (s *userSubRepoStubForAvailable) ListByGroupID(context.Context, int64, pagination.PaginationParams) ([]UserSubscription, *pagination.PaginationResult, error) {
	return nil, nil, nil
}
func (s *userSubRepoStubForAvailable) List(context.Context, pagination.PaginationParams, *int64, *int64, string, string, string, string) ([]UserSubscription, *pagination.PaginationResult, error) {
	return nil, nil, nil
}
func (s *userSubRepoStubForAvailable) ExistsByUserIDAndGroupID(context.Context, int64, int64) (bool, error) {
	return false, nil
}
func (s *userSubRepoStubForAvailable) ExistsActiveByUserIDAndGroupID(context.Context, int64, int64) (bool, error) {
	return false, nil
}
func (s *userSubRepoStubForAvailable) ExtendExpiry(context.Context, int64, time.Time) error {
	return nil
}
func (s *userSubRepoStubForAvailable) UpdateStatus(context.Context, int64, string) error { return nil }
func (s *userSubRepoStubForAvailable) UpdateNotes(context.Context, int64, string) error  { return nil }
func (s *userSubRepoStubForAvailable) ActivateWindows(context.Context, int64, time.Time) error {
	return nil
}
func (s *userSubRepoStubForAvailable) ResetUsageWindows(context.Context, int64, bool, bool, bool, time.Time) error {
	return nil
}
func (s *userSubRepoStubForAvailable) ResetDailyUsage(context.Context, int64, *time.Time, time.Time) error {
	return nil
}
func (s *userSubRepoStubForAvailable) ResetWeeklyUsage(context.Context, int64, *time.Time, time.Time) error {
	return nil
}
func (s *userSubRepoStubForAvailable) ResetMonthlyUsage(context.Context, int64, *time.Time, time.Time) error {
	return nil
}
func (s *userSubRepoStubForAvailable) IncrementUsage(context.Context, int64, float64) error {
	return nil
}
func (s *userSubRepoStubForAvailable) BatchUpdateExpiredStatus(context.Context) (int64, error) {
	return 0, nil
}

func TestGetAvailableGroups_ExcludesFullListActive(t *testing.T) {
	userRepo := &userRepoStub{user: &User{ID: 1, Role: RoleUser, Status: StatusActive}}
	groupRepo := &stubGroupRepoForProvision{
		listExcl: []Group{{ID: 10, Name: "public", SubscriptionType: SubscriptionTypeStandard, Status: StatusActive}},
	}
	groupRepo.listByIDsFn = func(ids []int64) ([]Group, error) {
		out := make([]Group, 0)
		for _, id := range ids {
			if id == 20 {
				out = append(out, Group{
					ID: 20, Name: PrivateGroupName(1, PlatformAnthropic),
					SubscriptionType: SubscriptionTypeSubscription, Status: StatusActive, IsExclusive: true,
				})
			}
		}
		return out, nil
	}
	subRepo := &userSubRepoStubForAvailable{
		subs: []UserSubscription{{UserID: 1, GroupID: 20, Status: SubscriptionStatusActive}},
	}
	svc := &APIKeyService{
		userRepo:    userRepo,
		groupRepo:   groupRepo,
		userSubRepo: subRepo,
	}
	groups, err := svc.GetAvailableGroups(context.Background(), 1)
	require.NoError(t, err)
	require.Equal(t, 0, groupRepo.listActiveCalls, "must not full-scan ListActive")
	require.Equal(t, 1, groupRepo.listExclCalls)
	require.Equal(t, 1, groupRepo.listByIDsCalls)
	ids := make([]int64, 0, len(groups))
	for _, g := range groups {
		ids = append(ids, g.ID)
	}
	require.Contains(t, ids, int64(10))
	require.Contains(t, ids, int64(20))
}

func TestSyncPrivateSubscriptionExpiresAt_RequiresConfiguredDate(t *testing.T) {
	p := newTestProvisioner(&User{ID: 1, Role: RoleUser}, nil, nil)
	_, err := p.SyncPrivateSubscriptionExpiresAt(context.Background())
	require.ErrorIs(t, err, ErrPrivateGroupExpiresDateNotConfigured)
}

func TestSyncPrivateSubscriptionExpiresAt_FutureTargetActivates(t *testing.T) {
	sub := &stubSubEnsure{
		syncUpdated: 3,
		syncPairs: []privateSubUserGroup{
			{UserID: 1, GroupID: 10},
			{UserID: 2, GroupID: 11},
			{UserID: 3, GroupID: 12},
		},
	}
	groupRepo := &stubGroupRepoForProvision{byName: map[string]*Group{}}
	userRepo := &userRepoStubForProvision{userRepoStub: userRepoStub{user: &User{ID: 1, Role: RoleUser}}}
	cfg := &config.Config{}
	settings := NewSettingService(&settingRepoStub{values: map[string]string{
		SettingKeyPrivateGroupExpiresDate: "2099-06-15",
	}}, cfg)
	p := &privateGroupProvisioner{
		groupRepo:      groupRepo,
		userRepo:       userRepo,
		subEnsure:      sub,
		settingService: settings,
	}

	result, err := p.SyncPrivateSubscriptionExpiresAt(context.Background())
	require.NoError(t, err)
	require.Equal(t, int64(3), result.Updated)
	require.Equal(t, SubscriptionStatusActive, result.Status)
	require.Equal(t, 1, sub.syncCalls)
	require.Equal(t, SubscriptionStatusActive, sub.syncStatus)
	// 上海日末 23:59:59
	require.Equal(t, 23, result.ExpiresAt.Hour())
	require.Equal(t, 59, result.ExpiresAt.Minute())
	require.Equal(t, 59, result.ExpiresAt.Second())
}

func TestSyncPrivateSubscriptionExpiresAt_PastTargetExpires(t *testing.T) {
	sub := &stubSubEnsure{syncUpdated: 1}
	settings := NewSettingService(&settingRepoStub{values: map[string]string{
		SettingKeyPrivateGroupExpiresDate: "2020-01-01",
	}}, &config.Config{})
	p := &privateGroupProvisioner{
		groupRepo:      &stubGroupRepoForProvision{},
		userRepo:       &userRepoStubForProvision{userRepoStub: userRepoStub{user: &User{ID: 1, Role: RoleUser}}},
		subEnsure:      sub,
		settingService: settings,
	}
	result, err := p.SyncPrivateSubscriptionExpiresAt(context.Background())
	require.NoError(t, err)
	require.Equal(t, SubscriptionStatusExpired, result.Status)
	require.Equal(t, SubscriptionStatusExpired, sub.syncStatus)
}
