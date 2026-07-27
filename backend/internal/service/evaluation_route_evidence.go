package service

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/shopspring/decimal"
	"go.uber.org/zap"
)

var ErrRouteEvidenceIdentityConflict = errors.New("route evidence identity conflict")

var evaluationEvidencePersistenceFailures atomic.Uint64

func RecordEvaluationEvidencePersistenceFailure() {
	evaluationEvidencePersistenceFailures.Add(1)
}

func EvaluationEvidencePersistenceFailureCount() uint64 {
	return evaluationEvidencePersistenceFailures.Load()
}

type EvaluationEvidenceRepository interface {
	UpsertTransport(ctx context.Context, evidence RouteEvidence) error
	AttachBilling(ctx context.Context, traceID string, usage RouteUsageEvidence) error
}

type RouteTraceConfig struct {
	HashKey []byte
	Region  string
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
	RouteTraceID        string               `json:"route_trace_id"`
	EvaluationRunID     string               `json:"evaluation_run_id"`
	SampleID            string               `json:"sample_id"`
	APIKeyID            int64                `json:"api_key_id"`
	RequestID           string               `json:"request_id"`
	RequestedModel      string               `json:"requested_model"`
	ResolvedModel       string               `json:"resolved_model"`
	RouteProfileVersion string               `json:"route_profile_version"`
	Provider            string               `json:"provider"`
	ChannelRef          string               `json:"channel_ref"`
	AccountPoolRef      string               `json:"account_pool_ref"`
	Region              string               `json:"region"`
	Attempts            int                  `json:"attempts"`
	FallbackChain       []RouteFallbackEntry `json:"fallback_chain"`
	TransportStatus     string               `json:"transport_status"`
	ErrorCode           string               `json:"error_code"`
	StartedAt           time.Time            `json:"started_at"`
	FinishedAt          *time.Time           `json:"finished_at"`
}

type RouteUsageEvidence struct {
	InputTokens  int
	OutputTokens int
	TTFT         *int
	Latency      *int
	BilledAmount decimal.Decimal
	FinishReason string
}

type evaluationEvidenceRepositoryContextKey struct{}

func WithEvaluationEvidenceRepository(ctx context.Context, repo EvaluationEvidenceRepository) context.Context {
	return context.WithValue(ctx, evaluationEvidenceRepositoryContextKey{}, repo)
}

func EvaluationEvidenceRepositoryFromContext(ctx context.Context) (EvaluationEvidenceRepository, bool) {
	repo, ok := ctx.Value(evaluationEvidenceRepositoryContextKey{}).(EvaluationEvidenceRepository)
	return repo, ok && repo != nil
}

func attachEvaluationBillingEvidence(ctx context.Context, usageLog *UsageLog, finishReason string) {
	if usageLog == nil {
		return
	}
	evaluation, ok := EvaluationContextFromContext(ctx)
	if !ok {
		return
	}
	repo, ok := EvaluationEvidenceRepositoryFromContext(ctx)
	if !ok {
		return
	}

	usage := RouteUsageEvidence{
		InputTokens:  usageLog.InputTokens,
		OutputTokens: usageLog.OutputTokens,
		TTFT:         usageLog.FirstTokenMs,
		Latency:      usageLog.DurationMs,
		BilledAmount: decimal.NewFromFloat(usageLog.ActualCost),
		FinishReason: strings.TrimSpace(finishReason),
	}
	if err := repo.AttachBilling(ctx, evaluation.RouteTraceID, usage); err != nil {
		RecordEvaluationEvidencePersistenceFailure()
		logger.FromContext(ctx).Warn("evaluation billing evidence persistence failed",
			zap.String("route_trace_id", evaluation.RouteTraceID),
			zap.Error(err),
		)
	}
}

// RouteTrace collects only redacted routing information for one evaluation request.
type RouteTrace struct {
	mu       sync.Mutex
	hashKey  []byte
	evidence RouteEvidence
}

func NewRouteTrace(_ EvaluationContext, cfg RouteTraceConfig) *RouteTrace {
	return &RouteTrace{
		hashKey: append([]byte(nil), cfg.HashKey...),
		evidence: RouteEvidence{
			Region: strings.TrimSpace(cfg.Region),
		},
	}
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
		Region:        t.evidence.Region,
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
