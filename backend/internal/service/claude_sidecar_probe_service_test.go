//go:build unit

package service

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestFilterSidecarProbeTargets(t *testing.T) {
	withToken := map[string]any{"access_token": "tok"}
	emptyToken := map[string]any{"access_token": ""}

	tests := []struct {
		name    string
		input   []Account
		wantIDs []int64
	}{
		{
			name: "active oauth with token is kept",
			input: []Account{
				{ID: 1, Status: StatusActive, Type: AccountTypeOAuth, Credentials: withToken},
			},
			wantIDs: []int64{1},
		},
		{
			name: "setup token account skipped (no profile scope)",
			input: []Account{
				{ID: 1, Status: StatusActive, Type: AccountTypeSetupToken, Credentials: withToken},
			},
			wantIDs: nil,
		},
		{
			name: "api key account skipped",
			input: []Account{
				{ID: 1, Status: StatusActive, Type: AccountTypeAPIKey, Credentials: withToken},
			},
			wantIDs: nil,
		},
		{
			name: "disabled oauth skipped",
			input: []Account{
				{ID: 1, Status: StatusDisabled, Type: AccountTypeOAuth, Credentials: withToken},
			},
			wantIDs: nil,
		},
		{
			name: "error oauth skipped",
			input: []Account{
				{ID: 1, Status: StatusError, Type: AccountTypeOAuth, Credentials: withToken},
			},
			wantIDs: nil,
		},
		{
			name: "oauth without access token skipped",
			input: []Account{
				{ID: 1, Status: StatusActive, Type: AccountTypeOAuth, Credentials: emptyToken},
			},
			wantIDs: nil,
		},
		{
			name: "mixed pool keeps only eligible",
			input: []Account{
				{ID: 1, Status: StatusActive, Type: AccountTypeOAuth, Credentials: withToken},
				{ID: 2, Status: StatusActive, Type: AccountTypeSetupToken, Credentials: withToken},
				{ID: 3, Status: StatusActive, Type: AccountTypeOAuth, Credentials: withToken},
				{ID: 4, Status: StatusError, Type: AccountTypeOAuth, Credentials: withToken},
			},
			wantIDs: []int64{1, 3},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := filterSidecarProbeTargets(tt.input)
			var gotIDs []int64
			for _, a := range got {
				gotIDs = append(gotIDs, a.ID)
			}
			require.Equal(t, tt.wantIDs, gotIDs)
		})
	}
}

func TestClaudeSidecarProbeJitterInterval(t *testing.T) {
	t.Run("returns min when max equals min", func(t *testing.T) {
		s := &ClaudeSidecarProbeService{
			minInterval: 5 * time.Minute,
			maxInterval: 5 * time.Minute,
		}
		require.Equal(t, 5*time.Minute, s.jitterInterval())
	})

	t.Run("returns value in [min,max) when range is valid", func(t *testing.T) {
		s := &ClaudeSidecarProbeService{
			minInterval: 5 * time.Minute,
			maxInterval: 15 * time.Minute,
		}
		// Sample enough times to exercise the jitter range.
		for i := 0; i < 200; i++ {
			v := s.jitterInterval()
			require.GreaterOrEqual(t, v, 5*time.Minute)
			require.Less(t, v, 15*time.Minute)
		}
	})
}

func TestClaudeSidecarProbeLifecycle(t *testing.T) {
	t.Run("nil receiver is safe", func(t *testing.T) {
		var s *ClaudeSidecarProbeService
		require.NotPanics(t, func() {
			s.Start()
			s.Stop()
		})
	})

	t.Run("missing deps skip loop", func(t *testing.T) {
		s := NewClaudeSidecarProbeService(nil, nil, 5*time.Minute, 15*time.Minute, false)
		// Start is a no-op because accountRepo / usageService are nil.
		// Stop must still be safe.
		require.NotPanics(t, func() {
			s.Start()
			s.Stop()
		})
	})

	t.Run("zero interval skips loop", func(t *testing.T) {
		s := NewClaudeSidecarProbeService(nil, nil, 0, 0, false)
		require.NotPanics(t, func() {
			s.Start()
			s.Stop()
		})
	})

	t.Run("double stop is safe", func(t *testing.T) {
		s := NewClaudeSidecarProbeService(nil, nil, 5*time.Minute, 15*time.Minute, false)
		s.Start()
		s.Stop()
		require.NotPanics(t, func() { s.Stop() })
	})
}

func TestSidecarProbeConstantsCoherent(t *testing.T) {
	require.Less(t, sidecarProbeRequestTimeout, sidecarProbeMinIntervalFloor,
		"request timeout must be smaller than min interval")
}
