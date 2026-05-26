package service

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"log/slog"
	"time"

	"github.com/pquerna/otp/totp"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

var (
	ErrTotpNotEnabled      = infraerrors.BadRequest("TOTP_NOT_ENABLED", "totp feature is not enabled")
	ErrTotpAlreadyEnabled  = infraerrors.BadRequest("TOTP_ALREADY_ENABLED", "totp is already enabled for this account")
	ErrTotpNotSetup        = infraerrors.BadRequest("TOTP_NOT_SETUP", "totp is not set up for this account")
	ErrTotpInvalidCode     = infraerrors.BadRequest("TOTP_INVALID_CODE", "invalid totp code")
	ErrTotpSetupExpired    = infraerrors.BadRequest("TOTP_SETUP_EXPIRED", "totp setup session expired")
	ErrTotpTooManyAttempts = infraerrors.TooManyRequests("TOTP_TOO_MANY_ATTEMPTS", "too many verification attempts, please try again later")
	ErrVerifyCodeRequired  = infraerrors.BadRequest("VERIFY_CODE_REQUIRED", "email verification code is required")
	ErrPasswordRequired    = infraerrors.BadRequest("PASSWORD_REQUIRED", "password is required")
)

// TotpCache defines cache operations for TOTP service
type TotpCache interface {
	// Setup session methods
	GetSetupSession(ctx context.Context, userID int64) (*TotpSetupSession, error)
	SetSetupSession(ctx context.Context, userID int64, session *TotpSetupSession, ttl time.Duration) error
	DeleteSetupSession(ctx context.Context, userID int64) error

	// Login session methods (for 2FA login flow)
	GetLoginSession(ctx context.Context, tempToken string) (*TotpLoginSession, error)
	SetLoginSession(ctx context.Context, tempToken string, session *TotpLoginSession, ttl time.Duration) error
	DeleteLoginSession(ctx context.Context, tempToken string) error

	// Rate limiting
	IncrementVerifyAttempts(ctx context.Context, userID int64) (int, error)
	GetVerifyAttempts(ctx context.Context, userID int64) (int, error)
	ClearVerifyAttempts(ctx context.Context, userID int64) error
}

// SecretEncryptor defines encryption operations for TOTP secrets
type SecretEncryptor interface {
	Encrypt(plaintext string) (string, error)
	Decrypt(ciphertext string) (string, error)
}

// TotpSetupSession represents a TOTP setup session
type TotpSetupSession struct {
	Secret     string // Plain text TOTP secret (not encrypted yet)
	SetupToken string // Random token to verify setup request
	CreatedAt  time.Time
}

// TotpLoginSession represents a pending 2FA login session
type TotpLoginSession struct {
	UserID           int64
	Email            string
	TokenExpiry      time.Time
	PendingOAuthBind *PendingOAuthBindLoginSession `json:"pending_oauth_bind,omitempty"`
}

type PendingOAuthBindLoginSession struct {
	PendingSessionToken string `json:"pending_session_token,omitempty"`
	BrowserSessionKey   string `json:"browser_session_key,omitempty"`
}

// TotpStatus represents the TOTP status for a user
type TotpStatus struct {
	Enabled        bool       `json:"enabled"`
	EnabledAt      *time.Time `json:"enabled_at,omitempty"`
	FeatureEnabled bool       `json:"feature_enabled"`
}

// TotpSetupResponse represents the response for initiating TOTP setup
type TotpSetupResponse struct {
	Secret     string `json:"secret"`
	QRCodeURL  string `json:"qr_code_url"`
	SetupToken string `json:"setup_token"`
	Countdown  int    `json:"countdown"` // seconds until setup expires
}

const (
	totpSetupTTL    = 5 * time.Minute
	totpLoginTTL    = 5 * time.Minute
	totpAttemptsTTL = 15 * time.Minute
	maxTotpAttempts = 5
	totpIssuer      = "Sub2API"
)

// TotpService handles TOTP operations
type TotpService struct {
	userRepo          UserRepository
	encryptor         SecretEncryptor
	cache             TotpCache
	settingService    *SettingService
	emailService      *EmailService
	emailQueueService *EmailQueueService
}

// NewTotpService creates a new TOTP service
func NewTotpService(
	userRepo UserRepository,
	encryptor SecretEncryptor,
	cache TotpCache,
	settingService *SettingService,
	emailService *EmailService,
	emailQueueService *EmailQueueService,
) *TotpService {
	return &TotpService{
		userRepo:          userRepo,
		encryptor:         encryptor,
		cache:             cache,
		settingService:    settingService,
		emailService:      emailService,
		emailQueueService: emailQueueService,
	}
}

// GetStatus returns the TOTP status for a user
func (s *TotpService) GetStatus(ctx context.Context, userID int64) (*TotpStatus, error) {
	featureEnabled := s.settingService.IsTotpEnabled(ctx)

	user, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("get user: %w", err)
	}

	return &TotpStatus{
		Enabled:        user.TotpEnabled,
		EnabledAt:      user.TotpEnabledAt,
		FeatureEnabled: featureEnabled,
	}, nil
}

// InitiateSetup starts the TOTP setup process
// If email verification is enabled, emailCode is required; otherwise password is required
func (s *TotpService) InitiateSetup(ctx context.Context, userID int64, emailCode, password string) (*TotpSetupResponse, error) {
	// Check if TOTP feature is enabled globally
	if !s.settingService.IsTotpEnabled(ctx) {
		return nil, ErrTotpNotEnabled
	}

	// Get user and check if TOTP is already enabled
	user, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("get user: %w", err)
	}

	if user.TotpEnabled {
		return nil, ErrTotpAlreadyEnabled
	}

	// Verify identity based on email verification setting
	if s.settingService.IsEmailVerifyEnabled(ctx) {
		// Email verification enabled - verify email code
		if emailCode == "" {
			return nil, ErrVerifyCodeRequired
		}
		if err := s.emailService.VerifyCode(ctx, user.Email, emailCode); err != nil {
			return nil, err
		}
	} else {
		// Email verification disabled - verify password
		if password == "" {
			return nil, ErrPasswordRequired
		}
		if !user.CheckPassword(password) {
			return nil, ErrPasswordIncorrect
		}
	}

	// Generate a new TOTP key
	key, err := totp.Generate(totp.GenerateOpts{
		Issuer:      totpIssuer,
		AccountName: user.Email,
	})
	if err != nil {
		return nil, fmt.Errorf("generate totp key: %w", err)
	}

	// Generate a random setup token
	setupToken, err := generateRandomToken(32)
	if err != nil {
		return nil, fmt.Errorf("generate setup token: %w", err)
	}

	// Store the setup session in cache
	session := &TotpSetupSession{
		Secret:     key.Secret(),
		SetupToken: setupToken,
		CreatedAt:  time.Now(),
	}

	if err := s.cache.SetSetupSession(ctx, userID, session, totpSetupTTL); err != nil {
		return nil, fmt.Errorf("store setup session: %w", err)
	}

	return &TotpSetupResponse{
		Secret:     key.Secret(),
		QRCodeURL:  key.URL(),
		SetupToken: setupToken,
		Countdown:  int(totpSetupTTL.Seconds()),
	}, nil
}

// CompleteSetup completes the TOTP setup by verifying the code
func (s *TotpService) CompleteSetup(ctx context.Context, userID int64, totpCode, setupToken string) error {
	// Check if TOTP feature is enabled globally
	if !s.settingService.IsTotpEnabled(ctx) {
		return ErrTotpNotEnabled
	}

	// Get the setup session
	session, err := s.cache.GetSetupSession(ctx, userID)
	if err != nil {
		return ErrTotpSetupExpired
	}

	if session == nil {
		return ErrTotpSetupExpired
	}

	// Verify the setup token (constant-time comparison)
	if subtle.ConstantTimeCompare([]byte(session.SetupToken), []byte(setupToken)) != 1 {
		return ErrTotpSetupExpired
	}

	// Verify the TOTP code
	if !totp.Validate(totpCode, session.Secret) {
		return ErrTotpInvalidCode
	}

	setupSecretPrefix := "N/A"
	if len(session.Secret) >= 4 {
		setupSecretPrefix = session.Secret[:4]
	}
	slog.Debug("totp_complete_setup_before_encrypt",
		"user_id", userID,
		"secret_len", len(session.Secret),
		"secret_prefix", setupSecretPrefix)

	// Encrypt the secret
	encryptedSecret, err := s.encryptor.Encrypt(session.Secret)
	if err != nil {
		return fmt.Errorf("encrypt totp secret: %w", err)
	}

	slog.Debug("totp_complete_setup_encrypted",
		"user_id", userID,
		"encrypted_len", len(encryptedSecret))

	// Verify encryption by decrypting
	decrypted, decErr := s.encryptor.Decrypt(encryptedSecret)
	if decErr != nil {
		slog.Debug("totp_complete_setup_verify_failed",
			"user_id", userID,
			"error", decErr)
	} else {
		decryptedPrefix := "N/A"
		if len(decrypted) >= 4 {
			decryptedPrefix = decrypted[:4]
		}
		slog.Debug("totp_complete_setup_verified",
			"user_id", userID,
			"original_len", len(session.Secret),
			"decrypted_len", len(decrypted),
			"match", session.Secret == decrypted,
			"decrypted_prefix", decryptedPrefix)
	}

	// Update user with encrypted TOTP secret
	if err := s.userRepo.UpdateTotpSecret(ctx, userID, &encryptedSecret); err != nil {
		return fmt.Errorf("update totp secret: %w", err)
	}

	// Enable TOTP for the user
	if err := s.userRepo.EnableTotp(ctx, userID); err != nil {
		return fmt.Errorf("enable totp: %w", err)
	}

	// Clean up the setup session
	_ = s.cache.DeleteSetupSession(ctx, userID)

	return nil
}

// Disable disables TOTP for a user
// If email verification is enabled, emailCode is required; otherwise password is required
func (s *TotpService) Disable(ctx context.Context, userID int64, emailCode, password string) error {
	// Get user
	user, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		return fmt.Errorf("get user: %w", err)
	}

	if !user.TotpEnabled {
		return ErrTotpNotSetup
	}

	// Verify identity based on email verification setting
	if s.settingService.IsEmailVerifyEnabled(ctx) {
		// Email verification enabled - verify email code
		if emailCode == "" {
			return ErrVerifyCodeRequired
		}
		if err := s.emailService.VerifyCode(ctx, user.Email, emailCode); err != nil {
			return err
		}
	} else {
		// Email verification disabled - verify password
		if password == "" {
			return ErrPasswordRequired
		}
		if !user.CheckPassword(password) {
			return ErrPasswordIncorrect
		}
	}

	// Disable TOTP
	if err := s.userRepo.DisableTotp(ctx, userID); err != nil {
		return fmt.Errorf("disable totp: %w", err)
	}

	return nil
}

// VerifyCode verifies a TOTP code for a user
func (s *TotpService) VerifyCode(ctx context.Context, userID int64, code string) error {
	slog.Debug("totp_verify_code_called",
		"user_id", userID,
		"code_len", len(code))

	// Check rate limiting
	attempts, err := s.cache.GetVerifyAttempts(ctx, userID)
	if err == nil && attempts >= maxTotpAttempts {
		return ErrTotpTooManyAttempts
	}

	// Get user
	user, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		slog.Debug("totp_verify_get_user_failed",
			"user_id", userID,
			"error", err)
		return infraerrors.InternalServer("TOTP_VERIFY_ERROR", "failed to verify totp code")
	}

	if !user.TotpEnabled || user.TotpSecretEncrypted == nil {
		slog.Debug("totp_verify_not_setup",
			"user_id", userID,
			"enabled", user.TotpEnabled,
			"has_secret", user.TotpSecretEncrypted != nil)
		return ErrTotpNotSetup
	}

	slog.Debug("totp_verify_encrypted_secret",
		"user_id", userID,
		"encrypted_len", len(*user.TotpSecretEncrypted))

	// Decrypt the secret
	secret, err := s.encryptor.Decrypt(*user.TotpSecretEncrypted)
	if err != nil {
		slog.Debug("totp_verify_decrypt_failed",
			"user_id", userID,
			"error", err)
		return infraerrors.InternalServer("TOTP_VERIFY_ERROR", "failed to verify totp code")
	}

	secretPrefix := "N/A"
	if len(secret) >= 4 {
		secretPrefix = secret[:4]
	}
	slog.Debug("totp_verify_decrypted",
		"user_id", userID,
		"secret_len", len(secret),
		"secret_prefix", secretPrefix)

	// Verify the code
	valid := totp.Validate(code, secret)
	slog.Debug("totp_verify_result",
		"user_id", userID,
		"valid", valid,
		"secret_len", len(secret),
		"secret_prefix", secretPrefix,
		"server_time", time.Now().UTC().Format(time.RFC3339))

	if !valid {
		// Increment failed attempts
		_, _ = s.cache.IncrementVerifyAttempts(ctx, userID)
		return ErrTotpInvalidCode
	}

	// Clear attempt counter on success
	_ = s.cache.ClearVerifyAttempts(ctx, userID)

	return nil
}

// CreateLoginSession creates a temporary login session for 2FA
func (s *TotpService) CreateLoginSession(ctx context.Context, userID int64, email string) (string, error) {
	return s.createLoginSession(ctx, userID, email, nil)
}

// CreatePendingOAuthBindLoginSession creates a temporary 2FA session that will
// finalize a pending OAuth bind after the TOTP code is verified.
func (s *TotpService) CreatePendingOAuthBindLoginSession(
	ctx context.Context,
	userID int64,
	email string,
	pendingSessionToken string,
	browserSessionKey string,
) (string, error) {
	return s.createLoginSession(ctx, userID, email, &PendingOAuthBindLoginSession{
		PendingSessionToken: pendingSessionToken,
		BrowserSessionKey:   browserSessionKey,
	})
}

func (s *TotpService) createLoginSession(
	ctx context.Context,
	userID int64,
	email string,
	pendingOAuthBind *PendingOAuthBindLoginSession,
) (string, error) {
	// Generate a random temp token
	tempToken, err := generateRandomToken(32)
	if err != nil {
		return "", fmt.Errorf("generate temp token: %w", err)
	}

	session := &TotpLoginSession{
		UserID:           userID,
		Email:            email,
		TokenExpiry:      time.Now().Add(totpLoginTTL),
		PendingOAuthBind: pendingOAuthBind,
	}

	if err := s.cache.SetLoginSession(ctx, tempToken, session, totpLoginTTL); err != nil {
		return "", fmt.Errorf("store login session: %w", err)
	}

	return tempToken, nil
}

// GetLoginSession retrieves a login session
func (s *TotpService) GetLoginSession(ctx context.Context, tempToken string) (*TotpLoginSession, error) {
	return s.cache.GetLoginSession(ctx, tempToken)
}

// DeleteLoginSession deletes a login session
func (s *TotpService) DeleteLoginSession(ctx context.Context, tempToken string) error {
	return s.cache.DeleteLoginSession(ctx, tempToken)
}

// IsTotpEnabledForUser checks if TOTP is enabled for a specific user
func (s *TotpService) IsTotpEnabledForUser(ctx context.Context, userID int64) (bool, error) {
	user, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		return false, fmt.Errorf("get user: %w", err)
	}
	return user.TotpEnabled, nil
}

// MaskEmail masks an email address for display
func MaskEmail(email string) string {
	if len(email) < 3 {
		return "***"
	}

	atIdx := -1
	for i, c := range email {
		if c == '@' {
			atIdx = i
			break
		}
	}

	if atIdx == -1 || atIdx < 1 {
		return email[:1] + "***"
	}

	localPart := email[:atIdx]
	domain := email[atIdx:]

	if len(localPart) <= 2 {
		return localPart[:1] + "***" + domain
	}

	return localPart[:1] + "***" + localPart[len(localPart)-1:] + domain
}

// generateRandomToken generates a random hex-encoded token
func generateRandomToken(byteLength int) (string, error) {
	b := make([]byte, byteLength)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// VerificationMethod represents the method required for TOTP operations
type VerificationMethod struct {
	Method string `json:"method"` // "email" or "password"
}

// GetVerificationMethod returns the verification method for TOTP operations
func (s *TotpService) GetVerificationMethod(ctx context.Context) *VerificationMethod {
	if s.settingService.IsEmailVerifyEnabled(ctx) {
		return &VerificationMethod{Method: "email"}
	}
	return &VerificationMethod{Method: "password"}
}

// SendVerifyCode sends an email verification code for TOTP operations
func (s *TotpService) SendVerifyCode(ctx context.Context, userID int64) error {
	// Check if email verification is enabled
	if !s.settingService.IsEmailVerifyEnabled(ctx) {
		return infraerrors.BadRequest("EMAIL_VERIFY_NOT_ENABLED", "email verification is not enabled")
	}

	// Get user email
	user, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		return fmt.Errorf("get user: %w", err)
	}

	// Get site name for email
	siteName := s.settingService.GetSiteName(ctx)

	// Send verification code via queue
	return s.emailQueueService.EnqueueVerifyCode(user.Email, siteName)
}
