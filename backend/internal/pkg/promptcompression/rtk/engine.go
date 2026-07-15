package rtk

import (
	"context"
	"encoding/json"
	"time"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// NewEngine compiles an immutable filter snapshot. Callers should construct a
// new engine when a filter revision is published and share it between requests.
func NewEngine(cfg Config, filters []Filter) (*Engine, error) {
	if cfg.Mode == "" {
		cfg.Mode = ModeOff
	}
	if cfg.Intensity == "" {
		cfg.Intensity = IntensityBalanced
	}
	if cfg.MinCandidateBytes <= 0 {
		cfg.MinCandidateBytes = 256
	}
	if cfg.MinCandidateTokens <= 0 {
		cfg.MinCandidateTokens = 64
	}
	if cfg.MinSavedTokens <= 0 {
		cfg.MinSavedTokens = 32
	}
	if cfg.MaxBodyBytes <= 0 {
		cfg.MaxBodyBytes = 10 * 1024 * 1024
	}
	if cfg.MaxResultBytes <= 0 {
		cfg.MaxResultBytes = 1024 * 1024
	}
	if len(filters) == 0 {
		filters = DefaultFilters()
	}
	compiled, err := compileFilters(sortFilters(filters))
	if err != nil {
		return nil, err
	}
	return &Engine{config: cfg, filters: compiled, tokens: TiktokenCounter{}}, nil
}

// NewEngineWithTokenCounter allows deterministic tests or a host-provided
// tokenizer while retaining the same immutable filter snapshot semantics.
func NewEngineWithTokenCounter(cfg Config, filters []Filter, counter TokenCounter) (*Engine, error) {
	e, err := NewEngine(cfg, filters)
	if err != nil {
		return nil, err
	}
	if counter != nil {
		e.tokens = counter
	}
	return e, nil
}

// Prepare performs one bounded, fail-open compression pass. It never mutates
// the input slice. In observe mode Result.Body is always the original body.
func (e *Engine) Prepare(ctx context.Context, body []byte, opts Options) (result Result) {
	started := time.Now()
	if ctx == nil {
		ctx = context.Background()
	}
	if e == nil {
		result.Body = append([]byte(nil), body...)
		result.BeforeBytes = len(body)
		result.AfterBytes = len(body)
		result.Outcome, result.SkipReason = "fallback", "nil_engine"
		return result
	}
	result.Body = append([]byte(nil), body...)
	result.BeforeBytes = len(body)
	result.AfterBytes = len(body)
	result.Mode = opts.Mode
	if result.Mode == "" {
		result.Mode = e.config.Mode
	}
	defer func() {
		result.Duration = time.Since(started)
		if result.Body == nil {
			result.Body = body
		}
		if result.AfterBytes == 0 {
			result.AfterBytes = len(result.Body)
		}
	}()
	if result.Mode == ModeOff {
		result.Outcome, result.SkipReason = "off", "disabled"
		return result
	}
	if result.Mode != ModeObserve && result.Mode != ModeEnforce {
		result.Outcome, result.SkipReason = "fallback", "unknown_mode"
		return result
	}
	if len(body) == 0 || len(body) > e.config.MaxBodyBytes {
		result.Outcome, result.SkipReason = "skipped", "body_limit"
		return result
	}
	if e.config.MaxDuration > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, e.config.MaxDuration)
		defer cancel()
	}
	targets, err := discoverTargets(body, opts.Protocol, opts.ProtectedJSONPaths)
	if err != nil {
		result.Outcome, result.SkipReason = "fallback", "invalid_json"
		return result
	}
	result.Targets = targets
	for _, target := range targets {
		if target.Eligible {
			result.EligibleTargets++
		}
	}
	if len(targets) == 0 {
		result.Outcome, result.SkipReason = "skipped", "no_targets"
		return result
	}
	if opts.Filters != nil {
		if _, err := compileFilters(sortFilters(opts.Filters)); err != nil {
			result.Outcome, result.SkipReason = "fallback", "invalid_filter_pack"
			return result
		}
	}

	working := append([]byte(nil), body...)
	changed := false
	for _, target := range targets {
		if err := ctx.Err(); err != nil {
			result.Outcome, result.SkipReason = "fallback", "deadline"
			return result
		}
		if !target.Eligible || target.CacheProtected || target.IsError || len(target.Text) < e.config.MinCandidateBytes {
			continue
		}
		beforeTokens, afterTokens, transformed, appliedFilters := e.transformTarget(ctx, target, opts)
		if transformed == target.Text || len(transformed) >= len(target.Text) || (e.config.MaxResultBytes > 0 && len(transformed) > e.config.MaxResultBytes) {
			continue
		}
		if beforeTokens < e.config.MinCandidateTokens || beforeTokens-afterTokens < e.config.MinSavedTokens {
			continue
		}
		updated, err := sjson.SetBytes(working, target.JSONPath, transformed)
		if err != nil {
			continue
		}
		working = updated
		changed = true
		result.ChangedTargets++
		result.AppliedFilters = appendUnique(result.AppliedFilters, appliedFilters...)
	}
	if !changed {
		result.Outcome, result.SkipReason = "skipped", "no_savings"
		return result
	}
	beforeBodyTokens, errBefore := e.countBody(opts.Model, body)
	afterBodyTokens, errAfter := e.countBody(opts.Model, working)
	if errBefore == nil {
		result.BeforeTokens = beforeBodyTokens
	}
	if errAfter == nil {
		result.AfterTokens = afterBodyTokens
	}
	if errBefore != nil || errAfter != nil || !gjson.ValidBytes(working) || !utf8JSONStrings(working) || len(working) >= len(body) || afterBodyTokens >= beforeBodyTokens {
		result.Outcome, result.SkipReason = "fallback", "integrity_or_inflation"
		result.AfterBytes = len(body)
		return result
	}
	if result.Mode == ModeObserve {
		result.Outcome, result.SkipReason = "observed", "observe_only"
		result.AfterBytes = len(working)
		return result
	}
	result.Body = working
	result.Applied = true
	result.AfterBytes = len(working)
	result.Outcome = "applied"
	return result
}

// Compress is a descriptive alias for Prepare used by non-gateway callers
// such as preview and benchmark endpoints.
func (e *Engine) Compress(ctx context.Context, body []byte, opts Options) Result {
	return e.Prepare(ctx, body, opts)
}

func (e *Engine) transformTarget(ctx context.Context, target Target, opts Options) (before, after int, text string, ids []string) {
	text = target.Text
	var err error
	before, err = e.count(opts.Model, text)
	if err != nil {
		return 0, 0, target.Text, nil
	}
	filters := e.filters
	if len(opts.Filters) > 0 {
		if compiled, err := compileFilters(sortFilters(opts.Filters)); err == nil {
			filters = compiled
		}
	}
	intensity := opts.Intensity
	if intensity == "" {
		intensity = e.config.Intensity
	}
	for _, filter := range filters {
		if err := ctx.Err(); err != nil {
			return before, before, target.Text, nil
		}
		if !filter.matches(target) {
			continue
		}
		candidate, modified := applyFilterText(text, filter, intensity)
		if modified {
			text = candidate
			ids = append(ids, filter.ID)
		}
	}
	after, err = e.count(opts.Model, text)
	if err != nil {
		return before, before, target.Text, nil
	}
	return before, after, text, ids
}

func (e *Engine) countBody(model string, body []byte) (int, error) {
	return e.count(model, string(body))
}

func (e *Engine) count(model, text string) (int, error) {
	// Use both common encodings for every provider/model. This is conservative
	// for unknown model names and prevents an encoding-specific token increase.
	a, b, err := e.tokens.ConservativeCount(model, text)
	if err != nil {
		return 0, err
	}
	if a > b {
		return a, nil
	}
	return b, nil
}

func appendUnique(dst []string, values ...string) []string {
	seen := make(map[string]struct{}, len(dst)+len(values))
	for _, v := range dst {
		seen[v] = struct{}{}
	}
	for _, v := range values {
		if v == "" {
			continue
		}
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		dst = append(dst, v)
	}
	return dst
}

// utf8JSONStrings validates the body through encoding/json without retaining a
// decoded object. This catches malformed surrogate/UTF-8 output before patching.
func utf8JSONStrings(body []byte) bool {
	var value any
	return json.Unmarshal(body, &value) == nil
}
