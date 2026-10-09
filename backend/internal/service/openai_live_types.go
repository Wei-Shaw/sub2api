package service

import (
	"context"
	"encoding/json"
	"errors"
	"time"
)

const (
	LiveControllerPending  = "pending"
	LiveControllerObserver = "observer"
	LiveControllerProxy    = "proxy"
	LiveControllerClosed   = "closed"
)

var (
	ErrLiveUnavailable            = errors.New("live is unavailable")
	ErrLiveConcurrencyFull        = errors.New("live concurrency is full")
	ErrLiveCallNotFound           = errors.New("live call not found")
	ErrLiveIdentityMismatch       = errors.New("live call identity mismatch")
	ErrLiveControllerChanged      = errors.New("live controller changed")
	ErrLiveLeaseSourceLost        = errors.New("live source reservation lost before transfer")
	ErrLiveLeaseTransferUncertain = errors.New("live lease transfer result is unknown")
	ErrLiveLeaseTransferFenced    = errors.New("live lease transfer was already fenced")
)

type LiveAttestationUnavailableError struct {
	Reason string
}

func (e *LiveAttestationUnavailableError) Error() string {
	if e == nil || e.Reason == "" {
		return "Live attestation is unavailable"
	}
	return "Live attestation is unavailable: " + e.Reason
}

// LiveCallRequest 是两个下游创建协议归一后的请求。Session 不做结构改写。
type LiveCallRequest struct {
	SDP     string          `json:"sdp"`
	Session json.RawMessage `json:"session"`
}

type LiveCallIdentity struct {
	APIKeyConcurrencyLimit int
	APIKeyID               int64
	UserID                 int64
	GroupID                *int64
	SubscriptionID         *int64
	UserAgent              string
	IPAddress              string
	InboundEndpoint        string
	// KeyReservation is the already-waited regular key slot that the Live
	// lease atomically consumes. Nil means no key-level limit applies.
	KeyReservation *APIKeySlotReservation
	// UserRequestID is the exact ordinary user member that the Live transfer
	// consumes. Empty is only valid when user concurrency is unlimited.
	UserRequestID string
}

type LiveCallRecord struct {
	CallID          string
	CallHash        string
	AccountID       int64
	APIKeyID        int64
	UserID          int64
	GroupID         int64
	SubscriptionID  int64
	LeaseID         string
	Model           string
	CreatedAt       time.Time
	ExpiresAt       time.Time
	Controller      string
	ControllerOwner string
	UserAgent       string
	IPAddress       string
	InboundEndpoint string
	// AttestationCiphertext 仅用于让同一会话的 Sideband 复用创建时的证明。
	AttestationCiphertext string
}

type LiveCallCreated struct {
	SDP      []byte
	CallID   string
	Location string
	Account  *Account
}

// LiveCallStore 由 GatewayCache 的 Redis 实现可选提供，避免扩大旧缓存接口。
type LiveCallStore interface {
	SaveLiveCall(ctx context.Context, record *LiveCallRecord, ttl time.Duration) error
	GetLiveCall(ctx context.Context, callHash string) (*LiveCallRecord, error)
	ClaimLiveController(ctx context.Context, callHash, controller, owner string) (bool, error)
	ReleaseLiveController(ctx context.Context, callHash, owner string) (bool, error)
	GetLiveController(ctx context.Context, callHash string) (string, error)
	MarkLiveCallClosed(ctx context.Context, callHash string, ttl time.Duration) (bool, error)
}

type LiveConcurrencyCache interface {
	AcquireLiveLease(
		ctx context.Context,
		accountID int64,
		accountMax int,
		userID int64,
		userMax int,
		apiKeyID int64,
		apiKeyMax int,
		leaseID string,
		replacingRegularSlots bool,
	) (bool, error)
	RefreshLiveLease(ctx context.Context, accountID, userID, apiKeyID int64, leaseID string) (bool, error)
	ReleaseLiveLease(ctx context.Context, accountID, userID, apiKeyID int64, leaseID string) error
}

// LiveLeaseTransferRequest carries one ordinary account/user/key reservation
// each. RequestID fields are the exact Redis members owned by the caller; an
// empty ID is only valid when the matching Max is 0 (the dimension is
// unlimited and has no ordinary member to move).
type LiveLeaseTransferRequest struct {
	AccountID        int64
	AccountMax       int
	AccountRequestID string
	UserID           int64
	UserMax          int
	UserRequestID    string
	APIKeyID         int64
	APIKeyMax        int
	KeyRequestID     string
	LeaseID          string
	// ReplacedAccountID is the account whose Live member is replaced when an
	// already-held joint Key/user lease migrates to a new account after a
	// retryable SDP failure. Zero means no previous account member is held.
	ReplacedAccountID int64
}

// LiveLeaseTransferCache is optionally implemented by the Live lease cache. It
// moves the exact ordinary members into one Live lease in a single atomic
// operation, so no user/account/key dimension is counted twice and a missing
// source fails closed.
type LiveLeaseTransferCache interface {
	AcquireLiveLeaseTransferring(ctx context.Context, request LiveLeaseTransferRequest) (bool, error)
	// MigrateLiveLeaseAccount keeps an existing joint Key/user Live lease and
	// atomically replaces its account member with the new ordinary account
	// reservation. The Key and user members are never released or re-queued.
	MigrateLiveLeaseAccount(ctx context.Context, request LiveLeaseTransferRequest) (bool, error)
	// ReleaseLiveLeaseAccount removes only the current account's Live member,
	// keeping the joint Key/user members for the next retry attempt.
	ReleaseLiveLeaseAccount(ctx context.Context, accountID int64, leaseID string) error
	// AbortLiveLeaseTransfer fences this exact transfer and removes its members
	// after an unknown acknowledgement. It runs caller-supplied bounded cleanup
	// and never touches another lease.
	AbortLiveLeaseTransfer(ctx context.Context, request LiveLeaseTransferRequest) error
}
