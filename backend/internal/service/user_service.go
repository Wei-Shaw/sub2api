package service

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"image"
	"image/color"
	stddraw "image/draw"
	_ "image/gif"
	"image/jpeg"
	_ "image/png"
	"log/slog"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	xdraw "golang.org/x/image/draw"
	"golang.org/x/sync/singleflight"
)

var (
	ErrUserNotFound             = infraerrors.NotFound("USER_NOT_FOUND", "user not found")
	ErrPasswordIncorrect        = infraerrors.BadRequest("PASSWORD_INCORRECT", "current password is incorrect")
	ErrInsufficientPerms        = infraerrors.Forbidden("INSUFFICIENT_PERMISSIONS", "insufficient permissions")
	ErrNotifyCodeUserRateLimit  = infraerrors.TooManyRequests("NOTIFY_CODE_USER_RATE_LIMIT", "too many verification codes requested, please try again later")
	ErrAvatarInvalid            = infraerrors.BadRequest("AVATAR_INVALID", "avatar must be a valid image data URL or http(s) URL")
	ErrAvatarTooLarge           = infraerrors.BadRequest("AVATAR_TOO_LARGE", "avatar image must be 100KB or smaller")
	ErrAvatarNotImage           = infraerrors.BadRequest("AVATAR_NOT_IMAGE", "avatar content must be an image")
	ErrIdentityProviderInvalid  = infraerrors.BadRequest("IDENTITY_PROVIDER_INVALID", "identity provider is invalid")
	ErrIdentityRedirectInvalid  = infraerrors.BadRequest("IDENTITY_REDIRECT_INVALID", "identity redirect path is invalid")
	ErrIdentityUnbindLastMethod = infraerrors.Conflict(
		"IDENTITY_UNBIND_LAST_METHOD",
		"bind another sign-in method before unbinding this provider",
	)
)

const (
	maxNotifyEmails      = 3 // Maximum number of notification emails per user
	maxInlineAvatarBytes = 100 * 1024
	targetAvatarBytes    = 20 * 1024

	// User-level rate limiting for notify email verification codes
	notifyCodeUserRateLimit  = 5
	notifyCodeUserRateWindow = 10 * time.Minute

	defaultUserIdentityRedirect = "/settings/profile"
	userLastActiveMinTouch      = 10 * time.Minute
	userLastActiveFailBackoff   = 30 * time.Second
)

var (
	avatarScaleSteps   = []float64{1, 0.92, 0.84, 0.76, 0.68, 0.6, 0.52, 0.44, 0.36}
	avatarQualitySteps = []int{88, 80, 72, 64, 56, 48, 40, 32}
)

// UserListFilters contains all filter options for listing users
type UserListFilters struct {
	Status     string           // User status filter
	Role       string           // User role filter
	Search     string           // Search in email, username
	GroupName  string           // Filter by allowed group name (fuzzy match)
	Attributes map[int64]string // Custom attribute filters: attributeID -> value
	// IncludeSubscriptions controls whether ListWithFilters should load active subscriptions.
	// For large datasets this can be expensive; admin list pages should enable it on demand.
	// nil means not specified (default: load subscriptions for backward compatibility).
	IncludeSubscriptions *bool
}

type UserRepository interface {
	Create(ctx context.Context, user *User) error
	GetByID(ctx context.Context, id int64) (*User, error)
	GetByEmail(ctx context.Context, email string) (*User, error)
	GetFirstAdmin(ctx context.Context) (*User, error)
	Update(ctx context.Context, user *User) error
	Delete(ctx context.Context, id int64) error
	GetUserAvatar(ctx context.Context, userID int64) (*UserAvatar, error)
	UpsertUserAvatar(ctx context.Context, userID int64, input UpsertUserAvatarInput) (*UserAvatar, error)
	DeleteUserAvatar(ctx context.Context, userID int64) error

	List(ctx context.Context, params pagination.PaginationParams) ([]User, *pagination.PaginationResult, error)
	ListWithFilters(ctx context.Context, params pagination.PaginationParams, filters UserListFilters) ([]User, *pagination.PaginationResult, error)
	GetLatestUsedAtByUserIDs(ctx context.Context, userIDs []int64) (map[int64]*time.Time, error)
	GetLatestUsedAtByUserID(ctx context.Context, userID int64) (*time.Time, error)
	UpdateUserLastActiveAt(ctx context.Context, userID int64, activeAt time.Time) error

	UpdateBalance(ctx context.Context, id int64, amount float64) error
	DeductBalance(ctx context.Context, id int64, amount float64) error
	UpdateConcurrency(ctx context.Context, id int64, amount int) error
	ExistsByEmail(ctx context.Context, email string) (bool, error)
	RemoveGroupFromAllowedGroups(ctx context.Context, groupID int64) (int64, error)
	// AddGroupToAllowedGroups 将指定分组增量添加到用户的 allowed_groups（幂等，冲突忽略）
	AddGroupToAllowedGroups(ctx context.Context, userID int64, groupID int64) error
	// RemoveGroupFromUserAllowedGroups 移除单个用户的指定分组权限
	RemoveGroupFromUserAllowedGroups(ctx context.Context, userID int64, groupID int64) error
	ListUserAuthIdentities(ctx context.Context, userID int64) ([]UserAuthIdentityRecord, error)
	UnbindUserAuthProvider(ctx context.Context, userID int64, provider string) error

	// TOTP 双因素认证
	UpdateTotpSecret(ctx context.Context, userID int64, encryptedSecret *string) error
	EnableTotp(ctx context.Context, userID int64) error
	DisableTotp(ctx context.Context, userID int64) error
}

type UserAuthIdentityRecord struct {
	ProviderType    string
	ProviderKey     string
	ProviderSubject string
	VerifiedAt      *time.Time
	Issuer          *string
	Metadata        map[string]any
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

type UserIdentitySummary struct {
	Provider      string     `json:"provider"`
	Bound         bool       `json:"bound"`
	BoundCount    int        `json:"bound_count"`
	DisplayName   string     `json:"display_name,omitempty"`
	AvatarURL     string     `json:"-"`
	SubjectHint   string     `json:"subject_hint,omitempty"`
	ProviderKey   string     `json:"provider_key,omitempty"`
	VerifiedAt    *time.Time `json:"verified_at,omitempty"`
	BindStartPath string     `json:"bind_start_path,omitempty"`
	CanBind       bool       `json:"can_bind"`
	CanUnbind     bool       `json:"can_unbind"`
	NoteKey       string     `json:"note_key,omitempty"`
	Note          string     `json:"note,omitempty"`
}

type UserIdentitySummarySet struct {
	Email   UserIdentitySummary `json:"email"`
	LinuxDo UserIdentitySummary `json:"linuxdo"`
	OIDC    UserIdentitySummary `json:"oidc"`
	WeChat  UserIdentitySummary `json:"wechat"`
}

type StartUserIdentityBindingRequest struct {
	Provider   string
	RedirectTo string
}

type StartUserIdentityBindingResult struct {
	Provider           string `json:"provider"`
	AuthorizeURL       string `json:"authorize_url"`
	Method             string `json:"method"`
	UseBrowserRedirect bool   `json:"use_browser_redirect"`
}

const (
	userIdentityNoteEmailManagedFromProfile = "profile.authBindings.notes.emailManagedFromProfile"
	userIdentityNoteCanUnbind               = "profile.authBindings.notes.canUnbind"
	userIdentityNoteBindAnotherBeforeUnbind = "profile.authBindings.notes.bindAnotherBeforeUnbind"
)

// UpdateProfileRequest 更新用户资料请求
type UpdateProfileRequest struct {
	Email                  *string  `json:"email"`
	Username               *string  `json:"username"`
	AvatarURL              *string  `json:"avatar_url"`
	Concurrency            *int     `json:"concurrency"`
	BalanceNotifyEnabled   *bool    `json:"balance_notify_enabled"`
	BalanceNotifyThreshold *float64 `json:"balance_notify_threshold"`
}

type UserAvatar struct {
	StorageProvider string
	StorageKey      string
	URL             string
	ContentType     string
	ByteSize        int
	SHA256          string
}

type UpsertUserAvatarInput struct {
	StorageProvider string
	StorageKey      string
	URL             string
	ContentType     string
	ByteSize        int
	SHA256          string
}

type userProfileIdentityTxRunner interface {
	WithUserProfileIdentityTx(ctx context.Context, fn func(txCtx context.Context) error) error
}

// ChangePasswordRequest 修改密码请求
type ChangePasswordRequest struct {
	CurrentPassword string `json:"current_password"`
	NewPassword     string `json:"new_password"`
}

// UserService 用户服务
type UserService struct {
	userRepo             UserRepository
	settingRepo          SettingRepository
	authCacheInvalidator APIKeyAuthCacheInvalidator
	billingCache         BillingCache
	lastActiveTouchL1    sync.Map
	lastActiveTouchSF    singleflight.Group
}

// NewUserService 创建用户服务实例
func NewUserService(userRepo UserRepository, settingRepo SettingRepository, authCacheInvalidator APIKeyAuthCacheInvalidator, billingCache BillingCache) *UserService {
	return &UserService{
		userRepo:             userRepo,
		settingRepo:          settingRepo,
		authCacheInvalidator: authCacheInvalidator,
		billingCache:         billingCache,
	}
}

// GetFirstAdmin 获取首个管理员用户（用于 Admin API Key 认证）
func (s *UserService) GetFirstAdmin(ctx context.Context) (*User, error) {
	admin, err := s.userRepo.GetFirstAdmin(ctx)
	if err != nil {
		return nil, fmt.Errorf("get first admin: %w", err)
	}
	return admin, nil
}

// GetProfile 获取用户资料
func (s *UserService) GetProfile(ctx context.Context, userID int64) (*User, error) {
	user, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("get user: %w", err)
	}
	normalizeLoadedUserTokenVersion(user)
	if err := s.hydrateUserAvatar(ctx, user); err != nil {
		return nil, fmt.Errorf("get user avatar: %w", err)
	}
	return user, nil
}

func (s *UserService) GetProfileIdentitySummaries(ctx context.Context, userID int64, user *User) (UserIdentitySummarySet, error) {
	if user == nil {
		var err error
		user, err = s.userRepo.GetByID(ctx, userID)
		if err != nil {
			return UserIdentitySummarySet{}, fmt.Errorf("get user: %w", err)
		}
	}

	records, err := s.listUserAuthIdentities(ctx, userID)
	if err != nil {
		return UserIdentitySummarySet{}, err
	}

	summaries := UserIdentitySummarySet{
		Email:   s.buildEmailIdentitySummary(user, records),
		LinuxDo: s.buildProviderIdentitySummary("linuxdo", user, records),
		OIDC:    s.buildProviderIdentitySummary("oidc", user, records),
		WeChat:  s.buildProviderIdentitySummary("wechat", user, records),
	}

	s.applyExplicitProviderAvailability(ctx, &summaries)
	return summaries, nil
}

func (s *UserService) applyExplicitProviderAvailability(ctx context.Context, summaries *UserIdentitySummarySet) {
	if s == nil || summaries == nil || s.settingRepo == nil {
		return
	}

	settings, err := s.settingRepo.GetMultiple(ctx, []string{
		SettingKeyLinuxDoConnectEnabled,
		SettingKeyOIDCConnectEnabled,
		SettingKeyWeChatConnectEnabled,
		SettingKeyWeChatConnectOpenEnabled,
		SettingKeyWeChatConnectMPEnabled,
		SettingKeyWeChatConnectMobileEnabled,
		SettingKeyWeChatConnectMode,
	})
	if err != nil {
		return
	}

	if raw, ok := settings[SettingKeyLinuxDoConnectEnabled]; ok && strings.TrimSpace(raw) != "" && raw != "true" {
		disableIdentityBindAction(&summaries.LinuxDo)
	}
	if raw, ok := settings[SettingKeyOIDCConnectEnabled]; ok && strings.TrimSpace(raw) != "" && raw != "true" {
		disableIdentityBindAction(&summaries.OIDC)
	}
	if raw, ok := settings[SettingKeyWeChatConnectEnabled]; ok && strings.TrimSpace(raw) != "" {
		if raw != "true" {
			disableIdentityBindAction(&summaries.WeChat)
			return
		}
		openEnabled, mpEnabled, _ := parseWeChatConnectCapabilitySettings(settings, true, settings[SettingKeyWeChatConnectMode])
		if !openEnabled && !mpEnabled {
			disableIdentityBindAction(&summaries.WeChat)
		}
	}
}

func disableIdentityBindAction(summary *UserIdentitySummary) {
	if summary == nil || summary.Bound {
		return
	}
	summary.CanBind = false
	summary.BindStartPath = ""
}

func (s *UserService) PrepareIdentityBindingStart(_ context.Context, req StartUserIdentityBindingRequest) (*StartUserIdentityBindingResult, error) {
	provider := normalizeUserIdentityProvider(req.Provider)
	if provider == "" {
		return nil, ErrIdentityProviderInvalid
	}

	authorizeURL, err := buildUserIdentityBindAuthorizeURL(provider, req.RedirectTo)
	if err != nil {
		return nil, err
	}

	return &StartUserIdentityBindingResult{
		Provider:           provider,
		AuthorizeURL:       authorizeURL,
		Method:             "GET",
		UseBrowserRedirect: true,
	}, nil
}

func (s *UserService) UnbindUserAuthProvider(ctx context.Context, userID int64, provider string) (*User, error) {
	user, _, err := s.UnbindUserAuthProviderWithResult(ctx, userID, provider)
	return user, err
}

func (s *UserService) UnbindUserAuthProviderWithResult(ctx context.Context, userID int64, provider string) (*User, bool, error) {
	provider = normalizeUserIdentityProvider(provider)
	if provider == "" || provider == "email" {
		return nil, false, ErrIdentityProviderInvalid
	}

	user, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		return nil, false, fmt.Errorf("get user: %w", err)
	}

	records, err := s.listUserAuthIdentities(ctx, userID)
	if err != nil {
		return nil, false, err
	}
	if len(filterUserAuthIdentities(records, provider)) == 0 {
		return user, false, nil
	}
	if !s.canUnbindProvider(provider, user, records) {
		return nil, false, ErrIdentityUnbindLastMethod
	}

	if err := s.userRepo.UnbindUserAuthProvider(ctx, userID, provider); err != nil {
		return nil, false, err
	}
	if s.authCacheInvalidator != nil {
		s.authCacheInvalidator.InvalidateAuthCacheByUserID(ctx, userID)
	}

	updatedUser, err := s.GetProfile(ctx, userID)
	if err != nil {
		return nil, false, err
	}
	return updatedUser, true, nil
}

// UpdateProfile 更新用户资料
func (s *UserService) UpdateProfile(ctx context.Context, userID int64, req UpdateProfileRequest) (*User, error) {
	if txRunner, ok := s.userRepo.(userProfileIdentityTxRunner); ok {
		var (
			updated        *User
			oldConcurrency int
		)
		if err := txRunner.WithUserProfileIdentityTx(ctx, func(txCtx context.Context) error {
			var err error
			updated, oldConcurrency, err = s.updateProfile(txCtx, userID, req)
			return err
		}); err != nil {
			return nil, err
		}
		if s.authCacheInvalidator != nil && updated != nil && updated.Concurrency != oldConcurrency {
			s.authCacheInvalidator.InvalidateAuthCacheByUserID(ctx, userID)
		}
		return updated, nil
	}

	updated, oldConcurrency, err := s.updateProfile(ctx, userID, req)
	if err != nil {
		return nil, err
	}
	if s.authCacheInvalidator != nil && updated.Concurrency != oldConcurrency {
		s.authCacheInvalidator.InvalidateAuthCacheByUserID(ctx, userID)
	}
	return updated, nil
}

func (s *UserService) updateProfile(ctx context.Context, userID int64, req UpdateProfileRequest) (*User, int, error) {
	user, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		return nil, 0, fmt.Errorf("get user: %w", err)
	}
	oldConcurrency := user.Concurrency

	// 更新字段
	if req.Email != nil {
		// 检查新邮箱是否已被使用
		exists, err := s.userRepo.ExistsByEmail(ctx, *req.Email)
		if err != nil {
			return nil, oldConcurrency, fmt.Errorf("check email exists: %w", err)
		}
		if exists && *req.Email != user.Email {
			return nil, oldConcurrency, ErrEmailExists
		}
		user.Email = *req.Email
	}

	if req.Username != nil {
		user.Username = *req.Username
	}

	if req.AvatarURL != nil {
		avatar, err := s.SetAvatar(ctx, userID, *req.AvatarURL)
		if err != nil {
			return nil, oldConcurrency, err
		}
		applyUserAvatar(user, avatar)
	}

	if req.Concurrency != nil {
		user.Concurrency = *req.Concurrency
	}

	if req.BalanceNotifyEnabled != nil {
		user.BalanceNotifyEnabled = *req.BalanceNotifyEnabled
	}
	if req.BalanceNotifyThreshold != nil {
		if *req.BalanceNotifyThreshold <= 0 {
			user.BalanceNotifyThreshold = nil // clear to system default
		} else {
			user.BalanceNotifyThreshold = req.BalanceNotifyThreshold
		}
	}

	if err := s.userRepo.Update(ctx, user); err != nil {
		return nil, oldConcurrency, fmt.Errorf("update user: %w", err)
	}

	return user, oldConcurrency, nil
}

func (s *UserService) SetAvatar(ctx context.Context, userID int64, raw string) (*UserAvatar, error) {
	avatarValue := strings.TrimSpace(raw)
	if avatarValue == "" {
		if err := s.userRepo.DeleteUserAvatar(ctx, userID); err != nil {
			return nil, fmt.Errorf("delete avatar: %w", err)
		}
		return nil, nil
	}

	avatarInput, err := normalizeUserAvatarInput(avatarValue)
	if err != nil {
		return nil, err
	}

	avatar, err := s.userRepo.UpsertUserAvatar(ctx, userID, avatarInput)
	if err != nil {
		return nil, fmt.Errorf("upsert avatar: %w", err)
	}
	return avatar, nil
}

func applyUserAvatar(user *User, avatar *UserAvatar) {
	if user == nil {
		return
	}
	if avatar == nil {
		user.AvatarURL = ""
		user.AvatarSource = ""
		user.AvatarMIME = ""
		user.AvatarByteSize = 0
		user.AvatarSHA256 = ""
		return
	}

	user.AvatarURL = avatar.URL
	user.AvatarSource = avatar.StorageProvider
	user.AvatarMIME = avatar.ContentType
	user.AvatarByteSize = avatar.ByteSize
	user.AvatarSHA256 = avatar.SHA256
}

func normalizeUserAvatarInput(raw string) (UpsertUserAvatarInput, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return UpsertUserAvatarInput{}, ErrAvatarInvalid
	}
	if strings.HasPrefix(raw, "data:") {
		return normalizeInlineUserAvatarInput(raw)
	}

	parsed, err := url.Parse(raw)
	if err != nil || parsed == nil {
		return UpsertUserAvatarInput{}, ErrAvatarInvalid
	}
	if !strings.EqualFold(parsed.Scheme, "http") && !strings.EqualFold(parsed.Scheme, "https") {
		return UpsertUserAvatarInput{}, ErrAvatarInvalid
	}
	if strings.TrimSpace(parsed.Host) == "" {
		return UpsertUserAvatarInput{}, ErrAvatarInvalid
	}

	return UpsertUserAvatarInput{
		StorageProvider: "remote_url",
		URL:             raw,
	}, nil
}

func ValidateUserAvatar(raw string) error {
	_, err := normalizeUserAvatarInput(raw)
	return err
}

func normalizeInlineUserAvatarInput(raw string) (UpsertUserAvatarInput, error) {
	body := strings.TrimPrefix(raw, "data:")
	meta, encoded, ok := strings.Cut(body, ",")
	if !ok {
		return UpsertUserAvatarInput{}, ErrAvatarInvalid
	}
	meta = strings.TrimSpace(meta)
	encoded = strings.TrimSpace(encoded)
	if !strings.HasSuffix(strings.ToLower(meta), ";base64") {
		return UpsertUserAvatarInput{}, ErrAvatarInvalid
	}

	contentType := strings.TrimSpace(meta[:len(meta)-len(";base64")])
	if contentType == "" || !strings.HasPrefix(strings.ToLower(contentType), "image/") {
		return UpsertUserAvatarInput{}, ErrAvatarNotImage
	}

	decoded, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return UpsertUserAvatarInput{}, ErrAvatarInvalid
	}
	if len(decoded) > maxInlineAvatarBytes {
		return UpsertUserAvatarInput{}, ErrAvatarTooLarge
	}

	if len(decoded) > targetAvatarBytes {
		decoded, contentType, err = compressInlineAvatar(decoded)
		if err != nil {
			return UpsertUserAvatarInput{}, err
		}
		raw = "data:" + contentType + ";base64," + base64.StdEncoding.EncodeToString(decoded)
	}

	sum := sha256.Sum256(decoded)
	return UpsertUserAvatarInput{
		StorageProvider: "inline",
		URL:             raw,
		ContentType:     contentType,
		ByteSize:        len(decoded),
		SHA256:          hex.EncodeToString(sum[:]),
	}, nil
}

func compressInlineAvatar(decoded []byte) ([]byte, string, error) {
	src, _, err := image.Decode(bytes.NewReader(decoded))
	if err != nil {
		return nil, "", ErrAvatarInvalid
	}

	srcBounds := src.Bounds()
	if srcBounds.Empty() {
		return nil, "", ErrAvatarInvalid
	}

	for _, scale := range avatarScaleSteps {
		width := max(1, int(float64(srcBounds.Dx())*scale))
		height := max(1, int(float64(srcBounds.Dy())*scale))
		dst := image.NewRGBA(image.Rect(0, 0, width, height))
		stddraw.Draw(dst, dst.Bounds(), &image.Uniform{C: color.White}, image.Point{}, stddraw.Src)
		xdraw.CatmullRom.Scale(dst, dst.Bounds(), src, srcBounds, stddraw.Over, nil)

		for _, quality := range avatarQualitySteps {
			var buf bytes.Buffer
			if err := jpeg.Encode(&buf, dst, &jpeg.Options{Quality: quality}); err != nil {
				return nil, "", ErrAvatarInvalid
			}
			if buf.Len() <= targetAvatarBytes {
				return buf.Bytes(), "image/jpeg", nil
			}
		}
	}

	return nil, "", ErrAvatarTooLarge
}

func (s *UserService) buildEmailIdentitySummary(user *User, records []UserAuthIdentityRecord) UserIdentitySummary {
	summary := UserIdentitySummary{
		Provider:  "email",
		CanBind:   false,
		CanUnbind: false,
		NoteKey:   userIdentityNoteEmailManagedFromProfile,
		Note:      "Primary account email is managed from the profile form.",
	}
	if user == nil {
		return summary
	}

	filtered := filterUserAuthIdentities(records, "email")
	if len(filtered) > 0 {
		primary := selectPrimaryUserAuthIdentity(filtered)
		email := strings.TrimSpace(firstStringIdentityValue(primary.Metadata, "email"))
		if email == "" {
			email = strings.TrimSpace(primary.ProviderSubject)
		}
		if email == "" || isReservedEmail(email) {
			email = strings.TrimSpace(user.Email)
		}
		if email == "" || isReservedEmail(email) {
			email = strings.TrimSpace(primary.ProviderKey)
		}

		summary.Bound = true
		summary.BoundCount = len(filtered)
		summary.DisplayName = email
		summary.SubjectHint = maskEmailIdentity(email)
		summary.ProviderKey = strings.TrimSpace(primary.ProviderKey)
		summary.VerifiedAt = primary.VerifiedAt
		return summary
	}

	// Compatibility fallback for legacy normal-email users that predate auth_identities backfill.
	email := strings.TrimSpace(user.Email)
	if email == "" || isReservedEmail(email) {
		return summary
	}
	summary.Bound = true
	summary.BoundCount = 1
	summary.DisplayName = email
	summary.SubjectHint = maskEmailIdentity(email)
	summary.ProviderKey = "email"
	return summary
}

func (s *UserService) buildProviderIdentitySummary(provider string, user *User, records []UserAuthIdentityRecord) UserIdentitySummary {
	summary := UserIdentitySummary{
		Provider:  provider,
		CanUnbind: false,
	}
	filtered := filterUserAuthIdentities(records, provider)
	if len(filtered) == 0 {
		summary.CanBind = true
		bindStartPath, err := buildUserIdentityBindAuthorizeURL(provider, "")
		if err == nil {
			summary.BindStartPath = bindStartPath
		}
		return summary
	}

	primary := selectPrimaryUserAuthIdentity(filtered)
	summary.Bound = true
	summary.BoundCount = len(filtered)
	summary.DisplayName = userAuthIdentityDisplayName(primary)
	summary.AvatarURL = strings.TrimSpace(firstStringIdentityValue(primary.Metadata, "avatar_url", "suggested_avatar_url", "headimgurl"))
	summary.SubjectHint = maskOpaqueIdentity(primary.ProviderSubject)
	summary.ProviderKey = strings.TrimSpace(primary.ProviderKey)
	summary.VerifiedAt = primary.VerifiedAt
	summary.CanUnbind = s.canUnbindProvider(provider, user, records)
	if summary.CanUnbind {
		summary.NoteKey = userIdentityNoteCanUnbind
		summary.Note = "You can unbind this sign-in method."
	} else {
		summary.NoteKey = userIdentityNoteBindAnotherBeforeUnbind
		summary.Note = "Bind another sign-in method before unbinding."
	}
	return summary
}

func (s *UserService) canUnbindProvider(provider string, user *User, records []UserAuthIdentityRecord) bool {
	if provider == "" || provider == "email" || len(filterUserAuthIdentities(records, provider)) == 0 {
		return false
	}

	if s.canUseEmailAsSignInMethod(user, records) {
		return true
	}

	for _, candidate := range []string{"linuxdo", "oidc", "wechat"} {
		if candidate == provider {
			continue
		}
		if len(filterUserAuthIdentities(records, candidate)) > 0 {
			return true
		}
	}

	return false
}

func (s *UserService) canUseEmailAsSignInMethod(user *User, records []UserAuthIdentityRecord) bool {
	if user == nil {
		return false
	}

	email := strings.ToLower(strings.TrimSpace(user.Email))
	if email == "" || isReservedEmail(email) {
		return false
	}

	if emailSignupSourceAllowsLogin(user.SignupSource) {
		return true
	}

	for _, record := range filterUserAuthIdentities(records, "email") {
		if emailIdentitySupportsSignIn(record) {
			return true
		}
	}

	return false
}

func emailSignupSourceAllowsLogin(signupSource string) bool {
	signupSource = strings.ToLower(strings.TrimSpace(signupSource))
	return signupSource == "" || signupSource == "email"
}

func emailIdentitySupportsSignIn(record UserAuthIdentityRecord) bool {
	source := strings.TrimSpace(firstStringIdentityValue(record.Metadata, "source"))
	switch source {
	case "auth_service_email_bind", "auth_service_login_backfill", "auth_service_dual_write":
		return true
	default:
		return false
	}
}

func (s *UserService) listUserAuthIdentities(ctx context.Context, userID int64) ([]UserAuthIdentityRecord, error) {
	if userID <= 0 || s == nil || s.userRepo == nil {
		return nil, nil
	}
	return s.userRepo.ListUserAuthIdentities(ctx, userID)
}

func buildUserIdentityBindAuthorizeURL(provider, redirectTo string) (string, error) {
	provider = normalizeUserIdentityProvider(provider)
	if provider == "" || provider == "email" {
		return "", ErrIdentityProviderInvalid
	}

	redirectTo, err := normalizeUserIdentityRedirect(redirectTo)
	if err != nil {
		return "", err
	}

	path := ""
	switch provider {
	case "linuxdo":
		path = "/api/v1/auth/oauth/linuxdo/bind/start"
	case "oidc":
		path = "/api/v1/auth/oauth/oidc/bind/start"
	case "wechat":
		path = "/api/v1/auth/oauth/wechat/bind/start"
	default:
		return "", ErrIdentityProviderInvalid
	}

	query := url.Values{}
	query.Set("redirect", redirectTo)
	query.Set("intent", "bind_current_user")
	return path + "?" + query.Encode(), nil
}

func normalizeUserIdentityProvider(provider string) string {
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case "linuxdo":
		return "linuxdo"
	case "oidc":
		return "oidc"
	case "wechat":
		return "wechat"
	case "email":
		return "email"
	default:
		return ""
	}
}

func normalizeUserIdentityRedirect(raw string) (string, error) {
	redirect := strings.TrimSpace(raw)
	if redirect == "" {
		return defaultUserIdentityRedirect, nil
	}
	if len(redirect) > 2048 || !strings.HasPrefix(redirect, "/") || strings.HasPrefix(redirect, "//") {
		return "", ErrIdentityRedirectInvalid
	}
	return redirect, nil
}

func filterUserAuthIdentities(records []UserAuthIdentityRecord, provider string) []UserAuthIdentityRecord {
	if len(records) == 0 {
		return nil
	}
	filtered := make([]UserAuthIdentityRecord, 0, len(records))
	for _, record := range records {
		if strings.EqualFold(strings.TrimSpace(record.ProviderType), provider) {
			filtered = append(filtered, record)
		}
	}
	return filtered
}

func selectPrimaryUserAuthIdentity(records []UserAuthIdentityRecord) UserAuthIdentityRecord {
	if len(records) == 0 {
		return UserAuthIdentityRecord{}
	}
	sort.SliceStable(records, func(i, j int) bool {
		left := userAuthIdentitySortTime(records[i])
		right := userAuthIdentitySortTime(records[j])
		if !left.Equal(right) {
			return left.After(right)
		}
		return records[i].ProviderKey < records[j].ProviderKey
	})
	return records[0]
}

func userAuthIdentitySortTime(record UserAuthIdentityRecord) time.Time {
	if record.VerifiedAt != nil && !record.VerifiedAt.IsZero() {
		return record.VerifiedAt.UTC()
	}
	if !record.UpdatedAt.IsZero() {
		return record.UpdatedAt.UTC()
	}
	if !record.CreatedAt.IsZero() {
		return record.CreatedAt.UTC()
	}
	return time.Time{}
}

func userAuthIdentityDisplayName(record UserAuthIdentityRecord) string {
	if displayName := firstStringIdentityValue(record.Metadata,
		"display_name",
		"suggested_display_name",
		"username",
		"name",
		"nickname",
		"email",
	); displayName != "" {
		return displayName
	}
	if subject := strings.TrimSpace(record.ProviderSubject); subject != "" {
		return subject
	}
	return strings.TrimSpace(record.ProviderType)
}

func firstStringIdentityValue(values map[string]any, keys ...string) string {
	for _, key := range keys {
		raw, ok := values[key]
		if !ok {
			continue
		}
		switch value := raw.(type) {
		case string:
			if trimmed := strings.TrimSpace(value); trimmed != "" {
				return trimmed
			}
		case fmt.Stringer:
			if trimmed := strings.TrimSpace(value.String()); trimmed != "" {
				return trimmed
			}
		}
	}
	return ""
}

func maskEmailIdentity(email string) string {
	local, domain, ok := strings.Cut(strings.TrimSpace(email), "@")
	if !ok || local == "" || domain == "" {
		return maskOpaqueIdentity(email)
	}
	runes := []rune(local)
	if len(runes) == 1 {
		return string(runes[0]) + "***@" + domain
	}
	return string(runes[0]) + "***" + string(runes[len(runes)-1]) + "@" + domain
}

func maskOpaqueIdentity(value string) string {
	value = strings.TrimSpace(value)
	runes := []rune(value)
	switch {
	case len(runes) == 0:
		return ""
	case len(runes) <= 4:
		return string(runes[0]) + "***"
	case len(runes) <= 8:
		return string(runes[:2]) + "***" + string(runes[len(runes)-1:])
	default:
		return string(runes[:3]) + "***" + string(runes[len(runes)-3:])
	}
}

// ChangePassword 修改密码
// Security: Increments TokenVersion to invalidate all existing JWT tokens
func (s *UserService) ChangePassword(ctx context.Context, userID int64, req ChangePasswordRequest) error {
	user, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		return fmt.Errorf("get user: %w", err)
	}

	// 验证当前密码
	if !user.CheckPassword(req.CurrentPassword) {
		return ErrPasswordIncorrect
	}

	if err := user.SetPassword(req.NewPassword); err != nil {
		return fmt.Errorf("set password: %w", err)
	}

	// Increment TokenVersion to invalidate all existing tokens
	// This ensures that any tokens issued before the password change become invalid
	user.TokenVersion++

	if err := s.userRepo.Update(ctx, user); err != nil {
		return fmt.Errorf("update user: %w", err)
	}

	return nil
}

// GetByID 根据ID获取用户（管理员功能）
func (s *UserService) GetByID(ctx context.Context, id int64) (*User, error) {
	user, err := s.userRepo.GetByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("get user: %w", err)
	}
	normalizeLoadedUserTokenVersion(user)
	if err := s.hydrateUserAvatar(ctx, user); err != nil {
		return nil, fmt.Errorf("get user avatar: %w", err)
	}
	return user, nil
}

func normalizeLoadedUserTokenVersion(user *User) {
	if user == nil || user.TokenVersionResolved {
		return
	}
	user.TokenVersion = resolvedTokenVersion(user)
	user.TokenVersionResolved = true
}

// TouchLastActive 通过防抖更新 users.last_active_at，减少鉴权热路径写放大。
// 该操作为尽力而为，不应中断正常请求。
func (s *UserService) TouchLastActive(ctx context.Context, userID int64) {
	if s == nil || s.userRepo == nil || userID <= 0 {
		return
	}

	user, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		slog.Debug("skip touch user last active after load failure", "user_id", userID, "error", err)
		return
	}
	s.TouchLastActiveForUser(ctx, user)
}

// TouchLastActiveForUser 使用已加载的用户信息更新 last_active_at，避免重复读取数据库。
func (s *UserService) TouchLastActiveForUser(ctx context.Context, user *User) {
	if s == nil || s.userRepo == nil || user == nil || user.ID <= 0 {
		return
	}

	now := time.Now()
	if userLastActiveFresh(user.LastActiveAt, now) {
		return
	}
	if v, ok := s.lastActiveTouchL1.Load(user.ID); ok {
		if nextAllowedAt, ok := v.(time.Time); ok && now.Before(nextAllowedAt) {
			return
		}
	}

	_, err, _ := s.lastActiveTouchSF.Do(strconv.FormatInt(user.ID, 10), func() (any, error) {
		latest := time.Now()
		if v, ok := s.lastActiveTouchL1.Load(user.ID); ok {
			if nextAllowedAt, ok := v.(time.Time); ok && latest.Before(nextAllowedAt) {
				return nil, nil
			}
		}
		if userLastActiveFresh(user.LastActiveAt, latest) {
			return nil, nil
		}
		if err := s.userRepo.UpdateUserLastActiveAt(ctx, user.ID, latest); err != nil {
			s.lastActiveTouchL1.Store(user.ID, latest.Add(userLastActiveFailBackoff))
			return nil, fmt.Errorf("touch user last active: %w", err)
		}
		s.lastActiveTouchL1.Store(user.ID, latest.Add(userLastActiveMinTouch))
		return nil, nil
	})
	if err != nil {
		slog.Warn("touch user last active failed", "user_id", user.ID, "error", err)
	}
}

func userLastActiveFresh(lastActiveAt *time.Time, now time.Time) bool {
	if lastActiveAt == nil {
		return false
	}
	return now.Before(lastActiveAt.Add(userLastActiveMinTouch))
}

func (s *UserService) hydrateUserAvatar(ctx context.Context, user *User) error {
	if s == nil || s.userRepo == nil || user == nil || user.ID == 0 {
		return nil
	}

	avatar, err := s.userRepo.GetUserAvatar(ctx, user.ID)
	if err != nil {
		return err
	}
	applyUserAvatar(user, avatar)
	return nil
}

// List 获取用户列表（管理员功能）
func (s *UserService) List(ctx context.Context, params pagination.PaginationParams) ([]User, *pagination.PaginationResult, error) {
	users, pagination, err := s.userRepo.List(ctx, params)
	if err != nil {
		return nil, nil, fmt.Errorf("list users: %w", err)
	}
	return users, pagination, nil
}

// UpdateBalance 更新用户余额（管理员功能）
func (s *UserService) UpdateBalance(ctx context.Context, userID int64, amount float64) error {
	if err := s.userRepo.UpdateBalance(ctx, userID, amount); err != nil {
		return fmt.Errorf("update balance: %w", err)
	}
	if s.authCacheInvalidator != nil {
		s.authCacheInvalidator.InvalidateAuthCacheByUserID(ctx, userID)
	}
	if s.billingCache != nil {
		go func() {
			defer func() {
				if r := recover(); r != nil {
					slog.Error("panic in balance cache invalidation", "user_id", userID, "recover", r)
				}
			}()
			cacheCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if err := s.billingCache.InvalidateUserBalance(cacheCtx, userID); err != nil {
				slog.Error("invalidate user balance cache failed", "user_id", userID, "error", err)
			}
		}()
	}
	return nil
}

// UpdateConcurrency 更新用户并发数（管理员功能）
func (s *UserService) UpdateConcurrency(ctx context.Context, userID int64, concurrency int) error {
	if err := s.userRepo.UpdateConcurrency(ctx, userID, concurrency); err != nil {
		return fmt.Errorf("update concurrency: %w", err)
	}
	if s.authCacheInvalidator != nil {
		s.authCacheInvalidator.InvalidateAuthCacheByUserID(ctx, userID)
	}
	return nil
}

// UpdateStatus 更新用户状态（管理员功能）
func (s *UserService) UpdateStatus(ctx context.Context, userID int64, status string) error {
	user, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		return fmt.Errorf("get user: %w", err)
	}

	user.Status = status

	if err := s.userRepo.Update(ctx, user); err != nil {
		return fmt.Errorf("update user: %w", err)
	}
	if s.authCacheInvalidator != nil {
		s.authCacheInvalidator.InvalidateAuthCacheByUserID(ctx, userID)
	}

	return nil
}

// Delete 删除用户（管理员功能）
func (s *UserService) Delete(ctx context.Context, userID int64) error {
	if s.authCacheInvalidator != nil {
		s.authCacheInvalidator.InvalidateAuthCacheByUserID(ctx, userID)
	}
	if err := s.userRepo.Delete(ctx, userID); err != nil {
		return fmt.Errorf("delete user: %w", err)
	}
	return nil
}

// SendNotifyEmailCode sends a verification code to the extra notification email.
func (s *UserService) SendNotifyEmailCode(ctx context.Context, userID int64, email string, emailService *EmailService, cache EmailCache) error {
	if err := checkNotifyCodeRateLimit(ctx, cache, userID, email); err != nil {
		return err
	}

	code, err := emailService.GenerateVerifyCode()
	if err != nil {
		return fmt.Errorf("generate code: %w", err)
	}

	// Send email first — if SMTP fails, don't write cache or increment counters,
	// so the user is not locked out by cooldown/rate-limit for a code they never received.
	if err := s.sendNotifyVerifyEmail(ctx, emailService, email, code); err != nil {
		return err
	}

	if err := saveNotifyVerifyCode(ctx, cache, email, code); err != nil {
		return err
	}

	// Increment user-level counter after successful save
	if _, err := cache.IncrNotifyCodeUserRate(ctx, userID, notifyCodeUserRateWindow); err != nil {
		slog.Error("failed to increment notify code user rate", "user_id", userID, "error", err)
	}

	return nil
}

// checkNotifyCodeRateLimit checks both email cooldown and user-level rate limit.
func checkNotifyCodeRateLimit(ctx context.Context, cache EmailCache, userID int64, email string) error {
	existing, err := cache.GetNotifyVerifyCode(ctx, email)
	if err == nil && existing != nil {
		if time.Since(existing.CreatedAt) < verifyCodeCooldown {
			return ErrVerifyCodeTooFrequent
		}
	}
	count, err := cache.GetNotifyCodeUserRate(ctx, userID)
	if err == nil && count >= notifyCodeUserRateLimit {
		return ErrNotifyCodeUserRateLimit
	}
	return nil
}

// saveNotifyVerifyCode saves the verification code to cache.
func saveNotifyVerifyCode(ctx context.Context, cache EmailCache, email, code string) error {
	data := &VerificationCodeData{
		Code:      code,
		Attempts:  0,
		CreatedAt: time.Now(),
		ExpiresAt: time.Now().Add(verifyCodeTTL),
	}
	if err := cache.SetNotifyVerifyCode(ctx, email, data, verifyCodeTTL); err != nil {
		return fmt.Errorf("save verify code: %w", err)
	}
	return nil
}

// sendNotifyVerifyEmail builds and sends the verification email.
func (s *UserService) sendNotifyVerifyEmail(ctx context.Context, emailService *EmailService, email, code string) error {
	siteName := "Sub2API"
	if s.settingRepo != nil {
		if name, err := s.settingRepo.GetValue(ctx, SettingKeySiteName); err == nil && name != "" {
			siteName = name
		}
	}
	subject := fmt.Sprintf("[%s] 通知邮箱验证码 / Notification Email Verification", siteName)
	body := buildNotifyVerifyEmailBody(code, siteName)
	return emailService.SendEmail(ctx, email, subject, body)
}

// VerifyAndAddNotifyEmail verifies the code and adds the email to user's extra emails.
func (s *UserService) VerifyAndAddNotifyEmail(ctx context.Context, userID int64, email, code string, cache EmailCache) error {
	if err := verifyNotifyCode(ctx, cache, email, code); err != nil {
		return err
	}
	_ = cache.DeleteNotifyVerifyCode(ctx, email)
	return s.addOrVerifyNotifyEmail(ctx, userID, email)
}

// verifyNotifyCode validates the verification code against the cached data.
func verifyNotifyCode(ctx context.Context, cache EmailCache, email, code string) error {
	data, err := cache.GetNotifyVerifyCode(ctx, email)
	if err != nil || data == nil {
		return ErrInvalidVerifyCode
	}
	if data.Attempts >= maxVerifyCodeAttempts {
		return ErrVerifyCodeMaxAttempts
	}
	if subtle.ConstantTimeCompare([]byte(data.Code), []byte(code)) != 1 {
		data.Attempts++
		remaining := time.Until(data.ExpiresAt)
		if remaining <= 0 {
			return ErrInvalidVerifyCode
		}
		if err := cache.SetNotifyVerifyCode(ctx, email, data, remaining); err != nil {
			slog.Error("failed to update notify verify code attempts", "email", email, "error", err)
		}
		if data.Attempts >= maxVerifyCodeAttempts {
			return ErrVerifyCodeMaxAttempts
		}
		return ErrInvalidVerifyCode
	}
	return nil
}

// addOrVerifyNotifyEmail adds the email to user's extra notification emails or marks it as verified.
// Note: concurrent calls for the same user could race on the read-modify-write of
// BalanceNotifyExtraEmails. The window is small (requires two verify flows completing
// simultaneously), and the worst case is a duplicate entry which is harmless.
func (s *UserService) addOrVerifyNotifyEmail(ctx context.Context, userID int64, email string) error {
	user, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		return err
	}
	for i, e := range user.BalanceNotifyExtraEmails {
		if strings.EqualFold(e.Email, email) {
			if !e.Verified {
				user.BalanceNotifyExtraEmails[i].Verified = true
				return s.userRepo.Update(ctx, user)
			}
			return nil // Already verified
		}
	}
	if len(user.BalanceNotifyExtraEmails) >= maxNotifyEmails {
		return infraerrors.BadRequest("TOO_MANY_NOTIFY_EMAILS", fmt.Sprintf("maximum %d notification emails allowed", maxNotifyEmails))
	}
	user.BalanceNotifyExtraEmails = append(user.BalanceNotifyExtraEmails, NotifyEmailEntry{
		Email:    email,
		Disabled: false,
		Verified: true,
	})
	return s.userRepo.Update(ctx, user)
}

// RemoveNotifyEmail removes an email from user's extra notification emails.
func (s *UserService) RemoveNotifyEmail(ctx context.Context, userID int64, email string) error {
	user, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		return err
	}

	filtered := make([]NotifyEmailEntry, 0, len(user.BalanceNotifyExtraEmails))
	found := false
	for _, e := range user.BalanceNotifyExtraEmails {
		if strings.EqualFold(e.Email, email) {
			found = true
		} else {
			filtered = append(filtered, e)
		}
	}
	if !found {
		return infraerrors.BadRequest("EMAIL_NOT_FOUND", "notification email not found")
	}
	user.BalanceNotifyExtraEmails = filtered
	return s.userRepo.Update(ctx, user)
}

// ToggleNotifyEmail toggles the disabled state of a notification email entry.
func (s *UserService) ToggleNotifyEmail(ctx context.Context, userID int64, email string, disabled bool) error {
	user, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		return err
	}

	found := false
	for i, e := range user.BalanceNotifyExtraEmails {
		if strings.EqualFold(e.Email, email) {
			user.BalanceNotifyExtraEmails[i].Disabled = disabled
			found = true
			break
		}
	}
	if !found {
		return infraerrors.BadRequest("EMAIL_NOT_FOUND", "notification email not found")
	}

	return s.userRepo.Update(ctx, user)
}

// notifyVerifyEmailTemplate is the HTML template for notify email verification.
// Format args: siteName, code.
const notifyVerifyEmailTemplate = `<!DOCTYPE html>
<html>
<head>
    <meta charset="UTF-8">
    <style>
        body { font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, Oxygen, Ubuntu, sans-serif; background-color: #f5f5f5; margin: 0; padding: 20px; }
        .container { max-width: 600px; margin: 0 auto; background-color: #ffffff; border-radius: 8px; overflow: hidden; box-shadow: 0 2px 8px rgba(0,0,0,0.1); }
        .header { background: linear-gradient(135deg, #667eea 0%%, #764ba2 100%%); color: white; padding: 30px; text-align: center; }
        .header h1 { margin: 0; font-size: 24px; }
        .content { padding: 40px 30px; text-align: center; }
        .code { font-size: 36px; font-weight: bold; letter-spacing: 8px; color: #333; background-color: #f8f9fa; padding: 20px 30px; border-radius: 8px; display: inline-block; margin: 20px 0; font-family: monospace; }
        .info { color: #666; font-size: 14px; line-height: 1.6; margin-top: 20px; }
        .footer { background-color: #f8f9fa; padding: 20px; text-align: center; color: #999; font-size: 12px; }
    </style>
</head>
<body>
    <div class="container">
        <div class="header">
            <h1>%s</h1>
        </div>
        <div class="content">
            <p style="font-size: 18px; color: #333;">通知邮箱验证码 / Notification Email Verification</p>
            <div class="code">%s</div>
            <div class="info">
                <p>您正在添加额外的通知邮箱，请输入此验证码完成验证。</p>
                <p>You are adding an extra notification email. Please enter this code to verify.</p>
                <p>此验证码将在 <strong>15 分钟</strong>后失效。</p>
                <p>This code will expire in <strong>15 minutes</strong>.</p>
                <p>如果您没有请求此验证码，请忽略此邮件。</p>
                <p>If you did not request this code, please ignore this email.</p>
            </div>
        </div>
        <div class="footer">
            <p>此邮件由系统自动发送，请勿回复。/ This is an automated message, please do not reply.</p>
        </div>
    </div>
</body>
</html>`

// buildNotifyVerifyEmailBody builds the HTML email body for notify email verification.
func buildNotifyVerifyEmailBody(code, siteName string) string {
	return fmt.Sprintf(notifyVerifyEmailTemplate, siteName, code)
}
