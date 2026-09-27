package service

import (
	"context"
	"errors"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
)

// --- stubs ---

type studentVerificationRepoStub struct {
	mu     sync.Mutex
	nextID int64
	byID   map[int64]*StudentVerification
}

func newStudentVerificationRepoStub() *studentVerificationRepoStub {
	return &studentVerificationRepoStub{nextID: 1, byID: map[int64]*StudentVerification{}}
}

func cloneStudentVerification(v *StudentVerification) *StudentVerification {
	if v == nil {
		return nil
	}
	out := *v
	out.GrantedGroupIDs = append([]int64(nil), v.GrantedGroupIDs...)
	return &out
}

func (r *studentVerificationRepoStub) GetByID(_ context.Context, id int64) (*StudentVerification, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if v, ok := r.byID[id]; ok {
		return cloneStudentVerification(v), nil
	}
	return nil, ErrStudentVerificationNotFound
}

func (r *studentVerificationRepoStub) GetByUserID(_ context.Context, userID int64) (*StudentVerification, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, v := range r.byID {
		if v.UserID == userID {
			return cloneStudentVerification(v), nil
		}
	}
	return nil, ErrStudentVerificationNotFound
}

func (r *studentVerificationRepoStub) GetByEmail(_ context.Context, normalizedEmail string) (*StudentVerification, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, v := range r.byID {
		if v.Email == normalizedEmail {
			return cloneStudentVerification(v), nil
		}
	}
	return nil, ErrStudentVerificationNotFound
}

func (r *studentVerificationRepoStub) UpsertActive(_ context.Context, v *StudentVerification) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, existing := range r.byID {
		if existing.Email == v.Email && existing.UserID != v.UserID {
			return ErrStudentEmailClaimed
		}
		if existing.UserID == v.UserID {
			v.ID = existing.ID
			r.byID[existing.ID] = cloneStudentVerification(v)
			return nil
		}
	}
	v.ID = r.nextID
	r.nextID++
	r.byID[v.ID] = cloneStudentVerification(v)
	return nil
}

func (r *studentVerificationRepoStub) UpdateStatus(_ context.Context, id int64, status string, revokedBy *int64, reason *string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	v, ok := r.byID[id]
	if !ok {
		return ErrStudentVerificationNotFound
	}
	v.Status = status
	if status == StudentVerificationStatusRevoked {
		now := time.Now().UTC()
		v.RevokedAt = &now
		v.RevokedBy = revokedBy
		v.RevokeReason = reason
	}
	return nil
}

func (r *studentVerificationRepoStub) ListActiveExpired(_ context.Context, now time.Time, limit int) ([]StudentVerification, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []StudentVerification
	for _, v := range r.byID {
		if v.Status == StudentVerificationStatusActive && !v.ExpiresAt.After(now) {
			out = append(out, *cloneStudentVerification(v))
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ExpiresAt.Before(out[j].ExpiresAt) })
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (r *studentVerificationRepoStub) List(_ context.Context, params pagination.PaginationParams, status, keyword string) ([]StudentVerification, *pagination.PaginationResult, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []StudentVerification
	for _, v := range r.byID {
		if status != "" && v.Status != status {
			continue
		}
		out = append(out, *cloneStudentVerification(v))
	}
	return out, &pagination.PaginationResult{Total: int64(len(out)), Page: params.Page, PageSize: params.Limit()}, nil
}

func (r *studentVerificationRepoStub) Delete(_ context.Context, id int64) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.byID[id]; !ok {
		return ErrStudentVerificationNotFound
	}
	delete(r.byID, id)
	return nil
}

func (r *studentVerificationRepoStub) get(id int64) *StudentVerification {
	r.mu.Lock()
	defer r.mu.Unlock()
	return cloneStudentVerification(r.byID[id])
}

type studentVerificationGroupStub struct {
	mu      sync.Mutex
	granted map[int64]map[int64]bool // userID -> groupID
	groups  map[int64]*Group
	removed [][2]int64
}

func newStudentVerificationGroupStub() *studentVerificationGroupStub {
	return &studentVerificationGroupStub{granted: map[int64]map[int64]bool{}, groups: map[int64]*Group{}}
}

func (g *studentVerificationGroupStub) addGroup(id int64, exclusive bool, subscriptionType, status string) {
	g.groups[id] = &Group{ID: id, IsExclusive: exclusive, SubscriptionType: subscriptionType, Status: status}
}

func (g *studentVerificationGroupStub) GetByID(_ context.Context, id int64) (*Group, error) {
	if group, ok := g.groups[id]; ok {
		return group, nil
	}
	return nil, ErrGroupNotFound
}

func (g *studentVerificationGroupStub) AddGroupToAllowedGroups(_ context.Context, userID int64, groupID int64) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.granted[userID] == nil {
		g.granted[userID] = map[int64]bool{}
	}
	g.granted[userID][groupID] = true
	return nil
}

func (g *studentVerificationGroupStub) RemoveGroupFromUserAllowedGroups(_ context.Context, userID int64, groupID int64) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	delete(g.granted[userID], groupID)
	g.removed = append(g.removed, [2]int64{userID, groupID})
	return nil
}

func (g *studentVerificationGroupStub) has(userID, groupID int64) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.granted[userID][groupID]
}

type studentVerificationAffiliateStub struct {
	mu    sync.Mutex
	rates map[int64]*float64
}

func newStudentVerificationAffiliateStub() *studentVerificationAffiliateStub {
	return &studentVerificationAffiliateStub{rates: map[int64]*float64{}}
}

func (a *studentVerificationAffiliateStub) EnsureUserAffiliate(_ context.Context, userID int64) (*AffiliateSummary, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	return &AffiliateSummary{UserID: userID, AffRebateRatePercent: cloneFloat64Ptr(a.rates[userID])}, nil
}

func (a *studentVerificationAffiliateStub) SetUserRebateRate(_ context.Context, userID int64, ratePercent *float64) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.rates[userID] = cloneFloat64Ptr(ratePercent)
	return nil
}

func (a *studentVerificationAffiliateStub) rate(userID int64) *float64 {
	a.mu.Lock()
	defer a.mu.Unlock()
	return cloneFloat64Ptr(a.rates[userID])
}

func cloneFloat64Ptr(v *float64) *float64 {
	if v == nil {
		return nil
	}
	out := *v
	return &out
}

type studentVerificationEmailStub struct {
	mu         sync.Mutex
	validCode  string
	sentTo     []string
	verifyFail error
}

func (e *studentVerificationEmailStub) SendVerifyCode(_ context.Context, email, _ string, _ ...string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.sentTo = append(e.sentTo, email)
	return nil
}

func (e *studentVerificationEmailStub) VerifyCode(_ context.Context, _ string, code string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.verifyFail != nil {
		return e.verifyFail
	}
	if code != e.validCode {
		return ErrInvalidVerifyCode
	}
	return nil
}

type studentVerificationCacheStub struct {
	mu          sync.Mutex
	invalidated []int64
}

func (c *studentVerificationCacheStub) InvalidateAuthCacheByKey(context.Context, string)    {}
func (c *studentVerificationCacheStub) InvalidateAuthCacheByGroupID(context.Context, int64) {}

func (c *studentVerificationCacheStub) InvalidateAuthCacheByUserID(_ context.Context, userID int64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.invalidated = append(c.invalidated, userID)
}

func (c *studentVerificationCacheStub) calls() []int64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]int64(nil), c.invalidated...)
}

type studentVerificationSettingRepoStub struct {
	mu     sync.Mutex
	values map[string]string
}

func newStudentVerificationSettingRepoStub() *studentVerificationSettingRepoStub {
	return &studentVerificationSettingRepoStub{values: map[string]string{}}
}

func (r *studentVerificationSettingRepoStub) Get(_ context.Context, key string) (*Setting, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if v, ok := r.values[key]; ok {
		return &Setting{Key: key, Value: v}, nil
	}
	return nil, ErrSettingNotFound
}

func (r *studentVerificationSettingRepoStub) GetValue(_ context.Context, key string) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if v, ok := r.values[key]; ok {
		return v, nil
	}
	return "", ErrSettingNotFound
}

func (r *studentVerificationSettingRepoStub) Set(_ context.Context, key, value string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.values[key] = value
	return nil
}

func (r *studentVerificationSettingRepoStub) GetMultiple(_ context.Context, keys []string) (map[string]string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := map[string]string{}
	for _, k := range keys {
		if v, ok := r.values[k]; ok {
			out[k] = v
		}
	}
	return out, nil
}

func (r *studentVerificationSettingRepoStub) SetMultiple(_ context.Context, settings map[string]string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for k, v := range settings {
		r.values[k] = v
	}
	return nil
}

func (r *studentVerificationSettingRepoStub) GetAll(_ context.Context) (map[string]string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := map[string]string{}
	for k, v := range r.values {
		out[k] = v
	}
	return out, nil
}

func (r *studentVerificationSettingRepoStub) Delete(_ context.Context, key string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.values, key)
	return nil
}

// --- fixture ---

type studentVerificationFixture struct {
	svc     *StudentVerificationService
	repo    *studentVerificationRepoStub
	groups  *studentVerificationGroupStub
	aff     *studentVerificationAffiliateStub
	email   *studentVerificationEmailStub
	cache   *studentVerificationCacheStub
	setting *studentVerificationSettingRepoStub
}

func newStudentVerificationFixture(t *testing.T) *studentVerificationFixture {
	t.Helper()
	repo := newStudentVerificationRepoStub()
	groups := newStudentVerificationGroupStub()
	aff := newStudentVerificationAffiliateStub()
	email := &studentVerificationEmailStub{validCode: "123456"}
	cache := &studentVerificationCacheStub{}
	settingRepo := newStudentVerificationSettingRepoStub()

	settingRepo.values[SettingKeyStudentVerificationEnabled] = "true"
	settingRepo.values[SettingKeyStudentVerificationEmailSuffixes] = `["*.example.edu"]`
	settingRepo.values[SettingKeyStudentVerificationValidityDays] = "365"
	settingRepo.values[SettingKeyStudentVerificationGroupIDs] = "[10,11]"
	settingRepo.values[SettingKeyStudentVerificationRebateRate] = "10"

	groups.addGroup(10, true, "", StatusActive)
	groups.addGroup(11, true, "", StatusActive)

	settingService := NewSettingService(settingRepo, &config.Config{})
	svc := NewStudentVerificationService(repo, groups, groups, aff, settingService, email, nil, cache)
	return &studentVerificationFixture{
		svc: svc, repo: repo, groups: groups, aff: aff, email: email, cache: cache, setting: settingRepo,
	}
}

func (f *studentVerificationFixture) seedRecord(v *StudentVerification) {
	f.repo.mu.Lock()
	defer f.repo.mu.Unlock()
	if v.ID == 0 {
		v.ID = f.repo.nextID
		f.repo.nextID++
	}
	f.repo.byID[v.ID] = cloneStudentVerification(v)
}

// --- tests ---

func TestStudentVerification_SendCode_Disabled(t *testing.T) {
	f := newStudentVerificationFixture(t)
	f.setting.values[SettingKeyStudentVerificationEnabled] = "false"
	err := f.svc.SendCode(context.Background(), 1, "a@stu.example.edu")
	if !errors.Is(err, ErrStudentVerificationDisabled) {
		t.Fatalf("expected disabled error, got %v", err)
	}
}

func TestStudentVerification_SendCode_DomainNotAllowed(t *testing.T) {
	f := newStudentVerificationFixture(t)
	err := f.svc.SendCode(context.Background(), 1, "a@gmail.com")
	if !errors.Is(err, ErrStudentEmailDomainNotAllowed) {
		t.Fatalf("expected domain-not-allowed error, got %v", err)
	}
	if len(f.email.sentTo) != 0 {
		t.Fatalf("no code should be sent for a rejected domain")
	}
}

func TestStudentVerification_SendCode_EmailClaimedByOtherUser(t *testing.T) {
	f := newStudentVerificationFixture(t)
	f.seedRecord(&StudentVerification{
		UserID: 99, Email: "taken@stu.example.edu", EmailRaw: "taken@stu.example.edu",
		Status: StudentVerificationStatusActive,
	})
	err := f.svc.SendCode(context.Background(), 1, "taken@stu.example.edu")
	if !errors.Is(err, ErrStudentEmailClaimed) {
		t.Fatalf("expected claimed error, got %v", err)
	}
}

func TestStudentVerification_SendCode_AliasVariantClaimed(t *testing.T) {
	f := newStudentVerificationFixture(t)
	f.seedRecord(&StudentVerification{
		UserID: 99, Email: "taken@stu.example.edu", EmailRaw: "taken@stu.example.edu",
		Status: StudentVerificationStatusActive,
	})
	// +alias normalizes to the same inbox identity — must still conflict.
	err := f.svc.SendCode(context.Background(), 1, "taken+alt@stu.example.edu")
	if !errors.Is(err, ErrStudentEmailClaimed) {
		t.Fatalf("expected claimed error for alias variant, got %v", err)
	}
}

func TestStudentVerification_SendCode_Success(t *testing.T) {
	f := newStudentVerificationFixture(t)
	if err := f.svc.SendCode(context.Background(), 1, "new@stu.example.edu"); err != nil {
		t.Fatalf("SendCode failed: %v", err)
	}
	if len(f.email.sentTo) != 1 || f.email.sentTo[0] != "new@stu.example.edu" {
		t.Fatalf("expected code sent to raw email, got %v", f.email.sentTo)
	}
}

func TestStudentVerification_Verify_SuccessGrantsGroupsAndRebate(t *testing.T) {
	f := newStudentVerificationFixture(t)
	v, err := f.svc.Verify(context.Background(), 7, "new@stu.example.edu", "123456")
	if err != nil {
		t.Fatalf("Verify failed: %v", err)
	}
	if v.Status != StudentVerificationStatusActive {
		t.Fatalf("expected active status, got %q", v.Status)
	}
	if !f.groups.has(7, 10) || !f.groups.has(7, 11) {
		t.Fatalf("expected both student groups granted, got %+v", f.groups.granted[7])
	}
	if rate := f.aff.rate(7); rate == nil || *rate != 10 {
		t.Fatalf("expected student rebate rate 10, got %v", rate)
	}
	if len(f.cache.calls()) != 1 || f.cache.calls()[0] != 7 {
		t.Fatalf("expected auth cache invalidation for user 7, got %v", f.cache.calls())
	}
	rec, err := f.repo.GetByUserID(context.Background(), 7)
	if err != nil {
		t.Fatalf("record not persisted: %v", err)
	}
	if rec.Email != "new@stu.example.edu" || len(rec.GrantedGroupIDs) != 2 {
		t.Fatalf("unexpected stored record: %+v", rec)
	}
}

func TestStudentVerification_Verify_InvalidCode(t *testing.T) {
	f := newStudentVerificationFixture(t)
	_, err := f.svc.Verify(context.Background(), 7, "new@stu.example.edu", "000000")
	if !errors.Is(err, ErrInvalidVerifyCode) {
		t.Fatalf("expected invalid code error, got %v", err)
	}
	if f.groups.has(7, 10) {
		t.Fatal("no groups should be granted on code failure")
	}
}

func TestStudentVerification_Verify_EmailClaimedConcurrently(t *testing.T) {
	f := newStudentVerificationFixture(t)
	f.email.validCode = "123456"
	// A second user verifies the same email first.
	if _, err := f.svc.Verify(context.Background(), 2, "shared@stu.example.edu", "123456"); err != nil {
		t.Fatalf("first verify failed: %v", err)
	}
	// The losing user's verify must hit the uniqueness guard.
	_, err := f.svc.Verify(context.Background(), 1, "shared@stu.example.edu", "123456")
	if !errors.Is(err, ErrStudentEmailClaimed) {
		t.Fatalf("expected claimed error for concurrent claim, got %v", err)
	}
	if f.groups.has(1, 10) || f.groups.has(1, 11) {
		t.Fatal("loser must not receive groups")
	}
}

func TestStudentVerification_Verify_RenewalSameUser(t *testing.T) {
	f := newStudentVerificationFixture(t)
	old := &StudentVerification{
		UserID: 5, Email: "old@stu.example.edu", EmailRaw: "old@stu.example.edu",
		Status: StudentVerificationStatusExpired, VerifiedAt: time.Now().AddDate(-1, 0, 0),
		ExpiresAt: time.Now().Add(-time.Hour), GrantedGroupIDs: []int64{10},
	}
	f.seedRecord(old)
	v, err := f.svc.Verify(context.Background(), 5, "old@stu.example.edu", "123456")
	if err != nil {
		t.Fatalf("renewal failed: %v", err)
	}
	if v.ID != old.ID {
		t.Fatalf("renewal should reuse the same record, got id %d want %d", v.ID, old.ID)
	}
	if v.Status != StudentVerificationStatusActive || time.Until(v.ExpiresAt) < 360*24*time.Hour {
		t.Fatalf("expected renewed active record, got %+v", v)
	}
}

func TestStudentVerification_Verify_ChangeEmailReleasesOld(t *testing.T) {
	f := newStudentVerificationFixture(t)
	old := &StudentVerification{
		UserID: 5, Email: "old@stu.example.edu", EmailRaw: "old@stu.example.edu",
		Status: StudentVerificationStatusExpired, VerifiedAt: time.Now().AddDate(-1, 0, 0),
		ExpiresAt: time.Now().Add(-time.Hour), GrantedGroupIDs: []int64{10},
	}
	f.seedRecord(old)
	v, err := f.svc.Verify(context.Background(), 5, "new@stu.example.edu", "123456")
	if err != nil {
		t.Fatalf("email change failed: %v", err)
	}
	if v.Email != "new@stu.example.edu" {
		t.Fatalf("expected new email stored, got %q", v.Email)
	}
	// Old inbox is free for a different user now.
	if _, err := f.svc.Verify(context.Background(), 9, "old@stu.example.edu", "123456"); err != nil {
		t.Fatalf("old email should be claimable after replacement, got %v", err)
	}
}

func TestStudentVerification_Verify_RevokedUserBlocked(t *testing.T) {
	f := newStudentVerificationFixture(t)
	f.seedRecord(&StudentVerification{
		UserID: 5, Email: "old@stu.example.edu", EmailRaw: "old@stu.example.edu",
		Status: StudentVerificationStatusRevoked,
	})
	if err := f.svc.SendCode(context.Background(), 5, "new@stu.example.edu"); !errors.Is(err, ErrStudentVerificationRevoked) {
		t.Fatalf("SendCode expected revoked error, got %v", err)
	}
	if _, err := f.svc.Verify(context.Background(), 5, "new@stu.example.edu", "123456"); !errors.Is(err, ErrStudentVerificationRevoked) {
		t.Fatalf("Verify expected revoked error, got %v", err)
	}
}

func TestStudentVerification_SweepExpired_RemovesEntitlements(t *testing.T) {
	f := newStudentVerificationFixture(t)
	applied := 10.0
	previous := 3.0
	v := &StudentVerification{
		UserID: 8, Email: "gone@stu.example.edu", EmailRaw: "gone@stu.example.edu",
		Status: StudentVerificationStatusActive, VerifiedAt: time.Now().AddDate(-1, 0, 0),
		ExpiresAt: time.Now().Add(-time.Hour), GrantedGroupIDs: []int64{10, 11},
		RebateRateApplied: &applied, PreviousRebateRate: &previous,
	}
	f.seedRecord(v)
	f.groups.granted[8] = map[int64]bool{10: true, 11: true, 42: true} // 42 unrelated
	f.aff.rates[8] = &applied

	n, err := f.svc.SweepExpired(context.Background(), 10)
	if err != nil {
		t.Fatalf("SweepExpired failed: %v", err)
	}
	if n != 1 {
		t.Fatalf("expected 1 record processed, got %d", n)
	}
	rec := f.repo.get(v.ID)
	if rec.Status != StudentVerificationStatusExpired {
		t.Fatalf("expected expired status, got %q", rec.Status)
	}
	if f.groups.has(8, 10) || f.groups.has(8, 11) {
		t.Fatal("student groups should be removed")
	}
	if !f.groups.has(8, 42) {
		t.Fatal("unrelated group must be preserved")
	}
	if rate := f.aff.rate(8); rate == nil || *rate != 3 {
		t.Fatalf("expected previous rebate 3 restored, got %v", rate)
	}
}

func TestStudentVerification_SweepExpired_PreservesAdminChangedRate(t *testing.T) {
	f := newStudentVerificationFixture(t)
	applied := 10.0
	adminSet := 99.0
	v := &StudentVerification{
		UserID: 8, Email: "x@stu.example.edu", EmailRaw: "x@stu.example.edu",
		Status: StudentVerificationStatusActive, ExpiresAt: time.Now().Add(-time.Hour),
		GrantedGroupIDs: []int64{10}, RebateRateApplied: &applied,
	}
	f.seedRecord(v)
	f.groups.granted[8] = map[int64]bool{10: true}
	f.aff.rates[8] = &adminSet // admin overrode the student rate in the meantime

	if _, err := f.svc.SweepExpired(context.Background(), 10); err != nil {
		t.Fatalf("SweepExpired failed: %v", err)
	}
	if rate := f.aff.rate(8); rate == nil || *rate != 99 {
		t.Fatalf("admin-set rate must be preserved, got %v", rate)
	}
}

func TestStudentVerification_AdminRevoke(t *testing.T) {
	f := newStudentVerificationFixture(t)
	applied := 10.0
	v := &StudentVerification{
		UserID: 8, Email: "y@stu.example.edu", EmailRaw: "y@stu.example.edu",
		Status: StudentVerificationStatusActive, ExpiresAt: time.Now().AddDate(0, 0, 30),
		GrantedGroupIDs: []int64{10}, RebateRateApplied: &applied,
	}
	f.seedRecord(v)
	f.groups.granted[8] = map[int64]bool{10: true}
	f.aff.rates[8] = &applied

	if err := f.svc.AdminRevoke(context.Background(), v.ID, 1, "abuse"); err != nil {
		t.Fatalf("AdminRevoke failed: %v", err)
	}
	rec := f.repo.get(v.ID)
	if rec.Status != StudentVerificationStatusRevoked || rec.RevokedBy == nil || *rec.RevokedBy != 1 {
		t.Fatalf("unexpected record after revoke: %+v", rec)
	}
	if f.groups.has(8, 10) {
		t.Fatal("student group should be removed")
	}
	if rate := f.aff.rate(8); rate != nil {
		t.Fatalf("expected rebate cleared (previous was nil), got %v", rate)
	}
	// Email claim is retained — another user still cannot claim it.
	if err := f.svc.SendCode(context.Background(), 3, "y@stu.example.edu"); !errors.Is(err, ErrStudentEmailClaimed) {
		t.Fatalf("revoked record must keep the email claimed, got %v", err)
	}
}

func TestStudentVerification_AdminDeleteReleasesEmail(t *testing.T) {
	f := newStudentVerificationFixture(t)
	v := &StudentVerification{
		UserID: 8, Email: "z@stu.example.edu", EmailRaw: "z@stu.example.edu",
		Status: StudentVerificationStatusRevoked, GrantedGroupIDs: []int64{10},
	}
	f.seedRecord(v)
	if err := f.svc.AdminDelete(context.Background(), v.ID); err != nil {
		t.Fatalf("AdminDelete failed: %v", err)
	}
	// Another user can now claim the released email.
	if _, err := f.svc.Verify(context.Background(), 3, "z@stu.example.edu", "123456"); err != nil {
		t.Fatalf("released email should be claimable, got %v", err)
	}
}

func TestStudentVerification_GroupFiltering(t *testing.T) {
	f := newStudentVerificationFixture(t)
	// Reconfigure: 10 exclusive standard, 20 subscription type, 30 deleted, 40 missing.
	f.groups.addGroup(20, true, SubscriptionTypeSubscription, StatusActive)
	f.groups.addGroup(30, true, "", "deleted")
	f.setting.values[SettingKeyStudentVerificationGroupIDs] = "[10,20,30,40]"
	v, err := f.svc.Verify(context.Background(), 1, "a@stu.example.edu", "123456")
	if err != nil {
		t.Fatalf("Verify failed: %v", err)
	}
	if len(v.GrantedGroupIDs) != 1 || v.GrantedGroupIDs[0] != 10 {
		t.Fatalf("only exclusive-standard group 10 should be granted, got %v", v.GrantedGroupIDs)
	}
}

func TestStudentVerification_GetStatusShowsExpiredBeforeSweep(t *testing.T) {
	f := newStudentVerificationFixture(t)
	f.seedRecord(&StudentVerification{
		UserID: 4, Email: "e@stu.example.edu", EmailRaw: "e@stu.example.edu",
		Status: StudentVerificationStatusActive, ExpiresAt: time.Now().Add(-time.Hour),
	})
	res, err := f.svc.GetStatus(context.Background(), 4)
	if err != nil {
		t.Fatalf("GetStatus failed: %v", err)
	}
	if res.Verification == nil || res.Verification.Status != StudentVerificationStatusExpired {
		t.Fatalf("expected expired status surfaced, got %+v", res.Verification)
	}
	if !res.Enabled || res.GroupCount != 2 || res.RebateRate != 10 {
		t.Fatalf("unexpected config projection: %+v", res)
	}
}
