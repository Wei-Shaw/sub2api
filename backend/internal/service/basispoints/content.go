package basispoints

import "fmt"

// ContentValidationError identifies a rejected history part using only safe
// protocol metadata, never the part's payload or caller-controlled type.
type ContentValidationError struct {
	Path        string
	ContentType string
	message     string
}

func (e *ContentValidationError) Error() string {
	return fmt.Sprintf("%s (path=%s; type=%s)", e.message, e.Path, e.ContentType)
}

func (b *Bridge) validateHistoryContent(value any, inputIndex int, field string) error {
	content, _ := value.([]any)
	for index, rawPart := range content {
		part, _ := rawPart.(object)
		path := fmt.Sprintf("input[%d].%s[%d]", inputIndex, field, index)
		switch text(part["type"]) {
		case "input_text", "output_text", "text", "refusal":
		case "input_image":
			var err error
			if field == "output" && b.nativeToolImages[text(part["image_url"])] {
				// Only this request's fully validated tool screenshots may remain inline.
				if _, exists := part["file_id"]; exists {
					err = fmt.Errorf("basispoints input_image requires exactly one image reference")
				} else {
					err = validateImageDetail(part)
				}
			} else {
				err = validateImage(part)
			}
			if err != nil {
				return &ContentValidationError{Path: path, ContentType: "input_image", message: err.Error()}
			}
		case "encrypted_content":
			return &ContentValidationError{Path: path, ContentType: "encrypted_content", message: "basispoints cannot forward encrypted_content message parts; refresh the model catalog and start a new conversation without a multi-agent v2 override, or resend the original plaintext"}
		default:
			return &ContentValidationError{Path: path, ContentType: contentTypeDiagnostic(part), message: "basispoints supports text and HTTPS input_image content only"}
		}
	}
	return nil
}

// Report only fixed protocol labels. A caller-controlled type can itself contain
// private data or log injection, so unrecognized values are never echoed.
func contentTypeDiagnostic(part object) string {
	if part == nil {
		return "non_object"
	}
	value, present := part["type"]
	if !present {
		return "missing"
	}
	kind, ok := value.(string)
	if !ok {
		return "non_string"
	}
	switch kind {
	case "image", "image_url", "input_file", "file", "document",
		"input_audio", "output_audio", "audio", "reasoning_text", "summary_text",
		"tool_use", "tool_result", "thinking", "redacted_thinking":
		return kind
	default:
		return "unknown"
	}
}
