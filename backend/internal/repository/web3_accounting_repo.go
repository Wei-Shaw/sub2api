package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/Wei-Shaw/sub2api/internal/web3deposit"
	"github.com/shopspring/decimal"
)

type Web3AccountingRepository struct {
	db *sql.DB
}

var _ web3deposit.AccountingStore = (*Web3AccountingRepository)(nil)

func NewWeb3AccountingRepository(db *sql.DB) *Web3AccountingRepository {
	return &Web3AccountingRepository{db: db}
}

func (r *Web3AccountingRepository) CreditDeposit(ctx context.Context, request web3deposit.CreditDepositRequest) (web3deposit.CreditDepositResult, error) {
	if request.Now.IsZero() {
		request.Now = time.Now()
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return web3deposit.CreditDepositResult{}, fmt.Errorf("begin web3 deposit credit transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var depositID, userID int64
	var tokenAmount, creditedAmount, status, userStatus string
	var retryCount int32
	var userDeletedAt sql.NullTime
	err = tx.QueryRowContext(ctx, `
		SELECT d.id, d.user_id, d.token_amount::text, COALESCE(d.credited_amount::text, ''),
		       d.status, d.retry_count, u.status, u.deleted_at
		FROM web3_deposits d
		JOIN users u ON u.id = d.user_id
		WHERE d.id = $1
		FOR UPDATE OF d, u
	`, request.DepositID).Scan(&depositID, &userID, &tokenAmount, &creditedAmount, &status, &retryCount, &userStatus, &userDeletedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return web3deposit.CreditDepositResult{}, web3deposit.ErrDepositNotFound
	}
	if err != nil {
		return web3deposit.CreditDepositResult{}, fmt.Errorf("lock web3 deposit for credit: %w", err)
	}
	if status == string(web3deposit.DepositStatusCredited) {
		result, err := loadCreditedDepositResult(ctx, tx, depositID, userID, creditedAmount)
		if err != nil {
			return web3deposit.CreditDepositResult{}, err
		}
		if err := tx.Commit(); err != nil {
			return web3deposit.CreditDepositResult{}, fmt.Errorf("commit idempotent web3 deposit credit: %w", err)
		}
		result.AlreadyCredited = true
		return result, nil
	}
	if request.ClaimVersion > 0 {
		if status != string(web3deposit.DepositStatusCrediting) || retryCount != request.ClaimVersion {
			return web3deposit.CreditDepositResult{}, web3deposit.ErrCreditClaimLost
		}
	} else if status != string(web3deposit.DepositStatusReadyToCredit) {
		return web3deposit.CreditDepositResult{}, web3deposit.ErrDepositNotCreditable
	}
	if userDeletedAt.Valid || userStatus != domain.StatusActive {
		return web3deposit.CreditDepositResult{}, web3deposit.ErrUserNotCreditable
	}
	amount, err := parseAccountingAmount(tokenAmount)
	if err != nil {
		return web3deposit.CreditDepositResult{}, err
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO web3_user_balances (user_id, asset_key)
		VALUES ($1, $2)
		ON CONFLICT (user_id, asset_key) DO NOTHING
	`, userID, web3deposit.AssetKeyUSDT); err != nil {
		return web3deposit.CreditDepositResult{}, fmt.Errorf("ensure web3 user balance: %w", err)
	}
	var balanceID int64
	var availableAmount, totalDeposited string
	err = tx.QueryRowContext(ctx, `
		SELECT id, available_amount::text, total_deposited::text
		FROM web3_user_balances
		WHERE user_id = $1 AND asset_key = $2
		FOR UPDATE
	`, userID, web3deposit.AssetKeyUSDT).Scan(&balanceID, &availableAmount, &totalDeposited)
	if err != nil {
		return web3deposit.CreditDepositResult{}, fmt.Errorf("lock web3 user balance for deposit: %w", err)
	}
	availableBefore, err := decimal.NewFromString(availableAmount)
	if err != nil {
		return web3deposit.CreditDepositResult{}, fmt.Errorf("parse web3 available balance: %w", err)
	}
	totalBefore, err := decimal.NewFromString(totalDeposited)
	if err != nil {
		return web3deposit.CreditDepositResult{}, fmt.Errorf("parse web3 total deposited: %w", err)
	}
	availableAfter := availableBefore.Add(amount)
	totalAfter := totalBefore.Add(amount)
	if _, err := tx.ExecContext(ctx, `
		UPDATE web3_user_balances
		SET available_amount = $2::numeric,
		    total_deposited = $3::numeric,
		    balance_version = balance_version + 1,
		    updated_at = $4
		WHERE id = $1
	`, balanceID, accountingString(availableAfter), accountingString(totalAfter), request.Now); err != nil {
		return web3deposit.CreditDepositResult{}, fmt.Errorf("increase web3 user balance: %w", err)
	}
	result, err := tx.ExecContext(ctx, `
		UPDATE web3_deposits
		SET status = 'credited', credited_amount = $2::numeric, credited_at = $3,
		    next_retry_at = NULL, failure_reason = NULL, updated_at = $3
		WHERE id = $1 AND status = $4 AND ($5 = 0 OR retry_count = $5)
	`, depositID, accountingString(amount), request.Now, status, request.ClaimVersion)
	if err != nil {
		return web3deposit.CreditDepositResult{}, fmt.Errorf("mark web3 deposit credited: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil || affected != 1 {
		return web3deposit.CreditDepositResult{}, web3deposit.ErrCreditClaimLost
	}
	if err := tx.Commit(); err != nil {
		return web3deposit.CreditDepositResult{}, fmt.Errorf("commit web3 deposit credit: %w", err)
	}
	return web3deposit.CreditDepositResult{
		DepositID: depositID, UserID: userID, Amount: accountingString(amount),
		BalanceBefore: accountingString(availableBefore), BalanceAfter: accountingString(availableAfter),
	}, nil
}

func loadCreditedDepositResult(ctx context.Context, tx *sql.Tx, depositID, userID int64, creditedAmount string) (web3deposit.CreditDepositResult, error) {
	var available string
	err := tx.QueryRowContext(ctx, `
		SELECT available_amount::text FROM web3_user_balances
		WHERE user_id = $1 AND asset_key = $2
	`, userID, web3deposit.AssetKeyUSDT).Scan(&available)
	if err != nil {
		return web3deposit.CreditDepositResult{}, fmt.Errorf("load credited web3 balance: %w", err)
	}
	return web3deposit.CreditDepositResult{DepositID: depositID, UserID: userID, Amount: creditedAmount, BalanceAfter: available}, nil
}

func (r *Web3AccountingRepository) TransferToMainBalance(ctx context.Context, request web3deposit.TransferToMainBalanceRequest) (web3deposit.TransferToMainBalanceResult, error) {
	request.IdempotencyKey = strings.TrimSpace(request.IdempotencyKey)
	if request.IdempotencyKey == "" {
		return web3deposit.TransferToMainBalanceResult{}, web3deposit.ErrIdempotencyKeyRequired
	}
	if request.AssetKey == "" {
		request.AssetKey = web3deposit.AssetKeyUSDT
	}
	if request.Now.IsZero() {
		request.Now = time.Now()
	}
	amount, err := parseAccountingAmount(request.Amount)
	if err != nil {
		return web3deposit.TransferToMainBalanceResult{}, web3deposit.ErrTransferAmountInvalid
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return web3deposit.TransferToMainBalanceResult{}, fmt.Errorf("begin web3 balance transfer transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if existing, found, err := findBalanceTransfer(ctx, tx, request.IdempotencyKey); err != nil {
		return web3deposit.TransferToMainBalanceResult{}, err
	} else if found {
		if err := tx.Commit(); err != nil {
			return web3deposit.TransferToMainBalanceResult{}, fmt.Errorf("commit idempotent web3 balance transfer: %w", err)
		}
		return web3deposit.TransferToMainBalanceResult{Transfer: existing, AlreadyDone: true}, nil
	}
	var balanceID int64
	var availableAmount, totalTransferred string
	err = tx.QueryRowContext(ctx, `
		SELECT id, available_amount::text, total_transferred::text
		FROM web3_user_balances
		WHERE user_id = $1 AND asset_key = $2
		FOR UPDATE
	`, request.UserID, request.AssetKey).Scan(&balanceID, &availableAmount, &totalTransferred)
	if errors.Is(err, sql.ErrNoRows) {
		return web3deposit.TransferToMainBalanceResult{}, web3deposit.ErrBalanceNotFound
	}
	if err != nil {
		return web3deposit.TransferToMainBalanceResult{}, fmt.Errorf("lock web3 balance for transfer: %w", err)
	}
	if existing, found, err := findBalanceTransfer(ctx, tx, request.IdempotencyKey); err != nil {
		return web3deposit.TransferToMainBalanceResult{}, err
	} else if found {
		if err := tx.Commit(); err != nil {
			return web3deposit.TransferToMainBalanceResult{}, err
		}
		return web3deposit.TransferToMainBalanceResult{Transfer: existing, AlreadyDone: true}, nil
	}
	availableBefore, err := decimal.NewFromString(availableAmount)
	if err != nil || availableBefore.LessThan(amount) {
		return web3deposit.TransferToMainBalanceResult{}, web3deposit.ErrInsufficientWeb3Balance
	}
	totalBefore, err := decimal.NewFromString(totalTransferred)
	if err != nil {
		return web3deposit.TransferToMainBalanceResult{}, fmt.Errorf("parse web3 transferred total: %w", err)
	}
	var userBalanceRaw, totalRechargedRaw, userStatus string
	var deletedAt sql.NullTime
	err = tx.QueryRowContext(ctx, `
		SELECT balance::text, total_recharged::text, status, deleted_at
		FROM users WHERE id = $1 FOR UPDATE
	`, request.UserID).Scan(&userBalanceRaw, &totalRechargedRaw, &userStatus, &deletedAt)
	if errors.Is(err, sql.ErrNoRows) || deletedAt.Valid || userStatus != domain.StatusActive {
		return web3deposit.TransferToMainBalanceResult{}, web3deposit.ErrUserNotCreditable
	}
	if err != nil {
		return web3deposit.TransferToMainBalanceResult{}, fmt.Errorf("lock user for web3 balance transfer: %w", err)
	}
	userBefore, err := decimal.NewFromString(userBalanceRaw)
	if err != nil {
		return web3deposit.TransferToMainBalanceResult{}, fmt.Errorf("parse user balance: %w", err)
	}
	totalRecharged, err := decimal.NewFromString(totalRechargedRaw)
	if err != nil {
		return web3deposit.TransferToMainBalanceResult{}, fmt.Errorf("parse user total recharged: %w", err)
	}
	availableAfter := availableBefore.Sub(amount)
	userAfter := userBefore.Add(amount)
	if _, err := tx.ExecContext(ctx, `
		UPDATE web3_user_balances SET available_amount = $2::numeric,
		 total_transferred = $3::numeric, balance_version = balance_version + 1, updated_at = $4
		WHERE id = $1
	`, balanceID, accountingString(availableAfter), accountingString(totalBefore.Add(amount)), request.Now); err != nil {
		return web3deposit.TransferToMainBalanceResult{}, fmt.Errorf("decrease web3 balance: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE users SET balance = $2::numeric, total_recharged = $3::numeric, updated_at = $4 WHERE id = $1
	`, request.UserID, accountingString(userAfter), accountingString(totalRecharged.Add(amount)), request.Now); err != nil {
		return web3deposit.TransferToMainBalanceResult{}, fmt.Errorf("increase user balance from web3 transfer: %w", err)
	}
	if request.Metadata == nil {
		request.Metadata = map[string]any{}
	}
	metadata, err := json.Marshal(request.Metadata)
	if err != nil {
		return web3deposit.TransferToMainBalanceResult{}, fmt.Errorf("marshal web3 transfer metadata: %w", err)
	}
	var transfer web3deposit.BalanceTransfer
	err = tx.QueryRowContext(ctx, `
		INSERT INTO web3_balance_transfers
		(user_id, web3_balance_id, amount, web3_balance_before, web3_balance_after,
		 user_balance_before, user_balance_after, idempotency_key, metadata, created_at)
		VALUES ($1,$2,$3::numeric,$4::numeric,$5::numeric,$6::numeric,$7::numeric,$8,$9,$10)
		RETURNING id, created_at
	`, request.UserID, balanceID, accountingString(amount), accountingString(availableBefore),
		accountingString(availableAfter), accountingString(userBefore), accountingString(userAfter),
		request.IdempotencyKey, metadata, request.Now).Scan(&transfer.ID, &transfer.CreatedAt)
	if err != nil {
		return web3deposit.TransferToMainBalanceResult{}, fmt.Errorf("create web3 balance transfer fact: %w", err)
	}
	transfer.UserID = request.UserID
	transfer.Web3BalanceID = balanceID
	transfer.Amount = accountingString(amount)
	transfer.Web3BalanceBefore = accountingString(availableBefore)
	transfer.Web3BalanceAfter = accountingString(availableAfter)
	transfer.UserBalanceBefore = accountingString(userBefore)
	transfer.UserBalanceAfter = accountingString(userAfter)
	transfer.IdempotencyKey = request.IdempotencyKey
	transfer.Metadata = request.Metadata
	if err := tx.Commit(); err != nil {
		return web3deposit.TransferToMainBalanceResult{}, fmt.Errorf("commit web3 balance transfer: %w", err)
	}
	return web3deposit.TransferToMainBalanceResult{Transfer: transfer}, nil
}

func findBalanceTransfer(ctx context.Context, tx *sql.Tx, key string) (web3deposit.BalanceTransfer, bool, error) {
	var transfer web3deposit.BalanceTransfer
	var metadata []byte
	err := tx.QueryRowContext(ctx, `
		SELECT id,user_id,web3_balance_id,amount::text,web3_balance_before::text,web3_balance_after::text,
		 user_balance_before::text,user_balance_after::text,idempotency_key,metadata,created_at
		FROM web3_balance_transfers WHERE idempotency_key = $1
	`, key).Scan(&transfer.ID, &transfer.UserID, &transfer.Web3BalanceID, &transfer.Amount,
		&transfer.Web3BalanceBefore, &transfer.Web3BalanceAfter, &transfer.UserBalanceBefore,
		&transfer.UserBalanceAfter, &transfer.IdempotencyKey, &metadata, &transfer.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return web3deposit.BalanceTransfer{}, false, nil
	}
	if err != nil {
		return web3deposit.BalanceTransfer{}, false, fmt.Errorf("find web3 balance transfer by idempotency key: %w", err)
	}
	_ = json.Unmarshal(metadata, &transfer.Metadata)
	return transfer, true, nil
}

func parseAccountingAmount(raw string) (decimal.Decimal, error) {
	amount, err := decimal.NewFromString(strings.TrimSpace(raw))
	if err != nil || !amount.IsPositive() || !amount.Equal(amount.Truncate(8)) || amount.GreaterThan(decimal.RequireFromString("999999999999.99999999")) {
		return decimal.Zero, web3deposit.ErrTransferAmountInvalid
	}
	return amount, nil
}

func accountingString(value decimal.Decimal) string {
	return value.StringFixed(8)
}
