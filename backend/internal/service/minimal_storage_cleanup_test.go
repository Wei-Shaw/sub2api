package service

import (
	"context"
	"regexp"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func minimalStorageTestConfig() *config.Config {
	return &config.Config{Storage: config.StorageConfig{
		Mode:                      config.StorageModeMinimal,
		UsageRetentionDays:        35,
		BillingDedupRetentionDays: 14,
		CleanupIntervalMinutes:    360,
	}}
}

func expectMinimalStorageLock(mock sqlmock.Sqlmock) {
	mock.ExpectQuery(regexp.QuoteMeta("SELECT pg_try_advisory_lock($1)")).
		WithArgs(minimalStorageCleanupLockKey).
		WillReturnRows(sqlmock.NewRows([]string{"locked"}).AddRow(true))
}

func expectMinimalStorageSessionStart(mock sqlmock.Sqlmock) {
	mock.ExpectExec(regexp.QuoteMeta("SET lock_timeout = '2000ms'")).
		WillReturnResult(sqlmock.NewResult(0, 0))
}

func expectMinimalStorageTableExistence(mock sqlmock.Sqlmock, tables []string, existing map[string]bool) {
	for _, table := range tables {
		mock.ExpectQuery(regexp.QuoteMeta("SELECT to_regclass($1) IS NOT NULL")).
			WithArgs("public." + table).
			WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(existing[table]))
	}
}

func expectNoMinimalStorageTables(mock sqlmock.Sqlmock) {
	expectMinimalStorageTableExistence(mock, minimalStorageOpsTables, nil)
	expectMinimalStorageTableExistence(mock, minimalStorageTruncateTables, nil)
}

func expectMinimalStorageSessionEnd(mock sqlmock.Sqlmock) {
	mock.ExpectExec(regexp.QuoteMeta("RESET lock_timeout")).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(regexp.QuoteMeta("SELECT pg_advisory_unlock($1)")).
		WithArgs(minimalStorageCleanupLockKey).
		WillReturnResult(sqlmock.NewResult(0, 1))
}

func TestMinimalStorageCleanupRunOnceSkipsWhenDisabled(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	svc := NewMinimalStorageCleanupService(db, &config.Config{})
	result, err := svc.RunOnce(context.Background())
	require.NoError(t, err)
	require.Equal(t, MinimalStorageCleanupResult{}, result)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestMinimalStorageCleanupRunOnceSkipsWhenLockIsHeld(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	mock.ExpectQuery(regexp.QuoteMeta("SELECT pg_try_advisory_lock($1)")).
		WithArgs(minimalStorageCleanupLockKey).
		WillReturnRows(sqlmock.NewRows([]string{"locked"}).AddRow(false))

	svc := NewMinimalStorageCleanupService(db, minimalStorageTestConfig())
	result, err := svc.RunOnce(context.Background())
	require.NoError(t, err)
	require.Equal(t, MinimalStorageCleanupResult{}, result)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestMinimalStorageCleanupDeletesExpiredDedupWithoutArchiving(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	expectMinimalStorageLock(mock)
	expectMinimalStorageSessionStart(mock)
	expectNoMinimalStorageTables(mock)
	mock.ExpectQuery(regexp.QuoteMeta("SELECT to_regclass('public.usage_logs') IS NOT NULL")).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT to_regclass('public.usage_billing_dedup') IS NOT NULL")).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectExec("(?s)DELETE FROM usage_billing_dedup").
		WithArgs(sqlmock.AnyArg(), minimalStorageDeleteBatch).
		WillReturnResult(sqlmock.NewResult(0, minimalStorageDeleteBatch))
	mock.ExpectExec("(?s)DELETE FROM usage_billing_dedup").
		WithArgs(sqlmock.AnyArg(), minimalStorageDeleteBatch).
		WillReturnResult(sqlmock.NewResult(0, 2))
	expectMinimalStorageSessionEnd(mock)

	svc := NewMinimalStorageCleanupService(db, minimalStorageTestConfig())
	result, err := svc.RunOnce(context.Background())
	require.NoError(t, err)
	require.EqualValues(t, minimalStorageDeleteBatch+2, result.DeletedBillingKeys)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestMinimalStorageCleanupBoundsDedupDeletesPerRun(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	expectMinimalStorageLock(mock)
	expectMinimalStorageSessionStart(mock)
	expectNoMinimalStorageTables(mock)
	mock.ExpectQuery(regexp.QuoteMeta("SELECT to_regclass('public.usage_logs') IS NOT NULL")).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT to_regclass('public.usage_billing_dedup') IS NOT NULL")).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	for range minimalStorageMaxDeleteBatches {
		mock.ExpectExec("(?s)DELETE FROM usage_billing_dedup").
			WithArgs(sqlmock.AnyArg(), minimalStorageDeleteBatch).
			WillReturnResult(sqlmock.NewResult(0, minimalStorageDeleteBatch))
	}
	expectMinimalStorageSessionEnd(mock)

	svc := NewMinimalStorageCleanupService(db, minimalStorageTestConfig())
	result, err := svc.RunOnce(context.Background())
	require.NoError(t, err)
	require.EqualValues(t, minimalStorageDeleteBatch*minimalStorageMaxDeleteBatches, result.DeletedBillingKeys)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestMinimalStorageCleanupTrimsUsageWithoutRemovingCurrentQuotaWindow(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	expectMinimalStorageLock(mock)
	expectMinimalStorageSessionStart(mock)
	expectNoMinimalStorageTables(mock)
	mock.ExpectQuery(regexp.QuoteMeta("SELECT to_regclass('public.usage_logs') IS NOT NULL")).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectQuery("(?s)SELECT child\\.relname.*FROM pg_inherits").
		WillReturnRows(sqlmock.NewRows([]string{"relname"}).
			AddRow("usage_logs_200001").
			AddRow("usage_logs_209901").
			AddRow("usage_logs_default"))
	mock.ExpectExec(regexp.QuoteMeta(`DROP TABLE IF EXISTS "public"."usage_logs_200001"`)).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("(?s)DELETE FROM usage_logs AS usage").
		WithArgs(sqlmock.AnyArg(), minimalStorageDeleteBatch).
		WillReturnResult(sqlmock.NewResult(0, 3))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT to_regclass('public.usage_billing_dedup') IS NOT NULL")).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))
	expectMinimalStorageSessionEnd(mock)

	svc := NewMinimalStorageCleanupService(db, minimalStorageTestConfig())
	result, err := svc.RunOnce(context.Background())
	require.NoError(t, err)
	require.Equal(t, 1, result.DroppedUsagePartitions)
	require.EqualValues(t, 3, result.DeletedUsageLogs)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestMinimalStorageCleanupReusesOpsExecutor(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	expectMinimalStorageLock(mock)
	expectMinimalStorageSessionStart(mock)
	expectMinimalStorageTableExistence(mock, minimalStorageOpsTables, map[string]bool{"ops_system_logs": true})
	mock.ExpectQuery(regexp.QuoteMeta("SELECT COUNT(*) FROM ops_system_logs")).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(12))
	mock.ExpectExec(regexp.QuoteMeta("TRUNCATE TABLE ops_system_logs")).
		WillReturnResult(sqlmock.NewResult(0, 0))
	expectMinimalStorageTableExistence(mock, minimalStorageTruncateTables, nil)
	mock.ExpectQuery(regexp.QuoteMeta("SELECT to_regclass('public.usage_logs') IS NOT NULL")).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT to_regclass('public.usage_billing_dedup') IS NOT NULL")).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))
	expectMinimalStorageSessionEnd(mock)

	svc := NewMinimalStorageCleanupService(db, minimalStorageTestConfig())
	result, err := svc.RunOnce(context.Background())
	require.NoError(t, err)
	require.Equal(t, 1, result.TruncatedTables)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestMinimalStorageCleanupTruncatesNonOpsTogetherWithoutCascade(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	tables := []string{"audit_logs", "prompt_audit_events", "prompt_audit_jobs"}
	existing := map[string]bool{}
	for _, table := range tables {
		existing[table] = true
	}
	expectMinimalStorageLock(mock)
	expectMinimalStorageSessionStart(mock)
	expectMinimalStorageTableExistence(mock, minimalStorageOpsTables, nil)
	expectMinimalStorageTableExistence(mock, minimalStorageTruncateTables, existing)
	query, err := minimalStorageTruncateQuery(tables)
	require.NoError(t, err)
	require.NotContains(t, strings.ToUpper(query), "CASCADE")
	mock.ExpectExec(regexp.QuoteMeta(query)).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT to_regclass('public.usage_logs') IS NOT NULL")).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT to_regclass('public.usage_billing_dedup') IS NOT NULL")).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))
	expectMinimalStorageSessionEnd(mock)

	svc := NewMinimalStorageCleanupService(db, minimalStorageTestConfig())
	result, err := svc.RunOnce(context.Background())
	require.NoError(t, err)
	require.Equal(t, len(tables), result.TruncatedTables)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestMinimalStorageCleanupNeverTargetsCoreTables(t *testing.T) {
	seen := map[string]struct{}{}
	tables := append(append([]string{}, minimalStorageOpsTables...), minimalStorageTruncateTables...)
	for _, table := range tables {
		_, duplicate := seen[table]
		require.Falsef(t, duplicate, "duplicate minimal storage cleanup table %q", table)
		seen[table] = struct{}{}
		_, protected := minimalStorageProtectedTables[table]
		require.Falsef(t, protected, "minimal storage cleanup must not target protected table %q", table)
	}
	for _, table := range []string{"users", "api_keys", "accounts", "groups", "settings", "user_subscriptions", "payment_orders"} {
		_, protected := minimalStorageProtectedTables[table]
		require.Truef(t, protected, "core table %q must be explicitly protected", table)
	}
	query, err := minimalStorageTruncateQuery([]string{"audit_logs", "users"})
	require.ErrorContains(t, err, "protected table users")
	require.Empty(t, query)
}

func TestMinimalStoragePolicyCompactsUsageMetadata(t *testing.T) {
	endpoint := "/v1/messages"
	userAgent := "large diagnostic user agent"
	ipAddress := "203.0.113.10"
	mapping := "requested-model -> upstream-model"
	reasoning := "high"
	duration := 123
	usage := &UsageLog{
		UserID:             1,
		APIKeyID:           2,
		AccountID:          3,
		RequestID:          "request-id",
		Model:              "model",
		TotalCost:          1.25,
		InboundEndpoint:    &endpoint,
		UpstreamEndpoint:   &endpoint,
		UserAgent:          &userAgent,
		IPAddress:          &ipAddress,
		ModelMappingChain:  &mapping,
		ReasoningEffort:    &reasoning,
		DurationMs:         &duration,
		FirstTokenMs:       &duration,
		ImageSizeBreakdown: map[string]int{"1024x1024": 2},
	}

	compactUsageLogForStorage(minimalStorageTestConfig(), usage)
	require.Equal(t, int64(1), usage.UserID)
	require.Equal(t, "request-id", usage.RequestID)
	require.Equal(t, "model", usage.Model)
	require.Equal(t, 1.25, usage.TotalCost)
	require.Nil(t, usage.InboundEndpoint)
	require.Nil(t, usage.UpstreamEndpoint)
	require.Nil(t, usage.UserAgent)
	require.Nil(t, usage.IPAddress)
	require.Nil(t, usage.ModelMappingChain)
	require.Nil(t, usage.ReasoningEffort)
	require.Nil(t, usage.DurationMs)
	require.Nil(t, usage.FirstTokenMs)
	require.Nil(t, usage.ImageSizeBreakdown)
}

func TestStandardStoragePolicyKeepsUsageMetadata(t *testing.T) {
	userAgent := "diagnostic user agent"
	usage := &UsageLog{UserAgent: &userAgent}
	compactUsageLogForStorage(&config.Config{}, usage)
	require.Same(t, &userAgent, usage.UserAgent)
}

func TestMinimalStoragePolicyDisablesContentModeration(t *testing.T) {
	svc := NewContentModerationService(nil, nil, nil, nil, nil, nil, nil)
	svc.SetMinimalStorage(true)
	cfg := defaultContentModerationConfig()
	cfg.Enabled = true
	cfg.Mode = ContentModerationModePreBlock
	cfg.RecordNonHits = true

	svc.applyStoragePolicy(cfg)
	require.False(t, cfg.Enabled)
	require.Equal(t, ContentModerationModeOff, cfg.Mode)
	require.False(t, cfg.RecordNonHits)
}
