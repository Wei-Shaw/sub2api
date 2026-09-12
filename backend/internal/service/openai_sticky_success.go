package service

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// OpenAIStickySuccessBinding is model-scoped so a fallback for one model cannot
// replace another model's working affinity. Revision also fences late completions.
type OpenAIStickySuccessBinding struct {
	AccountID int64  `json:"account_id"`
	Revision  string `json:"revision"`
}

type OpenAIStickySuccessCache interface {
	GetOpenAIStickySuccess(context.Context, int64, string, string) (OpenAIStickySuccessBinding, error)
	CompareAndSwapOpenAIStickySuccess(context.Context, int64, string, string, OpenAIStickySuccessBinding, OpenAIStickySuccessBinding, time.Duration) (bool, error)
}

type openAIStickySuccessKey struct{}

type openAIStickySuccessState struct {
	groupID        int64
	sessionHash    string
	model          string
	expected       OpenAIStickySuccessBinding
	originalID     int64
	preservePolicy bool
}

// BeginOpenAIStickySuccess leaves the original profit-policy binding intact.
// Only completed inference can establish the model-scoped success preference.
func (s *OpenAIGatewayService) BeginOpenAIStickySuccess(ctx context.Context, groupID *int64, sessionHash, model string) context.Context {
	if s == nil || s.cache == nil || !gatewayProfitControlGateActive(ctx) ||
		preserveOpenAIGuardianParentBinding(ctx, sessionHash) || sessionHash == "" || strings.TrimSpace(model) == "" {
		return ctx
	}
	cache, ok := s.cache.(OpenAIStickySuccessCache)
	if !ok || s.getOpenAIAccountScheduler(ctx) == nil {
		return ctx
	}
	state := &openAIStickySuccessState{groupID: derefGroupID(groupID), sessionHash: sessionHash, model: strings.TrimSpace(model)}
	binding, err := cache.GetOpenAIStickySuccess(ctx, state.groupID, sessionHash, state.model)
	if err != nil && !errors.Is(err, ErrStickySessionNotFound) {
		logger.FromContext(ctx).Warn("openai.sticky_success_read_failed", zap.Error(err))
		return ctx
	}
	state.expected = binding
	state.originalID = binding.AccountID
	if state.originalID == 0 {
		state.originalID, _ = s.getStickySessionAccountID(ctx, groupID, sessionHash)
	}
	if state.originalID > 0 {
		account, err := s.getSchedulableAccount(ctx, state.originalID)
		if err != nil {
			return ctx
		}
		if account != nil {
			state.preservePolicy, _ = openAIProfitControlVetoReason(ctx, account)
		}
	}
	return context.WithValue(ctx, openAIStickySuccessKey{}, state)
}

func openAIStickySuccessFromContext(ctx context.Context) *openAIStickySuccessState {
	state, _ := ctx.Value(openAIStickySuccessKey{}).(*openAIStickySuccessState)
	return state
}

func (s *OpenAIGatewayService) CommitOpenAIStickySuccess(ctx context.Context, account *Account, result *OpenAIForwardResult) {
	state := openAIStickySuccessFromContext(ctx)
	if state == nil || state.preservePolicy || account == nil || result == nil || result.ClientDisconnect || !result.SucceededForScheduling() || ctx.Err() != nil {
		return
	}
	cache, ok := s.cache.(OpenAIStickySuccessCache)
	if !ok {
		return
	}
	binding := OpenAIStickySuccessBinding{AccountID: account.ID, Revision: uuid.NewString()}
	updated, err := cache.CompareAndSwapOpenAIStickySuccess(ctx, state.groupID, state.sessionHash, state.model, state.expected, binding, s.openAIWSSessionStickyTTL())
	if err != nil {
		logger.FromContext(ctx).Warn("openai.sticky_success_commit_failed", zap.Int64("account_id", account.ID), zap.Error(err))
		return
	}
	if updated && state.originalID != account.ID {
		logger.FromContext(ctx).Info("openai.sticky_success_rebound", zap.Int64("previous_account_id", state.originalID),
			zap.Int64("account_id", account.ID), zap.Int64("group_id", state.groupID), zap.String("model", state.model))
	}
}
