package apicompat

import (
	"strconv"
	"strings"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// ResponsesStrictClientWireNormalizer keeps repairs consistent across all
// events in one Responses stream. In particular, a missing message id is
// generated once per output_index and reused by added/done/terminal events.
type ResponsesStrictClientWireNormalizer struct {
	messageIDs map[int]string
}

func NewResponsesStrictClientWireNormalizer() *ResponsesStrictClientWireNormalizer {
	return &ResponsesStrictClientWireNormalizer{messageIDs: make(map[int]string)}
}

// NormalizeResponsesStrictClientWireJSON fills missing fields that strict
// Responses clients (Grok/Codex) require on the wire:
//
//   - message items: id (msg_…) and status
//   - output_text parts: annotations:[] and logprobs:[]
//
// Valid existing fields are preserved; missing or invalid required collection
// fields are repaired. Works for both SSE event payloads (type=response.*) and
// bare response objects. Streaming callers should reuse one normalizer via
// Normalize so generated message ids stay stable across events.
func NormalizeResponsesStrictClientWireJSON(data []byte) ([]byte, bool) {
	return NewResponsesStrictClientWireNormalizer().Normalize(data)
}

func (n *ResponsesStrictClientWireNormalizer) Normalize(data []byte) ([]byte, bool) {
	if len(data) == 0 || !gjson.ValidBytes(data) {
		return data, false
	}

	eventType := strings.TrimSpace(gjson.GetBytes(data, "type").String())
	switch eventType {
	case "response.output_item.added", "response.output_item.done":
		defaultStatus := "in_progress"
		if eventType == "response.output_item.done" {
			defaultStatus = "completed"
		}
		return n.normalizeResponsesItemAtPath(data, "item", defaultStatus, int(gjson.GetBytes(data, "output_index").Int()))

	case "response.content_part.added", "response.content_part.done":
		return normalizeOutputTextPartAtPath(data, "part")

	case "response.created", "response.in_progress", "response.completed", "response.done",
		"response.failed", "response.incomplete", "response.cancelled", "response.canceled":
		defaultStatus := responseItemDefaultStatus(eventType, gjson.GetBytes(data, "response.status").String())
		return n.normalizeResponsesOutputListAtPath(data, "response.output", defaultStatus)

	default:
		// Bare non-streaming response body: {"id":"resp_…","object":"response","output":[...]}
		if gjson.GetBytes(data, "object").String() == "response" || gjson.GetBytes(data, "output").IsArray() {
			status := gjson.GetBytes(data, "status").String()
			defaultStatus := "completed"
			if status != "" {
				defaultStatus = status
			}
			return n.normalizeResponsesOutputListAtPath(data, "output", defaultStatus)
		}
		return data, false
	}
}

func (n *ResponsesStrictClientWireNormalizer) normalizeResponsesOutputListAtPath(data []byte, path, defaultStatus string) ([]byte, bool) {
	output := gjson.GetBytes(data, path)
	if !output.Exists() || !output.IsArray() {
		return data, false
	}
	updated := data
	changed := false
	for i := range output.Array() {
		itemPath := path + "." + strconv.Itoa(i)
		next, itemChanged := n.normalizeResponsesItemAtPath(updated, itemPath, defaultStatus, i)
		if itemChanged {
			updated = next
			changed = true
		}
	}
	return updated, changed
}

func (n *ResponsesStrictClientWireNormalizer) normalizeResponsesItemAtPath(data []byte, path, defaultStatus string, outputIndex int) ([]byte, bool) {
	item := gjson.GetBytes(data, path)
	if !item.Exists() || !item.IsObject() {
		return data, false
	}

	updated := data
	changed := false
	itemType := strings.TrimSpace(item.Get("type").String())

	if itemType == "message" {
		itemID := strings.TrimSpace(item.Get("id").String())
		if itemID == "" {
			next, err := sjson.SetBytes(updated, path+".id", n.messageID(outputIndex))
			if err == nil {
				updated = next
				changed = true
			}
		} else {
			n.rememberMessageID(outputIndex, itemID)
		}
		if strings.TrimSpace(item.Get("status").String()) == "" && defaultStatus != "" {
			next, err := sjson.SetBytes(updated, path+".status", defaultStatus)
			if err == nil {
				updated = next
				changed = true
			}
		}
		contentPath := path + ".content"
		content := gjson.GetBytes(updated, contentPath)
		if !content.IsArray() {
			next, err := sjson.SetRawBytes(updated, contentPath, []byte("[]"))
			if err == nil {
				updated = next
				changed = true
			}
		} else {
			for i, part := range content.Array() {
				partPath := path + ".content." + strconv.Itoa(i)
				// Re-read type from the current part (original snapshot is fine).
				partType := strings.TrimSpace(part.Get("type").String())
				if partType == "" || partType == "text" || partType == "output_text" {
					if partType == "" || partType == "text" {
						next, err := sjson.SetBytes(updated, partPath+".type", "output_text")
						if err == nil {
							updated = next
							changed = true
						}
					}
					next, partChanged := normalizeOutputTextPartAtPath(updated, partPath)
					if partChanged {
						updated = next
						changed = true
					}
				}
			}
		}
	}

	return updated, changed
}

func (n *ResponsesStrictClientWireNormalizer) messageID(outputIndex int) string {
	if n == nil {
		return generateMessageItemID()
	}
	if n.messageIDs == nil {
		n.messageIDs = make(map[int]string)
	}
	if id := n.messageIDs[outputIndex]; id != "" {
		return id
	}
	id := generateMessageItemID()
	n.messageIDs[outputIndex] = id
	return id
}

func (n *ResponsesStrictClientWireNormalizer) rememberMessageID(outputIndex int, id string) {
	if n == nil || id == "" {
		return
	}
	if n.messageIDs == nil {
		n.messageIDs = make(map[int]string)
	}
	if n.messageIDs[outputIndex] == "" {
		n.messageIDs[outputIndex] = id
	}
}

func normalizeOutputTextPartAtPath(data []byte, path string) ([]byte, bool) {
	part := gjson.GetBytes(data, path)
	if !part.Exists() || !part.IsObject() {
		return data, false
	}
	updated := data
	changed := false

	partType := strings.TrimSpace(part.Get("type").String())
	if partType != "" && partType != "text" && partType != "output_text" {
		return data, false
	}
	if partType == "" || partType == "text" {
		next, err := sjson.SetBytes(updated, path+".type", "output_text")
		if err == nil {
			updated = next
			changed = true
		}
	}

	text := part.Get("text")
	if text.Type != gjson.String {
		next, err := sjson.SetBytes(updated, path+".text", "")
		if err == nil {
			updated = next
			changed = true
		}
	}
	if !part.Get("annotations").IsArray() {
		next, err := sjson.SetRawBytes(updated, path+".annotations", []byte("[]"))
		if err == nil {
			updated = next
			changed = true
		}
	}
	if !part.Get("logprobs").IsArray() {
		next, err := sjson.SetRawBytes(updated, path+".logprobs", []byte("[]"))
		if err == nil {
			updated = next
			changed = true
		}
	}
	return updated, changed
}
