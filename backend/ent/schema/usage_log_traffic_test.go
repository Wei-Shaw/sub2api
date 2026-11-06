package schema_test

import (
	"strings"
	"testing"

	"entgo.io/ent/dialect"
	"entgo.io/ent/schema/field"
	"github.com/Wei-Shaw/sub2api/ent/schema"
	"github.com/Wei-Shaw/sub2api/migrations"
	"github.com/stretchr/testify/require"
)

func TestUsageTrafficSchemaNullableWithoutDefault(t *testing.T) {
	var descriptor *field.Descriptor
	for _, candidate := range (schema.UsageLog{}).Fields() {
		if candidate.Descriptor().Name == "traffic" {
			descriptor = candidate.Descriptor()
			break
		}
	}
	require.NotNil(t, descriptor)
	require.Equal(t, field.TypeJSON, descriptor.Info.Type)
	require.True(t, descriptor.Optional)
	require.Nil(t, descriptor.Default)
	require.Equal(t, "jsonb", descriptor.SchemaType[dialect.Postgres])

	migration, err := migrations.FS.ReadFile("246_add_usage_log_traffic.sql")
	require.NoError(t, err)
	var statements []string
	for _, line := range strings.Split(string(migration), "\n") {
		if !strings.HasPrefix(strings.TrimSpace(line), "--") {
			statements = append(statements, line)
		}
	}
	sql := strings.ToUpper(strings.Join(statements, "\n"))
	require.Contains(t, sql, "ALTER TABLE USAGE_LOGS ADD COLUMN IF NOT EXISTS TRAFFIC JSONB;")
	require.NotContains(t, sql, "DEFAULT")
	require.NotContains(t, sql, "NOT NULL")
	require.NotContains(t, sql, "UPDATE USAGE_LOGS")
}
