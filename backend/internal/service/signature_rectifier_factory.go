package service

import (
	"context"
	"io"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/Wei-Shaw/sub2api/internal/service/signature"
)

// signatureRectifierFactory picks the appropriate rectifier strategy (strip vs
// pool) per retry based on the current RectifierSettings and the account type.
//
// Decision table (matches the design confirmed with the user):
//
//	Enabled=false                                                            -> (rectifier itself doesn't even trigger — gated earlier by settings)
//	Enabled=true  + SignaturePoolSize==0                                     -> Strip
//	Enabled=true  + SignaturePoolSize>0 + account sub-switch off             -> Strip (rule: harvest/replace only for accounts whose own signature switch is on)
//	Enabled=true  + SignaturePoolSize>0 + account sub-switch on              -> Pool (rule A: pool empty → transparent pass-through, not a strip fallback)
//
// OAuth/SetupToken accounts share a single pool; APIKey accounts get per-id pools.
type signatureRectifierFactory struct {
	stripClaude      *signature.StripClaudeRectifier
	stripAntigravity *signature.StripAntigravityRectifier
	pool             signature.SignaturePool
	settingService   *SettingService
}

func newSignatureRectifierFactory(pool signature.SignaturePool, settingService *SettingService) *signatureRectifierFactory {
	return &signatureRectifierFactory{
		stripClaude: &signature.StripClaudeRectifier{
			FilterStage1: FilterThinkingBlocksForRetry,
			FilterStage2: FilterSignatureSensitiveBlocksForRetry,
		},
		stripAntigravity: &signature.StripAntigravityRectifier{
			StripStage1: stripThinkingFromClaudeRequest,
			StripStage2: stripSignatureSensitiveBlocksFromClaudeRequest,
		},
		pool:           pool,
		settingService: settingService,
	}
}

// shouldUsePool implements the decision table above. Shared between the
// Rectifier Factory (chooses strategy at retry time) and the Harvester
// (decides whether to ingest signatures from responses).
func (f *signatureRectifierFactory) shouldUsePool(ctx context.Context, account *Account) bool {
	if f == nil || f.pool == nil || f.settingService == nil || account == nil {
		return false
	}
	s, err := f.settingService.GetRectifierSettings(ctx)
	if err != nil || s == nil {
		return false
	}
	if !s.Enabled || s.SignaturePoolSize <= 0 {
		return false
	}
	switch account.Type {
	case AccountTypeOAuth, AccountTypeSetupToken:
		return s.ThinkingSignatureEnabled
	case AccountTypeAPIKey:
		return s.APIKeySignatureEnabled
	}
	return false
}

// poolCapacity returns the current pool size configured in settings (0 if
// pool is disabled or settings are unavailable).
func (f *signatureRectifierFactory) poolCapacity(ctx context.Context) int {
	if f == nil || f.settingService == nil {
		return 0
	}
	s, err := f.settingService.GetRectifierSettings(ctx)
	if err != nil || s == nil {
		return 0
	}
	return s.SignaturePoolSize
}

// ForClaude returns the rectifier to use for the next Claude-native retry.
func (f *signatureRectifierFactory) ForClaude(ctx context.Context, account *Account) signature.ClaudeRectifier {
	if f.shouldUsePool(ctx, account) {
		return &signature.PoolClaudeRectifier{
			Pool:     f.pool,
			Capacity: f.poolCapacity(ctx),
		}
	}
	return f.stripClaude
}

// ForAntigravity returns the rectifier to use for the next Antigravity retry.
func (f *signatureRectifierFactory) ForAntigravity(ctx context.Context, account *Account) signature.AntigravityRectifier {
	if f.shouldUsePool(ctx, account) {
		return &signature.PoolAntigravityRectifier{
			Pool:     f.pool,
			Capacity: f.poolCapacity(ctx),
		}
	}
	return f.stripAntigravity
}

// WrapResponseBody wraps an upstream response body with a Harvester that
// ingests thinking signatures into the pool. When pool is disabled for this
// account (per shouldUsePool), the original body is returned unchanged so
// the caller can use a single line at every resp.Body entry point.
//
// Retry requests are automatically skipped via ctxkey.IsSignatureRectifyRetry
// on the context — the harvester's Skip callback checks it at read time.
func (f *signatureRectifierFactory) WrapResponseBody(ctx context.Context, account *Account, body io.ReadCloser, streaming bool) io.ReadCloser {
	if f == nil || body == nil || !f.shouldUsePool(ctx, account) {
		return body
	}
	h := signature.NewHarvester(f.pool, f.poolCapacity(ctx))
	return h.Wrap(ctx, body, signature.HarvestOptions{
		Bucket:    signature.BucketFor(account.Type, account.Platform, account.ID),
		Streaming: streaming,
		Skip: func() bool {
			v, _ := ctx.Value(ctxkey.IsSignatureRectifyRetry).(bool)
			return v
		},
	})
}
