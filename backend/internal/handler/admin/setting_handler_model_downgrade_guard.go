package admin

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/handler/dto"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// ModelDowngradeGuardManager 是模型降级守卫管理页需要的运行时能力：列出受限账号与
// 观察记录、提前解除一条限制（或清除一条观察记录）、把观察记录立即转成真实处理。
// 生产实现是 *service.RateLimitService；抽成接口方便 handler 单测注入假实现。
type ModelDowngradeGuardManager interface {
	ListModelDowngradeBlocked(ctx context.Context) (*service.ModelDowngradeBlockedList, error)
	ReleaseModelDowngradeBlock(ctx context.Context, accountID int64, scope, model string) error
	ApplyModelDowngradeBlockNow(ctx context.Context, accountID int64, sentModel string) (*service.ModelDowngradeManualBlockResult, error)
}

// SetModelDowngradeGuardManager 注入模型降级守卫的运行时依赖（可选），
// 不改动构造函数签名，避免影响现有单测。
func (h *SettingHandler) SetModelDowngradeGuardManager(manager ModelDowngradeGuardManager) {
	h.modelDowngradeGuard = manager
}

// GetModelDowngradeGuardSettings 获取模型降级守卫配置
// GET /api/v1/admin/settings/model-downgrade-guard
func (h *SettingHandler) GetModelDowngradeGuardSettings(c *gin.Context) {
	settings, err := h.settingService.GetModelDowngradeGuardSettings(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	response.Success(c, modelDowngradeGuardSettingsDTO(settings))
}

// UpdateModelDowngradeGuardSettingsRequest 更新模型降级守卫配置请求
type UpdateModelDowngradeGuardSettingsRequest struct {
	Enabled                bool                     `json:"enabled"`
	Action                 string                   `json:"action"`
	Pairs                  []dto.ModelDowngradePair `json:"pairs"`
	ThresholdCount         int                      `json:"threshold_count"`
	ThresholdWindowMinutes int                      `json:"threshold_window_minutes"`
	BlockHours             int                      `json:"block_hours"`
	MaxBlockedRatio        float64                  `json:"max_blocked_ratio"`
}

// UpdateModelDowngradeGuardSettings 更新模型降级守卫配置
// PUT /api/v1/admin/settings/model-downgrade-guard
func (h *SettingHandler) UpdateModelDowngradeGuardSettings(c *gin.Context) {
	var req UpdateModelDowngradeGuardSettingsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}

	pairs := make([]service.ModelDowngradePair, 0, len(req.Pairs))
	for _, pair := range req.Pairs {
		pairs = append(pairs, service.ModelDowngradePair{
			SentModel:     pair.SentModel,
			ResponseModel: pair.ResponseModel,
		})
	}

	settings := &service.ModelDowngradeGuardSettings{
		Enabled:                req.Enabled,
		Action:                 req.Action,
		Pairs:                  pairs,
		ThresholdCount:         req.ThresholdCount,
		ThresholdWindowMinutes: req.ThresholdWindowMinutes,
		BlockHours:             req.BlockHours,
		MaxBlockedRatio:        req.MaxBlockedRatio,
	}

	if err := h.settingService.SetModelDowngradeGuardSettings(c.Request.Context(), settings); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	// 重新获取设置返回（Get 会做钳制修正，前端拿到的是最终生效值）
	updatedSettings, err := h.settingService.GetModelDowngradeGuardSettings(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	response.Success(c, modelDowngradeGuardSettingsDTO(updatedSettings))
}

func modelDowngradeGuardSettingsDTO(settings *service.ModelDowngradeGuardSettings) dto.ModelDowngradeGuardSettings {
	pairs := make([]dto.ModelDowngradePair, 0, len(settings.Pairs))
	for _, pair := range settings.Pairs {
		pairs = append(pairs, dto.ModelDowngradePair{
			SentModel:     pair.SentModel,
			ResponseModel: pair.ResponseModel,
		})
	}
	return dto.ModelDowngradeGuardSettings{
		Enabled:                settings.Enabled,
		Action:                 settings.Action,
		Pairs:                  pairs,
		ThresholdCount:         settings.ThresholdCount,
		ThresholdWindowMinutes: settings.ThresholdWindowMinutes,
		BlockHours:             settings.BlockHours,
		MaxBlockedRatio:        settings.MaxBlockedRatio,
	}
}

// GetModelDowngradeGuardBlocked 列出当前因降级受限的账号与观察记录
// GET /api/v1/admin/settings/model-downgrade-guard/blocked
func (h *SettingHandler) GetModelDowngradeGuardBlocked(c *gin.Context) {
	if h.modelDowngradeGuard == nil {
		response.InternalError(c, "model downgrade guard is not configured")
		return
	}

	result, err := h.modelDowngradeGuard.ListModelDowngradeBlocked(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	items := make([]dto.ModelDowngradeBlockedAccount, 0, len(result.Items))
	for _, item := range result.Items {
		status := item.Status
		if status == "" {
			// 没有标注状态的行就是真实受限。
			status = service.ModelDowngradeBlockedStatusBlocked
		}
		row := dto.ModelDowngradeBlockedAccount{
			AccountID:            item.AccountID,
			AccountName:          item.AccountName,
			Scope:                item.Scope,
			Status:               status,
			Cause:                item.Cause,
			Model:                item.Model,
			SentModel:            item.SentModel,
			ResponseModel:        item.ResponseModel,
			TriggerCount:         item.TriggerCount,
			TriggerThreshold:     item.TriggerThreshold,
			TriggerWindowMinutes: item.TriggerWindowMinutes,
			Until:                item.Until.UTC().Format(time.RFC3339),
			Blocked:              item.Blocked,
			Total:                item.Total,
			MaxBlockedRatio:      item.MaxBlockedRatio,
		}
		if !item.TriggeredAt.IsZero() {
			row.TriggeredAt = item.TriggeredAt.UTC().Format(time.RFC3339)
		}
		items = append(items, row)
	}

	response.Success(c, dto.ModelDowngradeBlockedResponse{
		Items: items,
		Summary: dto.ModelDowngradeBlockedSummary{
			Blocked:         result.Summary.Blocked,
			Observed:        result.Summary.Observed,
			TotalActive:     result.Summary.TotalActive,
			MaxBlockedRatio: result.Summary.MaxBlockedRatio,
		},
	})
}

// ReleaseModelDowngradeGuardBlocked 提前解除一条降级守卫写的限制，或清除一条观察记录
// DELETE /api/v1/admin/settings/model-downgrade-guard/blocked/:id?scope=account|model|observed&model=<model>
//
// 刻意不复用 DELETE /admin/accounts/:id/temp-unschedulable：后者会连带清空整个
// extra.model_rate_limits，会误伤图片模型等无关来源的 429 冷却。
func (h *SettingHandler) ReleaseModelDowngradeGuardBlocked(c *gin.Context) {
	if h.modelDowngradeGuard == nil {
		response.InternalError(c, "model downgrade guard is not configured")
		return
	}

	accountID, err := strconv.ParseInt(strings.TrimSpace(c.Param("id")), 10, 64)
	if err != nil || accountID <= 0 {
		response.BadRequest(c, "invalid account id")
		return
	}

	scope := strings.TrimSpace(c.Query("scope"))
	if scope == "" {
		scope = service.ModelDowngradeBlockedScopeAccount
	}
	model := strings.TrimSpace(c.Query("model"))

	if err := h.modelDowngradeGuard.ReleaseModelDowngradeBlock(c.Request.Context(), accountID, scope, model); err != nil {
		response.ErrorFrom(c, err)
		return
	}

	response.Success(c, dto.ModelDowngradeBlockedReleaseResponse{
		AccountID: accountID,
		Scope:     scope,
		Model:     model,
	})
}

// ApplyModelDowngradeGuardBlocked 把一条观察记录立即转成真实处理
// POST /api/v1/admin/settings/model-downgrade-guard/blocked/:id/apply?model=<sent_model>
//
// 切换处理方式时不会自动处理历史观察记录（旧记录可能已经不代表现状），
// 这个接口就是那条「人工转正」的路。动作按当前配置，观察模式下按默认动作（仅屏蔽该模型）。
func (h *SettingHandler) ApplyModelDowngradeGuardBlocked(c *gin.Context) {
	if h.modelDowngradeGuard == nil {
		response.InternalError(c, "model downgrade guard is not configured")
		return
	}

	accountID, err := strconv.ParseInt(strings.TrimSpace(c.Param("id")), 10, 64)
	if err != nil || accountID <= 0 {
		response.BadRequest(c, "invalid account id")
		return
	}

	// 观察记录按账号 + 发往上游模型存储，模型名是定位记录的必要条件。
	model := strings.TrimSpace(c.Query("model"))
	if model == "" {
		response.BadRequest(c, "model is required")
		return
	}

	result, err := h.modelDowngradeGuard.ApplyModelDowngradeBlockNow(c.Request.Context(), accountID, model)
	if err != nil {
		// 比例上限拦下时 ErrorFrom 会带上 blocked / total / max_blocked_ratio 元数据，
		// 前端据此提示到底卡在哪。
		response.ErrorFrom(c, err)
		return
	}

	response.Success(c, dto.ModelDowngradeBlockedApplyResponse{
		AccountID: result.AccountID,
		Scope:     result.Scope,
		Model:     result.Model,
		Until:     result.Until.UTC().Format(time.RFC3339),
	})
}
