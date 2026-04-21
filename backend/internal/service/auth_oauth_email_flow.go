package service

import (
	"context"
	"errors"
	"fmt"
	"net/mail"
	"strings"
	"time"
)

func normalizeOAuthSignupSource(signupSource string) string {
	signupSource = strings.TrimSpace(strings.ToLower(signupSource))
	if signupSource == "" {
		return "email"
REDACTED
	return signupSource
REDACTED

// SendPendingOAuthVerifyCode sends a local verification code for pending OAuth
// account-creation flows without relying on the public registration gate.
func (s *AuthService) SendPendingOAuthVerifyCode(ctx context.Context, email string) (*SendVerifyCodeResult, error) {
	email = strings.TrimSpace(strings.ToLower(email))
	if email == "" {
		return nil, ErrEmailVerifyRequired
REDACTED
	if _, err := mail.ParseAddress(email); err != nil {
		return nil, ErrEmailVerifyRequired
REDACTED
	if isReservedEmail(email) {
		return nil, ErrEmailReserved
REDACTED
	if s == nil || s.emailService == nil {
		return nil, ErrServiceUnavailable
REDACTED

	siteName := "Sub2API"
	if s.settingService != nil {
		siteName = s.settingService.GetSiteName(ctx)
REDACTED
	if err := s.emailService.SendVerifyCode(ctx, email, siteName); err != nil {
		return nil, err
REDACTED
	return &SendVerifyCodeResult{
		Countdown: int(verifyCodeCooldown / time.Second),
REDACTED, nil
REDACTED

func (s *AuthService) validateOAuthRegistrationInvitation(ctx context.Context, invitationCode string) (*RedeemCode, error) {
	if s == nil || s.settingService == nil || !s.settingService.IsInvitationCodeEnabled(ctx) {
		return nil, nil
REDACTED
	if s.redeemRepo == nil {
		return nil, ErrServiceUnavailable
REDACTED

	invitationCode = strings.TrimSpace(invitationCode)
	if invitationCode == "" {
		return nil, ErrInvitationCodeRequired
REDACTED

	redeemCode, err := s.redeemRepo.GetByCode(ctx, invitationCode)
	if err != nil {
		return nil, ErrInvitationCodeInvalid
REDACTED
	if redeemCode.Type != RedeemTypeInvitation || redeemCode.Status != StatusUnused {
		return nil, ErrInvitationCodeInvalid
REDACTED
	return redeemCode, nil
REDACTED

// VerifyOAuthEmailCode verifies the locally entered email verification code for
// third-party signup and binding flows. This is intentionally independent from
// the global registration email verification toggle.
func (s *AuthService) VerifyOAuthEmailCode(ctx context.Context, email, verifyCode string) error {
	email = strings.TrimSpace(strings.ToLower(email))
	verifyCode = strings.TrimSpace(verifyCode)

	if email == "" {
		return ErrEmailVerifyRequired
REDACTED
	if verifyCode == "" {
		return ErrEmailVerifyRequired
REDACTED
	if s == nil || s.emailService == nil {
		return ErrServiceUnavailable
REDACTED
	return s.emailService.VerifyCode(ctx, email, verifyCode)
REDACTED

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
REDACTED
	if s.settingService == nil || !s.settingService.IsRegistrationEnabled(ctx) {
		return nil, nil, ErrRegDisabled
REDACTED

	email = strings.TrimSpace(strings.ToLower(email))
	if isReservedEmail(email) {
		return nil, nil, ErrEmailReserved
REDACTED
	if err := s.validateRegistrationEmailPolicy(ctx, email); err != nil {
		return nil, nil, err
REDACTED
	if err := s.VerifyOAuthEmailCode(ctx, email, verifyCode); err != nil {
		return nil, nil, err
REDACTED

	if _, err := s.validateOAuthRegistrationInvitation(ctx, invitationCode); err != nil {
		return nil, nil, err
REDACTED

	existsEmail, err := s.userRepo.ExistsByEmail(ctx, email)
	if err != nil {
		return nil, nil, ErrServiceUnavailable
REDACTED
	if existsEmail {
		return nil, nil, ErrEmailExists
REDACTED

	hashedPassword, err := s.HashPassword(password)
	if err != nil {
		return nil, nil, fmt.Errorf("hash password: %w", err)
REDACTED

	signupSource = strings.TrimSpace(strings.ToLower(signupSource))
	if signupSource == "" {
		signupSource = "email"
REDACTED
	grantPlan := s.resolveSignupGrantPlan(ctx, signupSource)

	user := &User{
		Email:        email,
		PasswordHash: hashedPassword,
		Role:         RoleUser,
		Balance:      grantPlan.Balance,
		Concurrency:  grantPlan.Concurrency,
		Status:       StatusActive,
REDACTED

	if err := s.userRepo.Create(ctx, user); err != nil {
		if errors.Is(err, ErrEmailExists) {
			return nil, nil, ErrEmailExists
	REDACTED
		return nil, nil, ErrServiceUnavailable
REDACTED

	tokenPair, err := s.GenerateTokenPair(ctx, user, "")
	if err != nil {
		_ = s.RollbackOAuthEmailAccountCreation(ctx, user.ID, "")
		return nil, nil, fmt.Errorf("generate token pair: %w", err)
REDACTED
	return tokenPair, user, nil
REDACTED

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
REDACTED

	signupSource = normalizeOAuthSignupSource(signupSource)
	invitationRedeemCode, err := s.validateOAuthRegistrationInvitation(ctx, invitationCode)
	if err != nil {
		return err
REDACTED
	if invitationRedeemCode != nil {
		if err := s.redeemRepo.Use(ctx, invitationRedeemCode.ID, user.ID); err != nil {
			return ErrInvitationCodeInvalid
	REDACTED
REDACTED

	s.postAuthUserBootstrap(ctx, user, signupSource, false)
	grantPlan := s.resolveSignupGrantPlan(ctx, signupSource)
	s.assignSubscriptions(ctx, user.ID, grantPlan.Subscriptions, "auto assigned by signup defaults")
	return nil
REDACTED

// RollbackOAuthEmailAccountCreation removes a partially-created local account
// and restores any invitation code already consumed by that account.
func (s *AuthService) RollbackOAuthEmailAccountCreation(ctx context.Context, userID int64, invitationCode string) error {
	if s == nil || s.userRepo == nil || userID <= 0 {
		return ErrServiceUnavailable
REDACTED
	if err := s.restoreOAuthRegistrationInvitation(ctx, invitationCode, userID); err != nil {
		return err
REDACTED
	if err := s.userRepo.Delete(ctx, userID); err != nil {
		return fmt.Errorf("delete created oauth user: %w", err)
REDACTED
	return nil
REDACTED

func (s *AuthService) restoreOAuthRegistrationInvitation(ctx context.Context, invitationCode string, userID int64) error {
	if s == nil || s.settingService == nil || !s.settingService.IsInvitationCodeEnabled(ctx) {
		return nil
REDACTED
	if s.redeemRepo == nil {
		return ErrServiceUnavailable
REDACTED

	invitationCode = strings.TrimSpace(invitationCode)
	if invitationCode == "" || userID <= 0 {
		return nil
REDACTED

	redeemCode, err := s.redeemRepo.GetByCode(ctx, invitationCode)
	if err != nil {
		if errors.Is(err, ErrRedeemCodeNotFound) {
			return nil
	REDACTED
		return fmt.Errorf("load invitation code: %w", err)
REDACTED
	if redeemCode.Type != RedeemTypeInvitation || redeemCode.Status != StatusUsed || redeemCode.UsedBy == nil || *redeemCode.UsedBy != userID {
		return nil
REDACTED

	redeemCode.Status = StatusUnused
	redeemCode.UsedBy = nil
	redeemCode.UsedAt = nil
	if err := s.redeemRepo.Update(ctx, redeemCode); err != nil {
		return fmt.Errorf("restore invitation code: %w", err)
REDACTED
	return nil
REDACTED

// ValidatePasswordCredentials checks the local password without completing the
// login flow. This is used by pending third-party account adoption flows before
// the external identity has been bound.
func (s *AuthService) ValidatePasswordCredentials(ctx context.Context, email, password string) (*User, error) {
	if s == nil {
		return nil, ErrServiceUnavailable
REDACTED

	user, err := s.userRepo.GetByEmail(ctx, strings.TrimSpace(strings.ToLower(email)))
	if err != nil {
		if errors.Is(err, ErrUserNotFound) {
			return nil, ErrInvalidCredentials
	REDACTED
		return nil, ErrServiceUnavailable
REDACTED
	if !user.IsActive() {
		return nil, ErrUserNotActive
REDACTED
	if !s.CheckPassword(password, user.PasswordHash) {
		return nil, ErrInvalidCredentials
REDACTED
	return user, nil
REDACTED

// RecordSuccessfulLogin updates last-login activity after a non-standard login
// flow finishes with a real session.
func (s *AuthService) RecordSuccessfulLogin(ctx context.Context, userID int64) {
	if s != nil && s.userRepo != nil && userID > 0 {
		user, err := s.userRepo.GetByID(ctx, userID)
		if err == nil {
			s.backfillEmailIdentityOnSuccessfulLogin(ctx, user)
	REDACTED
REDACTED
	s.touchUserLogin(ctx, userID)
REDACTED
