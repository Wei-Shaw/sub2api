package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ip"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"

	coderws "github.com/coder/websocket"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

type CodexGoalBridgeHandler struct {
	service               codexGoalBridgeService
	gatewayService        codexGoalUsageRecorder
	apiKeyService         service.APIKeyQuotaUpdater
	usageRecordWorkerPool *service.UsageRecordWorkerPool
}

type codexGoalBridgeService interface {
	IsEnabled() bool
	Handle(ctx context.Context, req service.CodexGoalBridgeRequest) (*service.CodexGoalBridgeResponse, error)
}

type codexGoalBridgeStreamService interface {
	HandleStream(ctx context.Context, req service.CodexGoalBridgeRequest, sink service.CodexGoalBridgeStreamSink) (*service.CodexGoalBridgeResponse, error)
}

type codexGoalBridgeFileStoreService interface {
	StoreUploadedFile(ctx context.Context, filename, contentType, purpose string, body io.Reader) (*service.CodexGoalStoredFile, error)
	GetStoredFile(fileID string) (*service.CodexGoalStoredFile, error)
}

type codexGoalUsageRecorder interface {
	RecordUsage(ctx context.Context, input *service.RecordUsageInput) error
}

func NewCodexGoalBridgeHandler(
	bridgeService *service.CodexGoalBridgeService,
	gatewayService *service.GatewayService,
	apiKeyService *service.APIKeyService,
	usageRecordWorkerPool *service.UsageRecordWorkerPool,
) *CodexGoalBridgeHandler {
	return &CodexGoalBridgeHandler{
		service:               bridgeService,
		gatewayService:        gatewayService,
		apiKeyService:         apiKeyService,
		usageRecordWorkerPool: usageRecordWorkerPool,
	}
}

func (h *CodexGoalBridgeHandler) TryHandle(c *gin.Context, protocol, endpoint string) bool {
	if h == nil || h.service == nil || !h.service.IsEnabled() {
		return false
	}
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		h.writeError(c, protocol, &service.CodexGoalBridgeError{
			StatusCode: http.StatusBadRequest,
			Code:       "invalid_request_body",
			Message:    err.Error(),
		})
		return true
	}
	c.Request.Body = io.NopCloser(bytes.NewReader(body))

	var groupID *int64
	if apiKey, ok := middleware2.GetAPIKeyFromContext(c); ok && apiKey != nil {
		groupID = apiKey.GroupID
	}
	bridgeReq := service.CodexGoalBridgeRequest{
		Protocol: protocol,
		Endpoint: endpoint,
		Body:     body,
		GroupID:  groupID,
	}
	_, _, stream, err := service.ExtractCodexGoalObjective(bridgeReq)
	if err != nil {
		h.writeError(c, protocol, err)
		return true
	}
	if stream {
		if streamService, ok := h.service.(codexGoalBridgeStreamService); ok {
			h.tryHandleStream(c, streamService, bridgeReq)
			return true
		}
	}
	result, err := h.service.Handle(c.Request.Context(), bridgeReq)
	if err != nil {
		h.writeError(c, protocol, err)
		return true
	}
	h.writeSuccess(c, result)
	h.recordUsage(c, body, result, false)
	return true
}

func (h *CodexGoalBridgeHandler) TryHandleResponsesWebSocket(c *gin.Context) bool {
	if h == nil || h.service == nil || !h.service.IsEnabled() {
		return false
	}
	if !isCodexGoalWSUpgradeRequest(c.Request) {
		h.writeError(c, service.CodexGoalProtocolOpenAIResponses, &service.CodexGoalBridgeError{
			StatusCode: http.StatusUpgradeRequired,
			Code:       "invalid_request_error",
			Message:    "WebSocket upgrade required (Upgrade: websocket)",
		})
		return true
	}

	conn, err := coderws.Accept(c.Writer, c.Request, &coderws.AcceptOptions{
		CompressionMode: coderws.CompressionContextTakeover,
	})
	if err != nil {
		return true
	}
	defer func() {
		_ = conn.CloseNow()
	}()
	conn.SetReadLimit(8 * 1024 * 1024)

	for {
		readCtx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
		msgType, payload, err := conn.Read(readCtx)
		cancel()
		if err != nil {
			return true
		}
		if msgType != coderws.MessageText && msgType != coderws.MessageBinary {
			closeCodexGoalWS(conn, coderws.StatusPolicyViolation, "unsupported websocket message type")
			return true
		}
		if strings.TrimSpace(string(payload)) == "" {
			closeCodexGoalWS(conn, coderws.StatusPolicyViolation, "empty websocket payload")
			return true
		}
		var envelope map[string]any
		if err := json.Unmarshal(payload, &envelope); err != nil {
			writeCodexGoalWSError(c.Request.Context(), conn, http.StatusBadRequest, "invalid_request_error", "invalid JSON payload")
			closeCodexGoalWS(conn, coderws.StatusPolicyViolation, "invalid JSON payload")
			return true
		}
		if strings.TrimSpace(fmt.Sprint(envelope["type"])) != "response.create" {
			writeCodexGoalWSError(c.Request.Context(), conn, http.StatusBadRequest, "invalid_request_error", "expected response.create websocket payload")
			closeCodexGoalWS(conn, coderws.StatusPolicyViolation, "expected response.create websocket payload")
			return true
		}

		var groupID *int64
		if apiKey, ok := middleware2.GetAPIKeyFromContext(c); ok && apiKey != nil {
			groupID = apiKey.GroupID
		}
		bridgeReq := service.CodexGoalBridgeRequest{
			Protocol: service.CodexGoalProtocolOpenAIResponses,
			Endpoint: c.FullPath(),
			Body:     payload,
			GroupID:  groupID,
		}
		result, err := h.handleResponsesWebSocketGoal(c.Request.Context(), conn, bridgeReq)
		if err != nil {
			bridgeErr := normalizeCodexGoalBridgeError(err)
			writeCodexGoalWSError(c.Request.Context(), conn, bridgeErr.StatusCode, bridgeErr.Code, bridgeErr.Message)
			closeCodexGoalWS(conn, codexGoalWSCloseStatus(bridgeErr.StatusCode), bridgeErr.Message)
			return true
		}
		h.recordUsage(c, payload, result, true)
	}
}

func (h *CodexGoalBridgeHandler) handleResponsesWebSocketGoal(ctx context.Context, conn *coderws.Conn, req service.CodexGoalBridgeRequest) (*service.CodexGoalBridgeResponse, error) {
	if streamService, ok := h.service.(codexGoalBridgeStreamService); ok {
		var writer *codexGoalWSStreamWriter
		sink := func(event service.CodexGoalBridgeStreamEvent) error {
			if writer == nil {
				writer = newCodexGoalWSStreamWriter(ctx, conn, event.Model, event.CreatedAt, event.DeferResponsesOutputItem)
			}
			switch event.Type {
			case service.CodexGoalBridgeStreamEventStart:
				return writer.start()
			case service.CodexGoalBridgeStreamEventDelta:
				return writer.delta(event.Delta)
			case service.CodexGoalBridgeStreamEventToolEvent:
				return writer.toolEvent(event.ToolEventIndex, event.ToolEvent)
			case service.CodexGoalBridgeStreamEventFunctionCallStart:
				return writer.startFunctionCall(event.FunctionCallIndex, event.FunctionCall)
			case service.CodexGoalBridgeStreamEventFunctionArgumentsDelta:
				return writer.functionCallArgumentsDelta(event.FunctionCallIndex, event.FunctionCall, event.Delta)
			case service.CodexGoalBridgeStreamEventFunctionCallDone:
				return writer.finishFunctionCall(event.FunctionCallIndex, event.FunctionCall)
			default:
				return nil
			}
		}
		result, err := streamService.HandleStream(ctx, req, sink)
		if err != nil {
			return nil, err
		}
		if writer == nil {
			writer = newCodexGoalWSStreamWriter(ctx, conn, result.Model, result.CreatedAt, len(result.FunctionCalls) > 0)
		}
		if err := writer.complete(result); err != nil {
			return nil, err
		}
		return result, nil
	}
	result, err := h.service.Handle(ctx, req)
	if err != nil {
		return nil, err
	}
	if err := writeCodexGoalResponsesWSEvents(ctx, conn, result); err != nil {
		return nil, err
	}
	return result, nil
}

func (h *CodexGoalBridgeHandler) tryHandleStream(c *gin.Context, streamService codexGoalBridgeStreamService, req service.CodexGoalBridgeRequest) {
	var writer *codexGoalSSEStreamWriter
	sink := func(event service.CodexGoalBridgeStreamEvent) error {
		if writer == nil {
			writer = newCodexGoalSSEStreamWriter(c, event.Protocol, event.Model, event.CreatedAt, event.DeferResponsesOutputItem)
		}
		switch event.Type {
		case service.CodexGoalBridgeStreamEventStart:
			writer.start()
		case service.CodexGoalBridgeStreamEventDelta:
			writer.delta(event.Delta)
		case service.CodexGoalBridgeStreamEventToolEvent:
			writer.toolEvent(event.ToolEventIndex, event.ToolEvent)
		case service.CodexGoalBridgeStreamEventFunctionCallStart:
			writer.startFunctionCall(event.FunctionCallIndex, event.FunctionCall)
		case service.CodexGoalBridgeStreamEventFunctionArgumentsDelta:
			writer.functionCallArgumentsDelta(event.FunctionCallIndex, event.FunctionCall, event.Delta)
		case service.CodexGoalBridgeStreamEventFunctionCallDone:
			writer.finishFunctionCall(event.FunctionCallIndex, event.FunctionCall)
		}
		return nil
	}

	result, err := streamService.HandleStream(c.Request.Context(), req, sink)
	if err != nil {
		bridgeErr := normalizeCodexGoalBridgeError(err)
		if writer != nil && writer.started {
			writer.fail(bridgeErr)
			return
		}
		h.writeError(c, req.Protocol, bridgeErr)
		return
	}
	if writer == nil {
		writer = newCodexGoalSSEStreamWriter(c, result.Protocol, result.Model, result.CreatedAt, len(result.FunctionCalls) > 0)
	}
	writer.start()
	writer.complete(result)
	h.recordUsage(c, req.Body, result, false)
}

func (h *CodexGoalBridgeHandler) recordUsage(c *gin.Context, body []byte, result *service.CodexGoalBridgeResponse, openAIWSMode bool) {
	if h == nil || h.gatewayService == nil || result == nil {
		return
	}
	apiKey, ok := middleware2.GetAPIKeyFromContext(c)
	if !ok || apiKey == nil || apiKey.User == nil {
		return
	}
	account := result.Account
	if account == nil && result.AccountID > 0 {
		account = &service.Account{
			ID:       result.AccountID,
			Platform: service.PlatformOpenAI,
			Type:     service.AccountTypeOAuth,
		}
	}
	if account == nil || account.ID == 0 {
		return
	}
	platform := account.Platform
	if strings.TrimSpace(platform) == "" {
		platform = service.PlatformOpenAI
	}
	userAgent := c.GetHeader("User-Agent")
	clientIP := ip.GetClientIP(c)
	requestPayloadHash := service.HashUsageRequestPayload(body)
	inboundEndpoint := GetInboundEndpoint(c)
	const upstreamEndpoint = "/codex/goal"
	quotaPlatform := service.QuotaPlatform(c.Request.Context(), apiKey)
	forwardResult := &service.ForwardResult{
		Model:        result.Model,
		Stream:       result.Stream || openAIWSMode,
		OpenAIWSMode: openAIWSMode,
		Duration:     result.Duration,
	}
	if forwardResult.Model == "" {
		forwardResult.Model = "gpt-5.5"
	}
	if forwardResult.Duration < 0 {
		forwardResult.Duration = 0
	}

	h.submitUsageRecordTask(c.Request.Context(), func(ctx context.Context) {
		if err := h.gatewayService.RecordUsage(ctx, &service.RecordUsageInput{
			Result:             forwardResult,
			APIKey:             apiKey,
			User:               apiKey.User,
			Account:            account,
			InboundEndpoint:    inboundEndpoint,
			UpstreamEndpoint:   upstreamEndpoint,
			UserAgent:          userAgent,
			IPAddress:          clientIP,
			RequestPayloadHash: requestPayloadHash,
			QuotaPlatform:      quotaPlatform,
			APIKeyService:      h.apiKeyService,
		}); err != nil {
			requestLogger(c, "handler.codex_goal_bridge").Error("codex_goal_bridge.record_usage_failed",
				zap.Int64("account_id", account.ID),
				zap.Int64("api_key_id", apiKey.ID),
				zap.String("model", forwardResult.Model),
				zap.String("platform", platform),
				zap.Error(err),
			)
		}
	})
}

func (h *CodexGoalBridgeHandler) submitUsageRecordTask(parent context.Context, task service.UsageRecordTask) {
	if task == nil {
		return
	}
	task = wrapUsageRecordTaskContext(parent, task)
	if h.usageRecordWorkerPool != nil {
		h.usageRecordWorkerPool.Submit(task)
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	defer func() {
		if recovered := recover(); recovered != nil {
			logger.L().With(
				zap.String("component", "handler.codex_goal_bridge"),
				zap.Any("panic", recovered),
			).Error("codex_goal_bridge.usage_record_task_panic_recovered")
		}
	}()
	task(ctx)
}

func (h *CodexGoalBridgeHandler) RejectUnsupportedIfEnabled(c *gin.Context, protocol, code, message string) bool {
	if h == nil || h.service == nil || !h.service.IsEnabled() {
		return false
	}
	h.writeError(c, protocol, &service.CodexGoalBridgeError{
		StatusCode: http.StatusNotImplemented,
		Code:       code,
		Message:    message,
	})
	return true
}

func (h *CodexGoalBridgeHandler) UploadFile(c *gin.Context) {
	if h == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": gin.H{
			"type":    "not_found_error",
			"message": "Codex goal bridge is disabled",
		}})
		return
	}
	if h.service == nil || !h.service.IsEnabled() {
		h.writeError(c, service.CodexGoalProtocolOpenAIResponses, &service.CodexGoalBridgeError{
			StatusCode: http.StatusNotFound,
			Code:       "codex_goal_bridge_disabled",
			Message:    "Codex goal bridge is disabled",
		})
		return
	}
	store, ok := h.service.(codexGoalBridgeFileStoreService)
	if !ok {
		h.writeError(c, service.CodexGoalProtocolOpenAIResponses, &service.CodexGoalBridgeError{
			StatusCode: http.StatusNotFound,
			Code:       "codex_goal_file_store_unavailable",
			Message:    "Codex goal file store is unavailable",
		})
		return
	}
	fileHeader, err := c.FormFile("file")
	if err != nil {
		h.writeError(c, service.CodexGoalProtocolOpenAIResponses, &service.CodexGoalBridgeError{
			StatusCode: http.StatusBadRequest,
			Code:       "invalid_request_error",
			Message:    "file multipart field is required",
		})
		return
	}
	file, err := fileHeader.Open()
	if err != nil {
		h.writeError(c, service.CodexGoalProtocolOpenAIResponses, &service.CodexGoalBridgeError{
			StatusCode: http.StatusBadRequest,
			Code:       "invalid_request_error",
			Message:    err.Error(),
		})
		return
	}
	defer file.Close()
	stored, err := store.StoreUploadedFile(c.Request.Context(), fileHeader.Filename, fileHeader.Header.Get("Content-Type"), c.PostForm("purpose"), file)
	if err != nil {
		h.writeError(c, service.CodexGoalProtocolOpenAIResponses, err)
		return
	}
	writeCodexGoalStoredFile(c, stored)
}

func (h *CodexGoalBridgeHandler) GetFile(c *gin.Context) {
	if h == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": gin.H{
			"type":    "not_found_error",
			"message": "Codex goal bridge is disabled",
		}})
		return
	}
	if h.service == nil || !h.service.IsEnabled() {
		h.writeError(c, service.CodexGoalProtocolOpenAIResponses, &service.CodexGoalBridgeError{
			StatusCode: http.StatusNotFound,
			Code:       "codex_goal_bridge_disabled",
			Message:    "Codex goal bridge is disabled",
		})
		return
	}
	store, ok := h.service.(codexGoalBridgeFileStoreService)
	if !ok {
		h.writeError(c, service.CodexGoalProtocolOpenAIResponses, &service.CodexGoalBridgeError{
			StatusCode: http.StatusNotFound,
			Code:       "codex_goal_file_store_unavailable",
			Message:    "Codex goal file store is unavailable",
		})
		return
	}
	stored, err := store.GetStoredFile(c.Param("file_id"))
	if err != nil {
		h.writeError(c, service.CodexGoalProtocolOpenAIResponses, err)
		return
	}
	writeCodexGoalStoredFile(c, stored)
}

func writeCodexGoalStoredFile(c *gin.Context, stored *service.CodexGoalStoredFile) {
	if stored == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": gin.H{
			"type":    "not_found_error",
			"message": "file was not found",
		}})
		return
	}
	body := gin.H{
		"id":         stored.ID,
		"object":     "file",
		"bytes":      stored.Bytes,
		"created_at": stored.CreatedAt,
		"filename":   stored.Filename,
		"purpose":    stored.Purpose,
	}
	if stored.MIMEType != "" {
		body["mime_type"] = stored.MIMEType
	}
	c.JSON(http.StatusOK, body)
}

func (h *CodexGoalBridgeHandler) writeSuccess(c *gin.Context, result *service.CodexGoalBridgeResponse) {
	if result == nil {
		c.JSON(http.StatusBadGateway, gin.H{
			"error": gin.H{
				"type":    "codex_goal_empty_response",
				"message": "Codex goal bridge returned no response",
			},
		})
		return
	}
	if result.Stream {
		h.writeStream(c, result)
		return
	}
	created := result.CreatedAt
	if created.IsZero() {
		created = time.Now()
	}
	idSuffix := fmt.Sprintf("%d", created.UnixNano())
	switch result.Protocol {
	case service.CodexGoalProtocolOpenAIChat:
		message := gin.H{
			"role":    "assistant",
			"content": result.Text,
		}
		finishReason := "stop"
		if len(result.FunctionCalls) > 0 {
			message["content"] = nil
			message["tool_calls"] = codexGoalChatToolCalls(result.FunctionCalls)
			finishReason = "tool_calls"
		}
		c.JSON(http.StatusOK, gin.H{
			"id":      "chatcmpl-codex-goal-" + idSuffix,
			"object":  "chat.completion",
			"created": created.Unix(),
			"model":   result.Model,
			"choices": []gin.H{{
				"index":         0,
				"message":       message,
				"finish_reason": finishReason,
			}},
		})
	case service.CodexGoalProtocolAnthropic:
		content := []gin.H{{
			"type": "text",
			"text": result.Text,
		}}
		stopReason := "end_turn"
		if len(result.FunctionCalls) > 0 {
			content = codexGoalAnthropicToolUses(result.FunctionCalls)
			stopReason = "tool_use"
		}
		c.JSON(http.StatusOK, gin.H{
			"id":            "msg_codex_goal_" + idSuffix,
			"type":          "message",
			"role":          "assistant",
			"model":         result.Model,
			"content":       content,
			"stop_reason":   stopReason,
			"stop_sequence": nil,
			"usage": gin.H{
				"input_tokens":  0,
				"output_tokens": 0,
			},
		})
	case service.CodexGoalProtocolGemini:
		c.JSON(http.StatusOK, gin.H{
			"candidates": []gin.H{{
				"index": 0,
				"content": gin.H{
					"role":  "model",
					"parts": codexGoalGeminiParts(result.Text, result.FunctionCalls),
				},
				"finishReason": "STOP",
			}},
			"modelVersion": result.Model,
		})
	default:
		c.JSON(http.StatusOK, gin.H{
			"id":         "resp_codex_goal_" + idSuffix,
			"object":     "response",
			"created_at": created.Unix(),
			"status":     "completed",
			"model":      result.Model,
			"output":     codexGoalResponsesOutputItems(idSuffix, result.Text, result.ToolEvents, result.FunctionCalls),
		})
	}
}

func codexGoalChatToolCalls(functionCalls []service.CodexGoalFunctionCall) []gin.H {
	out := make([]gin.H, 0, len(functionCalls))
	for i, call := range functionCalls {
		id := strings.TrimSpace(call.CallID)
		if id == "" {
			id = strings.TrimSpace(call.ID)
		}
		if id == "" {
			id = fmt.Sprintf("call_codex_goal_%d", i)
		}
		arguments := strings.TrimSpace(call.Arguments)
		if arguments == "" {
			arguments = "{}"
		}
		out = append(out, gin.H{
			"id":   id,
			"type": "function",
			"function": gin.H{
				"name":      call.Name,
				"arguments": arguments,
			},
		})
	}
	return out
}

func codexGoalChatToolCallDelta(index int, call service.CodexGoalFunctionCall) gin.H {
	arguments := strings.TrimSpace(call.Arguments)
	if arguments == "" {
		arguments = "{}"
	}
	return codexGoalChatToolCallDeltaWithArguments(index, call, arguments, true)
}

func codexGoalChatToolCallDeltaWithArguments(index int, call service.CodexGoalFunctionCall, arguments string, includeName bool) gin.H {
	id := strings.TrimSpace(call.CallID)
	if id == "" {
		id = strings.TrimSpace(call.ID)
	}
	if id == "" {
		id = fmt.Sprintf("call_codex_goal_%d", index)
	}
	function := gin.H{
		"arguments": arguments,
	}
	if includeName {
		function["name"] = call.Name
	}
	delta := gin.H{
		"index":    index,
		"id":       id,
		"type":     "function",
		"function": function,
	}
	return delta
}

func codexGoalAnthropicToolUses(functionCalls []service.CodexGoalFunctionCall) []gin.H {
	out := make([]gin.H, 0, len(functionCalls))
	for i, call := range functionCalls {
		id := strings.TrimSpace(call.ID)
		if id == "" {
			id = strings.TrimSpace(call.CallID)
		}
		if id == "" {
			id = fmt.Sprintf("toolu_codex_goal_%d", i)
		}
		out = append(out, gin.H{
			"type":  "tool_use",
			"id":    id,
			"name":  call.Name,
			"input": codexGoalJSONAny(call.Arguments),
		})
	}
	return out
}

func codexGoalJSONAny(raw string) any {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return gin.H{}
	}
	var value any
	if err := json.Unmarshal([]byte(raw), &value); err == nil {
		return value
	}
	return raw
}

func codexGoalGeminiParts(text string, functionCalls []service.CodexGoalFunctionCall) []gin.H {
	if len(functionCalls) > 0 {
		return codexGoalGeminiFunctionParts(functionCalls)
	}
	return []gin.H{{
		"text": text,
	}}
}

func codexGoalGeminiFunctionParts(functionCalls []service.CodexGoalFunctionCall) []gin.H {
	out := make([]gin.H, 0, len(functionCalls))
	for i, call := range functionCalls {
		id := strings.TrimSpace(call.ID)
		if id == "" {
			id = strings.TrimSpace(call.CallID)
		}
		if id == "" {
			id = fmt.Sprintf("fc_codex_goal_%d", i)
		}
		functionCall := gin.H{
			"name": call.Name,
			"args": codexGoalJSONAny(call.Arguments),
		}
		if id != "" {
			functionCall["id"] = id
		}
		out = append(out, gin.H{
			"functionCall": functionCall,
		})
	}
	return out
}

func codexGoalGeminiStreamChunk(model, text string, functionCalls []service.CodexGoalFunctionCall, finishReason any) gin.H {
	candidate := gin.H{
		"index": 0,
	}
	if text != "" || len(functionCalls) > 0 {
		candidate["content"] = gin.H{
			"role":  "model",
			"parts": codexGoalGeminiParts(text, functionCalls),
		}
	}
	if finishReason != nil {
		candidate["finishReason"] = finishReason
	}
	return gin.H{
		"candidates":   []gin.H{candidate},
		"modelVersion": model,
	}
}

func codexGoalResponsesOutputItems(idSuffix, text string, toolEvents []service.CodexGoalToolEvent, functionCalls []service.CodexGoalFunctionCall) []gin.H {
	output := make([]gin.H, 0, len(functionCalls)+len(toolEvents)+1)
	for i, call := range functionCalls {
		id := strings.TrimSpace(call.ID)
		if id == "" {
			id = fmt.Sprintf("fc_codex_goal_%s_%d", idSuffix, i)
		}
		callID := strings.TrimSpace(call.CallID)
		if callID == "" {
			callID = fmt.Sprintf("call_codex_goal_%s_%d", idSuffix, i)
		}
		arguments := strings.TrimSpace(call.Arguments)
		if arguments == "" {
			arguments = "{}"
		}
		output = append(output, gin.H{
			"id":        id,
			"type":      "function_call",
			"status":    "completed",
			"call_id":   callID,
			"name":      call.Name,
			"arguments": arguments,
		})
	}
	for i, event := range toolEvents {
		switch event.Type {
		case "web_search_call":
			id := strings.TrimSpace(event.ID)
			if id == "" {
				id = fmt.Sprintf("ws_codex_goal_%s_%d", idSuffix, i)
			}
			item := gin.H{
				"id":     id,
				"type":   "web_search_call",
				"status": "completed",
			}
			if action := codexGoalToolEventAction(event); action != nil {
				item["action"] = action
			}
			output = append(output, item)
		case "mcp_call":
			id := strings.TrimSpace(event.ID)
			if id == "" {
				id = fmt.Sprintf("mcp_codex_goal_%s_%d", idSuffix, i)
			}
			var errValue any
			if strings.TrimSpace(event.Error) != "" {
				errValue = gin.H{"message": event.Error}
			}
			output = append(output, gin.H{
				"id":                  id,
				"type":                "mcp_call",
				"approval_request_id": nil,
				"arguments":           event.Arguments,
				"error":               errValue,
				"name":                event.Name,
				"output":              event.Output,
				"server_label":        event.ServerLabel,
			})
		}
	}
	if strings.TrimSpace(text) != "" || len(output) == 0 {
		output = append(output, gin.H{
			"id":     "msg_codex_goal_" + idSuffix,
			"type":   "message",
			"status": "completed",
			"role":   "assistant",
			"content": []gin.H{{
				"type":        "output_text",
				"text":        text,
				"annotations": []any{},
			}},
		})
	}
	return output
}

func codexGoalToolEventAction(event service.CodexGoalToolEvent) any {
	if len(event.Action) > 0 {
		var action any
		if err := json.Unmarshal(event.Action, &action); err == nil {
			return action
		}
	}
	if strings.TrimSpace(event.Query) != "" {
		return gin.H{
			"type":  "search",
			"query": event.Query,
		}
	}
	return nil
}

func codexGoalResponsesStreamingOutputItems(idSuffix, text string, toolEvents []service.CodexGoalToolEvent, functionCalls []service.CodexGoalFunctionCall) []gin.H {
	output := make([]gin.H, 0, len(functionCalls)+len(toolEvents)+1)
	if len(functionCalls) > 0 {
		output = append(output, codexGoalResponsesOutputItems(idSuffix, "", nil, functionCalls)...)
	}
	if strings.TrimSpace(text) != "" || len(output)+len(toolEvents) == 0 {
		output = append(output, gin.H{
			"id":     "msg_codex_goal_" + idSuffix,
			"type":   "message",
			"status": "completed",
			"role":   "assistant",
			"content": []gin.H{{
				"type":        "output_text",
				"text":        text,
				"annotations": []any{},
			}},
		})
	}
	if len(toolEvents) > 0 {
		output = append(output, codexGoalResponsesOutputItems(idSuffix, "", toolEvents, nil)...)
	}
	return output
}

func codexGoalResponsesToolEventItem(idSuffix string, outputIndex int, event service.CodexGoalToolEvent, status string) gin.H {
	switch event.Type {
	case "web_search_call":
		id := strings.TrimSpace(event.ID)
		if id == "" {
			id = fmt.Sprintf("ws_codex_goal_%s_%d", idSuffix, outputIndex)
		}
		item := gin.H{
			"id":     id,
			"type":   "web_search_call",
			"status": status,
		}
		if action := codexGoalToolEventAction(event); action != nil {
			item["action"] = action
		}
		return item
	case "mcp_call":
		id := strings.TrimSpace(event.ID)
		if id == "" {
			id = fmt.Sprintf("mcp_codex_goal_%s_%d", idSuffix, outputIndex)
		}
		var errValue any
		if strings.TrimSpace(event.Error) != "" {
			errValue = gin.H{"message": event.Error}
		}
		return gin.H{
			"id":                  id,
			"type":                "mcp_call",
			"status":              status,
			"approval_request_id": nil,
			"arguments":           event.Arguments,
			"error":               errValue,
			"name":                event.Name,
			"output":              event.Output,
			"server_label":        event.ServerLabel,
		}
	default:
		return nil
	}
}

type codexGoalSSEStreamWriter struct {
	c                    *gin.Context
	protocol             string
	model                string
	created              time.Time
	idSuffix             string
	responseID           string
	itemID               string
	chatID               string
	messageID            string
	contentIndex         int
	started              bool
	deferOutputItem      bool
	messageStarted       bool
	anthropicTextStarted bool
	functionCallStarted  map[int]service.CodexGoalFunctionCall
	functionCallDone     map[int]bool
	toolEventDone        map[string]bool
	text                 strings.Builder
}

func newCodexGoalSSEStreamWriter(c *gin.Context, protocol, model string, created time.Time, deferOutputItem ...bool) *codexGoalSSEStreamWriter {
	if created.IsZero() {
		created = time.Now()
	}
	idSuffix := fmt.Sprintf("%d", created.UnixNano())
	return &codexGoalSSEStreamWriter{
		c:               c,
		protocol:        protocol,
		model:           model,
		created:         created,
		idSuffix:        idSuffix,
		responseID:      "resp_codex_goal_" + idSuffix,
		itemID:          "msg_codex_goal_" + idSuffix,
		chatID:          "chatcmpl-codex-goal-" + idSuffix,
		messageID:       "msg_codex_goal_" + idSuffix,
		deferOutputItem: len(deferOutputItem) > 0 && deferOutputItem[0],
	}
}

func (w *codexGoalSSEStreamWriter) start() {
	if w == nil || w.started {
		return
	}
	w.started = true
	w.c.Header("Content-Type", "text/event-stream")
	w.c.Header("Cache-Control", "no-cache")
	w.c.Header("Connection", "keep-alive")
	switch w.protocol {
	case service.CodexGoalProtocolOpenAIChat:
		writeSSEData(w.c, gin.H{
			"id":      w.chatID,
			"object":  "chat.completion.chunk",
			"created": w.created.Unix(),
			"model":   w.model,
			"choices": []gin.H{{
				"index": 0,
				"delta": gin.H{
					"role": "assistant",
				},
				"finish_reason": nil,
			}},
		})
	case service.CodexGoalProtocolAnthropic:
		writeSSEEvent(w.c, "message_start", gin.H{
			"type": "message_start",
			"message": gin.H{
				"id":            w.messageID,
				"type":          "message",
				"role":          "assistant",
				"model":         w.model,
				"content":       []gin.H{},
				"stop_reason":   nil,
				"stop_sequence": nil,
				"usage": gin.H{
					"input_tokens":  0,
					"output_tokens": 0,
				},
			},
		})
		if w.deferOutputItem {
			flushSSE(w.c)
			return
		}
		w.startAnthropicTextBlock()
	case service.CodexGoalProtocolGemini:
		flushSSE(w.c)
	default:
		writeSSEEvent(w.c, "response.created", gin.H{
			"type": "response.created",
			"response": gin.H{
				"id":         w.responseID,
				"object":     "response",
				"created_at": w.created.Unix(),
				"status":     "in_progress",
				"model":      w.model,
				"output":     []gin.H{},
			},
		})
		if w.deferOutputItem {
			flushSSE(w.c)
			return
		}
		w.startResponsesMessageItem(0)
	}
	flushSSE(w.c)
}

func (w *codexGoalSSEStreamWriter) startAnthropicTextBlock() {
	if w == nil || w.anthropicTextStarted {
		return
	}
	w.anthropicTextStarted = true
	writeSSEEvent(w.c, "content_block_start", gin.H{
		"type":  "content_block_start",
		"index": 0,
		"content_block": gin.H{
			"type": "text",
			"text": "",
		},
	})
}

func (w *codexGoalSSEStreamWriter) startResponsesMessageItem(outputIndex int) {
	if w == nil || w.messageStarted {
		return
	}
	w.messageStarted = true
	writeSSEEvent(w.c, "response.output_item.added", gin.H{
		"type":         "response.output_item.added",
		"response_id":  w.responseID,
		"output_index": outputIndex,
		"item": gin.H{
			"id":      w.itemID,
			"type":    "message",
			"status":  "in_progress",
			"role":    "assistant",
			"content": []gin.H{},
		},
	})
	writeSSEEvent(w.c, "response.content_part.added", gin.H{
		"type":          "response.content_part.added",
		"response_id":   w.responseID,
		"item_id":       w.itemID,
		"output_index":  outputIndex,
		"content_index": w.contentIndex,
		"part": gin.H{
			"type": "output_text",
			"text": "",
		},
	})
}

func (w *codexGoalSSEStreamWriter) finishResponsesMessageItem(outputIndex int, text string) {
	if w == nil {
		return
	}
	w.startResponsesMessageItem(outputIndex)
	if text != "" && w.deferOutputItem {
		writeSSEEvent(w.c, "response.output_text.delta", gin.H{
			"type":          "response.output_text.delta",
			"response_id":   w.responseID,
			"item_id":       w.itemID,
			"output_index":  outputIndex,
			"content_index": w.contentIndex,
			"delta":         text,
		})
	}
	writeSSEEvent(w.c, "response.output_text.done", gin.H{
		"type":          "response.output_text.done",
		"response_id":   w.responseID,
		"item_id":       w.itemID,
		"output_index":  outputIndex,
		"content_index": w.contentIndex,
		"text":          text,
	})
	writeSSEEvent(w.c, "response.content_part.done", gin.H{
		"type":          "response.content_part.done",
		"response_id":   w.responseID,
		"item_id":       w.itemID,
		"output_index":  outputIndex,
		"content_index": w.contentIndex,
		"part": gin.H{
			"type":        "output_text",
			"text":        text,
			"annotations": []any{},
		},
	})
	writeSSEEvent(w.c, "response.output_item.done", gin.H{
		"type":         "response.output_item.done",
		"response_id":  w.responseID,
		"output_index": outputIndex,
		"item": gin.H{
			"id":     w.itemID,
			"type":   "message",
			"status": "completed",
			"role":   "assistant",
			"content": []gin.H{{
				"type":        "output_text",
				"text":        text,
				"annotations": []any{},
			}},
		},
	})
}

func (w *codexGoalSSEStreamWriter) finishResponsesFunctionCall(outputIndex int, call service.CodexGoalFunctionCall) {
	if w == nil {
		return
	}
	id := strings.TrimSpace(call.ID)
	if id == "" {
		id = fmt.Sprintf("fc_codex_goal_%s_%d", w.idSuffix, outputIndex)
	}
	callID := strings.TrimSpace(call.CallID)
	if callID == "" {
		callID = fmt.Sprintf("call_codex_goal_%s_%d", w.idSuffix, outputIndex)
	}
	arguments := strings.TrimSpace(call.Arguments)
	if arguments == "" {
		arguments = "{}"
	}
	name := strings.TrimSpace(call.Name)
	addedItem := gin.H{
		"id":        id,
		"type":      "function_call",
		"status":    "in_progress",
		"call_id":   callID,
		"name":      name,
		"arguments": "",
	}
	doneItem := gin.H{
		"id":        id,
		"type":      "function_call",
		"status":    "completed",
		"call_id":   callID,
		"name":      name,
		"arguments": arguments,
	}
	writeSSEEvent(w.c, "response.output_item.added", gin.H{
		"type":         "response.output_item.added",
		"response_id":  w.responseID,
		"output_index": outputIndex,
		"item":         addedItem,
	})
	if arguments != "" {
		writeSSEEvent(w.c, "response.function_call_arguments.delta", gin.H{
			"type":         "response.function_call_arguments.delta",
			"response_id":  w.responseID,
			"item_id":      id,
			"output_index": outputIndex,
			"call_id":      callID,
			"name":         name,
			"delta":        arguments,
		})
	}
	writeSSEEvent(w.c, "response.function_call_arguments.done", gin.H{
		"type":         "response.function_call_arguments.done",
		"response_id":  w.responseID,
		"item_id":      id,
		"output_index": outputIndex,
		"call_id":      callID,
		"name":         name,
		"arguments":    arguments,
	})
	writeSSEEvent(w.c, "response.output_item.done", gin.H{
		"type":         "response.output_item.done",
		"response_id":  w.responseID,
		"output_index": outputIndex,
		"item":         doneItem,
	})
}

func (w *codexGoalSSEStreamWriter) startFunctionCall(index int, call service.CodexGoalFunctionCall) {
	if w == nil {
		return
	}
	w.start()
	if w.functionCallStarted == nil {
		w.functionCallStarted = map[int]service.CodexGoalFunctionCall{}
	}
	if _, exists := w.functionCallStarted[index]; exists {
		return
	}
	w.functionCallStarted[index] = call
	switch w.protocol {
	case service.CodexGoalProtocolOpenAIChat:
		writeSSEData(w.c, gin.H{
			"id":      w.chatID,
			"object":  "chat.completion.chunk",
			"created": w.created.Unix(),
			"model":   w.model,
			"choices": []gin.H{{
				"index": 0,
				"delta": gin.H{
					"tool_calls": []gin.H{codexGoalChatToolCallDeltaWithArguments(index, call, "", true)},
				},
				"finish_reason": nil,
			}},
		})
	case service.CodexGoalProtocolAnthropic:
		toolUse := codexGoalAnthropicToolUses([]service.CodexGoalFunctionCall{call})[0]
		toolUse["input"] = gin.H{}
		writeSSEEvent(w.c, "content_block_start", gin.H{
			"type":          "content_block_start",
			"index":         index,
			"content_block": toolUse,
		})
	case service.CodexGoalProtocolGemini:
		// Gemini REST streaming normally surfaces functionCall as a part when it is available.
		// Keep the start silent and emit the complete functionCall in finishFunctionCall.
	default:
		id, callID, name := codexGoalFunctionCallIDs(w.idSuffix, index, call)
		writeSSEEvent(w.c, "response.output_item.added", gin.H{
			"type":         "response.output_item.added",
			"response_id":  w.responseID,
			"output_index": index,
			"item": gin.H{
				"id":        id,
				"type":      "function_call",
				"status":    "in_progress",
				"call_id":   callID,
				"name":      name,
				"arguments": "",
			},
		})
	}
	flushSSE(w.c)
}

func (w *codexGoalSSEStreamWriter) functionCallArgumentsDelta(index int, call service.CodexGoalFunctionCall, delta string) {
	if w == nil || delta == "" {
		return
	}
	w.startFunctionCall(index, call)
	switch w.protocol {
	case service.CodexGoalProtocolOpenAIChat:
		writeSSEData(w.c, gin.H{
			"id":      w.chatID,
			"object":  "chat.completion.chunk",
			"created": w.created.Unix(),
			"model":   w.model,
			"choices": []gin.H{{
				"index": 0,
				"delta": gin.H{
					"tool_calls": []gin.H{codexGoalChatToolCallDeltaWithArguments(index, call, delta, false)},
				},
				"finish_reason": nil,
			}},
		})
	case service.CodexGoalProtocolAnthropic:
		writeSSEEvent(w.c, "content_block_delta", gin.H{
			"type":  "content_block_delta",
			"index": index,
			"delta": gin.H{
				"type":         "input_json_delta",
				"partial_json": delta,
			},
		})
	case service.CodexGoalProtocolGemini:
		return
	default:
		id, callID, name := codexGoalFunctionCallIDs(w.idSuffix, index, call)
		writeSSEEvent(w.c, "response.function_call_arguments.delta", gin.H{
			"type":         "response.function_call_arguments.delta",
			"response_id":  w.responseID,
			"item_id":      id,
			"output_index": index,
			"call_id":      callID,
			"name":         name,
			"delta":        delta,
		})
	}
	flushSSE(w.c)
}

func (w *codexGoalSSEStreamWriter) finishFunctionCall(index int, call service.CodexGoalFunctionCall) {
	if w == nil {
		return
	}
	w.startFunctionCall(index, call)
	if w.functionCallDone == nil {
		w.functionCallDone = map[int]bool{}
	}
	if w.functionCallDone[index] {
		return
	}
	w.functionCallDone[index] = true
	switch w.protocol {
	case service.CodexGoalProtocolOpenAIChat:
		return
	case service.CodexGoalProtocolAnthropic:
		writeSSEEvent(w.c, "content_block_stop", gin.H{
			"type":  "content_block_stop",
			"index": index,
		})
	case service.CodexGoalProtocolGemini:
		writeSSEData(w.c, codexGoalGeminiStreamChunk(w.model, "", []service.CodexGoalFunctionCall{call}, "STOP"))
	default:
		id, callID, name := codexGoalFunctionCallIDs(w.idSuffix, index, call)
		arguments := strings.TrimSpace(call.Arguments)
		if arguments == "" {
			arguments = "{}"
		}
		writeSSEEvent(w.c, "response.function_call_arguments.done", gin.H{
			"type":         "response.function_call_arguments.done",
			"response_id":  w.responseID,
			"item_id":      id,
			"output_index": index,
			"call_id":      callID,
			"name":         name,
			"arguments":    arguments,
		})
		writeSSEEvent(w.c, "response.output_item.done", gin.H{
			"type":         "response.output_item.done",
			"response_id":  w.responseID,
			"output_index": index,
			"item": gin.H{
				"id":        id,
				"type":      "function_call",
				"status":    "completed",
				"call_id":   callID,
				"name":      name,
				"arguments": arguments,
			},
		})
	}
	flushSSE(w.c)
}

func (w *codexGoalSSEStreamWriter) functionCallWasStarted(index int) bool {
	if w == nil || w.functionCallStarted == nil {
		return false
	}
	_, ok := w.functionCallStarted[index]
	return ok
}

func (w *codexGoalSSEStreamWriter) functionCallWasDone(index int) bool {
	if w == nil || w.functionCallDone == nil {
		return false
	}
	return w.functionCallDone[index]
}

func codexGoalFunctionCallIDs(idSuffix string, index int, call service.CodexGoalFunctionCall) (id, callID, name string) {
	id = strings.TrimSpace(call.ID)
	if id == "" {
		id = fmt.Sprintf("fc_codex_goal_%s_%d", idSuffix, index)
	}
	callID = strings.TrimSpace(call.CallID)
	if callID == "" {
		callID = fmt.Sprintf("call_codex_goal_%s_%d", idSuffix, index)
	}
	name = strings.TrimSpace(call.Name)
	return id, callID, name
}

func (w *codexGoalSSEStreamWriter) finishResponsesToolEvent(outputIndex int, event service.CodexGoalToolEvent) {
	if w == nil {
		return
	}
	addedItem := codexGoalResponsesToolEventItem(w.idSuffix, outputIndex, event, "in_progress")
	doneItem := codexGoalResponsesToolEventItem(w.idSuffix, outputIndex, event, "completed")
	if addedItem == nil || doneItem == nil {
		return
	}
	if w.toolEventDone == nil {
		w.toolEventDone = map[string]bool{}
	}
	w.toolEventDone[codexGoalToolEventStreamKey(outputIndex, event)] = true
	writeSSEEvent(w.c, "response.output_item.added", gin.H{
		"type":         "response.output_item.added",
		"response_id":  w.responseID,
		"output_index": outputIndex,
		"item":         addedItem,
	})
	writeSSEEvent(w.c, "response.output_item.done", gin.H{
		"type":         "response.output_item.done",
		"response_id":  w.responseID,
		"output_index": outputIndex,
		"item":         doneItem,
	})
}

func (w *codexGoalSSEStreamWriter) toolEvent(index int, event service.CodexGoalToolEvent) {
	if w == nil {
		return
	}
	w.start()
	if w.protocol != service.CodexGoalProtocolOpenAIResponses {
		return
	}
	outputIndex := index
	if w.messageStarted {
		outputIndex++
	}
	w.finishResponsesToolEvent(outputIndex, event)
}

func (w *codexGoalSSEStreamWriter) toolEventWasDone(index int, event service.CodexGoalToolEvent) bool {
	if w == nil || w.toolEventDone == nil {
		return false
	}
	return w.toolEventDone[codexGoalToolEventStreamKey(index, event)] || w.toolEventDone[codexGoalToolEventStreamKey(index+1, event)]
}

func codexGoalToolEventStreamKey(index int, event service.CodexGoalToolEvent) string {
	if id := strings.TrimSpace(event.ID); id != "" {
		return event.Type + ":" + id
	}
	return fmt.Sprintf("%s:%d:%s:%s", event.Type, index, event.ServerLabel, event.Name)
}

func (w *codexGoalSSEStreamWriter) finishAnthropicToolUse(index int, call service.CodexGoalFunctionCall) {
	if w == nil {
		return
	}
	toolUse := codexGoalAnthropicToolUses([]service.CodexGoalFunctionCall{call})[0]
	writeSSEEvent(w.c, "content_block_start", gin.H{
		"type":          "content_block_start",
		"index":         index,
		"content_block": toolUse,
	})
	writeSSEEvent(w.c, "content_block_stop", gin.H{
		"type":  "content_block_stop",
		"index": index,
	})
}

func (w *codexGoalSSEStreamWriter) delta(delta string) {
	if w == nil || delta == "" {
		return
	}
	w.start()
	w.text.WriteString(delta)
	switch w.protocol {
	case service.CodexGoalProtocolOpenAIChat:
		writeSSEData(w.c, gin.H{
			"id":      w.chatID,
			"object":  "chat.completion.chunk",
			"created": w.created.Unix(),
			"model":   w.model,
			"choices": []gin.H{{
				"index": 0,
				"delta": gin.H{
					"content": delta,
				},
				"finish_reason": nil,
			}},
		})
	case service.CodexGoalProtocolAnthropic:
		w.startAnthropicTextBlock()
		writeSSEEvent(w.c, "content_block_delta", gin.H{
			"type":  "content_block_delta",
			"index": 0,
			"delta": gin.H{
				"type": "text_delta",
				"text": delta,
			},
		})
	case service.CodexGoalProtocolGemini:
		writeSSEData(w.c, codexGoalGeminiStreamChunk(w.model, delta, nil, nil))
	default:
		writeSSEEvent(w.c, "response.output_text.delta", gin.H{
			"type":          "response.output_text.delta",
			"response_id":   w.responseID,
			"item_id":       w.itemID,
			"output_index":  0,
			"content_index": w.contentIndex,
			"delta":         delta,
		})
	}
}

func (w *codexGoalSSEStreamWriter) complete(result *service.CodexGoalBridgeResponse) {
	if w == nil {
		return
	}
	w.start()
	text := strings.TrimSpace(w.text.String())
	var toolEvents []service.CodexGoalToolEvent
	var functionCalls []service.CodexGoalFunctionCall
	if result != nil {
		if strings.TrimSpace(result.Text) != "" {
			text = result.Text
		}
		toolEvents = result.ToolEvents
		functionCalls = result.FunctionCalls
	}
	switch w.protocol {
	case service.CodexGoalProtocolOpenAIChat:
		if len(functionCalls) > 0 {
			for i, call := range functionCalls {
				if w.functionCallWasStarted(i) {
					continue
				}
				writeSSEData(w.c, gin.H{
					"id":      w.chatID,
					"object":  "chat.completion.chunk",
					"created": w.created.Unix(),
					"model":   w.model,
					"choices": []gin.H{{
						"index": 0,
						"delta": gin.H{
							"tool_calls": []gin.H{codexGoalChatToolCallDelta(i, call)},
						},
						"finish_reason": nil,
					}},
				})
			}
			writeSSEData(w.c, gin.H{
				"id":      w.chatID,
				"object":  "chat.completion.chunk",
				"created": w.created.Unix(),
				"model":   w.model,
				"choices": []gin.H{{
					"index":         0,
					"delta":         gin.H{},
					"finish_reason": "tool_calls",
				}},
			})
			writeSSERaw(w.c, "data: [DONE]\n\n")
			return
		}
		if strings.TrimSpace(text) != "" && strings.TrimSpace(w.text.String()) == "" {
			writeSSEData(w.c, gin.H{
				"id":      w.chatID,
				"object":  "chat.completion.chunk",
				"created": w.created.Unix(),
				"model":   w.model,
				"choices": []gin.H{{
					"index": 0,
					"delta": gin.H{
						"content": text,
					},
					"finish_reason": nil,
				}},
			})
		}
		writeSSEData(w.c, gin.H{
			"id":      w.chatID,
			"object":  "chat.completion.chunk",
			"created": w.created.Unix(),
			"model":   w.model,
			"choices": []gin.H{{
				"index":         0,
				"delta":         gin.H{},
				"finish_reason": "stop",
			}},
		})
		writeSSERaw(w.c, "data: [DONE]\n\n")
	case service.CodexGoalProtocolAnthropic:
		if len(functionCalls) > 0 {
			for i, call := range functionCalls {
				if w.functionCallWasDone(i) {
					continue
				}
				w.finishAnthropicToolUse(i, call)
			}
			writeSSEEvent(w.c, "message_delta", gin.H{
				"type": "message_delta",
				"delta": gin.H{
					"stop_reason":   "tool_use",
					"stop_sequence": nil,
				},
				"usage": gin.H{
					"output_tokens": 0,
				},
			})
			writeSSEEvent(w.c, "message_stop", gin.H{"type": "message_stop"})
			flushSSE(w.c)
			return
		}
		if strings.TrimSpace(text) != "" && strings.TrimSpace(w.text.String()) == "" {
			w.startAnthropicTextBlock()
			writeSSEEvent(w.c, "content_block_delta", gin.H{
				"type":  "content_block_delta",
				"index": 0,
				"delta": gin.H{
					"type": "text_delta",
					"text": text,
				},
			})
		} else {
			w.startAnthropicTextBlock()
		}
		writeSSEEvent(w.c, "content_block_stop", gin.H{
			"type":  "content_block_stop",
			"index": 0,
		})
		writeSSEEvent(w.c, "message_delta", gin.H{
			"type": "message_delta",
			"delta": gin.H{
				"stop_reason":   "end_turn",
				"stop_sequence": nil,
			},
			"usage": gin.H{
				"output_tokens": 0,
			},
		})
		writeSSEEvent(w.c, "message_stop", gin.H{"type": "message_stop"})
	case service.CodexGoalProtocolGemini:
		if len(functionCalls) > 0 {
			var pending []service.CodexGoalFunctionCall
			for i, call := range functionCalls {
				if w.functionCallWasDone(i) {
					continue
				}
				pending = append(pending, call)
			}
			if len(pending) > 0 {
				writeSSEData(w.c, codexGoalGeminiStreamChunk(w.model, "", pending, "STOP"))
			}
			flushSSE(w.c)
			return
		}
		if strings.TrimSpace(text) != "" && strings.TrimSpace(w.text.String()) == "" {
			writeSSEData(w.c, codexGoalGeminiStreamChunk(w.model, text, nil, nil))
		}
		writeSSEData(w.c, gin.H{
			"candidates": []gin.H{{
				"index":        0,
				"finishReason": "STOP",
			}},
			"modelVersion": w.model,
		})
	default:
		for i, call := range functionCalls {
			if w.functionCallWasDone(i) {
				continue
			}
			w.finishResponsesFunctionCall(i, call)
		}
		outputIndex := len(functionCalls)
		if strings.TrimSpace(text) != "" || len(functionCalls)+len(toolEvents) == 0 {
			w.finishResponsesMessageItem(outputIndex, text)
			outputIndex++
		}
		for i, event := range toolEvents {
			if w.toolEventWasDone(i, event) {
				outputIndex++
				continue
			}
			w.finishResponsesToolEvent(outputIndex, event)
			outputIndex++
		}
		writeSSEEvent(w.c, "response.completed", gin.H{
			"type": "response.completed",
			"response": gin.H{
				"id":         w.responseID,
				"object":     "response",
				"created_at": w.created.Unix(),
				"status":     "completed",
				"model":      w.model,
				"output":     codexGoalResponsesStreamingOutputItems(w.idSuffix, text, toolEvents, functionCalls),
			},
		})
	}
	flushSSE(w.c)
}

func (w *codexGoalSSEStreamWriter) fail(err *service.CodexGoalBridgeError) {
	if w == nil || err == nil {
		return
	}
	w.start()
	payload := gin.H{
		"type":    err.Code,
		"code":    err.Code,
		"message": err.Message,
		"status":  err.StatusCode,
	}
	switch w.protocol {
	case service.CodexGoalProtocolOpenAIChat:
		writeSSEEvent(w.c, "error", gin.H{"error": payload})
		writeSSERaw(w.c, "data: [DONE]\n\n")
	case service.CodexGoalProtocolAnthropic:
		writeSSEEvent(w.c, "error", gin.H{
			"type":  "error",
			"error": payload,
		})
	case service.CodexGoalProtocolGemini:
		writeSSEData(w.c, gin.H{
			"error": gin.H{
				"code":    err.StatusCode,
				"message": err.Message,
				"status":  geminiStatusForHTTP(err.StatusCode),
			},
		})
	default:
		writeSSEEvent(w.c, "response.failed", gin.H{
			"type": "response.failed",
			"response": gin.H{
				"id":         w.responseID,
				"object":     "response",
				"created_at": w.created.Unix(),
				"status":     "failed",
				"model":      w.model,
				"error": gin.H{
					"code":    err.Code,
					"message": err.Message,
				},
			},
		})
	}
	flushSSE(w.c)
}

func (h *CodexGoalBridgeHandler) writeStream(c *gin.Context, result *service.CodexGoalBridgeResponse) {
	created := result.CreatedAt
	if created.IsZero() {
		created = time.Now()
	}
	idSuffix := fmt.Sprintf("%d", created.UnixNano())
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	switch result.Protocol {
	case service.CodexGoalProtocolOpenAIChat:
		id := "chatcmpl-codex-goal-" + idSuffix
		if len(result.FunctionCalls) > 0 {
			writeSSEData(c, gin.H{
				"id":      id,
				"object":  "chat.completion.chunk",
				"created": created.Unix(),
				"model":   result.Model,
				"choices": []gin.H{{
					"index": 0,
					"delta": gin.H{
						"role": "assistant",
					},
					"finish_reason": nil,
				}},
			})
			for i, call := range result.FunctionCalls {
				writeSSEData(c, gin.H{
					"id":      id,
					"object":  "chat.completion.chunk",
					"created": created.Unix(),
					"model":   result.Model,
					"choices": []gin.H{{
						"index": 0,
						"delta": gin.H{
							"tool_calls": []gin.H{codexGoalChatToolCallDelta(i, call)},
						},
						"finish_reason": nil,
					}},
				})
			}
			writeSSEData(c, gin.H{
				"id":      id,
				"object":  "chat.completion.chunk",
				"created": created.Unix(),
				"model":   result.Model,
				"choices": []gin.H{{
					"index":         0,
					"delta":         gin.H{},
					"finish_reason": "tool_calls",
				}},
			})
			writeSSERaw(c, "data: [DONE]\n\n")
			break
		}
		writeSSEData(c, gin.H{
			"id":      id,
			"object":  "chat.completion.chunk",
			"created": created.Unix(),
			"model":   result.Model,
			"choices": []gin.H{{
				"index": 0,
				"delta": gin.H{
					"role":    "assistant",
					"content": result.Text,
				},
				"finish_reason": nil,
			}},
		})
		writeSSEData(c, gin.H{
			"id":      id,
			"object":  "chat.completion.chunk",
			"created": created.Unix(),
			"model":   result.Model,
			"choices": []gin.H{{
				"index":         0,
				"delta":         gin.H{},
				"finish_reason": "stop",
			}},
		})
		writeSSERaw(c, "data: [DONE]\n\n")
	case service.CodexGoalProtocolAnthropic:
		id := "msg_codex_goal_" + idSuffix
		writeSSEEvent(c, "message_start", gin.H{
			"type": "message_start",
			"message": gin.H{
				"id":            id,
				"type":          "message",
				"role":          "assistant",
				"model":         result.Model,
				"content":       []gin.H{},
				"stop_reason":   nil,
				"stop_sequence": nil,
				"usage": gin.H{
					"input_tokens":  0,
					"output_tokens": 0,
				},
			},
		})
		if len(result.FunctionCalls) > 0 {
			for i, call := range result.FunctionCalls {
				toolUse := codexGoalAnthropicToolUses([]service.CodexGoalFunctionCall{call})[0]
				writeSSEEvent(c, "content_block_start", gin.H{
					"type":          "content_block_start",
					"index":         i,
					"content_block": toolUse,
				})
				writeSSEEvent(c, "content_block_stop", gin.H{
					"type":  "content_block_stop",
					"index": i,
				})
			}
			writeSSEEvent(c, "message_delta", gin.H{
				"type": "message_delta",
				"delta": gin.H{
					"stop_reason":   "tool_use",
					"stop_sequence": nil,
				},
				"usage": gin.H{
					"output_tokens": 0,
				},
			})
			writeSSEEvent(c, "message_stop", gin.H{"type": "message_stop"})
			break
		}
		writeSSEEvent(c, "content_block_start", gin.H{
			"type":  "content_block_start",
			"index": 0,
			"content_block": gin.H{
				"type": "text",
				"text": "",
			},
		})
		writeSSEEvent(c, "content_block_delta", gin.H{
			"type":  "content_block_delta",
			"index": 0,
			"delta": gin.H{
				"type": "text_delta",
				"text": result.Text,
			},
		})
		writeSSEEvent(c, "content_block_stop", gin.H{
			"type":  "content_block_stop",
			"index": 0,
		})
		writeSSEEvent(c, "message_delta", gin.H{
			"type": "message_delta",
			"delta": gin.H{
				"stop_reason":   "end_turn",
				"stop_sequence": nil,
			},
			"usage": gin.H{
				"output_tokens": 0,
			},
		})
		writeSSEEvent(c, "message_stop", gin.H{"type": "message_stop"})
	case service.CodexGoalProtocolGemini:
		writeSSEData(c, codexGoalGeminiStreamChunk(result.Model, result.Text, result.FunctionCalls, "STOP"))
	default:
		id := "resp_codex_goal_" + idSuffix
		msgID := "msg_codex_goal_" + idSuffix
		writeSSEEvent(c, "response.created", gin.H{
			"type": "response.created",
			"response": gin.H{
				"id":         id,
				"object":     "response",
				"created_at": created.Unix(),
				"status":     "in_progress",
				"model":      result.Model,
			},
		})
		writeSSEEvent(c, "response.output_text.delta", gin.H{
			"type":          "response.output_text.delta",
			"item_id":       msgID,
			"output_index":  0,
			"content_index": 0,
			"delta":         result.Text,
		})
		writeSSEEvent(c, "response.completed", gin.H{
			"type": "response.completed",
			"response": gin.H{
				"id":         id,
				"object":     "response",
				"created_at": created.Unix(),
				"status":     "completed",
				"model":      result.Model,
				"output":     codexGoalResponsesOutputItems(idSuffix, result.Text, result.ToolEvents, result.FunctionCalls),
			},
		})
	}
	flushSSE(c)
}

func (h *CodexGoalBridgeHandler) writeError(c *gin.Context, protocol string, err error) {
	bridgeErr := normalizeCodexGoalBridgeError(err)
	status := bridgeErr.StatusCode
	if status <= 0 {
		status = http.StatusBadGateway
	}
	code := strings.TrimSpace(bridgeErr.Code)
	if code == "" {
		code = "codex_goal_bridge_failed"
	}
	message := strings.TrimSpace(bridgeErr.Message)
	if message == "" {
		message = "Codex goal bridge failed"
	}
	switch protocol {
	case service.CodexGoalProtocolAnthropic:
		c.JSON(status, gin.H{
			"type": "error",
			"error": gin.H{
				"type":    code,
				"message": message,
			},
		})
	case service.CodexGoalProtocolGemini:
		c.JSON(status, gin.H{
			"error": gin.H{
				"code":    status,
				"message": message,
				"status":  geminiStatusForHTTP(status),
			},
		})
	default:
		c.JSON(status, gin.H{
			"error": gin.H{
				"type":    code,
				"code":    code,
				"message": message,
			},
		})
	}
}

func normalizeCodexGoalBridgeError(err error) *service.CodexGoalBridgeError {
	bridgeErr := &service.CodexGoalBridgeError{
		StatusCode: http.StatusBadGateway,
		Code:       "codex_goal_bridge_failed",
		Message:    err.Error(),
	}
	var typedErr *service.CodexGoalBridgeError
	if errors.As(err, &typedErr) && typedErr != nil {
		bridgeErr = typedErr
	}
	return bridgeErr
}

func geminiStatusForHTTP(status int) string {
	switch status {
	case http.StatusBadRequest:
		return "INVALID_ARGUMENT"
	case http.StatusUnauthorized:
		return "UNAUTHENTICATED"
	case http.StatusForbidden:
		return "PERMISSION_DENIED"
	case http.StatusNotFound:
		return "NOT_FOUND"
	case http.StatusTooManyRequests:
		return "RESOURCE_EXHAUSTED"
	default:
		return "UNAVAILABLE"
	}
}

func writeSSEData(c *gin.Context, payload any) {
	writeSSEEvent(c, "", payload)
}

func writeSSEEvent(c *gin.Context, event string, payload any) {
	data, err := json.Marshal(payload)
	if err != nil {
		return
	}
	if event != "" {
		writeSSERaw(c, "event: "+event+"\n")
	}
	writeSSERaw(c, "data: "+string(data)+"\n\n")
}

func writeSSERaw(c *gin.Context, text string) {
	_, _ = c.Writer.WriteString(text)
	flushSSE(c)
}

func flushSSE(c *gin.Context) {
	if flusher, ok := c.Writer.(http.Flusher); ok {
		flusher.Flush()
	}
}

type codexGoalWSStreamWriter struct {
	ctx                 context.Context
	conn                *coderws.Conn
	model               string
	created             time.Time
	idSuffix            string
	responseID          string
	itemID              string
	contentIndex        int
	started             bool
	deferOutputItem     bool
	messageStarted      bool
	functionCallStarted map[int]service.CodexGoalFunctionCall
	functionCallDone    map[int]bool
	toolEventDone       map[string]bool
	text                strings.Builder
}

func newCodexGoalWSStreamWriter(ctx context.Context, conn *coderws.Conn, model string, created time.Time, deferOutputItem ...bool) *codexGoalWSStreamWriter {
	if created.IsZero() {
		created = time.Now()
	}
	idSuffix := fmt.Sprintf("%d", created.UnixNano())
	return &codexGoalWSStreamWriter{
		ctx:             ctx,
		conn:            conn,
		model:           model,
		created:         created,
		idSuffix:        idSuffix,
		responseID:      "resp_codex_goal_" + idSuffix,
		itemID:          "msg_codex_goal_" + idSuffix,
		deferOutputItem: len(deferOutputItem) > 0 && deferOutputItem[0],
	}
}

func (w *codexGoalWSStreamWriter) write(event gin.H) error {
	if w == nil {
		return nil
	}
	return writeCodexGoalWSJSON(w.ctx, w.conn, event)
}

func (w *codexGoalWSStreamWriter) start() error {
	if w == nil || w.started {
		return nil
	}
	w.started = true
	if err := w.write(gin.H{
		"type": "response.created",
		"response": gin.H{
			"id":         w.responseID,
			"object":     "response",
			"created_at": w.created.Unix(),
			"status":     "in_progress",
			"model":      w.model,
			"output":     []gin.H{},
		},
	}); err != nil {
		return err
	}
	if w.deferOutputItem {
		return nil
	}
	return w.startMessageItem(0)
}

func (w *codexGoalWSStreamWriter) startMessageItem(outputIndex int) error {
	if w == nil || w.messageStarted {
		return nil
	}
	w.messageStarted = true
	if err := w.write(gin.H{
		"type":         "response.output_item.added",
		"response_id":  w.responseID,
		"output_index": outputIndex,
		"item": gin.H{
			"id":      w.itemID,
			"type":    "message",
			"status":  "in_progress",
			"role":    "assistant",
			"content": []gin.H{},
		},
	}); err != nil {
		return err
	}
	return w.write(gin.H{
		"type":          "response.content_part.added",
		"response_id":   w.responseID,
		"item_id":       w.itemID,
		"output_index":  outputIndex,
		"content_index": w.contentIndex,
		"part": gin.H{
			"type": "output_text",
			"text": "",
		},
	})
}

func (w *codexGoalWSStreamWriter) delta(delta string) error {
	if w == nil || delta == "" {
		return nil
	}
	if err := w.start(); err != nil {
		return err
	}
	if err := w.startMessageItem(0); err != nil {
		return err
	}
	w.text.WriteString(delta)
	return w.write(gin.H{
		"type":          "response.output_text.delta",
		"response_id":   w.responseID,
		"item_id":       w.itemID,
		"output_index":  0,
		"content_index": w.contentIndex,
		"delta":         delta,
	})
}

func (w *codexGoalWSStreamWriter) toolEvent(index int, event service.CodexGoalToolEvent) error {
	if w == nil {
		return nil
	}
	if err := w.start(); err != nil {
		return err
	}
	outputIndex := index
	if w.messageStarted {
		outputIndex++
	}
	return w.finishToolEvent(outputIndex, event)
}

func (w *codexGoalWSStreamWriter) finishToolEvent(outputIndex int, event service.CodexGoalToolEvent) error {
	addedItem := codexGoalResponsesToolEventItem(w.idSuffix, outputIndex, event, "in_progress")
	doneItem := codexGoalResponsesToolEventItem(w.idSuffix, outputIndex, event, "completed")
	if addedItem == nil || doneItem == nil {
		return nil
	}
	if w.toolEventDone == nil {
		w.toolEventDone = map[string]bool{}
	}
	w.toolEventDone[codexGoalToolEventStreamKey(outputIndex, event)] = true
	if err := w.write(gin.H{
		"type":         "response.output_item.added",
		"response_id":  w.responseID,
		"output_index": outputIndex,
		"item":         addedItem,
	}); err != nil {
		return err
	}
	return w.write(gin.H{
		"type":         "response.output_item.done",
		"response_id":  w.responseID,
		"output_index": outputIndex,
		"item":         doneItem,
	})
}

func (w *codexGoalWSStreamWriter) startFunctionCall(index int, call service.CodexGoalFunctionCall) error {
	if w == nil {
		return nil
	}
	if err := w.start(); err != nil {
		return err
	}
	if w.functionCallStarted == nil {
		w.functionCallStarted = map[int]service.CodexGoalFunctionCall{}
	}
	if _, exists := w.functionCallStarted[index]; exists {
		return nil
	}
	w.functionCallStarted[index] = call
	id, callID, name := codexGoalFunctionCallIDs(w.idSuffix, index, call)
	return w.write(gin.H{
		"type":         "response.output_item.added",
		"response_id":  w.responseID,
		"output_index": index,
		"item": gin.H{
			"id":        id,
			"type":      "function_call",
			"status":    "in_progress",
			"call_id":   callID,
			"name":      name,
			"arguments": "",
		},
	})
}

func (w *codexGoalWSStreamWriter) functionCallArgumentsDelta(index int, call service.CodexGoalFunctionCall, delta string) error {
	if w == nil || delta == "" {
		return nil
	}
	if err := w.startFunctionCall(index, call); err != nil {
		return err
	}
	id, callID, name := codexGoalFunctionCallIDs(w.idSuffix, index, call)
	return w.write(gin.H{
		"type":         "response.function_call_arguments.delta",
		"response_id":  w.responseID,
		"item_id":      id,
		"output_index": index,
		"call_id":      callID,
		"name":         name,
		"delta":        delta,
	})
}

func (w *codexGoalWSStreamWriter) finishFunctionCall(index int, call service.CodexGoalFunctionCall) error {
	if w == nil {
		return nil
	}
	if err := w.startFunctionCall(index, call); err != nil {
		return err
	}
	if w.functionCallDone == nil {
		w.functionCallDone = map[int]bool{}
	}
	if w.functionCallDone[index] {
		return nil
	}
	w.functionCallDone[index] = true
	id, callID, name := codexGoalFunctionCallIDs(w.idSuffix, index, call)
	arguments := strings.TrimSpace(call.Arguments)
	if arguments == "" {
		arguments = "{}"
	}
	if err := w.write(gin.H{
		"type":         "response.function_call_arguments.done",
		"response_id":  w.responseID,
		"item_id":      id,
		"output_index": index,
		"call_id":      callID,
		"name":         name,
		"arguments":    arguments,
	}); err != nil {
		return err
	}
	return w.write(gin.H{
		"type":         "response.output_item.done",
		"response_id":  w.responseID,
		"output_index": index,
		"item": gin.H{
			"id":        id,
			"type":      "function_call",
			"status":    "completed",
			"call_id":   callID,
			"name":      name,
			"arguments": arguments,
		},
	})
}

func (w *codexGoalWSStreamWriter) functionCallWasDone(index int) bool {
	return w != nil && w.functionCallDone != nil && w.functionCallDone[index]
}

func (w *codexGoalWSStreamWriter) toolEventWasDone(index int, event service.CodexGoalToolEvent) bool {
	if w == nil || w.toolEventDone == nil {
		return false
	}
	return w.toolEventDone[codexGoalToolEventStreamKey(index, event)] || w.toolEventDone[codexGoalToolEventStreamKey(index+1, event)]
}

func (w *codexGoalWSStreamWriter) finishMessage(outputIndex int, text string) error {
	if err := w.startMessageItem(outputIndex); err != nil {
		return err
	}
	if text != "" && w.text.Len() == 0 {
		if err := w.write(gin.H{
			"type":          "response.output_text.delta",
			"response_id":   w.responseID,
			"item_id":       w.itemID,
			"output_index":  outputIndex,
			"content_index": w.contentIndex,
			"delta":         text,
		}); err != nil {
			return err
		}
	}
	if err := w.write(gin.H{
		"type":          "response.output_text.done",
		"response_id":   w.responseID,
		"item_id":       w.itemID,
		"output_index":  outputIndex,
		"content_index": w.contentIndex,
		"text":          text,
	}); err != nil {
		return err
	}
	if err := w.write(gin.H{
		"type":          "response.content_part.done",
		"response_id":   w.responseID,
		"item_id":       w.itemID,
		"output_index":  outputIndex,
		"content_index": w.contentIndex,
		"part": gin.H{
			"type":        "output_text",
			"text":        text,
			"annotations": []any{},
		},
	}); err != nil {
		return err
	}
	return w.write(gin.H{
		"type":         "response.output_item.done",
		"response_id":  w.responseID,
		"output_index": outputIndex,
		"item": gin.H{
			"id":     w.itemID,
			"type":   "message",
			"status": "completed",
			"role":   "assistant",
			"content": []gin.H{{
				"type":        "output_text",
				"text":        text,
				"annotations": []any{},
			}},
		},
	})
}

func (w *codexGoalWSStreamWriter) complete(result *service.CodexGoalBridgeResponse) error {
	if result == nil {
		return fmt.Errorf("empty Codex goal bridge response")
	}
	if err := w.start(); err != nil {
		return err
	}
	text := strings.TrimSpace(w.text.String())
	if strings.TrimSpace(result.Text) != "" {
		text = result.Text
	}
	functionCalls := result.FunctionCalls
	for i, call := range functionCalls {
		if w.functionCallWasDone(i) {
			continue
		}
		if err := w.finishFunctionCall(i, call); err != nil {
			return err
		}
	}
	outputIndex := len(functionCalls)
	if strings.TrimSpace(text) != "" || len(functionCalls)+len(result.ToolEvents) == 0 {
		if err := w.finishMessage(outputIndex, text); err != nil {
			return err
		}
		outputIndex++
	}
	for i, event := range result.ToolEvents {
		if w.toolEventWasDone(i, event) {
			outputIndex++
			continue
		}
		if err := w.finishToolEvent(outputIndex, event); err != nil {
			return err
		}
		outputIndex++
	}
	return w.write(gin.H{
		"type": "response.completed",
		"response": gin.H{
			"id":         w.responseID,
			"object":     "response",
			"created_at": w.created.Unix(),
			"status":     "completed",
			"model":      result.Model,
			"output":     codexGoalResponsesOutputItems(w.idSuffix, text, result.ToolEvents, result.FunctionCalls),
		},
	})
}

func writeCodexGoalResponsesWSEvents(ctx context.Context, conn *coderws.Conn, result *service.CodexGoalBridgeResponse) error {
	if result == nil {
		return fmt.Errorf("empty Codex goal bridge response")
	}
	created := result.CreatedAt
	if created.IsZero() {
		created = time.Now()
	}
	idSuffix := fmt.Sprintf("%d", created.UnixNano())
	responseID := "resp_codex_goal_" + idSuffix
	itemID := "msg_codex_goal_" + idSuffix
	contentIndex := 0
	events := []gin.H{
		{
			"type": "response.created",
			"response": gin.H{
				"id":         responseID,
				"object":     "response",
				"created_at": created.Unix(),
				"status":     "in_progress",
				"model":      result.Model,
				"output":     []gin.H{},
			},
		},
		{
			"type":         "response.output_item.added",
			"response_id":  responseID,
			"output_index": 0,
			"item": gin.H{
				"id":      itemID,
				"type":    "message",
				"status":  "in_progress",
				"role":    "assistant",
				"content": []gin.H{},
			},
		},
		{
			"type":          "response.content_part.added",
			"response_id":   responseID,
			"item_id":       itemID,
			"output_index":  0,
			"content_index": contentIndex,
			"part": gin.H{
				"type": "output_text",
				"text": "",
			},
		},
		{
			"type":          "response.output_text.delta",
			"response_id":   responseID,
			"item_id":       itemID,
			"output_index":  0,
			"content_index": contentIndex,
			"delta":         result.Text,
		},
		{
			"type":          "response.output_text.done",
			"response_id":   responseID,
			"item_id":       itemID,
			"output_index":  0,
			"content_index": contentIndex,
			"text":          result.Text,
		},
		{
			"type":          "response.content_part.done",
			"response_id":   responseID,
			"item_id":       itemID,
			"output_index":  0,
			"content_index": contentIndex,
			"part": gin.H{
				"type":        "output_text",
				"text":        result.Text,
				"annotations": []any{},
			},
		},
		{
			"type":         "response.output_item.done",
			"response_id":  responseID,
			"output_index": 0,
			"item": gin.H{
				"id":     itemID,
				"type":   "message",
				"status": "completed",
				"role":   "assistant",
				"content": []gin.H{{
					"type":        "output_text",
					"text":        result.Text,
					"annotations": []any{},
				}},
			},
		},
		{
			"type": "response.completed",
			"response": gin.H{
				"id":         responseID,
				"object":     "response",
				"created_at": created.Unix(),
				"status":     "completed",
				"model":      result.Model,
				"output":     codexGoalResponsesOutputItems(idSuffix, result.Text, result.ToolEvents, result.FunctionCalls),
			},
		},
	}
	for _, event := range events {
		if err := writeCodexGoalWSJSON(ctx, conn, event); err != nil {
			return err
		}
	}
	return nil
}

func writeCodexGoalWSError(ctx context.Context, conn *coderws.Conn, status int, code, message string) {
	if code == "" {
		code = "codex_goal_bridge_failed"
	}
	if message == "" {
		message = "Codex goal bridge failed"
	}
	_ = writeCodexGoalWSJSON(ctx, conn, gin.H{
		"type": "error",
		"error": gin.H{
			"type":    code,
			"code":    code,
			"message": message,
			"status":  status,
		},
	})
}

func writeCodexGoalWSJSON(ctx context.Context, conn *coderws.Conn, payload any) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	writeCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	return conn.Write(writeCtx, coderws.MessageText, data)
}

func closeCodexGoalWS(conn *coderws.Conn, status coderws.StatusCode, reason string) {
	_ = conn.Close(status, reason)
}

func codexGoalWSCloseStatus(status int) coderws.StatusCode {
	if status >= 500 {
		return coderws.StatusInternalError
	}
	if status == http.StatusTooManyRequests {
		return coderws.StatusTryAgainLater
	}
	return coderws.StatusPolicyViolation
}

func isCodexGoalWSUpgradeRequest(r *http.Request) bool {
	if r == nil {
		return false
	}
	if !strings.EqualFold(strings.TrimSpace(r.Header.Get("Upgrade")), "websocket") {
		return false
	}
	return strings.Contains(strings.ToLower(r.Header.Get("Connection")), "upgrade")
}
