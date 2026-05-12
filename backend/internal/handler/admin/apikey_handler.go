package admin

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/handler/dto"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

const (
	managedKeyMarker       = "[managed-key]"
	managedKeySearchTerm   = "managed-key"
	managedKeyEmailDomain  = "managed.local"
	defaultManagedKeyFunds = 1000
)

// ManagedKeyAPIKeyService is the API key service surface used by managed-key flows.
type ManagedKeyAPIKeyService interface {
	Create(ctx context.Context, userID int64, req service.CreateAPIKeyRequest) (*service.APIKey, error)
	ResetIPLock(ctx context.Context, apiKeyID int64) error
	GetIPLock(ctx context.Context, apiKeyID int64) (string, error)
}

// AdminAPIKeyHandler handles admin API key management
type AdminAPIKeyHandler struct {
	adminService  service.AdminService
	apiKeyService ManagedKeyAPIKeyService
}

// NewAdminAPIKeyHandler creates a new admin API key handler
func NewAdminAPIKeyHandler(adminService service.AdminService, apiKeyService ManagedKeyAPIKeyService) *AdminAPIKeyHandler {
	return &AdminAPIKeyHandler{
		adminService:  adminService,
		apiKeyService: apiKeyService,
	}
}

// AdminUpdateAPIKeyGroupRequest represents the request to update an API key.
type AdminUpdateAPIKeyGroupRequest struct {
	GroupID             *int64 `json:"group_id"`               // nil=不修改, 0=解绑, >0=绑定到目标分组
	ResetRateLimitUsage *bool  `json:"reset_rate_limit_usage"` // true=重置 5h/1d/7d 限速用量
}

// CreateManagedKeyRequest creates an internal managed user plus a customer-facing API key.
type CreateManagedKeyRequest struct {
	CustomerName  string   `json:"customer_name" binding:"required"`
	Contact       string   `json:"contact"`
	KeyName       string   `json:"key_name"`
	GroupID       *int64   `json:"group_id"`
	Balance       *float64 `json:"balance"`
	Concurrency   int      `json:"concurrency"`
	RPMLimit      int      `json:"rpm_limit"`
	Quota         float64  `json:"quota"`
	ExpiresInDays *int     `json:"expires_in_days"`
	CustomKey     *string  `json:"custom_key"`
	IPWhitelist   []string `json:"ip_whitelist"`
	IPBlacklist   []string `json:"ip_blacklist"`
	RateLimit5h   float64  `json:"rate_limit_5h"`
	RateLimit1d   float64  `json:"rate_limit_1d"`
	RateLimit7d   float64  `json:"rate_limit_7d"`
	RateLimit1mo  float64  `json:"rate_limit_1mo"`
	IPLockMode    string   `json:"ip_lock_mode"`
	LimitAction   string   `json:"limit_action"`
	Notes         string   `json:"notes"`
}

type ManagedKey struct {
	User   *dto.AdminUser    `json:"user"`
	APIKey *dto.APIKey       `json:"api_key"`
	IPLock *ManagedKeyIPLock `json:"ip_lock,omitempty"`
}

type ManagedKeyIPLock struct {
	Mode     string `json:"mode"`
	LockedIP string `json:"locked_ip,omitempty"`
}

type ManagedKeyDelivery struct {
	APIKey              string `json:"api_key"`
	AuthorizationHeader string `json:"authorization_header"`
	BaseURL             string `json:"base_url"`
	OpenAIBaseURL       string `json:"openai_base_url"`
	ClaudeBaseURL       string `json:"claude_base_url"`
	GeminiBaseURL       string `json:"gemini_base_url"`
}

type ManagedKeyResponse struct {
	User     *dto.AdminUser     `json:"user"`
	APIKey   *dto.APIKey        `json:"api_key"`
	Delivery ManagedKeyDelivery `json:"delivery"`
}

// UpdateGroup handles updating an API key's admin-managed fields.
// PUT /api/v1/admin/api-keys/:id
func (h *AdminAPIKeyHandler) UpdateGroup(c *gin.Context) {
	keyID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid API key ID")
		return
	}

	var req AdminUpdateAPIKeyGroupRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}

	var resetKey *service.APIKey
	if req.ResetRateLimitUsage != nil && *req.ResetRateLimitUsage {
		resetKey, err = h.adminService.AdminResetAPIKeyRateLimitUsage(c.Request.Context(), keyID)
		if err != nil {
			response.ErrorFrom(c, err)
			return
		}
	}

	result, err := h.adminService.AdminUpdateAPIKeyGroupID(c.Request.Context(), keyID, req.GroupID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	if resetKey != nil && req.GroupID == nil {
		result.APIKey = resetKey
	}

	resp := struct {
		APIKey                 *dto.APIKey `json:"api_key"`
		AutoGrantedGroupAccess bool        `json:"auto_granted_group_access"`
		GrantedGroupID         *int64      `json:"granted_group_id,omitempty"`
		GrantedGroupName       string      `json:"granted_group_name,omitempty"`
	}{
		APIKey:                 dto.APIKeyFromService(result.APIKey),
		AutoGrantedGroupAccess: result.AutoGrantedGroupAccess,
		GrantedGroupID:         result.GrantedGroupID,
		GrantedGroupName:       result.GrantedGroupName,
	}
	response.Success(c, resp)
}

// ListManagedKeys lists managed users with their primary API key.
// GET /api/v1/admin/managed-keys
func (h *AdminAPIKeyHandler) ListManagedKeys(c *gin.Context) {
	page, pageSize := response.ParsePagination(c)
	users, total, err := h.adminService.ListUsers(
		c.Request.Context(),
		page,
		pageSize,
		service.UserListFilters{Role: service.RoleUser, CustomerType: service.CustomerTypeManaged},
		"created_at",
		"desc",
	)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	items := make([]ManagedKey, 0, len(users))
	for i := range users {
		user := users[i]
		if !isManagedUser(&user) {
			continue
		}
		keys, _, keyErr := h.adminService.GetUserAPIKeys(c.Request.Context(), user.ID, 1, 1, "created_at", "desc")
		if keyErr != nil {
			response.ErrorFrom(c, keyErr)
			return
		}
		var keyDTO *dto.APIKey
		var ipLock *ManagedKeyIPLock
		if len(keys) > 0 {
			keyDTO = dto.APIKeyFromService(&keys[0])
			ipLock = h.managedKeyIPLock(c.Request.Context(), &keys[0])
		}
		items = append(items, ManagedKey{
			User:   dto.UserFromServiceAdmin(&user),
			APIKey: keyDTO,
			IPLock: ipLock,
		})
	}

	response.Paginated(c, items, total, page, pageSize)
}

// CreateManagedKey creates an internal managed user and a customer-facing API key.
// POST /api/v1/admin/managed-keys
func (h *AdminAPIKeyHandler) CreateManagedKey(c *gin.Context) {
	if h.apiKeyService == nil {
		response.InternalError(c, "API key service is unavailable")
		return
	}

	var req CreateManagedKeyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}

	normalized, err := normalizeManagedKeyRequest(req)
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	password, err := randomHex(24)
	if err != nil {
		response.InternalError(c, "failed to generate managed user password")
		return
	}
	suffix, err := randomHex(6)
	if err != nil {
		response.InternalError(c, "failed to generate managed user email")
		return
	}

	email := fmt.Sprintf("%s-%s@%s", managedKeySearchTerm, suffix, managedKeyEmailDomain)
	allowedGroups := []int64(nil)
	if normalized.GroupID != nil {
		allowedGroups = []int64{*normalized.GroupID}
	}

	user, err := h.adminService.CreateUser(c.Request.Context(), &service.CreateUserInput{
		Email:         email,
		Password:      password,
		Username:      normalized.CustomerName,
		Notes:         buildManagedKeyNotes(normalized),
		CustomerType:  service.CustomerTypeManaged,
		Balance:       normalized.balanceValue,
		Concurrency:   normalized.Concurrency,
		RPMLimit:      normalized.RPMLimit,
		AllowedGroups: allowedGroups,
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	apiKey, err := h.apiKeyService.Create(c.Request.Context(), user.ID, service.CreateAPIKeyRequest{
		Name:          normalized.KeyName,
		GroupID:       normalized.GroupID,
		CustomKey:     normalized.CustomKey,
		IPWhitelist:   normalized.IPWhitelist,
		IPBlacklist:   normalized.IPBlacklist,
		Quota:         normalized.Quota,
		ExpiresInDays: normalized.ExpiresInDays,
		RateLimit5h:   normalized.RateLimit5h,
		RateLimit1d:   normalized.RateLimit1d,
		RateLimit7d:   normalized.RateLimit7d,
		RateLimit1mo:  normalized.RateLimit1mo,
		IPLockMode:    normalized.IPLockMode,
		LimitAction:   normalized.LimitAction,
	})
	if err != nil {
		_ = h.adminService.DeleteUser(c.Request.Context(), user.ID)
		response.ErrorFrom(c, err)
		return
	}

	response.Created(c, ManagedKeyResponse{
		User:     dto.UserFromServiceAdmin(user),
		APIKey:   dto.APIKeyFromService(apiKey),
		Delivery: buildManagedKeyDelivery(c, apiKey.Key),
	})
}

// GetManagedKeyDelivery returns customer-facing delivery details for an existing managed key.
// GET /api/v1/admin/managed-keys/:id/delivery
func (h *AdminAPIKeyHandler) GetManagedKeyDelivery(c *gin.Context) {
	user, apiKey, ok := h.loadManagedUserPrimaryKey(c)
	if !ok {
		return
	}
	response.Success(c, ManagedKeyResponse{
		User:     dto.UserFromServiceAdmin(user),
		APIKey:   dto.APIKeyFromService(apiKey),
		Delivery: buildManagedKeyDelivery(c, apiKey.Key),
	})
}

// ResetManagedKeyIPLock clears the dynamic IP lock for a managed key.
// POST /api/v1/admin/managed-keys/:id/reset-ip-lock
func (h *AdminAPIKeyHandler) ResetManagedKeyIPLock(c *gin.Context) {
	if h.apiKeyService == nil {
		response.InternalError(c, "API key service is unavailable")
		return
	}
	_, apiKey, ok := h.loadManagedUserPrimaryKey(c)
	if !ok {
		return
	}
	if err := h.apiKeyService.ResetIPLock(c.Request.Context(), apiKey.ID); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"message": "IP lock reset"})
}

type normalizedManagedKeyRequest struct {
	CreateManagedKeyRequest
	balanceValue float64
}

func normalizeManagedKeyRequest(req CreateManagedKeyRequest) (*normalizedManagedKeyRequest, error) {
	req.CustomerName = strings.TrimSpace(req.CustomerName)
	req.Contact = strings.TrimSpace(req.Contact)
	req.KeyName = strings.TrimSpace(req.KeyName)
	req.Notes = strings.TrimSpace(req.Notes)
	req.IPLockMode = strings.TrimSpace(req.IPLockMode)
	req.LimitAction = strings.TrimSpace(req.LimitAction)
	if req.CustomerName == "" {
		return nil, fmt.Errorf("customer_name is required")
	}
	if req.KeyName == "" {
		req.KeyName = req.CustomerName + " API Key"
	}
	if req.GroupID != nil && *req.GroupID <= 0 {
		req.GroupID = nil
	}
	if req.Concurrency <= 0 {
		req.Concurrency = 1
	}
	if req.RPMLimit < 0 {
		return nil, fmt.Errorf("rpm_limit cannot be negative")
	}
	if req.Quota < 0 {
		return nil, fmt.Errorf("quota cannot be negative")
	}
	if req.RateLimit5h < 0 || req.RateLimit1d < 0 || req.RateLimit7d < 0 || req.RateLimit1mo < 0 {
		return nil, fmt.Errorf("rate limits cannot be negative")
	}
	if req.IPLockMode == "" {
		req.IPLockMode = service.IPLockModeAutoSingleIP
	}
	if req.IPLockMode != service.IPLockModeOff && req.IPLockMode != service.IPLockModeAutoSingleIP {
		return nil, fmt.Errorf("invalid ip_lock_mode")
	}
	if req.LimitAction == "" {
		req.LimitAction = service.LimitActionSoftThrottle
	}
	if req.LimitAction != service.LimitActionHardBlock && req.LimitAction != service.LimitActionSoftThrottle {
		return nil, fmt.Errorf("invalid limit_action")
	}
	if req.ExpiresInDays != nil && *req.ExpiresInDays < 0 {
		return nil, fmt.Errorf("expires_in_days cannot be negative")
	}
	if req.CustomKey != nil {
		trimmed := strings.TrimSpace(*req.CustomKey)
		if trimmed == "" {
			req.CustomKey = nil
		} else {
			req.CustomKey = &trimmed
		}
	}

	balance := float64(defaultManagedKeyFunds)
	if req.Balance != nil {
		if *req.Balance < 0 {
			return nil, fmt.Errorf("balance cannot be negative")
		}
		balance = *req.Balance
	}

	return &normalizedManagedKeyRequest{
		CreateManagedKeyRequest: req,
		balanceValue:            balance,
	}, nil
}

func buildManagedKeyNotes(req *normalizedManagedKeyRequest) string {
	lines := []string{
		managedKeyMarker,
		"customer: " + compactNoteLine(req.CustomerName),
	}
	if req.Contact != "" {
		lines = append(lines, "contact: "+compactNoteLine(req.Contact))
	}
	if req.Notes != "" {
		lines = append(lines, "", req.Notes)
	}
	return strings.Join(lines, "\n")
}

func compactNoteLine(value string) string {
	return strings.Join(strings.Fields(value), " ")
}

func buildManagedKeyDelivery(c *gin.Context, key string) ManagedKeyDelivery {
	baseURL := requestBaseURL(c)
	v1Base := strings.TrimRight(baseURL, "/") + "/v1"
	return ManagedKeyDelivery{
		APIKey:              key,
		AuthorizationHeader: "Bearer " + key,
		BaseURL:             baseURL,
		OpenAIBaseURL:       v1Base,
		ClaudeBaseURL:       v1Base,
		GeminiBaseURL:       strings.TrimRight(baseURL, "/") + "/v1beta",
	}
}

func requestBaseURL(c *gin.Context) string {
	proto := strings.TrimSpace(c.GetHeader("X-Forwarded-Proto"))
	if proto == "" {
		proto = strings.TrimSpace(c.GetHeader("X-Forwarded-Scheme"))
	}
	if proto == "" {
		proto = "http"
		if c.Request != nil && c.Request.TLS != nil {
			proto = "https"
		}
	}

	host := strings.TrimSpace(c.GetHeader("X-Forwarded-Host"))
	if host == "" && c.Request != nil {
		host = c.Request.Host
	}
	if host == "" {
		return ""
	}
	return proto + "://" + host
}

func isManagedUser(user *service.User) bool {
	if user == nil {
		return false
	}
	return user.CustomerType == service.CustomerTypeManaged
}

func (h *AdminAPIKeyHandler) loadManagedUserPrimaryKey(c *gin.Context) (*service.User, *service.APIKey, bool) {
	userID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid managed key ID")
		return nil, nil, false
	}
	user, err := h.adminService.GetUser(c.Request.Context(), userID)
	if err != nil {
		response.ErrorFrom(c, err)
		return nil, nil, false
	}
	if !isManagedUser(user) {
		response.NotFound(c, "Managed key not found")
		return nil, nil, false
	}
	keys, _, err := h.adminService.GetUserAPIKeys(c.Request.Context(), user.ID, 1, 1, "created_at", "desc")
	if err != nil {
		response.ErrorFrom(c, err)
		return nil, nil, false
	}
	if len(keys) == 0 {
		response.NotFound(c, "Managed key API key not found")
		return nil, nil, false
	}
	return user, &keys[0], true
}

func (h *AdminAPIKeyHandler) managedKeyIPLock(ctx context.Context, apiKey *service.APIKey) *ManagedKeyIPLock {
	if apiKey == nil {
		return nil
	}
	status := &ManagedKeyIPLock{Mode: apiKey.IPLockMode}
	if h.apiKeyService == nil || apiKey.IPLockMode != service.IPLockModeAutoSingleIP {
		return status
	}
	if lockedIP, err := h.apiKeyService.GetIPLock(ctx, apiKey.ID); err == nil {
		status.LockedIP = lockedIP
	}
	return status
}

func randomHex(bytesLen int) (string, error) {
	buf := make([]byte, bytesLen)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}
