package routes

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	pkghttputil "github.com/Wei-Shaw/sub2api/internal/pkg/httputil"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// SmartRoutingPlanner 为智能路由分组构建有序成员候选计划。
// GatewayService.BuildSmartRoutingPlan 实现该接口。
type SmartRoutingPlanner interface {
	BuildSmartRoutingPlan(ctx context.Context, smartGroup *service.Group, requestedModel string) ([]service.SmartRoutingCandidate, error)
}

// smartRoutingDispatcher 让绑定「智能路由分组」的 API Key 透明地聚合多个成员分组：
//
//   - 请求先发给优先级最高的成员分组；该分组失败（无可用账号、上游不可重试错误等）
//     时，按优先级从高到低逐个回退重试，直到成功或候选耗尽；
//   - 同一优先级层内按权重加权随机选择首个候选，其余候选作为回退顺序；
//   - 每次尝试都以成员分组身份执行（平台调度、账号选择、计费倍率、用量归属
//     全部按实际服务的成员分组计算）。
//
// 为避免把失败响应直接写给客户端，分发器为每次尝试挂一个缓冲式 ResponseWriter：
// 只有当处理链真正开始向下游流式输出（首次 Flush/劫持连接）或本次尝试成功时，
// 才把缓冲内容落盘；失败且可重试时丢弃缓冲并切换下一个候选。
//
// 注意：分发器原地改写传入的 *gin.Context（Keys/Request/Writer/Errors），而不是
// 拷贝一份新的 Context——gin.Context 内含互斥锁，拷贝会触发 copylocks 检查，且
// 终端 handler 不会调用 c.Next()/c.Abort()/IsAborted()，原地改写是安全的。
type smartRoutingDispatcher struct {
	planner           SmartRoutingPlanner
	compositeResolver *service.CompositeRouteResolver
}

func newSmartRoutingDispatcher(planner SmartRoutingPlanner, compositeResolver *service.CompositeRouteResolver) *smartRoutingDispatcher {
	return &smartRoutingDispatcher{planner: planner, compositeResolver: compositeResolver}
}

// wrap 包装网关终端 handler。非智能路由 Key 原样透传，零额外开销。
func (d *smartRoutingDispatcher) wrap(next gin.HandlerFunc) gin.HandlerFunc {
	return func(c *gin.Context) {
		apiKey, ok := middleware.GetAPIKeyFromContext(c)
		if d == nil || d.planner == nil || !ok || apiKey == nil || apiKey.Group == nil || !apiKey.Group.IsSmartRouting() {
			next(c)
			return
		}

		// 读取并复位请求体，供模型提取与每次尝试重复使用。
		body, err := pkghttputil.ReadRequestBodyWithPrealloc(c.Request)
		if err != nil {
			status := http.StatusBadRequest
			message := "Failed to read request body"
			var maxErr *http.MaxBytesError
			if errors.As(err, &maxErr) {
				status = http.StatusRequestEntityTooLarge
				message = "Request body is too large"
			}
			c.JSON(status, gin.H{"error": gin.H{"type": "invalid_request_error", "message": message}})
			c.Abort()
			return
		}
		model := compositeRequestModelFromBody(c.GetHeader("Content-Type"), body)
		resetRequestBody(c, body)

		plan, err := d.planner.BuildSmartRoutingPlan(c.Request.Context(), apiKey.Group, model)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{"type": "server_error", "message": "Failed to build smart routing plan"}})
			c.Abort()
			return
		}
		if len(plan) == 0 {
			service.MarkOpsClientBusinessLimited(c, service.OpsClientBusinessLimitedReasonLocalFeatureGate)
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": gin.H{"type": "api_error", "message": "Smart routing group has no available member groups"}})
			c.Abort()
			return
		}

		baseCtx := c.Request.Context()
		baseKeys := copyGinKeys(c.Keys)
		origWriter := c.Writer
		origRequest := c.Request

		var lastRecorder *smartRouteRecorder
		for _, cand := range plan {
			// 每次尝试都从认证后的基础 request context 派生，隔离上次尝试写入的临时值，
			// 并把成员分组写入分组 context（覆盖智能路由分组自身）。
			attemptCtx := context.WithValue(baseCtx, ctxkey.Group, cand.Group)
			attemptBody := body
			// 成员本身是 composite 分组时，网关级 compositeTarget 中间件已在智能路由
			// Key 阶段跳过，需在这里按成员分组补做模型路由解析与模型改写。
			if cand.Group.Platform == service.PlatformComposite {
				attemptCtx, attemptBody = d.resolveCompositeMember(c, cand.Group, model, baseCtx, attemptBody)
			}

			rec := newSmartRouteRecorder(origWriter)
			lastRecorder = rec
			prepareSmartRoutingAttempt(c, attemptCtx, baseKeys, origRequest, apiKey, cand.Group, attemptBody, rec)

			next(c)

			// 已开始向客户端输出（流式/劫持）或结果不可重试：定稿该尝试。
			if rec.committed || !smartRoutingShouldRetryStatus(rec.status) {
				rec.flushToTarget()
				c.Writer = origWriter
				return
			}
			// 客户端已断开：不再发起后续尝试。
			if baseCtx.Err() != nil {
				c.Writer = origWriter
				return
			}
		}

		// 全部候选失败：把最后一次尝试的错误响应返回给客户端。
		// c 上已保留最后一次尝试的 Keys/Request（供后置中间件读取真实归属）。
		if lastRecorder != nil {
			lastRecorder.flushToTarget()
		}
		c.Writer = origWriter
	}
}

// resolveCompositeMember 为 composite 成员分组解析模型路由，返回携带路由决策的
// context 与（可能已改写模型的）请求体。解析失败时按原始 body 继续，交给成员
// 分组的调度器自行报错。
func (d *smartRoutingDispatcher) resolveCompositeMember(c *gin.Context, member *service.Group, model string, baseCtx context.Context, body []byte) (context.Context, []byte) {
	if d.compositeResolver == nil {
		return baseCtx, body
	}
	decision, err := d.compositeResolver.Resolve(baseCtx, member.ID, model, compositeRouteEndpointForPath(c.Request.URL.Path))
	if err != nil || !decision.Matched {
		return baseCtx, body
	}
	attemptCtx := service.WithCompositeRouteDecision(baseCtx, decision)
	if upstreamModel := strings.TrimSpace(decision.UpstreamModel); upstreamModel != "" && upstreamModel != model && gjson.ValidBytes(body) {
		if _, modelPath := compositeJSONRequestModel(body); modelPath != "" {
			if rewritten, rewriteErr := sjson.SetBytes(body, modelPath, upstreamModel); rewriteErr == nil {
				body = rewritten
			}
		}
	}
	return attemptCtx, body
}

// prepareSmartRoutingAttempt 原地把 *gin.Context 切换到某个成员分组的一次尝试：
// 重置 Keys（写入成员 API Key）、替换 Request（新 body + 成员分组 context）、
// 清空 Errors、挂上缓冲 ResponseWriter。
func prepareSmartRoutingAttempt(
	c *gin.Context,
	attemptCtx context.Context,
	baseKeys map[string]any,
	origRequest *http.Request,
	apiKey *service.APIKey,
	member *service.Group,
	body []byte,
	rec *smartRouteRecorder,
) {
	// Keys 重置为认证后基础键的副本，隔离上一次尝试写入的临时键，再写入成员 API Key。
	c.Keys = make(map[string]any, len(baseKeys)+1)
	for k, v := range baseKeys {
		c.Keys[k] = v
	}
	keyCopy := *apiKey
	memberID := member.ID
	keyCopy.Group = member
	keyCopy.GroupID = &memberID
	c.Set(string(middleware.ContextKeyAPIKey), &keyCopy)

	req := origRequest.WithContext(attemptCtx)
	req.Header = origRequest.Header.Clone()
	req.Body = io.NopCloser(bytes.NewReader(body))
	req.ContentLength = int64(len(body))
	req.Header.Set("Content-Length", strconv.Itoa(len(body)))
	req.GetBody = func() (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(body)), nil
	}
	c.Request = req

	c.Errors = nil
	c.Writer = rec
}

func copyGinKeys(src map[string]any) map[string]any {
	if src == nil {
		return map[string]any{}
	}
	out := make(map[string]any, len(src))
	for k, v := range src {
		out[k] = v
	}
	return out
}

// smartRoutingShouldRetryStatus 判断某次尝试的响应状态是否应触发跨分组回退。
// 仅对「换个分组可能成功」的瞬时/容量/模型缺失类错误回退；
// 4xx 客户端错误（鉴权、请求体非法、余额不足等）对任何分组都一致，直接返回。
func smartRoutingShouldRetryStatus(status int) bool {
	switch status {
	case http.StatusNotFound, // 模型在当前分组无可用账号
		http.StatusRequestTimeout,
		http.StatusTooEarly,
		http.StatusTooManyRequests, // 分组/账号侧限流
		http.StatusBadGateway,
		http.StatusServiceUnavailable,
		http.StatusGatewayTimeout:
		return true
	}
	// 5xx（含 Anthropic overload 529 等）一律回退；0 表示未写响应，按成功透传。
	return status >= http.StatusInternalServerError && status != 0
}

// smartRouteRecorder 是缓冲式 gin.ResponseWriter：
// 在「提交」前把所有写入留在内存里，供跨分组回退时丢弃失败响应；
// 首次 Flush/劫持/推送时提交并把缓冲内容落盘，之后切换为直通。
type smartRouteRecorder struct {
	target         gin.ResponseWriter
	header         http.Header
	buf            bytes.Buffer
	status         int
	committed      bool
	wroteHeaderNow bool
}

var _ gin.ResponseWriter = (*smartRouteRecorder)(nil)

func newSmartRouteRecorder(target gin.ResponseWriter) *smartRouteRecorder {
	return &smartRouteRecorder{
		target: target,
		header: make(http.Header),
	}
}

func (r *smartRouteRecorder) Header() http.Header { return r.header }

func (r *smartRouteRecorder) Write(b []byte) (int, error) {
	if r.committed {
		return r.target.Write(b)
	}
	return r.buf.Write(b)
}

func (r *smartRouteRecorder) WriteString(s string) (int, error) {
	if r.committed {
		return r.target.WriteString(s)
	}
	return r.buf.WriteString(s)
}

func (r *smartRouteRecorder) WriteHeader(code int) {
	if r.committed {
		r.target.WriteHeader(code)
		return
	}
	if r.status == 0 && code > 0 {
		r.status = code
	}
}

func (r *smartRouteRecorder) WriteHeaderNow() {
	if r.committed {
		r.target.WriteHeaderNow()
		return
	}
	// 未提交前不真正落盘：头部随失败响应一起丢弃，或随成功响应一起提交。
	r.wroteHeaderNow = true
}

func (r *smartRouteRecorder) Status() int {
	if r.committed {
		return r.target.Status()
	}
	if r.status == 0 {
		return http.StatusOK
	}
	return r.status
}

func (r *smartRouteRecorder) Size() int {
	if r.committed {
		return r.target.Size()
	}
	return r.buf.Len()
}

func (r *smartRouteRecorder) Written() bool {
	if r.committed {
		return r.target.Written()
	}
	return r.wroteHeaderNow || r.buf.Len() > 0
}

// Flush 是流式输出的提交点：一旦真正 Flush，就认为响应已不可回退。
func (r *smartRouteRecorder) Flush() {
	if !r.committed {
		r.commit()
	}
	if flusher, ok := r.target.(http.Flusher); ok {
		flusher.Flush()
	}
}

func (r *smartRouteRecorder) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	r.commit()
	hijacker, ok := r.target.(http.Hijacker)
	if !ok {
		return nil, nil, errors.New("smart routing recorder: target does not support hijack")
	}
	return hijacker.Hijack()
}

func (r *smartRouteRecorder) CloseNotify() <-chan bool {
	if notifier, ok := r.target.(http.CloseNotifier); ok { //nolint:staticcheck // CloseNotifier is deprecated but part of gin.ResponseWriter
		return notifier.CloseNotify()
	}
	return nil
}

func (r *smartRouteRecorder) Pusher() http.Pusher {
	return r.target.Pusher()
}

// commit 把缓冲的头部与响应体一次性落盘到真实 writer，并切换为直通模式。
func (r *smartRouteRecorder) commit() {
	if r.committed {
		return
	}
	r.committed = true
	dst := r.target.Header()
	for k, vv := range r.header {
		dst[k] = vv
	}
	status := r.status
	if status == 0 {
		status = http.StatusOK
	}
	r.target.WriteHeader(status)
	if r.buf.Len() > 0 {
		_, _ = r.target.Write(r.buf.Bytes())
	}
	r.target.WriteHeaderNow()
}

// flushToTarget 在尝试结束时定稿：未提交则把缓冲内容写给真实 writer。
// 已提交（流式直通）时是空操作。
func (r *smartRouteRecorder) flushToTarget() {
	if r == nil || r.committed {
		return
	}
	r.commit()
}
