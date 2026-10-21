package service

import (
	"context"
	"errors"
	"log/slog"
	"strings"

	"github.com/google/uuid"
)

type openAILegacyStickySuccessKey struct{}

// BeginOpenAILegacyStickySuccess is opt-in for migratable HTTP text requests.
// The handler excludes protocol-bound requests before calling it. The existing
// group/session/model CAS cache is reused with the OpenAI session namespace.
//
// Groups without profit control are armed too. Their session ownership is only
// written while *selecting* an account, so whether the account that actually
// succeeded takes over depends on it being the last one picked in that request:
// a candidate that is merely selected and then fails still replaces the account
// that previously succeeded. Such groups only hand over the *candidate source*
// (success preference first, legacy binding as the compatible fallback); the
// legacy sticky key keeps its eager write/renew/clear semantics verbatim, see
// openAILegacyStickySuccessWritesManaged.
func (s *OpenAIGatewayService) BeginOpenAILegacyStickySuccess(ctx context.Context, groupID *int64, sessionHash, model string) context.Context {
	if s == nil || s.cache == nil ||
		s.isOpenAIAdvancedSchedulerEnabled(ctx) || preserveOpenAIGuardianParentBinding(ctx, sessionHash) ||
		sessionHash == "" || strings.TrimSpace(model) == "" {
		return ctx
	}
	if state := openAILegacyStickySuccessFromContext(ctx); state != nil &&
		state.groupID == derefGroupID(groupID) && state.sessionHash == sessionHash && state.model == strings.TrimSpace(model) {
		return ctx
	}
	state := &gatewayStickySuccessState{
		groupID: derefGroupID(groupID), sessionHash: sessionHash, model: strings.TrimSpace(model),
		legacyWritesManaged: gatewayProfitControlGateActive(ctx),
	}
	binding, err := s.cache.GetGatewayStickySuccess(ctx, state.groupID, s.openAISessionCacheKey(sessionHash), state.model)
	if err != nil && !errors.Is(err, ErrStickySessionNotFound) {
		slog.Warn("openai.legacy_sticky_success_read_failed", "group_id", state.groupID, "error", err)
		return ctx
	}
	state.expected = binding
	state.originalID = binding.AccountID
	if state.originalID == 0 {
		state.originalID, _ = s.getStickySessionAccountID(ctx, groupID, sessionHash)
	}
	return context.WithValue(ctx, openAILegacyStickySuccessKey{}, state)
}

func openAILegacyStickySuccessFromContext(ctx context.Context) *gatewayStickySuccessState {
	state, _ := ctx.Value(openAILegacyStickySuccessKey{}).(*gatewayStickySuccessState)
	return state
}

func openAILegacyStickySuccessCandidate(ctx context.Context, groupID *int64, sessionHash string) (int64, bool) {
	state := openAILegacyStickySuccessFromContext(ctx)
	if state == nil || state.groupID != derefGroupID(groupID) || state.sessionHash != sessionHash {
		return 0, false
	}
	return state.originalID, true
}

// openAILegacyStickySuccessWritesManaged reports whether the legacy sticky key
// write/renew/clear calls are managed by the success preference as well.
//
// Only groups under a profit gate are: selection there never binds eagerly, so
// the single legacy write point is the post-admission binding and it has to be
// turned into "write only after a confirmed success" together with the
// preference. Groups without profit control merely swap the candidate source,
// so the legacy key keeps being written, renewed and cleared exactly as before
// and every other reader of it (WebSocket, count_tokens, the previous_response
// ownership chain, Guardian) is unaffected.
func openAILegacyStickySuccessWritesManaged(ctx context.Context, groupID *int64, sessionHash string) bool {
	state := openAILegacyStickySuccessFromContext(ctx)
	return state != nil && state.legacyWritesManaged &&
		state.groupID == derefGroupID(groupID) && state.sessionHash == sessionHash
}

// CommitOpenAILegacyStickySuccess is called only after Forward returns without
// error. Keep old bindings intact: other protocols must not inherit a preference
// learned by these HTTP requests. Late completions lose the CAS and do not retry.
func (s *OpenAIGatewayService) CommitOpenAILegacyStickySuccess(ctx context.Context, account *Account, result *OpenAIForwardResult) {
	state := openAILegacyStickySuccessFromContext(ctx)
	if state == nil || account == nil || result == nil || result.ClientDisconnect ||
		!result.SucceededForScheduling() || ctx.Err() != nil {
		return
	}
	next := GatewayStickySuccessBinding{AccountID: account.ID, Revision: uuid.NewString()}
	updated, err := s.cache.CompareAndSwapGatewayStickySuccess(ctx, state.groupID,
		s.openAISessionCacheKey(state.sessionHash), state.model, state.expected, next, openaiStickySessionTTL)
	if err != nil {
		slog.Warn("openai.legacy_sticky_success_commit_failed", "group_id", state.groupID, "account_id", account.ID, "error", err)
		return
	}
	if updated && state.originalID != account.ID {
		slog.Info("openai.legacy_sticky_success_rebound", "group_id", state.groupID,
			"previous_account_id", state.originalID, "account_id", account.ID)
	}
}
