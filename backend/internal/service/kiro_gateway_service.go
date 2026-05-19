package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/Wei-Shaw/sub2api/internal/pkg/kiro"
)

// kiroStreamingTimeout is the upper bound on an in-flight Kiro request
// (Anthropic SSE proxying). Kiro doesn't stream forever; 10 minutes is
// roomy enough for the largest realistic generation while preventing
// runaway connections.
const kiroStreamingTimeout = 10 * time.Minute

// KiroGatewayService orchestrates Anthropic /v1/messages → Kiro for a
// single account. It is the public surface called by the gateway
// handler (Phase 4c).
//
// Phase 4 ships Anthropic inbound only. OpenAI Chat Completions support
// arrives in Phase 5 via the same KiroGatewayService.
type KiroGatewayService struct {
	tokenProvider *KiroTokenProvider
	proxyRepo     ProxyRepository
	usageRecorder UsageRecorder
}

// UsageRecorder is the narrow interface the gateway needs to record one
// request's usage. The default implementation in this codebase is
// usage_record_worker_pool's RecordAsync; we declare it locally to avoid
// dragging in the worker pool's full surface here.
type UsageRecorder interface {
	RecordKiroUsage(ctx context.Context, args UsageRecordArgs)
}

// UsageRecordArgs is the shape of a single Kiro usage record. Pruned
// from the usage_log column set — gateway fills these in; the worker
// pool persists.
type UsageRecordArgs struct {
	AccountID    int64
	Model        string
	InputTokens  int
	OutputTokens int
	Credits      float64
	DurationMs   int64
	Stream       bool
	Error        string // empty for success
}

// NewKiroGatewayService constructs the service.
func NewKiroGatewayService(
	tokenProvider *KiroTokenProvider,
	proxyRepo ProxyRepository,
	usageRecorder UsageRecorder,
) *KiroGatewayService {
	return &KiroGatewayService{
		tokenProvider: tokenProvider,
		proxyRepo:     proxyRepo,
		usageRecorder: usageRecorder,
	}
}

// GetTokenProvider exposes the token provider for handlers that need to
// pre-warm it (mirrors AntigravityGatewayService.GetTokenProvider).
func (s *KiroGatewayService) GetTokenProvider() *KiroTokenProvider {
	return s.tokenProvider
}

// Forward proxies an Anthropic /v1/messages request to Kiro using the
// supplied account.
//
//   - On stream=true: writes Anthropic SSE frames to c.Writer as events
//     arrive, returns ForwardResult after the stream closes.
//   - On stream=false: aggregates the full event sequence, then writes a
//     single Anthropic Messages response.
//
// The caller (gateway_handler) sets the SSE headers via the existing
// shared helpers before invoking Forward in the stream=true case.
func (s *KiroGatewayService) Forward(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	body []byte,
) (*ForwardResult, error) {
	if account == nil {
		return nil, errors.New("kiro gateway: nil account")
	}

	startedAt := time.Now()
	req := &kiro.AnthropicRequest{}
	if err := json.Unmarshal(body, req); err != nil {
		return nil, fmt.Errorf("kiro gateway: decode request: %w", err)
	}
	requestedModel := req.Model

	// Per-account ProfileARN comes from extra (filled in at OAuth time).
	profileARN := account.GetExtraString("profile_arn")
	machineID := account.GetExtraString("machine_id")
	if machineID == "" {
		// Backfill once. Persistence is best-effort — if the update
		// fails we still use the generated value for this request.
		machineID = kiro.GenerateMachineID()
		if account.Extra == nil {
			account.Extra = map[string]any{}
		}
		account.Extra["machine_id"] = machineID
	}

	payload, err := kiro.TransformAnthropicRequest(req, profileARN)
	if err != nil {
		return nil, fmt.Errorf("kiro gateway: transform request: %w", err)
	}

	accessToken, err := s.tokenProvider.GetAccessToken(ctx, account)
	if err != nil {
		return nil, fmt.Errorf("kiro gateway: get access token: %w", err)
	}

	httpClient := kiro.HTTPStreamingClient(s.resolveProxyURL(ctx, account))

	prefEndpoint := account.GetExtraString("preferred_endpoint")
	callCtx, cancel := context.WithTimeout(ctx, kiroStreamingTimeout)
	defer cancel()

	callResult, err := kiro.Call(callCtx, kiro.CallOptions{
		AccessToken:       accessToken,
		MachineID:         machineID,
		PreferredEndpoint: prefEndpoint,
		EndpointFallback:  true,
		Payload:           payload,
		HTTPClient:        httpClient,
	})
	if err != nil {
		s.recordError(account, requestedModel, req.Stream, time.Since(startedAt), err)
		return nil, err
	}
	defer callResult.Response.Body.Close()

	result := &ForwardResult{
		RequestID: "req_" + uuid.New().String(),
		Model:     requestedModel,
		Stream:    req.Stream,
	}
	mappedModel := kiro.MapModel(requestedModel)
	if mappedModel != "" && mappedModel != requestedModel {
		result.UpstreamModel = mappedModel
	}

	var inputTokens, outputTokens int
	var credits float64
	var streamErr error

	if req.Stream {
		setKiroSSEHeaders(c)
		writer := kiro.NewAnthropicSSEWriter(c.Writer, c.Writer.Flush, mappedModel)
		dispatch, finalize := kiro.ProcessEventsFromCallback(payload.ToolNameMap, writer.Callback())
		// Tap credits / context_usage / token totals so we can record usage.
		dispatch = wrapDispatchForUsage(dispatch, &inputTokens, &outputTokens, &credits)

		streamErr = kiro.DecodeEventStream(callResult.Response.Body, dispatch)
		finalize()
		result.Duration = time.Since(startedAt)
	} else {
		var events []kiro.Event
		streamErr = kiro.DecodeEventStream(callResult.Response.Body, func(e kiro.Event) {
			events = append(events, e)
		})
		// Aggregate into a non-streaming response body.
		resp := kiro.BuildAnthropicNonStreamingResponse(events, mappedModel, payload.ToolNameMap)
		c.JSON(http.StatusOK, resp)
		inputTokens = resp.Usage["input_tokens"]
		outputTokens = resp.Usage["output_tokens"]
		credits = sumMeteringFromEvents(events)
		result.Duration = time.Since(startedAt)
	}

	result.Usage = ClaudeUsage{
		InputTokens:  inputTokens,
		OutputTokens: outputTokens,
	}
	s.recordSuccess(account, result, credits)
	return result, streamErr
}

// IsModelSupported reports whether the requested model id will route to
// a known Kiro model name. Used by gateway selection to filter accounts.
func (s *KiroGatewayService) IsModelSupported(requestedModel string) bool {
	mapped, _ := kiro.ParseModelAndThinking(requestedModel, kiro.ThinkingSuffix)
	return strings.HasPrefix(strings.ToLower(mapped), "claude-")
}

func (s *KiroGatewayService) resolveProxyURL(ctx context.Context, account *Account) string {
	if s.proxyRepo == nil || account == nil || account.ProxyID == nil {
		return ""
	}
	p, err := s.proxyRepo.GetByID(ctx, *account.ProxyID)
	if err != nil || p == nil {
		return ""
	}
	return p.URL()
}

func (s *KiroGatewayService) recordSuccess(account *Account, result *ForwardResult, credits float64) {
	if s.usageRecorder == nil {
		return
	}
	s.usageRecorder.RecordKiroUsage(context.Background(), UsageRecordArgs{
		AccountID:    account.ID,
		Model:        result.Model,
		InputTokens:  result.Usage.InputTokens,
		OutputTokens: result.Usage.OutputTokens,
		Credits:      credits,
		DurationMs:   result.Duration.Milliseconds(),
		Stream:       result.Stream,
	})
}

func (s *KiroGatewayService) recordError(account *Account, model string, stream bool, d time.Duration, err error) {
	if s.usageRecorder == nil {
		return
	}
	s.usageRecorder.RecordKiroUsage(context.Background(), UsageRecordArgs{
		AccountID:  account.ID,
		Model:      model,
		DurationMs: d.Milliseconds(),
		Stream:     stream,
		Error:      err.Error(),
	})
}

// setKiroSSEHeaders sets the streaming response headers. Matches what
// other gateway services do.
func setKiroSSEHeaders(c *gin.Context) {
	c.Writer.Header().Set("Content-Type", "text/event-stream")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("Connection", "keep-alive")
	c.Writer.Header().Set("X-Accel-Buffering", "no")
}

// wrapDispatchForUsage tees the event dispatcher so we can capture
// running token totals and credits without complicating the SSE writer.
func wrapDispatchForUsage(
	dispatch func(kiro.Event),
	inputTokens, outputTokens *int,
	credits *float64,
) func(kiro.Event) {
	var lastIn, lastOut int
	return func(ev kiro.Event) {
		if ev.Type == "meteringEvent" {
			if usage, ok := ev.Payload["usage"].(float64); ok {
				*credits += usage
			}
		}
		// Reuse the same token extraction as ProcessEvents so streamed
		// usage figures roll up monotonically.
		newIn, newOut := kiroPublicUpdateTokens(ev.Payload, lastIn, lastOut)
		if newIn != lastIn {
			lastIn = newIn
			*inputTokens = newIn
		}
		if newOut != lastOut {
			lastOut = newOut
			*outputTokens = newOut
		}
		dispatch(ev)
	}
}

// kiroPublicUpdateTokens is a thin wrapper exposing the package-private
// token tracker; we re-export through pkg/kiro for the gateway path.
// Defined in pkg/kiro/response_transformer.go via UpdateTokensFromEvent
// re-export (added below in this commit).
func kiroPublicUpdateTokens(event map[string]any, currentIn, currentOut int) (int, int) {
	return kiro.UpdateTokensFromEvent(event, currentIn, currentOut)
}

// sumMeteringFromEvents collects credits from the captured event slice
// (non-streaming path).
func sumMeteringFromEvents(events []kiro.Event) float64 {
	var total float64
	for _, ev := range events {
		if ev.Type != "meteringEvent" {
			continue
		}
		if usage, ok := ev.Payload["usage"].(float64); ok {
			total += usage
		}
	}
	return total
}

// drainHTTPBody is a defensive helper used for some error paths.
func drainHTTPBody(r io.Reader) {
	_, _ = io.Copy(io.Discard, r)
}
