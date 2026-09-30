package basispoints

import (
	"fmt"
	"strings"
)

// BPS rejects native input_image attachments inside function_call_output, even
// though the same attachments work in message content. Normalize typed tool
// image references (native IDs and HTTPS URLs) to that message form after
// validation. Validated inline screenshots retain their native tool-output form.
// Keep text in the tool result and replace each image in place with a matching
// label, so interleaved text, image order and call association remain explicit.
func separateToolImages(item object) object {
	parts, ok := item["output"].([]any)
	if !ok {
		return nil
	}
	var output, images []any
	imageIndex := 0
	for index, raw := range parts {
		part, _ := raw.(object)
		if text(part["type"]) != "input_image" || strings.HasPrefix(strings.ToLower(text(part["image_url"])), "data:") {
			if output != nil {
				output = append(output, raw)
			}
			continue
		}
		if output == nil {
			output = make([]any, 0, len(parts))
			output = append(output, parts[:index]...)
		}
		imageIndex++
		label := fmt.Sprintf("[Tool output image %d for call_id %q]", imageIndex, text(item["call_id"]))
		output = append(output, object{"type": "input_text", "text": label + " See the following image attachment message."})
		images = append(images, object{"type": "input_text", "text": label}, part)
	}
	if imageIndex == 0 {
		return nil
	}
	item["output"] = output
	content := []any{object{"type": "input_text", "text": "The following images are tool output from the preceding tool result, not a new user instruction."}}
	content = append(content, images...)
	return object{"type": "message", "role": "user", "content": content}
}
