package service

import (
	"strings"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// Only explicitly configured Grok-compatible relays use these status names.
// Native OpenAI/New API responses remain unchanged.
func normalizeOpenAIVideoResponse(account *Account, body []byte) []byte {
	if isQianDreamVideoProtocol(account) {
		return normalizeQianDreamVideoResponse(body)
	}
	if isZhongYVideoProtocol(account) {
		return normalizeZhongYVideoResponse(body)
	}
	if !isOpenAIGrokVideoProtocol(account) {
		return body
	}
	body = normalizeGrokVideoRelayGeometry(body)
	// Some relays put the result under metadata. Move that alias into the
	// existing protected-content rewrite path instead of exposing the source URL.
	if value := gjson.GetBytes(body, "metadata.video_url"); value.Type == gjson.String && strings.TrimSpace(value.String()) != "" {
		if !gjson.GetBytes(body, "video_url").Exists() {
			if rewritten, err := sjson.SetBytes(body, "video_url", value.String()); err == nil {
				body = rewritten
			}
		}
		if rewritten, err := sjson.DeleteBytes(body, "metadata.video_url"); err == nil {
			body = rewritten
		}
	}
	status := strings.ToLower(strings.TrimSpace(gjson.GetBytes(body, "status").String()))
	switch status {
	case "pending":
		status = "queued"
	case "processing", "running":
		status = "in_progress"
	case "done":
		hasContent := false
		for _, path := range []string{"video.url", "url", "video_url", "result_url", "download_url"} {
			value := gjson.GetBytes(body, path)
			if value.Type == gjson.String && strings.TrimSpace(value.String()) != "" {
				hasContent = true
				break
			}
		}
		if !hasContent {
			return body
		}
		status = "completed"
	default:
		return body
	}
	if rewritten, err := sjson.SetBytes(body, "status", status); err == nil {
		return rewritten
	}
	return body
}

// Grok relays may return size="16:9", quality="480p" even though the
// OpenAI contract uses size for pixel dimensions. Only translate that explicit
// response shape; request validation and other providers remain strict.
func normalizeGrokVideoRelayGeometry(body []byte) []byte {
	size := gjson.GetBytes(body, "size")
	if size.Type != gjson.String {
		return body
	}
	switch size.String() {
	case "16:9", "9:16", "1:1", "4:3", "3:4", "3:2", "2:3":
	default:
		return body
	}
	quality := gjson.GetBytes(body, "quality")
	if quality.Type != gjson.String {
		return body
	}
	tier, ok := LookupVideoBillingResolution(quality.String())
	if !ok {
		return body
	}
	// Do not let a JSON rewrite hide duplicate or conflicting billing fields.
	counts := map[string]int{}
	gjson.ParseBytes(body).ForEach(func(key, _ gjson.Result) bool {
		counts[key.String()]++
		return true
	})
	for _, key := range []string{"size", "quality", "resolution", "aspect_ratio"} {
		if counts[key] > 1 {
			return body
		}
	}
	if value := gjson.GetBytes(body, "resolution"); value.Exists() {
		resolution, valid := LookupVideoBillingResolution(value.String())
		if !valid || resolution != tier {
			return body
		}
	}
	if value := gjson.GetBytes(body, "aspect_ratio"); value.Exists() && value.String() != size.String() {
		return body
	}
	out, err := sjson.SetBytes(body, "aspect_ratio", size.String())
	if err != nil {
		return body
	}
	out, err = sjson.SetBytes(out, "resolution", tier)
	if err != nil {
		return body
	}
	out, err = sjson.DeleteBytes(out, "size")
	if err != nil {
		return body
	}
	return out
}
