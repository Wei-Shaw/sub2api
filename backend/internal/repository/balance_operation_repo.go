package repository

import (
	"context"
	"fmt"

	"github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/balanceoperation"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

type balanceOperationRepository struct {
	client *ent.Client
}

// NewBalanceOperationRepository 构造函数。
func NewBalanceOperationRepository(client *ent.Client) service.BalanceOperationRepository {
	return &balanceOperationRepository{client: client}
}

func (r *balanceOperationRepository) FindByExternalOpID(ctx context.Context, externalOpID string) (*service.BalanceOperation, error) {
	row, err := r.client.BalanceOperation.Query().
		Where(balanceoperation.ExternalOpIDEQ(externalOpID)).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("balance op lookup: %w", err)
	}
	return entToBalanceOp(row), nil
}

func (r *balanceOperationRepository) CreatePending(ctx context.Context, op service.BalanceOperation) (*service.BalanceOperation, error) {
	create := r.client.BalanceOperation.Create().
		SetExternalOpID(op.ExternalOpID).
		SetUserID(op.UserID).
		SetOpType(op.OpType).
		SetAmount(op.Amount).
		SetStatus("pending")
	if op.Note != nil && *op.Note != "" {
		create.SetNote(*op.Note)
	}
	if op.RequestPayload != nil {
		create.SetRequestPayload(op.RequestPayload)
	}
	row, err := create.Save(ctx)
	if err != nil {
		if ent.IsConstraintError(err) {
			return nil, service.ErrDuplicateExternalOpID
		}
		return nil, fmt.Errorf("balance op create: %w", err)
	}
	return entToBalanceOp(row), nil
}

func (r *balanceOperationRepository) MarkSucceeded(ctx context.Context, id int64, balanceBefore, balanceAfter float64) error {
	_, err := r.client.BalanceOperation.UpdateOneID(id).
		SetStatus("succeeded").
		SetBalanceBefore(balanceBefore).
		SetBalanceAfter(balanceAfter).
		Save(ctx)
	if err != nil {
		return fmt.Errorf("balance op mark succeeded: %w", err)
	}
	return nil
}

func (r *balanceOperationRepository) MarkFailed(ctx context.Context, id int64, reason string) error {
	_, err := r.client.BalanceOperation.UpdateOneID(id).
		SetStatus("failed").
		SetFailureReason(reason).
		Save(ctx)
	if err != nil {
		return fmt.Errorf("balance op mark failed: %w", err)
	}
	return nil
}

func entToBalanceOp(row *ent.BalanceOperation) *service.BalanceOperation {
	op := &service.BalanceOperation{
		ID:            row.ID,
		ExternalOpID:  row.ExternalOpID,
		UserID:        row.UserID,
		OpType:        row.OpType,
		Amount:        row.Amount,
		BalanceBefore: row.BalanceBefore,
		BalanceAfter:  row.BalanceAfter,
		Status:        row.Status,
	}
	if row.FailureReason != nil {
		op.FailureReason = row.FailureReason
	}
	if row.Note != nil {
		op.Note = row.Note
	}
	if row.RequestPayload != nil {
		op.RequestPayload = row.RequestPayload
	}
	return op
}
