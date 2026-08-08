package repository

import (
	"strings"
	"testing"

	dbmigrations "github.com/Wei-Shaw/sub2api/migrations"
)

func TestCostCenterMigrationIsAdditiveAndIdempotent(t *testing.T) {
	sql, err := dbmigrations.FS.ReadFile("200_cost_center.sql")
	if err != nil {
		t.Fatal(err)
	}
	text := string(sql)
	for _, required := range []string{"CREATE TABLE IF NOT EXISTS cost_center_events", "CREATE TABLE IF NOT EXISTS cost_center_expense_plans", "CREATE TABLE IF NOT EXISTS cost_center_subscription_entitlements", "CREATE UNIQUE INDEX IF NOT EXISTS"} {
		if !strings.Contains(text, required) {
			t.Fatalf("migration missing %q", required)
		}
	}
	if strings.Contains(strings.ToLower(text), "insert into usage_logs") || strings.Contains(strings.ToLower(text), "update usage_logs") {
		t.Fatal("cost center migration must not backfill usage history")
	}
}
