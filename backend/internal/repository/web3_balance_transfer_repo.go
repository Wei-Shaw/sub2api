package repository

import (
	"context"
	"fmt"
	"strings"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/web3balancetransfer"
	"github.com/Wei-Shaw/sub2api/internal/web3deposit"
)

type Web3BalanceTransferRepository struct {
	client *dbent.Client
}

func NewWeb3BalanceTransferRepository(client *dbent.Client) *Web3BalanceTransferRepository {
	return &Web3BalanceTransferRepository{client: client}
}

func (r *Web3BalanceTransferRepository) Create(ctx context.Context, transfer web3deposit.BalanceTransfer) (web3deposit.BalanceTransfer, error) {
	create := r.client.Web3BalanceTransfer.Create().
		SetUserID(transfer.UserID).
		SetWeb3BalanceID(transfer.Web3BalanceID).
		SetAmount(transfer.Amount).
		SetWeb3BalanceBefore(transfer.Web3BalanceBefore).
		SetWeb3BalanceAfter(transfer.Web3BalanceAfter).
		SetUserBalanceBefore(transfer.UserBalanceBefore).
		SetUserBalanceAfter(transfer.UserBalanceAfter).
		SetIdempotencyKey(transfer.IdempotencyKey)
	if transfer.Metadata != nil {
		create.SetMetadata(transfer.Metadata)
	}
	if !transfer.CreatedAt.IsZero() {
		create.SetCreatedAt(transfer.CreatedAt)
	}

	entity, err := create.Save(ctx)
	if isWeb3BalanceTransferIdempotencyConstraint(err) {
		return web3deposit.BalanceTransfer{}, web3deposit.ErrTransferAlreadyExists
	}
	if err != nil {
		return web3deposit.BalanceTransfer{}, fmt.Errorf("create web3 balance transfer: %w", err)
	}
	return web3BalanceTransferFromEnt(entity), nil
}

func isWeb3BalanceTransferIdempotencyConstraint(err error) bool {
	if !dbent.IsConstraintError(err) {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "unique") && strings.Contains(message, "idempotency_key")
}

func (r *Web3BalanceTransferRepository) GetByIdempotencyKey(ctx context.Context, idempotencyKey string) (web3deposit.BalanceTransfer, error) {
	entity, err := r.client.Web3BalanceTransfer.Query().
		Where(web3balancetransfer.IdempotencyKeyEQ(idempotencyKey)).
		Only(ctx)
	return web3BalanceTransferResult(entity, err, "get web3 balance transfer by idempotency key")
}

func (r *Web3BalanceTransferRepository) ListByUser(ctx context.Context, userID int64) ([]web3deposit.BalanceTransfer, error) {
	entities, err := r.client.Web3BalanceTransfer.Query().
		Where(web3balancetransfer.UserIDEQ(userID)).
		Order(
			dbent.Desc(web3balancetransfer.FieldCreatedAt),
			dbent.Desc(web3balancetransfer.FieldID),
		).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("list web3 balance transfers by user: %w", err)
	}

	transfers := make([]web3deposit.BalanceTransfer, 0, len(entities))
	for _, entity := range entities {
		transfers = append(transfers, web3BalanceTransferFromEnt(entity))
	}
	return transfers, nil
}

func web3BalanceTransferResult(entity *dbent.Web3BalanceTransfer, err error, operation string) (web3deposit.BalanceTransfer, error) {
	if dbent.IsNotFound(err) {
		return web3deposit.BalanceTransfer{}, web3deposit.ErrTransferNotFound
	}
	if err != nil {
		return web3deposit.BalanceTransfer{}, fmt.Errorf("%s: %w", operation, err)
	}
	return web3BalanceTransferFromEnt(entity), nil
}

func web3BalanceTransferFromEnt(entity *dbent.Web3BalanceTransfer) web3deposit.BalanceTransfer {
	return web3deposit.BalanceTransfer{
		ID:                entity.ID,
		UserID:            entity.UserID,
		Web3BalanceID:     entity.Web3BalanceID,
		Amount:            entity.Amount,
		Web3BalanceBefore: entity.Web3BalanceBefore,
		Web3BalanceAfter:  entity.Web3BalanceAfter,
		UserBalanceBefore: entity.UserBalanceBefore,
		UserBalanceAfter:  entity.UserBalanceAfter,
		IdempotencyKey:    entity.IdempotencyKey,
		Metadata:          entity.Metadata,
		CreatedAt:         entity.CreatedAt,
	}
}
