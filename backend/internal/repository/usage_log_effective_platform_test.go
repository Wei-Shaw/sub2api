package repository

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestUsageLogEffectivePlatformExprUsesAccountPlatformForCompositeGroups(t *testing.T) {
	expr := strings.ToLower(usageLogEffectivePlatformExpr)

	require.Contains(t, expr, "g.platform = 'composite'")
	require.Contains(t, expr, "then a.platform")
	require.Contains(t, expr, "coalesce")
}
