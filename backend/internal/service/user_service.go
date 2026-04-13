package service

import (
	"context"
	"crypto/subtle"
	"fmt"
	"log/slog"
	"strings"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
)

var (
	ErrUserNotFound           = infraerrors.NotFound("USER_NOT_FOUND", "user not found")
	ErrPasswordIncorrect      = infraerrors.BadRequest("PASSWORD_INCORRECT", "current password is incorrect")
	ErrInsufficientPerms      = infraerrors.Forbidden("INSUFFICIENT_PERMISSIONS", "insufficient permissions")
	ErrNotifyCodeUserRateLimit = infraerrors.TooManyRequests("NOTIFY_CODE_USER_RATE_LIMIT", "too many verification codes requested, please try again later")
)

const (
	maxNotifyEmails = 3 // Maximum number of notification emails per user

	// User-level rate limiting for notify email verification codes
	notifyCodeUserRateLimit  = 5
	notifyCodeUserRateWindow = 10 * time.Minute
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

	List(ctx context.Context, params pagination.PaginationParams) ([]User, *pagination.PaginationResult, error)
	ListWithFilters(ctx context.Context, params pagination.PaginationParams, filters UserListFilters) ([]User, *pagination.PaginationResult, error)

	UpdateBalance(ctx context.Context, id int64, amount float64) error
	DeductBalance(ctx context.Context, id int64, amount float64) error
	UpdateConcurrency(ctx context.Context, id int64, amount int) error
	ExistsByEmail(ctx context.Context, email string) (bool, error)
	RemoveGroupFromAllowedGroups(ctx context.Context, groupID int64) (int64, error)
	// AddGroupToAllowedGroups 将指定分组增量添加到用户的 allowed_groups（幂等，冲突忽略）
	AddGroupToAllowedGroups(ctx context.Context, userID int64, groupID int64) error
	// RemoveGroupFromUserAllowedGroups 移除单个用户的指定分组权限
	RemoveGroupFromUserAllowedGroups(ctx context.Context, userID int64, groupID int64) error

	// TOTP 双因素认证
	UpdateTotpSecret(ctx context.Context, userID int64, encryptedSecret *string) error
	EnableTotp(ctx context.Context, userID int64) error
	DisableTotp(ctx context.Context, userID int64) error
}

// UpdateProfileRequest 更新用户资料请求
type UpdateProfileRequest struct {
	Email                  *string  `json:"email"`
	Username               *string  `json:"username"`
	Concurrency            *int     `json:"concurrency"`
	BalanceNotifyEnabled   *bool    `json:"balance_notify_enabled"`
	BalanceNotifyThreshold *float64 `json:"balance_notify_threshold"`
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
	return user, nil
}

// UpdateProfile 更新用户资料
func (s *UserService) UpdateProfile(ctx context.Context, userID int64, req UpdateProfileRequest) (*User, error) {
	user, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("get user: %w", err)
	}
	oldConcurrency := user.Concurrency

	// 更新字段
	if req.Email != nil {
		// 检查新邮箱是否已被使用
		exists, err := s.userRepo.ExistsByEmail(ctx, *req.Email)
		if err != nil {
			return nil, fmt.Errorf("check email exists: %w", err)
		}
		if exists && *req.Email != user.Email {
			return nil, ErrEmailExists
		}
		user.Email = *req.Email
	}

	if req.Username != nil {
		user.Username = *req.Username
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
		return nil, fmt.Errorf("update user: %w", err)
	}
	if s.authCacheInvalidator != nil && user.Concurrency != oldConcurrency {
		s.authCacheInvalidator.InvalidateAuthCacheByUserID(ctx, userID)
	}

	return user, nil
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
	return user, nil
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

	if err := saveNotifyVerifyCode(ctx, cache, email, code); err != nil {
		return err
	}

	// Increment user-level counter after successful save
	if _, err := cache.IncrNotifyCodeUserRate(ctx, userID, notifyCodeUserRateWindow); err != nil {
		slog.Error("failed to increment notify code user rate", "user_id", userID, "error", err)
	}

	return s.sendNotifyVerifyEmail(ctx, emailService, email, code)
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
		if err := cache.SetNotifyVerifyCode(ctx, email, data, verifyCodeTTL); err != nil {
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
	for _, e := range user.BalanceNotifyExtraEmails {
		if !strings.EqualFold(e.Email, email) {
			filtered = append(filtered, e)
		}
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
