package backend

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

const defaultOpenCodeBaseURL = "https://opencode.ai/zen"

const openCodeStreamKeepaliveInterval = 10 * time.Second

type OpenCodeAdapter struct{}

func NewOpenCodeAdapter() *OpenCodeAdapter { return &OpenCodeAdapter{} }

func (*OpenCodeAdapter) Platform() string { return "opencode" }

func (*OpenCodeAdapter) Descriptor() PlatformDescriptor {
	return PlatformDescriptor{Key: "opencode", DisplayName: "OpenCode", Protocol: "opencode", DefaultBaseURL: defaultOpenCodeBaseURL}
}

func (*OpenCodeAdapter) NormalizeBaseURL(baseURL string) string {
	parsed, err := url.Parse(strings.TrimRight(strings.TrimSpace(baseURL), "/"))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return baseURL
	}
	if !strings.HasSuffix(strings.ToLower(strings.TrimRight(parsed.Path, "/")), "/v1") {
		parsed.Path = strings.TrimRight(parsed.Path, "/") + "/v1"
	}
	parsed.RawPath = ""
	return parsed.String()
}

func (adapter *OpenCodeAdapter) Handle(ctx context.Context, request PlatformRequest) (PlatformResponse, error) {
	endpoint := canonicalRelayPath(request.Endpoint)
	if endpoint == "responses" {
		body, bridgeContext, err := responsesRequestToChatCompletionsWithContext(request.Body)
		if err != nil {
			return PlatformResponse{StatusCode: http.StatusBadRequest}, err
		}
		bridgeContext.codexFileTools = isCodexOpenCodeRequest(request.Headers)
		config := request.Route
		config.BaseURL = adapter.NormalizeBaseURL(config.BaseURL)
		response, err := forwardOpenCodeResponsesRequest(ctx, request, config, body, bridgeContext)
		logOpenCodeResponsesBridge("response", body, response.StatusCode, response.Body)
		if err != nil || response.StatusCode < 200 || response.StatusCode >= 300 {
			return response, err
		}
		if response.Stream != nil {
			return response, nil
		}
		response.Body, err = chatCompletionToResponsesWithContext(response.Body, bridgeContext)
		if err != nil {
			return PlatformResponse{StatusCode: http.StatusBadGateway}, err
		}
		response.Headers.Set("Content-Type", "application/json")
		return response, nil
	}
	body := request.Body
	if endpoint == "chat/completions" {
		body = adapter.TransformRequestBody(endpoint, body)
	}
	config := request.Route
	config.BaseURL = adapter.NormalizeBaseURL(config.BaseURL)
	return forwardPlatformRequest(ctx, request, config, endpoint, request.Method, body)
}

func forwardOpenCodeResponsesRequest(ctx context.Context, request PlatformRequest, config RouteConfig, body []byte, bridgeContext responsesBridgeContext) (PlatformResponse, error) {
	upstreamURL, err := ResolveRouteEndpointURL(config, "chat/completions")
	if err != nil {
		return PlatformResponse{StatusCode: http.StatusBadGateway}, err
	}
	parsedURL, err := url.Parse(upstreamURL)
	if err != nil {
		return PlatformResponse{StatusCode: http.StatusBadGateway}, err
	}
	parsedURL.RawQuery = request.Query
	upstreamRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, parsedURL.String(), bytes.NewReader(body))
	if err != nil {
		return PlatformResponse{StatusCode: http.StatusBadGateway}, err
	}
	copyEndToEndHeaders(upstreamRequest.Header, request.Headers)
	response, err := request.Client.Do(upstreamRequest)
	if err != nil {
		return PlatformResponse{StatusCode: http.StatusBadGateway}, err
	}
	if response.StatusCode >= 200 && response.StatusCode < 300 && strings.Contains(strings.ToLower(response.Header.Get("Content-Type")), "text/event-stream") {
		headers := response.Header.Clone()
		headers.Set("Content-Type", "text/event-stream")
		return PlatformResponse{
			StatusCode: response.StatusCode,
			Headers:    headers,
			Stream: func(writer http.ResponseWriter) error {
				return streamOpenCodeResponses(ctx, response.Body, writer, bridgeContext)
			},
		}, nil
	}
	defer response.Body.Close()
	responseBody, err := io.ReadAll(response.Body)
	if err != nil {
		return PlatformResponse{StatusCode: http.StatusBadGateway}, err
	}
	return PlatformResponse{StatusCode: response.StatusCode, Headers: response.Header.Clone(), Body: responseBody}, nil
}

type openCodeStreamWriter struct {
	writer  io.Writer
	flusher http.Flusher
	mu      sync.Mutex
}

func newOpenCodeStreamWriter(writer io.Writer) *openCodeStreamWriter {
	flusher, _ := writer.(http.Flusher)
	return &openCodeStreamWriter{writer: writer, flusher: flusher}
}

func (w *openCodeStreamWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.writer.Write(p)
}

func (w *openCodeStreamWriter) Flush() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.flusher != nil {
		w.flusher.Flush()
	}
}

func (w *openCodeStreamWriter) writeKeepalive() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	const keepalive = ": keepalive\n\n"
	n, err := w.writer.Write([]byte(keepalive))
	if err != nil {
		return err
	}
	if n != len(keepalive) {
		return io.ErrShortWrite
	}
	if w.flusher != nil {
		w.flusher.Flush()
	}
	return nil
}

func streamOpenCodeResponses(ctx context.Context, body io.ReadCloser, writer io.Writer, bridgeContext responsesBridgeContext) error {
	return streamOpenCodeResponsesWithInterval(ctx, body, writer, bridgeContext, openCodeStreamKeepaliveInterval)
}

func streamOpenCodeResponsesWithInterval(ctx context.Context, body io.ReadCloser, writer io.Writer, bridgeContext responsesBridgeContext, keepaliveInterval time.Duration) error {
	streamWriter := newOpenCodeStreamWriter(writer)
	done := make(chan struct{})
	defer close(done)
	defer body.Close()
	heartbeatErr := make(chan error, 1)
	go func() {
		ticker := time.NewTicker(keepaliveInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				_ = body.Close()
				return
			case <-done:
				return
			case <-ticker.C:
				if err := streamWriter.writeKeepalive(); err != nil {
					heartbeatErr <- err
					_ = body.Close()
					return
				}
			}
		}
	}()
	err := chatCompletionSSEReaderToResponsesWithContext(body, streamWriter, bridgeContext)
	if ctx.Err() != nil {
		return ctx.Err()
	}
	select {
	case err := <-heartbeatErr:
		return err
	default:
	}
	return err
}

func isCodexOpenCodeRequest(headers http.Header) bool {
	if headers == nil {
		return false
	}
	originator := strings.ToLower(strings.TrimSpace(headers.Get("originator")))
	if strings.HasPrefix(originator, "codex_") || strings.HasPrefix(originator, "codex-") || strings.HasPrefix(originator, "codex ") {
		return true
	}
	userAgent := strings.ToLower(strings.TrimSpace(headers.Get("User-Agent")))
	return strings.HasPrefix(userAgent, "codex_") || strings.HasPrefix(userAgent, "codex-") || strings.HasPrefix(userAgent, "codex/") || strings.HasPrefix(userAgent, "codex ")
}

func (adapter *OpenCodeAdapter) TransformRequestBody(endpoint string, body []byte) []byte {
	if canonicalRelayPath(endpoint) != "chat/completions" {
		return body
	}
	var payload map[string]any
	if json.Unmarshal(body, &payload) != nil || payload == nil {
		return body
	}
	if _, hasMessages := payload["messages"].([]any); hasMessages {
		return body
	}
	messages := make([]map[string]string, 0, 2)
	if system, ok := payload["system"].(string); ok && strings.TrimSpace(system) != "" {
		messages = append(messages, map[string]string{"role": "system", "content": strings.TrimSpace(system)})
	}
	partsText := opencodePartsText(payload["parts"])
	if partsText != "" {
		messages = append(messages, map[string]string{"role": "user", "content": partsText})
	} else if message, ok := payload["message"].(string); ok && strings.TrimSpace(message) != "" {
		messages = append(messages, map[string]string{"role": "user", "content": strings.TrimSpace(message)})
	}
	if len(messages) == 0 {
		return body
	}
	converted := map[string]any{"messages": messages}
	switch model := payload["model"].(type) {
	case string:
		if strings.TrimSpace(model) != "" {
			converted["model"] = strings.TrimSpace(model)
		}
	case map[string]any:
		for _, key := range []string{"modelID", "model_id", "id"} {
			if value, ok := model[key].(string); ok && strings.TrimSpace(value) != "" {
				converted["model"] = strings.TrimSpace(value)
				break
			}
		}
	}
	for _, key := range []string{"stream", "temperature", "top_p", "max_tokens", "stop"} {
		if value, ok := payload[key]; ok {
			converted[key] = value
		}
	}
	result, err := json.Marshal(converted)
	if err != nil {
		return body
	}
	return result
}

func opencodePartsText(value any) string {
	parts, ok := value.([]any)
	if !ok {
		return ""
	}
	texts := make([]string, 0, len(parts))
	for _, part := range parts {
		switch item := part.(type) {
		case string:
			if strings.TrimSpace(item) != "" {
				texts = append(texts, strings.TrimSpace(item))
			}
		case map[string]any:
			for _, key := range []string{"text", "content"} {
				if text, ok := item[key].(string); ok && strings.TrimSpace(text) != "" {
					texts = append(texts, strings.TrimSpace(text))
					break
				}
			}
		}
	}
	return strings.Join(texts, "\n")
}

func logOpenCodeResponsesBridge(stage string, chatBody []byte, statusCode int, responseBody []byte) {
	if statusCode < http.StatusBadRequest {
		return
	}
	var payload map[string]any
	if json.Unmarshal(chatBody, &payload) != nil {
		return
	}
	roles := make([]string, 0)
	messageSummary := make([]string, 0)
	assistantToolSummary := make([]string, 0)
	toolContentLengths := make([]int, 0)
	if messages, ok := payload["messages"].([]any); ok {
		for _, raw := range messages {
			if message, ok := raw.(map[string]any); ok {
				if role, ok := message["role"].(string); ok {
					roles = append(roles, role)
					id := stringValue(message["id"])
					toolCallID := stringValue(message["tool_call_id"])
					if id == "" {
						id = "-"
					}
					if toolCallID != "" {
						messageSummary = append(messageSummary, role+":"+id+":tool="+toolCallID)
						if content, ok := message["content"].(string); ok {
							toolContentLengths = append(toolContentLengths, len(content))
						}
					} else {
						messageSummary = append(messageSummary, role+":"+id)
					}
					if role == "assistant" {
						if calls, ok := message["tool_calls"].([]any); ok {
							for _, rawCall := range calls {
								if call, ok := rawCall.(map[string]any); ok {
									function, _ := call["function"].(map[string]any)
									arguments := stringValue(function["arguments"])
									validJSON := false
									var decoded any
									validJSON = json.Unmarshal([]byte(arguments), &decoded) == nil
									assistantToolSummary = append(assistantToolSummary, stringValue(call["id"])+":"+stringValue(function["name"])+":args="+fmt.Sprint(len(arguments))+":json="+fmt.Sprint(validJSON))
								}
							}
						}
					}
				}
			}
		}
	}
	toolCount := 0
	toolNames := make([]string, 0)
	if tools, ok := payload["tools"].([]any); ok {
		toolCount = len(tools)
		for _, raw := range tools {
			if tool, ok := raw.(map[string]any); ok {
				if function, ok := tool["function"].(map[string]any); ok {
					if name := stringValue(function["name"]); name != "" {
						toolNames = append(toolNames, name)
					}
				}
			}
		}
	}
	responsePreview := ""
	if statusCode >= http.StatusBadRequest {
		responsePreview = string(responseBody)
		if len(responsePreview) > 1000 {
			responsePreview = responsePreview[:1000]
		}
	}
	log.Printf("opencode_responses_bridge stage=%s status=%d model=%q stream=%v roles=%q messages=%q assistant_tools=%q tool_content_lengths=%v tools=%d tool_names=%q tool_choice_type=%T max_tokens=%v response=%q", stage, statusCode, payload["model"], payload["stream"], roles, messageSummary, assistantToolSummary, toolContentLengths, toolCount, toolNames, payload["tool_choice"], payload["max_tokens"], responsePreview)
}
