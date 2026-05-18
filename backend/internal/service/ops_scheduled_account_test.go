package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestResolveOpsScheduledAccount_PrefersTopLevelSnapshot(t *testing.T) {
	t.Parallel()

	accountID := int64(101)
	resolvedID, resolvedName := ResolveOpsScheduledAccount(&accountID, "selected-account", `[{"account_id":202,"account_name":"fallback-account"}]`)
	require.NotNil(t, resolvedID)
	require.Equal(t, int64(101), *resolvedID)
	require.Equal(t, "selected-account", resolvedName)
}

func TestResolveOpsScheduledAccount_FallsBackToLastUpstreamEvent(t *testing.T) {
	t.Parallel()

	resolvedID, resolvedName := ResolveOpsScheduledAccount(nil, "", `[{"account_id":11,"account_name":"first"},{"account_id":22,"account_name":"last"}]`)
	require.NotNil(t, resolvedID)
	require.Equal(t, int64(22), *resolvedID)
	require.Equal(t, "last", resolvedName)
}

func TestResolveOpsScheduledAccount_UsesPartialTopLevelAndBackfillsName(t *testing.T) {
	t.Parallel()

	accountID := int64(303)
	resolvedID, resolvedName := ResolveOpsScheduledAccount(&accountID, "", `[{"account_id":404,"account_name":"fallback-name"}]`)
	require.NotNil(t, resolvedID)
	require.Equal(t, int64(303), *resolvedID)
	require.Equal(t, "fallback-name", resolvedName)
}

func TestResolveOpsScheduledAccount_IgnoresInvalidPayload(t *testing.T) {
	t.Parallel()

	resolvedID, resolvedName := ResolveOpsScheduledAccount(nil, "", `not-json`)
	require.Nil(t, resolvedID)
	require.Equal(t, "", resolvedName)
}
