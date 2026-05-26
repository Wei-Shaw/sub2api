package service

import "errors"

type usageLogCreateDisposition int

const (
	usageLogCreateDispositionUnknown usageLogCreateDisposition = iota
	usageLogCreateDispositionNotPersisted
	usageLogCreateDispositionDropped
)

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

func MarkUsageLogCreateNotPersisted(err error) error {
	if err == nil {
		return nil
	}
	return &UsageLogCreateError{
		err:         err,
		disposition: usageLogCreateDispositionNotPersisted,
	}
}

func MarkUsageLogCreateDropped(err error) error {
	if err == nil {
		return nil
	}
	return &UsageLogCreateError{
		err:         err,
		disposition: usageLogCreateDispositionDropped,
	}
}

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

func ShouldBillAfterUsageLogCreate(inserted bool, err error) bool {
	if inserted {
		return true
	}
	if err == nil {
		return false
	}
	return !IsUsageLogCreateNotPersisted(err)
}
