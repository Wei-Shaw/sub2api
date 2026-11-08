package admin

import (
	"errors"
	"sort"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

// UpstreamCostSettingsHandler 暴露「上游 A6 凭据」的读写接口。
//
// 为什么单独开一个 handler 而不是塞回原来那个：这段配置**只有一个消费者**——
// 后台按请求 ID 反查上游真实成本的那个取数任务。它有自己的生命周期与失败模式
// （凭据没配 = 成本永远取不到），和普通系统设置混在一起会让「成本为什么是空的」
// 这类问题变得难以定位。放在这里，接口路径本身就说明了它服务于什么。
//
// 只做转发与脱敏，所有校验（URL 合法性、令牌加密、覆盖优先级）都在
// service.ReconciliationA6SettingsService 里，不在这里重复实现一遍——
// 两处校验迟早会分叉，然后以「面板保存成功但实际没生效」的形式暴露出来。
type UpstreamCostSettingsHandler struct {
	settingsSvc *service.ReconciliationA6SettingsService
}

// NewUpstreamCostSettingsHandler 创建 A6 凭据设置 handler。
func NewUpstreamCostSettingsHandler(settingsSvc *service.ReconciliationA6SettingsService) *UpstreamCostSettingsHandler {
	return &UpstreamCostSettingsHandler{settingsSvc: settingsSvc}
}

// upstreamCostSettingsDTO 是 GET/PUT settings 的 data。
//
// 令牌只以「是否已配置 + 脱敏提示」两种形态出现，**任何情况下都不返回明文**：
// 这个结构体会被 JSON 序列化后发到浏览器，多一个字段就等于把令牌泄到前端。
type upstreamCostSettingsDTO struct {
	A6BaseURL         string   `json:"a6_base_url"`
	A6UserID          string   `json:"a6_user_id"`
	A6TokenConfigured bool     `json:"a6_token_configured"`
	A6TokenMask       string   `json:"a6_token_mask"`
	OverrideKeys      []string `json:"override_keys"`
}

func toUpstreamCostSettingsDTO(view service.ReconciliationA6SettingsView) upstreamCostSettingsDTO {
	// 前端直接遍历 override_keys，null 会让它崩掉。
	keys := view.OverrideKeys
	if keys == nil {
		keys = []string{}
	}
	return upstreamCostSettingsDTO{
		A6BaseURL:         view.A6BaseURL,
		A6UserID:          view.A6UserID,
		A6TokenConfigured: view.A6TokenConfigured,
		A6TokenMask:       view.A6TokenMask,
		OverrideKeys:      keys,
	}
}

// upstreamCostSettingsInputDTO 是 PUT settings 的入参。
//
// 全部字段可选（nil = 不改动），与 service 层入参的指针语义一一对应：
// 「没传这个字段」和「传了空串」必须能区分开，否则前端无法表达
// 「我只想改地址、令牌保持不动」——那会导致每次保存都把令牌清空。
type upstreamCostSettingsInputDTO struct {
	A6BaseURL          *string `json:"a6_base_url"`
	A6UserID           *string `json:"a6_user_id"`
	A6AccessToken      *string `json:"a6_access_token"`
	ClearA6AccessToken bool    `json:"clear_a6_access_token"`
}

// Settings 返回 A6 上游配置的生效状态。
//
// GET /admin/usage/upstream-cost/settings
func (h *UpstreamCostSettingsHandler) Settings(c *gin.Context) {
	if h == nil || h.settingsSvc == nil {
		response.InternalError(c, "UPSTREAM_COST_INTERNAL: A6 设置服务未装配")
		return
	}
	response.Success(c, toUpstreamCostSettingsDTO(h.settingsSvc.Effective(c.Request.Context()).View()))
}

// UpdateSettings 保存 A6 上游配置覆盖值。
//
// PUT /admin/usage/upstream-cost/settings
func (h *UpstreamCostSettingsHandler) UpdateSettings(c *gin.Context) {
	if h == nil || h.settingsSvc == nil {
		response.InternalError(c, "UPSTREAM_COST_INTERNAL: A6 设置服务未装配")
		return
	}

	var input upstreamCostSettingsInputDTO
	if err := c.ShouldBindJSON(&input); err != nil {
		response.BadRequest(c, "UPSTREAM_COST_BAD_REQUEST: 请求体不是合法的 JSON: "+err.Error())
		return
	}

	settings, err := h.settingsSvc.Update(c.Request.Context(), service.ReconciliationA6SettingsInput{
		BaseURL:          input.A6BaseURL,
		UserID:           input.A6UserID,
		AccessToken:      input.A6AccessToken,
		ClearAccessToken: input.ClearA6AccessToken,
	})
	if err != nil {
		writeUpstreamCostSettingsError(c, err)
		return
	}

	response.Success(c, toUpstreamCostSettingsDTO(settings.View()))
}

// upstreamBlockInputDTO 是「拉黑上游商户 / 渠道模型」的入参。
//
// 刻意由前端把 supplier_id / channel_id / model 直接带上来，而不是后端按 usage log id 回查：
//   - 这几个值本来就已经显示在那一行上，前端手上就有，回查纯属多跑一趟库；
//   - 后端回查要给本 handler 额外注入 usage 仓储，而它的职责就是"与上游 A6 打交道"，
//     多一条读库路径只会让依赖变宽、也让「谁能改上游」变得含糊。
//
// 代价是这些值会被拼进上游 URL 路径，所以下面逐项校验，不信任前端。
type upstreamBlockInputDTO struct {
	// Scope 拉黑范围："supplier" = 整个商户；"channel_model" = 某渠道的某个模型。
	Scope string `json:"scope"`
	// SupplierID 上游商户 ID（scope=supplier 时必填）。
	SupplierID int `json:"supplier_id"`
	// ChannelID 上游渠道 ID（scope=channel_model 时必填）。
	ChannelID int `json:"channel_id"`
	// Model 模型名（scope=channel_model 时必填）。
	Model string `json:"model"`
	// Reason 拉黑理由。2026-11-04 起上游 A6 把它改成必填项
	// （不带就回 user_supplier_block_reason_required 400），
	// 所以这里允许前端传，留空时由服务端补默认理由。
	Reason string `json:"reason"`
}

// BlockUpstream 在上游 A6 侧拉黑「整个商户」或「某渠道的某模型」。
//
// POST /admin/usage/upstream-block
//
// ⚠️ 这是**有副作用的写操作**：拉黑整个商户会让该商户名下所有渠道退出路由，
// 且**该商户名下的固定绑定不会自动改绑**（会变成悬空）。前端在调用前必须让操作者
// 二次确认并显示影响面。
//
// 不做在途重试：第一次可能已经成功、只是响应丢失，重试就会重复调用；
// 失败原因（令牌失效/无权限）从上游原话里带出来，交给人决定。
func (h *UpstreamCostSettingsHandler) BlockUpstream(c *gin.Context) {
	if h == nil || h.settingsSvc == nil {
		response.InternalError(c, "UPSTREAM_COST_INTERNAL: A6 设置服务未装配")
		return
	}

	var input upstreamBlockInputDTO
	if err := c.ShouldBindJSON(&input); err != nil {
		response.BadRequest(c, "UPSTREAM_BLOCK_BAD_REQUEST: 请求体不是合法的 JSON: "+err.Error())
		return
	}

	// 每次都用**当前生效**的凭据构造客户端：管理员刚在面板上改完令牌，
	// 下一次点按钮就该用上，不该等到进程重启。
	cfg := h.settingsSvc.Effective(c.Request.Context()).ClientConfig()
	client := service.NewA6Client(cfg)
	if !client.Configured() {
		response.BadRequest(c, "UPSTREAM_BLOCK_NOT_CONFIGURED: 上游 A6 凭据未配置，无法执行拉黑")
		return
	}

	scope := strings.TrimSpace(input.Scope)
	// 上游把 reason 变成必填之后，前端不传也不能让按钮坏掉：统一在这里兜底。
	reason := strings.TrimSpace(input.Reason)
	var err error
	switch scope {
	case "supplier":
		if input.SupplierID <= 0 {
			response.BadRequest(c, "UPSTREAM_BLOCK_BAD_REQUEST: 缺少商户 ID")
			return
		}
		logger.LegacyPrintf("handler.admin.upstream_block",
			"[UpstreamBlock] 请求拉黑整个商户: supplier_id=%d reason=%q", input.SupplierID, reason)
		err = client.BlockSupplier(c.Request.Context(), input.SupplierID, reason)
	case "channel_model":
		model := strings.TrimSpace(input.Model)
		if input.ChannelID <= 0 || model == "" {
			response.BadRequest(c, "UPSTREAM_BLOCK_BAD_REQUEST: 缺少渠道 ID 或模型名")
			return
		}
		logger.LegacyPrintf("handler.admin.upstream_block",
			"[UpstreamBlock] 请求拉黑渠道模型: channel_id=%d model=%s reason=%q", input.ChannelID, model, reason)
		err = client.DisableChannelModel(c.Request.Context(), input.ChannelID, model, reason)
	default:
		response.BadRequest(c, "UPSTREAM_BLOCK_BAD_REQUEST: scope 必须是 supplier 或 channel_model")
		return
	}

	if err != nil {
		// 把上游原话带回去：401/403 的原因（令牌失效、无权限）只在响应体里，
		// 换成笼统的「拉黑失败」会让操作者只能靠猜。
		logger.LegacyPrintf("handler.admin.upstream_block",
			"[UpstreamBlock] 拉黑失败: scope=%s supplier_id=%d channel_id=%d err=%v",
			scope, input.SupplierID, input.ChannelID, err)
		response.InternalError(c, "UPSTREAM_BLOCK_FAILED: 上游拉黑失败: "+err.Error())
		return
	}

	logger.LegacyPrintf("handler.admin.upstream_block",
		"[UpstreamBlock] 拉黑成功: scope=%s supplier_id=%d channel_id=%d model=%s",
		scope, input.SupplierID, input.ChannelID, strings.TrimSpace(input.Model))
	response.Success(c, gin.H{"scope": scope})
}

// upstreamBlocksDTO 是「上游当前黑名单」的返回体。
//
// 用**扁平的两个列表**而不是把黑名单整条透传：前端要回答的只是"这一行的商户/渠道
// 是否已在黑名单里"，集合结构最省事（可直接建 Set 做 O(1) 判断）。
//
// ⚠️ 渠道级黑名单上游可达上千条（实测 942 条），这是**每个页面加载都要拉一次**的
// 只读数据，因此刻意只回 ID 键、不带名称等展示字段。
type upstreamBlocksDTO struct {
	SupplierIDs      []int    `json:"supplier_ids"`
	ChannelModelKeys []string `json:"channel_model_keys"`
}

// UpstreamBlocks 读取上游当前黑名单，供页面把已拉黑的按钮置灰。
//
// GET /admin/usage/upstream-blocks
//
// 为什么每次都现问上游、不在本库另存一份：黑名单的权威在上游。本地存一份，
// 管理员直接在上游后台拉黑或恢复时本地不会知道，页面就会长期显示错的状态——
// 而"以为已经拉黑了"比"显示未拉黑"更危险。
//
// 读不到时**返回空集合而不是报错**：这只是给按钮上色的辅助信息，拉不到最多是
// 按钮不置灰（仍可点击，点击时上游会拒绝），不该因此让整张使用记录表加载失败。
func (h *UpstreamCostSettingsHandler) UpstreamBlocks(c *gin.Context) {
	out := upstreamBlocksDTO{SupplierIDs: []int{}, ChannelModelKeys: []string{}}
	if h == nil || h.settingsSvc == nil {
		response.Success(c, out)
		return
	}

	cfg := h.settingsSvc.Effective(c.Request.Context()).ClientConfig()
	client := service.NewA6Client(cfg)
	if !client.Configured() {
		response.Success(c, out)
		return
	}

	blocks, err := client.ListBlocks(c.Request.Context())
	if err != nil {
		// 记录原因但照常返回空集合：置灰是锦上添花，不能拖垮主列表。
		logger.LegacyPrintf("handler.admin.upstream_block",
			"[UpstreamBlocks] 读取上游黑名单失败（本次不做置灰）: err=%v", err)
		response.Success(c, out)
		return
	}

	for id := range blocks.SupplierIDs {
		out.SupplierIDs = append(out.SupplierIDs, id)
	}
	for key := range blocks.ChannelModelKeys {
		out.ChannelModelKeys = append(out.ChannelModelKeys, key)
	}
	sort.Ints(out.SupplierIDs)
	sort.Strings(out.ChannelModelKeys)
	response.Success(c, out)
}

// writeUpstreamCostSettingsError 把设置服务的哨兵错误翻译成 HTTP 状态码。
// 逐条映射而不是一律 500：这些错误的处置方式完全不同——URL 填错要人改，
// 加密器缺席要找运维。全都回 500 的话，面板只能显示「服务器错误」，
// 而真正该看到那句话的人（正在填表单的管理员）拿不到任何可行动的提示。
func writeUpstreamCostSettingsError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, service.ErrReconciliationInvalidBaseURL):
		response.BadRequest(c, "UPSTREAM_COST_INVALID_BASE_URL: A6 站点地址必须是合法的 http/https 地址")
	case errors.Is(err, service.ErrReconciliationInvalidFxRate):
		response.BadRequest(c, "UPSTREAM_COST_INVALID_FX_RATE: 汇率取值越界")
	case errors.Is(err, service.ErrReconciliationSecretEncryptorUnavailable):
		// 拒绝落明文是**有意**的：这是一张普通键值表，明文令牌会被任何一次
		// 库备份带走。这里必须把原因说清楚，否则运维会以为是随机故障。
		response.InternalError(c, "UPSTREAM_COST_ENCRYPTOR_UNAVAILABLE: 加密器未配置，拒绝以明文保存令牌")
	case errors.Is(err, service.ErrReconciliationSettingsStoreUnavailable):
		response.InternalError(c, "UPSTREAM_COST_STORE_UNAVAILABLE: 配置存储不可用")
	default:
		response.InternalError(c, "UPSTREAM_COST_INTERNAL: 保存 A6 配置失败: "+err.Error())
	}
}
