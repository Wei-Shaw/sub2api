package repository

import (
	"context"
	"database/sql/driver"
	"regexp"
	"strings"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestContentModerationRepositoryEngineMetaInsert(t *testing.T) {
	for _, meta := range []*service.ContentModerationEngineMeta{nil, {Engine: "typesafe", Model: "jev-fixed", RulesVersion: "rules-v1", SkippedImages: 1}} {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		args := make([]driver.Value, 26)
		for i := range args {
			args[i] = sqlmock.AnyArg()
		}
		if meta == nil {
			args[25] = nil
		} else {
			args[25] = `{"engine":"typesafe","model":"jev-fixed","rules_version":"rules-v1","skipped_images":1}`
		}
		mock.ExpectQuery("INSERT INTO content_moderation_logs").WithArgs(args...).WillReturnRows(sqlmock.NewRows([]string{"id", "created_at"}).AddRow(1, time.Now()))
		err = NewContentModerationRepository(db).CreateLog(context.Background(), &service.ContentModerationLog{EngineMeta: meta})
		require.NoError(t, err)
		require.NoError(t, mock.ExpectationsWereMet())
		mock.ExpectClose()
		require.NoError(t, db.Close())
	}
}

func TestContentModerationRepositoryEngineMetaRead(t *testing.T) {
	for _, meta := range []any{nil, `{"engine":"typesafe","model":"jev-fixed","rules_version":"rules-v1","skipped_images":1}`} {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		mock.ExpectQuery("SELECT COUNT").WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
		columns := []string{"id", "request_id", "user_id", "user_email", "api_key_id", "api_key_name", "group_id", "group_name", "endpoint", "provider", "model", "mode", "action", "flagged", "highest_category", "highest_score", "category_scores", "threshold_snapshot", "input_excerpt", "upstream_latency_ms", "error", "violation_count", "auto_banned", "email_sent", "status", "queue_delay_ms", "matched_keyword", "created_at", "engine_meta"}
		mock.ExpectQuery("SELECT[\\s\\S]*l.engine_meta").WillReturnRows(sqlmock.NewRows(columns).AddRow(1, "req", nil, "", nil, "", nil, "", "/v1/responses", "business", "gpt-model", "observe", "allow", false, "sexual", 0.1, `{"sexual":0.1}`, `{"sexual":0.8}`, "sample", 10, "", 0, false, false, "active", nil, "", time.Now(), meta))
		logs, _, err := NewContentModerationRepository(db).ListLogs(context.Background(), service.ContentModerationLogFilter{})
		require.NoError(t, err)
		require.Len(t, logs, 1)
		require.Equal(t, "gpt-model", logs[0].Model)
		if meta == nil {
			require.Nil(t, logs[0].EngineMeta)
		} else {
			require.Equal(t, "jev-fixed", logs[0].EngineMeta.Model)
			require.Equal(t, 1, logs[0].EngineMeta.SkippedImages)
		}
		require.NoError(t, mock.ExpectationsWereMet())
		mock.ExpectClose()
		require.NoError(t, db.Close())
	}
}

func TestBuildContentModerationLogWhere_BlockedIncludesAllBlockActions(t *testing.T) {
	where, args := buildContentModerationLogWhere(service.ContentModerationLogFilter{Result: "blocked"})

	require.Empty(t, args)
	sql := strings.Join(where, " AND ")
	require.Contains(t, sql, "l.action IN ('block', 'keyword_block', 'hash_block')")
	require.NotContains(t, sql, "l.action = 'block'")
}

func TestContentModerationRepositoryCountFlaggedByUserSince_ExcludesHashBlock(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	repo := NewContentModerationRepository(db)
	since := time.Now().Add(-time.Hour)
	mock.ExpectQuery(regexp.QuoteMeta("AND action <> 'hash_block'")).
		WithArgs(int64(1001), since, false).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(2))

	count, err := repo.CountFlaggedByUserSince(context.Background(), 1001, since, false)

	require.NoError(t, err)
	require.Equal(t, 2, count)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestContentModerationRepositoryCountFlaggedByUserSince_ExcludesCyberPolicyWhenRequested(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	repo := NewContentModerationRepository(db)
	since := time.Now().Add(-time.Hour)
	mock.ExpectQuery(regexp.QuoteMeta("AND ($3::bool IS FALSE OR action <> 'cyber_policy')")).
		WithArgs(int64(1001), since, true).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(3))

	count, err := repo.CountFlaggedByUserSince(context.Background(), 1001, since, true)

	require.NoError(t, err)
	require.Equal(t, 3, count)
	require.NoError(t, mock.ExpectationsWereMet())
}
