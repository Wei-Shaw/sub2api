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
	ActionFinanceBalanceRead     = "organization.finance.balance.read"
	ActionSharedBalanceUse       = "organization.balance.shared.use"
	IAMPrincipalDomain           = "opentk.ai"
)

var (
	ErrCompanyFeatureDisabled = infraerrors.Forbidden("COMPANY_FEATURE_DISABLED", "company account features are disabled")
	ErrCompanyNotEligible     = infraerrors.Forbidden("COMPANY_NOT_ELIGIBLE", "account is not eligible for company upgrade")
	ErrCompanyPending         = infraerrors.Conflict("COMPANY_APPLICATION_PENDING", "a company application is already pending")
	ErrCompanyNotFound        = infraerrors.NotFound("COMPANY_NOT_FOUND", "company organization not found")
	ErrApplicationNotFound    = infraerrors.NotFound("COMPANY_APPLICATION_NOT_FOUND", "company application not found")
	ErrApplicationTerminal    = infraerrors.Conflict("COMPANY_APPLICATION_TERMINAL", "company application is already decided")
	ErrReasonRequired         = infraerrors.BadRequest("REJECTION_REASON_REQUIRED", "rejection reason is required")
	ErrCompanyEnglishNameInvalid = infraerrors.BadRequest("COMPANY_ENGLISH_NAME_INVALID", "company english name is required and must contain only English letters, digits, spaces and basic punctuation")
	ErrCompanyEnglishNameTaken   = infraerrors.Conflict("COMPANY_ENGLISH_NAME_TAKEN", "company english name is already taken")
	ErrCompanySizeInvalid        = infraerrors.BadRequest("COMPANY_SIZE_INVALID", "company size is required and must be one of the allowed ranges")
	ErrIAMFeatureDisabled     = infraerrors.Forbidden("IAM_FEATURE_DISABLED", "IAM user creation is disabled")
	ErrIAMMemberLimit         = infraerrors.Conflict("IAM_MEMBER_LIMIT", "organization IAM member limit reached")
	ErrIAMLoginName           = infraerrors.BadRequest("IAM_LOGIN_NAME_INVALID", "IAM login name is invalid")
	ErrIAMPassword            = infraerrors.BadRequest("IAM_PASSWORD_INVALID", "IAM password must be between 8 and 72 bytes")
	ErrIAMMemberNotFound      = infraerrors.NotFound("IAM_MEMBER_NOT_FOUND", "IAM member not found")
	ErrOrganizationPermission = infraerrors.Forbidden("ORGANIZATION_PERMISSION_DENIED", "organization permission denied")
	ErrIAMFinancialOperation  = infraerrors.Forbidden("IAM_FINANCIAL_OPERATION_DENIED", "IAM users cannot perform this financial operation")
)

var iamLoginNamePattern = regexp.MustCompile(`^[A-Za-z0-9._-]{1,64}$`)
var rootAccountIDPattern = regexp.MustCompile(`^[1-9][0-9]{15}$`)

// englishCompanyNamePattern restricts the company english name to English
// letters, digits, spaces and a small set of common punctuation; a separate
// pattern requires at least one letter so pure-number names are rejected.
var englishCompanyNamePattern = regexp.MustCompile(`^[A-Za-z0-9 &.,'()\-]+$`)
var englishCompanyNameLetter = regexp.MustCompile(`[A-Za-z]`)

func CanonicalIAMPrincipal(loginName, accountID string) string {
	return strings.TrimSpace(loginName) + "@" + strings.TrimSpace(accountID) + "." + IAMPrincipalDomain
}

func parseIAMPrincipal(principal string) (string, string, bool) {
	loginName, host, found := strings.Cut(strings.TrimSpace(principal), "@")
	suffix := "." + IAMPrincipalDomain
	if !found || strings.Contains(host, "@") || !strings.HasSuffix(strings.ToLower(host), suffix) {
		return "", "", false
	}
	accountID := host[:len(host)-len(suffix)]
	if !iamLoginNamePattern.MatchString(loginName) || !rootAccountIDPattern.MatchString(accountID) {
		return "", "", false
	}
	return loginName, accountID, true
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
	RequestedEnglishName string `json:"requested_english_name"`
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
	ExternalUserID     string     `json:"external_user_id"`
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
}

type BillingContext struct {
	ConsumerUserID  int64
	OrganizationID  *int64
	PayerUserID     int64
	BalanceSource   string
	AuthzGeneration int64
}

type OrganizationUsageFilter struct {
	Start    time.Time
	End      time.Time
	MemberID *int64
	APIKeyID *int64
	Model    string
	Endpoint string
	Status   string
	Page     int
	PageSize int
}

type OrganizationUsageRow struct {
	ID            int64     `json:"id"`
	MemberUserID  int64     `json:"member_user_id"`
	MemberLogin   string    `json:"member_login"`
	APIKeyName    string    `json:"api_key_name"`
	Model         string    `json:"model"`
	InputTokens   int       `json:"input_tokens"`
	OutputTokens  int       `json:"output_tokens"`
	ActualCost    string    `json:"actual_cost"`
	Endpoint      string    `json:"endpoint"`
	Status        string    `json:"status"`
	DurationMS    *int      `json:"duration_ms,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
	BalanceSource string    `json:"balance_source"`
}

type OrganizationUsageStats struct {
	Requests     int64  `json:"requests"`
	InputTokens  int64  `json:"input_tokens"`
	OutputTokens int64  `json:"output_tokens"`
	ActualCost   string `json:"actual_cost"`
}

type OrganizationUsageTrendPoint struct {
	Bucket     time.Time `json:"bucket"`
	Requests   int64     `json:"requests"`
	Tokens     int64     `json:"tokens"`
	ActualCost string    `json:"actual_cost"`
}

type OrganizationRepository interface {
	GetContextForUser(ctx context.Context, userID int64) (*OrganizationContext, error)
	GetApplicationForUser(ctx context.Context, userID int64) (*CompanyApplication, error)
	GetApplication(ctx context.Context, applicationID int64) (*CompanyApplicationDetail, error)
	SubmitApplication(ctx context.Context, userID int64, name, normalizedName, englishName, normalizedEnglishName, companySize, idempotencyKey, fee, currency string) (*CompanyApplication, error)
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
	FindIAMByPrincipal(ctx context.Context, loginName, accountID string) (*User, *OrganizationContext, error)
	ListPolicies(ctx context.Context, actorID int64) ([]ManagedPolicyView, error)
	ListMemberPolicyAttachments(ctx context.Context, ownerID, memberUserID int64) ([]ManagedPolicyView, error)
	SetPolicyAttachment(ctx context.Context, ownerID, memberUserID int64, policyKey string, attach bool, correlationID string) error
	TransferBalance(ctx context.Context, ownerID, memberUserID int64, amount, idempotencyKey string, reclaim bool) error
	FinanceSummary(ctx context.Context, userID int64) (*FinanceSummary, error)
	ListUsage(ctx context.Context, userID int64, filter OrganizationUsageFilter) ([]OrganizationUsageRow, int64, error)
	UsageStats(ctx context.Context, userID int64, filter OrganizationUsageFilter) (*OrganizationUsageStats, error)
	UsageTrend(ctx context.Context, userID int64, filter OrganizationUsageFilter) ([]OrganizationUsageTrendPoint, error)
	ResolveBillingContext(ctx context.Context, consumerUserID int64) (*BillingContext, error)
	Reconcile(ctx context.Context) (map[string]int64, error)
	ListOrganizationUserIDs(ctx context.Context, organizationID int64) ([]int64, error)
}

// CompanyUpgradeChargeReader reports whether company upgrade should charge the
// upgrade fee / freeze funds. Injected from the settings layer.
type CompanyUpgradeChargeReader interface {
	IsCompanyUpgradeChargeEnabled(ctx context.Context) bool
}

type OrganizationService struct {
	repo         OrganizationRepository
	userRepo     UserRepository
	cfg          *config.Config
	authCache    APIKeyAuthCacheInvalidator
	chargeReader CompanyUpgradeChargeReader
}

func (s *OrganizationService) SetAuthCacheInvalidator(invalidator APIKeyAuthCacheInvalidator) {
	if s != nil {
		s.authCache = invalidator
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

// normalizeEnglishName validates and normalizes the globally-unique company
// english name. It returns the display form (whitespace-collapsed, original
// case preserved) and the normalized lowercase form used for uniqueness.
func normalizeEnglishName(value string) (string, string, error) {
	name := strings.Join(strings.Fields(strings.TrimSpace(value)), " ")
	if name == "" || len(name) > 255 ||
		!englishCompanyNamePattern.MatchString(name) ||
		!englishCompanyNameLetter.MatchString(name) {
		return "", "", ErrCompanyEnglishNameInvalid
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
	result := &CompanyUpgradeEligibility{FeeAmount: "20.00000000", FeeCurrency: "USD"}
	if s.cfg != nil {
		result.FeeAmount = decimal.NewFromFloat(s.cfg.Company.UpgradeFee).StringFixed(8)
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

func (s *OrganizationService) SubmitApplication(ctx context.Context, userID int64, name, englishName, companySize, idempotencyKey string) (*CompanyApplication, error) {
	if s.cfg == nil || !s.cfg.Company.ApplicationsEnabled {
		return nil, ErrCompanyFeatureDisabled
	}
	name, normalized, err := normalizeCompanyName(name)
	if err != nil {
		return nil, err
	}
	englishName, normalizedEnglish, err := normalizeEnglishName(englishName)
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
	fee := decimal.NewFromFloat(s.cfg.Company.UpgradeFee).StringFixed(8)
	if !s.upgradeChargeEnabled(ctx) {
		// Charging disabled: snapshot a zero fee so that reserve/capture/release
		// become no-op amount=0 operations and the balance is never frozen.
		fee = decimal.Zero.StringFixed(8)
	}
	return s.repo.SubmitApplication(ctx, userID, name, normalized, englishName, normalizedEnglish, companySize, idempotencyKey, fee, s.cfg.Company.UpgradeCurrency)
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

func (s *OrganizationService) CreateIAMMember(ctx context.Context, ownerID int64, loginName, recoveryEmail, password string, mustChangePassword bool) (*IAMMember, string, error) {
	if s.cfg == nil || !s.cfg.Company.IAMEnabled {
		return nil, "", ErrIAMFeatureDisabled
	}
	loginName = strings.ToLower(strings.TrimSpace(loginName))
	if !iamLoginNamePattern.MatchString(loginName) {
		return nil, "", ErrIAMLoginName
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
		IdentityType: IdentityTypeIAM, LoginName: loginName, PasswordHash: string(hash), Role: RoleUser,
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
	if s.cfg == nil || !s.cfg.Company.IAMEnabled {
		return nil, nil, ErrIAMFeatureDisabled
	}
	loginName, accountID, ok := parseIAMPrincipal(principal)
	if !ok {
		return nil, nil, ErrInvalidCredentials
	}
	user, org, err := s.repo.FindIAMByPrincipal(ctx, loginName, accountID)
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
	if policyKey != PolicyCompanyFinanceReadOnly && policyKey != PolicyCompanySharedBalance {
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

func (s *OrganizationService) FinanceSummary(ctx context.Context, userID int64) (*FinanceSummary, error) {
	return s.repo.FinanceSummary(ctx, userID)
}

func (s *OrganizationService) ListUsage(ctx context.Context, userID int64, filter OrganizationUsageFilter) ([]OrganizationUsageRow, int64, error) {
	return s.repo.ListUsage(ctx, userID, filter)
}

func (s *OrganizationService) UsageStats(ctx context.Context, userID int64, filter OrganizationUsageFilter) (*OrganizationUsageStats, error) {
	return s.repo.UsageStats(ctx, userID, filter)
}

func (s *OrganizationService) UsageTrend(ctx context.Context, userID int64, filter OrganizationUsageFilter) ([]OrganizationUsageTrendPoint, error) {
	return s.repo.UsageTrend(ctx, userID, filter)
}

func (s *OrganizationService) Reconcile(ctx context.Context) (map[string]int64, error) {
	return s.repo.Reconcile(ctx)
}

type BillingContextResolver struct{ repo OrganizationRepository }

func NewBillingContextResolver(repo OrganizationRepository) *BillingContextResolver {
	return &BillingContextResolver{repo: repo}
}

func (r *BillingContextResolver) Resolve(ctx context.Context, consumerUserID int64) (*BillingContext, error) {
	if r == nil || r.repo == nil {
		return &BillingContext{ConsumerUserID: consumerUserID, PayerUserID: consumerUserID, BalanceSource: "self"}, nil
	}
	resolved, err := r.repo.ResolveBillingContext(ctx, consumerUserID)
	if errors.Is(err, ErrCompanyNotFound) {
		return &BillingContext{ConsumerUserID: consumerUserID, PayerUserID: consumerUserID, BalanceSource: "self"}, nil
	}
	if err != nil {
		organizationRuntimeMetrics.payerResolutionFailures.Add(1)
		return nil, fmt.Errorf("resolve billing context: %w", err)
	}
	return resolved, nil
}

func GuardIAMFinancialOperation(user *User) error {
	if user != nil && user.IsIAM() {
		organizationRuntimeMetrics.deniedIAMFinancialOps.Add(1)
		return ErrIAMFinancialOperation
	}
	return nil
}
