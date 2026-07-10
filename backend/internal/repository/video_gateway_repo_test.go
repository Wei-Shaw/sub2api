package repository

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"regexp"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

func TestVideoGatewayRepositoryListRunnableTasksClaimsRows(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("new sqlmock: %v", err)
	}
	defer func() { _ = db.Close() }()

	now := time.Now().UTC()
	rows := sqlmock.NewRows([]string{
		"id", "provider_account_id", "provider", "model", "task_type", "prompt", "negative_prompt",
		"reference_image_url", "reference_video_url", "content_json", "has_video_input",
		"aspect_ratio", "duration", "resolution", "generate_audio", "watermark", "camera_fixed", "return_last_frame",
		"usage_total_tokens", "actual_resolution", "actual_duration", "last_frame_url",
		"status", "upstream_task_id", "result_url", "error_message", "cost_estimate", "poll_count",
		"local_asset_path", "local_asset_saved_at",
		"api_key_id", "creation_key", "creation_fingerprint", "reservation_id",
		"version", "dispatch_state", "settlement_status", "archive_status", "capture_status",
		"balance_charged_at", "worker_claimed_at", "worker_claimed_until",
		"created_by", "created_at", "updated_at", "completed_at", "display_name", "email", "username",
	}).AddRow(
		int64(42), int64(7), service.VideoProviderMock, "mock-video-v1", service.VideoTaskTypeTextToVideo,
		"claim me", "", "", "", []byte(`[]`), false, "16:9", 5, "720p", nil, nil, nil, nil,
		nil, nil, nil, nil, service.VideoStatusQueued, "", "", "", 0.0, 0, nil, nil,
		nil, nil, nil, nil, int64(1), service.VideoDispatchStatePending, service.VideoSettlementStatusPending, service.VideoSideEffectStatusPending, service.VideoSideEffectStatusPending,
		nil, now, now.Add(videoTaskClaimLeaseSeconds*time.Second),
		int64(9), now, now, nil,
		"Mock Provider", "user@example.test", "operator",
	)

	mock.ExpectQuery(`(?s)WITH candidate_ids AS .*FOR UPDATE SKIP LOCKED.*UPDATE video_tasks vt.*worker_claimed_until.*RETURNING vt\.\*.*FROM claimed vt`).
		WithArgs(2, videoTaskClaimLeaseSeconds).
		WillReturnRows(rows)

	repo := NewVideoGatewayRepository(db)
	tasks, err := repo.ListRunnableTasks(context.Background(), 2)
	if err != nil {
		t.Fatalf("list runnable tasks: %v", err)
	}
	if len(tasks) != 1 || tasks[0].ID != 42 {
		t.Fatalf("unexpected claimed tasks: %#v", tasks)
	}
	if tasks[0].WorkerClaimedAt == nil || tasks[0].WorkerClaimedUntil == nil {
		t.Fatalf("claimed task must expose authoritative claim timestamps: %#v", tasks[0])
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func TestVideoGatewayRepositoryListRunnableTasksExcludesDispatchUnknown(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("new sqlmock: %v", err)
	}
	defer func() { _ = db.Close() }()

	mock.ExpectQuery(`(?s)WITH candidate_ids AS .*WHERE .*dispatch_state.*unknown.*FOR UPDATE SKIP LOCKED`).
		WithArgs(5, videoTaskClaimLeaseSeconds).
		WillReturnRows(sqlmock.NewRows([]string{"id"}))

	repo := NewVideoGatewayRepository(db)
	tasks, err := repo.ListRunnableTasks(context.Background(), 5)
	if err != nil {
		t.Fatalf("list runnable tasks: %v", err)
	}
	if len(tasks) != 0 {
		t.Fatalf("unknown dispatch tasks must not be runnable: %#v", tasks)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func TestVideoGatewayRepositoryDispatchCASPersistsStateAndEventAtomically(t *testing.T) {
	for _, tc := range []struct {
		name       string
		task       *service.VideoTask
		eventType  string
		expectArgs []driver.Value
		updateRE   string
		invoke     func(*videoGatewayRepository, *service.VideoTask, *service.VideoTaskEvent) (bool, error)
	}{
		{
			name:       "pending to dispatching",
			task:       &service.VideoTask{ID: 41, Status: service.VideoStatusQueued, DispatchState: "pending", Version: 1},
			eventType:  "dispatching",
			expectArgs: []driver.Value{int64(41), int64(1), service.VideoStatusSubmitted, "dispatching"},
			updateRE:   `(?s)UPDATE video_tasks.*status = \$3.*dispatch_state = \$4.*version = version \+ 1.*WHERE id = \$1 AND version = \$2.*status = 'queued'.*dispatch_state = 'pending'.*RETURNING version, updated_at`,
			invoke: func(repo *videoGatewayRepository, task *service.VideoTask, event *service.VideoTaskEvent) (bool, error) {
				return repo.MarkDispatchingCAS(context.Background(), task, event)
			},
		},
		{
			name:       "dispatching to accepted",
			task:       &service.VideoTask{ID: 42, Status: service.VideoStatusSubmitted, UpstreamTaskID: "upstream-42", DispatchState: "dispatching", Version: 2},
			eventType:  "submitted",
			expectArgs: []driver.Value{int64(42), int64(2), service.VideoStatusSubmitted, "upstream-42", "accepted"},
			updateRE:   `(?s)UPDATE video_tasks.*status = \$3.*upstream_task_id = \$4.*dispatch_state = \$5.*version = version \+ 1.*worker_claimed_at = NULL.*worker_claimed_until = NULL.*WHERE id = \$1 AND version = \$2.*dispatch_state = 'dispatching'.*RETURNING version, updated_at`,
			invoke: func(repo *videoGatewayRepository, task *service.VideoTask, event *service.VideoTaskEvent) (bool, error) {
				return repo.MarkDispatchAcceptedCAS(context.Background(), task, event)
			},
		},
		{
			name:       "dispatching to unknown",
			task:       &service.VideoTask{ID: 43, Status: service.VideoStatusSubmitted, ErrorMessage: "provider outcome requires review", DispatchState: "dispatching", Version: 2},
			eventType:  "dispatch_unknown",
			expectArgs: []driver.Value{int64(43), int64(2), service.VideoStatusSubmitted, "provider outcome requires review", "unknown"},
			updateRE:   `(?s)UPDATE video_tasks.*status = \$3.*error_message = \$4.*dispatch_state = \$5.*version = version \+ 1.*worker_claimed_at = NULL.*worker_claimed_until = NULL.*WHERE id = \$1 AND version = \$2.*dispatch_state = 'dispatching'.*RETURNING version, updated_at`,
			invoke: func(repo *videoGatewayRepository, task *service.VideoTask, event *service.VideoTaskEvent) (bool, error) {
				return repo.MarkDispatchUnknownCAS(context.Background(), task, event)
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			if err != nil {
				t.Fatalf("new sqlmock: %v", err)
			}
			defer func() { _ = db.Close() }()

			now := time.Now().UTC()
			event := &service.VideoTaskEvent{VideoTaskID: tc.task.ID, EventType: tc.eventType, Message: "dispatch transition", Payload: map[string]any{"safe": true}}
			mock.ExpectBegin()
			expectUpdate := mock.ExpectQuery(tc.updateRE)
			expectUpdate.WithArgs(tc.expectArgs...)
			expectUpdate.WillReturnRows(sqlmock.NewRows([]string{"version", "updated_at"}).AddRow(tc.task.Version+1, now))
			mock.ExpectQuery(`(?s)INSERT INTO video_task_events .*VALUES \(\$1,\$2,\$3,\$4::jsonb\).*RETURNING id, created_at`).
				WithArgs(tc.task.ID, event.EventType, event.Message, `{"safe":true}`).
				WillReturnRows(sqlmock.NewRows([]string{"id", "created_at"}).AddRow(int64(99), now))
			mock.ExpectCommit()

			repo, ok := NewVideoGatewayRepository(db).(*videoGatewayRepository)
			if !ok {
				t.Fatal("video gateway repository has unexpected implementation")
			}
			applied, err := tc.invoke(repo, tc.task, event)
			if err != nil {
				t.Fatalf("dispatch CAS: %v", err)
			}
			if !applied || tc.task.Version < 2 || event.ID != 99 {
				t.Fatalf("transition not applied atomically: applied=%t task=%#v event=%#v", applied, tc.task, event)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatalf("unmet sql expectations: %v", err)
			}
		})
	}
}

func TestVideoGatewayRepositoryDispatchCASDoesNotOverwriteCancelledTaskOrAppendEvent(t *testing.T) {
	for _, tc := range []struct {
		name   string
		task   *service.VideoTask
		event  *service.VideoTaskEvent
		args   []driver.Value
		invoke func(*videoGatewayRepository, *service.VideoTask, *service.VideoTaskEvent) (bool, error)
	}{
		{
			name:  "accepted loses to cancel",
			task:  &service.VideoTask{ID: 42, Status: service.VideoStatusSubmitted, UpstreamTaskID: "stale-upstream", DispatchState: service.VideoDispatchStateDispatching, Version: 2},
			event: &service.VideoTaskEvent{VideoTaskID: 42, EventType: "submitted", Message: "must not append"},
			args:  []driver.Value{int64(42), int64(2), service.VideoStatusSubmitted, "stale-upstream", service.VideoDispatchStateAccepted},
			invoke: func(repo *videoGatewayRepository, task *service.VideoTask, event *service.VideoTaskEvent) (bool, error) {
				return repo.MarkDispatchAcceptedCAS(context.Background(), task, event)
			},
		},
		{
			name:  "unknown loses to cancel",
			task:  &service.VideoTask{ID: 43, Status: service.VideoStatusSubmitted, ErrorMessage: "ambiguous", DispatchState: service.VideoDispatchStateDispatching, Version: 2},
			event: &service.VideoTaskEvent{VideoTaskID: 43, EventType: "dispatch_unknown", Message: "must not append"},
			args:  []driver.Value{int64(43), int64(2), service.VideoStatusSubmitted, "ambiguous", service.VideoDispatchStateUnknown},
			invoke: func(repo *videoGatewayRepository, task *service.VideoTask, event *service.VideoTaskEvent) (bool, error) {
				return repo.MarkDispatchUnknownCAS(context.Background(), task, event)
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			if err != nil {
				t.Fatalf("new sqlmock: %v", err)
			}
			defer func() { _ = db.Close() }()

			mock.ExpectBegin()
			mock.ExpectQuery(`(?s)UPDATE video_tasks.*WHERE id = \$1 AND version = \$2.*status = 'submitted'.*dispatch_state = 'dispatching'`).
				WithArgs(tc.args...).
				WillReturnError(sql.ErrNoRows)
			mock.ExpectRollback()

			repo, ok := NewVideoGatewayRepository(db).(*videoGatewayRepository)
			if !ok {
				t.Fatal("video gateway repository has unexpected implementation")
			}
			applied, err := tc.invoke(repo, tc.task, tc.event)
			if err != nil {
				t.Fatalf("CAS conflict: %v", err)
			}
			if applied {
				t.Fatal("cancelled task must win the CAS race")
			}
			if tc.task.Version != 2 || tc.event.ID != 0 {
				t.Fatalf("terminal race mutated caller values: task=%#v event=%#v", tc.task, tc.event)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatalf("terminal race appended an event or missed rollback: %v", err)
			}
		})
	}
}

func TestVideoGatewayRepositoryInsertUsageLogIsIdempotent(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("new sqlmock: %v", err)
	}
	defer func() { _ = db.Close() }()

	repo := NewVideoGatewayRepository(db)
	task := &service.VideoTask{
		ID:             42,
		Provider:       service.VideoProviderSeedance,
		Model:          "seedance-2-0-pro",
		Status:         service.VideoStatusSucceeded,
		CostEstimate:   0.12,
		Duration:       3,
		Currency:       service.BillingCurrencyCNY,
		PricingSource:  service.PricingSourceProviderUsage,
		PricingVersion: service.VideoPricingVersionSeedance202603,
	}

	mock.ExpectExec(regexp.QuoteMeta(insertVideoUsageLogSQL)).
		WithArgs(task.ID, task.Provider, task.Model, task.Status, task.CostEstimate, task.Duration, task.Currency, task.PricingSource, task.PricingVersion).
		WillReturnResult(sqlmock.NewResult(0, 1))

	if err := repo.InsertUsageLog(context.Background(), task); err != nil {
		t.Fatalf("insert usage log: %v", err)
	}

	mock.ExpectExec(regexp.QuoteMeta(insertVideoUsageLogSQL)).
		WithArgs(task.ID, task.Provider, task.Model, task.Status, task.CostEstimate, task.Duration, task.Currency, task.PricingSource, task.PricingVersion).
		WillReturnResult(sqlmock.NewResult(0, 0))

	if err := repo.InsertUsageLog(context.Background(), task); err != nil {
		t.Fatalf("second insert usage log should be idempotent: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func TestVideoGatewayRepositoryClaimVideoBalanceCharge(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("new sqlmock: %v", err)
	}
	defer func() { _ = db.Close() }()

	repo := NewVideoGatewayRepository(db)

	claimedAt := time.Date(2026, 7, 6, 8, 0, 0, 0, time.UTC)
	mock.ExpectQuery(`(?s)UPDATE video_tasks.*balance_charged_at = NOW\(\).*WHERE id = \$1 AND balance_charged_at IS NULL.*RETURNING balance_charged_at`).
		WithArgs(int64(42)).
		WillReturnRows(sqlmock.NewRows([]string{"balance_charged_at"}).AddRow(claimedAt))
	gotClaimedAt, claimed, err := repo.ClaimVideoBalanceCharge(context.Background(), 42)
	if err != nil {
		t.Fatalf("claim balance charge: %v", err)
	}
	if !claimed {
		t.Fatalf("expected first claim to succeed")
	}
	if !gotClaimedAt.Equal(claimedAt) {
		t.Fatalf("claimed_at = %s, want %s", gotClaimedAt, claimedAt)
	}

	mock.ExpectQuery(`(?s)UPDATE video_tasks.*balance_charged_at = NOW\(\).*WHERE id = \$1 AND balance_charged_at IS NULL.*RETURNING balance_charged_at`).
		WithArgs(int64(42)).
		WillReturnError(sql.ErrNoRows)
	gotClaimedAt, claimed, err = repo.ClaimVideoBalanceCharge(context.Background(), 42)
	if err != nil {
		t.Fatalf("second claim balance charge: %v", err)
	}
	if claimed {
		t.Fatalf("expected second claim to be idempotent")
	}
	if !gotClaimedAt.IsZero() {
		t.Fatalf("unclaimed task should return zero claim time, got %s", gotClaimedAt)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func TestVideoGatewayRepositoryClearVideoBalanceChargeIfClaimedAt(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("new sqlmock: %v", err)
	}
	defer func() { _ = db.Close() }()

	repo := NewVideoGatewayRepository(db)
	claimedAt := time.Date(2026, 7, 6, 8, 0, 0, 0, time.UTC)

	mock.ExpectExec(`(?s)UPDATE video_tasks.*balance_charged_at = NULL.*WHERE id = \$1 AND balance_charged_at = \$2`).
		WithArgs(int64(42), claimedAt).
		WillReturnResult(sqlmock.NewResult(0, 1))
	cleared, err := repo.ClearVideoBalanceChargeIfClaimedAt(context.Background(), 42, claimedAt)
	if err != nil {
		t.Fatalf("clear balance charge: %v", err)
	}
	if !cleared {
		t.Fatalf("expected compare-clear to affect the matching claim")
	}

	mock.ExpectExec(`(?s)UPDATE video_tasks.*balance_charged_at = NULL.*WHERE id = \$1 AND balance_charged_at = \$2`).
		WithArgs(int64(42), claimedAt).
		WillReturnResult(sqlmock.NewResult(0, 0))
	cleared, err = repo.ClearVideoBalanceChargeIfClaimedAt(context.Background(), 42, claimedAt)
	if err != nil {
		t.Fatalf("second clear balance charge: %v", err)
	}
	if cleared {
		t.Fatalf("expected stale claim timestamp not to clear")
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func TestVideoGatewayRepositoryListUnchargedSucceededVideoTasks(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("new sqlmock: %v", err)
	}
	defer func() { _ = db.Close() }()

	now := time.Now().UTC()
	rows := sqlmock.NewRows([]string{
		"id", "provider_account_id", "provider", "model", "task_type", "prompt", "negative_prompt",
		"reference_image_url", "reference_video_url", "content_json", "has_video_input",
		"aspect_ratio", "duration", "resolution", "generate_audio", "watermark", "camera_fixed", "return_last_frame",
		"usage_total_tokens", "actual_resolution", "actual_duration", "last_frame_url",
		"status", "upstream_task_id", "result_url", "error_message", "cost_estimate", "poll_count",
		"local_asset_path", "local_asset_saved_at",
		"created_by", "created_at", "updated_at", "completed_at", "display_name", "email", "username",
	}).AddRow(
		int64(42), int64(7), service.VideoProviderSeedance, "doubao-seedance-2-0-260128", service.VideoTaskTypeTextToVideo,
		"charge me", "", "", "", []byte(`[]`), false, "16:9", 5, "720p", nil, nil, nil, nil,
		int64(102960), "720p", nil, nil, service.VideoStatusSucceeded, "upstream-42", "https://result.example/video.mp4", "", 4.73616, 2, nil, nil, int64(9), now, now, now,
		"Seedance", "user@example.test", "operator",
	)

	mock.ExpectQuery(`(?s)WHERE vt\.status = 'succeeded'.*vt\.balance_charged_at IS NULL.*ORDER BY vt\.completed_at ASC NULLS LAST, vt\.updated_at ASC, vt\.id ASC`).
		WithArgs(3).
		WillReturnRows(rows)

	repo := NewVideoGatewayRepository(db)
	tasks, err := repo.ListUnchargedSucceededVideoTasks(context.Background(), 3)
	if err != nil {
		t.Fatalf("list uncharged succeeded video tasks: %v", err)
	}
	if len(tasks) != 1 || tasks[0].ID != 42 || tasks[0].Status != service.VideoStatusSucceeded {
		t.Fatalf("unexpected uncharged tasks: %#v", tasks)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func TestVideoGatewayRepositoryUpdateTaskPersistsPollResponseDetails(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("new sqlmock: %v", err)
	}
	defer func() { _ = db.Close() }()

	repo := NewVideoGatewayRepository(db)
	now := time.Now().UTC()
	completedAt := now.Add(time.Second)
	tokens := int64(654321)
	actualDuration := 12
	task := &service.VideoTask{
		ID:               42,
		Status:           service.VideoStatusSucceeded,
		UpstreamTaskID:   "seedance-task-42",
		ResultURL:        "https://ark-content.cn-beijing.volces.com/v/ok.mp4",
		ErrorMessage:     "",
		CostEstimate:     0.12,
		CompletedAt:      &completedAt,
		PollCount:        3,
		UsageTotalTokens: &tokens,
		ActualResolution: "1080p",
		ActualDuration:   &actualDuration,
		LastFrameURL:     "https://ark-content.cn-beijing.volces.com/i/last.png",
	}

	mock.ExpectQuery(`(?s)UPDATE video_tasks.*usage_total_tokens = \$9.*actual_resolution = \$10.*actual_duration = \$11.*last_frame_url = \$12`).
		WithArgs(
			task.ID,
			task.Status,
			task.UpstreamTaskID,
			task.ResultURL,
			task.ErrorMessage,
			task.CostEstimate,
			completedAt,
			task.PollCount,
			tokens,
			task.ActualResolution,
			actualDuration,
			task.LastFrameURL,
		).
		WillReturnRows(sqlmock.NewRows([]string{"updated_at"}).AddRow(now))

	if err := repo.UpdateTask(context.Background(), task); err != nil {
		t.Fatalf("update task: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func TestVideoGatewayRepositoryCreateDailyTrialTaskRejectsExistingReservation(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("new sqlmock: %v", err)
	}
	defer func() { _ = db.Close() }()

	trialDate := time.Date(2026, 6, 28, 0, 0, 0, 0, time.Local)

	mock.ExpectBegin()
	mock.ExpectQuery(`(?s)INSERT INTO video_daily_trial_reservations .*ON CONFLICT \(provider, created_by, trial_date\) DO NOTHING.*RETURNING id`).
		WithArgs(service.VideoProviderSeedance, int64(7), trialDate).
		WillReturnError(sql.ErrNoRows)
	mock.ExpectRollback()

	repo := NewVideoGatewayRepository(db)
	reserved, err := repo.CreateDailyTrialTask(context.Background(), &service.VideoTask{}, service.VideoProviderSeedance, 7, trialDate)
	if err != nil {
		t.Fatalf("create daily trial task: %v", err)
	}
	if reserved {
		t.Fatalf("expected existing reservation to reject task creation")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func TestVideoGatewayRepositoryCreateDailyTrialTaskCreatesTaskInReservationTransaction(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("new sqlmock: %v", err)
	}
	defer func() { _ = db.Close() }()

	now := time.Now().UTC()
	trialDate := time.Date(2026, 6, 28, 0, 0, 0, 0, time.Local)
	generateAudio := false
	task := &service.VideoTask{
		ProviderAccountID: 3,
		Provider:          service.VideoProviderSeedance,
		Model:             "seedance-lite-test",
		TaskType:          service.VideoTaskTypeTextToVideo,
		Prompt:            "tiny real trial",
		Content: []service.VideoTaskContentItem{
			{Type: service.VideoContentTypeVideoURL, Role: service.VideoContentRoleReferenceVideo, URL: "https://assets.example.com/ref.mp4"},
		},
		HasVideoInput: true,
		AspectRatio:   "16:9",
		Duration:      3,
		Resolution:    "720p",
		GenerateAudio: &generateAudio,
		Status:        service.VideoStatusQueued,
		CreatedBy:     7,
	}

	mock.ExpectBegin()
	mock.ExpectQuery(`(?s)INSERT INTO video_daily_trial_reservations .*ON CONFLICT \(provider, created_by, trial_date\) DO NOTHING.*RETURNING id`).
		WithArgs(service.VideoProviderSeedance, int64(7), trialDate).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(88)))
	mock.ExpectQuery(regexp.QuoteMeta(createVideoTaskSQL)).
		WithArgs(
			task.ProviderAccountID,
			task.Provider,
			task.Model,
			task.TaskType,
			task.Prompt,
			task.NegativePrompt,
			task.ReferenceImageURL,
			task.ReferenceVideoURL,
			`[{"type":"video_url","role":"reference_video","url":"https://assets.example.com/ref.mp4"}]`,
			true,
			task.AspectRatio,
			task.Duration,
			task.Resolution,
			false,
			nil,
			nil,
			nil,
			task.Status,
			task.CreatedBy,
		).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at"}).AddRow(int64(42), now, now))
	mock.ExpectExec(`(?s)UPDATE video_daily_trial_reservations.*SET video_task_id = \$2.*WHERE id = \$1`).
		WithArgs(int64(88), int64(42)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	repo := NewVideoGatewayRepository(db)
	reserved, err := repo.CreateDailyTrialTask(context.Background(), task, service.VideoProviderSeedance, 7, trialDate)
	if err != nil {
		t.Fatalf("create daily trial task: %v", err)
	}
	if !reserved {
		t.Fatalf("expected reservation to create task")
	}
	if task.ID != 42 {
		t.Fatalf("expected task id 42, got %d", task.ID)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}
