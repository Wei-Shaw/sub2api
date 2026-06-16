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
REDACTED

func (s *AuthService) LoginOrRegisterVerifiedEmailOAuth(ctx context.Context, input EmailOAuthIdentityInput) (*TokenPair, *User, error) {
	return s.loginOrRegisterVerifiedEmailOAuth(ctx, input, "", "", "")
REDACTED

func (s *AuthService) LoginOrRegisterVerifiedEmailOAuthWithInvitation(
	ctx context.Context,
	input EmailOAuthIdentityInput,
	invitationCode string,
	affiliateCode string,
) (*TokenPair, *User, error) {
	return s.loginOrRegisterVerifiedEmailOAuth(ctx, input, invitationCode, affiliateCode, "")
REDACTED

func (s *AuthService) LoginOrRegisterVerifiedEmailOAuthWithSignupCodes(
	ctx context.Context,
	input EmailOAuthIdentityInput,
	invitationCode string,
	affiliateCode string,
	promoCode string,
) (*TokenPair, *User, error) {
	return s.loginOrRegisterVerifiedEmailOAuth(ctx, input, invitationCode, affiliateCode, promoCode)
REDACTED

func (s *AuthService) loginOrRegisterVerifiedEmailOAuth(
	ctx context.Context,
	input EmailOAuthIdentityInput,
	invitationCode string,
	affiliateCode string,
	promoCode string,
) (*TokenPair, *User, error) {
	if s == nil || s.userRepo == nil || s.entClient == nil {
		return nil, nil, ErrServiceUnavailable
REDACTED

	providerType := normalizeOAuthSignupSource(input.ProviderType)
	if providerType != "github" && providerType != "google" && providerType != "oidc" {
		return nil, nil, infraerrors.BadRequest("OAUTH_PROVIDER_INVALID", "oauth provider is invalid")
REDACTED
	providerKey := strings.TrimSpace(input.ProviderKey)
	if providerKey == "" {
		providerKey = providerType
REDACTED
	providerSubject := strings.TrimSpace(input.ProviderSubject)
	if providerSubject == "" {
		return nil, nil, infraerrors.BadRequest("OAUTH_SUBJECT_MISSING", "oauth subject is missing")
REDACTED
	if !input.EmailVerified {
		return nil, nil, infraerrors.Forbidden("OAUTH_EMAIL_NOT_VERIFIED", "oauth email is not verified")
REDACTED

	email := strings.TrimSpace(strings.ToLower(input.Email))
	if email == "" || len(email) > 255 {
		return nil, nil, infraerrors.BadRequest("INVALID_EMAIL", "invalid email")
REDACTED
	if _, err := mail.ParseAddress(email); err != nil {
		return nil, nil, infraerrors.BadRequest("INVALID_EMAIL", "invalid email")
REDACTED
	if isReservedEmail(email) {
		return nil, nil, ErrEmailReserved
REDACTED
	if err := s.validateRegistrationEmailPolicy(ctx, email); err != nil {
		return nil, nil, err
REDACTED

	identityUser, err := s.findEmailOAuthIdentityOwner(ctx, providerType, providerKey, providerSubject)
	if err != nil {
		return nil, nil, err
REDACTED
	if identityUser != nil && !strings.EqualFold(strings.TrimSpace(identityUser.Email), email) {
		return nil, nil, infraerrors.Conflict("AUTH_IDENTITY_EMAIL_MISMATCH", "oauth identity belongs to a different email")
REDACTED

	user := identityUser
	created := false
	if user == nil {
		user, err = s.userRepo.GetByEmail(ctx, email)
		if err != nil {
			if errors.Is(err, ErrUserNotFound) {
				user, err = s.createEmailOAuthUser(ctx, email, input.Username, providerType, invitationCode, affiliateCode)
				if err != nil {
					return nil, nil, err
			REDACTED
				created = true
		REDACTED else {
				logger.LegacyPrintf("service.auth", "[Auth] Database error during %s oauth login: %v", providerType, err)
				return nil, nil, ErrServiceUnavailable
		REDACTED
	REDACTED
REDACTED

	if !user.IsActive() {
		return nil, nil, ErrUserNotActive
REDACTED
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
REDACTED); err != nil {
		return nil, nil, err
REDACTED

	if user.Username == "" && strings.TrimSpace(input.Username) != "" {
		user.Username = strings.TrimSpace(input.Username)
		if err := s.userRepo.Update(ctx, user); err != nil {
			logger.LegacyPrintf("service.auth", "[Auth] Failed to update username after %s oauth login: %v", providerType, err)
	REDACTED
REDACTED
	if !created {
		if err := s.ApplyProviderDefaultSettingsOnFirstBind(ctx, user.ID, providerType); err != nil {
			logger.LegacyPrintf("service.auth", "[Auth] Failed to apply %s first bind defaults: %v", providerType, err)
	REDACTED
REDACTED else {
		user = s.applyOAuthSignupPromoCode(ctx, user, promoCode)
REDACTED
	s.RecordSuccessfulLogin(ctx, user.ID)

	tokenPair, err := s.GenerateTokenPair(ctx, user, "")
	if err != nil {
		return nil, nil, fmt.Errorf("generate token pair: %w", err)
REDACTED
	return tokenPair, user, nil
REDACTED

func (s *AuthService) createEmailOAuthUser(ctx context.Context, email, username, providerType, invitationCode, affiliateCode string) (*User, error) {
	if s.settingService == nil || !s.settingService.IsRegistrationEnabled(ctx) {
		return nil, ErrRegDisabled
REDACTED
	invitationRedeemCode, err := s.validateOAuthRegistrationInvitation(ctx, invitationCode)
	if err != nil {
		if errors.Is(err, ErrInvitationCodeRequired) {
			return nil, ErrOAuthInvitationRequired
	REDACTED
		return nil, err
REDACTED

	randomPassword, err := randomHexString(32)
	if err != nil {
		return nil, ErrServiceUnavailable
REDACTED
	hashedPassword, err := s.HashPassword(randomPassword)
	if err != nil {
		return nil, fmt.Errorf("hash password: %w", err)
REDACTED
	grantPlan := s.resolveSignupGrantPlan(ctx, providerType)
	var defaultRPMLimit int
	if s.settingService != nil {
		defaultRPMLimit = s.settingService.GetDefaultUserRPMLimit(ctx)
REDACTED
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
REDACTED
	if err := s.userRepo.Create(ctx, user); err != nil {
		if errors.Is(err, ErrEmailExists) {
			existing, loadErr := s.userRepo.GetByEmail(ctx, email)
			if loadErr != nil {
				return nil, ErrServiceUnavailable
		REDACTED
			return existing, nil
	REDACTED
		return nil, ErrServiceUnavailable
REDACTED
	s.postAuthUserBootstrap(ctx, user, providerType, false)
	s.assignSubscriptions(ctx, user.ID, grantPlan.Subscriptions, "auto assigned by signup defaults")
	// snapshot user × platform quota（fail-open）
	_ = s.snapshotPlatformQuotaDefaults(ctx, user.ID, &grantPlan)
	s.bindOAuthAffiliate(ctx, user.ID, affiliateCode)
	if invitationRedeemCode != nil {
		if err := s.useOAuthRegistrationInvitation(ctx, invitationRedeemCode.ID, user.ID); err != nil {
			_ = s.RollbackOAuthEmailAccountCreation(ctx, user.ID, invitationCode)
			return nil, ErrInvitationCodeInvalid
	REDACTED
REDACTED
	return user, nil
REDACTED

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
	REDACTED
		return nil, infraerrors.InternalServer("AUTH_IDENTITY_LOOKUP_FAILED", "failed to inspect auth identity ownership").WithCause(err)
REDACTED
	user, err := s.userRepo.GetByID(ctx, identity.UserID)
	if err != nil {
		if errors.Is(err, ErrUserNotFound) {
			return nil, nil
	REDACTED
		return nil, ErrServiceUnavailable
REDACTED
	return user, nil
REDACTED

func (s *AuthService) ensureEmailOAuthIdentity(ctx context.Context, userID int64, input EmailOAuthIdentityInput) error {
	metadata := map[string]any{
		"email":          strings.TrimSpace(strings.ToLower(input.Email)),
		"email_verified": input.EmailVerified,
REDACTED
	for key, value := range input.UpstreamMetadata {
		metadata[key] = value
REDACTED
	if strings.TrimSpace(input.Username) != "" {
		metadata["username"] = strings.TrimSpace(input.Username)
REDACTED
	if strings.TrimSpace(input.DisplayName) != "" {
		metadata["display_name"] = strings.TrimSpace(input.DisplayName)
REDACTED
	if strings.TrimSpace(input.AvatarURL) != "" {
		metadata["avatar_url"] = strings.TrimSpace(input.AvatarURL)
REDACTED

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
REDACTED
	if identity != nil {
		if identity.UserID != userID {
			return infraerrors.Conflict("AUTH_IDENTITY_OWNERSHIP_CONFLICT", "auth identity already belongs to another user")
	REDACTED
		_, err = s.entClient.AuthIdentity.UpdateOneID(identity.ID).
			SetMetadata(metadata).
			Save(ctx)
		return err
REDACTED
	_, err = s.entClient.AuthIdentity.Create().
		SetUserID(userID).
		SetProviderType(providerType).
		SetProviderKey(providerKey).
		SetProviderSubject(providerSubject).
		SetMetadata(metadata).
		Save(ctx)
	return err
REDACTED
