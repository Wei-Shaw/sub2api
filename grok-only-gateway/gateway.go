package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"strings"
	"time"
)

type upstreamBufferedResult struct {
	StatusCode int
	Header     http.Header
	Body       []byte
	AccountID  int64
	RequestID  string
}

func (a *App) handleModels(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"object": "list",
		"data":   DefaultModels(),
	})
}

func (a *App) handleResponses(w http.ResponseWriter, r *http.Request) {
	body, err := a.readRequestBody(w, r)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid_request_error", err.Error())
		return
	}
	patched, err := patchResponsesBody(body)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid_request_error", err.Error())
		return
	}
	a.forwardRawWithFailover(w, r, rawForwardInput{
		Method:      http.MethodPost,
		Path:        "/responses",
		Body:        patched,
		ContentType: "application/json",
		Accept:      "application/json, text/event-stream",
		Endpoint:    "/v1/responses",
	})
}

func (a *App) handleChatCompletions(w http.ResponseWriter, r *http.Request) {
	body, err := a.readRequestBody(w, r)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid_request_error", err.Error())
		return
	}
	patched, err := patchChatCompletionsBody(body)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid_request_error", err.Error())
		return
	}
	a.forwardRawWithFailover(w, r, rawForwardInput{
		Method:      http.MethodPost,
		Path:        "/chat/completions",
		Body:        patched,
		ContentType: "application/json",
		Accept:      "application/json, text/event-stream",
		Endpoint:    "/v1/chat/completions",
	})
}

func (a *App) handleMessages(w http.ResponseWriter, r *http.Request) {
	body, err := a.readRequestBody(w, r)
	if err != nil {
		writeAnthropicError(w, http.StatusBadRequest, "invalid_request_error", err.Error())
		return
	}
	chatBody, stream, model, err := anthropicMessagesToChatCompletions(body)
	if err != nil {
		writeAnthropicError(w, http.StatusBadRequest, "invalid_request_error", err.Error())
		return
	}
	result, err := a.callBufferedWithFailover(r.Context(), bufferedCallInput{
		Method:      http.MethodPost,
		Path:        "/chat/completions",
		Body:        chatBody,
		ContentType: "application/json",
		Accept:      "application/json",
		Endpoint:    "/v1/messages",
	})
	if err != nil {
		writeAnthropicError(w, http.StatusBadGateway, "api_error", err.Error())
		return
	}
	if result.StatusCode >= 400 {
		writeAnthropicError(w, result.StatusCode, "api_error", sanitizeLogBody(result.Body))
		return
	}
	anthropicResp, err := chatCompletionToAnthropic(result.Body, model)
	if err != nil {
		writeAnthropicError(w, http.StatusBadGateway, "api_error", err.Error())
		return
	}
	copyResponseHeaders(w.Header(), result.Header)
	if stream {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.WriteHeader(http.StatusOK)
		writeAnthropicSSE(w, anthropicResp)
		return
	}
	writeJSON(w, http.StatusOK, anthropicResp)
}

func (a *App) readRequestBody(w http.ResponseWriter, r *http.Request) ([]byte, error) {
	defer r.Body.Close()
	reader := http.MaxBytesReader(w, r.Body, a.config.MaxBodyBytes)
	body, err := io.ReadAll(reader)
	if err != nil {
		return nil, err
	}
	if len(bytes.TrimSpace(body)) == 0 {
		return nil, errors.New("request body is empty")
	}
	return body, nil
}

type rawForwardInput struct {
	Method      string
	Path        string
	Body        []byte
	ContentType string
	Accept      string
	Endpoint    string
	PreferID    int64
	OnSuccess   func(accountID int64, body []byte)
}

func (a *App) forwardRawWithFailover(w http.ResponseWriter, r *http.Request, input rawForwardInput) {
	failed := map[int64]bool{}
	maxAttempts := 4
	var lastBody []byte
	var lastStatus int
	var lastHeader http.Header
	for attempt := 0; attempt < maxAttempts; attempt++ {
		account, err := a.store.SelectAccount(input.PreferID, failed)
		if err != nil {
			writeJSONError(w, http.StatusBadGateway, "api_error", err.Error())
			return
		}
		token, refreshedAccount, err := a.accessTokenForAccount(r.Context(), account)
		if refreshedAccount != nil {
			account = refreshedAccount
		}
		if err != nil {
			a.store.ReleaseAccount(account.ID)
			a.store.MarkAccountError(account.ID, err.Error())
			failed[account.ID] = true
			lastStatus = http.StatusBadGateway
			lastBody = []byte(err.Error())
			continue
		}

		target := endpointURL(account.BaseURL, input.Path)
		req, err := http.NewRequestWithContext(r.Context(), input.Method, target, bytes.NewReader(input.Body))
		if err != nil {
			a.store.ReleaseAccount(account.ID)
			writeJSONError(w, http.StatusInternalServerError, "api_error", err.Error())
			return
		}
		setUpstreamHeaders(req, r, token, input.ContentType, input.Accept)
		client, err := a.httpClient.Client(account.ProxyURL)
		if err != nil {
			a.store.ReleaseAccount(account.ID)
			a.store.MarkAccountError(account.ID, err.Error())
			failed[account.ID] = true
			continue
		}
		resp, err := client.Do(req)
		if err != nil {
			a.store.ReleaseAccount(account.ID)
			a.store.MarkAccountError(account.ID, err.Error())
			failed[account.ID] = true
			lastStatus = http.StatusBadGateway
			lastBody = []byte(err.Error())
			continue
		}

		if quota := quotaFromHeaders(resp.Header, resp.StatusCode, input.Endpoint); quota != nil {
			a.store.UpdateQuota(account.ID, quota)
		}
		if resp.StatusCode >= 400 {
			body, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
			_ = resp.Body.Close()
			a.store.ReleaseAccount(account.ID)
			lastStatus = resp.StatusCode
			lastBody = body
			lastHeader = resp.Header.Clone()
			a.markAccountFromStatus(account.ID, resp.StatusCode, resp.Header, sanitizeLogBody(body))
			if shouldFailoverStatus(resp.StatusCode) && attempt < maxAttempts-1 {
				failed[account.ID] = true
				continue
			}
			break
		}

		copyResponseHeaders(w.Header(), resp.Header)
		contentType := resp.Header.Get("Content-Type")
		if contentType == "" {
			contentType = "application/json"
		}
		w.Header().Set("Content-Type", contentType)
		w.WriteHeader(resp.StatusCode)
		if isStreamingResponse(resp.Header) {
			_, _ = io.Copy(flushWriter{ResponseWriter: w}, resp.Body)
			_ = resp.Body.Close()
			a.store.ReleaseAccount(account.ID)
			a.store.AddLog(RequestLog{Endpoint: input.Endpoint, AccountID: account.ID, StatusCode: resp.StatusCode})
			return
		}
		body, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if input.OnSuccess != nil {
			input.OnSuccess(account.ID, body)
		}
		_, _ = w.Write(body)
		a.store.ReleaseAccount(account.ID)
		a.store.AddLog(RequestLog{Endpoint: input.Endpoint, AccountID: account.ID, StatusCode: resp.StatusCode})
		return
	}

	if lastStatus == 0 {
		lastStatus = http.StatusBadGateway
	}
	if lastHeader != nil {
		copyResponseHeaders(w.Header(), lastHeader)
	}
	writeJSONError(w, lastStatus, grokErrorType(lastStatus), firstNonEmpty(extractErrorMessage(lastBody), sanitizeLogBody(lastBody), "upstream request failed"))
}

type bufferedCallInput struct {
	Method      string
	Path        string
	Body        []byte
	ContentType string
	Accept      string
	Endpoint    string
	PreferID    int64
}

func (a *App) callBufferedWithFailover(ctx context.Context, input bufferedCallInput) (*upstreamBufferedResult, error) {
	failed := map[int64]bool{}
	maxAttempts := 4
	var last *upstreamBufferedResult
	for attempt := 0; attempt < maxAttempts; attempt++ {
		account, err := a.store.SelectAccount(input.PreferID, failed)
		if err != nil {
			if last != nil {
				return last, nil
			}
			return nil, err
		}
		token, refreshedAccount, err := a.accessTokenForAccount(ctx, account)
		if refreshedAccount != nil {
			account = refreshedAccount
		}
		if err != nil {
			a.store.ReleaseAccount(account.ID)
			a.store.MarkAccountError(account.ID, err.Error())
			failed[account.ID] = true
			last = &upstreamBufferedResult{StatusCode: http.StatusBadGateway, Body: []byte(err.Error()), AccountID: account.ID}
			continue
		}
		resp, body, err := a.doUpstream(ctx, account, input.Method, endpointURL(account.BaseURL, input.Path), input.Body, input.ContentType, input.Accept, token)
		a.store.ReleaseAccount(account.ID)
		if err != nil {
			a.store.MarkAccountError(account.ID, err.Error())
			failed[account.ID] = true
			last = &upstreamBufferedResult{StatusCode: http.StatusBadGateway, Body: []byte(err.Error()), AccountID: account.ID}
			continue
		}
		result := &upstreamBufferedResult{
			StatusCode: resp.StatusCode,
			Header:     resp.Header.Clone(),
			Body:       body,
			AccountID:  account.ID,
			RequestID:  firstNonEmpty(resp.Header.Get("x-request-id"), resp.Header.Get("xai-request-id")),
		}
		if quota := quotaFromHeaders(resp.Header, resp.StatusCode, input.Endpoint); quota != nil {
			a.store.UpdateQuota(account.ID, quota)
		}
		a.store.AddLog(RequestLog{Endpoint: input.Endpoint, AccountID: account.ID, StatusCode: resp.StatusCode})
		if resp.StatusCode >= 400 {
			a.markAccountFromStatus(account.ID, resp.StatusCode, resp.Header, sanitizeLogBody(body))
			last = result
			if shouldFailoverStatus(resp.StatusCode) && attempt < maxAttempts-1 {
				failed[account.ID] = true
				continue
			}
		}
		return result, nil
	}
	if last != nil {
		return last, nil
	}
	return nil, errors.New("upstream request failed")
}

func (a *App) doUpstream(ctx context.Context, account *Account, method, target string, body []byte, contentType, accept, token string) (*http.Response, []byte, error) {
	client, err := a.httpClient.Client(account.ProxyURL)
	if err != nil {
		return nil, nil, err
	}
	req, err := http.NewRequestWithContext(ctx, method, target, bytes.NewReader(body))
	if err != nil {
		return nil, nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("User-Agent", "grok-only-gateway/1.0")
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	if accept != "" {
		req.Header.Set("Accept", accept)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 64<<20))
	if err != nil {
		return nil, nil, err
	}
	return resp, respBody, nil
}

func setUpstreamHeaders(req *http.Request, clientReq *http.Request, token, contentType, accept string) {
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("User-Agent", firstNonEmpty(clientReq.Header.Get("User-Agent"), "grok-only-gateway/1.0"))
	req.Header.Set("Accept-Language", clientReq.Header.Get("Accept-Language"))
	if beta := strings.TrimSpace(clientReq.Header.Get("OpenAI-Beta")); beta != "" {
		req.Header.Set("OpenAI-Beta", beta)
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	if accept != "" {
		req.Header.Set("Accept", accept)
	}
}

func patchResponsesBody(body []byte) ([]byte, error) {
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, errors.New("invalid json request body")
	}
	model := strings.TrimSpace(asString(payload["model"]))
	if model == "" {
		return nil, errors.New("model is required")
	}
	payload["model"] = mapModel(model)
	delete(payload, "prompt_cache_retention")
	delete(payload, "safety_identifier")
	deleteRecursive(payload, "external_web_access")
	filterGrokTools(payload)
	return json.Marshal(payload)
}

func patchChatCompletionsBody(body []byte) ([]byte, error) {
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, errors.New("invalid json request body")
	}
	model := strings.TrimSpace(asString(payload["model"]))
	if model == "" {
		return nil, errors.New("model is required")
	}
	payload["model"] = mapModel(model)
	if stream, _ := payload["stream"].(bool); stream {
		options, _ := payload["stream_options"].(map[string]any)
		if options == nil {
			options = map[string]any{}
			payload["stream_options"] = options
		}
		options["include_usage"] = true
	}
	return json.Marshal(payload)
}

func mapModel(model string) string {
	if mapped := ModelMapping()[strings.TrimSpace(model)]; mapped != "" {
		return mapped
	}
	return strings.TrimSpace(model)
}

func filterGrokTools(payload map[string]any) {
	tools, ok := payload["tools"].([]any)
	if !ok {
		return
	}
	allowed := map[string]bool{
		"code_execution": true, "code_interpreter": true, "collections_search": true,
		"file_search": true, "function": true, "mcp": true, "shell": true,
		"web_search": true, "x_search": true,
	}
	filtered := make([]any, 0, len(tools))
	for _, tool := range tools {
		m, ok := tool.(map[string]any)
		if !ok {
			continue
		}
		if allowed[asString(m["type"])] {
			filtered = append(filtered, tool)
		}
	}
	if len(filtered) == 0 {
		delete(payload, "tools")
		delete(payload, "tool_choice")
		return
	}
	payload["tools"] = filtered
}

func deleteRecursive(value any, field string) {
	switch typed := value.(type) {
	case map[string]any:
		delete(typed, field)
		for _, child := range typed {
			deleteRecursive(child, field)
		}
	case []any:
		for _, child := range typed {
			deleteRecursive(child, field)
		}
	}
}

func shouldFailoverStatus(status int) bool {
	return status == http.StatusUnauthorized ||
		status == http.StatusForbidden ||
		status == http.StatusTooManyRequests ||
		status >= 500
}

func (a *App) markAccountFromStatus(accountID int64, status int, headers http.Header, message string) {
	switch status {
	case http.StatusUnauthorized:
		a.store.MarkAccountCooldown(accountID, time.Now().UTC().Add(10*time.Minute), "grok oauth token unauthorized")
	case http.StatusForbidden:
		a.store.MarkAccountCooldown(accountID, time.Now().UTC().Add(30*time.Minute), "grok entitlement or subscription tier denied")
	case http.StatusTooManyRequests:
		cooldown := retryAfter(headers)
		if cooldown <= 0 {
			cooldown = 2 * time.Minute
		}
		a.store.MarkAccountCooldown(accountID, time.Now().UTC().Add(cooldown), "grok rate limited")
	default:
		if status >= 500 {
			a.store.MarkAccountCooldown(accountID, time.Now().UTC().Add(2*time.Minute), "grok upstream temporary error")
		} else {
			a.store.MarkAccountError(accountID, message)
		}
	}
}

func copyResponseHeaders(dst, src http.Header) {
	for key, values := range src {
		lower := strings.ToLower(key)
		if lower == "connection" || lower == "keep-alive" || lower == "proxy-authenticate" ||
			lower == "proxy-authorization" || lower == "te" || lower == "trailer" ||
			lower == "transfer-encoding" || lower == "upgrade" || lower == "content-length" {
			continue
		}
		for _, value := range values {
			dst.Add(key, value)
		}
	}
}

func isStreamingResponse(headers http.Header) bool {
	mediaType, _, err := mime.ParseMediaType(headers.Get("Content-Type"))
	if err != nil {
		return strings.Contains(strings.ToLower(headers.Get("Content-Type")), "text/event-stream")
	}
	return strings.EqualFold(mediaType, "text/event-stream")
}

type flushWriter struct {
	http.ResponseWriter
}

func (w flushWriter) Write(p []byte) (int, error) {
	n, err := w.ResponseWriter.Write(p)
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
	return n, err
}

func grokErrorType(status int) string {
	switch status {
	case http.StatusBadRequest:
		return "invalid_request_error"
	case http.StatusUnauthorized:
		return "authentication_error"
	case http.StatusForbidden:
		return "permission_error"
	case http.StatusNotFound:
		return "not_found_error"
	case http.StatusTooManyRequests:
		return "rate_limit_error"
	default:
		return "upstream_error"
	}
}

func extractErrorMessage(body []byte) string {
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return ""
	}
	if errObj, ok := payload["error"].(map[string]any); ok {
		return firstNonEmpty(asString(errObj["message"]), asString(errObj["error"]))
	}
	return firstNonEmpty(asString(payload["message"]), asString(payload["error"]))
}

func asString(v any) string {
	switch typed := v.(type) {
	case string:
		return typed
	case json.Number:
		return typed.String()
	case fmt.Stringer:
		return typed.String()
	default:
		return ""
	}
}
