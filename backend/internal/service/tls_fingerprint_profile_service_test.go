//go:build unit

package service

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/model"
	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	"github.com/stretchr/testify/require"
)

func TestResolveTLSProfileOpenAIUsesDefaultAndHonorsExplicitProfile(t *testing.T) {
	service := &TLSFingerprintProfileService{
		localCache: map[int64]*model.TLSFingerprintProfile{
			17: {
				ID:           17,
				Name:         "operator override",
				CipherSuites: []uint16{0x1301},
			},
		},
	}

	for _, accountType := range []string{AccountTypeOAuth, AccountTypeAPIKey} {
		t.Run(accountType+" default", func(t *testing.T) {
			profile := service.ResolveTLSProfile(&Account{
				Platform: PlatformOpenAI,
				Type:     accountType,
			})

			require.NotNil(t, profile)
			require.Equal(t, tlsfingerprint.PresetChrome120HTTP1, profile.Preset)
		})

		t.Run(accountType+" explicit override", func(t *testing.T) {
			profile := service.ResolveTLSProfile(&Account{
				Platform: PlatformOpenAI,
				Type:     accountType,
				Extra: map[string]any{
					"tls_fingerprint_profile_id": int64(17),
				},
			})

			require.NotNil(t, profile)
			require.Equal(t, "operator override", profile.Name)
			require.Empty(t, profile.Preset)
			require.Equal(t, []uint16{0x1301}, profile.CipherSuites)
		})
	}
}

func TestResolveTLSProfileDoesNotDefaultUnsupportedOpenAIAccountType(t *testing.T) {
	service := &TLSFingerprintProfileService{}

	profile := service.ResolveTLSProfile(&Account{
		Platform: PlatformOpenAI,
		Type:     AccountTypeUpstream,
	})

	require.Nil(t, profile)

	profile = service.ResolveTLSProfile(&Account{
		Platform: PlatformAnthropic,
		Type:     AccountTypeOAuth,
		Extra:    map[string]any{"openai_oauth_tls_mode": "codex_rustls_fallback"},
	})
	require.Nil(t, profile, "the OpenAI-only mode must not change non-OpenAI accounts")
}

func TestResolveTLSProfileOpenAIOAuthTLSModePrecedence(t *testing.T) {
	service := &TLSFingerprintProfileService{
		localCache: map[int64]*model.TLSFingerprintProfile{
			17: {ID: 17, Name: "operator override", CipherSuites: []uint16{0x1301}},
		},
	}

	tests := []struct {
		name       string
		account    *Account
		wantName   string
		wantPreset tlsfingerprint.Preset
		wantRandom bool
	}{
		{
			name: "explicit profile wins over rustls mode",
			account: &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth, Extra: map[string]any{
				"tls_fingerprint_profile_id": int64(17),
				"openai_oauth_tls_mode":      "codex_rustls_fallback",
			}},
			wantName: "operator override",
		},
		{
			name: "random pool wins over rustls mode",
			account: &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth, Extra: map[string]any{
				"tls_fingerprint_profile_id": int64(-1),
				"openai_oauth_tls_mode":      "codex_rustls_fallback",
			}},
			wantName: "operator override",
		},
		{
			name: "stale explicit profile suppresses rustls mode and keeps legacy fallback",
			account: &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth, Extra: map[string]any{
				"tls_fingerprint_profile_id": int64(999),
				"openai_oauth_tls_mode":      "codex_rustls_fallback",
			}},
			wantPreset: tlsfingerprint.PresetChrome120HTTP1,
		},
		{
			name: "OAuth rustls mode is opt in",
			account: &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth, Extra: map[string]any{
				"openai_oauth_tls_mode": "codex_rustls_fallback",
			}},
			wantName:   "Codex 0.149.x rustls fallback (cold connection) approximation",
			wantRandom: true,
		},
		{
			name:       "unset OAuth mode keeps Chrome default",
			account:    &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth},
			wantPreset: tlsfingerprint.PresetChrome120HTTP1,
		},
		{
			name: "legacy Chrome mode keeps Chrome default",
			account: &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth, Extra: map[string]any{
				"openai_oauth_tls_mode": "legacy_chrome",
			}},
			wantPreset: tlsfingerprint.PresetChrome120HTTP1,
		},
		{
			name: "API key ignores rustls mode",
			account: &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Extra: map[string]any{
				"openai_oauth_tls_mode": "codex_rustls_fallback",
			}},
			wantPreset: tlsfingerprint.PresetChrome120HTTP1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			profile := service.ResolveTLSProfile(tt.account)
			require.NotNil(t, profile)
			if tt.wantName != "" {
				require.Equal(t, tt.wantName, profile.Name)
			}
			require.Equal(t, tt.wantPreset, profile.Preset)
			require.Equal(t, tt.wantRandom, profile.RandomizeExtensionOrder)
		})
	}

	emptyPoolProfile := (&TLSFingerprintProfileService{}).ResolveTLSProfile(&Account{
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
		Extra: map[string]any{
			"tls_fingerprint_profile_id": int64(-1),
			"openai_oauth_tls_mode":      "codex_rustls_fallback",
		},
	})
	require.Equal(t, tlsfingerprint.PresetChrome120HTTP1, emptyPoolProfile.Preset,
		"an explicitly selected but empty random pool must keep the legacy Chrome fallback")
}
