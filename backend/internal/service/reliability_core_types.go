package service

import (
	"context"
	"encoding/json"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/shopspring/decimal"
)

const (
	BillingReservationStatusActive         = "active"
	BillingReservationStatusSettled        = "settled"
	BillingReservationStatusReleased       = "released"
	BillingReservationStatusExpired        = "expired"
	BillingReservationStatusReviewRequired = "review_required"

	DomainOutboxStatusPending    = "pending"
	DomainOutboxStatusProcessing = "processing"
	DomainOutboxStatusCompleted  = "completed"
	DomainOutboxStatusDead       = "dead"

	DomainOutboxMaxErrorSummaryBytes = 1024
)

var (
	ErrBillingReservationNotFound = infraerrors.NotFound(
		"BILLING_RESERVATION_NOT_FOUND",
		"billing reservation not found",
	)
	ErrBillingReservationConflict = infraerrors.Conflict(
		"BILLING_RESERVATION_CONFLICT",
		"reservation key reused with different fields",
	)
	ErrBillingReservationBalanceUnavailable = infraerrors.ServiceUnavailable(
		"BILLING_BALANCE_UNAVAILABLE",
		"billing balance is unavailable",
	)
	ErrBillingReservationInsufficientBalance = infraerrors.Conflict(
		"BILLING_BALANCE_INSUFFICIENT",
		"available balance is insufficient for reservation",
	)
	ErrBillingTransactionNotFound = infraerrors.NotFound(
		"BILLING_TRANSACTION_NOT_FOUND",
		"billing transaction not found",
	)
	ErrBillingTransactionConflict = infraerrors.Conflict(
		"BILLING_TRANSACTION_CONFLICT",
		"transaction key reused with different fields",
	)
	ErrDomainOutboxNotFound = infraerrors.NotFound(
		"DOMAIN_OUTBOX_EVENT_NOT_FOUND",
		"domain outbox event not found",
	)
	ErrDomainOutboxConflict = infraerrors.Conflict(
		"DOMAIN_OUTBOX_CONFLICT",
		"outbox dedup key reused with different fields",
	)
	ErrDomainOutboxLeaseConflict = infraerrors.Conflict(
		"DOMAIN_OUTBOX_LEASE_CONFLICT",
		"domain outbox lease is not owned by this worker",
	)
)

type BillingReservation struct {
	ID                int64
	ReservationKey    string
	SourceType        string
	SourceID          int64
	UserID            int64
	APIKeyID          *int64
	ReservedAmountUSD Money
	SettledAmountUSD  Money
	Status            string
	ExpiresAt         time.Time
	CreatedAt         time.Time
	UpdatedAt         time.Time
	SettledAt         *time.Time
	ReleasedAt        *time.Time
}

type BillingTransaction struct {
	ID               int64
	TransactionKey   string
	SourceType       string
	SourceID         int64
	TransactionKind  string
	UserID           int64
	APIKeyID         *int64
	AccountID        *int64
	SubscriptionID   *int64
	ReservationID    *int64
	AmountOriginal   Money
	AmountUSD        Money
	ExchangeRate     decimal.Decimal
	ExchangeRateAsOf time.Time
	PricingSource    string
	PricingVersion   string
	BalanceBefore    Money
	BalanceAfter     Money
	Metadata         json.RawMessage
	CreatedAt        time.Time
}

type DomainOutboxEvent struct {
	ID            int64
	AggregateType string
	AggregateID   int64
	EventType     string
	DedupKey      string
	Payload       json.RawMessage
	Status        string
	AttemptCount  int
	NextAttemptAt time.Time
	LockedAt      *time.Time
	LockedUntil   *time.Time
	LockedBy      *string
	LastError     *string
	CreatedAt     time.Time
	CompletedAt   *time.Time
}

type DomainOutboxCounts struct {
	Pending    int64
	Processing int64
	Completed  int64
	Dead       int64
}

type BillingReservationRepository interface {
	Reserve(ctx context.Context, reservation *BillingReservation) (*BillingReservation, error)
	GetByID(ctx context.Context, id int64) (*BillingReservation, error)
	GetByKey(ctx context.Context, reservationKey string) (*BillingReservation, error)
	ListExpired(ctx context.Context, now time.Time, limit int) ([]*BillingReservation, error)
}

type BillingTransactionRepository interface {
	Append(ctx context.Context, transaction *BillingTransaction) (*BillingTransaction, error)
	GetByID(ctx context.Context, id int64) (*BillingTransaction, error)
	GetByKey(ctx context.Context, transactionKey string) (*BillingTransaction, error)
	ListBySource(ctx context.Context, sourceType string, sourceID int64, limit int) ([]*BillingTransaction, error)
}

type DomainOutboxRepository interface {
	Enqueue(ctx context.Context, event *DomainOutboxEvent) (*DomainOutboxEvent, error)
	GetByID(ctx context.Context, id int64) (*DomainOutboxEvent, error)
	ClaimBatch(ctx context.Context, workerID string, now time.Time, limit int, lease time.Duration) ([]*DomainOutboxEvent, error)
	Complete(ctx context.Context, id int64, workerID string, completedAt time.Time) (bool, error)
	Retry(ctx context.Context, id int64, workerID string, nextAttemptAt time.Time, dead bool, lastError string) (bool, error)
	ReapExpiredLeases(ctx context.Context, now time.Time, limit int) (int64, error)
	Counts(ctx context.Context) (DomainOutboxCounts, error)
}

// VideoOutboxSideEffectRepository is implemented by the SQL video repository.
// Completion/dead-letter updates are deliberately repository-owned transactions
// so a side-effect CAS can never be committed without its outbox state change.
type VideoOutboxSideEffectRepository interface {
	VideoGatewayRepository
	CompleteVideoOutboxSideEffect(ctx context.Context, eventID int64, workerID string, completedAt time.Time, taskID int64, effect string) (bool, error)
	DeadVideoOutboxSideEffect(ctx context.Context, eventID int64, workerID string, nextAttemptAt time.Time, taskID int64, effect string, lastError string) (bool, error)
}
