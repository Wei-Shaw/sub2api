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
	"github.com/Wei-Shaw/sub2api/internal/pkg/muse"
	"github.com/gin-gonic/gin"
)

func (s *OpenAIGatewayService) MuseCore() *MuseCoreService { return s.muse }

func (s *OpenAIGatewayService) forwardMuse(ctx context.Context, c *gin.Context, a *Account, body []byte, endpoint string) (*OpenAIForwardResult, error) {
	if c.Request != nil && strings.HasSuffix(c.Request.URL.Path, "/responses/compact") {
		return nil, writeMuseError(c, endpoint, muse.ErrCapability, false)
	}
	if s.muse == nil || !s.muse.Qualified() {
		return nil, writeMuseError(c, endpoint, muse.ErrTransportUnqualified, false)
	}
	key, _ := c.Get("api_key")
	apiKey, _ := key.(*APIKey)
	var subscription *UserSubscription
	if v, ok := c.Get("subscription"); ok {
		subscription, _ = v.(*UserSubscription)
	}
	input, err := parseMuseRequest(body, endpoint)
	if err != nil {
		return nil, writeMuseError(c, endpoint, err, false)
	}
	publicModel := input.Model
	chatState := apicompat.NewResponsesEventToChatState()
	if endpoint == "chat_completions" {
		var options struct {
			StreamOptions *apicompat.ChatStreamOptions `json:"stream_options"`
		}
		// parseMuseRequest already validated this request and option's shape.
		if err := json.Unmarshal(body, &options); err != nil {
			return nil, writeMuseError(c, endpoint, muse.ErrInvalid, false)
		}
		chatState.IncludeUsage = options.StreamOptions != nil && options.StreamOptions.IncludeUsage
	}

	anthropicState := apicompat.NewResponsesEventToAnthropicState()
	started := false
	start := time.Now()
	emit := func(event muse.Event) error {
		if !input.Stream {
			return nil
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if !started {
			c.Header("Content-Type", "text/event-stream")
			c.Header("Cache-Control", "no-cache")
			c.Header("X-Accel-Buffering", "no")
			c.Status(http.StatusOK)
			started = true
		}
		event.Data.Response = cloneMuseResponse(event.Data.Response, publicModel, museResponseID(event.OperationID))
		switch endpoint {
		case "chat_completions":
			for _, chunk := range apicompat.ResponsesEventToChatChunks(&event.Data, chatState) {
				data, err := json.Marshal(chunk)
				if err != nil {
					return err
				}
				if _, err = fmt.Fprintf(c.Writer, "data: %s\n\n", data); err != nil {
					return err
				}
			}
		case "messages":
			for _, item := range apicompat.ResponsesEventToAnthropicEvents(&event.Data, anthropicState) {
				data, err := json.Marshal(item)
				if err != nil {
					return err
				}
				if _, err = fmt.Fprintf(c.Writer, "event: %s\ndata: %s\n\n", item.Type, data); err != nil {
					return err
				}
			}
		default:
			data, err := json.Marshal(event.Data)
			if err != nil {
				return err
			}
			if _, err = fmt.Fprintf(c.Writer, "event: %s\ndata: %s\n\n", event.Data.Type, data); err != nil {
				return err
			}
		}
		c.Writer.Flush()
		return nil
	}
	result, turn, err := s.muse.Execute(ctx, apiKey, a, input, subscription, emit)
	forward := &OpenAIForwardResult{Model: publicModel, UpstreamModel: a.GetMappedModel(publicModel), UpstreamEndpoint: "muse/" + endpoint, Stream: input.Stream, Duration: time.Since(start)}
	if turn != nil {
		forward.RequestID = turn.ID
		forward.ResponseID = museResponseID(turn.ID)
		forward.MuseTurnID = turn.ID
	}
	if ctx.Err() != nil {
		forward.ClientDisconnect = true
	}
	if err != nil {
		return forward, writeMuseError(c, endpoint, err, started)
	}
	forward.UpstreamResponseModel = result.Response.Model
	response := cloneMuseResponse(result.Response, publicModel, forward.ResponseID)
	if input.Stream {
		if !started {
			return forward, writeMuseError(c, endpoint, muse.ErrInvalid, false)
		}
		if endpoint == "chat_completions" {
			_, err = fmt.Fprint(c.Writer, "data: [DONE]\n\n")
			c.Writer.Flush()
		}
		return forward, err
	}
	switch endpoint {
	case "chat_completions":
		c.JSON(http.StatusOK, apicompat.ResponsesToChatCompletions(response, publicModel))
	case "messages":
		c.JSON(http.StatusOK, apicompat.ResponsesToAnthropic(response, publicModel))
	default:
		c.JSON(http.StatusOK, response)
	}
	return forward, nil
}

func cloneMuseResponse(r *apicompat.ResponsesResponse, model, id string) *apicompat.ResponsesResponse {
	if r == nil {
		return nil
	}
	next := *r
	next.Model = model
	next.ID = id
	return &next
}

func parseMuseRequest(body []byte, endpoint string) (*apicompat.ResponsesRequest, error) {
	decode := func(v any) error {
		decoder := json.NewDecoder(bytes.NewReader(body))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(v); err != nil {
			return muse.ErrInvalid
		}
		var extra any
		if decoder.Decode(&extra) != io.EOF {
			return muse.ErrInvalid
		}
		return nil
	}
	// Shared converters add OpenAI-specific defaults. Restore the client's actual
	// controls here so Muse does not silently acquire streaming, token floors,
	// storage flags, or invented reasoning settings.
	switch endpoint {
	case "chat_completions":
		var input apicompat.ChatCompletionsRequest
		if err := decode(&input); err != nil {
			return nil, err
		}
		if len(input.Stop) > 0 || len(input.PromptCacheOptions) > 0 {
			return nil, muse.ErrCapability
		}
		out, err := apicompat.ChatCompletionsToResponses(&input)
		if err != nil {
			return nil, muse.ErrInvalid
		}
		out.Stream = input.Stream
		out.Store = nil
		out.Include = nil
		out.Temperature = input.Temperature
		out.TopP = input.TopP
		out.MaxOutputTokens = input.MaxTokens
		if input.MaxCompletionTokens != nil {
			out.MaxOutputTokens = input.MaxCompletionTokens
		}
		return out, nil
	case "messages":
		var input apicompat.AnthropicRequest
		if err := decode(&input); err != nil {
			return nil, err
		}
		if input.MaxTokens <= 0 {
			return nil, muse.ErrInvalid
		}
		if len(input.StopSeqs) > 0 || len(input.Metadata) > 0 || (input.Thinking != nil && input.Thinking.BudgetTokens > 0) {
			return nil, muse.ErrCapability
		}
		// Cache hints cannot be reproduced by the consumer app adapter.
		var raw any
		if json.Unmarshal(body, &raw) != nil {
			return nil, muse.ErrInvalid
		}
		if museHasCacheHints(raw) {
			return nil, muse.ErrCapability
		}
		out, err := apicompat.AnthropicToResponses(&input)
		if err != nil {
			return nil, muse.ErrInvalid
		}
		out.Store = nil
		out.Include = nil
		out.ParallelToolCalls = nil
		out.Text = nil
		out.MaxOutputTokens = &input.MaxTokens
		out.Temperature = input.Temperature
		out.TopP = input.TopP
		if input.Thinking == nil && input.OutputConfig == nil {
			out.Reasoning = nil
		}
		return out, nil
	case "responses":
		var input apicompat.ResponsesRequest
		if err := decode(&input); err != nil {
			return nil, err
		}
		if input.Model == "" || len(input.Input) == 0 {
			return nil, muse.ErrInvalid
		}
		return &input, nil
	default:
		return nil, muse.ErrCapability
	}
}

func museHasCacheHints(v any) bool {
	switch value := v.(type) {
	case map[string]any:
		for key, item := range value {
			if key == "cache_control" || key == "prompt_cache_breakpoint" {
				return true
			}
			if museHasCacheHints(item) {
				return true
			}
		}
	case []any:
		for _, item := range value {
			if museHasCacheHints(item) {
				return true
			}
		}
	}
	return false
}

func writeMuseError(c *gin.Context, endpoint string, err error, stream bool) error {
	status := http.StatusBadGateway
	code := "muse_turn_failed"
	message := "Muse turn could not be completed; accepted work will not be replayed"
	switch {
	case errors.Is(err, muse.ErrTransportUnqualified):
		status = http.StatusServiceUnavailable
		code = "muse_transport_unqualified"
		message = "Muse app protocol verification is required before this account can serve traffic"
	case errors.Is(err, muse.ErrCapability):
		status = http.StatusBadRequest
		code = "muse_capability_unsupported"
		message = "This Muse account does not support the requested model or capability"
	case errors.Is(err, muse.ErrOwner):
		status = http.StatusForbidden
		code = "muse_workspace_owner_mismatch"
		message = "This Muse workspace is assigned to another user"
	case errors.Is(err, muse.ErrBusy):
		status = http.StatusConflict
		code = "muse_workspace_busy"
		message = "Muse has unresolved work in this workspace"
	case errors.Is(err, muse.ErrGeneration) || errors.Is(err, muse.ErrLease):
		status = http.StatusConflict
		code = "muse_state_changed"
		message = "Muse session or turn ownership changed; verify the account again"
	case errors.Is(err, muse.ErrRejected):
		status = http.StatusBadRequest
		code = "muse_turn_rejected"
		message = "Muse rejected this request before accepting a task"
	case errors.Is(err, muse.ErrNotFound):
		status = http.StatusNotFound
		code = "muse_turn_not_found"
		message = "Owned Muse turn not found"
	case errors.Is(err, muse.ErrInvalid):
		status = http.StatusBadRequest
		code = "muse_request_invalid"
		message = "Unsupported or invalid Muse request"
	}
	if c.Request.Context().Err() != nil {
		return err
	}
	body := gin.H{"error": gin.H{"type": "api_error", "code": code, "message": message}}
	if endpoint == "messages" {
		body["type"] = "error"
	}
	if stream {
		data, _ := json.Marshal(body)
		_, _ = fmt.Fprintf(c.Writer, "event: error\ndata: %s\n\n", data)
		c.Writer.Flush()
	} else {
		c.JSON(status, body)
	}
	return err
}
