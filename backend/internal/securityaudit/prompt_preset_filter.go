package securityaudit

import (
	"encoding/json"
	"strings"
)

type agentPresetRule struct {
	identity string
	prefixes []string
}

var agentPresetRules = []agentPresetRule{
	{identity: "You are Codex", prefixes: []string{"# AGENTS.md instructions", "<environment_context>"}},
	{identity: "You are Claude Code", prefixes: []string{"<system-reminder>"}},
}

type promptRecordFilterOptions struct {
	Enabled     bool
	AgentPreset bool
	Skills      bool
}

func sanitizePromptRecordBody(protocol string, body []byte, options promptRecordFilterOptions) []byte {
	document, err := decodePromptDocument(body)
	if err != nil {
		return body
	}
	if _, isObject := document.(map[string]any); !isObject {
		if text, isText := document.(string); isText && looksLikeInlineMedia(text) {
			return []byte(`{}`)
		}
		return body
	}
	result, err := json.Marshal(sanitizePromptRecordDocument(protocol, document, options))
	if err != nil {
		return body
	}
	return result
}

func filterPresetRequestBody(protocol string, body []byte, options promptRecordFilterOptions) []byte {
	if !options.Enabled {
		return body
	}
	document, err := decodePromptDocument(body)
	if err != nil {
		return body
	}
	if _, isObject := document.(map[string]any); !isObject {
		return body
	}
	result, err := json.Marshal(filterPresetDocument(protocol, document, options))
	if err != nil {
		return body
	}
	return result
}

func interactionRole(role string) bool {
	return role == "user" || role == "assistant" || role == "model"
}

func textContentType(kind string) bool {
	return kind == "text" || kind == "input_text" || kind == "output_text"
}

func mediaContentType(kind string) bool {
	switch kind {
	case "image", "image_url", "input_image", "output_image", "audio", "audio_url", "input_audio", "output_audio",
		"video", "video_url", "input_video", "output_video", "file", "input_file", "output_file", "document":
		return true
	default:
		return false
	}
}

func nonInteractionType(kind string) bool {
	if kind == "" {
		return false
	}
	switch kind {
	case "additional_tools", "reasoning", "item_reference", "computer_initialize_state", "computer_screenshot":
		return true
	}
	return strings.Contains(kind, "tool") || strings.HasSuffix(kind, "_call") || strings.HasSuffix(kind, "_call_output")
}

var mediaInteractionRootKeys = []string{
	"prompt", "input_prompt", "text_prompt", "description", "query", "lyrics",
	"negative_prompt", "positive_prompt", "gpt_description_prompt", "prompt_en",
	"final_prompt", "final_zh_prompt", "orig_prompt", "actual_prompt", "image_prompt",
	"input", "request", "image", "images",
}

func isMediaProtocol(protocol string) bool {
	switch strings.ToLower(strings.TrimSpace(protocol)) {
	case "openai_images", "grok_media", "media", "images":
		return true
	default:
		return false
	}
}

func rawString(raw json.RawMessage) (string, bool) {
	var value string
	if len(raw) == 0 || json.Unmarshal(raw, &value) != nil {
		return "", false
	}
	return value, true
}

func isMultimodalPayloadKey(key string) bool {
	normalized := strings.NewReplacer("-", "_", " ", "_").Replace(strings.ToLower(strings.TrimSpace(key)))
	switch normalized {
	case "image", "images", "image_url", "image_urls", "image_data", "input_image", "input_images", "output_image", "output_images",
		"reference_image", "reference_images", "source_image", "source_images", "mask", "mask_image", "mask_image_url", "input_image_mask",
		"audio", "audios", "audio_url", "audio_urls", "audio_data", "input_audio", "input_audios", "output_audio", "output_audios",
		"video", "videos", "video_url", "video_urls", "video_data", "input_video", "input_videos", "output_video", "output_videos",
		"file", "files", "file_url", "file_urls", "file_data", "input_file", "input_files", "output_file", "output_files",
		"document", "documents", "inline_data", "inlinedata", "filedata", "blob", "binary", "bytes", "attachments":
		return true
	default:
		return false
	}
}

func looksLikeInlineMedia(value string) bool {
	lower := strings.ToLower(strings.TrimSpace(value))
	if strings.HasPrefix(lower, "data:image/") || strings.HasPrefix(lower, "data:audio/") || strings.HasPrefix(lower, "data:video/") {
		return true
	}
	if !strings.HasPrefix(lower, "data:") {
		return false
	}
	metadata := lower
	if comma := strings.IndexByte(metadata, ','); comma >= 0 {
		metadata = metadata[:comma]
	}
	return strings.Contains(metadata, ";base64") || strings.HasPrefix(metadata, "data:application/")
}

func extractAgentPresetBlocks(text string, rules []agentPresetRule) []string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	result := make([]string, 0, 1)
	for _, rule := range rules {
		for _, prefix := range rule.prefixes {
			closing := "</environment_context>"
			if prefix == "<system-reminder>" {
				closing = "</system-reminder>"
			}
			if strings.HasPrefix(prefix, "# AGENTS.md") {
				closing = "</INSTRUCTIONS>"
			}
			remaining := text
			for {
				start := strings.Index(remaining, prefix)
				if start < 0 {
					break
				}
				afterPrefix := remaining[start+len(prefix):]
				if strings.HasPrefix(prefix, "# AGENTS.md") {
					var valid bool
					afterPrefix, valid = afterAgentPresetHeading(afterPrefix)
					if !valid {
						remaining = remaining[start+len(prefix):]
						continue
					}
				}
				if strings.HasPrefix(prefix, "# AGENTS.md") && !strings.HasPrefix(strings.TrimSpace(afterPrefix), "<INSTRUCTIONS>") {
					remaining = afterPrefix
					continue
				}
				end := strings.Index(afterPrefix, closing)
				if end < 0 {
					break
				}
				end = len(remaining) - len(afterPrefix) + end + len(closing)
				result = append(result, strings.TrimSpace(remaining[start:end]))
				remaining = remaining[end:]
			}
		}
	}
	return result
}

func extractTaggedBlocks(text, opening, closing string) []string {
	result := make([]string, 0, 1)
	remaining := text
	for {
		start := strings.Index(remaining, opening)
		if start < 0 {
			return result
		}
		end := strings.Index(remaining[start+len(opening):], closing)
		if end < 0 {
			return result
		}
		end += start + len(opening) + len(closing)
		result = append(result, remaining[start:end])
		remaining = remaining[end:]
	}
}

func stripAgentPresetPrefix(text string, rules []agentPresetRule) string {
	remaining := text
	for {
		trimmed := strings.TrimSpace(strings.ReplaceAll(remaining, "\r\n", "\n"))
		end := -1
		for _, rule := range rules {
			for _, prefix := range rule.prefixes {
				if !strings.HasPrefix(trimmed, prefix) {
					continue
				}
				closing := "</environment_context>"
				if prefix == "<system-reminder>" {
					closing = "</system-reminder>"
				}
				if strings.HasPrefix(prefix, "# AGENTS.md") {
					afterHeading, valid := afterAgentPresetHeading(strings.TrimPrefix(trimmed, prefix))
					if !valid || !strings.HasPrefix(strings.TrimSpace(afterHeading), "<INSTRUCTIONS>") {
						continue
					}
					closing = "</INSTRUCTIONS>"
				}
				if index := strings.Index(trimmed, closing); index >= 0 {
					end = index + len(closing)
				}
			}
		}
		if end < 0 {
			return remaining
		}
		remaining = strings.TrimSpace(trimmed[end:])
	}
}

// Only accept the actual Codex heading grammar, followed by an instruction
// block. Quoted headings and similarly named prose must remain user content.
func afterAgentPresetHeading(suffix string) (string, bool) {
	heading, rest, found := strings.Cut(suffix, "\n")
	if !found {
		return "", false
	}
	heading = strings.TrimRight(heading, "\r")
	if heading != "" && (!strings.HasPrefix(heading, " for ") || strings.TrimSpace(strings.TrimPrefix(heading, " for ")) == "") {
		return "", false
	}
	return rest, true
}
