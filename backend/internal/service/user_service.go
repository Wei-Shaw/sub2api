package service

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"log/slog"
	"net/url"
	"sort"
	"strings"
	"time"
)

var (
	ErrUserNotFound            = infraerrors.NotFound("USER_NOT_FOUND", "user not found")
	ErrPasswordIncorrect       = infraerrors.BadRequest("PASSWORD_INCORRECT", "current password is incorrect")
	ErrInsufficientPerms       = infraerrors.Forbidden("INSUFFICIENT_PERMISSIONS", "insufficient permissions")
	ErrNotifyCodeUserRateLimit = infraerrors.TooManyRequests("NOTIFY_CODE_USER_RATE_LIMIT", "too many verification codes requested, please try again later")
	ErrAvatarInvalid           = infraerrors.BadRequest("AVATAR_INVALID", "avatar must be a valid image data URL or http(s) URL")
	ErrAvatarTooLarge          = infraerrors.BadRequest("AVATAR_TOO_LARGE", "avatar image must be 100KB or smaller")
	ErrAvatarNotImage          = infraerrors.BadRequest("AVATAR_NOT_IMAGE", "avatar content must be an image")
	ErrIdentityProviderInvalid = infraerrors.BadRequest("IDENTITY_PROVIDER_INVALID", "identity provider is invalid")
	ErrIdentityRedirectInvalid = infraerrors.BadRequest("IDENTITY_REDIRECT_INVALID", "identity redirect path is invalid")
)

const (
	maxNotifyEmails      = 3 // Maximum number of notification emails per user
	maxInlineAvatarBytes = 100 * 1024

	// User-level rate limiting for notify email verification codes
	notifyCodeUserRateLimit  = 5
	notifyCodeUserRateWindow = 10 * time.Minute

	defaultUserIdentityRedirect = "/settings/profile"
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
REDACTED

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

	// TOTP 双因素认证
	UpdateTotpSecret(ctx context.Context, userID int64, encryptedSecret *string) error
	EnableTotp(ctx context.Context, userID int64) error
	DisableTotp(ctx context.Context, userID int64) error
REDACTED

type UserAuthIdentityRecord struct {
	ProviderType    string
	ProviderKey     string
	ProviderSubject string
	VerifiedAt      *time.Time
	Issuer          *string
	Metadata        map[string]any
	CreatedAt       time.Time
	UpdatedAt       time.Time
REDACTED

type UserIdentitySummary struct {
	Provider      string     `json:"provider"`
	Bound         bool       `json:"bound"`
	BoundCount    int        `json:"bound_count"`
	DisplayName   string     `json:"display_name,omitempty"`
	SubjectHint   string     `json:"subject_hint,omitempty"`
	ProviderKey   string     `json:"provider_key,omitempty"`
	VerifiedAt    *time.Time `json:"verified_at,omitempty"`
	BindStartPath string     `json:"bind_start_path,omitempty"`
	CanBind       bool       `json:"can_bind"`
	CanUnbind     bool       `json:"can_unbind"`
	Note          string     `json:"note,omitempty"`
REDACTED

type UserIdentitySummarySet struct {
	Email   UserIdentitySummary `json:"email"`
	LinuxDo UserIdentitySummary `json:"linuxdo"`
	OIDC    UserIdentitySummary `json:"oidc"`
	WeChat  UserIdentitySummary `json:"wechat"`
REDACTED

type StartUserIdentityBindingRequest struct {
	Provider   string
	RedirectTo string
REDACTED

type StartUserIdentityBindingResult struct {
	Provider           string `json:"provider"`
	AuthorizeURL       string `json:"authorize_url"`
	Method             string `json:"method"`
	UseBrowserRedirect bool   `json:"use_browser_redirect"`
REDACTED

// UpdateProfileRequest 更新用户资料请求
type UpdateProfileRequest struct {
	Email                  *string  `json:"email"`
	Username               *string  `json:"username"`
	AvatarURL              *string  `json:"avatar_url"`
	Concurrency            *int     `json:"concurrency"`
	BalanceNotifyEnabled   *bool    `json:"balance_notify_enabled"`
	BalanceNotifyThreshold *float64 `json:"balance_notify_threshold"`
REDACTED

type UserAvatar struct {
	StorageProvider string
	StorageKey      string
	URL             string
	ContentType     string
	ByteSize        int
	SHA256          string
REDACTED

type UpsertUserAvatarInput struct {
	StorageProvider string
	StorageKey      string
	URL             string
	ContentType     string
	ByteSize        int
	SHA256          string
REDACTED

type userAuthIdentityReader interface {
	ListUserAuthIdentities(ctx context.Context, userID int64) ([]UserAuthIdentityRecord, error)
REDACTED

type emailAuthIdentitySynchronizer interface {
	EnsureEmailAuthIdentity(ctx context.Context, userID int64, email string) error
	ReplaceEmailAuthIdentity(ctx context.Context, userID int64, oldEmail, newEmail string) error
REDACTED

func ensureEmailAuthIdentitySync(ctx context.Context, repo UserRepository, userID int64, email string) error {
	syncer, ok := repo.(emailAuthIdentitySynchronizer)
	if !ok {
		return nil
REDACTED
	return syncer.EnsureEmailAuthIdentity(ctx, userID, email)
REDACTED

func replaceEmailAuthIdentitySync(ctx context.Context, repo UserRepository, userID int64, oldEmail, newEmail string) error {
	oldNormalized := strings.ToLower(strings.TrimSpace(oldEmail))
	newNormalized := strings.ToLower(strings.TrimSpace(newEmail))
	if oldNormalized == newNormalized {
		return nil
REDACTED

	syncer, ok := repo.(emailAuthIdentitySynchronizer)
	if !ok {
		return nil
REDACTED
	return syncer.ReplaceEmailAuthIdentity(ctx, userID, oldEmail, newEmail)
REDACTED

// ChangePasswordRequest 修改密码请求
type ChangePasswordRequest struct {
	CurrentPassword string `json:"current_password"`
	NewPassword     string `json:"new_password"`
REDACTED

// UserService 用户服务
type UserService struct {
	userRepo             UserRepository
	settingRepo          SettingRepository
	authCacheInvalidator APIKeyAuthCacheInvalidator
	billingCache         BillingCache
REDACTED

// NewUserService 创建用户服务实例
func NewUserService(userRepo UserRepository, settingRepo SettingRepository, authCacheInvalidator APIKeyAuthCacheInvalidator, billingCache BillingCache) *UserService {
	return &UserService{
		userRepo:             userRepo,
		settingRepo:          settingRepo,
		authCacheInvalidator: authCacheInvalidator,
		billingCache:         billingCache,
REDACTED
REDACTED

// GetFirstAdmin 获取首个管理员用户（用于 Admin API Key 认证）
func (s *UserService) GetFirstAdmin(ctx context.Context) (*User, error) {
	admin, err := s.userRepo.GetFirstAdmin(ctx)
	if err != nil {
		return nil, fmt.Errorf("get first admin: %w", err)
REDACTED
	return admin, nil
REDACTED

// GetProfile 获取用户资料
func (s *UserService) GetProfile(ctx context.Context, userID int64) (*User, error) {
	user, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("get user: %w", err)
REDACTED
	if err := s.hydrateUserAvatar(ctx, user); err != nil {
		return nil, fmt.Errorf("get user avatar: %w", err)
REDACTED
	return user, nil
REDACTED

func (s *UserService) GetProfileIdentitySummaries(ctx context.Context, userID int64, user *User) (UserIdentitySummarySet, error) {
	if user == nil {
		var err error
		user, err = s.userRepo.GetByID(ctx, userID)
		if err != nil {
			return UserIdentitySummarySet{REDACTED, fmt.Errorf("get user: %w", err)
	REDACTED
REDACTED

	records, err := s.listUserAuthIdentities(ctx, userID)
	if err != nil {
		return UserIdentitySummarySet{REDACTED, err
REDACTED

	return UserIdentitySummarySet{
		Email:   s.buildEmailIdentitySummary(user),
		LinuxDo: s.buildProviderIdentitySummary("linuxdo", records),
		OIDC:    s.buildProviderIdentitySummary("oidc", records),
		WeChat:  s.buildProviderIdentitySummary("wechat", records),
REDACTED, nil
REDACTED

func (s *UserService) PrepareIdentityBindingStart(_ context.Context, req StartUserIdentityBindingRequest) (*StartUserIdentityBindingResult, error) {
	provider := normalizeUserIdentityProvider(req.Provider)
	if provider == "" {
		return nil, ErrIdentityProviderInvalid
REDACTED

	authorizeURL, err := buildUserIdentityBindAuthorizeURL(provider, req.RedirectTo)
	if err != nil {
		return nil, err
REDACTED

	return &StartUserIdentityBindingResult{
		Provider:           provider,
		AuthorizeURL:       authorizeURL,
		Method:             "GET",
		UseBrowserRedirect: true,
REDACTED, nil
REDACTED

// UpdateProfile 更新用户资料
func (s *UserService) UpdateProfile(ctx context.Context, userID int64, req UpdateProfileRequest) (*User, error) {
	user, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("get user: %w", err)
REDACTED
	oldConcurrency := user.Concurrency
	oldEmail := user.Email

	// 更新字段
	if req.Email != nil {
		// 检查新邮箱是否已被使用
		exists, err := s.userRepo.ExistsByEmail(ctx, *req.Email)
		if err != nil {
			return nil, fmt.Errorf("check email exists: %w", err)
	REDACTED
		if exists && *req.Email != user.Email {
			return nil, ErrEmailExists
	REDACTED
		user.Email = *req.Email
REDACTED

	if req.Username != nil {
		user.Username = *req.Username
REDACTED

	if req.AvatarURL != nil {
		avatar, err := s.SetAvatar(ctx, userID, *req.AvatarURL)
		if err != nil {
			return nil, err
	REDACTED
		applyUserAvatar(user, avatar)
REDACTED

	if req.Concurrency != nil {
		user.Concurrency = *req.Concurrency
REDACTED

	if req.BalanceNotifyEnabled != nil {
		user.BalanceNotifyEnabled = *req.BalanceNotifyEnabled
REDACTED
	if req.BalanceNotifyThreshold != nil {
		if *req.BalanceNotifyThreshold <= 0 {
			user.BalanceNotifyThreshold = nil // clear to system default
	REDACTED else {
			user.BalanceNotifyThreshold = req.BalanceNotifyThreshold
	REDACTED
REDACTED

	if err := s.userRepo.Update(ctx, user); err != nil {
		return nil, fmt.Errorf("update user: %w", err)
REDACTED
	if err := replaceEmailAuthIdentitySync(ctx, s.userRepo, user.ID, oldEmail, user.Email); err != nil {
		return nil, fmt.Errorf("sync email auth identity: %w", err)
REDACTED
	if s.authCacheInvalidator != nil && user.Concurrency != oldConcurrency {
		s.authCacheInvalidator.InvalidateAuthCacheByUserID(ctx, userID)
REDACTED

	return user, nil
REDACTED

func (s *UserService) SetAvatar(ctx context.Context, userID int64, raw string) (*UserAvatar, error) {
	avatarValue := strings.TrimSpace(raw)
	if avatarValue == "" {
		if err := s.userRepo.DeleteUserAvatar(ctx, userID); err != nil {
			return nil, fmt.Errorf("delete avatar: %w", err)
	REDACTED
		return nil, nil
REDACTED

	avatarInput, err := normalizeUserAvatarInput(avatarValue)
	if err != nil {
		return nil, err
REDACTED

	avatar, err := s.userRepo.UpsertUserAvatar(ctx, userID, avatarInput)
	if err != nil {
		return nil, fmt.Errorf("upsert avatar: %w", err)
REDACTED
	return avatar, nil
REDACTED

func applyUserAvatar(user *User, avatar *UserAvatar) {
	if user == nil {
		return
REDACTED
	if avatar == nil {
		user.AvatarURL = ""
		user.AvatarSource = ""
		user.AvatarMIME = ""
		user.AvatarByteSize = 0
		user.AvatarSHA256 = ""
		return
REDACTED

	user.AvatarURL = avatar.URL
	user.AvatarSource = avatar.StorageProvider
	user.AvatarMIME = avatar.ContentType
	user.AvatarByteSize = avatar.ByteSize
	user.AvatarSHA256 = avatar.SHA256
REDACTED

func normalizeUserAvatarInput(raw string) (UpsertUserAvatarInput, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return UpsertUserAvatarInput{REDACTED, ErrAvatarInvalid
REDACTED
	if strings.HasPrefix(raw, "data:") {
		return normalizeInlineUserAvatarInput(raw)
REDACTED

	parsed, err := url.Parse(raw)
	if err != nil || parsed == nil {
		return UpsertUserAvatarInput{REDACTED, ErrAvatarInvalid
REDACTED
	if !strings.EqualFold(parsed.Scheme, "http") && !strings.EqualFold(parsed.Scheme, "https") {
		return UpsertUserAvatarInput{REDACTED, ErrAvatarInvalid
REDACTED
	if strings.TrimSpace(parsed.Host) == "" {
		return UpsertUserAvatarInput{REDACTED, ErrAvatarInvalid
REDACTED

	return UpsertUserAvatarInput{
		StorageProvider: "remote_url",
		URL:             raw,
REDACTED, nil
REDACTED

func normalizeInlineUserAvatarInput(raw string) (UpsertUserAvatarInput, error) {
	body := strings.TrimPrefix(raw, "data:")
	meta, encoded, ok := strings.Cut(body, ",")
	if !ok {
		return UpsertUserAvatarInput{REDACTED, ErrAvatarInvalid
REDACTED
	meta = strings.TrimSpace(meta)
	encoded = strings.TrimSpace(encoded)
	if !strings.HasSuffix(strings.ToLower(meta), ";base64") {
		return UpsertUserAvatarInput{REDACTED, ErrAvatarInvalid
REDACTED

	contentType := strings.TrimSpace(meta[:len(meta)-len(";base64")])
	if contentType == "" || !strings.HasPrefix(strings.ToLower(contentType), "image/") {
		return UpsertUserAvatarInput{REDACTED, ErrAvatarNotImage
REDACTED

	decoded, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return UpsertUserAvatarInput{REDACTED, ErrAvatarInvalid
REDACTED
	if len(decoded) > maxInlineAvatarBytes {
		return UpsertUserAvatarInput{REDACTED, ErrAvatarTooLarge
REDACTED

	sum := sha256.Sum256(decoded)
	return UpsertUserAvatarInput{
		StorageProvider: "inline",
		URL:             raw,
		ContentType:     contentType,
		ByteSize:        len(decoded),
		SHA256:          hex.EncodeToString(sum[:]),
REDACTED, nil
REDACTED

func (s *UserService) buildEmailIdentitySummary(user *User) UserIdentitySummary {
	summary := UserIdentitySummary{
		Provider:  "email",
		CanBind:   false,
		CanUnbind: false,
		Note:      "Primary account email is managed from the profile form.",
REDACTED
	if user == nil {
		return summary
REDACTED

	email := strings.TrimSpace(user.Email)
	if email == "" || isReservedEmail(email) {
		return summary
REDACTED

	summary.Bound = true
	summary.BoundCount = 1
	summary.DisplayName = email
	summary.SubjectHint = maskEmailIdentity(email)
	summary.ProviderKey = "email"
	return summary
REDACTED

func (s *UserService) buildProviderIdentitySummary(provider string, records []UserAuthIdentityRecord) UserIdentitySummary {
	summary := UserIdentitySummary{
		Provider:  provider,
		CanUnbind: false,
REDACTED
	filtered := filterUserAuthIdentities(records, provider)
	if len(filtered) == 0 {
		summary.CanBind = true
		bindStartPath, err := buildUserIdentityBindAuthorizeURL(provider, "")
		if err == nil {
			summary.BindStartPath = bindStartPath
	REDACTED
		return summary
REDACTED

	primary := selectPrimaryUserAuthIdentity(filtered)
	summary.Bound = true
	summary.BoundCount = len(filtered)
	summary.DisplayName = userAuthIdentityDisplayName(primary)
	summary.SubjectHint = maskOpaqueIdentity(primary.ProviderSubject)
	summary.ProviderKey = strings.TrimSpace(primary.ProviderKey)
	summary.VerifiedAt = primary.VerifiedAt
	summary.Note = "Unbind is not available yet."
	return summary
REDACTED

func (s *UserService) listUserAuthIdentities(ctx context.Context, userID int64) ([]UserAuthIdentityRecord, error) {
	if userID <= 0 || s == nil || s.userRepo == nil {
		return nil, nil
REDACTED
	return s.userRepo.ListUserAuthIdentities(ctx, userID)
REDACTED

func buildUserIdentityBindAuthorizeURL(provider, redirectTo string) (string, error) {
	provider = normalizeUserIdentityProvider(provider)
	if provider == "" || provider == "email" {
		return "", ErrIdentityProviderInvalid
REDACTED

	redirectTo, err := normalizeUserIdentityRedirect(redirectTo)
	if err != nil {
		return "", err
REDACTED

	path := ""
	switch provider {
	case "linuxdo":
		path = "/api/v1/auth/oauth/linuxdo/start"
	case "oidc":
		path = "/api/v1/auth/oauth/oidc/start"
	case "wechat":
		path = "/api/v1/auth/oauth/wechat/start"
	default:
		return "", ErrIdentityProviderInvalid
REDACTED

	query := url.Values{REDACTED
	query.Set("redirect", redirectTo)
	query.Set("intent", "bind_current_user")
	return path + "?" + query.Encode(), nil
REDACTED

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
REDACTED
REDACTED

func normalizeUserIdentityRedirect(raw string) (string, error) {
	redirect := strings.TrimSpace(raw)
	if redirect == "" {
		return defaultUserIdentityRedirect, nil
REDACTED
	if len(redirect) > 2048 || !strings.HasPrefix(redirect, "/") || strings.HasPrefix(redirect, "//") {
		return "", ErrIdentityRedirectInvalid
REDACTED
	return redirect, nil
REDACTED

func filterUserAuthIdentities(records []UserAuthIdentityRecord, provider string) []UserAuthIdentityRecord {
	if len(records) == 0 {
		return nil
REDACTED
	filtered := make([]UserAuthIdentityRecord, 0, len(records))
	for _, record := range records {
		if strings.EqualFold(strings.TrimSpace(record.ProviderType), provider) {
			filtered = append(filtered, record)
	REDACTED
REDACTED
	return filtered
REDACTED

func selectPrimaryUserAuthIdentity(records []UserAuthIdentityRecord) UserAuthIdentityRecord {
	if len(records) == 0 {
		return UserAuthIdentityRecord{REDACTED
REDACTED
	sort.SliceStable(records, func(i, j int) bool {
		left := userAuthIdentitySortTime(records[i])
		right := userAuthIdentitySortTime(records[j])
		if !left.Equal(right) {
			return left.After(right)
	REDACTED
		return records[i].ProviderKey < records[j].ProviderKey
REDACTED)
	return records[0]
REDACTED

func userAuthIdentitySortTime(record UserAuthIdentityRecord) time.Time {
	if record.VerifiedAt != nil && !record.VerifiedAt.IsZero() {
		return record.VerifiedAt.UTC()
REDACTED
	if !record.UpdatedAt.IsZero() {
		return record.UpdatedAt.UTC()
REDACTED
	if !record.CreatedAt.IsZero() {
		return record.CreatedAt.UTC()
REDACTED
	return time.Time{REDACTED
REDACTED

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
REDACTED
	if subject := strings.TrimSpace(record.ProviderSubject); subject != "" {
		return subject
REDACTED
	return strings.TrimSpace(record.ProviderType)
REDACTED

func firstStringIdentityValue(values map[string]any, keys ...string) string {
	for _, key := range keys {
		raw, ok := values[key]
		if !ok {
			continue
	REDACTED
		switch value := raw.(type) {
		case string:
			if trimmed := strings.TrimSpace(value); trimmed != "" {
				return trimmed
		REDACTED
		case fmt.Stringer:
			if trimmed := strings.TrimSpace(value.String()); trimmed != "" {
				return trimmed
		REDACTED
	REDACTED
REDACTED
	return ""
REDACTED

func maskEmailIdentity(email string) string {
	local, domain, ok := strings.Cut(strings.TrimSpace(email), "@")
	if !ok || local == "" || domain == "" {
		return maskOpaqueIdentity(email)
REDACTED
	runes := []rune(local)
	if len(runes) == 1 {
		return string(runes[0]) + "***@" + domain
REDACTED
	return string(runes[0]) + "***" + string(runes[len(runes)-1]) + "@" + domain
REDACTED

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
REDACTED
REDACTED

func cloneAnyMap(values map[string]any) map[string]any {
	if len(values) == 0 {
		return map[string]any{REDACTED
REDACTED
	cloned := make(map[string]any, len(values))
	for key, value := range values {
		cloned[key] = value
REDACTED
	return cloned
REDACTED

// ChangePassword 修改密码
// Security: Increments TokenVersion to invalidate all existing JWT tokens
func (s *UserService) ChangePassword(ctx context.Context, userID int64, req ChangePasswordRequest) error {
	user, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		return fmt.Errorf("get user: %w", err)
REDACTED

	// 验证当前密码
	if !user.CheckPassword(req.CurrentPassword) {
		return ErrPasswordIncorrect
REDACTED

	if err := user.SetPassword(req.NewPassword); err != nil {
		return fmt.Errorf("set password: %w", err)
REDACTED

	// Increment TokenVersion to invalidate all existing tokens
	// This ensures that any tokens issued before the password change become invalid
	user.TokenVersion++

	if err := s.userRepo.Update(ctx, user); err != nil {
		return fmt.Errorf("update user: %w", err)
REDACTED

	return nil
REDACTED

// GetByID 根据ID获取用户（管理员功能）
func (s *UserService) GetByID(ctx context.Context, id int64) (*User, error) {
	user, err := s.userRepo.GetByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("get user: %w", err)
REDACTED
	if err := s.hydrateUserAvatar(ctx, user); err != nil {
		return nil, fmt.Errorf("get user avatar: %w", err)
REDACTED
	return user, nil
REDACTED

func (s *UserService) hydrateUserAvatar(ctx context.Context, user *User) error {
	if s == nil || s.userRepo == nil || user == nil || user.ID == 0 {
		return nil
REDACTED

	avatar, err := s.userRepo.GetUserAvatar(ctx, user.ID)
	if err != nil {
		return err
REDACTED
	applyUserAvatar(user, avatar)
	return nil
REDACTED

// List 获取用户列表（管理员功能）
func (s *UserService) List(ctx context.Context, params pagination.PaginationParams) ([]User, *pagination.PaginationResult, error) {
	users, pagination, err := s.userRepo.List(ctx, params)
	if err != nil {
		return nil, nil, fmt.Errorf("list users: %w", err)
REDACTED
	return users, pagination, nil
REDACTED

// UpdateBalance 更新用户余额（管理员功能）
func (s *UserService) UpdateBalance(ctx context.Context, userID int64, amount float64) error {
	if err := s.userRepo.UpdateBalance(ctx, userID, amount); err != nil {
		return fmt.Errorf("update balance: %w", err)
REDACTED
	if s.authCacheInvalidator != nil {
		s.authCacheInvalidator.InvalidateAuthCacheByUserID(ctx, userID)
REDACTED
	if s.billingCache != nil {
		go func() {
			defer func() {
				if r := recover(); r != nil {
					slog.Error("panic in balance cache invalidation", "user_id", userID, "recover", r)
			REDACTED
		REDACTED()
			cacheCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if err := s.billingCache.InvalidateUserBalance(cacheCtx, userID); err != nil {
				slog.Error("invalidate user balance cache failed", "user_id", userID, "error", err)
		REDACTED
	REDACTED()
REDACTED
	return nil
REDACTED

// UpdateConcurrency 更新用户并发数（管理员功能）
func (s *UserService) UpdateConcurrency(ctx context.Context, userID int64, concurrency int) error {
	if err := s.userRepo.UpdateConcurrency(ctx, userID, concurrency); err != nil {
		return fmt.Errorf("update concurrency: %w", err)
REDACTED
	if s.authCacheInvalidator != nil {
		s.authCacheInvalidator.InvalidateAuthCacheByUserID(ctx, userID)
REDACTED
	return nil
REDACTED

// UpdateStatus 更新用户状态（管理员功能）
func (s *UserService) UpdateStatus(ctx context.Context, userID int64, status string) error {
	user, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		return fmt.Errorf("get user: %w", err)
REDACTED

	user.Status = status

	if err := s.userRepo.Update(ctx, user); err != nil {
		return fmt.Errorf("update user: %w", err)
REDACTED
	if s.authCacheInvalidator != nil {
		s.authCacheInvalidator.InvalidateAuthCacheByUserID(ctx, userID)
REDACTED

	return nil
REDACTED

// Delete 删除用户（管理员功能）
func (s *UserService) Delete(ctx context.Context, userID int64) error {
	if s.authCacheInvalidator != nil {
		s.authCacheInvalidator.InvalidateAuthCacheByUserID(ctx, userID)
REDACTED
	if err := s.userRepo.Delete(ctx, userID); err != nil {
		return fmt.Errorf("delete user: %w", err)
REDACTED
	return nil
REDACTED

// SendNotifyEmailCode sends a verification code to the extra notification email.
func (s *UserService) SendNotifyEmailCode(ctx context.Context, userID int64, email string, emailService *EmailService, cache EmailCache) error {
	if err := checkNotifyCodeRateLimit(ctx, cache, userID, email); err != nil {
		return err
REDACTED

	code, err := emailService.GenerateVerifyCode()
	if err != nil {
		return fmt.Errorf("generate code: %w", err)
REDACTED

	// Send email first — if SMTP fails, don't write cache or increment counters,
	// so the user is not locked out by cooldown/rate-limit for a code they never received.
	if err := s.sendNotifyVerifyEmail(ctx, emailService, email, code); err != nil {
		return err
REDACTED

	if err := saveNotifyVerifyCode(ctx, cache, email, code); err != nil {
		return err
REDACTED

	// Increment user-level counter after successful save
	if _, err := cache.IncrNotifyCodeUserRate(ctx, userID, notifyCodeUserRateWindow); err != nil {
		slog.Error("failed to increment notify code user rate", "user_id", userID, "error", err)
REDACTED

	return nil
REDACTED

// checkNotifyCodeRateLimit checks both email cooldown and user-level rate limit.
func checkNotifyCodeRateLimit(ctx context.Context, cache EmailCache, userID int64, email string) error {
	existing, err := cache.GetNotifyVerifyCode(ctx, email)
	if err == nil && existing != nil {
		if time.Since(existing.CreatedAt) < verifyCodeCooldown {
			return ErrVerifyCodeTooFrequent
	REDACTED
REDACTED
	count, err := cache.GetNotifyCodeUserRate(ctx, userID)
	if err == nil && count >= notifyCodeUserRateLimit {
		return ErrNotifyCodeUserRateLimit
REDACTED
	return nil
REDACTED

// saveNotifyVerifyCode saves the verification code to cache.
func saveNotifyVerifyCode(ctx context.Context, cache EmailCache, email, code string) error {
	data := &VerificationCodeData{
		Code:      code,
		Attempts:  0,
		CreatedAt: time.Now(),
		ExpiresAt: time.Now().Add(verifyCodeTTL),
REDACTED
	if err := cache.SetNotifyVerifyCode(ctx, email, data, verifyCodeTTL); err != nil {
		return fmt.Errorf("save verify code: %w", err)
REDACTED
	return nil
REDACTED

// sendNotifyVerifyEmail builds and sends the verification email.
func (s *UserService) sendNotifyVerifyEmail(ctx context.Context, emailService *EmailService, email, code string) error {
	siteName := "Sub2API"
	if s.settingRepo != nil {
		if name, err := s.settingRepo.GetValue(ctx, SettingKeySiteName); err == nil && name != "" {
			siteName = name
	REDACTED
REDACTED
	subject := fmt.Sprintf("[%s] 通知邮箱验证码 / Notification Email Verification", siteName)
	body := buildNotifyVerifyEmailBody(code, siteName)
	return emailService.SendEmail(ctx, email, subject, body)
REDACTED

// VerifyAndAddNotifyEmail verifies the code and adds the email to user's extra emails.
func (s *UserService) VerifyAndAddNotifyEmail(ctx context.Context, userID int64, email, code string, cache EmailCache) error {
	if err := verifyNotifyCode(ctx, cache, email, code); err != nil {
		return err
REDACTED
	_ = cache.DeleteNotifyVerifyCode(ctx, email)
	return s.addOrVerifyNotifyEmail(ctx, userID, email)
REDACTED

// verifyNotifyCode validates the verification code against the cached data.
func verifyNotifyCode(ctx context.Context, cache EmailCache, email, code string) error {
	data, err := cache.GetNotifyVerifyCode(ctx, email)
	if err != nil || data == nil {
		return ErrInvalidVerifyCode
REDACTED
	if data.Attempts >= maxVerifyCodeAttempts {
		return ErrVerifyCodeMaxAttempts
REDACTED
	if subtle.ConstantTimeCompare([]byte(data.Code), []byte(code)) != 1 {
		data.Attempts++
		remaining := time.Until(data.ExpiresAt)
		if remaining <= 0 {
			return ErrInvalidVerifyCode
	REDACTED
		if err := cache.SetNotifyVerifyCode(ctx, email, data, remaining); err != nil {
			slog.Error("failed to update notify verify code attempts", "email", email, "error", err)
	REDACTED
		if data.Attempts >= maxVerifyCodeAttempts {
			return ErrVerifyCodeMaxAttempts
	REDACTED
		return ErrInvalidVerifyCode
REDACTED
	return nil
REDACTED

// addOrVerifyNotifyEmail adds the email to user's extra notification emails or marks it as verified.
// Note: concurrent calls for the same user could race on the read-modify-write of
// BalanceNotifyExtraEmails. The window is small (requires two verify flows completing
// simultaneously), and the worst case is a duplicate entry which is harmless.
func (s *UserService) addOrVerifyNotifyEmail(ctx context.Context, userID int64, email string) error {
	user, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		return err
REDACTED
	for i, e := range user.BalanceNotifyExtraEmails {
		if strings.EqualFold(e.Email, email) {
			if !e.Verified {
				user.BalanceNotifyExtraEmails[i].Verified = true
				return s.userRepo.Update(ctx, user)
		REDACTED
			return nil // Already verified
	REDACTED
REDACTED
	if len(user.BalanceNotifyExtraEmails) >= maxNotifyEmails {
		return infraerrors.BadRequest("TOO_MANY_NOTIFY_EMAILS", fmt.Sprintf("maximum %d notification emails allowed", maxNotifyEmails))
REDACTED
	user.BalanceNotifyExtraEmails = append(user.BalanceNotifyExtraEmails, NotifyEmailEntry{
		Email:    email,
		Disabled: false,
		Verified: true,
REDACTED)
	return s.userRepo.Update(ctx, user)
REDACTED

// RemoveNotifyEmail removes an email from user's extra notification emails.
func (s *UserService) RemoveNotifyEmail(ctx context.Context, userID int64, email string) error {
	user, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		return err
REDACTED

	filtered := make([]NotifyEmailEntry, 0, len(user.BalanceNotifyExtraEmails))
	found := false
	for _, e := range user.BalanceNotifyExtraEmails {
		if strings.EqualFold(e.Email, email) {
			found = true
	REDACTED else {
			filtered = append(filtered, e)
	REDACTED
REDACTED
	if !found {
		return infraerrors.BadRequest("EMAIL_NOT_FOUND", "notification email not found")
REDACTED
	user.BalanceNotifyExtraEmails = filtered
	return s.userRepo.Update(ctx, user)
REDACTED

// ToggleNotifyEmail toggles the disabled state of a notification email entry.
func (s *UserService) ToggleNotifyEmail(ctx context.Context, userID int64, email string, disabled bool) error {
	user, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		return err
REDACTED

	found := false
	for i, e := range user.BalanceNotifyExtraEmails {
		if strings.EqualFold(e.Email, email) {
			user.BalanceNotifyExtraEmails[i].Disabled = disabled
			found = true
			break
	REDACTED
REDACTED
	if !found {
		return infraerrors.BadRequest("EMAIL_NOT_FOUND", "notification email not found")
REDACTED

	return s.userRepo.Update(ctx, user)
REDACTED

// notifyVerifyEmailTemplate is the HTML template for notify email verification.
// Format args: siteName, code.
const notifyVerifyEmailTemplate = `<!DOCTYPE html>
<html>
<head>
    <meta charset="UTF-8">
    <style>
        body { font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, Oxygen, Ubuntu, sans-serif; background-color: #f5f5f5; margin: 0; padding: 20px; REDACTED
        .container { max-width: 600px; margin: 0 auto; background-color: #ffffff; border-radius: 8px; overflow: hidden; box-shadow: 0 2px 8px rgba(0,0,0,0.1); REDACTED
        .header { background: linear-gradient(135deg, #667eea 0%%, #764ba2 100%%); color: white; padding: 30px; text-align: center; REDACTED
        .header h1 { margin: 0; font-size: 24px; REDACTED
        .content { padding: 40px 30px; text-align: center; REDACTED
        .code { font-size: 36px; font-weight: bold; letter-spacing: 8px; color: #333; background-color: #f8f9fa; padding: 20px 30px; border-radius: 8px; display: inline-block; margin: 20px 0; font-family: monospace; REDACTED
        .info { color: #666; font-size: 14px; line-height: 1.6; margin-top: 20px; REDACTED
        .footer { background-color: #f8f9fa; padding: 20px; text-align: center; color: #999; font-size: 12px; REDACTED
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
REDACTED
