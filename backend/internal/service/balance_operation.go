package service

import (
	"context"
	"errors"
)

// BalanceOperation domain 模型。external_op_id 是外部平台幂等键。
type BalanceOperation struct {
	ID             int64
	ExternalOpID   string
	UserID         int64
	OpType         string
	Amount         float64
	BalanceBefore  float64
	BalanceAfter   float64
	Status         string
	FailureReason  *string
	Note           *string
	RequestPayload map[string]any
}

// BalanceOperationRepository repository 接口。
type BalanceOperationRepository interface {
	FindByExternalOpID(ctx context.Context, externalOpID string) (*BalanceOperation, error)
	CreatePending(ctx context.Context, op BalanceOperation) (*BalanceOperation, error)
	MarkSucceeded(ctx context.Context, id int64, balanceBefore, balanceAfter float64) error
	MarkFailed(ctx context.Context, id int64, reason string) error
}

// ErrDuplicateExternalOpID 当 external_op_id 已存在时返回。
var ErrDuplicateExternalOpID = errors.New("duplicate external_op_id")
