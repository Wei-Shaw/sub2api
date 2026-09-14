// adapter.go 实现 llm.RequestMessages 与 Devin Connect RPC 的双向转换，
// 并对一次 GetChatMessage 调用做传输重试 / pre-content 重开 / 静默看门狗。
//
// 整体移植 WncFht/devin2api 的 internal/adapter/devin（MIT）：
// connect-go 客户端替换为本仓库 pkg/devin 的手写 Connect-RPC wire 层，
// debuglog 记录器与本地速率闸门交给 sub2api 网关层（请求日志/账号熔断），
// 其余语义（start 扣留、reopen、stall watchdog、AssignModel 路由）保持不变。
package adapter

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math/rand"
	"net/http"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/devin"
	"github.com/Wei-Shaw/sub2api/internal/pkg/devin/llm"
)

// Doer 是 adapter 发出的每个 HTTP 请求的传输入口。由网关服务绑定
// 账号代理/并发/TLS 指纹后注入。返回值 resp 由调用方负责 Close。
type Doer func(req *http.Request) (*http.Response, error)

// Config 保存 Devin adapter 的固定上游配置。
type Config struct {
	// BaseURL 是 Devin Connect 服务的基础地址；空时回落 devin.DefaultBaseURL。
	BaseURL string
	// Token 是 Devin session token（devin-session-token$…）；不会写入日志。
	Token string
	// ClientVersion/ClientOS 是 metadata 身份字段；空时回落默认常量。
	ClientVersion string
	ClientOS      string
	// Do 是传输入口（必填）。
	Do Doer
}

// Adapter 调用 Devin 的 ApiServerService/GetChatMessage。
type Adapter struct {
	config Config
	// catalogMu 保护 models/groups 缓存与失败冷却窗口。
	catalogMu    sync.RWMutex
	models       []devin.Model
	groups       []devin.GroupedModel
	catalogUntil time.Time
	// catalogRetryUntil/catalogErr 是目录拉取失败的冷却窗口：失败期间
	// 目录始终为空，不冷却会让每个请求都重试 GetCliModelConfigs，
	// 客户端重试风暴原样穿透到上游。窗口内有旧缓存回旧值，否则回错误。
	catalogRetryUntil time.Time
	catalogErr        error
	// assignments 缓存 (router uid, cascade id) 的 AssignModel 解析结果：
	// assignment jwt 绑 cascade_id（上游实测），同会话内复用省去
	// 每请求一次的解析往返。
	assignmentsMu sync.Mutex
	assignments   map[string]resolvedAssignment
}

// resolvedAssignment 是 AssignModel 对单个 router uid 的解析结果。
type resolvedAssignment struct {
	modelUID string
	jwt      string
}

// catalogRetryBackoff 是模型目录拉取失败且错误未带 reset hint 时的
// 冷却时长；带 hint 时按 hint 冷却（上游何时解除它自己最清楚）。
const catalogRetryBackoff = 30 * time.Second

// New 创建 Devin adapter。
func New(config Config) (*Adapter, error) {
	if strings.TrimSpace(config.BaseURL) == "" {
		config.BaseURL = devin.DefaultBaseURL
	}
	if strings.TrimSpace(config.Token) == "" {
		return nil, errors.New("devin token is required")
	}
	if config.Do == nil {
		return nil, errors.New("devin transport doer is required")
	}
	return &Adapter{
		config:      config,
		assignments: make(map[string]resolvedAssignment),
	}, nil
}

func (a *Adapter) clientOS() string {
	if s := strings.TrimSpace(a.config.ClientOS); s != "" {
		return s
	}
	return devin.ClientOS()
}

func (a *Adapter) clientVersion() string {
	if s := strings.TrimSpace(a.config.ClientVersion); s != "" {
		return s
	}
	return devin.DefaultClientVersion
}

// doUnary 发送一个 unary Connect 请求并返回响应 body；
// 非 2xx 或协议错误统一归一为 *devin.ConnectError。
func (a *Adapter) doUnary(ctx context.Context, rpc string, body []byte) ([]byte, error) {
	req, err := devin.NewUnaryRequest(a.config.BaseURL, rpc, a.config.Token, body)
	if err != nil {
		return nil, err
	}
	resp, err := a.config.Do(req.WithContext(ctx))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	return devin.ReadUnaryResponse(resp)
}

// streamHTTP 发送流式请求；返回的帧读取器与响应体由调用方 Close。
// 非 2xx 时由 ReadStreamResponse 把 body 解析为 ConnectError。
func (a *Adapter) streamHTTP(ctx context.Context, rpc string, body []byte) (*devin.ConnectFrameReader, io.Closer, error) {
	req, err := devin.NewStreamRequest(a.config.BaseURL, rpc, a.config.Token, body)
	if err != nil {
		return nil, nil, err
	}
	resp, err := a.config.Do(req.WithContext(ctx))
	if err != nil {
		return nil, nil, err
	}
	reader, err := devin.ReadStreamResponse(resp)
	if err != nil {
		resp.Body.Close()
		return nil, nil, err
	}
	return reader, resp.Body, nil
}

// ListModels 返回当前账号的模型目录（分组视图），结果带 TTL 缓存。
// 写锁内复查后再拉取：TTL 过期瞬间的并发 miss 收敛为单次上游调用。
func (a *Adapter) ListModels(ctx context.Context) ([]devin.GroupedModel, error) {
	a.catalogMu.RLock()
	if a.groups != nil && time.Now().Before(a.catalogUntil) {
		groups := a.groups
		a.catalogMu.RUnlock()
		return groups, nil
	}
	a.catalogMu.RUnlock()

	a.catalogMu.Lock()
	defer a.catalogMu.Unlock()
	if a.groups != nil && time.Now().Before(a.catalogUntil) {
		return a.groups, nil
	}
	if time.Now().Before(a.catalogRetryUntil) {
		if a.groups != nil {
			return a.groups, nil
		}
		return nil, a.catalogErr
	}
	body, err := a.doUnary(ctx, devin.PathGetCliModelConfigs,
		devin.MarshalGetCliModelConfigs(a.config.Token, a.clientVersion(), a.clientOS()))
	if err != nil {
		// 客户端断连的 ctx 取消不是上游失败，不上冷却。
		if !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
			backoff := catalogRetryBackoff
			if seconds, ok := devin.RetryAfterSeconds(connectErrorText(err)); ok {
				backoff = time.Duration(seconds) * time.Second
			}
			a.catalogRetryUntil = time.Now().Add(backoff)
			a.catalogErr = err
		}
		// 目录刷新失败但有旧缓存时回旧值：catalog 缺席会让 router 判定与
		// 能力位校验同时失去依据，比数据稍旧危害更大。
		if a.groups != nil {
			slog.Warn("devin: model catalog refresh failed; serving stale cache", "error", err)
			return a.groups, nil
		}
		return nil, err
	}
	models := devin.DecodeModelCatalog(body)
	a.models = models
	a.groups = devin.GroupModels(models)
	a.catalogUntil = time.Now().Add(5 * time.Minute)
	a.catalogRetryUntil = time.Time{}
	a.catalogErr = nil
	return a.groups, nil
}

// catalogEntry 查询目录缓存中该 uid 的能力条目。
func (a *Adapter) catalogEntry(model string) (devin.Model, bool) {
	a.catalogMu.RLock()
	defer a.catalogMu.RUnlock()
	for _, m := range a.models {
		if m.ID == model {
			return m, true
		}
	}
	return devin.Model{}, false
}

// warnIfModelAbsentFromCatalog 在目录已加载且目标 uid 缺席时记 Warn。
// 实测 alias 指向死模型时上游只回模糊的 permission_denied: an internal
// error occurred——排障只能靠日志里的这条提示定位到 alias 目标。
func (a *Adapter) warnIfModelAbsentFromCatalog(model string) {
	a.catalogMu.RLock()
	defer a.catalogMu.RUnlock()
	if len(a.models) == 0 {
		return
	}
	for _, m := range a.models {
		if m.ID == model {
			return
		}
	}
	slog.Warn("devin: model absent from upstream catalog; upstream will likely return a vague permission_denied",
		"model", model, "hint", "check model uid or bump client_version")
}

// ensureCatalog 尽力保证模型目录已加载：router 判定与图片能力位校验以
// 目录为依据，目录从未加载过时这些检查静默失效。拉取失败放行，
// 维持「交给上游裁决」的旧行为。
func (a *Adapter) ensureCatalog(ctx context.Context) {
	if _, err := a.ListModels(ctx); err != nil {
		slog.Warn("devin: model catalog unavailable; router detection skipped", "error", err)
	}
}

// validateImagesForModel 在本地尽早拒绝「无视觉能力模型 + 图片」组合，
// 错误信息对客户端可读。目录中有该模型时以目录的 supports_images 为准
// （上游实测确实回 invalid_argument），目录未覆盖时退回前缀启发式。
func (a *Adapter) validateImagesForModel(request llm.RequestMessages, model string) error {
	if !requestHasImages(request) {
		return nil
	}
	supported, known := false, false
	if entry, ok := a.catalogEntry(model); ok {
		supported, known = entry.SupportsImages, true
	}
	if !known {
		supported = modelLikelySupportsImages(model)
	}
	if !supported {
		return fmt.Errorf("model %q does not support image inputs (supports_images=false); use a vision-capable model or remove images", model)
	}
	return nil
}

// resolveModelRouting 对目录里标了 is_model_router 的 uid 调 AssignModel
// 解出真实 model_uid 与绑定 cascade_id 的 assignment jwt——router uid
// 直连上游只回 unavailable: third-party model provider，伪装成瞬时错误
// 的永久失败。目录未覆盖该模型时按原样放行，交给上游裁决。
func (a *Adapter) resolveModelRouting(ctx context.Context, request llm.RequestMessages, model string) (resolved string, assignmentJWT string, err error) {
	isRouter := false
	if entry, ok := a.catalogEntry(model); ok {
		isRouter = entry.IsRouter
	}
	if !isRouter {
		return model, "", nil
	}
	// jwt 绑 cascade_id：必须用与本请求 wire 一致的派生值。
	_, cascadeID := deriveSessionIDs(request)
	assignment, err := a.assignModel(ctx, model, cascadeID)
	if err != nil {
		return "", "", err
	}
	slog.Info("devin: resolved model router via AssignModel", "router", model, "model", assignment.modelUID)
	return assignment.modelUID, assignment.jwt, nil
}

// assignModel 调上游 AssignModel 把 router uid 解析为真实模型 + assignment
// jwt，结果按 (router uid, cascade id) 缓存。
func (a *Adapter) assignModel(ctx context.Context, routerUID, cascadeID string) (resolvedAssignment, error) {
	key := routerUID + "|" + cascadeID
	a.assignmentsMu.Lock()
	cached, ok := a.assignments[key]
	a.assignmentsMu.Unlock()
	if ok {
		return cached, nil
	}
	body, err := a.doUnary(ctx, devin.PathAssignModel,
		devin.MarshalAssignModel(a.config.Token, a.clientVersion(), a.clientOS(), routerUID, cascadeID))
	if err != nil {
		// 保留 %w：上层按 devin.IsCode(err, …) 判定错误类别。
		return resolvedAssignment{}, fmt.Errorf("AssignModel(%s): %w", routerUID, err)
	}
	assignment, err := devin.DecodeAssignModelResponse(body)
	if err != nil || assignment.ModelUID == "" || assignment.AssignmentJWT == "" {
		return resolvedAssignment{}, fmt.Errorf("invalid_argument: AssignModel(%s) returned empty assignment", routerUID)
	}
	result := resolvedAssignment{modelUID: assignment.ModelUID, jwt: assignment.AssignmentJWT}
	a.assignmentsMu.Lock()
	// 有界缓存：会话级键随运行时长累积，触顶整体清空让会话重新解析。
	if len(a.assignments) >= 4096 {
		a.assignments = make(map[string]resolvedAssignment)
	}
	a.assignments[key] = result
	a.assignmentsMu.Unlock()
	return result, nil
}

func requestHasImages(request llm.RequestMessages) bool {
	for _, message := range request.Messages {
		var content []llm.Content
		switch m := message.(type) {
		case llm.UserMessage:
			content = m.Content
		case llm.ToolResultMessage:
			content = m.Content
		default:
			continue
		}
		for _, block := range content {
			if _, ok := block.(llm.ImageContent); ok {
				return true
			}
		}
	}
	return false
}

// modelLikelySupportsImages 用已知无视觉模型名单；不确定时放行让上游裁决。
func modelLikelySupportsImages(model string) bool {
	m := strings.ToLower(strings.TrimSpace(model))
	if m == "" {
		return true
	}
	// 与 GetCascadeModelConfigs.supports_images=false 的常见 uid 对齐。
	noVisionPrefixes := []string{
		"glm-5-2", "glm-5", "glm-4.7", "glm-4-7", "glm-4",
		"deepseek", "kimi-k2", "qwen3-coder",
	}
	for _, p := range noVisionPrefixes {
		if m == p || strings.HasPrefix(m, p+"-") || strings.HasPrefix(m, p+"_") {
			return false
		}
	}
	if strings.HasPrefix(m, "o1") || strings.HasPrefix(m, "o3-mini") || strings.HasPrefix(m, "o4-mini") {
		return false
	}
	return true
}

// promptTooLongPattern 匹配上游「上下文溢出」错误文案（插件实测集合）。
var promptTooLongPattern = regexp.MustCompile(`(?i)prompt is too long|too long for this model|context window|token limit`)

// isPromptTooLong 判断错误是否为「prompt 超上下文」——插件对该形态
// 做一次 overflow uid 重试。
func isPromptTooLong(err error) bool {
	return err != nil && promptTooLongPattern.MatchString(connectErrorText(err))
}

// flatCatalog 返回缓存的扁平目录条目（PickOverflowUID 的输入）。
func (a *Adapter) flatCatalog() []devin.Model {
	a.catalogMu.RLock()
	defer a.catalogMu.RUnlock()
	return a.models
}

// groupedCatalog 返回缓存的分组目录（无缓存时为 nil）。
func (a *Adapter) groupedCatalog() []devin.GroupedModel {
	a.catalogMu.RLock()
	defer a.catalogMu.RUnlock()
	return a.groups
}

// connectErrorText 提取错误文案（ConnectError 取 code: message 形态）。
func connectErrorText(err error) string {
	if err == nil {
		return ""
	}
	var connectErr *devin.ConnectError
	if errors.As(err, &connectErr) {
		msg := strings.TrimSpace(connectErr.Message)
		if msg == "" {
			return connectErr.Error()
		}
		return connectErr.Code + ": " + msg
	}
	return err.Error()
}

// Stream 将一份中间请求转换为 Devin RPC，并返回一份中间响应事件流。
func (a *Adapter) Stream(ctx context.Context, request llm.RequestMessages) (llm.ResponseStream, error) {
	if err := request.Validate(); err != nil {
		return nil, devin.NewInvalidRequest(fmt.Errorf("validate Devin request: %w", err))
	}
	request, _ = sanitizeRequest(request)
	model := strings.TrimSpace(request.Model)
	if model == "" {
		return nil, devin.NewInvalidRequest(errors.New("devin: request model is required"))
	}
	// 目录是 router 判定与能力位校验的依据；懒加载时此处补一次拉取。
	a.ensureCatalog(ctx)
	// 分组 id（swe-2）+ effort 档经 thinkingLevelMap 解析为上游 uid；
	// 原始 uid / 未知模型原样透传。语义同插件 resolveModelUid。
	if resolved := devin.ResolveCatalogModelUID(a.groupedCatalog(), model, request.Reasoning); resolved != model {
		slog.Info("devin: resolved grouped model", "from", model, "effort", request.Reasoning, "to", resolved)
		model = resolved
	}
	a.warnIfModelAbsentFromCatalog(model)
	if err := a.validateImagesForModel(request, model); err != nil {
		return nil, devin.NewInvalidRequest(err)
	}
	model, assignmentJWT, err := a.resolveModelRouting(ctx, request, model)
	if err != nil {
		var connectErr *devin.ConnectError
		if !errors.As(err, &connectErr) {
			err = devin.NewInvalidRequest(err)
		}
		return nil, err
	}
	params, _, err := buildRequestParams(request, a.config.Token, a.clientVersion(), a.clientOS(), model, assignmentJWT)
	if err != nil {
		return nil, devin.NewInvalidRequest(err)
	}
	// streamCtx 由 responseStream 持有：看门狗判死或客户端断开时
	// cancel 是唯一打断泵协程内阻塞读取的手段。
	streamCtx, cancel := context.WithCancel(ctx)
	conn, err := a.getChatMessageWithRetry(streamCtx, params)
	if err != nil && isPromptTooLong(err) {
		// 「prompt too long」按插件语义重试一次：换一个更大上下文的 uid
		// （DEVIN_OVERFLOW_MODEL 显式指定 > fusion sidekick 配对 > ≥1M 非路由条目）。
		overflow := strings.TrimSpace(os.Getenv("DEVIN_OVERFLOW_MODEL"))
		if overflow == model {
			overflow = ""
		}
		if overflow == "" {
			overflow = devin.PickOverflowUID(model, a.flatCatalog())
		}
		if overflow != "" {
			slog.Warn("devin: prompt too long; retrying with overflow uid", "from", model, "to", overflow)
			retryModel, retryJWT, routeErr := a.resolveModelRouting(ctx, request, overflow)
			if routeErr == nil {
				params.ModelUID = retryModel
				params.AssignmentJWT = retryJWT
				conn, err = a.getChatMessageWithRetry(streamCtx, params)
			}
		}
	}
	if err != nil {
		cancel()
		return nil, err
	}
	return &responseStream{
		frames:  pumpUpstream(streamCtx, conn),
		cancel:  cancel,
		decoder: newResponseDecoder(model, request.StopSequences, customToolNames(request.Tools)),
		// 上游流建立后、产出任何内容前的失败允许整体重发一次：
		// 传输层断裂重试能改变结果；语义错误（invalid_argument 等）
		// 重试只会复现同样失败，直接放行。
		reopen: func(cause error, continueEmpty bool) (<-chan upstreamFrame, context.CancelFunc, error) {
			retryRequest := request
			if continueEmpty {
				// 空 end_turn（有 stopReason 零内容，上游实测存在的退化形态）：
				// 追加 "continue" 用户消息重发一次，让模型在同一上下文续说。
				retryRequest.Messages = append(append([]llm.Message{}, request.Messages...),
					llm.UserMessage{Content: []llm.Content{llm.TextContent{Text: "continue"}}})
				slog.Warn("devin: reopening stream: upstream ended with empty content")
			} else if !devin.IsTransientTransportError(cause) {
				return nil, nil, cause
			} else {
				slog.Warn("devin: reopening stream: transport error before first content", "error", cause)
			}
			retryCtx, retryCancel := context.WithCancel(ctx)
			rebuilt, _, buildErr := buildRequestParams(retryRequest, a.config.Token, a.clientVersion(), a.clientOS(), model, assignmentJWT)
			var reopened streamConn
			if buildErr == nil {
				reopened, buildErr = a.getChatMessageWithRetry(retryCtx, rebuilt)
			}
			if buildErr != nil {
				retryCancel()
				return nil, nil, buildErr
			}
			return pumpUpstream(retryCtx, reopened), retryCancel, nil
		},
		newDecoder: func() *responseDecoder {
			return newResponseDecoder(model, request.StopSequences, customToolNames(request.Tools))
		},
	}, nil
}

// maxConnectAttempts 是 GetChatMessage 建立阶段对瞬时传输错误的最大尝试次数。
const maxConnectAttempts = 3

// streamConn 是建立成功的上游流：帧读取器 + 待关闭的响应体。
type streamConn struct {
	reader *devin.ConnectFrameReader
	body   io.Closer
}

// getChatMessageWithRetry 在流建立前重试瞬时传输错误（EOF/连接重置/超时）。
// 只对建立阶段重试：流一旦建立，错误通过事件流上报，不再重发请求。
func (a *Adapter) getChatMessageWithRetry(ctx context.Context, params devin.ChatRequestParams) (streamConn, error) {
	body := devin.MarshalGetChatMessage(params)
	var lastErr error
	for attempt := 0; attempt < maxConnectAttempts; attempt++ {
		if attempt > 0 {
			// ±25% 抖动：上游瞬时拥塞时固定节拍的重试会相互叠加。
			base := time.Duration(attempt) * 400 * time.Millisecond
			backoff := time.Duration(float64(base) * (0.75 + 0.5*rand.Float64()))
			select {
			case <-ctx.Done():
				return streamConn{}, context.Cause(ctx)
			case <-time.After(backoff):
			}
		}
		reader, closer, err := a.streamHTTP(ctx, devin.PathGetChatMessage, body)
		if err == nil {
			return streamConn{reader: reader, body: closer}, nil
		}
		lastErr = err
		if !devin.IsTransientTransportError(err) {
			break
		}
	}
	return streamConn{}, lastErr
}
