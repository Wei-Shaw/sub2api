package repository

import (
	"context"
	"fmt"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/web3deposit"
	depositdomain "github.com/Wei-Shaw/sub2api/internal/web3deposit"
)

type Web3FinalizerBatchRepository struct {
	client *dbent.Client
}

var _ depositdomain.FinalizerBatchStore = (*Web3FinalizerBatchRepository)(nil)

func NewWeb3FinalizerBatchRepository(client *dbent.Client) *Web3FinalizerBatchRepository {
	return &Web3FinalizerBatchRepository{client: client}
}

func (r *Web3FinalizerBatchRepository) CommitFinalizedBatch(ctx context.Context, batch depositdomain.FinalizerBatch) (int, error) {
	if tx := dbent.TxFromContext(ctx); tx != nil {
		return commitFinalizedBatch(ctx, tx.Client(), batch)
	}
	tx, err := r.client.Tx(ctx)
	if err != nil {
		return 0, fmt.Errorf("begin web3 finalizer batch transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	txCtx := dbent.NewTxContext(ctx, tx)
	updated, err := commitFinalizedBatch(txCtx, tx.Client(), batch)
	if err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit web3 finalizer batch transaction: %w", err)
	}
	return updated, nil
}

func commitFinalizedBatch(ctx context.Context, client *dbent.Client, batch depositdomain.FinalizerBatch) (int, error) {
	updated := 0
	for _, decision := range batch.Decisions {
		if !isFinalizerTargetStatus(decision.Status) {
			return 0, fmt.Errorf("invalid web3 finalizer target status %q", decision.Status)
		}
		update := client.Web3Deposit.Update().
			Where(
				web3deposit.IDEQ(decision.DepositID),
				web3deposit.StatusIn(
					string(depositdomain.DepositStatusDetected),
					string(depositdomain.DepositStatusConfirming),
				),
			).
			SetStatus(string(decision.Status)).
			ClearNextRetryAt()
		switch decision.Status {
		case depositdomain.DepositStatusManualReview:
			update.ClearFailureReason().SetFinalizedAt(batch.Now)
			if decision.ReviewReason != "" {
				update.SetReviewReason(decision.ReviewReason)
			} else {
				update.ClearReviewReason()
			}
		case depositdomain.DepositStatusOrphaned:
			update.ClearReviewReason()
			update.ClearFinalizedAt()
			if decision.FailureReason != "" {
				update.SetFailureReason(decision.FailureReason)
			} else {
				update.ClearFailureReason()
			}
		case depositdomain.DepositStatusFailed:
			update.ClearReviewReason().SetFinalizedAt(batch.Now)
			if decision.FailureReason != "" {
				update.SetFailureReason(decision.FailureReason)
			} else {
				update.ClearFailureReason()
			}
		default:
			update.ClearReviewReason().ClearFailureReason().SetFinalizedAt(batch.Now)
		}
		count, err := update.Save(ctx)
		if err != nil {
			return 0, fmt.Errorf("update finalized web3 deposit %d: %w", decision.DepositID, err)
		}
		updated += count
	}
	if err := NewWeb3ScannerCursorRepository(client).AdvanceFinalizer(
		ctx,
		batch.ScannerKey,
		batch.LeaseToken,
		batch.FinalizedThrough,
		batch.Now,
	); err != nil {
		return 0, fmt.Errorf("advance web3 finalizer batch cursor: %w", err)
	}
	return updated, nil
}

func isFinalizerTargetStatus(status depositdomain.DepositStatus) bool {
	switch status {
	case depositdomain.DepositStatusReadyToCredit,
		depositdomain.DepositStatusBelowMinimum,
		depositdomain.DepositStatusManualReview,
		depositdomain.DepositStatusOrphaned,
		depositdomain.DepositStatusFailed:
		return true
	default:
		return false
	}
}
