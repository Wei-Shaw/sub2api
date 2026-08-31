package service

import "time"

// streamIdleTimer schedules the next check at the exact idle deadline.
// A fixed ticker can miss a recently refreshed lastRead and wait a full extra
// interval before checking again, effectively doubling the configured timeout.
type streamIdleTimer struct {
	timer   *time.Timer
	timeout time.Duration
}

func newStreamIdleTimer(timeout time.Duration) *streamIdleTimer {
	if timeout <= 0 {
		return nil
	}
	return &streamIdleTimer{
		timer:   time.NewTimer(timeout),
		timeout: timeout,
	}
}

func (t *streamIdleTimer) C() <-chan time.Time {
	if t == nil {
		return nil
	}
	return t.timer.C
}

func (t *streamIdleTimer) Stop() {
	if t != nil {
		t.timer.Stop()
	}
}

// ExpiredSince returns true once lastRead has been idle for the configured
// timeout. If the timer fired early relative to a newer read, it is reset only
// for the remaining duration rather than another full timeout interval.
func (t *streamIdleTimer) ExpiredSince(lastRead time.Time) bool {
	if t == nil {
		return false
	}
	remaining := t.timeout - time.Since(lastRead)
	if remaining <= 0 {
		return true
	}
	t.timer.Reset(remaining)
	return false
}
