package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

const (
	CustomDomainStatusPendingDNS = "pending_dns"
	CustomDomainStatusActive     = "active"
	CustomDomainStatusDisabled   = "disabled"
	CustomDomainStatusError      = "error"

	customDomainVerificationPrefix = "_sub2api-verify."
	customDomainTokenPrefix        = "sub2api-domain-verification="
)

var (
	ErrCustomDomainsDisabled = infraerrors.Forbidden("CUSTOM_DOMAINS_DISABLED", "custom domains are disabled")
	ErrCustomDomainNotFound  = infraerrors.NotFound("CUSTOM_DOMAIN_NOT_FOUND", "custom domain not found")
	ErrCustomDomainInactive  = infraerrors.Forbidden("CUSTOM_DOMAIN_INACTIVE", "custom domain is not active")
	ErrCustomDomainForbidden = infraerrors.Forbidden("CUSTOM_DOMAIN_FORBIDDEN", "custom domain does not belong to this API key")
	ErrCustomDomainConflict  = infraerrors.New(http.StatusConflict, "CUSTOM_DOMAIN_CONFLICT", "custom domain is already registered")
)

// CustomDomain is the service-layer representation of a verified API hostname.
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
	LastError            *string
	VerifiedAt           *time.Time
	LastCheckedAt        *time.Time
	DisabledAt           *time.Time
	DisabledReason       *string
	CreatedAt            time.Time
	UpdatedAt            time.Time
	DeletedAt            *time.Time

	User      *User
	UserIDs   []int64
	Users     []User
	CanManage bool
}

type CustomDomainListFilters struct {
	Domain   string
	Status   string
	UserID   *int64
	AllUsers *bool
}

type CustomDomainAccessInput struct {
	AllUsers bool
	UserIDs  []int64
}

type CustomDomainRepository interface {
	Create(ctx context.Context, domain *CustomDomain) (*CustomDomain, error)
	GetByID(ctx context.Context, id int64) (*CustomDomain, error)
	GetByDomain(ctx context.Context, domain string) (*CustomDomain, error)
	ListByUserID(ctx context.Context, userID int64) ([]CustomDomain, error)
	ListAll(ctx context.Context, filters CustomDomainListFilters) ([]CustomDomain, error)
	SetAccess(ctx context.Context, id int64, allUsers bool, userIDs []int64) (*CustomDomain, error)
	Update(ctx context.Context, domain *CustomDomain) (*CustomDomain, error)
	Delete(ctx context.Context, id int64) error
}

type CustomDomainDNSResolver interface {
	LookupTXT(ctx context.Context, name string) ([]string, error)
}

type netCustomDomainDNSResolver struct{}

func (netCustomDomainDNSResolver) LookupTXT(ctx context.Context, name string) ([]string, error) {
	return net.DefaultResolver.LookupTXT(ctx, name)
}

type CustomDomainService struct {
	repo      CustomDomainRepository
	setting   SettingRepository
	dns       CustomDomainDNSResolver
	now       func() time.Time
	tokenFunc func() (string, error)
}

func NewCustomDomainService(repo CustomDomainRepository, setting SettingRepository) *CustomDomainService {
	return &CustomDomainService{
		repo:      repo,
		setting:   setting,
		dns:       netCustomDomainDNSResolver{},
		now:       time.Now,
		tokenFunc: generateCustomDomainToken,
	}
}

func (s *CustomDomainService) SetDNSResolverForTest(resolver CustomDomainDNSResolver) {
	if resolver != nil {
		s.dns = resolver
	}
}

func (s *CustomDomainService) SetNowForTest(now func() time.Time) {
	if now != nil {
		s.now = now
	}
}

func (s *CustomDomainService) IsEnabled(ctx context.Context) bool {
	if s == nil || s.setting == nil {
		return false
	}
	raw, err := s.setting.GetValue(ctx, SettingKeyCustomDomainsEnabled)
	if err != nil {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(raw), "true")
}

func (s *CustomDomainService) SetEnabled(ctx context.Context, enabled bool) error {
	if s == nil || s.setting == nil {
		return errors.New("custom domain service is not configured")
	}
	return s.setting.Set(ctx, SettingKeyCustomDomainsEnabled, fmt.Sprintf("%t", enabled))
}

func (s *CustomDomainService) GatewayTarget(ctx context.Context) string {
	if s == nil || s.setting == nil {
		return ""
	}
	raw, err := s.setting.GetValue(ctx, SettingKeyAPIBaseURL)
	if err != nil {
		return ""
	}
	return normalizeCustomDomainHost(extractHost(raw))
}

func (s *CustomDomainService) ListForUser(ctx context.Context, userID int64) ([]CustomDomain, error) {
	if s == nil || s.repo == nil {
		return nil, errors.New("custom domain service is not configured")
	}
	if userID <= 0 {
		return nil, infraerrors.BadRequest("INVALID_USER", "user is required")
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
		return nil, errors.New("custom domain service is not configured")
	}
	filters.Domain = normalizeCustomDomainHost(filters.Domain)
	filters.Status = strings.TrimSpace(filters.Status)
	return s.repo.ListAll(ctx, filters)
}

func (s *CustomDomainService) CreateForUser(ctx context.Context, userID int64, rawDomain string) (*CustomDomain, error) {
	return s.CreateForUserWithAccess(ctx, userID, rawDomain, CustomDomainAccessInput{UserIDs: []int64{userID}})
}

func (s *CustomDomainService) CreateForUserWithAccess(ctx context.Context, userID int64, rawDomain string, access CustomDomainAccessInput) (*CustomDomain, error) {
	if s == nil || s.repo == nil {
		return nil, errors.New("custom domain service is not configured")
	}
	if !s.IsEnabled(ctx) {
		return nil, ErrCustomDomainsDisabled
	}
	if userID <= 0 {
		return nil, infraerrors.BadRequest("INVALID_USER", "user is required")
	}
	domain, err := ValidateCustomDomain(rawDomain)
	if err != nil {
		return nil, err
	}
	token, err := s.tokenFunc()
	if err != nil {
		return nil, fmt.Errorf("generate custom domain token: %w", err)
	}
	txtName := customDomainVerificationPrefix + domain
	target := s.GatewayTarget(ctx)
	var targetPtr *string
	if target != "" {
		targetPtr = &target
	}
	allUsers, userIDs := normalizeCustomDomainAccess(userID, access)
	record := &CustomDomain{
		UserID:               userID,
		AllUsers:             allUsers,
		UserIDs:              userIDs,
		Domain:               domain,
		Status:               CustomDomainStatusPendingDNS,
		VerificationToken:    token,
		VerificationTXTName:  txtName,
		VerificationTXTValue: customDomainTokenPrefix + token,
		CNAMETarget:          targetPtr,
	}
	created, err := s.repo.Create(ctx, record)
	if err != nil {
		if errors.Is(err, ErrCustomDomainConflict) {
			return nil, err
		}
		return nil, fmt.Errorf("create custom domain: %w", err)
	}
	created.CanManage = true
	return created, nil
}

func (s *CustomDomainService) UpdateAccessAsAdmin(ctx context.Context, id int64, access CustomDomainAccessInput) (*CustomDomain, error) {
	domain, err := s.getByID(ctx, id)
	if err != nil {
		return nil, err
	}
	allUsers, userIDs := normalizeCustomDomainAccess(domain.UserID, access)
	return s.repo.SetAccess(ctx, domain.ID, allUsers, userIDs)
}

func (s *CustomDomainService) VerifyForUser(ctx context.Context, userID, id int64) (*CustomDomain, error) {
	if !s.IsEnabled(ctx) {
		return nil, ErrCustomDomainsDisabled
	}
	return s.verify(ctx, userID, id)
}

func (s *CustomDomainService) VerifyAsAdmin(ctx context.Context, id int64) (*CustomDomain, error) {
	return s.verify(ctx, 0, id)
}

func (s *CustomDomainService) DeleteForUser(ctx context.Context, userID, id int64) error {
	domain, err := s.getOwned(ctx, userID, id)
	if err != nil {
		return err
	}
	return s.repo.Delete(ctx, domain.ID)
}

func (s *CustomDomainService) DeleteAsAdmin(ctx context.Context, id int64) error {
	if s == nil || s.repo == nil {
		return errors.New("custom domain service is not configured")
	}
	if id <= 0 {
		return infraerrors.BadRequest("INVALID_CUSTOM_DOMAIN", "custom domain id is required")
	}
	if _, err := s.repo.GetByID(ctx, id); err != nil {
		return err
	}
	return s.repo.Delete(ctx, id)
}

func (s *CustomDomainService) DisableAsAdmin(ctx context.Context, id int64, reason string) (*CustomDomain, error) {
	domain, err := s.getByID(ctx, id)
	if err != nil {
		return nil, err
	}
	now := s.now().UTC()
	domain.Status = CustomDomainStatusDisabled
	domain.DisabledAt = &now
	domain.DisabledReason = optionalTrimmedStringPtr(reason)
	return s.repo.Update(ctx, domain)
}

func (s *CustomDomainService) EnableAsAdmin(ctx context.Context, id int64) (*CustomDomain, error) {
	domain, err := s.getByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if domain.VerifiedAt != nil {
		domain.Status = CustomDomainStatusActive
	} else {
		domain.Status = CustomDomainStatusPendingDNS
	}
	domain.DisabledAt = nil
	domain.DisabledReason = nil
	domain.LastError = nil
	return s.repo.Update(ctx, domain)
}

func (s *CustomDomainService) ResolveRequestHost(ctx context.Context, host string) (*CustomDomain, bool, error) {
	if s == nil || s.repo == nil {
		return nil, false, nil
	}
	host = normalizeCustomDomainHost(host)
	if host == "" {
		return nil, false, nil
	}
	if target := s.GatewayTarget(ctx); target != "" && strings.EqualFold(host, target) {
		return nil, false, nil
	}
	domain, err := s.repo.GetByDomain(ctx, host)
	if err != nil {
		if errors.Is(err, ErrCustomDomainNotFound) {
			return nil, false, nil
		}
		return nil, false, err
	}
	if !s.IsEnabled(ctx) {
		return domain, true, ErrCustomDomainsDisabled
	}
	if domain.Status != CustomDomainStatusActive {
		return domain, true, ErrCustomDomainInactive
	}
	return domain, true, nil
}

func (d *CustomDomain) CanUse(userID int64) bool {
	if d == nil || userID <= 0 {
		return false
	}
	if d.UserID == userID || d.AllUsers {
		return true
	}
	for _, id := range d.UserIDs {
		if id == userID {
			return true
		}
	}
	return false
}

func (s *CustomDomainService) getByID(ctx context.Context, id int64) (*CustomDomain, error) {
	if s == nil || s.repo == nil {
		return nil, errors.New("custom domain service is not configured")
	}
	if id <= 0 {
		return nil, infraerrors.BadRequest("INVALID_CUSTOM_DOMAIN", "custom domain id is required")
	}
	return s.repo.GetByID(ctx, id)
}

func (s *CustomDomainService) getOwned(ctx context.Context, userID, id int64) (*CustomDomain, error) {
	if userID <= 0 {
		return nil, infraerrors.BadRequest("INVALID_USER", "user is required")
	}
	domain, err := s.getByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if domain.UserID != userID {
		return nil, ErrCustomDomainNotFound
	}
	return domain, nil
}

func (s *CustomDomainService) verify(ctx context.Context, userID, id int64) (*CustomDomain, error) {
	domain, err := s.getByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if userID > 0 && domain.UserID != userID {
		return nil, ErrCustomDomainNotFound
	}

	now := s.now().UTC()
	domain.LastCheckedAt = &now
	domain.DisabledAt = nil
	domain.DisabledReason = nil

	if err := s.verifyDNS(ctx, domain); err != nil {
		msg := err.Error()
		domain.Status = CustomDomainStatusPendingDNS
		domain.LastError = &msg
		updated, updateErr := s.repo.Update(ctx, domain)
		if updateErr != nil {
			return nil, updateErr
		}
		return updated, err
	}

	domain.Status = CustomDomainStatusActive
	domain.VerifiedAt = &now
	domain.LastError = nil
	return s.repo.Update(ctx, domain)
}

func normalizeCustomDomainAccess(ownerUserID int64, access CustomDomainAccessInput) (bool, []int64) {
	if access.AllUsers {
		return true, nil
	}

	seen := map[int64]struct{}{}
	out := make([]int64, 0, len(access.UserIDs)+1)
	add := func(id int64) {
		if id <= 0 {
			return
		}
		if _, ok := seen[id]; ok {
			return
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	add(ownerUserID)
	for _, id := range access.UserIDs {
		add(id)
	}
	return false, out
}

func (s *CustomDomainService) verifyDNS(ctx context.Context, domain *CustomDomain) error {
	if s == nil || s.dns == nil || domain == nil {
		return errors.New("custom domain DNS verifier is not configured")
	}
	txtValues, err := s.dns.LookupTXT(ctx, domain.VerificationTXTName)
	if err != nil {
		return fmt.Errorf("TXT record %s was not found", domain.VerificationTXTName)
	}
	txtOK := false
	for _, value := range txtValues {
		if strings.TrimSpace(value) == domain.VerificationTXTValue {
			txtOK = true
			break
		}
	}
	if !txtOK {
		return fmt.Errorf("TXT record %s does not contain the expected value", domain.VerificationTXTName)
	}

	return nil
}

func ValidateCustomDomain(raw string) (string, error) {
	domain := normalizeCustomDomainHost(raw)
	if domain == "" {
		return "", infraerrors.BadRequest("INVALID_CUSTOM_DOMAIN", "domain is required")
	}
	if strings.Contains(domain, "*") {
		return "", infraerrors.BadRequest("INVALID_CUSTOM_DOMAIN", "wildcard domains are not supported")
	}
	if net.ParseIP(domain) != nil {
		return "", infraerrors.BadRequest("INVALID_CUSTOM_DOMAIN", "IP addresses cannot be registered as custom domains")
	}
	if domain == "localhost" || strings.HasSuffix(domain, ".localhost") || strings.HasSuffix(domain, ".local") {
		return "", infraerrors.BadRequest("INVALID_CUSTOM_DOMAIN", "local hostnames cannot be registered as custom domains")
	}
	if !strings.Contains(domain, ".") {
		return "", infraerrors.BadRequest("INVALID_CUSTOM_DOMAIN", "domain must include a public suffix")
	}
	if len(domain) > 253 {
		return "", infraerrors.BadRequest("INVALID_CUSTOM_DOMAIN", "domain is too long")
	}
	labels := strings.Split(domain, ".")
	for _, label := range labels {
		if label == "" || len(label) > 63 {
			return "", infraerrors.BadRequest("INVALID_CUSTOM_DOMAIN", "domain contains an invalid label")
		}
		if strings.HasPrefix(label, "-") || strings.HasSuffix(label, "-") {
			return "", infraerrors.BadRequest("INVALID_CUSTOM_DOMAIN", "domain labels cannot start or end with hyphens")
		}
		for _, r := range label {
			if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
				continue
			}
			return "", infraerrors.BadRequest("INVALID_CUSTOM_DOMAIN", "domain must use ASCII letters, numbers, and hyphens")
		}
	}
	return domain, nil
}

func normalizeCustomDomainHost(raw string) string {
	raw = strings.TrimSpace(strings.ToLower(raw))
	raw = strings.TrimSuffix(raw, ".")
	if raw == "" {
		return ""
	}
	if strings.Contains(raw, "://") {
		raw = extractHost(raw)
	}
	raw = strings.TrimSpace(raw)
	if strings.Contains(raw, "/") || strings.Contains(raw, "?") || strings.Contains(raw, "#") || strings.Contains(raw, "@") {
		return ""
	}
	if host, _, err := net.SplitHostPort(raw); err == nil {
		raw = strings.Trim(host, "[]")
	}
	return strings.TrimSuffix(raw, ".")
}

func extractHost(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	parsed, err := url.Parse(raw)
	if err == nil && parsed.Host != "" {
		return parsed.Host
	}
	parsed, err = url.Parse("https://" + raw)
	if err == nil {
		return parsed.Host
	}
	return raw
}

func generateCustomDomainToken() (string, error) {
	buf := make([]byte, 24)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}
