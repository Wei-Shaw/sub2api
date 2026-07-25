package service

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"strconv"
	"strings"
	"sync"
)

type RouteTraceConfig struct {
	HashKey []byte
}

type RouteAttempt struct {
	Provider      string
	AccountID     int64
	ChannelID     int64
	ResolvedModel string
	Region        string
	ErrorCode     string
}

type RouteFallbackEntry struct {
	Ordinal        int    `json:"ordinal"`
	Provider       string `json:"provider"`
	AccountPoolRef string `json:"account_pool_ref"`
	ChannelRef     string `json:"channel_ref"`
	ResolvedModel  string `json:"resolved_model"`
	Region         string `json:"region"`
	ErrorCode      string `json:"error_code"`
}

type RouteEvidence struct {
	Attempts      int                  `json:"attempts"`
	FallbackChain []RouteFallbackEntry `json:"fallback_chain"`
}

// RouteTrace collects only redacted routing information for one evaluation request.
type RouteTrace struct {
	mu       sync.Mutex
	hashKey  []byte
	evidence RouteEvidence
}

func NewRouteTrace(_ EvaluationContext, cfg RouteTraceConfig) *RouteTrace {
	return &RouteTrace{hashKey: append([]byte(nil), cfg.HashKey...)}
}

func (t *RouteTrace) RecordAttempt(attempt RouteAttempt) {
	if t == nil {
		return
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	entry := RouteFallbackEntry{
		Ordinal:        len(t.evidence.FallbackChain) + 1,
		Provider:       strings.TrimSpace(attempt.Provider),
		AccountPoolRef: RedactedResourceRef("account", attempt.AccountID, t.hashKey),
		ChannelRef:     RedactedResourceRef("channel", attempt.ChannelID, t.hashKey),
		ResolvedModel:  strings.TrimSpace(attempt.ResolvedModel),
		Region:         strings.TrimSpace(attempt.Region),
		ErrorCode:      strings.TrimSpace(attempt.ErrorCode),
	}
	t.evidence.FallbackChain = append(t.evidence.FallbackChain, entry)
	t.evidence.Attempts = len(t.evidence.FallbackChain)
}

func (t *RouteTrace) RecordLatestAttemptError(errorCode string) {
	if t == nil {
		return
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	if last := len(t.evidence.FallbackChain) - 1; last >= 0 {
		t.evidence.FallbackChain[last].ErrorCode = strings.TrimSpace(errorCode)
	}
}

func (t *RouteTrace) Snapshot() RouteEvidence {
	if t == nil {
		return RouteEvidence{}
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	return RouteEvidence{
		Attempts:      t.evidence.Attempts,
		FallbackChain: append([]RouteFallbackEntry(nil), t.evidence.FallbackChain...),
	}
}

func RedactedResourceRef(kind string, id int64, key []byte) string {
	if id <= 0 {
		return ""
	}
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write([]byte(strings.TrimSpace(kind)))
	_, _ = mac.Write([]byte{':'})
	_, _ = mac.Write([]byte(strconv.FormatInt(id, 10)))
	return strings.TrimSpace(kind) + "_" + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func WithRouteTrace(ctx context.Context, trace *RouteTrace) context.Context {
	return updateRequestMetadata(ctx, false, func(md *RequestMetadata) {
		md.RouteTrace = trace
	}, nil)
}

func RouteTraceFromContext(ctx context.Context) (*RouteTrace, bool) {
	if md := metadataFromContext(ctx); md != nil && md.RouteTrace != nil {
		return md.RouteTrace, true
	}
	return nil, false
}
