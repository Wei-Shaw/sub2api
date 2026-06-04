package admin

import (
	"strconv"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// RechargePromoHandler 暴露 admin 端的充值赠送活动 CRUD 接口。
type RechargePromoHandler struct {
	svc *service.RechargePromoActivityService
}

// NewRechargePromoHandler 构造 RechargePromoHandler。
func NewRechargePromoHandler(svc *service.RechargePromoActivityService) *RechargePromoHandler {
	return &RechargePromoHandler{svc: svc}
}

// adminPromoTier 是 admin 端 DTO 档位结构。
type adminPromoTier struct {
	MinAmount float64 `json:"min_amount"`
	BonusRate float64 `json:"bonus_rate"`
}

// adminPromoActivity 是 admin 列表/详情返回的活动 DTO。
type adminPromoActivity struct {
	ID         int64            `json:"id"`
	Name       string           `json:"name"`
	Enabled    bool             `json:"enabled"`
	ValidFrom  *time.Time       `json:"valid_from,omitempty"`
	ValidUntil *time.Time       `json:"valid_until,omitempty"`
	Tiers      []adminPromoTier `json:"tiers"`
	Operator   string           `json:"operator"`
	Note       *string          `json:"note,omitempty"`
	CreatedAt  time.Time        `json:"created_at"`
	UpdatedAt  time.Time        `json:"updated_at"`
}

func toAdminPromoActivity(row *dbent.RechargePromoActivity) *adminPromoActivity {
	if row == nil {
		return nil
	}
	tiers := make([]adminPromoTier, 0, len(row.Tiers))
	for _, t := range row.Tiers {
		tiers = append(tiers, adminPromoTier{MinAmount: t.MinAmount, BonusRate: t.BonusRate})
	}
	return &adminPromoActivity{
		ID:         row.ID,
		Name:       row.Name,
		Enabled:    row.Enabled,
		ValidFrom:  row.ValidFrom,
		ValidUntil: row.ValidUntil,
		Tiers:      tiers,
		Operator:   row.Operator,
		Note:       row.Note,
		CreatedAt:  row.CreatedAt,
		UpdatedAt:  row.UpdatedAt,
	}
}

// createOrUpdatePromoRequest 创建/更新共享 payload。Update 时所有 promo 字段整体替换。
type createOrUpdatePromoRequest struct {
	Name       string           `json:"name"`
	Enabled    bool             `json:"enabled"`
	ValidFrom  *time.Time       `json:"valid_from"`
	ValidUntil *time.Time       `json:"valid_until"`
	Tiers      []adminPromoTier `json:"tiers"`
	Note       *string          `json:"note"`
}

func (r createOrUpdatePromoRequest) toServicePromo() *service.RechargePromo {
	tiers := make([]service.RechargePromoTier, 0, len(r.Tiers))
	for _, t := range r.Tiers {
		tiers = append(tiers, service.RechargePromoTier{MinAmount: t.MinAmount, BonusRate: t.BonusRate})
	}
	return &service.RechargePromo{
		Enabled:    r.Enabled,
		ValidFrom:  r.ValidFrom,
		ValidUntil: r.ValidUntil,
		Tiers:      tiers,
	}
}

// operatorFromCtx 从 admin 鉴权上下文构造 audit 用的 operator 字符串。
func operatorFromCtx(c *gin.Context) string {
	if subj, ok := middleware.GetAuthSubjectFromContext(c); ok && subj.UserID > 0 {
		return "admin:" + strconv.FormatInt(subj.UserID, 10)
	}
	return "system"
}

// List GET /api/v1/admin/recharge-promos
func (h *RechargePromoHandler) List(c *gin.Context) {
	page, pageSize := response.ParsePagination(c)
	rows, total, err := h.svc.List(c.Request.Context(), page, pageSize)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	out := make([]*adminPromoActivity, 0, len(rows))
	for _, r := range rows {
		out = append(out, toAdminPromoActivity(r))
	}
	response.Paginated(c, out, int64(total), page, pageSize)
}

// Get GET /api/v1/admin/recharge-promos/:id
func (h *RechargePromoHandler) Get(c *gin.Context) {
	id, ok := parseIDParam(c, "id")
	if !ok {
		return
	}
	row, err := h.svc.GetByID(c.Request.Context(), id)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	if row == nil {
		response.NotFound(c, "recharge promo activity not found")
		return
	}
	response.Success(c, toAdminPromoActivity(row))
}

// Create POST /api/v1/admin/recharge-promos
func (h *RechargePromoHandler) Create(c *gin.Context) {
	var req createOrUpdatePromoRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	note := ""
	if req.Note != nil {
		note = *req.Note
	}
	row, err := h.svc.Create(c.Request.Context(), service.CreateActivityInput{
		Name:     req.Name,
		Promo:    req.toServicePromo(),
		Operator: operatorFromCtx(c),
		Note:     note,
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Created(c, toAdminPromoActivity(row))
}

// Update PUT /api/v1/admin/recharge-promos/:id
func (h *RechargePromoHandler) Update(c *gin.Context) {
	id, ok := parseIDParam(c, "id")
	if !ok {
		return
	}
	var req createOrUpdatePromoRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	name := req.Name
	row, err := h.svc.Update(c.Request.Context(), id, service.UpdateActivityInput{
		Name:     &name,
		Promo:    req.toServicePromo(),
		Operator: operatorFromCtx(c),
		Note:     req.Note,
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, toAdminPromoActivity(row))
}

// toggleRequest 请求体仅包含 enabled。
type toggleRequest struct {
	Enabled bool `json:"enabled"`
}

// Toggle POST /api/v1/admin/recharge-promos/:id/toggle
func (h *RechargePromoHandler) Toggle(c *gin.Context) {
	id, ok := parseIDParam(c, "id")
	if !ok {
		return
	}
	var req toggleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	row, err := h.svc.SetEnabled(c.Request.Context(), id, req.Enabled)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, toAdminPromoActivity(row))
}

// Delete DELETE /api/v1/admin/recharge-promos/:id
func (h *RechargePromoHandler) Delete(c *gin.Context) {
	id, ok := parseIDParam(c, "id")
	if !ok {
		return
	}
	if err := h.svc.Delete(c.Request.Context(), id); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"message": "deleted"})
}
