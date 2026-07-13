package service

import (
	"context"
	"errors"
	"time"
)

type AdmissionClass string

const (
	AdmissionClassStandard AdmissionClass = "standard"
	AdmissionClassExtra    AdmissionClass = "extra"
)

type UserLeaseRequest struct {
	RequestID     string
	UserID        int64
	StandardLimit int
	ExtraLimit    int
	MaxWaiting    int
	WaitTimeout   time.Duration
}

type UserLeaseResult struct {
	Acquired  bool
	Class     AdmissionClass
	Unlimited bool
	QueueFull bool
	Draining  bool
	Expired   bool
}

type TargetLeaseRequest struct {
	RequestID        string
	Platform         string
	AccountID        int64
	AccountLimit     int
	PlatformCapacity int
	ReservedSlots    int
	Class            AdmissionClass
	WaitTimeout      time.Duration
	Unlimited        bool
}

type TargetLeaseResult struct {
	Acquired bool
	Expired  bool
	Draining bool
}

type TargetDispatchRequest struct {
	RequestID string
	Platform  string
	AccountID int64
	Class     AdmissionClass
	Unlimited bool
}

type TargetDispatchResult struct {
	Started  bool
	Draining bool
}

type ExtraConcurrencyUnavailableError struct {
	Timeout bool
}

func (e *ExtraConcurrencyUnavailableError) Error() string {
	return "extra concurrency is unavailable"
}

type GatewayAdmissionQueueFullError struct{}

var ErrGatewayAdmissionDraining = errors.New("gateway admission is draining")

func (e *GatewayAdmissionQueueFullError) Error() string {
	return "gateway admission wait queue is full"
}

type GatewayAdmissionTimeoutError struct {
	SlotType string
}

func (e *GatewayAdmissionTimeoutError) Error() string {
	return "gateway admission timed out waiting for " + e.SlotType + " concurrency"
}

func gatewayAdmissionWaitTimeoutError(class AdmissionClass, slotType string) error {
	if class == AdmissionClassExtra {
		return &ExtraConcurrencyUnavailableError{Timeout: true}
	}
	return &GatewayAdmissionTimeoutError{SlotType: slotType}
}

// GatewayAdmissionStore owns the distributed state used by gateway admission.
type GatewayAdmissionStore interface {
	TryAcquireUserLease(ctx context.Context, request UserLeaseRequest) (UserLeaseResult, error)
	RenewUserLease(ctx context.Context, userID int64, requestID string, class AdmissionClass) (bool, error)
	ReleaseUserLease(ctx context.Context, userID int64, requestID string) error
	TryAcquireTargetLease(ctx context.Context, request TargetLeaseRequest) (TargetLeaseResult, error)
	BeginTargetDispatch(ctx context.Context, request TargetDispatchRequest) (TargetDispatchResult, error)
	RenewTargetLease(ctx context.Context, platform string, accountID int64, requestID string) (bool, error)
	ReleaseTargetLease(ctx context.Context, platform string, accountID int64, requestID string) error
}
