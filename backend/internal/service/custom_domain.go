package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

const (
	CustomDomainStatusPendingDNS   = "pending_dns"
	CustomDomainStatusActive       = "active"
	CustomDomainStatusDisabled     = "disabled"
	SettingKeyCustomDomainsEnabled = "custom_domains_enabled"

	customDomainResolveCacheTTL       = time.Minute
	customDomainGatewayTargetCacheTTL = time.Minute
	customDomainVerificationPrefix    = "sub2api-domain-verification="
)

var (
	ErrCustomDomainNotFound            = infraerrors.NotFound("CUSTOM_DOMAIN_NOT_FOUND", "custom domain not found")
	ErrCustomDomainConflict            = infraerrors.Conflict("CUSTOM_DOMAIN_CONFLICT", "custom domain already exists")
	ErrCustomDomainsDisabled           = infraerrors.Forbidden("CUSTOM_DOMAINS_DISABLED", "custom domains are disabled")
	ErrCustomDomainForbidden           = infraerrors.Forbidden("CUSTOM_DOMAIN_FORBIDDEN", "custom domain does not belong to this API key")
	ErrCustomDomainInactive            = infraerrors.Forbidden("CUSTOM_DOMAIN_INACTIVE", "custom domain is not active")
	ErrCustomDomainVerificationPending = errors.New("custom domain verification pending")
)

type CustomDomain struct {
	ID                   int64
	UserID               int64
	AllUsers             bool
	Domain               string
	Status               string
	VerificationToken    string
	VerificationTXTName  string
	VerificationTXTValue string
	CNAMETarget          *string
	VerifiedAt           *time.Time
	LastCheckedAt        *time.Time
	LastError            *string
	DisabledAt           *time.Time
	DisabledReason       *string
	CreatedAt            time.Time
	UpdatedAt            time.Time
	DeletedAt            *time.Time
	User                 *User
	AuthorizedUserIDs    []int64
	AuthorizedUsers      []User
	CanManage            bool
}

func (d *CustomDomain) CanUse(userID int64) bool {
	if d == nil || d.Status != CustomDomainStatusActive {
		return false
	}
	if d.AllUsers || d.UserID == userID {
		return true
	}
	for _, allowedID := range d.AuthorizedUserIDs {
		if allowedID == userID {
			return true
		}
	}
	return false
}

type CustomDomainListFilters struct {
	Domain   string
	Status   string
	UserID   int64
	AllUsers *bool
}

type CustomDomainRepository interface {
	Create(context.Context, *CustomDomain) (*CustomDomain, error)
	GetByID(context.Context, int64) (*CustomDomain, error)
	GetByDomain(context.Context, string) (*CustomDomain, error)
	ListByUserID(context.Context, int64) ([]CustomDomain, error)
	ListAll(context.Context, CustomDomainListFilters) ([]CustomDomain, error)
	Update(context.Context, *CustomDomain) (*CustomDomain, error)
	Delete(context.Context, int64) error
	SetAccess(context.Context, int64, bool, []int64) (*CustomDomain, error)
}

type CustomDomainDNSResolver interface {
	LookupTXT(context.Context, string) ([]string, error)
}

type netCustomDomainDNSResolver struct {
	resolver *net.Resolver
}

func (r netCustomDomainDNSResolver) LookupTXT(ctx context.Context, name string) ([]string, error) {
	resolver := r.resolver
	if resolver == nil {
		resolver = net.DefaultResolver
	}
	return resolver.LookupTXT(ctx, name)
}

type customDomainResolveCacheEntry struct {
	domain    *CustomDomain
	matched   bool
	expiresAt time.Time
}

type customDomainGatewayTargetCacheEntry struct {
	target    string
	expiresAt time.Time
}

type CustomDomainService struct {
	repo        CustomDomainRepository
	settingRepo SettingRepository
	dns         CustomDomainDNSResolver
	now         func() time.Time

	resolveCacheMu  sync.RWMutex
	resolveCache    map[string]customDomainResolveCacheEntry
	resolveCacheTTL time.Duration
	gatewayCacheMu  sync.RWMutex
	gatewayCache    *customDomainGatewayTargetCacheEntry
	gatewayCacheTTL time.Duration
}

func NewCustomDomainService(repo CustomDomainRepository, settingRepo SettingRepository, dns CustomDomainDNSResolver) *CustomDomainService {
	if dns == nil {
		dns = netCustomDomainDNSResolver{}
	}
	return &CustomDomainService{
		repo:            repo,
		settingRepo:     settingRepo,
		dns:             dns,
		now:             time.Now,
		resolveCache:    make(map[string]customDomainResolveCacheEntry),
		resolveCacheTTL: customDomainResolveCacheTTL,
		gatewayCacheTTL: customDomainGatewayTargetCacheTTL,
	}
}

func (s *CustomDomainService) IsEnabled(ctx context.Context) bool {
	if s == nil || s.settingRepo == nil {
		return false
	}
	value, err := s.settingRepo.GetValue(ctx, SettingKeyCustomDomainsEnabled)
	return err == nil && strings.EqualFold(strings.TrimSpace(value), "true")
}

func (s *CustomDomainService) SetEnabled(ctx context.Context, enabled bool) error {
	if s == nil || s.settingRepo == nil {
		return fmt.Errorf("custom domain service is not configured")
	}
	value := "false"
	if enabled {
		value = "true"
	}
	if err := s.settingRepo.Set(ctx, SettingKeyCustomDomainsEnabled, value); err != nil {
		return err
	}
	s.clearCustomDomainCaches()
	return nil
}

func (s *CustomDomainService) GatewayTarget(ctx context.Context) string {
	if target, ok := s.cachedGatewayTarget(); ok {
		return target
	}
	if s == nil || s.settingRepo == nil {
		return ""
	}
	raw, err := s.settingRepo.GetValue(ctx, SettingKeyAPIBaseURL)
	if err != nil {
		return ""
	}
	target := normalizeCustomDomainHost(extractHost(raw))
	s.storeGatewayTargetCache(target)
	return target
}

func (s *CustomDomainService) CreateForUserWithAccess(ctx context.Context, userID int64, name string, allUsers bool, userIDs []int64) (*CustomDomain, error) {
	if !s.IsEnabled(ctx) {
		return nil, ErrCustomDomainsDisabled
	}
	name = normalizeCustomDomainHost(name)
	if err := ValidateCustomDomain(name); err != nil {
		return nil, err
	}
	token, err := generateCustomDomainToken()
	if err != nil {
		return nil, fmt.Errorf("generate custom domain token: %w", err)
	}
	allUsers, userIDs = normalizeCustomDomainAccess(allUsers, userIDs)
	now := time.Now().UTC()
	domain := &CustomDomain{
		UserID:               userID,
		Domain:               name,
		Status:               CustomDomainStatusPendingDNS,
		AllUsers:             allUsers,
		VerificationToken:    token,
		VerificationTXTName:  "_sub2api." + name,
		VerificationTXTValue: customDomainVerificationPrefix + token,
		AuthorizedUserIDs:    userIDs,
		CanManage:            true,
		CreatedAt:            now,
		UpdatedAt:            now,
	}
	if target := s.GatewayTarget(ctx); target != "" {
		domain.CNAMETarget = &target
	}
	if s.repo == nil {
		return nil, fmt.Errorf("custom domain repository is not configured")
	}
	domain, err = s.repo.Create(ctx, domain)
	if err != nil {
		return nil, err
	}
	s.clearResolvedDomainCache()
	return domain, nil
}

func (s *CustomDomainService) ListForUser(ctx context.Context, userID int64) ([]CustomDomain, error) {
	if !s.IsEnabled(ctx) {
		return nil, ErrCustomDomainsDisabled
	}
	domains, err := s.repo.ListByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}
	for i := range domains {
		domains[i].CanManage = domains[i].UserID == userID
	}
	return domains, nil
}

func (s *CustomDomainService) ListAll(ctx context.Context, filters CustomDomainListFilters) ([]CustomDomain, error) {
	if s == nil || s.repo == nil {
		return nil, errors.New("custom domain repository is not configured")
	}
	filters.Domain = normalizeCustomDomainHost(filters.Domain)
	filters.Status = strings.TrimSpace(filters.Status)
	return s.repo.ListAll(ctx, filters)
}

func (s *CustomDomainService) VerifyForUser(ctx context.Context, userID, id int64) (*CustomDomain, error) {
	if !s.IsEnabled(ctx) {
		return nil, ErrCustomDomainsDisabled
	}
	domain, err := s.verify(ctx, userID, id)
	if domain != nil {
		domain.CanManage = true
	}
	return domain, err
}

func (s *CustomDomainService) DeleteForUser(ctx context.Context, userID, id int64) error {
	if _, err := s.getOwned(ctx, userID, id); err != nil {
		return err
	}
	if err := s.repo.Delete(ctx, id); err != nil {
		return err
	}
	s.clearResolvedDomainCache()
	return nil
}

func (s *CustomDomainService) DeleteAsAdmin(ctx context.Context, id int64) error {
	if s == nil || s.repo == nil {
		return fmt.Errorf("custom domain service is not configured")
	}
	if id <= 0 {
		return infraerrors.BadRequest("INVALID_CUSTOM_DOMAIN", "custom domain id is required")
	}
	if _, err := s.repo.GetByID(ctx, id); err != nil {
		return err
	}
	if err := s.repo.Delete(ctx, id); err != nil {
		return err
	}
	s.clearResolvedDomainCache()
	return nil
}

func (s *CustomDomainService) DisableAsAdmin(ctx context.Context, id int64, reason string) (*CustomDomain, error) {
	domain, err := s.getByID(ctx, id)
	if err != nil {
		return nil, err
	}
	now := s.now().UTC()
	reason = strings.TrimSpace(reason)
	domain.Status = CustomDomainStatusDisabled
	domain.DisabledAt = &now
	domain.DisabledReason = nil
	if reason != "" {
		domain.DisabledReason = &reason
	}
	domain, err = s.repo.Update(ctx, domain)
	if err != nil {
		return nil, err
	}
	s.clearResolvedDomainCache()
	return domain, nil
}

func (s *CustomDomainService) EnableAsAdmin(ctx context.Context, id int64) (*CustomDomain, error) {
	domain, err := s.getByID(ctx, id)
	if err != nil {
		return nil, err
	}
	domain.Status = CustomDomainStatusPendingDNS
	domain.DisabledAt = nil
	domain.DisabledReason = nil
	domain.UpdatedAt = time.Now().UTC()
	domain, err = s.repo.Update(ctx, domain)
	if err != nil {
		return nil, err
	}
	s.clearResolvedDomainCache()
	return domain, nil
}

func (s *CustomDomainService) UpdateAccessAsAdmin(ctx context.Context, id int64, allUsers bool, userIDs []int64) (*CustomDomain, error) {
	allUsers, userIDs = normalizeCustomDomainAccess(allUsers, userIDs)
	domain, err := s.repo.SetAccess(ctx, id, allUsers, userIDs)
	if err != nil {
		return nil, err
	}
	s.clearResolvedDomainCache()
	return domain, nil
}

func (s *CustomDomainService) ResolveRequestHost(ctx context.Context, requestHost string) (*CustomDomain, bool, error) {
	if s == nil || s.repo == nil {
		return nil, false, nil
	}
	host := normalizeCustomDomainHost(extractHost(requestHost))
	if host == "" {
		return nil, false, nil
	}
	if strings.EqualFold(host, s.GatewayTarget(ctx)) {
		return nil, false, nil
	}
	domain, err := s.resolveDomainByHost(ctx, host)
	if err != nil {
		if errors.Is(err, ErrCustomDomainNotFound) {
			return nil, false, nil
		}
		return nil, true, err
	}
	if !s.IsEnabled(ctx) {
		return domain, true, ErrCustomDomainsDisabled
	}
	if domain.Status != CustomDomainStatusActive {
		return nil, true, ErrCustomDomainInactive
	}
	return domain, true, nil
}

func (s *CustomDomainService) resolveDomainByHost(ctx context.Context, host string) (*CustomDomain, error) {
	if entry, ok := s.getResolveCache(host); ok {
		if !entry.matched {
			return nil, ErrCustomDomainNotFound
		}
		if entry.domain == nil {
			return nil, nil
		}
		domain := *entry.domain
		if entry.domain.AuthorizedUserIDs != nil {
			domain.AuthorizedUserIDs = append([]int64(nil), entry.domain.AuthorizedUserIDs...)
		}
		if entry.domain.AuthorizedUsers != nil {
			domain.AuthorizedUsers = append([]User(nil), entry.domain.AuthorizedUsers...)
		}
		if entry.domain.User != nil {
			user := *entry.domain.User
			domain.User = &user
		}
		return &domain, nil
	}
	domain, err := s.repo.GetByDomain(ctx, host)
	if err != nil {
		if errors.Is(err, ErrCustomDomainNotFound) {
			s.storeResolveCache(host, nil, false)
		}
		return nil, err
	}
	s.storeResolveCache(host, domain, true)
	return domain, nil
}

func (s *CustomDomainService) verify(ctx context.Context, userID, id int64) (*CustomDomain, error) {
	domain, err := s.getByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if userID > 0 && domain.UserID != userID {
		return nil, ErrCustomDomainForbidden
	}
	now := s.now().UTC()
	domain.LastCheckedAt = &now
	err = s.verifyDNS(ctx, domain)
	if err != nil {
		message := err.Error()
		domain.LastError = &message
		domain.Status = CustomDomainStatusPendingDNS
		domain, updateErr := s.repo.Update(ctx, domain)
		if updateErr != nil {
			return nil, updateErr
		}
		s.clearResolvedDomainCache()
		return domain, err
	}
	domain.Status = CustomDomainStatusActive
	domain.VerifiedAt = &now
	domain.LastError = nil
	domain.DisabledAt = nil
	domain.DisabledReason = nil
	domain, err = s.repo.Update(ctx, domain)
	if err != nil {
		return nil, err
	}
	s.clearResolvedDomainCache()
	return domain, nil
}

func (s *CustomDomainService) verifyDNS(ctx context.Context, domain *CustomDomain) error {
	if s == nil || s.dns == nil || domain == nil {
		return fmt.Errorf("custom domain DNS verifier is not configured")
	}
	records, err := s.dns.LookupTXT(ctx, domain.VerificationTXTName)
	if err != nil {
		return fmt.Errorf("TXT record %s was not found", domain.VerificationTXTName)
	}
	for _, record := range records {
		if strings.TrimSpace(record) == domain.VerificationTXTValue {
			return nil
		}
	}
	return fmt.Errorf("TXT record %s does not contain the expected value", domain.VerificationTXTName)
}

func (s *CustomDomainService) getByID(ctx context.Context, id int64) (*CustomDomain, error) {
	if s == nil || s.repo == nil {
		return nil, fmt.Errorf("custom domain service is not configured")
	}
	return s.repo.GetByID(ctx, id)
}

func (s *CustomDomainService) getOwned(ctx context.Context, userID, id int64) (*CustomDomain, error) {
	domain, err := s.getByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if domain.UserID != userID {
		return nil, ErrCustomDomainForbidden
	}
	domain.CanManage = true
	return domain, nil
}

func (s *CustomDomainService) getResolveCache(host string) (customDomainResolveCacheEntry, bool) {
	if s == nil || s.resolveCacheTTL <= 0 {
		return customDomainResolveCacheEntry{}, false
	}
	now := s.now()
	s.resolveCacheMu.RLock()
	entry, ok := s.resolveCache[host]
	s.resolveCacheMu.RUnlock()
	if !ok {
		return customDomainResolveCacheEntry{}, false
	}
	if !now.Before(entry.expiresAt) {
		s.resolveCacheMu.Lock()
		if current, exists := s.resolveCache[host]; exists && !now.Before(current.expiresAt) {
			delete(s.resolveCache, host)
		}
		s.resolveCacheMu.Unlock()
		return customDomainResolveCacheEntry{}, false
	}
	entry.domain = cloneCustomDomain(entry.domain)
	return entry, true
}

func (s *CustomDomainService) storeResolveCache(host string, domain *CustomDomain, matched bool) {
	if s == nil || s.resolveCacheTTL <= 0 {
		return
	}
	s.resolveCacheMu.Lock()
	defer s.resolveCacheMu.Unlock()
	if s.resolveCache == nil {
		s.resolveCache = make(map[string]customDomainResolveCacheEntry)
	}
	s.resolveCache[host] = customDomainResolveCacheEntry{
		domain:    cloneCustomDomain(domain),
		matched:   matched,
		expiresAt: s.now().Add(s.resolveCacheTTL),
	}
}

func cloneCustomDomain(domain *CustomDomain) *CustomDomain {
	if domain == nil {
		return nil
	}
	clone := *domain
	if domain.User != nil {
		user := *domain.User
		clone.User = &user
	}
	clone.AuthorizedUserIDs = append([]int64(nil), domain.AuthorizedUserIDs...)
	clone.AuthorizedUsers = append([]User(nil), domain.AuthorizedUsers...)
	return &clone
}

func (s *CustomDomainService) clearResolvedDomainCache() {
	if s == nil {
		return
	}
	s.resolveCacheMu.Lock()
	defer s.resolveCacheMu.Unlock()
	s.resolveCache = make(map[string]customDomainResolveCacheEntry)
}

func (s *CustomDomainService) cachedGatewayTarget() (string, bool) {
	if s == nil || s.gatewayCacheTTL <= 0 {
		return "", false
	}
	now := s.now()
	s.gatewayCacheMu.RLock()
	entry := s.gatewayCache
	s.gatewayCacheMu.RUnlock()
	if entry != nil && now.Before(entry.expiresAt) {
		return entry.target, true
	}
	if entry != nil {
		s.gatewayCacheMu.Lock()
		entry = s.gatewayCache
		if entry != nil && !now.Before(entry.expiresAt) {
			s.gatewayCache = nil
		}
		s.gatewayCacheMu.Unlock()
	}
	return "", false
}

func (s *CustomDomainService) storeGatewayTargetCache(target string) {
	if s == nil {
		return
	}
	s.gatewayCacheMu.Lock()
	defer s.gatewayCacheMu.Unlock()
	s.gatewayCache = &customDomainGatewayTargetCacheEntry{target: target, expiresAt: s.now().Add(s.gatewayCacheTTL)}
}

func (s *CustomDomainService) clearCustomDomainCaches() {
	if s == nil {
		return
	}
	s.resolveCacheMu.Lock()
	defer s.resolveCacheMu.Unlock()
	s.resolveCache = make(map[string]customDomainResolveCacheEntry)
}

func ValidateCustomDomain(name string) error {
	name = normalizeCustomDomainHost(name)
	if name == "" {
		return infraerrors.BadRequest("INVALID_CUSTOM_DOMAIN", "domain is required")
	}
	if strings.Contains(name, "*") {
		return infraerrors.BadRequest("INVALID_CUSTOM_DOMAIN", "wildcard domains are not supported")
	}
	if net.ParseIP(name) != nil {
		return infraerrors.BadRequest("INVALID_CUSTOM_DOMAIN", "IP addresses cannot be registered as custom domains")
	}
	if name == "localhost" || strings.HasSuffix(name, ".localhost") || strings.HasSuffix(name, ".local") {
		return infraerrors.BadRequest("INVALID_CUSTOM_DOMAIN", "local hostnames cannot be registered as custom domains")
	}
	if !strings.Contains(name, ".") {
		return infraerrors.BadRequest("INVALID_CUSTOM_DOMAIN", "domain must include a public suffix")
	}
	if len(name) > 253 {
		return infraerrors.BadRequest("INVALID_CUSTOM_DOMAIN", "domain is too long")
	}
	for _, label := range strings.Split(name, ".") {
		if label == "" || len(label) > 63 {
			return infraerrors.BadRequest("INVALID_CUSTOM_DOMAIN", "domain contains an invalid label")
		}
		if label[0] == '-' || label[len(label)-1] == '-' {
			return infraerrors.BadRequest("INVALID_CUSTOM_DOMAIN", "domain labels cannot start or end with hyphens")
		}
		for _, char := range label {
			if (char < 'a' || char > 'z') && (char < '0' || char > '9') && char != '-' {
				return infraerrors.BadRequest("INVALID_CUSTOM_DOMAIN", "domain must use ASCII letters, numbers, and hyphens")
			}
		}
	}
	return nil
}

func normalizeCustomDomainAccess(allUsers bool, userIDs []int64) (bool, []int64) {
	if allUsers {
		return true, nil
	}
	seen := make(map[int64]struct{}, len(userIDs))
	normalized := make([]int64, 0, len(userIDs))
	for _, userID := range userIDs {
		if userID <= 0 {
			continue
		}
		if _, ok := seen[userID]; ok {
			continue
		}
		seen[userID] = struct{}{}
		normalized = append(normalized, userID)
	}
	sort.Slice(normalized, func(i, j int) bool { return normalized[i] < normalized[j] })
	return false, normalized
}

func generateCustomDomainToken() (string, error) {
	raw := make([]byte, 24)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return hex.EncodeToString(raw), nil
}

func normalizeCustomDomainHost(value string) string {
	host := strings.TrimSuffix(strings.ToLower(strings.TrimSpace(value)), ".")
	if host == "" {
		return ""
	}
	if strings.Contains(host, "://") {
		host = extractHost(host)
	}
	host = strings.ToLower(host)
	for _, separator := range []byte{'/', '?', '#'} {
		if index := strings.IndexByte(host, separator); index >= 0 {
			host = host[:index]
		}
	}
	if strings.Contains(host, ":") {
		if splitHost, _, err := net.SplitHostPort(host); err == nil {
			host = strings.Trim(splitHost, "[]")
		}
	}
	return strings.TrimSuffix(host, ".")
}

func extractHost(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if parsed, err := url.Parse(value); err == nil && parsed.Host != "" {
		return parsed.Hostname()
	}
	if host, _, err := net.SplitHostPort(value); err == nil {
		return host
	}
	if slash := strings.IndexByte(value, '/'); slash >= 0 {
		value = value[:slash]
	}
	return strings.Trim(value, "[]")
}
