//go:build integration

package repository

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestChannelMonitorV2AccountBalanceTaxonomy(t *testing.T) {
	for _, test := range []struct {
		name, message, category string
	}{
		{"account balance", "Insufficient account balance", "quota_or_balance"},
		{"case insensitive", "INSUFFICIENT ACCOUNT BALANCE", "quota_or_balance"},
		{"unrelated forbidden", "Access denied", "upstream_forbidden"},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx := context.Background()
			tx := testTx(t)
			start := time.Now().UTC().Truncate(time.Minute)
			const model = "channel-monitor-balance-taxonomy"
			_, err := tx.ExecContext(ctx, `
				INSERT INTO ops_error_logs
					(platform, model, error_phase, error_type, status_code, error_message, created_at)
				VALUES ('openai', $1, 'request', 'api_error', 403, $2, $3)`,
				model, test.message, start.Add(time.Second))
			require.NoError(t, err)

			_, err = tx.ExecContext(ctx, channelMonitorV2ErrorAggregationSQL, start, start.Add(time.Minute))
			require.NoError(t, err)
			var category string
			var count int64
			require.NoError(t, tx.QueryRowContext(ctx, `
				SELECT error_category, error_requests FROM channel_monitor_v2_error_metrics_1m
				WHERE bucket_start = $1 AND model = $2`, start, model).Scan(&category, &count))
			require.Equal(t, test.category, category)
			require.EqualValues(t, 1, count)
			require.Equal(t, category, service.ClassifyChannelMonitorV2Error(service.ChannelMonitorV2ErrorInput{
				ErrorType: "api_error", StatusCode: 403, Message: test.message,
			}))
		})
	}
}
