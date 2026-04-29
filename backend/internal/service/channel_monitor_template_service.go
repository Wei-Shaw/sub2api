package service

import (
	"context"
	"fmt"
	"regexp"
	"strings"
)

// ChannelMonitorRequestTemplateRepository 模板数据访问接口。
type ChannelMonitorRequestTemplateRepository interface {
	Create(ctx context.Context, t *ChannelMonitorRequestTemplate) error
	GetByID(ctx context.Context, id int64) (*ChannelMonitorRequestTemplate, error)
	Update(ctx context.Context, t *ChannelMonitorRequestTemplate) error
	Delete(ctx context.Context, id int64) error
	List(ctx context.Context, params ChannelMonitorRequestTemplateListParams) ([]*ChannelMonitorRequestTemplate, error)
	// ApplyToMonitors 把模板当前的 extra_headers / body_override_mode / body_override
	// 批量覆盖到指定 monitorIDs 的监控上（同时还要求这些监控当前 template_id = id，
	// 防止误覆盖未关联的监控）。monitorIDs 必须非空；空列表直接返回 0 不写库。
	// 返回被覆盖的监控数量。
	ApplyToMonitors(ctx context.Context, id int64, monitorIDs []int64) (int64, error)
	// CountAssociatedMonitors 统计 template_id = id 的监控数（用于 UI 展示「应用到 N 个配置」）。
	CountAssociatedMonitors(ctx context.Context, id int64) (int64, error)
	// ListAssociatedMonitors 列出所有 template_id = id 的监控简略信息（id/name/provider/enabled）
	// 给 apply picker UI 用，避免前端再做一次 list+filter。
	ListAssociatedMonitors(ctx context.Context, id int64) ([]*AssociatedMonitorBrief, error)
}

// AssociatedMonitorBrief 模板关联监控的简略信息（picker / 列表展示用）。
type AssociatedMonitorBrief struct {
	ID       int64
	Name     string
	Provider string
	Enabled  bool
}

// ChannelMonitorRequestTemplateService 模板管理 service。
type ChannelMonitorRequestTemplateService struct {
	repo ChannelMonitorRequestTemplateRepository
}

// NewChannelMonitorRequestTemplateService 创建模板 service。
func NewChannelMonitorRequestTemplateService(repo ChannelMonitorRequestTemplateRepository) *ChannelMonitorRequestTemplateService {
	return &ChannelMonitorRequestTemplateService{repo: repo}
}

// ---------- CRUD ----------

// List 按 provider 过滤（空串 = 全部），不分页（模板量级小）。
func (s *ChannelMonitorRequestTemplateService) List(ctx context.Context, params ChannelMonitorRequestTemplateListParams) ([]*ChannelMonitorRequestTemplate, error) {
	if params.Provider != "" {
		if err := validateProvider(params.Provider); err != nil {
			return nil, err
		}
	}
	return s.repo.List(ctx, params)
}

// Get 返回单个模板。
func (s *ChannelMonitorRequestTemplateService) Get(ctx context.Context, id int64) (*ChannelMonitorRequestTemplate, error) {
	return s.repo.GetByID(ctx, id)
}

// Create 创建模板（会校验 headers 黑名单和 body 模式匹配）。
func (s *ChannelMonitorRequestTemplateService) Create(ctx context.Context, p ChannelMonitorRequestTemplateCreateParams) (*ChannelMonitorRequestTemplate, error) {
	if err := validateTemplateCreateParams(p); err != nil {
		return nil, err
	}
	t := &ChannelMonitorRequestTemplate{
		Name:             strings.TrimSpace(p.Name),
		Provider:         p.Provider,
		Description:      strings.TrimSpace(p.Description),
		ExtraHeaders:     emptyHeadersIfNil(p.ExtraHeaders),
		BodyOverrideMode: defaultBodyMode(p.BodyOverrideMode),
		BodyOverride:     p.BodyOverride,
	}
	if err := s.repo.Create(ctx, t); err != nil {
		return nil, fmt.Errorf("create template: %w", err)
	}
	return t, nil
}

// Update 更新模板（provider 不可改）。
func (s *ChannelMonitorRequestTemplateService) Update(ctx context.Context, id int64, p ChannelMonitorRequestTemplateUpdateParams) (*ChannelMonitorRequestTemplate, error) {
	existing, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if err := applyTemplateUpdate(existing, p); err != nil {
		return nil, err
	}
	if err := s.repo.Update(ctx, existing); err != nil {
		return nil, fmt.Errorf("update template: %w", err)
	}
	return existing, nil
}

// Delete 删除模板。关联监控的 template_id 会被 SET NULL，监控保留快照继续跑。
func (s *ChannelMonitorRequestTemplateService) Delete(ctx context.Context, id int64) error {
	if err := s.repo.Delete(ctx, id); err != nil {
		return fmt.Errorf("delete template: %w", err)
	}
	return nil
}

// ApplyToMonitors 把模板当前配置应用到 monitorIDs 列表里的关联监控。
// monitorIDs 必须非空且每个 id 都必须当前 template_id = id；不满足条件的会被 SQL WHERE 过滤掉。
// 返回实际被覆盖的监控数。
func (s *ChannelMonitorRequestTemplateService) ApplyToMonitors(ctx context.Context, id int64, monitorIDs []int64) (int64, error) {
	if _, err := s.repo.GetByID(ctx, id); err != nil {
		return 0, err
	}
	if len(monitorIDs) == 0 {
		return 0, ErrChannelMonitorTemplateApplyEmpty
	}
	affected, err := s.repo.ApplyToMonitors(ctx, id, monitorIDs)
	if err != nil {
		return 0, fmt.Errorf("apply template to monitors: %w", err)
	}
	return affected, nil
}

// CountAssociatedMonitors 返回关联监控数。
func (s *ChannelMonitorRequestTemplateService) CountAssociatedMonitors(ctx context.Context, id int64) (int64, error) {
	return s.repo.CountAssociatedMonitors(ctx, id)
}

// ListAssociatedMonitors 返回模板关联的所有监控简略信息。
// 给前端 apply picker 用，handler 直接吐 JSON 不再做 join。
func (s *ChannelMonitorRequestTemplateService) ListAssociatedMonitors(ctx context.Context, id int64) ([]*AssociatedMonitorBrief, error) {
	if _, err := s.repo.GetByID(ctx, id); err != nil {
		return nil, err
	}
	return s.repo.ListAssociatedMonitors(ctx, id)
}

// ---------- 校验 & 工具 ----------

// validateTemplateCreateParams 聚合 create 入参校验，避免函数超过 30 行。
func validateTemplateCreateParams(p ChannelMonitorRequestTemplateCreateParams) error {
	if strings.TrimSpace(p.Name) == "" {
		return ErrChannelMonitorTemplateMissingName
	}
	if err := validateProvider(p.Provider); err != nil {
		return ErrChannelMonitorTemplateInvalidProvider
	}
	if err := validateBodyModeParams(p.BodyOverrideMode, p.BodyOverride); err != nil {
		return err
	}
	if err := validateExtraHeaders(p.ExtraHeaders); err != nil {
		return err
	}
	return nil
}

// applyTemplateUpdate 把 update params 中非 nil 字段应用到 existing 上。
func applyTemplateUpdate(existing *ChannelMonitorRequestTemplate, p ChannelMonitorRequestTemplateUpdateParams) error {
	if p.Name != nil {
		name := strings.TrimSpace(*p.Name)
		if name == "" {
			return ErrChannelMonitorTemplateMissingName
		}
		existing.Name = name
	}
	if p.Description != nil {
		existing.Description = strings.TrimSpace(*p.Description)
	}
	if p.ExtraHeaders != nil {
		if err := validateExtraHeaders(*p.ExtraHeaders); err != nil {
			return err
		}
		existing.ExtraHeaders = emptyHeadersIfNil(*p.ExtraHeaders)
	}
	// BodyOverrideMode / BodyOverride 联合校验：任一变化都用「更新后的值」做校验。
	newMode := existing.BodyOverrideMode
	newBody := existing.BodyOverride
	if p.BodyOverrideMode != nil {
		newMode = *p.BodyOverrideMode
	}
	if p.BodyOverride != nil {
		newBody = *p.BodyOverride
	}
	if err := validateBodyModeParams(newMode, newBody); err != nil {
		return err
	}
	existing.BodyOverrideMode = defaultBodyMode(newMode)
	existing.BodyOverride = newBody
	return nil
}

// validateBodyModeParams 校验 body_override_mode 合法，且 merge/replace 模式下 body_override 非空。
func validateBodyModeParams(mode string, body map[string]any) error {
	switch mode {
	case "", MonitorBodyOverrideModeOff:
		return nil
	case MonitorBodyOverrideModeMerge, MonitorBodyOverrideModeReplace:
		if len(body) == 0 {
			return ErrChannelMonitorTemplateBodyRequired
		}
		return nil
	default:
		return ErrChannelMonitorTemplateInvalidBodyMode
	}
}

// headerNameRegex 合法 header 名：RFC 7230 token（ASCII 可见字符减特殊符号）。
var headerNameRegex = regexp.MustCompile(`^[A-Za-z0-9!#$%&'*+\-.^_` + "`" + `|~]+$`)

// forbiddenHeaderNames hop-by-hop + HTTP 客户端自管的 header；禁止用户覆盖，
// 否则会让 Go http.Client 行为异常（双重 Content-Length、连接复用错乱等）。
var forbiddenHeaderNames = map[string]bool{
	"host":              true,
	"content-length":    true,
	"content-encoding":  true,
	"transfer-encoding": true,
	"connection":        true,
}

// IsForbiddenHeaderName 对外暴露，checker 运行时也会再过滤一次做兜底。
func IsForbiddenHeaderName(name string) bool {
	return forbiddenHeaderNames[strings.ToLower(strings.TrimSpace(name))]
}

// validateExtraHeaders 校验 header 名字格式 + 黑名单。保存时就拒绝非法 header，早失败。
func validateExtraHeaders(h map[string]string) error {
	for k := range h {
		if !headerNameRegex.MatchString(k) {
			return ErrChannelMonitorTemplateHeaderInvalidName
		}
		if IsForbiddenHeaderName(k) {
			return ErrChannelMonitorTemplateHeaderForbidden
		}
	}
	return nil
}

// emptyHeadersIfNil 把 nil map 归一成空 map（repo 层写库时 JSONB 需要非 nil）。
func emptyHeadersIfNil(h map[string]string) map[string]string {
	if h == nil {
		return map[string]string{}
	}
	return h
}

// defaultBodyMode 空串归一为 off。
func defaultBodyMode(mode string) string {
	if mode == "" {
		return MonitorBodyOverrideModeOff
	}
	return mode
}
