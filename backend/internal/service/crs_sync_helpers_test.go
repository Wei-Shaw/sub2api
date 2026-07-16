package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBuildSelectedSet(t *testing.T) {
	tests := []struct {
		name     string
		ids      []string
		wantNil  bool
		wantSize int
	}{
		{
			name:    "nil input returns nil (backward compatible: create all)",
			ids:     nil,
			wantNil: true,
		},
		{
			name:     "empty slice returns empty map (create none)",
			ids:      []string{},
			wantNil:  false,
			wantSize: 0,
		},
		{
			name:     "single ID",
			ids:      []string{"abc-123"},
			wantNil:  false,
			wantSize: 1,
		},
		{
			name:     "multiple IDs",
			ids:      []string{"a", "b", "c"},
			wantNil:  false,
			wantSize: 3,
		},
		{
			name:     "duplicate IDs are deduplicated",
			ids:      []string{"a", "a", "b"},
			wantNil:  false,
			wantSize: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildSelectedSet(tt.ids)
			if tt.wantNil {
				if got != nil {
					t.Errorf("buildSelectedSet(%v) = %v, want nil", tt.ids, got)
				}
				return
			}
			if got == nil {
				t.Fatalf("buildSelectedSet(%v) = nil, want non-nil map", tt.ids)
			}
			if len(got) != tt.wantSize {
				t.Errorf("buildSelectedSet(%v) has %d entries, want %d", tt.ids, len(got), tt.wantSize)
			}
			// Verify all unique IDs are present
			for _, id := range tt.ids {
				if _, ok := got[id]; !ok {
					t.Errorf("buildSelectedSet(%v) missing key %q", tt.ids, id)
				}
			}
		})
	}
}

func TestShouldCreateAccount(t *testing.T) {
	tests := []struct {
		name        string
		crsID       string
		selectedSet map[string]struct{}
		want        bool
	}{
		{
			name:        "nil set allows all (backward compatible)",
			crsID:       "any-id",
			selectedSet: nil,
			want:        true,
		},
		{
			name:        "empty set blocks all",
			crsID:       "any-id",
			selectedSet: map[string]struct{}{},
			want:        false,
		},
		{
			name:        "ID in set is allowed",
			crsID:       "abc-123",
			selectedSet: map[string]struct{}{"abc-123": {}, "def-456": {}},
			want:        true,
		},
		{
			name:        "ID not in set is blocked",
			crsID:       "xyz-789",
			selectedSet: map[string]struct{}{"abc-123": {}, "def-456": {}},
			want:        false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := shouldCreateAccount(tt.crsID, tt.selectedSet)
			if got != tt.want {
				t.Errorf("shouldCreateAccount(%q, %v) = %v, want %v",
					tt.crsID, tt.selectedSet, got, tt.want)
			}
		})
	}
}

func TestReconcileCRSUpstreamBillingProbeExtra(t *testing.T) {
	remote := map[string]any{
		"crs_account_id":                    "remote-1",
		UpstreamBillingProbeEnabledExtraKey: true,
		UpstreamBillingProbeExtraKey:        map[string]any{"status": "remote"},
	}

	t.Run("create drops remote managed fields", func(t *testing.T) {
		extra := mergeMap(nil, remote)
		reconcileCRSUpstreamBillingProbeExtra(nil, PlatformOpenAI, AccountTypeAPIKey, map[string]any{"api_key": "new"}, extra)
		require.NotContains(t, extra, UpstreamBillingProbeEnabledExtraKey)
		require.NotContains(t, extra, UpstreamBillingProbeExtraKey)
	})

	existing := &Account{
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Credentials: map[string]any{"api_key": "local", "base_url": "http://127.0.0.1:8080"},
		Extra: map[string]any{
			UpstreamBillingProbeEnabledExtraKey: false,
			UpstreamBillingProbeExtraKey:        map[string]any{"status": "local"},
		},
	}

	t.Run("same identity keeps local state", func(t *testing.T) {
		extra := mergeMap(existing.Extra, remote)
		reconcileCRSUpstreamBillingProbeExtra(existing, existing.Platform, existing.Type, mergeMap(existing.Credentials, nil), extra)
		require.Equal(t, false, extra[UpstreamBillingProbeEnabledExtraKey])
		require.Equal(t, map[string]any{"status": "local"}, extra[UpstreamBillingProbeExtraKey])
	})

	t.Run("identity change keeps enabled and clears snapshot", func(t *testing.T) {
		extra := mergeMap(existing.Extra, remote)
		reconcileCRSUpstreamBillingProbeExtra(existing, PlatformOpenAI, AccountTypeAPIKey, map[string]any{"api_key": "changed"}, extra)
		require.Equal(t, false, extra[UpstreamBillingProbeEnabledExtraKey])
		require.NotContains(t, extra, UpstreamBillingProbeExtraKey)
	})

	for _, target := range []struct {
		name     string
		platform string
		typeName string
	}{
		{name: "anthropic oauth", platform: PlatformAnthropic, typeName: AccountTypeOAuth},
		{name: "anthropic api key", platform: PlatformAnthropic, typeName: AccountTypeAPIKey},
		{name: "openai oauth", platform: PlatformOpenAI, typeName: AccountTypeOAuth},
		{name: "gemini oauth", platform: PlatformGemini, typeName: AccountTypeOAuth},
		{name: "gemini api key", platform: PlatformGemini, typeName: AccountTypeAPIKey},
	} {
		t.Run(target.name+" removes inapplicable state", func(t *testing.T) {
			extra := mergeMap(existing.Extra, remote)
			reconcileCRSUpstreamBillingProbeExtra(existing, target.platform, target.typeName, existing.Credentials, extra)
			require.NotContains(t, extra, UpstreamBillingProbeEnabledExtraKey)
			require.NotContains(t, extra, UpstreamBillingProbeExtraKey)
		})
	}
}
