package ent

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/ent/group"
	"github.com/stretchr/testify/require"
)

// Both upstream translations and the HA scheduler use JSON columns.
func TestGroupScanI18nAndSchedulerConfig(t *testing.T) {
	entity := &Group{}
	columns := []string{group.FieldI18n, group.FieldSchedulerConfig}
	values, err := entity.scanValues(columns)
	require.NoError(t, err)

	payloads := []string{
		`{"en":{"name":"HA group","description":"High availability"}}`,
		`{"strategy":"high_availability","selection_mode":"strict_health"}`,
	}
	for i, payload := range payloads {
		target, ok := values[i].(*[]byte)
		require.True(t, ok, "column %s must scan as JSON", columns[i])
		*target = []byte(payload)
	}

	require.NoError(t, entity.assignValues(columns, values))
	require.Equal(t, "HA group", entity.I18n["en"].Name)
	require.Equal(t, "High availability", entity.I18n["en"].Description)
	require.Equal(t, "high_availability", entity.SchedulerConfig["strategy"])
	require.Equal(t, "strict_health", entity.SchedulerConfig["selection_mode"])
}
