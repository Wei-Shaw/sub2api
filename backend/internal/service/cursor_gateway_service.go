package service

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
	"github.com/Wei-Shaw/sub2api/internal/pkg/cursor"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/gin-gonic/gin"
)

// CursorGatewayService translates OpenAI Chat Completions requests into
// Cursor's Connect-RPC protobuf format, streams the response, and converts
// it back into OpenAI SSE format.
type CursorGatewayService struct {
	accountRepo     AccountRepository
	refreshAPI      *OAuthRefreshAPI
	refresher       *CursorTokenRefresher
	streamChat      func(ctx context.Context, creds cursor.Credentials, messages []cursor.ChatMessage, model string, thinkingLevel int) (*http.Response, error)
	availableModels func(ctx context.Context, creds cursor.Credentials) ([]cursor.AvailableModel, error)

	catalogMu        sync.Mutex
	catalogByAccount map[int64]cursorCatalogEntry
}

type cursorCatalogEntry struct {
	models []cursor.AvailableModel
	expiry time.Time
}

const cursorCatalogTTL = 5 * time.Minute

// NewCursorGatewayService creates a new CursorGatewayService.
func NewCursorGatewayService(accountRepo AccountRepository, refreshAPI *OAuthRefreshAPI) *CursorGatewayService {
	return &CursorGatewayService{
		accountRepo: accountRepo,
		refreshAPI:  refreshAPI,
		refresher:   NewCursorTokenRefresher(),
	}
}

// ForwardAsChatCompletions receives an OpenAI Chat Completions request body,
// translates it to Cursor's format, streams the response, and writes it back
// to the client in OpenAI SSE format.
func (s *CursorGatewayService) ForwardAsChatCompletions(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	body []byte,
) (*ForwardResult, error) {
	startTime := time.Now()

	var ccReq struct {
		Model           string          `json:"model"`
		Messages        json.RawMessage `json:"messages"`
		Stream          bool            `json:"stream"`
		ReasoningEffort *string         `json:"reasoning_effort"`
		Reasoning       *struct {
			Effort *string `json:"effort"`
		} `json:"reasoning"`
		Fast          *bool `json:"fast"`
		Thinking      *bool `json:"thinking"`
		StreamOptions *struct {
			IncludeUsage bool `json:"include_usage"`
		} `json:"stream_options,omitempty"`
	}
	if err := json.Unmarshal(body, &ccReq); err != nil {
		return nil, s.writeChatCompletionsError(c, http.StatusBadRequest, "invalid_request_error", "Failed to parse request body")
	}
	if strings.TrimSpace(ccReq.Model) == "" {
		return nil, s.writeChatCompletionsError(c, http.StatusBadRequest, "invalid_request_error", "model is required")
	}

	// Parse messages into cursor format
	var openAIMessages []struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	}
	if err := json.Unmarshal(ccReq.Messages, &openAIMessages); err != nil {
		return nil, s.writeChatCompletionsError(c, http.StatusBadRequest, "invalid_request_error", "Failed to parse messages")
	}

	cursorMessages := make([]cursor.ChatMessage, len(openAIMessages))
	for i, m := range openAIMessages {
		cursorMessages[i] = cursor.ChatMessage{Role: m.Role, Content: m.Content}
	}

	mappedModel := account.GetMappedModel(ccReq.Model)
	opts := cursorRunOpts(ccReq.ReasoningEffort, ccReq.Reasoning, ccReq.Fast, ccReq.Thinking)
	resp, _, warnings, err := s.startCursorChat(ctx, c, account, cursorMessages, mappedModel, opts)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	includeUsage := ccReq.StreamOptions != nil && ccReq.StreamOptions.IncludeUsage
	if ccReq.Stream {
		return s.streamResponse(c, resp.Body, ccReq.Model, warnings, startTime, includeUsage)
	}
	return s.nonStreamResponse(c, resp.Body, ccReq.Model, warnings, startTime)
}

func resolveCursorChatModel(requested string, opts cursor.RunOpts) (upstream string, warnings []map[string]string) {
	return resolveCursorRunModel(requested, opts, nil)
}

func resolveCursorRunModel(requested string, opts cursor.RunOpts, catalog []cursor.AvailableModel) (upstream string, warnings []map[string]string) {
	resolved := cursor.ResolveRunModel(requested, opts, catalog)
	if resolved.RunSlug == "" {
		return requested, nil
	}
	if resolved.AliasFallback {
		warnings = append(warnings, map[string]string{
			"code":    "model_fallback",
			"message": fmt.Sprintf("Requested model %q is not a Cursor picker slug; using fallback %q", requested, resolved.PickerID),
		})
	}
	if resolved.VariantApplied {
		warnings = append(warnings, map[string]string{
			"code":    "model_variant",
			"message": fmt.Sprintf("Requested model %q is a Cursor picker family; using AgentService slug %q", requested, resolved.RunSlug),
		})
	}
	return resolved.RunSlug, warnings
}

func (s *CursorGatewayService) startCursorChat(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	messages []cursor.ChatMessage,
	requestedModel string,
	opts cursor.RunOpts,
) (*http.Response, string, []map[string]string, error) {
	if err := s.ensureCursorAccessToken(ctx, account); err != nil {
		logger.LegacyPrintf("service.cursor", "[Cursor] token refresh account=%d: %v", accountID(account), err)
	}

	upstreamModel, warnings := resolveCursorRunModel(requestedModel, opts, s.liveRunCatalog(ctx, account))
	if requestedModel != "" && !strings.EqualFold(requestedModel, upstreamModel) {
		c.Header("X-Sub2API-Model-Variant", requestedModel+" -> "+upstreamModel)
	}
	if len(warnings) > 0 {
		c.Header("X-Sub2API-Model-Fallback", requestedModel+" -> "+upstreamModel)
		logger.LegacyPrintf("service.cursor", "[Cursor] model resolve requested=%s upstream=%s", requestedModel, upstreamModel)
	}

	thinkingLevel := cursor.ThinkingLevelUnspecified
	if strings.Contains(strings.ToLower(upstreamModel), "thinking") || strings.Contains(strings.ToLower(upstreamModel), "think") {
		thinkingLevel = cursor.ThinkingLevelHigh
	}

	resp, err := s.doStreamChat(ctx, account, messages, upstreamModel, thinkingLevel)
	if err != nil && isCursorAuthError(err) {
		if refreshErr := s.refreshCursorAccount(ctx, account, true); refreshErr != nil {
			logger.LegacyPrintf("service.cursor", "[Cursor] auth retry refresh account=%d: %v", accountID(account), refreshErr)
		} else {
			resp, err = s.doStreamChat(ctx, account, messages, upstreamModel, thinkingLevel)
		}
	}
	if err != nil {
		return nil, "", warnings, &UpstreamFailoverError{
			StatusCode:   http.StatusBadGateway,
			ResponseBody: []byte(fmt.Sprintf("Cursor upstream error: %v", err)),
		}
	}
	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, "", warnings, &UpstreamFailoverError{
			StatusCode:   resp.StatusCode,
			ResponseBody: respBody,
		}
	}
	return resp, upstreamModel, warnings, nil
}

func cursorRunOptsFromAnthropic(req *apicompat.AnthropicRequest) cursor.RunOpts {
	var opts cursor.RunOpts
	if req == nil {
		return opts
	}
	if req.OutputConfig != nil {
		opts.Effort = cursor.NormalizeEffort(req.OutputConfig.Effort)
	}
	if req.Thinking != nil {
		enabled := strings.EqualFold(req.Thinking.Type, "enabled") || strings.EqualFold(req.Thinking.Type, "adaptive")
		opts.Thinking = &enabled
	}
	return opts
}

func cursorRunOptsFromResponses(req *apicompat.ResponsesRequest) cursor.RunOpts {
	var opts cursor.RunOpts
	if req == nil || req.Reasoning == nil {
		return opts
	}
	opts.Effort = cursor.NormalizeEffort(req.Reasoning.Effort)
	return opts
}

func cursorRunOpts(flatEffort *string, reasoning *struct {
	Effort *string `json:"effort"`
}, fast, thinking *bool) cursor.RunOpts {
	opts := cursor.RunOpts{Thinking: thinking}
	if fast != nil {
		opts.Fast = *fast
	}
	effort := ""
	if reasoning != nil && reasoning.Effort != nil {
		effort = *reasoning.Effort
	}
	if flatEffort != nil && strings.TrimSpace(*flatEffort) != "" {
		effort = *flatEffort
	}
	opts.Effort = cursor.NormalizeEffort(effort)
	return opts
}

func (s *CursorGatewayService) buildCredentials(account *Account) cursor.Credentials {
	return cursorCredentialsFromAccount(account)
}

func (s *CursorGatewayService) doStreamChat(
	ctx context.Context,
	account *Account,
	messages []cursor.ChatMessage,
	model string,
	thinkingLevel int,
) (*http.Response, error) {
	creds := s.buildCredentials(account)
	if s != nil && s.streamChat != nil {
		return s.streamChat(ctx, creds, messages, model, thinkingLevel)
	}
	return cursor.NewClient(creds).StreamChat(ctx, messages, model, thinkingLevel)
}

func accountID(account *Account) int64 {
	if account == nil {
		return 0
	}
	return account.ID
}

func cursorCredentialsFromAccount(account *Account) cursor.Credentials {
	if account == nil {
		return cursor.Credentials{}
	}
	return cursor.Credentials{
		AccessToken:   normalizeCursorAccessToken(account.GetCredential("access_token")),
		MachineID:     account.GetCredential("machine_id"),
		MacMachineID:  account.GetCredential("mac_machine_id"),
		ClientVersion: account.GetCredential("client_version"),
		ClientCommit:  account.GetCredential("client_commit"),
		GhostMode:     account.GetCredential("ghost_mode") == "true",
	}
}

func normalizeCursorAccessToken(token string) string {
	token = strings.TrimSpace(token)
	if i := strings.Index(token, "::"); i >= 0 {
		token = strings.TrimSpace(token[i+2:])
	}
	return token
}

func fetchCursorAvailableModels(ctx context.Context, creds cursor.Credentials) ([]cursor.AvailableModel, error) {
	if strings.TrimSpace(creds.AccessToken) == "" {
		return nil, fmt.Errorf("cursor: missing access_token")
	}
	return cursor.NewClient(creds).AvailableModels(ctx)
}

func (s *CursorGatewayService) liveRunCatalog(ctx context.Context, account *Account) []cursor.AvailableModel {
	if s == nil || account == nil {
		return nil
	}
	if models := s.cachedCatalog(account.ID); models != nil {
		return models
	}
	models, err := s.fetchRunCatalog(ctx, account)
	if err != nil && isCursorAuthError(err) {
		if refreshErr := s.refreshCursorAccount(ctx, account, true); refreshErr == nil {
			models, err = s.fetchRunCatalog(ctx, account)
		}
	}
	if err != nil {
		logger.LegacyPrintf("service.cursor", "[Cursor] AvailableModels account=%d: %v", account.ID, err)
		return nil
	}
	s.storeCatalog(account.ID, models)
	return models
}

func (s *CursorGatewayService) fetchRunCatalog(ctx context.Context, account *Account) ([]cursor.AvailableModel, error) {
	creds := cursorCredentialsFromAccount(account)
	if s.availableModels != nil {
		return s.availableModels(ctx, creds)
	}
	return fetchCursorAvailableModels(ctx, creds)
}

func (s *CursorGatewayService) cachedCatalog(accountID int64) []cursor.AvailableModel {
	s.catalogMu.Lock()
	defer s.catalogMu.Unlock()
	entry, ok := s.catalogByAccount[accountID]
	if !ok || time.Now().After(entry.expiry) {
		return nil
	}
	return entry.models
}

func (s *CursorGatewayService) storeCatalog(accountID int64, models []cursor.AvailableModel) {
	s.catalogMu.Lock()
	defer s.catalogMu.Unlock()
	if s.catalogByAccount == nil {
		s.catalogByAccount = make(map[int64]cursorCatalogEntry)
	}
	s.catalogByAccount[accountID] = cursorCatalogEntry{
		models: models,
		expiry: time.Now().Add(cursorCatalogTTL),
	}
}

// FetchCursorPickerModels loads the live Cursor picker catalog for an account.
func FetchCursorPickerModels(ctx context.Context, account *Account) ([]cursor.AvailableModel, error) {
	return fetchCursorAvailableModels(ctx, cursorCredentialsFromAccount(account))
}

func (s *GatewayService) cursorPickerModelIDs(ctx context.Context, accounts []Account) []string {
	if s == nil {
		return nil
	}
	return cursorPickerIDsFromAccounts(ctx, accounts, s.cursorAvailableModels)
}

func cursorPickerIDsFromAccounts(
	ctx context.Context,
	accounts []Account,
	fetch func(context.Context, cursor.Credentials) ([]cursor.AvailableModel, error),
) []string {
	if fetch == nil {
		fetch = fetchCursorAvailableModels
	}
	for i := range accounts {
		acc := &accounts[i]
		if acc.Platform != PlatformCursor {
			continue
		}
		creds := cursorCredentialsFromAccount(acc)
		if creds.AccessToken == "" {
			continue
		}
		models, err := fetch(ctx, creds)
		if err != nil {
			logger.LegacyPrintf("service.cursor", "[Cursor] AvailableModels account=%d: %v", acc.ID, err)
			continue
		}
		ids := cursor.ModelIDs(models)
		if len(ids) > 0 {
			return ids
		}
	}
	return nil
}

func (s *CursorGatewayService) streamResponse(
	c *gin.Context,
	body io.Reader,
	model string,
	warnings []map[string]string,
	startTime time.Time,
	includeUsage bool,
) (*ForwardResult, error) {
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Writer.Flush()

	if len(warnings) > 0 {
		fmt.Fprintf(c.Writer, ": sub2api-model-fallback %s\n\n", warnings[0]["message"])
		c.Writer.Flush()
	}

	var firstTokenMs *int
	var emitted bool
	completionID := "chatcmpl-cursor-" + time.Now().Format("20060102150405")

	usage, connectErr := cursor.ConsumeAssistantStream(body, func(ev cursor.StreamEvent) error {
		switch ev.Type {
		case "text", "thinking":
		default:
			return nil
		}
		if firstTokenMs == nil {
			ms := int(time.Since(startTime).Milliseconds())
			firstTokenMs = &ms
		}
		content, reasoning := "", ""
		if ev.Type == "thinking" {
			reasoning = ev.Text
		} else {
			content = ev.Text
		}
		emitted = true
		fmt.Fprintf(c.Writer, "data: %s\n\n", buildCursorSSEChunk(completionID, model, content, reasoning, ""))
		c.Writer.Flush()
		return nil
	})

	if connectErr != "" && !emitted {
		_, errType, message := classifyCursorConnectError(connectErr)
		errChunk, _ := json.Marshal(map[string]any{
			"error": map[string]string{
				"type":    errType,
				"message": message,
			},
		})
		fmt.Fprintf(c.Writer, "data: %s\n\n", errChunk)
		c.Writer.Flush()
	} else {
		fmt.Fprintf(c.Writer, "data: %s\n\n", buildCursorSSEChunk(completionID, model, "", "", "stop"))
		c.Writer.Flush()
		if includeUsage {
			if chatUsage := chatUsageFromCursor(usage); chatUsage != nil {
				fmt.Fprintf(c.Writer, "data: %s\n\n", buildCursorUsageChunk(completionID, model, chatUsage))
				c.Writer.Flush()
			}
		}
	}

	fmt.Fprintf(c.Writer, "data: [DONE]\n\n")
	c.Writer.Flush()

	return cursorForwardResult(model, true, startTime, firstTokenMs, usage), nil
}

func (s *CursorGatewayService) nonStreamResponse(
	c *gin.Context,
	body io.Reader,
	model string,
	warnings []map[string]string,
	startTime time.Time,
) (*ForwardResult, error) {
	var totalText, thinking strings.Builder
	usage, connectErr := cursor.ConsumeAssistantStream(body, func(ev cursor.StreamEvent) error {
		switch ev.Type {
		case "text":
			totalText.WriteString(ev.Text)
		case "thinking":
			thinking.WriteString(ev.Text)
		}
		return nil
	})

	if connectErr != "" && totalText.Len() == 0 && thinking.Len() == 0 {
		status, errType, message := classifyCursorConnectError(connectErr)
		return nil, s.writeChatCompletionsError(c, status, errType, message)
	}

	message := map[string]any{
		"role":    "assistant",
		"content": totalText.String(),
	}
	if thinking.Len() > 0 {
		message["reasoning_content"] = thinking.String()
	}

	completionID := "chatcmpl-cursor-" + time.Now().Format("20060102150405")
	response := map[string]any{
		"id":      completionID,
		"object":  "chat.completion",
		"created": time.Now().Unix(),
		"model":   model,
		"choices": []map[string]any{
			{
				"index":         0,
				"message":       message,
				"finish_reason": "stop",
			},
		},
	}
	if chatUsage := chatUsageFromCursor(usage); chatUsage != nil {
		response["usage"] = chatUsage
	}
	if len(warnings) > 0 {
		response["warnings"] = warnings
	}

	c.JSON(http.StatusOK, response)

	return cursorForwardResult(model, false, startTime, nil, usage), nil
}

func cursorForwardResult(model string, stream bool, startTime time.Time, firstTokenMs *int, usage cursor.TokenUsage) *ForwardResult {
	return &ForwardResult{
		Model:        model,
		Stream:       stream,
		Duration:     time.Since(startTime),
		FirstTokenMs: firstTokenMs,
		Usage:        claudeUsageFromCursor(usage),
	}
}

func claudeUsageFromCursor(u cursor.TokenUsage) ClaudeUsage {
	out := u.OutputTokens
	if out == 0 && u.ReasoningTokens > 0 {
		out = u.ReasoningTokens
	}
	return ClaudeUsage{
		InputTokens:              u.InputTokens,
		OutputTokens:             out,
		CacheCreationInputTokens: u.CacheWriteTokens,
		CacheReadInputTokens:     u.CacheReadTokens,
	}
}

func chatUsageFromCursor(u cursor.TokenUsage) *apicompat.ChatUsage {
	if u.Empty() {
		return nil
	}
	prompt := u.InputTokens + u.CacheReadTokens + u.CacheWriteTokens
	completion := u.OutputTokens
	if completion == 0 && u.ReasoningTokens > 0 {
		completion = u.ReasoningTokens
	}
	usage := &apicompat.ChatUsage{
		PromptTokens:     prompt,
		CompletionTokens: completion,
		TotalTokens:      prompt + completion,
	}
	if u.CacheReadTokens > 0 || u.CacheWriteTokens > 0 {
		usage.PromptTokensDetails = &apicompat.ChatTokenDetails{
			CachedTokens:     u.CacheReadTokens,
			CacheWriteTokens: u.CacheWriteTokens,
		}
	}
	if u.ReasoningTokens > 0 {
		usage.CompletionTokensDetails = &apicompat.ChatTokenDetails{
			ReasoningTokens: u.ReasoningTokens,
		}
	}
	return usage
}

func buildCursorSSEChunk(id, model, content, reasoning, finishReason string) string {
	delta := map[string]any{}
	if content != "" {
		delta["content"] = content
	}
	if reasoning != "" {
		delta["reasoning_content"] = reasoning
	}
	choice := map[string]any{
		"index": 0,
		"delta": delta,
	}
	if finishReason != "" {
		choice["finish_reason"] = finishReason
	}
	chunk := map[string]any{
		"id":      id,
		"object":  "chat.completion.chunk",
		"created": time.Now().Unix(),
		"model":   model,
		"choices": []map[string]any{choice},
	}
	data, _ := json.Marshal(chunk)
	return string(data)
}

func buildCursorUsageChunk(id, model string, usage *apicompat.ChatUsage) string {
	chunk := map[string]any{
		"id":      id,
		"object":  "chat.completion.chunk",
		"created": time.Now().Unix(),
		"model":   model,
		"choices": []any{},
		"usage":   usage,
	}
	data, _ := json.Marshal(chunk)
	return string(data)
}

func (s *CursorGatewayService) writeChatCompletionsError(c *gin.Context, status int, errType, message string) error {
	c.JSON(status, gin.H{
		"error": gin.H{
			"type":    errType,
			"message": message,
		},
	})
	return fmt.Errorf("cursor: %s: %s", errType, message)
}

func classifyCursorConnectError(raw string) (status int, errType, message string) {
	parsed, ok := cursor.ParseConnectError(raw)
	if !ok {
		return http.StatusBadGateway, "upstream_error", "Cursor upstream error"
	}
	message = strings.TrimSpace(parsed.Message)
	if message == "" {
		message = strings.TrimSpace(raw)
	}
	if parsed.IsBadModelName() {
		return http.StatusBadRequest, "invalid_request_error", message
	}
	return http.StatusBadGateway, "upstream_error", message
}
