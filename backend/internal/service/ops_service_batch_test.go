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

func TestOpsServiceRecordErrorBatch_RedactsOpenCodeImageMessages(t *testing.T) {
	t.Parallel()

	sample := "aGVsbG8="
	imageURL := "data:image/png;base64," + sample
	assertNoImageLeak := func(t testing.TB, label string, value string) {
		t.Helper()
		require.NotContains(t, value, "data:image", label)
		require.NotContains(t, value, sample, label)
	}

	var capturedSingle []*OpsInsertErrorLogInput
	singleRepo := &opsRepoMock{
		InsertErrorLogFn: func(ctx context.Context, input *OpsInsertErrorLogInput) (int64, error) {
			capturedSingle = append(capturedSingle, input)
			return 1, nil
		},
	}
	singleSvc := NewOpsService(singleRepo, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	upstreamMessage := "upstream echoed generated image " + imageURL + " https://example.com?access_token=secret-value"
	errorMessage := "malformed upstream body copied generated image " + imageURL

	require.NoError(t, singleSvc.RecordError(context.Background(), &OpsInsertErrorLogInput{
		ErrorPhase:           "upstream",
		ErrorType:            "upstream_error",
		ErrorMessage:         errorMessage,
		UpstreamErrorMessage: &upstreamMessage,
	}, nil))
	require.Len(t, capturedSingle, 1)
	assertNoImageLeak(t, "ErrorMessage", capturedSingle[0].ErrorMessage)
	require.NotNil(t, capturedSingle[0].UpstreamErrorMessage)
	assertNoImageLeak(t, "UpstreamErrorMessage", *capturedSingle[0].UpstreamErrorMessage)
	require.Contains(t, *capturedSingle[0].UpstreamErrorMessage, "access_token=***")
	require.NotContains(t, *capturedSingle[0].UpstreamErrorMessage, "secret-value")

	var capturedBatch []*OpsInsertErrorLogInput
	batchRepo := &opsRepoMock{
		BatchInsertErrorLogsFn: func(ctx context.Context, inputs []*OpsInsertErrorLogInput) (int64, error) {
			capturedBatch = append(capturedBatch, inputs...)
			return int64(len(inputs)), nil
		},
	}
	batchSvc := NewOpsService(batchRepo, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)

	require.NoError(t, batchSvc.RecordErrorBatch(context.Background(), []*OpsInsertErrorLogInput{
		{
			ErrorPhase: "upstream",
			ErrorType:  "upstream_error",
			UpstreamErrors: []*OpsUpstreamErrorEvent{
				{Kind: "http_error", Message: "event echoed generated image " + imageURL},
			},
		},
		{ErrorPhase: "upstream", ErrorType: "upstream_error", ErrorBody: "plain"},
	}))
	require.Len(t, capturedBatch, 2)
	require.NotNil(t, capturedBatch[0].UpstreamErrorsJSON)
	assertNoImageLeak(t, "UpstreamErrorsJSON", *capturedBatch[0].UpstreamErrorsJSON)
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
