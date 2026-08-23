package service

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"strconv"
	"strings"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// openAIWSStrictWireNormalizer keeps output-item fields stable across the
// lifecycle events emitted by Responses-compatible upstreams. Some upstreams
// send a complete output_item.done item but a reduced response.completed item.
// Strict clients deserialize both forms and require the fields to agree.
type openAIWSStrictWireNormalizer struct {
	itemIDs   map[int]string
	itemTypes map[int]string
	snapshots map[int]json.RawMessage
}

func newOpenAIWSStrictWireNormalizer() *openAIWSStrictWireNormalizer {
	return &openAIWSStrictWireNormalizer{
		itemIDs:   make(map[int]string),
		itemTypes: make(map[int]string),
		snapshots: make(map[int]json.RawMessage),
	}
}

// normalize repairs only known Responses output item shapes. Existing values
// and unknown fields remain untouched, so this is safe for vendor extensions.
func (n *openAIWSStrictWireNormalizer) normalize(data []byte) ([]byte, bool) {
	if n == nil || len(data) == 0 || !gjson.ValidBytes(data) {
		return data, false
	}

	eventType := strings.TrimSpace(gjson.GetBytes(data, "type").String())
	switch eventType {
	case "response.output_item.added", "response.output_item.done":
		status := "in_progress"
		if eventType == "response.output_item.done" {
			status = "completed"
		}
		index := int(gjson.GetBytes(data, "output_index").Int())
		updated, changed := n.normalizeItem(data, "item", status, index, eventType)
		if item := gjson.GetBytes(updated, "item"); item.IsObject() {
			n.rememberSnapshot(index, item.Raw)
		}
		return updated, changed

	case "response.content_part.added", "response.content_part.done":
		return normalizeOpenAIWSOutputTextPart(data, "part")

	case "response.created", "response.in_progress", "response.completed", "response.done",
		"response.failed", "response.incomplete", "response.cancelled", "response.canceled":
		responseStatus := strings.TrimSpace(gjson.GetBytes(data, "response.status").String())
		status := openAIWSStrictItemStatusForResponseStatus(responseStatus)
		if responseStatus == "" && (eventType == "response.completed" || eventType == "response.done") {
			status = "completed"
		}
		return n.normalizeOutputList(data, "response.output", status, eventType)
	default:
		if gjson.GetBytes(data, "object").String() == "response" || gjson.GetBytes(data, "output").IsArray() {
			responseStatus := strings.TrimSpace(gjson.GetBytes(data, "status").String())
			status := openAIWSStrictItemStatusForResponseStatus(responseStatus)
			if responseStatus == "" {
				status = "completed"
			}
			return n.normalizeOutputList(data, "output", status, "response")
		}
		return data, false
	}
}

func (n *openAIWSStrictWireNormalizer) normalizeOutputList(data []byte, path, status, eventType string) ([]byte, bool) {
	output := gjson.GetBytes(data, path)
	if !output.IsArray() {
		return data, false
	}
	updated := data
	changed := false
	for index := range output.Array() {
		itemPath := path + "." + strconv.Itoa(index)
		next, itemChanged := n.normalizeItem(updated, itemPath, status, index, eventType)
		if itemChanged {
			updated = next
			changed = true
		}
	}
	return updated, changed
}

func (n *openAIWSStrictWireNormalizer) normalizeItem(
	data []byte,
	path string,
	defaultStatus string,
	outputIndex int,
	eventType string,
) ([]byte, bool) {
	item := gjson.GetBytes(data, path)
	if !item.IsObject() {
		return data, false
	}

	updated := data
	changed := false
	itemType := strings.TrimSpace(item.Get("type").String())
	// A terminal rebuild may drop or reorder items, so the same array position
	// can hold a different item than it did during the lifecycle events. Only
	// inherit remembered state when the type still agrees, otherwise fields
	// from one item type leak onto another (a reasoning summary landing on a
	// message, for example).
	inheritsRememberedState := n.rememberedTypeMatches(outputIndex, itemType)
	if snapshot := n.snapshots[outputIndex]; inheritsRememberedState && len(snapshot) > 0 {
		var snapshotObject map[string]json.RawMessage
		if json.Unmarshal(snapshot, &snapshotObject) == nil {
			updated, changed = mergeOpenAIWSItemSnapshot(updated, path, snapshotObject)
			item = gjson.GetBytes(updated, path)
		}
	}

	prefix, strict := openAIWSStrictItemIDPrefix(itemType)
	if !strict {
		return updated, changed
	}

	itemID := strings.TrimSpace(item.Get("id").String())
	if itemID == "" {
		if inheritsRememberedState {
			itemID = n.itemIDs[outputIndex]
		}
		if itemID == "" {
			itemID = generateOpenAIWSStrictItemID(prefix)
		}
		next, err := sjson.SetBytes(updated, path+".id", itemID)
		if err == nil {
			updated = next
			changed = true
		}
	} else {
		n.itemIDs[outputIndex] = itemID
		n.itemTypes[outputIndex] = itemType
	}

	item = gjson.GetBytes(updated, path)
	if (item.Get("status").Type == gjson.Null || !item.Get("status").Exists()) && defaultStatus != "" {
		next, err := sjson.SetBytes(updated, path+".status", defaultStatus)
		if err == nil {
			updated = next
			changed = true
		}
	}

	if itemType == "message" {
		contentPath := path + ".content"
		content := gjson.GetBytes(updated, contentPath)
		if !content.IsArray() {
			next, err := sjson.SetRawBytes(updated, contentPath, []byte("[]"))
			if err == nil {
				updated = next
				changed = true
			}
		} else {
			for index, part := range content.Array() {
				partType := strings.TrimSpace(part.Get("type").String())
				if partType == "" || partType == "text" || partType == "output_text" {
					partPath := contentPath + "." + strconv.Itoa(index)
					next, partChanged := normalizeOpenAIWSOutputTextPart(updated, partPath)
					if partChanged {
						updated = next
						changed = true
					}
				}
			}
		}
	}

	if eventType == "response.output_item.added" || eventType == "response.output_item.done" {
		if item := gjson.GetBytes(updated, path); item.IsObject() {
			n.rememberSnapshot(outputIndex, item.Raw)
		}
	}
	return updated, changed
}

func mergeOpenAIWSItemSnapshot(
	data []byte,
	path string,
	snapshot map[string]json.RawMessage,
) ([]byte, bool) {
	updated := data
	changed := false
	for _, field := range []string{
		"id", "phase", "role", "call_id", "name", "arguments", "namespace", "input",
		"encrypted_content", "summary",
	} {
		raw, ok := snapshot[field]
		if !ok || len(raw) == 0 {
			continue
		}
		current := gjson.GetBytes(updated, path+"."+field)
		if current.Exists() && current.Type != gjson.Null {
			continue
		}
		next, err := sjson.SetRawBytes(updated, path+"."+field, raw)
		if err == nil {
			updated = next
			changed = true
		}
	}
	// The status reported by output_item.added/done outranks anything derived
	// from the response-level status: an item that already reached a terminal
	// state keeps it even when the response as a whole later fails.
	if raw, ok := snapshot["status"]; ok && len(raw) > 0 {
		current := gjson.GetBytes(updated, path+".status")
		if !current.Exists() || current.Type == gjson.Null {
			next, err := sjson.SetRawBytes(updated, path+".status", raw)
			if err == nil {
				updated = next
				changed = true
			}
		}
	}

	contentRaw, ok := snapshot["content"]
	if !ok || len(contentRaw) == 0 {
		return updated, changed
	}
	currentContent := gjson.GetBytes(updated, path+".content")
	if !currentContent.IsArray() {
		next, err := sjson.SetRawBytes(updated, path+".content", contentRaw)
		if err == nil {
			return next, true
		}
		return updated, changed
	}
	var snapshotParts []json.RawMessage
	if json.Unmarshal(contentRaw, &snapshotParts) != nil {
		return updated, changed
	}
	for index, partRaw := range snapshotParts {
		if index >= len(currentContent.Array()) {
			break
		}
		var snapshotPart map[string]json.RawMessage
		if json.Unmarshal(partRaw, &snapshotPart) != nil {
			continue
		}
		partPath := path + ".content." + strconv.Itoa(index)
		for _, field := range []string{"type", "text", "annotations", "logprobs"} {
			raw, exists := snapshotPart[field]
			if !exists || len(raw) == 0 {
				continue
			}
			current := gjson.GetBytes(updated, partPath+"."+field)
			if current.Exists() && current.Type != gjson.Null {
				continue
			}
			next, err := sjson.SetRawBytes(updated, partPath+"."+field, raw)
			if err == nil {
				updated = next
				changed = true
			}
		}
	}
	return updated, changed
}

func normalizeOpenAIWSOutputTextPart(data []byte, path string) ([]byte, bool) {
	part := gjson.GetBytes(data, path)
	if !part.IsObject() {
		return data, false
	}
	partType := strings.TrimSpace(part.Get("type").String())
	if partType != "" && partType != "text" && partType != "output_text" {
		return data, false
	}
	updated := data
	changed := false
	if partType == "" || partType == "text" {
		next, err := sjson.SetBytes(updated, path+".type", "output_text")
		if err == nil {
			updated = next
			changed = true
		}
	}
	part = gjson.GetBytes(updated, path)
	if part.Get("text").Type != gjson.String {
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

func (n *openAIWSStrictWireNormalizer) rememberSnapshot(index int, raw string) {
	if n == nil || raw == "" {
		return
	}
	n.snapshots[index] = json.RawMessage(append([]byte(nil), raw...))
	if itemType := strings.TrimSpace(gjson.Get(raw, "type").String()); itemType != "" {
		n.itemTypes[index] = itemType
	}
}

// rememberedTypeMatches reports whether the state remembered for outputIndex
// describes an item of the same type. An unknown or disagreeing type means the
// remembered state belongs to a different item and must not be applied.
func (n *openAIWSStrictWireNormalizer) rememberedTypeMatches(outputIndex int, itemType string) bool {
	if n == nil || itemType == "" {
		return false
	}
	remembered := strings.TrimSpace(n.itemTypes[outputIndex])
	return remembered != "" && remembered == itemType
}

// openAIWSStrictItemStatusForResponseStatus maps a response-level status onto
// a status the Responses protocol allows on an output item. Response terminal
// states such as failed or cancelled have no item-level counterpart, so items
// that never reported one of their own are reported as incomplete. An empty
// result means "say nothing", which leaves the item status to the lifecycle
// snapshot instead of inventing a value strict clients would reject.
func openAIWSStrictItemStatusForResponseStatus(responseStatus string) string {
	switch strings.TrimSpace(responseStatus) {
	case "completed":
		return "completed"
	case "in_progress", "queued":
		return "in_progress"
	case "incomplete", "failed", "cancelled", "canceled":
		return "incomplete"
	default:
		return ""
	}
}

func openAIWSStrictItemIDPrefix(itemType string) (string, bool) {
	switch strings.TrimSpace(itemType) {
	case "message":
		return "msg", true
	case "reasoning":
		return "rs", true
	case "function_call":
		return "fc", true
	case "custom_tool_call":
		return "ctc", true
	case "tool_search_call":
		return "tsc", true
	default:
		return "", false
	}
}

func generateOpenAIWSStrictItemID(prefix string) string {
	buf := make([]byte, 12)
	_, _ = rand.Read(buf)
	return prefix + "_" + hex.EncodeToString(buf)
}
