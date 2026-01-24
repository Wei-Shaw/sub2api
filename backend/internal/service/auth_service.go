package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"net/mail"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

var (
	ErrInvalidCredentials  = infraerrors.Unauthorized("INVALID_CREDENTIALS", "invalid email or password")
	ErrUserNotActive       = infraerrors.Forbidden("USER_NOT_ACTIVE", "user is not active")
	ErrEmailExists         = infraerrors.Conflict("EMAIL_EXISTS", "email already exists")
	ErrEmailReserved       = infraerrors.BadRequest("EMAIL_RESERVED", "email is reserved")
	ErrInvalidToken        = infraerrors.Unauthorized("INVALID_TOKEN", "invalid token")
	ErrTokenExpired        = infraerrors.Unauthorized("TOKEN_EXPIRED", "token has expired")
	ErrTokenTooLarge       = infraerrors.BadRequest("TOKEN_TOO_LARGE", "token too large")
	ErrTokenRevoked        = infraerrors.Unauthorized("TOKEN_REVOKED", "token has been revoked")
	ErrEmailVerifyRequired = infraerrors.BadRequest("EMAIL_VERIFY_REQUIRED", "email verification is required")
	ErrRegDisabled         = infraerrors.Forbidden("REGISTRATION_DISABLED", "registration is currently disabled")
	ErrServiceUnavailable  = infraerrors.ServiceUnavailable("SERVICE_UNAVAILABLE", "service temporarily unavailable")
)

// maxTokenLength 限制 token 大小，避免超长 header 触发解析时的异常内存分配。
const maxTokenLength = 8192

// JWTClaims JWT载荷数据
type JWTClaims struct {
	UserID       int64  `json:"user_id"`
	Email        string `json:"email"`
	Role         string `json:"role"`
	TokenVersion int64  `json:"token_version"` // Used to invalidate tokens on password change
	jwt.RegisteredClaims
REDACTED

// AuthService 认证服务
type AuthService struct {
	userRepo          UserRepository
	cfg               *config.Config
	settingService    *SettingService
	emailService      *EmailService
	turnstileService  *TurnstileService
	emailQueueService *EmailQueueService
	promoService      *PromoService
REDACTED

// NewAuthService 创建认证服务实例
func NewAuthService(
	userRepo UserRepository,
	cfg *config.Config,
	settingService *SettingService,
	emailService *EmailService,
	turnstileService *TurnstileService,
	emailQueueService *EmailQueueService,
	promoService *PromoService,
) *AuthService {
	return &AuthService{
		userRepo:          userRepo,
		cfg:               cfg,
		settingService:    settingService,
		emailService:      emailService,
		turnstileService:  turnstileService,
		emailQueueService: emailQueueService,
		promoService:      promoService,
REDACTED
REDACTED

// Register 用户注册，返回token和用户
func (s *AuthService) Register(ctx context.Context, email, password string) (string, *User, error) {
	return s.RegisterWithVerification(ctx, email, password, "", "")
REDACTED

// RegisterWithVerification 用户注册（支持邮件验证和优惠码），返回token和用户
func (s *AuthService) RegisterWithVerification(ctx context.Context, email, password, verifyCode, promoCode string) (string, *User, error) {
	// 检查是否开放注册（默认关闭：settingService 未配置时不允许注册）
	if s.settingService == nil || !s.settingService.IsRegistrationEnabled(ctx) {
		return "", nil, ErrRegDisabled
REDACTED

	// 防止用户注册 LinuxDo OAuth 合成邮箱，避免第三方登录与本地账号发生碰撞。
	if isReservedEmail(email) {
		return "", nil, ErrEmailReserved
REDACTED

	// 检查是否需要邮件验证
	if s.settingService != nil && s.settingService.IsEmailVerifyEnabled(ctx) {
		// 如果邮件验证已开启但邮件服务未配置，拒绝注册
		// 这是一个配置错误，不应该允许绕过验证
		if s.emailService == nil {
			log.Println("[Auth] Email verification enabled but email service not configured, rejecting registration")
			return "", nil, ErrServiceUnavailable
	REDACTED
		if verifyCode == "" {
			return "", nil, ErrEmailVerifyRequired
	REDACTED
		// 验证邮箱验证码
		if err := s.emailService.VerifyCode(ctx, email, verifyCode); err != nil {
			return "", nil, fmt.Errorf("verify code: %w", err)
	REDACTED
REDACTED

	// 检查邮箱是否已存在
	existsEmail, err := s.userRepo.ExistsByEmail(ctx, email)
	if err != nil {
		log.Printf("[Auth] Database error checking email exists: %v", err)
		return "", nil, ErrServiceUnavailable
REDACTED
	if existsEmail {
		return "", nil, ErrEmailExists
REDACTED

	// 密码哈希
	hashedPassword, err := s.HashPassword(password)
	if err != nil {
		return "", nil, fmt.Errorf("hash password: %w", err)
REDACTED

	// 获取默认配置
	defaultBalance := s.cfg.Default.UserBalance
	defaultConcurrency := s.cfg.Default.UserConcurrency
	if s.settingService != nil {
		defaultBalance = s.settingService.GetDefaultBalance(ctx)
		defaultConcurrency = s.settingService.GetDefaultConcurrency(ctx)
REDACTED

	// 创建用户
	user := &User{
		Email:        email,
		PasswordHash: hashedPassword,
		Role:         RoleUser,
		Balance:      defaultBalance,
		Concurrency:  defaultConcurrency,
		Status:       StatusActive,
REDACTED

	if err := s.userRepo.Create(ctx, user); err != nil {
		// 优先检查邮箱冲突错误（竞态条件下可能发生）
		if errors.Is(err, ErrEmailExists) {
			return "", nil, ErrEmailExists
	REDACTED
		log.Printf("[Auth] Database error creating user: %v", err)
		return "", nil, ErrServiceUnavailable
REDACTED

	// 应用优惠码（如果提供且功能已启用）
	if promoCode != "" && s.promoService != nil && s.settingService != nil && s.settingService.IsPromoCodeEnabled(ctx) {
		if err := s.promoService.ApplyPromoCode(ctx, user.ID, promoCode); err != nil {
			// 优惠码应用失败不影响注册，只记录日志
			log.Printf("[Auth] Failed to apply promo code for user %d: %v", user.ID, err)
	REDACTED else {
			// 重新获取用户信息以获取更新后的余额
			if updatedUser, err := s.userRepo.GetByID(ctx, user.ID); err == nil {
				user = updatedUser
		REDACTED
	REDACTED
REDACTED

	// 生成token
	token, err := s.GenerateToken(user)
	if err != nil {
		return "", nil, fmt.Errorf("generate token: %w", err)
REDACTED

	return token, user, nil
REDACTED

// SendVerifyCodeResult 发送验证码返回结果
type SendVerifyCodeResult struct {
	Countdown int `json:"countdown"` // 倒计时秒数
REDACTED

// SendVerifyCode 发送邮箱验证码（同步方式）
func (s *AuthService) SendVerifyCode(ctx context.Context, email string) error {
	// 检查是否开放注册（默认关闭）
	if s.settingService == nil || !s.settingService.IsRegistrationEnabled(ctx) {
		return ErrRegDisabled
REDACTED

	if isReservedEmail(email) {
		return ErrEmailReserved
REDACTED

	// 检查邮箱是否已存在
	existsEmail, err := s.userRepo.ExistsByEmail(ctx, email)
	if err != nil {
		log.Printf("[Auth] Database error checking email exists: %v", err)
		return ErrServiceUnavailable
REDACTED
	if existsEmail {
		return ErrEmailExists
REDACTED

	// 发送验证码
	if s.emailService == nil {
		return errors.New("email service not configured")
REDACTED

	// 获取网站名称
	siteName := "Sub2API"
	if s.settingService != nil {
		siteName = s.settingService.GetSiteName(ctx)
REDACTED

	return s.emailService.SendVerifyCode(ctx, email, siteName)
REDACTED

// SendVerifyCodeAsync 异步发送邮箱验证码并返回倒计时
func (s *AuthService) SendVerifyCodeAsync(ctx context.Context, email string) (*SendVerifyCodeResult, error) {
	log.Printf("[Auth] SendVerifyCodeAsync called for email: %s", email)

	// 检查是否开放注册（默认关闭）
	if s.settingService == nil || !s.settingService.IsRegistrationEnabled(ctx) {
		log.Println("[Auth] Registration is disabled")
		return nil, ErrRegDisabled
REDACTED

	if isReservedEmail(email) {
		return nil, ErrEmailReserved
REDACTED

	// 检查邮箱是否已存在
	existsEmail, err := s.userRepo.ExistsByEmail(ctx, email)
	if err != nil {
		log.Printf("[Auth] Database error checking email exists: %v", err)
		return nil, ErrServiceUnavailable
REDACTED
	if existsEmail {
		log.Printf("[Auth] Email already exists: %s", email)
		return nil, ErrEmailExists
REDACTED

	// 检查邮件队列服务是否配置
	if s.emailQueueService == nil {
		log.Println("[Auth] Email queue service not configured")
		return nil, errors.New("email queue service not configured")
REDACTED

	// 获取网站名称
	siteName := "Sub2API"
	if s.settingService != nil {
		siteName = s.settingService.GetSiteName(ctx)
REDACTED

	// 异步发送
	log.Printf("[Auth] Enqueueing verify code for: %s", email)
	if err := s.emailQueueService.EnqueueVerifyCode(email, siteName); err != nil {
		log.Printf("[Auth] Failed to enqueue: %v", err)
		return nil, fmt.Errorf("enqueue verify code: %w", err)
REDACTED

	log.Printf("[Auth] Verify code enqueued successfully for: %s", email)
	return &SendVerifyCodeResult{
		Countdown: 60, // 60秒倒计时
REDACTED, nil
REDACTED

// VerifyTurnstile 验证Turnstile token
func (s *AuthService) VerifyTurnstile(ctx context.Context, token string, remoteIP string) error {
	required := s.cfg != nil && s.cfg.Server.Mode == "release" && s.cfg.Turnstile.Required

	if required {
		if s.settingService == nil {
			log.Println("[Auth] Turnstile required but settings service is not configured")
			return ErrTurnstileNotConfigured
	REDACTED
		enabled := s.settingService.IsTurnstileEnabled(ctx)
		secretConfigured := s.settingService.GetTurnstileSecretKey(ctx) != ""
		if !enabled || !secretConfigured {
			log.Printf("[Auth] Turnstile required but not configured (enabled=%v, secret_configured=%v)", enabled, secretConfigured)
			return ErrTurnstileNotConfigured
	REDACTED
REDACTED

	if s.turnstileService == nil {
		if required {
			log.Println("[Auth] Turnstile required but service not configured")
			return ErrTurnstileNotConfigured
	REDACTED
		return nil // 服务未配置则跳过验证
REDACTED

	if !required && s.settingService != nil && s.settingService.IsTurnstileEnabled(ctx) && s.settingService.GetTurnstileSecretKey(ctx) == "" {
		log.Println("[Auth] Turnstile enabled but secret key not configured")
REDACTED

	return s.turnstileService.VerifyToken(ctx, token, remoteIP)
REDACTED

// IsTurnstileEnabled 检查是否启用Turnstile验证
func (s *AuthService) IsTurnstileEnabled(ctx context.Context) bool {
	if s.turnstileService == nil {
		return false
REDACTED
	return s.turnstileService.IsEnabled(ctx)
REDACTED

// IsRegistrationEnabled 检查是否开放注册
func (s *AuthService) IsRegistrationEnabled(ctx context.Context) bool {
	if s.settingService == nil {
		return false // 安全默认：settingService 未配置时关闭注册
REDACTED
	return s.settingService.IsRegistrationEnabled(ctx)
REDACTED

// IsEmailVerifyEnabled 检查是否开启邮件验证
func (s *AuthService) IsEmailVerifyEnabled(ctx context.Context) bool {
	if s.settingService == nil {
		return false
REDACTED
	return s.settingService.IsEmailVerifyEnabled(ctx)
REDACTED

// Login 用户登录，返回JWT token
func (s *AuthService) Login(ctx context.Context, email, password string) (string, *User, error) {
	// 查找用户
	user, err := s.userRepo.GetByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, ErrUserNotFound) {
			return "", nil, ErrInvalidCredentials
	REDACTED
		// 记录数据库错误但不暴露给用户
		log.Printf("[Auth] Database error during login: %v", err)
		return "", nil, ErrServiceUnavailable
REDACTED

	// 验证密码
	if !s.CheckPassword(password, user.PasswordHash) {
		return "", nil, ErrInvalidCredentials
REDACTED

	// 检查用户状态
	if !user.IsActive() {
		return "", nil, ErrUserNotActive
REDACTED

	// 生成JWT token
	token, err := s.GenerateToken(user)
	if err != nil {
		return "", nil, fmt.Errorf("generate token: %w", err)
REDACTED

	return token, user, nil
REDACTED

// LoginOrRegisterOAuth 用于第三方 OAuth/SSO 登录：
// - 如果邮箱已存在：直接登录（不需要本地密码）
// - 如果邮箱不存在：创建新用户并登录
//
// 注意：该函数用于 LinuxDo OAuth 登录场景（不同于上游账号的 OAuth，例如 Claude/OpenAI/Gemini）。
// 为了满足现有数据库约束（需要密码哈希），新用户会生成随机密码并进行哈希保存。
func (s *AuthService) LoginOrRegisterOAuth(ctx context.Context, email, username string) (string, *User, error) {
	email = strings.TrimSpace(email)
	if email == "" || len(email) > 255 {
		return "", nil, infraerrors.BadRequest("INVALID_EMAIL", "invalid email")
REDACTED
	if _, err := mail.ParseAddress(email); err != nil {
		return "", nil, infraerrors.BadRequest("INVALID_EMAIL", "invalid email")
REDACTED

	username = strings.TrimSpace(username)
	if len([]rune(username)) > 100 {
		username = string([]rune(username)[:100])
REDACTED

	user, err := s.userRepo.GetByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, ErrUserNotFound) {
			// OAuth 首次登录视为注册（fail-close：settingService 未配置时不允许注册）
			if s.settingService == nil || !s.settingService.IsRegistrationEnabled(ctx) {
				return "", nil, ErrRegDisabled
		REDACTED

			randomPassword, err := randomHexString(32)
			if err != nil {
				log.Printf("[Auth] Failed to generate random password for oauth signup: %v", err)
				return "", nil, ErrServiceUnavailable
		REDACTED
			hashedPassword, err := s.HashPassword(randomPassword)
			if err != nil {
				return "", nil, fmt.Errorf("hash password: %w", err)
		REDACTED

			// 新用户默认值。
			defaultBalance := s.cfg.Default.UserBalance
			defaultConcurrency := s.cfg.Default.UserConcurrency
			if s.settingService != nil {
				defaultBalance = s.settingService.GetDefaultBalance(ctx)
				defaultConcurrency = s.settingService.GetDefaultConcurrency(ctx)
		REDACTED

			newUser := &User{
				Email:        email,
				Username:     username,
				PasswordHash: hashedPassword,
				Role:         RoleUser,
				Balance:      defaultBalance,
				Concurrency:  defaultConcurrency,
				Status:       StatusActive,
		REDACTED

			if err := s.userRepo.Create(ctx, newUser); err != nil {
				if errors.Is(err, ErrEmailExists) {
					// 并发场景：GetByEmail 与 Create 之间用户被创建。
					user, err = s.userRepo.GetByEmail(ctx, email)
					if err != nil {
						log.Printf("[Auth] Database error getting user after conflict: %v", err)
						return "", nil, ErrServiceUnavailable
				REDACTED
			REDACTED else {
					log.Printf("[Auth] Database error creating oauth user: %v", err)
					return "", nil, ErrServiceUnavailable
			REDACTED
		REDACTED else {
				user = newUser
		REDACTED
	REDACTED else {
			log.Printf("[Auth] Database error during oauth login: %v", err)
			return "", nil, ErrServiceUnavailable
	REDACTED
REDACTED

	if !user.IsActive() {
		return "", nil, ErrUserNotActive
REDACTED

	// 尽力补全：当用户名为空时，使用第三方返回的用户名回填。
	if user.Username == "" && username != "" {
		user.Username = username
		if err := s.userRepo.Update(ctx, user); err != nil {
			log.Printf("[Auth] Failed to update username after oauth login: %v", err)
	REDACTED
REDACTED

	token, err := s.GenerateToken(user)
	if err != nil {
		return "", nil, fmt.Errorf("generate token: %w", err)
REDACTED
	return token, user, nil
REDACTED

// ValidateToken 验证JWT token并返回用户声明
func (s *AuthService) ValidateToken(tokenString string) (*JWTClaims, error) {
	// 先做长度校验，尽早拒绝异常超长 token，降低 DoS 风险。
	if len(tokenString) > maxTokenLength {
		return nil, ErrTokenTooLarge
REDACTED

	// 使用解析器并限制可接受的签名算法，防止算法混淆。
	parser := jwt.NewParser(jwt.WithValidMethods([]string{
		jwt.SigningMethodHS256.Name,
		jwt.SigningMethodHS384.Name,
		jwt.SigningMethodHS512.Name,
REDACTED))

	// 保留默认 claims 校验（exp/nbf），避免放行过期或未生效的 token。
	token, err := parser.ParseWithClaims(tokenString, &JWTClaims{REDACTED, func(token *jwt.Token) (any, error) {
		// 验证签名方法
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
	REDACTED
		return []byte(s.cfg.JWT.Secret), nil
REDACTED)

	if err != nil {
		if errors.Is(err, jwt.ErrTokenExpired) {
			// token 过期但仍返回 claims（用于 RefreshToken 等场景）
			// jwt-go 在解析时即使遇到过期错误，token.Claims 仍会被填充
			if claims, ok := token.Claims.(*JWTClaims); ok {
				return claims, ErrTokenExpired
		REDACTED
			return nil, ErrTokenExpired
	REDACTED
		return nil, ErrInvalidToken
REDACTED

	if claims, ok := token.Claims.(*JWTClaims); ok && token.Valid {
		return claims, nil
REDACTED

	return nil, ErrInvalidToken
REDACTED

func randomHexString(byteLength int) (string, error) {
	if byteLength <= 0 {
		byteLength = 16
REDACTED
	buf := make([]byte, byteLength)
	if _, err := rand.Read(buf); err != nil {
		return "", err
REDACTED
	return hex.EncodeToString(buf), nil
REDACTED

func isReservedEmail(email string) bool {
	normalized := strings.ToLower(strings.TrimSpace(email))
	return strings.HasSuffix(normalized, LinuxDoConnectSyntheticEmailDomain)
REDACTED

// GenerateToken 生成JWT token
func (s *AuthService) GenerateToken(user *User) (string, error) {
	now := time.Now()
	expiresAt := now.Add(time.Duration(s.cfg.JWT.ExpireHour) * time.Hour)

	claims := &JWTClaims{
		UserID:       user.ID,
		Email:        user.Email,
		Role:         user.Role,
		TokenVersion: user.TokenVersion,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(expiresAt),
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
	REDACTED,
REDACTED

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString([]byte(s.cfg.JWT.Secret))
	if err != nil {
		return "", fmt.Errorf("sign token: %w", err)
REDACTED

	return tokenString, nil
REDACTED

// HashPassword 使用bcrypt加密密码
func (s *AuthService) HashPassword(password string) (string, error) {
	hashedBytes, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
REDACTED
	return string(hashedBytes), nil
REDACTED

// CheckPassword 验证密码是否匹配
func (s *AuthService) CheckPassword(password, hashedPassword string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(hashedPassword), []byte(password))
	return err == nil
REDACTED

// RefreshToken 刷新token
func (s *AuthService) RefreshToken(ctx context.Context, oldTokenString string) (string, error) {
	// 验证旧token（即使过期也允许，用于刷新）
	claims, err := s.ValidateToken(oldTokenString)
	if err != nil && !errors.Is(err, ErrTokenExpired) {
		return "", err
REDACTED

	// 获取最新的用户信息
	user, err := s.userRepo.GetByID(ctx, claims.UserID)
	if err != nil {
		if errors.Is(err, ErrUserNotFound) {
			return "", ErrInvalidToken
	REDACTED
		log.Printf("[Auth] Database error refreshing token: %v", err)
		return "", ErrServiceUnavailable
REDACTED

	// 检查用户状态
	if !user.IsActive() {
		return "", ErrUserNotActive
REDACTED

	// Security: Check TokenVersion to prevent refreshing revoked tokens
	// This ensures tokens issued before a password change cannot be refreshed
	if claims.TokenVersion != user.TokenVersion {
		return "", ErrTokenRevoked
REDACTED

	// 生成新token
	return s.GenerateToken(user)
REDACTED

// IsPasswordResetEnabled 检查是否启用密码重置功能
// 要求：必须同时开启邮件验证且 SMTP 配置正确
func (s *AuthService) IsPasswordResetEnabled(ctx context.Context) bool {
	if s.settingService == nil {
		return false
REDACTED
	// Must have email verification enabled and SMTP configured
	if !s.settingService.IsEmailVerifyEnabled(ctx) {
		return false
REDACTED
	return s.settingService.IsPasswordResetEnabled(ctx)
REDACTED

// preparePasswordReset validates the password reset request and returns necessary data
// Returns (siteName, resetURL, shouldProceed)
// shouldProceed is false when we should silently return success (to prevent enumeration)
func (s *AuthService) preparePasswordReset(ctx context.Context, email, frontendBaseURL string) (string, string, bool) {
	// Check if user exists (but don't reveal this to the caller)
	user, err := s.userRepo.GetByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, ErrUserNotFound) {
			// Security: Log but don't reveal that user doesn't exist
			log.Printf("[Auth] Password reset requested for non-existent email: %s", email)
			return "", "", false
	REDACTED
		log.Printf("[Auth] Database error checking email for password reset: %v", err)
		return "", "", false
REDACTED

	// Check if user is active
	if !user.IsActive() {
		log.Printf("[Auth] Password reset requested for inactive user: %s", email)
		return "", "", false
REDACTED

	// Get site name
	siteName := "Sub2API"
	if s.settingService != nil {
		siteName = s.settingService.GetSiteName(ctx)
REDACTED

	// Build reset URL base
	resetURL := fmt.Sprintf("%s/reset-password", strings.TrimSuffix(frontendBaseURL, "/"))

	return siteName, resetURL, true
REDACTED

// RequestPasswordReset 请求密码重置（同步发送）
// Security: Returns the same response regardless of whether the email exists (prevent user enumeration)
func (s *AuthService) RequestPasswordReset(ctx context.Context, email, frontendBaseURL string) error {
	if !s.IsPasswordResetEnabled(ctx) {
		return infraerrors.Forbidden("PASSWORD_RESET_DISABLED", "password reset is not enabled")
REDACTED
	if s.emailService == nil {
		return ErrServiceUnavailable
REDACTED

	siteName, resetURL, shouldProceed := s.preparePasswordReset(ctx, email, frontendBaseURL)
	if !shouldProceed {
		return nil // Silent success to prevent enumeration
REDACTED

	if err := s.emailService.SendPasswordResetEmail(ctx, email, siteName, resetURL); err != nil {
		log.Printf("[Auth] Failed to send password reset email to %s: %v", email, err)
		return nil // Silent success to prevent enumeration
REDACTED

	log.Printf("[Auth] Password reset email sent to: %s", email)
	return nil
REDACTED

// RequestPasswordResetAsync 异步请求密码重置（队列发送）
// Security: Returns the same response regardless of whether the email exists (prevent user enumeration)
func (s *AuthService) RequestPasswordResetAsync(ctx context.Context, email, frontendBaseURL string) error {
	if !s.IsPasswordResetEnabled(ctx) {
		return infraerrors.Forbidden("PASSWORD_RESET_DISABLED", "password reset is not enabled")
REDACTED
	if s.emailQueueService == nil {
		return ErrServiceUnavailable
REDACTED

	siteName, resetURL, shouldProceed := s.preparePasswordReset(ctx, email, frontendBaseURL)
	if !shouldProceed {
		return nil // Silent success to prevent enumeration
REDACTED

	if err := s.emailQueueService.EnqueuePasswordReset(email, siteName, resetURL); err != nil {
		log.Printf("[Auth] Failed to enqueue password reset email for %s: %v", email, err)
		return nil // Silent success to prevent enumeration
REDACTED

	log.Printf("[Auth] Password reset email enqueued for: %s", email)
	return nil
REDACTED

// ResetPassword 重置密码
// Security: Increments TokenVersion to invalidate all existing JWT tokens
func (s *AuthService) ResetPassword(ctx context.Context, email, token, newPassword string) error {
	// Check if password reset is enabled
	if !s.IsPasswordResetEnabled(ctx) {
		return infraerrors.Forbidden("PASSWORD_RESET_DISABLED", "password reset is not enabled")
REDACTED

	if s.emailService == nil {
		return ErrServiceUnavailable
REDACTED

	// Verify and consume the reset token (one-time use)
	if err := s.emailService.ConsumePasswordResetToken(ctx, email, token); err != nil {
		return err
REDACTED

	// Get user
	user, err := s.userRepo.GetByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, ErrUserNotFound) {
			return ErrInvalidResetToken // Token was valid but user was deleted
	REDACTED
		log.Printf("[Auth] Database error getting user for password reset: %v", err)
		return ErrServiceUnavailable
REDACTED

	// Check if user is active
	if !user.IsActive() {
		return ErrUserNotActive
REDACTED

	// Hash new password
	hashedPassword, err := s.HashPassword(newPassword)
	if err != nil {
		return fmt.Errorf("hash password: %w", err)
REDACTED

	// Update password and increment TokenVersion
	user.PasswordHash = hashedPassword
	user.TokenVersion++ // Invalidate all existing tokens

	if err := s.userRepo.Update(ctx, user); err != nil {
		log.Printf("[Auth] Database error updating password for user %d: %v", user.ID, err)
		return ErrServiceUnavailable
REDACTED

	log.Printf("[Auth] Password reset successful for user: %s", email)
	return nil
REDACTED
