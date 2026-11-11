package service

import (
	"fmt"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/openai"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// Explicit prompt-cache hints — OpenAI's `prompt_cache_breakpoint` on content
// parts plus the top-level `prompt_cache_options` — are documented for GPT-5.6
// and later, but whether an upstream accepts them is decided by the
// implementation behind the account, not by the model slug. The same
// gpt-6.1-sol answers `prompt_cache_breakpoint is not supported on this model`
// behind a Codex OAuth upstream while the public API accepts the field.
//
// Forwarding a hint the upstream does not implement fails the whole request
// with a 400. Dropping it only gives up explicit cache-write control: implicit
// caching keeps matching through prompt_cache_key, which the Chat Completions
// path injects on its own. Stripping by default is therefore the cheap side of
// that trade-off, and accounts that are known to terminate at the official API
// keep their hints.
//
// Chat Completions bodies reach an upstream in two shapes — converted to
// Responses (`input[].content[]`) or forwarded as-is to a Chat Completions
// upstream (`messages[].content[]`). Both are handled here, and only at the
// exact structural positions OpenAI defines: tool schemas and function_call
// argument strings may legitimately contain a key with the same name.
//
// See https://github.com/Wei-Shaw/sub2api/issues/7546.

// promptCacheHintsCredentialKey lets one account override the policy derived by
// resolvePromptCacheHintPolicy: "allow" keeps explicit hints, "strip" removes
// them. Unknown values fall back to the derived policy.
const promptCacheHintsCredentialKey = "prompt_cache_hints"

// promptCacheHintPolicy says which explicit prompt-cache controls may be
// forwarded to the upstream a request is mapped onto.
type promptCacheHintPolicy struct {
	StripBreakpoints bool
	StripOptions     bool
}

func (p promptCacheHintPolicy) stripsHints() bool { return p.StripBreakpoints || p.StripOptions }

func resolvePromptCacheHintPolicy(account *Account, upstreamModel string) promptCacheHintPolicy {
	stripAll := promptCacheHintPolicy{StripBreakpoints: true, StripOptions: true}
	if account == nil {
		return stripAll
	}
	switch strings.ToLower(strings.TrimSpace(account.GetCredential(promptCacheHintsCredentialKey))) {
	case "allow", "keep", "preserve", "passthrough":
		return promptCacheHintPolicy{}
	case "strip", "drop", "off":
		return stripAll
	}
	// Subscription-backed Codex upstreams do not implement explicit caching at
	// all, and they are the accounts that answer chat-shaped requests today.
	if account.UsesOpenAICodexProtocol() {
		return stripAll
	}
	if !account.IsOpenAIApiKey() {
		// Anything else (multi-protocol relays, upstream proxy accounts) proxies
		// to an implementation we cannot vouch for.
		return stripAll
	}
	baseURL := strings.TrimSpace(account.GetCredential("base_url"))
	if baseURL == "" {
		baseURL = account.GetOpenAIBaseURL()
	}
	if !isOfficialOpenAIModelsBaseURL(baseURL) {
		return stripAll
	}
	if !modelSupportsExplicitPromptCache(upstreamModel) {
		return stripAll
	}
	return promptCacheHintPolicy{}
}

// modelSupportsExplicitPromptCache reports whether OpenAI documents explicit
// prompt caching for the model slug: GPT-5.6 and later.
func modelSupportsExplicitPromptCache(model string) bool {
	canonical := openai.CanonicalizeOpenAIModelAliasSpelling(model)
	if canonical == "" {
		return false
	}
	return strings.HasPrefix(canonical, "gpt-5.6") || strings.HasPrefix(canonical, "gpt-6")
}

// promptCacheSanitizeReport describes what sanitizePromptCacheHints removed so
// callers can log it and, later, count it.
type promptCacheSanitizeReport struct {
	Breakpoints int
	Options     bool
}

func (r promptCacheSanitizeReport) changed() bool { return r.Breakpoints > 0 || r.Options }

// sanitizePromptCacheHints removes the explicit prompt-cache controls the
// policy forbids, leaving prompt_cache_key and every same-named key inside tool
// schemas or argument strings untouched.
func sanitizePromptCacheHints(body []byte, policy promptCacheHintPolicy) ([]byte, promptCacheSanitizeReport, error) {
	report := promptCacheSanitizeReport{}
	if len(body) == 0 || !policy.stripsHints() || !gjson.ValidBytes(body) {
		return body, report, nil
	}

	out := body
	if policy.StripBreakpoints {
		next, removed, err := removePromptCacheBreakpoints(out)
		if err != nil {
			return nil, report, err
		}
		out, report.Breakpoints = next, removed
	}

	// `prompt_cache_options.mode=explicit` turns the automatic breakpoint off.
	// Leaving it behind after dropping the explicit breakpoints would silently
	// disable caching for the request instead of failing it, which is worse
	// than the 400 this change fixes.
	stripOptions := policy.StripOptions || (report.Breakpoints > 0 && promptCacheOptionsMode(out) == "explicit")
	if stripOptions && gjson.GetBytes(out, "prompt_cache_options").Exists() {
		next, err := sjson.DeleteBytes(out, "prompt_cache_options")
		if err != nil {
			return nil, report, fmt.Errorf("delete prompt_cache_options: %w", err)
		}
		out, report.Options = next, true
	}
	return out, report, nil
}

func removePromptCacheBreakpoints(body []byte) ([]byte, int, error) {
	out := body
	removed := 0

	// A top-level hint is not part of the documented schema, but relays add it;
	// /v1/responses already retries without it, so drop it up front too.
	next, deleted, err := deletePromptCacheBreakpointAt(out, "prompt_cache_breakpoint")
	if err != nil {
		return nil, removed, err
	}
	out, removed = next, removed+deleted

	for _, root := range []string{"input", "messages"} {
		items := gjson.GetBytes(body, root)
		if !items.IsArray() {
			continue
		}
		for itemIndex := range items.Array() {
			itemPath := fmt.Sprintf("%s.%d", root, itemIndex)
			if !gjson.GetBytes(body, itemPath).IsObject() {
				continue
			}
			next, deleted, err := deletePromptCacheBreakpointAt(out, itemPath+".prompt_cache_breakpoint")
			if err != nil {
				return nil, removed, err
			}
			out, removed = next, removed+deleted

			content := gjson.GetBytes(body, itemPath+".content")
			if !content.IsArray() {
				continue
			}
			for partIndex := range content.Array() {
				partPath := fmt.Sprintf("%s.content.%d.prompt_cache_breakpoint", itemPath, partIndex)
				next, deleted, err := deletePromptCacheBreakpointAt(out, partPath)
				if err != nil {
					return nil, removed, err
				}
				out, removed = next, removed+deleted
			}
		}
	}
	return out, removed, nil
}

func deletePromptCacheBreakpointAt(body []byte, path string) ([]byte, int, error) {
	if !gjson.GetBytes(body, path).Exists() {
		return body, 0, nil
	}
	next, err := sjson.DeleteBytes(body, path)
	if err != nil {
		return nil, 0, fmt.Errorf("delete %s: %w", path, err)
	}
	return next, 1, nil
}

func promptCacheOptionsMode(body []byte) string {
	return strings.ToLower(strings.TrimSpace(gjson.GetBytes(body, "prompt_cache_options.mode").String()))
}
