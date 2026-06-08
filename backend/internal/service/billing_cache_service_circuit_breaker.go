package service

// BillingCacheService 依赖的熔断器实现。
// 主 struct 在 billing_cache_service.go 持有 *billingCircuitBreaker 字段，
// 本文件承载熔断器类型、状态转移与日志。

import (
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
)

type billingCircuitBreakerState int

const (
	billingCircuitClosed billingCircuitBreakerState = iota
	billingCircuitOpen
	billingCircuitHalfOpen
)

type billingCircuitBreaker struct {
	mu                sync.Mutex
	state             billingCircuitBreakerState
	failures          int
	openedAt          time.Time
	failureThreshold  int
	resetTimeout      time.Duration
	halfOpenRequests  int
	halfOpenRemaining int
}

func newBillingCircuitBreaker(cfg config.CircuitBreakerConfig) *billingCircuitBreaker {
	if !cfg.Enabled {
		return nil
	}
	resetTimeout := time.Duration(cfg.ResetTimeoutSeconds) * time.Second
	if resetTimeout <= 0 {
		resetTimeout = 30 * time.Second
	}
	halfOpen := cfg.HalfOpenRequests
	if halfOpen <= 0 {
		halfOpen = 1
	}
	threshold := cfg.FailureThreshold
	if threshold <= 0 {
		threshold = 5
	}
	return &billingCircuitBreaker{
		state:            billingCircuitClosed,
		failureThreshold: threshold,
		resetTimeout:     resetTimeout,
		halfOpenRequests: halfOpen,
	}
}

func (b *billingCircuitBreaker) Allow() bool {
	b.mu.Lock()
	defer b.mu.Unlock()

	switch b.state {
	case billingCircuitClosed:
		return true
	case billingCircuitOpen:
		if time.Since(b.openedAt) < b.resetTimeout {
			return false
		}
		b.state = billingCircuitHalfOpen
		b.halfOpenRemaining = b.halfOpenRequests
		logger.LegacyPrintf("service.billing_cache", "ALERT: billing circuit breaker entering half-open state")
		fallthrough
	case billingCircuitHalfOpen:
		if b.halfOpenRemaining <= 0 {
			return false
		}
		b.halfOpenRemaining--
		return true
	default:
		return false
	}
}

func (b *billingCircuitBreaker) OnFailure(err error) {
	if b == nil {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()

	switch b.state {
	case billingCircuitOpen:
		return
	case billingCircuitHalfOpen:
		b.state = billingCircuitOpen
		b.openedAt = time.Now()
		b.halfOpenRemaining = 0
		logger.LegacyPrintf("service.billing_cache", "ALERT: billing circuit breaker opened after half-open failure: %v", err)
		return
	default:
		b.failures++
		if b.failures >= b.failureThreshold {
			b.state = billingCircuitOpen
			b.openedAt = time.Now()
			b.halfOpenRemaining = 0
			logger.LegacyPrintf("service.billing_cache", "ALERT: billing circuit breaker opened after %d failures: %v", b.failures, err)
		}
	}
}

func (b *billingCircuitBreaker) OnSuccess() {
	if b == nil {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()

	previousState := b.state
	previousFailures := b.failures

	b.state = billingCircuitClosed
	b.failures = 0
	b.halfOpenRemaining = 0

	// 只有状态真正发生变化时才记录日志
	if previousState != billingCircuitClosed {
		logger.LegacyPrintf("service.billing_cache", "ALERT: billing circuit breaker closed (was %s)", circuitStateString(previousState))
	} else if previousFailures > 0 {
		logger.LegacyPrintf("service.billing_cache", "INFO: billing circuit breaker failures reset from %d", previousFailures)
	}
}

func circuitStateString(state billingCircuitBreakerState) string {
	switch state {
	case billingCircuitClosed:
		return "closed"
	case billingCircuitOpen:
		return "open"
	case billingCircuitHalfOpen:
		return "half-open"
	default:
		return "unknown"
	}
}
