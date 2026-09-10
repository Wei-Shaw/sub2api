package handler

import (
	"context"
	"strconv"
	"strings"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

const maxAccountVisibleUsers = 5000

type accountVisibilityService interface {
	ListVisibleAccounts(ctx context.Context, userID int64, params pagination.PaginationParams, filters service.AccountVisibilityFilters) ([]*service.Account, *pagination.PaginationResult, error)
	CanViewAccount(ctx context.Context, userID, accountID int64) (bool, error)
	ListVisibleGroups(ctx context.Context, userID int64) ([]service.AccountVisibilityGroup, error)
	ListAccountVisibleUsers(ctx context.Context, accountID int64) ([]service.AccountVisibleUser, error)
	ReplaceAccountVisibleUsers(ctx context.Context, accountID int64, userIDs []int64) error
}

type AccountVisibilityHandler struct {
	service accountVisibilityService
}

func NewAccountVisibilityHandler(service *service.AccountVisibilityService) *AccountVisibilityHandler {
	return &AccountVisibilityHandler{service: service}
}

func (h *AccountVisibilityHandler) RequireVisibleAccount() gin.HandlerFunc {
	return func(c *gin.Context) {
		subject, ok := middleware.GetAuthSubjectFromContext(c)
		if !ok || subject.UserID <= 0 {
			response.Unauthorized(c, "unauthorized")
			c.Abort()
			return
		}
		accountID, err := parsePositiveID(c.Param("id"))
		if err != nil {
			response.ErrorFrom(c, err)
			c.Abort()
			return
		}
		visible, err := h.service.CanViewAccount(c.Request.Context(), subject.UserID, accountID)
		if err != nil {
			response.ErrorFrom(c, err)
			c.Abort()
			return
		}
		if !visible {
			response.NotFound(c, "account not found")
			c.Abort()
			return
		}
		c.Next()
	}
}

type visibleAccountGroupResponse struct {
	ID       int64  `json:"id"`
	Name     string `json:"name"`
	Platform string `json:"platform"`
}

type visibleAccountResponse struct {
	ID                     int64                         `json:"id"`
	Name                   string                        `json:"name"`
	Platform               string                        `json:"platform"`
	Type                   string                        `json:"type"`
	Concurrency            int                           `json:"concurrency"`
	Priority               int                           `json:"priority"`
	Status                 string                        `json:"status"`
	Schedulable            bool                          `json:"schedulable"`
	LastUsedAt             *time.Time                    `json:"last_used_at"`
	ExpiresAt              *int64                        `json:"expires_at"`
	CreatedAt              time.Time                     `json:"created_at"`
	RateLimitResetAt       *time.Time                    `json:"rate_limit_reset_at"`
	OverloadUntil          *time.Time                    `json:"overload_until"`
	TempUnschedulableUntil *time.Time                    `json:"temp_unschedulable_until"`
	Groups                 []visibleAccountGroupResponse `json:"groups"`
	GroupIDs               []int64                       `json:"group_ids"`
}

func (h *AccountVisibilityHandler) List(c *gin.Context) {
	subject, ok := middleware.GetAuthSubjectFromContext(c)
	if !ok || subject.UserID <= 0 {
		response.Unauthorized(c, "unauthorized")
		return
	}
	page, pageSize := response.ParsePagination(c)
	search := strings.TrimSpace(c.Query("search"))
	if len(search) > 100 {
		search = search[:100]
	}
	groupID, err := parseVisibleAccountGroup(c.Query("group"))
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	params := pagination.PaginationParams{
		Page: page, PageSize: pageSize,
		SortBy:    c.DefaultQuery("sort_by", "name"),
		SortOrder: c.DefaultQuery("sort_order", pagination.SortOrderAsc),
	}
	filters := service.AccountVisibilityFilters{
		Platform: c.Query("platform"), Type: c.Query("type"), Status: c.Query("status"),
		Search: search, GroupID: groupID,
	}
	accounts, result, err := h.service.ListVisibleAccounts(c.Request.Context(), subject.UserID, params, filters)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	items := make([]visibleAccountResponse, 0, len(accounts))
	for _, account := range accounts {
		items = append(items, visibleAccountFromService(account))
	}
	response.Paginated(c, items, result.Total, result.Page, result.PageSize)
}

func (h *AccountVisibilityHandler) ListGroups(c *gin.Context) {
	subject, ok := middleware.GetAuthSubjectFromContext(c)
	if !ok || subject.UserID <= 0 {
		response.Unauthorized(c, "unauthorized")
		return
	}
	groups, err := h.service.ListVisibleGroups(c.Request.Context(), subject.UserID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, groups)
}

func (h *AccountVisibilityHandler) ListAccountVisibleUsers(c *gin.Context) {
	accountID, err := parsePositiveID(c.Param("id"))
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	users, err := h.service.ListAccountVisibleUsers(c.Request.Context(), accountID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, users)
}

func (h *AccountVisibilityHandler) UpdateAccountVisibleUsers(c *gin.Context) {
	accountID, err := parsePositiveID(c.Param("id"))
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	var req struct {
		UserIDs []int64 `json:"user_ids" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.ErrorFrom(c, infraerrors.BadRequest("INVALID_VISIBLE_USERS", "user_ids must be an array"))
		return
	}
	if len(req.UserIDs) > maxAccountVisibleUsers {
		response.ErrorFrom(c, infraerrors.BadRequest("VISIBLE_USERS_LIMIT_EXCEEDED", "too many visible users"))
		return
	}
	if err := h.service.ReplaceAccountVisibleUsers(c.Request.Context(), accountID, req.UserIDs); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"user_ids": req.UserIDs})
}

func parseVisibleAccountGroup(value string) (int64, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, nil
	}
	if value == "ungrouped" {
		return service.AccountListGroupUngrouped, nil
	}
	id, err := strconv.ParseInt(value, 10, 64)
	if err != nil || id <= 0 {
		return 0, infraerrors.BadRequest("INVALID_GROUP_FILTER", "invalid group filter")
	}
	return id, nil
}

func parsePositiveID(value string) (int64, error) {
	id, err := strconv.ParseInt(value, 10, 64)
	if err != nil || id <= 0 {
		return 0, infraerrors.BadRequest("INVALID_ID", "invalid id")
	}
	return id, nil
}

func visibleAccountFromService(account *service.Account) visibleAccountResponse {
	item := visibleAccountResponse{
		ID: account.ID, Name: account.Name, Platform: account.Platform, Type: account.Type,
		Concurrency: account.Concurrency, Priority: account.Priority, Status: account.Status,
		Schedulable: account.Schedulable, LastUsedAt: account.LastUsedAt, CreatedAt: account.CreatedAt,
		RateLimitResetAt: account.RateLimitResetAt, OverloadUntil: account.OverloadUntil,
		TempUnschedulableUntil: account.TempUnschedulableUntil,
		Groups:                 make([]visibleAccountGroupResponse, 0, len(account.Groups)),
		GroupIDs:               append([]int64(nil), account.GroupIDs...),
	}
	if account.ExpiresAt != nil {
		value := account.ExpiresAt.Unix()
		item.ExpiresAt = &value
	}
	for _, group := range account.Groups {
		if group == nil {
			continue
		}
		item.Groups = append(item.Groups, visibleAccountGroupResponse{ID: group.ID, Name: group.Name, Platform: group.Platform})
	}
	return item
}
