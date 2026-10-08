package apicompat

import (
	"bytes"
	"encoding/json"
	"strings"
)

type chatJSONField struct {
	name  string
	value json.RawMessage
}

func chatJSONFields(raw json.RawMessage) []chatJSONField {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	opening, err := decoder.Token()
	if err != nil || opening != json.Delim('{') {
		return nil
	}
	var fields []chatJSONField
	for decoder.More() {
		key, err := decoder.Token()
		if err != nil {
			return nil
		}
		var value json.RawMessage
		if decoder.Decode(&value) != nil {
			return nil
		}
		name, ok := key.(string)
		if !ok {
			return nil
		}
		fields = append(fields, chatJSONField{name: name, value: value})
	}
	return fields
}

func chatDuplicateFields(fields []chatJSONField, names ...string) bool {
	seen := make(map[string]bool)
	for _, field := range fields {
		for _, name := range names {
			if strings.EqualFold(field.name, name) {
				if seen[name] {
					return true
				}
				seen[name] = true
			}
		}
	}
	return false
}

func chatContentAudioStatus(raw json.RawMessage) (bool, bool) {
	var parts []json.RawMessage
	if json.Unmarshal(raw, &parts) != nil {
		return false, false
	}
	hasAudio, duplicate := false, false
	for _, part := range parts {
		fields := chatJSONFields(part)
		duplicate = duplicate || chatDuplicateFields(fields, "type", "text", "image_url", "file", "input_audio", "prompt_cache_breakpoint")
		for _, field := range fields {
			switch {
			case strings.EqualFold(field.name, "type"):
				var partType string
				if json.Unmarshal(field.value, &partType) == nil && partType == "input_audio" {
					hasAudio = true
				}
			case strings.EqualFold(field.name, "input_audio"):
				duplicate = duplicate || chatDuplicateFields(chatJSONFields(field.value), "data", "format")
			case strings.EqualFold(field.name, "image_url"):
				duplicate = duplicate || chatDuplicateFields(chatJSONFields(field.value), "url", "detail")
			case strings.EqualFold(field.name, "file"):
				duplicate = duplicate || chatDuplicateFields(chatJSONFields(field.value), "file_data", "file_id", "filename")
			}
		}
	}
	return hasAudio, hasAudio && duplicate
}

func chatRequestHasAmbiguousAudio(raw []byte) bool {
	fields := chatJSONFields(raw)
	hasAudio, ambiguous := false, false
	for _, field := range fields {
		if !strings.EqualFold(field.name, "messages") {
			continue
		}
		var messages []json.RawMessage
		if json.Unmarshal(field.value, &messages) != nil {
			continue
		}
		for _, message := range messages {
			messageFields := chatJSONFields(message)
			messageAudio := false
			for _, messageField := range messageFields {
				if strings.EqualFold(messageField.name, "content") {
					contentAudio, contentAmbiguous := chatContentAudioStatus(messageField.value)
					messageAudio = messageAudio || contentAudio
					ambiguous = ambiguous || contentAmbiguous
				}
			}
			hasAudio = hasAudio || messageAudio
			ambiguous = ambiguous || (messageAudio && chatDuplicateFields(messageFields, "content", "role"))
		}
	}
	return ambiguous || (hasAudio && chatDuplicateFields(fields, "messages"))
}

func (request *ChatCompletionsRequest) UnmarshalJSON(data []byte) error {
	type plainChatRequest ChatCompletionsRequest
	if err := json.Unmarshal(data, (*plainChatRequest)(request)); err != nil {
		return err
	}
	request.ambiguousInputAudio = chatRequestHasAmbiguousAudio(data)
	return nil
}
