package service

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/domain"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/util/urlvalidator"
)

type RelayEarning struct {
	ID            int64      `json:"id"`
	AccountID     int64      `json:"account_id"`
	GroupID       int64      `json:"group_id"`
	RequestID     string     `json:"request_id"`
	ChargedAmount float64    `json:"charged_amount"`
	SupplierRate  float64    `json:"supplier_rate"`
	Amount        float64    `json:"amount"`
	SelfUse       bool       `json:"self_use"`
	CreatedAt     time.Time  `json:"created_at"`
	CreditedAt    *time.Time `json:"credited_at"`
}

type RelayEarnings struct {
	Items          []RelayEarning `json:"items"`
	Total          int64          `json:"total"`
	TotalEarned    float64        `json:"total_earned"`
	TodayEarned    float64        `json:"today_earned"`
	PendingEarned  float64        `json:"pending_earned"`
	BalanceChanged bool           `json:"-"`
}

type RelayRepository interface {
	ListPrivateGroups(context.Context, int64) ([]Group, error)
	ListOwnedAccountIDs(context.Context, int64, int, int) ([]int64, int64, error)
	Earnings(context.Context, int64, int, int) (*RelayEarnings, error)
}

type RelayService struct {
	repo     RelayRepository
	accounts AccountRepository
	groups   GroupRepository
	users    UserRepository
	admin    AdminService
	keys     *APIKeyService
	billing  *BillingCacheService
}

func NewRelayService(repo RelayRepository, accounts AccountRepository, groups GroupRepository, users UserRepository, admin AdminService, keys *APIKeyService, billing *BillingCacheService) *RelayService {
	return &RelayService{repo: repo, accounts: accounts, groups: groups, users: users, admin: admin, keys: keys, billing: billing}
}

func (s *RelayService) Groups(ctx context.Context, userID int64) ([]Group, error) {
	groups, err := s.repo.ListPrivateGroups(ctx, userID)
	if err != nil || len(groups) > 0 {
		return groups, err
	}
	_, err = s.CreateGroup(ctx, userID, "My relay", PlatformComposite)
	if err != nil && !errors.Is(err, ErrGroupExists) {
		return nil, err
	}
	return s.repo.ListPrivateGroups(ctx, userID)
}

func (s *RelayService) CreateGroup(ctx context.Context, userID int64, name, platform string) (*Group, error) {
	name = strings.TrimSpace(name)
	if name == "" || len(fmt.Sprintf("u%d: %s", userID, name)) > 100 || len([]rune(name)) > 70 || !domain.IsGroupPlatform(platform) {
		return nil, infraerrors.BadRequest("INVALID_RELAY_GROUP", "a group name and supported platform are required")
	}
	g := &Group{OwnerUserID: &userID, Name: fmt.Sprintf("u%d: %s", userID, name), Platform: platform,
		IsExclusive: true, Status: StatusActive, SubscriptionType: SubscriptionTypeStandard,
		RateMultiplier: 0, SupplierRate: 0, ImageRateMultiplier: 0, VideoRateMultiplier: 0,
		AllowImageGeneration: true, AllowMessagesDispatch: true, AllowLive: true, SupportedModelScopes: []string{},
		DefaultValidityDays: 30, PeakRateMultiplier: 1}
	if err := s.groups.Create(ctx, g); err != nil {
		return nil, err
	}
	return g, nil
}

func (s *RelayService) ownedGroup(ctx context.Context, userID, id int64) (*Group, error) {
	g, err := s.groups.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if !g.IsPrivate() || *g.OwnerUserID != userID {
		return nil, ErrGroupNotFound
	}
	return g, nil
}

func (s *RelayService) UpdateGroup(ctx context.Context, userID, id int64, name, description, status string) (*Group, error) {
	g, err := s.ownedGroup(ctx, userID, id)
	if err != nil {
		return nil, err
	}
	name = strings.TrimSpace(name)
	if name == "" || len(fmt.Sprintf("u%d: %s", userID, name)) > 100 || len([]rune(name)) > 70 || (status != StatusActive && status != StatusDisabled && status != "inactive") {
		return nil, infraerrors.BadRequest("INVALID_RELAY_GROUP", "invalid group name or status")
	}
	g.Name = fmt.Sprintf("u%d: %s", userID, name)
	g.Description = description
	g.Status = status
	if err = s.groups.Update(ctx, g); err != nil {
		return nil, err
	}
	s.keys.InvalidateAuthCacheByGroupID(ctx, id)
	return g, nil
}

func (s *RelayService) DeleteGroup(ctx context.Context, userID, id int64) error {
	if _, err := s.ownedGroup(ctx, userID, id); err != nil {
		return err
	}
	return s.admin.DeleteGroup(ctx, id)
}

func (s *RelayService) Accounts(ctx context.Context, userID int64, page, size int) ([]*Account, int64, error) {
	ids, total, err := s.repo.ListOwnedAccountIDs(ctx, userID, page, size)
	if err != nil {
		return nil, 0, err
	}
	if len(ids) == 0 {
		return []*Account{}, total, nil
	}
	accounts, err := s.accounts.GetByIDs(ctx, ids)
	return accounts, total, err
}

func (s *RelayService) OwnedAccount(ctx context.Context, userID, id int64) (*Account, error) {
	a, err := s.accounts.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if a.OwnerUserID == nil || *a.OwnerUserID != userID {
		return nil, ErrAccountNotFound
	}
	return a, nil
}

func (s *RelayService) validateGroups(ctx context.Context, userID int64, platform, kind string, ids []int64) error {
	if len(ids) == 0 {
		return infraerrors.BadRequest("RELAY_GROUP_REQUIRED", "select at least one private or public group")
	}
	user, err := s.users.GetByID(ctx, userID)
	if err != nil {
		return err
	}
	seen := map[int64]bool{}
	for _, id := range ids {
		if seen[id] {
			return infraerrors.BadRequest("DUPLICATE_RELAY_GROUP", "duplicate group")
		}
		seen[id] = true
		g, err := s.groups.GetByID(ctx, id)
		if err != nil {
			return err
		}
		if !g.CanUse(user) || (!g.IsPrivate() && !g.IsPublicRelay()) {
			return ErrGroupNotAllowed
		}
		if !g.IsActive() || (g.Platform != platform && g.Platform != PlatformComposite) {
			return infraerrors.BadRequest("RELAY_GROUP_PLATFORM_MISMATCH", "group is disabled or incompatible with the account platform")
		}
		if g.RequireOAuthOnly && kind != AccountTypeOAuth && kind != AccountTypeSetupToken {
			return infraerrors.BadRequest("RELAY_GROUP_OAUTH_REQUIRED", "this group accepts only OAuth accounts")
		}
	}
	return nil
}

func ValidateRelayCredentials(kind string, credentials map[string]any) error {
	if kind != AccountTypeAPIKey && kind != AccountTypeOAuth && kind != AccountTypeSetupToken {
		return infraerrors.BadRequest("INVALID_RELAY_ACCOUNT_TYPE", "unsupported account credential type")
	}
	allowed := map[string]bool{"api_key": true, "base_url": true, "api_base_urls": true, "access_token": true, "refresh_token": true,
		"expires_at": true, "token_type": true, "scope": true, "email": true, "organization_id": true, "organization_uuid": true,
		"project_id": true, "model_mapping": true, "account_mode": true, "upstream_protocol": true, "auth_mode": true}
	for key := range credentials {
		if !allowed[key] {
			return infraerrors.BadRequest("INVALID_RELAY_CREDENTIAL_FIELD", "unsupported credential field: "+key)
		}
	}
	if raw, ok := credentials["base_url"]; ok {
		value, ok := raw.(string)
		if !ok {
			return infraerrors.BadRequest("INVALID_RELAY_URL", "base_url must be a string")
		}
		if strings.TrimSpace(value) != "" {
			if err := validateRelayURL(value); err != nil {
				return err
			}
		}
	}
	if raw, ok := credentials["api_base_urls"]; ok {
		urls, ok := raw.(map[string]any)
		if !ok {
			return infraerrors.BadRequest("INVALID_RELAY_URL", "api_base_urls must be an object")
		}
		for _, raw := range urls {
			v, ok := raw.(string)
			if !ok {
				return infraerrors.BadRequest("INVALID_RELAY_URL", "upstream URL must be a string")
			}
			if err := validateRelayURL(v); err != nil {
				return err
			}
		}
	}
	return nil
}

func validateRelayURL(raw string) error {
	normalized, err := urlvalidator.ValidateHTTPURL(raw, true, urlvalidator.ValidationOptions{AllowPrivate: false})
	if err != nil {
		return infraerrors.BadRequest("INVALID_RELAY_URL", "upstream URL must use a public HTTP(S) host")
	}
	u, err := url.Parse(normalized)
	if err != nil || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return infraerrors.BadRequest("INVALID_RELAY_URL", "upstream URL cannot contain user info, query, or fragment")
	}
	return nil
}

func (s *RelayService) CreateAccount(ctx context.Context, userID int64, input *CreateAccountInput) (*Account, error) {
	input.Name = strings.TrimSpace(input.Name)
	if input.Name == "" || len(input.Name) > 100 {
		return nil, infraerrors.BadRequest("INVALID_RELAY_ACCOUNT_NAME", "account name is missing or too long")
	}
	if !domain.IsConcretePlatform(input.Platform) {
		return nil, infraerrors.BadRequest("INVALID_RELAY_PLATFORM", "unsupported platform")
	}
	if err := ValidateRelayCredentials(input.Type, input.Credentials); err != nil {
		return nil, err
	}
	credentialKey := "access_token"
	if input.Type == AccountTypeAPIKey {
		credentialKey = "api_key"
	}
	value, _ := input.Credentials[credentialKey].(string)
	if strings.TrimSpace(value) == "" {
		return nil, infraerrors.BadRequest("RELAY_CREDENTIAL_REQUIRED", credentialKey+" is required")
	}
	if err := s.validateGroups(ctx, userID, input.Platform, input.Type, input.GroupIDs); err != nil {
		return nil, err
	}
	input.OwnerUserID = &userID
	input.ProxyID = nil
	input.Extra = nil
	input.Priority = 50
	input.SkipDefaultGroupBind = true
	return s.admin.CreateAccount(ctx, input)
}

func (s *RelayService) UpdateAccount(ctx context.Context, userID, id int64, input *UpdateAccountInput) (*Account, error) {
	a, err := s.OwnedAccount(ctx, userID, id)
	if err != nil {
		return nil, err
	}
	if input.Status != "" && input.Status != StatusActive && input.Status != StatusDisabled && input.Status != a.Status {
		return nil, infraerrors.BadRequest("INVALID_RELAY_ACCOUNT_STATUS", "invalid account status")
	}
	input.Name = strings.TrimSpace(input.Name)
	if input.Name == "" || len(input.Name) > 100 {
		return nil, infraerrors.BadRequest("INVALID_RELAY_ACCOUNT_NAME", "account name is missing or too long")
	}
	if input.Credentials != nil {
		if err = ValidateRelayCredentials(a.Type, input.Credentials); err != nil {
			return nil, err
		}
		merged := make(map[string]any, len(a.Credentials)+len(input.Credentials))
		for key, value := range a.Credentials {
			merged[key] = value
		}
		for key, value := range input.Credentials {
			merged[key] = value
		}
		input.Credentials = merged
	}
	if input.GroupIDs != nil {
		if err = s.validateGroups(ctx, userID, a.Platform, a.Type, *input.GroupIDs); err != nil {
			return nil, err
		}
	}
	input.Type = ""
	input.ProxyID = nil
	input.Extra = nil
	input.Priority = nil
	input.RateMultiplier = nil
	return s.admin.UpdateAccount(ctx, id, input)
}

func (s *RelayService) DeleteAccount(ctx context.Context, userID, id int64) error {
	if _, err := s.OwnedAccount(ctx, userID, id); err != nil {
		return err
	}
	return s.admin.DeleteAccount(ctx, id)
}

func (s *RelayService) Earnings(ctx context.Context, userID int64, page, size int) (*RelayEarnings, error) {
	out, err := s.repo.Earnings(ctx, userID, page, size)
	if err == nil && out.BalanceChanged {
		if s.billing != nil {
			_ = s.billing.InvalidateUserBalance(ctx, userID)
		}
		if s.keys != nil {
			s.keys.InvalidateAuthCacheByUserID(ctx, userID)
		}
	}
	return out, err
}

// User-supplied upstreams retain the public-host policy even when admin hosts allow private networks.
func RestrictRelayUpstreamRequest(req *http.Request, account *Account) {
	if req != nil && account != nil && account.OwnerUserID != nil {
		*req = *req.WithContext(WithHTTPUpstreamRedirectsDisabled(WithHTTPUpstreamPublicHostsOnly(req.Context())))
	}
}

type relayUsageContextKey struct{}

func WithRelayUsage(ctx context.Context, account *Account) context.Context {
	if account == nil || account.OwnerUserID == nil {
		return ctx
	}
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, relayUsageContextKey{}, true)
}

func IsRelayUsage(ctx context.Context) bool {
	return ctx != nil && ctx.Value(relayUsageContextKey{}) == true
}
