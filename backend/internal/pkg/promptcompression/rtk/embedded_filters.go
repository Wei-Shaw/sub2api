package rtk

import (
	"embed"
	"encoding/json"
	"io/fs"
	"regexp"
	"strings"
)

//go:embed filters/*.json
var embeddedFilterFiles embed.FS

type omniFilter struct {
	ID    string `json:"id"`
	Match struct {
		Commands        []string `json:"commands"`
		ExcludeCommands []string `json:"excludeCommands"`
		Patterns        []string `json:"patterns"`
		ExcludePatterns []string `json:"excludePatterns"`
	} `json:"match"`
	Rules struct {
		StripANSI        bool     `json:"stripAnsi"`
		IncludePatterns  []string `json:"includePatterns"`
		DropPatterns     []string `json:"dropPatterns"`
		CollapsePatterns []string `json:"collapsePatterns"`
		Deduplicate      bool     `json:"deduplicate"`
		MaxLines         int      `json:"maxLines"`
		HeadLines        int      `json:"headLines"`
		TailLines        int      `json:"tailLines"`
		TruncateLineAt   int      `json:"truncateLineAt"`
		MatchOutput      []struct {
			Pattern string `json:"pattern"`
			Message string `json:"message"`
		} `json:"matchOutput"`
	} `json:"rules"`
	Preserve struct {
		ErrorPatterns   []string `json:"errorPatterns"`
		SummaryPatterns []string `json:"summaryPatterns"`
	} `json:"preserve"`
}

func embeddedFilters() []Filter {
	entries, err := fs.Glob(embeddedFilterFiles, "filters/*.json")
	if err != nil {
		return nil
	}
	result := make([]Filter, 0, len(entries))
	for _, name := range entries {
		data, err := fs.ReadFile(embeddedFilterFiles, name)
		if err != nil {
			continue
		}
		var raw omniFilter
		if json.Unmarshal(data, &raw) != nil || strings.TrimSpace(raw.ID) == "" {
			continue
		}
		f := Filter{
			ID:       raw.ID,
			Commands: normalizePatterns(raw.Match.Commands), ExcludeCommands: normalizePatterns(raw.Match.ExcludeCommands),
			Patterns: normalizePatterns(raw.Match.Patterns), ExcludePatterns: normalizePatterns(raw.Match.ExcludePatterns),
			StripANSI: raw.Rules.StripANSI, Include: normalizePatterns(raw.Rules.IncludePatterns), Drop: normalizePatterns(raw.Rules.DropPatterns),
			Collapse: len(raw.Rules.CollapsePatterns) > 0, Deduplicate: raw.Rules.Deduplicate,
			MaxLines: raw.Rules.MaxLines, HeadLines: raw.Rules.HeadLines, TailLines: raw.Rules.TailLines, TruncateLineAt: raw.Rules.TruncateLineAt,
			Preserve: append(normalizePatterns(raw.Preserve.ErrorPatterns), normalizePatterns(raw.Preserve.SummaryPatterns)...),
		}
		for _, rule := range raw.Rules.MatchOutput {
			if strings.TrimSpace(rule.Pattern) != "" {
				f.Replace = append(f.Replace, ReplaceRule{Pattern: normalizeRE2(rule.Pattern), With: rule.Message})
			}
		}
		// Validate each migrated filter independently. A single unsupported JS
		// expression must not disable the remaining catalog.
		if _, err := compileFilters([]Filter{f}); err != nil {
			continue
		}
		result = append(result, f)
	}
	return result
}

func normalizePatterns(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = normalizeRE2(value)
		if _, err := regexp.Compile(value); err != nil {
			continue
		}
		out = append(out, value)
	}
	return out
}
