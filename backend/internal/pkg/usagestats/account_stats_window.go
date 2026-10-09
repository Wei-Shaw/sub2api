package usagestats

import (
	"fmt"
	"time"
)

const (
	MaxAccountStatsWindows = 100
	MaxAccountStatsWindow  = 31 * 24 * time.Hour
)

// AccountStatsWindow is an exact, half-open interval for one account.
// Accounts can have different subscription reset times.
type AccountStatsWindow struct {
	AccountID int64     `json:"account_id"`
	StartAt   time.Time `json:"start_at"`
	EndAt     time.Time `json:"end_at"`
}

// ValidateAccountStatsWindows bounds work and prevents ambiguous map keys.
func ValidateAccountStatsWindows(windows []AccountStatsWindow) error {
	if len(windows) == 0 || len(windows) > MaxAccountStatsWindows {
		return fmt.Errorf("windows must contain between 1 and %d accounts", MaxAccountStatsWindows)
	}
	seen := make(map[int64]struct{}, len(windows))
	for i, window := range windows {
		if window.AccountID <= 0 {
			return fmt.Errorf("windows[%d].account_id must be positive", i)
		}
		if _, exists := seen[window.AccountID]; exists {
			return fmt.Errorf("duplicate account_id %d", window.AccountID)
		}
		seen[window.AccountID] = struct{}{}
		if window.StartAt.IsZero() || window.EndAt.IsZero() || !window.EndAt.After(window.StartAt) {
			return fmt.Errorf("windows[%d] must have start_at before end_at", i)
		}
		if window.EndAt.Sub(window.StartAt) > MaxAccountStatsWindow {
			return fmt.Errorf("windows[%d] must not exceed 31 days", i)
		}
	}
	return nil
}
