package service

import (
	"context"
	"crypto/rand"
	"fmt"
	"math/big"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
)

const (
	CheckInModeStandard = "standard"
	CheckInModeReduced  = "reduced"
)

var ErrCheckInDisabled = infraerrors.Forbidden("CHECKIN_DISABLED", "daily check-in is disabled")

type CheckInConfig struct {
	Enabled          bool      `json:"enabled"`
	StandardMin      int       `json:"standard_min"`
	StandardMax      int       `json:"standard_max"`
	ReducedThreshold float64   `json:"reduced_threshold"`
	ReducedMin       int       `json:"reduced_min"`
	ReducedMax       int       `json:"reduced_max"`
	UpdatedAt        time.Time `json:"updated_at"`
}

func (c CheckInConfig) RewardRange(cycleReward float64) (mode string, minReward, maxReward int) {
	if cycleReward >= c.ReducedThreshold {
		return CheckInModeReduced, c.ReducedMin, c.ReducedMax
	}
	return CheckInModeStandard, c.StandardMin, c.StandardMax
}

type CheckInRecord struct {
	ID               int64     `json:"id"`
	Date             string    `json:"date"`
	Reward           float64   `json:"reward"`
	Mode             string    `json:"mode"`
	CycleRewardAfter float64   `json:"cycle_reward_after"`
	CreatedAt        time.Time `json:"created_at"`
}

type CheckInStatus struct {
	Config         CheckInConfig   `json:"config"`
	CheckedToday   bool            `json:"checked_today"`
	TodayReward    float64         `json:"today_reward"`
	CycleReward    float64         `json:"cycle_reward"`
	TotalReward    float64         `json:"total_reward"`
	NextRewardMode string          `json:"next_reward_mode"`
	NextRewardMin  int             `json:"next_reward_min"`
	NextRewardMax  int             `json:"next_reward_max"`
	RecentCheckIns []CheckInRecord `json:"recent_checkins"`
	ServerDate     string          `json:"server_date"`
	ServerTimezone string          `json:"server_timezone"`
}

type CheckInResult struct {
	Record         CheckInRecord `json:"record"`
	AlreadyChecked bool          `json:"already_checked"`
	NewBalance     float64       `json:"new_balance"`
}

type CheckInTrendPoint struct {
	Date   string  `json:"date"`
	Users  int64   `json:"users"`
	Reward float64 `json:"reward"`
}

type CheckInAdminStats struct {
	Config           CheckInConfig       `json:"config"`
	TodayUsers       int64               `json:"today_users"`
	TodayReward      float64             `json:"today_reward"`
	TotalUsers       int64               `json:"total_users"`
	TotalReward      float64             `json:"total_reward"`
	ReducedModeUsers int64               `json:"reduced_mode_users"`
	Trend            []CheckInTrendPoint `json:"trend"`
	ServerDate       string              `json:"server_date"`
	ServerTimezone   string              `json:"server_timezone"`
}

type CheckInResetResult struct {
	AffectedUsers int64     `json:"affected_users"`
	ResetAt       time.Time `json:"reset_at"`
}

type CheckInRewardPicker func(min, max int) (int, error)

type CheckInRepository interface {
	GetUserStatus(ctx context.Context, userID int64, day time.Time, historyLimit int) (*CheckInStatus, error)
	CheckIn(ctx context.Context, userID int64, day time.Time, picker CheckInRewardPicker) (*CheckInResult, error)
	GetAdminStats(ctx context.Context, day time.Time, days int) (*CheckInAdminStats, error)
	SetEnabled(ctx context.Context, enabled bool) (*CheckInConfig, error)
	ResetAllCycles(ctx context.Context, resetAt time.Time) (int64, error)
}

type CheckInService struct {
	repo                 CheckInRepository
	billingCacheService  *BillingCacheService
	authCacheInvalidator APIKeyAuthCacheInvalidator
	pickReward           CheckInRewardPicker
}

func NewCheckInService(repo CheckInRepository, billingCacheService *BillingCacheService, authCacheInvalidator APIKeyAuthCacheInvalidator) *CheckInService {
	return &CheckInService{
		repo:                 repo,
		billingCacheService:  billingCacheService,
		authCacheInvalidator: authCacheInvalidator,
		pickReward:           randomCheckInReward,
	}
}

func (s *CheckInService) GetUserStatus(ctx context.Context, userID int64) (*CheckInStatus, error) {
	status, err := s.repo.GetUserStatus(ctx, userID, timezone.Today(), 14)
	if err != nil {
		return nil, err
	}
	s.decorateStatus(status)
	return status, nil
}

func (s *CheckInService) CheckIn(ctx context.Context, userID int64) (*CheckInResult, error) {
	result, err := s.repo.CheckIn(ctx, userID, timezone.Today(), s.pickReward)
	if err != nil {
		return nil, err
	}
	if result.AlreadyChecked {
		return result, nil
	}

	if s.authCacheInvalidator != nil {
		s.authCacheInvalidator.InvalidateAuthCacheByUserID(ctx, userID)
	}
	if s.billingCacheService != nil {
		cacheCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = s.billingCacheService.InvalidateUserBalance(cacheCtx, userID)
	}
	return result, nil
}

func (s *CheckInService) GetAdminStats(ctx context.Context, days int) (*CheckInAdminStats, error) {
	if days < 1 {
		days = 30
	}
	if days > 90 {
		days = 90
	}
	stats, err := s.repo.GetAdminStats(ctx, timezone.Today(), days)
	if err != nil {
		return nil, err
	}
	stats.ServerDate = timezone.Today().Format("2006-01-02")
	stats.ServerTimezone = timezone.Name()
	return stats, nil
}

func (s *CheckInService) SetEnabled(ctx context.Context, enabled bool) (*CheckInConfig, error) {
	return s.repo.SetEnabled(ctx, enabled)
}

func (s *CheckInService) ResetAllCycles(ctx context.Context) (*CheckInResetResult, error) {
	now := timezone.Now()
	affected, err := s.repo.ResetAllCycles(ctx, now)
	if err != nil {
		return nil, err
	}
	return &CheckInResetResult{AffectedUsers: affected, ResetAt: now}, nil
}

func (s *CheckInService) decorateStatus(status *CheckInStatus) {
	if status == nil {
		return
	}
	status.ServerDate = timezone.Today().Format("2006-01-02")
	status.ServerTimezone = timezone.Name()
	status.NextRewardMode, status.NextRewardMin, status.NextRewardMax = status.Config.RewardRange(status.CycleReward)
	if status.RecentCheckIns == nil {
		status.RecentCheckIns = []CheckInRecord{}
	}
}

func randomCheckInReward(min, max int) (int, error) {
	if min < 0 || max < min {
		return 0, fmt.Errorf("invalid check-in reward range: %d-%d", min, max)
	}
	span := int64(max - min + 1)
	n, err := rand.Int(rand.Reader, big.NewInt(span))
	if err != nil {
		return 0, fmt.Errorf("generate check-in reward: %w", err)
	}
	return min + int(n.Int64()), nil
}
