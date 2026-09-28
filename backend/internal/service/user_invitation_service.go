package service

import (
	"context"
	"fmt"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

// 用户自助邀请：在「邀请码注册」模式下，允许已注册用户自助生成一次性邀请码邀请新用户，
// 管理员通过设置控制开关、每人可邀请次数与邀请码有效期。
// 生成的邀请码复用 redeem_codes（type=invitation），注册流程无需任何改动即可校验与占用。

// 用户自助邀请设置项（需同时开启注册与邀请码注册才生效）。
const (
	SettingKeyUserInvitationEnabled          = "user_invitation_enabled"            // 是否允许已注册用户生成邀请码
	SettingKeyUserInvitationMaxCodesPerUser  = "user_invitation_max_codes_per_user" // 每人可邀请次数（0=不限）
	SettingKeyUserInvitationCodeValidityDays = "user_invitation_code_validity_days" // 用户邀请码有效期（天，0=永不过期）
)

const (
	// UserInvitationMaxCodesPerUserDefault 每个用户默认可生成的邀请码数量（0 = 不限）。
	UserInvitationMaxCodesPerUserDefault = 5
	// UserInvitationCodeValidityDaysDefault 用户邀请码默认有效期（天，0 = 永不过期）。
	UserInvitationCodeValidityDaysDefault = 7
	// UserInvitationCodeValidityDaysMax 用户邀请码有效期上限（天）。
	UserInvitationCodeValidityDaysMax = 3650
	// UserInvitationMaxCodesPerUserMax 每人邀请码数量上限。
	UserInvitationMaxCodesPerUserMax = 10000
	// userInvitationListLimit 邀请记录列表返回的最大条数。
	userInvitationListLimit = 100
)

var (
	ErrUserInvitationDisabled     = infraerrors.Forbidden("USER_INVITATION_DISABLED", "user invitation is not enabled")
	ErrUserInvitationLimitReached = infraerrors.Conflict("USER_INVITATION_LIMIT_REACHED", "invitation limit reached")
)

// UserInvitationRepository 用户自助邀请码的持久化接口（由 redeem code 仓储实现）。
type UserInvitationRepository interface {
	// CountActiveInvitationsByCreator 统计占用邀请名额的邀请码：已使用的码 + 未过期的未使用码。
	// 过期未使用的码会释放名额。
	CountActiveInvitationsByCreator(ctx context.Context, userID int64, now time.Time) (int, error)
	// ListInvitationsByCreator 按创建时间倒序返回用户生成的邀请码（附带使用者信息）。
	ListInvitationsByCreator(ctx context.Context, userID int64, limit int) ([]RedeemCode, error)
	// CreateInvitationForUser 在锁定生成者的事务内校验名额并创建邀请码；
	// maxCodes <= 0 表示不限；名额用尽时返回 ErrUserInvitationLimitReached。
	CreateInvitationForUser(ctx context.Context, code *RedeemCode, maxCodes int, now time.Time) error
}

// UserInvitationConfig 用户自助邀请的管理员配置。
type UserInvitationConfig struct {
	// Enabled 管理员开关本身。
	Enabled bool
	// Available 功能是否实际可用：需同时开启注册、邀请码注册与用户邀请。
	Available        bool
	MaxCodesPerUser  int
	CodeValidityDays int
}

// UserInvitationOverview 用户邀请页面所需的数据。
type UserInvitationOverview struct {
	Available        bool
	MaxCodesPerUser  int
	UsedQuota        int
	CodeValidityDays int
	Codes            []RedeemCode
}

// Remaining 剩余可生成数量；不限时返回 -1。
func (o *UserInvitationOverview) Remaining() int {
	if o.MaxCodesPerUser <= 0 {
		return -1
	}
	if o.UsedQuota >= o.MaxCodesPerUser {
		return 0
	}
	return o.MaxCodesPerUser - o.UsedQuota
}

// GetUserInvitationConfig 读取用户自助邀请配置。
func (s *SettingService) GetUserInvitationConfig(ctx context.Context) UserInvitationConfig {
	values, err := s.settingRepo.GetMultiple(ctx, []string{
		SettingKeyRegistrationEnabled,
		SettingKeyInvitationCodeEnabled,
		SettingKeyUserInvitationEnabled,
		SettingKeyUserInvitationMaxCodesPerUser,
		SettingKeyUserInvitationCodeValidityDays,
	})
	if err != nil {
		return UserInvitationConfig{
			MaxCodesPerUser:  UserInvitationMaxCodesPerUserDefault,
			CodeValidityDays: UserInvitationCodeValidityDaysDefault,
		}
	}
	return userInvitationConfigFromSettings(values)
}

func userInvitationConfigFromSettings(settings map[string]string) UserInvitationConfig {
	enabled := settings[SettingKeyUserInvitationEnabled] == "true"
	return UserInvitationConfig{
		Enabled: enabled,
		Available: enabled &&
			settings[SettingKeyRegistrationEnabled] == "true" &&
			settings[SettingKeyInvitationCodeEnabled] == "true",
		MaxCodesPerUser: parseBoundedNonNegativeInt(
			settings[SettingKeyUserInvitationMaxCodesPerUser],
			UserInvitationMaxCodesPerUserDefault,
			UserInvitationMaxCodesPerUserMax,
		),
		CodeValidityDays: parseBoundedNonNegativeInt(
			settings[SettingKeyUserInvitationCodeValidityDays],
			UserInvitationCodeValidityDaysDefault,
			UserInvitationCodeValidityDaysMax,
		),
	}
}

// clampUserInvitationSetting 将用户邀请的整数设置截断到 [0, max]。
func clampUserInvitationSetting(value, max int) int {
	if value < 0 {
		return 0
	}
	if value > max {
		return max
	}
	return value
}

// UserInvitationService 用户自助邀请服务。
type UserInvitationService struct {
	repo           UserInvitationRepository
	settingService *SettingService
	now            func() time.Time
}

// NewUserInvitationService 创建用户自助邀请服务。
func NewUserInvitationService(repo UserInvitationRepository, settingService *SettingService) *UserInvitationService {
	return &UserInvitationService{
		repo:           repo,
		settingService: settingService,
		now:            time.Now,
	}
}

// GetOverview 返回用户的邀请名额与邀请记录。功能关闭时仍返回历史记录，但 Available=false。
func (s *UserInvitationService) GetOverview(ctx context.Context, userID int64) (*UserInvitationOverview, error) {
	cfg := s.settingService.GetUserInvitationConfig(ctx)
	now := s.now()

	used, err := s.repo.CountActiveInvitationsByCreator(ctx, userID, now)
	if err != nil {
		return nil, fmt.Errorf("count user invitations: %w", err)
	}
	codes, err := s.repo.ListInvitationsByCreator(ctx, userID, userInvitationListLimit)
	if err != nil {
		return nil, fmt.Errorf("list user invitations: %w", err)
	}

	return &UserInvitationOverview{
		Available:        cfg.Available,
		MaxCodesPerUser:  cfg.MaxCodesPerUser,
		UsedQuota:        used,
		CodeValidityDays: cfg.CodeValidityDays,
		Codes:            codes,
	}, nil
}

// CreateInvitation 为用户生成一个一次性邀请码。
func (s *UserInvitationService) CreateInvitation(ctx context.Context, userID int64) (*RedeemCode, error) {
	cfg := s.settingService.GetUserInvitationConfig(ctx)
	if !cfg.Available {
		return nil, ErrUserInvitationDisabled
	}

	codeValue, err := GenerateRedeemCode()
	if err != nil {
		return nil, fmt.Errorf("generate invitation code: %w", err)
	}

	now := s.now()
	creator := userID
	code := &RedeemCode{
		Code:      codeValue,
		Type:      RedeemTypeInvitation,
		Status:    StatusUnused,
		Notes:     fmt.Sprintf("user invitation by user #%d", userID),
		CreatedBy: &creator,
	}
	if cfg.CodeValidityDays > 0 {
		expiresAt := now.Add(time.Duration(cfg.CodeValidityDays) * 24 * time.Hour)
		code.ExpiresAt = &expiresAt
	}

	if err := s.repo.CreateInvitationForUser(ctx, code, cfg.MaxCodesPerUser, now); err != nil {
		return nil, err
	}
	return code, nil
}
