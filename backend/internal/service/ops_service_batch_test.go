package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestOpsServiceRecordErrorBatch_SanitizesAndBatches(t *testing.T) {
	t.Parallel()

	var captured []*OpsInsertErrorLogInput
	repo := &opsRepoMock{
		BatchInsertErrorLogsFn: func(ctx context.Context, inputs []*OpsInsertErrorLogInput) (int64, error) {
			captured = append(captured, inputs...)
			return int64(len(inputs)), nil
		},
	}
	svc := NewOpsService(repo, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)

	msg := " upstream failed: https://example.com?access_token=secret-value "
	detail := `{"authorization":"Bearer secret-token"}`
	entries := []*OpsInsertErrorLogInput{
		{
			ErrorBody:            `{"error":"bad","access_token":"secret"}`,
			UpstreamStatusCode:   intPtr(-10),
			UpstreamErrorMessage: strPtr(msg),
			UpstreamErrorDetail:  strPtr(detail),
			UpstreamErrors: []*OpsUpstreamErrorEvent{
				{
					AccountID:           -2,
					UpstreamStatusCode:  429,
					Message:             " token leaked ",
					Detail:              `{"refresh_token":"secret"}`,
					UpstreamRequestBody: `{"api_key":"secret","messages":[{"role":"user","content":"hello"}]}`,
				},
			},
		},
		{
			ErrorPhase: "upstream",
			ErrorType:  "upstream_error",
			CreatedAt:  time.Now().UTC(),
		},
	}

	require.NoError(t, svc.RecordErrorBatch(context.Background(), entries))
	require.Len(t, captured, 2)

	first := captured[0]
	require.Equal(t, "internal", first.ErrorPhase)
	require.Equal(t, "api_error", first.ErrorType)
	require.Nil(t, first.UpstreamStatusCode)
	require.NotNil(t, first.UpstreamErrorMessage)
	require.NotContains(t, *first.UpstreamErrorMessage, "secret-value")
	require.Contains(t, *first.UpstreamErrorMessage, "access_token=***")
	require.NotNil(t, first.UpstreamErrorDetail)
	require.NotContains(t, *first.UpstreamErrorDetail, "secret-token")
	require.NotContains(t, first.ErrorBody, "secret")
	require.Nil(t, first.UpstreamErrors)
	require.NotNil(t, first.UpstreamErrorsJSON)
	require.NotContains(t, *first.UpstreamErrorsJSON, "secret")
	require.Contains(t, *first.UpstreamErrorsJSON, "[REDACTED]")

	second := captured[1]
	require.Equal(t, "upstream", second.ErrorPhase)
	require.Equal(t, "upstream_error", second.ErrorType)
	require.False(t, second.CreatedAt.IsZero())
}

func TestOpsServiceRecordErrorBatch_FallsBackToSingleInsert(t *testing.T) {
	t.Parallel()

	var (
		batchCalls  int
		singleCalls int
	)
	repo := &opsRepoMock{
		BatchInsertErrorLogsFn: func(ctx context.Context, inputs []*OpsInsertErrorLogInput) (int64, error) {
			batchCalls++
			return 0, errors.New("batch failed")
		},
		InsertErrorLogFn: func(ctx context.Context, input *OpsInsertErrorLogInput) (int64, error) {
			singleCalls++
			return int64(singleCalls), nil
		},
	}
	svc := NewOpsService(repo, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)

	err := svc.RecordErrorBatch(context.Background(), []*OpsInsertErrorLogInput{
		{ErrorMessage: "first"},
		{ErrorMessage: "second"},
	})
	require.NoError(t, err)
	require.Equal(t, 1, batchCalls)
	require.Equal(t, 2, singleCalls)
}

func TestOpsServiceRecordErrorBatch_PreservesOpenAIRoutingFields(t *testing.T) {
	t.Parallel()

	var captured []*OpsInsertErrorLogInput
	repo := &opsRepoMock{
		InsertErrorLogFn: func(ctx context.Context, input *OpsInsertErrorLogInput) (int64, error) {
			captured = append(captured, input)
			return 1, nil
		},
	}
	svc := NewOpsService(repo, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)

	entry := &OpsInsertErrorLogInput{
		ErrorPhase:                 "upstream",
		ErrorType:                  "upstream_error",
		RoutingTargetGroup:         "exhausted",
		RoutingScheduleLayer:       "load_balance",
		RoutingSelectedAccountID:   i64p(66),
		RoutingSelectedAccountName: strPtr("acc-66"),
		RoutingRequestedModel:      "gpt-5.4-Sys",
		RoutingEffectiveModel:      "gpt-5.4",
		RoutingFailoverCount:       1,
		RoutingFailoverFinalReason: "selected_exhausted_fallback",
	}

	require.NoError(t, svc.RecordErrorBatch(context.Background(), []*OpsInsertErrorLogInput{entry}))
	require.Len(t, captured, 1)
	require.Equal(t, entry.RoutingTargetGroup, captured[0].RoutingTargetGroup)
	require.Equal(t, entry.RoutingScheduleLayer, captured[0].RoutingScheduleLayer)
	require.Equal(t, entry.RoutingSelectedAccountID, captured[0].RoutingSelectedAccountID)
	require.Equal(t, entry.RoutingSelectedAccountName, captured[0].RoutingSelectedAccountName)
	require.Equal(t, entry.RoutingRequestedModel, captured[0].RoutingRequestedModel)
	require.Equal(t, entry.RoutingEffectiveModel, captured[0].RoutingEffectiveModel)
	require.Equal(t, entry.RoutingFailoverCount, captured[0].RoutingFailoverCount)
	require.Equal(t, entry.RoutingFailoverFinalReason, captured[0].RoutingFailoverFinalReason)
}

func strPtr(v string) *string {
	return &v
}
