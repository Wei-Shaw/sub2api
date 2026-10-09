package usagestats

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestValidateAccountStatsWindows(t *testing.T) {
	start := time.Date(2026, 9, 1, 1, 2, 3, 456000000, time.UTC)
	valid := AccountStatsWindow{AccountID: 1, StartAt: start, EndAt: start.Add(7 * 24 * time.Hour)}
	for _, tc := range []struct {
		name    string
		windows []AccountStatsWindow
	}{
		{"empty", nil},
		{"zero ID", []AccountStatsWindow{{StartAt: start, EndAt: valid.EndAt}}},
		{"negative ID", []AccountStatsWindow{{AccountID: -1, StartAt: start, EndAt: valid.EndAt}}},
		{"missing start", []AccountStatsWindow{{AccountID: 1, EndAt: valid.EndAt}}},
		{"missing end", []AccountStatsWindow{{AccountID: 1, StartAt: start}}},
		{"equal bounds", []AccountStatsWindow{{AccountID: 1, StartAt: start, EndAt: start}}},
		{"reversed", []AccountStatsWindow{{AccountID: 1, StartAt: valid.EndAt, EndAt: start}}},
		{"too long", []AccountStatsWindow{{AccountID: 1, StartAt: start, EndAt: start.Add(MaxAccountStatsWindow + time.Nanosecond)}}},
		{"duplicate", []AccountStatsWindow{valid, valid}},
		{"too many", make([]AccountStatsWindow, MaxAccountStatsWindows+1)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			require.Error(t, ValidateAccountStatsWindows(tc.windows))
		})
	}
	windows := make([]AccountStatsWindow, MaxAccountStatsWindows)
	for i := range windows {
		windows[i] = AccountStatsWindow{
			AccountID: int64(i + 1), StartAt: start.Add(time.Duration(i) * time.Second),
			EndAt: start.Add(time.Duration(i)*time.Second + MaxAccountStatsWindow),
		}
	}
	require.NoError(t, ValidateAccountStatsWindows(windows))
}
