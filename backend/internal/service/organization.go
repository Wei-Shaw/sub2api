package service

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"net/mail"
	"regexp"
	"strings"
	"sync/atomic"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/usagestats"
	"github.com/shopspring/decimal"
	"golang.org/x/crypto/bcrypt"
)

const (
	IdentityTypeRoot = "root"
	IdentityTypeIAM  = "iam"

	OrganizationRoleOwner  = "owner"
	OrganizationRoleMember = "member"

	OrganizationStatusActive    = "active"
	OrganizationStatusSuspended = "suspended"

	MembershipStatusActive   = "active"
	MembershipStatusDisabled = "disabled"
	MembershipStatusArchived = "archived"

	PolicyCompanyFinanceReadOnly = "CompanyFinanceReadOnly"
	PolicyCompanySharedBalance   = "CompanySharedBalanceUse"
	PolicyCompanyFinanceManage   = "CompanyFinanceManage"
	PolicyIAMUserManage          = "IAMUserManage"
	ActionFinanceBalanceRead     = "organization.finance.balance.read"
	ActionSharedBalanceUse       = "organization.balance.shared.use"
	ActionBalanceAllocate        = "organization.balance.allocate"
	ActionSpendLimitManage       = "organization.spend_limit.manage"
	ActionSubscriptionManage     = "organization.subscription.manage"
	ActionIAMMemberManage        = "organization.iam.member.manage"
	IAMPrincipalDomain           = "opentk.ai"

	BalanceSourceSelf         = "self"
	BalanceSourceAllocated    = "allocated"
	BalanceSourceLegacyShared = "shared"
	BalanceSourceCompany      = "company"
	BalanceSourceSubscription = "subscription"
)

var (
	ErrCompanyFeatureDisabled = infraerrors.Forbidden("COMPANY_FEATURE_DISABLED", "company account features are disabled")
	ErrCompanyNotEligible     = infraerrors.Forbidden("COMPANY_NOT_ELIGIBLE", "account is not eligible for company upgrade")
	ErrCompanyPending         = infraerrors.Conflict("COMPANY_APPLICATION_PENDING", "a company application is already pending")
	ErrCompanyNotFound        = infraerrors.NotFound("COMPANY_NOT_FOUND", "company organization not found")
	ErrApplicationNotFound    = infraerrors.NotFound("COMPANY_APPLICATION_NOT_FOUND", "company application not found")
	ErrApplicationTerminal    = infraerrors.Conflict("COMPANY_APPLICATION_TERMINAL", "company application is already decided")
	ErrReasonRequired         = infraerrors.BadRequest("REJECTION_REASON_REQUIRED", "rejection reason is required")
	ErrCompanySizeInvalid     = infraerrors.BadRequest("COMPANY_SIZE_INVALID", "company size is required and must be one of the allowed ranges")
	ErrIAMFeatureDisabled     = infraerrors.Forbidden("IAM_FEATURE_DISABLED", "IAM user creation is disabled")
	ErrIAMMemberLimit         = infraerrors.Conflict("IAM_MEMBER_LIMIT", "organization IAM member limit reached")
	ErrIAMLoginName           = infraerrors.BadRequest("IAM_LOGIN_NAME_INVALID", "IAM login name is invalid")
	ErrIAMPassword            = infraerrors.BadRequest("IAM_PASSWORD_INVALID", "IAM password must be between 8 and 72 bytes")
	ErrIAMMemberNotFound      = infraerrors.NotFound("IAM_MEMBER_NOT_FOUND", "IAM member not found")
	ErrOrganizationPermission = infraerrors.Forbidden("ORGANIZATION_PERMISSION_DENIED", "organization permission denied")
	ErrOrganizationSuspended  = infraerrors.Conflict("ORGANIZATION_SUSPENDED", "organization is suspended")
	ErrIAMFinancialOperation  = infraerrors.Forbidden("IAM_FINANCIAL_OPERATION_DENIED", "IAM users cannot perform this financial operation")

	ErrSubscriptionGroupInvalid  = infraerrors.BadRequest("SUBSCRIPTION_GROUP_INVALID", "the subscription group is invalid or unavailable")
	ErrOrgSubscriptionExists     = infraerrors.Conflict("ORGANIZATION_SUBSCRIPTION_EXISTS", "an active subscription for this group already exists")
	ErrOrgSubscriptionNotFound   = infraerrors.NotFound("ORGANIZATION_SUBSCRIPTION_NOT_FOUND", "organization subscription not found")
	ErrSubscriptionValidityRange = infraerrors.BadRequest("SUBSCRIPTION_VALIDITY_INVALID", "validity days must be between 0 and 3650")
	ErrSpendLimitInvalid         = infraerrors.BadRequest("ORGANIZATION_SPEND_LIMIT_INVALID", "a positive daily or monthly spend limit is required")
	ErrSpendLimitExceeded        = infraerrors.Conflict("ORGANIZATION_SPEND_LIMIT_EXCEEDED", "organization spend limit would be exceeded")
	ErrDailySpendLimitExceeded   = infraerrors.Conflict(
		"ORGANIZATION_DAILY_SPEND_LIMIT_EXCEEDED",
		"organization daily spend limit would be exceeded",
	).WithCause(ErrSpendLimitExceeded).WithMetadata(map[string]string{"period": "daily"})
	ErrMonthlySpendLimitExceeded = infraerrors.Conflict(
		"ORGANIZATION_MONTHLY_SPEND_LIMIT_EXCEEDED",
		"organization monthly spend limit would be exceeded",
	).WithCause(ErrSpendLimitExceeded).WithMetadata(map[string]string{"period": "monthly"})
	ErrDailyAndMonthlySpendLimitExceeded = infraerrors.Conflict(
		"ORGANIZATION_DAILY_AND_MONTHLY_SPEND_LIMIT_EXCEEDED",
		"organization daily and monthly spend limits would be exceeded",
	).WithCause(ErrSpendLimitExceeded).WithMetadata(map[string]string{"period": "daily,monthly"})
	ErrSpendLimitThreshold = infraerrors.BadRequest("ORGANIZATION_SPEND_ALERT_THRESHOLD_INVALID", "alert threshold must be between 1 and 100 percent")
)

var iamLoginNamePattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9._-]{0,63}$`)

// companyIDPattern matches a company identifier: a leading 'c' prefix followed
// by 15 digits (first digit 1-9), e.g. "c123456789012345". IAM principals use
// the company id as the login suffix.
var companyIDPattern = regexp.MustCompile(`^c[1-9][0-9]{14}$`)

func CanonicalIAMPrincipal(loginName, companyID string) string {
	return strings.TrimSpace(loginName) + "@" + strings.TrimSpace(companyID) + "." + IAMPrincipalDomain
}

func parseIAMPrincipal(principal string) (string, string, bool) {
	loginName, host, found := strings.Cut(strings.TrimSpace(principal), "@")
	suffix := "." + IAMPrincipalDomain
	if !found || strings.Contains(host, "@") || !strings.HasSuffix(strings.ToLower(host), suffix) {
		return "", "", false
	}
	companyID := host[:len(host)-len(suffix)]
	if !iamLoginNamePattern.MatchString(loginName) || !companyIDPattern.MatchString(companyID) {
		return "", "", false
	}
	return loginName, companyID, true
}

var organizationRuntimeMetrics struct {
	payerResolutionFailures atomic.Uint64
	deniedIAMFinancialOps   atomic.Uint64
}

type OrganizationRuntimeMetrics struct {
	PayerResolutionFailures uint64 `json:"payer_resolution_failures"`
	DeniedIAMFinancialOps   uint64 `json:"denied_iam_financial_operations"`
}

func CurrentOrganizationRuntimeMetrics() OrganizationRuntimeMetrics {
	return OrganizationRuntimeMetrics{
		PayerResolutionFailures: organizationRuntimeMetrics.payerResolutionFailures.Load(),
		DeniedIAMFinancialOps:   organizationRuntimeMetrics.deniedIAMFinancialOps.Load(),
	}
}

type OrganizationContext struct {
	OrganizationID     int64     `json:"organization_id"`
	AccountID          string    `json:"account_id"`
	CompanyID          string    `json:"company_id"`
	OwnerUserID        int64     `json:"owner_user_id"`
	CompanyName        string    `json:"company_name"`
	OrganizationStatus string    `json:"organization_status"`
	MembershipID       int64     `json:"membership_id"`
	Role               string    `json:"role"`
	MembershipStatus   string    `json:"membership_status"`
	AuthzGeneration    int64     `json:"authz_generation"`
	PolicyNames        []string  `json:"policy_names"`
	Actions            []string  `json:"actions"`
	EffectiveAt        time.Time `json:"effective_at"`
}

func (c *OrganizationContext) Active() bool {
	return c != nil && c.OrganizationStatus == OrganizationStatusActive && c.MembershipStatus == MembershipStatusActive
}

func (c *OrganizationContext) Owner() bool { return c != nil && c.Role == OrganizationRoleOwner }

func (c *OrganizationContext) HasAction(action string) bool {
	if c == nil {
		return false
	}
	if c.Owner() {
		return true
	}
	for _, candidate := range c.Actions {
		if candidate == action {
			return true
		}
	}
	return false
}

type CompanyApplication struct {
	ID              int64      `json:"id"`
	ApplicantUserID int64      `json:"applicant_user_id"`
	ApplicantEmail  string     `json:"applicant_email,omitempty"`
	RequestedName   string     `json:"requested_name"`
	CompanySize     string     `json:"company_size"`
	Status          string     `json:"status"`
	FeeAmount       string     `json:"fee_amount"`
	FeeCurrency     string     `json:"fee_currency"`
	ReviewerUserID  *int64     `json:"reviewer_user_id,omitempty"`
	ReviewReason    string     `json:"review_reason,omitempty"`
	OrganizationID  *int64     `json:"organization_id,omitempty"`
	SimilarNames    []string   `json:"similar_names"`
	CreatedAt       time.Time  `json:"created_at"`
	DecidedAt       *time.Time `json:"decided_at,omitempty"`
}

type CompanyUpgradeEligibility struct {
	Eligible    bool                `json:"eligible"`
	Reason      string              `json:"reason,omitempty"`
	FeeAmount   string              `json:"fee_amount"`
	FeeCurrency string              `json:"fee_currency"`
	Application *CompanyApplication `json:"application,omitempty"`
}

type OrganizationAuditEvent struct {
	ID            int64          `json:"id"`
	ActorUserID   *int64         `json:"actor_user_id,omitempty"`
	SubjectUserID *int64         `json:"subject_user_id,omitempty"`
	Action        string         `json:"action"`
	Result        string         `json:"result"`
	CorrelationID string         `json:"correlation_id,omitempty"`
	Metadata      map[string]any `json:"metadata"`
	CreatedAt     time.Time      `json:"created_at"`
}

type CompanyApplicationDetail struct {
	Application CompanyApplication       `json:"application"`
	Audit       []OrganizationAuditEvent `json:"audit"`
}

type OrganizationNameChangeRequest struct {
	ID              int64      `json:"id"`
	OrganizationID  int64      `json:"organization_id"`
	ApplicantUserID int64      `json:"applicant_user_id"`
	CompanyName     string     `json:"company_name"`
	OldName         string     `json:"old_name"`
	NewName         string     `json:"new_name"`
	Status          string     `json:"status"`
	ReviewerUserID  *int64     `json:"reviewer_user_id,omitempty"`
	ReviewReason    string     `json:"review_reason,omitempty"`
	SimilarNames    []string   `json:"similar_names"`
	CreatedAt       time.Time  `json:"created_at"`
	DecidedAt       *time.Time `json:"decided_at,omitempty"`
}

type AdminOrganization struct {
	ID          int64     `json:"id"`
	AccountID   string    `json:"account_id"`
	CompanyID   string    `json:"company_id"`
	Name        string    `json:"name"`
	Status      string    `json:"status"`
	OwnerUserID int64     `json:"owner_user_id"`
	OwnerEmail  string    `json:"owner_email,omitempty"`
	MemberCount int       `json:"member_count"`
	MemberLimit int       `json:"member_limit"`
	EffectiveAt time.Time `json:"effective_at"`
	CreatedAt   time.Time `json:"created_at"`
}

type AdminOrganizationDetail struct {
	Organization AdminOrganization        `json:"organization"`
	Audit        []OrganizationAuditEvent `json:"audit"`
}

type IAMMember struct {
	UserID             int64      `json:"user_id"`
	AccountID          string     `json:"account_id"`
	Username           string     `json:"username"`
	LoginName          string     `json:"login_name"`
	Principal          string     `json:"principal"`
	Status             string     `json:"status"`
	Balance            string     `json:"balance"`
	FrozenBalance      string     `json:"frozen_balance"`
	RecoveryEmail      string     `json:"recovery_email,omitempty"`
	RecoveryVerifiedAt *time.Time `json:"recovery_email_verified_at,omitempty"`
	MustChangePassword bool       `json:"must_change_password"`
	PolicyNames        []string   `json:"policy_names"`
	CreatedAt          time.Time  `json:"created_at"`
}

type ManagedPolicyView struct {
	ID          int64    `json:"id"`
	Key         string   `json:"key"`
	DisplayName string   `json:"display_name"`
	Type        string   `json:"type"`
	Description string   `json:"description"`
	Version     int      `json:"version"`
	Actions     []string `json:"actions"`
}

type FinanceSummary struct {
	BalanceSource string  `json:"balance_source"`
	Available     *string `json:"available,omitempty"`
	Frozen        *string `json:"frozen,omitempty"`
	Total         *string `json:"total,omitempty"`
	// Company balance is only populated for privileged viewers (the owner or an
	// account holding organization.finance.balance.read). It is independent from
	// the owner's personal balance above.
	CompanyAvailable *string `json:"company_available,omitempty"`
	CompanyFrozen    *string `json:"company_frozen,omitempty"`
	CompanyTotal     *string `json:"company_total,omitempty"`
}

// OrganizationSubscription is a subscription plan (group) held by a company,
// independent from any individual user's subscription. Quota limits are read
// from the referenced group; the usage counters below track the current
// sliding windows. Enterprise API keys bind to one of these subscriptions.
type OrganizationSubscription struct {
	ID               int64     `json:"id"`
	OrganizationID   int64     `json:"organization_id"`
	OrganizationName string    `json:"organization_name,omitempty"`
	CompanyID        string    `json:"company_id,omitempty"`
	GroupID          int64     `json:"group_id"`
	GroupName        string    `json:"group_name"`
	Platform         string    `json:"platform"`
	SubscriptionType string    `json:"subscription_type"`
	RateMultiplier   float64   `json:"rate_multiplier"`
	StartsAt         time.Time `json:"starts_at"`
	ExpiresAt        time.Time `json:"expires_at"`
	Status           string    `json:"status"`
	DailyLimitUSD    *string   `json:"daily_limit_usd,omitempty"`
	WeeklyLimitUSD   *string   `json:"weekly_limit_usd,omitempty"`
	MonthlyLimitUSD  *string   `json:"monthly_limit_usd,omitempty"`
	DailyUsageUSD    string    `json:"daily_usage_usd"`
	WeeklyUsageUSD   string    `json:"weekly_usage_usd"`
	MonthlyUsageUSD  string    `json:"monthly_usage_usd"`
	Notes            string    `json:"notes,omitempty"`
	AssignedBy       *int64    `json:"assigned_by,omitempty"`
	AssignedAt       time.Time `json:"assigned_at"`
	CreatedAt        time.Time `json:"created_at"`
}

// OrgSubscriptionRuntime is the internal, billing-oriented view of a company
// subscription. It mirrors UserSubscription's rolling-window semantics
// (daily = 24h, weekly = 7*24h, monthly = 30*24h) so enterprise API keys behave
// consistently with personal subscriptions. Limits are resolved from the group.
type OrgSubscriptionRuntime struct {
	ID             int64
	OrganizationID int64
	GroupID        int64
	Status         string
	StartsAt       time.Time
	ExpiresAt      time.Time

	DailyWindowStart   *time.Time
	WeeklyWindowStart  *time.Time
	MonthlyWindowStart *time.Time

	DailyUsageUSD   float64
	WeeklyUsageUSD  float64
	MonthlyUsageUSD float64

	DailyLimitUSD   *float64
	WeeklyLimitUSD  *float64
	MonthlyLimitUSD *float64
}

// IsActive reports whether the subscription is usable right now.
func (s *OrgSubscriptionRuntime) IsActive() bool {
	return s != nil && s.Status == SubscriptionStatusActive && time.Now().Before(s.ExpiresAt)
}

func (s *OrgSubscriptionRuntime) needsDailyReset(now time.Time) bool {
	return s.DailyWindowStart != nil && !now.Before(s.DailyWindowStart.Add(24*time.Hour))
}

func (s *OrgSubscriptionRuntime) needsWeeklyReset(now time.Time) bool {
	return s.WeeklyWindowStart != nil && now.Sub(*s.WeeklyWindowStart) >= 7*24*time.Hour
}

func (s *OrgSubscriptionRuntime) needsMonthlyReset(now time.Time) bool {
	return s.MonthlyWindowStart != nil && now.Sub(*s.MonthlyWindowStart) >= 30*24*time.Hour
}

// effectiveUsage returns current usage treating expired windows as reset to 0.
func (s *OrgSubscriptionRuntime) effectiveUsage(now time.Time) (daily, weekly, monthly float64) {
	daily, weekly, monthly = s.DailyUsageUSD, s.WeeklyUsageUSD, s.MonthlyUsageUSD
	if s.needsDailyReset(now) {
		daily = 0
	}
	if s.needsWeeklyReset(now) {
		weekly = 0
	}
	if s.needsMonthlyReset(now) {
		monthly = 0
	}
	return
}

// hasLimit 与上游 Group.HasDailyLimit 语义保持一致：nil 或 <=0 视为无限制。
// 注意历史脏数据可能把"无限制"误存为数字 0，这里统一按无限制处理，避免误拒。
func hasOrgSpendLimit(limit *float64) bool {
	return limit != nil && *limit > 0
}

// CheckAllLimits reports whether adding additionalCost keeps each configured
// limit satisfied (window-aware). A nil limit, or a limit <= 0, means unlimited
// (consistent with upstream Group.HasDailyLimit).
func (s *OrgSubscriptionRuntime) CheckAllLimits(additionalCost float64) (daily, weekly, monthly bool) {
	now := time.Now()
	du, wu, mu := s.effectiveUsage(now)
	daily = !hasOrgSpendLimit(s.DailyLimitUSD) || du+additionalCost <= *s.DailyLimitUSD
	weekly = !hasOrgSpendLimit(s.WeeklyLimitUSD) || wu+additionalCost <= *s.WeeklyLimitUSD
	monthly = !hasOrgSpendLimit(s.MonthlyLimitUSD) || mu+additionalCost <= *s.MonthlyLimitUSD
	return
}

type BillingContext struct {
	ConsumerUserID  int64
	OrganizationID  *int64
	PayerUserID     int64
	BalanceSource   string
	AuthzGeneration int64
}

func (c *BillingContext) UsesCompanyBalance() bool {
	return c != nil && c.BalanceSource == BalanceSourceCompany
}

// OrganizationBalanceRepository provides transaction-aware access to the
// organization's independent wallet. It is separate from OrganizationRepository
// so lightweight test repositories remain source-compatible.
type OrganizationBalanceRepository interface {
	GetOrganizationBalance(ctx context.Context, organizationID int64) (float64, error)
	DeductOrganizationBalance(ctx context.Context, organizationID int64, amount float64) (float64, error)
	CreditOrganizationBalance(ctx context.Context, organizationID int64, amount float64) (float64, error)
}

// OrganizationSpendLimitRule is an owner-managed company-sponsored spend cap.
// MemberUserID nil denotes the all-member default rule.
type OrganizationSpendLimitRule struct {
	ID                   int64     `json:"id"`
	OrganizationID       int64     `json:"organization_id"`
	MemberUserID         *int64    `json:"member_user_id,omitempty"`
	MemberLogin          string    `json:"member_login,omitempty"`
	MemberUsername       string    `json:"member_username,omitempty"`
	DailyLimitUSD        *string   `json:"daily_limit_usd,omitempty"`
	MonthlyLimitUSD      *string   `json:"monthly_limit_usd,omitempty"`
	AlertEnabled         bool      `json:"alert_enabled"`
	AlertThresholdPct    float64   `json:"alert_threshold_pct"`
	AdditionalRecipients []string  `json:"additional_recipients"`
	Revision             int64     `json:"revision"`
	CreatedAt            time.Time `json:"created_at"`
	UpdatedAt            time.Time `json:"updated_at"`
}

type OrganizationSpendUsage struct {
	MemberUserID    int64   `json:"member_user_id"`
	MemberLogin     string  `json:"member_login"`
	MemberUsername  string  `json:"member_username"`
	DailyUsedUSD    string  `json:"daily_used_usd"`
	MonthlyUsedUSD  string  `json:"monthly_used_usd"`
	DailyLimitUSD   *string `json:"daily_limit_usd,omitempty"`
	MonthlyLimitUSD *string `json:"monthly_limit_usd,omitempty"`
}

// OrganizationSpendLimitRepository is optional so older repository test stubs
// remain source-compatible while the feature is rolled out.
type OrganizationSpendLimitRepository interface {
	ListSpendLimitRules(ctx context.Context, actorID int64) ([]OrganizationSpendLimitRule, error)
	UpsertSpendLimitRules(ctx context.Context, ownerID int64, memberIDs []int64, daily, monthly *string, alertEnabled bool, threshold float64, recipients []string) ([]OrganizationSpendLimitRule, error)
	DeleteSpendLimitRule(ctx context.Context, ownerID int64, memberID *int64) error
	ListSpendLimitUsage(ctx context.Context, actorID int64) ([]OrganizationSpendUsage, error)
	CheckOrganizationSpendLimit(ctx context.Context, consumerUserID int64, balanceSource string, amount float64) error
	RecordSpendLimitAlert(ctx context.Context, consumerUserID int64, balanceSource string) error
}

// OrganizationSettings holds company-wide feature toggles. Extend with more
// fields as future switches are introduced; new columns must be nullable /
// have a default so hot-path callers stay backwards compatible.
type OrganizationSettings struct {
	OrganizationID         int64     `json:"organization_id"`
	AutoSwitchSubscription bool      `json:"auto_switch_subscription"`
	CreatedAt              time.Time `json:"created_at"`
	UpdatedAt              time.Time `json:"updated_at"`
}

// OrganizationSettingsRepository exposes CRUD for organization-level feature
// toggles. Implemented on top of the `organization_settings` table. Optional
// so older repository stubs used in tests remain source-compatible while the
// feature is rolled out.
type OrganizationSettingsRepository interface {
	// GetOrganizationSettings returns the settings row for the caller's
	// organization. When no row exists yet the repository returns a zero-value
	// struct with defaults so callers can render / consume it uniformly.
	GetOrganizationSettings(ctx context.Context, actorID int64) (*OrganizationSettings, error)
	// UpsertOrganizationSettings creates or updates the settings row. Access
	// control is enforced inside the repository (owner or holder of
	// CompanyFinanceManage).
	UpsertOrganizationSettings(ctx context.Context, actorID int64, settings OrganizationSettings) (*OrganizationSettings, error)
	// GetOrganizationSettingsByID is the internal, request-hot-path variant
	// used by the auth middleware to decide whether to fall over to another
	// subscription. It does not enforce user-level ACLs because the caller has
	// already been authenticated via the API key.
	GetOrganizationSettingsByID(ctx context.Context, organizationID int64) (*OrganizationSettings, error)
	// ListFallbackCandidateSubscriptions returns other active organization
	// subscriptions on the same platform as `currentSubscriptionID`, sorted by
	// `starts_at` ascending. It excludes the current subscription itself. Used
	// both by the auth middleware fallback and the API Key list UI so the
	// candidate chain shown to users matches what would actually be selected.
	ListFallbackCandidateSubscriptions(ctx context.Context, organizationID, currentSubscriptionID int64) ([]OrganizationSubscription, error)
	// ResolveNextOrganizationSubscription picks the first fallback candidate
	// that is currently under all its limits (i.e. the next-usable plan). It
	// returns ErrOrgSubscriptionNotFound when no candidate qualifies.
	ResolveNextOrganizationSubscription(ctx context.Context, organizationID, currentSubscriptionID int64) (*OrgSubscriptionRuntime, error)
}

type OrganizationUsageFilter struct {
	UsageID     *int64
	Start       time.Time
	End         time.Time
	MemberID    *int64
	APIKeyID    *int64
	GroupID     *int64
	BillingType *int8
	BillingMode string
	Model       string
	Endpoint    string
	Status      string
	Granularity string
	Page        int
	PageSize    int
}

type OrganizationUsageRow struct {
	ID                    int64          `json:"id"`
	MemberUserID          int64          `json:"member_user_id"`
	MemberLogin           string         `json:"member_login"`
	MemberUsername        string         `json:"member_username"`
	APIKeyName            string         `json:"api_key_name"`
	Model                 string         `json:"model"`
	InputTokens           int            `json:"input_tokens"`
	OutputTokens          int            `json:"output_tokens"`
	CacheCreationTokens   int            `json:"cache_creation_tokens"`
	CacheReadTokens       int            `json:"cache_read_tokens"`
	CacheCreation5mTokens int            `json:"cache_creation_5m_tokens"`
	CacheCreation1hTokens int            `json:"cache_creation_1h_tokens"`
	InputCost             float64        `json:"input_cost"`
	OutputCost            float64        `json:"output_cost"`
	CacheCreationCost     float64        `json:"cache_creation_cost"`
	CacheReadCost         float64        `json:"cache_read_cost"`
	ActualCost            string         `json:"actual_cost"`
	TotalCost             string         `json:"total_cost"`
	RateMultiplier        float64        `json:"rate_multiplier"`
	Endpoint              string         `json:"endpoint"`
	GroupID               *int64         `json:"group_id,omitempty"`
	GroupName             string         `json:"group_name"`
	RequestType           string         `json:"request_type"`
	BillingType           int8           `json:"billing_type"`
	BillingMode           string         `json:"billing_mode"`
	ImageCount            int            `json:"image_count"`
	ImageInputTokens      int            `json:"image_input_tokens"`
	ImageInputCost        float64        `json:"image_input_cost"`
	ImageOutputTokens     int            `json:"image_output_tokens"`
	ImageOutputCost       float64        `json:"image_output_cost"`
	ImageSize             *string        `json:"image_size,omitempty"`
	ImageInputSize        *string        `json:"image_input_size,omitempty"`
	ImageOutputSize       *string        `json:"image_output_size,omitempty"`
	ImageSizeSource       *string        `json:"image_size_source,omitempty"`
	ImageSizeBreakdown    map[string]int `json:"image_size_breakdown,omitempty"`
	VideoCount            int            `json:"video_count"`
	VideoResolution       *string        `json:"video_resolution,omitempty"`
	VideoDurationSeconds  *int           `json:"video_duration_seconds,omitempty"`
	ImageURLs             []string       `json:"image_urls"`
	CosURLs               []string       `json:"cos_urls"`
	IPAddress             string         `json:"ip_address"`
	UserAgent             string         `json:"user_agent"`
	Status                string         `json:"status"`
	FirstTokenMS          *int           `json:"first_token_ms"`
	DurationMS            *int           `json:"duration_ms,omitempty"`
	CreatedAt             time.Time      `json:"created_at"`
	BalanceSource         string         `json:"balance_source"`
	// TaskID 关联的 async_video_tasks.id。仅视频计费行（billing_mode='video'）会有值。
	// 前端据此在使用记录行渲染"详情"按钮，点击弹窗调用视频任务详情接口。
	TaskID *int64 `json:"task_id,omitempty"`
}

type OrganizationUsageStats struct {
	Requests            int64  `json:"requests"`
	InputTokens         int64  `json:"input_tokens"`
	OutputTokens        int64  `json:"output_tokens"`
	CacheCreationTokens int64  `json:"cache_creation_tokens"`
	CacheReadTokens     int64  `json:"cache_read_tokens"`
	TotalTokens         int64  `json:"total_tokens"`
	ActualCost          string `json:"actual_cost"`
}

type OrganizationAPIKeyOption struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

type OrganizationUsageTrendPoint struct {
	Date                string  `json:"date"`
	Requests            int64   `json:"requests"`
	InputTokens         int64   `json:"input_tokens"`
	OutputTokens        int64   `json:"output_tokens"`
	CacheCreationTokens int64   `json:"cache_creation_tokens"`
	CacheReadTokens     int64   `json:"cache_read_tokens"`
	TotalTokens         int64   `json:"total_tokens"`
	Cost                float64 `json:"cost"`
	ActualCost          float64 `json:"actual_cost"`
}

type OrganizationModelStat struct {
	Model               string  `json:"model"`
	Requests            int64   `json:"requests"`
	InputTokens         int64   `json:"input_tokens"`
	OutputTokens        int64   `json:"output_tokens"`
	CacheCreationTokens int64   `json:"cache_creation_tokens"`
	CacheReadTokens     int64   `json:"cache_read_tokens"`
	TotalTokens         int64   `json:"total_tokens"`
	Cost                float64 `json:"cost"`
	ActualCost          float64 `json:"actual_cost"`
}

type OrganizationGroupStat struct {
	GroupID     int64   `json:"group_id"`
	GroupName   string  `json:"group_name"`
	Requests    int64   `json:"requests"`
	TotalTokens int64   `json:"total_tokens"`
	Cost        float64 `json:"cost"`
	ActualCost  float64 `json:"actual_cost"`
}

type OrganizationEndpointStat struct {
	Endpoint    string  `json:"endpoint"`
	Requests    int64   `json:"requests"`
	TotalTokens int64   `json:"total_tokens"`
	Cost        float64 `json:"cost"`
	ActualCost  float64 `json:"actual_cost"`
}

type OrganizationUsageCharts struct {
	Trend     []OrganizationUsageTrendPoint `json:"trend"`
	Models    []OrganizationModelStat       `json:"models"`
	Groups    []OrganizationGroupStat       `json:"groups"`
	Endpoints []OrganizationEndpointStat    `json:"endpoints"`
}

type OrganizationRepository interface {
	GetContextForUser(ctx context.Context, userID int64) (*OrganizationContext, error)
	GetApplicationForUser(ctx context.Context, userID int64) (*CompanyApplication, error)
	GetApplication(ctx context.Context, applicationID int64) (*CompanyApplicationDetail, error)
	SubmitApplication(ctx context.Context, userID int64, name, normalizedName, companySize, idempotencyKey, fee, currency string) (*CompanyApplication, error)
	WithdrawApplication(ctx context.Context, userID, applicationID int64) (*CompanyApplication, error)
	ListApplications(ctx context.Context, status string, page, pageSize int) ([]CompanyApplication, int64, error)
	ListNameChangeRequests(ctx context.Context, status string, page, pageSize int) ([]OrganizationNameChangeRequest, int64, error)
	GetNameChangeRequest(ctx context.Context, requestID int64) (*OrganizationNameChangeRequest, error)
	ListOrganizations(ctx context.Context, actorID int64, status string, page, pageSize int) ([]AdminOrganization, int64, error)
	GetOrganization(ctx context.Context, actorID, organizationID int64) (*AdminOrganizationDetail, error)
	DecideApplication(ctx context.Context, reviewerID, applicationID int64, approve bool, reason string, memberLimit int) (*CompanyApplication, error)
	RequestNameChange(ctx context.Context, userID int64, name, normalizedName string) error
	DecideNameChange(ctx context.Context, reviewerID, requestID int64, approve bool, reason string) error
	SetOrganizationStatus(ctx context.Context, actorID, organizationID int64, status string) error
	CreateIAMMember(ctx context.Context, ownerID int64, user *User, memberLimit int) (*IAMMember, error)
	ListIAMMembers(ctx context.Context, actorID int64) ([]IAMMember, int, error)
	GetIAMMember(ctx context.Context, actorID, memberUserID int64) (*IAMMember, error)
	SetIAMMemberStatus(ctx context.Context, ownerID, memberUserID int64, status string) error
	UpdateIAMPassword(ctx context.Context, actorID, memberUserID int64, passwordHash string, requireChange bool) error
	FindIAMByPrincipal(ctx context.Context, loginName, companyID string) (*User, *OrganizationContext, error)
	ListPolicies(ctx context.Context, actorID int64) ([]ManagedPolicyView, error)
	ListMemberPolicyAttachments(ctx context.Context, ownerID, memberUserID int64) ([]ManagedPolicyView, error)
	SetPolicyAttachment(ctx context.Context, ownerID, memberUserID int64, policyKey string, attach bool, correlationID string) error
	TransferBalance(ctx context.Context, ownerID, memberUserID int64, amount, idempotencyKey string, reclaim bool) error
	DepositToCompany(ctx context.Context, ownerID int64, amount, idempotencyKey string, withdraw bool) error
	CreateOrganizationSubscription(ctx context.Context, userID, groupID int64, validityDays int, notes string) (*OrganizationSubscription, error)
	AdminCreateOrganizationSubscription(ctx context.Context, actorID, organizationID, groupID int64, validityDays int, notes string) (*OrganizationSubscription, error)
	AdminListOrganizationSubscriptions(ctx context.Context, actorID int64, page, pageSize int, groupID *int64, status, platform, sortBy, sortOrder string) ([]OrganizationSubscription, int64, error)
	AdminExtendOrganizationSubscription(ctx context.Context, actorID, subscriptionID int64, days int) error
	AdminResetOrganizationSubscriptionQuota(ctx context.Context, actorID, subscriptionID int64) error
	AdminRevokeOrganizationSubscription(ctx context.Context, actorID, subscriptionID int64) error
	// AssignOrExtendOrganizationSubscription provisions or extends a company
	// subscription during paid-order fulfillment. It operates directly on the
	// company (orgID) rather than resolving an owner, and is idempotent on
	// orderID so webhook retries do not double-provision.
	AssignOrExtendOrganizationSubscription(ctx context.Context, orgID, groupID int64, validityDays int, orderID int64) error
	ListOrganizationSubscriptions(ctx context.Context, userID int64) ([]OrganizationSubscription, error)
	CancelOrganizationSubscription(ctx context.Context, userID, subscriptionID int64) error
	// ListActiveOrganizationSubscriptionsForMember returns the active, non-expired
	// company subscriptions that the given user (as an active member of the org)
	// may bind an enterprise API key to.
	ListActiveOrganizationSubscriptionsForMember(ctx context.Context, userID int64) ([]OrganizationSubscription, error)
	// GetActiveOrganizationSubscriptionForMember validates that the user is an
	// active member of the organization owning the subscription and that the
	// subscription is active and not expired, then returns it.
	GetActiveOrganizationSubscriptionForMember(ctx context.Context, userID, subscriptionID int64) (*OrganizationSubscription, error)
	// GetOrganizationSubscriptionForBilling loads a company subscription's usage
	// windows, counters and group limits for request-time validation and billing.
	GetOrganizationSubscriptionForBilling(ctx context.Context, subscriptionID int64) (*OrgSubscriptionRuntime, error)
	// GetOrganizationSubscriptionOrganizationID returns the organization_id of
	// the given subscription regardless of its lifecycle state (allows soft-
	// deleted, expired, or cancelled rows). Used purely as an ACL helper for
	// views that must remain visible after a subscription is revoked/cleaned.
	// Returns ErrOrgSubscriptionNotFound only when the id does not exist at all.
	GetOrganizationSubscriptionOrganizationID(ctx context.Context, subscriptionID int64) (int64, error)
	// IncrementOrganizationSubscriptionUsage atomically adds costUSD to the
	// subscription's daily/weekly/monthly usage counters (window-aware).
	IncrementOrganizationSubscriptionUsage(ctx context.Context, subscriptionID int64, costUSD float64) error
	FinanceSummary(ctx context.Context, userID int64) (*FinanceSummary, error)
	// ListAuditEvents returns paginated audit records for an organization, optionally
	// filtered by a coarse category (recharge / authorize / allocate / spend_limit).
	// Owner-only access is enforced in the service layer.
	ListAuditEvents(ctx context.Context, organizationID int64, filter OrganizationAuditFilter) ([]OrganizationAuditLogEntry, int64, error)
	ListUsage(ctx context.Context, userID int64, filter OrganizationUsageFilter) ([]OrganizationUsageRow, int64, error)
	UsageStats(ctx context.Context, userID int64, filter OrganizationUsageFilter) (*OrganizationUsageStats, error)
	UsageTrend(ctx context.Context, userID int64, filter OrganizationUsageFilter) ([]OrganizationUsageTrendPoint, error)
	UsageCharts(ctx context.Context, userID int64, filter OrganizationUsageFilter) (*OrganizationUsageCharts, error)
	OrganizationDashboard(ctx context.Context, userID int64) (*usagestats.DashboardStats, error)
	OrganizationSpendingRanking(ctx context.Context, userID int64, filter OrganizationUsageFilter, limit int) (*usagestats.UserSpendingRankingResponse, error)
	OrganizationUserBreakdown(ctx context.Context, userID int64, filter OrganizationUsageFilter, limit int) ([]usagestats.UserBreakdownItem, error)
	OrganizationUsersTrend(ctx context.Context, userID int64, filter OrganizationUsageFilter, limit int) ([]usagestats.UserUsageTrendPoint, error)
	SearchOrganizationAPIKeys(ctx context.Context, userID int64, memberID *int64, query string, limit int) ([]OrganizationAPIKeyOption, error)
	ResolveBillingContext(ctx context.Context, consumerUserID int64, requiredAmount float64) (*BillingContext, error)
	Reconcile(ctx context.Context) (map[string]int64, error)
	ListOrganizationUserIDs(ctx context.Context, organizationID int64) ([]int64, error)
}

// CompanyUpgradeChargeReader reports whether company upgrade should charge the
// upgrade fee / freeze funds. Injected from the settings layer.
type CompanyUpgradeChargeReader interface {
	IsCompanyUpgradeChargeEnabled(ctx context.Context) bool
}

type CompanyUpgradeFeeReader interface {
	GetCompanyUpgradeFee(ctx context.Context) float64
}

type CompanyFeatureReader interface {
	IsCompanyApplicationsEnabled(ctx context.Context) bool
	IsCompanyIAMEnabled(ctx context.Context) bool
}

// SubscriptionGroupLister lists all active groups so the organization service
// can surface subscription-type plans available for the owner to subscribe to.
type SubscriptionGroupLister interface {
	ListActive(ctx context.Context) ([]Group, error)
}

// SubscriptionOrderCreator places a subscription payment order through the
// standard payment gateway flow. The organization service delegates enterprise
// subscription purchases to it after resolving and authorizing the company
// context, so that the full personal payment pipeline (gateway selection, QR
// code, polling, webhook fulfillment) is reused verbatim — only the order now
// carries an OrganizationID and is fulfilled onto the company subject.
type SubscriptionOrderCreator interface {
	CreateOrder(ctx context.Context, req CreateOrderRequest) (*CreateOrderResponse, error)
}

// OrganizationSubscriptionOrderInput carries the request-scoped fields needed
// to place an enterprise subscription order. Everything except PlanID mirrors
// the personal CreateOrder request and is populated by the handler from the
// gin context / request body.
type OrganizationSubscriptionOrderInput struct {
	PlanID          int64
	PaymentType     string
	OpenID          string
	ClientIP        string
	IsMobile        bool
	IsWeChatBrowser bool
	SrcHost         string
	SrcURL          string
	ReturnURL       string
	PaymentSource   string
	Locale          string
}

type OrganizationService struct {
	repo          OrganizationRepository
	userRepo      UserRepository
	cfg           *config.Config
	authCache     APIKeyAuthCacheInvalidator
	chargeReader  CompanyUpgradeChargeReader
	feeReader     CompanyUpgradeFeeReader
	featureReader CompanyFeatureReader
	groupLister   SubscriptionGroupLister
	orderCreator  SubscriptionOrderCreator
}

func (s *OrganizationService) SetAuthCacheInvalidator(invalidator APIKeyAuthCacheInvalidator) {
	if s != nil {
		s.authCache = invalidator
	}
}

// SetSubscriptionGroupLister injects the catalog used to list subscription
// plans the owner can subscribe to.
func (s *OrganizationService) SetSubscriptionGroupLister(lister SubscriptionGroupLister) {
	if s != nil {
		s.groupLister = lister
	}
}

// SetSubscriptionOrderCreator injects the payment pipeline used to place
// enterprise subscription orders through the standard gateway flow.
func (s *OrganizationService) SetSubscriptionOrderCreator(creator SubscriptionOrderCreator) {
	if s != nil {
		s.orderCreator = creator
	}
}

// SetUpgradeChargeReader injects the settings-backed switch controlling whether
// the company upgrade fee is charged. When nil or when the switch is enabled,
// the historical charging behavior is preserved.
func (s *OrganizationService) SetUpgradeChargeReader(reader CompanyUpgradeChargeReader) {
	if s != nil {
		s.chargeReader = reader
	}
}

func (s *OrganizationService) SetUpgradeFeeReader(reader CompanyUpgradeFeeReader) {
	if s != nil {
		s.feeReader = reader
	}
}

func (s *OrganizationService) upgradeFee(ctx context.Context) float64 {
	if s != nil && s.feeReader != nil {
		return s.feeReader.GetCompanyUpgradeFee(ctx)
	}
	if s != nil && s.cfg != nil && s.cfg.Company.UpgradeFee > 0 {
		return s.cfg.Company.UpgradeFee
	}
	return 20
}

func (s *OrganizationService) SetCompanyFeatureReader(reader CompanyFeatureReader) {
	if s != nil {
		s.featureReader = reader
	}
}

func (s *OrganizationService) companyApplicationsEnabled(ctx context.Context) bool {
	if s != nil && s.featureReader != nil {
		return s.featureReader.IsCompanyApplicationsEnabled(ctx)
	}
	return s != nil && s.cfg != nil && s.cfg.Company.ApplicationsEnabled
}

func (s *OrganizationService) companyIAMEnabled(ctx context.Context) bool {
	if s != nil && s.featureReader != nil {
		return s.featureReader.IsCompanyIAMEnabled(ctx)
	}
	return s != nil && s.cfg != nil && s.cfg.Company.IAMEnabled
}

// upgradeChargeEnabled returns true when the upgrade fee should be charged.
// Defaults to true (historical behavior) when no reader is wired.
func (s *OrganizationService) upgradeChargeEnabled(ctx context.Context) bool {
	if s == nil || s.chargeReader == nil {
		return true
	}
	return s.chargeReader.IsCompanyUpgradeChargeEnabled(ctx)
}

func (s *OrganizationService) invalidateUserAuthorization(ctx context.Context, userID int64) {
	if s != nil && s.authCache != nil {
		s.authCache.InvalidateAuthCacheByUserID(ctx, userID)
	}
}

func NewOrganizationService(repo OrganizationRepository, userRepo UserRepository, cfg *config.Config) *OrganizationService {
	return &OrganizationService{repo: repo, userRepo: userRepo, cfg: cfg}
}

func normalizeCompanyName(value string) (string, string, error) {
	name := strings.Join(strings.Fields(strings.TrimSpace(value)), " ")
	if name == "" || len([]rune(name)) > 255 {
		return "", "", infraerrors.BadRequest("COMPANY_NAME_INVALID", "company name is required and must not exceed 255 characters")
	}
	return name, strings.ToLower(name), nil
}

// allowedCompanySizes enumerates the accepted company size ranges. The set is
// kept in sync with the frontend dropdown options.
var allowedCompanySizes = map[string]struct{}{
	"1-20":     {},
	"20-100":   {},
	"100-300":  {},
	"300-1000": {},
	"1000+":    {},
}

// normalizeCompanySize validates the submitted company size against the
// allowed enumeration and returns the canonical value.
func normalizeCompanySize(value string) (string, error) {
	size := strings.TrimSpace(value)
	if _, ok := allowedCompanySizes[size]; !ok {
		return "", ErrCompanySizeInvalid
	}
	return size, nil
}

func (s *OrganizationService) Context(ctx context.Context, userID int64) (*OrganizationContext, error) {
	return s.repo.GetContextForUser(ctx, userID)
}

func (s *OrganizationService) CurrentApplication(ctx context.Context, userID int64) (*CompanyApplication, error) {
	return s.repo.GetApplicationForUser(ctx, userID)
}

func (s *OrganizationService) UpgradeEligibility(ctx context.Context, userID int64) (*CompanyUpgradeEligibility, error) {
	result := &CompanyUpgradeEligibility{
		FeeAmount:   decimal.NewFromFloat(s.upgradeFee(ctx)).StringFixed(8),
		FeeCurrency: "USD",
	}
	if s.cfg != nil {
		result.FeeCurrency = s.cfg.Company.UpgradeCurrency
	}
	if !s.upgradeChargeEnabled(ctx) {
		// Charging disabled: surface a zero fee so the client shows "free".
		result.FeeAmount = decimal.Zero.StringFixed(8)
	}
	user, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	if user.IdentityType != IdentityTypeRoot || !user.IsActive() {
		result.Reason = "not_personal_root"
		return result, nil
	}
	if _, err := s.repo.GetContextForUser(ctx, userID); err == nil {
		result.Reason = "already_company_account"
		return result, nil
	} else if !errors.Is(err, ErrCompanyNotFound) {
		return nil, err
	}
	application, err := s.repo.GetApplicationForUser(ctx, userID)
	if err == nil && application.Status == "pending" {
		result.Reason = "application_pending"
		result.Application = application
		return result, nil
	}
	if err != nil && !errors.Is(err, ErrApplicationNotFound) {
		return nil, err
	}
	result.Eligible = true
	return result, nil
}

func (s *OrganizationService) GetApplication(ctx context.Context, applicationID int64) (*CompanyApplicationDetail, error) {
	return s.repo.GetApplication(ctx, applicationID)
}

func (s *OrganizationService) SubmitApplication(ctx context.Context, userID int64, name, companySize, idempotencyKey string) (*CompanyApplication, error) {
	if !s.companyApplicationsEnabled(ctx) {
		return nil, ErrCompanyFeatureDisabled
	}
	name, normalized, err := normalizeCompanyName(name)
	if err != nil {
		return nil, err
	}
	companySize, err = normalizeCompanySize(companySize)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(idempotencyKey) == "" || len(idempotencyKey) > 128 {
		return nil, infraerrors.BadRequest("IDEMPOTENCY_KEY_REQUIRED", "a valid idempotency key is required")
	}
	fee := decimal.NewFromFloat(s.upgradeFee(ctx)).StringFixed(8)
	if !s.upgradeChargeEnabled(ctx) {
		// Charging disabled: snapshot a zero fee so that reserve/capture/release
		// become no-op amount=0 operations and the balance is never frozen.
		fee = decimal.Zero.StringFixed(8)
	}
	return s.repo.SubmitApplication(ctx, userID, name, normalized, companySize, idempotencyKey, fee, s.cfg.Company.UpgradeCurrency)
}

func (s *OrganizationService) WithdrawApplication(ctx context.Context, userID, applicationID int64) (*CompanyApplication, error) {
	return s.repo.WithdrawApplication(ctx, userID, applicationID)
}

func (s *OrganizationService) ListApplications(ctx context.Context, status string, page, pageSize int) ([]CompanyApplication, int64, error) {
	return s.repo.ListApplications(ctx, status, page, pageSize)
}

func (s *OrganizationService) ListNameChangeRequests(ctx context.Context, status string, page, pageSize int) ([]OrganizationNameChangeRequest, int64, error) {
	return s.repo.ListNameChangeRequests(ctx, status, page, pageSize)
}

func (s *OrganizationService) GetNameChangeRequest(ctx context.Context, requestID int64) (*OrganizationNameChangeRequest, error) {
	return s.repo.GetNameChangeRequest(ctx, requestID)
}

func (s *OrganizationService) ListOrganizations(ctx context.Context, actorID int64, status string, page, pageSize int) ([]AdminOrganization, int64, error) {
	return s.repo.ListOrganizations(ctx, actorID, status, page, pageSize)
}

func (s *OrganizationService) GetOrganization(ctx context.Context, actorID, organizationID int64) (*AdminOrganizationDetail, error) {
	return s.repo.GetOrganization(ctx, actorID, organizationID)
}

func (s *OrganizationService) AdminCreateOrganizationSubscription(ctx context.Context, actorID, organizationID, groupID int64, validityDays int, notes string) (*OrganizationSubscription, error) {
	if validityDays < 1 || validityDays > 36500 {
		return nil, infraerrors.BadRequest("SUBSCRIPTION_VALIDITY_INVALID", "validity days must be between 1 and 36500")
	}
	return s.repo.AdminCreateOrganizationSubscription(ctx, actorID, organizationID, groupID, validityDays, strings.TrimSpace(notes))
}

func (s *OrganizationService) AdminListOrganizationSubscriptions(ctx context.Context, actorID int64, page, pageSize int, groupID *int64, status, platform, sortBy, sortOrder string) ([]OrganizationSubscription, int64, error) {
	return s.repo.AdminListOrganizationSubscriptions(ctx, actorID, page, pageSize, groupID, status, platform, sortBy, sortOrder)
}

func (s *OrganizationService) AdminExtendOrganizationSubscription(ctx context.Context, actorID, subscriptionID int64, days int) error {
	if days == 0 || days < -36500 || days > 36500 {
		return infraerrors.BadRequest("SUBSCRIPTION_VALIDITY_INVALID", "adjustment days must be between -36500 and 36500 and cannot be zero")
	}
	return s.repo.AdminExtendOrganizationSubscription(ctx, actorID, subscriptionID, days)
}

func (s *OrganizationService) AdminResetOrganizationSubscriptionQuota(ctx context.Context, actorID, subscriptionID int64) error {
	return s.repo.AdminResetOrganizationSubscriptionQuota(ctx, actorID, subscriptionID)
}

func (s *OrganizationService) AdminRevokeOrganizationSubscription(ctx context.Context, actorID, subscriptionID int64) error {
	return s.repo.AdminRevokeOrganizationSubscription(ctx, actorID, subscriptionID)
}

func (s *OrganizationService) DecideApplication(ctx context.Context, reviewerID, applicationID int64, approve bool, reason string) (*CompanyApplication, error) {
	if !approve && strings.TrimSpace(reason) == "" {
		return nil, ErrReasonRequired
	}
	limit := 20
	if s.cfg != nil && s.cfg.Company.DefaultMemberLimit > 0 {
		limit = s.cfg.Company.DefaultMemberLimit
	}
	return s.repo.DecideApplication(ctx, reviewerID, applicationID, approve, strings.TrimSpace(reason), limit)
}

func (s *OrganizationService) RequestNameChange(ctx context.Context, userID int64, name string) error {
	name, normalized, err := normalizeCompanyName(name)
	if err != nil {
		return err
	}
	return s.repo.RequestNameChange(ctx, userID, name, normalized)
}

func (s *OrganizationService) DecideNameChange(ctx context.Context, reviewerID, requestID int64, approve bool, reason string) error {
	if !approve && strings.TrimSpace(reason) == "" {
		return ErrReasonRequired
	}
	return s.repo.DecideNameChange(ctx, reviewerID, requestID, approve, strings.TrimSpace(reason))
}

func (s *OrganizationService) SetOrganizationStatus(ctx context.Context, actorID, organizationID int64, status string) error {
	if status != OrganizationStatusActive && status != OrganizationStatusSuspended {
		return infraerrors.BadRequest("ORGANIZATION_STATUS_INVALID", "organization status is invalid")
	}
	if err := s.repo.SetOrganizationStatus(ctx, actorID, organizationID, status); err != nil {
		return err
	}
	if userIDs, err := s.repo.ListOrganizationUserIDs(ctx, organizationID); err == nil {
		for _, userID := range userIDs {
			s.invalidateUserAuthorization(ctx, userID)
		}
	}
	return nil
}

func generateInitialPassword() (string, error) {
	buf := make([]byte, 24)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

func (s *OrganizationService) CreateIAMMember(ctx context.Context, ownerID int64, loginName, username, recoveryEmail, password string, mustChangePassword bool) (*IAMMember, string, error) {
	if !s.companyIAMEnabled(ctx) {
		return nil, "", ErrIAMFeatureDisabled
	}
	loginName = strings.ToLower(strings.TrimSpace(loginName))
	if !iamLoginNamePattern.MatchString(loginName) {
		return nil, "", ErrIAMLoginName
	}
	username = strings.TrimSpace(username)
	if len([]rune(username)) > 100 {
		return nil, "", infraerrors.BadRequest("USERNAME_INVALID", "username must not exceed 100 characters")
	}
	recoveryEmail = strings.TrimSpace(recoveryEmail)
	if recoveryEmail != "" {
		parsed, parseErr := mail.ParseAddress(recoveryEmail)
		if parseErr != nil || !strings.EqualFold(parsed.Address, recoveryEmail) {
			return nil, "", infraerrors.BadRequest("RECOVERY_EMAIL_INVALID", "recovery email is invalid")
		}
	}
	if len(password) < 8 || len(password) > 72 {
		return nil, "", ErrIAMPassword
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, "", err
	}
	limit := s.cfg.Company.DefaultMemberLimit
	member, err := s.repo.CreateIAMMember(ctx, ownerID, &User{
		IdentityType: IdentityTypeIAM, LoginName: loginName, Username: username, PasswordHash: string(hash), Role: RoleUser,
		Status: StatusActive, RecoveryEmail: recoveryEmail, MustChangePassword: mustChangePassword, AuthzGeneration: 1,
	}, limit)
	if err != nil {
		return nil, "", err
	}
	return member, password, nil
}

func (s *OrganizationService) ListIAMMembers(ctx context.Context, actorID int64) ([]IAMMember, int, error) {
	return s.repo.ListIAMMembers(ctx, actorID)
}

func (s *OrganizationService) GetIAMMember(ctx context.Context, actorID, memberUserID int64) (*IAMMember, error) {
	return s.repo.GetIAMMember(ctx, actorID, memberUserID)
}

func normalizeSpendLimitAmount(value *string) (*string, error) {
	if value == nil || strings.TrimSpace(*value) == "" {
		return nil, nil
	}
	amount, err := decimal.NewFromString(strings.TrimSpace(*value))
	if err != nil || !amount.IsPositive() || !amount.Equal(amount.Round(10)) || amount.GreaterThanOrEqual(decimal.New(1, 10)) {
		return nil, ErrSpendLimitInvalid
	}
	normalized := amount.StringFixed(10)
	return &normalized, nil
}

func normalizeSpendLimitRecipients(values []string) ([]string, error) {
	if len(values) > 20 {
		return nil, infraerrors.BadRequest("ORGANIZATION_SPEND_LIMIT_RECIPIENTS_INVALID", "at most 20 additional recipients are allowed")
	}
	seen := make(map[string]struct{}, len(values))
	normalized := make([]string, 0, len(values))
	for _, value := range values {
		email := strings.ToLower(strings.TrimSpace(value))
		parsed, err := mail.ParseAddress(email)
		if err != nil || parsed.Address != email || len(email) > 255 {
			return nil, infraerrors.BadRequest("ORGANIZATION_SPEND_LIMIT_RECIPIENT_INVALID", "an additional recipient email is invalid")
		}
		if _, exists := seen[email]; exists {
			continue
		}
		seen[email] = struct{}{}
		normalized = append(normalized, email)
	}
	return normalized, nil
}

func (s *OrganizationService) ListSpendLimitRules(ctx context.Context, actorID int64) ([]OrganizationSpendLimitRule, error) {
	repo, ok := s.repo.(OrganizationSpendLimitRepository)
	if !ok {
		return nil, ErrOrganizationPermission
	}
	return repo.ListSpendLimitRules(ctx, actorID)
}

func (s *OrganizationService) UpsertSpendLimitRules(ctx context.Context, ownerID int64, memberIDs []int64, daily, monthly *string, alertEnabled bool, threshold float64, recipients []string) ([]OrganizationSpendLimitRule, error) {
	daily, err := normalizeSpendLimitAmount(daily)
	if err != nil {
		return nil, err
	}
	monthly, err = normalizeSpendLimitAmount(monthly)
	if err != nil {
		return nil, err
	}
	if daily == nil && monthly == nil {
		return nil, ErrSpendLimitInvalid
	}
	if !alertEnabled && threshold == 0 {
		threshold = 80
	}
	if threshold < 1 || threshold > 100 {
		return nil, ErrSpendLimitThreshold
	}
	recipients, err = normalizeSpendLimitRecipients(recipients)
	if err != nil {
		return nil, err
	}
	uniqueMembers := make([]int64, 0, len(memberIDs))
	seen := make(map[int64]struct{}, len(memberIDs))
	for _, memberID := range memberIDs {
		if memberID <= 0 {
			return nil, ErrIAMMemberNotFound
		}
		if _, exists := seen[memberID]; exists {
			continue
		}
		seen[memberID] = struct{}{}
		uniqueMembers = append(uniqueMembers, memberID)
	}
	repo, ok := s.repo.(OrganizationSpendLimitRepository)
	if !ok {
		return nil, ErrOrganizationPermission
	}
	return repo.UpsertSpendLimitRules(ctx, ownerID, uniqueMembers, daily, monthly, alertEnabled, threshold, recipients)
}

func (s *OrganizationService) DeleteSpendLimitRule(ctx context.Context, ownerID int64, memberID *int64) error {
	repo, ok := s.repo.(OrganizationSpendLimitRepository)
	if !ok {
		return ErrOrganizationPermission
	}
	return repo.DeleteSpendLimitRule(ctx, ownerID, memberID)
}

func (s *OrganizationService) ListSpendLimitUsage(ctx context.Context, actorID int64) ([]OrganizationSpendUsage, error) {
	repo, ok := s.repo.(OrganizationSpendLimitRepository)
	if !ok {
		return nil, ErrOrganizationPermission
	}
	return repo.ListSpendLimitUsage(ctx, actorID)
}

func (s *OrganizationService) SetIAMMemberStatus(ctx context.Context, ownerID, memberUserID int64, status string) error {
	if status != MembershipStatusActive && status != MembershipStatusDisabled && status != MembershipStatusArchived {
		return infraerrors.BadRequest("IAM_MEMBER_STATUS_INVALID", "IAM member status is invalid")
	}
	if err := s.repo.SetIAMMemberStatus(ctx, ownerID, memberUserID, status); err != nil {
		return err
	}
	s.invalidateUserAuthorization(ctx, memberUserID)
	return nil
}

func (s *OrganizationService) ResetIAMPassword(ctx context.Context, ownerID, memberUserID int64) (string, error) {
	password, err := generateInitialPassword()
	if err != nil {
		return "", err
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	if err := s.repo.UpdateIAMPassword(ctx, ownerID, memberUserID, string(hash), true); err != nil {
		return "", err
	}
	s.invalidateUserAuthorization(ctx, memberUserID)
	return password, nil
}

// GetOrganizationSettings returns the current feature-toggle configuration
// for the caller's organization. Visible to owners and to members holding the
// `CompanyFinanceManage` policy; other members receive ErrOrganizationPermission.
// Repository returns default (zero) settings when no row exists so the UI can
// render the switches with their default states.
func (s *OrganizationService) GetOrganizationSettings(ctx context.Context, actorID int64) (*OrganizationSettings, error) {
	repo, ok := s.repo.(OrganizationSettingsRepository)
	if !ok {
		// Repository not yet updated: fall back to defaults so old deployments
		// keep working. The UI still shows the switches in their default states.
		return &OrganizationSettings{AutoSwitchSubscription: true}, nil
	}
	return repo.GetOrganizationSettings(ctx, actorID)
}

// UpdateOrganizationSettings persists the toggle values. Access control is
// enforced inside the repository (owner or CompanyFinanceManage holder).
func (s *OrganizationService) UpdateOrganizationSettings(ctx context.Context, actorID int64, settings OrganizationSettings) (*OrganizationSettings, error) {
	repo, ok := s.repo.(OrganizationSettingsRepository)
	if !ok {
		return nil, ErrOrganizationPermission
	}
	return repo.UpsertOrganizationSettings(ctx, actorID, settings)
}

// ListFallbackCandidatesForSubscription returns the ordered list of
// same-platform organization subscriptions this member may fall over to when
// `currentSubscriptionID` is exhausted. Used by the API Key list UI to render
// the candidate chain so users can see exactly which plans will be tried next.
// Empty result is a valid state (no other plans available); callers should
// render an appropriate hint. Non-members / non-authorized users receive
// ErrOrganizationPermission.
//
// ACL: 与 GetSubscriptionFallbackView 一致，只要求"调用者是订阅所在组织的
// 活跃成员"；不要求订阅本身仍 active/未过期，因为 fallback 页面正是要在过
// 期/失效时展示候选。
func (s *OrganizationService) ListFallbackCandidatesForSubscription(ctx context.Context, userID, currentSubscriptionID int64) ([]OrganizationSubscription, error) {
	organizationID, err := s.repo.GetOrganizationSubscriptionOrganizationID(ctx, currentSubscriptionID)
	if err != nil {
		return nil, err
	}
	orgCtx, err := s.repo.GetContextForUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	if !orgCtx.Active() || orgCtx.OrganizationID != organizationID {
		return nil, ErrOrganizationPermission
	}
	repo, ok := s.repo.(OrganizationSettingsRepository)
	if !ok {
		return []OrganizationSubscription{}, nil
	}
	return repo.ListFallbackCandidateSubscriptions(ctx, organizationID, currentSubscriptionID)
}

// SubscriptionFallbackView bundles the ordered candidate chain with the
// organization-level auto-switch flag so the API Key UI can render both in a
// single roundtrip.
type SubscriptionFallbackView struct {
	AutoSwitchEnabled bool                       `json:"auto_switch_enabled"`
	Candidates        []OrganizationSubscription `json:"candidates"`
}

// GetSubscriptionFallbackView is the composite helper used by the API Key list
// UI. Any active member of the organization that owns `currentSubscriptionID`
// may call it; the returned auto-switch flag is read via the repository's
// internal-by-id accessor so IAM members without CompanyFinanceManage still see
// the correct state (they just can't toggle it).
//
// ACL: 仅校验"调用者属于当前订阅所在组织的活跃成员"；不要求当前订阅仍处于
// active/未过期状态——这正是 fallback 存在的场景（过期/失效/被清理时用户依
// 然应能看到候选链）。因此使用 GetOrganizationSubscriptionOrganizationID
// 拿 organization_id（该方法允许软删/无关联 group），再用 GetContextForUser
// 验证成员身份。
func (s *OrganizationService) GetSubscriptionFallbackView(ctx context.Context, userID, currentSubscriptionID int64) (*SubscriptionFallbackView, error) {
	organizationID, err := s.repo.GetOrganizationSubscriptionOrganizationID(ctx, currentSubscriptionID)
	if err != nil {
		return nil, err
	}
	orgCtx, err := s.repo.GetContextForUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	if !orgCtx.Active() || orgCtx.OrganizationID != organizationID {
		return nil, ErrOrganizationPermission
	}
	repo, ok := s.repo.(OrganizationSettingsRepository)
	if !ok {
		return &SubscriptionFallbackView{AutoSwitchEnabled: true, Candidates: []OrganizationSubscription{}}, nil
	}
	settings, err := repo.GetOrganizationSettingsByID(ctx, organizationID)
	autoSwitch := true
	if err == nil && settings != nil {
		autoSwitch = settings.AutoSwitchSubscription
	}
	candidates, err := repo.ListFallbackCandidateSubscriptions(ctx, organizationID, currentSubscriptionID)
	if err != nil {
		return nil, err
	}
	return &SubscriptionFallbackView{AutoSwitchEnabled: autoSwitch, Candidates: candidates}, nil
}

func (s *OrganizationService) ChangeIAMPassword(ctx context.Context, userID int64, password string) (*User, error) {
	if len(password) < 8 {
		return nil, infraerrors.BadRequest("PASSWORD_TOO_SHORT", "password must be at least 8 characters")
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}
	if err := s.repo.UpdateIAMPassword(ctx, userID, userID, string(hash), false); err != nil {
		return nil, err
	}
	s.invalidateUserAuthorization(ctx, userID)
	return s.userRepo.GetByID(ctx, userID)
}

func (s *OrganizationService) AuthenticateIAM(ctx context.Context, principal, password string) (*User, *OrganizationContext, error) {
	if !s.companyIAMEnabled(ctx) {
		return nil, nil, ErrIAMFeatureDisabled
	}
	loginName, companyID, ok := parseIAMPrincipal(principal)
	if !ok {
		return nil, nil, ErrInvalidCredentials
	}
	user, org, err := s.repo.FindIAMByPrincipal(ctx, loginName, companyID)
	if err != nil || user == nil || org == nil || !user.IsActive() || !org.Active() || !user.CheckPassword(password) {
		return nil, nil, ErrInvalidCredentials
	}
	return user, org, nil
}

func (s *OrganizationService) ListPolicies(ctx context.Context, actorID int64) ([]ManagedPolicyView, error) {
	return s.repo.ListPolicies(ctx, actorID)
}

func (s *OrganizationService) ListMemberPolicyAttachments(ctx context.Context, ownerID, memberUserID int64) ([]ManagedPolicyView, error) {
	return s.repo.ListMemberPolicyAttachments(ctx, ownerID, memberUserID)
}

func (s *OrganizationService) SetPolicyAttachment(ctx context.Context, ownerID, memberID int64, policyKey string, attach bool, correlationID string) error {
	switch policyKey {
	case PolicyCompanyFinanceReadOnly, PolicyCompanySharedBalance, PolicyCompanyFinanceManage, PolicyIAMUserManage:
	default:
		return infraerrors.BadRequest("POLICY_INVALID", "managed policy is invalid")
	}
	if err := s.repo.SetPolicyAttachment(ctx, ownerID, memberID, policyKey, attach, correlationID); err != nil {
		return err
	}
	s.invalidateUserAuthorization(ctx, memberID)
	return nil
}

func (s *OrganizationService) TransferBalance(ctx context.Context, ownerID, memberID int64, amount, idempotencyKey string, reclaim bool) error {
	d, err := decimal.NewFromString(strings.TrimSpace(amount))
	if err != nil || !d.IsPositive() {
		return infraerrors.BadRequest("AMOUNT_INVALID", "amount must be positive")
	}
	idempotencyKey = strings.TrimSpace(idempotencyKey)
	if idempotencyKey == "" || len(idempotencyKey) > 96 {
		return infraerrors.BadRequest("IDEMPOTENCY_KEY_REQUIRED", "an idempotency key is required")
	}
	return s.repo.TransferBalance(ctx, ownerID, memberID, d.StringFixed(8), idempotencyKey, reclaim)
}

// DepositToCompany moves funds between the caller's personal balance and the
// company balance. When withdraw is false the caller tops the company balance
// up from their personal balance; when true the flow is reversed. Only the
// organization owner may perform this (enforced in the repository).
func (s *OrganizationService) DepositToCompany(ctx context.Context, ownerID int64, amount, idempotencyKey string, withdraw bool) error {
	d, err := decimal.NewFromString(strings.TrimSpace(amount))
	if err != nil || !d.IsPositive() {
		return infraerrors.BadRequest("AMOUNT_INVALID", "amount must be positive")
	}
	idempotencyKey = strings.TrimSpace(idempotencyKey)
	if idempotencyKey == "" || len(idempotencyKey) > 96 {
		return infraerrors.BadRequest("IDEMPOTENCY_KEY_REQUIRED", "an idempotency key is required")
	}
	return s.repo.DepositToCompany(ctx, ownerID, d.StringFixed(8), idempotencyKey, withdraw)
}

func (s *OrganizationService) FinanceSummary(ctx context.Context, userID int64) (*FinanceSummary, error) {
	return s.repo.FinanceSummary(ctx, userID)
}

// OrganizationAuditLogEntry is a single audit row returned by the operation log
// API used by the enterprise "Audit log" page. It extends OrganizationAuditEvent
// with UI-friendly fields (login names, coarse category) so the frontend can
// render human-readable rows without extra lookups.
type OrganizationAuditLogEntry struct {
	ID               int64  `json:"id"`
	OrganizationID   *int64 `json:"organization_id,omitempty"`
	ActorUserID      *int64 `json:"actor_user_id,omitempty"`
	ActorLoginName   string `json:"actor_login_name,omitempty"`
	ActorUsername    string `json:"actor_username,omitempty"`
	ActorEmail       string `json:"actor_email,omitempty"`
	SubjectUserID    *int64 `json:"subject_user_id,omitempty"`
	SubjectLoginName string `json:"subject_login_name,omitempty"`
	SubjectUsername  string `json:"subject_username,omitempty"`
	SubjectEmail     string `json:"subject_email,omitempty"`
	Action           string `json:"action"`
	// Category is the coarse bucket used for UI filtering. Derived from Action.
	// One of: recharge, authorize, allocate, spend_limit, other.
	Category  string         `json:"category"`
	Result    string         `json:"result"`
	Metadata  map[string]any `json:"metadata,omitempty"`
	CreatedAt time.Time      `json:"created_at"`
}

// OrganizationAuditFilter narrows down the audit event query used by the
// enterprise "Audit log" page.
type OrganizationAuditFilter struct {
	Category string    // "" | "recharge" | "authorize" | "allocate" | "spend_limit"
	Start    time.Time // zero value means no lower bound
	End      time.Time // zero value means no upper bound
	Page     int
	PageSize int
}

// AuditActionsForCategory maps a coarse UI category to the underlying action
// names stored in organization_audit_events. Exposed so the repository layer
// can reuse the same mapping without duplicating the string list.
func AuditActionsForCategory(category string) []string {
	switch category {
	case "recharge":
		return []string{"organization.balance.company_deposit", "organization.balance.company_withdraw"}
	case "authorize":
		return []string{"iam.policy.change"}
	case "allocate":
		return []string{"organization.balance.allocate", "organization.balance.reclaim"}
	case "spend_limit":
		return []string{"spend_limit.upsert", "spend_limit.delete"}
	}
	return nil
}

// AuditCategoryForAction is the inverse of AuditActionsForCategory: given a
// raw action string it returns the coarse category used by the UI, or "other".
func AuditCategoryForAction(action string) string {
	switch action {
	case "organization.balance.company_deposit", "organization.balance.company_withdraw":
		return "recharge"
	case "iam.policy.change":
		return "authorize"
	case "organization.balance.allocate", "organization.balance.reclaim":
		return "allocate"
	case "spend_limit.upsert", "spend_limit.delete":
		return "spend_limit"
	}
	return "other"
}

// ListAuditEvents returns paginated operation records for the organization
// bound to the caller. Owner-only access — non-owners get ErrOrganizationPermission
// so the endpoint cannot be used to snoop on peers' actions.
func (s *OrganizationService) ListAuditEvents(ctx context.Context, userID int64, filter OrganizationAuditFilter) ([]OrganizationAuditLogEntry, int64, error) {
	org, err := s.repo.GetContextForUser(ctx, userID)
	if err != nil {
		return nil, 0, err
	}
	if org == nil || !org.Active() || !org.Owner() {
		return nil, 0, ErrOrganizationPermission
	}
	if filter.Page <= 0 {
		filter.Page = 1
	}
	if filter.PageSize <= 0 || filter.PageSize > 100 {
		filter.PageSize = 20
	}
	// Normalize an unknown category to empty (all) so a stale UI cannot force
	// the repository to filter by a bogus action set that matches nothing.
	if filter.Category != "" && AuditActionsForCategory(filter.Category) == nil {
		filter.Category = ""
	}
	return s.repo.ListAuditEvents(ctx, org.OrganizationID, filter)
}

// CreateOrganizationSubscription provisions a subscription plan (group) for the
// caller's company. Only the organization owner may do this (enforced in the
// repository). When validityDays is 0 the group's default validity is used.
func (s *OrganizationService) CreateOrganizationSubscription(ctx context.Context, userID, groupID int64, validityDays int, notes string) (*OrganizationSubscription, error) {
	if groupID <= 0 {
		return nil, ErrSubscriptionGroupInvalid
	}
	if validityDays < 0 || validityDays > 3650 {
		return nil, ErrSubscriptionValidityRange
	}
	notes = strings.TrimSpace(notes)
	if len([]rune(notes)) > 500 {
		return nil, infraerrors.BadRequest("SUBSCRIPTION_NOTES_TOO_LONG", "notes must not exceed 500 characters")
	}
	return s.repo.CreateOrganizationSubscription(ctx, userID, groupID, validityDays, notes)
}

// ListOrganizationSubscriptions returns the company's subscriptions. Visible to
// the owner and to accounts holding organization.finance.balance.read.
func (s *OrganizationService) ListOrganizationSubscriptions(ctx context.Context, userID int64) ([]OrganizationSubscription, error) {
	return s.repo.ListOrganizationSubscriptions(ctx, userID)
}

// ListSubscriptionGroups returns the active subscription-type groups that the
// owner may subscribe the company to. Unlike /groups/available (which only
// surfaces subscription groups the caller already subscribed to), this returns
// the full catalog of subscribable plans. Owner-only.
func (s *OrganizationService) ListSubscriptionGroups(ctx context.Context, userID int64) ([]Group, error) {
	orgCtx, err := s.repo.GetContextForUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	if !orgCtx.Active() {
		return nil, ErrOrganizationPermission
	}
	if !orgCtx.Owner() && !orgCtx.HasAction(ActionSubscriptionManage) {
		return nil, ErrOrganizationPermission
	}
	if s.groupLister == nil {
		return []Group{}, nil
	}
	groups, err := s.groupLister.ListActive(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]Group, 0, len(groups))
	for i := range groups {
		if groups[i].IsSubscriptionType() && groups[i].IsActive() {
			result = append(result, groups[i])
		}
	}
	return result, nil
}

// CreateSubscriptionOrder places a payment order for a company subscription.
//
// The owner pays through the standard personal payment gateway (WeChat /
// Alipay / Stripe / ...), exactly like a personal subscription purchase. The
// only difference is that the resulting order carries the company's
// OrganizationID, so once payment is confirmed the fulfillment path provisions
// an organization_subscriptions row for the company instead of a personal
// user_subscriptions row.
//
// Authorization: only an active owner of an active company may place the order.
func (s *OrganizationService) CreateSubscriptionOrder(ctx context.Context, userID int64, input OrganizationSubscriptionOrderInput) (*CreateOrderResponse, error) {
	if input.PlanID <= 0 {
		return nil, ErrOrgSubscriptionNotFound
	}
	orgCtx, err := s.repo.GetContextForUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	if !orgCtx.Active() {
		return nil, ErrOrganizationPermission
	}
	if !orgCtx.Owner() && !orgCtx.HasAction(ActionSubscriptionManage) {
		return nil, ErrOrganizationPermission
	}
	if s.orderCreator == nil {
		return nil, ErrOrganizationPermission
	}
	return s.orderCreator.CreateOrder(ctx, CreateOrderRequest{
		UserID:          userID,
		PaymentType:     input.PaymentType,
		OpenID:          input.OpenID,
		ClientIP:        input.ClientIP,
		IsMobile:        input.IsMobile,
		IsWeChatBrowser: input.IsWeChatBrowser,
		SrcHost:         input.SrcHost,
		SrcURL:          input.SrcURL,
		ReturnURL:       input.ReturnURL,
		PaymentSource:   input.PaymentSource,
		OrderType:       "subscription",
		PlanID:          input.PlanID,
		OrganizationID:  orgCtx.OrganizationID,
		Locale:          input.Locale,
	})
}

// FulfillOrganizationSubscriptionOrder provisions (or extends) the company
// subscription once a paid enterprise subscription order is confirmed. It is
// invoked by the payment fulfillment pipeline (as an
// OrganizationSubscriptionFulfiller) and must be idempotent on orderID.
func (s *OrganizationService) FulfillOrganizationSubscriptionOrder(ctx context.Context, orgID, groupID int64, validityDays int, orderID int64) error {
	return s.repo.AssignOrExtendOrganizationSubscription(ctx, orgID, groupID, validityDays, orderID)
}

// CancelOrganizationSubscription cancels (soft-deletes) a company subscription.
// Owner-only (enforced in the repository).
func (s *OrganizationService) CancelOrganizationSubscription(ctx context.Context, userID, subscriptionID int64) error {
	if subscriptionID <= 0 {
		return ErrOrgSubscriptionNotFound
	}
	return s.repo.CancelOrganizationSubscription(ctx, userID, subscriptionID)
}

func (s *OrganizationService) ListUsage(ctx context.Context, userID int64, filter OrganizationUsageFilter) ([]OrganizationUsageRow, int64, error) {
	return s.repo.ListUsage(ctx, userID, filter)
}

func (s *OrganizationService) ListOrganizationUserIDs(ctx context.Context, organizationID int64) ([]int64, error) {
	return s.repo.ListOrganizationUserIDs(ctx, organizationID)
}

func (s *OrganizationService) UsageStats(ctx context.Context, userID int64, filter OrganizationUsageFilter) (*OrganizationUsageStats, error) {
	return s.repo.UsageStats(ctx, userID, filter)
}

func (s *OrganizationService) UsageTrend(ctx context.Context, userID int64, filter OrganizationUsageFilter) ([]OrganizationUsageTrendPoint, error) {
	return s.repo.UsageTrend(ctx, userID, filter)
}

func (s *OrganizationService) UsageCharts(ctx context.Context, userID int64, filter OrganizationUsageFilter) (*OrganizationUsageCharts, error) {
	return s.repo.UsageCharts(ctx, userID, filter)
}

func (s *OrganizationService) OrganizationDashboard(ctx context.Context, userID int64) (*usagestats.DashboardStats, error) {
	return s.repo.OrganizationDashboard(ctx, userID)
}

func (s *OrganizationService) OrganizationSpendingRanking(ctx context.Context, userID int64, filter OrganizationUsageFilter, limit int) (*usagestats.UserSpendingRankingResponse, error) {
	return s.repo.OrganizationSpendingRanking(ctx, userID, filter, limit)
}

func (s *OrganizationService) OrganizationUserBreakdown(ctx context.Context, userID int64, filter OrganizationUsageFilter, limit int) ([]usagestats.UserBreakdownItem, error) {
	return s.repo.OrganizationUserBreakdown(ctx, userID, filter, limit)
}

func (s *OrganizationService) OrganizationUsersTrend(ctx context.Context, userID int64, filter OrganizationUsageFilter, limit int) ([]usagestats.UserUsageTrendPoint, error) {
	return s.repo.OrganizationUsersTrend(ctx, userID, filter, limit)
}

func (s *OrganizationService) SearchOrganizationAPIKeys(ctx context.Context, userID int64, memberID *int64, query string, limit int) ([]OrganizationAPIKeyOption, error) {
	return s.repo.SearchOrganizationAPIKeys(ctx, userID, memberID, query, limit)
}

func (s *OrganizationService) Reconcile(ctx context.Context) (map[string]int64, error) {
	return s.repo.Reconcile(ctx)
}

type BillingContextResolver struct{ repo OrganizationRepository }

func NewBillingContextResolver(repo OrganizationRepository) *BillingContextResolver {
	return &BillingContextResolver{repo: repo}
}

func (r *BillingContextResolver) Resolve(ctx context.Context, consumerUserID int64) (*BillingContext, error) {
	return r.ResolveForAmount(ctx, consumerUserID, 0)
}

// ResolveForAmount prefers an IAM member's allocation when it can cover the
// complete charge, and uses shared balance only as an authorized fallback.
func (r *BillingContextResolver) ResolveForAmount(ctx context.Context, consumerUserID int64, requiredAmount float64) (*BillingContext, error) {
	return r.resolveForAmount(ctx, consumerUserID, requiredAmount, true)
}

// ResolveForSettlement selects the wallet for an already completed request.
// Spend limits are enforced before upstream execution; settlement must always
// persist and charge completed usage instead of making the usage record vanish.
func (r *BillingContextResolver) ResolveForSettlement(ctx context.Context, consumerUserID int64, requiredAmount float64) (*BillingContext, error) {
	return r.resolveForAmount(ctx, consumerUserID, requiredAmount, false)
}

func (r *BillingContextResolver) resolveForAmount(ctx context.Context, consumerUserID int64, requiredAmount float64, enforceSpendLimit bool) (*BillingContext, error) {
	if r == nil || r.repo == nil {
		return &BillingContext{ConsumerUserID: consumerUserID, PayerUserID: consumerUserID, BalanceSource: BalanceSourceSelf}, nil
	}
	resolved, err := r.repo.ResolveBillingContext(ctx, consumerUserID, requiredAmount)
	if errors.Is(err, ErrCompanyNotFound) {
		return &BillingContext{ConsumerUserID: consumerUserID, PayerUserID: consumerUserID, BalanceSource: BalanceSourceSelf}, nil
	}
	if err != nil {
		organizationRuntimeMetrics.payerResolutionFailures.Add(1)
		return nil, fmt.Errorf("resolve billing context: %w", err)
	}
	if enforceSpendLimit {
		if err := r.CheckSpendLimit(ctx, resolved, requiredAmount); err != nil {
			return nil, err
		}
	}
	return resolved, nil
}

func (r *BillingContextResolver) CheckSpendLimit(ctx context.Context, billing *BillingContext, amount float64) error {
	if r == nil || billing == nil || amount < 0 || !companySponsoredBalanceSource(billing.BalanceSource) {
		return nil
	}
	repo, ok := r.repo.(OrganizationSpendLimitRepository)
	if !ok {
		return nil
	}
	return repo.CheckOrganizationSpendLimit(ctx, billing.ConsumerUserID, billing.BalanceSource, amount)
}

func (r *BillingContextResolver) RecordSpendLimitAlert(ctx context.Context, billing *BillingContext) error {
	if r == nil || billing == nil || !companySponsoredBalanceSource(billing.BalanceSource) {
		return nil
	}
	repo, ok := r.repo.(OrganizationSpendLimitRepository)
	if !ok {
		return nil
	}
	return repo.RecordSpendLimitAlert(ctx, billing.ConsumerUserID, billing.BalanceSource)
}

func companySponsoredBalanceSource(source string) bool {
	return source == BalanceSourceCompany || source == BalanceSourceLegacyShared || source == BalanceSourceSubscription
}

func (r *BillingContextResolver) GetOrganizationBalance(ctx context.Context, billing *BillingContext) (float64, error) {
	repo, organizationID, err := r.organizationBalanceRepository(billing)
	if err != nil {
		return 0, err
	}
	return repo.GetOrganizationBalance(ctx, organizationID)
}

func (r *BillingContextResolver) DeductOrganizationBalance(ctx context.Context, billing *BillingContext, amount float64) (float64, error) {
	repo, organizationID, err := r.organizationBalanceRepository(billing)
	if err != nil {
		return 0, err
	}
	return repo.DeductOrganizationBalance(ctx, organizationID, amount)
}

func (r *BillingContextResolver) CreditOrganizationBalance(ctx context.Context, billing *BillingContext, amount float64) (float64, error) {
	repo, organizationID, err := r.organizationBalanceRepository(billing)
	if err != nil {
		return 0, err
	}
	return repo.CreditOrganizationBalance(ctx, organizationID, amount)
}

func (r *BillingContextResolver) organizationBalanceRepository(billing *BillingContext) (OrganizationBalanceRepository, int64, error) {
	if r == nil || r.repo == nil || billing == nil || !billing.UsesCompanyBalance() {
		return nil, 0, fmt.Errorf("organization balance context is unavailable")
	}
	if billing.OrganizationID == nil || *billing.OrganizationID <= 0 {
		return nil, 0, ErrCompanyNotFound
	}
	repo, ok := r.repo.(OrganizationBalanceRepository)
	if !ok {
		return nil, 0, fmt.Errorf("organization balance repository is unavailable")
	}
	return repo, *billing.OrganizationID, nil
}

func GuardIAMFinancialOperation(user *User) error {
	if user != nil && user.IsIAM() {
		organizationRuntimeMetrics.deniedIAMFinancialOps.Add(1)
		return ErrIAMFinancialOperation
	}
	return nil
}
