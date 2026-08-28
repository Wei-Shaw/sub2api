package ip

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNormalizeClientIPForSecurity(t *testing.T) {
	require.Equal(t, "2001:db8:abcd:1234::", NormalizeClientIPForSecurity("2001:db8:abcd:1234:ffff::1"))
	require.Equal(t, "2001:db8:abcd:1234::", NormalizeClientIPForSecurity("2001:db8:abcd:1234:1111::1"))
	require.Equal(t,
		NormalizeClientIPForSecurity("2001:db8:abcd:1234:1111::1"),
		NormalizeClientIPForSecurity("2001:db8:abcd:1234:ffff::2"),
	)
	require.Equal(t, "192.0.2.4", NormalizeClientIPForSecurity("::ffff:192.0.2.4"))
	require.Equal(t, "192.0.2.10", NormalizeClientIPForSecurity("192.0.2.10"))
	require.Equal(t, "", NormalizeClientIPForSecurity("not-an-ip"), "invalid → empty (skip member)")
	require.Equal(t, "", NormalizeClientIPForSecurity(""))
}

func TestSecurityEvidenceWhitelistRule(t *testing.T) {
	rule, ok := SecurityEvidenceWhitelistRule("2001:db8:abcd:1234::")
	require.True(t, ok)
	require.Equal(t, "2001:db8:abcd:1234::/64", rule)

	rule, ok = SecurityEvidenceWhitelistRule("2001:db8:abcd:1234:ffff::1")
	require.True(t, ok)
	require.Equal(t, "2001:db8:abcd:1234::/64", rule)

	rule, ok = SecurityEvidenceWhitelistRule("192.0.2.10")
	require.True(t, ok)
	require.Equal(t, "192.0.2.10", rule)

	_, ok = SecurityEvidenceWhitelistRule("not-an-ip")
	require.False(t, ok)
}
