package web3deposit

import (
	"context"
	"errors"
	"time"
)

var (
	ErrDepositNotCreditable    = errors.New("web3 deposit is not creditable")
	ErrCreditClaimLost         = errors.New("web3 deposit credit claim is no longer held")
	ErrUserNotCreditable       = errors.New("web3 deposit user is not creditable")
	ErrTransferAmountInvalid   = errors.New("web3 balance transfer amount is invalid")
	ErrInsufficientWeb3Balance = errors.New("insufficient web3 balance")
	ErrIdempotencyKeyRequired  = errors.New("web3 balance transfer idempotency key is required")
)

type CreditDepositRequest struct {
	DepositID    int64
	ClaimVersion int32
	Now          time.Time
}

type CreditDepositResult struct {
	DepositID       int64
	UserID          int64
	Amount          string
	BalanceBefore   string
	BalanceAfter    string
	AlreadyCredited bool
}

type TransferToMainBalanceRequest struct {
	UserID         int64
	AssetKey       string
	Amount         string
	IdempotencyKey string
	Metadata       map[string]any
	Now            time.Time
}

type TransferToMainBalanceResult struct {
	Transfer    BalanceTransfer
	AlreadyDone bool
}

type AccountingStore interface {
	CreditDeposit(ctx context.Context, request CreditDepositRequest) (CreditDepositResult, error)
	TransferToMainBalance(ctx context.Context, request TransferToMainBalanceRequest) (TransferToMainBalanceResult, error)
}
