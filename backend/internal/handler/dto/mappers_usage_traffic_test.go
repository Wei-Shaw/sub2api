package dto

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/traffic"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestUsageTrafficUserDTOCannotContainUpstream(t *testing.T) {
	_, hasUpstream := reflect.TypeOf(UserUsageTraffic{}).FieldByName("Upstream")
	require.False(t, hasUpstream, "role isolation must be structural, not only mapper filtering")
	zero, bytes := int64(0), int64(4321)
	for _, tc := range []struct {
		name  string
		value *traffic.Usage
	}{
		{"historical-null", nil},
		{"empty-unknown", &traffic.Usage{}},
		{"upstream-only", &traffic.Usage{Upstream: &traffic.Link{Request: &traffic.Measurement{ActualBytes: &bytes}}}},
		{"consumed-upstream-response", &traffic.Usage{Upstream: &traffic.Link{Response: &traffic.Measurement{ActualBytes: &bytes, Compressed: true}}}},
		{"unknown-downstream", &traffic.Usage{Upstream: &traffic.Link{Request: &traffic.Measurement{ActualBytes: &bytes}}, Downstream: &traffic.Link{}}},
		{"measured-zero", &traffic.Usage{Upstream: &traffic.Link{Request: &traffic.Measurement{ActualBytes: &bytes}}, Downstream: &traffic.Link{Request: &traffic.Measurement{OriginalBytes: &zero, ActualBytes: &zero}}}},
		{"partial-compressed", &traffic.Usage{Upstream: &traffic.Link{Request: &traffic.Measurement{ActualBytes: &bytes}}, Downstream: &traffic.Link{Response: &traffic.Measurement{ActualBytes: &bytes, Compressed: true}}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			log := &service.UsageLog{Traffic: tc.value}
			user := UsageLogFromService(log)
			admin := UsageLogFromServiceAdmin(log)
			userJSON, err := json.Marshal(user)
			require.NoError(t, err)
			adminJSON, err := json.Marshal(admin)
			require.NoError(t, err)
			var userFields, adminFields map[string]json.RawMessage
			require.NoError(t, json.Unmarshal(userJSON, &userFields))
			require.NoError(t, json.Unmarshal(adminJSON, &adminFields))
			require.Contains(t, userFields, "traffic", "historical rows explicitly expose null")
			require.Contains(t, adminFields, "traffic")
			require.NotContains(t, string(userFields["traffic"]), `"upstream"`)
			if tc.value == nil || tc.value.Downstream == nil {
				require.JSONEq(t, `null`, string(userFields["traffic"]))
			} else {
				var actual map[string]json.RawMessage
				require.NoError(t, json.Unmarshal(userFields["traffic"], &actual))
				require.Len(t, actual, 1)
				want, err := json.Marshal(tc.value.Downstream)
				require.NoError(t, err)
				require.JSONEq(t, string(want), string(actual["downstream"]))
			}
			wantAdmin, err := json.Marshal(tc.value)
			require.NoError(t, err)
			require.JSONEq(t, string(wantAdmin), string(adminFields["traffic"]))
			// Embedding an admin DTO's common view is still safe for reuse by user endpoints.
			commonJSON, err := json.Marshal(admin.UsageLog)
			require.NoError(t, err)
			require.JSONEq(t, string(userJSON), string(commonJSON))
			require.Same(t, tc.value, log.Traffic, "redaction must not mutate the service record")
		})
	}
}

func TestUsageTrafficDTOZeroUnknownAndNegativeSavings(t *testing.T) {
	zero, original, actual := int64(0), int64(4), int64(8)
	log := &service.UsageLog{Traffic: &traffic.Usage{Downstream: &traffic.Link{
		Request:  &traffic.Measurement{ActualBytes: &zero},
		Response: &traffic.Measurement{OriginalBytes: &original, ActualBytes: &actual, Compressed: true},
	}}}
	got := UsageLogFromService(log)
	payload, err := json.Marshal(got.Traffic)
	require.NoError(t, err)
	require.JSONEq(t, `{"downstream":{"request":{"original_bytes":null,"actual_bytes":0,"compressed":false},"response":{"original_bytes":4,"actual_bytes":8,"compressed":true}}}`, string(payload))
	require.Nil(t, got.Traffic.Downstream.Request.OriginalBytes)
	require.Equal(t, float64(-100), *got.Traffic.Downstream.Response.Ratio())
}
