package repository

import (
	"context"
	"fmt"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/web3scannercursor"
	"github.com/Wei-Shaw/sub2api/internal/web3deposit"
)

type Web3ScannerCursorRepository struct {
	client *dbent.Client
}

func NewWeb3ScannerCursorRepository(client *dbent.Client) *Web3ScannerCursorRepository {
	return &Web3ScannerCursorRepository{client: client}
}

func (r *Web3ScannerCursorRepository) Initialize(
	ctx context.Context,
	scannerKey string,
	chainID uint64,
	tokenContract string,
	scanStartBlock uint64,
) (web3deposit.ScannerCursor, error) {
	storedChainID, err := web3DepositUint64ToInt64(chainID, "scanner chain ID")
	if err != nil {
		return web3deposit.ScannerCursor{}, err
	}
	storedStartBlock, err := web3DepositUint64ToInt64(scanStartBlock, "scan start block")
	if err != nil {
		return web3deposit.ScannerCursor{}, err
	}

	entity, err := r.client.Web3ScannerCursor.Create().
		SetScannerKey(scannerKey).
		SetChainID(storedChainID).
		SetTokenContract(tokenContract).
		SetScanStartBlock(storedStartBlock).
		SetLastScannedBlock(storedStartBlock).
		SetLastFinalizedBlock(storedStartBlock).
		Save(ctx)
	if err == nil {
		return web3ScannerCursorFromEnt(entity), nil
	}
	if !dbent.IsConstraintError(err) {
		return web3deposit.ScannerCursor{}, fmt.Errorf("initialize web3 scanner cursor: %w", err)
	}

	existing, queryErr := r.getEntityByKey(ctx, scannerKey)
	if dbent.IsNotFound(queryErr) {
		return web3deposit.ScannerCursor{}, web3deposit.ErrCursorIdentityConflict
	}
	if queryErr != nil {
		return web3deposit.ScannerCursor{}, fmt.Errorf("get existing web3 scanner cursor after initialize conflict: %w", queryErr)
	}
	if existing.ChainID != storedChainID || existing.TokenContract != tokenContract {
		return web3deposit.ScannerCursor{}, web3deposit.ErrCursorIdentityConflict
	}
	return web3ScannerCursorFromEnt(existing), nil
}

func (r *Web3ScannerCursorRepository) GetByKey(ctx context.Context, scannerKey string) (web3deposit.ScannerCursor, error) {
	entity, err := r.getEntityByKey(ctx, scannerKey)
	if dbent.IsNotFound(err) {
		return web3deposit.ScannerCursor{}, web3deposit.ErrCursorNotFound
	}
	if err != nil {
		return web3deposit.ScannerCursor{}, fmt.Errorf("get web3 scanner cursor: %w", err)
	}
	return web3ScannerCursorFromEnt(entity), nil
}

func (r *Web3ScannerCursorRepository) AcquireLease(
	ctx context.Context,
	scannerKey string,
	owner string,
	token string,
	now time.Time,
	ttl time.Duration,
) (bool, error) {
	if ttl <= 0 {
		return false, fmt.Errorf("web3 scanner lease TTL must be positive")
	}

	updated, err := r.client.Web3ScannerCursor.Update().
		Where(
			web3scannercursor.ScannerKeyEQ(scannerKey),
			web3scannercursor.Or(
				web3scannercursor.LeaseExpiresAtIsNil(),
				web3scannercursor.LeaseExpiresAtLTE(now),
			),
		).
		SetLeaseOwner(owner).
		SetLeaseToken(token).
		SetLeaseExpiresAt(now.Add(ttl)).
		Save(ctx)
	if err != nil {
		return false, fmt.Errorf("acquire web3 scanner lease: %w", err)
	}
	return updated == 1, nil
}

func (r *Web3ScannerCursorRepository) RenewLease(
	ctx context.Context,
	scannerKey string,
	owner string,
	token string,
	now time.Time,
	ttl time.Duration,
) (bool, error) {
	if ttl <= 0 {
		return false, fmt.Errorf("web3 scanner lease TTL must be positive")
	}

	updated, err := r.client.Web3ScannerCursor.Update().
		Where(
			web3scannercursor.ScannerKeyEQ(scannerKey),
			web3scannercursor.LeaseOwnerEQ(owner),
			web3scannercursor.LeaseTokenEQ(token),
			web3scannercursor.LeaseExpiresAtGT(now),
		).
		SetLeaseExpiresAt(now.Add(ttl)).
		Save(ctx)
	if err != nil {
		return false, fmt.Errorf("renew web3 scanner lease: %w", err)
	}
	return updated == 1, nil
}

func (r *Web3ScannerCursorRepository) ReleaseLease(
	ctx context.Context,
	scannerKey string,
	owner string,
	token string,
) (bool, error) {
	updated, err := r.client.Web3ScannerCursor.Update().
		Where(
			web3scannercursor.ScannerKeyEQ(scannerKey),
			web3scannercursor.LeaseOwnerEQ(owner),
			web3scannercursor.LeaseTokenEQ(token),
		).
		ClearLeaseOwner().
		ClearLeaseToken().
		ClearLeaseExpiresAt().
		Save(ctx)
	if err != nil {
		return false, fmt.Errorf("release web3 scanner lease: %w", err)
	}
	return updated == 1, nil
}

func (r *Web3ScannerCursorRepository) AdvanceScanner(
	ctx context.Context,
	scannerKey string,
	token string,
	blockNumber uint64,
	now time.Time,
) error {
	storedBlock, err := web3DepositUint64ToInt64(blockNumber, "scanner block number")
	if err != nil {
		return err
	}

	updated, err := r.client.Web3ScannerCursor.Update().
		Where(
			web3scannercursor.ScannerKeyEQ(scannerKey),
			web3scannercursor.LeaseTokenEQ(token),
			web3scannercursor.LeaseExpiresAtGT(now),
			web3scannercursor.LastScannedBlockLTE(storedBlock),
		).
		SetLastScannedBlock(storedBlock).
		SetLastSuccessAt(now).
		ClearLastError().
		Save(ctx)
	if err != nil {
		return fmt.Errorf("advance web3 scanner cursor: %w", err)
	}
	if updated == 1 {
		return nil
	}
	return r.classifyAdvanceRejection(ctx, scannerKey, token, storedBlock, now, false)
}

func (r *Web3ScannerCursorRepository) AdvanceFinalizer(
	ctx context.Context,
	scannerKey string,
	token string,
	blockNumber uint64,
	now time.Time,
) error {
	storedBlock, err := web3DepositUint64ToInt64(blockNumber, "finalizer block number")
	if err != nil {
		return err
	}

	updated, err := r.client.Web3ScannerCursor.Update().
		Where(
			web3scannercursor.ScannerKeyEQ(scannerKey),
			web3scannercursor.LeaseTokenEQ(token),
			web3scannercursor.LeaseExpiresAtGT(now),
			web3scannercursor.LastFinalizedBlockLTE(storedBlock),
			web3scannercursor.LastScannedBlockGTE(storedBlock),
		).
		SetLastFinalizedBlock(storedBlock).
		SetLastSuccessAt(now).
		ClearLastError().
		Save(ctx)
	if err != nil {
		return fmt.Errorf("advance web3 finalizer cursor: %w", err)
	}
	if updated == 1 {
		return nil
	}
	return r.classifyAdvanceRejection(ctx, scannerKey, token, storedBlock, now, true)
}

func (r *Web3ScannerCursorRepository) RecordError(
	ctx context.Context,
	scannerKey string,
	token string,
	now time.Time,
	message string,
) error {
	updated, err := r.client.Web3ScannerCursor.Update().
		Where(
			web3scannercursor.ScannerKeyEQ(scannerKey),
			web3scannercursor.LeaseTokenEQ(token),
			web3scannercursor.LeaseExpiresAtGT(now),
		).
		SetLastError(message).
		Save(ctx)
	if err != nil {
		return fmt.Errorf("record web3 scanner error: %w", err)
	}
	if updated != 1 {
		return web3deposit.ErrLeaseNotHeld
	}
	return nil
}

func (r *Web3ScannerCursorRepository) classifyAdvanceRejection(
	ctx context.Context,
	scannerKey string,
	token string,
	blockNumber int64,
	now time.Time,
	finalizer bool,
) error {
	entity, err := r.getEntityByKey(ctx, scannerKey)
	if dbent.IsNotFound(err) {
		return web3deposit.ErrCursorNotFound
	}
	if err != nil {
		return fmt.Errorf("classify web3 scanner cursor advance rejection: %w", err)
	}
	if entity.LeaseToken == nil ||
		*entity.LeaseToken != token ||
		entity.LeaseExpiresAt == nil ||
		!entity.LeaseExpiresAt.After(now) {
		return web3deposit.ErrLeaseNotHeld
	}
	if finalizer {
		if blockNumber < entity.LastFinalizedBlock {
			return web3deposit.ErrCursorWouldRegress
		}
		if blockNumber > entity.LastScannedBlock {
			return web3deposit.ErrFinalizerAheadOfScanner
		}
	} else if blockNumber < entity.LastScannedBlock {
		return web3deposit.ErrCursorWouldRegress
	}
	return web3deposit.ErrCursorAdvanceRejected
}

func (r *Web3ScannerCursorRepository) getEntityByKey(ctx context.Context, scannerKey string) (*dbent.Web3ScannerCursor, error) {
	return r.client.Web3ScannerCursor.Query().
		Where(web3scannercursor.ScannerKeyEQ(scannerKey)).
		Only(ctx)
}

func web3ScannerCursorFromEnt(entity *dbent.Web3ScannerCursor) web3deposit.ScannerCursor {
	return web3deposit.ScannerCursor{
		ID:                 entity.ID,
		ScannerKey:         entity.ScannerKey,
		ChainID:            uint64(entity.ChainID),
		TokenContract:      entity.TokenContract,
		ScanStartBlock:     uint64(entity.ScanStartBlock),
		LastScannedBlock:   uint64(entity.LastScannedBlock),
		LastFinalizedBlock: uint64(entity.LastFinalizedBlock),
		LeaseOwner:         entity.LeaseOwner,
		LeaseToken:         entity.LeaseToken,
		LeaseExpiresAt:     entity.LeaseExpiresAt,
		LastError:          entity.LastError,
		LastSuccessAt:      entity.LastSuccessAt,
		CreatedAt:          entity.CreatedAt,
		UpdatedAt:          entity.UpdatedAt,
	}
}
