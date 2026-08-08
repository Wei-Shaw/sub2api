package migrations

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestUsageLogMixedImageCostsMigrationDefinesAuditColumns(t *testing.T) {
	content, err := FS.ReadFile("196_usage_log_mixed_image_costs.sql")
	require.NoError(t, err)

	sql := string(content)
	require.Contains(t, sql, "image_generation_cost DECIMAL(20, 10) NOT NULL DEFAULT 0")
	require.Contains(t, sql, "image_actual_cost DECIMAL(20, 10) NOT NULL DEFAULT 0")
	require.Contains(t, sql, "image_rate_multiplier DECIMAL(10, 4) NOT NULL DEFAULT 0")
}
