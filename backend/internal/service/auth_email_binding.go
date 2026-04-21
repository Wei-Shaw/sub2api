package service

import (
	"context"
	"errors"
	"fmt"
	"net/mail"
	"strings"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

// BindEmailIdentity verifies and binds a local email/password identity to the current user.
func (s *AuthService) BindEmailIdentity(
	ctx context.Context,
	userID int64,
	email string,
	verifyCode string,
	password string,
) (*User, error) {
	if s == nil {
		return nil, ErrServiceUnavailable
REDACTED

	normalizedEmail, err := normalizeEmailForIdentityBinding(email)
	if err != nil {
		return nil, err
REDACTED
	if isReservedEmail(normalizedEmail) {
		return nil, ErrEmailReserved
REDACTED
	if strings.TrimSpace(password) == "" {
		return nil, ErrPasswordRequired
REDACTED
	if err := s.VerifyOAuthEmailCode(ctx, normalizedEmail, verifyCode); err != nil {
		return nil, err
REDACTED

	currentUser, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		return nil, err
REDACTED

	existingUser, err := s.userRepo.GetByEmail(ctx, normalizedEmail)
	switch {
	case err == nil && existingUser != nil && existingUser.ID != userID:
		return nil, ErrEmailExists
	case err != nil && !errors.Is(err, ErrUserNotFound):
		return nil, ErrServiceUnavailable
REDACTED

	hashedPassword, err := s.HashPassword(password)
	if err != nil {
		return nil, fmt.Errorf("hash password: %w", err)
REDACTED

	firstRealEmailBind := !hasBindableEmailIdentitySubject(currentUser.Email)
	currentUser.Email = normalizedEmail
	currentUser.PasswordHash = hashedPassword
	if err := s.userRepo.Update(ctx, currentUser); err != nil {
		if errors.Is(err, ErrEmailExists) {
			return nil, ErrEmailExists
	REDACTED
		return nil, ErrServiceUnavailable
REDACTED

	if firstRealEmailBind {
		if err := s.ApplyProviderDefaultSettingsOnFirstBind(ctx, userID, "email"); err != nil {
			return nil, fmt.Errorf("apply email first bind defaults: %w", err)
	REDACTED
REDACTED

	return currentUser, nil
REDACTED

// SendEmailIdentityBindCode sends a verification code for authenticated email binding flows.
func (s *AuthService) SendEmailIdentityBindCode(ctx context.Context, userID int64, email string) error {
	if s == nil {
		return ErrServiceUnavailable
REDACTED

	normalizedEmail, err := normalizeEmailForIdentityBinding(email)
	if err != nil {
		return err
REDACTED
	if isReservedEmail(normalizedEmail) {
		return ErrEmailReserved
REDACTED
	if s.emailService == nil {
		return ErrServiceUnavailable
REDACTED
	if _, err := s.userRepo.GetByID(ctx, userID); err != nil {
		if errors.Is(err, ErrUserNotFound) {
			return ErrUserNotFound
	REDACTED
		return ErrServiceUnavailable
REDACTED

	existingUser, err := s.userRepo.GetByEmail(ctx, normalizedEmail)
	switch {
	case err == nil && existingUser != nil && existingUser.ID != userID:
		return ErrEmailExists
	case err != nil && !errors.Is(err, ErrUserNotFound):
		return ErrServiceUnavailable
REDACTED

	siteName := "Sub2API"
	if s.settingService != nil {
		siteName = s.settingService.GetSiteName(ctx)
REDACTED
	return s.emailService.SendVerifyCode(ctx, normalizedEmail, siteName)
REDACTED

func normalizeEmailForIdentityBinding(email string) (string, error) {
	normalized := strings.ToLower(strings.TrimSpace(email))
	if normalized == "" || len(normalized) > 255 {
		return "", infraerrors.BadRequest("INVALID_EMAIL", "invalid email")
REDACTED
	if _, err := mail.ParseAddress(normalized); err != nil {
		return "", infraerrors.BadRequest("INVALID_EMAIL", "invalid email")
REDACTED
	return normalized, nil
REDACTED

func hasBindableEmailIdentitySubject(email string) bool {
	normalized := strings.ToLower(strings.TrimSpace(email))
	return normalized != "" && !isReservedEmail(normalized)
REDACTED
