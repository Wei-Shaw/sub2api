package handler

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

const (
	playgroundDefaultModel     = "gpt-5.4"
	playgroundDefaultPrompt    = "请回复 OK"
	playgroundMaxPromptRunes   = 4000
	playgroundDefaultMaxTokens = 128
	playgroundMaxTokens        = 1024
	playgroundBillingPolls     = 10
	playgroundBillingPollDelay = 200 * time.Millisecond
)

type PlaygroundHandler struct {
	apiKeyService       *service.APIKeyService
	userService         *service.UserService
	subscriptionService *service.SubscriptionService
	gateway             *GatewayHandler
	openaiGateway       *OpenAIGatewayHandler
}

func NewPlaygroundHandler(
	apiKeyService *service.APIKeyService,
	userService *service.UserService,
	subscriptionService *service.SubscriptionService,
	gateway *GatewayHandler,
	openaiGateway *OpenAIGatewayHandler,
) *PlaygroundHandler {
	return &PlaygroundHandler{
		apiKeyService:       apiKeyService,
		userService:         userService,
		subscriptionService: subscriptionService,
		gateway:             gateway,
		openaiGateway:       openaiGateway,
	}
}

type PlaygroundChatRequest struct {
	APIKeyID  *int64 `json:"api_key_id"`
	Model     string `json:"model"`
	Prompt    string `json:"prompt"`
	MaxTokens int    `json:"max_tokens"`
}

type PlaygroundChatResponse struct {
	Success        bool             `json:"success"`
	APIKeyID       int64            `json:"api_key_id"`
	APIKeyName     string           `json:"api_key_name"`
	Model          string           `json:"model"`
	ResolvedModel  string           `json:"resolved_model,omitempty"`
	Endpoint       string           `json:"endpoint"`
	DurationMS     int64            `json:"duration_ms"`
	Text           string           `json:"text,omitempty"`
	Usage          map[string]any   `json:"usage,omitempty"`
	BalanceBefore  float64          `json:"balance_before"`
	BalanceAfter   float64          `json:"balance_after"`
	Cost           float64          `json:"cost"`
	BillingSettled bool             `json:"billing_settled"`
	RawStatus      int              `json:"raw_status"`
	Error          *PlaygroundError `json:"error,omitempty"`
}

type PlaygroundModelsResponse struct {
	APIKeyID     int64    `json:"api_key_id"`
	APIKeyName   string   `json:"api_key_name"`
	GroupName    string   `json:"group_name,omitempty"`
	Platform     string   `json:"platform,omitempty"`
	Models       []string `json:"models"`
	DefaultModel string   `json:"default_model,omitempty"`
	Source       string   `json:"source"`
}

type PlaygroundError struct {
	Code       string `json:"code"`
	Message    string `json:"message"`
	Suggestion string `json:"suggestion,omitempty"`
}

// Chat tests a text model through the real /v1/chat/completions gateway path.
func (h *PlaygroundHandler) Chat(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}

	var req PlaygroundChatRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	req.normalize()
	if err := req.validate(); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	apiKey, err := h.selectAPIKey(c, subject.UserID, req.APIKeyID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	userBefore, err := h.userService.GetByID(c.Request.Context(), subject.UserID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	body, err := json.Marshal(map[string]any{
		"model": req.Model,
		"messages": []map[string]string{
			{"role": "user", "content": req.Prompt},
		},
		"max_tokens": req.MaxTokens,
		"stream":     false,
	})
	if err != nil {
		response.InternalError(c, "Failed to build playground request")
		return
	}

	start := time.Now()
	recorder := httptest.NewRecorder()
	internalCtx, _ := gin.CreateTestContext(recorder)
	internalCtx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body)).WithContext(c.Request.Context())
	internalCtx.Request.Header.Set("Content-Type", "application/json")
	internalCtx.Request.Header.Set("User-Agent", c.GetHeader("User-Agent"))
	internalCtx.Request.RemoteAddr = c.Request.RemoteAddr
	internalCtx.Set(string(middleware2.ContextKeyAPIKey), apiKey)
	internalCtx.Set(string(middleware2.ContextKeyUser), middleware2.AuthSubject{
		UserID:      apiKey.User.ID,
		Concurrency: apiKey.User.Concurrency,
	})
	internalCtx.Set(string(middleware2.ContextKeyUserRole), apiKey.User.Role)
	if h.subscriptionService != nil && apiKey.Group != nil && apiKey.Group.IsSubscriptionType() {
		if sub, subErr := h.subscriptionService.GetActiveSubscription(c.Request.Context(), apiKey.User.ID, apiKey.Group.ID); subErr == nil {
			internalCtx.Set(string(middleware2.ContextKeySubscription), sub)
		}
	}

	h.forwardChat(apiKey, internalCtx)

	duration := time.Since(start)
	raw := recorder.Body.Bytes()
	result := PlaygroundChatResponse{
		Success:       recorder.Code >= 200 && recorder.Code < 300,
		APIKeyID:      apiKey.ID,
		APIKeyName:    apiKey.Name,
		Model:         req.Model,
		Endpoint:      "/v1/chat/completions",
		DurationMS:    duration.Milliseconds(),
		BalanceBefore: userBefore.Balance,
		BalanceAfter:  userBefore.Balance,
		RawStatus:     recorder.Code,
	}

	h.applyGatewayResponse(raw, &result)
	userAfter := h.waitForBalanceUpdate(c, subject.UserID, userBefore.Balance)
	if userAfter != nil {
		result.BalanceAfter = userAfter.Balance
		result.Cost = maxFloat(0, userBefore.Balance-userAfter.Balance)
		result.BillingSettled = result.Cost > 0 || result.BalanceAfter != result.BalanceBefore
	}

	response.Success(c, result)
}

func (h *PlaygroundHandler) forwardChat(apiKey *service.APIKey, c *gin.Context) {
	if playgroundUsesOpenAIGateway(apiKey) {
		h.openaiGateway.ChatCompletions(c)
		return
	}
	h.gateway.ChatCompletions(c)
}

func playgroundUsesOpenAIGateway(apiKey *service.APIKey) bool {
	if apiKey == nil || apiKey.Group == nil {
		return false
	}
	return apiKey.Group.Platform == service.PlatformOpenAI || apiKey.Group.Platform == service.PlatformGrok
}

// Models returns model candidates for the selected API key's group.
func (h *PlaygroundHandler) Models(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}

	var requestedID *int64
	if raw := strings.TrimSpace(c.Query("api_key_id")); raw != "" {
		id, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			response.BadRequest(c, "Invalid api_key_id")
			return
		}
		requestedID = &id
	}

	apiKey, err := h.selectAPIKey(c, subject.UserID, requestedID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	models, defaultModel, source := h.modelsForAPIKey(c, apiKey)
	out := PlaygroundModelsResponse{
		APIKeyID:     apiKey.ID,
		APIKeyName:   apiKey.Name,
		Models:       models,
		DefaultModel: defaultModel,
		Source:       source,
	}
	if apiKey.Group != nil {
		out.GroupName = apiKey.Group.Name
		out.Platform = apiKey.Group.Platform
	}
	response.Success(c, out)
}

func (r *PlaygroundChatRequest) normalize() {
	r.Model = strings.TrimSpace(r.Model)
	if r.Model == "" {
		r.Model = playgroundDefaultModel
	}
	r.Prompt = strings.TrimSpace(r.Prompt)
	if r.Prompt == "" {
		r.Prompt = playgroundDefaultPrompt
	}
	if r.MaxTokens <= 0 {
		r.MaxTokens = playgroundDefaultMaxTokens
	}
	if r.MaxTokens > playgroundMaxTokens {
		r.MaxTokens = playgroundMaxTokens
	}
}

func (r PlaygroundChatRequest) validate() error {
	if len([]rune(r.Prompt)) > playgroundMaxPromptRunes {
		return fmt.Errorf("prompt is too long, max %d characters", playgroundMaxPromptRunes)
	}
	return nil
}

func (h *PlaygroundHandler) selectAPIKey(c *gin.Context, userID int64, requestedID *int64) (*service.APIKey, error) {
	ctx := c.Request.Context()
	if requestedID != nil {
		validIDs, err := h.apiKeyService.VerifyOwnership(ctx, userID, []int64{*requestedID})
		if err != nil {
			return nil, err
		}
		if len(validIDs) != 1 {
			return nil, service.ErrAPIKeyNotFound
		}

		apiKey, err := h.apiKeyService.GetByID(ctx, *requestedID)
		if err != nil {
			return nil, err
		}
		if apiKey == nil || apiKey.UserID != userID || !apiKey.IsActive() {
			return nil, service.ErrAPIKeyNotFound
		}
		return apiKey, nil
	}

	params := pagination.PaginationParams{Page: 1, PageSize: 1, SortBy: "created_at", SortOrder: "desc"}
	keys, _, err := h.apiKeyService.List(c.Request.Context(), userID, params, service.APIKeyListFilters{Status: service.StatusActive})
	if err != nil {
		return nil, err
	}
	if len(keys) == 0 {
		return nil, service.ErrAPIKeyNotFound
	}

	apiKey, err := h.apiKeyService.GetByID(ctx, keys[0].ID)
	if err != nil {
		return nil, err
	}
	if apiKey == nil || apiKey.UserID != userID || !apiKey.IsActive() {
		return nil, service.ErrAPIKeyNotFound
	}
	return apiKey, nil
}

func (h *PlaygroundHandler) applyGatewayResponse(raw []byte, result *PlaygroundChatResponse) {
	if len(raw) == 0 || result == nil {
		return
	}
	if result.Success {
		result.ResolvedModel = gjson.GetBytes(raw, "model").String()
		result.Text = gjson.GetBytes(raw, "choices.0.message.content").String()
		if usage := gjson.GetBytes(raw, "usage"); usage.Exists() {
			var m map[string]any
			if err := json.Unmarshal([]byte(usage.Raw), &m); err == nil {
				result.Usage = m
			}
		}
		return
	}
	msg := gjson.GetBytes(raw, "error.message").String()
	code := gjson.GetBytes(raw, "error.type").String()
	if code == "" {
		code = "playground_error"
	}
	if msg == "" {
		msg = strings.TrimSpace(string(raw))
	}
	result.Error = &PlaygroundError{
		Code:       code,
		Message:    msg,
		Suggestion: playgroundSuggestion(code, msg),
	}
}

func (h *PlaygroundHandler) waitForBalanceUpdate(c *gin.Context, userID int64, before float64) *service.User {
	var latest *service.User
	for i := 0; i < playgroundBillingPolls; i++ {
		user, err := h.userService.GetByID(c.Request.Context(), userID)
		if err == nil {
			latest = user
			if user.Balance != before {
				return user
			}
		}
		time.Sleep(playgroundBillingPollDelay)
	}
	return latest
}

func (h *PlaygroundHandler) modelsForAPIKey(c *gin.Context, apiKey *service.APIKey) ([]string, string, string) {
	if apiKey == nil || apiKey.Group == nil {
		return []string{playgroundDefaultModel}, playgroundDefaultModel, "default"
	}

	group := apiKey.Group
	groupID := group.ID
	platform := group.Platform
	source := "account"
	models := h.gateway.gatewayService.GetAvailableModels(c.Request.Context(), &groupID, platform)
	if len(models) == 0 {
		models = playgroundDefaultModelsForPlatform(platform)
		source = "platform_default"
	}
	defaultModel := firstNonEmptyPlayground(group.DefaultMappedModel, firstModel(models), playgroundDefaultModel)
	models = prependUniqueModel(models, defaultModel)
	return models, defaultModel, source
}

func prependUniqueModel(models []string, model string) []string {
	model = strings.TrimSpace(model)
	if model == "" {
		return models
	}
	out := make([]string, 0, len(models)+1)
	seen := map[string]struct{}{model: {}}
	out = append(out, model)
	for _, item := range models {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		out = append(out, item)
	}
	return out
}

func firstModel(models []string) string {
	for _, model := range models {
		if strings.TrimSpace(model) != "" {
			return strings.TrimSpace(model)
		}
	}
	return ""
}

func firstNonEmptyPlayground(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func playgroundDefaultModelsForPlatform(platform string) []string {
	switch platform {
	case service.PlatformOpenAI:
		return []string{"gpt-5.4", "gpt-5.4-mini", "gpt-5.3-codex"}
	case service.PlatformGrok:
		return []string{"grok-4", "grok-4-fast"}
	case service.PlatformGemini:
		return []string{"gemini-3.1-pro", "gemini-3.1-flash"}
	case service.PlatformAntigravity:
		return []string{"claude-sonnet-4-6", "claude-opus-4-6", "gemini-3.1-pro"}
	default:
		return []string{"claude-sonnet-4-6", "claude-opus-4-6", "claude-haiku-4-5"}
	}
}

func playgroundSuggestion(code, message string) string {
	text := strings.ToLower(code + " " + message)
	switch {
	case strings.Contains(text, "no available") || strings.Contains(text, "exhausted"):
		return "当前分组没有可用上游账号或并发已满，请稍后重试或切换模型。"
	case strings.Contains(text, "balance") || strings.Contains(text, "billing"):
		return "请检查账户余额、订阅状态或当前 API Key 的配额限制。"
	case strings.Contains(text, "model"):
		return "请确认模型名是否在当前 Key 所属分组中可用。"
	case strings.Contains(text, "rate"):
		return "请求触发限流，请稍后重试。"
	default:
		return "请保留该错误信息并联系管理员排查渠道和上游账号状态。"
	}
}

func maxFloat(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}
