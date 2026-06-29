package service

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"math"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/anthropictokenizer"
	kiropkg "github.com/Wei-Shaw/sub2api/internal/pkg/kiro"
)

const (
	kiroCacheDefaultTTL          = 5 * time.Minute
	kiroCacheOneHourTTL          = time.Hour
	kiroCacheMaxSupportedTTL     = time.Hour
	kiroTokensPerTool            = 150
	kiroTokensPerMessage         = 4
	kiroCacheMinTokensDefault    = 1024
	kiroCacheMinTokensOpus       = 4096
	kiroCacheMinTokensHaiku3     = 2048
	kiroCachePrefixLookbackLimit = 10
)

type kiroCacheEmulationUsage struct {
	InputTokens                int
	CacheReadInputTokens       int
	CacheCreationInputTokens   int
	CacheCreation5mInputTokens int
	CacheCreation1hInputTokens int
}

type kiroCacheEntry struct {
	tokens    int
	ttl       time.Duration
	expiresAt time.Time
}

type kiroCacheTracker struct {
	mu      sync.Mutex
	entries map[uint64]map[[32]byte]kiroCacheEntry
}

var globalKiroCacheTracker = &kiroCacheTracker{entries: make(map[uint64]map[[32]byte]kiroCacheEntry)}

func (s *GatewayService) buildKiroCacheEmulationUsage(account *Account, group *Group, body []byte, model string, inputTokens int) *kiroCacheEmulationUsage {
	NormalizeGroupRuntimeFields(group)
	if group == nil || !group.EffectiveKiroCacheEmulationEnabled() || account == nil || account.ID <= 0 || len(body) == 0 {
		return nil
	}
	profile, ok := buildKiroCacheProfile(body, model, inputTokens)
	if !ok {
		return nil
	}
	cacheKey := kiroCacheCredentialKey(account)
	if cacheKey == 0 {
		return nil
	}
	result := globalKiroCacheTracker.compute(cacheKey, profile)
	globalKiroCacheTracker.update(cacheKey, profile)
	ratio := group.EffectiveKiroCacheEmulationRatio()
	result.CacheReadInputTokens = scaleKiroCacheTokens(result.CacheReadInputTokens, ratio)
	result.CacheCreationInputTokens = scaleKiroCacheTokens(result.CacheCreationInputTokens, ratio)
	result.CacheCreation5mInputTokens = scaleKiroCacheTokens(result.CacheCreation5mInputTokens, ratio)
	result.CacheCreation1hInputTokens = scaleKiroCacheTokens(result.CacheCreation1hInputTokens, ratio)
	result.InputTokens = inputTokens - result.CacheReadInputTokens - result.CacheCreationInputTokens
	if result.InputTokens < 0 {
		result.InputTokens = 0
	}
	if result.CacheReadInputTokens == 0 && result.CacheCreationInputTokens == 0 {
		return nil
	}
	return result
}

func scaleKiroCacheTokens(tokens int, ratio float64) int {
	if tokens <= 0 || ratio <= 0 {
		return 0
	}
	if ratio >= 1 {
		return tokens
	}
	return int(math.Round(float64(tokens) * ratio))
}

type kiroCacheProfile struct {
	totalInputTokens int
	minCacheable     int
	blocks           []kiroCacheBlock
	breakpoints      []kiroCacheBreakpoint
}

type kiroCacheBlock struct {
	prefixFingerprint [32]byte
	cumulativeTokens  int
}

type kiroCacheBreakpoint struct {
	blockIndex int
	ttl        time.Duration
}

type kiroResolvedBreakpoint struct {
	blockIndex       int
	cumulativeTokens int
	ttl              time.Duration
}

type kiroPendingBlock struct {
	value         any
	tokens        int
	breakpointTTL *time.Duration
	messageIndex  *int
	isMessageEnd  bool
}

func buildKiroCacheProfile(body []byte, model string, inputTokens int) (*kiroCacheProfile, bool) {
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, false
	}
	blocks := flattenKiroCacheBlocks(payload)
	if len(blocks) == 0 {
		return nil, false
	}
	totalTokens := inputTokens
	if totalTokens <= 0 {
		totalTokens = countKiroInputTokensFromPayload(payload)
	}
	prelude, err := canonicalJSON(map[string]any{
		"model":       payload["model"],
		"tool_choice": payload["tool_choice"],
	})
	if err != nil {
		return nil, false
	}
	prefixState := make([]byte, 8+len(prelude))
	binary.BigEndian.PutUint64(prefixState[:8], uint64(len(prelude)))
	copy(prefixState[8:], prelude)

	profile := &kiroCacheProfile{totalInputTokens: max(totalTokens, 0), minCacheable: kiroMinimumCacheableTokens(model)}
	cumulativeTokens := 0
	var activeTTL *time.Duration
	seenBreakpoints := make(map[int]struct{})
	for index, block := range blocks {
		cumulativeTokens += max(block.tokens, 0)
		blockJSON, err := canonicalJSON(block.value)
		if err != nil {
			return nil, false
		}
		blockHash := sha256.Sum256(blockJSON)
		h := sha256.New()
		_, _ = h.Write(prefixState)
		_, _ = h.Write(blockHash[:])
		prefixFingerprint := [32]byte(h.Sum(nil))
		prefixState = prefixFingerprint[:]
		profile.blocks = append(profile.blocks, kiroCacheBlock{prefixFingerprint: prefixFingerprint, cumulativeTokens: cumulativeTokens})

		if block.breakpointTTL != nil {
			ttl := minDuration(*block.breakpointTTL, kiroCacheMaxSupportedTTL)
			activeTTL = &ttl
			if _, ok := seenBreakpoints[index]; !ok {
				profile.breakpoints = append(profile.breakpoints, kiroCacheBreakpoint{blockIndex: index, ttl: ttl})
				seenBreakpoints[index] = struct{}{}
			}
		}
		if block.isMessageEnd && block.messageIndex != nil && activeTTL != nil {
			if _, ok := seenBreakpoints[index]; !ok {
				profile.breakpoints = append(profile.breakpoints, kiroCacheBreakpoint{blockIndex: index, ttl: *activeTTL})
				seenBreakpoints[index] = struct{}{}
			}
		}
	}
	if profile.lastCacheableBreakpoint() == nil {
		return nil, false
	}
	return profile, true
}

func flattenKiroCacheBlocks(payload map[string]any) []kiroPendingBlock {
	var blocks []kiroPendingBlock
	if tools, ok := payload["tools"].([]any); ok {
		for toolIndex, tool := range tools {
			value := stripKiroCacheControl(tool)
			blocks = append(blocks, kiroPendingBlock{
				value:  map[string]any{"kind": "tool", "tool_index": toolIndex, "tool": value},
				tokens: kiroTokensPerTool, breakpointTTL: extractKiroCacheTTL(tool),
			})
		}
	}
	for systemIndex, systemBlock := range normalizeKiroSystemBlocks(payload["system"]) {
		value := stripKiroCacheControl(systemBlock)
		canonicalizeKiroSystemBlock(value)
		blocks = append(blocks, kiroPendingBlock{
			value:  map[string]any{"kind": "system", "system_index": systemIndex, "block": value},
			tokens: countKiroSystemBlockTokens(systemBlock), breakpointTTL: extractKiroCacheTTL(systemBlock),
		})
	}
	messages, _ := payload["messages"].([]any)
	for messageIndex, rawMessage := range messages {
		message, _ := rawMessage.(map[string]any)
		role, _ := message["role"].(string)
		content := message["content"]
		switch typed := content.(type) {
		case string:
			mi := messageIndex
			block := map[string]any{"type": "text", "text": typed}
			blocks = append(blocks, kiroPendingBlock{
				value:  map[string]any{"kind": "message", "message_index": messageIndex, "role": role, "block_index": 0, "block": block},
				tokens: countKiroMessageContentTokens(block), messageIndex: &mi, isMessageEnd: true,
			})
		case []any:
			lastBlockIndex := len(typed) - 1
			for blockIndex, rawBlock := range typed {
				mi := messageIndex
				value := stripKiroCacheControl(rawBlock)
				blocks = append(blocks, kiroPendingBlock{
					value:  map[string]any{"kind": "message", "message_index": messageIndex, "role": role, "block_index": blockIndex, "block": value},
					tokens: countKiroMessageContentTokens(rawBlock), breakpointTTL: extractKiroCacheTTL(rawBlock), messageIndex: &mi, isMessageEnd: blockIndex == lastBlockIndex,
				})
			}
		}
	}
	return blocks
}

func normalizeKiroSystemBlocks(system any) []any {
	switch typed := system.(type) {
	case nil:
		return nil
	case string:
		return []any{map[string]any{"type": "text", "text": typed}}
	case []any:
		return typed
	default:
		return []any{typed}
	}
}

func canonicalizeKiroSystemBlock(value any) {
	obj, ok := value.(map[string]any)
	if !ok {
		return
	}
	blockType, _ := obj["type"].(string)
	if blockType != "" && blockType != "text" {
		return
	}
	text, _ := obj["text"].(string)
	if strings.HasPrefix(text, "x-anthropic-billing-header:") {
		obj["text"] = "__anthropic_billing_header__"
	}
}

func extractKiroCacheTTL(value any) *time.Duration {
	obj, ok := value.(map[string]any)
	if !ok {
		return nil
	}
	cc, ok := obj["cache_control"].(map[string]any)
	if !ok || !strings.EqualFold(strings.TrimSpace(kiroCacheAsString(cc["type"])), "ephemeral") {
		return nil
	}
	ttl := kiroCacheDefaultTTL
	if strings.EqualFold(strings.TrimSpace(kiroCacheAsString(cc["ttl"])), "1h") {
		ttl = kiroCacheOneHourTTL
	}
	return &ttl
}

func (p *kiroCacheProfile) cacheableBreakpoints() []kiroResolvedBreakpoint {
	if p == nil {
		return nil
	}
	resolved := make([]kiroResolvedBreakpoint, 0, len(p.breakpoints))
	for _, breakpoint := range p.breakpoints {
		if breakpoint.blockIndex < 0 || breakpoint.blockIndex >= len(p.blocks) {
			continue
		}
		block := p.blocks[breakpoint.blockIndex]
		if block.cumulativeTokens < p.minCacheable {
			continue
		}
		resolved = append(resolved, kiroResolvedBreakpoint{blockIndex: breakpoint.blockIndex, cumulativeTokens: block.cumulativeTokens, ttl: breakpoint.ttl})
	}
	return resolved
}

func (p *kiroCacheProfile) lastCacheableBreakpoint() *kiroResolvedBreakpoint {
	breakpoints := p.cacheableBreakpoints()
	if len(breakpoints) == 0 {
		return nil
	}
	last := breakpoints[len(breakpoints)-1]
	return &last
}

func (t *kiroCacheTracker) compute(cacheKey uint64, profile *kiroCacheProfile) *kiroCacheEmulationUsage {
	out := &kiroCacheEmulationUsage{}
	if t == nil || profile == nil || cacheKey == 0 {
		return out
	}
	lastBreakpoint := profile.lastCacheableBreakpoint()
	if lastBreakpoint == nil {
		return out
	}
	lastBreakpointTokens := min(lastBreakpoint.cumulativeTokens, profile.totalInputTokens)
	now := time.Now()
	t.mu.Lock()
	defer t.mu.Unlock()
	t.pruneLocked(now)

	matchedTokens := 0
	if accountEntries := t.entries[cacheKey]; accountEntries != nil {
		breakpoints := profile.cacheableBreakpoints()
		for i, seen := len(breakpoints)-1, 0; i >= 0 && seen < kiroCachePrefixLookbackLimit; i, seen = i-1, seen+1 {
			breakpoint := breakpoints[i]
			candidate := profile.blocks[breakpoint.blockIndex]
			entry, ok := accountEntries[candidate.prefixFingerprint]
			if !ok || !entry.expiresAt.After(now) {
				continue
			}
			entry.expiresAt = now.Add(entry.ttl)
			accountEntries[candidate.prefixFingerprint] = entry
			matchedTokens = min(breakpoint.cumulativeTokens, profile.totalInputTokens)
			break
		}
	}
	newTokens := max(lastBreakpointTokens-matchedTokens, 0)
	out.CacheReadInputTokens = max(matchedTokens, 0)
	out.CacheCreationInputTokens = newTokens
	out.CacheCreation5mInputTokens, out.CacheCreation1hInputTokens = profile.ttlBreakdown(matchedTokens)
	return out
}

func (p *kiroCacheProfile) ttlBreakdown(matchedTokens int) (int, int) {
	lastBreakpoint := p.lastCacheableBreakpoint()
	if lastBreakpoint == nil {
		return 0, 0
	}
	newTokens := max(min(lastBreakpoint.cumulativeTokens, p.totalInputTokens)-matchedTokens, 0)
	if newTokens == 0 {
		return 0, 0
	}
	if lastBreakpoint.ttl >= kiroCacheOneHourTTL {
		return 0, newTokens
	}
	return newTokens, 0
}

func (t *kiroCacheTracker) update(cacheKey uint64, profile *kiroCacheProfile) {
	if t == nil || profile == nil || cacheKey == 0 {
		return
	}
	now := time.Now()
	t.mu.Lock()
	defer t.mu.Unlock()
	t.pruneLocked(now)
	accountEntries := t.entries[cacheKey]
	if accountEntries == nil {
		accountEntries = make(map[[32]byte]kiroCacheEntry)
		t.entries[cacheKey] = accountEntries
	}
	for _, breakpoint := range profile.cacheableBreakpoints() {
		block := profile.blocks[breakpoint.blockIndex]
		expiresAt := now.Add(breakpoint.ttl)
		entry, ok := accountEntries[block.prefixFingerprint]
		if ok {
			entry.tokens = max(entry.tokens, block.cumulativeTokens)
			entry.ttl = maxDuration(entry.ttl, breakpoint.ttl)
			if expiresAt.After(entry.expiresAt) {
				entry.expiresAt = expiresAt
			}
			accountEntries[block.prefixFingerprint] = entry
			continue
		}
		accountEntries[block.prefixFingerprint] = kiroCacheEntry{tokens: block.cumulativeTokens, ttl: breakpoint.ttl, expiresAt: expiresAt}
	}
}

func (t *kiroCacheTracker) pruneLocked(now time.Time) {
	for cacheKey, accountEntries := range t.entries {
		for fp, entry := range accountEntries {
			if !entry.expiresAt.After(now) {
				delete(accountEntries, fp)
			}
		}
		if len(accountEntries) == 0 {
			delete(t.entries, cacheKey)
		}
	}
}

func kiroCacheCredentialKey(account *Account) uint64 {
	stableKey := strings.TrimSpace(kiroCacheCredentialIdentity(account))
	if stableKey == "" {
		return 0
	}
	h := fnv.New64a()
	_, _ = h.Write([]byte(stableKey))
	return h.Sum64()
}

func kiroCacheCredentialIdentity(account *Account) string {
	if account == nil {
		return ""
	}
	parts := make([]string, 0, 8)
	for _, key := range []string{"client_id_hash", "client_id", "refresh_token", "profile_arn", "kiro_api_key", "kiroApiKey", "api_key"} {
		if value := strings.TrimSpace(account.GetCredential(key)); value != "" {
			parts = append(parts, key+":"+value)
		}
	}
	if len(parts) == 0 && account.ID > 0 {
		parts = append(parts, "account:"+fmt.Sprint(account.ID))
	}
	return strings.Join(parts, "|")
}

func kiroMinimumCacheableTokens(model string) int {
	m := strings.ToLower(model)
	switch {
	case strings.Contains(m, "opus"):
		return kiroCacheMinTokensOpus
	case strings.Contains(m, "haiku-3") || strings.Contains(m, "haiku_3"):
		return kiroCacheMinTokensHaiku3
	default:
		return kiroCacheMinTokensDefault
	}
}

func stripKiroCacheControl(v any) any {
	switch x := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(x))
		for k, child := range x {
			if k == "cache_control" {
				continue
			}
			out[k] = stripKiroCacheControl(child)
		}
		return out
	case []any:
		out := make([]any, len(x))
		for i, child := range x {
			out[i] = stripKiroCacheControl(child)
		}
		return out
	default:
		return v
	}
}

func countKiroInputTokensFromPayload(payload map[string]any) int {
	if payload == nil {
		return 1
	}
	tokens := 0
	for _, block := range normalizeKiroSystemBlocks(payload["system"]) {
		tokens += countKiroSystemBlockTokens(block)
	}
	messages, _ := payload["messages"].([]any)
	if len(messages) > 0 {
		canonical, err := canonicalJSON(messages)
		if err == nil {
			tokens += anthropictokenizer.CountTokens(string(canonical))
		}
		tokens += len(messages) * kiroTokensPerMessage
	}
	if tools, ok := payload["tools"].([]any); ok {
		tokens += len(tools) * kiroTokensPerTool
	}
	return max(tokens, 1)
}

func countKiroSystemBlockTokens(value any) int {
	switch typed := value.(type) {
	case string:
		return anthropictokenizer.CountTokens(typed)
	case map[string]any:
		if text, ok := typed["text"].(string); ok {
			return anthropictokenizer.CountTokens(text)
		}
		return 0
	default:
		return 0
	}
}

func countKiroMessageContentTokens(value any) int {
	switch typed := value.(type) {
	case nil:
		return 0
	case string:
		return anthropictokenizer.CountTokens(typed)
	case []any:
		total := 0
		for _, item := range typed {
			total += countKiroMessageContentTokens(item)
		}
		return total
	case map[string]any:
		if text, ok := typed["text"].(string); ok {
			return anthropictokenizer.CountTokens(text)
		}
		if thinking, ok := typed["thinking"].(string); ok {
			return anthropictokenizer.CountTokens(thinking)
		}
		if input, ok := typed["input"]; ok {
			return countKiroSerializedValueTokens(input)
		}
		if content, ok := typed["content"]; ok {
			return countKiroMessageContentTokens(content)
		}
		return 0
	default:
		return 0
	}
}

func countKiroSerializedValueTokens(value any) int {
	canonical, err := canonicalJSON(value)
	if err != nil {
		return 0
	}
	return anthropictokenizer.CountTokens(string(canonical))
}

func canonicalJSON(v any) ([]byte, error) {
	var buf bytes.Buffer
	if err := writeCanonicalJSON(&buf, v); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func writeCanonicalJSON(buf *bytes.Buffer, v any) error {
	switch x := v.(type) {
	case map[string]any:
		keys := make([]string, 0, len(x))
		for k := range x {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		_ = buf.WriteByte('{')
		for i, k := range keys {
			if i > 0 {
				_ = buf.WriteByte(',')
			}
			kb, _ := json.Marshal(k)
			_, _ = buf.Write(kb)
			_ = buf.WriteByte(':')
			if err := writeCanonicalJSON(buf, x[k]); err != nil {
				return err
			}
		}
		_ = buf.WriteByte('}')
		return nil
	case []any:
		_ = buf.WriteByte('[')
		for i, child := range x {
			if i > 0 {
				_ = buf.WriteByte(',')
			}
			if err := writeCanonicalJSON(buf, child); err != nil {
				return err
			}
		}
		_ = buf.WriteByte(']')
		return nil
	default:
		b, err := json.Marshal(v)
		if err != nil {
			return err
		}
		_, _ = buf.Write(b)
		return nil
	}
}

func kiroCacheAsString(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

func minDuration(a, b time.Duration) time.Duration {
	if a < b {
		return a
	}
	return b
}

func maxDuration(a, b time.Duration) time.Duration {
	if a > b {
		return a
	}
	return b
}

func (u *kiroCacheEmulationUsage) toKiroUsage() *kiropkg.Usage {
	if u == nil {
		return nil
	}
	return &kiropkg.Usage{
		InputTokens:                u.InputTokens,
		CacheReadInputTokens:       u.CacheReadInputTokens,
		CacheCreationInputTokens:   u.CacheCreationInputTokens,
		CacheCreation5mInputTokens: u.CacheCreation5mInputTokens,
		CacheCreation1hInputTokens: u.CacheCreation1hInputTokens,
	}
}
