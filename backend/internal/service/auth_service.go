package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"net/mail"
	"strconv"
	"strings"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/authidentity"
	"github.com/Wei-Shaw/sub2api/internal/config"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

var (
	ErrInvalidCredentials      = infraerrors.Unauthorized("INVALID_CREDENTIALS", "invalid email or password")
	ErrUserNotActive           = infraerrors.Forbidden("USER_NOT_ACTIVE", "user is not active")
	ErrEmailExists             = infraerrors.Conflict("EMAIL_EXISTS", "email already exists")
	ErrEmailReserved           = infraerrors.BadRequest("EMAIL_RESERVED", "email is reserved")
	ErrInvalidToken            = infraerrors.Unauthorized("INVALID_TOKEN", "invalid token")
	ErrTokenExpired            = infraerrors.Unauthorized("TOKEN_EXPIRED", "token has expired")
	ErrAccessTokenExpired      = infraerrors.Unauthorized("ACCESS_TOKEN_EXPIRED", "access token has expired")
	ErrTokenTooLarge           = infraerrors.BadRequest("TOKEN_TOO_LARGE", "token too large")
	ErrTokenRevoked            = infraerrors.Unauthorized("TOKEN_REVOKED", "token has been revoked")
	ErrRefreshTokenInvalid     = infraerrors.Unauthorized("REFRESH_TOKEN_INVALID", "invalid refresh token")
	ErrRefreshTokenExpired     = infraerrors.Unauthorized("REFRESH_TOKEN_EXPIRED", "refresh token has expired")
	ErrRefreshTokenReused      = infraerrors.Unauthorized("REFRESH_TOKEN_REUSED", "refresh token has been reused")
	ErrEmailVerifyRequired     = infraerrors.BadRequest("EMAIL_VERIFY_REQUIRED", "email verification is required")
	ErrEmailSuffixNotAllowed   = infraerrors.BadRequest("EMAIL_SUFFIX_NOT_ALLOWED", "email suffix is not allowed")
	ErrRegDisabled             = infraerrors.Forbidden("REGISTRATION_DISABLED", "registration is currently disabled")
	ErrServiceUnavailable      = infraerrors.ServiceUnavailable("SERVICE_UNAVAILABLE", "service temporarily unavailable")
	ErrInvitationCodeRequired  = infraerrors.BadRequest("INVITATION_CODE_REQUIRED", "invitation code is required")
	ErrInvitationCodeInvalid   = infraerrors.BadRequest("INVITATION_CODE_INVALID", "invalid or used invitation code")
	ErrOAuthInvitationRequired = infraerrors.Forbidden("OAUTH_INVITATION_REQUIRED", "invitation code required to complete oauth registration")
)

// maxTokenLength 限制 token 大小，避免超长 header 触发解析时的异常内存分配。
const maxTokenLength = 8192

// refreshTokenPrefix is the prefix for refresh tokens to distinguish them from access tokens.
const refreshTokenPrefix = "rt_"

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
	entClient             *dbent.Client
	userRepo              UserRepository
	redeemRepo            RedeemCodeRepository
	refreshTokenCache     RefreshTokenCache
	cfg                   *config.Config
	settingService        *SettingService
	emailService          *EmailService
	turnstileService      *TurnstileService
	emailQueueService     *EmailQueueService
	promoService          *PromoService
	affiliateService      *AffiliateService
	defaultSubAssigner    DefaultSubscriptionAssigner
	userPlatformQuotaRepo UserPlatformQuotaRepository
REDACTED

type DefaultSubscriptionAssigner interface {
	AssignOrExtendSubscription(ctx context.Context, input *AssignSubscriptionInput) (*UserSubscription, bool, error)
REDACTED

type signupGrantPlan struct {
	Balance        float64
	Concurrency    int
	Subscriptions  []DefaultSubscriptionSetting
	PlatformQuotas map[string]*DefaultPlatformQuotaSetting
REDACTED

// NewAuthService 创建认证服务实例
func NewAuthService(
	entClient *dbent.Client,
	userRepo UserRepository,
	redeemRepo RedeemCodeRepository,
	refreshTokenCache RefreshTokenCache,
	cfg *config.Config,
	settingService *SettingService,
	emailService *EmailService,
	turnstileService *TurnstileService,
	emailQueueService *EmailQueueService,
	promoService *PromoService,
	defaultSubAssigner DefaultSubscriptionAssigner,
	affiliateService *AffiliateService,
	userPlatformQuotaRepo UserPlatformQuotaRepository,
) *AuthService {
	return &AuthService{
		entClient:             entClient,
		userRepo:              userRepo,
		redeemRepo:            redeemRepo,
		refreshTokenCache:     refreshTokenCache,
		cfg:                   cfg,
		settingService:        settingService,
		emailService:          emailService,
		turnstileService:      turnstileService,
		emailQueueService:     emailQueueService,
		promoService:          promoService,
		affiliateService:      affiliateService,
		defaultSubAssigner:    defaultSubAssigner,
		userPlatformQuotaRepo: userPlatformQuotaRepo,
REDACTED
REDACTED

func (s *AuthService) EntClient() *dbent.Client {
	if s == nil {
		return nil
REDACTED
	return s.entClient
REDACTED

// Register 用户注册，返回token和用户
func (s *AuthService) Register(ctx context.Context, email, password string) (string, *User, error) {
	return s.RegisterWithVerification(ctx, email, password, "", "", "", "")
REDACTED

// RegisterWithVerification 用户注册（支持邮件验证、优惠码、邀请码和邀请返利码），返回token和用户。
func (s *AuthService) RegisterWithVerification(ctx context.Context, email, password, verifyCode, promoCode, invitationCode, affiliateCode string) (string, *User, error) {
	// 检查是否开放注册（默认关闭：settingService 未配置时不允许注册）
	if s.settingService == nil || !s.settingService.IsRegistrationEnabled(ctx) {
		return "", nil, ErrRegDisabled
REDACTED

	// 防止用户注册 LinuxDo OAuth 合成邮箱，避免第三方登录与本地账号发生碰撞。
	if isReservedEmail(email) {
		return "", nil, ErrEmailReserved
REDACTED
	if err := s.validateRegistrationEmailPolicy(ctx, email); err != nil {
		return "", nil, err
REDACTED

	// 检查是否需要邀请码
	var invitationRedeemCode *RedeemCode
	if s.settingService != nil && s.settingService.IsInvitationCodeEnabled(ctx) {
		if invitationCode == "" {
			return "", nil, ErrInvitationCodeRequired
	REDACTED
		// 验证邀请码
		redeemCode, err := s.redeemRepo.GetByCode(ctx, invitationCode)
		if err != nil {
			logger.LegacyPrintf("service.auth", "[Auth] Invalid invitation code: %s, error: %v", invitationCode, err)
			return "", nil, ErrInvitationCodeInvalid
	REDACTED
		// 检查类型和状态
		if redeemCode.Type != RedeemTypeInvitation || !redeemCode.CanUse() {
			logger.LegacyPrintf("service.auth", "[Auth] Invitation code invalid: type=%s, status=%s", redeemCode.Type, redeemCode.Status)
			return "", nil, ErrInvitationCodeInvalid
	REDACTED
		invitationRedeemCode = redeemCode
REDACTED

	// 检查是否需要邮件验证
	if s.settingService != nil && s.settingService.IsEmailVerifyEnabled(ctx) {
		// 如果邮件验证已开启但邮件服务未配置，拒绝注册
		// 这是一个配置错误，不应该允许绕过验证
		if s.emailService == nil {
			logger.LegacyPrintf("service.auth", "%s", "[Auth] Email verification enabled but email service not configured, rejecting registration")
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
		logger.LegacyPrintf("service.auth", "[Auth] Database error checking email exists: %v", err)
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

	grantPlan := s.resolveSignupGrantPlan(ctx, "email")

	// 新用户默认 RPM（0 = 不限制）。注册时写入，后续作为用户级兜底。
	var defaultRPMLimit int
	if s.settingService != nil {
		defaultRPMLimit = s.settingService.GetDefaultUserRPMLimit(ctx)
REDACTED

	// 创建用户
	user := &User{
		Email:        email,
		PasswordHash: hashedPassword,
		Role:         RoleUser,
		Balance:      grantPlan.Balance,
		Concurrency:  grantPlan.Concurrency,
		RPMLimit:     defaultRPMLimit,
		Status:       StatusActive,
REDACTED

	if err := s.userRepo.Create(ctx, user); err != nil {
		// 优先检查邮箱冲突错误（竞态条件下可能发生）
		if errors.Is(err, ErrEmailExists) {
			return "", nil, ErrEmailExists
	REDACTED
		logger.LegacyPrintf("service.auth", "[Auth] Database error creating user: %v", err)
		return "", nil, ErrServiceUnavailable
REDACTED
	s.postAuthUserBootstrap(ctx, user, "email", true)
	s.assignSubscriptions(ctx, user.ID, grantPlan.Subscriptions, "auto assigned by signup defaults")
	// snapshot user × platform quota（fail-open）
	_ = s.snapshotPlatformQuotaDefaults(ctx, user.ID, &grantPlan)
	if s.affiliateService != nil {
		if _, err := s.affiliateService.EnsureUserAffiliate(ctx, user.ID); err != nil {
			logger.LegacyPrintf("service.auth", "[Auth] Failed to initialize affiliate profile for user %d: %v", user.ID, err)
	REDACTED
		if code := strings.TrimSpace(affiliateCode); code != "" {
			if err := s.affiliateService.BindInviterByCode(ctx, user.ID, code); err != nil {
				// 邀请返利码绑定失败不影响注册，只记录日志
				logger.LegacyPrintf("service.auth", "[Auth] Failed to bind affiliate inviter for user %d: %v", user.ID, err)
		REDACTED
	REDACTED
REDACTED

	// 标记邀请码为已使用（如果使用了邀请码）
	if invitationRedeemCode != nil {
		if err := s.redeemRepo.Use(ctx, invitationRedeemCode.ID, user.ID); err != nil {
			// 邀请码标记失败不影响注册，只记录日志
			logger.LegacyPrintf("service.auth", "[Auth] Failed to mark invitation code as used for user %d: %v", user.ID, err)
	REDACTED
REDACTED
	// 应用优惠码（如果提供且功能已启用）
	if promoCode != "" && s.promoService != nil && s.settingService != nil && s.settingService.IsPromoCodeEnabled(ctx) {
		if err := s.promoService.ApplyPromoCode(ctx, user.ID, promoCode); err != nil {
			// 优惠码应用失败不影响注册，只记录日志
			logger.LegacyPrintf("service.auth", "[Auth] Failed to apply promo code for user %d: %v", user.ID, err)
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
func (s *AuthService) SendVerifyCode(ctx context.Context, email string, locale ...string) error {
	// 检查是否开放注册（默认关闭）
	if s.settingService == nil || !s.settingService.IsRegistrationEnabled(ctx) {
		return ErrRegDisabled
REDACTED

	if isReservedEmail(email) {
		return ErrEmailReserved
REDACTED
	if err := s.validateRegistrationEmailPolicy(ctx, email); err != nil {
		return err
REDACTED

	// 检查邮箱是否已存在
	existsEmail, err := s.userRepo.ExistsByEmail(ctx, email)
	if err != nil {
		logger.LegacyPrintf("service.auth", "[Auth] Database error checking email exists: %v", err)
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

	return s.emailService.SendVerifyCode(ctx, email, siteName, firstEmailLocale(locale))
REDACTED

// SendVerifyCodeAsync 异步发送邮箱验证码并返回倒计时
func (s *AuthService) SendVerifyCodeAsync(ctx context.Context, email string, locale ...string) (*SendVerifyCodeResult, error) {
	logger.LegacyPrintf("service.auth", "[Auth] SendVerifyCodeAsync called for email: %s", email)

	// 检查是否开放注册（默认关闭）
	if s.settingService == nil || !s.settingService.IsRegistrationEnabled(ctx) {
		logger.LegacyPrintf("service.auth", "%s", "[Auth] Registration is disabled")
		return nil, ErrRegDisabled
REDACTED

	if isReservedEmail(email) {
		return nil, ErrEmailReserved
REDACTED
	if err := s.validateRegistrationEmailPolicy(ctx, email); err != nil {
		return nil, err
REDACTED

	// 检查邮箱是否已存在
	existsEmail, err := s.userRepo.ExistsByEmail(ctx, email)
	if err != nil {
		logger.LegacyPrintf("service.auth", "[Auth] Database error checking email exists: %v", err)
		return nil, ErrServiceUnavailable
REDACTED
	if existsEmail {
		logger.LegacyPrintf("service.auth", "[Auth] Email already exists: %s", email)
		return nil, ErrEmailExists
REDACTED

	// 检查邮件队列服务是否配置
	if s.emailQueueService == nil {
		logger.LegacyPrintf("service.auth", "%s", "[Auth] Email queue service not configured")
		return nil, errors.New("email queue service not configured")
REDACTED

	// 获取网站名称
	siteName := "Sub2API"
	if s.settingService != nil {
		siteName = s.settingService.GetSiteName(ctx)
REDACTED

	// 异步发送
	logger.LegacyPrintf("service.auth", "[Auth] Enqueueing verify code for: %s", email)
	if err := s.emailQueueService.EnqueueVerifyCode(email, siteName, firstEmailLocale(locale)); err != nil {
		logger.LegacyPrintf("service.auth", "[Auth] Failed to enqueue: %v", err)
		return nil, fmt.Errorf("enqueue verify code: %w", err)
REDACTED

	logger.LegacyPrintf("service.auth", "[Auth] Verify code enqueued successfully for: %s", email)
	return &SendVerifyCodeResult{
		Countdown: 60, // 60秒倒计时
REDACTED, nil
REDACTED

// VerifyTurnstileForRegister 在注册场景下验证 Turnstile。
// 当邮箱验证开启且已提交验证码时，说明验证码发送阶段已完成 Turnstile 校验，
// 此处跳过二次校验，避免一次性 token 在注册提交时重复使用导致误报失败。
func (s *AuthService) VerifyTurnstileForRegister(ctx context.Context, token, remoteIP, verifyCode string) error {
	if s.IsEmailVerifyEnabled(ctx) && strings.TrimSpace(verifyCode) != "" {
		logger.LegacyPrintf("service.auth", "%s", "[Auth] Email verify flow detected, skip duplicate Turnstile check on register")
		return nil
REDACTED
	return s.VerifyTurnstile(ctx, token, remoteIP)
REDACTED

// VerifyTurnstile 验证Turnstile token
func (s *AuthService) VerifyTurnstile(ctx context.Context, token string, remoteIP string) error {
	required := s.cfg != nil && s.cfg.Server.Mode == "release" && s.cfg.Turnstile.Required

	if required {
		if s.settingService == nil {
			logger.LegacyPrintf("service.auth", "%s", "[Auth] Turnstile required but settings service is not configured")
			return ErrTurnstileNotConfigured
	REDACTED
		enabled := s.settingService.IsTurnstileEnabled(ctx)
		secretConfigured := s.settingService.GetTurnstileSecretKey(ctx) != ""
		if !enabled || !secretConfigured {
			logger.LegacyPrintf("service.auth", "[Auth] Turnstile required but not configured (enabled=%v, secret_configured=%v)", enabled, secretConfigured)
			return ErrTurnstileNotConfigured
	REDACTED
REDACTED

	if s.turnstileService == nil {
		if required {
			logger.LegacyPrintf("service.auth", "%s", "[Auth] Turnstile required but service not configured")
			return ErrTurnstileNotConfigured
	REDACTED
		return nil // 服务未配置则跳过验证
REDACTED

	if !required && s.settingService != nil && s.settingService.IsTurnstileEnabled(ctx) && s.settingService.GetTurnstileSecretKey(ctx) == "" {
		logger.LegacyPrintf("service.auth", "%s", "[Auth] Turnstile enabled but secret key not configured")
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
		logger.LegacyPrintf("service.auth", "[Auth] Database error during login: %v", err)
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
				logger.LegacyPrintf("service.auth", "[Auth] Failed to generate random password for oauth signup: %v", err)
				return "", nil, ErrServiceUnavailable
		REDACTED
			hashedPassword, err := s.HashPassword(randomPassword)
			if err != nil {
				return "", nil, fmt.Errorf("hash password: %w", err)
		REDACTED

			signupSource := inferLegacySignupSource(email)
			grantPlan := s.resolveSignupGrantPlan(ctx, signupSource)
			var defaultRPMLimit int
			if s.settingService != nil {
				defaultRPMLimit = s.settingService.GetDefaultUserRPMLimit(ctx)
		REDACTED

			newUser := &User{
				Email:        email,
				Username:     username,
				PasswordHash: hashedPassword,
				Role:         RoleUser,
				Balance:      grantPlan.Balance,
				Concurrency:  grantPlan.Concurrency,
				RPMLimit:     defaultRPMLimit,
				Status:       StatusActive,
				SignupSource: signupSource,
		REDACTED

			if err := s.userRepo.Create(ctx, newUser); err != nil {
				if errors.Is(err, ErrEmailExists) {
					// 并发场景：GetByEmail 与 Create 之间用户被创建。
					user, err = s.userRepo.GetByEmail(ctx, email)
					if err != nil {
						logger.LegacyPrintf("service.auth", "[Auth] Database error getting user after conflict: %v", err)
						return "", nil, ErrServiceUnavailable
				REDACTED
			REDACTED else {
					logger.LegacyPrintf("service.auth", "[Auth] Database error creating oauth user: %v", err)
					return "", nil, ErrServiceUnavailable
			REDACTED
		REDACTED else {
				user = newUser
				s.postAuthUserBootstrap(ctx, user, signupSource, false)
				s.assignSubscriptions(ctx, user.ID, grantPlan.Subscriptions, "auto assigned by signup defaults")
				// snapshot user × platform quota（fail-open）
				_ = s.snapshotPlatformQuotaDefaults(ctx, user.ID, &grantPlan)
		REDACTED
	REDACTED else {
			logger.LegacyPrintf("service.auth", "[Auth] Database error during oauth login: %v", err)
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
			logger.LegacyPrintf("service.auth", "[Auth] Failed to update username after oauth login: %v", err)
	REDACTED
REDACTED
	token, err := s.GenerateToken(user)
	if err != nil {
		return "", nil, fmt.Errorf("generate token: %w", err)
REDACTED
	return token, user, nil
REDACTED

// canBypassRegistrationDisabledForOAuth 在钉钉企业模式（internal_only）且
// dingtalk_connect_bypass_registration=true 时，允许跳过全局 registration_enabled 检查。
func (s *AuthService) canBypassRegistrationDisabledForOAuth(ctx context.Context, signupSource string) bool {
	if signupSource != "dingtalk" {
		return false
REDACTED
	cfg, err := s.settingService.GetDingTalkConnectOAuthConfig(ctx)
	if err != nil || !cfg.Enabled || !cfg.BypassRegistration {
		return false
REDACTED
	return cfg.CorpRestrictionPolicy == "internal_only"
REDACTED

// LoginOrRegisterOAuthWithTokenPair 用于第三方 OAuth/SSO 登录，返回完整的 TokenPair。
// 与 LoginOrRegisterOAuth 功能相同，但返回 TokenPair 而非单个 token。
// invitationCode 仅在邀请码注册模式下新用户注册时使用；已有账号登录时忽略。
// affiliateCode 用于邀请返利绑定，仅在新用户注册时使用。
// signupSource 标识来源渠道（"dingtalk"/"linuxdo"/"wechat"/"oidc" 等），仅用于豁免检查。
func (s *AuthService) LoginOrRegisterOAuthWithTokenPair(ctx context.Context, email, username, invitationCode, affiliateCode, signupSource string) (*TokenPair, *User, error) {
	return s.loginOrRegisterOAuthWithTokenPair(ctx, email, username, invitationCode, affiliateCode, "", signupSource)
REDACTED

// LoginOrRegisterOAuthWithTokenPairAndPromoCode behaves like
// LoginOrRegisterOAuthWithTokenPair and applies promoCode only when a new user
// is created.
func (s *AuthService) LoginOrRegisterOAuthWithTokenPairAndPromoCode(ctx context.Context, email, username, invitationCode, affiliateCode, promoCode, signupSource string) (*TokenPair, *User, error) {
	return s.loginOrRegisterOAuthWithTokenPair(ctx, email, username, invitationCode, affiliateCode, promoCode, signupSource)
REDACTED

func (s *AuthService) loginOrRegisterOAuthWithTokenPair(ctx context.Context, email, username, invitationCode, affiliateCode, promoCode, signupSource string) (*TokenPair, *User, error) {
	// 检查 refreshTokenCache 是否可用
	if s.refreshTokenCache == nil {
		return nil, nil, errors.New("refresh token cache not configured")
REDACTED

	email = strings.TrimSpace(email)
	if email == "" || len(email) > 255 {
		return nil, nil, infraerrors.BadRequest("INVALID_EMAIL", "invalid email")
REDACTED
	if _, err := mail.ParseAddress(email); err != nil {
		return nil, nil, infraerrors.BadRequest("INVALID_EMAIL", "invalid email")
REDACTED

	username = strings.TrimSpace(username)
	if len([]rune(username)) > 100 {
		username = string([]rune(username)[:100])
REDACTED

	user, err := s.userRepo.GetByEmail(ctx, email)
	created := false
	if err != nil {
		if errors.Is(err, ErrUserNotFound) {
			// OAuth 首次登录视为注册
			if s.settingService == nil || (!s.settingService.IsRegistrationEnabled(ctx) && !s.canBypassRegistrationDisabledForOAuth(ctx, signupSource)) {
				return nil, nil, ErrRegDisabled
		REDACTED

			// 检查是否需要邀请码
			var invitationRedeemCode *RedeemCode
			if s.settingService != nil && s.settingService.IsInvitationCodeEnabled(ctx) {
				if invitationCode == "" {
					return nil, nil, ErrOAuthInvitationRequired
			REDACTED
				redeemCode, err := s.redeemRepo.GetByCode(ctx, invitationCode)
				if err != nil {
					return nil, nil, ErrInvitationCodeInvalid
			REDACTED
				if redeemCode.Type != RedeemTypeInvitation || !redeemCode.CanUse() {
					return nil, nil, ErrInvitationCodeInvalid
			REDACTED
				invitationRedeemCode = redeemCode
		REDACTED

			randomPassword, err := randomHexString(32)
			if err != nil {
				logger.LegacyPrintf("service.auth", "[Auth] Failed to generate random password for oauth signup: %v", err)
				return nil, nil, ErrServiceUnavailable
		REDACTED
			hashedPassword, err := s.HashPassword(randomPassword)
			if err != nil {
				return nil, nil, fmt.Errorf("hash password: %w", err)
		REDACTED

			// 优先用 caller 显式传入的 signupSource（如 "dingtalk" / "linuxdo" / "oidc" / "wechat"），
			// 否则才按邮箱后缀推断——避免有真实邮箱的 OAuth 用户被推断为 "email" 渠道，导致渠道授权错读。
			if strings.TrimSpace(signupSource) == "" {
				signupSource = inferLegacySignupSource(email)
		REDACTED
			grantPlan := s.resolveSignupGrantPlan(ctx, signupSource)
			var defaultRPMLimit int
			if s.settingService != nil {
				defaultRPMLimit = s.settingService.GetDefaultUserRPMLimit(ctx)
		REDACTED

			newUser := &User{
				Email:        email,
				Username:     username,
				PasswordHash: hashedPassword,
				Role:         RoleUser,
				Balance:      grantPlan.Balance,
				Concurrency:  grantPlan.Concurrency,
				RPMLimit:     defaultRPMLimit,
				Status:       StatusActive,
				SignupSource: signupSource,
		REDACTED

			if s.entClient != nil && invitationRedeemCode != nil {
				tx, err := s.entClient.Tx(ctx)
				if err != nil {
					logger.LegacyPrintf("service.auth", "[Auth] Failed to begin transaction for oauth registration: %v", err)
					return nil, nil, ErrServiceUnavailable
			REDACTED
				defer func() { _ = tx.Rollback() REDACTED()
				txCtx := dbent.NewTxContext(ctx, tx)

				if err := s.userRepo.Create(txCtx, newUser); err != nil {
					if errors.Is(err, ErrEmailExists) {
						user, err = s.userRepo.GetByEmail(ctx, email)
						if err != nil {
							logger.LegacyPrintf("service.auth", "[Auth] Database error getting user after conflict: %v", err)
							return nil, nil, ErrServiceUnavailable
					REDACTED
				REDACTED else {
						logger.LegacyPrintf("service.auth", "[Auth] Database error creating oauth user: %v", err)
						return nil, nil, ErrServiceUnavailable
				REDACTED
			REDACTED else {
					if err := s.redeemRepo.Use(txCtx, invitationRedeemCode.ID, newUser.ID); err != nil {
						return nil, nil, ErrInvitationCodeInvalid
				REDACTED
					if err := tx.Commit(); err != nil {
						logger.LegacyPrintf("service.auth", "[Auth] Failed to commit oauth registration transaction: %v", err)
						return nil, nil, ErrServiceUnavailable
				REDACTED
					user = newUser
					created = true
					s.postAuthUserBootstrap(ctx, user, signupSource, false)
					s.assignSubscriptions(ctx, user.ID, grantPlan.Subscriptions, "auto assigned by signup defaults")
					// snapshot user × platform quota（fail-open）
					_ = s.snapshotPlatformQuotaDefaults(ctx, user.ID, &grantPlan)
					s.bindOAuthAffiliate(ctx, user.ID, affiliateCode)
			REDACTED
		REDACTED else {
				if err := s.userRepo.Create(ctx, newUser); err != nil {
					if errors.Is(err, ErrEmailExists) {
						user, err = s.userRepo.GetByEmail(ctx, email)
						if err != nil {
							logger.LegacyPrintf("service.auth", "[Auth] Database error getting user after conflict: %v", err)
							return nil, nil, ErrServiceUnavailable
					REDACTED
				REDACTED else {
						logger.LegacyPrintf("service.auth", "[Auth] Database error creating oauth user: %v", err)
						return nil, nil, ErrServiceUnavailable
				REDACTED
			REDACTED else {
					user = newUser
					created = true
					s.postAuthUserBootstrap(ctx, user, signupSource, false)
					s.assignSubscriptions(ctx, user.ID, grantPlan.Subscriptions, "auto assigned by signup defaults")
					// snapshot user × platform quota（fail-open）
					_ = s.snapshotPlatformQuotaDefaults(ctx, user.ID, &grantPlan)
					s.bindOAuthAffiliate(ctx, user.ID, affiliateCode)
					if invitationRedeemCode != nil {
						if err := s.redeemRepo.Use(ctx, invitationRedeemCode.ID, user.ID); err != nil {
							return nil, nil, ErrInvitationCodeInvalid
					REDACTED
				REDACTED
			REDACTED
		REDACTED
	REDACTED else {
			logger.LegacyPrintf("service.auth", "[Auth] Database error during oauth login: %v", err)
			return nil, nil, ErrServiceUnavailable
	REDACTED
REDACTED

	if !user.IsActive() {
		return nil, nil, ErrUserNotActive
REDACTED

	if user.Username == "" && username != "" {
		user.Username = username
		if err := s.userRepo.Update(ctx, user); err != nil {
			logger.LegacyPrintf("service.auth", "[Auth] Failed to update username after oauth login: %v", err)
	REDACTED
REDACTED
	if created {
		user = s.applyOAuthSignupPromoCode(ctx, user, promoCode)
REDACTED
	tokenPair, err := s.GenerateTokenPair(ctx, user, "")
	if err != nil {
		return nil, nil, fmt.Errorf("generate token pair: %w", err)
REDACTED
	return tokenPair, user, nil
REDACTED

func (s *AuthService) ApplyOAuthSignupPromoCode(ctx context.Context, userID int64, promoCode string) {
	if userID <= 0 {
		return
REDACTED
	s.applyOAuthSignupPromoCode(ctx, &User{ID: userIDREDACTED, promoCode)
REDACTED

func (s *AuthService) applyOAuthSignupPromoCode(ctx context.Context, user *User, promoCode string) *User {
	promoCode = strings.TrimSpace(promoCode)
	if user == nil || user.ID <= 0 || promoCode == "" || s.promoService == nil || s.settingService == nil || !s.settingService.IsPromoCodeEnabled(ctx) {
		return user
REDACTED
	if err := s.promoService.ApplyPromoCode(ctx, user.ID, promoCode); err != nil {
		logger.LegacyPrintf("service.auth", "[Auth] Failed to apply promo code for oauth user %d: %v", user.ID, err)
		return user
REDACTED
	if updatedUser, err := s.userRepo.GetByID(ctx, user.ID); err == nil {
		return updatedUser
REDACTED
	return user
REDACTED

func (s *AuthService) assignSubscriptions(ctx context.Context, userID int64, items []DefaultSubscriptionSetting, notes string) {
	if s.settingService == nil || s.defaultSubAssigner == nil || userID <= 0 {
		return
REDACTED
	for _, item := range items {
		if _, _, err := s.defaultSubAssigner.AssignOrExtendSubscription(ctx, &AssignSubscriptionInput{
			UserID:       userID,
			GroupID:      item.GroupID,
			ValidityDays: item.ValidityDays,
			Notes:        notes,
	REDACTED); err != nil {
			logger.LegacyPrintf("service.auth", "[Auth] Failed to assign default subscription: user_id=%d group_id=%d err=%v", userID, item.GroupID, err)
	REDACTED
REDACTED
REDACTED

func (s *AuthService) resolveSignupGrantPlan(ctx context.Context, signupSource string) signupGrantPlan {
	plan := signupGrantPlan{REDACTED
	if s != nil && s.cfg != nil {
		plan.Balance = s.cfg.Default.UserBalance
		plan.Concurrency = s.cfg.Default.UserConcurrency
REDACTED
	if s == nil || s.settingService == nil {
		return plan
REDACTED

	plan.Balance = s.settingService.GetDefaultBalance(ctx)
	plan.Concurrency = s.settingService.GetDefaultConcurrency(ctx)
	plan.Subscriptions = s.settingService.GetDefaultSubscriptions(ctx)

	// ============ 全局 quota 装载（必须在 ResolveAuthSourceGrantSettings 之前） ============
	// 无论 auth source 是否 enabled，全局层都要先装载，确保 !enabled 早退路径也携带全局 quota。
	if quotas, err := s.settingService.GetDefaultPlatformQuotas(ctx); err == nil {
		plan.PlatformQuotas = quotas
REDACTED else {
		logger.LegacyPrintf("service.auth", "[Auth] Warning: load default platform quotas failed: %v (fail-open)", err)
REDACTED
	// ============================================================================================

	resolved, enabled, err := s.settingService.ResolveAuthSourceGrantSettings(ctx, signupSource, false)
	if err != nil {
		logger.LegacyPrintf("service.auth", "[Auth] Failed to load auth source signup defaults for %s: %v", signupSource, err)
		return plan
REDACTED
	if !enabled {
		return plan // plan.PlatformQuotas 已含全局层
REDACTED

	plan.Balance = resolved.Balance
	plan.Concurrency = resolved.Concurrency
	plan.Subscriptions = resolved.Subscriptions

	// ============ auth source quota merge（仅在 enabled 分支内） ============
	asQuotas := s.settingService.GetAuthSourcePlatformQuotas(ctx, signupSource)
	if plan.PlatformQuotas != nil {
		for platform, patch := range asQuotas {
			if dst := plan.PlatformQuotas[platform]; dst != nil {
				mergePlatformQuotaDefaults(dst, patch)
		REDACTED
	REDACTED
REDACTED
	// ==============================================================================

	return plan
REDACTED

func authSourceSignupSettings(defaults *AuthSourceDefaultSettings, signupSource string) (ProviderDefaultGrantSettings, bool) {
	if defaults == nil {
		return ProviderDefaultGrantSettings{REDACTED, false
REDACTED

	switch strings.ToLower(strings.TrimSpace(signupSource)) {
	case "email":
		return defaults.Email, true
	case "linuxdo":
		return defaults.LinuxDo, true
	case "oidc":
		return defaults.OIDC, true
	case "wechat":
		return defaults.WeChat, true
	case "github":
		return defaults.GitHub, true
	case "google":
		return defaults.Google, true
	case "dingtalk":
		return defaults.DingTalk, true
	default:
		return ProviderDefaultGrantSettings{REDACTED, false
REDACTED
REDACTED

// bindOAuthAffiliate initializes the affiliate profile and binds the inviter
// for an OAuth-registered user. Failures are logged but never block registration.
func (s *AuthService) bindOAuthAffiliate(ctx context.Context, userID int64, affiliateCode string) {
	if s.affiliateService == nil || userID <= 0 {
		return
REDACTED
	if _, err := s.affiliateService.EnsureUserAffiliate(ctx, userID); err != nil {
		logger.LegacyPrintf("service.auth", "[Auth] Failed to initialize affiliate profile for user %d: %v", userID, err)
REDACTED
	if code := strings.TrimSpace(affiliateCode); code != "" {
		if err := s.affiliateService.BindInviterByCode(ctx, userID, code); err != nil {
			logger.LegacyPrintf("service.auth", "[Auth] Failed to bind affiliate inviter for user %d: %v", userID, err)
	REDACTED
REDACTED
REDACTED

func (s *AuthService) postAuthUserBootstrap(ctx context.Context, user *User, signupSource string, touchLogin bool) {
	if user == nil || user.ID <= 0 {
		return
REDACTED

	if strings.TrimSpace(signupSource) == "" {
		signupSource = "email"
REDACTED
	s.updateUserSignupSource(ctx, user.ID, signupSource)

	if touchLogin {
		s.touchUserLogin(ctx, user.ID)
REDACTED
REDACTED

func (s *AuthService) updateUserSignupSource(ctx context.Context, userID int64, signupSource string) {
	if s == nil || s.entClient == nil || userID <= 0 {
		return
REDACTED
	if strings.TrimSpace(signupSource) == "" {
		return
REDACTED
	if err := s.entClient.User.UpdateOneID(userID).
		SetSignupSource(signupSource).
		Exec(ctx); err != nil {
		logger.LegacyPrintf("service.auth", "[Auth] Failed to update signup source: user_id=%d source=%s err=%v", userID, signupSource, err)
REDACTED
REDACTED

func (s *AuthService) touchUserLogin(ctx context.Context, userID int64) {
	if s == nil || s.entClient == nil || userID <= 0 {
		return
REDACTED
	now := time.Now().UTC()
	if err := s.entClient.User.UpdateOneID(userID).
		SetLastLoginAt(now).
		SetLastActiveAt(now).
		Exec(ctx); err != nil {
		logger.LegacyPrintf("service.auth", "[Auth] Failed to touch login timestamps: user_id=%d err=%v", userID, err)
REDACTED
REDACTED

func (s *AuthService) backfillEmailIdentityOnSuccessfulLogin(ctx context.Context, user *User) {
	if s == nil || user == nil || user.ID <= 0 {
		return
REDACTED
	identity, created := s.ensureEmailAuthIdentity(ctx, user, "auth_service_login_backfill")
	if s.shouldApplyEmailFirstBindDefaults(ctx, user.ID, identity, created) {
		if err := s.ApplyProviderDefaultSettingsOnFirstBind(ctx, user.ID, "email"); err != nil {
			logger.LegacyPrintf("service.auth", "[Auth] Failed to apply email first bind defaults: user_id=%d err=%v", user.ID, err)
	REDACTED
REDACTED
REDACTED

func (s *AuthService) shouldApplyEmailFirstBindDefaults(
	ctx context.Context,
	userID int64,
	identity *dbent.AuthIdentity,
	created bool,
) bool {
	source := emailAuthIdentitySource(identity.Metadata)
	if source == "auth_service_login_backfill" {
		return false
REDACTED
	if created {
		return true
REDACTED
	if s == nil || s.entClient == nil || userID <= 0 || identity == nil || identity.UserID != userID {
		return false
REDACTED
	if source != "auth_service_dual_write" {
		return false
REDACTED

	hasGrant, err := s.hasProviderGrantRecord(ctx, userID, "email", "first_bind")
	if err != nil {
		logger.LegacyPrintf("service.auth", "[Auth] Failed to inspect email first bind grant state: user_id=%d err=%v", userID, err)
		return false
REDACTED
	return !hasGrant
REDACTED

func emailAuthIdentitySource(metadata map[string]any) string {
	if len(metadata) == 0 {
		return ""
REDACTED
	raw, ok := metadata["source"]
	if !ok {
		return ""
REDACTED
	return strings.TrimSpace(fmt.Sprint(raw))
REDACTED

func (s *AuthService) hasProviderGrantRecord(
	ctx context.Context,
	userID int64,
	providerType string,
	grantReason string,
) (bool, error) {
	if s == nil || s.entClient == nil || userID <= 0 {
		return false, nil
REDACTED

	rows, err := s.entClient.QueryContext(
		ctx,
		`SELECT 1 FROM user_provider_default_grants WHERE user_id = $1 AND provider_type = $2 AND grant_reason = $3 LIMIT 1`,
		userID,
		strings.TrimSpace(providerType),
		strings.TrimSpace(grantReason),
	)
	if err != nil {
		return false, err
REDACTED
	defer func() { _ = rows.Close() REDACTED()
	return rows.Next(), rows.Err()
REDACTED

func (s *AuthService) ensureEmailAuthIdentity(ctx context.Context, user *User, source string) (*dbent.AuthIdentity, bool) {
	if s == nil || s.entClient == nil || user == nil || user.ID <= 0 {
		return nil, false
REDACTED

	email := strings.ToLower(strings.TrimSpace(user.Email))
	if email == "" || isReservedEmail(email) {
		return nil, false
REDACTED
	if strings.TrimSpace(source) == "" {
		source = "auth_service_dual_write"
REDACTED

	client := s.entClient
	if tx := dbent.TxFromContext(ctx); tx != nil {
		client = tx.Client()
REDACTED

	buildQuery := func() *dbent.AuthIdentityQuery {
		return client.AuthIdentity.Query().Where(
			authidentity.ProviderTypeEQ("email"),
			authidentity.ProviderKeyEQ("email"),
			authidentity.ProviderSubjectEQ(email),
		)
REDACTED

	existed, err := buildQuery().Exist(ctx)
	if err != nil {
		logger.LegacyPrintf("service.auth", "[Auth] Failed to inspect email auth identity: user_id=%d email=%s err=%v", user.ID, email, err)
		return nil, false
REDACTED

	if !existed {
		if err := client.AuthIdentity.Create().
			SetUserID(user.ID).
			SetProviderType("email").
			SetProviderKey("email").
			SetProviderSubject(email).
			SetVerifiedAt(time.Now().UTC()).
			SetMetadata(map[string]any{
				"source": strings.TrimSpace(source),
		REDACTED).
			OnConflictColumns(
				authidentity.FieldProviderType,
				authidentity.FieldProviderKey,
				authidentity.FieldProviderSubject,
			).
			DoNothing().
			Exec(ctx); err != nil {
			if isSQLNoRowsError(err) {
				return nil, false
		REDACTED
	REDACTED
		if err != nil {
			logger.LegacyPrintf("service.auth", "[Auth] Failed to ensure email auth identity: user_id=%d email=%s err=%v", user.ID, email, err)
			return nil, false
	REDACTED
REDACTED

	identity, err := buildQuery().Only(ctx)
	if err != nil {
		logger.LegacyPrintf("service.auth", "[Auth] Failed to reload email auth identity: user_id=%d email=%s err=%v", user.ID, email, err)
		return nil, false
REDACTED
	if identity.UserID != user.ID {
		logger.LegacyPrintf("service.auth", "[Auth] Email auth identity ownership mismatch: user_id=%d email=%s owner_id=%d", user.ID, email, identity.UserID)
		return nil, false
REDACTED

	return identity, !existed
REDACTED

func inferLegacySignupSource(email string) string {
	normalized := strings.ToLower(strings.TrimSpace(email))
	switch {
	case strings.HasSuffix(normalized, DingTalkConnectSyntheticEmailDomain):
		return "dingtalk"
	case strings.HasSuffix(normalized, LinuxDoConnectSyntheticEmailDomain):
		return "linuxdo"
	case strings.HasSuffix(normalized, OIDCConnectSyntheticEmailDomain):
		return "oidc"
	case strings.HasSuffix(normalized, WeChatConnectSyntheticEmailDomain):
		return "wechat"
	default:
		return "email"
REDACTED
REDACTED

func (s *AuthService) validateRegistrationEmailPolicy(ctx context.Context, email string) error {
	if s.settingService == nil {
		return nil
REDACTED
	whitelist := s.settingService.GetRegistrationEmailSuffixWhitelist(ctx)
	if !IsRegistrationEmailSuffixAllowed(email, whitelist) {
		return buildEmailSuffixNotAllowedError(whitelist)
REDACTED
	return nil
REDACTED

func buildEmailSuffixNotAllowedError(whitelist []string) error {
	if len(whitelist) == 0 {
		return ErrEmailSuffixNotAllowed
REDACTED

	allowed := strings.Join(whitelist, ", ")
	return infraerrors.BadRequest(
		"EMAIL_SUFFIX_NOT_ALLOWED",
		fmt.Sprintf("email suffix is not allowed, allowed suffixes: %s", allowed),
	).WithMetadata(map[string]string{
		"allowed_suffixes":     strings.Join(whitelist, ","),
		"allowed_suffix_count": strconv.Itoa(len(whitelist)),
REDACTED)
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
	return strings.HasSuffix(normalized, LinuxDoConnectSyntheticEmailDomain) ||
		strings.HasSuffix(normalized, OIDCConnectSyntheticEmailDomain) ||
		strings.HasSuffix(normalized, WeChatConnectSyntheticEmailDomain) ||
		strings.HasSuffix(normalized, DingTalkConnectSyntheticEmailDomain)
REDACTED

// GenerateToken 生成JWT access token
// 使用新的access_token_expire_minutes配置项（如果配置了），否则回退到expire_hour
func (s *AuthService) GenerateToken(user *User) (string, error) {
	now := time.Now()
	var expiresAt time.Time
	if s.cfg.JWT.AccessTokenExpireMinutes > 0 {
		expiresAt = now.Add(time.Duration(s.cfg.JWT.AccessTokenExpireMinutes) * time.Minute)
REDACTED else {
		// 向后兼容：使用旧的expire_hour配置
		expiresAt = now.Add(time.Duration(s.cfg.JWT.ExpireHour) * time.Hour)
REDACTED

	claims := &JWTClaims{
		UserID:       user.ID,
		Email:        user.Email,
		Role:         user.Role,
		TokenVersion: resolvedTokenVersion(user),
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

// GetAccessTokenExpiresIn 返回Access Token的有效期（秒）
// 用于前端设置刷新定时器
func (s *AuthService) GetAccessTokenExpiresIn() int {
	if s.cfg.JWT.AccessTokenExpireMinutes > 0 {
		return s.cfg.JWT.AccessTokenExpireMinutes * 60
REDACTED
	return s.cfg.JWT.ExpireHour * 3600
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
		logger.LegacyPrintf("service.auth", "[Auth] Database error refreshing token: %v", err)
		return "", ErrServiceUnavailable
REDACTED

	// 检查用户状态
	if !user.IsActive() {
		return "", ErrUserNotActive
REDACTED

	// Security: Check TokenVersion to prevent refreshing revoked tokens
	// This ensures tokens issued before a password change cannot be refreshed
	if claims.TokenVersion != resolvedTokenVersion(user) {
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
			logger.LegacyPrintf("service.auth", "[Auth] Password reset requested for non-existent email: %s", email)
			return "", "", false
	REDACTED
		logger.LegacyPrintf("service.auth", "[Auth] Database error checking email for password reset: %v", err)
		return "", "", false
REDACTED

	// Check if user is active
	if !user.IsActive() {
		logger.LegacyPrintf("service.auth", "[Auth] Password reset requested for inactive user: %s", email)
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
func (s *AuthService) RequestPasswordReset(ctx context.Context, email, frontendBaseURL string, locale ...string) error {
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

	if err := s.emailService.SendPasswordResetEmail(ctx, email, siteName, resetURL, firstEmailLocale(locale)); err != nil {
		logger.LegacyPrintf("service.auth", "[Auth] Failed to send password reset email to %s: %v", email, err)
		return nil // Silent success to prevent enumeration
REDACTED

	logger.LegacyPrintf("service.auth", "[Auth] Password reset email sent to: %s", email)
	return nil
REDACTED

// RequestPasswordResetAsync 异步请求密码重置（队列发送）
// Security: Returns the same response regardless of whether the email exists (prevent user enumeration)
func (s *AuthService) RequestPasswordResetAsync(ctx context.Context, email, frontendBaseURL string, locale ...string) error {
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

	if err := s.emailQueueService.EnqueuePasswordReset(email, siteName, resetURL, firstEmailLocale(locale)); err != nil {
		logger.LegacyPrintf("service.auth", "[Auth] Failed to enqueue password reset email for %s: %v", email, err)
		return nil // Silent success to prevent enumeration
REDACTED

	logger.LegacyPrintf("service.auth", "[Auth] Password reset email enqueued for: %s", email)
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
		logger.LegacyPrintf("service.auth", "[Auth] Database error getting user for password reset: %v", err)
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
		logger.LegacyPrintf("service.auth", "[Auth] Database error updating password for user %d: %v", user.ID, err)
		return ErrServiceUnavailable
REDACTED

	// Also revoke all refresh tokens for this user
	if err := s.RevokeAllUserSessions(ctx, user.ID); err != nil {
		logger.LegacyPrintf("service.auth", "[Auth] Failed to revoke refresh tokens for user %d: %v", user.ID, err)
		// Don't return error - password was already changed successfully
REDACTED

	logger.LegacyPrintf("service.auth", "[Auth] Password reset successful for user: %s", email)
	return nil
REDACTED

// ==================== Refresh Token Methods ====================

// TokenPair 包含Access Token和Refresh Token
type TokenPair struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"` // Access Token有效期（秒）
REDACTED

// TokenPairWithUser extends TokenPair with user role for backend mode checks
type TokenPairWithUser struct {
	TokenPair
	UserRole string
REDACTED

// GenerateTokenPair 生成Access Token和Refresh Token对
// familyID: 可选的Token家族ID，用于Token轮转时保持家族关系
func (s *AuthService) GenerateTokenPair(ctx context.Context, user *User, familyID string) (*TokenPair, error) {
	// 检查 refreshTokenCache 是否可用
	if s.refreshTokenCache == nil {
		return nil, errors.New("refresh token cache not configured")
REDACTED

	// 生成Access Token
	accessToken, err := s.GenerateToken(user)
	if err != nil {
		return nil, fmt.Errorf("generate access token: %w", err)
REDACTED

	// 生成Refresh Token
	refreshToken, err := s.generateRefreshToken(ctx, user, familyID)
	if err != nil {
		return nil, fmt.Errorf("generate refresh token: %w", err)
REDACTED

	return &TokenPair{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		ExpiresIn:    s.GetAccessTokenExpiresIn(),
REDACTED, nil
REDACTED

// generateRefreshToken 生成并存储Refresh Token
func (s *AuthService) generateRefreshToken(ctx context.Context, user *User, familyID string) (string, error) {
	// 生成随机Token
	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		return "", fmt.Errorf("generate random bytes: %w", err)
REDACTED
	rawToken := refreshTokenPrefix + hex.EncodeToString(tokenBytes)

	// 计算Token哈希（存储哈希而非原始Token）
	tokenHash := hashToken(rawToken)

	// 如果没有提供familyID，生成新的
	if familyID == "" {
		familyBytes := make([]byte, 16)
		if _, err := rand.Read(familyBytes); err != nil {
			return "", fmt.Errorf("generate family id: %w", err)
	REDACTED
		familyID = hex.EncodeToString(familyBytes)
REDACTED

	now := time.Now()
	ttl := time.Duration(s.cfg.JWT.RefreshTokenExpireDays) * 24 * time.Hour

	data := &RefreshTokenData{
		UserID:       user.ID,
		TokenVersion: resolvedTokenVersion(user),
		FamilyID:     familyID,
		CreatedAt:    now,
		ExpiresAt:    now.Add(ttl),
REDACTED

	// 存储Token数据
	if err := s.refreshTokenCache.StoreRefreshToken(ctx, tokenHash, data, ttl); err != nil {
		return "", fmt.Errorf("store refresh token: %w", err)
REDACTED

	// 添加到用户Token集合
	if err := s.refreshTokenCache.AddToUserTokenSet(ctx, user.ID, tokenHash, ttl); err != nil {
		logger.LegacyPrintf("service.auth", "[Auth] Failed to add token to user set: %v", err)
		// 不影响主流程
REDACTED

	// 添加到家族Token集合
	if err := s.refreshTokenCache.AddToFamilyTokenSet(ctx, familyID, tokenHash, ttl); err != nil {
		logger.LegacyPrintf("service.auth", "[Auth] Failed to add token to family set: %v", err)
		// 不影响主流程
REDACTED

	return rawToken, nil
REDACTED

// RefreshTokenPair 使用Refresh Token刷新Token对
// 实现Token轮转：每次刷新都会生成新的Refresh Token，旧Token立即失效
func (s *AuthService) RefreshTokenPair(ctx context.Context, refreshToken string) (*TokenPairWithUser, error) {
	// 检查 refreshTokenCache 是否可用
	if s.refreshTokenCache == nil {
		return nil, ErrRefreshTokenInvalid
REDACTED

	// 验证Token格式
	if !strings.HasPrefix(refreshToken, refreshTokenPrefix) {
		return nil, ErrRefreshTokenInvalid
REDACTED

	tokenHash := hashToken(refreshToken)

	// 获取Token数据
	data, err := s.refreshTokenCache.GetRefreshToken(ctx, tokenHash)
	if err != nil {
		if errors.Is(err, ErrRefreshTokenNotFound) {
			// Token不存在，可能是已被使用（Token轮转）或已过期
			logger.LegacyPrintf("service.auth", "[Auth] Refresh token not found, possible reuse attack")
			return nil, ErrRefreshTokenInvalid
	REDACTED
		logger.LegacyPrintf("service.auth", "[Auth] Error getting refresh token: %v", err)
		return nil, ErrServiceUnavailable
REDACTED

	// 检查Token是否过期
	if time.Now().After(data.ExpiresAt) {
		// 删除过期Token
		_ = s.refreshTokenCache.DeleteRefreshToken(ctx, tokenHash)
		return nil, ErrRefreshTokenExpired
REDACTED

	// 获取用户信息
	user, err := s.userRepo.GetByID(ctx, data.UserID)
	if err != nil {
		if errors.Is(err, ErrUserNotFound) {
			// 用户已删除，撤销整个Token家族
			_ = s.refreshTokenCache.DeleteTokenFamily(ctx, data.FamilyID)
			return nil, ErrRefreshTokenInvalid
	REDACTED
		logger.LegacyPrintf("service.auth", "[Auth] Database error getting user for token refresh: %v", err)
		return nil, ErrServiceUnavailable
REDACTED

	// 检查用户状态
	if !user.IsActive() {
		// 用户被禁用，撤销整个Token家族
		_ = s.refreshTokenCache.DeleteTokenFamily(ctx, data.FamilyID)
		return nil, ErrUserNotActive
REDACTED

	// 检查TokenVersion（密码更改后所有Token失效）
	if data.TokenVersion != resolvedTokenVersion(user) {
		// TokenVersion不匹配，撤销整个Token家族
		_ = s.refreshTokenCache.DeleteTokenFamily(ctx, data.FamilyID)
		return nil, ErrTokenRevoked
REDACTED

	// Token轮转：立即使旧Token失效
	if err := s.refreshTokenCache.DeleteRefreshToken(ctx, tokenHash); err != nil {
		logger.LegacyPrintf("service.auth", "[Auth] Failed to delete old refresh token: %v", err)
		// 继续处理，不影响主流程
REDACTED

	// 生成新的Token对，保持同一个家族ID
	pair, err := s.GenerateTokenPair(ctx, user, data.FamilyID)
	if err != nil {
		return nil, err
REDACTED
	return &TokenPairWithUser{
		TokenPair: *pair,
		UserRole:  user.Role,
REDACTED, nil
REDACTED

// RevokeRefreshToken 撤销单个Refresh Token
func (s *AuthService) RevokeRefreshToken(ctx context.Context, refreshToken string) error {
	if s.refreshTokenCache == nil {
		return nil // No-op if cache not configured
REDACTED
	if !strings.HasPrefix(refreshToken, refreshTokenPrefix) {
		return ErrRefreshTokenInvalid
REDACTED

	tokenHash := hashToken(refreshToken)
	return s.refreshTokenCache.DeleteRefreshToken(ctx, tokenHash)
REDACTED

// RevokeAllUserSessions 撤销用户的所有会话（所有Refresh Token）
// 用于密码更改或用户主动登出所有设备
func (s *AuthService) RevokeAllUserSessions(ctx context.Context, userID int64) error {
	if s.refreshTokenCache == nil {
		return nil // No-op if cache not configured
REDACTED
	return s.refreshTokenCache.DeleteUserRefreshTokens(ctx, userID)
REDACTED

// RevokeAllUserTokens invalidates both stateless access tokens and refresh sessions.
// Access/refresh token verification both depend on TokenVersion, so bumping it provides
// immediate revocation even if refresh-token cache cleanup later fails.
func (s *AuthService) RevokeAllUserTokens(ctx context.Context, userID int64) error {
	user, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		return fmt.Errorf("get user: %w", err)
REDACTED

	user.TokenVersion++
	if err := s.userRepo.Update(ctx, user); err != nil {
		return fmt.Errorf("update user: %w", err)
REDACTED

	if err := s.RevokeAllUserSessions(ctx, userID); err != nil {
		logger.LegacyPrintf("service.auth", "[Auth] Failed to revoke refresh sessions after token invalidation for user %d: %v", userID, err)
REDACTED
	return nil
REDACTED

// hashToken 计算Token的SHA256哈希
func hashToken(token string) string {
	hash := sha256.Sum256([]byte(token))
	return hex.EncodeToString(hash[:])
REDACTED

func resolvedTokenVersion(user *User) int64 {
	if user == nil {
		return 0
REDACTED
	if user.TokenVersionResolved {
		return user.TokenVersion
REDACTED

	material := strings.ToLower(strings.TrimSpace(user.Email)) + "\n" + user.PasswordHash
	sum := sha256.Sum256([]byte(material))
	fingerprint := int64(binary.BigEndian.Uint64(sum[:8]) & 0x7fffffffffffffff)
	return user.TokenVersion ^ fingerprint
REDACTED

// snapshotPlatformQuotaDefaults 把 plan.PlatformQuotas（4 platform × 3 window）以
// BulkInsertInitial 形式写入 user_platform_quotas 表。失败 fail-open（仅 warn log）。
func (s *AuthService) snapshotPlatformQuotaDefaults(ctx context.Context, userID int64, plan *signupGrantPlan) error {
	if s.userPlatformQuotaRepo == nil || plan == nil || len(plan.PlatformQuotas) == 0 {
		return nil
REDACTED
	records := make([]UserPlatformQuotaRecord, 0, len(plan.PlatformQuotas))
	for platform, q := range plan.PlatformQuotas {
		rec := UserPlatformQuotaRecord{
			UserID:   userID,
			Platform: platform,
	REDACTED
		if q != nil {
			rec.DailyLimitUSD = q.DailyLimitUSD
			rec.WeeklyLimitUSD = q.WeeklyLimitUSD
			rec.MonthlyLimitUSD = q.MonthlyLimitUSD
	REDACTED
		records = append(records, rec)
REDACTED
	if err := s.userPlatformQuotaRepo.BulkInsertInitial(ctx, records); err != nil {
		logger.LegacyPrintf("service.auth", "[Auth] Warning: snapshot platform quota failed user=%d: %v (fail-open)", userID, err)
		return nil // fail-open：返回 nil，让调用方继续
REDACTED
	return nil
REDACTED
