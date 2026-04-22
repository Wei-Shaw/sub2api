package service

import (
	"context"
	"errors"
	"fmt"
	"net/mail"
	"strings"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/redeemcode"
)

func normalizeOAuthSignupSource(signupSource string) string {
	signupSource = strings.TrimSpace(strings.ToLower(signupSource))
	switch signupSource {
	case "", "email":
		return "email"
	case "linuxdo", "wechat", "oidc":
		return signupSource
	default:
		return "email"
	}
}

// SendPendingOAuthVerifyCode sends a local verification code for pending OAuth
// account-creation flows without relying on the public registration gate.
func (s *AuthService) SendPendingOAuthVerifyCode(ctx context.Context, email string) (*SendVerifyCodeResult, error) {
	email = strings.TrimSpace(strings.ToLower(email))
	if email == "" {
		return nil, ErrEmailVerifyRequired
	}
	if _, err := mail.ParseAddress(email); err != nil {
		return nil, ErrEmailVerifyRequired
	}
	if isReservedEmail(email) {
		return nil, ErrEmailReserved
	}
	if s == nil || s.emailService == nil {
		return nil, ErrServiceUnavailable
	}

	siteName := "Sub2API"
	if s.settingService != nil {
		siteName = s.settingService.GetSiteName(ctx)
	}
	if err := s.emailService.SendVerifyCode(ctx, email, siteName); err != nil {
		return nil, err
	}
	return &SendVerifyCodeResult{
		Countdown: int(verifyCodeCooldown / time.Second),
	}, nil
}

func (s *AuthService) validateOAuthRegistrationInvitation(ctx context.Context, invitationCode string) (*RedeemCode, error) {
	if s == nil || s.settingService == nil || !s.settingService.IsInvitationCodeEnabled(ctx) {
		return nil, nil
	}
	if s.redeemRepo == nil && s.oauthEmailFlowClient(ctx) == nil {
		return nil, ErrServiceUnavailable
	}

	invitationCode = strings.TrimSpace(invitationCode)
	if invitationCode == "" {
		return nil, ErrInvitationCodeRequired
	}

	redeemCode, err := s.loadOAuthRegistrationInvitation(ctx, invitationCode)
	if err != nil {
		return nil, ErrInvitationCodeInvalid
	}
	if redeemCode.Type != RedeemTypeInvitation || redeemCode.Status != StatusUnused {
		return nil, ErrInvitationCodeInvalid
	}
	return redeemCode, nil
}

// VerifyOAuthEmailCode verifies the locally entered email verification code for
// third-party signup and binding flows. This is intentionally independent from
// the global registration email verification toggle.
func (s *AuthService) VerifyOAuthEmailCode(ctx context.Context, email, verifyCode string) error {
	email = strings.TrimSpace(strings.ToLower(email))
	verifyCode = strings.TrimSpace(verifyCode)

	if email == "" {
		return ErrEmailVerifyRequired
	}
	if verifyCode == "" {
		return ErrEmailVerifyRequired
	}
	if s == nil || s.emailService == nil {
		return ErrServiceUnavailable
	}
	return s.emailService.VerifyCode(ctx, email, verifyCode)
}

// RegisterOAuthEmailAccount creates a local account from a third-party first
// login after the user has verified a local email address.
func (s *AuthService) RegisterOAuthEmailAccount(
	ctx context.Context,
	email string,
	password string,
	verifyCode string,
	invitationCode string,
	signupSource string,
) (*TokenPair, *User, error) {
	if s == nil {
		return nil, nil, ErrServiceUnavailable
	}
	if s.settingService == nil || !s.settingService.IsRegistrationEnabled(ctx) {
		return nil, nil, ErrRegDisabled
	}

	email = strings.TrimSpace(strings.ToLower(email))
	if isReservedEmail(email) {
		return nil, nil, ErrEmailReserved
	}
	if err := s.validateRegistrationEmailPolicy(ctx, email); err != nil {
		return nil, nil, err
	}
	if err := s.VerifyOAuthEmailCode(ctx, email, verifyCode); err != nil {
		return nil, nil, err
	}

	if _, err := s.validateOAuthRegistrationInvitation(ctx, invitationCode); err != nil {
		return nil, nil, err
	}

	existsEmail, err := s.userRepo.ExistsByEmail(ctx, email)
	if err != nil {
		return nil, nil, ErrServiceUnavailable
	}
	if existsEmail {
		return nil, nil, ErrEmailExists
	}

	hashedPassword, err := s.HashPassword(password)
	if err != nil {
		return nil, nil, fmt.Errorf("hash password: %w", err)
	}

	signupSource = normalizeOAuthSignupSource(signupSource)
	grantPlan := s.resolveSignupGrantPlan(ctx, signupSource)

	user := &User{
		Email:        email,
		PasswordHash: hashedPassword,
		Role:         RoleUser,
		Balance:      grantPlan.Balance,
		Concurrency:  grantPlan.Concurrency,
		Status:       StatusActive,
		SignupSource: signupSource,
	}

	if err := s.userRepo.Create(ctx, user); err != nil {
		if errors.Is(err, ErrEmailExists) {
			return nil, nil, ErrEmailExists
		}
		return nil, nil, ErrServiceUnavailable
	}

	tokenPair, err := s.GenerateTokenPair(ctx, user, "")
	if err != nil {
		_ = s.RollbackOAuthEmailAccountCreation(ctx, user.ID, "")
		return nil, nil, fmt.Errorf("generate token pair: %w", err)
	}
	return tokenPair, user, nil
}

// FinalizeOAuthEmailAccount applies invitation usage and normal signup bootstrap
// only after the pending OAuth flow has fully reached its last reversible step.
func (s *AuthService) FinalizeOAuthEmailAccount(
	ctx context.Context,
	user *User,
	invitationCode string,
	signupSource string,
) error {
	if s == nil || user == nil || user.ID <= 0 {
		return ErrServiceUnavailable
	}

	signupSource = normalizeOAuthSignupSource(signupSource)
	invitationRedeemCode, err := s.validateOAuthRegistrationInvitation(ctx, invitationCode)
	if err != nil {
		return err
	}
	if invitationRedeemCode != nil {
		if err := s.useOAuthRegistrationInvitation(ctx, invitationRedeemCode.ID, user.ID); err != nil {
			return ErrInvitationCodeInvalid
		}
	}

	s.updateOAuthSignupSource(ctx, user.ID, signupSource)
	grantPlan := s.resolveSignupGrantPlan(ctx, signupSource)
	s.assignSubscriptions(ctx, user.ID, grantPlan.Subscriptions, "auto assigned by signup defaults")
	return nil
}

// RollbackOAuthEmailAccountCreation removes a partially-created local account
// and restores any invitation code already consumed by that account.
func (s *AuthService) RollbackOAuthEmailAccountCreation(ctx context.Context, userID int64, invitationCode string) error {
	if s == nil || s.userRepo == nil || userID <= 0 {
		return ErrServiceUnavailable
	}
	if err := s.restoreOAuthRegistrationInvitation(ctx, invitationCode, userID); err != nil {
		return err
	}
	if err := s.userRepo.Delete(ctx, userID); err != nil {
		return fmt.Errorf("delete created oauth user: %w", err)
	}
	return nil
}

func (s *AuthService) restoreOAuthRegistrationInvitation(ctx context.Context, invitationCode string, userID int64) error {
	if s == nil || s.settingService == nil || !s.settingService.IsInvitationCodeEnabled(ctx) {
		return nil
	}
	if s.redeemRepo == nil && s.oauthEmailFlowClient(ctx) == nil {
		return ErrServiceUnavailable
	}

	invitationCode = strings.TrimSpace(invitationCode)
	if invitationCode == "" || userID <= 0 {
		return nil
	}

	redeemCode, err := s.loadOAuthRegistrationInvitation(ctx, invitationCode)
	if err != nil {
		if errors.Is(err, ErrRedeemCodeNotFound) {
			return nil
		}
		return fmt.Errorf("load invitation code: %w", err)
	}
	if redeemCode.Type != RedeemTypeInvitation || redeemCode.Status != StatusUsed || redeemCode.UsedBy == nil || *redeemCode.UsedBy != userID {
		return nil
	}

	redeemCode.Status = StatusUnused
	redeemCode.UsedBy = nil
	redeemCode.UsedAt = nil
	if err := s.updateOAuthRegistrationInvitation(ctx, redeemCode); err != nil {
		return fmt.Errorf("restore invitation code: %w", err)
	}
	return nil
}

func (s *AuthService) oauthEmailFlowClient(ctx context.Context) *dbent.Client {
	if s == nil || s.entClient == nil {
		return nil
	}
	if tx := dbent.TxFromContext(ctx); tx != nil {
		return tx.Client()
	}
	return s.entClient
}

func (s *AuthService) loadOAuthRegistrationInvitation(ctx context.Context, invitationCode string) (*RedeemCode, error) {
	if client := s.oauthEmailFlowClient(ctx); client != nil {
		entity, err := client.RedeemCode.Query().Where(redeemcode.CodeEQ(invitationCode)).Only(ctx)
		if err != nil {
			if dbent.IsNotFound(err) {
				return nil, ErrRedeemCodeNotFound
			}
			return nil, err
		}
		return &RedeemCode{
			ID:           entity.ID,
			Code:         entity.Code,
			Type:         entity.Type,
			Value:        entity.Value,
			Status:       entity.Status,
			UsedBy:       entity.UsedBy,
			UsedAt:       entity.UsedAt,
			Notes:        oauthEmailFlowStringValue(entity.Notes),
			CreatedAt:    entity.CreatedAt,
			GroupID:      entity.GroupID,
			ValidityDays: entity.ValidityDays,
		}, nil
	}
	return s.redeemRepo.GetByCode(ctx, invitationCode)
}

func (s *AuthService) useOAuthRegistrationInvitation(ctx context.Context, invitationID, userID int64) error {
	if client := s.oauthEmailFlowClient(ctx); client != nil {
		affected, err := client.RedeemCode.Update().
			Where(redeemcode.IDEQ(invitationID), redeemcode.StatusEQ(StatusUnused)).
			SetStatus(StatusUsed).
			SetUsedBy(userID).
			SetUsedAt(time.Now().UTC()).
			Save(ctx)
		if err != nil {
			return err
		}
		if affected == 0 {
			return ErrRedeemCodeUsed
		}
		return nil
	}
	return s.redeemRepo.Use(ctx, invitationID, userID)
}

func (s *AuthService) updateOAuthRegistrationInvitation(ctx context.Context, code *RedeemCode) error {
	if code == nil {
		return nil
	}
	if client := s.oauthEmailFlowClient(ctx); client != nil {
		update := client.RedeemCode.UpdateOneID(code.ID).
			SetCode(code.Code).
			SetType(code.Type).
			SetValue(code.Value).
			SetStatus(code.Status).
			SetNotes(code.Notes).
			SetValidityDays(code.ValidityDays)
		if code.UsedBy != nil {
			update = update.SetUsedBy(*code.UsedBy)
		} else {
			update = update.ClearUsedBy()
		}
		if code.UsedAt != nil {
			update = update.SetUsedAt(*code.UsedAt)
		} else {
			update = update.ClearUsedAt()
		}
		if code.GroupID != nil {
			update = update.SetGroupID(*code.GroupID)
		} else {
			update = update.ClearGroupID()
		}
		_, err := update.Save(ctx)
		return err
	}
	return s.redeemRepo.Update(ctx, code)
}

func (s *AuthService) updateOAuthSignupSource(ctx context.Context, userID int64, signupSource string) {
	client := s.oauthEmailFlowClient(ctx)
	if client == nil || userID <= 0 || strings.TrimSpace(signupSource) == "" {
		return
	}
	_ = client.User.UpdateOneID(userID).SetSignupSource(signupSource).Exec(ctx)
}

func oauthEmailFlowStringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

// ValidatePasswordCredentials checks the local password without completing the
// login flow. This is used by pending third-party account adoption flows before
// the external identity has been bound.
func (s *AuthService) ValidatePasswordCredentials(ctx context.Context, email, password string) (*User, error) {
	if s == nil {
		return nil, ErrServiceUnavailable
	}

	user, err := s.userRepo.GetByEmail(ctx, strings.TrimSpace(strings.ToLower(email)))
	if err != nil {
		if errors.Is(err, ErrUserNotFound) {
			return nil, ErrInvalidCredentials
		}
		return nil, ErrServiceUnavailable
	}
	if !user.IsActive() {
		return nil, ErrUserNotActive
	}
	if !s.CheckPassword(password, user.PasswordHash) {
		return nil, ErrInvalidCredentials
	}
	return user, nil
}

// RecordSuccessfulLogin updates last-login activity after a non-standard login
// flow finishes with a real session.
func (s *AuthService) RecordSuccessfulLogin(ctx context.Context, userID int64) {
	if s != nil && s.userRepo != nil && userID > 0 {
		user, err := s.userRepo.GetByID(ctx, userID)
		if err == nil && user != nil && !isReservedEmail(user.Email) {
			s.backfillEmailIdentityOnSuccessfulLogin(ctx, user)
		}
	}
	s.touchUserLogin(ctx, userID)
}
