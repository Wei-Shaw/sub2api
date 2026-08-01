//go:build unit

package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFormatNonNegativeFloatExact(t *testing.T) {
	t.Parallel()

	zero := 0.0
	precise := 7.123456789
	negative := -1.0

	require.Equal(t, "", formatNonNegativeFloatExact(nil))
	require.Equal(t, "", formatNonNegativeFloatExact(&negative))
	require.Equal(t, "0", formatNonNegativeFloatExact(&zero), "zero must remain an explicit conversion-disabled value")
	require.Equal(t, "7.123456789", formatNonNegativeFloatExact(&precise), "exchange-rate precision must not be rounded to two decimals")
}
