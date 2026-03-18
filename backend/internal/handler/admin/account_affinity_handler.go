package admin

import (
	"context"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

// 账号亲和调度 API 与契约说明见 docs/ACCOUNT_SCHEDULING_FLOW.md 。

// GetAffinityClients returns the list of affinity clients for an account with last active timestamps.
// GET /api/v1/admin/accounts/:id/affinity-clients
func (h *AccountHandler) GetAffinityClients(c *gin.Context) {
	accountID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid account ID")
		return
	}

	account, err := h.adminService.GetAccount(c.Request.Context(), accountID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	if !account.IsAffinityEnabled() {
		response.Success(c, []service.AffinityClient{})
		return
	}

	if h.gatewayCache == nil || len(account.GroupIDs) == 0 {
		response.Success(c, []service.AffinityClient{})
		return
	}

	clients, err := h.gatewayCache.GetAccountAffinityClientsWithScores(
		c.Request.Context(), accountID, account.GroupIDs, service.ClientAffinityTTL,
	)
	if err != nil {
		response.Success(c, []service.AffinityClient{})
		return
	}

	response.Success(c, clients)
}

// clearAccountAffinity 清除指定账号在所有分组的亲和记录。
func (h *AccountHandler) clearAccountAffinity(ctx context.Context, accountID int64, groupIDs []int64) {
	if h.gatewayCache == nil || len(groupIDs) == 0 {
		return
	}
	if err := h.gatewayCache.ClearAccountAffinity(ctx, accountID, groupIDs); err != nil {
		// 清理失败不影响主流程，记录日志即可
		slog.Warn("clear account affinity failed",
			"account_id", accountID,
			"error", err,
		)
	}
}

// countUniqueUsersFromAffinityMembers 从 "{userID}/{clientID}" 格式的成员列表中计算唯一用户数。
func countUniqueUsersFromAffinityMembers(members []string) int64 {
	users := make(map[string]struct{}, len(members))
	for _, m := range members {
		if idx := strings.Index(m, "/"); idx > 0 {
			users[m[:idx]] = struct{}{}
		}
	}
	return int64(len(users))
}

// AffinityDetailClient 亲和详情中的单个客户端信息
type AffinityDetailClient struct {
	ClientID   string    `json:"client_id"`
	LastActive time.Time `json:"last_active"`
	IsPinned   bool      `json:"is_pinned"`
}

// AffinityDetailUser 亲和详情中按用户分组的信息
type AffinityDetailUser struct {
	UserID      int64                  `json:"user_id"`
	UserEmail   string                 `json:"user_email"`
	ClientCount int                    `json:"client_count"`
	IsPinned    bool                   `json:"is_pinned"`
	Clients     []AffinityDetailClient `json:"clients"`
}

// AffinityDetailsResponse 亲和详情响应
type AffinityDetailsResponse struct {
	Users        []AffinityDetailUser `json:"users"`
	TotalUsers   int                  `json:"total_users"`
	TotalClients int                  `json:"total_clients"`
	PinnedUsers  []int64              `json:"pinned_users"`
}

type affinityStateSnapshot struct {
	enabled  bool
	groupIDs []int64
}

func (h *AccountHandler) captureAffinityStates(ctx context.Context, accountIDs []int64) map[int64]affinityStateSnapshot {
	states := make(map[int64]affinityStateSnapshot, len(accountIDs))
	if h.gatewayCache == nil || len(accountIDs) == 0 {
		return states
	}
	for _, accountID := range accountIDs {
		account, err := h.adminService.GetAccount(ctx, accountID)
		if err != nil || account == nil {
			continue
		}
		states[accountID] = affinityStateSnapshot{
			enabled:  account.IsAffinityEnabled(),
			groupIDs: append([]int64(nil), account.GroupIDs...),
		}
	}
	return states
}

func (h *AccountHandler) clearAffinityCacheIfDisabled(ctx context.Context, accountID int64, oldState affinityStateSnapshot) {
	if h.gatewayCache == nil || !oldState.enabled {
		return
	}
	account, err := h.adminService.GetAccount(ctx, accountID)
	if err != nil || account == nil || account.IsAffinityEnabled() {
		return
	}
	groupIDs := oldState.groupIDs
	if len(account.GroupIDs) > 0 {
		groupIDs = mergeGroupIDs(groupIDs, account.GroupIDs)
	}
	h.clearAccountAffinity(ctx, accountID, groupIDs)
}

func (h *AccountHandler) clearAffinityCacheForBulkIfDisabled(ctx context.Context, accountIDs []int64, oldStates map[int64]affinityStateSnapshot) {
	for _, accountID := range accountIDs {
		oldState, ok := oldStates[accountID]
		if !ok {
			continue
		}
		h.clearAffinityCacheIfDisabled(ctx, accountID, oldState)
	}
}

// GetAffinityDetails returns the affinity details grouped by user for an account.
// GET /api/v1/admin/accounts/:id/affinity-details
func (h *AccountHandler) GetAffinityDetails(c *gin.Context) {
	accountID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid account ID")
		return
	}

	emptyResp := AffinityDetailsResponse{
		Users:        []AffinityDetailUser{},
		TotalUsers:   0,
		TotalClients: 0,
		PinnedUsers:  []int64{},
	}

	account, err := h.adminService.GetAccount(c.Request.Context(), accountID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	if !account.IsAffinityEnabled() {
		response.Success(c, emptyResp)
		return
	}

	pinnedUsers := account.GetPinnedUsers()
	if pinnedUsers == nil {
		pinnedUsers = []int64{}
	}

	if h.gatewayCache == nil || len(account.GroupIDs) == 0 {
		emptyResp.PinnedUsers = pinnedUsers
		response.Success(c, emptyResp)
		return
	}

	clients, err := h.gatewayCache.GetAccountAffinityClientsWithScores(
		c.Request.Context(), accountID, account.GroupIDs, service.ClientAffinityTTL,
	)
	if err != nil {
		emptyResp.PinnedUsers = pinnedUsers
		response.Success(c, emptyResp)
		return
	}

	pinnedSet := make(map[int64]struct{}, len(pinnedUsers))
	for _, uid := range pinnedUsers {
		pinnedSet[uid] = struct{}{}
	}

	// 按 UserID 分组
	userMap := make(map[int64]*AffinityDetailUser)
	var userOrder []int64
	for _, cl := range clients {
		u, ok := userMap[cl.UserID]
		if !ok {
			_, pinned := pinnedSet[cl.UserID]
			u = &AffinityDetailUser{
				UserID:      cl.UserID,
				UserEmail:   "",
				ClientCount: 0,
				IsPinned:    pinned,
				Clients:     []AffinityDetailClient{},
			}
			userMap[cl.UserID] = u
			userOrder = append(userOrder, cl.UserID)
		}
		u.Clients = append(u.Clients, AffinityDetailClient{
			ClientID:   cl.ClientID,
			LastActive: cl.LastActive,
			IsPinned:   false, // 客户端级别暂无 pinned 概念
		})
	}

	// 关联用户邮箱（查询失败时保持空字符串，不影响主流程）
	for _, uid := range userOrder {
		user, uErr := h.adminService.GetUser(c.Request.Context(), uid)
		if uErr != nil || user == nil {
			continue
		}
		if u, ok := userMap[uid]; ok {
			if user.Email != "" {
				u.UserEmail = user.Email
			} else {
				u.UserEmail = user.Username
			}
		}
	}

	users := make([]AffinityDetailUser, 0, len(userOrder))
	for _, uid := range userOrder {
		u := userMap[uid]
		u.ClientCount = len(u.Clients)
		users = append(users, *u)
	}

	response.Success(c, AffinityDetailsResponse{
		Users:        users,
		TotalUsers:   len(users),
		TotalClients: len(clients),
		PinnedUsers:  pinnedUsers,
	})
}
