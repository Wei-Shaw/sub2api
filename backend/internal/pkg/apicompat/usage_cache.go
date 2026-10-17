package apicompat

import (
	"encoding/json"

	"github.com/tidwall/gjson"
)

// CachedTokensFromUsage resolves the cache-read counter of an OpenAI-shaped
// usage object. It is shared by raw gateway accounting and typed protocol
// conversion so clients see the count that is billed. Canonical nested
// counters, including explicit zero or null, take priority; otherwise the
// first positive top-level relay alias wins. The bool reports whether any
// counter was present.
func CachedTokensFromUsage(value gjson.Result) (int, bool) {
	for _, path := range []string{"input_tokens_details.cached_tokens", "prompt_tokens_details.cached_tokens"} {
		if counter := value.Get(path); counter.Exists() {
			return max(int(counter.Int()), 0), true
		}
	}
	present := false
	for _, path := range []string{"cache_read_input_tokens", "cache_read_tokens", "cached_tokens"} {
		counter := value.Get(path)
		present = present || counter.Exists()
		if n := int(counter.Int()); n > 0 {
			return n, true
		}
	}
	return 0, present
}

// cacheCreationSource records which counter won, so typed usage can keep the
// upstream spelling of the winning counter.
type cacheCreationSource uint8

const (
	cacheCreationAbsent cacheCreationSource = iota
	cacheCreationTopLevel
	cacheCreationNestedWrite
	cacheCreationNestedCreation
)

// CacheCreationTokensFromUsage resolves the cache-write counter of an
// OpenAI-shaped usage object for raw gateway accounting and typed protocol
// conversion. Canonical nested counters, including explicit zero or null, take
// priority; otherwise the first positive top-level relay alias wins.
func CacheCreationTokensFromUsage(value gjson.Result) int {
	tokens, _ := cacheCreationCounterFromUsage(value)
	return tokens
}

func cacheCreationCounterFromUsage(value gjson.Result) (int, cacheCreationSource) {
	for _, nested := range []struct {
		path   string
		source cacheCreationSource
	}{
		{"input_tokens_details.cache_write_tokens", cacheCreationNestedWrite},
		{"prompt_tokens_details.cache_write_tokens", cacheCreationNestedWrite},
		{"input_tokens_details.cache_creation_tokens", cacheCreationNestedCreation},
		{"prompt_tokens_details.cache_creation_tokens", cacheCreationNestedCreation},
	} {
		if counter := value.Get(nested.path); counter.Exists() {
			return max(int(counter.Int()), 0), nested.source
		}
	}
	source := cacheCreationAbsent
	for _, path := range []string{"cache_write_tokens", "cache_creation_input_tokens", "cache_write_input_tokens", "cache_creation_tokens"} {
		counter := value.Get(path)
		if counter.Exists() {
			source = cacheCreationTopLevel
		}
		if n := int(counter.Int()); n > 0 {
			return n, cacheCreationTopLevel
		}
	}
	return 0, source
}

// keepWinningSpelling stores the canonical counter under the spelling that won
// and clears the other, so a later conversion cannot revive a lower-priority value.
func keepWinningSpelling(creation, write *int, tokens int, source cacheCreationSource) {
	*creation, *write = tokens, 0
	if source == cacheCreationNestedWrite {
		*creation, *write = 0, tokens
	}
}

// applyCacheUsage makes typed Responses usage report the cache counters that
// raw accounting bills.
func (u *ResponsesUsage) applyCacheUsage(data []byte) {
	usage := gjson.ParseBytes(data)
	if cached, present := CachedTokensFromUsage(usage); present && (cached > 0 || u.InputTokensDetails != nil) {
		if u.InputTokensDetails == nil {
			u.InputTokensDetails = &ResponsesInputTokensDetails{}
		}
		u.InputTokensDetails.CachedTokens = cached
	}
	tokens, source := cacheCreationCounterFromUsage(usage)
	if source == cacheCreationAbsent {
		return
	}
	u.CacheCreationInputTokens = tokens
	if source == cacheCreationTopLevel {
		return
	}
	if u.InputTokensDetails == nil {
		u.InputTokensDetails = &ResponsesInputTokensDetails{}
	}
	keepWinningSpelling(&u.InputTokensDetails.CacheCreationTokens, &u.InputTokensDetails.CacheWriteTokens, tokens, source)
}

// UnmarshalJSON accepts the top-level cache aliases third-party Chat
// Completions relays report, using the same resolution as raw accounting.
func (u *ChatUsage) UnmarshalJSON(data []byte) error {
	type chatUsageAlias ChatUsage
	var decoded chatUsageAlias
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	*u = ChatUsage(decoded)
	usage := gjson.ParseBytes(data)
	if cached, present := CachedTokensFromUsage(usage); present && (cached > 0 || u.PromptTokensDetails != nil) {
		if u.PromptTokensDetails == nil {
			u.PromptTokensDetails = &ChatTokenDetails{}
		}
		u.PromptTokensDetails.CachedTokens = cached
	}
	tokens, source := cacheCreationCounterFromUsage(usage)
	if source == cacheCreationAbsent || (tokens == 0 && u.PromptTokensDetails == nil) {
		return nil
	}
	if u.PromptTokensDetails == nil {
		u.PromptTokensDetails = &ChatTokenDetails{}
	}
	keepWinningSpelling(&u.PromptTokensDetails.CacheCreationTokens, &u.PromptTokensDetails.CacheWriteTokens, tokens, source)
	return nil
}
