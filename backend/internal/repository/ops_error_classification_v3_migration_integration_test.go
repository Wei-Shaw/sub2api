//go:build integration

package repository

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	dbmigrations "github.com/Wei-Shaw/sub2api/migrations"
	"github.com/stretchr/testify/require"
)

type opsV3MigrationExpectation struct {
	name            string
	statusCode      int
	upstreamStatus  int
	upstreamNull    bool
	errorPhase      string
	errorSource     string
	errorType       string
	businessLimited bool
	finalOutcome    string
	message         string
	wantOutcome     string
	wantOwner       string
	wantCategory    string
	wantSLA         bool
	wantFamily      string
}

func TestMigration209BackfillsAndGuardsMixedVersionOpsWrites(t *testing.T) {
	tx := testTx(t)
	ctx := context.Background()
	triggerMigrationSQL, err := dbmigrations.FS.ReadFile("209_reclassify_confirmed_upstream_failures.sql")
	require.NoError(t, err)
	backfillMigrationSQL, err := dbmigrations.FS.ReadFile("210_backfill_ops_error_classification_v3.sql")
	require.NoError(t, err)

	_, err = tx.ExecContext(ctx, "DROP TRIGGER ops_error_logs_normalize_v3_mixed_writer ON ops_error_logs")
	require.NoError(t, err)

	fixtures := []opsV3MigrationExpectation{
		{
			name: "unknown upstream 403", statusCode: 502, upstreamStatus: 403,
			finalOutcome: "client_rejected", message: "provider rejected request",
			wantOutcome: "provider_failed", wantOwner: "provider", wantCategory: "provider_server",
			wantSLA: true, wantFamily: "provider_health",
		},
		{
			name: "revoked OAuth token", statusCode: 502, upstreamStatus: 403,
			finalOutcome: "client_rejected", message: "OAuth token has been revoked",
			wantOutcome: "platform_failed", wantOwner: "platform", wantCategory: "platform_credential",
			wantSLA: true, wantFamily: "credential",
		},
		{
			name: "disabled capability", statusCode: 502, upstreamStatus: 403,
			finalOutcome: "client_rejected", message: "permission is not enabled for this workspace",
			wantOutcome: "platform_failed", wantOwner: "platform", wantCategory: "product_compatibility",
			wantSLA: false, wantFamily: "compatibility",
		},
		{
			name: "context limit hidden by gateway", statusCode: 502, upstreamStatus: 400,
			finalOutcome: "client_rejected", message: "maximum context length exceeded",
			wantOutcome: "platform_failed", wantOwner: "platform", wantCategory: "product_compatibility",
			wantSLA: false, wantFamily: "compatibility",
		},
		{
			name: "unsupported parameter hidden by gateway", statusCode: 502, upstreamStatus: 400,
			finalOutcome: "client_rejected", message: "unsupported parameter: reasoning_mode",
			wantOutcome: "platform_failed", wantOwner: "platform", wantCategory: "product_compatibility",
			wantSLA: false, wantFamily: "compatibility",
		},
		{
			name: "invalid request type hidden by gateway", statusCode: 502, upstreamStatus: 400,
			errorType: "invalid_request_error", finalOutcome: "client_rejected",
			message:     "provider rejected request without semantic message",
			wantOutcome: "platform_failed", wantOwner: "platform", wantCategory: "product_compatibility",
			wantSLA: false, wantFamily: "compatibility",
		},
		{
			name: "invalid request type preserved as upstream 400", statusCode: 400, upstreamStatus: 400,
			errorType: "invalid_request_error", finalOutcome: "client_rejected",
			message:     "X-OpenAI-Internal-Codex-Responses-Lite requires reasoning.context to be all_turns",
			wantOutcome: "client_rejected", wantOwner: "client", wantCategory: "invalid_request",
			wantSLA: false, wantFamily: "client_quality",
		},
		{
			name: "empty input hidden by gateway", statusCode: 502, upstreamStatus: 400,
			errorType: "api_error", finalOutcome: "client_rejected", message: "Empty input messages",
			wantOutcome: "platform_failed", wantOwner: "platform", wantCategory: "product_compatibility",
			wantSLA: false, wantFamily: "compatibility",
		},
		{
			name: "local group model mismatch with stale upstream status", statusCode: 404, upstreamStatus: 503,
			errorType: "model_not_found", finalOutcome: "client_rejected",
			message:     `Model "example" is not supported by any configured account in this group`,
			wantOutcome: "client_rejected", wantOwner: "client", wantCategory: "unsupported_model",
			wantSLA: false, wantFamily: "compatibility",
		},
		{
			name: "capacity rejection with upstream 402", statusCode: 502, upstreamStatus: 402,
			finalOutcome: "client_rejected", message: "insufficient_balance",
			wantOutcome: "platform_failed", wantOwner: "platform", wantCategory: "platform_capacity",
			wantSLA: true, wantFamily: "capacity",
		},
		{
			name: "account pool capacity with stale upstream 503", statusCode: 499, upstreamStatus: 503,
			finalOutcome: "client_rejected", message: "No available accounts",
			wantOutcome: "platform_failed", wantOwner: "platform", wantCategory: "platform_capacity",
			wantSLA: true, wantFamily: "capacity",
		},
		{
			name: "account concurrency with stale upstream 429", statusCode: 499, upstreamStatus: 429,
			finalOutcome: "client_rejected", message: "Concurrency limit exceeded for account",
			wantOutcome: "platform_failed", wantOwner: "platform", wantCategory: "platform_capacity",
			wantSLA: true, wantFamily: "capacity",
		},
		{
			name: "capability rejection with upstream 404", statusCode: 502, upstreamStatus: 404,
			finalOutcome: "client_rejected", message: "capability is not enabled",
			wantOutcome: "platform_failed", wantOwner: "platform", wantCategory: "product_compatibility",
			wantSLA: false, wantFamily: "compatibility",
		},
		{
			name: "recovered expired token", statusCode: 200, upstreamStatus: 403,
			finalOutcome: "recovered", message: "OAuth access token expired",
			wantOutcome: "recovered", wantOwner: "platform", wantCategory: "recovered",
			wantSLA: false, wantFamily: "credential",
		},
		{
			name: "recovered unsupported model", statusCode: 200, upstreamStatus: 400,
			finalOutcome: "recovered", message: "model is not supported by this provider",
			wantOutcome: "recovered", wantOwner: "provider", wantCategory: "recovered",
			wantSLA: false, wantFamily: "compatibility",
		},
		{
			name: "trusted recovered broken pipe without upstream status", statusCode: 200, upstreamNull: true,
			finalOutcome: "recovered", message: "Recovered upstream error: upstream stream disconnected: broken pipe",
			wantOutcome: "recovered", wantOwner: "provider", wantCategory: "recovered",
			wantSLA: false, wantFamily: "provider_health",
		},
		{
			name: "trusted recovered upstream failure without status", statusCode: 200, upstreamNull: true,
			finalOutcome: "recovered", message: "Recovered upstream error: connection reset",
			wantOutcome: "recovered", wantOwner: "provider", wantCategory: "recovered",
			wantSLA: false, wantFamily: "provider_health",
		},
		{
			name: "recovered semantic failure with upstream 503", statusCode: 200, upstreamStatus: 503,
			finalOutcome: "recovered", message: "maximum context length exceeded",
			wantOutcome: "recovered", wantOwner: "platform", wantCategory: "recovered",
			wantSLA: false, wantFamily: "compatibility",
		},
		{
			name: "recovered empty input with upstream 503", statusCode: 200, upstreamStatus: 503,
			finalOutcome: "recovered", message: "Empty input messages",
			wantOutcome: "recovered", wantOwner: "platform", wantCategory: "recovered",
			wantSLA: false, wantFamily: "compatibility",
		},
		{
			name: "recovered client cancellation with upstream 503", statusCode: 200, upstreamStatus: 503,
			errorType: "api_error", finalOutcome: "recovered", message: "client disconnected after upstream attempt",
			wantOutcome: "recovered", wantOwner: "client", wantCategory: "recovered",
			wantSLA: false, wantFamily: "client_quality",
		},
		{
			name: "recovered invalid request type with upstream 503", statusCode: 200, upstreamStatus: 503,
			errorType: "invalid_request_error", finalOutcome: "recovered", message: "provider failed without request semantics",
			wantOutcome: "recovered", wantOwner: "provider", wantCategory: "recovered",
			wantSLA: false, wantFamily: "provider_health",
		},
		{
			name: "recovered model not configured with upstream 503", statusCode: 200, upstreamStatus: 503,
			finalOutcome: "recovered", message: "model is not configured for this provider",
			wantOutcome: "recovered", wantOwner: "provider", wantCategory: "recovered",
			wantSLA: false, wantFamily: "compatibility",
		},
		{
			name: "historical local user concurrency limit", statusCode: 200, upstreamNull: true,
			errorPhase: "request", errorType: "rate_limit_error", businessLimited: true,
			finalOutcome: "recovered", message: "Concurrency limit exceeded for user 42",
			wantOutcome: "business_limited", wantOwner: "client", wantCategory: "user_concurrency",
			wantSLA: false, wantFamily: "client_quality",
		},
	}

	ids := make(map[string]int64, len(fixtures))
	for _, fixture := range fixtures {
		errorType := fixture.errorType
		if errorType == "" {
			errorType = "upstream_error"
		}
		errorPhase := fixture.errorPhase
		if errorPhase == "" {
			errorPhase = "upstream"
		}
		var id int64
		var upstreamValue any = fixture.upstreamStatus
		if fixture.upstreamNull {
			upstreamValue = nil
		}
		err := tx.QueryRowContext(ctx, `
			INSERT INTO ops_error_logs (
				platform, error_phase, error_source, error_type, status_code, upstream_status_code,
				error_message, final_outcome, responsibility, error_category,
				counts_toward_sla, alert_family, classification_reason,
				classification_version, is_business_limited, is_count_tokens, created_at
			) VALUES (
				'ops-migration-209', $1, $2, $3, $4, $5,
				$6, $7, 'client', 'invalid_request',
				FALSE, 'client_quality', 'legacy_v2_fixture',
				2, $8, FALSE, NOW()
			) RETURNING id
		`, errorPhase, fixture.errorSource, errorType, fixture.statusCode, upstreamValue,
			fixture.message, fixture.finalOutcome, fixture.businessLimited).Scan(&id)
		require.NoError(t, err, fixture.name)
		ids[fixture.name] = id
	}

	_, err = tx.ExecContext(ctx, string(triggerMigrationSQL))
	require.NoError(t, err)
	_, err = tx.ExecContext(ctx, string(backfillMigrationSQL))
	require.NoError(t, err)

	for _, fixture := range fixtures {
		assertOpsV3MigrationRow(t, tx, ids[fixture.name], fixture)
	}

	rollingWriter := opsV3MigrationExpectation{
		name: "rolling v2 writer unknown 403", statusCode: 502, upstreamStatus: 403,
		finalOutcome: "client_rejected", message: "provider rejected request after backfill",
		wantOutcome: "provider_failed", wantOwner: "provider", wantCategory: "provider_server",
		wantSLA: true, wantFamily: "provider_health",
	}
	var rollingID int64
	err = tx.QueryRowContext(ctx, `
		INSERT INTO ops_error_logs (
			platform, error_phase, error_type, status_code, upstream_status_code,
			error_message, final_outcome, responsibility, error_category,
			counts_toward_sla, alert_family, classification_reason,
			classification_version, is_count_tokens, created_at
		) VALUES (
			'ops-migration-209', 'upstream', 'upstream_error', $1, $2,
			$3, $4, 'client', 'invalid_request',
			FALSE, 'client_quality', 'rolling_v2_fixture',
			2, FALSE, NOW()
		) RETURNING id
	`, rollingWriter.statusCode, rollingWriter.upstreamStatus, rollingWriter.message, rollingWriter.finalOutcome).Scan(&rollingID)
	require.NoError(t, err)
	assertOpsV3MigrationRow(t, tx, rollingID, rollingWriter)

	rollingBoundaryCases := []struct {
		fixture       opsV3MigrationExpectation
		errorType     string
		upstreamValue any
		errorPhase    string
		businessLimit bool
	}{
		{
			fixture: opsV3MigrationExpectation{
				name: "invalid request type with upstream 503", statusCode: 502, upstreamStatus: 503,
				finalOutcome: "client_rejected", message: "provider failed without request semantics",
				wantOutcome: "provider_failed", wantOwner: "provider", wantCategory: "provider_server",
				wantSLA: true, wantFamily: "provider_health",
			},
			errorType: "invalid_request_error", upstreamValue: 503,
		},
		{
			fixture: opsV3MigrationExpectation{
				name: "rolling invalid request preserved as upstream 400", statusCode: 400, upstreamStatus: 400,
				finalOutcome: "client_rejected", message: "Invalid value: ultra. Supported values are: low, medium, high",
				wantOutcome: "client_rejected", wantOwner: "client", wantCategory: "invalid_request",
				wantSLA: false, wantFamily: "client_quality",
			},
			errorType: "invalid_request_error", upstreamValue: 400,
		},
		{
			fixture: opsV3MigrationExpectation{
				name: "rolling empty input hidden by gateway", statusCode: 502, upstreamStatus: 400,
				finalOutcome: "client_rejected", message: "Empty input messages",
				wantOutcome: "platform_failed", wantOwner: "platform", wantCategory: "product_compatibility",
				wantSLA: false, wantFamily: "compatibility",
			},
			errorType: "api_error", upstreamValue: 400,
		},
		{
			fixture: opsV3MigrationExpectation{
				name: "rolling local group model mismatch with stale upstream status", statusCode: 404, upstreamStatus: 503,
				finalOutcome: "client_rejected", message: `Model "example" is not supported by any configured account in this group`,
				wantOutcome: "client_rejected", wantOwner: "client", wantCategory: "unsupported_model",
				wantSLA: false, wantFamily: "compatibility",
			},
			errorType: "model_not_found", upstreamValue: 503,
		},
		{
			fixture: opsV3MigrationExpectation{
				name: "rolling account pool capacity with stale upstream 503", statusCode: 499, upstreamStatus: 503,
				finalOutcome: "client_rejected", message: "No available accounts",
				wantOutcome: "platform_failed", wantOwner: "platform", wantCategory: "platform_capacity",
				wantSLA: true, wantFamily: "capacity",
			},
			errorType: "upstream_error", upstreamValue: 503,
		},
		{
			fixture: opsV3MigrationExpectation{
				name: "rolling account concurrency with stale upstream 429", statusCode: 499, upstreamStatus: 429,
				finalOutcome: "client_rejected", message: "Concurrency limit exceeded for account",
				wantOutcome: "platform_failed", wantOwner: "platform", wantCategory: "platform_capacity",
				wantSLA: true, wantFamily: "capacity",
			},
			errorType: "upstream_error", upstreamValue: 429,
		},
		{
			fixture: opsV3MigrationExpectation{
				name: "client cancellation without upstream status", statusCode: 499,
				finalOutcome: "client_rejected", message: "client closed request",
				wantOutcome: "cancelled", wantOwner: "client", wantCategory: "client_cancelled",
				wantSLA: false, wantFamily: "client_quality",
			},
			errorType: "upstream_error", upstreamValue: nil,
		},
		{
			fixture: opsV3MigrationExpectation{
				name: "overload type without upstream status", statusCode: 502,
				finalOutcome: "client_rejected", message: "provider request failed",
				wantOutcome: "provider_failed", wantOwner: "provider", wantCategory: "provider_overloaded",
				wantSLA: true, wantFamily: "provider_health",
			},
			errorType: "overloaded_error", upstreamValue: nil,
		},
		{
			fixture: opsV3MigrationExpectation{
				name: "upstream broken pipe without status", statusCode: 499,
				finalOutcome: "cancelled", message: "upstream stream disconnected: broken pipe",
				wantOutcome: "platform_failed", wantOwner: "platform", wantCategory: "network_transport",
				wantSLA: true, wantFamily: "provider_health",
			},
			errorType: "upstream_error", upstreamValue: nil,
		},
		{
			fixture: opsV3MigrationExpectation{
				name: "recovered upstream phase without status", statusCode: 200,
				finalOutcome: "recovered", message: "Recovered upstream error: connection reset",
				wantOutcome: "recovered", wantOwner: "provider", wantCategory: "recovered",
				wantSLA: false, wantFamily: "provider_health",
			},
			errorType: "upstream_error", upstreamValue: nil,
		},
		{
			fixture: opsV3MigrationExpectation{
				name: "rolling recovered unsupported model", statusCode: 200, upstreamStatus: 400,
				finalOutcome: "recovered", message: "model is not supported by this provider",
				wantOutcome: "recovered", wantOwner: "provider", wantCategory: "recovered",
				wantSLA: false, wantFamily: "compatibility",
			},
			errorType: "upstream_error", upstreamValue: 400,
		},
		{
			fixture: opsV3MigrationExpectation{
				name: "rolling recovered semantic failure with upstream 503", statusCode: 200, upstreamStatus: 503,
				finalOutcome: "recovered", message: "maximum context length exceeded",
				wantOutcome: "recovered", wantOwner: "platform", wantCategory: "recovered",
				wantSLA: false, wantFamily: "compatibility",
			},
			errorType: "upstream_error", upstreamValue: 503,
		},
		{
			fixture: opsV3MigrationExpectation{
				name: "recovered client cancellation", statusCode: 200,
				finalOutcome: "recovered", message: "client disconnected after upstream attempt",
				wantOutcome: "recovered", wantOwner: "client", wantCategory: "recovered",
				wantSLA: false, wantFamily: "client_quality",
			},
			errorType: "api_error", upstreamValue: 503,
		},
		{
			fixture: opsV3MigrationExpectation{
				name: "rolling recovered invalid request type with upstream 503", statusCode: 200, upstreamStatus: 503,
				finalOutcome: "recovered", message: "provider failed without request semantics",
				wantOutcome: "recovered", wantOwner: "provider", wantCategory: "recovered",
				wantSLA: false, wantFamily: "provider_health",
			},
			errorType: "invalid_request_error", upstreamValue: 503,
		},
		{
			fixture: opsV3MigrationExpectation{
				name: "rolling recovered model not configured with upstream 503", statusCode: 200, upstreamStatus: 503,
				finalOutcome: "recovered", message: "model is not configured for this provider",
				wantOutcome: "recovered", wantOwner: "provider", wantCategory: "recovered",
				wantSLA: false, wantFamily: "compatibility",
			},
			errorType: "upstream_error", upstreamValue: 503,
		},
		{
			fixture: opsV3MigrationExpectation{
				name: "rolling local user concurrency limit", statusCode: 200,
				finalOutcome: "recovered", message: "Concurrency limit exceeded for user 42",
				wantOutcome: "business_limited", wantOwner: "client", wantCategory: "user_concurrency",
				wantSLA: false, wantFamily: "client_quality",
			},
			errorType: "rate_limit_error", upstreamValue: nil, errorPhase: "request", businessLimit: true,
		},
	}
	for _, boundary := range rollingBoundaryCases {
		errorPhase := boundary.errorPhase
		if errorPhase == "" {
			errorPhase = "upstream"
		}
		var id int64
		err = tx.QueryRowContext(ctx, `
			INSERT INTO ops_error_logs (
				platform, error_phase, error_source, error_owner,
				error_type, status_code, upstream_status_code,
				error_message, final_outcome, responsibility, error_category,
				counts_toward_sla, alert_family, classification_reason,
				classification_version, is_business_limited, is_count_tokens, created_at
			) VALUES (
				'ops-migration-209', $1::varchar,
				CASE WHEN $1::varchar IN ('request', 'auth') THEN 'client_request' ELSE 'upstream_http' END,
				CASE WHEN $1::varchar IN ('request', 'auth') THEN 'client' ELSE 'provider' END,
				$2, $3, $4, $5, $6, 'client', 'invalid_request',
				FALSE, 'client_quality', 'rolling_v2_boundary_fixture',
				2, $7, FALSE, NOW()
			) RETURNING id
		`, errorPhase, boundary.errorType, boundary.fixture.statusCode, boundary.upstreamValue,
			boundary.fixture.message, boundary.fixture.finalOutcome, boundary.businessLimit).Scan(&id)
		require.NoError(t, err, boundary.fixture.name)
		assertOpsV3MigrationRow(t, tx, id, boundary.fixture)
	}

	var preservedVersion int
	err = tx.QueryRowContext(ctx, `
		INSERT INTO ops_error_logs (
			platform, error_phase, error_type, status_code, upstream_status_code,
			final_outcome, responsibility, error_category, counts_toward_sla,
			alert_family, classification_reason, classification_version,
			is_count_tokens, created_at
		) VALUES (
			'ops-migration-209', 'upstream', 'upstream_error', 502, 403,
			'unknown_failed', 'unknown', 'unknown', FALSE,
			'unknown_failure', 'v3_writer_controls_classification', 3,
			FALSE, NOW()
		) RETURNING classification_version
	`).Scan(&preservedVersion)
	require.NoError(t, err)
	require.Equal(t, 3, preservedVersion)

}

func TestMigration210BackfillDoesNotBlockMixedVersionInserts(t *testing.T) {
	ctx := context.Background()
	platform := "ops-migration-210-concurrency"
	t.Cleanup(func() {
		_, _ = integrationDB.ExecContext(context.Background(), "DELETE FROM ops_error_logs WHERE platform = $1", platform)
	})

	backfillMigrationSQL, err := dbmigrations.FS.ReadFile("210_backfill_ops_error_classification_v3.sql")
	require.NoError(t, err)
	backfillTx, err := integrationDB.BeginTx(ctx, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = backfillTx.Rollback() })
	_, err = backfillTx.ExecContext(ctx, string(backfillMigrationSQL))
	require.NoError(t, err)

	insertCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	var outcome, owner, category string
	err = integrationDB.QueryRowContext(insertCtx, `
		INSERT INTO ops_error_logs (
			platform, error_phase, error_type, status_code, upstream_status_code,
			error_message, final_outcome, responsibility, error_category,
			counts_toward_sla, alert_family, classification_reason,
			classification_version, is_count_tokens, created_at
		) VALUES (
			$1, 'upstream', 'upstream_error', 502, 403,
			'provider rejected request during backfill', 'client_rejected', 'client', 'invalid_request',
			FALSE, 'client_quality', 'rolling_v2_during_backfill',
			2, FALSE, NOW()
		)
		RETURNING final_outcome, responsibility, error_category
	`, platform).Scan(&outcome, &owner, &category)
	require.NoError(t, err, "backfill transaction must not block the error-log writer")
	require.Equal(t, "provider_failed", outcome)
	require.Equal(t, "provider", owner)
	require.Equal(t, "provider_server", category)
}

func TestOpsCredentialStatsExcludeRecoveredSignals(t *testing.T) {
	ctx := context.Background()
	platform := "ops-credential-stats-v3"
	now := time.Now().UTC()
	t.Cleanup(func() {
		_, _ = integrationDB.ExecContext(context.Background(), "DELETE FROM ops_error_logs WHERE platform = $1", platform)
	})

	_, err := integrationDB.ExecContext(ctx, `
		INSERT INTO ops_error_logs (
			platform, error_phase, error_type, status_code, upstream_status_code,
			final_outcome, responsibility, error_category, counts_toward_sla,
			alert_family, classification_reason, classification_version,
			is_count_tokens, created_at
		) VALUES
			($1, 'upstream', 'upstream_error', 502, 403,
			 'platform_failed', 'platform', 'platform_credential', TRUE,
			 'credential', 'managed_upstream_credential_rejected', 3, FALSE, $2),
			($1, 'upstream', 'upstream_error', 200, 403,
			 'recovered', 'platform', 'recovered', FALSE,
			 'credential', 'final_request_recovered', 3, FALSE, $2),
			($1, 'upstream', 'upstream_error', 502, 403,
			 'platform_failed', 'platform', 'platform_credential', FALSE,
			 'credential', 'non_sla_fixture', 3, FALSE, $2)
	`, platform, now)
	require.NoError(t, err)

	repo := &opsRepository{db: integrationDB}
	stats, err := repo.GetErrorClassificationStats(ctx, &service.OpsDashboardFilter{
		StartTime: now.Add(-time.Minute),
		EndTime:   now.Add(time.Minute),
		Platform:  platform,
	})
	require.NoError(t, err)
	require.EqualValues(t, 1, stats.PlatformCredentialCount,
		"credential failures must exclude recovered and non-SLA rows")
}

func assertOpsV3MigrationRow(
	t *testing.T,
	tx interface {
		QueryRowContext(context.Context, string, ...any) *sql.Row
	},
	id int64,
	want opsV3MigrationExpectation,
) {
	t.Helper()
	var outcome, owner, category, family string
	var countsTowardSLA bool
	var version int
	err := tx.QueryRowContext(context.Background(), `
		SELECT final_outcome, responsibility, error_category,
		       counts_toward_sla, alert_family, classification_version
		FROM ops_error_logs
		WHERE id = $1
	`, id).Scan(&outcome, &owner, &category, &countsTowardSLA, &family, &version)
	require.NoError(t, err, want.name)
	require.Equal(t, want.wantOutcome, outcome, want.name)
	require.Equal(t, want.wantOwner, owner, want.name)
	require.Equal(t, want.wantCategory, category, want.name)
	require.Equal(t, want.wantSLA, countsTowardSLA, want.name)
	require.Equal(t, want.wantFamily, family, want.name)
	require.Equal(t, 3, version, want.name)
}
