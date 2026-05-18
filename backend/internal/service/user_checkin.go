package service

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

var (
	ErrDailyCheckInDisabled       = infraerrors.Forbidden("DAILY_CHECK_IN_DISABLED", "daily check-in is disabled")
	ErrDailyCheckInAlreadyChecked = infraerrors.Conflict("DAILY_CHECK_IN_ALREADY_CHECKED", "daily check-in already completed today")
	ErrDailyCheckInInvalidMonth   = infraerrors.BadRequest("DAILY_CHECK_IN_INVALID_MONTH", "month must be between 1 and 12")
)

const (
	defaultDailyCheckInEnabled = true
	defaultDailyCheckInReward  = 0.1
)

type DailyCheckInStatus struct {
	Enabled        bool       `json:"enabled"`
	RewardAmount   float64    `json:"reward_amount"`
	CheckInDays    int        `json:"check_in_days"`
	LastCheckInAt  *time.Time `json:"last_check_in_at,omitempty"`
	CheckedInToday bool       `json:"checked_in_today"`
}

type DailyCheckInCalendar struct {
	Enabled        bool     `json:"enabled"`
	Year           int      `json:"year"`
	Month          int      `json:"month"`
	CheckedInDates []string `json:"checked_in_dates"`
	CheckedInDays  int      `json:"checked_in_days"`
}

func (s *UserService) GetDailyCheckInStatus(ctx context.Context, userID int64) (*DailyCheckInStatus, error) {
	enabled, rewardAmount := s.getDailyCheckInSettings(ctx)
	if !enabled {
		return &DailyCheckInStatus{
			Enabled:      false,
			RewardAmount: rewardAmount,
		}, nil
	}

	checkInDays, lastCheckInAt, err := s.userRepo.GetDailyCheckInStatus(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("get daily check-in status: %w", err)
	}

	return buildDailyCheckInStatus(enabled, rewardAmount, checkInDays, lastCheckInAt, time.Now()), nil
}

func (s *UserService) GetDailyCheckInCalendar(ctx context.Context, userID int64, year int, month time.Month) (*DailyCheckInCalendar, error) {
	if month < time.January || month > time.December {
		return nil, ErrDailyCheckInInvalidMonth
	}

	enabled, _ := s.getDailyCheckInSettings(ctx)
	result := &DailyCheckInCalendar{
		Enabled:        enabled,
		Year:           year,
		Month:          int(month),
		CheckedInDates: []string{},
	}
	if !enabled {
		return result, nil
	}

	dates, err := s.userRepo.GetDailyCheckInMonth(ctx, userID, year, month)
	if err != nil {
		return nil, fmt.Errorf("get daily check-in calendar: %w", err)
	}

	result.CheckedInDates = make([]string, 0, len(dates))
	for _, date := range dates {
		result.CheckedInDates = append(result.CheckedInDates, date.In(time.Local).Format("2006-01-02"))
	}
	result.CheckedInDays = len(result.CheckedInDates)
	return result, nil
}

func (s *UserService) ApplyDailyCheckIn(ctx context.Context, userID int64) (*DailyCheckInStatus, error) {
	enabled, rewardAmount := s.getDailyCheckInSettings(ctx)
	if !enabled {
		return nil, ErrDailyCheckInDisabled
	}

	now := time.Now()
	checkInDays, lastCheckInAt, err := s.userRepo.ApplyDailyCheckIn(ctx, userID, rewardAmount, now)
	if err != nil {
		return nil, fmt.Errorf("apply daily check-in: %w", err)
	}
	s.invalidateBalanceRelatedCaches(ctx, userID)

	return buildDailyCheckInStatus(true, rewardAmount, checkInDays, lastCheckInAt, now), nil
}

func (s *UserService) getDailyCheckInSettings(ctx context.Context) (bool, float64) {
	if s == nil || s.settingRepo == nil {
		return defaultDailyCheckInEnabled, defaultDailyCheckInReward
	}

	values, err := s.settingRepo.GetMultiple(ctx, []string{
		SettingKeyDailyCheckInEnabled,
		SettingKeyDailyCheckInRewardAmount,
	})
	if err != nil {
		return defaultDailyCheckInEnabled, defaultDailyCheckInReward
	}

	enabled := defaultDailyCheckInEnabled
	if raw, ok := values[SettingKeyDailyCheckInEnabled]; ok && strings.TrimSpace(raw) != "" {
		enabled = raw == "true"
	}

	rewardAmount := defaultDailyCheckInReward
	if raw, ok := values[SettingKeyDailyCheckInRewardAmount]; ok && strings.TrimSpace(raw) != "" {
		if parsed, parseErr := strconv.ParseFloat(strings.TrimSpace(raw), 64); parseErr == nil && parsed >= 0 {
			rewardAmount = parsed
		}
	}

	return enabled, rewardAmount
}

func buildDailyCheckInStatus(enabled bool, rewardAmount float64, checkInDays int, lastCheckInAt *time.Time, now time.Time) *DailyCheckInStatus {
	return &DailyCheckInStatus{
		Enabled:        enabled,
		RewardAmount:   rewardAmount,
		CheckInDays:    checkInDays,
		LastCheckInAt:  normalizeCheckInTime(lastCheckInAt),
		CheckedInToday: isSameLocalDay(lastCheckInAt, now),
	}
}

func normalizeCheckInTime(value *time.Time) *time.Time {
	if value == nil || value.IsZero() {
		return nil
	}
	ts := value.UTC()
	return &ts
}

func isSameLocalDay(left *time.Time, right time.Time) bool {
	if left == nil || left.IsZero() {
		return false
	}
	ly, lm, ld := left.In(time.Local).Date()
	ry, rm, rd := right.In(time.Local).Date()
	return ly == ry && lm == rm && ld == rd
}

func isPreviousLocalDay(left *time.Time, right time.Time) bool {
	if left == nil || left.IsZero() {
		return false
	}
	leftStart := startOfLocalDay(*left)
	rightStart := startOfLocalDay(right)
	return leftStart.AddDate(0, 0, 1).Equal(rightStart)
}

func startOfLocalDay(value time.Time) time.Time {
	year, month, day := value.In(time.Local).Date()
	return time.Date(year, month, day, 0, 0, 0, 0, time.Local)
}
