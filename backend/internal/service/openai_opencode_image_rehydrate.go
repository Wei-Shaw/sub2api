package service

import (
	"context"
	"encoding/base64"
	"errors"
	"regexp"
	"sort"
	"strings"
)

var (
	openCodeRehydrateImageMarkerPattern         = regexp.MustCompile(`sub2api-image://(img_[A-Za-z0-9_-]{32,})`)
	openCodeRehydrateDownloadPathPattern        = regexp.MustCompile(`/sub2api/generated-images/(img_[A-Za-z0-9_-]{32,})\.(png|jpe?g|webp)`)
	openCodeRehydrateAbsoluteDownloadURLPattern = regexp.MustCompile(`https?://[^\s"'<>]+/sub2api/generated-images/(img_[A-Za-z0-9_-]{32,})\.(png|jpe?g|webp)`)
	openCodeRehydratedAttachedTextPattern       = regexp.MustCompile(`^Attached generated image (img_[A-Za-z0-9_-]{32,}) from the previous response\.$`)
	openCodeRehydratedUnavailableTextPattern    = regexp.MustCompile(`^Generated image (img_[A-Za-z0-9_-]{32,}): image bytes (?:unavailable|were not attached because the image is too large)\b`)
)

type openCodeImageRehydrateOptions struct {
	MaxImages         int
	MaxRehydrateBytes int64
}

func rehydrateOpenCodeGeneratedImageMarkers(ctx context.Context, reqBody map[string]any, store *OpenAIGeneratedImageStore, opts openCodeImageRehydrateOptions) (bool, error) {
	if reqBody == nil || store == nil {
		return false, nil
	}
	input, ok := reqBody["input"]
	if !ok {
		return false, nil
	}

	ids := dedupeOpenCodeImageMarkerIDs(scanOpenCodeGeneratedImageMarkers(input))
	if len(ids) == 0 {
		return false, nil
	}
	maxImages := opts.MaxImages
	if maxImages <= 0 {
		maxImages = 3
	}
	if len(ids) > maxImages {
		ids = ids[len(ids)-maxImages:]
	}
	if alreadyRehydrated := scanOpenCodeRehydratedSyntheticImageIDs(input); len(alreadyRehydrated) > 0 {
		filtered := ids[:0]
		for _, id := range ids {
			if _, ok := alreadyRehydrated[id]; ok {
				continue
			}
			filtered = append(filtered, id)
		}
		ids = filtered
		if len(ids) == 0 {
			return false, nil
		}
	}
	messages := make([]any, 0, len(ids))
	for _, id := range ids {
		rec, data, err := loadOpenCodeGeneratedImageForRehydrate(ctx, store, id, opts.MaxRehydrateBytes)
		switch {
		case err == nil:
			messages = append(messages, buildOpenCodeRehydratedInputImageMessage(id, rec.MIME, data))
		case errors.Is(err, errOpenAIGeneratedImageTooLarge):
			messages = append(messages, buildOpenCodeRehydratedTextMessage("Generated image "+id+": image bytes were not attached because the image is too large. Use the download reference from the conversation instead."))
		case errors.Is(err, errOpenAIGeneratedImageExpired), errors.Is(err, errOpenAIGeneratedImageNotFound), errors.Is(err, errOpenAIGeneratedImageInvalid):
			messages = append(messages, buildOpenCodeRehydratedTextMessage("Generated image "+id+": image bytes unavailable."))
		default:
			return false, err
		}
	}
	reqBody["input"] = appendOpenCodeRehydratedMessages(input, messages)
	return true, nil
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

func scanOpenCodeGeneratedImageMarkers(value any) []string {
	matches := make([]string, 0, 1)
	var scan func(any)
	scan = func(v any) {
		switch typed := v.(type) {
		case string:
			matches = append(matches, extractOpenCodeGeneratedImageMarkerIDs(typed)...)
		case []any:
			for _, item := range typed {
				scan(item)
			}
		case map[string]any:
			typeValue, _ := typed["type"].(string)
			switch strings.TrimSpace(typeValue) {
			case "input_text", "output_text", "text":
				if text, ok := typed["text"].(string); ok {
					matches = append(matches, extractOpenCodeGeneratedImageMarkerIDs(text)...)
				}
			default:
				if content, ok := typed["content"]; ok {
					scan(content)
				}
			}
		}
	}
	scan(value)
	return matches
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

func extractOpenCodeGeneratedImageMarkerIDs(text string) []string {
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
	addRegexMatches(openCodeRehydrateImageMarkerPattern, 1)
	addRegexMatches(openCodeRehydrateDownloadPathPattern, 1)
	addRegexMatches(openCodeRehydrateAbsoluteDownloadURLPattern, 1)
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

func dedupeOpenCodeImageMarkerIDs(ids []string) []string {
	if len(ids) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(ids))
	uniqueReversed := make([]string, 0, len(ids))
	for i := len(ids) - 1; i >= 0; i-- {
		id := ids[i]
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		uniqueReversed = append(uniqueReversed, id)
	}
	unique := make([]string, len(uniqueReversed))
	for i := range uniqueReversed {
		unique[len(uniqueReversed)-1-i] = uniqueReversed[i]
	}
	return unique
}

func buildOpenCodeRehydratedInputImageMessage(id string, mime string, data []byte) map[string]any {
	mime = strings.TrimSpace(mime)
	if mime == "" {
		mime = "image/png"
	}
	return map[string]any{
		"role": "user",
		"content": []any{
			map[string]any{"type": "input_text", "text": "Attached generated image " + id + " from the previous response."},
			map[string]any{"type": "input_image", "image_url": "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(data)},
		},
	}
}

func buildOpenCodeRehydratedTextMessage(text string) map[string]any {
	return map[string]any{
		"role":    "user",
		"content": []any{map[string]any{"type": "input_text", "text": text}},
	}
}

func appendOpenCodeRehydratedMessages(input any, messages []any) []any {
	items := normalizeOpenCodeRehydratedInput(input)
	items = append(items, messages...)
	return items
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
