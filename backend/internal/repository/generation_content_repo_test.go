package repository

import (
	"context"
	"database/sql"
	"regexp"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

func TestGenerationContentRepositoryCreateVideoTaskContentUsesTaskIDConflict(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("new sqlmock: %v", err)
	}
	defer func() { _ = db.Close() }()

	userID := int64(7)
	taskID := int64(99)
	content := &service.GenerationContent{
		UserID:            &userID,
		TaskID:            &taskID,
		Model:             "mock-video-v1",
		PromptRedacted:    `{"prompt":"safe"}`,
		ResponseRedacted:  `{"metadata_summary":true}`,
		PromptBytes:       17,
		ResponseBytes:     25,
		ResponseTruncated: false,
		RedactionVersion:  2,
	}
	now := time.Now().UTC()

	mock.ExpectQuery(`(?s)INSERT INTO ai_generation_content .*task_id.*ON CONFLICT \(task_id\) WHERE task_id IS NOT NULL DO NOTHING.*RETURNING id, created_at`).
		WithArgs(
			content.RequestID,
			nil,
			userID,
			nil,
			nil,
			taskID,
			content.Model,
			content.RequestPayloadHash,
			content.PromptRedacted,
			content.ResponseRedacted,
			content.PromptBytes,
			content.ResponseBytes,
			content.ResponseTruncated,
			content.RedactionVersion,
		).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at"}).AddRow(int64(123), now))

	repo := NewGenerationContentRepository(db)
	if err := repo.CreateVideoTaskContent(context.Background(), content); err != nil {
		t.Fatalf("create video task content: %v", err)
	}
	if content.ID != 123 || !content.CreatedAt.Equal(now) {
		t.Fatalf("expected returned id/created_at, got id=%d created_at=%s", content.ID, content.CreatedAt)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func TestGenerationContentRepositoryCreateVideoTaskContentDuplicateIsNoOp(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("new sqlmock: %v", err)
	}
	defer func() { _ = db.Close() }()

	taskID := int64(99)
	content := &service.GenerationContent{TaskID: &taskID, Model: "mock-video-v1"}

	mock.ExpectQuery(regexp.QuoteMeta(`
INSERT INTO ai_generation_content (
    request_id, api_key_id, user_id, group_id, account_id, task_id, model, request_payload_hash,
    prompt_redacted, response_redacted, prompt_bytes, response_bytes, response_truncated, redaction_version
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8,
    $9, $10, $11, $12, $13, $14
)
ON CONFLICT (task_id) WHERE task_id IS NOT NULL DO NOTHING
RETURNING id, created_at`)).
		WillReturnError(sql.ErrNoRows)

	repo := NewGenerationContentRepository(db)
	if err := repo.CreateVideoTaskContent(context.Background(), content); err != nil {
		t.Fatalf("duplicate video task content should be no-op: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func TestGenerationContentRepositoryUpdateVideoTaskAdoptionUsesTaskID(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("new sqlmock: %v", err)
	}
	defer func() { _ = db.Close() }()

	score := 0.875
	input := service.GenerationContentAdoptionInput{
		TaskID:         42,
		AdoptionStatus: "adopted",
		QualityScore:   &score,
		Notes:          "picked for episode cut",
	}

	mock.ExpectQuery(`(?s)UPDATE ai_generation_content.*adoption_status.*quality_score.*adoption_notes.*WHERE task_id = \$1.*RETURNING task_id, adoption_status, quality_score, adoption_notes`).
		WithArgs(input.TaskID, input.AdoptionStatus, input.QualityScore, input.Notes).
		WillReturnRows(sqlmock.NewRows([]string{"task_id", "adoption_status", "quality_score", "adoption_notes"}).
			AddRow(input.TaskID, input.AdoptionStatus, score, input.Notes))

	repo := NewGenerationContentRepository(db)
	got, err := repo.UpdateVideoTaskAdoption(context.Background(), input)
	if err != nil {
		t.Fatalf("update adoption: %v", err)
	}
	if got == nil || !got.Saved || got.TaskID != input.TaskID || got.AdoptionStatus != input.AdoptionStatus || got.QualityScore == nil || *got.QualityScore != score || got.Notes != input.Notes {
		t.Fatalf("unexpected adoption result: %+v", got)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func TestGenerationContentRepositoryWeeklyReportAggregatesLedgerSignals(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("new sqlmock: %v", err)
	}
	defer func() { _ = db.Close() }()

	start := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	end := start.AddDate(0, 0, 7)

	mock.ExpectQuery(`(?s)FROM ai_generation_content c.*LEFT JOIN video_tasks vt ON vt\.id = c\.task_id.*WHERE c\.created_at >= \$1 AND c\.created_at < \$2`).
		WithArgs(start, end).
		WillReturnRows(sqlmock.NewRows([]string{
			"entries",
			"video_tasks",
			"total_cost_estimate",
			"adopted_count",
			"rejected_count",
			"pending_count",
			"unreviewed_count",
			"failed_task_count",
			"missing_task_join_count",
			"truncated_count",
		}).AddRow(int64(10), int64(4), 1.25, int64(3), int64(1), int64(2), int64(4), int64(1), int64(1), int64(2)))

	repo := NewGenerationContentRepository(db)
	got, err := repo.GetWeeklyReport(context.Background(), start, end)
	if err != nil {
		t.Fatalf("weekly report: %v", err)
	}
	if got == nil || got.Entries != 10 || got.VideoTasks != 4 || got.TotalCostEstimate != 1.25 || got.AdoptionRate != 0.3 {
		t.Fatalf("unexpected weekly report: %+v", got)
	}
	if got.Anomalies.FailedTasks != 1 || got.Anomalies.MissingTaskJoins != 1 || got.Anomalies.TruncatedRows != 2 {
		t.Fatalf("unexpected anomalies: %+v", got.Anomalies)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func TestGenerationContentRepositoryWeeklyReportNormalizesMixedCurrencyCosts(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("new sqlmock: %v", err)
	}
	defer func() { _ = db.Close() }()

	start := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	end := start.AddDate(0, 0, 7)

	mock.ExpectQuery(`(?s)settings.*usd_cny_rate.*CASE.*currency.*CNY.*cost_estimate.*NULLIF.*usd_cny_rate.*LEFT JOIN video_tasks vt ON vt\.id = c\.task_id.*WHERE c\.created_at >= \$1 AND c\.created_at < \$2`).
		WithArgs(start, end).
		WillReturnRows(sqlmock.NewRows([]string{
			"entries",
			"video_tasks",
			"total_cost_estimate",
			"adopted_count",
			"rejected_count",
			"pending_count",
			"unreviewed_count",
			"failed_task_count",
			"missing_task_join_count",
			"truncated_count",
		}).AddRow(int64(2), int64(2), 2.0, int64(1), int64(0), int64(0), int64(1), int64(0), int64(0), int64(0)))

	repo := NewGenerationContentRepository(db)
	got, err := repo.GetWeeklyReport(context.Background(), start, end)
	if err != nil {
		t.Fatalf("weekly report: %v", err)
	}
	if got == nil || got.TotalCostEstimate != 2.0 {
		t.Fatalf("expected USD-normalized total 2.0, got %+v", got)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func TestGenerationContentRepositoryGetRecentReturnsCurrency(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("new sqlmock: %v", err)
	}
	defer func() { _ = db.Close() }()

	taskID := int64(77)
	createdAt := time.Date(2026, 7, 6, 8, 0, 0, 0, time.UTC)

	mock.ExpectQuery(`(?s)COALESCE\(NULLIF\(vul\.currency, ''\), NULLIF\(vt\.currency, ''\), 'USD'\).*LEFT JOIN video_tasks vt ON vt\.id = c\.task_id.*LEFT JOIN video_usage_logs vul ON vul\.video_task_id = c\.task_id.*ORDER BY c\.created_at DESC`).
		WithArgs(20).
		WillReturnRows(sqlmock.NewRows([]string{
			"task_id",
			"model",
			"created_at",
			"prompt_redacted",
			"response_redacted",
			"prompt_bytes",
			"response_bytes",
			"response_truncated",
			"username",
			"email",
			"group_name",
			"adoption_status",
			"quality_score",
			"adoption_notes",
			"video_status",
			"cost_estimate",
			"currency",
		}).AddRow(taskID, "seedance-v1", createdAt, "p", "r", int64(1), int64(2), false, "operator", "op@example.com", "team", "pending", nil, "", "succeeded", 5.0094, "CNY"))

	repo := NewGenerationContentRepository(db)
	got, err := repo.GetRecent(context.Background(), 20)
	if err != nil {
		t.Fatalf("get recent: %v", err)
	}
	if len(got) != 1 || got[0].TaskID == nil || *got[0].TaskID != taskID || got[0].Currency != "CNY" {
		t.Fatalf("unexpected recent rows: %+v", got)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}
