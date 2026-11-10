package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/codexapp"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

const (
	OpenAIExecutionBackendExtraKey  = "openai_execution_backend"
	OpenAIExecutionBackendAppServer = "codex_app_server"
	codexAppServerSessionTTL        = 24 * time.Hour
)

// CodexAppServerBackend is implemented by codexapp.Pool. The interface keeps
// process lifecycle and thread affinity out of the HTTP gateway service.
type CodexAppServerBackend interface {
	RunTurn(
		ctx context.Context,
		workerKey string,
		sessionKey string,
		request codexapp.TurnRequest,
		sink codexapp.EventSink,
	) (codexapp.TurnResult, error)
}

// SetCodexAppServerBackend installs the process pool used only by accounts
// explicitly configured with openai_execution_backend=codex_app_server.
func (s *OpenAIGatewayService) SetCodexAppServerBackend(backend CodexAppServerBackend) {
	if s != nil {
		s.codexAppServerBackend = backend
	}
}

func shouldForwardViaCodexAppServer(account *Account) bool {
	return account != nil && strings.EqualFold(
		strings.TrimSpace(account.GetExtraString(OpenAIExecutionBackendExtraKey)),
		OpenAIExecutionBackendAppServer,
	)
}

// A Provider serializes turns on one app-server worker. Until the pool has
// per-account lanes, advertising a larger account concurrency would only move
// the queue behind an in-process mutex and hide it from the existing scheduler.
func openAIAccountSchedulingConcurrency(account *Account) int {
	if shouldForwardViaCodexAppServer(account) {
		return 1
	}
	if account == nil {
		return 0
	}
	return account.Concurrency
}

func openAIAccountSchedulingLoadFactor(account *Account) int {
	if shouldForwardViaCodexAppServer(account) {
		return 1
	}
	return account.EffectiveLoadFactor()
}

type codexAppServerRequest struct {
	Model              string          `json:"model"`
	Input              json.RawMessage `json:"input"`
	Stream             bool            `json:"stream"`
	PromptCacheKey     string          `json:"prompt_cache_key"`
	PreviousResponseID string          `json:"previous_response_id"`
}

type codexAppServerSessionBinding struct {
	SessionKey string
	ExpiresAt  time.Time
}

func (b codexAppServerSessionBinding) sessionExpiresAt() time.Time { return b.ExpiresAt }
func (b codexAppServerSessionBinding) sessionCost() int            { return len(b.SessionKey) + 32 }
func (b codexAppServerSessionBinding) cloneSessionValue() sessionMemoryValue {
	return b
}

func (s *OpenAIGatewayService) forwardResponsesViaCodexAppServer(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	body []byte,
) (*OpenAIForwardResult, error) {
	if s == nil || s.codexAppServerBackend == nil {
		return nil, errors.New("codex app-server backend is not configured")
	}

	var request codexAppServerRequest
	if err := json.Unmarshal(body, &request); err != nil {
		return nil, fmt.Errorf("decode Codex app-server request: %w", err)
	}
	request.Model = strings.TrimSpace(request.Model)
	if request.Model == "" {
		return nil, errors.New("model is required for Codex app-server requests")
	}
	input, err := decodeCodexAppServerInput(request.Input)
	if err != nil {
		return nil, err
	}

	responseID := "resp_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	messageID := "msg_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	apiKeyID := getAPIKeyIDFromContext(c)
	sessionKey := s.resolveCodexAppServerSessionKey(apiKeyID, request)
	workerKey := "account:" + strconv.FormatInt(account.ID, 10)
	startedAt := time.Now()
	var firstTokenMs *int
	var output strings.Builder
	streamStarted := false

	startStream := func() error {
		if streamStarted {
			return nil
		}
		streamStarted = true
		c.Header("Content-Type", "text/event-stream")
		c.Header("Cache-Control", "no-cache")
		c.Header("Connection", "keep-alive")
		c.Status(http.StatusOK)
		return writeCodexAppServerSSE(c, "response.created", map[string]any{
			"type": "response.created",
			"response": map[string]any{
				"id": responseID, "object": "response", "status": "in_progress", "model": request.Model,
			},
		})
	}

	result, err := s.codexAppServerBackend.RunTurn(ctx, workerKey, sessionKey, codexapp.TurnRequest{
		Model: request.Model, Input: input,
	}, func(event codexapp.Event) error {
		if event.Type != codexapp.EventAgentMessageDelta || event.Delta == "" {
			return nil
		}
		if firstTokenMs == nil {
			elapsed := int(time.Since(startedAt).Milliseconds())
			firstTokenMs = &elapsed
		}
		_, _ = output.WriteString(event.Delta)
		if !request.Stream {
			return nil
		}
		if err := startStream(); err != nil {
			return err
		}
		return writeCodexAppServerSSE(c, "response.output_text.delta", map[string]any{
			"type": "response.output_text.delta", "response_id": responseID,
			"item_id": messageID, "output_index": 0, "content_index": 0, "delta": event.Delta,
		})
	})
	if err != nil {
		return nil, fmt.Errorf("run Codex app-server turn: %w", err)
	}
	if result.Status != "completed" {
		return nil, fmt.Errorf("codex app-server turn ended with status %q", result.Status)
	}

	s.codexAppServerSessions.Store(responseID, codexAppServerSessionBinding{
		SessionKey: sessionKey, ExpiresAt: time.Now().Add(codexAppServerSessionTTL),
	})
	response := codexAppServerResponse(responseID, messageID, request.Model, output.String(), result.Usage)
	if request.Stream {
		if err := startStream(); err != nil {
			return nil, err
		}
		if err := writeCodexAppServerSSE(c, "response.completed", map[string]any{
			"type": "response.completed", "response": response,
		}); err != nil {
			return nil, err
		}
	} else {
		c.JSON(http.StatusOK, response)
	}

	duration := time.Since(startedAt)
	return &OpenAIForwardResult{
		ResponseID: responseID,
		Usage: OpenAIUsage{
			InputTokens: result.Usage.InputTokens, CacheReadInputTokens: result.Usage.CachedInputTokens,
			OutputTokens: result.Usage.OutputTokens,
		},
		Model: request.Model, UpstreamModel: request.Model, Stream: request.Stream,
		UpstreamEndpoint: "codex-app-server", Duration: duration, FirstTokenMs: firstTokenMs,
	}, nil
}

func (s *OpenAIGatewayService) resolveCodexAppServerSessionKey(apiKeyID int64, request codexAppServerRequest) string {
	apiPrefix := "api:" + strconv.FormatInt(apiKeyID, 10) + ":"
	if promptCacheKey := strings.TrimSpace(request.PromptCacheKey); promptCacheKey != "" {
		return apiPrefix + "prompt:" + promptCacheKey
	}
	if previousResponseID := strings.TrimSpace(request.PreviousResponseID); previousResponseID != "" {
		if raw, ok := s.codexAppServerSessions.Load(previousResponseID); ok {
			if binding, ok := raw.(codexAppServerSessionBinding); ok && binding.SessionKey != "" {
				return binding.SessionKey
			}
		}
	}
	return apiPrefix + "response:" + uuid.NewString()
}

func decodeCodexAppServerInput(raw json.RawMessage) ([]codexapp.InputItem, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, errors.New("input is required for Codex app-server requests")
	}
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil, fmt.Errorf("decode Codex app-server input: %w", err)
	}
	var texts []string
	if err := collectCodexAppServerInputText(value, &texts); err != nil {
		return nil, err
	}
	items := make([]codexapp.InputItem, 0, len(texts))
	for _, text := range texts {
		if text = strings.TrimSpace(text); text != "" {
			items = append(items, codexapp.InputItem{Type: "text", Text: text})
		}
	}
	if len(items) == 0 {
		return nil, errors.New("codex app-server input does not contain text")
	}
	return items, nil
}

func collectCodexAppServerInputText(value any, texts *[]string) error {
	switch typed := value.(type) {
	case string:
		*texts = append(*texts, typed)
		return nil
	case []any:
		for _, item := range typed {
			if err := collectCodexAppServerInputText(item, texts); err != nil {
				return err
			}
		}
		return nil
	case map[string]any:
		kind, _ := typed["type"].(string)
		if kind == "function_call_output" || kind == "computer_call_output" {
			return fmt.Errorf("codex app-server input type %q is not supported by this provider", kind)
		}
		if text, ok := typed["text"].(string); ok {
			*texts = append(*texts, text)
			return nil
		}
		if content, ok := typed["content"]; ok {
			return collectCodexAppServerInputText(content, texts)
		}
		return fmt.Errorf("codex app-server input object type %q does not contain text", kind)
	default:
		return fmt.Errorf("unsupported Codex app-server input value %T", value)
	}
}

func codexAppServerResponse(responseID, messageID, model, text string, usage codexapp.TokenUsage) map[string]any {
	return map[string]any{
		"id": responseID, "object": "response", "created_at": time.Now().Unix(),
		"status": "completed", "model": model,
		"output": []any{map[string]any{
			"id": messageID, "type": "message", "status": "completed", "role": "assistant",
			"content": []any{map[string]any{"type": "output_text", "text": text, "annotations": []any{}}},
		}},
		"usage": map[string]any{
			"input_tokens": usage.InputTokens, "output_tokens": usage.OutputTokens,
			"total_tokens":          usage.TotalTokens,
			"input_tokens_details":  map[string]any{"cached_tokens": usage.CachedInputTokens},
			"output_tokens_details": map[string]any{"reasoning_tokens": usage.ReasoningOutputTokens},
		},
	}
}

func writeCodexAppServerSSE(c *gin.Context, event string, payload any) error {
	encoded, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintf(c.Writer, "event: %s\ndata: %s\n\n", event, encoded); err != nil {
		return err
	}
	c.Writer.Flush()
	return nil
}
