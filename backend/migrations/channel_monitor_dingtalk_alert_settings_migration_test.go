package migrations

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestChannelMonitorDingTalkAlertSettingsMigration(t *testing.T) {
	content, err := FS.ReadFile("194_channel_monitor_dingtalk_alert_settings.sql")
	require.NoError(t, err)

	sql := strings.Join(strings.Fields(string(content)), " ")
	require.Contains(t, sql, "('channel_monitor_dingtalk_enabled', 'false', NOW())")
	require.Contains(t, sql, "('channel_monitor_dingtalk_webhook', '', NOW())")
	require.Contains(t, sql, "('channel_monitor_dingtalk_secret', '', NOW())")
	require.Contains(t, sql, "ON CONFLICT (key) DO NOTHING")
}
