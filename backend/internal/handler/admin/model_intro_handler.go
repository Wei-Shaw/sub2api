// Package admin provides admin handlers.
package admin

import (
	"sort"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// extractModelIntroKey 从 gin wildcard 参数解出 model_key。
// wildcard "*model_key" 匹配得到形如 "/gpt-4o" 或
// "/bytedance/seedance-2.5/text-to-video" 的字符串，需 trim 掉前置 "/"。
func extractModelIntroKey(c *gin.Context) string {
	raw := c.Param("model_key")
	return strings.TrimPrefix(raw, "/")
}

// ModelIntroHandler 暴露 admin 端的"模型介绍"CRUD 接口。
type ModelIntroHandler struct {
	svc            *service.ModelIntroService
	accountService *service.AccountService
}

// NewModelIntroHandler 构造 handler。
//
// accountService 用于 ListCandidates 接口：聚合所有 fal 账号中开启
// "支持视频模型"开关（Extra["fal_video_models_enabled"] == true）的
// 上游模型清单，供 admin 端在配置 model_intro 时下拉选择。
func NewModelIntroHandler(
	svc *service.ModelIntroService,
	accountService *service.AccountService,
) *ModelIntroHandler {
	return &ModelIntroHandler{
		svc:            svc,
		accountService: accountService,
	}
}

// adminModelIntroDTO 用作 List / Get / Create / Update 的返回体。
type adminModelIntroDTO struct {
	ModelKey    string `json:"model_key"`
	Title       string `json:"title"`
	Description string `json:"description"`
	// DescriptionEn 英文模型介绍（可为空）。与 Description 共同支持中英双文；
	// 前端展示时按当前 locale 选择，缺失时回落到另一语种。
	DescriptionEn string                    `json:"description_en"`
	CoverURL      string                    `json:"cover_url"`
	DefaultParams map[string]interface{}    `json:"default_params"`
	SortOrder     int                       `json:"sort_order"`
	Enabled       bool                      `json:"enabled"`
	OutputFields  []service.OutputFieldSpec `json:"output_fields"`
	ResultField   string                    `json:"result_field"`
	ResultType    string                    `json:"result_type"`
	CreatedAt     time.Time                 `json:"created_at"`
	UpdatedAt     time.Time                 `json:"updated_at"`
}

func toAdminModelIntroDTO(m *service.ModelIntro) *adminModelIntroDTO {
	if m == nil {
		return nil
	}
	params := m.DefaultParams
	if params == nil {
		params = map[string]interface{}{}
	}
	fields := m.OutputFields
	if fields == nil {
		fields = []service.OutputFieldSpec{}
	}
	return &adminModelIntroDTO{
		ModelKey:      m.ModelKey,
		Title:         m.Title,
		Description:   m.Description,
		DescriptionEn: m.DescriptionEn,
		CoverURL:      m.CoverURL,
		DefaultParams: params,
		SortOrder:     m.SortOrder,
		Enabled:       m.Enabled,
		OutputFields:  fields,
		ResultField:   m.ResultField,
		ResultType:    m.ResultType,
		CreatedAt:     m.CreatedAt,
		UpdatedAt:     m.UpdatedAt,
	}
}

// upsertModelIntroRequest 是 Create（含 model_key）/ Update 共用的 payload。
// Update 时 model_key 从 URL 取，请求体的 model_key 字段将被忽略。
type upsertModelIntroRequest struct {
	ModelKey    string `json:"model_key"`
	Title       string `json:"title"`
	Description string `json:"description"`
	// DescriptionEn 英文模型介绍；允许缺失（为空字符串）。
	DescriptionEn string                    `json:"description_en"`
	CoverURL      string                    `json:"cover_url"`
	DefaultParams map[string]interface{}    `json:"default_params"`
	SortOrder     int                       `json:"sort_order"`
	Enabled       bool                      `json:"enabled"`
	OutputFields  []service.OutputFieldSpec `json:"output_fields"`
	ResultField   string                    `json:"result_field"`
	ResultType    string                    `json:"result_type"`
}

func (r upsertModelIntroRequest) toServiceInput() service.UpsertModelIntroInput {
	return service.UpsertModelIntroInput{
		ModelKey:      r.ModelKey,
		Title:         r.Title,
		Description:   r.Description,
		DescriptionEn: r.DescriptionEn,
		CoverURL:      r.CoverURL,
		DefaultParams: r.DefaultParams,
		SortOrder:     r.SortOrder,
		Enabled:       r.Enabled,
		OutputFields:  r.OutputFields,
		ResultField:   r.ResultField,
		ResultType:    r.ResultType,
	}
}

// List GET /api/v1/admin/model-intros
func (h *ModelIntroHandler) List(c *gin.Context) {
	page, pageSize := response.ParsePagination(c)
	keyword := c.Query("keyword")
	rows, total, err := h.svc.List(c.Request.Context(), page, pageSize, keyword)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	out := make([]*adminModelIntroDTO, 0, len(rows))
	for _, r := range rows {
		out = append(out, toAdminModelIntroDTO(r))
	}
	response.Paginated(c, out, int64(total), page, pageSize)
}

// Get GET /api/v1/admin/model-intros/:model_key
//
// 使用 :model_key 作为路径参数；因为 model_key 可能含 /（如
// "bytedance/seedance-2.5/text-to-video"），前端在拼 URL 时用
// encodeURIComponent 编码后走 wildcard 路径。这里通过独立子路径
// 实现（见 routes/admin.go 中挂载方式）。
func (h *ModelIntroHandler) Get(c *gin.Context) {
	key := extractModelIntroKey(c)
	item, err := h.svc.Get(c.Request.Context(), key)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	if item == nil {
		response.NotFound(c, "model intro not found")
		return
	}
	response.Success(c, toAdminModelIntroDTO(item))
}

// Create POST /api/v1/admin/model-intros
func (h *ModelIntroHandler) Create(c *gin.Context) {
	var req upsertModelIntroRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	row, err := h.svc.Create(c.Request.Context(), req.toServiceInput())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Created(c, toAdminModelIntroDTO(row))
}

// Update PUT /api/v1/admin/model-intros/:model_key
func (h *ModelIntroHandler) Update(c *gin.Context) {
	key := extractModelIntroKey(c)
	var req upsertModelIntroRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	// 忽略 body 中的 model_key，以 URL 为准。
	req.ModelKey = key
	row, err := h.svc.Update(c.Request.Context(), key, req.toServiceInput())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, toAdminModelIntroDTO(row))
}

// Delete DELETE /api/v1/admin/model-intros/:model_key
func (h *ModelIntroHandler) Delete(c *gin.Context) {
	key := extractModelIntroKey(c)
	if err := h.svc.Delete(c.Request.Context(), key); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"message": "deleted"})
}

// modelCandidateDTO 是 GET /admin/model-intros/candidates 单条响应。
//
// 用途：admin 在配置 model_intro 时，除了自由输入外，还能下拉挑选来自
// 上游账号 model_mapping 的实际模型名。列表不携带定价/分组信息，仅用于
// 帮助管理员避免手抖打错模型名。
type modelCandidateDTO struct {
	ModelKey string `json:"model_key"`
	// AccountCount 有多少个账号声明支持该模型（去重后）。仅供参考展示。
	AccountCount int `json:"account_count"`
}

// ListCandidates GET /api/v1/admin/model-intros/candidates
//
// 聚合所有 fal 账号（不受 groupID 限制，管理员可看全量）中
// Extra["fal_video_models_enabled"] == true 的账号，从其
// model_mapping 中提取 fal endpoint，经 NormalizeFalVideoModelEndpoint
// 剥掉 "fal-ai/" 前缀后作为候选 model_key。
//
// 与用户端 GET /user/video-models 的差异：不做 groupID 过滤、不做定价
// 解析、不做状态过滤（Disabled/Error 的账号也计入，方便管理员配置将来
// 会恢复的账号）。
func (h *ModelIntroHandler) ListCandidates(c *gin.Context) {
	if h.accountService == nil {
		response.Success(c, gin.H{"items": []modelCandidateDTO{}, "total": 0})
		return
	}
	ctx := c.Request.Context()

	accounts, err := h.accountService.ListByPlatform(ctx, domain.PlatformFal)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	// 聚合：model_key（大小写不敏感 dedupe）-> 账号计数。
	counter := make(map[string]int, 16)
	original := make(map[string]string, 16)
	for i := range accounts {
		a := &accounts[i]
		if !domain.IsFalVideoModelsEnabled(a.Extra) {
			continue
		}
		mapping := a.GetModelMapping()
		if len(mapping) == 0 {
			continue
		}
		// 每个账号内先去重，避免同账号内多次 mapping 到同一 endpoint 时被重复计数。
		seenInAccount := make(map[string]struct{}, len(mapping))
		for _, endpoint := range mapping {
			slug := domain.NormalizeFalVideoModelEndpoint(endpoint)
			if slug == "" {
				continue
			}
			low := strings.ToLower(slug)
			if _, dup := seenInAccount[low]; dup {
				continue
			}
			seenInAccount[low] = struct{}{}
			if _, ok := original[low]; !ok {
				original[low] = slug
			}
			counter[low]++
		}
	}

	items := make([]modelCandidateDTO, 0, len(counter))
	for low, count := range counter {
		items = append(items, modelCandidateDTO{
			ModelKey:     original[low],
			AccountCount: count,
		})
	}
	sort.SliceStable(items, func(i, j int) bool {
		return items[i].ModelKey < items[j].ModelKey
	})

	response.Success(c, gin.H{
		"items": items,
		"total": len(items),
	})
}
