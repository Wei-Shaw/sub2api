package service

import (
	"regexp"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

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

func TestGenerateRedeemCodeForType_UsesExpectedPrefixesAndFormat(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		codeType string
		pattern  string
	}{
		{name: "balance", codeType: RedeemTypeBalance, pattern: `^BAL-[A-Z0-9]{4}-[A-Z0-9]{4}-[A-Z0-9]{4}$`},
		{name: "subscription", codeType: RedeemTypeSubscription, pattern: `^SUB-[A-Z0-9]{4}-[A-Z0-9]{4}-[A-Z0-9]{4}$`},
		{name: "invitation", codeType: RedeemTypeInvitation, pattern: `^INV-[A-Z0-9]{4}-[A-Z0-9]{4}-[A-Z0-9]{4}$`},
		{name: "concurrency", codeType: RedeemTypeConcurrency, pattern: `^CON-[A-Z0-9]{4}-[A-Z0-9]{4}-[A-Z0-9]{4}$`},
		{name: "admin balance adjustment", codeType: AdjustmentTypeAdminBalance, pattern: `^BAL-[A-Z0-9]{4}-[A-Z0-9]{4}-[A-Z0-9]{4}$`},
		{name: "admin concurrency adjustment", codeType: AdjustmentTypeAdminConcurrency, pattern: `^CON-[A-Z0-9]{4}-[A-Z0-9]{4}-[A-Z0-9]{4}$`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			code, err := GenerateRedeemCodeForType(tt.codeType)
			require.NoError(t, err)
			require.Regexp(t, regexp.MustCompile(tt.pattern), code)
		})
	}
}

func TestGenerateRedeemCodeForType_UnknownTypeReturnsError(t *testing.T) {
	t.Parallel()

	code, err := GenerateRedeemCodeForType("mystery")
	require.Error(t, err)
	require.Empty(t, code)
}

func TestNormalizeRedeemCode(t *testing.T) {
	t.Parallel()

	require.Equal(t, "BAL-CGBA-WX9R-N1PA", NormalizeRedeemCode("  bal-cgba-wx9r-n1pa  "))
	require.Equal(t, "SUB-ABCD-EF12-GH34", NormalizeRedeemCode("sub-abcd-ef12-gh34"))
	require.Equal(t, "", NormalizeRedeemCode("   "))
}
