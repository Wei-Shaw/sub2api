package domain

import "errors"

type usageLogCreateDisposition int

const (
	usageLogCreateDispositionUnknown usageLogCreateDisposition = iota
	usageLogCreateDispositionNotPersisted
	usageLogCreateDispositionDropped
)

// UsageLogCreateError wraps a usage-log creation error with its disposition
// (not-persisted vs dropped), used by the billing-decision path to decide
// whether to bill a request whose log was not durably stored.
type UsageLogCreateError struct {
	err         error
	disposition usageLogCreateDisposition
}

func (e *UsageLogCreateError) Error() string {
	if e == nil || e.err == nil {
		return "usage log create error"
	}
	return e.err.Error()
}

func (e *UsageLogCreateError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.err
}

// MarkUsageLogCreateNotPersisted marks err as a not-persisted disposition.
func MarkUsageLogCreateNotPersisted(err error) error {
	if err == nil {
		return nil
	}
	return &UsageLogCreateError{
		err:         err,
		disposition: usageLogCreateDispositionNotPersisted,
	}
}

// MarkUsageLogCreateDropped marks err as a dropped disposition.
func MarkUsageLogCreateDropped(err error) error {
	if err == nil {
		return nil
	}
	return &UsageLogCreateError{
		err:         err,
		disposition: usageLogCreateDispositionDropped,
	}
}

// IsUsageLogCreateNotPersisted reports whether err carries the not-persisted disposition.
func IsUsageLogCreateNotPersisted(err error) bool {
	if err == nil {
		return false
	}
	var target *UsageLogCreateError
	if !errors.As(err, &target) {
		return false
	}
	return target.disposition == usageLogCreateDispositionNotPersisted
}

// IsUsageLogCreateDropped reports whether err carries the dropped disposition.
func IsUsageLogCreateDropped(err error) bool {
	if err == nil {
		return false
	}
	var target *UsageLogCreateError
	if !errors.As(err, &target) {
		return false
	}
	return target.disposition == usageLogCreateDispositionDropped
}

// ShouldBillAfterUsageLogCreate decides whether a request should still be billed
// when its usage log was not durably inserted.
func ShouldBillAfterUsageLogCreate(inserted bool, err error) bool {
	if inserted {
		return true
	}
	if err == nil {
		return false
	}
	return !IsUsageLogCreateNotPersisted(err)
}
