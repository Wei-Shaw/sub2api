package handler

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"strings"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/config"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

const dingTalkAutoSignupStep = "dingtalk_auto_signup"

func canAutoRegisterDingTalk(cfg config.DingTalkConnectConfig, staff *DingTalkStaffInfo, blocked, forceEmail, invitation bool) bool {
	return cfg.AutoRegister && cfg.CorpRestrictionPolicy == "internal_only" && cfg.AppType == "internal" &&
		staff != nil && strings.TrimSpace(staff.UserID) != "" && strings.TrimSpace(staff.OrgEmail) != "" &&
		!blocked && !forceEmail && !invitation
}

// Only the server-side callback may create this step after successful directory
// lookup. Email/identity never come from the exchange request body. In particular,
// an email collision must go through password binding, never login-by-email.
func (h *AuthHandler) completeDingTalkAutoSignup(c *gin.Context, session *dbent.PendingAuthSession) {
	ctx := c.Request.Context()
	cfg, err := h.getDingTalkOAuthConfig(ctx)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	email := strings.ToLower(strings.TrimSpace(session.ResolvedEmail))
	staff := &DingTalkStaffInfo{
		UserID:   pendingSessionStringValue(session.UpstreamIdentityClaims, "corp_user_id"),
		OrgEmail: pendingSessionStringValue(session.UpstreamIdentityClaims, "enterprise_verified_email"),
	}
	if session.ProviderType != "dingtalk" || session.ProviderKey != "dingtalk" ||
		session.Intent != oauthIntentLogin || session.TargetUserID != nil || strings.TrimSpace(session.ProviderSubject) == "" ||
		email != staff.OrgEmail || !canAutoRegisterDingTalk(cfg, staff, h.isDingTalkSignupBlocked(ctx, cfg),
		h.isForceEmailOnThirdPartySignup(ctx), h.settingSvc != nil && h.settingSvc.IsInvitationCodeEnabled(ctx)) {
		response.ErrorFrom(c, infraerrors.BadRequest("DINGTALK_AUTO_SIGNUP_UNAVAILABLE", "Automatic registration is unavailable; restart DingTalk sign-in."))
		return
	}
	if err := h.ensureBackendModeAllowsNewUserLogin(ctx); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	client := h.entClient()
	if client == nil {
		response.ErrorFrom(c, service.ErrServiceUnavailable)
		return
	}
	// No user receives or chooses this random password. Subsequent sign-in uses
	// the bound DingTalk identity. Local password recovery remains available.
	password := make([]byte, 32)
	if _, err := rand.Read(password); err != nil {
		response.ErrorFrom(c, service.ErrServiceUnavailable)
		return
	}
	tokens, user, err := h.authService.RegisterVerifiedOAuthEmailAccount(ctx, email, hex.EncodeToString(password), "", "dingtalk")
	if err != nil {
		if errors.Is(err, service.ErrEmailExists) {
			existing, lookupErr := findUserByNormalizedEmail(ctx, client, email)
			if lookupErr != nil {
				response.ErrorFrom(c, err)
				return
			}
			updated, updateErr := h.transitionPendingOAuthAccountToChoiceState(c, client, session, existing, email)
			if updateErr != nil {
				response.ErrorFrom(c, updateErr)
				return
			}
			c.JSON(http.StatusOK, buildPendingOAuthSessionStatusPayload(updated))
			return
		}
		response.ErrorFrom(c, err)
		return
	}
	rollback := func(cause error) {
		if rollbackErr := h.authService.RollbackOAuthEmailAccountCreation(ctx, user.ID, ""); rollbackErr != nil {
			cause = infraerrors.InternalServer("DINGTALK_AUTO_SIGNUP_ROLLBACK_FAILED", "Failed to roll back automatic registration").
				WithCause(fmt.Errorf("registration: %w; rollback: %v", cause, rollbackErr))
		}
		response.ErrorFrom(c, cause)
	}
	tx, err := client.Tx(ctx)
	if err != nil {
		rollback(err)
		return
	}
	defer func() { _ = tx.Rollback() }()
	txCtx := dbent.NewTxContext(ctx, tx)
	if err := applyPendingOAuthBinding(txCtx, client, h.authService, h.userService, session, nil, &user.ID, true, false); err != nil {
		_ = tx.Rollback()
		rollback(err)
		return
	}
	if cfg.SyncDisplayName {
		name := normalizeAdoptedOAuthDisplayName(pendingSessionStringValue(session.UpstreamIdentityClaims, "username"))
		if name != "" {
			if err := tx.Client().User.UpdateOneID(user.ID).SetUsername(name).Exec(txCtx); err != nil {
				_ = tx.Rollback()
				rollback(err)
				return
			}
		}
	}
	if err := h.authService.FinalizeOAuthEmailAccount(txCtx, user, "", "dingtalk", ""); err != nil {
		_ = tx.Rollback()
		rollback(err)
		return
	}
	if err := consumePendingOAuthBrowserSessionTx(txCtx, tx, session); err != nil {
		_ = tx.Rollback()
		rollback(err)
		return
	}
	if err := tx.Commit(); err != nil {
		rollback(err)
		return
	}
	h.authService.ApplyOAuthSignupPromoCode(ctx, user.ID, pendingOAuthPromoCode(session))
	h.authService.RecordSuccessfulLogin(ctx, user.ID)
	h.maybeSyncDingTalkAfterRegistration(ctx, session, user.ID)
	clearOAuthPendingSessionCookie(c, isRequestHTTPS(c))
	clearOAuthPendingBrowserCookie(c, isRequestHTTPS(c))
	writeOAuthTokenPairResponse(c, tokens)
}
