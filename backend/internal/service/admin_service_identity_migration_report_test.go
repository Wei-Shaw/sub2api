package service

import (
	"context"
	"database/sql"
	"testing"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/enttest"
	"github.com/stretchr/testify/require"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	_ "modernc.org/sqlite"
)

func newAdminServiceMigrationReportTestClient(t *testing.T) *dbent.Client {
REDACTED

	db, err := sql.Open("sqlite", "file:admin_service_migration_reports?mode=memory&cache=shared&_fk=1")
REDACTED
	t.Cleanup(func() { _ = db.Close() REDACTED)

	_, err = db.Exec("PRAGMA foreign_keys = ON")
REDACTED

	_, err = db.Exec(`CREATE TABLE auth_identity_migration_reports (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		report_type TEXT NOT NULL,
		report_key TEXT NOT NULL,
		details TEXT NOT NULL DEFAULT '{REDACTED',
		created_at DATETIME NOT NULL,
		resolved_at DATETIME NULL,
		resolved_by_user_id INTEGER NULL,
		resolution_note TEXT NOT NULL DEFAULT ''
	)`)
REDACTED

	drv := entsql.OpenDB(dialect.SQLite, db)
	client := enttest.NewClient(t, enttest.WithOptions(dbent.Driver(drv)))
	t.Cleanup(func() { _ = client.Close() REDACTED)
	return client
REDACTED

func TestAdminServiceListAuthIdentityMigrationReports(t *testing.T) {
	client := newAdminServiceMigrationReportTestClient(t)
	driver, ok := client.Driver().(*entsql.Driver)
	require.True(t, ok)

	now := time.Now().UTC()
	_, err := driver.DB().ExecContext(context.Background(), `
INSERT INTO auth_identity_migration_reports (report_type, report_key, details, created_at)
VALUES
	($1, $2, $3, $4),
	($5, $6, $7, $8)`,
		"oidc_synthetic_email_requires_manual_recovery", "u-1", `{"user_id":1REDACTED`, now,
		"wechat_provider_key_conflict", "u-2", `{"user_id":2REDACTED`, now.Add(-time.Minute),
	)
REDACTED

	svc := &adminServiceImpl{entClient: clientREDACTED
	reports, total, err := svc.ListAuthIdentityMigrationReports(context.Background(), "oidc_synthetic_email_requires_manual_recovery", 1, 20)
REDACTED
	require.Equal(t, int64(1), total)
	require.Len(t, reports, 1)
	require.Equal(t, "oidc_synthetic_email_requires_manual_recovery", reports[0].ReportType)
	require.Equal(t, float64(1), reports[0].Details["user_id"])
REDACTED

func TestAdminServiceGetAuthIdentityMigrationReportSummary(t *testing.T) {
	client := newAdminServiceMigrationReportTestClient(t)
	driver, ok := client.Driver().(*entsql.Driver)
	require.True(t, ok)

	now := time.Now().UTC()
	_, err := driver.DB().ExecContext(context.Background(), `
INSERT INTO auth_identity_migration_reports (report_type, report_key, details, created_at)
VALUES
	($1, $2, $3, $4),
	($5, $6, $7, $8),
	($9, $10, $11, $12)`,
		"oidc_synthetic_email_requires_manual_recovery", "u-1", `{"user_id":1REDACTED`, now,
		"wechat_provider_key_conflict", "u-2", `{"user_id":2REDACTED`, now.Add(-time.Minute),
		"wechat_provider_key_conflict", "u-3", `{"user_id":3REDACTED`, now.Add(-2*time.Minute),
	)
REDACTED

	svc := &adminServiceImpl{entClient: clientREDACTED
	summary, err := svc.GetAuthIdentityMigrationReportSummary(context.Background())
REDACTED
	require.Equal(t, int64(3), summary.Total)
	require.Equal(t, int64(3), summary.OpenTotal)
	require.Zero(t, summary.ResolvedTotal)
	require.Equal(t, int64(1), summary.ByType["oidc_synthetic_email_requires_manual_recovery"])
	require.Equal(t, int64(2), summary.ByType["wechat_provider_key_conflict"])
REDACTED

func TestAdminServiceResolveAuthIdentityMigrationReport(t *testing.T) {
	client := newAdminServiceMigrationReportTestClient(t)
	driver, ok := client.Driver().(*entsql.Driver)
	require.True(t, ok)

	now := time.Now().UTC()
	_, err := driver.DB().ExecContext(context.Background(), `
INSERT INTO auth_identity_migration_reports (report_type, report_key, details, created_at)
VALUES ($1, $2, $3, $4)`,
		"oidc_synthetic_email_requires_manual_recovery", "u-1", `{"user_id":1REDACTED`, now,
	)
REDACTED

	svc := &adminServiceImpl{entClient: clientREDACTED
	report, err := svc.ResolveAuthIdentityMigrationReport(context.Background(), 1, 99, "resolved by admin binding")
REDACTED
	require.NotNil(t, report.ResolvedAt)
	require.NotNil(t, report.ResolvedByUserID)
	require.Equal(t, int64(99), *report.ResolvedByUserID)
	require.Equal(t, "resolved by admin binding", report.ResolutionNote)

	summary, err := svc.GetAuthIdentityMigrationReportSummary(context.Background())
REDACTED
	require.Zero(t, summary.OpenTotal)
	require.Equal(t, int64(1), summary.ResolvedTotal)
REDACTED
