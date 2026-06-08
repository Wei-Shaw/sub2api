package service

import (
	"context"
	"errors"
	"fmt"
	"net/mail"
	"strings"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/authidentity"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
)

type EmailOAuthIdentityInput struct {
	ProviderType     string
	ProviderKey      string
	ProviderSubject  string
	Email            string
	EmailVerified    bool
	Username         string
	DisplayName      string
	AvatarURL        string
	UpstreamMetadata map[string]any
}

func (s *AuthService) LoginOrRegisterVerifiedEmailOAuth(ctx context.Context, input EmailOAuthIdentityInput) (*TokenPair, *User, error) {
	return s.loginOrRegisterVerifiedEmailOAuth(ctx, input, "", "")
}

func (s *AuthService) LoginOrRegisterVerifiedEmailOAuthWithInvitation(
	ctx context.Context,
	input EmailOAuthIdentityInput,
	invitationCode string,
	affiliateCode string,
) (*TokenPair, *User, error) {
	return s.loginOrRegisterVerifiedEmailOAuth(ctx, input, invitationCode, affiliateCode)
}

func (s *AuthService) loginOrRegisterVerifiedEmailOAuth(
	ctx context.Context,
	input EmailOAuthIdentityInput,
	invitationCode string,
	affiliateCode string,
) (*TokenPair, *User, error) {
	if s == nil || s.userRepo == nil || s.entClient == nil {
		return nil, nil, ErrServiceUnavailable
	}

	providerType := normalizeOAuthSignupSource(input.ProviderType)
	if providerType != "github" && providerType != "google" {
		return nil, nil, infraerrors.BadRequest("OAUTH_PROVIDER_INVALID", "oauth provider is invalid")
	}
	providerKey := strings.TrimSpace(input.ProviderKey)
	if providerKey == "" {
		providerKey = providerType
	}
	providerSubject := strings.TrimSpace(input.ProviderSubject)
	if providerSubject == "" {
		return nil, nil, infraerrors.BadRequest("OAUTH_SUBJECT_MISSING", "oauth subject is missing")
	}
	if !input.EmailVerified {
		return nil, nil, infraerrors.Forbidden("OAUTH_EMAIL_NOT_VERIFIED", "oauth email is not verified")
	}

	email := strings.TrimSpace(strings.ToLower(input.Email))
	if email == "" || len(email) > 255 {
		return nil, nil, infraerrors.BadRequest("INVALID_EMAIL", "invalid email")
	}
	if _, err := mail.ParseAddress(email); err != nil {
		return nil, nil, infraerrors.BadRequest("INVALID_EMAIL", "invalid email")
	}
	if isReservedEmail(email) {
		return nil, nil, ErrEmailReserved
	}
	if err := s.validateRegistrationEmailPolicy(ctx, email); err != nil {
		return nil, nil, err
	}

	identityUser, err := s.findEmailOAuthIdentityOwner(ctx, providerType, providerKey, providerSubject)
	if err != nil {
		return nil, nil, err
	}
	if identityUser != nil && !strings.EqualFold(strings.TrimSpace(identityUser.Email), email) {
		return nil, nil, infraerrors.Conflict("AUTH_IDENTITY_EMAIL_MISMATCH", "oauth identity belongs to a different email")
	}

	user := identityUser
	created := false
	if user == nil {
		user, err = s.userRepo.GetByEmail(ctx, email)
		if err != nil {
			if errors.Is(err, ErrUserNotFound) {
				user, err = s.createEmailOAuthUser(ctx, email, input.Username, providerType, invitationCode, affiliateCode)
				if err != nil {
					return nil, nil, err
				}
				created = true
			} else {
				logger.LegacyPrintf("service.auth", "[Auth] Database error during %s oauth login: %v", providerType, err)
				return nil, nil, ErrServiceUnavailable
			}
		}
	}

	if !user.IsActive() {
		return nil, nil, ErrUserNotActive
	}
	if err := s.ensureEmailOAuthIdentity(ctx, user.ID, EmailOAuthIdentityInput{
		ProviderType:     providerType,
		ProviderKey:      providerKey,
		ProviderSubject:  providerSubject,
		Email:            email,
		EmailVerified:    input.EmailVerified,
		Username:         input.Username,
		DisplayName:      input.DisplayName,
		AvatarURL:        input.AvatarURL,
		UpstreamMetadata: input.UpstreamMetadata,
	}); err != nil {
		return nil, nil, err
	}

	if user.Username == "" && strings.TrimSpace(input.Username) != "" {
		user.Username = strings.TrimSpace(input.Username)
		if err := s.userRepo.Update(ctx, user); err != nil {
			logger.LegacyPrintf("service.auth", "[Auth] Failed to update username after %s oauth login: %v", providerType, err)
		}
	}
	if !created {
		if err := s.ApplyProviderDefaultSettingsOnFirstBind(ctx, user.ID, providerType); err != nil {
			logger.LegacyPrintf("service.auth", "[Auth] Failed to apply %s first bind defaults: %v", providerType, err)
		}
	}
	s.RecordSuccessfulLogin(ctx, user.ID)

	tokenPair, err := s.GenerateTokenPair(ctx, user, "")
	if err != nil {
		return nil, nil, fmt.Errorf("generate token pair: %w", err)
	}
	return tokenPair, user, nil
}

func (s *AuthService) createEmailOAuthUser(ctx context.Context, email, username, providerType, invitationCode, affiliateCode string) (*User, error) {
	if s.settingService == nil || !s.settingService.IsRegistrationEnabled(ctx) {
		return nil, ErrRegDisabled
	}
	invitationRedeemCode, err := s.validateOAuthRegistrationInvitation(ctx, invitationCode)
	if err != nil {
		if errors.Is(err, ErrInvitationCodeRequired) {
			return nil, ErrOAuthInvitationRequired
		}
		return nil, err
	}

	randomPassword, err := randomHexString(32)
	if err != nil {
		return nil, ErrServiceUnavailable
	}
	hashedPassword, err := s.HashPassword(randomPassword)
	if err != nil {
		return nil, fmt.Errorf("hash password: %w", err)
	}
	grantPlan := s.resolveSignupGrantPlan(ctx, providerType)
	var defaultRPMLimit int
	if s.settingService != nil {
		defaultRPMLimit = s.settingService.GetDefaultUserRPMLimit(ctx)
	}
	user := &User{
		Email:        email,
		Username:     strings.TrimSpace(username),
		PasswordHash: hashedPassword,
		Role:         RoleUser,
		Balance:      grantPlan.Balance,
		Concurrency:  grantPlan.Concurrency,
		RPMLimit:     defaultRPMLimit,
		Status:       StatusActive,
		SignupSource: providerType,
	}
	if err := s.userRepo.Create(ctx, user); err != nil {
		if errors.Is(err, ErrEmailExists) {
			existing, loadErr := s.userRepo.GetByEmail(ctx, email)
			if loadErr != nil {
				return nil, ErrServiceUnavailable
			}
			return existing, nil
		}
		return nil, ErrServiceUnavailable
	}
	s.postAuthUserBootstrap(ctx, user, providerType, false)
	s.assignSubscriptions(ctx, user.ID, grantPlan.Subscriptions, "auto assigned by signup defaults")
	s.bindOAuthAffiliate(ctx, user.ID, affiliateCode)
	if invitationRedeemCode != nil {
		if err := s.useOAuthRegistrationInvitation(ctx, invitationRedeemCode.ID, user.ID); err != nil {
			_ = s.RollbackOAuthEmailAccountCreation(ctx, user.ID, invitationCode)
			return nil, ErrInvitationCodeInvalid
		}
	}
	return user, nil
}

func (s *AuthService) findEmailOAuthIdentityOwner(ctx context.Context, providerType, providerKey, providerSubject string) (*User, error) {
	identity, err := s.entClient.AuthIdentity.Query().
		Where(
			authidentity.ProviderTypeEQ(providerType),
			authidentity.ProviderKeyEQ(providerKey),
			authidentity.ProviderSubjectEQ(providerSubject),
		).
		Only(ctx)
	if err != nil {
		if dbent.IsNotFound(err) {
			return nil, nil
		}
		return nil, infraerrors.InternalServer("AUTH_IDENTITY_LOOKUP_FAILED", "failed to inspect auth identity ownership").WithCause(err)
	}
	user, err := s.userRepo.GetByID(ctx, identity.UserID)
	if err != nil {
		if errors.Is(err, ErrUserNotFound) {
			return nil, nil
		}
		return nil, ErrServiceUnavailable
	}
	return user, nil
}

func (s *AuthService) ensureEmailOAuthIdentity(ctx context.Context, userID int64, input EmailOAuthIdentityInput) error {
	metadata := map[string]any{
		"email":          strings.TrimSpace(strings.ToLower(input.Email)),
		"email_verified": input.EmailVerified,
	}
	for key, value := range input.UpstreamMetadata {
		metadata[key] = value
	}
	if strings.TrimSpace(input.Username) != "" {
		metadata["username"] = strings.TrimSpace(input.Username)
	}
	if strings.TrimSpace(input.DisplayName) != "" {
		metadata["display_name"] = strings.TrimSpace(input.DisplayName)
	}
	if strings.TrimSpace(input.AvatarURL) != "" {
		metadata["avatar_url"] = strings.TrimSpace(input.AvatarURL)
	}

	providerType := normalizeOAuthSignupSource(input.ProviderType)
	providerKey := strings.TrimSpace(input.ProviderKey)
	providerSubject := strings.TrimSpace(input.ProviderSubject)
	identity, err := s.entClient.AuthIdentity.Query().
		Where(
			authidentity.ProviderTypeEQ(providerType),
			authidentity.ProviderKeyEQ(providerKey),
			authidentity.ProviderSubjectEQ(providerSubject),
		).
		Only(ctx)
	if err != nil && !dbent.IsNotFound(err) {
		return infraerrors.InternalServer("AUTH_IDENTITY_LOOKUP_FAILED", "failed to inspect auth identity ownership").WithCause(err)
	}
	if identity != nil {
		if identity.UserID != userID {
			return infraerrors.Conflict("AUTH_IDENTITY_OWNERSHIP_CONFLICT", "auth identity already belongs to another user")
		}
		_, err = s.entClient.AuthIdentity.UpdateOneID(identity.ID).
			SetMetadata(metadata).
			Save(ctx)
		return err
	}
	_, err = s.entClient.AuthIdentity.Create().
		SetUserID(userID).
		SetProviderType(providerType).
		SetProviderKey(providerKey).
		SetProviderSubject(providerSubject).
		SetMetadata(metadata).
		Save(ctx)
	return err
}
