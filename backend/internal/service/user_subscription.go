package service

import "time"

const (
	subscriptionDayDuration = 24 * time.Hour
	dailyQuotaWindow        = 24 * time.Hour
	weeklyQuotaWindow       = 7 * dailyQuotaWindow
	monthlyQuotaWindow      = 30 * dailyQuotaWindow
)

type UserSubscription struct {
	ID      int64
	UserID  int64
	GroupID int64

	StartsAt  time.Time
	ExpiresAt time.Time
	Status    string

	DailyWindowStart   *time.Time
	WeeklyWindowStart  *time.Time
	MonthlyWindowStart *time.Time

	DailyUsageUSD   float64
	WeeklyUsageUSD  float64
	MonthlyUsageUSD float64

	AssignedBy *int64
	AssignedAt time.Time
	Notes      string

	CreatedAt time.Time
	UpdatedAt time.Time
	DeletedAt *time.Time

	User           *User
	Group          *Group
	AssignedByUser *User
}

func (s *UserSubscription) IsActive() bool {
	return s.Status == SubscriptionStatusActive && time.Now().Before(s.ExpiresAt)
}

func (s *UserSubscription) IsExpired() bool {
	return time.Now().After(s.ExpiresAt)
}

func (s *UserSubscription) DaysRemaining() int {
	return s.daysRemainingAt(time.Now())
}

func (s *UserSubscription) daysRemainingAt(now time.Time) int {
	remaining := s.ExpiresAt.Sub(now)
	if remaining <= 0 {
		return 0
	}

	days := int(remaining / subscriptionDayDuration)
	if remaining%subscriptionDayDuration != 0 {
		days++
	}
	return days
}

func (s *UserSubscription) IsWindowActivated() bool {
	return s.DailyWindowStart != nil || s.WeeklyWindowStart != nil || s.MonthlyWindowStart != nil
}

func (s *UserSubscription) NeedsDailyReset() bool {
	return s.NeedsDailyResetAt(time.Now())
}

func (s *UserSubscription) NeedsDailyResetAt(now time.Time) bool {
	if s.DailyWindowStart == nil {
		return false
	}
	if !s.ExpiresAt.IsZero() && !now.Before(s.ExpiresAt) {
		return false
	}
	return s.CurrentDailyWindowStart(now).After(*s.DailyWindowStart)
}

func (s *UserSubscription) NeedsWeeklyReset() bool {
	return s.NeedsWeeklyResetAt(time.Now())
}

func (s *UserSubscription) NeedsWeeklyResetAt(now time.Time) bool {
	if s.WeeklyWindowStart == nil {
		return false
	}
	if !s.ExpiresAt.IsZero() && !now.Before(s.ExpiresAt) {
		return false
	}
	return s.CurrentWeeklyWindowStart(now).After(*s.WeeklyWindowStart)
}

func (s *UserSubscription) NeedsMonthlyReset() bool {
	return s.NeedsMonthlyResetAt(time.Now())
}

func (s *UserSubscription) NeedsMonthlyResetAt(now time.Time) bool {
	if s.MonthlyWindowStart == nil {
		return false
	}
	if !s.ExpiresAt.IsZero() && !now.Before(s.ExpiresAt) {
		return false
	}
	return s.CurrentMonthlyWindowStart(now).After(*s.MonthlyWindowStart)
}

func (s *UserSubscription) DailyResetTime() *time.Time {
	return s.windowResetTime(s.DailyWindowStart, dailyQuotaWindow)
}

func (s *UserSubscription) WeeklyResetTime() *time.Time {
	return s.windowResetTime(s.WeeklyWindowStart, weeklyQuotaWindow)
}

func (s *UserSubscription) MonthlyResetTime() *time.Time {
	return s.windowResetTime(s.MonthlyWindowStart, monthlyQuotaWindow)
}

func (s *UserSubscription) CurrentDailyWindowStart(now time.Time) time.Time {
	return s.currentWindowStart(now, dailyQuotaWindow)
}

func (s *UserSubscription) CurrentWeeklyWindowStart(now time.Time) time.Time {
	return s.currentWindowStart(now, weeklyQuotaWindow)
}

func (s *UserSubscription) CurrentMonthlyWindowStart(now time.Time) time.Time {
	return s.currentWindowStart(now, monthlyQuotaWindow)
}

func (s *UserSubscription) currentWindowStart(now time.Time, window time.Duration) time.Time {
	if s == nil || s.StartsAt.IsZero() || window <= 0 {
		return startOfDay(now)
	}
	if now.Before(s.StartsAt) {
		return s.StartsAt
	}
	elapsed := now.Sub(s.StartsAt)
	periods := elapsed / window
	return s.StartsAt.Add(periods * window)
}

func (s *UserSubscription) windowResetTime(windowStart *time.Time, window time.Duration) *time.Time {
	if windowStart == nil {
		return nil
	}
	t := windowStart.Add(window)
	if !s.ExpiresAt.IsZero() && t.After(s.ExpiresAt) {
		t = s.ExpiresAt
	}
	return &t
}

func (s *UserSubscription) CheckDailyLimit(group *Group, additionalCost float64) bool {
	if !group.HasDailyLimit() {
		return true
	}
	return s.DailyUsageUSD+additionalCost <= *group.DailyLimitUSD
}

func (s *UserSubscription) CheckWeeklyLimit(group *Group, additionalCost float64) bool {
	if !group.HasWeeklyLimit() {
		return true
	}
	return s.WeeklyUsageUSD+additionalCost <= *group.WeeklyLimitUSD
}

func (s *UserSubscription) CheckMonthlyLimit(group *Group, additionalCost float64) bool {
	if !group.HasMonthlyLimit() {
		return true
	}
	return s.MonthlyUsageUSD+additionalCost <= *group.MonthlyLimitUSD
}

func (s *UserSubscription) CheckAllLimits(group *Group, additionalCost float64) (daily, weekly, monthly bool) {
	daily = s.CheckDailyLimit(group, additionalCost)
	weekly = s.CheckWeeklyLimit(group, additionalCost)
	monthly = s.CheckMonthlyLimit(group, additionalCost)
	return
}
