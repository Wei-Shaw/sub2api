package handler

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/pendingauthsession"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

func (h *AuthHandler) createWeComMobilePendingSession(ctx context.Context, input weComMobilePendingSessionInput) (*dbent.PendingAuthSession, error) {
	svc, err := h.pendingIdentityService()
	if err != nil {
		return nil, err
	}
	localFlowState := map[string]any{
		weComMobileOAuthStateKey:  hashWeComMobileState(input.state),
		weComMobileOAuthStatusKey: weComMobileOAuthStatusPending,
		"authorize_url":           input.authorizeURL,
	}
	if promoCode := strings.TrimSpace(input.promoCode); promoCode != "" {
		localFlowState[oauthPromoCodeStateKey] = promoCode
	}
	return svc.CreatePendingSession(ctx, service.CreatePendingAuthSessionInput{
		Intent:            input.intent,
		Identity:          service.PendingAuthIdentityKey{ProviderType: "wecom", ProviderKey: weComOAuthProviderKey, ProviderSubject: pendingWeComMobileSubject(input.state)},
		TargetUserID:      input.targetUserID,
		RedirectTo:        input.redirectTo,
		BrowserSessionKey: input.browserSessionKey,
		LocalFlowState:    localFlowState,
		ExpiresAt:         time.Now().UTC().Add(weComMobileOAuthSessionTTL),
	})
}

func (h *AuthHandler) loadWeComMobileStatusSession(ctx context.Context, sessionToken string, browserSessionKey string) (*dbent.PendingAuthSession, error) {
	svc, err := h.pendingIdentityService()
	if err != nil {
		return nil, err
	}
	session, err := svc.GetBrowserSession(ctx, sessionToken, browserSessionKey)
	if err != nil {
		return nil, err
	}
	if !isWeComMobileSession(session) {
		return nil, service.ErrPendingAuthSessionNotFound
	}
	return session, nil
}

func (h *AuthHandler) loadWeComMobileCallbackSession(ctx context.Context, state string) (*dbent.PendingAuthSession, error) {
	client := h.entClient()
	if client == nil {
		return nil, infraerrors.ServiceUnavailable("PENDING_AUTH_NOT_READY", "pending auth service is not ready")
	}
	session, err := client.PendingAuthSession.Query().
		Where(
			pendingauthsession.ProviderTypeEQ("wecom"),
			pendingauthsession.ProviderKeyEQ(weComOAuthProviderKey),
			pendingauthsession.ProviderSubjectEQ(pendingWeComMobileSubject(state)),
		).
		Only(ctx)
	if err != nil {
		if dbent.IsNotFound(err) {
			return nil, service.ErrPendingAuthSessionNotFound
		}
		return nil, err
	}
	if err := validateWeComMobileSessionState(session); err != nil {
		return nil, err
	}
	if hash := pendingSessionStringValue(session.LocalFlowState, weComMobileOAuthStateKey); hash != hashWeComMobileState(state) {
		return nil, infraerrors.Unauthorized("INVALID_OAUTH_STATE", "invalid oauth state")
	}
	return session, nil
}

func (h *AuthHandler) markWeComMobileSessionFailed(ctx context.Context, session *dbent.PendingAuthSession, code string, message string) error {
	client := h.entClient()
	if client == nil || session == nil {
		return nil
	}
	localState := clonePendingMap(session.LocalFlowState)
	localState[weComMobileOAuthStatusKey] = weComMobileOAuthStatusFailed
	localState[weComMobileOAuthErrorCodeKey] = strings.TrimSpace(code)
	localState[weComMobileOAuthErrorMessageKey] = strings.TrimSpace(message)
	_, err := client.PendingAuthSession.UpdateOneID(session.ID).SetLocalFlowState(localState).Save(ctx)
	return err
}

func validateWeComMobileSessionState(session *dbent.PendingAuthSession) error {
	if session == nil {
		return service.ErrPendingAuthSessionNotFound
	}
	if session.ConsumedAt != nil {
		return service.ErrPendingAuthSessionConsumed
	}
	if !session.ExpiresAt.IsZero() && time.Now().UTC().After(session.ExpiresAt) {
		return service.ErrPendingAuthSessionExpired
	}
	return nil
}

func pendingWeComMobileSubject(state string) string {
	return "mobile:" + hashWeComMobileState(state)
}

func hashWeComMobileState(state string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(state)))
	return hex.EncodeToString(sum[:])
}
