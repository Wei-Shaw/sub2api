package service

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

var (
	openCodeRehydrateSpecificMarkerPattern      = regexp.MustCompile(`\[\[sub2api-generated-image:id=(img_[A-Za-z0-9_-]{32,})\]\]`)
	openCodeRehydrateImageMarkerPattern         = regexp.MustCompile(`sub2api-image://(img_[A-Za-z0-9_-]{32,})`)
	openCodeRehydrateDownloadPathPattern        = regexp.MustCompile(`/sub2api/generated-images/(img_[A-Za-z0-9_-]{32,})\.(png|jpe?g|webp)`)
	openCodeRehydrateAbsoluteDownloadURLPattern = regexp.MustCompile(`https?://[^\s"'<>]+/sub2api/generated-images/(img_[A-Za-z0-9_-]{32,})\.(png|jpe?g|webp)`)
	openCodeLegacyGeneratedImageLinePattern     = regexp.MustCompile(`(?m)^Generated image: sub2api-image://(img_[A-Za-z0-9_-]{32,})\s*$`)
	openCodeLegacyDownloadLinePattern           = regexp.MustCompile(`(?m)^(?:Download|I'll download from URL): (?:https?://[^\s]+)?/sub2api/generated-images/(img_[A-Za-z0-9_-]{32,})\.(?:png|jpe?g|webp)\s*$`)
	openCodeRehydratedAttachedTextPattern       = regexp.MustCompile(`^Attached generated image (img_[A-Za-z0-9_-]{32,}) from the previous response\.$`)
	openCodeRehydratedUnavailableTextPattern    = regexp.MustCompile(`^Generated image (img_[A-Za-z0-9_-]{32,}): image bytes (?:unavailable|were not attached because the image is too large)\b`)
)

const openCodeGeneratedImageToolName = "sub2api_generated_image"
const openCodeImageSyntheticToolCallIDPrefix = "call_sub2api_image_img_"

var openCodeUnavailableImageReports = newOpenCodeUnavailableImageReportCache(256, time.Hour)

type openCodeImageRehydrateOptions struct {
	MaxImages         int
	MaxRehydrateBytes int64
}

type openCodeImageToolInsert struct {
	index int
	items []any
}

func rehydrateOpenCodeGeneratedImageMarkers(ctx context.Context, reqBody map[string]any, store *OpenAIGeneratedImageStore, opts openCodeImageRehydrateOptions) (bool, error) {
	if reqBody == nil || store == nil {
		return false, nil
	}
	input, ok := reqBody["input"]
	if !ok {
		return false, nil
	}

	maxImages := opts.MaxImages
	if maxImages <= 0 {
		maxImages = 3
	}
	items := normalizeOpenCodeRehydratedInput(input)
	refs := selectOpenCodeImageMarkerRefs(items, maxImages)
	if len(refs) == 0 {
		return false, nil
	}
	usedCallIDs := collectOpenCodeInputCallIDs(items)
	inserts := make([]openCodeImageToolInsert, 0, len(refs))
	for _, ref := range refs {
		rec, data, err := loadOpenCodeGeneratedImageForRehydrate(ctx, store, ref.id, opts.MaxRehydrateBytes)
		var parts []any
		switch {
		case err == nil:
			parts = buildOpenCodeRehydratedInputImageParts(ref.id, rec.MIME, data)
		case errors.Is(err, errOpenAIGeneratedImageTooLarge):
			if !shouldReportOpenCodeUnavailableImage(ref.id, "too_large", ref.explicit && ref.currentUser) {
				continue
			}
			parts = buildOpenCodeUnavailableImageToolParts(ref.id)
		case errors.Is(err, errOpenAIGeneratedImageExpired):
			if !shouldReportOpenCodeUnavailableImage(ref.id, "expired", ref.explicit && ref.currentUser) {
				continue
			}
			parts = buildOpenCodeUnavailableImageToolParts(ref.id)
		case errors.Is(err, errOpenAIGeneratedImageNotFound):
			if !shouldReportOpenCodeUnavailableImage(ref.id, "not_found", ref.explicit && ref.currentUser) {
				continue
			}
			parts = buildOpenCodeUnavailableImageToolParts(ref.id)
		case errors.Is(err, errOpenAIGeneratedImageInvalid):
			if !shouldReportOpenCodeUnavailableImage(ref.id, "invalid", ref.explicit && ref.currentUser) {
				continue
			}
			parts = buildOpenCodeUnavailableImageToolParts(ref.id)
		default:
			return false, err
		}
		call, callID := buildOpenCodeImageToolCall(ref.id, usedCallIDs)
		inserts = append(inserts, openCodeImageToolInsert{index: ref.index, items: []any{call, buildOpenCodeImageToolOutput(callID, parts)}})
	}
	if len(inserts) == 0 {
		return false, nil
	}
	reqBody["input"] = insertOpenCodeImageToolPairs(items, inserts)
	return true, nil
}

func shouldReportOpenCodeUnavailableImage(id string, reason string, explicit bool) bool {
	if !explicit {
		return false
	}
	return openCodeUnavailableImageReports.Mark(id + "\x00" + reason)
}

type openCodeUnavailableImageReportCache struct {
	mu       sync.Mutex
	entries  map[string]time.Time
	order    []string
	capacity int
	ttl      time.Duration
	now      func() time.Time
}

func newOpenCodeUnavailableImageReportCache(capacity int, ttl time.Duration) *openCodeUnavailableImageReportCache {
	return &openCodeUnavailableImageReportCache{
		entries:  make(map[string]time.Time),
		capacity: capacity,
		ttl:      ttl,
		now:      time.Now,
	}
}

func (c *openCodeUnavailableImageReportCache) Mark(key string) bool {
	if c == nil {
		return true
	}
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.capacity <= 0 {
		// Disabled storage: allow the report, but do not retain process state.
		return true
	}
	now := time.Now()
	if c.now != nil {
		now = c.now()
	}
	if c.entries == nil {
		c.entries = make(map[string]time.Time)
	}
	c.evictExpiredLocked(now)
	if reportedAt, ok := c.entries[key]; ok && !c.isExpiredLocked(reportedAt, now) {
		return false
	}
	if _, ok := c.entries[key]; !ok {
		c.order = append(c.order, key)
	}
	c.entries[key] = now
	c.evictExpiredLocked(now)
	c.evictOverflowLocked()
	return true
}

func (c *openCodeUnavailableImageReportCache) evictExpiredLocked(now time.Time) {
	if c.ttl <= 0 || len(c.entries) == 0 {
		return
	}
	removed := false
	for key, reportedAt := range c.entries {
		if c.isExpiredLocked(reportedAt, now) {
			delete(c.entries, key)
			removed = true
		}
	}
	if removed {
		c.compactOrderLocked()
	}
}

func (c *openCodeUnavailableImageReportCache) evictOverflowLocked() {
	for len(c.entries) > c.capacity {
		if len(c.order) == 0 {
			c.entries = make(map[string]time.Time)
			return
		}
		oldest := c.order[0]
		c.order = c.order[1:]
		delete(c.entries, oldest)
	}
}

func (c *openCodeUnavailableImageReportCache) compactOrderLocked() {
	if len(c.order) == 0 {
		return
	}
	compacted := c.order[:0]
	seen := make(map[string]struct{}, len(c.entries))
	for _, key := range c.order {
		if _, ok := c.entries[key]; !ok {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		compacted = append(compacted, key)
	}
	c.order = compacted
}

func (c *openCodeUnavailableImageReportCache) isExpiredLocked(reportedAt time.Time, now time.Time) bool {
	return c.ttl > 0 && !reportedAt.Add(c.ttl).After(now)
}

func loadOpenCodeGeneratedImageForRehydrate(ctx context.Context, store *OpenAIGeneratedImageStore, id string, maxRehydrateBytes int64) (OpenAIGeneratedImageRecord, []byte, error) {
	if maxRehydrateBytes > 0 {
		return store.loadWithMaxRehydrateBytes(ctx, id, maxRehydrateBytes)
	}
	return store.Load(ctx, id)
}

type openCodeImageMarkerMatch struct {
	id  string
	pos int
	seq int
}

type openCodeImageMarkerRef struct {
	id          string
	index       int
	legacy      bool
	seq         int
	explicit    bool
	currentUser bool
}

func scanOpenCodeGeneratedImageMarkers(value any) []string {
	refs := scanOpenCodeGeneratedImageMarkerRefs(value)
	matches := make([]string, 0, len(refs))
	for _, ref := range refs {
		matches = append(matches, ref.id)
	}
	return matches
}

func scanOpenCodeGeneratedImageMarkerRefs(input any) []openCodeImageMarkerRef {
	items := normalizeOpenCodeRehydratedInput(input)
	matches := make([]openCodeImageMarkerRef, 0, 1)
	seq := 0
	for idx, item := range items {
		m, ok := item.(map[string]any)
		if !ok || isOpenCodeImageScanBlockedItem(m) || isOpenCodeSyntheticImageToolItem(m) || isOpenCodeSysDummyItem(m) {
			continue
		}
		isCurrentUser := isOpenCodeCurrentUserInput(items, idx, m)
		for _, text := range openCodeImageTextFields(m) {
			for _, id := range extractOpenCodeSpecificGeneratedImageIDs(text) {
				if !isOpenCodeSpecificImageSource(m, isCurrentUser) {
					continue
				}
				matches = append(matches, openCodeImageMarkerRef{id: id, index: idx, seq: seq, explicit: isOpenCodeUserRole(m), currentUser: isCurrentUser})
				seq++
			}
			for _, id := range extractOpenCodeLegacyGeneratedImageIDs(text) {
				if !isOpenCodeLegacyImageSource(m, text) {
					continue
				}
				matches = append(matches, openCodeImageMarkerRef{id: id, index: idx, legacy: true, seq: seq})
				seq++
			}
		}
	}
	return matches
}

func selectOpenCodeImageMarkerRefs(input any, maxImages int) []openCodeImageMarkerRef {
	if maxImages <= 0 {
		maxImages = 3
	}
	items := normalizeOpenCodeRehydratedInput(input)
	refs := scanOpenCodeGeneratedImageMarkerRefs(items)
	if len(refs) == 0 {
		return nil
	}
	dummyStart, hasDummyTail := findSysDummyTail(items)
	lastByID := make(map[string]openCodeImageMarkerRef, len(refs))
	for _, ref := range refs {
		if hasDummyTail && ref.index >= dummyStart {
			continue
		}
		lastByID[ref.id] = ref
	}
	if len(lastByID) == 0 {
		return nil
	}
	selected := make([]openCodeImageMarkerRef, 0, len(lastByID))
	for _, ref := range lastByID {
		selected = append(selected, ref)
	}
	sort.Slice(selected, func(i, j int) bool {
		return selected[i].seq < selected[j].seq
	})
	if len(selected) > maxImages {
		selected = selected[len(selected)-maxImages:]
	}
	if alreadyRehydrated := scanOpenCodeRehydratedSyntheticImageIDs(items); len(alreadyRehydrated) > 0 {
		filtered := selected[:0]
		for _, ref := range selected {
			if _, ok := alreadyRehydrated[ref.id]; ok {
				continue
			}
			filtered = append(filtered, ref)
		}
		selected = filtered
		if len(selected) == 0 {
			return nil
		}
	}
	sort.Slice(selected, func(i, j int) bool {
		if selected[i].index == selected[j].index {
			return selected[i].seq < selected[j].seq
		}
		return selected[i].index < selected[j].index
	})
	return selected
}

func collectOpenCodeInputCallIDs(items []any) map[string]struct{} {
	used := make(map[string]struct{})
	for _, item := range items {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		callID := strings.TrimSpace(asStringMaybe(m["call_id"]))
		if callID == "" {
			continue
		}
		used[callID] = struct{}{}
	}
	return used
}

func openCodeImageTextFields(item map[string]any) []string {
	texts := make([]string, 0, 2)
	appendText := func(value any) {
		text, ok := value.(string)
		if !ok || strings.TrimSpace(text) == "" {
			return
		}
		texts = append(texts, text)
	}
	appendText(item["text"])
	content, ok := item["content"].([]any)
	if !ok {
		return texts
	}
	for _, part := range content {
		partMap, ok := part.(map[string]any)
		if !ok {
			continue
		}
		switch strings.TrimSpace(asStringMaybe(partMap["type"])) {
		case "input_text", "output_text", "text":
			appendText(partMap["text"])
		}
	}
	return texts
}

func isOpenCodeImageScanBlockedItem(item map[string]any) bool {
	if strings.TrimSpace(asStringMaybe(item["type"])) == "reasoning" {
		return true
	}
	for _, text := range openCodeImageTextFields(item) {
		if strings.Contains(text, "[Compressed conversation section]") || strings.Contains(text, "What did we do so far?") {
			return true
		}
	}
	return false
}

func isOpenCodeSyntheticImageToolItem(item map[string]any) bool {
	if !isOpenCodeImageCallID(item["call_id"]) {
		return false
	}
	switch strings.TrimSpace(asStringMaybe(item["type"])) {
	case "function_call":
		return strings.TrimSpace(asStringMaybe(item["name"])) == openCodeGeneratedImageToolName
	case "function_call_output":
		return true
	default:
		return false
	}
}

func isOpenCodeSysDummyItem(item map[string]any) bool {
	if strings.TrimSpace(asStringMaybe(item["call_id"])) != sysDummyToolCallID {
		return false
	}
	switch strings.TrimSpace(asStringMaybe(item["type"])) {
	case "function_call":
		return strings.TrimSpace(asStringMaybe(item["name"])) == sysDummyToolName
	case "function_call_output":
		return strings.TrimSpace(asStringMaybe(item["output"])) == sysDummyToolOutput
	default:
		return false
	}
}

func isOpenCodeImageCallID(value any) bool {
	callID, ok := value.(string)
	return ok && strings.HasPrefix(strings.TrimSpace(callID), openCodeImageSyntheticToolCallIDPrefix)
}

func imageIDFromOpenCodeImageCallID(value any) string {
	callID, _ := value.(string)
	id := strings.TrimPrefix(strings.TrimSpace(callID), "call_sub2api_image_")
	if idx := strings.LastIndex(id, "_dup"); idx >= 0 && hasOnlyASCIIDigits(id[idx+4:]) {
		id = id[:idx]
	}
	return id
}

func imageIDFromOpenCodeImageSyntheticCallID(value any) (string, bool) {
	callID := strings.TrimSpace(asStringMaybe(value))
	var id string
	switch {
	case strings.HasPrefix(callID, "call_sub2api_image_img_"):
		id = strings.TrimPrefix(callID, "call_sub2api_image_")
	case strings.HasPrefix(callID, "fcsub2api_image_img_"):
		id = strings.TrimPrefix(callID, "fcsub2api_image_")
	default:
		return "", false
	}
	if idx := strings.LastIndex(id, "_dup"); idx >= 0 && hasOnlyASCIIDigits(id[idx+4:]) {
		id = id[:idx]
	}
	return id, strings.TrimSpace(id) != ""
}

func hasOnlyASCIIDigits(text string) bool {
	if text == "" {
		return false
	}
	for _, r := range text {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func buildOpenCodeImageToolCall(id string, used map[string]struct{}) (map[string]any, string) {
	base := "call_sub2api_image_" + id
	callID := uniqueOpenCodeImageCallID(base, used)
	return map[string]any{"type": "function_call", "call_id": callID, "name": openCodeGeneratedImageToolName, "arguments": "{}"}, callID
}

func buildOpenCodeImageToolOutput(callID string, parts []any) map[string]any {
	return map[string]any{"type": "function_call_output", "call_id": callID, "output": parts}
}

func buildOpenCodeImageToolOutputStringFallback(id, url string) string {
	id = strings.TrimSpace(id)
	return "Generated image reference available. Image reference: " + buildOpenCodeSpecificGeneratedImageMarker(id)
}

func buildOpenCodeSpecificGeneratedImageMarker(id string) string {
	return "[[sub2api-generated-image:id=" + strings.TrimSpace(id) + "]]"
}

func rewriteOpenCodeImageToolOutputsToStringFallback(reqBody map[string]any) bool {
	input, ok := reqBody["input"].([]any)
	if !ok {
		return false
	}
	syntheticCallIDs := collectOpenCodeImageSyntheticToolCallIDs(input)
	if len(syntheticCallIDs) == 0 {
		return false
	}
	changed := false
	for _, item := range input {
		m, ok := item.(map[string]any)
		if !ok || strings.TrimSpace(asStringMaybe(m["type"])) != "function_call_output" {
			continue
		}
		id, ok := syntheticCallIDs[strings.TrimSpace(asStringMaybe(m["call_id"]))]
		if !ok {
			continue
		}
		parts, ok := m["output"].([]any)
		if !ok {
			continue
		}
		m["output"] = buildOpenCodeImageToolOutputStringFallbackFromParts(id, parts)
		changed = true
	}
	return changed
}

func collectOpenCodeImageSyntheticToolCallIDs(input []any) map[string]string {
	callIDs := make(map[string]string)
	for _, item := range input {
		m, ok := item.(map[string]any)
		if !ok || strings.TrimSpace(asStringMaybe(m["type"])) != "function_call" || strings.TrimSpace(asStringMaybe(m["name"])) != openCodeGeneratedImageToolName {
			continue
		}
		callID := strings.TrimSpace(asStringMaybe(m["call_id"]))
		if callID == "" {
			continue
		}
		id, ok := imageIDFromOpenCodeImageSyntheticCallID(callID)
		if !ok {
			continue
		}
		callIDs[callID] = id
	}
	return callIDs
}

func buildOpenCodeImageToolOutputStringFallbackFromParts(id string, parts []any) string {
	texts := make([]string, 0, len(parts)+2)
	hasImage := false
	for _, part := range parts {
		partMap, ok := part.(map[string]any)
		if !ok {
			continue
		}
		switch strings.TrimSpace(asStringMaybe(partMap["type"])) {
		case "input_image":
			hasImage = true
		case "input_text", "output_text", "text":
			if text := sanitizeOpenCodeImageToolOutputFallbackText(asStringMaybe(partMap["text"])); text != "" {
				texts = append(texts, text)
			}
		}
	}
	if len(texts) == 0 {
		texts = append(texts, "Generated image context is available through the image reference.")
	}
	if hasImage {
		texts = append(texts, "Compatibility fallback: image pixels were not attached.")
	}
	texts = append(texts, "Image reference: "+buildOpenCodeSpecificGeneratedImageMarker(id))
	return strings.Join(texts, " ")
}

func sanitizeOpenCodeImageToolOutputFallbackText(text string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}
	text = openCodeRehydrateAbsoluteDownloadURLPattern.ReplaceAllString(text, "[download URL omitted]")
	text = openCodeRehydrateDownloadPathPattern.ReplaceAllString(text, "[download URL omitted]")
	lower := strings.ToLower(text)
	if strings.Contains(lower, "data:image") || strings.Contains(lower, "base64,") {
		return ""
	}
	text = strings.Join(strings.Fields(text), " ")
	const maxFallbackTextLen = 240
	if len(text) > maxFallbackTextLen {
		text = strings.TrimSpace(text[:maxFallbackTextLen]) + "..."
	}
	return text
}

func cloneOpenAIRequestBodyMap(reqBody map[string]any) map[string]any {
	if reqBody == nil {
		return nil
	}
	data, err := json.Marshal(reqBody)
	if err != nil {
		return nil
	}
	var cloned map[string]any
	if err := json.Unmarshal(data, &cloned); err != nil {
		return nil
	}
	return cloned
}

func uniqueOpenCodeImageCallID(base string, used map[string]struct{}) string {
	if used == nil {
		used = make(map[string]struct{})
	}
	if _, ok := used[base]; !ok {
		used[base] = struct{}{}
		return base
	}
	for i := 1; ; i++ {
		candidate := base + "_dup" + strconv.Itoa(i)
		if _, ok := used[candidate]; !ok {
			used[candidate] = struct{}{}
			return candidate
		}
	}
}

func findSysDummyTail(items []any) (int, bool) {
	if len(items) < 2 {
		return 0, false
	}
	call, ok := items[len(items)-2].(map[string]any)
	if !ok {
		return 0, false
	}
	output, ok := items[len(items)-1].(map[string]any)
	if !ok {
		return 0, false
	}
	if call["type"] != "function_call" || call["name"] != sysDummyToolName || call["call_id"] != sysDummyToolCallID {
		return 0, false
	}
	if output["type"] != "function_call_output" || output["call_id"] != sysDummyToolCallID {
		return 0, false
	}
	return len(items) - 2, true
}

func isOpenCodeSpecificImageSource(item map[string]any, currentUser bool) bool {
	if isOpenCodeUserRole(item) {
		return currentUser
	}
	return isOpenCodeSub2APIImageAssistantMessage(item)
}

func isOpenCodeLegacyImageSource(item map[string]any, text string) bool {
	if !isOpenCodeSub2APIImageAssistantMessage(item) {
		return false
	}
	trimmed := strings.TrimSpace(text)
	return openCodeLegacyGeneratedImageLinePattern.MatchString(trimmed) || openCodeLegacyDownloadLinePattern.MatchString(trimmed)
}

func isOpenCodeSub2APIImageAssistantMessage(item map[string]any) bool {
	if strings.TrimSpace(asStringMaybe(item["role"])) != "assistant" {
		return false
	}
	if strings.HasPrefix(strings.TrimSpace(asStringMaybe(item["id"])), "msg_sub2api_img_") {
		return true
	}
	for _, text := range openCodeImageTextFields(item) {
		if isOpenCodeSpecificGeneratedImageMessageText(text) || isOpenCodeLegacyGeneratedImageMessageText(text) {
			return true
		}
	}
	return false
}

func isOpenCodeSpecificGeneratedImageMessageText(text string) bool {
	lines := strings.Split(strings.TrimSpace(text), "\n")
	if len(lines) != 2 && len(lines) != 3 {
		return false
	}
	if lines[0] != "Generated image saved by sub2api." {
		return false
	}
	marker, ok := strings.CutPrefix(lines[1], "Image reference: ")
	if !ok || !regexMatchesEntireString(openCodeRehydrateSpecificMarkerPattern, marker) {
		return false
	}
	if len(lines) == 2 {
		return true
	}
	downloadURL, ok := strings.CutPrefix(lines[2], "Temporary download URL: ")
	return ok && regexMatchesEntireString(openCodeRehydrateAbsoluteDownloadURLPattern, downloadURL)
}

func isOpenCodeLegacyGeneratedImageMessageText(text string) bool {
	lines := strings.Split(strings.TrimSpace(text), "\n")
	if len(lines) != 1 && len(lines) != 2 {
		return false
	}
	if !regexMatchesEntireString(openCodeLegacyGeneratedImageLinePattern, lines[0]) {
		return false
	}
	return len(lines) == 1 || regexMatchesEntireString(openCodeLegacyDownloadLinePattern, lines[1])
}

func regexMatchesEntireString(re *regexp.Regexp, text string) bool {
	match := re.FindString(text)
	return match == text
}

func isOpenCodeUserRole(item map[string]any) bool {
	return strings.TrimSpace(asStringMaybe(item["role"])) == "user"
}

func isOpenCodeCurrentUserInput(items []any, idx int, item map[string]any) bool {
	if !isOpenCodeUserRole(item) || isOpenCodeImageScanBlockedItem(item) {
		return false
	}
	for i := len(items) - 1; i >= 0; i-- {
		candidate, ok := items[i].(map[string]any)
		if !ok || isOpenCodeImageScanBlockedItem(candidate) {
			continue
		}
		if isOpenCodeUserRole(candidate) {
			return i == idx
		}
	}
	return false
}

func extractOpenCodeSpecificGeneratedImageIDs(text string) []string {
	return extractIDsWithRegex(text, openCodeRehydrateSpecificMarkerPattern, 1)
}

func extractOpenCodeLegacyGeneratedImageIDs(text string) []string {
	if strings.TrimSpace(text) == "" {
		return nil
	}
	matches := make([]openCodeImageMarkerMatch, 0, 1)
	seq := 0
	addRegexMatches := func(re *regexp.Regexp, idGroup int) {
		for _, idx := range re.FindAllStringSubmatchIndex(text, -1) {
			groupStart := idGroup * 2
			if len(idx) <= groupStart+1 || idx[groupStart] < 0 || idx[groupStart+1] < 0 {
				continue
			}
			matches = append(matches, openCodeImageMarkerMatch{id: text[idx[groupStart]:idx[groupStart+1]], pos: idx[0], seq: seq})
			seq++
		}
	}
	addRegexMatches(openCodeLegacyGeneratedImageLinePattern, 1)
	addRegexMatches(openCodeLegacyDownloadLinePattern, 1)
	sort.SliceStable(matches, func(i, j int) bool {
		if matches[i].pos == matches[j].pos {
			return matches[i].seq < matches[j].seq
		}
		return matches[i].pos < matches[j].pos
	})
	ids := make([]string, 0, len(matches))
	for _, match := range matches {
		ids = append(ids, match.id)
	}
	return ids
}

func extractIDsWithRegex(text string, re *regexp.Regexp, group int) []string {
	if strings.TrimSpace(text) == "" || re == nil || group < 0 {
		return nil
	}
	var ids []string
	for _, match := range re.FindAllStringSubmatch(text, -1) {
		if len(match) > group {
			ids = append(ids, match[group])
		}
	}
	return ids
}

func scanOpenCodeRehydratedSyntheticImageIDs(value any) map[string]struct{} {
	ids := make(map[string]struct{})
	var scan func(any)
	scan = func(v any) {
		switch typed := v.(type) {
		case []any:
			for _, item := range typed {
				scan(item)
			}
		case map[string]any:
			if isOpenCodeImageCallID(typed["call_id"]) && strings.TrimSpace(asStringMaybe(typed["type"])) == "function_call" && strings.TrimSpace(asStringMaybe(typed["name"])) == openCodeGeneratedImageToolName {
				if id := imageIDFromOpenCodeImageCallID(typed["call_id"]); strings.TrimSpace(id) != "" {
					ids[id] = struct{}{}
				}
				return
			}
			if isOpenCodeSyntheticImageToolItem(typed) {
				return
			}
			typeValue, _ := typed["type"].(string)
			switch strings.TrimSpace(typeValue) {
			case "input_text", "output_text", "text":
				if text, ok := typed["text"].(string); ok {
					for _, id := range extractOpenCodeRehydratedSyntheticImageIDs(text) {
						ids[id] = struct{}{}
					}
				}
			default:
				if content, ok := typed["content"]; ok {
					scan(content)
				}
			}
		}
	}
	scan(value)
	return ids
}

func extractOpenCodeRehydratedSyntheticImageIDs(text string) []string {
	if strings.TrimSpace(text) == "" {
		return nil
	}
	for _, re := range []*regexp.Regexp{openCodeRehydratedAttachedTextPattern, openCodeRehydratedUnavailableTextPattern} {
		match := re.FindStringSubmatch(text)
		if len(match) > 1 {
			return []string{match[1]}
		}
	}
	return nil
}

func buildOpenCodeRehydratedInputImageParts(id string, mime string, data []byte) []any {
	mime = strings.TrimSpace(mime)
	if mime == "" {
		mime = "image/png"
	}
	return []any{
		map[string]any{"type": "input_text", "text": "Generated image " + id + " restored by sub2api from the nearby image marker."},
		map[string]any{"type": "input_image", "image_url": "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(data)},
	}
}

func buildOpenCodeUnavailableImageToolParts(id string) []any {
	return []any{map[string]any{"type": "input_text", "text": "Generated image " + id + " is no longer available. Use the nearby marker only as historical context."}}
}

func insertOpenCodeImageToolPairs(input any, inserts []openCodeImageToolInsert) []any {
	items := normalizeOpenCodeRehydratedInput(input)
	if len(inserts) == 0 {
		return items
	}
	byIndex := make(map[int][]any, len(inserts))
	for _, insert := range inserts {
		byIndex[insert.index] = append(byIndex[insert.index], insert.items...)
	}
	result := make([]any, 0, len(items)+len(inserts)*2)
	for idx, item := range items {
		result = append(result, item)
		if extra := byIndex[idx]; len(extra) > 0 {
			result = append(result, extra...)
		}
	}
	return result
}

func normalizeOpenCodeRehydratedInput(input any) []any {
	switch typed := input.(type) {
	case nil:
		return nil
	case []any:
		return typed
	case string:
		return []any{map[string]any{"role": "user", "content": []any{map[string]any{"type": "input_text", "text": typed}}}}
	default:
		return []any{typed}
	}
}
