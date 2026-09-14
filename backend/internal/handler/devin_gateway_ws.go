// Devin 平台 /v1/responses 的 WebSocket 传输桥。
// 移植自 devin2api internal/app/websocket.go，适配 sub2api 的转发管线：
// 每轮规范化后的请求体被包进一个临时的 *gin.Context，复用 forward()
// 的账号选择/并发/计费路径；SSE 帧写出层把 data JSON 拆成 WS 文本帧。
//
// OpenAI Responses WebSocket 模式把 HTTP/SSE 的事件流映射为 WebSocket 文本帧：
// 客户端在同一连接上反复发送 {"type":"response.create"|"response.append", ...} JSON；
// 服务端把每轮 SSE 事件的 data JSON 作为一条 WebSocket 文本消息发回，直到
// response.completed/response.failed/error。
//
// 上游没有 previous_response_id 语义，多轮通过 responses.WSSession 把增量
// input 展开成完整 transcript 再走常规流水线（见 ws_session.go）。
package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/devin"
	devinresponses "github.com/Wei-Shaw/sub2api/internal/pkg/devin/api/openai/responses"
	"github.com/Wei-Shaw/sub2api/internal/pkg/ip"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/Wei-Shaw/sub2api/internal/pkg/requestmodel"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"go.uber.org/zap"
)

var devinWSUpgrader = websocket.Upgrader{
	// 允许跨源（Codex 本地调用可能使用不同 origin）。
	CheckOrigin: func(r *http.Request) bool { return true },
	// Codex/OpenAI Responses WebSocket 协商此子协议。
	Subprotocols: []string{"responses_websockets=2026-02-06"},
	// 给足读写缓冲，避免高频事件时频繁系统调用。
	ReadBufferSize:  8192,
	WriteBufferSize: 8192,
}

const (
	// devinWSWriteDeadline 是单次 WebSocket 写操作的预算；每条消息写出前续约，
	// 不会像一次性绝对 deadline 那样在长轮次中途截断流。
	devinWSWriteDeadline = 60 * time.Second
	// devinWSIdleTimeout 是轮次之间/消息之间允许的空闲上限；PongHandler 与每条
	// 客户端消息都会续约。
	devinWSIdleTimeout = 5 * time.Minute
	// devinWSPingInterval 是服务端主动 ping 的周期；客户端 pong 在 PongHandler 里
	// 续约 read deadline，双向确认存活。WriteControl 不经写锁，不会被大
	// 数据帧饿死。
	devinWSPingInterval = 2 * time.Minute
)

// devinWSInboundMessage 是 reader goroutine 交给主循环的一帧。
type devinWSInboundMessage struct {
	messageType int
	payload     []byte
}

// errDevinWSConnectionClosed 表示 close 帧已发出、写管线应停止。它从
// writeFrame 冒泡到 runWSTurn，主循环见到任何 turnErr 都会退出——这个哨兵
// 只是阻止关闭后继续向客户端写数据帧。
var errDevinWSConnectionClosed = errors.New("websocket close frame sent")

// devinWSResponseWriter 把 HTTP SSE 响应解析为单个 WebSocket 文本帧，同时收集
// 本轮 output item 供会话状态回放。
// forward 写入的每段 SSE 帧被暂存在 buf 中，遇到 "\n\n" 分隔时拆成完整事件，
// 只把 data 行作为 JSON 文本帧发回客户端（与 OpenAI Responses WebSocket 协议一致）。
type devinWSResponseWriter struct {
	conn       *websocket.Conn
	header     http.Header
	statusCode int
	buf        []byte
	err        error
	// 以下为轮级状态收集：completed/failed 标记终结形态，surfaced 表示
	// 客户端已收到可终结本轮的信号，output items 供 WSSession 回放。
	completed           bool
	failed              bool
	surfaced            bool
	completedResponseID string
	// lastTerminal 是最后收到的终结形态事件 payload（completed/failed 的
	// data JSON），commit 时从它提取 response.output。
	lastTerminal    json.RawMessage
	outputItems     map[int64]json.RawMessage
	outputUnindexed []json.RawMessage
	outputBytes     int64
}

func newDevinWSResponseWriter(conn *websocket.Conn) *devinWSResponseWriter {
	return &devinWSResponseWriter{
		conn:        conn,
		header:      make(http.Header),
		outputItems: make(map[int64]json.RawMessage),
	}
}

func (w *devinWSResponseWriter) Header() http.Header        { return w.header }
func (w *devinWSResponseWriter) WriteHeader(statusCode int) { w.statusCode = statusCode }

func (w *devinWSResponseWriter) Write(p []byte) (int, error) {
	if w.err != nil {
		return 0, w.err
	}
	w.buf = append(w.buf, p...)
	for {
		idx := bytes.Index(w.buf, []byte("\n\n"))
		if idx < 0 {
			break
		}
		frame := w.buf[:idx]
		w.buf = w.buf[idx+2:]
		if err := w.writeFrame(frame); err != nil {
			w.err = err
			return 0, err
		}
	}
	return len(p), nil
}

func (w *devinWSResponseWriter) Flush() {
	// forward 在每次写入 SSE 后都会 Flush；
	// 但实际消息在 Write 遇到 "\n\n" 时已经发送，这里不需要额外动作。
}

// flushTail 把缓冲区里不构成完整 SSE 帧的残余内容（如非流式 JSON 错误体）
// 作为一条文本帧发出；没有它，非流式错误在 WS 路径上会被静默吞掉。
// 常规 {"error":{...}} 体会被包成 {"type":"error","status":N,"error":{...}}
// 事件形状——WS 客户端靠 type 字段分发，裸错误 JSON 无法终结回合。
func (w *devinWSResponseWriter) flushTail() {
	if w.err != nil || len(w.buf) == 0 {
		return
	}
	payload := bytes.TrimSpace(w.buf)
	w.buf = nil
	if len(payload) == 0 {
		return
	}
	payload = w.wrapErrorPayload(payload)
	w.surfaced = true
	_ = w.conn.SetWriteDeadline(time.Now().Add(devinWSWriteDeadline))
	w.err = w.conn.WriteMessage(websocket.TextMessage, payload)
}

// wrapErrorPayload 把非流式 {"error":{...}} 响应体转成 WS error 事件；
// 已是事件形状（带 type 字段）或其他 JSON 原样透传。
func (w *devinWSResponseWriter) wrapErrorPayload(payload []byte) []byte {
	if devinWSJSONString(payload, "type") != "" {
		return payload
	}
	var envelope struct {
		Error json.RawMessage `json:"error"`
	}
	if json.Unmarshal(payload, &envelope) != nil || len(envelope.Error) == 0 {
		return payload
	}
	status := w.statusCode
	if status == 0 {
		status = http.StatusInternalServerError
	}
	event, err := json.Marshal(map[string]any{
		"type":   "error",
		"status": status,
		"error":  envelope.Error,
	})
	if err != nil {
		return payload
	}
	return event
}

// writeFrame 解析单条 SSE 帧，把 data 行作为 JSON 文本消息发出。
// 帧级职责：收集 output item、识别终结事件、镜像 message_too_big 为 close 1009。
// SSE 注释行（": ..."）转换为 WebSocket Ping，承担同等的保活作用。
func (w *devinWSResponseWriter) writeFrame(frame []byte) error {
	if bytes.HasPrefix(frame, []byte(":")) {
		return w.conn.WriteControl(websocket.PingMessage, nil, time.Now().Add(devinWSWriteDeadline))
	}
	var data []byte
	lines := bytes.Split(frame, []byte("\n"))
	for _, line := range lines {
		line = bytes.TrimSuffix(line, []byte("\r"))
		if len(line) == 0 {
			continue
		}
		if bytes.HasPrefix(line, []byte("data: ")) {
			data = append(data, line[len("data: "):]...)
		} else if bytes.HasPrefix(line, []byte("data:")) {
			data = append(data, line[len("data:"):]...)
		}
	}
	if len(data) == 0 {
		return nil
	}
	// 单帧一次解析：type/error.code/response.id/item 从同一棵字段树直取。
	// 非 object JSON（数组/标量/非法文本）原样透传文本帧。
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		_ = w.conn.SetWriteDeadline(time.Now().Add(devinWSWriteDeadline))
		return w.conn.WriteMessage(websocket.TextMessage, data)
	}
	eventType := devinWSRawString(fields["type"])
	if err := w.collectOutputItem(eventType, fields); err != nil {
		return err
	}
	switch eventType {
	case "response.completed", "response.done", "response.incomplete":
		w.completed = true
		w.surfaced = true
		w.lastTerminal = bytes.Clone(data)
		w.completedResponseID = devinWSJSONString(fields["response"], "id")
	case "response.failed":
		w.failed = true
		w.surfaced = true
		w.lastTerminal = bytes.Clone(data)
		w.completedResponseID = devinWSJSONString(fields["response"], "id")
	}
	if devinWSJSONString(fields["error"], "code") == "message_too_big" {
		_ = w.conn.WriteControl(
			websocket.CloseMessage,
			websocket.FormatCloseMessage(websocket.CloseMessageTooBig, "upstream websocket message too big"),
			time.Now().Add(devinWSWriteDeadline),
		)
		return errDevinWSConnectionClosed
	}
	// surfaced 语义 = 客户端已收到可终结本轮的信号（终结事件/error 事件/
	// flushTail 兜底）。普通增量帧不算——只发过 delta 就断流仍属于
	// 中途断，需要补 upstream_stream_interrupted 让客户端收尾。
	if eventType == "error" {
		w.surfaced = true
	}
	_ = w.conn.SetWriteDeadline(time.Now().Add(devinWSWriteDeadline))
	return w.conn.WriteMessage(websocket.TextMessage, data)
}

// collectOutputItem 累积 response.output_item.done 的 item 快照。
// completed 事件的 response.output 才是回放基准；collected 只在它缺失/为空
// 时兜底（见 turnResult）。fields 是 writeFrame 已解析的顶层字段树。
func (w *devinWSResponseWriter) collectOutputItem(eventType string, fields map[string]json.RawMessage) error {
	if eventType != "response.output_item.done" {
		return nil
	}
	item, has := fields["item"]
	if !has {
		return nil
	}
	item = bytes.Clone(bytes.TrimSpace(item))
	indexRaw, hasIndex := fields["output_index"]
	var index int64 = -1
	if hasIndex {
		_ = json.Unmarshal(indexRaw, &index)
	}
	if index >= 0 {
		w.outputBytes += int64(len(item)) - int64(len(w.outputItems[index]))
		w.outputItems[index] = item
	} else {
		w.outputBytes += int64(len(item))
		w.outputUnindexed = append(w.outputUnindexed, item)
	}
	if w.outputBytes > devinresponses.WSMaxTranscriptBytes {
		// 收集的 output 超过 transcript 上限与 message_too_big 同义：
		// 回 close 1009 让客户端降级，同时停掉写管线。
		_ = w.conn.WriteControl(
			websocket.CloseMessage,
			websocket.FormatCloseMessage(websocket.CloseMessageTooBig, "response output exceeds websocket transcript limit"),
			time.Now().Add(devinWSWriteDeadline),
		)
		return errDevinWSConnectionClosed
	}
	return nil
}

// turnResult 汇总本轮提交内容：优先用终结事件的 response.output；
// 没有（或为空）时退回 output_item.done 收集的项（过滤不完整 tool call）。
func (w *devinWSResponseWriter) turnResult(completedOutput json.RawMessage) devinresponses.WSTurnResult {
	output := completedOutput
	if len(bytes.TrimSpace(output)) <= 2 {
		output = w.collectedOutput()
	}
	return devinresponses.WSTurnResult{
		CompletedOutput:     output,
		CompletedResponseID: w.completedResponseID,
		PendingToolCallIDs:  devinresponses.WSPendingToolCallIDs(output),
	}
}

// collectedOutput 把 output_item.done 收集的项按 output_index 排序拼回数组；
// 不完整 tool call（缺 call_id/name/arguments）被过滤——回放半成品 call 会让
// 客户端的 output 变孤儿。
func (w *devinWSResponseWriter) collectedOutput() json.RawMessage {
	items := make([]json.RawMessage, 0, len(w.outputItems)+len(w.outputUnindexed))
	appendItem := func(raw json.RawMessage) {
		var fields struct {
			Type      string          `json:"type"`
			CallID    string          `json:"call_id"`
			Name      string          `json:"name"`
			Arguments json.RawMessage `json:"arguments"`
			Input     json.RawMessage `json:"input"`
		}
		_ = json.Unmarshal(raw, &fields)
		t := fields.Type
		isToolCall := t == "function_call" || t == "custom_tool_call"
		if isToolCall {
			body := fields.Arguments
			if t == "custom_tool_call" {
				body = fields.Input
			}
			var s string
			complete := fields.CallID != "" && fields.Name != "" && json.Unmarshal(body, &s) == nil
			if !complete {
				return
			}
		}
		items = append(items, raw)
	}
	indices := make([]int, 0, len(w.outputItems))
	for index := range w.outputItems {
		indices = append(indices, int(index))
	}
	sort.Ints(indices)
	for _, index := range indices {
		appendItem(w.outputItems[int64(index)])
	}
	for _, raw := range w.outputUnindexed {
		appendItem(raw)
	}
	if len(items) == 0 {
		return json.RawMessage("[]")
	}
	encoded, err := json.Marshal(items)
	if err != nil {
		return json.RawMessage("[]")
	}
	return encoded
}

// ResponsesWebSocket 处理 GET /v1/responses 的 WebSocket 传输形态：
// 一条连接上按 turn 串行处理 response.create/response.append。
// 读循环与 turn 处理分离——turn 进行中 reader goroutine 继续消费控制帧
// （ping/pong/close）与排队下一帧，客户端断连能及时取消上游。
func (h *DevinGatewayHandler) ResponsesWebSocket(c *gin.Context) {
	if !websocket.IsWebSocketUpgrade(c.Request) {
		h.devinError(c, devinProtocolResponses, http.StatusUpgradeRequired, "invalid_request_error", "WebSocket upgrade required (Upgrade: websocket)")
		return
	}
	apiKey, ok := middleware2.GetAPIKeyFromContext(c)
	if !ok || apiKey == nil {
		h.devinError(c, devinProtocolResponses, http.StatusUnauthorized, "authentication_error", "Invalid API key")
		return
	}
	subject, subjectOK := middleware2.GetAuthSubjectFromContext(c)
	if !subjectOK {
		h.devinError(c, devinProtocolResponses, http.StatusInternalServerError, "server_error", "User context not found")
		return
	}
	subscription, _ := middleware2.GetSubscriptionFromContext(c)

	reqLog := logger.L().With(
		zap.String("component", "handler.gateway.devin.ws"),
		zap.Int64("user_id", subject.UserID),
		zap.Int64("api_key_id", apiKey.ID),
		zap.Any("group_id", apiKey.GroupID),
		zap.String("client_ip", ip.GetClientIP(c)),
	)

	// 连接级准入：与上游并发槽分开计量（复用 OpenAI WS 的 ingress 连接上限）。
	ctx := c.Request.Context()
	maxIngressConnections := 0
	if h.cfg != nil {
		maxIngressConnections = h.cfg.Gateway.OpenAIWS.MaxIngressConnectionsPerAPIKey
	}
	ingressLease, ingressLeaseAcquired, ingressLeaseErr := h.concurrencyHelper.AcquireOpenAIWSIngressLease(ctx, apiKey.ID, maxIngressConnections)
	if ingressLeaseErr != nil {
		reqLog.Error("devin.websocket_ingress_lease_acquire_failed", zap.Error(ingressLeaseErr))
		h.devinError(c, devinProtocolResponses, http.StatusServiceUnavailable, "server_error", "WebSocket ingress capacity is temporarily unavailable")
		return
	}
	if !ingressLeaseAcquired {
		reqLog.Info("devin.websocket_ingress_capacity_rejected", zap.Int("max_ingress_connections_per_api_key", maxIngressConnections))
		c.Header("Retry-After", "5")
		h.devinError(c, devinProtocolResponses, http.StatusTooManyRequests, "rate_limit_error", "Too many open WebSocket connections, please retry later")
		return
	}
	if ingressLease != nil {
		defer ingressLease.Release()
		ctx = ingressLease.Context()
	}

	conn, err := devinWSUpgrader.Upgrade(c.Writer, c.Request, devinWSUpgradeHeaders(c.Request))
	if err != nil {
		reqLog.Warn("devin.websocket_upgrade_failed", zap.Error(err))
		return
	}
	defer func() { _ = conn.Close() }()
	conn.SetReadLimit(service.ResolveOpenAIWSClientReadLimitBytes(h.cfg))

	connCtx, cancelConn := context.WithCancel(ctx)
	defer cancelConn()

	// 读协程常驻：turn 进行中也要继续消费帧——gorilla 只在 ReadMessage 里
	// 处理 ping/pong/close 控制帧，且这是发现客户端断连的唯一手段。
	// channel 带缓冲：turn 进行中读到的数据帧排队等主循环，控制帧照常应答；
	// 读端一旦出错立即 cancelConn，让在途上游随 ctx 取消而不是空跑到结束。
	firstMessageTimeout := service.ResolveOpenAIWSClientFirstMessageTimeout(h.cfg)
	messages := make(chan devinWSInboundMessage, 16)
	go func() {
		defer close(messages)
		first := true
		for {
			if first {
				_ = conn.SetReadDeadline(time.Now().Add(firstMessageTimeout))
			} else {
				_ = conn.SetReadDeadline(time.Now().Add(devinWSIdleTimeout))
			}
			messageType, payload, err := conn.ReadMessage()
			if err != nil {
				cancelConn()
				return
			}
			first = false
			select {
			case messages <- devinWSInboundMessage{messageType: messageType, payload: payload}:
			case <-connCtx.Done():
				return
			}
		}
	}()
	// PongHandler 续约 read deadline：reader 的 deadline 由 ping/pong 心跳与
	// 客户端消息共同维持，纯空闲（客户端一言不发）的连接也靠这个活过 idle 窗。
	conn.SetPongHandler(func(string) error {
		return conn.SetReadDeadline(time.Now().Add(devinWSIdleTimeout))
	})
	go func() {
		ticker := time.NewTicker(devinWSPingInterval)
		defer ticker.Stop()
		for {
			select {
			case <-connCtx.Done():
				return
			case <-ticker.C:
				if err := conn.WriteControl(websocket.PingMessage, nil, time.Now().Add(devinWSWriteDeadline)); err != nil {
					cancelConn()
					return
				}
			}
		}
	}()

	session := devinresponses.NewWSSession()
	for {
		var message devinWSInboundMessage
		select {
		case <-connCtx.Done():
			return
		case inbound, ok := <-messages:
			if !ok {
				return
			}
			message = inbound
		}

		if message.messageType != websocket.TextMessage {
			if err := writeDevinWSErrorEvent(conn, http.StatusBadRequest, "invalid_request_error", "unsupported_frame", "", "only text websocket messages are supported"); err != nil {
				return
			}
			continue
		}

		normalized, err := session.NormalizeRequest(message.payload)
		if err != nil {
			code := "invalid_request"
			param := ""
			if errors.Is(err, devinresponses.ErrWSPreviousResponseNotFound) {
				code = "previous_response_not_found"
				param = "previous_response_id"
			} else if errors.Is(err, devinresponses.ErrWSUnsupportedRequestType) {
				code = "unsupported_event"
			}
			if err := writeDevinWSErrorEvent(conn, http.StatusBadRequest, "invalid_request_error", code, param, err.Error()); err != nil {
				return
			}
			continue
		}

		// 分组级模型白名单：按轮校验规范化后的 model 字段。
		if blocked := blockedModelAllowlistCandidate(apiKey.Group, requestmodel.FromBodyCandidates("", "application/json", normalized)); blocked != "" {
			service.MarkOpsClientBusinessLimited(c, service.OpsClientBusinessLimitedReasonLocalModelConfiguration)
			middleware2.MarkIngressRejected(c, middleware2.IngressRejectModelNotAllowed)
			if err := writeDevinWSErrorEvent(conn, http.StatusBadRequest, "invalid_request_error", "model_not_allowed", "model", fmt.Sprintf("Model %q is not available for this group", blocked)); err != nil {
				return
			}
			continue
		}

		// 预热帧：generate:false 的请求本地合成 created+completed，input 计入
		// transcript 但不打上游。判定放在 normalize 之后——normalize 会把
		// generate 字段剥掉，且本轮的规范化错误仍按常规路径报给客户端。
		if devinresponses.WSGenerateDisabled(message.payload) {
			result, err := writeDevinWSPrewarm(conn, normalized)
			if err != nil {
				return
			}
			session.Commit(result)
			continue
		}

		turnWriter, turnErr := h.runDevinWSTurn(connCtx, conn, c.Request, normalized, apiKey, subject, subscription)
		if turnErr != nil {
			// 写出层失败（断连/close sent）——连接已不可用，直接退出。
			return
		}
		completedOutput := devinresponses.WSCompletedOutputFromEvent(turnWriter.lastTerminal)
		switch {
		case turnWriter.completed:
			session.Commit(turnWriter.turnResult(completedOutput))
		case turnWriter.failed:
			// response.failed 已转发客户端；本轮不推进会话——续链 prev_id
			// 会 404 触发重放，符合预期。
			session.RequireReplacementReplay()
		case !turnWriter.surfaced:
			// 流结束但客户端没收到任何可终结本轮的信号：补一个中断事件，
			// 并标记下次 create 为全量替换（客户端会重放完整 transcript）。
			session.RequireReplacementReplay()
			if err := writeDevinWSErrorEvent(conn, http.StatusBadGateway, "server_error", "upstream_stream_interrupted",
				"", "upstream response was interrupted; resend the full conversation input"); err != nil {
				return
			}
		default:
			// surfaced 但既非 completed 也非 failed（如 error 事件兜底）：
			// 不推进会话，下一轮照增量/替换规则处理。
			session.RequireReplacementReplay()
		}
	}
}

// runDevinWSTurn 把一条规范化请求交给常规 /v1/responses 转发流水线执行：
// 构造携带本轮 body 的内部 POST 请求，包进临时 *gin.Context（writer 指向
// turnWriter），复用 forward() 的鉴权信息/账号选择/并发/计费路径。
// 返回的 writer 供调用方读取轮级状态（completed/failed/output 收集）。
func (h *DevinGatewayHandler) runDevinWSTurn(
	connCtx context.Context,
	conn *websocket.Conn,
	upgradeRequest *http.Request,
	body json.RawMessage,
	apiKey *service.APIKey,
	subject middleware2.AuthSubject,
	subscription *service.UserSubscription,
) (*devinWSResponseWriter, error) {
	innerRequest, err := http.NewRequestWithContext(connCtx, http.MethodPost, "/v1/responses", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	innerRequest.Header.Set("Content-Type", "application/json")
	// innerRequest 不经过 HTTP middleware，鉴权与日志关联所需的头逐一手动透传。
	for _, name := range []string{
		"Authorization", "X-Api-Key", "X-Session-Id", "User-Agent",
		"Session-Id", "Session_id", "Thread-Id", "X-Codex-Turn-Metadata",
		"X-Forwarded-For", "X-Real-Ip",
	} {
		if value := upgradeRequest.Header.Get(name); value != "" {
			innerRequest.Header.Set(name, value)
		}
	}
	innerRequest.RemoteAddr = upgradeRequest.RemoteAddr

	turnWriter := newDevinWSResponseWriter(conn)
	turnCtx, _ := gin.CreateTestContext(turnWriter)
	turnCtx.Request = innerRequest
	turnCtx.Set(string(middleware2.ContextKeyAPIKey), apiKey)
	turnCtx.Set(string(middleware2.ContextKeyUser), subject)
	if apiKey.User != nil {
		turnCtx.Set(string(middleware2.ContextKeyUserRole), apiKey.User.Role)
	}
	if subscription != nil {
		turnCtx.Set(string(middleware2.ContextKeySubscription), subscription)
	}

	h.forward(turnCtx, devinProtocolResponses)
	turnWriter.flushTail()
	if turnWriter.err != nil {
		return turnWriter, turnWriter.err
	}
	return turnWriter, nil
}

// writeDevinWSPrewarm 合成预热回合的 response.created + response.completed。
// Codex 发 generate:false 是为了让连接进入可续链状态而不消耗上游配额；
// 合成响应必须带 resp_ 前缀 id 供下一轮 previous_response_id 引用。
func writeDevinWSPrewarm(conn *websocket.Conn, request json.RawMessage) (devinresponses.WSTurnResult, error) {
	responseID := devin.PrefixedID("resp_prewarm_")
	createdAt := time.Now().Unix()
	model := devinWSJSONString(request, "model")
	response := map[string]any{
		"id": responseID, "object": "response", "created_at": createdAt,
		"status": "in_progress", "background": false, "error": nil,
		"model": model, "output": []any{},
	}
	created, err := json.Marshal(map[string]any{
		"type": "response.created", "sequence_number": 0, "response": response,
	})
	if err != nil {
		return devinresponses.WSTurnResult{}, err
	}
	if err := writeDevinWSPayload(conn, created); err != nil {
		return devinresponses.WSTurnResult{}, err
	}
	response["status"] = "completed"
	response["usage"] = map[string]any{"input_tokens": 0, "output_tokens": 0, "total_tokens": 0}
	completed, err := json.Marshal(map[string]any{
		"type": "response.completed", "sequence_number": 1, "response": response,
	})
	if err != nil {
		return devinresponses.WSTurnResult{}, err
	}
	if err := writeDevinWSPayload(conn, completed); err != nil {
		return devinresponses.WSTurnResult{}, err
	}
	return devinresponses.WSTurnResult{
		CompletedOutput:     json.RawMessage("[]"),
		CompletedResponseID: responseID,
	}, nil
}

// writeDevinWSErrorEvent 发送 {"type":"error","status":N,"error":{...}} 事件。
// 非终结错误走这条路——连接保持开启，客户端可继续下发一帧。
func writeDevinWSErrorEvent(conn *websocket.Conn, status int, errorType, code, param, message string) error {
	body := map[string]any{"message": message, "type": errorType}
	if code != "" {
		body["code"] = code
	}
	if param != "" {
		body["param"] = param
	}
	event, err := json.Marshal(map[string]any{
		"type":   "error",
		"status": status,
		"error":  body,
	})
	if err != nil {
		return err
	}
	return writeDevinWSPayload(conn, event)
}

func writeDevinWSPayload(conn *websocket.Conn, payload []byte) error {
	if conn == nil {
		return errors.New("websocket connection is nil")
	}
	if err := conn.SetWriteDeadline(time.Now().Add(devinWSWriteDeadline)); err != nil {
		return err
	}
	return conn.WriteMessage(websocket.TextMessage, payload)
}

// devinWSUpgradeHeaders 在 upgrade 101 响应里回带 x-codex-turn-state——Codex 靠它
// 在重连后保持 turn 状态粘性（参照 CLIProxyAPI websocketUpgradeHeaders）。
func devinWSUpgradeHeaders(request *http.Request) http.Header {
	headers := http.Header{}
	if request == nil {
		return headers
	}
	if turnState := request.Header.Get("x-codex-turn-state"); turnState != "" {
		headers.Set("x-codex-turn-state", turnState)
	}
	return headers
}

// --- WS 帧 JSON 手术小工具（作用域与 ws_session.go 内部 helper 相同，但
// 本文件在 handler 包内，需要一份局部副本）。 ---

func devinWSJSONField(payload json.RawMessage, key string) (value json.RawMessage, has bool) {
	var object map[string]json.RawMessage
	if json.Unmarshal(payload, &object) != nil {
		return nil, false
	}
	raw, ok := object[key]
	if !ok {
		return nil, false
	}
	return raw, true
}

func devinWSJSONString(payload json.RawMessage, key string) string {
	raw, has := devinWSJSONField(payload, key)
	if !has {
		return ""
	}
	return devinWSRawString(raw)
}

func devinWSRawString(raw json.RawMessage) string {
	var s string
	if json.Unmarshal(raw, &s) != nil {
		return ""
	}
	return s
}
