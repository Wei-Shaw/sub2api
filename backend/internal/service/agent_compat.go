package service

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

	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
	"github.com/Wei-Shaw/sub2api/internal/pkg/connectrpc"
	"github.com/Wei-Shaw/sub2api/internal/pkg/cursor"
	"github.com/Wei-Shaw/sub2api/internal/pkg/devin"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type agentCompatProtocol uint8

const (
	agentCompatMessages agentCompatProtocol = iota
	agentCompatResponses
	agentCompatChatCompletions
)

type agentRequest struct {
	Model        string
	System       string
	UserText     string
	Messages     []devin.ChatMessage
	Tools        []agentTool
	Conversation string
}

type agentTool struct {
	Name        string
	Description string
	SchemaJSON  string
}

type agentEvent struct {
	Kind      string
	Text      string
	CallID    string
	ToolName  string
	Arguments string
	Input     int
	Output    int
	CacheR    int
	CacheW    int
	Done      bool
}

type AgentCompatGatewayService struct {
	cursorTokens *CursorTokenProvider
	stream       func(ctx context.Context, account *Account, req agentRequest, emit func(agentEvent) error) error
}

func NewAgentCompatGatewayService(cursorTokens *CursorTokenProvider) *AgentCompatGatewayService {
	svc := &AgentCompatGatewayService{cursorTokens: cursorTokens}
	svc.stream = svc.streamUpstream
	return svc
}

func ShouldUseAgentCompat(account *Account) bool {
	return account != nil && (account.Platform == PlatformCursor || account.Platform == PlatformDevin)
}

func (s *AgentCompatGatewayService) Forward(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	body []byte,
	parsed *ParsedRequest,
) (*ForwardResult, error) {
	var request apicompat.AnthropicRequest
	if json.Unmarshal(body, &request) != nil {
		return nil, s.writeError(c, http.StatusBadRequest, "invalid_request_error", "Failed to parse request body")
	}
	if strings.TrimSpace(request.Model) == "" {
		return nil, s.writeError(c, http.StatusBadRequest, "invalid_request_error", "model is required")
	}
	responsesRequest, err := apicompat.AnthropicToResponses(&request)
	if err != nil {
		return nil, s.writeError(c, http.StatusBadRequest, "invalid_request_error", err.Error())
	}
	return s.forward(ctx, c, account, agentCompatMessages, responsesRequest, parsed, request.Stream, false, time.Now())
}

func (s *AgentCompatGatewayService) ForwardAsResponses(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	body []byte,
	parsed *ParsedRequest,
) (*ForwardResult, error) {
	var request apicompat.ResponsesRequest
	if json.Unmarshal(body, &request) != nil {
		return nil, s.writeError(c, http.StatusBadRequest, "invalid_request_error", "Failed to parse request body")
	}
	if strings.TrimSpace(request.Model) == "" {
		return nil, s.writeError(c, http.StatusBadRequest, "invalid_request_error", "model is required")
	}
	return s.forward(ctx, c, account, agentCompatResponses, &request, parsed, request.Stream, false, time.Now())
}

func (s *AgentCompatGatewayService) ForwardAsChatCompletions(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	body []byte,
	parsed *ParsedRequest,
) (*ForwardResult, error) {
	var request apicompat.ChatCompletionsRequest
	if json.Unmarshal(body, &request) != nil {
		return nil, s.writeError(c, http.StatusBadRequest, "invalid_request_error", "Failed to parse request body")
	}
	if strings.TrimSpace(request.Model) == "" {
		return nil, s.writeError(c, http.StatusBadRequest, "invalid_request_error", "model is required")
	}
	responsesRequest, err := apicompat.ChatCompletionsToResponses(&request)
	if err != nil {
		return nil, s.writeError(c, http.StatusBadRequest, "invalid_request_error", err.Error())
	}
	includeUsage := request.StreamOptions != nil && request.StreamOptions.IncludeUsage
	return s.forward(ctx, c, account, agentCompatChatCompletions, responsesRequest, parsed, request.Stream, includeUsage, time.Now())
}

func (s *AgentCompatGatewayService) forward(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	protocol agentCompatProtocol,
	request *apicompat.ResponsesRequest,
	parsed *ParsedRequest,
	stream bool,
	includeUsage bool,
	startTime time.Time,
) (*ForwardResult, error) {
	if account == nil || !ShouldUseAgentCompat(account) {
		return nil, s.writeError(c, http.StatusBadRequest, "invalid_request_error", "account is not a Cursor or Devin OAuth account")
	}
	agentReq, err := flattenAgentRequest(request)
	if err != nil {
		return nil, s.writeError(c, http.StatusBadRequest, "invalid_request_error", err.Error())
	}
	// The body already carries channel mapping; apply account mapping only to
	// the upstream request, leaving the public model intact for every protocol.
	agentReq.Model = account.GetMappedModel(agentReq.Model)
	originalModel := request.Model
	if parsed != nil && strings.TrimSpace(parsed.Model) != "" {
		originalModel = parsed.Model
	}
	if publicModel, ok := RequestedPublicModelFromContext(ctx); ok {
		originalModel = publicModel
	}
	if s.stream == nil {
		return nil, s.writeError(c, http.StatusBadGateway, "api_error", "agent compat stream is not configured")
	}

	emitter := newAgentResponsesEmitter(originalModel)
	anthState := apicompat.NewResponsesEventToAnthropicState()
	chatState := apicompat.NewResponsesEventToChatState()
	chatState.IncludeUsage = includeUsage
	var collected []apicompat.ResponsesStreamEvent
	var firstToken *int
	sawContent := false

	writeEvent := func(evt apicompat.ResponsesStreamEvent) error {
		if stream {
			return writeAgentCompatSSE(c, protocol, evt, anthState, chatState)
		}
		collected = append(collected, evt)
		return nil
	}

	err = s.stream(ctx, account, agentReq, func(ev agentEvent) error {
		if ev.Kind == "text" || ev.Kind == "thinking" || ev.Kind == "tool_start" {
			if !sawContent {
				sawContent = true
				ms := int(time.Since(startTime).Milliseconds())
				firstToken = &ms
			}
		}
		for _, evt := range emitter.apply(ev) {
			if err := writeEvent(evt); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return nil, s.writeError(c, http.StatusBadGateway, "client_disconnected", "Client disconnected before upstream response")
		}
		return nil, &UpstreamFailoverError{
			StatusCode: http.StatusBadGateway,
			Stage:      GatewayFailureStageInference,
			Scope:      GatewayFailureScopeAccount,
			Reason:     GatewayFailureReason("agent_compat_upstream_error"),
		}
	}
	for _, evt := range emitter.finish() {
		if err := writeEvent(evt); err != nil {
			return nil, err
		}
	}
	if stream {
		if protocol == agentCompatChatCompletions {
			_, _ = c.Writer.WriteString("data: [DONE]\n\n")
			if flusher, ok := c.Writer.(http.Flusher); ok {
				flusher.Flush()
			}
		}
	} else if err := writeAgentCompatJSON(c, protocol, originalModel, collected); err != nil {
		return nil, err
	}

	usage := ClaudeUsage{
		InputTokens:              emitter.usage.InputTokens,
		OutputTokens:             emitter.usage.OutputTokens,
		CacheReadInputTokens:     agentCacheReadTokens(emitter.usage),
		CacheCreationInputTokens: emitter.usage.CacheCreationInputTokens,
	}
	if usage.InputTokens == 0 && usage.OutputTokens == 0 {
		usage.OutputTokens = 1
	}
	return &ForwardResult{
		Model:         originalModel,
		UpstreamModel: agentReq.Model,
		Usage:         usage,
		Stream:        stream,
		Duration:      time.Since(startTime),
		FirstTokenMs:  firstToken,
	}, nil
}

func flattenAgentRequest(req *apicompat.ResponsesRequest) (agentRequest, error) {
	if req == nil {
		return agentRequest{}, errors.New("request is required")
	}
	out := agentRequest{
		Model:        strings.TrimSpace(req.Model),
		System:       strings.TrimSpace(req.Instructions),
		Conversation: uuid.NewString(),
	}
	if out.Model == "" {
		return agentRequest{}, errors.New("model is required")
	}
	for _, tool := range req.Tools {
		if tool.Type != "" && tool.Type != "function" && tool.Type != "custom" {
			continue
		}
		name := strings.TrimSpace(tool.Name)
		if name == "" {
			continue
		}
		schema := "{}"
		if len(tool.Parameters) > 0 {
			schema = string(tool.Parameters)
		}
		out.Tools = append(out.Tools, agentTool{Name: name, Description: tool.Description, SchemaJSON: schema})
	}

	var items []apicompat.ResponsesInputItem
	raw := bytes.TrimSpace(req.Input)
	if len(raw) > 0 {
		if raw[0] == '"' {
			var text string
			if err := json.Unmarshal(raw, &text); err == nil {
				out.UserText = strings.TrimSpace(text)
				if out.UserText != "" {
					out.Messages = append(out.Messages, devin.ChatMessage{ID: uuid.NewString(), Source: devin.SourceUser, Prompt: out.UserText})
				}
				return out, nil
			}
		}
		if err := json.Unmarshal(raw, &items); err != nil {
			return agentRequest{}, fmt.Errorf("invalid input: %w", err)
		}
	}
	for _, item := range items {
		switch {
		case item.Type == "function_call":
			out.Messages = append(out.Messages, devin.ChatMessage{
				ID:     firstNonEmpty(item.ID, uuid.NewString()),
				Source: devin.SourceSystem,
				Prompt: item.Name + " " + item.Arguments,
			})
		case item.Type == "function_call_output":
			out.Messages = append(out.Messages, devin.ChatMessage{
				ID:     firstNonEmpty(item.ID, uuid.NewString()),
				Source: devin.SourceTool,
				Prompt: item.Output,
				ToolID: item.CallID,
			})
		default:
			text := flattenResponsesContent(item)
			if text == "" {
				continue
			}
			source := devin.SourceUser
			switch item.Role {
			case "assistant":
				source = devin.SourceSystem
			case "system", "developer":
				if out.System == "" {
					out.System = text
					continue
				}
				source = devin.SourceSystem
			}
			out.Messages = append(out.Messages, devin.ChatMessage{ID: firstNonEmpty(item.ID, uuid.NewString()), Source: source, Prompt: text})
			if source == devin.SourceUser {
				out.UserText = text
			}
		}
	}
	if out.UserText == "" && len(out.Messages) > 0 {
		out.UserText = out.Messages[len(out.Messages)-1].Prompt
	}
	if out.UserText == "" {
		return agentRequest{}, errors.New("input is required")
	}
	return out, nil
}

func flattenResponsesContent(item apicompat.ResponsesInputItem) string {
	raw := bytes.TrimSpace(item.Content)
	if len(raw) == 0 {
		return strings.TrimSpace(item.Output)
	}
	if raw[0] == '"' {
		var text string
		if err := json.Unmarshal(raw, &text); err == nil {
			return strings.TrimSpace(text)
		}
	}
	var parts []apicompat.ResponsesContentPart
	if err := json.Unmarshal(raw, &parts); err != nil {
		return strings.TrimSpace(string(raw))
	}
	var b strings.Builder
	for _, part := range parts {
		if text := strings.TrimSpace(part.Text); text != "" {
			if b.Len() > 0 {
				b.WriteByte('\n')
			}
			b.WriteString(text)
		}
	}
	return b.String()
}

func (s *AgentCompatGatewayService) writeError(c *gin.Context, status int, errType, message string) error {
	MarkResponseCommitted(c)
	c.JSON(status, gin.H{
		"error": gin.H{
			"message": message,
			"type":    errType,
			"param":   nil,
			"code":    nil,
		},
	})
	return errors.New(message)
}

func writeAgentCompatSSE(
	c *gin.Context,
	protocol agentCompatProtocol,
	evt apicompat.ResponsesStreamEvent,
	anthState *apicompat.ResponsesEventToAnthropicState,
	chatState *apicompat.ResponsesEventToChatState,
) error {
	if c.Writer.Header().Get("Content-Type") == "" {
		c.Header("Content-Type", "text/event-stream")
		c.Header("Cache-Control", "no-cache")
		c.Header("Connection", "keep-alive")
		c.Status(http.StatusOK)
	}
	var frames []string
	switch protocol {
	case agentCompatMessages:
		for _, event := range apicompat.ResponsesEventToAnthropicEvents(&evt, anthState) {
			line, err := apicompat.ResponsesAnthropicEventToSSE(event)
			if err != nil {
				return err
			}
			frames = append(frames, line)
		}
	case agentCompatChatCompletions:
		for _, chunk := range apicompat.ResponsesEventToChatChunks(&evt, chatState) {
			line, err := apicompat.ChatChunkToSSE(chunk)
			if err != nil {
				return err
			}
			frames = append(frames, line)
		}
	default:
		line, err := apicompat.ResponsesEventToSSE(evt)
		if err != nil {
			return err
		}
		frames = append(frames, line)
	}
	for _, frame := range frames {
		if _, err := c.Writer.WriteString(frame); err != nil {
			return err
		}
	}
	if flusher, ok := c.Writer.(http.Flusher); ok {
		flusher.Flush()
	}
	return nil
}

func writeAgentCompatJSON(c *gin.Context, protocol agentCompatProtocol, model string, events []apicompat.ResponsesStreamEvent) error {
	acc := newAgentResponsesAccumulator(model)
	for i := range events {
		acc.apply(&events[i])
	}
	resp := acc.response()
	switch protocol {
	case agentCompatMessages:
		c.JSON(http.StatusOK, apicompat.ResponsesToAnthropic(resp, model))
	case agentCompatChatCompletions:
		c.JSON(http.StatusOK, apicompat.ResponsesToChatCompletions(resp, model))
	default:
		c.JSON(http.StatusOK, resp)
	}
	return nil
}

func accountProxyURL(account *Account) string {
	if account == nil || account.Proxy == nil {
		return ""
	}
	return account.Proxy.URL()
}

func (s *AgentCompatGatewayService) streamUpstream(ctx context.Context, account *Account, req agentRequest, emit func(agentEvent) error) error {
	switch account.Platform {
	case PlatformCursor:
		return s.streamCursor(ctx, account, req, emit)
	case PlatformDevin:
		return s.streamDevin(ctx, account, req, emit)
	default:
		return fmt.Errorf("unsupported agent platform %s", account.Platform)
	}
}

func (s *AgentCompatGatewayService) streamCursor(ctx context.Context, account *Account, req agentRequest, emit func(agentEvent) error) error {
	accessToken := strings.TrimSpace(account.GetCursorAccessToken())
	if s.cursorTokens != nil && account.IsCursorOAuth() {
		token, err := s.cursorTokens.GetAccessToken(ctx, account)
		if err != nil {
			return err
		}
		accessToken = token
	}
	if accessToken == "" {
		return errors.New("cursor access token is missing")
	}
	client, err := cursor.NewHTTP2Client(accountProxyURL(account))
	if err != nil {
		return err
	}
	tools := make([]cursor.AgentTool, 0, len(req.Tools))
	for _, tool := range req.Tools {
		tools = append(tools, cursor.AgentTool{Name: tool.Name, Description: tool.Description, SchemaJSON: tool.SchemaJSON})
	}
	body := cursor.EncodeRunRequest(req.Model, req.Conversation, req.UserText, req.System, tools)
	pr, pw := io.Pipe()
	go func() {
		if _, err := pw.Write(connectrpc.Encode(body, 0)); err != nil {
			_ = pw.CloseWithError(err)
		}
	}()
	reqHTTP, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(cursor.APIBaseURL, "/")+cursor.RunPath, pr)
	if err != nil {
		_ = pw.Close()
		return err
	}
	reqHTTP.Header.Set("Content-Type", "application/connect+proto")
	reqHTTP.Header.Set("Connect-Protocol-Version", "1")
	reqHTTP.Header.Set("Authorization", "Bearer "+accessToken)
	reqHTTP.Header.Set("x-ghost-mode", "true")
	reqHTTP.Header.Set("x-cursor-client-type", "cli")
	reqHTTP.Header.Set("x-cursor-client-version", cursor.DefaultClientVersion)
	reqHTTP.Header.Set("x-request-id", fmt.Sprintf("%d", time.Now().UnixNano()))
	resp, err := client.Do(reqHTTP)
	if err != nil {
		_ = pw.Close()
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<16))
		_ = pw.Close()
		return fmt.Errorf("cursor %s: %s", resp.Status, strings.TrimSpace(string(raw)))
	}
	err = connectrpc.ReadFrames(resp.Body, func(frame connectrpc.Frame) error {
		if frame.Flags&connectrpc.FlagEndStream != 0 {
			return nil
		}
		events, controls := cursor.DecodeServerMessage(frame.Payload)
		for _, control := range controls {
			var reply []byte
			switch control.Kind {
			case "query":
				if cursor.NetworkQuery(control.QueryField) {
					reply = cursor.EncodeInteractionResponse(control.QueryID, control.QueryField, true, "")
				} else {
					reply = cursor.EncodeInteractionResponse(control.QueryID, control.QueryField, false, "interactive queries are not supported by this gateway")
				}
			case "exec":
				reply = cursor.EncodeExecReject(control.ExecID, "native tools are not executed by this gateway")
			}
			if len(reply) > 0 {
				if _, writeErr := pw.Write(connectrpc.Encode(reply, 0)); writeErr != nil {
					return writeErr
				}
			}
		}
		for _, ev := range events {
			if err := emit(cursorEvent(ev)); err != nil {
				return err
			}
		}
		return nil
	})
	_ = pw.Close()
	return err
}

func cursorEvent(ev cursor.AgentEvent) agentEvent {
	out := agentEvent{Kind: ev.Kind, Text: ev.Text, CallID: ev.CallID, ToolName: ev.ToolName, Arguments: ev.Arguments, Done: ev.Done}
	if ev.Kind == "usage" {
		out.Output = ev.Tokens
	}
	return out
}

func (s *AgentCompatGatewayService) streamDevin(ctx context.Context, account *Account, req agentRequest, emit func(agentEvent) error) error {
	sessionToken := strings.TrimSpace(account.GetDevinAccessToken())
	if sessionToken == "" {
		return errors.New("devin access token is missing")
	}
	sessionToken = devin.NormalizeSessionToken(sessionToken)
	client, err := devin.NewStreamClient(accountProxyURL(account))
	if err != nil {
		return err
	}
	userJWT := sessionToken
	if jwt, err := devin.ValidateSession(ctx, client, sessionToken); err == nil && jwt != "" {
		userJWT = jwt
	}
	modelUID, assignmentJWT, err := assignDevinModel(ctx, client, sessionToken, req.Model, req.Conversation, req.UserText)
	if err != nil {
		return err
	}
	if modelUID == "" {
		modelUID = req.Model
	}
	tools := make([][]byte, 0, len(req.Tools))
	for _, tool := range req.Tools {
		tools = append(tools, devin.EncodeTool(tool.Name, tool.Description, tool.SchemaJSON))
	}
	body := devin.EncodeChatRequest(sessionToken, userJWT, modelUID, assignmentJWT, req.Conversation, req.System, req.Messages, tools)
	resp, err := devin.PostStream(ctx, client, devin.ChatBaseURL, devin.ChatPath, body)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return connectrpc.ReadFrames(resp.Body, func(frame connectrpc.Frame) error {
		if frame.Flags&connectrpc.FlagEndStream != 0 {
			return nil
		}
		ev := devin.DecodeChatEvent(frame.Payload)
		if ev.Kind == "" && ev.Stop == 0 && ev.Input == 0 && ev.Output == 0 {
			return nil
		}
		out := agentEvent{Kind: ev.Kind, Text: ev.Text, CallID: ev.CallID, ToolName: ev.ToolName, Arguments: ev.ArgsJSON, Input: ev.Input, Output: ev.Output, CacheR: ev.CacheR, CacheW: ev.CacheW}
		if ev.Stop != 0 {
			out.Kind = "done"
			out.Done = true
		}
		return emit(out)
	})
}

func assignDevinModel(ctx context.Context, client *http.Client, sessionToken, model, cascadeID, prompt string) (string, string, error) {
	payload, err := devin.PostUnary(ctx, client, devin.ChatBaseURL, devin.AssignModelPath, devin.EncodeAssignModel(sessionToken, model, cascadeID, prompt))
	if err != nil {
		return "", "", err
	}
	modelUID, assignmentJWT := devin.DecodeAssignment(payload)
	return modelUID, assignmentJWT, nil
}
