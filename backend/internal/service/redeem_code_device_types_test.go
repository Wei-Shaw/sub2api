//go:build unit

package service

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGenerateRedeemCodeForTypeSupportsDeviceCodes(t *testing.T) {
	claimCode, err := GenerateRedeemCodeForType(RedeemTypeDeviceClaim)
	require.NoError(t, err)
	require.True(t, strings.HasPrefix(claimCode, "DCL-"), claimCode)

	loginCode, err := GenerateRedeemCodeForType(RedeemTypeDeviceLogin)
	require.NoError(t, err)
	require.True(t, strings.HasPrefix(loginCode, "DLG-"), loginCode)
}
