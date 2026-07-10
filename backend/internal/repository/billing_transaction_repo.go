package repository

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/shopspring/decimal"
)

const billingTransactionColumns = `
	id, transaction_key, source_type, source_id, transaction_kind, user_id,
	api_key_id, account_id, subscription_id, reservation_id,
	amount_original::text, currency_original, amount_usd::text,
	exchange_rate::text, exchange_rate_as_of, pricing_source, pricing_version,
	balance_before::text, balance_after::text, metadata, created_at`

type billingTransactionRepository struct {
	db *sql.DB
}

func NewBillingTransactionRepository(db *sql.DB) service.BillingTransactionRepository {
	return &billingTransactionRepository{db: db}
}

func (r *billingTransactionRepository) Append(ctx context.Context, input *service.BillingTransaction) (*service.BillingTransaction, error) {
	return appendBillingTransactionWith(ctx, r.db, input)
}

// appendBillingTransactionInTx is the transaction-composition primitive used
// by future repository-owned finalization transactions. It intentionally stays
// package-private so database/sql does not leak into the service contract.
func appendBillingTransactionInTx(ctx context.Context, tx *sql.Tx, input *service.BillingTransaction) (*service.BillingTransaction, error) {
	return appendBillingTransactionWith(ctx, tx, input)
}

func appendBillingTransactionWith(ctx context.Context, queryer sqlQueryRower, input *service.BillingTransaction) (*service.BillingTransaction, error) {
	metadata, err := validateBillingTransactionInput(input)
	if err != nil {
		return nil, err
	}

	row := queryer.QueryRowContext(ctx, `
		INSERT INTO billing_transactions (
			transaction_key, source_type, source_id, transaction_kind, user_id,
			api_key_id, account_id, subscription_id, reservation_id,
			amount_original, currency_original, amount_usd,
			exchange_rate, exchange_rate_as_of, pricing_source, pricing_version,
			balance_before, balance_after, metadata
		)
		VALUES (
			$1, $2, $3, $4, $5,
			$6, $7, $8, $9,
			$10, $11, $12,
			$13, $14, $15, $16,
			$17, $18, $19::jsonb
		)
		ON CONFLICT (transaction_key) DO NOTHING
		RETURNING `+billingTransactionColumns,
		input.TransactionKey,
		input.SourceType,
		input.SourceID,
		input.TransactionKind,
		input.UserID,
		input.APIKeyID,
		input.AccountID,
		input.SubscriptionID,
		input.ReservationID,
		input.AmountOriginal.String(),
		string(input.AmountOriginal.Currency()),
		input.AmountUSD.String(),
		input.ExchangeRate.StringFixed(10),
		input.ExchangeRateAsOf.Truncate(time.Microsecond),
		input.PricingSource,
		input.PricingVersion,
		input.BalanceBefore.String(),
		input.BalanceAfter.String(),
		string(metadata),
	)
	inserted, err := scanBillingTransaction(row)
	if err == nil {
		return inserted, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}

	existing, err := getBillingTransactionByKeyWith(ctx, queryer, input.TransactionKey)
	if err != nil {
		return nil, err
	}
	if !sameBillingTransactionRequest(existing, input) {
		return nil, service.ErrBillingTransactionConflict
	}
	return existing, nil
}

func (r *billingTransactionRepository) GetByID(ctx context.Context, id int64) (*service.BillingTransaction, error) {
	item, err := scanBillingTransaction(r.db.QueryRowContext(ctx, `
		SELECT `+billingTransactionColumns+`
		FROM billing_transactions
		WHERE id = $1
	`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, service.ErrBillingTransactionNotFound.WithCause(err)
	}
	return item, err
}

func (r *billingTransactionRepository) GetByKey(ctx context.Context, transactionKey string) (*service.BillingTransaction, error) {
	item, err := getBillingTransactionByKeyWith(ctx, r.db, transactionKey)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, service.ErrBillingTransactionNotFound.WithCause(err)
	}
	return item, err
}

func getBillingTransactionByKeyWith(ctx context.Context, queryer sqlQueryRower, transactionKey string) (*service.BillingTransaction, error) {
	return scanBillingTransaction(queryer.QueryRowContext(ctx, `
		SELECT `+billingTransactionColumns+`
		FROM billing_transactions
		WHERE transaction_key = $1
	`, transactionKey))
}

func (r *billingTransactionRepository) ListBySource(ctx context.Context, sourceType string, sourceID int64, limit int) ([]*service.BillingTransaction, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := r.db.QueryContext(ctx, `
		SELECT `+billingTransactionColumns+`
		FROM billing_transactions
		WHERE source_type = $1 AND source_id = $2
		ORDER BY created_at ASC, id ASC
		LIMIT $3
	`, sourceType, sourceID, limit)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	items := make([]*service.BillingTransaction, 0, limit)
	for rows.Next() {
		item, err := scanBillingTransaction(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

func scanBillingTransaction(scanner sqlRowScanner) (*service.BillingTransaction, error) {
	var (
		item                                           service.BillingTransaction
		apiKeyID, accountID, subscriptionID, reserveID sql.NullInt64
		originalRaw, originalCurrency, usdRaw          string
		exchangeRateRaw, beforeRaw, afterRaw           string
		metadataRaw                                    []byte
	)
	if err := scanner.Scan(
		&item.ID,
		&item.TransactionKey,
		&item.SourceType,
		&item.SourceID,
		&item.TransactionKind,
		&item.UserID,
		&apiKeyID,
		&accountID,
		&subscriptionID,
		&reserveID,
		&originalRaw,
		&originalCurrency,
		&usdRaw,
		&exchangeRateRaw,
		&item.ExchangeRateAsOf,
		&item.PricingSource,
		&item.PricingVersion,
		&beforeRaw,
		&afterRaw,
		&metadataRaw,
		&item.CreatedAt,
	); err != nil {
		return nil, err
	}

	original, err := service.NewMoney(originalRaw, service.Currency(originalCurrency))
	if err != nil {
		return nil, fmt.Errorf("decode original amount: %w", err)
	}
	usd, err := service.NewMoney(usdRaw, service.CurrencyUSD)
	if err != nil {
		return nil, fmt.Errorf("decode USD amount: %w", err)
	}
	before, err := service.NewMoney(beforeRaw, service.CurrencyUSD)
	if err != nil {
		return nil, fmt.Errorf("decode balance before: %w", err)
	}
	after, err := service.NewMoney(afterRaw, service.CurrencyUSD)
	if err != nil {
		return nil, fmt.Errorf("decode balance after: %w", err)
	}
	rate, err := decimal.NewFromString(exchangeRateRaw)
	if err != nil {
		return nil, fmt.Errorf("decode exchange rate: %w", err)
	}

	item.APIKeyID = nullableInt64(apiKeyID)
	item.AccountID = nullableInt64(accountID)
	item.SubscriptionID = nullableInt64(subscriptionID)
	item.ReservationID = nullableInt64(reserveID)
	item.AmountOriginal = original
	item.AmountUSD = usd
	item.ExchangeRate = rate
	item.BalanceBefore = before
	item.BalanceAfter = after
	item.Metadata = append(json.RawMessage(nil), metadataRaw...)
	return &item, nil
}

func validateBillingTransactionInput(input *service.BillingTransaction) (json.RawMessage, error) {
	if input == nil {
		return nil, fmt.Errorf("billing transaction is required")
	}
	if strings.TrimSpace(input.TransactionKey) == "" {
		return nil, fmt.Errorf("transaction key is required")
	}
	if input.UserID <= 0 {
		return nil, fmt.Errorf("transaction user is required")
	}
	if input.AmountOriginal.Currency() == "" {
		return nil, fmt.Errorf("original amount currency is required")
	}
	if input.AmountUSD.Currency() != service.CurrencyUSD ||
		input.BalanceBefore.Currency() != service.CurrencyUSD ||
		input.BalanceAfter.Currency() != service.CurrencyUSD {
		return nil, fmt.Errorf("USD amount and balance fields must use USD")
	}
	if input.ExchangeRate.IsZero() || input.ExchangeRate.IsNegative() {
		return nil, fmt.Errorf("exchange rate must be positive")
	}
	if input.ExchangeRate.Exponent() < -10 {
		return nil, fmt.Errorf("exchange rate must not have more than 10 decimal places")
	}
	if input.ExchangeRateAsOf.IsZero() {
		return nil, fmt.Errorf("exchange rate timestamp is required")
	}
	metadata := input.Metadata
	if len(metadata) == 0 {
		metadata = json.RawMessage(`{}`)
	}
	if !json.Valid(metadata) {
		return nil, fmt.Errorf("transaction metadata must be valid JSON")
	}
	return metadata, nil
}

func sameBillingTransactionRequest(stored, input *service.BillingTransaction) bool {
	if stored == nil || input == nil {
		return false
	}
	return stored.TransactionKey == input.TransactionKey &&
		stored.SourceType == input.SourceType &&
		stored.SourceID == input.SourceID &&
		stored.TransactionKind == input.TransactionKind &&
		stored.UserID == input.UserID &&
		sameOptionalInt64(stored.APIKeyID, input.APIKeyID) &&
		sameOptionalInt64(stored.AccountID, input.AccountID) &&
		sameOptionalInt64(stored.SubscriptionID, input.SubscriptionID) &&
		sameOptionalInt64(stored.ReservationID, input.ReservationID) &&
		stored.AmountOriginal.Currency() == input.AmountOriginal.Currency() &&
		stored.AmountOriginal.Decimal().Equal(input.AmountOriginal.Decimal()) &&
		stored.AmountUSD.Decimal().Equal(input.AmountUSD.Decimal()) &&
		stored.ExchangeRate.Equal(input.ExchangeRate) &&
		stored.ExchangeRateAsOf.Equal(input.ExchangeRateAsOf.Truncate(time.Microsecond)) &&
		stored.PricingSource == input.PricingSource &&
		stored.PricingVersion == input.PricingVersion &&
		stored.BalanceBefore.Decimal().Equal(input.BalanceBefore.Decimal()) &&
		stored.BalanceAfter.Decimal().Equal(input.BalanceAfter.Decimal()) &&
		jsonValuesEqual(stored.Metadata, input.Metadata)
}

func jsonValuesEqual(left, right json.RawMessage) bool {
	if len(left) == 0 {
		left = json.RawMessage(`{}`)
	}
	if len(right) == 0 {
		right = json.RawMessage(`{}`)
	}
	var leftValue, rightValue any
	leftDecoder := json.NewDecoder(bytes.NewReader(left))
	leftDecoder.UseNumber()
	rightDecoder := json.NewDecoder(bytes.NewReader(right))
	rightDecoder.UseNumber()
	if leftDecoder.Decode(&leftValue) != nil || rightDecoder.Decode(&rightValue) != nil {
		return false
	}
	return jsonDecodedValuesEqual(leftValue, rightValue)
}

func jsonDecodedValuesEqual(left, right any) bool {
	switch leftValue := left.(type) {
	case nil:
		return right == nil
	case json.Number:
		rightValue, ok := right.(json.Number)
		if !ok {
			return false
		}
		leftDecimal, leftErr := decimal.NewFromString(leftValue.String())
		rightDecimal, rightErr := decimal.NewFromString(rightValue.String())
		return leftErr == nil && rightErr == nil && leftDecimal.Equal(rightDecimal)
	case string:
		rightValue, ok := right.(string)
		return ok && leftValue == rightValue
	case bool:
		rightValue, ok := right.(bool)
		return ok && leftValue == rightValue
	case []any:
		rightValue, ok := right.([]any)
		if !ok || len(leftValue) != len(rightValue) {
			return false
		}
		for index := range leftValue {
			if !jsonDecodedValuesEqual(leftValue[index], rightValue[index]) {
				return false
			}
		}
		return true
	case map[string]any:
		rightValue, ok := right.(map[string]any)
		if !ok || len(leftValue) != len(rightValue) {
			return false
		}
		for key, leftItem := range leftValue {
			rightItem, exists := rightValue[key]
			if !exists || !jsonDecodedValuesEqual(leftItem, rightItem) {
				return false
			}
		}
		return true
	default:
		return false
	}
}
