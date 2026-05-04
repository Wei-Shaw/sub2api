package handler

import (
	"fmt"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

func openAIModelAccessAllowed(c *gin.Context, model string) (bool, string) {
	if service.IsSysModel(model) {
		return true, ""
	}
	role, _ := middleware.GetUserRoleFromContext(c)
	if role == service.RoleAdmin {
		return true, ""
	}
	model = strings.TrimSpace(model)
	return false, fmt.Sprintf("model %q is restricted to administrators; please use %q instead", model, model+"-Sys")
}
