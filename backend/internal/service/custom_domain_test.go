//go:build unit

package service

import (
	"context"
	"errors"
	"testing"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/stretchr/testify/require"
)

type customDomainRepoStub struct {
	domains          map[int64]*CustomDomain
	nextID           int64
	getByDomainCalls int
	lastFilters      CustomDomainListFilters
}

func newCustomDomainRepoStub() *customDomainRepoStub {
	return &customDomainRepoStub{domains: make(map[int64]*CustomDomain), nextID: 1}
}

func (r *customDomainRepoStub) Create(_ context.Context, domain *CustomDomain) (*CustomDomain, error) {
	for _, existing := range r.domains {
		if existing.Domain == domain.Domain {
			return nil, ErrCustomDomainConflict
		}
	}
	domain.ID = r.nextID
	r.nextID++
	copy := *domain
	r.domains[copy.ID] = &copy
	return &copy, nil
}

func (r *customDomainRepoStub) GetByID(_ context.Context, id int64) (*CustomDomain, error) {
	domain, ok := r.domains[id]
	if !ok {
		return nil, ErrCustomDomainNotFound
	}
	copy := *domain
	return &copy, nil
}

func (r *customDomainRepoStub) GetByDomain(_ context.Context, name string) (*CustomDomain, error) {
	r.getByDomainCalls++
	name = normalizeCustomDomainHost(name)
	for _, domain := range r.domains {
		if domain.Domain == name {
			copy := *domain
			return &copy, nil
		}
	}
	return nil, ErrCustomDomainNotFound
}

func TestCustomDomainServiceResolveCacheCopiesAndCachesMisses(t *testing.T) {
	ctx := context.Background()
	repo := newCustomDomainRepoStub()
	settings := newMockSettingRepo()
	require.NoError(t, settings.Set(ctx, SettingKeyCustomDomainsEnabled, "true"))
	require.NoError(t, settings.Set(ctx, SettingKeyAPIBaseURL, "https://gateway.example.com"))
	repo.domains[1] = &CustomDomain{
		ID:                1,
		UserID:            41,
		Domain:            "api.example.com",
		Status:            CustomDomainStatusActive,
		User:              &User{ID: 41},
		AuthorizedUserIDs: []int64{42},
		AuthorizedUsers:   []User{{ID: 42}},
	}
	svc := NewCustomDomainService(repo, settings, nil)

	first, matched, err := svc.ResolveRequestHost(ctx, "api.example.com")
	require.NoError(t, err)
	require.True(t, matched)
	first.AuthorizedUserIDs[0] = 99
	first.User.ID = 99
	first.AuthorizedUsers[0].ID = 99
	second, matched, err := svc.ResolveRequestHost(ctx, "api.example.com")
	require.NoError(t, err)
	require.True(t, matched)
	require.Equal(t, []int64{42}, second.AuthorizedUserIDs)
	require.Equal(t, int64(41), second.User.ID)
	require.Equal(t, int64(42), second.AuthorizedUsers[0].ID)

	for range 2 {
		domain, found, resolveErr := svc.ResolveRequestHost(ctx, "unknown.example.com")
		require.NoError(t, resolveErr)
		require.False(t, found)
		require.Nil(t, domain)
	}
	require.Equal(t, 2, repo.getByDomainCalls, "one hit and one miss should each query the repository once")
}

func TestCustomDomainServiceResolveCacheUsesServiceClockForExpiry(t *testing.T) {
	ctx := context.Background()
	repo := newCustomDomainRepoStub()
	settings := newMockSettingRepo()
	require.NoError(t, settings.Set(ctx, SettingKeyCustomDomainsEnabled, "true"))
	require.NoError(t, settings.Set(ctx, SettingKeyAPIBaseURL, "https://gateway.example.com"))
	repo.domains[1] = &CustomDomain{ID: 1, UserID: 41, Domain: "api.example.com", Status: CustomDomainStatusActive}
	svc := NewCustomDomainService(repo, settings, nil)
	now := time.Date(2026, 7, 25, 3, 0, 0, 0, time.UTC)
	svc.now = func() time.Time { return now }
	svc.resolveCacheTTL = time.Minute

	_, matched, err := svc.ResolveRequestHost(ctx, "api.example.com")
	require.NoError(t, err)
	require.True(t, matched)
	require.Equal(t, 1, repo.getByDomainCalls)

	now = now.Add(59 * time.Second)
	_, matched, err = svc.ResolveRequestHost(ctx, "api.example.com")
	require.NoError(t, err)
	require.True(t, matched)
	require.Equal(t, 1, repo.getByDomainCalls)

	now = now.Add(time.Second)
	_, matched, err = svc.ResolveRequestHost(ctx, "api.example.com")
	require.NoError(t, err)
	require.True(t, matched)
	require.Equal(t, 2, repo.getByDomainCalls)
}

func TestCustomDomainServiceGetResolveCacheReturnsDeepCopy(t *testing.T) {
	svc := NewCustomDomainService(newCustomDomainRepoStub(), newMockSettingRepo(), nil)
	svc.storeResolveCache("api.example.com", &CustomDomain{
		ID:                1,
		User:              &User{ID: 41},
		AuthorizedUserIDs: []int64{42},
		AuthorizedUsers:   []User{{ID: 42}},
	}, true)

	first, ok := svc.getResolveCache("api.example.com")
	require.True(t, ok)
	first.domain.User.ID = 99
	first.domain.AuthorizedUserIDs[0] = 99
	first.domain.AuthorizedUsers[0].ID = 99

	second, ok := svc.getResolveCache("api.example.com")
	require.True(t, ok)
	require.Equal(t, int64(41), second.domain.User.ID)
	require.Equal(t, []int64{42}, second.domain.AuthorizedUserIDs)
	require.Equal(t, int64(42), second.domain.AuthorizedUsers[0].ID)
}

func (r *customDomainRepoStub) ListByUserID(_ context.Context, userID int64) ([]CustomDomain, error) {
	var domains []CustomDomain
	for _, domain := range r.domains {
		accessible := domain.UserID == userID || domain.AllUsers
		for _, allowedID := range domain.AuthorizedUserIDs {
			accessible = accessible || allowedID == userID
		}
		if accessible {
			domains = append(domains, *domain)
		}
	}
	return domains, nil
}

func (r *customDomainRepoStub) ListAll(_ context.Context, filters CustomDomainListFilters) ([]CustomDomain, error) {
	r.lastFilters = filters
	domains := make([]CustomDomain, 0, len(r.domains))
	for _, domain := range r.domains {
		domains = append(domains, *domain)
	}
	return domains, nil
}

func TestCustomDomainServiceListAllNormalizesFilters(t *testing.T) {
	repo := newCustomDomainRepoStub()
	svc := NewCustomDomainService(repo, newMockSettingRepo(), nil)

	_, err := svc.ListAll(context.Background(), CustomDomainListFilters{
		Domain: " API.Example.COM. ",
		Status: " active ",
		UserID: 4,
	})

	require.NoError(t, err)
	require.Equal(t, "api.example.com", repo.lastFilters.Domain)
	require.Equal(t, "active", repo.lastFilters.Status)
	require.Equal(t, int64(4), repo.lastFilters.UserID)
}

func (r *customDomainRepoStub) Update(_ context.Context, domain *CustomDomain) (*CustomDomain, error) {
	if _, ok := r.domains[domain.ID]; !ok {
		return nil, ErrCustomDomainNotFound
	}
	copy := *domain
	r.domains[domain.ID] = &copy
	return &copy, nil
}

func (r *customDomainRepoStub) Delete(_ context.Context, id int64) error {
	if _, ok := r.domains[id]; !ok {
		return ErrCustomDomainNotFound
	}
	delete(r.domains, id)
	return nil
}

func TestCustomDomainServiceDeleteAsAdminValidatesServiceAndID(t *testing.T) {
	ctx := context.Background()
	var nilService *CustomDomainService
	require.EqualError(t, nilService.DeleteAsAdmin(ctx, 1), "custom domain service is not configured")

	repo := newCustomDomainRepoStub()
	svc := NewCustomDomainService(repo, newMockSettingRepo(), nil)
	err := svc.DeleteAsAdmin(ctx, 0)
	var applicationErr *infraerrors.ApplicationError
	require.ErrorAs(t, err, &applicationErr)
	require.Equal(t, int32(400), applicationErr.Code)
	require.Equal(t, "INVALID_CUSTOM_DOMAIN", applicationErr.Reason)
	require.Equal(t, "custom domain id is required", applicationErr.Message)

	repo.domains[1] = &CustomDomain{ID: 1, Domain: "api.example.com"}
	require.NoError(t, svc.DeleteAsAdmin(ctx, 1))
	_, exists := repo.domains[1]
	require.False(t, exists)
}

func TestCustomDomainServiceDisableAsAdminUsesServiceClock(t *testing.T) {
	ctx := context.Background()
	repo := newCustomDomainRepoStub()
	previousUpdate := time.Date(2026, 7, 20, 1, 2, 3, 0, time.UTC)
	now := time.Date(2026, 7, 26, 11, 55, 0, 0, time.UTC)
	repo.domains[1] = &CustomDomain{
		ID:        1,
		Domain:    "disable.example.com",
		Status:    CustomDomainStatusActive,
		UpdatedAt: previousUpdate,
	}
	svc := NewCustomDomainService(repo, newMockSettingRepo(), nil)
	svc.now = func() time.Time { return now }

	domain, err := svc.DisableAsAdmin(ctx, 1, " maintenance ")
	require.NoError(t, err)
	require.Equal(t, CustomDomainStatusDisabled, domain.Status)
	require.Equal(t, now, *domain.DisabledAt)
	require.Equal(t, "maintenance", *domain.DisabledReason)
	require.Equal(t, previousUpdate, domain.UpdatedAt)
}

func (r *customDomainRepoStub) SetAccess(_ context.Context, id int64, allUsers bool, userIDs []int64) (*CustomDomain, error) {
	domain, ok := r.domains[id]
	if !ok {
		return nil, ErrCustomDomainNotFound
	}
	domain.AllUsers = allUsers
	domain.AuthorizedUserIDs = append([]int64(nil), userIDs...)
	copy := *domain
	return &copy, nil
}

type customDomainDNSStub struct {
	records map[string][]string
	err     error
	calls   int
}

func (r *customDomainDNSStub) LookupTXT(_ context.Context, name string) ([]string, error) {
	r.calls++
	if r.err != nil {
		return nil, r.err
	}
	return append([]string(nil), r.records[name]...), nil
}

func TestValidateCustomDomain(t *testing.T) {
	require.NoError(t, ValidateCustomDomain("API.Example.COM."))
	for _, invalid := range []string{"localhost", "api.local", "127.0.0.1", "bad_domain.example.com", "-api.example.com"} {
		require.Error(t, ValidateCustomDomain(invalid), invalid)
	}
}

func TestCustomDomainServiceCreateVerifyResolveAndCache(t *testing.T) {
	ctx := context.Background()
	repo := newCustomDomainRepoStub()
	settings := newMockSettingRepo()
	require.NoError(t, settings.Set(ctx, SettingKeyCustomDomainsEnabled, "true"))
	require.NoError(t, settings.Set(ctx, SettingKeyAPIBaseURL, "https://gateway.example.com/openai"))
	dns := &customDomainDNSStub{records: make(map[string][]string)}
	svc := NewCustomDomainService(repo, settings, dns)

	domain, err := svc.CreateForUserWithAccess(ctx, 41, " API.Example.COM. ", false, []int64{42, 42, 0})
	require.NoError(t, err)
	require.Equal(t, "api.example.com", domain.Domain)
	require.Equal(t, CustomDomainStatusPendingDNS, domain.Status)
	require.Equal(t, []int64{42}, domain.AuthorizedUserIDs)
	require.True(t, domain.CanManage)
	require.NotEmpty(t, domain.VerificationToken)
	require.Equal(t, "_sub2api.api.example.com", domain.VerificationTXTName)
	require.Equal(t, "sub2api-domain-verification="+domain.VerificationToken, domain.VerificationTXTValue)
	require.Equal(t, "gateway.example.com", svc.GatewayTarget(ctx))

	ownerDomains, err := svc.ListForUser(ctx, 41)
	require.NoError(t, err)
	require.Len(t, ownerDomains, 1)
	require.True(t, ownerDomains[0].CanManage)
	allowedDomains, err := svc.ListForUser(ctx, 42)
	require.NoError(t, err)
	require.Len(t, allowedDomains, 1)
	require.False(t, allowedDomains[0].CanManage)

	dns.records[domain.VerificationTXTName] = []string{"other", domain.VerificationTXTValue}
	repo.domains[domain.ID].CanManage = false
	verified, err := svc.VerifyForUser(ctx, 41, domain.ID)
	require.NoError(t, err)
	require.Equal(t, CustomDomainStatusActive, verified.Status)
	require.NotNil(t, verified.VerifiedAt)
	require.True(t, verified.CanManage)

	resolved, matched, err := svc.ResolveRequestHost(ctx, "https://API.EXAMPLE.COM/v1/messages")
	require.NoError(t, err)
	require.True(t, matched)
	require.Equal(t, domain.ID, resolved.ID)
	require.Equal(t, 1, dns.calls)

	repo.domains[domain.ID].Status = CustomDomainStatusDisabled
	resolved, matched, err = svc.ResolveRequestHost(ctx, "api.example.com:443")
	require.NoError(t, err)
	require.True(t, matched)
	require.Equal(t, domain.ID, resolved.ID, "the bounded resolve cache should serve its active entry")
	require.Equal(t, 1, dns.calls)

	svc.clearCustomDomainCaches()
	_, matched, err = svc.ResolveRequestHost(ctx, "api.example.com")
	require.True(t, matched)
	require.ErrorIs(t, err, ErrCustomDomainInactive)

	resolved, matched, err = svc.ResolveRequestHost(ctx, "gateway.example.com")
	require.NoError(t, err)
	require.False(t, matched)
	require.Nil(t, resolved)

	resolved, matched, err = svc.ResolveRequestHost(ctx, "unknown.example.com")
	require.NoError(t, err)
	require.False(t, matched)
	require.Nil(t, resolved)
}

func TestCustomDomainServiceCachedGatewayTargetDropsExpiredEntry(t *testing.T) {
	svc := NewCustomDomainService(nil, nil, nil)
	now := time.Date(2026, time.July, 25, 7, 0, 0, 0, time.UTC)
	svc.now = func() time.Time { return now }
	svc.gatewayCacheTTL = time.Minute
	svc.storeGatewayTargetCache("gateway.example.com")

	now = now.Add(2 * time.Minute)
	target, ok := svc.cachedGatewayTarget()

	require.False(t, ok)
	require.Empty(t, target)
}

func TestCustomDomainServiceFailClosedContracts(t *testing.T) {
	ctx := context.Background()
	repo := newCustomDomainRepoStub()
	settings := newMockSettingRepo()
	dns := &customDomainDNSStub{err: errors.New("resolver unavailable")}
	svc := NewCustomDomainService(repo, settings, dns)

	_, err := svc.CreateForUserWithAccess(ctx, 1, "api.example.com", false, nil)
	require.ErrorIs(t, err, ErrCustomDomainsDisabled)

	require.NoError(t, settings.Set(ctx, SettingKeyCustomDomainsEnabled, "true"))
	domain, err := svc.CreateForUserWithAccess(ctx, 1, "api.example.com", false, nil)
	require.NoError(t, err)

	_, err = svc.VerifyForUser(ctx, 2, domain.ID)
	require.ErrorIs(t, err, ErrCustomDomainForbidden)

	_, err = svc.VerifyForUser(ctx, 1, domain.ID)
	require.Error(t, err)
	stored := repo.domains[domain.ID]
	require.Equal(t, CustomDomainStatusPendingDNS, stored.Status)
	require.NotNil(t, stored.LastCheckedAt)
	require.NotNil(t, stored.LastError)
	require.WithinDuration(t, time.Now(), *stored.LastCheckedAt, time.Second)
}

func TestNormalizeCustomDomainHostRetainedInputShapes(t *testing.T) {
	tests := map[string]string{
		" HTTPS://API.Example.COM.:443/v1/messages?x=1#fragment ": "api.example.com",
		"API.Example.COM.:443":     "api.example.com",
		"api.example.com/path":     "api.example.com",
		"api.example.com?x=1":      "api.example.com",
		"api.example.com#fragment": "api.example.com",
		"[API.Example.COM]":        "[api.example.com]",
	}
	for input, expected := range tests {
		t.Run(input, func(t *testing.T) {
			require.Equal(t, expected, normalizeCustomDomainHost(input))
		})
	}
}

func TestValidateCustomDomainRetainedErrors(t *testing.T) {
	tests := map[string]string{
		"":                 "domain is required",
		"localhost":        "local hostnames cannot be registered as custom domains",
		"internal":         "domain must include a public suffix",
		"*.example.com":    "wildcard domains are not supported",
		"127.0.0.1":        "IP addresses cannot be registered as custom domains",
		"api.localhost":    "local hostnames cannot be registered as custom domains",
		"-api.example.com": "domain labels cannot start or end with hyphens",
	}
	for input, message := range tests {
		t.Run(input, func(t *testing.T) {
			err := ValidateCustomDomain(input)
			require.Error(t, err)
			require.Contains(t, err.Error(), message)
		})
	}
}

func TestCustomDomainServiceVerifyPendingReturnsDomainAndDNSFailure(t *testing.T) {
	ctx := context.Background()
	repo := newCustomDomainRepoStub()
	settings := newMockSettingRepo()
	require.NoError(t, settings.Set(ctx, SettingKeyCustomDomainsEnabled, "true"))
	svc := NewCustomDomainService(repo, settings, &customDomainDNSStub{records: make(map[string][]string)})
	domain, err := svc.CreateForUserWithAccess(ctx, 41, "api.example.com", false, nil)
	require.NoError(t, err)

	verified, err := svc.VerifyForUser(ctx, 41, domain.ID)

	require.EqualError(t, err, "TXT record _sub2api.api.example.com does not contain the expected value")
	require.NotNil(t, verified)
	require.Equal(t, domain.ID, verified.ID)
	require.Equal(t, CustomDomainStatusPendingDNS, verified.Status)
	require.Equal(t, "TXT record _sub2api.api.example.com does not contain the expected value", *verified.LastError)
}

func TestCustomDomainServiceVerifyDNSRetainedErrors(t *testing.T) {
	ctx := context.Background()
	domain := &CustomDomain{
		VerificationTXTName:  "_sub2api.api.example.com",
		VerificationTXTValue: "sub2api-domain-verification=expected",
	}

	svc := NewCustomDomainService(newCustomDomainRepoStub(), newMockSettingRepo(), nil)
	require.EqualError(t, svc.verifyDNS(ctx, nil), "custom domain DNS verifier is not configured")

	svc.dns = &customDomainDNSStub{err: errors.New("resolver unavailable")}
	require.EqualError(t, svc.verifyDNS(ctx, domain), "TXT record _sub2api.api.example.com was not found")

	svc.dns = &customDomainDNSStub{records: map[string][]string{
		domain.VerificationTXTName: {"different-value"},
	}}
	require.EqualError(t, svc.verifyDNS(ctx, domain), "TXT record _sub2api.api.example.com does not contain the expected value")

	svc.dns = &customDomainDNSStub{records: map[string][]string{
		domain.VerificationTXTName: {"  " + domain.VerificationTXTValue + "  "},
	}}
	require.NoError(t, svc.verifyDNS(ctx, domain))
}

func TestCustomDomainServiceVerifyDNSFailurePersistsPendingDomainAndClearsCache(t *testing.T) {
	ctx := context.Background()
	repo := newCustomDomainRepoStub()
	settings := newMockSettingRepo()
	require.NoError(t, settings.Set(ctx, SettingKeyCustomDomainsEnabled, "true"))
	dnsErr := errors.New("resolver unavailable")
	svc := NewCustomDomainService(repo, settings, &customDomainDNSStub{err: dnsErr})
	fixedNow := time.Date(2026, 7, 25, 6, 0, 0, 0, time.UTC)
	svc.now = func() time.Time { return fixedNow }
	domain, err := svc.CreateForUserWithAccess(ctx, 41, "api.example.com", false, nil)
	require.NoError(t, err)
	repo.domains[domain.ID].Status = CustomDomainStatusActive
	domain.Status = CustomDomainStatusActive
	svc.storeResolveCache(domain.Domain, domain, true)

	verified, err := svc.VerifyForUser(ctx, 41, domain.ID)

	require.EqualError(t, err, "TXT record _sub2api.api.example.com was not found")
	require.NotNil(t, verified)
	require.Equal(t, CustomDomainStatusPendingDNS, verified.Status)
	require.Equal(t, fixedNow, *verified.LastCheckedAt)
	require.Equal(t, "TXT record _sub2api.api.example.com was not found", *verified.LastError)
	_, cached := svc.getResolveCache(domain.Domain)
	require.False(t, cached)
}
