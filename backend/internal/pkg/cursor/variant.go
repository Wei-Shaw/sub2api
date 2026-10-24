package cursor

import "strings"

// RunOpts selects a parameterized AgentService/Run slug for a picker family.
// Empty Effort means "use this family's default" (medium when Cursor lists it).
type RunOpts struct {
	Effort   string
	Fast     bool
	Thinking *bool
}

// RunResolution is the AgentService/Run slug for a client model id.
type RunResolution struct {
	RunSlug         string
	PickerID        string
	AliasFallback   bool
	VariantApplied  bool
	RequestedIsSlug bool
}

type parsedRunSlug struct {
	Raw           string
	FamilyHint    string
	Effort        string
	Fast          bool
	Thinking      bool
	Parameterized bool
}

type runVariant struct {
	Slug     string
	Effort   string
	Fast     bool
	Thinking bool
}

type runIndex struct {
	slugsByPicker map[string][]string
	pickerBySlug  map[string]string
	pickerByHint  map[string]string
}

var (
	cursorEffortTokens = []string{
		"extra-high",
		"xhigh",
		"medium",
		"minimal",
		"none",
		"high",
		"max",
		"low",
	}

	effortRank = map[string]int{
		"none":       0,
		"minimal":    1,
		"low":        2,
		"medium":     3,
		"high":       4,
		"extra-high": 5,
		"xhigh":      5,
		"max":        6,
	}
)

// NormalizeEffort canonicalizes OpenAI/Cursor effort labels onto Cursor tokens.
func NormalizeEffort(raw string) string {
	compact := strings.ToLower(strings.TrimSpace(raw))
	compact = strings.NewReplacer("-", "", "_", "", " ", "").Replace(compact)
	switch compact {
	case "none":
		return "none"
	case "minimal":
		return "minimal"
	case "low":
		return "low"
	case "medium":
		return "medium"
	case "high":
		return "high"
	case "xhigh", "extrahigh":
		return "xhigh"
	case "max":
		return "max"
	default:
		return ""
	}
}

func effortKey(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "extra-high", "extrahigh", "xhigh":
		return "xhigh"
	default:
		return strings.ToLower(strings.TrimSpace(value))
	}
}

func effortsMatch(a, b string) bool {
	if a == "" || b == "" {
		return a == b
	}
	return effortKey(a) == effortKey(b)
}

func parseRunSlug(slug string) parsedRunSlug {
	raw := strings.TrimSpace(slug)
	s := raw
	out := parsedRunSlug{Raw: raw}
	if s == "" {
		return out
	}

	for {
		changed := false
		if strings.HasSuffix(s, "-fast") {
			out.Fast = true
			s = strings.TrimSuffix(s, "-fast")
			changed = true
		}
		if strings.HasSuffix(s, "-thinking") {
			out.Thinking = true
			s = strings.TrimSuffix(s, "-thinking")
			changed = true
		}
		for _, effort := range cursorEffortTokens {
			suffix := "-" + effort
			if strings.HasSuffix(s, suffix) {
				out.Effort = effort
				s = strings.TrimSuffix(s, suffix)
				changed = true
				break
			}
		}
		if !changed {
			break
		}
	}

	if strings.HasPrefix(s, "cursor-") {
		out.FamilyHint = strings.TrimPrefix(s, "cursor-")
	} else {
		out.FamilyHint = s
	}
	out.Parameterized = out.Fast || out.Thinking || out.Effort != ""
	return out
}

func newRunIndex() *runIndex {
	return &runIndex{
		slugsByPicker: make(map[string][]string),
		pickerBySlug:  make(map[string]string),
		pickerByHint:  make(map[string]string),
	}
}

func (idx *runIndex) clone() *runIndex {
	out := newRunIndex()
	for k, v := range idx.slugsByPicker {
		out.slugsByPicker[k] = append([]string{}, v...)
	}
	for k, v := range idx.pickerBySlug {
		out.pickerBySlug[k] = v
	}
	for k, v := range idx.pickerByHint {
		out.pickerByHint[k] = v
	}
	return out
}

func (idx *runIndex) add(name string, display string, aliases, slugs []string) {
	name = strings.TrimSpace(name)
	if name == "" {
		return
	}
	idx.slugsByPicker[name] = uniqueStrings(slugs)
	idx.setSlug(name, name)
	idx.setHint(name, name)
	if display = strings.TrimSpace(display); display != "" {
		idx.setSlug(display, name)
		idx.setHint(display, name)
	}
	for _, slug := range idx.slugsByPicker[name] {
		idx.setSlug(slug, name)
		parsed := parseRunSlug(slug)
		if parsed.FamilyHint != "" {
			idx.setHint(parsed.FamilyHint, name)
		}
	}
	for _, alias := range aliases {
		idx.setSlug(alias, name)
	}
}

func (idx *runIndex) setSlug(key, picker string) {
	key = strings.ToLower(strings.TrimSpace(key))
	if key == "" {
		return
	}
	if _, exists := idx.pickerBySlug[key]; !exists {
		idx.pickerBySlug[key] = picker
	}
}

func (idx *runIndex) setHint(key, picker string) {
	key = strings.ToLower(strings.TrimSpace(key))
	if key == "" {
		return
	}
	if _, exists := idx.pickerByHint[key]; !exists {
		idx.pickerByHint[key] = picker
	}
}

func (idx *runIndex) overlay(models []AvailableModel) *runIndex {
	if len(models) == 0 {
		return idx
	}
	out := idx.clone()
	for _, model := range models {
		out.replace(model.Name, model.DisplayName, model.Aliases, model.LegacySlugs)
	}
	return out
}

func (idx *runIndex) replace(name, display string, aliases, slugs []string) {
	name = strings.TrimSpace(name)
	if name == "" {
		return
	}
	delete(idx.slugsByPicker, name)
	idx.add(name, display, aliases, slugs)
}

func uniqueStrings(values []string) []string {
	out := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		key := strings.ToLower(value)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, value)
	}
	return out
}

func containsFold(values []string, want string) bool {
	for _, value := range values {
		if strings.EqualFold(value, want) {
			return true
		}
	}
	return false
}

func canonicalSlug(values []string, want string) string {
	for _, value := range values {
		if strings.EqualFold(value, want) {
			return value
		}
	}
	return want
}

func variantsFor(picker string, slugs []string) []runVariant {
	out := make([]runVariant, 0, len(slugs))
	for _, slug := range slugs {
		parsed := parseRunSlug(slug)
		out = append(out, runVariant{
			Slug:     slug,
			Effort:   parsed.Effort,
			Fast:     parsed.Fast,
			Thinking: parsed.Thinking,
		})
	}
	return out
}

func familyHasEffort(variants []runVariant) bool {
	for _, v := range variants {
		if v.Effort != "" {
			return true
		}
	}
	return false
}

func findVariant(variants []runVariant, effort string, thinking, fast bool) (runVariant, bool) {
	for _, v := range variants {
		if v.Thinking == thinking && v.Fast == fast && effortsMatch(v.Effort, effort) {
			return v, true
		}
	}
	return runVariant{}, false
}

func closestEffort(variants []runVariant, want string, thinking, fast *bool) (runVariant, bool) {
	wantRank, ok := effortRank[effortKey(want)]
	if !ok {
		return runVariant{}, false
	}
	bestIdx := -1
	bestDelta := 0
	for i, v := range variants {
		if thinking != nil && v.Thinking != *thinking {
			continue
		}
		if fast != nil && v.Fast != *fast {
			continue
		}
		rank, ok := effortRank[effortKey(v.Effort)]
		if !ok {
			continue
		}
		delta := rank - wantRank
		if delta < 0 {
			delta = -delta
		}
		if bestIdx < 0 || delta < bestDelta || (delta == bestDelta && rank > effortRank[effortKey(variants[bestIdx].Effort)]) {
			bestIdx = i
			bestDelta = delta
		}
	}
	if bestIdx < 0 {
		return runVariant{}, false
	}
	return variants[bestIdx], true
}

func pickRunSlug(picker string, slugs []string, opts RunOpts) string {
	picker = strings.TrimSpace(picker)
	if picker == "" {
		return picker
	}
	variants := variantsFor(picker, slugs)
	if len(variants) == 0 {
		return picker
	}

	wantFast := opts.Fast
	wantThinking := false
	thinkingSet := opts.Thinking != nil
	if thinkingSet {
		wantThinking = *opts.Thinking
	}
	wantEffort := NormalizeEffort(opts.Effort)

	if wantEffort != "" {
		if v, ok := findVariant(variants, wantEffort, wantThinking, wantFast); ok {
			return v.Slug
		}
		if !thinkingSet {
			if v, ok := findVariant(variants, wantEffort, true, wantFast); ok {
				return v.Slug
			}
			if v, ok := findVariant(variants, wantEffort, false, wantFast); ok {
				return v.Slug
			}
		}
		thinkingPtr := &wantThinking
		if !thinkingSet {
			thinkingPtr = nil
		}
		fastPtr := &wantFast
		if v, ok := closestEffort(variants, wantEffort, thinkingPtr, fastPtr); ok {
			return v.Slug
		}
		if v, ok := closestEffort(variants, wantEffort, nil, fastPtr); ok {
			return v.Slug
		}
		if v, ok := closestEffort(variants, wantEffort, nil, nil); ok {
			return v.Slug
		}
	}

	if wantFast || thinkingSet {
		if v, ok := findVariant(variants, wantEffort, wantThinking, wantFast); ok {
			return v.Slug
		}
		if wantEffort == "" {
			if v, ok := findVariant(variants, "medium", wantThinking, wantFast); ok {
				return v.Slug
			}
			if familyHasEffort(variants) {
				if v, ok := findVariant(variants, "", wantThinking, wantFast); ok {
					return v.Slug
				}
				if v, ok := findVariant(variants, "high", wantThinking, wantFast); ok {
					return v.Slug
				}
			}
			if v, ok := findVariant(variants, "", wantThinking, wantFast); ok {
				return v.Slug
			}
		}
	}

	if familyHasEffort(variants) {
		if v, ok := findVariant(variants, "medium", false, false); ok {
			return v.Slug
		}
		if v, ok := findVariant(variants, "", false, true); ok {
			return v.Slug
		}
		if v, ok := findVariant(variants, "high", false, false); ok {
			return v.Slug
		}
		if v, ok := findVariant(variants, "low", false, false); ok {
			return v.Slug
		}
		if v, ok := findVariant(variants, "medium", false, true); ok {
			return v.Slug
		}
		return variants[0].Slug
	}

	if v, ok := findVariant(variants, "", false, false); ok {
		return v.Slug
	}
	return picker
}

func buildDefaultRunIndex() *runIndex {
	idx := newRunIndex()
	slugs := defaultRunSlugTable()
	for _, model := range DefaultModels {
		idx.add(model.ID, model.DisplayName, model.Aliases, slugs[model.ID])
	}
	for name, list := range slugs {
		if _, ok := idx.slugsByPicker[name]; ok {
			continue
		}
		idx.add(name, "", nil, list)
	}
	return idx
}

var defaultRunIndexValue = buildDefaultRunIndex()

func indexForCatalog(catalog []AvailableModel) *runIndex {
	return defaultRunIndexValue.overlay(catalog)
}

// SnapshotLegacySlugs returns the bundled AgentService/Run slugs for a picker id.
// Live catalogs from AvailableModels override this at resolve time when provided.
func SnapshotLegacySlugs(pickerID string) []string {
	return append([]string{}, defaultRunSlugTable()[strings.TrimSpace(pickerID)]...)
}

// ResolveRunModel maps a client model id onto an AgentService/Run slug.
// catalog is optional; when present its LegacySlugs win over the bundled snapshot.
func ResolveRunModel(requested string, opts RunOpts, catalog []AvailableModel) RunResolution {
	requested = strings.TrimSpace(requested)
	if requested == "" {
		return RunResolution{}
	}
	idx := indexForCatalog(catalog)
	picker, aliasFallback, exactSlug := resolveRunPicker(requested, idx)
	slugs := idx.slugsByPicker[picker]
	if exactSlug && !runOptsOverride(opts) {
		slug := canonicalSlug(slugs, requested)
		return RunResolution{
			RunSlug:         slug,
			PickerID:        picker,
			AliasFallback:   aliasFallback,
			RequestedIsSlug: true,
		}
	}

	merged := opts
	parsed := parseRunSlug(requested)
	if parsed.Parameterized && !runOptsOverride(opts) {
		merged.Effort = parsed.Effort
		merged.Fast = parsed.Fast
		thinking := parsed.Thinking
		merged.Thinking = &thinking
	}

	runSlug := pickRunSlug(picker, slugs, merged)
	if runSlug == "" {
		runSlug = picker
	}
	return RunResolution{
		RunSlug:         runSlug,
		PickerID:        picker,
		AliasFallback:   aliasFallback,
		VariantApplied:  !strings.EqualFold(runSlug, picker),
		RequestedIsSlug: exactSlug,
	}
}

func runOptsOverride(opts RunOpts) bool {
	return NormalizeEffort(opts.Effort) != "" || opts.Fast || opts.Thinking != nil
}

func resolveRunPicker(requested string, idx *runIndex) (picker string, aliasFallback, exactSlug bool) {
	if picker, ok := idx.pickerBySlug[strings.ToLower(requested)]; ok {
		if containsFold(idx.slugsByPicker[picker], requested) && !strings.EqualFold(requested, picker) {
			return picker, false, true
		}
		if strings.EqualFold(requested, picker) {
			return picker, false, false
		}
		return picker, true, false
	}
	if id, ok := lookupPickerModel(requested); ok {
		return id, !strings.EqualFold(id, requested), false
	}
	parsed := parseRunSlug(requested)
	if parsed.FamilyHint != "" {
		if picker, ok := idx.pickerByHint[strings.ToLower(parsed.FamilyHint)]; ok {
			if containsFold(idx.slugsByPicker[picker], requested) {
				return picker, false, true
			}
			return picker, false, false
		}
	}
	return requested, false, false
}
