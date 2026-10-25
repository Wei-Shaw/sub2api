package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func stepPlanUsageRepositoryAccount() *service.Account {
	return &service.Account{
		ID:       41,
		Platform: service.PlatformStepfun,
		Type:     service.AccountTypeAPIKey,
		Credentials: map[string]any{
			"account_mode": service.AccountModeCoding,
			"api_key":      "sk-stepfun",
			"base_url":     service.DefaultStepFunCodingBaseURL,
		},
		Extra: map[string]any{
			service.StepFunUsageAutoRefreshExtraKey: true,
			service.StepFunUsageSnapshotExtraKey:    map[string]any{"status": "forged"},
			"stepfun_monthly_used_percent":          99.0,
			"custom":                                "keep",
		},
	}
}

func stepPlanTestManagedExtraKeys() []string {
	return []string{
		service.StepFunUsageAutoRefreshExtraKey,
		service.StepFunUsageSnapshotExtraKey,
		service.StepFunUsageAttemptSeqExtraKey,
		"stepfun_5h_used_percent",
		"stepfun_5h_reset_at",
		"stepfun_weekly_used_percent",
		"stepfun_weekly_reset_at",
		"stepfun_monthly_used_percent",
		"stepfun_monthly_reset_at",
		"stepfun_usage_updated_at",
	}
}

func stepPlanMergeMockColumns() []string {
	return openCodeGoMergeMockColumns()
}

func TestStepFunCodingBaseURLRegexMatchesOfficialStepPlanBoundary(t *testing.T) {
	matcher := regexp.MustCompile(stepFunCodingBaseURLRegexSQL)
	for _, baseURL := range []string{
		"https://api.stepfun.com/step_plan",
		"https://api.stepfun.com/step_plan/",
		"https://api.stepfun.com/step_plan/v1",
		"https://api.stepfun.com/step_plan/v1/",
		"https://api.stepfun.com:443/step_plan/v1",
		"https://api.stepfun.ai/step_plan",
		"https://API.STEPFUN.AI/step_plan/v1",
	} {
		require.True(t, matcher.MatchString(baseURL), baseURL)
	}
	for _, baseURL := range []string{
		"http://api.stepfun.com/step_plan/v1",
		"https://api.stepfun.com/v1",
		"https://api.stepfun.com/step_plan/v2",
		"https://api.stepfun.com/step_plan-other",
		"https://api.stepfun.com.evil.example/step_plan/v1",
		"https://relay.example.com/step_plan/v1",
		"https://user@api.stepfun.com/step_plan/v1",
		"https://api.stepfun.com/step_plan/v1?target=evil",
		"https://api.stepfun.com/step_plan/v1#evil",
		"https://api.stepfun.com/STEP_PLAN/v1",
	} {
		require.False(t, matcher.MatchString(baseURL), baseURL)
	}
}

func TestLockAndMergeAccountProbeExtraPreservesStepPlanManagedState(t *testing.T) {
	databaseManagedExtra := `{"stepfun_usage_auto_refresh":false,"stepfun_usage_snapshot":{"status":"ok","plan_level":"Plus"},"stepfun_usage_attempt_seq":7,"stepfun_monthly_used_percent":42}`
	tests := []struct {
		name              string
		account           *service.Account
		groupUnchanged    bool
		wantManaged       bool
		wantAutoRefresh   any
		wantMonthlyUsed   any
		wantSnapshotState string
	}{
		{
			name:              "identity unchanged restores latest database state over forged input",
			account:           stepPlanUsageRepositoryAccount(),
			groupUnchanged:    true,
			wantManaged:       true,
			wantAutoRefresh:   false,
			wantMonthlyUsed:   float64(42),
			wantSnapshotState: "ok",
		},
		{
			name:           "identity changed clears stale and forged managed state",
			account:        stepPlanUsageRepositoryAccount(),
			groupUnchanged: false,
			wantManaged:    false,
		},
		{
			name: "custom relay is not eligible and cannot inherit managed state",
			account: func() *service.Account {
				account := stepPlanUsageRepositoryAccount()
				account.Credentials["base_url"] = "https://relay.example.com/step_plan/v1"
				return account
			}(),
			groupUnchanged: false,
			wantManaged:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, mock := newOllamaCloudUsageRepositoryTestClient(t)
			credentials, err := json.Marshal(normalizeJSONMap(tt.account.Credentials))
			require.NoError(t, err)
			mock.ExpectQuery(`(?s)`+regexp.QuoteMeta("SELECT")+`.*`+regexp.QuoteMeta("FOR NO KEY UPDATE")).
				WithArgs(tt.account.ID, tt.account.Platform, tt.account.Type, string(credentials), nil).
				WillReturnRows(sqlmock.NewRows(stepPlanMergeMockColumns()).AddRow(
					false, false, true,
					nil, nil, nil,
					nil, nil, nil,
					false, nil, nil,
					tt.groupUnchanged, databaseManagedExtra,
				))

			got, err := lockAndMergeAccountProbeExtra(context.Background(), client, tt.account, nil, nil)
			require.NoError(t, err)
			require.Equal(t, "keep", got["custom"])
			if tt.wantManaged {
				require.Equal(t, tt.wantAutoRefresh, got[service.StepFunUsageAutoRefreshExtraKey])
				require.Equal(t, tt.wantMonthlyUsed, got["stepfun_monthly_used_percent"])
				snapshot, ok := got[service.StepFunUsageSnapshotExtraKey].(map[string]any)
				require.True(t, ok)
				require.Equal(t, tt.wantSnapshotState, snapshot["status"])
			} else {
				for _, key := range stepPlanTestManagedExtraKeys() {
					require.NotContains(t, got, key)
				}
			}
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestBulkUpdateStepPlanIdentityChangeClearsManagedExtraAndDropsForgedValues(t *testing.T) {
	exec := &recordingSQLExecutor{result: rowsAffectedResult(1)}
	repo := newAccountRepositoryWithSQL(nil, exec, nil)

	_, err := repo.BulkUpdate(context.Background(), []int64{41}, service.AccountBulkUpdate{
		Credentials: map[string]any{"api_key": "sk-stepfun-new"},
		Extra: map[string]any{
			service.StepFunUsageAutoRefreshExtraKey: true,
			service.StepFunUsageSnapshotExtraKey:    map[string]any{"status": "forged"},
			"stepfun_monthly_used_percent":          99.0,
			"custom":                                "keep",
		},
	})

	require.NoError(t, err)
	require.NotEmpty(t, exec.execQueries)
	query := normalizeSQLWhitespace(exec.execQueries[0])
	require.Contains(t, query, "platform = 'stepfun'")
	require.Contains(t, query, "credentials ->> 'account_mode'")
	require.Contains(t, query, "credentials ->> 'base_url'")
	require.Contains(t, query, "- 'stepfun_usage_auto_refresh'")
	require.Contains(t, query, "- 'stepfun_usage_snapshot'")
	require.Contains(t, query, "- 'stepfun_monthly_used_percent'")

	payloads := make([]string, 0, len(exec.execArgs[0]))
	for _, arg := range exec.execArgs[0] {
		if raw, ok := arg.([]byte); ok {
			payloads = append(payloads, string(raw))
			continue
		}
		payloads = append(payloads, fmt.Sprint(arg))
	}
	joinedPayloads := strings.Join(payloads, "\n")
	require.NotContains(t, joinedPayloads, service.StepFunUsageAutoRefreshExtraKey)
	require.NotContains(t, joinedPayloads, service.StepFunUsageSnapshotExtraKey)
	require.NotContains(t, joinedPayloads, "stepfun_monthly_used_percent")
	require.Contains(t, joinedPayloads, "custom")
}

func TestBulkUpdateStepPlanUnchangedIdentityDoesNotMergeForgedManagedExtra(t *testing.T) {
	exec := &recordingSQLExecutor{result: rowsAffectedResult(1)}
	repo := newAccountRepositoryWithSQL(nil, exec, nil)

	_, err := repo.BulkUpdate(context.Background(), []int64{41}, service.AccountBulkUpdate{
		Credentials: map[string]any{"api_key": "sk-stepfun"},
		Extra: map[string]any{
			service.StepFunUsageAutoRefreshExtraKey: true,
			service.StepFunUsageSnapshotExtraKey:    map[string]any{"status": "forged"},
			"custom":                                "keep",
		},
	})

	require.NoError(t, err)
	require.NotEmpty(t, exec.execQueries)
	for _, arg := range exec.execArgs[0] {
		payload := fmt.Sprint(arg)
		if raw, ok := arg.([]byte); ok {
			payload = string(raw)
		}
		require.NotContains(t, payload, service.StepFunUsageAutoRefreshExtraKey)
		require.NotContains(t, payload, service.StepFunUsageSnapshotExtraKey)
	}
}

func TestUpdateCredentialsStepPlanIdentityChangeClearsManagedExtra(t *testing.T) {
	client, mock := newOllamaCloudUsageRepositoryTestClient(t)
	credentials := map[string]any{
		"account_mode": service.AccountModeCoding,
		"api_key":      "sk-stepfun-new",
		"base_url":     service.DefaultStepFunCodingBaseURL,
	}
	payload, err := json.Marshal(normalizeJSONMap(credentials))
	require.NoError(t, err)

	mock.ExpectBegin()
	mock.ExpectExec(`(?s)UPDATE accounts.*- 'stepfun_usage_auto_refresh'.*- 'stepfun_usage_snapshot'.*- 'stepfun_monthly_used_percent'.*- 'upstream_billing_probe'`).
		WithArgs(string(payload), int64(41)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO scheduler_outbox")).
		WithArgs(service.SchedulerOutboxEventAccountChanged, int64(41), nil, nil, sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()
	repo := newAccountRepositoryWithSQL(client, nil, nil)

	err = repo.UpdateCredentials(context.Background(), 41, credentials)

	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestNextStepPlanUsageAttemptSeqAllocatesDatabaseSequence(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	mock.ExpectQuery(regexp.QuoteMeta("SELECT nextval('stepfun_usage_attempt_seq')")).
		WillReturnRows(sqlmock.NewRows([]string{"nextval"}).AddRow(int64(17)))
	repo := newAccountRepositoryWithSQL(nil, db, nil)

	seq, err := repo.NextStepPlanUsageAttemptSeq(context.Background())

	require.NoError(t, err)
	require.Equal(t, int64(17), seq)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestUpdateStepPlanExtraIfIdentityUnchangedUsesAtomicIdentityAndAttemptPredicate(t *testing.T) {
	exec := &recordingSQLExecutor{result: rowsAffectedResult(1)}
	repo := newAccountRepositoryWithSQL(nil, exec, nil)
	updates := map[string]any{
		service.StepFunUsageAutoRefreshExtraKey: false,
	}
	attemptSeq := int64(17)

	err := repo.UpdateStepPlanExtraIfIdentityUnchanged(
		context.Background(),
		41,
		"sk-stepfun",
		service.DefaultStepFunCodingBaseURL,
		updates,
		attemptSeq,
	)

	require.NoError(t, err)
	require.NotEmpty(t, exec.execQueries)
	query := normalizeSQLWhitespace(exec.execQueries[0])
	require.Contains(t, query, "UPDATE accounts")
	require.Contains(t, query, "platform = 'stepfun'")
	require.Contains(t, query, "type = 'apikey'")
	require.Contains(t, query, "account_mode")
	require.Contains(t, query, "COALESCE(credentials ->> 'api_key', '') = $3")
	require.Contains(t, query, "IS NOT DISTINCT FROM $4")
	require.Contains(t, query, "stepfun_usage_attempt_seq")
	require.Contains(t, query, "< $5::bigint")
	require.Equal(t, int64(41), exec.execArgs[0][1])
	require.Equal(t, "sk-stepfun", exec.execArgs[0][2])
	require.Equal(t, service.DefaultStepFunCodingBaseURL, exec.execArgs[0][3])
	require.Equal(t, attemptSeq, exec.execArgs[0][4])
	var payload map[string]any
	require.NoError(t, json.Unmarshal([]byte(fmt.Sprint(exec.execArgs[0][0])), &payload))
	require.Equal(t, float64(attemptSeq), payload[service.StepFunUsageAttemptSeqExtraKey])
}

func TestUpdateStepPlanExtraIfIdentityUnchangedReturnsConflictWhenNoRowMatches(t *testing.T) {
	exec := &recordingSQLExecutor{result: rowsAffectedResult(0)}
	repo := newAccountRepositoryWithSQL(nil, exec, nil)

	err := repo.UpdateStepPlanExtraIfIdentityUnchanged(
		context.Background(),
		41,
		"sk-stepfun",
		service.DefaultStepFunCodingBaseURL,
		map[string]any{service.StepFunUsageAutoRefreshExtraKey: false},
		0,
	)

	require.ErrorIs(t, err, service.ErrCNQuotaIdentityChanged)
	require.NotEmpty(t, exec.execQueries)
}
