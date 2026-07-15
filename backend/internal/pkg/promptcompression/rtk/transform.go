package rtk

import (
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"
)

var ansiPattern = regexp.MustCompile(`\x1b(?:\[[0-?]*[ -/]*[@-~]|\][^\a]*(?:\a|\x1b\\))`)
var controlPattern = regexp.MustCompile(`[\x00-\x08\x0b\x0c\x0e-\x1f\x7f]`)

func cleanANSI(s string) string {
	s = ansiPattern.ReplaceAllString(s, "")
	return controlPattern.ReplaceAllString(s, "")
}

func applyFilterText(text string, f compiledFilter, intensity Intensity) (string, bool) {
	original := text
	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	// Preserve marks are established before any transform. Protected lines are
	// hard guards: no ANSI cleanup, replacement, drop, deduplication, or
	// truncation may alter them.
	protected := make([]bool, len(lines))
	for i, line := range lines {
		for _, p := range f.preserve {
			if p.MatchString(line) {
				protected[i] = true
				break
			}
		}
	}
	for i, line := range lines {
		if protected[i] {
			continue
		}
		if f.StripANSI {
			line = cleanANSI(line)
		}
		for _, r := range f.replace {
			line = r.re.ReplaceAllString(line, r.with)
		}
		lines[i] = line
	}
	if len(f.include) > 0 {
		kept := lines[:0]
		for i, line := range lines {
			if protected[i] {
				kept = append(kept, line)
				continue
			}
			for _, p := range f.include {
				if p.MatchString(line) {
					kept = append(kept, line)
					break
				}
			}
		}
		lines = kept
	}
	protected = markProtected(lines, f.preserve)
	if len(f.drop) > 0 {
		kept := lines[:0]
		for i, line := range lines {
			drop := false
			for _, p := range f.drop {
				if p.MatchString(line) {
					drop = true
					break
				}
			}
			if protected[i] {
				drop = false
			}
			if !drop {
				kept = append(kept, line)
			}
		}
		lines = kept
	}
	protected = markProtected(lines, f.preserve)
	if f.Collapse {
		lines = collapseLines(lines, protected)
	}
	if f.Deduplicate {
		lines = dedupLines(lines, protected)
	}
	// Recompute indexes after shape-changing transforms so a protected line can
	// never become an out-of-range index (and remains protected across stages).
	if len(protected) != len(lines) {
		protected = make([]bool, len(lines))
		for i, line := range lines {
			for _, p := range f.preserve {
				if p.MatchString(line) {
					protected[i] = true
					break
				}
			}
		}
	}
	limit := f.MaxLines
	if intensity == IntensityAggressive && limit > 0 {
		limit = maxInt(1, limit*2/3)
	}
	if intensity == IntensitySafe && limit > 0 {
		limit = limit * 4 / 3
	}
	if limit > 0 && len(lines) > limit {
		lines = truncateLines(lines, protected, limit, f.HeadLines, f.TailLines)
	}
	if f.TruncateLineAt > 0 {
		for i := range lines {
			if i < len(protected) && protected[i] {
				continue
			}
			if utf8.RuneCountInString(lines[i]) > f.TruncateLineAt {
				lines[i] = truncateRuneString(lines[i], f.TruncateLineAt)
			}
		}
	}
	text = strings.Join(lines, "\n")
	if strings.TrimSpace(text) == "" && strings.TrimSpace(original) != "" && strings.EqualFold(f.OnEmpty, "keep-original") {
		return original, false
	}
	return text, text != original
}

func markProtected(lines []string, patterns []*regexp.Regexp) []bool {
	protected := make([]bool, len(lines))
	for i, line := range lines {
		for _, p := range patterns {
			if p.MatchString(line) {
				protected[i] = true
				break
			}
		}
	}
	return protected
}

func collapseLines(lines []string, protected []bool) []string {
	if len(lines) < 2 {
		return lines
	}
	result := make([]string, 0, len(lines))
	for i, line := range lines {
		if i > 0 && strings.TrimSpace(line) == "" && strings.TrimSpace(result[len(result)-1]) == "" && !protected[i] {
			continue
		}
		result = append(result, line)
	}
	return result
}

func dedupLines(lines []string, protected []bool) []string {
	seen := make(map[string]bool, len(lines))
	result := make([]string, 0, len(lines))
	for i, line := range lines {
		key := strings.TrimSpace(line)
		if key != "" && seen[key] && !protected[i] {
			continue
		}
		if key != "" {
			seen[key] = true
		}
		result = append(result, line)
	}
	return result
}

func truncateLines(lines []string, protected []bool, maxLines, head, tail int) []string {
	if maxLines <= 0 || len(lines) <= maxLines {
		return lines
	}
	if head <= 0 && tail <= 0 {
		head = maxLines / 2
		tail = maxLines - head
	}
	if head+tail > maxLines {
		tail = maxLines - head
		if tail < 0 {
			tail = 0
		}
	}
	// Preserve protected lines by expanding the retained set. If doing so would
	// violate the budget, the caller's body-level guard will keep the original.
	keep := make(map[int]bool)
	for i := 0; i < head && i < len(lines); i++ {
		keep[i] = true
	}
	for i := maxInt(0, len(lines)-tail); i < len(lines); i++ {
		keep[i] = true
	}
	for i, p := range protected {
		if p {
			keep[i] = true
		}
	}
	indices := make([]int, 0, len(keep))
	for i := range keep {
		indices = append(indices, i)
	}
	sort.Ints(indices)
	if len(indices) > maxLines {
		return lines
	}
	result := make([]string, 0, len(indices)+1)
	last := -1
	for _, i := range indices {
		if last >= 0 && i > last+1 {
			result = append(result, "… [output omitted] …")
		}
		result = append(result, lines[i])
		last = i
	}
	return result
}

func truncateRuneString(s string, n int) string {
	if n <= 0 {
		return ""
	}
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
