//go:build integration

package repository

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestUsageCleanupExclusiveBoundaryPostgres(t *testing.T) {
	tx := testTx(t)
	_, err := tx.Exec(`CREATE TEMP TABLE cleanup_boundary (id integer, created_at timestamptz) ON COMMIT DROP`)
	require.NoError(t, err)
	start := time.Date(2026, 9, 10, 15, 0, 0, 0, time.UTC)
	end := start.Add(30 * time.Minute)
	for _, tc := range []struct {
		name      string
		exclusive bool
		remaining int
	}{
		{"new exclusive tasks", true, 3}, {"legacy inclusive tasks", false, 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := tx.Exec("TRUNCATE cleanup_boundary")
			require.NoError(t, err)
			for i, instant := range []time.Time{start.Add(-time.Microsecond), start, end.Add(-time.Microsecond), end, end.Add(time.Microsecond)} {
				_, err = tx.Exec("INSERT INTO cleanup_boundary VALUES ($1, $2)", i, instant)
				require.NoError(t, err)
			}
			original := service.UsageCleanupFilters{StartTime: start, EndTime: end, EndExclusive: tc.exclusive}
			stored, err := json.Marshal(original)
			require.NoError(t, err)
			var restored service.UsageCleanupFilters
			require.NoError(t, json.Unmarshal(stored, &restored))
			where, args := buildUsageCleanupWhere(restored)
			_, err = tx.Exec("DELETE FROM cleanup_boundary WHERE "+where, args...)
			require.NoError(t, err)
			var count int
			require.NoError(t, tx.QueryRow("SELECT count(*) FROM cleanup_boundary").Scan(&count))
			require.Equal(t, tc.remaining, count)
			var retained bool
			require.NoError(t, tx.QueryRow("SELECT EXISTS(SELECT 1 FROM cleanup_boundary WHERE created_at=$1)", end).Scan(&retained))
			require.Equal(t, tc.exclusive, retained)
		})
	}
}
