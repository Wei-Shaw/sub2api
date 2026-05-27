package handler

import (
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/handler/dto"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// OpenAPIHandler 处理 /api/v1/openapi/* 路由（M2M 数据集成接口）。
type OpenAPIHandler struct {
	svc *service.OpenAPIService
}

// NewOpenAPIHandler 构造函数。
func NewOpenAPIHandler(svc *service.OpenAPIService) *OpenAPIHandler {
	return &OpenAPIHandler{svc: svc}
}

// OpenAPICreateUserRequestBody 创建用户入参（HTTP 层 DTO）。
type OpenAPICreateUserRequestBody struct {
	Email          string  `json:"email" binding:"required,email"`
	ExternalUserID string  `json:"external_user_id"`
	InitialBalance float64 `json:"initial_balance"`
	KeyName        string  `json:"key_name"`
	GroupID        *int64  `json:"group_id"`
}

// CreateUser POST /api/v1/openapi/users
func (h *OpenAPIHandler) CreateUser(c *gin.Context) {
	var req OpenAPICreateUserRequestBody
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid_request: "+err.Error())
		return
	}
	res, err := h.svc.CreateUser(c.Request.Context(), service.OpenAPICreateUserRequest{
		Email:          req.Email,
		ExternalUserID: req.ExternalUserID,
		InitialBalance: req.InitialBalance,
		KeyName:        req.KeyName,
		GroupID:        req.GroupID,
	})
	if err != nil {
		writeOpenAPIError(c, err)
		return
	}
	payload := gin.H{
		"user_id":    res.UserID,
		"email":      res.Email,
		"status":     res.Status,
		"balance":    res.Balance,
		"first_time": res.FirstTime,
	}
	if res.FirstTime {
		payload["api_key"] = res.APIKey
		payload["api_key_id"] = res.APIKeyID
	}
	response.Success(c, payload)
}

// GetUserByEmail GET /api/v1/openapi/users/:email
func (h *OpenAPIHandler) GetUserByEmail(c *gin.Context) {
	email, err := decodeEmailParam(c)
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	user, err := h.svc.GetUserByEmail(c.Request.Context(), email)
	if err != nil {
		writeOpenAPIError(c, err)
		return
	}
	response.Success(c, gin.H{
		"user_id":         user.ID,
		"email":           user.Email,
		"status":          user.Status,
		"role":            user.Role,
		"balance":         user.Balance,
		"total_recharged": user.TotalRecharged,
		"concurrency":     user.Concurrency,
		"created_at":      user.CreatedAt,
	})
}

// OpenAPIAdjustBalanceRequestBody 调整余额入参（HTTP 层 DTO）。
type OpenAPIAdjustBalanceRequestBody struct {
	ExternalOpID string  `json:"external_op_id" binding:"required"`
	OpType       string  `json:"op_type" binding:"required,oneof=set add"`
	Amount       float64 `json:"amount" binding:"gte=0"`
	Note         string  `json:"note"`
}

// AdjustBalance PATCH /api/v1/openapi/users/:email/balance
func (h *OpenAPIHandler) AdjustBalance(c *gin.Context) {
	email, err := decodeEmailParam(c)
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	var req OpenAPIAdjustBalanceRequestBody
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid_request: "+err.Error())
		return
	}
	res, err := h.svc.AdjustBalance(c.Request.Context(), email, service.OpenAPIAdjustBalanceRequest{
		ExternalOpID: req.ExternalOpID,
		OpType:       req.OpType,
		Amount:       req.Amount,
		Note:         req.Note,
		Payload: map[string]any{
			"external_op_id": req.ExternalOpID,
			"op_type":        req.OpType,
			"amount":         req.Amount,
			"note":           req.Note,
		},
	})
	if err != nil {
		writeOpenAPIError(c, err)
		return
	}
	response.Success(c, gin.H{
		"operation_id":      res.OperationID,
		"user_id":           res.UserID,
		"email":             res.Email,
		"op_type":           res.OpType,
		"amount":            res.Amount,
		"balance_before":    res.BalanceBefore,
		"balance_after":     res.BalanceAfter,
		"idempotent_replay": res.IdempotentReplay,
	})
}

// CreateKey POST /api/v1/openapi/users/:email/keys
func (h *OpenAPIHandler) CreateKey(c *gin.Context) {
	email, err := decodeEmailParam(c)
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	var req CreateAPIKeyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid_request: "+err.Error())
		return
	}
	svcReq := service.CreateAPIKeyRequest{
		Name:          req.Name,
		GroupID:       req.GroupID,
		CustomKey:     req.CustomKey,
		IPWhitelist:   req.IPWhitelist,
		IPBlacklist:   req.IPBlacklist,
		ExpiresInDays: req.ExpiresInDays,
	}
	if req.Quota != nil {
		svcReq.Quota = *req.Quota
	}
	if req.RateLimit5h != nil {
		svcReq.RateLimit5h = *req.RateLimit5h
	}
	if req.RateLimit1d != nil {
		svcReq.RateLimit1d = *req.RateLimit1d
	}
	if req.RateLimit7d != nil {
		svcReq.RateLimit7d = *req.RateLimit7d
	}
	key, err := h.svc.CreateKeyForEmail(c.Request.Context(), email, svcReq)
	if err != nil {
		writeOpenAPIError(c, err)
		return
	}
	response.Success(c, dto.APIKeyFromService(key))
}

// ListKeys GET /api/v1/openapi/users/:email/keys
func (h *OpenAPIHandler) ListKeys(c *gin.Context) {
	email, err := decodeEmailParam(c)
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	filters := service.APIKeyListFilters{
		Status: c.Query("status"),
	}
	keys, err := h.svc.ListKeysForEmail(c.Request.Context(), email, filters)
	if err != nil {
		writeOpenAPIError(c, err)
		return
	}
	out := make([]dto.APIKey, 0, len(keys))
	for i := range keys {
		out = append(out, *dto.APIKeyFromService(&keys[i]))
	}
	response.Success(c, out)
}

// UpdateKey PATCH /api/v1/openapi/users/:email/keys/:key_id
func (h *OpenAPIHandler) UpdateKey(c *gin.Context) {
	email, err := decodeEmailParam(c)
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	keyID, err := strconv.ParseInt(c.Param("key_id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "invalid key_id")
		return
	}
	var req UpdateAPIKeyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid_request: "+err.Error())
		return
	}

	svcReq := service.UpdateAPIKeyRequest{
		IPWhitelist:         req.IPWhitelist,
		IPBlacklist:         req.IPBlacklist,
		Quota:               req.Quota,
		ResetQuota:          req.ResetQuota,
		RateLimit5h:         req.RateLimit5h,
		RateLimit1d:         req.RateLimit1d,
		RateLimit7d:         req.RateLimit7d,
		ResetRateLimitUsage: req.ResetRateLimitUsage,
	}
	if req.Name != "" {
		name := req.Name
		svcReq.Name = &name
	}
	svcReq.GroupID = req.GroupID
	if req.Status != "" {
		status := req.Status
		svcReq.Status = &status
	}
	if req.ExpiresAt != nil {
		if *req.ExpiresAt == "" {
			svcReq.ExpiresAt = nil
			svcReq.ClearExpiration = true
		} else {
			t, err := time.Parse(time.RFC3339, *req.ExpiresAt)
			if err != nil {
				response.BadRequest(c, "invalid expires_at format: "+err.Error())
				return
			}
			svcReq.ExpiresAt = &t
		}
	}

	key, err := h.svc.UpdateKeyForEmail(c.Request.Context(), email, keyID, svcReq)
	if err != nil {
		writeOpenAPIError(c, err)
		return
	}
	response.Success(c, dto.APIKeyFromService(key))
}

// DeleteKey DELETE /api/v1/openapi/users/:email/keys/:key_id
func (h *OpenAPIHandler) DeleteKey(c *gin.Context) {
	email, err := decodeEmailParam(c)
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	keyID, err := strconv.ParseInt(c.Param("key_id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "invalid key_id")
		return
	}
	if err := h.svc.DeleteKeyForEmail(c.Request.Context(), email, keyID); err != nil {
		writeOpenAPIError(c, err)
		return
	}
	response.Success(c, gin.H{"deleted": true})
}

// ListUsage GET /api/v1/openapi/users/:email/usage
func (h *OpenAPIHandler) ListUsage(c *gin.Context) {
	email, err := decodeEmailParam(c)
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	page, pageSize := response.ParsePagination(c)
	params := service.OpenAPIUsageListParams{
		Page:     page,
		PageSize: pageSize,
		Model:    c.Query("model"),
	}
	if apiKeyIDStr := c.Query("api_key_id"); apiKeyIDStr != "" {
		if id, parseErr := strconv.ParseInt(apiKeyIDStr, 10, 64); parseErr == nil {
			params.APIKeyID = &id
		}
	}
	items, paginationRes, err := h.svc.ListUsageForEmail(c.Request.Context(), email, params)
	if err != nil {
		writeOpenAPIError(c, err)
		return
	}
	if paginationRes != nil {
		response.Paginated(c, items, paginationRes.Total, page, pageSize)
		return
	}
	response.Success(c, items)
}

// UsageStats GET /api/v1/openapi/users/:email/usage/stats
func (h *OpenAPIHandler) UsageStats(c *gin.Context) {
	email, err := decodeEmailParam(c)
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	params := service.OpenAPIUsageStatsParams{}
	if startStr := c.Query("start_at"); startStr != "" {
		if t, parseErr := time.Parse(time.RFC3339, startStr); parseErr == nil {
			params.StartAt = t
		} else {
			response.BadRequest(c, "invalid start_at: "+parseErr.Error())
			return
		}
	}
	if endStr := c.Query("end_at"); endStr != "" {
		if t, parseErr := time.Parse(time.RFC3339, endStr); parseErr == nil {
			params.EndAt = t
		} else {
			response.BadRequest(c, "invalid end_at: "+parseErr.Error())
			return
		}
	}
	stats, err := h.svc.UsageStatsForEmail(c.Request.Context(), email, params)
	if err != nil {
		writeOpenAPIError(c, err)
		return
	}
	response.Success(c, stats)
}

// decodeEmailParam 解码 path 里 URL-encoded 的 email。
func decodeEmailParam(c *gin.Context) (string, error) {
	raw := c.Param("email")
	decoded, err := url.PathUnescape(raw)
	if err != nil || strings.TrimSpace(decoded) == "" {
		return "", errors.New("invalid email parameter")
	}
	return decoded, nil
}

// writeOpenAPIError 把 service 层错误映射为合适的 HTTP 错误响应。
func writeOpenAPIError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, service.ErrOpenAPIUserNotFound):
		response.Error(c, http.StatusNotFound, "user_not_found")
	case errors.Is(err, service.ErrOpenAPIDuplicateOpID):
		response.Error(c, http.StatusConflict, "operation_pending")
	case errors.Is(err, service.ErrOpenAPIOpFailed):
		response.Error(c, http.StatusConflict, "operation_failed")
	case errors.Is(err, service.ErrOpenAPIBalanceUnderflow):
		response.Error(c, http.StatusUnprocessableEntity, "insufficient_balance")
	case errors.Is(err, service.ErrOpenAPIInvalidOpType),
		errors.Is(err, service.ErrOpenAPINegativeAmount),
		errors.Is(err, service.ErrOpenAPIEmailRequired):
		response.BadRequest(c, "invalid_request: "+err.Error())
	default:
		if response.ErrorFrom(c, err) {
			return
		}
		response.InternalError(c, err.Error())
	}
}
