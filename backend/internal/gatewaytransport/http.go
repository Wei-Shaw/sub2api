package gatewaytransport

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

// Mount on a dedicated private listener, explicitly. This handler does not
// install stock server, scheduler, admin, probe or logging middleware.
func (h *Handler) ServeHTTP(raw http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost || r.URL.RawQuery != "" || r.Header.Get("Content-Type") != "application/json" || r.Header.Get("Content-Encoding") != "" {
		http.Error(raw, "private native transport denied", http.StatusBadRequest)
		return
	}
	owner := r.URL.Path == transportsPath+"/read-owner" || r.URL.Path == transportsPath+"/cancel-owner" || r.URL.Path == transportsPath+"/ack-owner"
	if r.URL.Path != transportsPath && r.URL.Path != transportsPath+"/cancel" && r.URL.Path != transportsPath+"/read" && r.URL.Path != transportsPath+"/ack" && !owner {
		http.NotFound(raw, r)
		return
	}
	peer, err := h.cfg.Authorize(r)
	if err != nil || !identifier.MatchString(peer.ConsumerID) ||
		(r.URL.Path == transportsPath || owner) && peer.Role != "execution" || r.URL.Path != transportsPath && !owner && peer.Role != "cleanup" {
		http.Error(raw, "private native transport denied", http.StatusForbidden)
		return
	}
	slots := h.controls
	if r.URL.Path == transportsPath {
		slots = h.executions
	}
	if !h.acquireHTTP(raw, r, slots) {
		return
	}
	// Includes envelope reads, callback, Forward/physical Close and all writes.
	// Local handler release never acknowledges upstream or durable closure.
	defer func() { <-slots }()
	controller := http.NewResponseController(raw)
	if controller.SetReadDeadline(time.Now().Add(h.cfg.IOTimeout)) != nil || controller.SetWriteDeadline(time.Now().Add(h.cfg.IOTimeout)) != nil {
		http.Error(raw, "private native transport denied", http.StatusServiceUnavailable)
		return
	}
	data, err := io.ReadAll(io.LimitReader(r.Body, h.cfg.EnvelopeBytes+1))
	if err != nil || int64(len(data)) > h.cfg.EnvelopeBytes {
		http.Error(raw, "private native transport denied", http.StatusBadRequest)
		return
	}
	if r.URL.Path != transportsPath {
		if owner {
			var input ownerClosureRequest
			if strictJSON(data, &input) != nil {
				http.Error(raw, "private native transport denied", http.StatusBadRequest)
				return
			}
			result, err := h.ownerClosure(r.Context(), peer.ConsumerID, strings.TrimPrefix(r.URL.Path, transportsPath+"/"), input.Proof)
			if err != nil {
				http.Error(raw, "private native transport denied", http.StatusConflict)
				return
			}
			writeReceipt(raw, result)
			return
		}
		var input cleanupRequest
		if strictJSON(data, &input) != nil {
			http.Error(raw, "private native transport denied", http.StatusBadRequest)
			return
		}
		result, err := h.cleanup(r.Context(), peer.ConsumerID, r.URL.Path[len(transportsPath)+1:], input)
		if err != nil {
			http.Error(raw, "private native transport denied", http.StatusConflict)
			return
		}
		writeReceipt(raw, result)
		return
	}
	var input Request
	if strictJSON(data, &input) != nil {
		http.Error(raw, "private native transport denied", http.StatusBadRequest)
		return
	}
	h.serveRequest(raw, r, peer, input, "")
}

// Both ingress forms use the same reservation, provider-entry CAS and lifetime.
func (h *Handler) serveRequest(raw http.ResponseWriter, r *http.Request, peer Peer, input Request, executionBearer string) {
	public := executionBearer != ""
	var terminal *publicTerminalWriter
	if public {
		terminal = &publicTerminalWriter{ResponseWriter: raw, budget: h.terminalBuffers, maximum: int(min(input.Admission.Limits.OutputBytes, 1<<20))}
		raw = terminal
		defer terminal.dispose()
	}
	controller := http.NewResponseController(raw)
	profile, configured := h.profile(input.Admission.ProfileID)
	if !configured || !input.Admission.valid(time.Now()) || !identifier.MatchString(input.RequestRef) || !canonicalUUID.MatchString(input.Worker) ||
		input.Descriptor.AccountID < 1 || !canonicalUUID.MatchString(input.Descriptor.Generation) || input.Descriptor.CreatedAt.IsZero() ||
		input.Descriptor.Profile != profile.Profile || input.Descriptor.Model != profile.Model || input.Descriptor.BaseURL != profile.BaseURL ||
		input.Admission.Limits.RequestBytes > profile.RequestBytes || input.Admission.Limits.OutputBytes > profile.OutputBytes || input.Admission.Limits.Tokens > profile.Tokens ||
		int64(len(input.Payload)) > input.Admission.Limits.RequestBytes {
		writeExecutionError(raw, public, http.StatusBadRequest, input.RequestRef)
		return
	}
	nativeMessages := public && r.URL.Path == "/v1/messages"
	requestedModel := profile.Model
	payload := []byte(input.Payload)
	var err error
	if nativeMessages {
		if profile.Profile != service.GatewayClaudeOAuthMessagesProfile ||
			service.ValidateGatewayNativeClaudeMessagesBounded(payload, input.Admission.Limits.RequestBytes, input.Admission.Limits.Tokens) != nil {
			writeExecutionError(raw, public, http.StatusBadRequest, input.RequestRef)
			return
		}
	} else {
		requireTier := !service.GatewayNativeCodingPlanProfileIDValid(profile.Profile) && profile.Profile != service.GatewayGrokOAuthResponsesProfile
		if profile.Profile == service.GatewayCodexOAuthResponsesProfile || profile.Profile == service.GatewayOpenRouterResponsesProfile || service.GatewayCodingPlanCatalogProfile(profile.Profile) {
			fields, valid := service.GatewayNativePayloadFields(input.Payload, "model")
			if !valid || json.Unmarshal(fields["model"], &requestedModel) != nil || !service.GatewayAccountModelIDValid(requestedModel) ||
				profile.Profile == service.GatewayOpenRouterResponsesProfile && !service.GatewayOpenRouterModelValid(requestedModel) {
				writeExecutionError(raw, public, http.StatusBadRequest, input.RequestRef)
				return
			}
		}
		payload, err = qualifyPayloadWithDefaultTier(input.Payload, requestedModel, input.Admission.Limits.Tokens, profile.ProviderTokenUpperBound, requireTier)
		if err != nil || int64(len(payload)) > input.Admission.Limits.RequestBytes {
			writeExecutionError(raw, public, http.StatusBadRequest, input.RequestRef)
			return
		}
		if profile.Profile == service.GatewayClaudeOAuthMessagesProfile && service.ValidateGatewayNativeClaudeResponsesBounded(payload, input.Admission.Limits.RequestBytes) != nil {
			writeExecutionError(raw, public, http.StatusBadRequest, input.RequestRef)
			return
		}
	}
	if service.GatewayNativeCodingPlanProfileIDValid(profile.Profile) || profile.Profile == service.GatewayGrokOAuthResponsesProfile {
		if service.ValidateGatewayNativeProviderResponsesBounded(profile.Profile, payload, input.Admission.Limits.RequestBytes) != nil {
			writeExecutionError(raw, public, http.StatusBadRequest, input.RequestRef)
			return
		}
	}
	deadline, _ := parseInstant(input.Admission.ExpiresAt)
	ctx, cancel := context.WithDeadline(r.Context(), deadline)
	if nativeMessages {
		ctx = service.WithGatewayNativeClaudeMessages(ctx)
	}
	// The original writer implements deadlines even when the Gin writer does not
	// implement Unwrap. Cancellation interrupts both a blocked write and read.
	var deadlineMu sync.Mutex
	interrupt := func() {
		deadlineMu.Lock()
		defer deadlineMu.Unlock()
		_ = controller.SetWriteDeadline(time.Now())
		_ = controller.SetReadDeadline(time.Now())
	}
	e, first, err := h.reserve(peer.ConsumerID, input, ctx, cancel, interrupt)
	if err != nil || !first {
		// Do not interrupt the response writer for this duplicate's own receipt.
		cancel()
		if err != nil {
			writeExecutionError(raw, public, http.StatusConflict, input.RequestRef)
			return
		}
		_ = controller.SetWriteDeadline(time.Now().Add(h.cfg.IOTimeout))
		h.writeExecutionReceipt(raw, peer.ConsumerID, e, public)
		return
	}
	if executionBearer != "" {
		h.mu.Lock()
		e.executionBearer = executionBearer
		// Retain only this validated scalar for public usage attribution. The
		// first reservation owns it; duplicates returned above cannot replace it.
		e.requestedModel = requestedModel
		h.mu.Unlock()
		defer func() {
			durable := h.settlePublic(peer.ConsumerID, e)
			lifetime := e.life.Snapshot()
			if !lifetime.Entered {
				return
			}
			// Only terminal delivery gets a fresh finite I/O deadline after
			// durable settlement. Forwarding is already closed; its canceled
			// context and approved provider lifetime must never be renewed.
			if !lifetime.Completed || !durable || controller.SetWriteDeadline(time.Now().Add(h.cfg.IOTimeout)) != nil || terminal.release() != nil {
				// No clean streaming EOF until terminal effect and physical
				// closure are durable. Accepted bytes/effects are not replayed.
				_ = controller.SetWriteDeadline(time.Now())
			}
		}()
	}
	permitted, err := h.admit(ctx, peer.ConsumerID, e)
	if err != nil || !permitted {
		// Positive no Forward/body entry, independently of whether SQL claimed.
		// Record it before cancellation so the original keepalive connection is
		// not interrupted when there is no forwarding I/O to unblock.
		h.finishNoEntry(e)
		h.seal(e)
		_ = controller.SetWriteDeadline(time.Now().Add(h.cfg.IOTimeout))
		h.writeExecutionReceipt(raw, peer.ConsumerID, e, public)
		return
	}
	ctx, err = service.WithGatewayNativeConsumer(ctx, peer.ConsumerID)
	if err != nil {
		h.finishNoEntry(e)
		h.seal(e)
		return
	}
	ctx = service.WithGatewayNativeCustody(ctx, h.cfg.Custody)
	ctx = service.WithGatewayNativeUsageObserver(ctx, func(usage service.GatewayNativeUsage) {
		h.mu.Lock()
		defer h.mu.Unlock()
		e.usage = &usage
	})
	ctx = service.WithGatewayNativeLifetime(ctx, e.life)
	ctx = service.WithGatewayNativeProviderReadIdle(ctx, h.cfg.ProviderReadIdle)
	ctx = service.WithGatewayProviderLimitsWriter(ctx, h.cfg.ProviderLimits)
	if profile.Profile == service.GatewayCodexOAuthResponsesProfile {
		ctx, err = h.cfg.OAuth.BindApproved(ctx, input.Descriptor, input.Admission.AccountRef, input.Admission.AuthorizationEpoch, input.Admission.Limits.RequestBytes, input.Admission.Limits.Tokens)
		if err != nil {
			h.finishNoEntry(e)
			h.seal(e)
			_ = controller.SetWriteDeadline(time.Now().Add(h.cfg.IOTimeout))
			h.writeExecutionReceipt(raw, peer.ConsumerID, e, public)
			return
		}
	}
	if profile.Profile == service.GatewayClaudeOAuthMessagesProfile {
		ctx, err = h.cfg.ClaudeOAuth.BindApproved(ctx, input.Descriptor, input.Admission.AccountRef, input.Admission.AuthorizationEpoch, input.Admission.Limits.RequestBytes, input.Admission.Limits.Tokens)
		if err != nil {
			h.finishNoEntry(e)
			h.seal(e)
			_ = controller.SetWriteDeadline(time.Now().Add(h.cfg.IOTimeout))
			h.writeExecutionReceipt(raw, peer.ConsumerID, e, public)
			return
		}
	}
	if profile.Profile == service.GatewayGrokOAuthResponsesProfile {
		scope, operation, resolveErr := h.cfg.GrokOAuth.ResolveApproved(ctx, input.Descriptor, input.Admission.AccountRef)
		if resolveErr == nil {
			ctx, resolveErr = service.WithGatewayNativeGrokDispatch(ctx, h.cfg.GrokOAuth, scope, operation, input.Admission.Limits.RequestBytes, input.Admission.Limits.Tokens)
		}
		if resolveErr != nil {
			h.finishNoEntry(e)
			h.seal(e)
			_ = controller.SetWriteDeadline(time.Now().Add(h.cfg.IOTimeout))
			h.writeExecutionReceipt(raw, peer.ConsumerID, e, public)
			return
		}
	}
	// Renew each output operation, never the approved lifetime. Serialize with
	// cancellation so a later renewal cannot undo its blocked-I/O interrupt.
	renewWriteDeadline := func() error {
		deadlineMu.Lock()
		defer deadlineMu.Unlock()
		if ctx.Err() != nil || e.life.Snapshot().Sealed {
			return context.Canceled
		}
		writeDeadline := time.Now().Add(h.cfg.IOTimeout)
		if deadline.Before(writeDeadline) {
			writeDeadline = deadline
		}
		return controller.SetWriteDeadline(writeDeadline)
	}
	if renewWriteDeadline() != nil {
		h.finishNoEntry(e)
		h.seal(e)
		return
	}
	c, _ := gin.CreateTestContext(raw)
	c.Writer = &privateGinWriter{ResponseWriter: c.Writer, raw: raw, renewWriteDeadline: renewWriteDeadline}
	ctx = service.WithGatewayNativeProviderQuotaObserver(ctx, func(quota service.GatewayNativeProviderQuota) {
		if c.Writer.Written() || renewWriteDeadline() != nil {
			return
		}
		raw.Header().Set("X-Gateway-Provider-Quota-Limit", strconv.FormatInt(quota.Limit, 10))
		raw.Header().Set("X-Gateway-Provider-Quota-Remaining", "0")
		raw.Header().Set("X-Gateway-Provider-Quota-Reset-At", quota.ResetAt)
		c.Writer.WriteHeader(http.StatusBadGateway)
		c.Writer.WriteHeaderNow()
	})
	c.Request = r.Clone(ctx)
	raw.Header().Set("X-Gateway-Request-Ref", input.RequestRef)
	_, _, _ = h.cfg.Gateway.ForwardGatewayRoute(ctx, c, input.Descriptor, payload)

	// Service observation records forward return/physical Close and terminal
	// delivery. HTTP completion itself never acknowledges kernel settlement.
	h.mu.Lock()
	e.sealed = true
	h.mu.Unlock()
}

// Management uses the SAME reserved control budget as transport cleanup and
// owner read/cancel/ack. Authentication precedes admission and body buffering;
// the existing management router still establishes protected consumer scope.
func (h *Handler) ManagementHandler(next http.Handler) http.Handler {
	return h.controlHandler("management", next)
}

// OAuth cleanup shares the reserved controls but accepts only cleanup peers.
func (h *Handler) CleanupHandler(next http.Handler) http.Handler {
	return h.controlHandler("cleanup", next)
}
func (h *Handler) controlHandler(role string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		peer, err := h.cfg.Authorize(r)
		if err != nil || peer.Role != role || !identifier.MatchString(peer.ConsumerID) {
			http.Error(w, "private native transport denied", http.StatusForbidden)
			return
		}
		if !h.acquireHTTP(w, r, h.controls) {
			return
		}
		defer func() { <-h.controls }()
		next.ServeHTTP(w, r)
	})
}

func (h *Handler) acquireHTTP(w http.ResponseWriter, r *http.Request, slots chan struct{}) bool {
	select {
	case slots <- struct{}{}:
		return true
	default:
		// Do not let net/http drain a held rejected body before sending the
		// rejection. No envelope read, registry reservation or callback occurs.
		r.Close = true
		w.Header().Set("Connection", "close")
		_ = http.NewResponseController(w).SetWriteDeadline(time.Now().Add(h.cfg.IOTimeout))
		http.Error(w, "private native transport busy", http.StatusServiceUnavailable)
		return false
	}
}

func (h *Handler) finishNoEntry(e *reservation) { e.life.NoForward() }
func writeReceipt(w http.ResponseWriter, r Receipt) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	_ = json.NewEncoder(w).Encode(r)
}

type privateGinWriter struct {
	gin.ResponseWriter
	raw                http.ResponseWriter
	renewWriteDeadline func() error
}

func (w *privateGinWriter) Unwrap() http.ResponseWriter { return w.raw }
func (w *privateGinWriter) Write(data []byte) (int, error) {
	if err := w.renewWriteDeadline(); err != nil {
		return 0, err
	}
	return w.ResponseWriter.Write(data)
}
func (w *privateGinWriter) FlushError() error {
	if err := w.renewWriteDeadline(); err != nil {
		return err
	}
	return http.NewResponseController(w.raw).Flush()
}

// The persisted one-use callback capability is its sole authorization. It still
// consumes the same eight management/cleanup slots, never execution capacity.
func (h *Handler) NativeOAuthCallbackHandler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/auth/callback" || r.URL.RawPath != "" {
			http.Error(w, "private native transport denied", http.StatusBadRequest)
			return
		}
		if !h.acquireHTTP(w, r, h.controls) {
			return
		}
		defer func() { <-h.controls }()
		next.ServeHTTP(w, r)
	})
}
