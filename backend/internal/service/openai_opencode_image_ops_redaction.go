package service

import (
	"encoding/json"
	"regexp"
	"strings"
)

var (
	openCodeGeneratedImageAbsoluteURLForOpsPattern = regexp.MustCompile(`https?://[^\s"'<>]+/sub2api/generated-images/(img_[A-Za-z0-9_-]{32,})\.(png|jpe?g|webp)`)
	openCodeDataImageURLForOpsPattern              = regexp.MustCompile(`data:image/[A-Za-z0-9.+-]+(?:;[A-Za-z0-9=._+-]+)*;base64,[A-Za-z0-9+/=_-]+`)
	openCodeImageResultFieldForOpsPattern          = regexp.MustCompile(`("result"\s*:\s*")[A-Za-z0-9+/=_-]+`)
	openCodePartialImageFieldForOpsPattern         = regexp.MustCompile(`("partial_image_b64"\s*:\s*")[A-Za-z0-9+/=_-]+`)
)

func redactOpenCodeGeneratedImagesForOps(body []byte) []byte {
	if len(body) == 0 {
		return body
	}
	var payload any
	if err := json.Unmarshal(body, &payload); err != nil {
		return []byte(redactOpenCodeGeneratedImageTokensForOps(string(body)))
	}
	redacted, changed := redactOpenCodeGeneratedImagesForOpsValue(payload)
	if !changed {
		return body
	}
	encoded, err := json.Marshal(redacted)
	if err != nil {
		return body
	}
	return encoded
}

func redactOpenCodeGeneratedImagesForOpsValue(value any) (any, bool) {
	switch typed := value.(type) {
	case map[string]any:
		changed := false
		if typeValue, _ := typed["type"].(string); strings.TrimSpace(typeValue) == "input_image" {
			if imageURL, ok := typed["image_url"].(string); ok && strings.HasPrefix(strings.TrimSpace(imageURL), "data:") {
				typed["image_url"] = "[redacted-input-image]"
				changed = true
			}
		}
		if typeValue, _ := typed["type"].(string); strings.TrimSpace(typeValue) == "image_generation_call" {
			if _, ok := typed["result"].(string); ok {
				typed["result"] = "[redacted-image-result]"
				changed = true
			}
		}
		if _, ok := typed["partial_image_b64"].(string); ok {
			typed["partial_image_b64"] = "[redacted-partial-image]"
			changed = true
		}
		for key, child := range typed {
			redacted, childChanged := redactOpenCodeGeneratedImagesForOpsValue(child)
			if childChanged {
				typed[key] = redacted
				changed = true
			}
		}
		return typed, changed
	case []any:
		changed := false
		for idx, child := range typed {
			redacted, childChanged := redactOpenCodeGeneratedImagesForOpsValue(child)
			if childChanged {
				typed[idx] = redacted
				changed = true
			}
		}
		return typed, changed
	case string:
		redacted := redactOpenCodeGeneratedImageTokensForOps(typed)
		return redacted, redacted != typed
	default:
		return value, false
	}
}

func redactOpenCodeGeneratedImageTokensForOps(value string) string {
	if value == "" {
		return value
	}
	redacted := openCodeDataImageURLForOpsPattern.ReplaceAllString(value, "[redacted-input-image]")
	redacted = openCodeImageResultFieldForOpsPattern.ReplaceAllString(redacted, "${1}[redacted-image-result]")
	redacted = openCodePartialImageFieldForOpsPattern.ReplaceAllString(redacted, "${1}[redacted-partial-image]")
	redacted = openCodeGeneratedImageAbsoluteURLForOpsPattern.ReplaceAllString(redacted, "[redacted-generated-image-url]")
	redacted = openCodeRehydrateDownloadPathPattern.ReplaceAllString(redacted, "/sub2api/generated-images/[redacted]")
	redacted = openCodeRehydrateImageMarkerPattern.ReplaceAllString(redacted, "sub2api-image://[redacted]")
	return redacted
}
