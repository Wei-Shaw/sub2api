package signature

import (
	"context"
	"log/slog"
)

// PoolClaudeRectifier implements the pool-replace strategy for the Claude native
// retry path. On stage ThinkingOnly it fetches the newest signatures from the
// pool and rewrites every bad signature in the request body by cycling through
// them. Any further stage is a no-op: pool replacement is strictly one-shot
// (rule A: pool empty → transparent pass-through, no strip fallback).
type PoolClaudeRectifier struct {
	Pool SignaturePool
	// Capacity is the per-request cap on signatures fetched. Set from
	// RectifierSettings.SignaturePoolSize; if ≤0, a safe default of 64 is used.
	Capacity int
}

const defaultPoolFetchCap = 64

func (r *PoolClaudeRectifier) Apply(ctx context.Context, in ClaudeInput, stage Stage) ([]byte, bool) {
	if r == nil || r.Pool == nil {
		return nil, false
	}
	if stage != StageThinkingOnly {
		return nil, false
	}
	bucket := BucketFor(in.AccountType, in.Platform, in.AccountID)
	if bucket == "" {
		return nil, false
	}
	n := r.Capacity
	if n <= 0 {
		n = defaultPoolFetchCap
	}
	sigs, err := r.Pool.TopN(ctx, bucket, n)
	if err != nil || len(sigs) == 0 {
		return nil, false
	}
	newBody, replaced := ReplaceThinkingSignaturesInBody(in.Body, sigs)
	if replaced == 0 {
		return nil, false
	}
	slog.Warn("signature_pool.claude_replace",
		"account_id", in.AccountID, "bucket", bucket,
		"pool_size", len(sigs), "replaced", replaced)
	return newBody, true
}

// PoolAntigravityRectifier implements the pool-replace strategy for the
// Antigravity (v1internal Claude-sub-link) path. It mutates the intermediate
// ClaudeRequest struct in place. Stages() returns [ThinkingOnly] so the
// outer loop only iterates once; applied=true with proceed=false signals
// "retry this one time then stop" to the retry loop.
type PoolAntigravityRectifier struct {
	Pool     SignaturePool
	Capacity int
}

func (r *PoolAntigravityRectifier) Apply(ctx context.Context, in AntigravityInput, stage Stage) (bool, bool, error) {
	if r == nil || r.Pool == nil || in.Request == nil {
		return false, false, nil
	}
	if stage != StageThinkingOnly {
		return false, false, nil
	}
	bucket := BucketFor(in.AccountType, in.Platform, in.AccountID)
	if bucket == "" {
		return false, false, nil
	}
	n := r.Capacity
	if n <= 0 {
		n = defaultPoolFetchCap
	}
	sigs, err := r.Pool.TopN(ctx, bucket, n)
	if err != nil || len(sigs) == 0 {
		return false, false, nil
	}
	replaced, err := ReplaceThinkingSignaturesInClaudeRequest(in.Request, sigs)
	if err != nil {
		return false, false, err
	}
	if replaced == 0 {
		return false, false, nil
	}
	slog.Warn("signature_pool.antigravity_replace",
		"account_id", in.AccountID, "bucket", bucket,
		"pool_size", len(sigs), "replaced", replaced)
	return true, false, nil
}

func (r *PoolAntigravityRectifier) Stages() []Stage {
	return []Stage{StageThinkingOnly}
}
