package dto

import (
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

// CheckInConfig exposes only the user-facing switch, never reward rules.
type CheckInConfig struct {
	Enabled bool `json:"enabled"`
}

type CheckInRecord struct {
	ID        int64     `json:"id"`
	Date      string    `json:"date"`
	Reward    float64   `json:"reward"`
	CreatedAt time.Time `json:"created_at"`
}

type CheckInStatus struct {
	Config         CheckInConfig   `json:"config"`
	CheckedToday   bool            `json:"checked_today"`
	TodayReward    float64         `json:"today_reward"`
	TotalReward    float64         `json:"total_reward"`
	RecentCheckIns []CheckInRecord `json:"recent_checkins"`
	ServerDate     string          `json:"server_date"`
	ServerTimezone string          `json:"server_timezone"`
}

type CheckInResult struct {
	Record         CheckInRecord `json:"record"`
	AlreadyChecked bool          `json:"already_checked"`
	NewBalance     float64       `json:"new_balance"`
}

func ToCheckInRecord(record service.CheckInRecord) CheckInRecord {
	return CheckInRecord{
		ID: record.ID, Date: record.Date, Reward: record.Reward, CreatedAt: record.CreatedAt,
	}
}

func ToCheckInStatus(status *service.CheckInStatus) *CheckInStatus {
	if status == nil {
		return nil
	}
	result := &CheckInStatus{
		Config:         CheckInConfig{Enabled: status.Config.Enabled},
		CheckedToday:   status.CheckedToday,
		TodayReward:    status.TodayReward,
		TotalReward:    status.TotalReward,
		RecentCheckIns: make([]CheckInRecord, 0, len(status.RecentCheckIns)),
		ServerDate:     status.ServerDate,
		ServerTimezone: status.ServerTimezone,
	}
	for _, record := range status.RecentCheckIns {
		result.RecentCheckIns = append(result.RecentCheckIns, ToCheckInRecord(record))
	}
	return result
}

func ToCheckInResult(result *service.CheckInResult) *CheckInResult {
	if result == nil {
		return nil
	}
	return &CheckInResult{
		Record:         ToCheckInRecord(result.Record),
		AlreadyChecked: result.AlreadyChecked,
		NewBalance:     result.NewBalance,
	}
}
