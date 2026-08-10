package web3deposit

import (
	"context"
	"errors"
	"time"
)

const AssetKeyUSDT = "usdt"

var (
	ErrBalanceNotFound       = errors.New("web3 user balance not found")
	ErrTransferNotFound      = errors.New("web3 balance transfer not found")
	ErrTransferAlreadyExists = errors.New("web3 balance transfer already exists")
)

type UserBalance struct {
	ID               int64
	UserID           int64
	AssetKey         string
	AvailableAmount  string
	TotalDeposited   string
	TotalTransferred string
	BalanceVersion   int64
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

type UserBalanceReader interface {
	ListUserBalances(ctx context.Context, userID int64) ([]UserBalance, error)
}

type BalanceTransfer struct {
	ID                int64
	UserID            int64
	Web3BalanceID     int64
	Amount            string
	Web3BalanceBefore string
	Web3BalanceAfter  string
	UserBalanceBefore string
	UserBalanceAfter  string
	IdempotencyKey    string
	Metadata          map[string]any
	CreatedAt         time.Time
}
