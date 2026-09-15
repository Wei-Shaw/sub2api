package service

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/tidwall/gjson"
	"go.uber.org/zap"
)

const openAICapacityUsageDrainTimeout = 2 * time.Second

// A persistent WS binding must not bypass state written by a previous turn.
func (s *OpenAIGatewayService) OpenAIWSAPIKeyAccountAvailable(ctx context.Context, account *Account, groupID *int64, model string) bool {
	if account == nil || account.Platform != PlatformOpenAI || account.Type != AccountTypeAPIKey {
		return true
	}
	if s.accountRepo == nil {
		return false
	}
	latest, err := s.accountRepo.GetByID(ctx, account.ID)
	return err == nil && latest != nil && s.openAIAccountMatchesSchedulingGroup(latest, groupID) &&
		isOpenAICompatibleAccountEligibleForRequestBeforeProfit(ctx, latest, PlatformOpenAI, model, false, OpenAIEndpointCapabilityResponses) &&
		!s.isOpenAIAccountRequestRuntimeBlocked(latest, model)
}

// OpenAIStreamTerminalError retains a committed failure without making it replayable.
type OpenAIStreamTerminalError struct {
	StatusCode    int
	ResponseBody  []byte
	Message       string
	healthHandled bool
}

func (e *OpenAIStreamTerminalError) Error() string {
	return fmt.Sprintf("upstream response failed: %s", e.Message)
}

func isOpenAIAPIKeyCapacityFailure(account *Account, payload []byte) bool {
	return account != nil && account.Platform == PlatformOpenAI && account.Type == AccountTypeAPIKey &&
		isOpenAICapacityHealthPayload(payload)
}

func isOpenAICapacityHealthPayload(payload []byte) bool {
	if openAIStreamFailureStatus(payload, "") != http.StatusServiceUnavailable || isOpenAIContextWindowError("", payload) {
		return false
	}
	cyber, _, _ := detectOpenAICyberPolicy(payload)
	return !cyber && isOpenAIRequestScopedCapacityShed("", payload)
}

func markOpenAIHealthHandled(err error) {
	var streamErr *OpenAIStreamTerminalError
	if errors.As(err, &streamErr) {
		streamErr.healthHandled = true
	}
	var failoverErr *UpstreamFailoverError
	if errors.As(err, &failoverErr) {
		failoverErr.AccountHealthHandled = true
	}
}

func (s *OpenAIGatewayService) observeOpenAICapacityTerminalFailure(ctx context.Context, account *Account, model string, headers http.Header, payload []byte, message string) *OpenAIStreamTerminalError {
	err := &OpenAIStreamTerminalError{
		StatusCode:   openAIStreamFailureStatus(payload, message),
		ResponseBody: append([]byte(nil), payload...),
		Message:      sanitizeUpstreamErrorMessage(message),
	}
	err.healthHandled = s.handleOpenAIAccountUpstreamError(ctx, account, err.StatusCode, headers, payload, model)
	policyMatched := err.healthHandled
	s.ObserveOpenAIAccountHealthFailure(ctx, account, err)
	logger.FromContext(ctx).Info("openai.capacity_terminal_observed",
		zap.Int64("account_id", account.ID), zap.String("upstream_model", model),
		zap.Int("upstream_status", err.StatusCode), zap.Bool("explicit_rule_handled", policyMatched))
	return err
}

// Only Close runs in the timer goroutine. Parsing, health and downstream writes
// stay in the stream owner. Closing also bounds the timeout-disabled Scan path.
type openAICapacityUsageDrain struct {
	timer   *time.Timer
	done    chan struct{}
	expired atomic.Bool
}

func (d *openAICapacityUsageDrain) start(body io.Closer) {
	if d.timer != nil {
		return
	}
	d.done = make(chan struct{})
	d.timer = time.AfterFunc(openAICapacityUsageDrainTimeout, func() {
		defer close(d.done)
		d.expired.Store(true)
		_ = body.Close()
	})
}

func (d *openAICapacityUsageDrain) stop() {
	if d.timer != nil && !d.timer.Stop() {
		<-d.done
	}
}

type openAICapacityStreamDiagnostics struct {
	firstSemanticAt    time.Time
	firstSemanticEvent string
	semanticBytes      int
	errorObservedAt    time.Time
	failureDeliveredAt time.Time
	finishReason       string
	keepaliveLogged    bool
}

// Record only the shape of the first unrecognized heartbeat in an attempt.
// Values and nested keys can contain user content and must not enter the log.
func (d *openAICapacityStreamDiagnostics) unrecognizedKeepalive(ctx context.Context, account *Account, path string, data []byte) {
	if d.keepaliveLogged || account == nil || !account.IsOpenAIApiKey() {
		return
	}
	d.keepaliveLogged = true
	kindOf := func(value gjson.Result) string {
		switch value.Type {
		case gjson.String:
			return "string"
		case gjson.Number:
			return "number"
		case gjson.True, gjson.False:
			return "boolean"
		case gjson.JSON:
			if value.IsArray() {
				return "array"
			}
			return "object"
		default:
			return "null"
		}
	}
	kind := "invalid_json"
	fieldTypes := make(map[string]string)
	omitted := false
	if gjson.ValidBytes(data) {
		payload := gjson.ParseBytes(data)
		kind = kindOf(payload)
		if payload.IsObject() {
			payload.ForEach(func(key, value gjson.Result) bool {
				if len(fieldTypes) >= 16 {
					omitted = true
					return false
				}
				if len(key.Str) > 64 {
					omitted = true
					return true
				}
				fieldTypes[key.Str] = kindOf(value)
				return true
			})
		}
	}
	logger.FromContext(ctx).Warn("openai.unrecognized_keepalive",
		zap.Int64("account_id", account.ID), zap.String("path", path),
		zap.Int("data_bytes", len(data)), zap.String("payload_kind", kind),
		zap.Any("field_types", fieldTypes), zap.Bool("fields_omitted", omitted))
}

func (d *openAICapacityStreamDiagnostics) committed(eventType string, size int) {
	if d.firstSemanticAt.IsZero() {
		d.firstSemanticAt = time.Now()
		d.firstSemanticEvent = eventType
	}
	d.semanticBytes += size
}

func (d *openAICapacityStreamDiagnostics) finish(ctx context.Context, account *Account, model, path, requestID string, usage *OpenAIUsage, drainExpired, disconnected bool) {
	if drainExpired {
		d.finishReason = "usage_drain_timed_out"
	} else if d.finishReason == "" {
		d.finishReason = "upstream_end"
	}
	fields := []zap.Field{
		zap.Int64("account_id", account.ID), zap.String("upstream_model", model),
		zap.String("path", path), zap.String("upstream_request_id", requestID),
		zap.String("first_semantic_event", d.firstSemanticEvent), zap.Int("semantic_bytes", d.semanticBytes),
		zap.Time("error_observed_at", d.errorObservedAt), zap.Time("finished_at", time.Now()),
		zap.String("finish_reason", d.finishReason), zap.Bool("client_disconnected", disconnected),
		zap.Bool("usage_observed", usage != nil && (usage.InputTokens > 0 || usage.OutputTokens > 0 || usage.CacheReadInputTokens > 0)),
	}
	if !d.firstSemanticAt.IsZero() {
		fields = append(fields, zap.Time("first_semantic_committed_at", d.firstSemanticAt))
	}
	if !d.failureDeliveredAt.IsZero() {
		fields = append(fields, zap.Time("failure_flushed_at", d.failureDeliveredAt))
	}
	logger.FromContext(ctx).Warn("openai.capacity_stream_finished", fields...)
}
