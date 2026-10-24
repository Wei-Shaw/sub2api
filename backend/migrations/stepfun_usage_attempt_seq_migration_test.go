package migrations

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestStepFunUsageAttemptSequenceMigration(t *testing.T) {
	content, err := FS.ReadFile("247_stepfun_usage_attempt_seq.sql")
	require.NoError(t, err)

	sql := strings.Join(strings.Fields(string(content)), " ")
	require.Contains(t, sql, "CREATE SEQUENCE IF NOT EXISTS stepfun_usage_attempt_seq AS BIGINT")
}
