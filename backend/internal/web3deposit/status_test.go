package web3deposit

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDepositStatusIsValid(t *testing.T) {
	t.Parallel()

	for _, status := range allDepositStatuses() {
		require.True(t, status.IsValid(), status)
	}
	require.False(t, DepositStatus("").IsValid())
	require.False(t, DepositStatus("unknown").IsValid())
}

func TestDepositStatusCanTransitionTo(t *testing.T) {
	t.Parallel()

	allowed := map[DepositStatus]map[DepositStatus]struct{}{
		DepositStatusDetected: {
			DepositStatusConfirming:    {},
			DepositStatusReadyToCredit: {},
			DepositStatusBelowMinimum:  {},
			DepositStatusManualReview:  {},
			DepositStatusOrphaned:      {},
			DepositStatusFailed:        {},
		},
		DepositStatusConfirming: {
			DepositStatusReadyToCredit: {},
			DepositStatusBelowMinimum:  {},
			DepositStatusManualReview:  {},
			DepositStatusOrphaned:      {},
			DepositStatusFailed:        {},
		},
		DepositStatusReadyToCredit: {
			DepositStatusCrediting: {},
			DepositStatusFailed:    {},
		},
		DepositStatusCrediting: {
			DepositStatusCredited:      {},
			DepositStatusFailed:        {},
			DepositStatusReadyToCredit: {},
		},
		DepositStatusManualReview: {
			DepositStatusReadyToCredit: {},
			DepositStatusIgnored:       {},
		},
		DepositStatusFailed: {
			DepositStatusReadyToCredit: {},
		},
	}

	statuses := allDepositStatuses()
	for _, from := range statuses {
		for _, to := range statuses {
			_, want := allowed[from][to]
			require.Equalf(t, want, from.CanTransitionTo(to), "%s -> %s", from, to)
		}
	}

	require.False(t, DepositStatus("unknown").CanTransitionTo(DepositStatusDetected))
	require.False(t, DepositStatusDetected.CanTransitionTo(DepositStatus("unknown")))
}

func allDepositStatuses() []DepositStatus {
	return []DepositStatus{
		DepositStatusDetected,
		DepositStatusConfirming,
		DepositStatusReadyToCredit,
		DepositStatusCrediting,
		DepositStatusCredited,
		DepositStatusBelowMinimum,
		DepositStatusManualReview,
		DepositStatusOrphaned,
		DepositStatusFailed,
		DepositStatusIgnored,
	}
}
