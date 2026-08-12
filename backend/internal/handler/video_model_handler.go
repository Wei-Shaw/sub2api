package handler

import (
	"context"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

// VideoModelHandler 用户端视频模型只读接口。
//
// 数据来源：
//  1. 从 JWT 上下文取当前用户 ID → APIKeyService.GetAvailableGroups 拿到用户可访问的 group 集合；
//  2. 拉取 fal / atlascloud / apiz 三类视频平台账号；
//  3. 过滤条件：账号状态 = active、GroupIDs 与用户 group 集合有交集、
//     Extra["video_models_enabled"] == true；
//  4. 从 account.GetModelMapping() 的 value 中提取 fal endpoint，
//     经 NormalizeFalVideoModelEndpoint（剥掉 "fal-ai/"、要求 ≥2 段）后作为对外模型名；
//  5. 对每个模型，遍历用户可访问的 group，用 ModelPricingResolver.Resolve 拿视频定价
//     （BillingMode=video），把 Intervals[].TierLabel/PerRequestPrice 拍平为
//     [{resolution, price_per_second}]；同一 model 首次解析到的定价胜出，
//     实现 Q-C: "配什么展示什么"。
type VideoModelHandler struct {
	apiKeyService     *service.APIKeyService
	accountRepo       service.AccountRepository
	pricingResolver   *service.ModelPricingResolver
	modelIntroService *service.ModelIntroService
	videoService      *service.AsyncVideoService
}

// NewVideoModelHandler 构造视频模型只读 handler。
//
// 依赖均为已在 wire 中就位的现成组件；本 handler 无独立状态。
// modelIntroService 可选：为 nil 时 List 不携带 intro 信息，保持向下兼容。
// videoService 用于查询演练台历史任务（Q3-1 B 方案：按 slug 过滤当前用户历史）。
func NewVideoModelHandler(
	apiKeyService *service.APIKeyService,
	accountRepo service.AccountRepository,
	pricingResolver *service.ModelPricingResolver,
	modelIntroService *service.ModelIntroService,
	videoService *service.AsyncVideoService,
) *VideoModelHandler {
	return &VideoModelHandler{
		apiKeyService:     apiKeyService,
		accountRepo:       accountRepo,
		pricingResolver:   pricingResolver,
		modelIntroService: modelIntroService,
		videoService:      videoService,
	}
}

// videoModelPricingItem 单档视频定价。
//   - Resolution     分辨率标签（渠道 pricing_intervals.tier_label，原样透出，如 "480p"/"720p"）
//   - PricePerSecond 每秒单价（USD）
//   - Currency       固定 "USD"
//   - Enabled        当前档位是否可用（>0 视为可用）
type videoModelPricingItem struct {
	Resolution     string  `json:"resolution"`
	PricePerSecond float64 `json:"price_per_second"`
	Currency       string  `json:"currency"`
	Enabled        bool    `json:"enabled"`
}

// videoModelIntroDTO 是管理员在“模型介绍”菜单里为该 model_key 配置的
// 展示信息（封面图/描述/默认参数/输出字段声明）。model_intros.enabled=false 时不下发，
// 前端拿到 nil 时应退化为一个普通卡片。
//
// OutputFields 供演练台在任务完成后按声明的字段提取并渲染（string / number / boolean / object / array，遵循 JSON Schema 标准）。
// ResultField / ResultType 指示"主结果字段"：ResultField 为空时前端取第一个
// video/image 类型的字段作为主结果；非空时强制以 ResultType 渲染该字段。
type videoModelIntroDTO struct {
	Title string `json:"title"`
	// Description 中文模型介绍；DescriptionEn 为英文对应项。
	// 前端按当前 locale 选择展示，任一侧为空时自动回落到另一侧。
	Description   string                    `json:"description"`
	DescriptionEn string                    `json:"description_en"`
	CoverURL      string                    `json:"cover_url"`
	DefaultParams map[string]any            `json:"default_params"`
	OutputFields  []service.OutputFieldSpec `json:"output_fields"`
	ResultField   string                    `json:"result_field"`
	ResultType    string                    `json:"result_type"`
}

// videoModelItem 是 GET /user/video-models 单条响应的 DTO。
type videoModelItem struct {
	Slug        string                  `json:"slug"`
	Family      string                  `json:"family"`
	Variant     string                  `json:"variant"`
	DisplayName string                  `json:"display_name"`
	SubmitPath  string                  `json:"submit_path"`
	StatusPath  string                  `json:"status_path"`
	ResultPath  string                  `json:"result_path"`
	CancelPath  string                  `json:"cancel_path"`
	Pricing     []videoModelPricingItem `json:"pricing"`
	Available   bool                    `json:"available"`
	Intro       *videoModelIntroDTO     `json:"intro,omitempty"`
}

// List GET /user/video-models 列出当前用户可用的视频模型。
//
// 认证：由 middleware.JWTAuthMiddleware 保护，未登录会在中间件层直接 401。
//
// 错误策略：
//   - GetAvailableGroups 失败 → 500，防止空静默；
//   - ListByPlatform 失败 → 500；
//   - 无任何 fal 账号或无匹配开关 → 200 + 空列表（不算错误）。
func (h *VideoModelHandler) List(c *gin.Context) {
	subject, ok := middleware.GetAuthSubjectFromContext(c)
	if !ok || subject.UserID <= 0 {
		response.Error(c, http.StatusUnauthorized, "unauthorized")
		return
	}

	ctx := c.Request.Context()

	// 1. 当前用户可访问的 group 集合。
	userGroups, err := h.apiKeyService.GetAvailableGroups(ctx, subject.UserID)
	if err != nil {
		response.Error(c, http.StatusInternalServerError, "load available groups: "+err.Error())
		return
	}
	if len(userGroups) == 0 {
		respondEmptyVideoModels(c)
		return
	}
	groupSet := make(map[int64]struct{}, len(userGroups))
	groupIDList := make([]int64, 0, len(userGroups))
	for i := range userGroups {
		groupSet[userGroups[i].ID] = struct{}{}
		groupIDList = append(groupIDList, userGroups[i].ID)
	}

	// 2. 所有视频平台账号（含非 active 的一并拉，稍后按 status 过滤）。
	accounts := make([]service.Account, 0, 16)
	for _, platform := range []string{domain.PlatformFal, domain.PlatformAtlasCloud, domain.PlatformApiz} {
		platformAccounts, listErr := h.accountRepo.ListByPlatform(ctx, platform)
		if listErr != nil {
			response.Error(c, http.StatusInternalServerError, "list "+platform+" accounts: "+listErr.Error())
			return
		}
		accounts = append(accounts, platformAccounts...)
	}

	// 3+4. 过滤并聚合：模型名去重（大小写不敏感），首次出现的原始大小写胜出。
	type seenValue struct{}
	seen := make(map[string]seenValue, 16)
	items := make([]videoModelItem, 0, 16)

	for ai := range accounts {
		a := &accounts[ai]
		if a.Status != service.StatusActive {
			continue
		}
		if !accountBelongsToAny(a, groupSet) {
			continue
		}
		if !domain.IsVideoModelsEnabled(a.Extra) {
			continue
		}
		for _, slug := range videoModelSlugsForAccount(a) {
			low := strings.ToLower(slug)
			if _, dup := seen[low]; dup {
				continue
			}
			seen[low] = seenValue{}
			items = append(items, h.buildVideoModelItem(ctx, slug, groupIDList))
		}
	}

	sort.SliceStable(items, func(i, j int) bool {
		return items[i].Slug < items[j].Slug
	})

	response.Success(c, gin.H{
		"items":                 items,
		"total":                 len(items),
		"supported_resolutions": []string{"480p", "720p", "1080p", "4k"},
	})
}

// videoModelSlugsForAccount 返回账号对外暴露的统一视频模型名。
// fal 的 mapping value 是 endpoint；atlascloud/apiz 的 key 是对外模型名，
// value 则是各自上游的内部模型标识。
func videoModelSlugsForAccount(account *service.Account) []string {
	if account == nil {
		return nil
	}
	return domain.VideoModelSlugs(account.Platform, account.GetModelMapping())
}

// respondEmptyVideoModels 统一空列表响应，避免多处重复。
func respondEmptyVideoModels(c *gin.Context) {
	response.Success(c, gin.H{
		"items":                 []videoModelItem{},
		"total":                 0,
		"supported_resolutions": []string{"480p", "720p", "1080p", "4k"},
	})
}

// accountBelongsToAny 判断账号是否属于给定 group 集合之一。
func accountBelongsToAny(a *service.Account, groupSet map[int64]struct{}) bool {
	if a == nil || len(groupSet) == 0 {
		return false
	}
	for _, gid := range a.GroupIDs {
		if _, ok := groupSet[gid]; ok {
			return true
		}
	}
	return false
}

// buildVideoModelItem 从 slug 拆分 family/variant 并组装完整 item，含渠道视频定价。
//
// 定价解析（Q-C: 配什么展示什么）：
//   - 遍历用户 groupID 列表，逐个 Resolve(model, groupID)；
//   - 只接受 Mode == BillingModeVideo 的解析结果；
//   - 首个命中的分组作为该 model 的定价来源（多分组不合并、不去重档位）；
//   - 把 RequestTiers[].TierLabel/PerRequestPrice 原样拍平；
//   - DefaultPerRequestPrice > 0 时追加一档 resolution="default"，作为未命中档位的兜底提示。
func (h *VideoModelHandler) buildVideoModelItem(ctx context.Context, slug string, groupIDs []int64) videoModelItem {
	parts := strings.Split(slug, "/")
	family := ""
	variant := ""
	if len(parts) >= 2 {
		family = parts[1]
	} else if len(parts) == 1 {
		family = parts[0]
	}
	if len(parts) >= 1 {
		variant = parts[len(parts)-1]
	}

	pricing := h.resolveVideoPricing(ctx, slug, groupIDs)
	intro := h.resolveModelIntro(ctx, slug)

	return videoModelItem{
		Slug:        slug,
		Family:      family,
		Variant:     variant,
		DisplayName: slug,
		SubmitPath:  "/api/v1/model/" + slug,
		StatusPath:  "",
		ResultPath:  "",
		CancelPath:  "",
		Pricing:     pricing,
		Available:   true,
		Intro:       intro,
	}
}

// resolveModelIntro 以 slug 作为 model_key 去 model_intros 表中查展示信息。
//
// 返回 nil 的情况：
//   - service 未注入（兼容历史部署）
//   - 未配置该 model_key 的介绍
//   - 配置了但 enabled=false（管理员下线了展示）
//   - 读取报错（降级为无 intro，不阻断整个列表接口）
func (h *VideoModelHandler) resolveModelIntro(ctx context.Context, slug string) *videoModelIntroDTO {
	if h.modelIntroService == nil {
		return nil
	}
	intro, err := h.modelIntroService.Get(ctx, slug)
	if err != nil || intro == nil || !intro.Enabled {
		return nil
	}
	fields := intro.OutputFields
	if fields == nil {
		fields = []service.OutputFieldSpec{}
	}
	return &videoModelIntroDTO{
		Title:         intro.Title,
		Description:   intro.Description,
		DescriptionEn: intro.DescriptionEn,
		CoverURL:      intro.CoverURL,
		DefaultParams: intro.DefaultParams,
		OutputFields:  fields,
		ResultField:   intro.ResultField,
		ResultType:    intro.ResultType,
	}
}

// resolveVideoPricing 用 ModelPricingResolver 从用户所有 group 中查找该模型的视频定价。
//
// 保证返回值非 nil（空列表 = 尚未配置定价）。
func (h *VideoModelHandler) resolveVideoPricing(ctx context.Context, slug string, groupIDs []int64) []videoModelPricingItem {
	if h.pricingResolver == nil {
		return []videoModelPricingItem{}
	}
	for _, gid := range groupIDs {
		g := gid
		resolved := h.pricingResolver.Resolve(ctx, service.PricingInput{
			Model:   slug,
			GroupID: &g,
		})
		if resolved == nil || resolved.Mode != service.BillingModeVideo {
			continue
		}
		out := make([]videoModelPricingItem, 0, len(resolved.RequestTiers)+1)
		for _, tier := range resolved.RequestTiers {
			if tier.PerRequestPrice == nil {
				continue
			}
			label := strings.TrimSpace(tier.TierLabel)
			if label == "" {
				continue
			}
			out = append(out, videoModelPricingItem{
				Resolution:     label,
				PricePerSecond: *tier.PerRequestPrice,
				Currency:       "USD",
				Enabled:        *tier.PerRequestPrice > 0,
			})
		}
		if resolved.DefaultPerRequestPrice > 0 {
			out = append(out, videoModelPricingItem{
				Resolution:     "default",
				PricePerSecond: resolved.DefaultPerRequestPrice,
				Currency:       "USD",
				Enabled:        true,
			})
		}
		return out
	}
	return []videoModelPricingItem{}
}

// videoTaskItem 是 GET /user/video-models/tasks 单条响应 DTO。
//
// 保持字段扁平（避免嵌套 map[string]any）以便前端稳定绑定；request_payload /
// result_payload 原样透传，以便"重放"按钮把 request_payload 塞回演练表单。
type videoTaskItem struct {
	ID                int64          `json:"id"`
	InternalRequestID string         `json:"internal_request_id"`
	UpstreamRequestID string         `json:"upstream_request_id"`
	RequestedModel    string         `json:"requested_model"`
	Status            string         `json:"status"`
	Resolution        string         `json:"resolution"`
	DurationSeconds   int            `json:"duration_seconds"`
	AspectRatio       string         `json:"aspect_ratio"`
	FinalCost         float64        `json:"final_cost"`
	HeldCost          float64        `json:"held_cost"`
	ErrorReason       string         `json:"error_reason"`
	VideoURLs         []string       `json:"video_urls"`
	CosURLs           []string       `json:"cos_urls"`
	RequestPayload    map[string]any `json:"request_payload"`
	ResultPayload     map[string]any `json:"result_payload"`
	CreatedAt         string         `json:"created_at"`
	FinishedAt        string         `json:"finished_at,omitempty"`
}

// ListTasks GET /user/video-models/tasks 返回当前用户在指定 slug 下的历史任务。
//
// Query 参数：
//   - slug     : 模型 slug（Q3-1 B 方案，必填；空串则拒绝，防止跨模型串扰）
//   - page     : 页码（默认 1）
//   - page_size: 每页条数（默认 20，最大 100）
//
// 认证：走同一 JWT 中间件；service 层内 SQL 强制 WHERE user_id=? 保证不会跨用户读取。
func (h *VideoModelHandler) ListTasks(c *gin.Context) {
	subject, ok := middleware.GetAuthSubjectFromContext(c)
	if !ok || subject.UserID <= 0 {
		response.Error(c, http.StatusUnauthorized, "unauthorized")
		return
	}
	if h.videoService == nil {
		response.Error(c, http.StatusInternalServerError, "video service unavailable")
		return
	}

	slug := strings.TrimSpace(c.Query("slug"))
	if slug == "" {
		// Q3-1 采用 B 方案：必须携带 slug，避免误查全部历史。
		response.Error(c, http.StatusBadRequest, "missing 'slug'")
		return
	}

	page := parseIntQuery(c, "page", 1, 1, 1_000_000)
	pageSize := parseIntQuery(c, "page_size", 20, 1, 100)
	offset := (page - 1) * pageSize

	tasks, total, err := h.videoService.ListByUserAndSlug(c.Request.Context(), subject.UserID, slug, offset, pageSize)
	if err != nil {
		response.Error(c, http.StatusInternalServerError, "list video tasks: "+err.Error())
		return
	}

	items := make([]videoTaskItem, 0, len(tasks))
	for _, t := range tasks {
		items = append(items, toVideoTaskItem(t))
	}
	response.Success(c, gin.H{
		"items":     items,
		"total":     total,
		"page":      page,
		"page_size": pageSize,
	})
}

// GetTaskByRequestID GET /user/video-models/tasks/by-request/:rid
//
// 用途：演练台任务终态后前端拉一次拿"实扣费用" final_cost + 上游 result_payload。
// 权限：强制 WHERE user_id = subject.UserID，避免通过 request_id 横向越权。
func (h *VideoModelHandler) GetTaskByRequestID(c *gin.Context) {
	subject, ok := middleware.GetAuthSubjectFromContext(c)
	if !ok || subject.UserID <= 0 {
		response.Error(c, http.StatusUnauthorized, "unauthorized")
		return
	}
	if h.videoService == nil {
		response.Error(c, http.StatusInternalServerError, "video service unavailable")
		return
	}
	rid := strings.TrimSpace(c.Param("rid"))
	if rid == "" {
		response.Error(c, http.StatusBadRequest, "missing 'rid'")
		return
	}
	task, err := h.videoService.GetTaskByInternalID(c.Request.Context(), rid)
	if err != nil {
		response.Error(c, http.StatusInternalServerError, "get video task: "+err.Error())
		return
	}
	if task == nil || task.UserID != subject.UserID {
		// 保持"未找到"和"非本人任务"响应一致，防止探测存在性。
		response.Error(c, http.StatusNotFound, "task not found")
		return
	}
	response.Success(c, toVideoTaskItem(task))
}

// GetTaskByID GET /user/video-models/tasks/:id
//
// 用途：使用记录页视频行"详情"入口——usage_logs.task_id 存的就是 async_video_tasks.id，
// 前端拿到 task_id 后调本接口拉完整任务信息（upstream_request_id、prompt、result_payload 等）。
// 权限：与 GetTaskByRequestID 相同——强制 WHERE user_id = subject.UserID，
// 找不到或非本人任务统一 404，防止通过 id 递增探测存在性。
func (h *VideoModelHandler) GetTaskByID(c *gin.Context) {
	subject, ok := middleware.GetAuthSubjectFromContext(c)
	if !ok || subject.UserID <= 0 {
		response.Error(c, http.StatusUnauthorized, "unauthorized")
		return
	}
	if h.videoService == nil {
		response.Error(c, http.StatusInternalServerError, "video service unavailable")
		return
	}
	idStr := strings.TrimSpace(c.Param("id"))
	if idStr == "" {
		response.Error(c, http.StatusBadRequest, "missing 'id'")
		return
	}
	id, parseErr := strconv.ParseInt(idStr, 10, 64)
	if parseErr != nil || id <= 0 {
		response.Error(c, http.StatusBadRequest, "invalid 'id'")
		return
	}
	task, err := h.videoService.GetTaskByID(c.Request.Context(), id)
	if err != nil {
		response.Error(c, http.StatusInternalServerError, "get video task: "+err.Error())
		return
	}
	if task == nil || task.UserID != subject.UserID {
		response.Error(c, http.StatusNotFound, "task not found")
		return
	}
	response.Success(c, toVideoTaskItem(task))
}

// GetTaskByIDAdmin GET /admin/video-tasks/by-id/:id
//
// 管理员版按数据库主键查询任意用户的视频任务详情。与用户版 GetTaskByID 相比：
//   - 不强制 task.user_id 归属校验（管理员可查看所有用户的任务）；
//   - 上游 AdminAuthMiddleware 已保证只有管理员能进入本接口。
//
// 用途：管理员使用记录页视频行"详情"入口。
func (h *VideoModelHandler) GetTaskByIDAdmin(c *gin.Context) {
	if h.videoService == nil {
		response.Error(c, http.StatusInternalServerError, "video service unavailable")
		return
	}
	idStr := strings.TrimSpace(c.Param("id"))
	if idStr == "" {
		response.Error(c, http.StatusBadRequest, "missing 'id'")
		return
	}
	id, parseErr := strconv.ParseInt(idStr, 10, 64)
	if parseErr != nil || id <= 0 {
		response.Error(c, http.StatusBadRequest, "invalid 'id'")
		return
	}
	task, err := h.videoService.GetTaskByID(c.Request.Context(), id)
	if err != nil {
		response.Error(c, http.StatusInternalServerError, "get video task: "+err.Error())
		return
	}
	if task == nil {
		response.Error(c, http.StatusNotFound, "task not found")
		return
	}
	response.Success(c, toVideoTaskItem(task))
}

// toVideoTaskItem 将领域模型映射为对外 DTO；nil 指针字段展开为空字符串，避免前端处理 optional。
func toVideoTaskItem(t *service.AsyncVideoTask) videoTaskItem {
	if t == nil {
		return videoTaskItem{}
	}
	deref := func(p *string) string {
		if p == nil {
			return ""
		}
		return *p
	}
	item := videoTaskItem{
		ID:                t.ID,
		InternalRequestID: t.InternalRequestID,
		UpstreamRequestID: deref(t.UpstreamRequestID),
		RequestedModel:    t.RequestedModel,
		Status:            t.Status,
		Resolution:        deref(t.Resolution),
		DurationSeconds:   t.DurationSeconds,
		AspectRatio:       deref(t.AspectRatio),
		FinalCost:         t.FinalCost,
		HeldCost:          t.HeldCost,
		ErrorReason:       service.SanitizeVideoErrorReason(deref(t.ErrorReason)),
		VideoURLs:         t.VideoURLs,
		CosURLs:           t.CosURLs,
		RequestPayload:    t.RequestPayload,
		ResultPayload:     t.ResultPayload,
		CreatedAt:         t.CreatedAt.UTC().Format("2006-01-02T15:04:05Z"),
	}
	if t.FinishedAt != nil && !t.FinishedAt.IsZero() {
		item.FinishedAt = t.FinishedAt.UTC().Format("2006-01-02T15:04:05Z")
	}
	if item.VideoURLs == nil {
		item.VideoURLs = []string{}
	}
	if item.CosURLs == nil {
		item.CosURLs = []string{}
	}
	return item
}

// parseIntQuery 是 c.Query 的整数化封装，越界回落默认值。
// 独立小函数，避免 handler 内散落解析逻辑。
func parseIntQuery(c *gin.Context, key string, def, min, max int) int {
	raw := strings.TrimSpace(c.Query(key))
	if raw == "" {
		return def
	}
	n := 0
	for _, r := range raw {
		if r < '0' || r > '9' {
			return def
		}
		n = n*10 + int(r-'0')
		if n > max {
			return max
		}
	}
	if n < min {
		return def
	}
	return n
}
