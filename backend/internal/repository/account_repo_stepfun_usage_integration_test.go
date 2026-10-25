//go:build integration

package repository

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestUpdateStepPlanExtraRejectsOlderDatabaseAttemptSequence(t *testing.T) {
	ctx := context.Background()
	tx := testEntTx(t)
	account := mustCreateAccount(t, tx.Client(), &service.Account{
		Name:     "stepfun-step-plan-cas-sequence",
		Platform: service.PlatformStepfun,
		Type:     service.AccountTypeAPIKey,
		Credentials: map[string]any{
			"account_mode": service.AccountModeCoding,
			"api_key":      "sk-stepfun-sequence",
			"base_url":     service.DefaultStepFunCodingBaseURL,
		},
	})
	repo := newAccountRepositoryWithSQL(tx.Client(), tx, nil)

	olderSeq, err := repo.NextStepPlanUsageAttemptSeq(ctx)
	require.NoError(t, err)
	newerSeq, err := repo.NextStepPlanUsageAttemptSeq(ctx)
	require.NoError(t, err)
	require.Greater(t, newerSeq, olderSeq)

	attemptAt := time.Now().UTC()
	require.NoError(t, repo.UpdateStepPlanExtraIfIdentityUnchanged(
		ctx,
		account.ID,
		account.GetCNAPIKey(),
		account.GetOpenAIBaseURL(),
		map[string]any{
			service.StepFunUsageSnapshotExtraKey: service.StepFunUsageSnapshot{
				Status:          service.StepFunUsageStatusOK,
				CredentialValid: true,
				PlanLevel:       "newer",
				Tiers:           []service.CNQuotaTier{{Window: "monthly", UsedPercent: 55}},
				LastAttemptAt:   attemptAt,
			},
		},
		newerSeq,
	))

	err = repo.UpdateStepPlanExtraIfIdentityUnchanged(
		ctx,
		account.ID,
		account.GetCNAPIKey(),
		account.GetOpenAIBaseURL(),
		map[string]any{
			service.StepFunUsageSnapshotExtraKey: service.StepFunUsageSnapshot{
				Status:          service.StepFunUsageStatusOK,
				CredentialValid: true,
				PlanLevel:       "older",
				Tiers:           []service.CNQuotaTier{{Window: "monthly", UsedPercent: 30}},
				LastAttemptAt:   attemptAt.Add(-time.Minute),
			},
		},
		olderSeq,
	)
	require.ErrorIs(t, err, service.ErrCNQuotaIdentityChanged)

	var storedPlan, storedSeq string
	require.NoError(t, scanSingleRow(ctx, tx, `
		SELECT extra #>> '{stepfun_usage_snapshot,plan_level}',
		       extra ->> 'stepfun_usage_attempt_seq'
		FROM accounts
		WHERE id = $1
	`, []any{account.ID}, &storedPlan, &storedSeq))
	require.Equal(t, "newer", storedPlan)
	require.Equal(t, strconv.FormatInt(newerSeq, 10), storedSeq)
}
