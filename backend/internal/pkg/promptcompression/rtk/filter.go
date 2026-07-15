package rtk

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

type ReplaceRule struct {
	Pattern string `json:"pattern" yaml:"pattern"`
	With    string `json:"with" yaml:"with"`
}

// Filter is the stable, JSON/YAML-friendly filter DSL. Regexes are compiled
// when a filter is published; invalid filters never replace the active pack.
type Filter struct {
	ID              string        `json:"id" yaml:"id"`
	Commands        []string      `json:"commands,omitempty" yaml:"commands,omitempty"`
	ExcludeCommands []string      `json:"excludeCommands,omitempty" yaml:"excludeCommands,omitempty"`
	Patterns        []string      `json:"patterns,omitempty" yaml:"patterns,omitempty"`
	ExcludePatterns []string      `json:"excludePatterns,omitempty" yaml:"excludePatterns,omitempty"`
	MatchOutput     []string      `json:"matchOutput,omitempty" yaml:"matchOutput,omitempty"`
	OutputTypes     []string      `json:"outputTypes,omitempty" yaml:"outputTypes,omitempty"`
	MinConfidence   float64       `json:"minConfidence,omitempty" yaml:"minConfidence,omitempty"`
	StripANSI       bool          `json:"stripAnsi,omitempty" yaml:"stripAnsi,omitempty"`
	Replace         []ReplaceRule `json:"replace,omitempty" yaml:"replace,omitempty"`
	Include         []string      `json:"include,omitempty" yaml:"include,omitempty"`
	Drop            []string      `json:"drop,omitempty" yaml:"drop,omitempty"`
	Collapse        bool          `json:"collapse,omitempty" yaml:"collapse,omitempty"`
	Deduplicate     bool          `json:"deduplicate,omitempty" yaml:"deduplicate,omitempty"`
	TruncateLineAt  int           `json:"truncateLineAt,omitempty" yaml:"truncateLineAt,omitempty"`
	MaxLines        int           `json:"maxLines,omitempty" yaml:"maxLines,omitempty"`
	HeadLines       int           `json:"head,omitempty" yaml:"head,omitempty"`
	TailLines       int           `json:"tail,omitempty" yaml:"tail,omitempty"`
	OnEmpty         string        `json:"onEmpty,omitempty" yaml:"onEmpty,omitempty"`
	Preserve        []string      `json:"preserve,omitempty" yaml:"preserve,omitempty"`
	Intensity       Intensity     `json:"intensity,omitempty" yaml:"intensity,omitempty"`
}

type compiledFilter struct {
	Filter
	patterns, excludedPatterns []*regexp.Regexp
	matchOutput                []*regexp.Regexp
	include, drop, preserve    []*regexp.Regexp
	replace                    []compiledReplace
}
type compiledReplace struct {
	re   *regexp.Regexp
	with string
}

func compileFilters(filters []Filter) ([]compiledFilter, error) {
	seen := make(map[string]bool, len(filters))
	compiled := make([]compiledFilter, 0, len(filters))
	for _, f := range filters {
		f = cloneFilter(f)
		f.ID = strings.TrimSpace(f.ID)
		if f.ID == "" {
			return nil, fmt.Errorf("filter id is required")
		}
		if seen[f.ID] {
			return nil, fmt.Errorf("duplicate filter id %q", f.ID)
		}
		seen[f.ID] = true
		if f.MaxLines < 0 || f.HeadLines < 0 || f.TailLines < 0 || f.TruncateLineAt < 0 {
			return nil, fmt.Errorf("filter %q has negative limit", f.ID)
		}
		cf := compiledFilter{Filter: f}
		var err error
		cf.patterns, err = compileRegexps(f.Patterns)
		if err != nil {
			return nil, fmt.Errorf("filter %q patterns: %w", f.ID, err)
		}
		cf.excludedPatterns, err = compileRegexps(f.ExcludePatterns)
		if err != nil {
			return nil, fmt.Errorf("filter %q excludePatterns: %w", f.ID, err)
		}
		cf.matchOutput, err = compileRegexps(f.MatchOutput)
		if err != nil {
			return nil, fmt.Errorf("filter %q matchOutput: %w", f.ID, err)
		}
		cf.include, err = compileRegexps(f.Include)
		if err != nil {
			return nil, fmt.Errorf("filter %q include: %w", f.ID, err)
		}
		cf.drop, err = compileRegexps(f.Drop)
		if err != nil {
			return nil, fmt.Errorf("filter %q drop: %w", f.ID, err)
		}
		cf.preserve, err = compileRegexps(f.Preserve)
		if err != nil {
			return nil, fmt.Errorf("filter %q preserve: %w", f.ID, err)
		}
		for _, r := range f.Replace {
			re, e := regexp.Compile(r.Pattern)
			if e != nil {
				return nil, fmt.Errorf("filter %q replace: %w", f.ID, e)
			}
			cf.replace = append(cf.replace, compiledReplace{re: re, with: r.With})
		}
		compiled = append(compiled, cf)
	}
	return compiled, nil
}

func cloneFilter(f Filter) Filter {
	f.Commands = append([]string(nil), f.Commands...)
	f.ExcludeCommands = append([]string(nil), f.ExcludeCommands...)
	f.Patterns = append([]string(nil), f.Patterns...)
	f.ExcludePatterns = append([]string(nil), f.ExcludePatterns...)
	f.MatchOutput = append([]string(nil), f.MatchOutput...)
	f.OutputTypes = append([]string(nil), f.OutputTypes...)
	f.Include = append([]string(nil), f.Include...)
	f.Drop = append([]string(nil), f.Drop...)
	f.Preserve = append([]string(nil), f.Preserve...)
	f.Replace = append([]ReplaceRule(nil), f.Replace...)
	return f
}

func compileRegexps(items []string) ([]*regexp.Regexp, error) {
	result := make([]*regexp.Regexp, 0, len(items))
	for _, pattern := range items {
		re, err := regexp.Compile(pattern)
		if err != nil {
			return nil, err
		}
		result = append(result, re)
	}
	return result, nil
}

func (f compiledFilter) matches(t Target) bool {
	tool := strings.ToLower(strings.TrimSpace(t.ToolName))
	command := strings.TrimSpace(t.Command)
	if len(f.Commands) > 0 && !matchesCommand(f.Commands, tool, command) {
		return false
	}
	if len(f.ExcludeCommands) > 0 && matchesCommand(f.ExcludeCommands, tool, command) {
		return false
	}
	if len(f.OutputTypes) > 0 && t.OutputType != "" && !containsFold(f.OutputTypes, t.OutputType) {
		return false
	}
	if f.MinConfidence > 0 && t.Confidence < f.MinConfidence {
		return false
	}
	for _, p := range f.excludedPatterns {
		if p.MatchString(t.Command) || p.MatchString(t.Text) {
			return false
		}
	}
	for _, p := range f.patterns {
		if !p.MatchString(t.Command) && !p.MatchString(t.Text) {
			return false
		}
	}
	if len(f.matchOutput) > 0 {
		matched := false
		for _, p := range f.matchOutput { if p.MatchString(t.Text) { matched = true; break } }
		if !matched { return false }
	}
	if f.Intensity != "" && f.Intensity != IntensitySafe && f.Intensity != IntensityBalanced && f.Intensity != IntensityAggressive {
		return false
	}
	return true
}

func matchesCommand(patterns []string, tool, command string) bool {
	candidates := shellCommandCandidates(command)
	candidates = append(candidates, tool)
	for _, raw := range patterns {
		pattern := strings.TrimSpace(raw)
		for _, candidate := range candidates {
			if strings.EqualFold(pattern, candidate) {
				return true
			}
		}
		if re, err := regexp.Compile(normalizeRE2(pattern)); err == nil {
			for _, candidate := range candidates {
				if re.MatchString(candidate) {
					return true
				}
			}
		}
	}
	return false
}

// shellCommandCandidates splits only on unquoted shell separators. It does not
// execute or evaluate shell syntax; candidates are solely used for filter
// matching. Both the full segment and its first command token are returned.
func shellCommandCandidates(command string) []string {
	var result []string
	start := 0
	quote := byte(0)
	escaped := false
	flush := func(end int) {
		segment := strings.TrimSpace(command[start:end])
		if segment == "" {
			return
		}
		// Keep a sanitized full segment for patterns such as `git status`, but
		// remove quoted arguments so words inside strings cannot look like a
		// command candidate.
		sanitized := stripQuotedShellText(segment)
		result = append(result, strings.TrimSpace(sanitized))
		if fields := strings.Fields(sanitized); len(fields) > 0 {
			result = append(result, fields[0])
		}
	}
	for i := 0; i < len(command); i++ {
		ch := command[i]
		if escaped {
			escaped = false
			continue
		}
		if ch == '\\' && quote != '\'' {
			escaped = true
			continue
		}
		if quote != 0 {
			if ch == quote {
				quote = 0
			}
			continue
		}
		if ch == '\'' || ch == '"' {
			quote = ch
			continue
		}
		if ch == ';' || ch == '|' || ch == '\n' {
			flush(i)
			start = i + 1
			if ch == '|' && i+1 < len(command) && command[i+1] == '|' {
				i++
				start = i + 1
			}
			continue
		}
		if ch == '&' && i+1 < len(command) && command[i+1] == '&' {
			flush(i)
			i++
			start = i + 1
		}
	}
	flush(len(command))
	return result
}

func stripQuotedShellText(segment string) string {
	var b strings.Builder
	quote := byte(0)
	escaped := false
	for i := 0; i < len(segment); i++ {
		ch := segment[i]
		if escaped {
			escaped = false
			if quote == 0 {
				b.WriteByte(ch)
			}
			continue
		}
		if quote != 0 {
			if ch == '\\' && quote == '"' {
				escaped = true
			} else if ch == quote {
				quote = 0
			}
			continue
		}
		if ch == '\'' || ch == '"' {
			quote = ch
			continue
		}
		b.WriteByte(ch)
	}
	return b.String()
}

func normalizeRE2(pattern string) string {
	return strings.ReplaceAll(pattern, "(?:", "(")
}

func containsFold(items []string, value string) bool {
	for _, item := range items {
		if strings.EqualFold(strings.TrimSpace(item), value) {
			return true
		}
	}
	return false
}

// DefaultFilters returns the embedded, provenance-pinned OmniRoute filter
// catalog plus a conservative generic fallback. Invalid JavaScript-only
// patterns are omitted individually; they never make the active snapshot
// unusable and are reported by the migration test.
func DefaultFilters() []Filter {
	filters := []Filter{{
		ID:          "builtin-shell-output",
		StripANSI:   true,
		Collapse:    true,
		Deduplicate: true,
		MaxLines:    0,
		OnEmpty:     "keep-original",
	}}
	filters = append(filters, embeddedFilters()...)
	return filters
}

func sortFilters(filters []Filter) []Filter {
	result := append([]Filter(nil), filters...)
	sort.SliceStable(result, func(i, j int) bool { return result[i].ID < result[j].ID })
	return result
}
