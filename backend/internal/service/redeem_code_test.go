package service

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestNormalizeRedeemValidityDays(t *testing.T) {
	days, seconds, err := NormalizeRedeemValidityDays(0.5)
	require.NoError(t, err)
	require.Equal(t, 0, days)
	require.Equal(t, int64(12*60*60), seconds)

	days, seconds, err = NormalizeRedeemValidityDays(2)
	require.NoError(t, err)
	require.Equal(t, 2, days)
	require.Equal(t, int64(0), seconds)

	days, seconds, err = NormalizeRedeemValidityDays(-7)
	require.NoError(t, err)
	require.Equal(t, -7, days)
	require.Equal(t, int64(0), seconds)

	_, _, err = NormalizeRedeemValidityDays(-0.5)
	require.Error(t, err)
}

func TestRedeemValiditySecondsNoteRoundTrip(t *testing.T) {
	encoded := EncodeRedeemValiditySecondsNote("manual note", 12*60*60)
	notes, seconds := DecodeRedeemValiditySecondsNote(encoded)
	require.Equal(t, "manual note", notes)
	require.Equal(t, int64(12*60*60), seconds)
}

func TestRedeemCodeExpiry(t *testing.T) {
	now := time.Now().UTC()
	past := now.Add(-time.Hour)
	future := now.Add(time.Hour)

	tests := []struct {
		name        string
		code        RedeemCode
		wantExpired bool
		wantCanUse  bool
	}{
		{
			name:        "unused without expiry can be used",
			code:        RedeemCode{Status: StatusUnused},
			wantExpired: false,
			wantCanUse:  true,
		},
		{
			name:        "unused before expiry can be used",
			code:        RedeemCode{Status: StatusUnused, ExpiresAt: &future},
			wantExpired: false,
			wantCanUse:  true,
		},
		{
			name:        "unused after expiry cannot be used",
			code:        RedeemCode{Status: StatusUnused, ExpiresAt: &past},
			wantExpired: true,
			wantCanUse:  false,
		},
		{
			name:        "explicit expired status is expired",
			code:        RedeemCode{Status: StatusExpired},
			wantExpired: true,
			wantCanUse:  false,
		},
		{
			name:        "used code remains used even after expiry time",
			code:        RedeemCode{Status: StatusUsed, ExpiresAt: &past},
			wantExpired: false,
			wantCanUse:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.wantExpired, tt.code.IsExpiredAt(now))
			require.Equal(t, tt.wantCanUse, tt.code.CanUse())
		})
	}
}
