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
}
