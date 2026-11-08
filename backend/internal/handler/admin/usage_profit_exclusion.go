package admin

import (
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

// UsageProfitExclusionHandler 暴露「不计入盈亏的用户名单」的读写接口。
//
// 业务背景：内部人员也在用本站，但他们的余额是管理员手工调整的、并没有真实付款，
// 所以他们的「收入」是假的；可他们消耗掉的上游额度是真花钱，成本必须照实计入。
// 这份名单就是给收入侧用的——名单里的人，其收入不计入盈亏，成本照算。
//
// 为什么不并进 UpstreamCostSettingsHandler 那个 DTO：两者的失败模式完全不同。
// 凭据保存失败要重填令牌，名单保存失败只影响统计口径；塞进同一个 PUT 之后，
// 一次「令牌校验没过」会让名单也跟着不生效，反之亦然。分成两个接口，
// 前端两张卡各自提交，互不牵连。
type UsageProfitExclusionHandler struct {
	exclusionSvc *service.UsageProfitExclusionService
	// adminService 只用来把用户 ID 翻译成邮箱，方便面板直接显示「是谁」。
	// 允许为 nil：那时只回 ID，面板退化成显示编号，不影响保存。
	adminService service.AdminService
}

// NewUsageProfitExclusionHandler 创建盈亏排除名单 handler。
func NewUsageProfitExclusionHandler(exclusionSvc *service.UsageProfitExclusionService, adminService service.AdminService) *UsageProfitExclusionHandler {
	return &UsageProfitExclusionHandler{exclusionSvc: exclusionSvc, adminService: adminService}
}

// profitExcludedUserDTO 是名单里的一项。
//
// Email/Username 为空**不代表这个 ID 无效**：用户可能已被删除或改名。前端据此
// 把这一项显示成「已删除/未知用户 #id」而不是直接隐藏——隐藏会让面板显示的名单
// 与库里存的名单不一致，管理员将无法解释「为什么统计口径和界面对不上」。
type profitExcludedUserDTO struct {
	ID       int64  `json:"id"`
	Email    string `json:"email"`
	Username string `json:"username"`
}

// profitExclusionDTO 是 GET/PUT 的 data。
type profitExclusionDTO struct {
	// UserIDs 名单本体，升序去重，永不为 nil（前端直接遍历）。
	UserIDs []int64 `json:"user_ids"`
	// Users 是 UserIDs 的展示信息，顺序与 UserIDs 一致。
	Users []profitExcludedUserDTO `json:"users"`
}

// profitExclusionInputDTO 是 PUT 的入参。
//
// 用「整份覆盖」而不是「增删单个」：名单很短，整份覆盖让前端只需维护一个数组，
// 不必为「加了又删」的中间态设计额外接口，也不会出现两次请求互相覆盖的竞态。
type profitExclusionInputDTO struct {
	UserIDs []int64 `json:"user_ids"`
}

// Settings 返回当前生效的盈亏排除名单。
//
// GET /admin/usage/profit-exclusion
func (h *UsageProfitExclusionHandler) Settings(c *gin.Context) {
	if h == nil || h.exclusionSvc == nil {
		response.InternalError(c, "USAGE_PROFIT_EXCLUSION_INTERNAL: 盈亏排除名单服务未装配")
		return
	}

	view, err := h.exclusionSvc.Effective(c.Request.Context())
	if err != nil {
		writeUsageProfitExclusionError(c, err)
		return
	}

	response.Success(c, h.toDTO(c, view.UserIDs))
}

// UpdateSettings 覆盖盈亏排除名单。
//
// PUT /admin/usage/profit-exclusion
func (h *UsageProfitExclusionHandler) UpdateSettings(c *gin.Context) {
	if h == nil || h.exclusionSvc == nil {
		response.InternalError(c, "USAGE_PROFIT_EXCLUSION_INTERNAL: 盈亏排除名单服务未装配")
		return
	}

	var input profitExclusionInputDTO
	if err := c.ShouldBindJSON(&input); err != nil {
		response.BadRequest(c, "USAGE_PROFIT_EXCLUSION_BAD_REQUEST: 请求体不是合法的 JSON: "+err.Error())
		return
	}

	view, err := h.exclusionSvc.Update(c.Request.Context(), input.UserIDs)
	if err != nil {
		writeUsageProfitExclusionError(c, err)
		return
	}

	response.Success(c, h.toDTO(c, view.UserIDs))
}

// toDTO 把 ID 列表补上展示信息。
//
// 单个用户查不到（已删除）时保留该 ID 并把邮箱留空，**绝不静默丢弃**：
// 丢弃会让「面板看到的名单」比「库里存的名单」短，而统计口径仍按完整的名单算，
// 于是管理员会看到一对无法解释的数字。
func (h *UsageProfitExclusionHandler) toDTO(c *gin.Context, userIDs []int64) profitExclusionDTO {
	ids := userIDs
	if ids == nil {
		ids = []int64{}
	}
	users := make([]profitExcludedUserDTO, 0, len(ids))
	for _, id := range ids {
		item := profitExcludedUserDTO{ID: id}
		if h.adminService != nil {
			if user, err := h.adminService.GetUserIncludeDeleted(c.Request.Context(), id); err == nil && user != nil {
				item.Email = user.Email
				item.Username = user.Username
			}
		}
		users = append(users, item)
	}
	return profitExclusionDTO{UserIDs: ids, Users: users}
}

// writeUsageProfitExclusionError 把名单服务的哨兵错误翻译成 HTTP 状态码。
//
// 读名单失败必须报错而不是退回空名单：空名单会把内部人员的虚假收入算进毛利，
// 图上只是利润偏高，看不出任何异常。宁可让这次查询失败得明明白白。
func writeUsageProfitExclusionError(c *gin.Context, err error) {
	response.InternalError(c, "USAGE_PROFIT_EXCLUSION_STORE_UNAVAILABLE: 无法读取盈亏排除名单，已中止本次统计以免给出偏高的利润: "+err.Error())
}
