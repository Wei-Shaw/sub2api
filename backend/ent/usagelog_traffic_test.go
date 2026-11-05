package ent

import (
	"encoding/json"
	"testing"

	"github.com/Wei-Shaw/sub2api/ent/usagelog"
	"github.com/Wei-Shaw/sub2api/internal/pkg/traffic"
	"github.com/stretchr/testify/require"
)

func TestUsageTrafficEntMapping(t *testing.T) {
	zero, actual := int64(0), int64(17)
	for _, value := range []*traffic.Usage{
		nil,
		{Upstream: &traffic.Link{Response: &traffic.Measurement{ActualBytes: &actual, Compressed: true}}, Downstream: &traffic.Link{Request: &traffic.Measurement{OriginalBytes: &zero, ActualBytes: &zero}}},
	} {
		var entity UsageLog
		columns := []string{usagelog.FieldTraffic}
		values, err := entity.scanValues(columns)
		require.NoError(t, err)
		payload, err := json.Marshal(value)
		require.NoError(t, err)
		trafficValue, ok := values[0].(*[]byte)
		require.True(t, ok)
		*trafficValue = payload
		require.NoError(t, entity.assignValues(columns, values))
		require.Equal(t, value, entity.Traffic)

		builder := NewClient().UsageLog.Create()
		if value != nil {
			builder.SetTraffic(value)
		}
		stored, exists := builder.Mutation().Traffic()
		require.Equal(t, value != nil, exists)
		require.Equal(t, value, stored)
	}

	var legacy UsageLog
	values, err := legacy.scanValues([]string{usagelog.FieldTraffic})
	require.NoError(t, err)
	require.NoError(t, legacy.assignValues([]string{usagelog.FieldTraffic}, values))
	require.Nil(t, legacy.Traffic, "SQL NULL must remain unknown")
}
