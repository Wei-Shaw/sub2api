package basispoints

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
)

const imageInputUnavailableMessage = "[Image input is unavailable because Excel / BPS image support is disabled. " +
	"The model cannot see this image. Do not retry view_image or other image-reading tools while image support is disabled. " +
	"Continue using the available text and explain this limitation if the task requires the image.]"

// StripInputImages replaces image parts with explicit unavailable notices before
// validation or attachment handling. Do not traverse tool arguments, schemas or
// text: image-shaped application data there is not a Responses image input.
func StripInputImages(raw []byte) ([]byte, error) {
	var source object
	if err := decode(raw, &source); err != nil || source == nil {
		return nil, fmt.Errorf("invalid Basispoints request JSON")
	}
	input, _ := source["input"].([]any)
	changed := false
	for _, rawItem := range input {
		item, _ := rawItem.(object)
		field := "content"
		switch text(item["type"]) {
		case "", "message", "agent_message":
		case "function_call_output", "custom_tool_call_output":
			field = "output"
		default:
			continue
		}
		parts, ok := item[field].([]any)
		if !ok {
			continue
		}
		for i, rawPart := range parts {
			part, _ := rawPart.(object)
			if text(part["type"]) == "input_image" {
				// Mixed outputs also need a notice: view_image may include only
				// metadata or blank text beside the image. Silently dropping it
				// makes the result look empty or successful and invites retries.
				parts[i] = object{"type": "input_text", "text": imageInputUnavailableMessage}
				changed = true
			}
		}
	}
	if !changed {
		return raw, nil
	}
	return json.Marshal(source)
}

// Accept HTTPS URLs or validated native attachment references.
func validateImage(part object) error {
	if fileID, exists := part["file_id"]; exists {
		id, ok := fileID.(string)
		if !ok || !ValidAttachmentID(id) {
			return fmt.Errorf("basispoints input_image requires a valid file_id")
		}
		if _, exists := part["image_url"]; exists {
			return fmt.Errorf("basispoints input_image requires exactly one image reference")
		}
	} else {
		raw, ok := part["image_url"].(string)
		if !ok || raw == "" {
			return fmt.Errorf("basispoints input_image requires an HTTPS image_url or file_id")
		}
		if strings.HasPrefix(strings.ToLower(strings.TrimSpace(raw)), "data:") {
			return fmt.Errorf("basispoints does not accept data:image input while image support is disabled; ask an administrator to enable the selected BPS account's 'Ignore image inputs when image support is disabled' option (openai_excel_bps_ignore_images), enable BPS image support, provide an HTTPS image URL, or disable Basispoints and start a new conversation")
		}
		parsed, err := url.Parse(raw)
		if err != nil || parsed.Scheme != "https" || parsed.Hostname() == "" || parsed.User != nil || parsed.Opaque != "" || strings.TrimSpace(raw) != raw {
			return fmt.Errorf("basispoints input_image requires an absolute HTTPS image URL without embedded credentials")
		}
	}
	return validateImageDetail(part)
}

func validateImageDetail(part object) error {
	if detail, exists := part["detail"]; exists && detail != nil {
		switch text(detail) {
		case "auto", "low", "high", "original":
		default:
			return fmt.Errorf("basispoints image detail must be auto, low, high or original")
		}
	}
	return nil
}

// Normalize only validated, translated messages. This also covers file images
// moved out of tool results; inline tool screenshots and HTTPS URLs keep their
// own contract. Running after validation must not hide malformed references.
func normalizeMessageFileImages(input []any) {
	for _, raw := range input {
		item, _ := raw.(object)
		kind := text(item["type"])
		if kind != "message" && (kind != "" || text(item["role"]) == "") {
			continue
		}
		parts, _ := item["content"].([]any)
		for i, rawPart := range parts {
			part, _ := rawPart.(object)
			if text(part["type"]) == "input_image" && text(part["file_id"]) != "" {
				parts[i] = object{"type": "input_image", "file_id": part["file_id"]}
			}
		}
	}
}
