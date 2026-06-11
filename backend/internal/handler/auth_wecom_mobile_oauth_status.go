package handler

import (
	"context"
	"strings"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"

	"github.com/gin-gonic/gin"
)

func (h *AuthHandler) updateWeComMobilePendingSession(
	ctx context.Context,
	session *dbent.PendingAuthSession,
	resolved weComResolvedOAuthIdentity,
	resolvedEmail string,
	targetUserID *int64,
	completionResponse map[string]any,
) (*dbent.PendingAuthSession, error) {
	client := h.entClient()
	if client == nil {
		return nil, infraerrors.ServiceUnavailable("PENDING_AUTH_NOT_READY", "pending auth service is not ready")
	}
	localState := clonePendingMap(session.LocalFlowState)
	localState[weComMobileOAuthStatusKey] = weComMobileOAuthStatusCompleted
	localState[oauthCompletionResponseKey] = scrubPendingOAuthCompletionResponseForStorage(completionResponse)
	update := client.PendingAuthSession.UpdateOneID(session.ID).
		SetProviderSubject(resolved.providerSubject).
		SetResolvedEmail(strings.TrimSpace(resolvedEmail)).
		SetUpstreamIdentityClaims(resolved.upstreamClaims).
		SetLocalFlowState(localState)
	if targetUserID != nil && *targetUserID > 0 {
		update = update.SetTargetUserID(*targetUserID)
	} else {
		update = update.ClearTargetUserID()
	}
	return update.Save(ctx)
}

func buildWeComMobileStatusPayload(session *dbent.PendingAuthSession) gin.H {
	status := pendingSessionStringValue(session.LocalFlowState, weComMobileOAuthStatusKey)
	if status == "" {
		status = weComMobileOAuthStatusPending
	}
	payload := gin.H{
		"status":           status,
		"expires_at":       session.ExpiresAt.UTC().Format(time.RFC3339),
		"poll_interval_ms": weComMobileOAuthPollIntervalMS,
	}
	if pendingSessionBoolValue(session.LocalFlowState, weComMobileOAuthPrivateInfoKey) {
		payload["privateinfo_required"] = true
		payload["authorize_url"] = pendingSessionStringValue(session.LocalFlowState, "authorize_url")
	}
	if status == weComMobileOAuthStatusFailed {
		payload["error"] = pendingSessionStringValue(session.LocalFlowState, weComMobileOAuthErrorCodeKey)
		payload["message"] = pendingSessionStringValue(session.LocalFlowState, weComMobileOAuthErrorMessageKey)
	}
	if status == weComMobileOAuthStatusCompleted {
		payload["redirect"] = session.RedirectTo
		payload["provider"] = "wecom"
	}
	return payload
}

func pendingSessionBoolValue(values map[string]any, key string) bool {
	value, ok := values[key]
	if !ok {
		return false
	}
	typed, ok := value.(bool)
	return ok && typed
}

func isWeComMobileSession(session *dbent.PendingAuthSession) bool {
	if session == nil {
		return false
	}
	return pendingSessionStringValue(session.LocalFlowState, weComMobileOAuthStateKey) != ""
}
