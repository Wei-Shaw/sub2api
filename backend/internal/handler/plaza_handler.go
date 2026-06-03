package handler

import (
	"context"
	"strconv"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/handler/dto"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// plazaSearchMaxLen 是 ?q= 子串过滤参数的最大长度。
const plazaSearchMaxLen = 64

// plazaServiceAPI 收窄了 PlazaHandler 对 service 的依赖，方便单测注入替身。
type plazaServiceAPI interface {
	ListModelRows(ctx context.Context, filter service.PlazaModelFilter) ([]service.PlazaModelRow, service.PlazaCurrencyMeta, error)
	ListPlanCards(ctx context.Context) ([]service.PlazaPlanCard, service.PlazaCurrencyMeta, error)
}

// PlazaHandler 处理公开计费广场（model / plan）相关请求；无需鉴权。
type PlazaHandler struct {
	plazaService plazaServiceAPI
}

// NewPlazaHandler 构造 PlazaHandler。
func NewPlazaHandler(plazaService *service.PlazaService) *PlazaHandler {
	return &PlazaHandler{plazaService: plazaService}
}

// ListModels GET /api/v1/plaza/models
//
// 可选查询参数：
//   - group_id 整数，0 = 不过滤
//   - platform 字符串，大小写不敏感精确匹配
//   - q       模型名子串（大小写不敏感），最长 64 字符
func (h *PlazaHandler) ListModels(c *gin.Context) {
	filter := service.PlazaModelFilter{}

	if raw := strings.TrimSpace(c.Query("group_id")); raw != "" {
		gid, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || gid < 0 {
			response.ErrorFrom(c, infraerrors.BadRequest("INVALID_GROUP_ID", "group_id must be a non-negative integer"))
			return
		}
		filter.GroupID = gid
	}

	if raw := strings.TrimSpace(c.Query("platform")); raw != "" {
		filter.Platform = raw
	}

	if raw := strings.TrimSpace(c.Query("q")); raw != "" {
		if len(raw) > plazaSearchMaxLen {
			response.ErrorFrom(c, infraerrors.BadRequest("INVALID_SEARCH_LENGTH", "q must be 64 characters or fewer"))
			return
		}
		filter.Q = raw
	}

	rows, meta, err := h.plazaService.ListModelRows(c.Request.Context(), filter)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	response.Success(c, dto.PlazaModelsResponseDTO{
		Rows:         plazaModelRowsToDTO(rows),
		CurrencyMeta: plazaCurrencyMetaToDTO(meta),
	})
}

// ListPlans GET /api/v1/plaza/plans
func (h *PlazaHandler) ListPlans(c *gin.Context) {
	cards, meta, err := h.plazaService.ListPlanCards(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	response.Success(c, dto.PlazaPlansResponseDTO{
		Cards:        plazaPlanCardsToDTO(cards),
		CurrencyMeta: plazaCurrencyMetaToDTO(meta),
	})
}

// ---------- domain → DTO mappers ----------

func plazaCurrencyMetaToDTO(m service.PlazaCurrencyMeta) dto.PlazaCurrencyMetaDTO {
	return dto.PlazaCurrencyMetaDTO{
		BalanceRechargeMultiplier: m.BalanceRechargeMultiplier,
		ModelNative:               m.ModelNative,
		PlanNative:                m.PlanNative,
	}
}

func plazaModelRowsToDTO(rows []service.PlazaModelRow) []dto.PlazaModelRowDTO {
	out := make([]dto.PlazaModelRowDTO, len(rows))
	for i, r := range rows {
		out[i] = dto.PlazaModelRowDTO{
			GroupID:                    r.GroupID,
			GroupName:                  r.GroupName,
			Platform:                   r.Platform,
			Model:                      r.Model,
			Type:                       r.Type,
			InputPricePerMTok:          r.InputPricePerMTok,
			OutputPricePerMTok:         r.OutputPricePerMTok,
			SiteInputPricePerMTok:      r.SiteInputPricePerMTok,
			SiteOutputPricePerMTok:     r.SiteOutputPricePerMTok,
			CacheWritePricePerMTok:     r.CacheWritePricePerMTok,
			CacheReadPricePerMTok:      r.CacheReadPricePerMTok,
			SiteCacheWritePricePerMTok: r.SiteCacheWritePricePerMTok,
			SiteCacheReadPricePerMTok:  r.SiteCacheReadPricePerMTok,
			BaseImagePrices:            plazaImagePricesToDTO(r.BaseImagePrices),
			SiteImagePrices:            plazaImagePricesToDTO(r.SiteImagePrices),
			Multiplier:                 r.Multiplier,
			DiscountPercent:            r.DiscountPercent,
		}
	}
	return out
}

func plazaImagePricesToDTO(p *service.PlazaImagePrices) *dto.PlazaImagePricesDTO {
	if p == nil {
		return nil
	}
	return &dto.PlazaImagePricesDTO{
		Tier1K: p.Tier1K,
		Tier2K: p.Tier2K,
		Tier4K: p.Tier4K,
	}
}

func plazaPlanCardsToDTO(cards []service.PlazaPlanCard) []dto.PlazaPlanCardDTO {
	out := make([]dto.PlazaPlanCardDTO, len(cards))
	for i, c := range cards {
		out[i] = dto.PlazaPlanCardDTO{
			ID:             c.ID,
			Name:           c.Name,
			Description:    c.Description,
			Price:          c.Price,
			OriginalPrice:  c.OriginalPrice,
			ValidityDays:   c.ValidityDays,
			ValidityUnit:   c.ValidityUnit,
			Features:       c.Features,
			GroupID:        c.GroupID,
			GroupName:      c.GroupName,
			Platform:       c.Platform,
			Models:         c.Models,
			ModelsOverflow: c.ModelsOverflow,
		}
	}
	return out
}
