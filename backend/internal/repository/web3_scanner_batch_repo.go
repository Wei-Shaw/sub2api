package repository

import (
	"context"
	"fmt"
	"strings"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/web3deposit"
)

type Web3ScannerBatchRepository struct {
	client *dbent.Client
}

var _ web3deposit.ScannerBatchStore = (*Web3ScannerBatchRepository)(nil)

func NewWeb3ScannerBatchRepository(client *dbent.Client) *Web3ScannerBatchRepository {
	return &Web3ScannerBatchRepository{client: client}
}

func (r *Web3ScannerBatchRepository) CommitDetectedBatch(
	ctx context.Context,
	batch web3deposit.ScannerBatch,
) ([]web3deposit.Deposit, error) {
	if tx := dbent.TxFromContext(ctx); tx != nil {
		return commitDetectedScannerBatch(ctx, tx.Client(), batch)
	}

	tx, err := r.client.Tx(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin web3 scanner batch transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	txCtx := dbent.NewTxContext(ctx, tx)
	deposits, err := commitDetectedScannerBatch(txCtx, tx.Client(), batch)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit web3 scanner batch transaction: %w", err)
	}
	return deposits, nil
}

func commitDetectedScannerBatch(
	ctx context.Context,
	client *dbent.Client,
	batch web3deposit.ScannerBatch,
) ([]web3deposit.Deposit, error) {
	cursorRepo := NewWeb3ScannerCursorRepository(client)
	cursor, err := cursorRepo.GetByKey(ctx, batch.ScannerKey)
	if err != nil {
		return nil, fmt.Errorf("get web3 scanner batch cursor: %w", err)
	}
	if cursor.ChainID != batch.Config.ChainID ||
		cursor.TokenContract != strings.ToLower(batch.Config.TokenAddress.Hex()) {
		return nil, web3deposit.ErrCursorIdentityConflict
	}

	persister := web3deposit.NewDepositEventPersister(NewWeb3DepositRepository(client), batch.Config)
	deposits, err := persister.PersistDetected(ctx, batch.Matches)
	if err != nil {
		return nil, fmt.Errorf("persist web3 scanner batch deposits: %w", err)
	}

	if err := cursorRepo.AdvanceScanner(
		ctx,
		batch.ScannerKey,
		batch.LeaseToken,
		batch.ScannedThrough,
		batch.Now,
	); err != nil {
		return nil, fmt.Errorf("advance web3 scanner batch cursor: %w", err)
	}
	return deposits, nil
}
