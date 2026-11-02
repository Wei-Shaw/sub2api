package service

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/gin-gonic/gin"
)

// ResponseOutcome records what the gateway observed while delivering a
// response downstream. It is independent of request_type (transport) and of
// consumption: a response that never reached the client may still carry
// confirmed upstream usage. Empty means the path does not track delivery and is
// persisted as NULL, which never implies proven delivery.
type ResponseOutcome string

const (
	// ResponseOutcomeWritten only proves the server finished writing the
	// response, not that the client application consumed it.
	ResponseOutcomeWritten         ResponseOutcome = "response_written"
	ResponseOutcomeClientCancelled ResponseOutcome = "client_cancelled"
	ResponseOutcomeWriteFailed     ResponseOutcome = "write_failed"
	ResponseOutcomeUpstreamFailed  ResponseOutcome = "upstream_failed"
)

// Delivery phases used in OpenAIDeliveryError and audit logs.
const (
	OpenAIDeliveryPhaseBeforeHeaders = "before_headers"
	OpenAIDeliveryPhaseBuffering     = "buffering"
	OpenAIDeliveryPhaseStreaming     = "streaming"
	OpenAIDeliveryPhaseWriting       = "writing"
)

func (o ResponseOutcome) IsValid() bool {
	switch o {
	case ResponseOutcomeWritten, ResponseOutcomeClientCancelled, ResponseOutcomeWriteFailed, ResponseOutcomeUpstreamFailed:
		return true
	default:
		return false
	}
}

// Failed reports a non-NULL outcome other than response_written.
func (o ResponseOutcome) Failed() bool {
	return o.IsValid() && o != ResponseOutcomeWritten
}

// ResponseOutcomePtr returns the persisted form; empty/unknown values are NULL.
func ResponseOutcomePtr(o ResponseOutcome) *string {
	if !o.IsValid() {
		return nil
	}
	value := string(o)
	return &value
}

// OpenAIDeliveryError reports that a GPT HTTP response could not be delivered
// to the downstream client. It is a downstream outcome, never an upstream
// failover signal: callers must not fail over, penalize, reset or report the
// account as scheduled successfully because of it.
type OpenAIDeliveryError struct {
	Outcome ResponseOutcome
	Phase   string
	Cause   error
}

func (e *OpenAIDeliveryError) Error() string {
	if e == nil {
		return ""
	}
	if e.Cause == nil {
		return fmt.Sprintf("openai downstream delivery %s during %s", e.Outcome, e.Phase)
	}
	return fmt.Sprintf("openai downstream delivery %s during %s: %v", e.Outcome, e.Phase, e.Cause)
}

func (e *OpenAIDeliveryError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

// AsOpenAIDeliveryError extracts a downstream delivery failure from err.
func AsOpenAIDeliveryError(err error) (*OpenAIDeliveryError, bool) {
	var deliveryErr *OpenAIDeliveryError
	if errors.As(err, &deliveryErr) && deliveryErr != nil {
		return deliveryErr, true
	}
	return nil, false
}

// OpenAIUpstreamFailedWithUsageError replaces an upstream failover signal once
// the supplier already reported nonzero usage for the attempt. It deliberately
// does not unwrap to *UpstreamFailoverError: the handler must stop retrying,
// answer with Failover's client response and record the consumption once.
type OpenAIUpstreamFailedWithUsageError struct {
	Failover *UpstreamFailoverError
}

func (e *OpenAIUpstreamFailedWithUsageError) Error() string {
	if e == nil || e.Failover == nil {
		return "openai upstream failed after reporting usage"
	}
	return "openai upstream failed after reporting usage: " + e.Failover.Error()
}

// AsOpenAIUpstreamFailedWithUsage extracts the stop-failover signal from err.
func AsOpenAIUpstreamFailedWithUsage(err error) (*OpenAIUpstreamFailedWithUsageError, bool) {
	var target *OpenAIUpstreamFailedWithUsageError
	if errors.As(err, &target) && target != nil && target.Failover != nil {
		return target, true
	}
	return nil, false
}

// newOpenAIDeliveryError classifies a delivery failure: a finished downstream
// request context means the client cancelled (or its deadline elapsed);
// otherwise the write itself failed.
func newOpenAIDeliveryError(ctx context.Context, phase string, cause error) *OpenAIDeliveryError {
	outcome := ResponseOutcomeWriteFailed
	if ctx != nil && ctx.Err() != nil {
		outcome = ResponseOutcomeClientCancelled
		if cause == nil {
			cause = ctx.Err()
		}
	}
	return &OpenAIDeliveryError{Outcome: outcome, Phase: phase, Cause: cause}
}

// openAIDownstreamGone reports whether a GPT request's downstream lifecycle has
// ended. Only OpenAI accounts are covered; other vendors keep detached drains.
func openAIDownstreamGone(ctx context.Context, account *Account) bool {
	return account != nil && account.IsOpenAI() && ctx != nil && ctx.Err() != nil
}

// openAIHTTPUpstreamContext derives the upstream request context for GPT HTTP
// paths. Unlike detachUpstreamContext it keeps the downstream cancellation and
// deadline, so a disconnect stops generation instead of letting it run unseen.
// linked=false keeps the original detach contract for every other vendor. The
// returned cancel must run once the response body is consumed or abandoned.
func openAIHTTPUpstreamContext(ctx context.Context, account *Account) (upstreamCtx context.Context, cancel context.CancelFunc, linked bool) {
	if account == nil || !account.IsOpenAI() {
		upstreamCtx, cancel = detachUpstreamContext(ctx)
		return upstreamCtx, cancel, false
	}
	if ctx == nil {
		ctx = context.Background()
	}
	upstreamCtx, cancel = context.WithCancel(ctx)
	return upstreamCtx, cancel, true
}

// bindOpenAIUpstreamBodyCancel makes closing the body also cancel the linked
// upstream context (idempotent), so every exit path releases it.
func bindOpenAIUpstreamBodyCancel(resp *http.Response, cancel context.CancelFunc) {
	if resp == nil || resp.Body == nil || cancel == nil {
		return
	}
	resp.Body = &openAIRequestContextReadCloser{ReadCloser: resp.Body, cleanup: cancel}
}

// HasObservedConsumption reports whether the upstream reported any billable
// consumption. Delivery failures without it must not create zero-usage rows.
func (r *OpenAIForwardResult) HasObservedConsumption() bool {
	if r == nil {
		return false
	}
	return openAIUsageHasTokens(&r.Usage) || r.ImageCount > 0 || r.SearchCount > 0 ||
		r.WebSearchCalls > 0 || r.VideoCount > 0 || r.AudioUsage != nil
}

// FlushOpenAIDownstream flushes the whole gin writer chain (ops capture, server
// timing, ...) and then reports the real network flush error from the raw
// net/http writer recorded by middleware.RawResponseWriterHandler. gin's Flush
// has no error result and http.ResponseController stops at the first
// http.Flusher, so the chain alone cannot surface a broken connection. A nil
// result only proves the server flushed, not that the client application read.
func FlushOpenAIDownstream(c *gin.Context) error {
	if c == nil || c.Writer == nil {
		return nil
	}
	c.Writer.Flush()
	if c.Request != nil {
		if raw, ok := c.Request.Context().Value(ctxkey.RawResponseWriter).(http.ResponseWriter); ok && raw != nil {
			if err := http.NewResponseController(raw).Flush(); err != nil && !errors.Is(err, http.ErrNotSupported) {
				return err
			}
			return nil
		}
	}
	// Without the raw writer (tests/other servers) use any FlushError in the chain.
	var w http.ResponseWriter = c.Writer
	for w != nil {
		if flusher, ok := w.(interface{ FlushError() error }); ok {
			return flusher.FlushError()
		}
		unwrapper, ok := w.(interface{ Unwrap() http.ResponseWriter })
		if !ok {
			break
		}
		w = unwrapper.Unwrap()
	}
	return nil
}

// writeOpenAIBufferedResponse writes a complete buffered body like c.Data but
// reports write errors, short writes and flush errors. Content-Type is only set
// when absent, matching gin's render.Data.
func writeOpenAIBufferedResponse(c *gin.Context, status int, contentType string, body []byte) error {
	c.Status(status)
	header := c.Writer.Header()
	if header.Get("Content-Type") == "" && contentType != "" {
		header.Set("Content-Type", contentType)
	}
	if status < http.StatusOK || status == http.StatusNoContent || status == http.StatusNotModified {
		c.Writer.WriteHeaderNow()
		return FlushOpenAIDownstream(c)
	}
	n, err := c.Writer.Write(body)
	if err == nil && n < len(body) {
		err = io.ErrShortWrite
	}
	if err != nil {
		return err
	}
	return FlushOpenAIDownstream(c)
}

// deliverOpenAIBufferedResponse writes a finished nonstream GPT response and
// classifies the result. A request that is already gone is not written.
func deliverOpenAIBufferedResponse(ctx context.Context, c *gin.Context, status int, contentType string, body []byte) (ResponseOutcome, error) {
	if ctx != nil && ctx.Err() != nil {
		return ResponseOutcomeClientCancelled, &OpenAIDeliveryError{
			Outcome: ResponseOutcomeClientCancelled, Phase: OpenAIDeliveryPhaseWriting, Cause: ctx.Err(),
		}
	}
	writeStarted := time.Now()
	err := writeOpenAIBufferedResponse(c, status, contentType, body)
	SetOpsLatencyMs(c, OpsResponseWriteMsKey, time.Since(writeStarted).Milliseconds())
	if err == nil && ctx != nil && ctx.Err() != nil {
		// Canceled during the native write/flush: not a delivered response.
		err = ctx.Err()
	}
	if err != nil {
		deliveryErr := newOpenAIDeliveryError(ctx, OpenAIDeliveryPhaseWriting, err)
		return deliveryErr.Outcome, deliveryErr
	}
	return ResponseOutcomeWritten, nil
}

// openAIFirstOutputObserverLineLimit bounds the per-line prefix kept while
// looking for the first semantic event; event types sit at the start of lines.
const openAIFirstOutputObserverLineLimit = 64 * 1024

// openAIFirstOutputObserver wraps a buffered (nonstream) GPT upstream body and
// records when the first semantic SSE output event arrives. Response headers
// and metadata events (response.created / in_progress) never count, and a
// JSON-only body produces no measurement (unknown, never header time).
type openAIFirstOutputObserver struct {
	io.ReadCloser
	start     time.Time
	line      []byte
	eventType string
	firstMs   *int
}

func newOpenAIFirstOutputObserver(body io.ReadCloser, start time.Time) *openAIFirstOutputObserver {
	return &openAIFirstOutputObserver{ReadCloser: body, start: start}
}

func (r *openAIFirstOutputObserver) Read(p []byte) (int, error) {
	n, err := r.ReadCloser.Read(p)
	if n > 0 && r.firstMs == nil {
		r.observe(p[:n])
	}
	return n, err
}

// FirstOutputMs returns the semantic first-output latency, or nil if unknown.
func (r *openAIFirstOutputObserver) FirstOutputMs() *int {
	if r == nil {
		return nil
	}
	return r.firstMs
}

func (r *openAIFirstOutputObserver) observe(chunk []byte) {
	for len(chunk) > 0 && r.firstMs == nil {
		idx := bytes.IndexByte(chunk, '\n')
		part := chunk
		if idx >= 0 {
			part = chunk[:idx]
		}
		if room := openAIFirstOutputObserverLineLimit - len(r.line); room > 0 {
			if len(part) > room {
				part = part[:room]
			}
			r.line = append(r.line, part...)
		}
		if idx < 0 {
			return
		}
		r.processLine(strings.TrimRight(string(r.line), "\r"))
		r.line = r.line[:0]
		chunk = chunk[idx+1:]
	}
}

func (r *openAIFirstOutputObserver) processLine(line string) {
	if line == "" {
		r.eventType = ""
		return
	}
	if eventType, ok := extractOpenAISSEEventLine(line); ok {
		r.eventType = strings.TrimSpace(eventType)
		return
	}
	data, ok := extractOpenAISSEDataLine(line)
	if !ok {
		return
	}
	if openAIStreamDataStartsSemanticTTFT(data, effectiveOpenAISSEEventType([]byte(data), r.eventType)) {
		ms := int(time.Since(r.start).Milliseconds())
		r.firstMs = &ms
	}
}

// classifyOpenAIHTTPPartialFailure normalizes partial results at raw Chat and
// Responses-via-Chat adapter boundaries. Failed consumption is never persisted
// with a legacy NULL outcome and mistaken for a delivered request.
func classifyOpenAIHTTPPartialFailure(c *gin.Context, account *Account, result *OpenAIForwardResult, err error) (*OpenAIForwardResult, error) {
	if result == nil || err == nil {
		return result, err
	}
	if outcome, outcomeErr, ok := openAIHTTPErrorOutcome(c, account, err, &result.Usage); ok {
		result.ResponseOutcome = outcome
		result.ClientDisconnect = outcome == ResponseOutcomeClientCancelled || outcome == ResponseOutcomeWriteFailed
		return result, outcomeErr
	}
	return result, err
}
