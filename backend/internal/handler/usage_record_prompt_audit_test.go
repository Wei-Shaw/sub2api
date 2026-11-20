package handler

import (
	"context"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

// A usage task runs on a worker context, not the request context. The audit
// fee taken at submission must reach the context the task runs on, and a later
// audit on the same request must not leak into an earlier task.
func TestWrapUsageRecordTaskCarriesPromptAuditFeeToWorkerContext(t *testing.T) {
	charge := service.NewPromptAuditCharge()
	parent := service.WithPromptAuditCharge(context.Background(), charge)
	charge.Add(0.00019, nil)

	var billed []float64
	record := func(ctx context.Context) {
		cost, ok := service.PromptAuditCostFromContext(ctx)
		require.True(t, ok)
		billed = append(billed, cost)
	}
	first, _ := wrapUsageRecordTaskContext(parent, record)
	charge.Add(0.00007, nil) // the next WebSocket turn is audited before the first task runs
	second, _ := wrapUsageRecordTaskContext(parent, record)

	first(context.Background())
	second(context.Background())
	require.InDeltaSlice(t, []float64{0.00019, 0.00007}, billed, 1e-12)
	require.Zero(t, charge.Take())

	withoutCharge, _ := wrapUsageRecordTaskContext(context.Background(), func(ctx context.Context) {
		cost, ok := service.PromptAuditCostFromContext(ctx)
		require.True(t, ok)
		require.Zero(t, cost)
	})
	withoutCharge(context.Background())
}
