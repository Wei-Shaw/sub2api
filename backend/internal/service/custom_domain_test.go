package service

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

type customDomainRepoStub struct {
	nextID           int64
	getByDomainCalls int
	byID             map[int64]*CustomDomain
	byHost           map[string]int64
}

func newCustomDomainRepoStub() *customDomainRepoStub {
	return &customDomainRepoStub{
		nextID: 1,
		byID:   map[int64]*CustomDomain{},
		byHost: map[string]int64{},
	}
}

func (r *customDomainRepoStub) Create(ctx context.Context, domain *CustomDomain) (*CustomDomain, error) {
	if _, exists := r.byHost[domain.Domain]; exists {
		return nil, ErrCustomDomainConflict
	}
	cp := *domain
	cp.ID = r.nextID
	r.nextID++
	r.byID[cp.ID] = &cp
	r.byHost[cp.Domain] = cp.ID
	return cloneCustomDomain(&cp), nil
}

func (r *customDomainRepoStub) GetByID(ctx context.Context, id int64) (*CustomDomain, error) {
	domain, ok := r.byID[id]
	if !ok {
		return nil, ErrCustomDomainNotFound
	}
	return cloneCustomDomain(domain), nil
}

func (r *customDomainRepoStub) GetByDomain(ctx context.Context, domain string) (*CustomDomain, error) {
	r.getByDomainCalls++
	id, ok := r.byHost[normalizeCustomDomainHost(domain)]
	if !ok {
		return nil, ErrCustomDomainNotFound
	}
	return r.GetByID(ctx, id)
}

func (r *customDomainRepoStub) ListByUserID(ctx context.Context, userID int64) ([]CustomDomain, error) {
	out := []CustomDomain{}
	for _, domain := range r.byID {
		if domain.CanUse(userID) {
			out = append(out, *cloneCustomDomain(domain))
		}
	}
	return out, nil
}

func (r *customDomainRepoStub) ListAll(ctx context.Context, filters CustomDomainListFilters) ([]CustomDomain, error) {
	out := []CustomDomain{}
	for _, domain := range r.byID {
		if filters.Domain != "" && !strings.Contains(domain.Domain, filters.Domain) {
			continue
		}
		if filters.Status != "" && domain.Status != filters.Status {
			continue
		}
		if filters.UserID != nil && !domain.CanUse(*filters.UserID) {
			continue
		}
		out = append(out, *cloneCustomDomain(domain))
	}
	return out, nil
}

func (r *customDomainRepoStub) SetAccess(ctx context.Context, id int64, allUsers bool, userIDs []int64) (*CustomDomain, error) {
	domain, ok := r.byID[id]
	if !ok {
		return nil, ErrCustomDomainNotFound
	}
	cp := *domain
	cp.AllUsers = allUsers
	cp.UserIDs = append([]int64(nil), userIDs...)
	r.byID[id] = &cp
	return cloneCustomDomain(&cp), nil
}

func (r *customDomainRepoStub) Update(ctx context.Context, domain *CustomDomain) (*CustomDomain, error) {
	if _, ok := r.byID[domain.ID]; !ok {
		return nil, ErrCustomDomainNotFound
	}
	cp := *domain
	r.byID[domain.ID] = &cp
	r.byHost[domain.Domain] = domain.ID
	return cloneCustomDomain(&cp), nil
}

func (r *customDomainRepoStub) Delete(ctx context.Context, id int64) error {
	domain, ok := r.byID[id]
	if !ok {
		return ErrCustomDomainNotFound
	}
	delete(r.byHost, domain.Domain)
	delete(r.byID, id)
	return nil
}

type customDomainSettingStub struct {
	values map[string]string
}

func (s *customDomainSettingStub) Get(ctx context.Context, key string) (*Setting, error) {
	value, ok := s.values[key]
	if !ok {
		return nil, ErrSettingNotFound
	}
	return &Setting{Key: key, Value: value}, nil
}

func (s *customDomainSettingStub) GetValue(ctx context.Context, key string) (string, error) {
	value, ok := s.values[key]
	if !ok {
		return "", ErrSettingNotFound
	}
	return value, nil
}

func (s *customDomainSettingStub) Set(ctx context.Context, key, value string) error {
	s.values[key] = value
	return nil
}

func (s *customDomainSettingStub) GetMultiple(ctx context.Context, keys []string) (map[string]string, error) {
	out := map[string]string{}
	for _, key := range keys {
		if value, ok := s.values[key]; ok {
			out[key] = value
		}
	}
	return out, nil
}

func (s *customDomainSettingStub) SetMultiple(ctx context.Context, settings map[string]string) error {
	for key, value := range settings {
		s.values[key] = value
	}
	return nil
}

func (s *customDomainSettingStub) GetAll(ctx context.Context) (map[string]string, error) {
	out := map[string]string{}
	for key, value := range s.values {
		out[key] = value
	}
	return out, nil
}

func (s *customDomainSettingStub) Delete(ctx context.Context, key string) error {
	delete(s.values, key)
	return nil
}

type customDomainDNSStub struct {
	txt map[string][]string
}

func (d *customDomainDNSStub) LookupTXT(ctx context.Context, name string) ([]string, error) {
	values, ok := d.txt[name]
	if !ok {
		return nil, errors.New("not found")
	}
	return values, nil
}

func TestCustomDomainCreateNormalizesAndRejectsDuplicateClaims(t *testing.T) {
	ctx := context.Background()
	repo := newCustomDomainRepoStub()
	svc := newCustomDomainTestService(repo)

	domain, err := svc.CreateForUser(ctx, 42, "HTTPS://API.Customer.Example")
	if err != nil {
		t.Fatalf("CreateForUser returned error: %v", err)
	}
	if domain.Domain != "api.customer.example" {
		t.Fatalf("domain was not normalized: %q", domain.Domain)
	}
	if domain.Status != CustomDomainStatusPendingDNS {
		t.Fatalf("new domains should start pending_dns, got %q", domain.Status)
	}
	if domain.VerificationTXTName != "_sub2api-verify.api.customer.example" {
		t.Fatalf("unexpected TXT name: %q", domain.VerificationTXTName)
	}
	if domain.VerificationTXTValue != "sub2api-domain-verification=fixed-token" {
		t.Fatalf("unexpected TXT value: %q", domain.VerificationTXTValue)
	}
	if domain.CNAMETarget == nil || *domain.CNAMETarget != "gateway.example.com" {
		t.Fatalf("unexpected CNAME target: %#v", domain.CNAMETarget)
	}
	if !domain.CanUse(42) || domain.CanUse(7) {
		t.Fatalf("self-created domain should only authorize the owner by default: %#v", domain.UserIDs)
	}

	if _, err := svc.CreateForUser(ctx, 7, "api.customer.example"); !errors.Is(err, ErrCustomDomainConflict) {
		t.Fatalf("duplicate claim should be rejected, got %v", err)
	}
}

func TestCustomDomainAccessAllowsMultipleUsersAndAllUsers(t *testing.T) {
	ctx := context.Background()
	repo := newCustomDomainRepoStub()
	svc := newCustomDomainTestService(repo)

	domain, err := svc.CreateForUserWithAccess(ctx, 42, "api.customer.example", CustomDomainAccessInput{UserIDs: []int64{99, 42, 99}})
	if err != nil {
		t.Fatalf("CreateForUserWithAccess returned error: %v", err)
	}
	if !domain.CanUse(42) || !domain.CanUse(99) || domain.CanUse(100) {
		t.Fatalf("domain access should include owner and selected users only: %#v", domain.UserIDs)
	}

	filtered, err := svc.ListForUser(ctx, 99)
	if err != nil {
		t.Fatalf("ListForUser returned error: %v", err)
	}
	if len(filtered) != 1 || filtered[0].ID != domain.ID || filtered[0].CanManage {
		t.Fatalf("shared user should see but not manage the domain: %#v", filtered)
	}

	updated, err := svc.UpdateAccessAsAdmin(ctx, domain.ID, CustomDomainAccessInput{AllUsers: true})
	if err != nil {
		t.Fatalf("UpdateAccessAsAdmin returned error: %v", err)
	}
	if !updated.AllUsers || !updated.CanUse(12345) {
		t.Fatalf("all-users domain should authorize any positive user id: %#v", updated)
	}
}

func TestCustomDomainVerifyRecordsPendingAndActiveStates(t *testing.T) {
	ctx := context.Background()
	repo := newCustomDomainRepoStub()
	svc := newCustomDomainTestService(repo)
	created, err := svc.CreateForUser(ctx, 42, "api.customer.example")
	if err != nil {
		t.Fatalf("CreateForUser returned error: %v", err)
	}

	dns := &customDomainDNSStub{
		txt: map[string][]string{
			created.VerificationTXTName: {"wrong-value"},
		},
	}
	svc.SetDNSResolverForTest(dns)
	pendingResponse, err := svc.VerifyForUser(ctx, 42, created.ID)
	if err == nil {
		t.Fatalf("VerifyForUser should fail when TXT is missing expected value")
	}
	if pendingResponse == nil || !pendingResponse.CanManage {
		t.Fatalf("owner verification response should remain manageable while pending: %#v", pendingResponse)
	}
	pending, err := repo.GetByID(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetByID returned error: %v", err)
	}
	if pending.Status != CustomDomainStatusPendingDNS || pending.LastError == nil || pending.LastCheckedAt == nil {
		t.Fatalf("failed verification should persist pending status and error: %#v", pending)
	}

	dns.txt[created.VerificationTXTName] = []string{created.VerificationTXTValue}
	active, err := svc.VerifyForUser(ctx, 42, created.ID)
	if err != nil {
		t.Fatalf("VerifyForUser returned error with correct TXT ownership record: %v", err)
	}
	if active.Status != CustomDomainStatusActive || active.VerifiedAt == nil || active.LastError != nil {
		t.Fatalf("successful verification should activate domain: %#v", active)
	}
	if !active.CanManage {
		t.Fatalf("owner verification response should remain manageable after activation: %#v", active)
	}
}

func TestCustomDomainResolveRequestHost(t *testing.T) {
	ctx := context.Background()
	repo := newCustomDomainRepoStub()
	svc := newCustomDomainTestService(repo)
	active, err := svc.CreateForUser(ctx, 42, "api.customer.example")
	if err != nil {
		t.Fatalf("CreateForUser returned error: %v", err)
	}
	now := time.Date(2026, 7, 7, 0, 0, 0, 0, time.UTC)
	active.Status = CustomDomainStatusActive
	active.VerifiedAt = &now
	if _, err := repo.Update(ctx, active); err != nil {
		t.Fatalf("Update returned error: %v", err)
	}
	pending, err := svc.CreateForUser(ctx, 42, "pending.customer.example")
	if err != nil {
		t.Fatalf("CreateForUser returned error: %v", err)
	}

	if domain, matched, err := svc.ResolveRequestHost(ctx, "api.customer.example:443"); err != nil || !matched || domain.ID != active.ID {
		t.Fatalf("active custom domain should match, domain=%#v matched=%v err=%v", domain, matched, err)
	}
	if domain, matched, err := svc.ResolveRequestHost(ctx, "pending.customer.example"); !errors.Is(err, ErrCustomDomainInactive) || !matched || domain.ID != pending.ID {
		t.Fatalf("inactive custom domain should reject, domain=%#v matched=%v err=%v", domain, matched, err)
	}
	if domain, matched, err := svc.ResolveRequestHost(ctx, "gateway.example.com"); err != nil || matched || domain != nil {
		t.Fatalf("gateway target should fall through, domain=%#v matched=%v err=%v", domain, matched, err)
	}
	if domain, matched, err := svc.ResolveRequestHost(ctx, "unknown.example.com"); err != nil || matched || domain != nil {
		t.Fatalf("unknown host should fall through, domain=%#v matched=%v err=%v", domain, matched, err)
	}
}

func TestCustomDomainResolveRequestHostCachesDomainLookupAndInvalidatesAccessChanges(t *testing.T) {
	ctx := context.Background()
	repo := newCustomDomainRepoStub()
	svc := newCustomDomainTestService(repo)
	active, err := svc.CreateForUser(ctx, 42, "api.customer.example")
	if err != nil {
		t.Fatalf("CreateForUser returned error: %v", err)
	}
	now := time.Date(2026, 7, 7, 0, 0, 0, 0, time.UTC)
	active.Status = CustomDomainStatusActive
	active.VerifiedAt = &now
	if _, err := repo.Update(ctx, active); err != nil {
		t.Fatalf("Update returned error: %v", err)
	}

	first, matched, err := svc.ResolveRequestHost(ctx, "api.customer.example")
	if err != nil || !matched || first == nil {
		t.Fatalf("first resolve failed, domain=%#v matched=%v err=%v", first, matched, err)
	}
	second, matched, err := svc.ResolveRequestHost(ctx, "api.customer.example")
	if err != nil || !matched || second == nil {
		t.Fatalf("second resolve failed, domain=%#v matched=%v err=%v", second, matched, err)
	}
	if repo.getByDomainCalls != 1 {
		t.Fatalf("expected cached second resolve to reuse domain lookup, got %d calls", repo.getByDomainCalls)
	}
	if second.CanUse(99) {
		t.Fatalf("user 99 should not be authorized before access update: %#v", second.UserIDs)
	}

	if _, err := svc.UpdateAccessAsAdmin(ctx, active.ID, CustomDomainAccessInput{UserIDs: []int64{99}}); err != nil {
		t.Fatalf("UpdateAccessAsAdmin returned error: %v", err)
	}
	updated, matched, err := svc.ResolveRequestHost(ctx, "api.customer.example")
	if err != nil || !matched || updated == nil {
		t.Fatalf("resolve after access update failed, domain=%#v matched=%v err=%v", updated, matched, err)
	}
	if !updated.CanUse(99) {
		t.Fatalf("cache was not invalidated after access update: %#v", updated.UserIDs)
	}
	if repo.getByDomainCalls != 2 {
		t.Fatalf("expected resolve after access change to reload domain, got %d calls", repo.getByDomainCalls)
	}
}

func TestCustomDomainResolveRequestHostFailsClosedWhenFeatureDisabled(t *testing.T) {
	ctx := context.Background()
	repo := newCustomDomainRepoStub()
	svc := newCustomDomainTestService(repo)
	active, err := svc.CreateForUser(ctx, 42, "api.customer.example")
	if err != nil {
		t.Fatalf("CreateForUser returned error: %v", err)
	}
	now := time.Date(2026, 7, 7, 0, 0, 0, 0, time.UTC)
	active.Status = CustomDomainStatusActive
	active.VerifiedAt = &now
	if _, err := repo.Update(ctx, active); err != nil {
		t.Fatalf("Update returned error: %v", err)
	}
	if err := svc.SetEnabled(ctx, false); err != nil {
		t.Fatalf("SetEnabled returned error: %v", err)
	}

	if domain, matched, err := svc.ResolveRequestHost(ctx, "api.customer.example"); !errors.Is(err, ErrCustomDomainsDisabled) || !matched || domain.ID != active.ID {
		t.Fatalf("disabled known custom domain should fail closed, domain=%#v matched=%v err=%v", domain, matched, err)
	}
	if domain, matched, err := svc.ResolveRequestHost(ctx, "gateway.example.com"); err != nil || matched || domain != nil {
		t.Fatalf("gateway target should still fall through when custom domains are disabled, domain=%#v matched=%v err=%v", domain, matched, err)
	}
}

func newCustomDomainTestService(repo *customDomainRepoStub) *CustomDomainService {
	setting := &customDomainSettingStub{values: map[string]string{
		SettingKeyCustomDomainsEnabled: "true",
		SettingKeyAPIBaseURL:           "https://gateway.example.com",
	}}
	svc := NewCustomDomainService(repo, setting)
	svc.tokenFunc = func() (string, error) { return "fixed-token", nil }
	svc.SetNowForTest(func() time.Time {
		return time.Date(2026, 7, 7, 12, 0, 0, 0, time.UTC)
	})
	return svc
}

func cloneCustomDomain(domain *CustomDomain) *CustomDomain {
	if domain == nil {
		return nil
	}
	cp := *domain
	cp.UserIDs = append([]int64(nil), domain.UserIDs...)
	cp.Users = append([]User(nil), domain.Users...)
	return &cp
}
