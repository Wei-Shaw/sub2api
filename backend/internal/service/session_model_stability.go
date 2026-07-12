package service

import (
	"context"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/redis/go-redis/v9"
)

type sessionModelStore interface {
	GetSessionModel(ctx context.Context, groupID int64, sessionHash, requestedModel string, routeVersion int64) (string, error)
	SetSessionModel(ctx context.Context, groupID int64, sessionHash, requestedModel string, routeVersion int64, upstreamModel string, ttl time.Duration) error
}

type stableModelState struct {
	groupID, routeVersion                      int64
	sessionHash, requestedModel, upstreamModel string
}

func (s *OpenAIGatewayService) PrepareStableModelContext(ctx context.Context, group *Group, sessionHash, requestedModel string) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if s == nil || group == nil || !group.SessionModelStabilityEnabled || sessionHash == "" || strings.TrimSpace(requestedModel) == "" {
		return ctx
	}
	state := &stableModelState{groupID: group.ID, routeVersion: group.UpdatedAt.UnixNano(), sessionHash: sessionHash, requestedModel: strings.TrimSpace(requestedModel)}
	if store, ok := s.cache.(sessionModelStore); ok {
		if model, err := store.GetSessionModel(ctx, state.groupID, state.sessionHash, state.requestedModel, state.routeVersion); err == nil {
			state.upstreamModel = strings.TrimSpace(model)
		} else if err != redis.Nil {
			// Cache failures are deliberately fail-open; routing remains available.
		}
	}
	return context.WithValue(ctx, ctxkey.StableUpstreamModel, state)
}

func stableUpstreamModelFromContext(ctx context.Context) string {
	state, _ := ctx.Value(ctxkey.StableUpstreamModel).(*stableModelState)
	if state == nil {
		return ""
	}
	return state.upstreamModel
}

func accountMatchesStableUpstreamModel(ctx context.Context, account *Account, requestedModel string) bool {
	if fallback, _ := ctx.Value(ctxkey.StableModelFallback).(bool); fallback {
		return true
	}
	stable := stableUpstreamModelFromContext(ctx)
	return stable == "" || account == nil || strings.TrimSpace(account.GetMappedModel(requestedModel)) == stable
}

func (s *OpenAIGatewayService) bindStableModelChoice(ctx context.Context, upstreamModel string) {
	state, _ := ctx.Value(ctxkey.StableUpstreamModel).(*stableModelState)
	upstreamModel = strings.TrimSpace(upstreamModel)
	if s == nil || state == nil || state.upstreamModel != "" || upstreamModel == "" {
		return
	}
	store, ok := s.cache.(sessionModelStore)
	if !ok {
		return
	}
	ttl := openaiStickySessionTTL
	if s.cfg != nil && s.cfg.Gateway.OpenAIWS.StickySessionTTLSeconds > 0 {
		ttl = time.Duration(s.cfg.Gateway.OpenAIWS.StickySessionTTLSeconds) * time.Second
	}
	if store.SetSessionModel(ctx, state.groupID, state.sessionHash, state.requestedModel, state.routeVersion, upstreamModel, ttl) == nil {
		state.upstreamModel = upstreamModel
	}
}
