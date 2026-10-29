package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAccountSparkShadowHelpers(t *testing.T) {
	pid := int64(100)
	normal := &Account{ID: 100}
	require.False(t, normal.IsShadow())
	require.False(t, normal.IsCredentialShadow())
	require.Equal(t, QuotaDimensionGlobal, normal.QuotaDimensionOrDefault())
	shadow := &Account{ID: 200, ParentAccountID: &pid, QuotaDimension: QuotaDimensionSpark}
	require.True(t, shadow.IsShadow())
	require.True(t, shadow.IsCredentialShadow())
	require.Equal(t, QuotaDimensionSpark, shadow.QuotaDimensionOrDefault())
}
