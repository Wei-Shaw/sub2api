package handler

import (
	"context"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/handler/dto"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"

	dbent "github.com/Wei-Shaw/sub2api/ent"

	"github.com/gin-gonic/gin"
)

// rechargePromoActivityAPI 收窄 PlazaHandler 对活动 service 的依赖，便于单测注入替身。
//
// 真正实现是 *service.RechargePromoActivityService，handler 只用其
// GetCurrent；其它 admin CRUD 接口与本端点无关。
type rechargePromoActivityAPI interface {
	GetCurrent(ctx context.Context) (*dbent.RechargePromoActivity, error)
}

// promoNowProvider 仅用于测试时注入"假的当前时间"。生产代码使用 time.Now。
type promoNowProvider func() time.Time

// GetPublicRechargePromo GET /api/v1/plaza/recharge-promo
//
// 匿名可访问端点，返回当前生效的充值赠送活动用于首页等公开 marketing surfaces。
//
// 行为约定（详见 specs/recharge-bonus/spec.md::Public Recharge Promo Endpoint）：
//   - 无 enabled=true 行 → `{ "promo": null }` HTTP 200
//   - 有行但已过期 / 未生效 / tiers 空 → `{ "promo": null }` HTTP 200
//   - 后端取数失败 → `{ "promo": null }` HTTP 200（silent skip，避免阻塞首页）
//   - 任何 Authorization 头被忽略，handler 不依赖 user context
//
// 响应仅包含 name / valid_from / valid_until / tiers / version；
// 不返回 enabled（出现即生效）、不返回 activity_id（内部审计字段）。
func (h *PlazaHandler) GetPublicRechargePromo(c *gin.Context) {
	resp := dto.PublicRechargePromoResponseDTO{Promo: nil}

	if h.promoActivity == nil {
		// service 未注入（旧 wiring 兜底）：等价于"无活动"。
		response.Success(c, resp)
		return
	}

	now := time.Now()
	if h.promoNow != nil {
		now = h.promoNow()
	}

	row, err := h.promoActivity.GetCurrent(c.Request.Context())
	if err != nil || row == nil {
		// silent skip：上游错误等价于"无活动"，让 marketing 页面不显示 banner
		// 即可，不向匿名访客抛出 5xx 干扰整页展示。
		response.Success(c, resp)
		return
	}

	promo := service.ActivityToPromo(row)
	if promo == nil || !promo.IsActiveAt(now) || len(promo.Tiers) == 0 {
		response.Success(c, resp)
		return
	}

	resp.Promo = publicRechargePromoFromService(promo)
	response.Success(c, resp)
}

// publicRechargePromoFromService 把 service 层 RechargePromo 映射为公开 DTO；
// 故意舍弃 Enabled / ActivityID 两个内部字段。
func publicRechargePromoFromService(p *service.RechargePromo) *dto.PublicRechargePromoDTO {
	if p == nil {
		return nil
	}
	tiers := make([]dto.PublicRechargePromoTierDTO, 0, len(p.Tiers))
	for _, t := range p.Tiers {
		tiers = append(tiers, dto.PublicRechargePromoTierDTO{
			MinAmount: t.MinAmount,
			BonusRate: t.BonusRate,
		})
	}
	return &dto.PublicRechargePromoDTO{
		Name:       p.Name,
		ValidFrom:  p.ValidFrom,
		ValidUntil: p.ValidUntil,
		Tiers:      tiers,
		Version:    p.Version,
	}
}
