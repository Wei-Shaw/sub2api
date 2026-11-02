package service

import (
	"context"
	"errors"

	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"go.uber.org/zap"
)

var ErrAccountSlotYieldToNewSession = errors.New("account slot reserved for a new session")

type AccountAdmissionState struct {
	NewWaiting          int
	ContinuationWaiting int
	ContinuationBurst   int
}

func (s AccountAdmissionState) NewSessionTurn(limit int) bool {
	return limit > 0 && s.NewWaiting > 0 && s.ContinuationBurst >= limit
}

type AccountAdmissionCache interface {
	AcquireAccountSlotForClass(ctx context.Context, accountID int64, maxConcurrency int, requestID string, class AccountWaitClass, burstLimit int) (bool, error)
	ReuseAccountSlot(ctx context.Context, accountID int64, requestID, admissionID string, burstLimit int) (bool, error)
	GetAccountAdmissionState(ctx context.Context, accountID int64) (*AccountAdmissionState, error)
}

func (s *ConcurrencyService) AcquireAccountSlotForClass(ctx context.Context, accountID int64, maxConcurrency int, class AccountWaitClass, burstLimit int) (*AcquireResult, error) {
	if class == AccountWaitClassLegacy || burstLimit <= 0 {
		if class == AccountWaitClassNewSession {
			waiting, err := s.GetAccountContinuationWaitingCount(ctx, accountID)
			if err == nil && waiting > 0 {
				return &AcquireResult{}, nil
			}
		}
		return s.AcquireAccountSlot(ctx, accountID, maxConcurrency)
	}
	if maxConcurrency <= 0 {
		return &AcquireResult{Acquired: true, ReleaseFunc: func() {}}, nil
	}
	cache, ok := s.cache.(AccountAdmissionCache)
	if !ok {
		return nil, errors.New("account admission cache is unsupported")
	}
	requestID := generateRequestID()
	acquired, err := cache.AcquireAccountSlotForClass(ctx, accountID, maxConcurrency, requestID, class, burstLimit)
	if err != nil {
		return nil, err
	}
	result := s.accountSlotResult(accountID, requestID, acquired)
	if acquired {
		logAccountAdmission(ctx, accountID, class, burstLimit, "acquire")
		result.ReuseFunc = func(ctx context.Context) (bool, error) {
			reused, err := cache.ReuseAccountSlot(ctx, accountID, requestID, generateRequestID(), burstLimit)
			if err == nil && reused {
				logAccountAdmission(ctx, accountID, AccountWaitClassContinuation, burstLimit, "reuse")
			}
			return reused, err
		}
	}
	return result, nil
}

func (s *ConcurrencyService) HeldAccountSlotYieldReason(ctx context.Context, accountID int64, burstLimit int) (string, error) {
	if burstLimit <= 0 {
		waiting, err := s.GetAccountContinuationWaitingCount(ctx, accountID)
		if err == nil && waiting > 0 {
			return "continuation_waiters", nil
		}
		return "", err
	}
	cache, ok := s.cache.(AccountAdmissionCache)
	if !ok {
		return "", errors.New("account admission cache is unsupported")
	}
	state, err := cache.GetAccountAdmissionState(ctx, accountID)
	if err != nil {
		return "", err
	}
	if state.NewSessionTurn(burstLimit) {
		return "new_session_turn", nil
	}
	if state.ContinuationWaiting > 0 {
		return "continuation_waiters", nil
	}
	return "", nil
}

func logAccountAdmission(ctx context.Context, accountID int64, class AccountWaitClass, burstLimit int, source string) {
	logger.FromContext(ctx).Debug("openai.account_slot_admitted", zap.Int64("account_id", accountID), zap.String("class", class.String()), zap.Int("continuation_burst_limit", burstLimit), zap.String("source", source))
}

func (s *OpenAIGatewayService) OpenAIContinuationBurstLimit() int {
	if s == nil || s.cfg == nil {
		return 0
	}
	return s.cfg.Gateway.Scheduling.ContinuationBurstLimit
}
