package web3deposit

type DepositStatus string

const (
	DepositStatusDetected      DepositStatus = "detected"
	DepositStatusConfirming    DepositStatus = "confirming"
	DepositStatusReadyToCredit DepositStatus = "ready_to_credit"
	DepositStatusCrediting     DepositStatus = "crediting"
	DepositStatusCredited      DepositStatus = "credited"
	DepositStatusBelowMinimum  DepositStatus = "below_minimum"
	DepositStatusManualReview  DepositStatus = "manual_review"
	DepositStatusOrphaned      DepositStatus = "orphaned"
	DepositStatusFailed        DepositStatus = "failed"
	DepositStatusIgnored       DepositStatus = "ignored"
)

func (s DepositStatus) IsValid() bool {
	switch s {
	case DepositStatusDetected,
		DepositStatusConfirming,
		DepositStatusReadyToCredit,
		DepositStatusCrediting,
		DepositStatusCredited,
		DepositStatusBelowMinimum,
		DepositStatusManualReview,
		DepositStatusOrphaned,
		DepositStatusFailed,
		DepositStatusIgnored:
		return true
	default:
		return false
	}
}

func (s DepositStatus) CanTransitionTo(next DepositStatus) bool {
	if !s.IsValid() || !next.IsValid() {
		return false
	}
	switch s {
	case DepositStatusDetected:
		return next == DepositStatusConfirming ||
			next == DepositStatusReadyToCredit ||
			next == DepositStatusBelowMinimum ||
			next == DepositStatusManualReview ||
			next == DepositStatusOrphaned
	case DepositStatusConfirming:
		return next == DepositStatusReadyToCredit ||
			next == DepositStatusBelowMinimum ||
			next == DepositStatusManualReview ||
			next == DepositStatusOrphaned
	case DepositStatusReadyToCredit:
		return next == DepositStatusCrediting || next == DepositStatusFailed
	case DepositStatusCrediting:
		return next == DepositStatusCredited ||
			next == DepositStatusFailed ||
			next == DepositStatusReadyToCredit
	case DepositStatusManualReview:
		return next == DepositStatusReadyToCredit || next == DepositStatusIgnored
	case DepositStatusFailed:
		return next == DepositStatusReadyToCredit
	default:
		return false
	}
}
