package handler

import (
	"strconv"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

func (h *DingTalkOrganizationHandler) Statistics(c *gin.Context) {
	actor, admin, ok := dingTalkActor(c)
	if !ok {
		return
	}
	start, err := time.Parse(time.RFC3339, c.Query("start"))
	if err != nil {
		response.BadRequest(c, "Start must be an RFC3339 timestamp")
		return
	}
	end, err := time.Parse(time.RFC3339, c.Query("end"))
	if err != nil || !start.Before(end) {
		response.BadRequest(c, "End must be after start")
		return
	}
	department, err := strconv.ParseInt(c.DefaultQuery("department_id", "0"), 10, 64)
	if err != nil || department < 0 {
		response.BadRequest(c, "Invalid department")
		return
	}
	configured, err := h.settings.GetDingTalkApps(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	apps := []service.DingTalkStatisticsApp{}
	if cfg, e := h.settings.GetDingTalkConnectOAuthConfig(c.Request.Context()); e == nil {
		company := cfg.InternalCorpID
		if company == "" {
			company = "app:default"
		}
		apps = append(apps, service.DingTalkStatisticsApp{ID: "default", Name: "DingTalk", CompanyID: company})
	}
	for _, a := range configured {
		if a.Enabled {
			company := a.CorpID
			if company == "" {
				company = "app:" + a.ID
			}
			apps = append(apps, service.DingTalkStatisticsApp{ID: a.ID, Name: a.Name, CompanyID: company})
		}
	}
	result, err := h.organization.Statistics(c.Request.Context(), apps, actor, admin, start, end, c.Query("company_id"), c.Query("app_id"), department)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, result)
}
