package web3deposit

import (
	"context"
	"errors"
	"time"
)

var (
	ErrDepositNotFound      = errors.New("web3 deposit not found")
	ErrDepositAlreadyExists = errors.New("web3 deposit already exists")
)

type Deposit struct {
	ID               int64
	UserID           int64
	DepositAddressID int64
	ChainID          uint64
	TokenContract    string
	TxHash           string
	LogIndex         uint64
	BlockNumber      uint64
	BlockHash        string
	FromAddress      string
	ToAddress        string
	RawAmount        string
	TokenDecimals    int32
	TokenAmount      string
	CreditedAmount   *string
	Status           DepositStatus
	ReviewReason     *string
	FailureReason    *string
	RetryCount       int32
	NextRetryAt      *time.Time
	DetectedAt       time.Time
	FinalizedAt      *time.Time
	CreditedAt       *time.Time
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

type UserDepositFilter struct {
	ChainID       uint64
	TokenContract string
	Page          int
	PageSize      int
}

type UserDepositReader interface {
	ListUserDeposits(ctx context.Context, userID int64, filter UserDepositFilter) ([]Deposit, int64, error)
	GetUserDeposit(ctx context.Context, userID, depositID int64) (Deposit, error)
}
