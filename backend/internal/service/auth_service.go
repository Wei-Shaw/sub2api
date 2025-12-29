package service

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/infrastructure/errors"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

var (
	ErrInvalidCredentials  = infraerrors.Unauthorized("INVALID_CREDENTIALS", "invalid email or password")
	ErrUserNotActive       = infraerrors.Forbidden("USER_NOT_ACTIVE", "user is not active")
	ErrEmailExists         = infraerrors.Conflict("EMAIL_EXISTS", "email already exists")
	ErrInvalidToken        = infraerrors.Unauthorized("INVALID_TOKEN", "invalid token")
	ErrTokenExpired        = infraerrors.Unauthorized("TOKEN_EXPIRED", "token has expired")
	ErrTokenRevoked        = infraerrors.Unauthorized("TOKEN_REVOKED", "token has been revoked")
	ErrEmailVerifyRequired = infraerrors.BadRequest("EMAIL_VERIFY_REQUIRED", "email verification is required")
	ErrRegDisabled         = infraerrors.Forbidden("REGISTRATION_DISABLED", "registration is currently disabled")
	ErrServiceUnavailable  = infraerrors.ServiceUnavailable("SERVICE_UNAVAILABLE", "service temporarily unavailable")
)

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
REDACTED

// NewAuthService 创建认证服务实例
func NewAuthService(
	userRepo UserRepository,
	cfg *config.Config,
	settingService *SettingService,
	emailService *EmailService,
	turnstileService *TurnstileService,
	emailQueueService *EmailQueueService,
) *AuthService {
	return &AuthService{
		userRepo:          userRepo,
		cfg:               cfg,
		settingService:    settingService,
		emailService:      emailService,
		turnstileService:  turnstileService,
		emailQueueService: emailQueueService,
REDACTED
REDACTED

// Register 用户注册，返回token和用户
func (s *AuthService) Register(ctx context.Context, email, password string) (string, *User, error) {
	return s.RegisterWithVerification(ctx, email, password, "")
REDACTED

// RegisterWithVerification 用户注册（支持邮件验证），返回token和用户
func (s *AuthService) RegisterWithVerification(ctx context.Context, email, password, verifyCode string) (string, *User, error) {
	// 检查是否开放注册
	if s.settingService != nil && !s.settingService.IsRegistrationEnabled(ctx) {
		return "", nil, ErrRegDisabled
REDACTED

	// 检查是否需要邮件验证
	if s.settingService != nil && s.settingService.IsEmailVerifyEnabled(ctx) {
		if verifyCode == "" {
			return "", nil, ErrEmailVerifyRequired
	REDACTED
		// 验证邮箱验证码
		if s.emailService != nil {
			if err := s.emailService.VerifyCode(ctx, email, verifyCode); err != nil {
				return "", nil, fmt.Errorf("verify code: %w", err)
		REDACTED
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
		log.Printf("[Auth] Database error creating user: %v", err)
		return "", nil, ErrServiceUnavailable
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
	// 检查是否开放注册
	if s.settingService != nil && !s.settingService.IsRegistrationEnabled(ctx) {
		return ErrRegDisabled
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

	// 检查是否开放注册
	if s.settingService != nil && !s.settingService.IsRegistrationEnabled(ctx) {
		log.Println("[Auth] Registration is disabled")
		return nil, ErrRegDisabled
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
	if s.turnstileService == nil {
		return nil // 服务未配置则跳过验证
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
		return true
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

// ValidateToken 验证JWT token并返回用户声明
func (s *AuthService) ValidateToken(tokenString string) (*JWTClaims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &JWTClaims{REDACTED, func(token *jwt.Token) (any, error) {
		// 验证签名方法
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
	REDACTED
		return []byte(s.cfg.JWT.Secret), nil
REDACTED)

	if err != nil {
		if errors.Is(err, jwt.ErrTokenExpired) {
			return nil, ErrTokenExpired
	REDACTED
		return nil, ErrInvalidToken
REDACTED

	if claims, ok := token.Claims.(*JWTClaims); ok && token.Valid {
		return claims, nil
REDACTED

	return nil, ErrInvalidToken
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
