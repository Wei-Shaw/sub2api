package admin

import (
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

func isMarketingRequest(c *gin.Context) bool {
	role, _ := middleware.GetUserRoleFromContext(c)
	return role == service.RoleMarketing
}

func marketingSubjectUserID(c *gin.Context) (int64, bool) {
	subject, ok := middleware.GetAuthSubjectFromContext(c)
	if !ok || subject.UserID <= 0 {
		response.ErrorFrom(c, infraerrors.Forbidden("FORBIDDEN", "Marketing user scope required"))
		return 0, false
	}
	return subject.UserID, true
}

func marketingOwnedUserIDs(c *gin.Context, adminService service.AdminService) ([]int64, bool) {
	if !isMarketingRequest(c) {
		return nil, true
	}
	marketingUserID, ok := marketingSubjectUserID(c)
	if !ok {
		return nil, false
	}

	const pageSize = 500
	ids := make([]int64, 0)
	for page := 1; ; page++ {
		filters := service.UserListFilters{AffiliateInviterID: &marketingUserID}
		users, _, err := adminService.ListUsers(c.Request.Context(), page, pageSize, filters, "id", "asc")
		if err != nil {
			response.ErrorFrom(c, err)
			return nil, false
		}
		for i := range users {
			ids = append(ids, users[i].ID)
		}
		if len(users) < pageSize {
			break
		}
	}
	return ids, true
}

func ensureMarketingCanManageUser(c *gin.Context, adminService service.AdminService, userID int64) bool {
	if !isMarketingRequest(c) {
		return true
	}
	if userID <= 0 {
		response.BadRequest(c, "Invalid user ID")
		return false
	}
	marketingUserID, ok := marketingSubjectUserID(c)
	if !ok {
		return false
	}
	filters := service.UserListFilters{AffiliateInviterID: &marketingUserID, UserID: &userID}
	users, _, err := adminService.ListUsers(c.Request.Context(), 1, 1, filters, "created_at", "desc")
	if err != nil {
		response.ErrorFrom(c, err)
		return false
	}
	for i := range users {
		if users[i].ID == userID {
			return true
		}
	}
	response.ErrorFrom(c, infraerrors.Forbidden("FORBIDDEN", "Marketing role cannot manage users outside its affiliate scope"))
	return false
}

func restrictExplicitUserIDToMarketingScope(c *gin.Context, adminService service.AdminService, explicitUserID *int64) ([]int64, bool) {
	if !isMarketingRequest(c) {
		return nil, true
	}
	if explicitUserID != nil {
		if !ensureMarketingCanManageUser(c, adminService, *explicitUserID) {
			return nil, false
		}
		return []int64{*explicitUserID}, true
	}
	return marketingOwnedUserIDs(c, adminService)
}
