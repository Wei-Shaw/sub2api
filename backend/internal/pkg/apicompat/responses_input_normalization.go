package apicompat

import (
	"bytes"
	"encoding/json"
	"strings"
)

func jsonStringRawMessage(s string) json.RawMessage {
	raw, _ := json.Marshal(s)
	return raw
}

func responsesToolArgumentsForAnthropic(raw json.RawMessage) json.RawMessage {
	raw = bytesTrimSpace(raw)
	if len(raw) == 0 || string(raw) == "null" {
		return json.RawMessage("{}")
	}
	if rawIsJSONObject(raw) {
		return compactRawJSON(raw)
	}

	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		return json.RawMessage("{}")
	}
	s = strings.TrimSpace(s)
	if s == "" {
		return json.RawMessage("{}")
	}
	raw = json.RawMessage(s)
	if !rawIsJSONObject(raw) {
		return json.RawMessage("{}")
	}
	return compactRawJSON(raw)
}

func responsesToolArgumentsForChat(raw json.RawMessage) string {
	raw = bytesTrimSpace(raw)
	if len(raw) == 0 || string(raw) == "null" {
		return "{}"
	}
	if rawIsJSONObject(raw) {
		return string(compactRawJSON(raw))
	}

	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		return "{}"
	}
	s = strings.TrimSpace(s)
	if s == "" {
		return "{}"
	}
	return s
}

func responsesToolOutputText(raw json.RawMessage) string {
	raw = bytesTrimSpace(raw)
	if len(raw) == 0 || string(raw) == "null" {
		return ""
	}

	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}

	var parts []ResponsesContentPart
	if err := json.Unmarshal(raw, &parts); err != nil {
		return ""
	}

	var texts []string
	for _, part := range parts {
		if (part.Type == "output_text" || part.Type == "text") && part.Text != "" {
			texts = append(texts, part.Text)
		}
	}
	return strings.Join(texts, "\n\n")
}

func rawIsJSONObject(raw json.RawMessage) bool {
	raw = bytesTrimSpace(raw)
	if len(raw) == 0 || raw[0] != '{' {
		return false
	}
	var obj map[string]json.RawMessage
	return json.Unmarshal(raw, &obj) == nil
}

func compactRawJSON(raw json.RawMessage) json.RawMessage {
	var buf bytes.Buffer
	if err := json.Compact(&buf, raw); err != nil {
		return raw
	}
	return json.RawMessage(buf.Bytes())
}
