package service

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseRegistrationFingerprintFromUA(t *testing.T) {
	tests := []struct {
		name     string
		ua       string
		wantOS   string
		wantArch string
		wantNil  bool
	}{
		{
			name:    "empty UA returns nil",
			ua:      "",
			wantNil: true,
		},
		{
			name:    "whitespace UA returns nil",
			ua:      "   ",
			wantNil: true,
		},
		{
			name:     "macOS Safari",
			ua:       "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.0 Safari/605.1.15",
			wantOS:   "MacOS",
			wantArch: "arm64",
		},
		{
			name:     "macOS Chrome",
			ua:       "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
			wantOS:   "MacOS",
			wantArch: "arm64",
		},
		{
			name:     "Windows 10 Chrome x64",
			ua:       "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
			wantOS:   "Windows",
			wantArch: "x64",
		},
		{
			name:     "Windows 11 Edge",
			ua:       "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36 Edg/120.0.0.0",
			wantOS:   "Windows",
			wantArch: "x64",
		},
		{
			name:     "Linux x86_64",
			ua:       "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
			wantOS:   "Linux",
			wantArch: "x64",
		},
		{
			name:     "Linux aarch64",
			ua:       "Mozilla/5.0 (X11; Linux aarch64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
			wantOS:   "Linux",
			wantArch: "arm64",
		},
		{
			name:     "Android phone",
			ua:       "Mozilla/5.0 (Linux; Android 14; SM-S918B) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Mobile Safari/537.36",
			wantOS:   "Android", // Android takes priority over Linux
			wantArch: "arm64",
		},
		{
			name:     "iPhone",
			ua:       "Mozilla/5.0 (iPhone; CPU iPhone OS 17_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.0 Mobile/15E148 Safari/604.1",
			wantOS:   "iOS",
			wantArch: "arm64",
		},
		{
			name:     "iPad",
			ua:       "Mozilla/5.0 (iPad; CPU OS 17_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.0 Mobile/15E148 Safari/604.1",
			wantOS:   "iOS",
			wantArch: "arm64",
		},
		{
			name:     "Unknown UA captures raw text but no OS/arch",
			ua:       "SomeRandomBot/1.0",
			wantOS:   "",
			wantArch: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fp := ParseRegistrationFingerprintFromUA(tt.ua)
			if tt.wantNil {
				require.Nil(t, fp)
				return
			}
			require.NotNil(t, fp)
			require.Equal(t, tt.wantOS, fp.OS, "OS mismatch")
			require.Equal(t, tt.wantArch, fp.Arch, "Arch mismatch")
			require.Equal(t, "node", fp.Runtime, "runtime should always default to node for CLI mimicry")
			require.Equal(t, "v24.3.0", fp.RuntimeVersion)
			require.Equal(t, tt.ua, fp.UserAgent)
			require.Greater(t, fp.CapturedAt, int64(0), "CapturedAt should be set")
		})
	}
}

func TestGetRegistrationFingerprint(t *testing.T) {
	t.Run("nil account returns nil", func(t *testing.T) {
		require.Nil(t, GetRegistrationFingerprint(nil))
	})

	t.Run("nil extra returns nil", func(t *testing.T) {
		account := &Account{}
		require.Nil(t, GetRegistrationFingerprint(account))
	})

	t.Run("missing key returns nil", func(t *testing.T) {
		account := &Account{Extra: map[string]any{"other_key": "value"}}
		require.Nil(t, GetRegistrationFingerprint(account))
	})

	t.Run("nil value returns nil", func(t *testing.T) {
		account := &Account{Extra: map[string]any{ExtraKeyRegistrationFingerprint: nil}}
		require.Nil(t, GetRegistrationFingerprint(account))
	})

	t.Run("pointer value returns same pointer", func(t *testing.T) {
		fp := &RegistrationFingerprint{OS: "MacOS", Arch: "arm64"}
		account := &Account{Extra: map[string]any{ExtraKeyRegistrationFingerprint: fp}}
		got := GetRegistrationFingerprint(account)
		require.NotNil(t, got)
		require.Equal(t, "MacOS", got.OS)
		require.Equal(t, "arm64", got.Arch)
	})

	t.Run("struct value (not pointer) decoded correctly", func(t *testing.T) {
		fp := RegistrationFingerprint{OS: "Windows", Arch: "x64"}
		account := &Account{Extra: map[string]any{ExtraKeyRegistrationFingerprint: fp}}
		got := GetRegistrationFingerprint(account)
		require.NotNil(t, got)
		require.Equal(t, "Windows", got.OS)
		require.Equal(t, "x64", got.Arch)
	})

	t.Run("map value (post-jsonb-decode) decoded correctly", func(t *testing.T) {
		// Simulate Postgres jsonb round-trip
		raw, _ := json.Marshal(map[string]any{
			"os":              "Linux",
			"arch":            "arm64",
			"runtime":         "node",
			"runtime_version": "v22.11.0",
			"user_agent":      "Mozilla/5.0 (X11; Linux aarch64)",
			"captured_at":     int64(1234567890),
		})
		var asMap map[string]any
		_ = json.Unmarshal(raw, &asMap)
		account := &Account{Extra: map[string]any{ExtraKeyRegistrationFingerprint: asMap}}
		got := GetRegistrationFingerprint(account)
		require.NotNil(t, got)
		require.Equal(t, "Linux", got.OS)
		require.Equal(t, "arm64", got.Arch)
		require.Equal(t, "node", got.Runtime)
		require.Equal(t, int64(1234567890), got.CapturedAt)
	})

	t.Run("invalid type returns nil", func(t *testing.T) {
		account := &Account{Extra: map[string]any{ExtraKeyRegistrationFingerprint: 12345}}
		require.Nil(t, GetRegistrationFingerprint(account))
	})
}

func TestSetRegistrationFingerprintInExtra(t *testing.T) {
	t.Run("nil fp leaves extra unchanged", func(t *testing.T) {
		extra := map[string]any{"key": "value"}
		got := SetRegistrationFingerprintInExtra(extra, nil)
		require.Equal(t, "value", got["key"])
		require.NotContains(t, got, ExtraKeyRegistrationFingerprint)
	})

	t.Run("nil extra creates new map", func(t *testing.T) {
		fp := &RegistrationFingerprint{OS: "MacOS"}
		got := SetRegistrationFingerprintInExtra(nil, fp)
		require.NotNil(t, got)
		require.Contains(t, got, ExtraKeyRegistrationFingerprint)
	})

	t.Run("existing extra gets fp added", func(t *testing.T) {
		extra := map[string]any{"existing": "value"}
		fp := &RegistrationFingerprint{OS: "Windows", Arch: "x64"}
		got := SetRegistrationFingerprintInExtra(extra, fp)
		require.Equal(t, "value", got["existing"])
		require.Contains(t, got, ExtraKeyRegistrationFingerprint)

		// Verify roundtrip
		account := &Account{Extra: got}
		retrieved := GetRegistrationFingerprint(account)
		require.NotNil(t, retrieved)
		require.Equal(t, "Windows", retrieved.OS)
	})
}
