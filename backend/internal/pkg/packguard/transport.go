package packguard

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

type Next func(*http.Request) (*http.Response, error)
type dispatchContextKey struct{}
type denialContextKey struct{}

// IsDenial identifies a locally generated response without trusting upstream
// headers or error text. The marker is never placed on a dispatched request.
func IsDenial(resp *http.Response) bool {
	return resp != nil && resp.Request != nil && resp.Request.Context().Value(denialContextKey{}) == true
}

// IsGuarded marks requests whose funding has been checked, including status reads.
func IsGuarded(ctx context.Context) bool { return ctx.Value(dispatchContextKey{}) == true }

type Transport struct{ Ledger Ledger }

func NewTransport(ledger Ledger) (*Transport, error) {
	if ledger == nil {
		return nil, fmt.Errorf("prepaid funding store is required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := ledger.Ready(ctx); err != nil {
		return nil, fmt.Errorf("prepaid funding store is unavailable: %w", err)
	}
	return &Transport{Ledger: ledger}, nil
}
func isArkHost(host string) bool {
	return host == "ark.ap-southeast.bytepluses.com" || host == "ark.eu-west.bytepluses.com"
}
func denied(req *http.Request, status int, code, message string) (*http.Response, error) {
	req = req.WithContext(context.WithValue(req.Context(), denialContextKey{}, true))
	body, _ := json.Marshal(map[string]any{"error": map[string]string{"type": "prepaid_pack_error", "code": code, "message": message}})
	return &http.Response{StatusCode: status, Status: http.StatusText(status), Header: http.Header{"Content-Type": []string{"application/json"}, "Cache-Control": []string{"no-store"}}, Body: io.NopCloser(bytes.NewReader(body)), ContentLength: int64(len(body)), Request: req}, nil
}

// RoundTrip is the final ModelArk transport gate, after protocol conversion and
// credential/header overrides. All routes using a funded credential share it.
func (t *Transport) RoundTrip(req *http.Request, accountID int64, next Next) (*http.Response, error) {
	if req == nil || req.URL == nil {
		return nil, fmt.Errorf("upstream request is required")
	}
	// A fresh indexed lookup is deliberate: TTL caches can let a newly enrolled
	// credential or a changed account endpoint bypass enforcement on another replica.
	auth := strings.Fields(req.Header.Get("Authorization"))
	key := ""
	if len(auth) == 2 && strings.EqualFold(auth[0], "Bearer") {
		key = auth[1]
	}
	scope, err := t.Ledger.Lookup(req.Context(), accountID, HashCredential(key), req.URL)
	if err != nil {
		return denied(req, 503, "prepaid_pack_state_unavailable", "Prepaid funding could not be verified; no request was sent upstream.")
	}
	if scope == nil {
		return next(req)
	} // Unenrolled credentials keep ordinary PAYG behavior.
	req = req.WithContext(context.WithValue(req.Context(), dispatchContextKey{}, true))
	base, _ := url.Parse(scope.APIBase)
	path := strings.TrimPrefix(req.URL.Path, strings.TrimRight(base.Path, "/"))
	taskID := ""
	if strings.HasPrefix(path, "/contents/generations/tasks/") {
		taskID = strings.TrimPrefix(path, "/contents/generations/tasks/")
		if taskID == "" || strings.Contains(taskID, "/") || len(taskID) > 256 {
			return denied(req, 400, "prepaid_pack_unsupported", "Invalid task identifier.")
		}
	}
	if req.Method == http.MethodGet && path == "/models" {
		return next(req)
	}
	if taskID != "" && (req.Method == http.MethodGet || req.Method == http.MethodDelete) {
		response, e := next(req)
		if e != nil {
			return response, e
		}
		if req.Method == http.MethodGet && response.StatusCode >= 200 && response.StatusCode < 300 {
			data, e := readResponse(response)
			if e != nil {
				return nil, e
			}
			response.Body = io.NopCloser(bytes.NewReader(data))
			t.settleTask(scope, taskID, data)
		}
		// DELETE is not proof that a running task stopped; keep its reservation.
		return response, nil
	}
	if req.Method != http.MethodPost || req.URL.RawQuery != "" {
		return denied(req, 400, "prepaid_pack_unsupported", "This operation is not qualified for prepaid-only use.")
	}
	if scope.APIBase != QualifiedAPIBase {
		return denied(req, 400, "prepaid_pack_unsupported", ErrRegion.Error())
	}
	if req.Body == nil {
		return denied(req, 400, "prepaid_pack_unsupported", "A bounded JSON request body is required.")
	}
	body, e := io.ReadAll(io.LimitReader(req.Body, MaxBodyBytes+1))
	_ = req.Body.Close()
	if e != nil {
		return denied(req, 400, "prepaid_pack_unsupported", "Could not read the request body.")
	}
	plan, canonical, e := BuildPlan(path, body)
	if e != nil {
		return denied(req, 400, "prepaid_pack_unsupported", e.Error())
	}
	reservation, e := t.Ledger.Reserve(req.Context(), scope, plan)
	if e != nil {
		if errors.Is(e, ErrExhausted) || errors.Is(e, ErrFrozen) {
			return denied(req, 402, "prepaid_pack_exhausted", "No prepaid capacity can safely cover this request through its lifetime. Pay-as-you-go is disabled for this route.")
		}
		return denied(req, 503, "prepaid_pack_state_unavailable", "Could not reserve prepaid capacity; no request was sent upstream.")
	}
	timeout := 5 * time.Minute
	if plan.Async {
		timeout = 30 * time.Second
	}
	ctx, cancel := context.WithTimeout(req.Context(), timeout)
	guarded := req.Clone(ctx)
	guarded.Body = io.NopCloser(bytes.NewReader(canonical))
	guarded.ContentLength = int64(len(canonical))
	// Disable transparent transport replay of a possibly accepted POST.
	guarded.GetBody = nil
	guarded.Header = guarded.Header.Clone()
	guarded.Header.Del("Idempotency-Key")
	guarded.Header.Del("X-Idempotency-Key")
	response, e := next(guarded)
	if e != nil {
		cancel()
		t.uncertain(reservation.ID)
		return response, e
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		response.Body = &cancelBody{ReadCloser: response.Body, cancel: cancel}
		// A timeout/5xx may have followed acceptance. Never refund it on a timer.
		if response.StatusCode >= 400 && response.StatusCode < 500 && response.StatusCode != 408 && response.StatusCode != 499 {
			t.finish(reservation.ID, 0, false)
		} else {
			t.uncertain(reservation.ID)
		}
		return response, nil
	}
	if plan.Async {
		data, e := readResponse(response)
		cancel()
		if e != nil {
			t.uncertain(reservation.ID)
			return nil, e
		}
		response.Body = io.NopCloser(bytes.NewReader(data))
		var v struct {
			ID string `json:"id"`
		}
		if json.Unmarshal(data, &v) != nil || v.ID == "" {
			t.uncertain(reservation.ID)
			return response, nil
		}
		ctx, done := ledgerContext()
		e = t.Ledger.Attach(ctx, reservation.ID, v.ID)
		done()
		if e != nil {
			t.uncertain(reservation.ID)
		}
		return response, nil
	}
	// Only explicit terminal usage permits settlement. A caller may close after
	// [DONE] without reading EOF; early close without terminal proof keeps funds.
	response.Body = &usageBody{ReadCloser: response.Body, transport: t, reservation: reservation.ID, cancel: cancel, stream: strings.Contains(response.Header.Get("Content-Type"), "text/event-stream")}
	return response, nil
}

func readResponse(response *http.Response) ([]byte, error) {
	defer func() { _ = response.Body.Close() }()
	data, err := io.ReadAll(io.LimitReader(response.Body, MaxResponseBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > MaxResponseBytes {
		return nil, fmt.Errorf("prepaid response exceeded safety limit")
	}
	return data, nil
}
func ledgerContext() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), 5*time.Second)
}
func (t *Transport) uncertain(id string) {
	ctx, cancel := ledgerContext()
	defer cancel()
	if err := t.Ledger.Uncertain(ctx, id); err != nil {
		slog.Error("byteplus_pack_hold_update_failed", "reservation", id)
	}
}
func (t *Transport) finish(id string, tokens int64, success bool) {
	ctx, cancel := ledgerContext()
	defer cancel()
	if err := t.Ledger.Finish(ctx, id, tokens, success); err != nil {
		slog.Error("byteplus_pack_settlement_failed_hold_retained", "reservation", id)
	}
}
func (t *Transport) settleTask(scope *Scope, task string, data []byte) {
	var v struct {
		ID, Status string
		Usage      struct {
			CompletionTokens int64 `json:"completion_tokens"`
		} `json:"usage"`
	}
	if json.Unmarshal(data, &v) != nil || (v.ID != "" && v.ID != task) {
		return
	}
	ctx, cancel := ledgerContext()
	defer cancel()
	switch v.Status {
	case "succeeded":
		if err := t.Ledger.FinishTask(ctx, scope.ID, task, v.Usage.CompletionTokens, true); err != nil {
			slog.Error("byteplus_pack_task_settlement_failed_hold_retained", "task", task)
		}
	case "failed", "expired", "cancelled", "canceled":
		if err := t.Ledger.FinishTask(ctx, scope.ID, task, 0, false); err != nil {
			slog.Error("byteplus_pack_task_release_failed_hold_retained", "task", task)
		}
	}
}

type usageBody struct {
	io.ReadCloser
	transport   *Transport
	reservation string
	cancel      context.CancelFunc
	stream      bool
	buffer      bytes.Buffer
	overflow    bool
	once        sync.Once
}
type cancelBody struct {
	io.ReadCloser
	cancel context.CancelFunc
}

func (b *cancelBody) Read(p []byte) (int, error) {
	n, err := b.ReadCloser.Read(p)
	if err != nil {
		b.cancel()
	}
	return n, err
}
func (b *cancelBody) Close() error { b.cancel(); return b.ReadCloser.Close() }
func (b *usageBody) Read(p []byte) (int, error) {
	n, err := b.ReadCloser.Read(p)
	if n > 0 && !b.overflow {
		if b.buffer.Len()+n > MaxResponseBytes {
			b.overflow = true
			b.buffer.Reset()
		} else {
			_, _ = b.buffer.Write(p[:n])
		}
	}
	if err == io.EOF {
		b.complete(true)
	} else if err != nil {
		b.complete(false)
	}
	return n, err
}
func (b *usageBody) Close() error { b.complete(false); return b.ReadCloser.Close() }
func (b *usageBody) complete(_ bool) {
	b.once.Do(func() {
		defer b.cancel()
		if b.overflow {
			b.transport.uncertain(b.reservation)
			return
		}
		tokens := usageTokens(b.buffer.Bytes(), b.stream)
		if tokens <= 0 {
			b.transport.uncertain(b.reservation)
			return
		}
		b.transport.finish(b.reservation, tokens, true)
	})
}

func usageTokens(data []byte, stream bool) int64 {
	terminal := false
	failed := false
	chatSummary := int64(0)
	responseSummary := int64(0)
	extract := func(data []byte) int64 {
		// Explicit tags below handle the API's snake_case names.
		var raw map[string]json.RawMessage
		if json.Unmarshal(data, &raw) != nil {
			return 0
		}
		var kind, status string
		_ = json.Unmarshal(raw["type"], &kind)
		_ = json.Unmarshal(raw["status"], &status)
		if kind == "error" || kind == "response.failed" || len(raw["error"]) > 0 && string(raw["error"]) != "null" {
			failed = true
		}
		if kind == "response.completed" || kind == "response.incomplete" || status == "completed" || status == "incomplete" {
			terminal = true
		}
		var choices []struct {
			FinishReason *string `json:"finish_reason"`
		}
		_ = json.Unmarshal(raw["choices"], &choices)
		for _, choice := range choices {
			if choice.FinishReason != nil {
				terminal = true
			}
		}
		if child := raw["response"]; len(child) > 0 {
			var nested map[string]json.RawMessage
			if json.Unmarshal(child, &nested) == nil && len(nested["usage"]) > 0 {
				raw = nested
			}
		}
		var u struct {
			Total      int64 `json:"total_tokens"`
			Prompt     int64 `json:"prompt_tokens"`
			Completion int64 `json:"completion_tokens"`
			Input      int64 `json:"input_tokens"`
			Output     int64 `json:"output_tokens"`
		}
		if json.Unmarshal(raw["usage"], &u) != nil {
			return 0
		}
		total := max(u.Total, u.Prompt+u.Completion, u.Input+u.Output)
		if stream {
			if kind == "response.completed" || kind == "response.incomplete" {
				responseSummary = max(responseSummary, total)
			}
			if raw["choices"] != nil && len(choices) == 0 {
				chatSummary = max(chatSummary, total)
			}
		}
		return total
	}
	if !stream {
		total := extract(data)
		if !terminal || failed {
			return 0
		}
		return total
	}
	scanner := bufio.NewScanner(bytes.NewReader(data))
	scanner.Buffer(make([]byte, 4096), MaxResponseBytes)
	done := false
	for scanner.Scan() {
		line := scanner.Bytes()
		if bytes.HasPrefix(line, []byte("data:")) {
			chunk := bytes.TrimSpace(line[5:])
			if bytes.Equal(chunk, []byte("[DONE]")) {
				terminal = true
				done = true
			} else {
				extract(chunk)
			}
		}
	}
	if scanner.Err() != nil || !terminal || failed {
		return 0
	}
	if responseSummary > 0 {
		return responseSummary
	}
	if done {
		return chatSummary
	}
	return 0
}
