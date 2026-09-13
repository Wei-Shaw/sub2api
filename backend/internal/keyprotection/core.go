// Package keyprotection protects supported credentials in model-facing content.
// State is request-local; its sensitive entries must never be logged or sent to a model.
package keyprotection

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
)

const (
	ModeReversible    = "reversible"
	ModeRedact        = "redact"
	ScopeTextAndTools = "text_and_tools"
	ScopeToolsOnly    = "tools_only"
)

// Rule defines an administrator-supplied RE2 pattern. The complete match is
// replaced; names are diagnostic categories and must not contain credentials.
type Rule struct {
	Name    string `json:"name"`
	Pattern string `json:"pattern"`
}

type Config struct {
	Enabled      bool     `json:"enabled"`
	UserIDs      []int64  `json:"user_ids"`
	GroupIDs     []int64  `json:"group_ids"`
	Mode         string   `json:"mode"`
	RestoreScope string   `json:"restore_scope"`
	Rules        []string `json:"rules"`
	CustomRules  []Rule   `json:"custom_rules"`
	TTLSeconds   int      `json:"ttl_seconds"`
	MaxMappings  int      `json:"max_mappings"`
	MaxSessions  int      `json:"max_sessions"`
}

func DefaultConfig() Config {
	return Config{Mode: ModeReversible, RestoreScope: ScopeTextAndTools, TTLSeconds: 3600, MaxMappings: 256, MaxSessions: 10000}
}

// Normalized returns a detached configuration with defaults for omitted fields.
func (c Config) Normalized() Config { return c.normalized() }

func (c Config) normalized() Config {
	d := DefaultConfig()
	if c.Mode == "" {
		c.Mode = d.Mode
	}
	if c.RestoreScope == "" {
		c.RestoreScope = d.RestoreScope
	}
	if c.TTLSeconds == 0 {
		c.TTLSeconds = d.TTLSeconds
	}
	if c.MaxMappings == 0 {
		c.MaxMappings = d.MaxMappings
	}
	if c.MaxSessions == 0 {
		c.MaxSessions = d.MaxSessions
	}
	c.UserIDs = append([]int64(nil), c.UserIDs...)
	c.GroupIDs = append([]int64(nil), c.GroupIDs...)
	c.Rules = append([]string(nil), c.Rules...)
	c.CustomRules = append([]Rule(nil), c.CustomRules...)
	return c
}

func (c Config) Validate() error {
	c = c.normalized()
	if c.Mode != ModeReversible && c.Mode != ModeRedact {
		return errors.New("invalid key protection mode")
	}
	if c.RestoreScope != ScopeTextAndTools && c.RestoreScope != ScopeToolsOnly {
		return errors.New("invalid key protection restore scope")
	}
	if c.TTLSeconds < 60 || c.TTLSeconds > 30*24*60*60 {
		return errors.New("key protection TTL must be between 60 and 2592000 seconds")
	}
	if c.MaxMappings < 1 || c.MaxMappings > 10000 {
		return errors.New("key protection mapping limit must be between 1 and 10000")
	}
	if c.MaxSessions < 1 || c.MaxSessions > 1000000 {
		return errors.New("key protection session limit must be between 1 and 1000000")
	}
	if len(c.UserIDs)+len(c.GroupIDs) > 100000 {
		return errors.New("too many key protection scope entries")
	}
	for _, ids := range [][]int64{c.UserIDs, c.GroupIDs} {
		for _, id := range ids {
			if id <= 0 {
				return errors.New("key protection scope IDs must be positive")
			}
		}
	}
	if len(c.CustomRules) > 32 {
		return errors.New("too many custom key protection rules")
	}
	known := make(map[string]bool)
	for _, rule := range builtinRules {
		known[rule.Name] = true
	}
	for _, name := range c.Rules {
		if !known[name] {
			return errors.New("unknown key protection rule")
		}
	}
	names := make(map[string]bool)
	for _, rule := range c.CustomRules {
		if !regexp.MustCompile(`^[a-z][a-z0-9_]{0,47}$`).MatchString(rule.Name) || known[rule.Name] || names[rule.Name] {
			return errors.New("invalid or duplicate custom key protection rule name")
		}
		names[rule.Name] = true
		if len(rule.Pattern) == 0 || len(rule.Pattern) > 2048 {
			return errors.New("invalid custom key protection pattern length")
		}
		re, err := regexp.Compile(rule.Pattern)
		if err != nil || re.MatchString("") {
			return errors.New("invalid custom key protection pattern")
		}
	}
	return nil
}

// Applies receives authenticated internal IDs only. Empty lists apply to all users;
// otherwise an explicit user or group match is sufficient.
func (c Config) Applies(userID, groupID int64) bool {
	if !c.Enabled || userID <= 0 {
		return false
	}
	if len(c.UserIDs) == 0 && len(c.GroupIDs) == 0 {
		return true
	}
	for _, id := range c.UserIDs {
		if id == userID {
			return true
		}
	}
	for _, id := range c.GroupIDs {
		if id == groupID {
			return true
		}
	}
	return false
}

const PlaceholderLength = 69
const RedactedMarker = "[REDACTED]"
const MaxSecretBytes = 1 << 20
const MaxMappingBytes = 8 << 20

var (
	ErrCapacity    = errors.New("key protection mapping capacity exceeded")
	ErrContent     = errors.New("key protection cannot safely parse this content")
	ErrUnsupported = errors.New("key protection does not support this content or protocol")
)

// State is fixed for one request, including retries. Its configuration is copied,
// and Entries returns a copy to prevent callers mutating an active mapping.
// State is not safe for concurrent mutation; finish input protection before use
// by concurrent model/response branches, which may read it concurrently.
type State struct {
	config       Config
	rules        []compiledRule
	entries      map[string]string
	reverse      map[string]string
	counts       map[string]int
	seed         []byte
	mappingBytes int
}

// Safe formatting prevents ordinary diagnostics from exposing private state.
func (s State) String() string {
	return fmt.Sprintf("keyprotection.State{mode:%s,mappings:%d}", s.config.Mode, len(s.entries))
}
func (s State) GoString() string { return s.String() }

func NewState(config Config, entries map[string]string) (*State, error) {
	seed := make([]byte, 32)
	if _, err := rand.Read(seed); err != nil {
		return nil, errors.New("key protection random source failed")
	}
	return NewStateWithSeed(config, entries, seed)
}

// NewStateWithSeed uses a purpose- and authenticated-scope-derived platform seed
// for deterministic HMAC placeholders. Only an authorized mapping permits output
// restoration; the digest itself never grants access to a stored credential.
func NewStateWithSeed(config Config, entries map[string]string, seed []byte) (*State, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}
	config = config.normalized()
	if len(seed) < 32 {
		return nil, errors.New("key protection seed must contain at least 32 bytes")
	}
	if len(entries) > config.MaxMappings {
		return nil, ErrCapacity
	}
	s := &State{config: config, rules: compileRules(config), entries: make(map[string]string), reverse: make(map[string]string), counts: make(map[string]int), seed: append([]byte(nil), seed...)}
	if config.Mode == ModeRedact {
		return s, nil
	}
	for token, secret := range entries {
		if !IsPlaceholderToken(token) || secret == "" || len(secret) > MaxSecretBytes || IsPlaceholderToken(secret) {
			return nil, errors.New("invalid key protection mapping")
		}
		if _, exists := s.reverse[secret]; exists {
			return nil, errors.New("duplicate key protection mapping")
		}
		if s.mappingBytes+len(token)+len(secret) > MaxMappingBytes {
			return nil, ErrCapacity
		}
		s.entries[token] = secret
		s.reverse[secret] = token
		s.mappingBytes += len(token) + len(secret)
	}
	return s, nil
}

func IsPlaceholderToken(token string) bool {
	if len(token) != PlaceholderLength || !strings.HasPrefix(token, "keyx_") {
		return false
	}
	for _, c := range token[5:] {
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return false
		}
	}
	return true
}

func (s *State) Config() Config { return s.config.normalized() }

func (s *State) Entries() map[string]string {
	entries := make(map[string]string, len(s.entries))
	for token, secret := range s.entries {
		entries[token] = secret
	}
	return entries
}

func (s *State) Counts() map[string]int {
	counts := make(map[string]int, len(s.counts))
	for name, count := range s.counts {
		counts[name] = count
	}
	return counts
}

func (s *State) ProtectText(text string) (string, error) {
	matches := s.matches(text)
	if len(matches) == 0 {
		return text, nil
	}
	var out strings.Builder
	last := 0
	for _, found := range matches {
		if found.start < last {
			continue
		}
		secret := text[found.start:found.end]
		if len(secret) > MaxSecretBytes {
			return "", ErrCapacity
		}
		replacement := RedactedMarker
		if s.config.Mode == ModeReversible {
			var ok bool
			replacement, ok = s.reverse[secret]
			if !ok {
				if len(s.entries) >= s.config.MaxMappings || s.mappingBytes+PlaceholderLength+len(secret) > MaxMappingBytes {
					return "", ErrCapacity
				}
				digest := hmac.New(sha256.New, s.seed)
				_, _ = digest.Write([]byte(secret))
				replacement = "keyx_" + hex.EncodeToString(digest.Sum(nil))
				if previous, collision := s.entries[replacement]; collision && previous != secret {
					return "", errors.New("key protection mapping collision")
				}
				s.entries[replacement] = secret
				s.reverse[secret] = replacement
				s.mappingBytes += PlaceholderLength + len(secret)
			}
		}
		out.WriteString(text[last:found.start])
		out.WriteString(replacement)
		last = found.end
		s.counts[found.rule]++
	}
	out.WriteString(text[last:])
	return out.String(), nil
}

// RestoreText only restores exact, complete tokens owned by this state. It does
// not consult storage or other sessions and never treats a malformed token as a
// lookup. Callers enforce the text/tool restore policy at protocol field level.
func (s *State) RestoreText(text string) string {
	if s.config.Mode != ModeReversible || len(s.entries) == 0 {
		return text
	}
	var out strings.Builder
	last, search := 0, 0
	for search < len(text) {
		at := strings.Index(text[search:], "keyx_")
		if at < 0 {
			break
		}
		start := search + at
		end := start + PlaceholderLength
		if end > len(text) {
			break
		}
		if bounded(text, start, end) {
			if secret, ok := s.entries[text[start:end]]; ok {
				out.WriteString(text[last:start])
				out.WriteString(secret)
				last = end
			}
		}
		search = start + 5
	}
	if last == 0 {
		return text
	}
	out.WriteString(text[last:])
	return out.String()
}

// Rules intentionally identify formats, not all possible passwords. No candidate
// is sent to an external validation service. Legacy unprefixed random secrets are
// deliberately omitted; custom rules can cover deployment-specific formats.
var builtinRules = []Rule{
	{Name: "openai", Pattern: `sk-(?:proj-|svcacct-)[A-Za-z0-9_-]{20,240}|sk-[A-Za-z0-9]{48}`},
	{Name: "anthropic", Pattern: `sk-ant-api[0-9]{2}-[A-Za-z0-9_-]{40,240}`},
	{Name: "github", Pattern: `gh[pousr]_[A-Za-z0-9]{36}|github_pat_[A-Za-z0-9_]{50,255}`},
	{Name: "gitlab", Pattern: `glpat-[A-Za-z0-9_-]{20,64}`},
	{Name: "google", Pattern: `AIza[A-Za-z0-9_-]{35}`},
	{Name: "stripe", Pattern: `[sr]k_(?:test|live)_[A-Za-z0-9]{24,128}`},
	{Name: "slack", Pattern: `xox[baprs]-[A-Za-z0-9-]{20,200}`},
	{Name: "huggingface", Pattern: `hf_[A-Za-z0-9]{34}`},
	{Name: "groq", Pattern: `gsk_[A-Za-z0-9]{52}`},
	{Name: "npm", Pattern: `npm_[A-Za-z0-9]{36}`},
}

func BuiltinRuleNames() []string {
	names := make([]string, 0, len(builtinRules))
	for _, rule := range builtinRules {
		names = append(names, rule.Name)
	}
	return names
}

type compiledRule struct {
	name       string
	expression *regexp.Regexp
}
type match struct {
	start, end int
	rule       string
}

func compileRules(c Config) []compiledRule {
	selected := make(map[string]bool)
	for _, name := range c.Rules {
		selected[name] = true
	}
	rules := make([]compiledRule, 0, len(builtinRules)+len(c.CustomRules))
	for _, rule := range builtinRules {
		if len(selected) == 0 || selected[rule.Name] {
			rules = append(rules, compiledRule{rule.Name, regexp.MustCompile(rule.Pattern)})
		}
	}
	for _, rule := range c.CustomRules {
		rules = append(rules, compiledRule{rule.Name, regexp.MustCompile(rule.Pattern)})
	}
	return rules
}

func tokenCharacter(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '_' || c == '-'
}

func bounded(text string, start, end int) bool {
	return (start == 0 || !tokenCharacter(text[start-1])) && (end == len(text) || !tokenCharacter(text[end]))
}

func (s *State) matches(text string) []match {
	var found []match
	for _, rule := range s.rules {
		for _, span := range rule.expression.FindAllStringIndex(text, -1) {
			if span[0] != span[1] && bounded(text, span[0], span[1]) && !IsPlaceholderToken(text[span[0]:span[1]]) {
				found = append(found, match{span[0], span[1], rule.name})
			}
		}
	}
	// Prefer the longest candidate at the same position; skip overlapping matches.
	sort.SliceStable(found, func(i, j int) bool {
		if found[i].start == found[j].start {
			return found[i].end > found[j].end
		}
		return found[i].start < found[j].start
	})
	return found
}

type protectedContextKey struct{}

// WithProtected stores only a policy flag. Sensitive mapping state deliberately
// stays outside request metadata, trace attributes and serializable contexts.
func WithProtected(ctx context.Context) context.Context {
	return context.WithValue(ctx, protectedContextKey{}, true)
}
func IsProtected(ctx context.Context) bool { v, _ := ctx.Value(protectedContextKey{}).(bool); return v }

const ProtectedGinKey = "sub2api.key_protection.active"
