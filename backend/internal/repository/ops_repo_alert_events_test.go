package repository

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestBuildOpsAlertEventsWhereUsesEmailSentFilter(t *testing.T) {
	sent := true
	where, args := buildOpsAlertEventsWhere(&service.OpsAlertEventFilter{EmailSent: &sent})

	require.Contains(t, where, "e.email_sent = $1")
	require.NotContains(t, where, "e.email_queued")
	require.Equal(t, []any{true}, args)
}
