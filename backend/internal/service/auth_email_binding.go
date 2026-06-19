package service

import (
	"context"
	"errors"
	"fmt"
	"net/mail"
	"strings"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/authidentity"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
)

// BindEmailIdentity verifies and binds a local email/password identity to the
// current user, or replaces the existing bound primary email.
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
	if err := s.validateRegistrationEmailPolicy(ctx, normalizedEmail); err != nil {
		return nil, err
REDACTED

	currentUser, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		return nil, err
REDACTED
	firstRealEmailBind := !hasBindableEmailIdentitySubject(currentUser.Email)
	if firstRealEmailBind && len(password) < 6 {
		return nil, infraerrors.BadRequest("PASSWORD_TOO_SHORT", "password must be at least 6 characters")
REDACTED
	if !firstRealEmailBind && !s.CheckPassword(password, currentUser.PasswordHash) {
		return nil, ErrPasswordIncorrect
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

	if s.entClient != nil {
		if err := s.updateBoundEmailIdentityTx(ctx, currentUser, normalizedEmail, hashedPassword, firstRealEmailBind); err != nil {
			return nil, err
	REDACTED
		s.revokeEmailIdentitySessions(ctx, userID)
		return currentUser, nil
REDACTED

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

	s.revokeEmailIdentitySessions(ctx, userID)
	return currentUser, nil
REDACTED

// SendEmailIdentityBindCode sends a verification code for authenticated email binding flows.
func (s *AuthService) SendEmailIdentityBindCode(ctx context.Context, userID int64, email string, locale ...string) error {
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
	if err := s.validateRegistrationEmailPolicy(ctx, normalizedEmail); err != nil {
		return err
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
	return s.emailService.SendVerifyCode(ctx, normalizedEmail, siteName, firstEmailLocale(locale))
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

func (s *AuthService) updateBoundEmailIdentityTx(
	ctx context.Context,
	currentUser *User,
	email string,
	hashedPassword string,
	applyFirstBindDefaults bool,
) error {
	if tx := dbent.TxFromContext(ctx); tx != nil {
		return s.updateBoundEmailIdentityWithClient(ctx, tx.Client(), currentUser, email, hashedPassword, applyFirstBindDefaults)
REDACTED

	tx, err := s.entClient.Tx(ctx)
	if err != nil {
		return ErrServiceUnavailable
REDACTED
	defer func() { _ = tx.Rollback() REDACTED()

	txCtx := dbent.NewTxContext(ctx, tx)
	if err := s.updateBoundEmailIdentityWithClient(txCtx, tx.Client(), currentUser, email, hashedPassword, applyFirstBindDefaults); err != nil {
		return err
REDACTED
	if err := tx.Commit(); err != nil {
		return ErrServiceUnavailable
REDACTED
	return nil
REDACTED

func (s *AuthService) updateBoundEmailIdentityWithClient(
	ctx context.Context,
	client *dbent.Client,
	currentUser *User,
	email string,
	hashedPassword string,
	applyFirstBindDefaults bool,
) error {
	if client == nil || currentUser == nil || currentUser.ID <= 0 {
		return ErrServiceUnavailable
REDACTED

	oldEmail := currentUser.Email
	if _, err := client.User.UpdateOneID(currentUser.ID).
		SetEmail(email).
		SetPasswordHash(hashedPassword).
		Save(ctx); err != nil {
		if dbent.IsConstraintError(err) {
			return ErrEmailExists
	REDACTED
		return ErrServiceUnavailable
REDACTED

	if err := replaceBoundEmailAuthIdentityWithClient(ctx, client, currentUser.ID, oldEmail, email, "auth_service_email_bind"); err != nil {
		if errors.Is(err, ErrEmailExists) {
			return ErrEmailExists
	REDACTED
		return ErrServiceUnavailable
REDACTED

	if applyFirstBindDefaults {
		if err := s.ApplyProviderDefaultSettingsOnFirstBind(ctx, currentUser.ID, "email"); err != nil {
			return fmt.Errorf("apply email first bind defaults: %w", err)
	REDACTED
REDACTED

	updatedUser, err := client.User.Get(ctx, currentUser.ID)
	if err != nil {
		return ErrServiceUnavailable
REDACTED
	currentUser.Email = updatedUser.Email
	currentUser.PasswordHash = updatedUser.PasswordHash
	currentUser.Balance = updatedUser.Balance
	currentUser.Concurrency = updatedUser.Concurrency
	currentUser.UpdatedAt = updatedUser.UpdatedAt
	return nil
REDACTED

func (s *AuthService) revokeEmailIdentitySessions(ctx context.Context, userID int64) {
	if err := s.RevokeAllUserSessions(ctx, userID); err != nil {
		logger.LegacyPrintf("service.auth", "[Auth] Failed to revoke refresh sessions after email identity bind for user %d: %v", userID, err)
REDACTED
REDACTED

func replaceBoundEmailAuthIdentityWithClient(
	ctx context.Context,
	client *dbent.Client,
	userID int64,
	oldEmail string,
	newEmail string,
	source string,
) error {
	newSubject := normalizeBoundEmailAuthIdentitySubject(newEmail)
	if err := ensureBoundEmailAuthIdentityWithClient(ctx, client, userID, newSubject, source); err != nil {
		return err
REDACTED

	oldSubject := normalizeBoundEmailAuthIdentitySubject(oldEmail)
	if oldSubject == "" || oldSubject == newSubject {
		return nil
REDACTED

	_, err := client.AuthIdentity.Delete().
		Where(
			authidentity.UserIDEQ(userID),
			authidentity.ProviderTypeEQ("email"),
			authidentity.ProviderKeyEQ("email"),
			authidentity.ProviderSubjectEQ(oldSubject),
		).
		Exec(ctx)
	return err
REDACTED

func ensureBoundEmailAuthIdentityWithClient(
	ctx context.Context,
	client *dbent.Client,
	userID int64,
	subject string,
	source string,
) error {
	if client == nil || userID <= 0 || subject == "" {
		return nil
REDACTED

	if strings.TrimSpace(source) == "" {
		source = "auth_service_email_bind"
REDACTED

	if err := client.AuthIdentity.Create().
		SetUserID(userID).
		SetProviderType("email").
		SetProviderKey("email").
		SetProviderSubject(subject).
		SetVerifiedAt(time.Now().UTC()).
		SetMetadata(map[string]any{"source": strings.TrimSpace(source)REDACTED).
		OnConflictColumns(
			authidentity.FieldProviderType,
			authidentity.FieldProviderKey,
			authidentity.FieldProviderSubject,
		).
		DoNothing().
		Exec(ctx); err != nil {
		if !isSQLNoRowsError(err) {
			return err
	REDACTED
REDACTED

	identity, err := client.AuthIdentity.Query().
		Where(
			authidentity.ProviderTypeEQ("email"),
			authidentity.ProviderKeyEQ("email"),
			authidentity.ProviderSubjectEQ(subject),
		).
		Only(ctx)
	if err != nil {
		if dbent.IsNotFound(err) {
			return nil
	REDACTED
		return err
REDACTED
	if identity.UserID != userID {
		return ErrEmailExists
REDACTED
	return nil
REDACTED

func normalizeBoundEmailAuthIdentitySubject(email string) string {
	normalized := strings.ToLower(strings.TrimSpace(email))
	if normalized == "" || isReservedEmail(normalized) {
		return ""
REDACTED
	return normalized
REDACTED
