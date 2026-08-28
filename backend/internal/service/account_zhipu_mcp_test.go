//go:build unit

package service

import (
	"net/http"
	"testing"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/stretchr/testify/require"
)

func TestIsZhipuMCPCapable(t *testing.T) {
	tests := []struct {
		name    string
		account *Account
		want    bool
	}{
		{
			name: "coding plan with enabled flag",
			account: &Account{
				Platform:    PlatformZhipu,
				Credentials: map[string]any{"account_mode": AccountModeCoding},
				Extra:       map[string]any{ZhipuMCPCapabilityExtraKey: true},
			},
			want: true,
		},
		{
			name: "payg with enabled flag",
			account: &Account{
				Platform:    PlatformZhipu,
				Credentials: map[string]any{"account_mode": AccountModePayG},
				Extra:       map[string]any{ZhipuMCPCapabilityExtraKey: true},
			},
			want: false,
		},
		{
			name: "coding plan without flag",
			account: &Account{
				Platform:    PlatformZhipu,
				Credentials: map[string]any{"account_mode": AccountModeCoding},
				Extra:       map[string]any{},
			},
			want: false,
		},
		{
			name: "non zhipu platform with enabled flag",
			account: &Account{
				Platform:    PlatformOpenAI,
				Credentials: map[string]any{"account_mode": AccountModeCoding},
				Extra:       map[string]any{ZhipuMCPCapabilityExtraKey: true},
			},
			want: false,
		},
		{
			name: "nil extra",
			account: &Account{
				Platform:    PlatformZhipu,
				Credentials: map[string]any{"account_mode": AccountModeCoding},
			},
			want: false,
		},
		{
			name:    "nil account",
			account: nil,
			want:    false,
		},
		{
			name: "malformed flag value",
			account: &Account{
				Platform:    PlatformZhipu,
				Credentials: map[string]any{"account_mode": AccountModeCoding},
				Extra:       map[string]any{ZhipuMCPCapabilityExtraKey: "true"},
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, tt.account.IsZhipuMCPCapable())
		})
	}
}

func TestValidateZhipuMCPExtra(t *testing.T) {
	t.Run("non zhipu platform is ignored", func(t *testing.T) {
		err := ValidateZhipuMCPExtra(PlatformOpenAI,
			map[string]any{"account_mode": AccountModePayG},
			map[string]any{ZhipuMCPCapabilityExtraKey: "not-a-bool"})

		require.NoError(t, err)
	})

	t.Run("missing key passes", func(t *testing.T) {
		err := ValidateZhipuMCPExtra(PlatformZhipu,
			map[string]any{"account_mode": AccountModePayG},
			map[string]any{"other": true})

		require.NoError(t, err)
	})

	t.Run("non bool value is rejected", func(t *testing.T) {
		err := ValidateZhipuMCPExtra(PlatformZhipu,
			map[string]any{"account_mode": AccountModeCoding},
			map[string]any{ZhipuMCPCapabilityExtraKey: "true"})

		require.Error(t, err)
		require.Equal(t, http.StatusBadRequest, infraerrors.Code(err))
	})

	t.Run("coding plan with true passes", func(t *testing.T) {
		err := ValidateZhipuMCPExtra(PlatformZhipu,
			map[string]any{"account_mode": AccountModeCoding},
			map[string]any{ZhipuMCPCapabilityExtraKey: true})

		require.NoError(t, err)
	})

	t.Run("payg with true is rejected", func(t *testing.T) {
		err := ValidateZhipuMCPExtra(PlatformZhipu,
			map[string]any{"account_mode": AccountModePayG},
			map[string]any{ZhipuMCPCapabilityExtraKey: true})

		require.Error(t, err)
		require.Equal(t, http.StatusBadRequest, infraerrors.Code(err))
	})

	t.Run("missing account_mode with true is rejected", func(t *testing.T) {
		err := ValidateZhipuMCPExtra(PlatformZhipu, nil, map[string]any{ZhipuMCPCapabilityExtraKey: true})

		require.Error(t, err)
		require.Equal(t, http.StatusBadRequest, infraerrors.Code(err))
	})

	t.Run("payg with false passes", func(t *testing.T) {
		err := ValidateZhipuMCPExtra(PlatformZhipu,
			map[string]any{"account_mode": AccountModePayG},
			map[string]any{ZhipuMCPCapabilityExtraKey: false})

		require.NoError(t, err)
	})
}

func TestNormalizeZhipuMCPUpdateExtra(t *testing.T) {
	codingAccount := &Account{
		Platform:    PlatformZhipu,
		Credentials: map[string]any{"account_mode": AccountModeCoding},
		Extra:       map[string]any{ZhipuMCPCapabilityExtraKey: true},
	}

	t.Run("omitted key preserves current value", func(t *testing.T) {
		input := &UpdateAccountInput{Extra: map[string]any{"base_rpm": float64(60)}}
		normalized, err := normalizeZhipuMCPUpdateExtra(codingAccount, input, map[string]any{"base_rpm": float64(60)})

		require.NoError(t, err)
		require.Equal(t, true, normalized[ZhipuMCPCapabilityExtraKey])
		require.Equal(t, float64(60), normalized["base_rpm"])
	})

	t.Run("provided key replaces current value", func(t *testing.T) {
		input := &UpdateAccountInput{Extra: map[string]any{ZhipuMCPCapabilityExtraKey: false}}
		normalized, err := normalizeZhipuMCPUpdateExtra(codingAccount, input, map[string]any{ZhipuMCPCapabilityExtraKey: false})

		require.NoError(t, err)
		require.Equal(t, false, normalized[ZhipuMCPCapabilityExtraKey])
	})

	t.Run("non zhipu account is unchanged", func(t *testing.T) {
		normalized := map[string]any{ZhipuMCPCapabilityExtraKey: "provider-owned"}
		got, err := normalizeZhipuMCPUpdateExtra(&Account{Platform: PlatformOpenAI},
			&UpdateAccountInput{Extra: map[string]any{}}, normalized)

		require.NoError(t, err)
		require.Equal(t, normalized, got)
	})

	t.Run("non bool value is rejected on update", func(t *testing.T) {
		input := &UpdateAccountInput{Extra: map[string]any{ZhipuMCPCapabilityExtraKey: "true"}}
		_, err := normalizeZhipuMCPUpdateExtra(codingAccount, input, nil)

		require.Error(t, err)
		require.Equal(t, http.StatusBadRequest, infraerrors.Code(err))
	})

	t.Run("new payg credentials with true is rejected", func(t *testing.T) {
		input := &UpdateAccountInput{
			Credentials: map[string]any{"account_mode": AccountModePayG},
			Extra:       map[string]any{ZhipuMCPCapabilityExtraKey: true},
		}
		_, err := normalizeZhipuMCPUpdateExtra(codingAccount, input, nil)

		require.Error(t, err)
		require.Equal(t, http.StatusBadRequest, infraerrors.Code(err))
	})

	t.Run("new coding credentials with true passes for payg account", func(t *testing.T) {
		paygAccount := &Account{
			Platform:    PlatformZhipu,
			Credentials: map[string]any{"account_mode": AccountModePayG},
			Extra:       map[string]any{},
		}
		input := &UpdateAccountInput{
			Credentials: map[string]any{"account_mode": AccountModeCoding},
			Extra:       map[string]any{ZhipuMCPCapabilityExtraKey: true},
		}
		normalized, err := normalizeZhipuMCPUpdateExtra(paygAccount, input, map[string]any{ZhipuMCPCapabilityExtraKey: true})

		require.NoError(t, err)
		require.Equal(t, true, normalized[ZhipuMCPCapabilityExtraKey])
	})
}
