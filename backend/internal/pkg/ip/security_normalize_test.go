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
