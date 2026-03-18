package admin

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/handler/dto"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// semverPattern 预编译 semver 格式校验正则
var semverPattern = regexp.MustCompile(`^\d+\.\d+\.\d+$`)

// menuItemIDPattern validates custom menu item IDs: alphanumeric, hyphens, underscores only.
var menuItemIDPattern = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

// generateMenuItemID generates a short random hex ID for a custom menu item.
func generateMenuItemID() (string, error) {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate menu item ID: %w", err)
	}
	return hex.EncodeToString(b), nil
}

// SettingHandler 系统设置处理器
type SettingHandler struct {
	settingService        *service.SettingService
	emailService          *service.EmailService
	turnstileService      *service.TurnstileService
	opsService            *service.OpsService
	soraS3Storage         *service.SoraS3Storage
	soraGDriveStorage     *service.SoraGDriveStorage
	soraGenerationService *service.SoraGenerationService
}

// NewSettingHandler 创建系统设置处理器
func NewSettingHandler(settingService *service.SettingService, emailService *service.EmailService, turnstileService *service.TurnstileService, opsService *service.OpsService, soraS3Storage *service.SoraS3Storage, soraGDriveStorage *service.SoraGDriveStorage, soraGenerationService *service.SoraGenerationService) *SettingHandler {
	return &SettingHandler{
		settingService:        settingService,
		emailService:          emailService,
		turnstileService:      turnstileService,
		opsService:            opsService,
		soraS3Storage:         soraS3Storage,
		soraGDriveStorage:     soraGDriveStorage,
		soraGenerationService: soraGenerationService,
	}
}

// GetSettings 获取所有系统设置
// GET /api/v1/admin/settings
func (h *SettingHandler) GetSettings(c *gin.Context) {
	settings, err := h.settingService.GetAllSettings(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	// Check if ops monitoring is enabled (respects config.ops.enabled)
	opsEnabled := h.opsService != nil && h.opsService.IsMonitoringEnabled(c.Request.Context())
	defaultSubscriptions := make([]dto.DefaultSubscriptionSetting, 0, len(settings.DefaultSubscriptions))
	for _, sub := range settings.DefaultSubscriptions {
		defaultSubscriptions = append(defaultSubscriptions, dto.DefaultSubscriptionSetting{
			GroupID:      sub.GroupID,
			ValidityDays: sub.ValidityDays,
		})
	}

	response.Success(c, dto.SystemSettings{
		RegistrationEnabled:                  settings.RegistrationEnabled,
		EmailVerifyEnabled:                   settings.EmailVerifyEnabled,
		RegistrationEmailSuffixWhitelist:     settings.RegistrationEmailSuffixWhitelist,
		PromoCodeEnabled:                     settings.PromoCodeEnabled,
		PasswordResetEnabled:                 settings.PasswordResetEnabled,
		FrontendURL:                          settings.FrontendURL,
		InvitationCodeEnabled:                settings.InvitationCodeEnabled,
		TotpEnabled:                          settings.TotpEnabled,
		TotpEncryptionKeyConfigured:          h.settingService.IsTotpEncryptionKeyConfigured(),
		SMTPHost:                             settings.SMTPHost,
		SMTPPort:                             settings.SMTPPort,
		SMTPUsername:                         settings.SMTPUsername,
		SMTPPasswordConfigured:               settings.SMTPPasswordConfigured,
		SMTPFrom:                             settings.SMTPFrom,
		SMTPFromName:                         settings.SMTPFromName,
		SMTPUseTLS:                           settings.SMTPUseTLS,
		TurnstileEnabled:                     settings.TurnstileEnabled,
		TurnstileSiteKey:                     settings.TurnstileSiteKey,
		TurnstileSecretKeyConfigured:         settings.TurnstileSecretKeyConfigured,
		LinuxDoConnectEnabled:                settings.LinuxDoConnectEnabled,
		LinuxDoConnectClientID:               settings.LinuxDoConnectClientID,
		LinuxDoConnectClientSecretConfigured: settings.LinuxDoConnectClientSecretConfigured,
		LinuxDoConnectRedirectURL:            settings.LinuxDoConnectRedirectURL,
		SiteName:                             settings.SiteName,
		SiteLogo:                             settings.SiteLogo,
		SiteSubtitle:                         settings.SiteSubtitle,
		APIBaseURL:                           settings.APIBaseURL,
		ContactInfo:                          settings.ContactInfo,
		DocURL:                               settings.DocURL,
		HomeContent:                          settings.HomeContent,
		HideCcsImportButton:                  settings.HideCcsImportButton,
		PurchaseSubscriptionEnabled:          settings.PurchaseSubscriptionEnabled,
		PurchaseSubscriptionURL:              settings.PurchaseSubscriptionURL,
		SoraClientEnabled:                    settings.SoraClientEnabled,
		CustomMenuItems:                      dto.ParseCustomMenuItems(settings.CustomMenuItems),
		DefaultConcurrency:                   settings.DefaultConcurrency,
		DefaultBalance:                       settings.DefaultBalance,
		DefaultSubscriptions:                 defaultSubscriptions,
		EnableModelFallback:                  settings.EnableModelFallback,
		FallbackModelAnthropic:               settings.FallbackModelAnthropic,
		FallbackModelOpenAI:                  settings.FallbackModelOpenAI,
		FallbackModelGemini:                  settings.FallbackModelGemini,
		FallbackModelAntigravity:             settings.FallbackModelAntigravity,
		EnableIdentityPatch:                  settings.EnableIdentityPatch,
		IdentityPatchPrompt:                  settings.IdentityPatchPrompt,
		OpsMonitoringEnabled:                 opsEnabled && settings.OpsMonitoringEnabled,
		OpsRealtimeMonitoringEnabled:         settings.OpsRealtimeMonitoringEnabled,
		OpsQueryModeDefault:                  settings.OpsQueryModeDefault,
		OpsMetricsIntervalSeconds:            settings.OpsMetricsIntervalSeconds,
		MinClaudeCodeVersion:                 settings.MinClaudeCodeVersion,
		AllowUngroupedKeyScheduling:          settings.AllowUngroupedKeyScheduling,
		BackendModeEnabled:                   settings.BackendModeEnabled,
	})
}

// UpdateSettingsRequest 更新设置请求
type UpdateSettingsRequest struct {
	// 注册设置
	RegistrationEnabled              bool     `json:"registration_enabled"`
	EmailVerifyEnabled               bool     `json:"email_verify_enabled"`
	RegistrationEmailSuffixWhitelist []string `json:"registration_email_suffix_whitelist"`
	PromoCodeEnabled                 bool     `json:"promo_code_enabled"`
	PasswordResetEnabled             bool     `json:"password_reset_enabled"`
	FrontendURL                      string   `json:"frontend_url"`
	InvitationCodeEnabled            bool     `json:"invitation_code_enabled"`
	TotpEnabled                      bool     `json:"totp_enabled"` // TOTP 双因素认证

	// 邮件服务设置
	SMTPHost     string `json:"smtp_host"`
	SMTPPort     int    `json:"smtp_port"`
	SMTPUsername string `json:"smtp_username"`
	SMTPPassword string `json:"smtp_password"`
	SMTPFrom     string `json:"smtp_from_email"`
	SMTPFromName string `json:"smtp_from_name"`
	SMTPUseTLS   bool   `json:"smtp_use_tls"`

	// Cloudflare Turnstile 设置
	TurnstileEnabled   bool   `json:"turnstile_enabled"`
	TurnstileSiteKey   string `json:"turnstile_site_key"`
	TurnstileSecretKey string `json:"turnstile_secret_key"`

	// LinuxDo Connect OAuth 登录
	LinuxDoConnectEnabled      bool   `json:"linuxdo_connect_enabled"`
	LinuxDoConnectClientID     string `json:"linuxdo_connect_client_id"`
	LinuxDoConnectClientSecret string `json:"linuxdo_connect_client_secret"`
	LinuxDoConnectRedirectURL  string `json:"linuxdo_connect_redirect_url"`

	// OEM设置
	SiteName                    string                `json:"site_name"`
	SiteLogo                    string                `json:"site_logo"`
	SiteSubtitle                string                `json:"site_subtitle"`
	APIBaseURL                  string                `json:"api_base_url"`
	ContactInfo                 string                `json:"contact_info"`
	DocURL                      string                `json:"doc_url"`
	HomeContent                 string                `json:"home_content"`
	HideCcsImportButton         bool                  `json:"hide_ccs_import_button"`
	PurchaseSubscriptionEnabled *bool                 `json:"purchase_subscription_enabled"`
	PurchaseSubscriptionURL     *string               `json:"purchase_subscription_url"`
	SoraClientEnabled           bool                  `json:"sora_client_enabled"`
	CustomMenuItems             *[]dto.CustomMenuItem `json:"custom_menu_items"`

	// 默认配置
	DefaultConcurrency   int                              `json:"default_concurrency"`
	DefaultBalance       float64                          `json:"default_balance"`
	DefaultSubscriptions []dto.DefaultSubscriptionSetting `json:"default_subscriptions"`

	// Model fallback configuration
	EnableModelFallback      bool   `json:"enable_model_fallback"`
	FallbackModelAnthropic   string `json:"fallback_model_anthropic"`
	FallbackModelOpenAI      string `json:"fallback_model_openai"`
	FallbackModelGemini      string `json:"fallback_model_gemini"`
	FallbackModelAntigravity string `json:"fallback_model_antigravity"`

	// Identity patch configuration (Claude -> Gemini)
	EnableIdentityPatch bool   `json:"enable_identity_patch"`
	IdentityPatchPrompt string `json:"identity_patch_prompt"`

	// Ops monitoring (vNext)
	OpsMonitoringEnabled         *bool   `json:"ops_monitoring_enabled"`
	OpsRealtimeMonitoringEnabled *bool   `json:"ops_realtime_monitoring_enabled"`
	OpsQueryModeDefault          *string `json:"ops_query_mode_default"`
	OpsMetricsIntervalSeconds    *int    `json:"ops_metrics_interval_seconds"`

	MinClaudeCodeVersion string `json:"min_claude_code_version"`

	// 分组隔离
	AllowUngroupedKeyScheduling bool `json:"allow_ungrouped_key_scheduling"`

	// Backend Mode
	BackendModeEnabled bool `json:"backend_mode_enabled"`
}

// UpdateSettings 更新系统设置
// PUT /api/v1/admin/settings
func (h *SettingHandler) UpdateSettings(c *gin.Context) {
	var req UpdateSettingsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}

	previousSettings, err := h.settingService.GetAllSettings(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	// 验证参数
	if req.DefaultConcurrency < 1 {
		req.DefaultConcurrency = 1
	}
	if req.DefaultBalance < 0 {
		req.DefaultBalance = 0
	}
	if req.SMTPPort <= 0 {
		req.SMTPPort = 587
	}
	req.DefaultSubscriptions = normalizeDefaultSubscriptions(req.DefaultSubscriptions)

	// Turnstile 参数验证
	if req.TurnstileEnabled {
		// 检查必填字段
		if req.TurnstileSiteKey == "" {
			response.BadRequest(c, "Turnstile Site Key is required when enabled")
			return
		}
		// 如果未提供 secret key，使用已保存的值（留空保留当前值）
		if req.TurnstileSecretKey == "" {
			if previousSettings.TurnstileSecretKey == "" {
				response.BadRequest(c, "Turnstile Secret Key is required when enabled")
				return
			}
			req.TurnstileSecretKey = previousSettings.TurnstileSecretKey
		}

		// 当 site_key 或 secret_key 任一变化时验证（避免配置错误导致无法登录）
		siteKeyChanged := previousSettings.TurnstileSiteKey != req.TurnstileSiteKey
		secretKeyChanged := previousSettings.TurnstileSecretKey != req.TurnstileSecretKey
		if siteKeyChanged || secretKeyChanged {
			if err := h.turnstileService.ValidateSecretKey(c.Request.Context(), req.TurnstileSecretKey); err != nil {
				response.ErrorFrom(c, err)
				return
			}
		}
	}

	// TOTP 双因素认证参数验证
	// 只有手动配置了加密密钥才允许启用 TOTP 功能
	if req.TotpEnabled && !previousSettings.TotpEnabled {
		// 尝试启用 TOTP，检查加密密钥是否已手动配置
		if !h.settingService.IsTotpEncryptionKeyConfigured() {
			response.BadRequest(c, "Cannot enable TOTP: TOTP_ENCRYPTION_KEY environment variable must be configured first. Generate a key with 'openssl rand -hex 32' and set it in your environment.")
			return
		}
	}

	// LinuxDo Connect 参数验证
	if req.LinuxDoConnectEnabled {
		req.LinuxDoConnectClientID = strings.TrimSpace(req.LinuxDoConnectClientID)
		req.LinuxDoConnectClientSecret = strings.TrimSpace(req.LinuxDoConnectClientSecret)
		req.LinuxDoConnectRedirectURL = strings.TrimSpace(req.LinuxDoConnectRedirectURL)

		if req.LinuxDoConnectClientID == "" {
			response.BadRequest(c, "LinuxDo Client ID is required when enabled")
			return
		}
		if req.LinuxDoConnectRedirectURL == "" {
			response.BadRequest(c, "LinuxDo Redirect URL is required when enabled")
			return
		}
		if err := config.ValidateAbsoluteHTTPURL(req.LinuxDoConnectRedirectURL); err != nil {
			response.BadRequest(c, "LinuxDo Redirect URL must be an absolute http(s) URL")
			return
		}

		// 如果未提供 client_secret，则保留现有值（如有）。
		if req.LinuxDoConnectClientSecret == "" {
			if previousSettings.LinuxDoConnectClientSecret == "" {
				response.BadRequest(c, "LinuxDo Client Secret is required when enabled")
				return
			}
			req.LinuxDoConnectClientSecret = previousSettings.LinuxDoConnectClientSecret
		}
	}

	// “购买订阅”页面配置验证
	purchaseEnabled := previousSettings.PurchaseSubscriptionEnabled
	if req.PurchaseSubscriptionEnabled != nil {
		purchaseEnabled = *req.PurchaseSubscriptionEnabled
	}
	purchaseURL := previousSettings.PurchaseSubscriptionURL
	if req.PurchaseSubscriptionURL != nil {
		purchaseURL = strings.TrimSpace(*req.PurchaseSubscriptionURL)
	}

	// - 启用时要求 URL 合法且非空
	// - 禁用时允许为空；若提供了 URL 也做基本校验，避免误配置
	if purchaseEnabled {
		if purchaseURL == "" {
			response.BadRequest(c, "Purchase Subscription URL is required when enabled")
			return
		}
		if err := config.ValidateAbsoluteHTTPURL(purchaseURL); err != nil {
			response.BadRequest(c, "Purchase Subscription URL must be an absolute http(s) URL")
			return
		}
	} else if purchaseURL != "" {
		if err := config.ValidateAbsoluteHTTPURL(purchaseURL); err != nil {
			response.BadRequest(c, "Purchase Subscription URL must be an absolute http(s) URL")
			return
		}
	}

	// Frontend URL 验证
	req.FrontendURL = strings.TrimSpace(req.FrontendURL)
	if req.FrontendURL != "" {
		if err := config.ValidateAbsoluteHTTPURL(req.FrontendURL); err != nil {
			response.BadRequest(c, "Frontend URL must be an absolute http(s) URL")
			return
		}
	}

	// 自定义菜单项验证
	const (
		maxCustomMenuItems    = 20
		maxMenuItemLabelLen   = 50
		maxMenuItemURLLen     = 2048
		maxMenuItemIconSVGLen = 10 * 1024 // 10KB
		maxMenuItemIDLen      = 32
	)

	customMenuJSON := previousSettings.CustomMenuItems
	if req.CustomMenuItems != nil {
		items := *req.CustomMenuItems
		if len(items) > maxCustomMenuItems {
			response.BadRequest(c, "Too many custom menu items (max 20)")
			return
		}
		for i, item := range items {
			if strings.TrimSpace(item.Label) == "" {
				response.BadRequest(c, "Custom menu item label is required")
				return
			}
			if len(item.Label) > maxMenuItemLabelLen {
				response.BadRequest(c, "Custom menu item label is too long (max 50 characters)")
				return
			}
			if strings.TrimSpace(item.URL) == "" {
				response.BadRequest(c, "Custom menu item URL is required")
				return
			}
			if len(item.URL) > maxMenuItemURLLen {
				response.BadRequest(c, "Custom menu item URL is too long (max 2048 characters)")
				return
			}
			if err := config.ValidateAbsoluteHTTPURL(strings.TrimSpace(item.URL)); err != nil {
				response.BadRequest(c, "Custom menu item URL must be an absolute http(s) URL")
				return
			}
			if item.Visibility != "user" && item.Visibility != "admin" {
				response.BadRequest(c, "Custom menu item visibility must be 'user' or 'admin'")
				return
			}
			if len(item.IconSVG) > maxMenuItemIconSVGLen {
				response.BadRequest(c, "Custom menu item icon SVG is too large (max 10KB)")
				return
			}
			// Auto-generate ID if missing
			if strings.TrimSpace(item.ID) == "" {
				id, err := generateMenuItemID()
				if err != nil {
					response.Error(c, http.StatusInternalServerError, "Failed to generate menu item ID")
					return
				}
				items[i].ID = id
			} else if len(item.ID) > maxMenuItemIDLen {
				response.BadRequest(c, "Custom menu item ID is too long (max 32 characters)")
				return
			} else if !menuItemIDPattern.MatchString(item.ID) {
				response.BadRequest(c, "Custom menu item ID contains invalid characters (only a-z, A-Z, 0-9, - and _ are allowed)")
				return
			}
		}
		// ID uniqueness check
		seen := make(map[string]struct{}, len(items))
		for _, item := range items {
			if _, exists := seen[item.ID]; exists {
				response.BadRequest(c, "Duplicate custom menu item ID: "+item.ID)
				return
			}
			seen[item.ID] = struct{}{}
		}
		menuBytes, err := json.Marshal(items)
		if err != nil {
			response.BadRequest(c, "Failed to serialize custom menu items")
			return
		}
		customMenuJSON = string(menuBytes)
	}

	// Ops metrics collector interval validation (seconds).
	if req.OpsMetricsIntervalSeconds != nil {
		v := *req.OpsMetricsIntervalSeconds
		if v < 60 {
			v = 60
		}
		if v > 3600 {
			v = 3600
		}
		req.OpsMetricsIntervalSeconds = &v
	}
	defaultSubscriptions := make([]service.DefaultSubscriptionSetting, 0, len(req.DefaultSubscriptions))
	for _, sub := range req.DefaultSubscriptions {
		defaultSubscriptions = append(defaultSubscriptions, service.DefaultSubscriptionSetting{
			GroupID:      sub.GroupID,
			ValidityDays: sub.ValidityDays,
		})
	}

	// 验证最低版本号格式（空字符串=禁用，或合法 semver）
	if req.MinClaudeCodeVersion != "" {
		if !semverPattern.MatchString(req.MinClaudeCodeVersion) {
			response.Error(c, http.StatusBadRequest, "min_claude_code_version must be empty or a valid semver (e.g. 2.1.63)")
			return
		}
	}

	settings := &service.SystemSettings{
		RegistrationEnabled:              req.RegistrationEnabled,
		EmailVerifyEnabled:               req.EmailVerifyEnabled,
		RegistrationEmailSuffixWhitelist: req.RegistrationEmailSuffixWhitelist,
		PromoCodeEnabled:                 req.PromoCodeEnabled,
		PasswordResetEnabled:             req.PasswordResetEnabled,
		FrontendURL:                      req.FrontendURL,
		InvitationCodeEnabled:            req.InvitationCodeEnabled,
		TotpEnabled:                      req.TotpEnabled,
		SMTPHost:                         req.SMTPHost,
		SMTPPort:                         req.SMTPPort,
		SMTPUsername:                     req.SMTPUsername,
		SMTPPassword:                     req.SMTPPassword,
		SMTPFrom:                         req.SMTPFrom,
		SMTPFromName:                     req.SMTPFromName,
		SMTPUseTLS:                       req.SMTPUseTLS,
		TurnstileEnabled:                 req.TurnstileEnabled,
		TurnstileSiteKey:                 req.TurnstileSiteKey,
		TurnstileSecretKey:               req.TurnstileSecretKey,
		LinuxDoConnectEnabled:            req.LinuxDoConnectEnabled,
		LinuxDoConnectClientID:           req.LinuxDoConnectClientID,
		LinuxDoConnectClientSecret:       req.LinuxDoConnectClientSecret,
		LinuxDoConnectRedirectURL:        req.LinuxDoConnectRedirectURL,
		SiteName:                         req.SiteName,
		SiteLogo:                         req.SiteLogo,
		SiteSubtitle:                     req.SiteSubtitle,
		APIBaseURL:                       req.APIBaseURL,
		ContactInfo:                      req.ContactInfo,
		DocURL:                           req.DocURL,
		HomeContent:                      req.HomeContent,
		HideCcsImportButton:              req.HideCcsImportButton,
		PurchaseSubscriptionEnabled:      purchaseEnabled,
		PurchaseSubscriptionURL:          purchaseURL,
		SoraClientEnabled:                req.SoraClientEnabled,
		CustomMenuItems:                  customMenuJSON,
		DefaultConcurrency:               req.DefaultConcurrency,
		DefaultBalance:                   req.DefaultBalance,
		DefaultSubscriptions:             defaultSubscriptions,
		EnableModelFallback:              req.EnableModelFallback,
		FallbackModelAnthropic:           req.FallbackModelAnthropic,
		FallbackModelOpenAI:              req.FallbackModelOpenAI,
		FallbackModelGemini:              req.FallbackModelGemini,
		FallbackModelAntigravity:         req.FallbackModelAntigravity,
		EnableIdentityPatch:              req.EnableIdentityPatch,
		IdentityPatchPrompt:              req.IdentityPatchPrompt,
		MinClaudeCodeVersion:             req.MinClaudeCodeVersion,
		AllowUngroupedKeyScheduling:      req.AllowUngroupedKeyScheduling,
		BackendModeEnabled:               req.BackendModeEnabled,
		OpsMonitoringEnabled: func() bool {
			if req.OpsMonitoringEnabled != nil {
				return *req.OpsMonitoringEnabled
			}
			return previousSettings.OpsMonitoringEnabled
		}(),
		OpsRealtimeMonitoringEnabled: func() bool {
			if req.OpsRealtimeMonitoringEnabled != nil {
				return *req.OpsRealtimeMonitoringEnabled
			}
			return previousSettings.OpsRealtimeMonitoringEnabled
		}(),
		OpsQueryModeDefault: func() string {
			if req.OpsQueryModeDefault != nil {
				return *req.OpsQueryModeDefault
			}
			return previousSettings.OpsQueryModeDefault
		}(),
		OpsMetricsIntervalSeconds: func() int {
			if req.OpsMetricsIntervalSeconds != nil {
				return *req.OpsMetricsIntervalSeconds
			}
			return previousSettings.OpsMetricsIntervalSeconds
		}(),
	}

	if err := h.settingService.UpdateSettings(c.Request.Context(), settings); err != nil {
		response.ErrorFrom(c, err)
		return
	}

	h.auditSettingsUpdate(c, previousSettings, settings, req)

	// 重新获取设置返回
	updatedSettings, err := h.settingService.GetAllSettings(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	updatedDefaultSubscriptions := make([]dto.DefaultSubscriptionSetting, 0, len(updatedSettings.DefaultSubscriptions))
	for _, sub := range updatedSettings.DefaultSubscriptions {
		updatedDefaultSubscriptions = append(updatedDefaultSubscriptions, dto.DefaultSubscriptionSetting{
			GroupID:      sub.GroupID,
			ValidityDays: sub.ValidityDays,
		})
	}

	response.Success(c, dto.SystemSettings{
		RegistrationEnabled:                  updatedSettings.RegistrationEnabled,
		EmailVerifyEnabled:                   updatedSettings.EmailVerifyEnabled,
		RegistrationEmailSuffixWhitelist:     updatedSettings.RegistrationEmailSuffixWhitelist,
		PromoCodeEnabled:                     updatedSettings.PromoCodeEnabled,
		PasswordResetEnabled:                 updatedSettings.PasswordResetEnabled,
		FrontendURL:                          updatedSettings.FrontendURL,
		InvitationCodeEnabled:                updatedSettings.InvitationCodeEnabled,
		TotpEnabled:                          updatedSettings.TotpEnabled,
		TotpEncryptionKeyConfigured:          h.settingService.IsTotpEncryptionKeyConfigured(),
		SMTPHost:                             updatedSettings.SMTPHost,
		SMTPPort:                             updatedSettings.SMTPPort,
		SMTPUsername:                         updatedSettings.SMTPUsername,
		SMTPPasswordConfigured:               updatedSettings.SMTPPasswordConfigured,
		SMTPFrom:                             updatedSettings.SMTPFrom,
		SMTPFromName:                         updatedSettings.SMTPFromName,
		SMTPUseTLS:                           updatedSettings.SMTPUseTLS,
		TurnstileEnabled:                     updatedSettings.TurnstileEnabled,
		TurnstileSiteKey:                     updatedSettings.TurnstileSiteKey,
		TurnstileSecretKeyConfigured:         updatedSettings.TurnstileSecretKeyConfigured,
		LinuxDoConnectEnabled:                updatedSettings.LinuxDoConnectEnabled,
		LinuxDoConnectClientID:               updatedSettings.LinuxDoConnectClientID,
		LinuxDoConnectClientSecretConfigured: updatedSettings.LinuxDoConnectClientSecretConfigured,
		LinuxDoConnectRedirectURL:            updatedSettings.LinuxDoConnectRedirectURL,
		SiteName:                             updatedSettings.SiteName,
		SiteLogo:                             updatedSettings.SiteLogo,
		SiteSubtitle:                         updatedSettings.SiteSubtitle,
		APIBaseURL:                           updatedSettings.APIBaseURL,
		ContactInfo:                          updatedSettings.ContactInfo,
		DocURL:                               updatedSettings.DocURL,
		HomeContent:                          updatedSettings.HomeContent,
		HideCcsImportButton:                  updatedSettings.HideCcsImportButton,
		PurchaseSubscriptionEnabled:          updatedSettings.PurchaseSubscriptionEnabled,
		PurchaseSubscriptionURL:              updatedSettings.PurchaseSubscriptionURL,
		SoraClientEnabled:                    updatedSettings.SoraClientEnabled,
		CustomMenuItems:                      dto.ParseCustomMenuItems(updatedSettings.CustomMenuItems),
		DefaultConcurrency:                   updatedSettings.DefaultConcurrency,
		DefaultBalance:                       updatedSettings.DefaultBalance,
		DefaultSubscriptions:                 updatedDefaultSubscriptions,
		EnableModelFallback:                  updatedSettings.EnableModelFallback,
		FallbackModelAnthropic:               updatedSettings.FallbackModelAnthropic,
		FallbackModelOpenAI:                  updatedSettings.FallbackModelOpenAI,
		FallbackModelGemini:                  updatedSettings.FallbackModelGemini,
		FallbackModelAntigravity:             updatedSettings.FallbackModelAntigravity,
		EnableIdentityPatch:                  updatedSettings.EnableIdentityPatch,
		IdentityPatchPrompt:                  updatedSettings.IdentityPatchPrompt,
		OpsMonitoringEnabled:                 updatedSettings.OpsMonitoringEnabled,
		OpsRealtimeMonitoringEnabled:         updatedSettings.OpsRealtimeMonitoringEnabled,
		OpsQueryModeDefault:                  updatedSettings.OpsQueryModeDefault,
		OpsMetricsIntervalSeconds:            updatedSettings.OpsMetricsIntervalSeconds,
		MinClaudeCodeVersion:                 updatedSettings.MinClaudeCodeVersion,
		AllowUngroupedKeyScheduling:          updatedSettings.AllowUngroupedKeyScheduling,
		BackendModeEnabled:                   updatedSettings.BackendModeEnabled,
	})
}

func (h *SettingHandler) auditSettingsUpdate(c *gin.Context, before *service.SystemSettings, after *service.SystemSettings, req UpdateSettingsRequest) {
	if before == nil || after == nil {
		return
	}

	changed := diffSettings(before, after, req)
	if len(changed) == 0 {
		return
	}

	subject, _ := middleware.GetAuthSubjectFromContext(c)
	role, _ := middleware.GetUserRoleFromContext(c)
	log.Printf("AUDIT: settings updated at=%s user_id=%d role=%s changed=%v",
		time.Now().UTC().Format(time.RFC3339),
		subject.UserID,
		role,
		changed,
	)
}

func diffSettings(before *service.SystemSettings, after *service.SystemSettings, req UpdateSettingsRequest) []string {
	changed := make([]string, 0, 20)
	if before.RegistrationEnabled != after.RegistrationEnabled {
		changed = append(changed, "registration_enabled")
	}
	if before.EmailVerifyEnabled != after.EmailVerifyEnabled {
		changed = append(changed, "email_verify_enabled")
	}
	if !equalStringSlice(before.RegistrationEmailSuffixWhitelist, after.RegistrationEmailSuffixWhitelist) {
		changed = append(changed, "registration_email_suffix_whitelist")
	}
	if before.PasswordResetEnabled != after.PasswordResetEnabled {
		changed = append(changed, "password_reset_enabled")
	}
	if before.FrontendURL != after.FrontendURL {
		changed = append(changed, "frontend_url")
	}
	if before.TotpEnabled != after.TotpEnabled {
		changed = append(changed, "totp_enabled")
	}
	if before.SMTPHost != after.SMTPHost {
		changed = append(changed, "smtp_host")
	}
	if before.SMTPPort != after.SMTPPort {
		changed = append(changed, "smtp_port")
	}
	if before.SMTPUsername != after.SMTPUsername {
		changed = append(changed, "smtp_username")
	}
	if req.SMTPPassword != "" {
		changed = append(changed, "smtp_password")
	}
	if before.SMTPFrom != after.SMTPFrom {
		changed = append(changed, "smtp_from_email")
	}
	if before.SMTPFromName != after.SMTPFromName {
		changed = append(changed, "smtp_from_name")
	}
	if before.SMTPUseTLS != after.SMTPUseTLS {
		changed = append(changed, "smtp_use_tls")
	}
	if before.TurnstileEnabled != after.TurnstileEnabled {
		changed = append(changed, "turnstile_enabled")
	}
	if before.TurnstileSiteKey != after.TurnstileSiteKey {
		changed = append(changed, "turnstile_site_key")
	}
	if req.TurnstileSecretKey != "" {
		changed = append(changed, "turnstile_secret_key")
	}
	if before.LinuxDoConnectEnabled != after.LinuxDoConnectEnabled {
		changed = append(changed, "linuxdo_connect_enabled")
	}
	if before.LinuxDoConnectClientID != after.LinuxDoConnectClientID {
		changed = append(changed, "linuxdo_connect_client_id")
	}
	if req.LinuxDoConnectClientSecret != "" {
		changed = append(changed, "linuxdo_connect_client_secret")
	}
	if before.LinuxDoConnectRedirectURL != after.LinuxDoConnectRedirectURL {
		changed = append(changed, "linuxdo_connect_redirect_url")
	}
	if before.SiteName != after.SiteName {
		changed = append(changed, "site_name")
	}
	if before.SiteLogo != after.SiteLogo {
		changed = append(changed, "site_logo")
	}
	if before.SiteSubtitle != after.SiteSubtitle {
		changed = append(changed, "site_subtitle")
	}
	if before.APIBaseURL != after.APIBaseURL {
		changed = append(changed, "api_base_url")
	}
	if before.ContactInfo != after.ContactInfo {
		changed = append(changed, "contact_info")
	}
	if before.DocURL != after.DocURL {
		changed = append(changed, "doc_url")
	}
	if before.HomeContent != after.HomeContent {
		changed = append(changed, "home_content")
	}
	if before.HideCcsImportButton != after.HideCcsImportButton {
		changed = append(changed, "hide_ccs_import_button")
	}
	if before.DefaultConcurrency != after.DefaultConcurrency {
		changed = append(changed, "default_concurrency")
	}
	if before.DefaultBalance != after.DefaultBalance {
		changed = append(changed, "default_balance")
	}
	if !equalDefaultSubscriptions(before.DefaultSubscriptions, after.DefaultSubscriptions) {
		changed = append(changed, "default_subscriptions")
	}
	if before.EnableModelFallback != after.EnableModelFallback {
		changed = append(changed, "enable_model_fallback")
	}
	if before.FallbackModelAnthropic != after.FallbackModelAnthropic {
		changed = append(changed, "fallback_model_anthropic")
	}
	if before.FallbackModelOpenAI != after.FallbackModelOpenAI {
		changed = append(changed, "fallback_model_openai")
	}
	if before.FallbackModelGemini != after.FallbackModelGemini {
		changed = append(changed, "fallback_model_gemini")
	}
	if before.FallbackModelAntigravity != after.FallbackModelAntigravity {
		changed = append(changed, "fallback_model_antigravity")
	}
	if before.EnableIdentityPatch != after.EnableIdentityPatch {
		changed = append(changed, "enable_identity_patch")
	}
	if before.IdentityPatchPrompt != after.IdentityPatchPrompt {
		changed = append(changed, "identity_patch_prompt")
	}
	if before.OpsMonitoringEnabled != after.OpsMonitoringEnabled {
		changed = append(changed, "ops_monitoring_enabled")
	}
	if before.OpsRealtimeMonitoringEnabled != after.OpsRealtimeMonitoringEnabled {
		changed = append(changed, "ops_realtime_monitoring_enabled")
	}
	if before.OpsQueryModeDefault != after.OpsQueryModeDefault {
		changed = append(changed, "ops_query_mode_default")
	}
	if before.OpsMetricsIntervalSeconds != after.OpsMetricsIntervalSeconds {
		changed = append(changed, "ops_metrics_interval_seconds")
	}
	if before.MinClaudeCodeVersion != after.MinClaudeCodeVersion {
		changed = append(changed, "min_claude_code_version")
	}
	if before.AllowUngroupedKeyScheduling != after.AllowUngroupedKeyScheduling {
		changed = append(changed, "allow_ungrouped_key_scheduling")
	}
	if before.BackendModeEnabled != after.BackendModeEnabled {
		changed = append(changed, "backend_mode_enabled")
	}
	if before.PurchaseSubscriptionEnabled != after.PurchaseSubscriptionEnabled {
		changed = append(changed, "purchase_subscription_enabled")
	}
	if before.PurchaseSubscriptionURL != after.PurchaseSubscriptionURL {
		changed = append(changed, "purchase_subscription_url")
	}
	if before.CustomMenuItems != after.CustomMenuItems {
		changed = append(changed, "custom_menu_items")
	}
	return changed
}

func normalizeDefaultSubscriptions(input []dto.DefaultSubscriptionSetting) []dto.DefaultSubscriptionSetting {
	if len(input) == 0 {
		return nil
	}
	normalized := make([]dto.DefaultSubscriptionSetting, 0, len(input))
	for _, item := range input {
		if item.GroupID <= 0 || item.ValidityDays <= 0 {
			continue
		}
		if item.ValidityDays > service.MaxValidityDays {
			item.ValidityDays = service.MaxValidityDays
		}
		normalized = append(normalized, item)
	}
	return normalized
}

func equalStringSlice(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func equalDefaultSubscriptions(a, b []service.DefaultSubscriptionSetting) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].GroupID != b[i].GroupID || a[i].ValidityDays != b[i].ValidityDays {
			return false
		}
	}
	return true
}

// TestSMTPRequest 测试SMTP连接请求
type TestSMTPRequest struct {
	SMTPHost     string `json:"smtp_host" binding:"required"`
	SMTPPort     int    `json:"smtp_port"`
	SMTPUsername string `json:"smtp_username"`
	SMTPPassword string `json:"smtp_password"`
	SMTPUseTLS   bool   `json:"smtp_use_tls"`
}

// TestSMTPConnection 测试SMTP连接
// POST /api/v1/admin/settings/test-smtp
func (h *SettingHandler) TestSMTPConnection(c *gin.Context) {
	var req TestSMTPRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}

	if req.SMTPPort <= 0 {
		req.SMTPPort = 587
	}

	// 如果未提供密码，从数据库获取已保存的密码
	password := req.SMTPPassword
	if password == "" {
		savedConfig, err := h.emailService.GetSMTPConfig(c.Request.Context())
		if err == nil && savedConfig != nil {
			password = savedConfig.Password
		}
	}

	config := &service.SMTPConfig{
		Host:     req.SMTPHost,
		Port:     req.SMTPPort,
		Username: req.SMTPUsername,
		Password: password,
		UseTLS:   req.SMTPUseTLS,
	}

	err := h.emailService.TestSMTPConnectionWithConfig(config)
	if err != nil {
		response.BadRequest(c, "SMTP connection test failed: "+err.Error())
		return
	}

	response.Success(c, gin.H{"message": "SMTP connection successful"})
}

// SendTestEmailRequest 发送测试邮件请求
type SendTestEmailRequest struct {
	Email        string `json:"email" binding:"required,email"`
	SMTPHost     string `json:"smtp_host" binding:"required"`
	SMTPPort     int    `json:"smtp_port"`
	SMTPUsername string `json:"smtp_username"`
	SMTPPassword string `json:"smtp_password"`
	SMTPFrom     string `json:"smtp_from_email"`
	SMTPFromName string `json:"smtp_from_name"`
	SMTPUseTLS   bool   `json:"smtp_use_tls"`
}

// SendTestEmail 发送测试邮件
// POST /api/v1/admin/settings/send-test-email
func (h *SettingHandler) SendTestEmail(c *gin.Context) {
	var req SendTestEmailRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}

	if req.SMTPPort <= 0 {
		req.SMTPPort = 587
	}

	// 如果未提供密码，从数据库获取已保存的密码
	password := req.SMTPPassword
	if password == "" {
		savedConfig, err := h.emailService.GetSMTPConfig(c.Request.Context())
		if err == nil && savedConfig != nil {
			password = savedConfig.Password
		}
	}

	config := &service.SMTPConfig{
		Host:     req.SMTPHost,
		Port:     req.SMTPPort,
		Username: req.SMTPUsername,
		Password: password,
		From:     req.SMTPFrom,
		FromName: req.SMTPFromName,
		UseTLS:   req.SMTPUseTLS,
	}

	siteName := h.settingService.GetSiteName(c.Request.Context())
	subject := "[" + siteName + "] Test Email"
	body := `
<!DOCTYPE html>
<html>
<head>
    <meta charset="UTF-8">
    <style>
        body { font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif; background-color: #f5f5f5; margin: 0; padding: 20px; }
        .container { max-width: 600px; margin: 0 auto; background-color: #ffffff; border-radius: 8px; overflow: hidden; box-shadow: 0 2px 8px rgba(0,0,0,0.1); }
        .header { background: linear-gradient(135deg, #667eea 0%, #764ba2 100%); color: white; padding: 30px; text-align: center; }
        .content { padding: 40px 30px; text-align: center; }
        .success { color: #10b981; font-size: 48px; margin-bottom: 20px; }
        .footer { background-color: #f8f9fa; padding: 20px; text-align: center; color: #999; font-size: 12px; }
    </style>
</head>
<body>
    <div class="container">
        <div class="header">
            <h1>` + siteName + `</h1>
        </div>
        <div class="content">
            <div class="success">✓</div>
            <h2>Email Configuration Successful!</h2>
            <p>This is a test email to verify your SMTP settings are working correctly.</p>
        </div>
        <div class="footer">
            <p>This is an automated test message.</p>
        </div>
    </div>
</body>
</html>
`

	if err := h.emailService.SendEmailWithConfig(config, req.Email, subject, body); err != nil {
		response.BadRequest(c, "Failed to send test email: "+err.Error())
		return
	}

	response.Success(c, gin.H{"message": "Test email sent successfully"})
}

// GetAdminAPIKey 获取管理员 API Key 状态
// GET /api/v1/admin/settings/admin-api-key
func (h *SettingHandler) GetAdminAPIKey(c *gin.Context) {
	maskedKey, exists, err := h.settingService.GetAdminAPIKeyStatus(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	response.Success(c, gin.H{
		"exists":     exists,
		"masked_key": maskedKey,
	})
}

// RegenerateAdminAPIKey 生成/重新生成管理员 API Key
// POST /api/v1/admin/settings/admin-api-key/regenerate
func (h *SettingHandler) RegenerateAdminAPIKey(c *gin.Context) {
	key, err := h.settingService.GenerateAdminAPIKey(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	response.Success(c, gin.H{
		"key": key, // 完整 key 只在生成时返回一次
	})
}

// DeleteAdminAPIKey 删除管理员 API Key
// DELETE /api/v1/admin/settings/admin-api-key
func (h *SettingHandler) DeleteAdminAPIKey(c *gin.Context) {
	if err := h.settingService.DeleteAdminAPIKey(c.Request.Context()); err != nil {
		response.ErrorFrom(c, err)
		return
	}

	response.Success(c, gin.H{"message": "Admin API key deleted"})
}

// GetOverloadCooldownSettings 获取529过载冷却配置
// GET /api/v1/admin/settings/overload-cooldown
func (h *SettingHandler) GetOverloadCooldownSettings(c *gin.Context) {
	settings, err := h.settingService.GetOverloadCooldownSettings(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	response.Success(c, dto.OverloadCooldownSettings{
		Enabled:         settings.Enabled,
		CooldownMinutes: settings.CooldownMinutes,
	})
}

// UpdateOverloadCooldownSettingsRequest 更新529过载冷却配置请求
type UpdateOverloadCooldownSettingsRequest struct {
	Enabled         bool `json:"enabled"`
	CooldownMinutes int  `json:"cooldown_minutes"`
}

// UpdateOverloadCooldownSettings 更新529过载冷却配置
// PUT /api/v1/admin/settings/overload-cooldown
func (h *SettingHandler) UpdateOverloadCooldownSettings(c *gin.Context) {
	var req UpdateOverloadCooldownSettingsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}

	settings := &service.OverloadCooldownSettings{
		Enabled:         req.Enabled,
		CooldownMinutes: req.CooldownMinutes,
	}

	if err := h.settingService.SetOverloadCooldownSettings(c.Request.Context(), settings); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	updatedSettings, err := h.settingService.GetOverloadCooldownSettings(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	response.Success(c, dto.OverloadCooldownSettings{
		Enabled:         updatedSettings.Enabled,
		CooldownMinutes: updatedSettings.CooldownMinutes,
	})
}

// GetStreamTimeoutSettings 获取流超时处理配置
// GET /api/v1/admin/settings/stream-timeout
func (h *SettingHandler) GetStreamTimeoutSettings(c *gin.Context) {
	settings, err := h.settingService.GetStreamTimeoutSettings(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	response.Success(c, dto.StreamTimeoutSettings{
		Enabled:                settings.Enabled,
		Action:                 settings.Action,
		TempUnschedMinutes:     settings.TempUnschedMinutes,
		ThresholdCount:         settings.ThresholdCount,
		ThresholdWindowMinutes: settings.ThresholdWindowMinutes,
	})
}

func toSoraS3SettingsDTO(settings *service.SoraS3Settings) dto.SoraS3Settings {
	if settings == nil {
		return dto.SoraS3Settings{}
	}
	return dto.SoraS3Settings{
		Enabled:                   settings.Enabled,
		Endpoint:                  settings.Endpoint,
		Region:                    settings.Region,
		Bucket:                    settings.Bucket,
		AccessKeyID:               settings.AccessKeyID,
		SecretAccessKeyConfigured: settings.SecretAccessKeyConfigured,
		Prefix:                    settings.Prefix,
		ForcePathStyle:            settings.ForcePathStyle,
		CDNURL:                    settings.CDNURL,
		DefaultStorageQuotaBytes:  settings.DefaultStorageQuotaBytes,
	}
}

func toSoraS3ProfileDTO(profile service.SoraS3Profile) dto.SoraS3Profile {
	return dto.SoraS3Profile{
		ProfileID:                 profile.ProfileID,
		Name:                      profile.Name,
		IsActive:                  profile.IsActive,
		Provider:                  profile.GetProvider(),
		AccessMode:                profile.AccessMode,
		Enabled:                   profile.Enabled,
		Endpoint:                  profile.Endpoint,
		Region:                    profile.Region,
		Bucket:                    profile.Bucket,
		AccessKeyID:               profile.AccessKeyID,
		SecretAccessKeyConfigured: profile.SecretAccessKeyConfigured,
		Prefix:                    profile.Prefix,
		ForcePathStyle:            profile.ForcePathStyle,
		CDNURL:                    profile.CDNURL,
		DefaultStorageQuotaBytes:  profile.DefaultStorageQuotaBytes,
		UpdatedAt:                 profile.UpdatedAt,
		// Google Drive 专属
		AuthType:                 profile.AuthType,
		ClientID:                 profile.ClientID,
		ClientSecretConfigured:   profile.ClientSecretConfigured,
		RefreshTokenConfigured:   profile.RefreshTokenConfigured,
		ServiceAccountConfigured: profile.ServiceAccountConfigured,
		FolderID:                 profile.FolderID,
	}
}

func validateSoraS3RequiredWhenEnabled(enabled bool, endpoint, bucket, accessKeyID, secretAccessKey string, hasStoredSecret bool) error {
	if !enabled {
		return nil
	}
	if strings.TrimSpace(endpoint) == "" {
		return fmt.Errorf("S3 Endpoint is required when enabled")
	}
	if strings.TrimSpace(bucket) == "" {
		return fmt.Errorf("S3 Bucket is required when enabled")
	}
	if strings.TrimSpace(accessKeyID) == "" {
		return fmt.Errorf("S3 Access Key ID is required when enabled")
	}
	if strings.TrimSpace(secretAccessKey) != "" || hasStoredSecret {
		return nil
	}
	return fmt.Errorf("S3 Secret Access Key is required when enabled")
}

func findSoraS3ProfileByID(items []service.SoraS3Profile, profileID string) *service.SoraS3Profile {
	for idx := range items {
		if items[idx].ProfileID == profileID {
			return &items[idx]
		}
	}
	return nil
}

// GetSoraS3Settings 获取 Sora S3 存储配置（兼容旧单配置接口）
// GET /api/v1/admin/settings/sora-s3
func (h *SettingHandler) GetSoraS3Settings(c *gin.Context) {
	settings, err := h.settingService.GetSoraS3Settings(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, toSoraS3SettingsDTO(settings))
}

// ListSoraS3Profiles 获取 Sora S3 多配置
// GET /api/v1/admin/settings/sora-s3/profiles
func (h *SettingHandler) ListSoraS3Profiles(c *gin.Context) {
	result, err := h.settingService.ListSoraS3Profiles(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	items := make([]dto.SoraS3Profile, 0, len(result.Items))
	for idx := range result.Items {
		items = append(items, toSoraS3ProfileDTO(result.Items[idx]))
	}
	response.Success(c, dto.ListSoraS3ProfilesResponse{
		ActiveProfileID: result.ActiveProfileID,
		Items:           items,
	})
}

// UpdateSoraS3SettingsRequest 更新/测试 Sora S3 配置请求（兼容旧接口）
type UpdateSoraS3SettingsRequest struct {
	ProfileID                string `json:"profile_id"`
	Enabled                  bool   `json:"enabled"`
	Endpoint                 string `json:"endpoint"`
	Region                   string `json:"region"`
	Bucket                   string `json:"bucket"`
	AccessKeyID              string `json:"access_key_id"`
	SecretAccessKey          string `json:"secret_access_key"`
	Prefix                   string `json:"prefix"`
	ForcePathStyle           bool   `json:"force_path_style"`
	CDNURL                   string `json:"cdn_url"`
	DefaultStorageQuotaBytes int64  `json:"default_storage_quota_bytes"`
}

type CreateSoraS3ProfileRequest struct {
	ProfileID                string `json:"profile_id"`
	Name                     string `json:"name"`
	SetActive                bool   `json:"set_active"`
	Provider                 string `json:"provider"`    // "s3" / "gdrive"
	AccessMode               string `json:"access_mode"` // "direct" / "proxy"
	Enabled                  bool   `json:"enabled"`
	Endpoint                 string `json:"endpoint"`
	Region                   string `json:"region"`
	Bucket                   string `json:"bucket"`
	AccessKeyID              string `json:"access_key_id"`
	SecretAccessKey          string `json:"secret_access_key"`
	Prefix                   string `json:"prefix"`
	ForcePathStyle           bool   `json:"force_path_style"`
	CDNURL                   string `json:"cdn_url"`
	DefaultStorageQuotaBytes int64  `json:"default_storage_quota_bytes"`
	// Google Drive 专属
	AuthType           string `json:"auth_type,omitempty"`
	ClientID           string `json:"client_id,omitempty"`
	ClientSecret       string `json:"client_secret,omitempty"`
	RefreshToken       string `json:"refresh_token,omitempty"`
	ServiceAccountJSON string `json:"service_account_json,omitempty"`
	FolderID           string `json:"folder_id,omitempty"`
}

type UpdateSoraS3ProfileRequest struct {
	Name                     string `json:"name"`
	Provider                 string `json:"provider"`
	AccessMode               string `json:"access_mode"`
	Enabled                  bool   `json:"enabled"`
	Endpoint                 string `json:"endpoint"`
	Region                   string `json:"region"`
	Bucket                   string `json:"bucket"`
	AccessKeyID              string `json:"access_key_id"`
	SecretAccessKey          string `json:"secret_access_key"`
	Prefix                   string `json:"prefix"`
	ForcePathStyle           bool   `json:"force_path_style"`
	CDNURL                   string `json:"cdn_url"`
	DefaultStorageQuotaBytes int64  `json:"default_storage_quota_bytes"`
	// Google Drive 专属
	AuthType           string `json:"auth_type,omitempty"`
	ClientID           string `json:"client_id,omitempty"`
	ClientSecret       string `json:"client_secret,omitempty"`
	RefreshToken       string `json:"refresh_token,omitempty"`
	ServiceAccountJSON string `json:"service_account_json,omitempty"`
	FolderID           string `json:"folder_id,omitempty"`
}

// CreateSoraS3Profile 创建 Sora S3 配置
// POST /api/v1/admin/settings/sora-s3/profiles
func (h *SettingHandler) CreateSoraS3Profile(c *gin.Context) {
	var req CreateSoraS3ProfileRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}

	if req.DefaultStorageQuotaBytes < 0 {
		req.DefaultStorageQuotaBytes = 0
	}
	if strings.TrimSpace(req.Name) == "" {
		response.BadRequest(c, "Name is required")
		return
	}
	if strings.TrimSpace(req.ProfileID) == "" {
		response.BadRequest(c, "Profile ID is required")
		return
	}
	// S3 专属字段验证：仅当 provider 为 s3（或未指定）时校验
	provider := req.Provider
	if provider == "" {
		provider = "s3"
	}
	if provider == "s3" {
		if err := validateSoraS3RequiredWhenEnabled(req.Enabled, req.Endpoint, req.Bucket, req.AccessKeyID, req.SecretAccessKey, false); err != nil {
			response.BadRequest(c, err.Error())
			return
		}
	}

	created, err := h.settingService.CreateSoraS3Profile(c.Request.Context(), &service.SoraS3Profile{
		ProfileID:                req.ProfileID,
		Name:                     req.Name,
		Provider:                 req.Provider,
		AccessMode:               req.AccessMode,
		Enabled:                  req.Enabled,
		Endpoint:                 req.Endpoint,
		Region:                   req.Region,
		Bucket:                   req.Bucket,
		AccessKeyID:              req.AccessKeyID,
		SecretAccessKey:          req.SecretAccessKey,
		Prefix:                   req.Prefix,
		ForcePathStyle:           req.ForcePathStyle,
		CDNURL:                   req.CDNURL,
		DefaultStorageQuotaBytes: req.DefaultStorageQuotaBytes,
		// Google Drive 专属
		AuthType:           req.AuthType,
		ClientID:           req.ClientID,
		ClientSecret:       req.ClientSecret,
		RefreshToken:       req.RefreshToken,
		ServiceAccountJSON: req.ServiceAccountJSON,
		FolderID:           req.FolderID,
	}, req.SetActive)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	response.Success(c, toSoraS3ProfileDTO(*created))
}

// UpdateSoraS3Profile 更新 Sora S3 配置
// PUT /api/v1/admin/settings/sora-s3/profiles/:profile_id
func (h *SettingHandler) UpdateSoraS3Profile(c *gin.Context) {
	profileID := strings.TrimSpace(c.Param("profile_id"))
	if profileID == "" {
		response.BadRequest(c, "Profile ID is required")
		return
	}

	var req UpdateSoraS3ProfileRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}

	if req.DefaultStorageQuotaBytes < 0 {
		req.DefaultStorageQuotaBytes = 0
	}
	if strings.TrimSpace(req.Name) == "" {
		response.BadRequest(c, "Name is required")
		return
	}

	existingList, err := h.settingService.ListSoraS3Profiles(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	existing := findSoraS3ProfileByID(existingList.Items, profileID)
	if existing == nil {
		response.ErrorFrom(c, service.ErrSoraS3ProfileNotFound)
		return
	}
	// S3 专属字段验证
	provider := req.Provider
	if provider == "" && existing != nil {
		provider = existing.GetProvider()
	}
	if provider == "" {
		provider = "s3"
	}
	if provider == "s3" {
		if err := validateSoraS3RequiredWhenEnabled(req.Enabled, req.Endpoint, req.Bucket, req.AccessKeyID, req.SecretAccessKey, existing.SecretAccessKeyConfigured); err != nil {
			response.BadRequest(c, err.Error())
			return
		}
	}

	updated, updateErr := h.settingService.UpdateSoraS3Profile(c.Request.Context(), profileID, &service.SoraS3Profile{
		Name:                     req.Name,
		Provider:                 req.Provider,
		AccessMode:               req.AccessMode,
		Enabled:                  req.Enabled,
		Endpoint:                 req.Endpoint,
		Region:                   req.Region,
		Bucket:                   req.Bucket,
		AccessKeyID:              req.AccessKeyID,
		SecretAccessKey:          req.SecretAccessKey,
		Prefix:                   req.Prefix,
		ForcePathStyle:           req.ForcePathStyle,
		CDNURL:                   req.CDNURL,
		DefaultStorageQuotaBytes: req.DefaultStorageQuotaBytes,
		// Google Drive 专属
		AuthType:           req.AuthType,
		ClientID:           req.ClientID,
		ClientSecret:       req.ClientSecret,
		RefreshToken:       req.RefreshToken,
		ServiceAccountJSON: req.ServiceAccountJSON,
		FolderID:           req.FolderID,
	})
	if updateErr != nil {
		response.ErrorFrom(c, updateErr)
		return
	}

	response.Success(c, toSoraS3ProfileDTO(*updated))
}

// DeleteSoraS3Profile 删除 Sora S3 配置
// DELETE /api/v1/admin/settings/sora-s3/profiles/:profile_id
func (h *SettingHandler) DeleteSoraS3Profile(c *gin.Context) {
	profileID := strings.TrimSpace(c.Param("profile_id"))
	if profileID == "" {
		response.BadRequest(c, "Profile ID is required")
		return
	}
	if err := h.settingService.DeleteSoraS3Profile(c.Request.Context(), profileID); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"deleted": true})
}

// SetActiveSoraS3Profile 切换激活 Sora S3 配置
// POST /api/v1/admin/settings/sora-s3/profiles/:profile_id/activate
func (h *SettingHandler) SetActiveSoraS3Profile(c *gin.Context) {
	profileID := strings.TrimSpace(c.Param("profile_id"))
	if profileID == "" {
		response.BadRequest(c, "Profile ID is required")
		return
	}
	active, err := h.settingService.SetActiveSoraS3Profile(c.Request.Context(), profileID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, toSoraS3ProfileDTO(*active))
}

// UpdateSoraS3Settings 更新 Sora S3 存储配置（兼容旧单配置接口）
// PUT /api/v1/admin/settings/sora-s3
func (h *SettingHandler) UpdateSoraS3Settings(c *gin.Context) {
	var req UpdateSoraS3SettingsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}

	existing, err := h.settingService.GetSoraS3Settings(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	if req.DefaultStorageQuotaBytes < 0 {
		req.DefaultStorageQuotaBytes = 0
	}
	if err := validateSoraS3RequiredWhenEnabled(req.Enabled, req.Endpoint, req.Bucket, req.AccessKeyID, req.SecretAccessKey, existing.SecretAccessKeyConfigured); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	settings := &service.SoraS3Settings{
		Enabled:                  req.Enabled,
		Endpoint:                 req.Endpoint,
		Region:                   req.Region,
		Bucket:                   req.Bucket,
		AccessKeyID:              req.AccessKeyID,
		SecretAccessKey:          req.SecretAccessKey,
		Prefix:                   req.Prefix,
		ForcePathStyle:           req.ForcePathStyle,
		CDNURL:                   req.CDNURL,
		DefaultStorageQuotaBytes: req.DefaultStorageQuotaBytes,
	}
	if err := h.settingService.SetSoraS3Settings(c.Request.Context(), settings); err != nil {
		response.ErrorFrom(c, err)
		return
	}

	updatedSettings, err := h.settingService.GetSoraS3Settings(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, toSoraS3SettingsDTO(updatedSettings))
}

// TestSoraS3Connection 测试 Sora S3 连接（HeadBucket）
// POST /api/v1/admin/settings/sora-s3/test
func (h *SettingHandler) TestSoraS3Connection(c *gin.Context) {
	if h.soraS3Storage == nil {
		response.Error(c, 500, "S3 存储服务未初始化")
		return
	}

	var req UpdateSoraS3SettingsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	if !req.Enabled {
		response.BadRequest(c, "S3 未启用，无法测试连接")
		return
	}

	if req.SecretAccessKey == "" {
		if req.ProfileID != "" {
			profiles, err := h.settingService.ListSoraS3Profiles(c.Request.Context())
			if err == nil {
				profile := findSoraS3ProfileByID(profiles.Items, req.ProfileID)
				if profile != nil {
					req.SecretAccessKey = profile.SecretAccessKey
				}
			}
		}
		if req.SecretAccessKey == "" {
			existing, err := h.settingService.GetSoraS3Settings(c.Request.Context())
			if err == nil {
				req.SecretAccessKey = existing.SecretAccessKey
			}
		}
	}

	testCfg := &service.SoraS3Settings{
		Enabled:         true,
		Endpoint:        req.Endpoint,
		Region:          req.Region,
		Bucket:          req.Bucket,
		AccessKeyID:     req.AccessKeyID,
		SecretAccessKey: req.SecretAccessKey,
		Prefix:          req.Prefix,
		ForcePathStyle:  req.ForcePathStyle,
		CDNURL:          req.CDNURL,
	}
	if err := h.soraS3Storage.TestConnectionWithSettings(c.Request.Context(), testCfg); err != nil {
		response.Error(c, 400, "S3 连接测试失败: "+err.Error())
		return
	}
	response.Success(c, gin.H{"message": "S3 连接成功"})
}

// GetRectifierSettings 获取请求整流器配置
// GET /api/v1/admin/settings/rectifier
func (h *SettingHandler) GetRectifierSettings(c *gin.Context) {
	settings, err := h.settingService.GetRectifierSettings(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	response.Success(c, dto.RectifierSettings{
		Enabled:                  settings.Enabled,
		ThinkingSignatureEnabled: settings.ThinkingSignatureEnabled,
		ThinkingBudgetEnabled:    settings.ThinkingBudgetEnabled,
	})
}

// UpdateRectifierSettingsRequest 更新整流器配置请求
type UpdateRectifierSettingsRequest struct {
	Enabled                  bool `json:"enabled"`
	ThinkingSignatureEnabled bool `json:"thinking_signature_enabled"`
	ThinkingBudgetEnabled    bool `json:"thinking_budget_enabled"`
}

// UpdateRectifierSettings 更新请求整流器配置
// PUT /api/v1/admin/settings/rectifier
func (h *SettingHandler) UpdateRectifierSettings(c *gin.Context) {
	var req UpdateRectifierSettingsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}

	settings := &service.RectifierSettings{
		Enabled:                  req.Enabled,
		ThinkingSignatureEnabled: req.ThinkingSignatureEnabled,
		ThinkingBudgetEnabled:    req.ThinkingBudgetEnabled,
	}

	if err := h.settingService.SetRectifierSettings(c.Request.Context(), settings); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	// 重新获取设置返回
	updatedSettings, err := h.settingService.GetRectifierSettings(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	response.Success(c, dto.RectifierSettings{
		Enabled:                  updatedSettings.Enabled,
		ThinkingSignatureEnabled: updatedSettings.ThinkingSignatureEnabled,
		ThinkingBudgetEnabled:    updatedSettings.ThinkingBudgetEnabled,
	})
}

// GetBetaPolicySettings 获取 Beta 策略配置
// GET /api/v1/admin/settings/beta-policy
func (h *SettingHandler) GetBetaPolicySettings(c *gin.Context) {
	settings, err := h.settingService.GetBetaPolicySettings(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	rules := make([]dto.BetaPolicyRule, len(settings.Rules))
	for i, r := range settings.Rules {
		rules[i] = dto.BetaPolicyRule(r)
	}
	response.Success(c, dto.BetaPolicySettings{Rules: rules})
}

// UpdateBetaPolicySettingsRequest 更新 Beta 策略配置请求
type UpdateBetaPolicySettingsRequest struct {
	Rules []dto.BetaPolicyRule `json:"rules"`
}

// UpdateBetaPolicySettings 更新 Beta 策略配置
// PUT /api/v1/admin/settings/beta-policy
func (h *SettingHandler) UpdateBetaPolicySettings(c *gin.Context) {
	var req UpdateBetaPolicySettingsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}

	rules := make([]service.BetaPolicyRule, len(req.Rules))
	for i, r := range req.Rules {
		rules[i] = service.BetaPolicyRule(r)
	}

	settings := &service.BetaPolicySettings{Rules: rules}
	if err := h.settingService.SetBetaPolicySettings(c.Request.Context(), settings); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	// Re-fetch to return updated settings
	updated, err := h.settingService.GetBetaPolicySettings(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	outRules := make([]dto.BetaPolicyRule, len(updated.Rules))
	for i, r := range updated.Rules {
		outRules[i] = dto.BetaPolicyRule(r)
	}
	response.Success(c, dto.BetaPolicySettings{Rules: outRules})
}

// UpdateStreamTimeoutSettingsRequest 更新流超时配置请求
type UpdateStreamTimeoutSettingsRequest struct {
	Enabled                bool   `json:"enabled"`
	Action                 string `json:"action"`
	TempUnschedMinutes     int    `json:"temp_unsched_minutes"`
	ThresholdCount         int    `json:"threshold_count"`
	ThresholdWindowMinutes int    `json:"threshold_window_minutes"`
}

// UpdateStreamTimeoutSettings 更新流超时处理配置
// PUT /api/v1/admin/settings/stream-timeout
func (h *SettingHandler) UpdateStreamTimeoutSettings(c *gin.Context) {
	var req UpdateStreamTimeoutSettingsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}

	settings := &service.StreamTimeoutSettings{
		Enabled:                req.Enabled,
		Action:                 req.Action,
		TempUnschedMinutes:     req.TempUnschedMinutes,
		ThresholdCount:         req.ThresholdCount,
		ThresholdWindowMinutes: req.ThresholdWindowMinutes,
	}

	if err := h.settingService.SetStreamTimeoutSettings(c.Request.Context(), settings); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	// 重新获取设置返回
	updatedSettings, err := h.settingService.GetStreamTimeoutSettings(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	response.Success(c, dto.StreamTimeoutSettings{
		Enabled:                updatedSettings.Enabled,
		Action:                 updatedSettings.Action,
		TempUnschedMinutes:     updatedSettings.TempUnschedMinutes,
		ThresholdCount:         updatedSettings.ThresholdCount,
		ThresholdWindowMinutes: updatedSettings.ThresholdWindowMinutes,
	})
}

// GetGDriveQuota 获取 Google Drive 配额信息。
// GET /api/v1/admin/settings/sora-storage/gdrive-quota
func (h *SettingHandler) GetGDriveQuota(c *gin.Context) {
	if h.soraGDriveStorage == nil {
		response.Error(c, http.StatusServiceUnavailable, "GDrive storage not configured")
		return
	}
	quota, err := h.soraGDriveStorage.GetQuotaInfo(c.Request.Context())
	if err != nil {
		response.Error(c, http.StatusInternalServerError, fmt.Sprintf("failed to get GDrive quota: %v", err))
		return
	}
	response.Success(c, quota)
}

// GetStorageVideoStats 获取各存储类型的视频统计信息。
// GET /api/v1/admin/settings/sora-storage/video-stats
func (h *SettingHandler) GetStorageVideoStats(c *gin.Context) {
	if h.soraGenerationService == nil {
		response.Error(c, http.StatusServiceUnavailable, "generation service not configured")
		return
	}

	storageTypes := []string{service.SoraStorageTypeS3, service.SoraStorageTypeGDrive}
	result := make(map[string]*service.StorageVideoStats, len(storageTypes))

	for _, st := range storageTypes {
		completed, inProgress, err := h.soraGenerationService.CountByStorageType(c.Request.Context(), st)
		if err != nil {
			log.Printf("[SettingHandler] CountByStorageType(%s) error: %v", st, err)
			continue
		}
		result[st] = &service.StorageVideoStats{
			Completed:  completed,
			InProgress: inProgress,
		}
	}

	response.Success(c, result)
}
